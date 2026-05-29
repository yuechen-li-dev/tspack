package pack

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/graph"
)

type Options struct {
	OutputDir string
	DryRun    bool
	Verify    bool
}
type Result struct {
	Diagnostics []diag.Diagnostic
	Artifacts   []Artifact
	Preview     []File
	Plans       []Plan
}
type Artifact struct {
	PackageName, Version, Path, Hash string
	Size                             int64
	Verified                         bool
}
type File struct {
	PackageName, SourcePath, ArchivePath string
	Size                                 int64
	Reason                               string
}

type Plan struct {
	PackageName   string
	Version       string
	Path          string
	License       string
	files         map[string]matchedFile
	keys          []string
	rootTarget    *plannedTarget
	targets       []plannedTarget
	peerDeps      map[string]string
	optionalPeers map[string]bool
}

type plannedTarget struct {
	Export  string
	Runtime string
	Types   string
}

type matchedFile struct {
	source, archive, reason string
	mode                    int64
	size                    int64
	data                    []byte
}

func Pack(root string, pkg *graph.PackageNode, opts Options) Result {
	planResult := PlanPackage(root, pkg, opts)
	if hasErrors(planResult.Diagnostics) || opts.DryRun {
		return planResult
	}

	writeResult := WritePlans([]Plan{planResult.Plans[0]}, opts)
	planResult.Diagnostics = append(planResult.Diagnostics, writeResult.Diagnostics...)
	planResult.Artifacts = append(planResult.Artifacts, writeResult.Artifacts...)
	diag.SortDiagnostics(planResult.Diagnostics)
	return planResult
}

func PlanPackage(root string, pkg *graph.PackageNode, opts Options) Result {
	var out Result
	if strings.TrimSpace(pkg.Version) == "" {
		out.Diagnostics = append(out.Diagnostics, dErr("TSPACK_PACK_MISSING_PUBLISH_POLICY", "package version is required", "package="+pkg.Name))
		return out
	}
	if len(pkg.Publish.Include) == 0 {
		out.Diagnostics = append(out.Diagnostics, dErr("TSPACK_PACK_MISSING_PUBLISH_POLICY", "publish.include is required", "package="+pkg.Name))
		return out
	}
	pkgRoot := filepath.Join(root, pkg.Root)
	files := map[string]matchedFile{}
	realFileCount := 0
	for _, inc := range pkg.Publish.Include {
		if badPath(inc) {
			out.Diagnostics = append(out.Diagnostics, dErr("TSPACK_PACK_INVALID_PUBLISH_PATH", "invalid publish include path", "package="+pkg.Name, "pattern="+inc))
			continue
		}
		matches, diags := resolvePattern(pkgRoot, inc)
		out.Diagnostics = append(out.Diagnostics, diags...)
		if len(matches) == 0 {
			out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{
				Code:     "TSPACK_PACK_INCLUDE_MATCHED_NOTHING",
				Severity: diag.SeverityError,
				Message:  "include matched nothing",
				Details: []string{
					"package=" + pkg.Name,
					"pattern=" + inc,
					"packageRoot=" + pkgRoot,
					"build outputs first or remove/update the include pattern",
				},
			})
		}
		for _, m := range matches {
			realFileCount++
			files[m.archive] = m
		}
	}
	for _, exc := range pkg.Publish.Exclude {
		if badPath(exc) {
			out.Diagnostics = append(out.Diagnostics, dErr("TSPACK_PACK_INVALID_PUBLISH_PATH", "invalid publish exclude path", "package="+pkg.Name, "pattern="+exc))
			continue
		}
		em, _ := resolvePattern(pkgRoot, exc)
		for _, m := range em {
			delete(files, m.archive)
		}
	}
	out.Diagnostics = append(out.Diagnostics, changelogNotIncludedDiagnostics(pkgRoot, pkg, files)...)
	for _, t := range pkg.Targets {
		if _, err := os.Stat(filepath.Join(pkgRoot, t.Runtime)); err != nil {
			out.Diagnostics = append(out.Diagnostics, dErr("TSPACK_PACK_MISSING_RUNTIME_OUTPUT", "missing runtime output", "package="+pkg.Name, t.Runtime))
		}
		if _, err := os.Stat(filepath.Join(pkgRoot, t.Types)); err != nil {
			out.Diagnostics = append(out.Diagnostics, dErr("TSPACK_PACK_MISSING_TYPE_OUTPUT", "missing type output", "package="+pkg.Name, t.Types))
		}
		out.Diagnostics = append(out.Diagnostics, unpublishablePeerDiagnostics(pkg, t)...)
	}
	if hasErrors(out.Diagnostics) {
		diag.SortDiagnostics(out.Diagnostics)
		return out
	}
	if realFileCount == 0 {
		out.Diagnostics = append(out.Diagnostics, dErr("TSPACK_PACK_EMPTY_PACKAGE", "publish policy matched no files", "package="+pkg.Name))
		diag.SortDiagnostics(out.Diagnostics)
		return out
	}
	if _, ok := files["package.json"]; !ok {
		b := generatedPackageJSON(pkg)
		files["package.json"] = matchedFile{source: "<generated>", archive: "package.json", reason: "generated package.json", mode: 0o644, size: int64(len(b)), data: b}
	}
	if len(files) == 0 {
		out.Diagnostics = append(out.Diagnostics, dErr("TSPACK_PACK_EMPTY_PACKAGE", "package content is empty", "package="+pkg.Name))
		return out
	}
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		f := files[k]
		out.Preview = append(out.Preview, File{PackageName: pkg.Name, SourcePath: f.source, ArchivePath: "package/" + f.archive, Size: f.size, Reason: f.reason})
	}

	outputDir := opts.OutputDir
	if outputDir == "" {
		outputDir = filepath.Join(root, "tspack-artifacts")
	}
	name := strings.ReplaceAll(strings.TrimPrefix(pkg.Name, "@"), "/", "-") + "-" + pkg.Version + ".tgz"
	out.Plans = append(out.Plans, planForPackage(pkg, filepath.Join(outputDir, name), files, keys))
	return out
}

