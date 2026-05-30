package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	sourceScanMaxFiles        = 2000
	sourceScanMaxBytesPerFile = 1024 * 1024
)

type sourceScanEvidence struct {
	Enabled             bool
	SkippedReason       string
	Roots               []string
	FilesScanned        int
	FilesSkipped        int
	Truncated           bool
	Warnings            []sourceScanWarning
	Packages            []sourceImportPackage
	Builtins            []sourceImportBuiltin
	UnknownDynamicCount int
}

type sourceScanWarning struct {
	Path    string
	Message string
}

type sourceImportPackage struct {
	PackageName   string
	Declaration   string
	Files         []string
	Samples       []string
	RuntimeCount  int
	TypeOnlyCount int
	MixedCount    int
	DynamicCount  int
	RequireCount  int
}

type sourceImportBuiltin struct {
	Name  string
	Files []string
	Count int
}

type sourceImportObservation struct {
	Module string
	File   string
	Kind   string
}

type sourceImportAccumulator struct {
	PackageName string
	Files       map[string]bool
	Samples     map[string]bool
	Runtime     int
	TypeOnly    int
	Mixed       int
	Dynamic     int
	Require     int
}

type sourceBuiltinAccumulator struct {
	Name  string
	Files map[string]bool
	Count int
}

var sourceFileExtensions = map[string]bool{
	".ts":  true,
	".tsx": true,
	".js":  true,
	".jsx": true,
	".mts": true,
	".cts": true,
	".mjs": true,
	".cjs": true,
}

var sourceIgnoredDirectories = map[string]bool{
	"node_modules":     true,
	"dist":             true,
	"build":            true,
	"coverage":         true,
	".git":             true,
	".tspack":          true,
	"tspack-artifacts": true,
	"temp":             true,
	"tmp":              true,
	"generated":        true,
	"__generated__":    true,
	".next":            true,
	".turbo":           true,
}

var nodeBuiltinModules = map[string]bool{
	"assert":              true,
	"buffer":              true,
	"child_process":       true,
	"cluster":             true,
	"console":             true,
	"constants":           true,
	"crypto":              true,
	"dgram":               true,
	"diagnostics_channel": true,
	"dns":                 true,
	"domain":              true,
	"events":              true,
	"fs":                  true,
	"http":                true,
	"http2":               true,
	"https":               true,
	"inspector":           true,
	"module":              true,
	"net":                 true,
	"os":                  true,
	"path":                true,
	"perf_hooks":          true,
	"process":             true,
	"punycode":            true,
	"querystring":         true,
	"readline":            true,
	"repl":                true,
	"stream":              true,
	"string_decoder":      true,
	"timers":              true,
	"tls":                 true,
	"tty":                 true,
	"url":                 true,
	"util":                true,
	"v8":                  true,
	"vm":                  true,
	"wasi":                true,
	"worker_threads":      true,
	"zlib":                true,
}

var staticImportPattern = regexp.MustCompile(`(?m)\bimport\s+(type\s+)?(?:(.*?)\s+from\s+)?["']([^"']+)["']`)
var staticExportPattern = regexp.MustCompile(`(?m)\bexport\s+(type\s+)?(.*?)\s+from\s+["']([^"']+)["']`)
var dynamicImportPattern = regexp.MustCompile(`\bimport\s*\(\s*["']([^"']+)["']\s*\)`)
var requirePattern = regexp.MustCompile(`\brequire\s*\(\s*["']([^"']+)["']\s*\)`)
var unknownDynamicImportPattern = regexp.MustCompile(`\bimport\s*\(\s*[^"'\s][^)]*\)`)

