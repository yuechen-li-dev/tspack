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
}
type File struct {
	PackageName, SourcePath, ArchivePath string
	Size                                 int64
	Reason                               string
}

type Plan struct {
	PackageName string
	Version     string
	Path        string
	files       map[string]matchedFile
	keys        []string
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
	out.Plans = append(out.Plans, Plan{PackageName: pkg.Name, Version: pkg.Version, Path: filepath.Join(outputDir, name), files: files, keys: keys})
	return out
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
		pendingArtifacts = append(pendingArtifacts, Artifact{PackageName: plan.PackageName, Version: plan.Version, Path: plan.Path, Hash: "sha256:" + hex.EncodeToString(h[:]), Size: int64(len(archive))})
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