func changelogNotIncludedDiagnostics(pkgRoot string, pkg *graph.PackageNode, files map[string]matchedFile) []diag.Diagnostic {
	const changelogPath = "CHANGELOG.md"

	info, err := os.Stat(filepath.Join(pkgRoot, changelogPath))
	if err != nil || info.IsDir() {
		return nil
	}
	if _, ok := files[changelogPath]; ok {
		return nil
	}

	details := []string{
		"package=" + pkg.Name,
		"changelog=" + changelogPath,
		"publish include:",
	}
	for _, include := range pkg.Publish.Include {
		details = append(details, "  "+include)
	}
	details = append(details,
		"fix:",
		`  add "CHANGELOG.md" to <Publish include={[...]} />`,
		"note: TSPack does not auto-include files outside Publish.include",
	)

	return []diag.Diagnostic{{
		Code:     "TSPACK_PACK_CHANGELOG_NOT_INCLUDED",
		Severity: diag.SeverityWarning,
		Message:  "changelog file exists but is not included in publish policy",
		Details:  details,
		Fixes:    []string{`Add "CHANGELOG.md" to <Publish include={[...]} /> if this package should publish its changelog.`},
	}}
}

func planForPackage(pkg *graph.PackageNode, path string, files map[string]matchedFile, keys []string) Plan {
	peerDependencies, optionalPeers := packageJSONPeerDependencies(pkg)
	optionalPeerSet := map[string]bool{}
	for _, peerName := range optionalPeers {
		optionalPeerSet[peerName] = true
	}

	var rootTargetPlan *plannedTarget
	targets := make([]plannedTarget, 0, len(pkg.Targets))
	for _, target := range pkg.Targets {
		targetPlan := plannedTarget{
			Export:  target.Export,
			Runtime: target.Runtime,
			Types:   target.Types,
		}
		targets = append(targets, targetPlan)
		if target.Export == "." {
			copy := targetPlan
			rootTargetPlan = &copy
		}
	}

	return Plan{
		PackageName:   pkg.Name,
		Version:       pkg.Version,
		Path:          path,
		License:       pkg.License,
		files:         files,
		keys:          keys,
		rootTarget:    rootTargetPlan,
		targets:       targets,
		peerDeps:      peerDependencies,
		optionalPeers: optionalPeerSet,
	}
}