func loadSourceScanEvidence(cfg migrateConfig, pkg packageJSONModel) sourceScanEvidence {
	evidence := sourceScanEvidence{Enabled: !cfg.noSourceScan}
	if cfg.noSourceScan {
		evidence.SkippedReason = "skipped by `--no-source-scan`"
		return evidence
	}

	roots := defaultSourceScanRoots(cfg.root, pkg)
	evidence.Roots = roots
	if len(roots) == 0 {
		evidence.SkippedReason = "no default source roots found"
		return evidence
	}

	files := collectSourceScanFiles(cfg.root, roots, &evidence)
	observations := make([]sourceImportObservation, 0)
	packageAccumulators := map[string]*sourceImportAccumulator{}
	builtinAccumulators := map[string]*sourceBuiltinAccumulator{}

	for _, file := range files {
		content, err := os.ReadFile(filepath.Join(cfg.root, file))
		if err != nil {
			evidence.Warnings = append(evidence.Warnings, sourceScanWarning{Path: file, Message: err.Error()})
			continue
		}
		evidence.FilesScanned++
		observations = append(observations, scanImportsInFile(file, string(content))...)
	}

	for _, observation := range observations {
		module := observation.Module
		if isRelativeOrAbsoluteImport(module) {
			continue
		}
		if builtinName, ok := sourceBuiltinName(module); ok {
			accumulator := builtinAccumulators[builtinName]
			if accumulator == nil {
				accumulator = &sourceBuiltinAccumulator{Name: builtinName, Files: map[string]bool{}}
				builtinAccumulators[builtinName] = accumulator
			}
			accumulator.Count++
			accumulator.Files[observation.File] = true
			continue
		}
		packageName, ok := packageNameFromImport(module)
		if !ok {
			continue
		}
		accumulator := packageAccumulators[packageName]
		if accumulator == nil {
			accumulator = &sourceImportAccumulator{PackageName: packageName, Files: map[string]bool{}, Samples: map[string]bool{}}
			packageAccumulators[packageName] = accumulator
		}
		accumulator.Files[observation.File] = true
		accumulator.Samples[module] = true
		switch observation.Kind {
		case "type-only":
			accumulator.TypeOnly++
		case "mixed":
			accumulator.Mixed++
		case "dynamic":
			accumulator.Dynamic++
			accumulator.Runtime++
		case "require":
			accumulator.Require++
			accumulator.Runtime++
		default:
			accumulator.Runtime++
		}
	}

	evidence.UnknownDynamicCount = countUnknownDynamicImports(files, cfg.root)
	evidence.Packages = finalizeSourcePackages(packageAccumulators, pkg)
	evidence.Builtins = finalizeSourceBuiltins(builtinAccumulators)
	return evidence
}

func defaultSourceScanRoots(root string, pkg packageJSONModel) []string {
	candidates := []string{"src", "lib", "app"}
	if pkg.Source != "" {
		candidates = append(candidates, pkg.Source)
	}
	seen := map[string]bool{}
	var roots []string
	for _, candidate := range candidates {
		cleaned := filepath.Clean(candidate)
		if cleaned == "." || strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
			continue
		}
		if seen[cleaned] {
			continue
		}
		info, err := os.Stat(filepath.Join(root, cleaned))
		if err != nil {
			continue
		}
		if info.IsDir() || sourceFileExtensions[filepath.Ext(cleaned)] {
			seen[cleaned] = true
			roots = append(roots, cleaned)
		}
	}
	sort.Strings(roots)
	return roots
}