func WritePlans(plans []Plan, opts Options) Result {
	var out Result
	if opts.DryRun || len(plans) == 0 {
		return out
	}

	outputDirs := map[string]bool{}
	for _, plan := range plans {
		outputDirs[filepath.Dir(plan.Path)] = true
	}
	for outputDir := range outputDirs {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			out.Diagnostics = append(out.Diagnostics, writeFailedDiagnostic("failed to create output directory", outputDir, err))
			diag.SortDiagnostics(out.Diagnostics)
			return out
		}
	}

	tempPaths := []string{}
	finalPaths := []string{}
	pendingArtifacts := []Artifact{}
	for index, plan := range plans {
		archive, err := buildArchive(plan)
		if err != nil {
			cleanupFiles(tempPaths)
			out.Diagnostics = append(out.Diagnostics, writeFailedDiagnostic("failed to build archive", plan.Path, err))
			diag.SortDiagnostics(out.Diagnostics)
			return out
		}

		tempPath := plan.Path + ".tmp-" + deterministicTempSuffix(index)
		_ = os.Remove(tempPath)
		if err := os.WriteFile(tempPath, archive, 0o644); err != nil {
			cleanupFiles(append(tempPaths, tempPath))
			out.Diagnostics = append(out.Diagnostics, writeFailedDiagnostic("failed to write archive", tempPath, err))
			diag.SortDiagnostics(out.Diagnostics)
			return out
		}
		tempPaths = append(tempPaths, tempPath)

		h := sha256.Sum256(archive)
		pendingArtifacts = append(pendingArtifacts, Artifact{PackageName: plan.PackageName, Version: plan.Version, Path: plan.Path, Hash: "sha256:" + hex.EncodeToString(h[:]), Size: int64(len(archive)), Verified: opts.Verify})
	}

	if opts.Verify {
		for index, plan := range plans {
			tempPath := tempPaths[index]
			verifyDiagnostics := VerifyArchive(tempPath, plan)
			if hasErrors(verifyDiagnostics) {
				cleanupFiles(tempPaths)
				out.Diagnostics = append(out.Diagnostics, verifyDiagnostics...)
				diag.SortDiagnostics(out.Diagnostics)
				return out
			}
		}
	}

	for index, plan := range plans {
		tempPath := tempPaths[index]
		if err := os.Rename(tempPath, plan.Path); err != nil {
			cleanupFiles(tempPaths[index:])
			cleanupFiles(finalPaths)
			out.Diagnostics = append(out.Diagnostics, writeFailedDiagnostic("failed to move archive into place", plan.Path, err))
			diag.SortDiagnostics(out.Diagnostics)
			return out
		}
		finalPaths = append(finalPaths, plan.Path)
	}

	out.Artifacts = append(out.Artifacts, pendingArtifacts...)
	return out
}