func collectSourceScanFiles(root string, roots []string, evidence *sourceScanEvidence) []string {
	var files []string
	for _, sourceRoot := range roots {
		fullRoot := filepath.Join(root, sourceRoot)
		info, err := os.Stat(fullRoot)
		if err != nil {
			evidence.Warnings = append(evidence.Warnings, sourceScanWarning{Path: sourceRoot, Message: err.Error()})
			continue
		}
		if !info.IsDir() {
			maybeAddSourceFile(sourceRoot, info, &files, evidence)
			continue
		}
		err = filepath.WalkDir(fullRoot, func(path string, entry os.DirEntry, walkErr error) error {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			rel = filepath.ToSlash(rel)
			if walkErr != nil {
				evidence.Warnings = append(evidence.Warnings, sourceScanWarning{Path: rel, Message: walkErr.Error()})
				return nil
			}
			if entry.IsDir() {
				if shouldSkipSourceDirectory(entry.Name()) && rel != sourceRoot {
					evidence.FilesSkipped++
					return filepath.SkipDir
				}
				return nil
			}
			info, infoErr := entry.Info()
			if infoErr != nil {
				evidence.Warnings = append(evidence.Warnings, sourceScanWarning{Path: rel, Message: infoErr.Error()})
				return nil
			}
			maybeAddSourceFile(rel, info, &files, evidence)
			if len(files) >= sourceScanMaxFiles {
				evidence.Truncated = true
				return filepath.SkipAll
			}
			return nil
		})
		if err != nil {
			evidence.Warnings = append(evidence.Warnings, sourceScanWarning{Path: sourceRoot, Message: err.Error()})
		}
	}
	sort.Strings(files)
	if len(files) > sourceScanMaxFiles {
		files = files[:sourceScanMaxFiles]
		evidence.Truncated = true
	}
	return files
}

func maybeAddSourceFile(rel string, info os.FileInfo, files *[]string, evidence *sourceScanEvidence) {
	if !sourceFileExtensions[filepath.Ext(rel)] {
		return
	}
	if info.Size() > sourceScanMaxBytesPerFile {
		evidence.FilesSkipped++
		evidence.Warnings = append(evidence.Warnings, sourceScanWarning{Path: filepath.ToSlash(rel), Message: "file exceeds 1 MiB source scan limit"})
		return
	}
	*files = append(*files, filepath.ToSlash(rel))
}

func shouldSkipSourceDirectory(name string) bool {
	if sourceIgnoredDirectories[name] {
		return true
	}
	if strings.HasPrefix(name, ".") {
		return true
	}
	return false
}

func scanImportsInFile(file string, content string) []sourceImportObservation {
	var observations []sourceImportObservation
	for _, match := range staticImportPattern.FindAllStringSubmatch(content, -1) {
		kind := classifyStaticImport(match[1], match[2])
		observations = append(observations, sourceImportObservation{Module: match[3], File: file, Kind: kind})
	}
	for _, match := range staticExportPattern.FindAllStringSubmatch(content, -1) {
		kind := classifyStaticImport(match[1], match[2])
		observations = append(observations, sourceImportObservation{Module: match[3], File: file, Kind: kind})
	}
	for _, match := range dynamicImportPattern.FindAllStringSubmatch(content, -1) {
		observations = append(observations, sourceImportObservation{Module: match[1], File: file, Kind: "dynamic"})
	}
	for _, match := range requirePattern.FindAllStringSubmatch(content, -1) {
		observations = append(observations, sourceImportObservation{Module: match[1], File: file, Kind: "require"})
	}
	return observations
}

func classifyStaticImport(typeKeyword string, specifier string) string {
	if strings.TrimSpace(typeKeyword) != "" {
		return "type-only"
	}
	trimmed := strings.TrimSpace(specifier)
	if trimmed == "" {
		return "runtime"
	}
	if !strings.Contains(trimmed, "{") || !strings.Contains(trimmed, "type ") {
		return "runtime"
	}
	prefix := strings.TrimSpace(trimmed[:strings.Index(trimmed, "{")])
	inside := trimmed[strings.Index(trimmed, "{")+1:]
	inside = inside[:strings.LastIndex(inside, "}")]
	hasRuntime := prefix != "" && prefix != ","
	hasType := false
	for _, part := range strings.Split(inside, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, "type ") {
			hasType = true
		} else {
			hasRuntime = true
		}
	}
	if hasType && hasRuntime {
		return "mixed"
	}
	if hasType {
		return "type-only"
	}
	return "runtime"
}

func countUnknownDynamicImports(files []string, root string) int {
	count := 0
	for _, file := range files {
		content, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			continue
		}
		count += len(unknownDynamicImportPattern.FindAll(content, -1))
	}
	return count
}

func isRelativeOrAbsoluteImport(module string) bool {
	return strings.HasPrefix(module, ".") || strings.HasPrefix(module, "/")
}