func buildArchive(plan Plan) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	gz, err := gzip.NewWriterLevel(buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	gz.Header.ModTime = time.Unix(0, 0)
	tw := tar.NewWriter(gz)
	for _, k := range plan.keys {
		f := plan.files[k]
		b := f.data
		if b == nil {
			var err error
			b, err = os.ReadFile(f.source)
			if err != nil {
				_ = tw.Close()
				_ = gz.Close()
				return nil, err
			}
		}
		hdr := &tar.Header{Name: "package/" + k, Mode: f.mode, Size: int64(len(b)), ModTime: time.Unix(0, 0), Format: tar.FormatPAX}
		if err := tw.WriteHeader(hdr); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			return nil, err
		}
		if _, err := tw.Write(b); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func deterministicTempSuffix(index int) string {
	bytes := []byte{
		byte(index >> 24),
		byte(index >> 16),
		byte(index >> 8),
		byte(index),
	}
	return hex.EncodeToString(bytes)
}

func cleanupFiles(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

func VerifyArchive(archivePath string, plan Plan) []diag.Diagnostic {
	entries, packageJSON, readDiagnostics := readArchiveForVerification(archivePath, plan)
	if len(readDiagnostics) > 0 {
		return readDiagnostics
	}

	if packageJSON == nil {
		return []diag.Diagnostic{verifyDiagnostic("TSPACK_PACK_VERIFY_PACKAGE_JSON_MISSING", "packed package is missing package.json", plan, archivePath, "package.json", "")}
	}

	var parsed map[string]any
	if err := json.Unmarshal(packageJSON, &parsed); err != nil {
		return []diag.Diagnostic{verifyDiagnostic("TSPACK_PACK_VERIFY_PACKAGE_JSON_INVALID", "packed package.json is invalid", plan, archivePath, "package.json", err.Error())}
	}

	var diagnostics []diag.Diagnostic
	diagnostics = append(diagnostics, verifyRequiredMetadata(parsed, plan, archivePath)...)
	diagnostics = append(diagnostics, verifyReferencedPackagePaths(parsed, entries, plan, archivePath)...)
	diagnostics = append(diagnostics, verifyPeerMetadata(parsed, plan, archivePath)...)
	diagnostics = append(diagnostics, verifyArchivePayload(entries, plan, archivePath)...)
	diag.SortDiagnostics(diagnostics)
	return diagnostics
}

func readArchiveForVerification(archivePath string, plan Plan) (map[string]bool, []byte, []diag.Diagnostic) {
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, nil, []diag.Diagnostic{verifyDiagnostic("TSPACK_PACK_VERIFY_FAILED", "pack failed verification; artifacts are invalid", plan, archivePath, "archive", err.Error())}
	}
	defer file.Close()

	gz, err := gzip.NewReader(file)
	if err != nil {
		return nil, nil, []diag.Diagnostic{verifyDiagnostic("TSPACK_PACK_VERIFY_FAILED", "pack failed verification; artifacts are invalid", plan, archivePath, "archive", err.Error())}
	}
	defer gz.Close()

	entries := map[string]bool{}
	var packageJSON []byte
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, []diag.Diagnostic{verifyDiagnostic("TSPACK_PACK_VERIFY_FAILED", "pack failed verification; artifacts are invalid", plan, archivePath, "archive", err.Error())}
		}
		name := filepath.ToSlash(hdr.Name)
		if !validArchiveEntryName(name) {
			return nil, nil, []diag.Diagnostic{verifyDiagnostic("TSPACK_PACK_VERIFY_INVALID_PACKAGE_PATH", "packed archive contains unsafe entry path", plan, archivePath, "archive", name)}
		}
		entries[name] = true
		if name == "package/package.json" {
			body, err := io.ReadAll(tr)
			if err != nil {
				return nil, nil, []diag.Diagnostic{verifyDiagnostic("TSPACK_PACK_VERIFY_FAILED", "pack failed verification; artifacts are invalid", plan, archivePath, "package.json", err.Error())}
			}
			packageJSON = body
		}
	}
	return entries, packageJSON, nil
}

func validArchiveEntryName(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	if strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	if !strings.HasPrefix(name, "package/") {
		return false
	}
	cleaned := pathClean(name)
	return cleaned == name && !strings.Contains(name, "/../") && !strings.HasSuffix(name, "/..")
}

func verifyRequiredMetadata(parsed map[string]any, plan Plan, archivePath string) []diag.Diagnostic {
	var diagnostics []diag.Diagnostic
	diagnostics = append(diagnostics, verifyStringField(parsed, "name", plan.PackageName, plan, archivePath)...)
	diagnostics = append(diagnostics, verifyStringField(parsed, "version", plan.Version, plan, archivePath)...)
	if strings.TrimSpace(plan.License) != "" {
		diagnostics = append(diagnostics, verifyStringField(parsed, "license", plan.License, plan, archivePath)...)
	}
	if plan.rootTarget != nil && strings.TrimSpace(plan.rootTarget.Runtime) != "" {
		expected := packageJSONPath(plan.rootTarget.Runtime)
		diagnostics = append(diagnostics, verifyStringField(parsed, "main", expected, plan, archivePath)...)
	}
	if plan.rootTarget != nil && strings.TrimSpace(plan.rootTarget.Types) != "" {
		expected := packageJSONPath(plan.rootTarget.Types)
		diagnostics = append(diagnostics, verifyStringField(parsed, "types", expected, plan, archivePath)...)
	}
	if len(plan.targets) > 0 {
		if _, ok := parsed["exports"]; !ok {
			diagnostics = append(diagnostics, verifyDiagnostic("TSPACK_PACK_VERIFY_METADATA_MISMATCH", "packed package metadata does not match manifest", plan, archivePath, "exports", "expected exports metadata"))
		}
	}
	return diagnostics
}

func verifyStringField(parsed map[string]any, field string, expected string, plan Plan, archivePath string) []diag.Diagnostic {
	actual, ok := parsed[field].(string)
	if !ok || actual != expected {
		return []diag.Diagnostic{verifyDiagnostic("TSPACK_PACK_VERIFY_METADATA_MISMATCH", "packed package metadata does not match manifest", plan, archivePath, field, "expected="+expected, "actual="+stringValue(parsed[field]))}
	}
	return nil
}

func verifyReferencedPackagePaths(parsed map[string]any, entries map[string]bool, plan Plan, archivePath string) []diag.Diagnostic {
	var diagnostics []diag.Diagnostic
	if mainValue, ok := parsed["main"].(string); ok {
		diagnostics = append(diagnostics, verifyPackagePathReference(entries, plan, archivePath, "main", mainValue)...)
	}
	if typesValue, ok := parsed["types"].(string); ok {
		diagnostics = append(diagnostics, verifyPackagePathReference(entries, plan, archivePath, "types", typesValue)...)
	}
	if exportsValue, ok := parsed["exports"]; ok {
		diagnostics = append(diagnostics, verifyExportsReferences(entries, plan, archivePath, "exports", exportsValue)...)
	}
	return diagnostics
}

func verifyExportsReferences(entries map[string]bool, plan Plan, archivePath string, field string, value any) []diag.Diagnostic {
	switch typed := value.(type) {
	case string:
		return verifyPackagePathReference(entries, plan, archivePath, field, typed)
	case map[string]any:
		var diagnostics []diag.Diagnostic
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			childField := field + "." + key
			diagnostics = append(diagnostics, verifyExportsReferences(entries, plan, archivePath, childField, typed[key])...)
		}
		return diagnostics
	case nil:
		return nil
	case bool:
		return nil
	default:
		return []diag.Diagnostic{verifyDiagnostic("TSPACK_PACK_VERIFY_METADATA_MISMATCH", "packed package exports metadata has unsupported target", plan, archivePath, field, "actual="+stringValue(value))}
	}
}

func verifyPackagePathReference(entries map[string]bool, plan Plan, archivePath string, field string, value string) []diag.Diagnostic {
	archiveEntry, ok := normalizePackageReference(value)
	if !ok {
		return []diag.Diagnostic{verifyDiagnostic("TSPACK_PACK_VERIFY_INVALID_PACKAGE_PATH", "packed package references invalid path", plan, archivePath, field, "path="+value)}
	}
	if !entries[archiveEntry] {
		return []diag.Diagnostic{verifyDiagnostic("TSPACK_PACK_VERIFY_MISSING_FILE", "packed package references missing file", plan, archivePath, field, "path="+value)}
	}
	return nil
}

func normalizePackageReference(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "." {
		return "", false
	}
	if strings.Contains(trimmed, "\\") || strings.Contains(trimmed, "://") || strings.HasPrefix(trimmed, "/") {
		return "", false
	}
	withoutDot := strings.TrimPrefix(trimmed, "./")
	if withoutDot == "" || strings.HasPrefix(withoutDot, "../") || withoutDot == ".." {
		return "", false
	}
	cleaned := pathClean(withoutDot)
	if cleaned != withoutDot || strings.HasPrefix(cleaned, "../") || cleaned == "." || cleaned == ".." {
		return "", false
	}
	return "package/" + cleaned, true
}

func verifyPeerMetadata(parsed map[string]any, plan Plan, archivePath string) []diag.Diagnostic {
	if len(plan.peerDeps) == 0 {
		return nil
	}
	peerDependencies, _ := parsed["peerDependencies"].(map[string]any)
	var diagnostics []diag.Diagnostic
	for _, peerName := range sortedStringKeys(plan.peerDeps) {
		expectedRange := plan.peerDeps[peerName]
		actualRange, ok := peerDependencies[peerName].(string)
		if !ok || actualRange != expectedRange {
			diagnostics = append(diagnostics, verifyDiagnostic("TSPACK_PACK_VERIFY_METADATA_MISMATCH", "packed package peer dependency metadata does not match manifest", plan, archivePath, "peerDependencies."+peerName, "expected="+expectedRange, "actual="+stringValue(peerDependencies[peerName])))
		}
	}

	peerDependenciesMeta, _ := parsed["peerDependenciesMeta"].(map[string]any)
	for _, peerName := range sortedBoolKeys(plan.optionalPeers) {
		meta, _ := peerDependenciesMeta[peerName].(map[string]any)
		optional, ok := meta["optional"].(bool)
		if !ok || !optional {
			diagnostics = append(diagnostics, verifyDiagnostic("TSPACK_PACK_VERIFY_METADATA_MISMATCH", "packed package optional peer metadata does not match manifest", plan, archivePath, "peerDependenciesMeta."+peerName+".optional", "expected=true", "actual="+stringValue(meta["optional"])))
		}
	}
	return diagnostics
}