func sourceBuiltinName(module string) (string, bool) {
	name := strings.TrimPrefix(module, "node:")
	if strings.Contains(name, "/") {
		name = strings.SplitN(name, "/", 2)[0]
	}
	return name, nodeBuiltinModules[name]
}

func packageNameFromImport(module string) (string, bool) {
	if module == "" || isRelativeOrAbsoluteImport(module) {
		return "", false
	}
	parts := strings.Split(module, "/")
	if strings.HasPrefix(module, "@") {
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return "", false
		}
		return parts[0] + "/" + parts[1], true
	}
	return parts[0], true
}

func finalizeSourcePackages(accumulators map[string]*sourceImportAccumulator, pkg packageJSONModel) []sourceImportPackage {
	keys := make([]string, 0, len(accumulators))
	for key := range accumulators {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	packages := make([]sourceImportPackage, 0, len(keys))
	for _, key := range keys {
		accumulator := accumulators[key]
		packages = append(packages, sourceImportPackage{
			PackageName:   accumulator.PackageName,
			Declaration:   sourceDeclarationForPackage(accumulator.PackageName, pkg),
			Files:         cappedSortedKeys(accumulator.Files, 5),
			Samples:       cappedSortedKeys(accumulator.Samples, 4),
			RuntimeCount:  accumulator.Runtime,
			TypeOnlyCount: accumulator.TypeOnly,
			MixedCount:    accumulator.Mixed,
			DynamicCount:  accumulator.Dynamic,
			RequireCount:  accumulator.Require,
		})
	}
	return packages
}

func finalizeSourceBuiltins(accumulators map[string]*sourceBuiltinAccumulator) []sourceImportBuiltin {
	keys := make([]string, 0, len(accumulators))
	for key := range accumulators {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	builtins := make([]sourceImportBuiltin, 0, len(keys))
	for _, key := range keys {
		accumulator := accumulators[key]
		builtins = append(builtins, sourceImportBuiltin{Name: key, Files: cappedSortedKeys(accumulator.Files, 5), Count: accumulator.Count})
	}
	return builtins
}

func sourceDeclarationForPackage(name string, pkg packageJSONModel) string {
	if _, ok := pkg.PeerDependencies[name]; ok {
		return "peerDependency"
	}
	if _, ok := pkg.Dependencies[name]; ok {
		return "dependency"
	}
	if _, ok := pkg.OptionalDependencies[name]; ok {
		return "optionalDependency"
	}
	if _, ok := pkg.DevDependencies[name]; ok {
		return "devDependency"
	}
	return "missing"
}

func cappedSortedKeys(values map[string]bool, limit int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > limit {
		return keys[:limit]
	}
	return keys
}

func sourceObservedUsage(pkg sourceImportPackage) string {
	var parts []string
	if pkg.RuntimeCount > 0 {
		parts = append(parts, "runtime")
	}
	if pkg.TypeOnlyCount > 0 {
		parts = append(parts, "type")
	}
	if pkg.MixedCount > 0 {
		parts = append(parts, "mixed")
	}
	if pkg.DynamicCount > 0 {
		parts = append(parts, "dynamic")
	}
	if len(parts) == 0 {
		return "observed"
	}
	return strings.Join(parts, "+")
}

func sourceEvidenceForDependency(draft *migrationDraft, packageName string) (sourceImportPackage, bool) {
	for _, pkg := range draft.SourceEvidence.Packages {
		if pkg.PackageName == packageName {
			return pkg, true
		}
	}
	return sourceImportPackage{}, false
}

func sourceScanMissingDeclarationCount(evidence sourceScanEvidence) int {
	count := 0
	for _, pkg := range evidence.Packages {
		if pkg.Declaration == "missing" {
			count++
		}
	}
	return count
}

func sourceScanDevRuntimeMismatchCount(evidence sourceScanEvidence) int {
	count := 0
	for _, pkg := range evidence.Packages {
		if pkg.Declaration == "devDependency" && (pkg.RuntimeCount > 0 || pkg.MixedCount > 0 || pkg.DynamicCount > 0 || pkg.RequireCount > 0) {
			count++
		}
	}
	return count
}

func sourceScanTypeOnlyCandidateCount(evidence sourceScanEvidence) int {
	count := 0
	for _, pkg := range evidence.Packages {
		if pkg.TypeOnlyCount > 0 && pkg.RuntimeCount == 0 && pkg.MixedCount == 0 && pkg.DynamicCount == 0 && pkg.RequireCount == 0 {
			count++
		}
	}
	return count
}

func sourceFileListForComment(files []string) string {
	if len(files) == 0 {
		return "source files"
	}
	return strings.Join(files, ", ")
}

func formatSourceScanRoots(roots []string) string {
	if len(roots) == 0 {
		return "-"
	}
	quoted := make([]string, 0, len(roots))
	for _, root := range roots {
		quoted = append(quoted, "`"+root+"`")
	}
	return strings.Join(quoted, ", ")
}

func formatSourcePackageList(packages []string) string {
	if len(packages) == 0 {
		return "none"
	}
	return "`" + strings.Join(packages, "`, `") + "`"
}

func sourceRuntimeDevPackages(evidence sourceScanEvidence) []string {
	var names []string
	for _, pkg := range evidence.Packages {
		if pkg.Declaration == "devDependency" && (pkg.RuntimeCount > 0 || pkg.MixedCount > 0 || pkg.DynamicCount > 0 || pkg.RequireCount > 0) {
			names = append(names, pkg.PackageName)
		}
	}
	return names
}

func sourceTypeOnlyPackages(evidence sourceScanEvidence) []string {
	var names []string
	for _, pkg := range evidence.Packages {
		if pkg.TypeOnlyCount > 0 && pkg.RuntimeCount == 0 && pkg.MixedCount == 0 && pkg.DynamicCount == 0 && pkg.RequireCount == 0 {
			names = append(names, pkg.PackageName)
		}
	}
	return names
}

func sourceMissingPackages(evidence sourceScanEvidence) []string {
	var names []string
	for _, pkg := range evidence.Packages {
		if pkg.Declaration == "missing" {
			names = append(names, pkg.PackageName)
		}
	}
	return names
}

func sourceUnusedDeclaredPackages(draft *migrationDraft) []string {
	observed := map[string]bool{}
	for _, pkg := range draft.SourceEvidence.Packages {
		observed[pkg.PackageName] = true
	}
	var names []string
	for _, dep := range draft.Dependencies {
		if dep.KnownTool {
			continue
		}
		if !observed[dep.PackageName] {
			names = append(names, dep.PackageName)
		}
	}
	sort.Strings(names)
	return names
}

func sourceTargetHints(draft *migrationDraft) []string {
	var hints []string
	if !draft.SourceEvidence.Enabled || draft.SourceEvidence.FilesScanned == 0 {
		return hints
	}
	files := map[string]bool{}
	for _, pkg := range draft.SourceEvidence.Packages {
		for _, file := range pkg.Files {
			files[file] = true
		}
	}
	for _, target := range draft.Targets {
		entry := strings.TrimPrefix(target.Entry, "./")
		if files[entry] {
			hints = append(hints, fmt.Sprintf("Target `%s` entry `%s` was scanned and contains external import evidence. %s", target.Name, target.Entry, migrationTodoTargets))
		}
	}
	return hints
}

func sourcePackageHasRuntimeUse(pkg sourceImportPackage) bool {
	return pkg.RuntimeCount > 0 || pkg.MixedCount > 0 || pkg.DynamicCount > 0 || pkg.RequireCount > 0
}

func sourcePackageIsTypeOnly(pkg sourceImportPackage) bool {
	return pkg.TypeOnlyCount > 0 && !sourcePackageHasRuntimeUse(pkg)
}

func sourceDependencyNeedsTODO(pkg sourceImportPackage, hasSourceEvidence bool) bool {
	if !hasSourceEvidence {
		return false
	}
	if pkg.Declaration == "devDependency" && sourcePackageHasRuntimeUse(pkg) {
		return true
	}
	return false
}

func migrationDiagnosticsFromSourceEvidence(evidence sourceScanEvidence) []migrationDiagnostic {
	var diagnostics []migrationDiagnostic
	if !evidence.Enabled {
		return diagnostics
	}
	if evidence.FilesScanned == 0 && len(evidence.Roots) > 0 && len(evidence.Warnings) > 0 {
		diagnostics = append(diagnostics, migrationDiagnostic{
			Code:    "TSPACK_MIGRATE_SOURCE_SCAN_FAILED",
			Message: "source scan could not read any files from discovered source roots",
			Details: []string{"roots: " + strings.Join(evidence.Roots, ", ")},
			Fixes:   []string{"Use `--no-source-scan` to skip source evidence or review source permissions."},
		})
	}
	if evidence.Truncated {
		diagnostics = append(diagnostics, migrationDiagnostic{
			Code:    "TSPACK_MIGRATE_SOURCE_SCAN_TRUNCATED",
			Message: "source scan hit conservative limits and was truncated",
			Details: []string{fmt.Sprintf("maxFiles: %d", sourceScanMaxFiles)},
			Fixes:   []string{"Use the migration report as partial evidence, or review source imports manually."},
		})
	}
	for _, warning := range evidence.Warnings {
		diagnostics = append(diagnostics, migrationDiagnostic{
			Code:    "TSPACK_MIGRATE_SOURCE_PARSE_WARNING",
			Message: "source scan warning",
			Details: []string{"path: " + warning.Path, "warning: " + warning.Message},
			Fixes:   []string{"Review the file manually if source import evidence is important."},
		})
	}
	return diagnostics
}

func migrationSourceInputSummary(evidence sourceScanEvidence) string {
	if !evidence.Enabled {
		return evidence.SkippedReason
	}
	if evidence.SkippedReason != "" {
		return evidence.SkippedReason
	}
	return fmt.Sprintf("enabled; scanned %d files from %s", evidence.FilesScanned, formatSourceScanRoots(evidence.Roots))
}

func renderSourceImportEvidenceSection(builder *strings.Builder, draft *migrationDraft) {
	evidence := draft.SourceEvidence
	builder.WriteString("## Source import evidence\n\n")
	builder.WriteString("### Scan summary\n")
	if !evidence.Enabled || evidence.SkippedReason != "" {
		builder.WriteString("- status: " + migrationSourceInputSummary(evidence) + "\n\n")
		return
	}
	builder.WriteString("- status: enabled\n")
	builder.WriteString("- roots scanned: " + formatSourceScanRoots(evidence.Roots) + "\n")
	builder.WriteString(fmt.Sprintf("- files scanned: %d\n", evidence.FilesScanned))
	builder.WriteString(fmt.Sprintf("- files skipped: %d\n", evidence.FilesSkipped))
	builder.WriteString(fmt.Sprintf("- external packages observed: %d\n", len(evidence.Packages)))
	builder.WriteString(fmt.Sprintf("- missing declarations: %d\n", sourceScanMissingDeclarationCount(evidence)))
	builder.WriteString(fmt.Sprintf("- dev-runtime mismatches: %d\n", sourceScanDevRuntimeMismatchCount(evidence)))
	builder.WriteString(fmt.Sprintf("- type-only candidates: %d\n", sourceScanTypeOnlyCandidateCount(evidence)))
	if evidence.Truncated {
		builder.WriteString("- warning: source scan truncated by conservative limits\n")
	}
	if len(evidence.Warnings) > 0 {
		builder.WriteString(fmt.Sprintf("- warnings: %d\n", len(evidence.Warnings)))
	}
	builder.WriteString("\n")

	builder.WriteString("### Observed external imports\n")
	if len(evidence.Packages) == 0 {
		builder.WriteString("No external package imports were observed in scanned source files.\n\n")
	} else {
		builder.WriteString("| package | observed usage | package.json declaration | files | samples |\n")
		builder.WriteString("|---|---|---|---|---|\n")
		for _, pkg := range evidence.Packages {
			builder.WriteString("| `" + pkg.PackageName + "` | `" + sourceObservedUsage(pkg) + "` | `" + pkg.Declaration + "` | " + markdownInlineList(pkg.Files) + " | " + markdownInlineList(pkg.Samples) + " |\n")
		}
		builder.WriteString("\n")
	}

	builder.WriteString("### Classification hints\n")
	runtimeDev := sourceRuntimeDevPackages(evidence)
	if len(runtimeDev) > 0 {
		builder.WriteString("- " + migrationTodoDepClassification + ": runtime imports declared only as devDependencies: " + formatSourcePackageList(runtimeDev) + ".\n")
	} else {
		builder.WriteString("- No runtime imports declared only as devDependencies were observed.\n")
	}
	typeOnly := sourceTypeOnlyPackages(evidence)
	if len(typeOnly) > 0 {
		builder.WriteString("- " + migrationTodoTypes + ": packages observed only through type-only imports: " + formatSourcePackageList(typeOnly) + ".\n")
	}
	missing := sourceMissingPackages(evidence)
	if len(missing) > 0 {
		builder.WriteString("- " + migrationTodoDepClassification + ": imported packages missing from direct package.json declarations: " + formatSourcePackageList(missing) + ".\n")
	}
	unused := sourceUnusedDeclaredPackages(draft)
	if len(unused) > 0 {
		builder.WriteString("- Declared packages not observed in scanned source: " + formatSourcePackageList(unused) + ". These may still be build-only, runtime-only, optional, or used from unscanned files.\n")
	}
	if len(evidence.Builtins) > 0 {
		builder.WriteString("- Node builtin imports observed: ")
		var parts []string
		for _, builtin := range evidence.Builtins {
			parts = append(parts, fmt.Sprintf("`%s` (%d)", builtin.Name, builtin.Count))
		}
		builder.WriteString(strings.Join(parts, ", ") + ". Review runtime environment assumptions.\n")
	}
	if evidence.UnknownDynamicCount > 0 {
		builder.WriteString(fmt.Sprintf("- Unknown dynamic import expressions observed: %d. Review manually; no package names were inferred.\n", evidence.UnknownDynamicCount))
	}
	builder.WriteString("\n")

	hints := sourceTargetHints(draft)
	if len(hints) > 0 {
		builder.WriteString("### Target hints\n")
		for _, hint := range hints {
			builder.WriteString("- " + hint + "\n")
		}
		builder.WriteString("\n")
	}

	if len(evidence.Warnings) > 0 {
		builder.WriteString("### Source scan warnings\n")
		for _, warning := range evidence.Warnings {
			builder.WriteString("- `" + warning.Path + "`: " + warning.Message + "\n")
		}
		builder.WriteString("\n")
	}
}

func markdownInlineList(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, "`"+value+"`")
	}
	return strings.Join(quoted, ", ")
}

func printMigrationSourceDryRun(evidence sourceScanEvidence) {
	fmt.Println("Source scan:")
	if !evidence.Enabled || evidence.SkippedReason != "" {
		fmt.Println("  " + migrationSourceInputSummary(evidence))
		return
	}
	fmt.Println("  enabled")
	fmt.Printf("  files scanned: %d\n", evidence.FilesScanned)
	fmt.Printf("  external packages: %d\n", len(evidence.Packages))
	fmt.Printf("  missing declarations: %d\n", sourceScanMissingDeclarationCount(evidence))
	fmt.Printf("  dev-runtime mismatches: %d\n", sourceScanDevRuntimeMismatchCount(evidence))
	fmt.Printf("  type-only candidates: %d\n", sourceScanTypeOnlyCandidateCount(evidence))
	if evidence.Truncated {
		fmt.Println("  warning: source scan truncated")
	}
	if len(evidence.Warnings) > 0 {
		fmt.Printf("  warnings: %d\n", len(evidence.Warnings))
	}
}