func verifyArchivePayload(entries map[string]bool, plan Plan, archivePath string) []diag.Diagnostic {
	payloadCount := 0
	for entryName := range entries {
		if entryName != "package/package.json" && !strings.HasSuffix(entryName, "/") {
			payloadCount++
		}
	}
	if payloadCount == 0 {
		return []diag.Diagnostic{verifyDiagnostic("TSPACK_PACK_VERIFY_FAILED", "packed package has no payload files", plan, archivePath, "archive", "expected at least one non-package.json entry")}
	}
	return nil
}

func verifyDiagnostic(code string, message string, plan Plan, archivePath string, field string, details ...string) diag.Diagnostic {
	allDetails := []string{
		"package=" + plan.PackageName,
		"archive=" + archivePath,
	}
	if field != "" {
		allDetails = append(allDetails, "field="+field)
	}
	allDetails = append(allDetails, details...)
	return diag.Diagnostic{Code: code, Severity: diag.SeverityError, Message: message, File: archivePath, Details: allDetails}
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedBoolKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func stringValue(value any) string {
	if value == nil {
		return "<missing>"
	}
	if text, ok := value.(string); ok {
		return text
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "<unprintable>"
	}
	return string(encoded)
}

func pathClean(value string) string {
	parts := []string{}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			if len(parts) == 0 {
				return "../" + strings.Join(parts, "/")
			}
			parts = parts[:len(parts)-1]
			continue
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return "."
	}
	return strings.Join(parts, "/")
}

func writeFailedDiagnostic(message string, path string, err error) diag.Diagnostic {
	return diag.Diagnostic{
		Code:     "TSPACK_PACK_WRITE_FAILED",
		Severity: diag.SeverityError,
		Message:  message,
		File:     path,
		Details:  []string{err.Error()},
	}
}

func resolvePattern(root, pattern string) ([]matchedFile, []diag.Diagnostic) {
	var out []matchedFile
	glob := strings.Contains(pattern, "*")
	if strings.HasSuffix(pattern, "/**") {
		pattern = strings.TrimSuffix(pattern, "/**")
		glob = true
	}
	if !glob {
		p := filepath.Join(root, pattern)
		st, err := os.Lstat(p)
		if err != nil {
			return nil, nil
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return nil, []diag.Diagnostic{dErr("TSPACK_PACK_SYMLINK_UNSUPPORTED", "symlink unsupported", pattern)}
		}
		if st.IsDir() {
			_ = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				info, _ := d.Info()
				if info.Mode()&os.ModeSymlink != 0 {
					return nil
				}
				rel, _ := filepath.Rel(root, path)
				out = append(out, matchedFile{source: path, archive: filepath.ToSlash(rel), reason: "matched include \"" + pattern + "\"", mode: 0o644, size: info.Size()})
				return nil
			})
			return out, nil
		}
		return []matchedFile{{source: p, archive: filepath.ToSlash(pattern), reason: "included explicit " + pattern, mode: 0o644, size: st.Size()}}, nil
	}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		m, _ := filepath.Match(strings.ReplaceAll(pattern, "**", "*"), rel)
		if strings.HasPrefix(rel, strings.TrimSuffix(pattern, "/*")) || m {
			info, _ := d.Info()
			if info.Mode()&os.ModeSymlink != 0 {
				out = append(out, matchedFile{source: path, archive: rel, reason: "symlink", mode: 0, size: 0})
				return nil
			}
			out = append(out, matchedFile{source: path, archive: rel, reason: "matched include \"" + pattern + "\"", mode: 0o644, size: info.Size()})
		}
		return nil
	})
	sort.SliceStable(out, func(i, j int) bool { return out[i].archive < out[j].archive })
	for _, item := range out {
		if item.reason == "symlink" {
			return nil, []diag.Diagnostic{dErr("TSPACK_PACK_SYMLINK_UNSUPPORTED", "symlink unsupported", item.archive)}
		}
	}
	return out, nil
}
func generatedPackageJSON(pkg *graph.PackageNode) []byte {
	pj := map[string]any{
		"name":    pkg.Name,
		"version": pkg.Version,
	}

	if strings.TrimSpace(pkg.License) != "" {
		pj["license"] = pkg.License
	}

	if rootTarget := rootExportTarget(pkg); rootTarget != nil {
		pj["main"] = packageJSONPath(rootTarget.Runtime)
		if strings.TrimSpace(rootTarget.Types) != "" {
			pj["types"] = packageJSONPath(rootTarget.Types)
		}
	}

	peerDependencies, optionalPeers := packageJSONPeerDependencies(pkg)
	if len(peerDependencies) > 0 {
		pj["peerDependencies"] = peerDependencies
	}
	if len(optionalPeers) > 0 {
		peerDependenciesMeta := map[string]any{}
		for _, peerName := range optionalPeers {
			peerDependenciesMeta[peerName] = map[string]bool{"optional": true}
		}
		pj["peerDependenciesMeta"] = peerDependenciesMeta
	}

	exports := map[string]any{}
	for _, t := range pkg.Targets {
		entry := map[string]string{
			"default": packageJSONPath(t.Runtime),
			"types":   packageJSONPath(t.Types),
		}
		exports[t.Export] = entry
	}
	pj["exports"] = exports

	buf := bytes.NewBuffer(nil)
	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(pj)
	return buf.Bytes()
}

func unpublishablePeerDiagnostics(pkg *graph.PackageNode, target *graph.TargetNode) []diag.Diagnostic {
	var diagnostics []diag.Diagnostic
	for _, dep := range target.PeerDeps {
		if dep.Source.Kind == "npm" {
			continue
		}
		diagnostics = append(diagnostics, dErr(
			"TSPACK_PACK_UNPUBLISHABLE_PEER_DEPENDENCY",
			"peer dependency cannot be represented in npm package.json",
			pkg.Name,
			target.Name,
			dep.Key,
			dep.Source.Kind,
		))
	}
	return diagnostics
}

func rootExportTarget(pkg *graph.PackageNode) *graph.TargetNode {
	for _, target := range pkg.Targets {
		if target.Export == "." {
			return target
		}
	}
	return nil
}

func packageJSONPath(value string) string {
	normalized := filepath.ToSlash(strings.TrimSpace(value))
	normalized = strings.TrimPrefix(normalized, "./")
	return "./" + normalized
}

func packageJSONPeerDependencies(pkg *graph.PackageNode) (map[string]string, []string) {
	peerDependencies := map[string]string{}
	optionalPeerNames := map[string]bool{}

	for _, target := range pkg.Targets {
		for _, dep := range target.PeerDeps {
			if dep.Source.Kind != "npm" {
				continue
			}
			if dep.Source.Package == "" || dep.Source.Range == "" {
				continue
			}
			peerDependencies[dep.Source.Package] = dep.Source.Range
			if dep.Optional {
				optionalPeerNames[dep.Source.Package] = true
			}
		}
	}

	optionalPeers := make([]string, 0, len(optionalPeerNames))
	for peerName := range optionalPeerNames {
		optionalPeers = append(optionalPeers, peerName)
	}
	sort.Strings(optionalPeers)

	return peerDependencies, optionalPeers
}
func badPath(p string) bool {
	return strings.TrimSpace(p) == "" || filepath.IsAbs(p) || strings.Contains(p, "..") || strings.Contains(p, "\\")
}
func hasErrors(diags []diag.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return true
		}
	}
	return false
}
func dErr(code, msg string, details ...string) diag.Diagnostic {
	return diag.Diagnostic{Code: code, Severity: diag.SeverityError, Message: msg, Details: details}
}

func ReadTarEntries(tgz []byte) ([]string, error) {
	gz, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	out := []string{}
	for {
		hdr, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
		out = append(out, hdr.Name)
	}
	return out, nil
}
