package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/nodecmd"
)

const (
	migrationTodoTargets           = "MIGRATION_TODO_TARGETS"
	migrationTodoDepClassification = "MIGRATION_TODO_DEP_CLASSIFICATION"
	migrationTodoRunTargets        = "MIGRATION_TODO_RUN_TARGETS"
	migrationTodoPublish           = "MIGRATION_TODO_PUBLISH"
	migrationTodoBoundaries        = "MIGRATION_TODO_BOUNDARIES"
	migrationTodoTypes             = "MIGRATION_TODO_TYPES"
	migrationTodoSecurity          = "MIGRATION_TODO_SECURITY"
)

var migrationTodoOrder = []string{
	migrationTodoTargets,
	migrationTodoDepClassification,
	migrationTodoRunTargets,
	migrationTodoPublish,
	migrationTodoBoundaries,
	migrationTodoTypes,
	migrationTodoSecurity,
}

var knownMigrationTools = map[string]bool{
	"typescript":       true,
	"vite":             true,
	"vitest":           true,
	"tsup":             true,
	"rollup":           true,
	"webpack":          true,
	"esbuild":          true,
	"@biomejs/biome":   true,
	"biome":            true,
	"eslint":           true,
	"prettier":         true,
	"jest":             true,
	"playwright":       true,
	"@playwright/test": true,
	"turbo":            true,
	"nx":               true,
}

var lifecycleScriptNames = map[string]bool{
	"preinstall":     true,
	"install":        true,
	"postinstall":    true,
	"prepack":        true,
	"prepare":        true,
	"postpack":       true,
	"prepublish":     true,
	"prepublishOnly": true,
	"postpublish":    true,
}

type migrateConfig struct {
	root            string
	packageJSONPath string
	packageLockPath string
	outManifestPath string
	outReportPath   string
	write           bool
	check           bool
	force           bool
	noLockEvidence  bool
	noSourceScan    bool
	scanSource      bool
}

type packageJSONModel struct {
	Name                 string
	Version              string
	License              string
	Private              bool
	HasPrivate           bool
	Type                 string
	Main                 string
	Module               string
	Types                string
	Typings              string
	Exports              any
	Files                []string
	Dependencies         map[string]string
	PeerDependencies     map[string]string
	PeerDependenciesMeta map[string]peerDependencyMeta
	DevDependencies      map[string]string
	OptionalDependencies map[string]string
	Scripts              map[string]string
	Bin                  any
	Repository           any
	Homepage             string
	Source               string
	Bugs                 any
	InvalidFields        []string
}

type peerDependencyMeta struct {
	Optional bool
}

type migrationDraft struct {
	Config               migrateConfig
	Package              packageJSONModel
	Kind                 string
	WorkspaceName        string
	Manifest             string
	Report               string
	Dependencies         []migratedDependency
	Targets              []migratedTarget
	PublishInclude       []string
	DuplicatePeerDeps    []string
	IdentifierCollisions []string
	SkippedRanges        []string
	SubpathTodos         []string
	LifecycleScripts     []string
	ScriptAnalyses       []scriptAnalysis
	LockEvidence         packageLockEvidence
	SourceEvidence       sourceScanEvidence
	Diagnostics          []migrationDiagnostic
	Validation           migrationValidationResult
	TodoCounts           map[string]int
	TotalTodos           int
}

type migratedDependency struct {
	Key          string
	PackageName  string
	Range        string
	Kind         string
	SourceField  string
	OptionalPeer bool
	KnownTool    bool
	NeedsTODO    bool
}

type migratedTarget struct {
	Name        string
	Export      string
	Entry       string
	Runtime     string
	Types       string
	NeedsTODO   bool
	Description string
}

type scriptAnalysis struct {
	Name        string
	Command     string
	Category    string
	Confidence  string
	Rationale   string
	Action      string
	NeedsReview bool
	ReviewNotes []string
	Argv        []string
	Suggestion  *runTargetSuggestion
}

type runTargetSuggestion struct {
	Name       string
	Command    []string
	Cwd        string
	URL        string
	Ready      string
	Confidence string
	Notes      []string
}

type simpleExportInfo struct {
	Runtime string
	Types   string
}

type migrationDiagnostic struct {
	Code    string
	Message string
	Details []string
	Fixes   []string
}

type migrationValidationResult struct {
	Ran                 bool
	Passed              bool
	ManifestFrontend    string
	ManifestIR          string
	RemainingTodos      int
	TodoCounts          map[string]int
	FrontendDiagnostics []diag.Diagnostic
	IRDiagnostics       []diag.Diagnostic
	TempPath            string
}

var runMigrationValidation = validateMigrationDraft

func runMigrateCommand(args []string) {
	cfg, parseDiags := parseMigrateArgs(args)
	if len(parseDiags) > 0 {
		for _, diagnostic := range parseDiags {
			printMigrationDiagnostic(diagnostic)
		}
		exit(1)
	}

	draft, diagnostic := buildMigrationDraft(cfg)
	if diagnostic != nil {
		printMigrationDiagnostic(*diagnostic)
		exit(1)
	}
	for _, diagnostic := range draft.Diagnostics {
		printMigrationDiagnostic(diagnostic)
	}

	outputs := []plannedFile{
		{path: cfg.outManifestPath, content: draft.Manifest},
		{path: cfg.outReportPath, content: draft.Report},
	}

	if cfg.write && !cfg.force {
		for _, output := range outputs {
			if _, err := os.Stat(output.path); err == nil {
				printMigrationDiagnostic(migrationDiagnostic{
					Code:    "TSPACK_MIGRATE_OUTPUT_EXISTS",
					Message: "migration output already exists",
					Details: []string{
						"root: " + cfg.root,
						"packageJsonPath: " + cfg.packageJSONPath,
						"outputPath: " + output.path,
					},
					Fixes: []string{"Choose a different --out-manifest/--out-report path, remove the existing file, or pass --force."},
				})
				exit(1)
			}
		}
	}

	var validationDiagnostic *migrationDiagnostic
	if cfg.check {
		result, validationErr := runMigrationValidation(draft)
		draft.Validation = result
		draft.Report = renderMigrationReport(&draft)
		outputs[0].content = draft.Manifest
		outputs[1].content = draft.Report
		validationDiagnostic = validationErr
	}

	if !cfg.write {
		printMigrationDryRun(draft)
		if validationDiagnostic != nil {
			printMigrationDiagnostic(*validationDiagnostic)
			exit(1)
		}
		return
	}

	if validationDiagnostic != nil {
		fmt.Println("Migration validation failed; no files were written.")
		fmt.Println()
		printMigrationValidationSummary(draft.Validation)
		printMigrationDiagnostic(*validationDiagnostic)
		exit(1)
	}

	for _, output := range outputs {
		if err := writeGeneratedFile(output.path, output.content, true); err != nil {
			printMigrationDiagnostic(migrationDiagnostic{
				Code:    "TSPACK_MIGRATE_WRITE_FAILED",
				Message: "failed to write migration output",
				Details: []string{
					"root: " + cfg.root,
					"packageJsonPath: " + cfg.packageJSONPath,
					"outputPath: " + output.path,
					"error: " + err.Error(),
				},
				Fixes: []string{"Check output directory permissions and retry."},
			})
			exit(1)
		}
	}

	printMigrationWriteSummary(draft)
}

func validateMigrationDraft(draft migrationDraft) (migrationValidationResult, *migrationDiagnostic) {
	result := migrationValidationResult{
		Ran:              true,
		ManifestFrontend: "not run",
		ManifestIR:       "not run",
		RemainingTodos:   draft.TotalTodos,
		TodoCounts:       copyTodoCounts(draft.TodoCounts),
	}

	tempDir, err := os.MkdirTemp("", "tspack-migrate-check-*")
	if err != nil {
		result.ManifestFrontend = "failed"
		return result, &migrationDiagnostic{
			Code:    "TSPACK_MIGRATE_CHECK_TEMP_WRITE_FAILED",
			Message: "failed to create temporary manifest validation directory",
			Details: []string{"error: " + err.Error()},
			Fixes:   []string{"Check temporary directory permissions and retry."},
		}
	}
	defer os.RemoveAll(tempDir)

	tempManifestPath := filepath.Join(tempDir, "manifest.tsx")
	result.TempPath = tempManifestPath
	if err := os.WriteFile(tempManifestPath, []byte(draft.Manifest), 0o644); err != nil {
		result.ManifestFrontend = "failed"
		return result, &migrationDiagnostic{
			Code:    "TSPACK_MIGRATE_CHECK_TEMP_WRITE_FAILED",
			Message: "failed to write temporary manifest validation file",
			Details: []string{
				"manifestDraftPath: " + tempManifestPath,
				"error: " + err.Error(),
			},
			Fixes: []string{"Check temporary directory permissions and retry."},
		}
	}

	frontendCLIPath := migrationFrontendCLIPath(repoRootForMigrateRuntime())
	frontendResult := runMigrationManifestFrontend(frontendCLIPath, tempManifestPath)
	result.FrontendDiagnostics = frontendResult.Diagnostics
	if !frontendResult.OK || len(frontendResult.Diagnostics) > 0 {
		result.ManifestFrontend = "failed"
		return result, migrationManifestInvalidDiagnostic(result, frontendResult.Diagnostics)
	}
	result.ManifestFrontend = "passed"

	_, irDiagnostics := manifest.LoadBytes(tempManifestPath, frontendResult.IR)
	result.IRDiagnostics = irDiagnostics
	if hasMigrationErrorDiagnostics(irDiagnostics) {
		result.ManifestIR = "failed"
		return result, migrationIRInvalidDiagnostic(result, irDiagnostics)
	}
	result.ManifestIR = "passed"
	result.Passed = true
	return result, nil
}

type migrationFrontendResult struct {
	OK          bool              `json:"ok"`
	IR          json.RawMessage   `json:"ir"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

func migrationFrontendCLIPath(repoRoot string) string {
	if override := strings.TrimSpace(os.Getenv("TSPACK_MANIFEST_FRONTEND")); override != "" {
		return override
	}
	if override := strings.TrimSpace(os.Getenv("TSPACK_MANIFEST_FRONTEND_CLI")); override != "" {
		return override
	}

	candidates := migrationFrontendCLICandidates(repoRoot)
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return candidates[0]
}

func migrationFrontendCLICandidates(repoRoot string) []string {
	return []string{
		filepath.Join(repoRoot, "manifest-frontend", "dist", "cli.js"),
		filepath.Join(repoRoot, "manifest-frontend", "dist", "src", "cli.js"),
	}
}

func runMigrationManifestFrontend(frontendCLIPath string, manifestPath string) migrationFrontendResult {
	if _, err := os.Stat(frontendCLIPath); err != nil {
		return migrationFrontendResult{
			OK: false,
			Diagnostics: []diag.Diagnostic{{
				Code:     "TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED",
				Severity: diag.SeverityError,
				Message:  "manifest frontend CLI not found; run `cd manifest-frontend && npm run build`",
				File:     manifestPath,
				Details:  []string{frontendCLIPath},
			}},
		}
	}

	cmd, err := nodecmd.Command(frontendCLIPath, manifestPath)
	if err != nil {
		if nodecmd.IsNotFound(err) {
			return migrationFrontendResult{
				OK: false,
				Diagnostics: []diag.Diagnostic{{
					Code:     nodecmd.DiagnosticCode,
					Severity: diag.SeverityError,
					Message:  "Node.js was not found on PATH.",
					File:     manifestPath,
					Details:  nodecmd.GuidanceLines(),
				}},
			}
		}
		return migrationFrontendResult{
			OK: false,
			Diagnostics: []diag.Diagnostic{{
				Code:     "TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED",
				Severity: diag.SeverityError,
				Message:  "failed to prepare manifest frontend command",
				File:     manifestPath,
				Details:  []string{err.Error()},
			}},
		}
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil && stdout.Len() == 0 {
		return migrationFrontendResult{
			OK: false,
			Diagnostics: []diag.Diagnostic{{
				Code:     "TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED",
				Severity: diag.SeverityError,
				Message:  "manifest frontend failed",
				File:     manifestPath,
				Details:  []string{err.Error(), strings.TrimSpace(stderr.String())},
			}},
		}
	}

	var parsed migrationFrontendResult
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		return migrationFrontendResult{
			OK: false,
			Diagnostics: []diag.Diagnostic{{
				Code:     "TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED",
				Severity: diag.SeverityError,
				Message:  "invalid frontend JSON",
				File:     manifestPath,
				Details:  []string{err.Error(), strings.TrimSpace(stderr.String())},
			}},
		}
	}
	return parsed
}

func migrationManifestInvalidDiagnostic(result migrationValidationResult, diagnostics []diag.Diagnostic) *migrationDiagnostic {
	details := []string{
		"manifestDraftPath: " + result.TempPath,
		fmt.Sprintf("remainingTodos: %d", result.RemainingTodos),
		"todosAreErrors: false",
	}
	details = append(details, migrationDiagnosticDetails("frontend diagnostic", diagnostics)...)
	return &migrationDiagnostic{
		Code:    "TSPACK_MIGRATE_GENERATED_MANIFEST_INVALID",
		Message: "generated manifest draft did not pass manifest frontend validation",
		Details: details,
		Fixes: []string{
			"Review the generated manifest structure; MIGRATION_TODO_* comments do not fail validation by themselves.",
			"If this came from ordinary package.json input, treat it as a tspack migrate bug.",
		},
	}
}

func migrationIRInvalidDiagnostic(result migrationValidationResult, diagnostics []diag.Diagnostic) *migrationDiagnostic {
	details := []string{
		"manifestDraftPath: " + result.TempPath,
		fmt.Sprintf("remainingTodos: %d", result.RemainingTodos),
		"todosAreErrors: false",
	}
	if migrationDiagnosticsContainCode(diagnostics, "TSPACK_IR_UNKNOWN_DEPENDENCY_REF") {
		details = append(details, "dependencyRefHint: generated manifest contains dependency refs that do not match declared dependency identities; this is likely a migration generator bug or an alias/key mismatch")
	}
	details = append(details, migrationDiagnosticDetails("IR diagnostic", diagnostics)...)
	return &migrationDiagnostic{
		Code:    "TSPACK_MIGRATE_GENERATED_IR_INVALID",
		Message: "generated manifest draft frontend IR did not pass Go manifest validation",
		Details: details,
		Fixes: []string{
			"Review the generated manifest structure; MIGRATION_TODO_* comments do not fail validation by themselves.",
			"If this came from ordinary package.json input, treat it as a tspack migrate bug.",
		},
	}
}

func migrationDiagnosticDetails(prefix string, diagnostics []diag.Diagnostic) []string {
	var details []string
	for _, diagnostic := range diagnostics {
		message := strings.TrimSpace(diagnostic.Message)
		if message == "" {
			message = string(diagnostic.Severity)
		}
		detail := prefix + ": " + diagnostic.Code + " " + message
		if diagnostic.File != "" {
			detail += " (" + diagnostic.File + ")"
		}
		details = append(details, detail)
		for _, nested := range diagnostic.Details {
			if strings.TrimSpace(nested) != "" {
				details = append(details, "  "+nested)
			}
		}
	}
	if len(details) == 0 {
		return []string{prefix + ": no structured diagnostics returned"}
	}
	return details
}

func migrationDiagnosticsContainCode(diagnostics []diag.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func hasMigrationErrorDiagnostics(diagnostics []diag.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == diag.SeverityError || diagnostic.Severity == "" {
			return true
		}
	}
	return false
}

func copyTodoCounts(counts map[string]int) map[string]int {
	out := map[string]int{}
	for key, value := range counts {
		out[key] = value
	}
	return out
}

func repoRootForMigrateRuntime() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, frontendErr := os.Stat(filepath.Join(dir, "manifest-frontend")); frontendErr == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return wd
		}
	}
}

func parseMigrateArgs(args []string) (migrateConfig, []migrationDiagnostic) {
	cfg := migrateConfig{root: "."}
	var diags []migrationDiagnostic

	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--write":
			cfg.write = true
		case "--check":
			cfg.check = true
		case "--force":
			cfg.force = true
		case "--root":
			value, ok := readFlagValue(args, &i)
			if !ok {
				diags = append(diags, migrationFlagDiagnostic(arg))
				continue
			}
			cfg.root = value
		case "--package-json":
			value, ok := readFlagValue(args, &i)
			if !ok {
				diags = append(diags, migrationFlagDiagnostic(arg))
				continue
			}
			cfg.packageJSONPath = value
		case "--package-lock":
			value, ok := readFlagValue(args, &i)
			if !ok {
				diags = append(diags, migrationFlagDiagnostic(arg))
				continue
			}
			cfg.packageLockPath = value
		case "--no-lock-evidence":
			cfg.noLockEvidence = true
		case "--scan-source":
			cfg.scanSource = true
		case "--no-source-scan":
			cfg.noSourceScan = true
		case "--out-manifest":
			value, ok := readFlagValue(args, &i)
			if !ok {
				diags = append(diags, migrationFlagDiagnostic(arg))
				continue
			}
			cfg.outManifestPath = value
		case "--out-report":
			value, ok := readFlagValue(args, &i)
			if !ok {
				diags = append(diags, migrationFlagDiagnostic(arg))
				continue
			}
			cfg.outReportPath = value
		default:
			diags = append(diags, migrationDiagnostic{
				Code:    "TSPACK_MIGRATE_INVALID_ARGS",
				Message: "unknown migrate flag: " + arg,
				Fixes:   []string{"Run `tspack migrate --write` or `tspack migrate --root <root>`."},
			})
		}
	}

	absRoot, err := filepath.Abs(cfg.root)
	if err == nil {
		cfg.root = absRoot
	}
	if cfg.packageJSONPath == "" {
		cfg.packageJSONPath = filepath.Join(cfg.root, "package.json")
	} else if !filepath.IsAbs(cfg.packageJSONPath) {
		cfg.packageJSONPath = filepath.Join(cfg.root, cfg.packageJSONPath)
	}
	if cfg.scanSource && cfg.noSourceScan {
		diags = append(diags, migrationDiagnostic{
			Code:    "TSPACK_MIGRATE_INVALID_ARGS",
			Message: "--scan-source cannot be combined with --no-source-scan",
			Fixes:   []string{"Remove --scan-source or remove --no-source-scan."},
		})
	}
	if cfg.packageLockPath != "" && cfg.noLockEvidence {
		diags = append(diags, migrationDiagnostic{
			Code:    "TSPACK_MIGRATE_INVALID_ARGS",
			Message: "--package-lock cannot be combined with --no-lock-evidence",
			Fixes:   []string{"Remove --package-lock or remove --no-lock-evidence."},
		})
	}
	if cfg.outManifestPath == "" {
		cfg.outManifestPath = filepath.Join(cfg.root, "manifest.migrated.tsx")
	} else if !filepath.IsAbs(cfg.outManifestPath) {
		cfg.outManifestPath = filepath.Join(cfg.root, cfg.outManifestPath)
	}
	if cfg.outReportPath == "" {
		cfg.outReportPath = filepath.Join(cfg.root, "tspack-migration.md")
	} else if !filepath.IsAbs(cfg.outReportPath) {
		cfg.outReportPath = filepath.Join(cfg.root, cfg.outReportPath)
	}
	if cfg.packageLockPath != "" && !filepath.IsAbs(cfg.packageLockPath) {
		cfg.packageLockPath = filepath.Join(cfg.root, cfg.packageLockPath)
	}

	cfg.packageJSONPath = filepath.Clean(cfg.packageJSONPath)
	if cfg.packageLockPath != "" {
		cfg.packageLockPath = filepath.Clean(cfg.packageLockPath)
	}
	cfg.outManifestPath = filepath.Clean(cfg.outManifestPath)
	cfg.outReportPath = filepath.Clean(cfg.outReportPath)

	return cfg, diags
}

func readFlagValue(args []string, index *int) (string, bool) {
	nextIndex := *index + 1
	if nextIndex >= len(args) {
		return "", false
	}
	*index = nextIndex
	return args[nextIndex], true
}

func migrationFlagDiagnostic(flag string) migrationDiagnostic {
	return migrationDiagnostic{
		Code:    "TSPACK_MIGRATE_INVALID_ARGS",
		Message: flag + " requires a value",
		Fixes:   []string{"Provide a value for " + flag + "."},
	}
}

func buildMigrationDraft(cfg migrateConfig) (migrationDraft, *migrationDiagnostic) {
	pkg, diagnostic := loadPackageJSONForMigration(cfg)
	if diagnostic != nil {
		return migrationDraft{}, diagnostic
	}

	draft := migrationDraft{
		Config:        cfg,
		Package:       pkg,
		Kind:          inferMigrationKind(pkg),
		WorkspaceName: workspaceNameFromPackage(defaultString(pkg.Name, "migrated")),
		TodoCounts:    map[string]int{},
	}
	draft.Dependencies = migrateDependencies(pkg, &draft)
	lockEvidence, lockDiagnostic := loadPackageLockEvidence(cfg, draft.Dependencies)
	if lockDiagnostic != nil {
		return migrationDraft{}, lockDiagnostic
	}
	draft.LockEvidence = lockEvidence
	draft.SourceEvidence = loadSourceScanEvidence(cfg, pkg)
	draft.Diagnostics = migrationDiagnosticsFromLockEvidence(lockEvidence)
	draft.Diagnostics = append(draft.Diagnostics, migrationDiagnosticsFromSourceEvidence(draft.SourceEvidence)...)
	draft.Targets = inferMigrationTargets(pkg, draft.Kind, &draft)
	draft.PublishInclude = inferMigrationPublishInclude(pkg, &draft)
	draft.LifecycleScripts = findMigrationLifecycleScripts(pkg.Scripts)
	draft.ScriptAnalyses = analyzePackageScripts(pkg.Scripts)
	draft.Manifest = renderMigrationManifest(&draft)
	draft.Report = renderMigrationReport(&draft)
	return draft, nil
}

func loadPackageJSONForMigration(cfg migrateConfig) (packageJSONModel, *migrationDiagnostic) {
	content, err := os.ReadFile(cfg.packageJSONPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return packageJSONModel{}, &migrationDiagnostic{
				Code:    "TSPACK_MIGRATE_PACKAGE_JSON_MISSING",
				Message: "package.json was not found",
				Details: []string{
					"root: " + cfg.root,
					"packageJsonPath: " + cfg.packageJSONPath,
				},
				Fixes: []string{"Create package.json, pass --root <root>, or pass --package-json <path>."},
			}
		}
		return packageJSONModel{}, &migrationDiagnostic{
			Code:    "TSPACK_MIGRATE_PACKAGE_JSON_INVALID",
			Message: "package.json could not be read",
			Details: []string{
				"root: " + cfg.root,
				"packageJsonPath: " + cfg.packageJSONPath,
				"error: " + err.Error(),
			},
			Fixes: []string{"Check package.json permissions and retry."},
		}
	}

	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	var raw map[string]any
	if err := decoder.Decode(&raw); err != nil {
		return packageJSONModel{}, &migrationDiagnostic{
			Code:    "TSPACK_MIGRATE_PACKAGE_JSON_INVALID",
			Message: "package.json is not valid JSON",
			Details: []string{
				"root: " + cfg.root,
				"packageJsonPath: " + cfg.packageJSONPath,
				"error: " + err.Error(),
			},
			Fixes: []string{"Fix JSON syntax and rerun `tspack migrate`."},
		}
	}

	pkg := packageJSONModel{
		Name:                 readStringField(raw, "name"),
		Version:              readStringField(raw, "version"),
		License:              readStringField(raw, "license"),
		Type:                 readStringField(raw, "type"),
		Main:                 readStringField(raw, "main"),
		Module:               readStringField(raw, "module"),
		Types:                readStringField(raw, "types"),
		Typings:              readStringField(raw, "typings"),
		Exports:              raw["exports"],
		Files:                readStringArrayField(raw, "files"),
		Dependencies:         readStringMapField(raw, "dependencies"),
		PeerDependencies:     readStringMapField(raw, "peerDependencies"),
		PeerDependenciesMeta: readPeerDependencyMeta(raw["peerDependenciesMeta"]),
		DevDependencies:      readStringMapField(raw, "devDependencies"),
		OptionalDependencies: readStringMapField(raw, "optionalDependencies"),
		Scripts:              readStringMapField(raw, "scripts"),
		Bin:                  raw["bin"],
		Repository:           raw["repository"],
		Homepage:             readStringField(raw, "homepage"),
		Source:               readStringField(raw, "source"),
		Bugs:                 raw["bugs"],
	}
	if privateValue, ok := raw["private"].(bool); ok {
		pkg.Private = privateValue
		pkg.HasPrivate = true
	}
	pkg.InvalidFields = findInvalidMigrationFields(raw)
	return pkg, nil
}

func readStringField(raw map[string]any, field string) string {
	value, ok := raw[field].(string)
	if !ok {
		return ""
	}
	return value
}

func readStringArrayField(raw map[string]any, field string) []string {
	values, ok := raw[field].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, value := range values {
		stringValue, ok := value.(string)
		if ok && stringValue != "" {
			out = append(out, stringValue)
		}
	}
	return out
}

func readStringMapField(raw map[string]any, field string) map[string]string {
	values, ok := raw[field].(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for key, value := range values {
		stringValue, ok := value.(string)
		if ok && stringValue != "" {
			out[key] = stringValue
		}
	}
	return out
}

func readPeerDependencyMeta(value any) map[string]peerDependencyMeta {
	rows, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]peerDependencyMeta{}
	for name, rawMeta := range rows {
		metaObject, ok := rawMeta.(map[string]any)
		if !ok {
			continue
		}
		optional, _ := metaObject["optional"].(bool)
		out[name] = peerDependencyMeta{Optional: optional}
	}
	return out
}

func findInvalidMigrationFields(raw map[string]any) []string {
	var invalid []string
	for _, field := range []string{"dependencies", "peerDependencies", "devDependencies", "optionalDependencies", "scripts"} {
		value, exists := raw[field]
		if !exists {
			continue
		}
		objectValue, ok := value.(map[string]any)
		if !ok {
			invalid = append(invalid, field+" is not an object")
			continue
		}
		for name, rangeValue := range objectValue {
			if _, ok := rangeValue.(string); !ok {
				invalid = append(invalid, field+"."+name+" is not a string")
			}
		}
	}
	sort.Strings(invalid)
	return invalid
}

func inferMigrationKind(pkg packageJSONModel) string {
	if pkg.Private && pkg.Main == "" && pkg.Module == "" && migrationTypesField(pkg) == "" && pkg.Exports == nil {
		return "app"
	}
	return "library"
}

func migrateDependencies(pkg packageJSONModel, draft *migrationDraft) []migratedDependency {
	usedKeys := map[string]string{}
	var deps []migratedDependency
	peerNames := map[string]bool{}
	for _, name := range sortedMapKeys(pkg.PeerDependencies) {
		peerNames[name] = true
		key := migrationIdentifierForPackage(name, usedKeys, draft)
		meta := pkg.PeerDependenciesMeta[name]
		deps = append(deps, migratedDependency{
			Key:          key,
			PackageName:  name,
			Range:        pkg.PeerDependencies[name],
			Kind:         "peer",
			SourceField:  "peerDependencies",
			OptionalPeer: meta.Optional,
		})
	}
	for _, name := range sortedMapKeys(pkg.Dependencies) {
		if peerNames[name] {
			draft.DuplicatePeerDeps = append(draft.DuplicatePeerDeps, name)
			continue
		}
		key := migrationIdentifierForPackage(name, usedKeys, draft)
		deps = append(deps, migratedDependency{
			Key:         key,
			PackageName: name,
			Range:       pkg.Dependencies[name],
			Kind:        "dep",
			SourceField: "dependencies",
			NeedsTODO:   true,
		})
	}
	for _, name := range sortedMapKeys(pkg.OptionalDependencies) {
		if peerNames[name] {
			draft.DuplicatePeerDeps = append(draft.DuplicatePeerDeps, name)
			continue
		}
		key := migrationIdentifierForPackage(name, usedKeys, draft)
		deps = append(deps, migratedDependency{
			Key:         key,
			PackageName: name,
			Range:       pkg.OptionalDependencies[name],
			Kind:        "dep",
			SourceField: "optionalDependencies",
			NeedsTODO:   true,
		})
	}
	for _, name := range sortedMapKeys(pkg.DevDependencies) {
		if peerNames[name] {
			draft.DuplicatePeerDeps = append(draft.DuplicatePeerDeps, name)
			continue
		}
		key := migrationIdentifierForPackage(name, usedKeys, draft)
		knownTool := knownMigrationTools[name]
		deps = append(deps, migratedDependency{
			Key:         key,
			PackageName: name,
			Range:       pkg.DevDependencies[name],
			Kind:        "tool",
			SourceField: "devDependencies",
			KnownTool:   knownTool,
			NeedsTODO:   !knownTool,
		})
	}
	return deps
}

func migrationIdentifierForPackage(packageName string, used map[string]string, draft *migrationDraft) string {
	base := packageNameToIdentifier(packageName)
	key := base
	if key == "" {
		key = "dependency"
	}
	if previous, exists := used[key]; !exists {
		used[key] = packageName
		return key
	} else if previous == packageName {
		return key
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s%d", key, suffix)
		if _, exists := used[candidate]; !exists {
			used[candidate] = packageName
			draft.IdentifierCollisions = append(draft.IdentifierCollisions, fmt.Sprintf("%s -> %s", packageName, candidate))
			return candidate
		}
	}
}

func packageNameToIdentifier(packageName string) string {
	trimmed := strings.TrimPrefix(packageName, "@")
	parts := regexp.MustCompile(`[^A-Za-z0-9]+`).Split(trimmed, -1)
	var cleaned []string
	for _, part := range parts {
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}
	identifier := strings.ToLower(cleaned[0][:1]) + cleaned[0][1:]
	for _, part := range cleaned[1:] {
		identifier += strings.ToUpper(part[:1]) + part[1:]
	}
	if identifier == "" {
		return "dependency"
	}
	if identifier[0] >= '0' && identifier[0] <= '9' {
		identifier = "dep" + identifier
	}
	return identifier
}

func inferMigrationTargets(pkg packageJSONModel, kind string, draft *migrationDraft) []migratedTarget {
	if kind == "app" {
		return []migratedTarget{{
			Name:        "app",
			Export:      ".",
			Entry:       "src/main.ts",
			Runtime:     "dist/main.js",
			Types:       "",
			NeedsTODO:   true,
			Description: "private app placeholder target",
		}}
	}

	targetsByExport := map[string]migratedTarget{}
	if rootExport, ok := simpleExportFromValue(rootExportValue(pkg.Exports)); ok {
		targetsByExport["."] = targetFromExport("core", ".", rootExport, pkg, "root export", true)
	}

	if len(targetsByExport) == 0 && (pkg.Main != "" || pkg.Module != "" || migrationTypesField(pkg) != "") {
		runtime := canonicalMigrationPackagePath(firstNonEmpty(pkg.Module, pkg.Main, "dist/index.js"))
		types := canonicalMigrationPackagePath(firstNonEmpty(migrationTypesField(pkg), "dist/index.d.ts"))
		targetsByExport["."] = migratedTarget{
			Name:        "core",
			Export:      ".",
			Entry:       guessMigrationEntry(runtime, "."),
			Runtime:     runtime,
			Types:       types,
			NeedsTODO:   true,
			Description: "main/module/types fields",
		}
	}

	if len(targetsByExport) == 0 {
		targetsByExport["."] = migratedTarget{
			Name:        "core",
			Export:      ".",
			Entry:       "src/index.ts",
			Runtime:     "dist/index.js",
			Types:       "dist/index.d.ts",
			NeedsTODO:   true,
			Description: "placeholder because package.json did not declare main/types/exports",
		}
	}

	subpathExports := subpathExportValues(pkg.Exports)
	for _, exportName := range sortedAnyMapKeys(subpathExports) {
		if exportName == "." {
			continue
		}
		exportValue := subpathExports[exportName]
		info, ok := simpleExportFromValue(exportValue)
		if !ok || info.Runtime == "" {
			draft.SubpathTodos = append(draft.SubpathTodos, exportName)
			continue
		}
		targetsByExport[exportName] = targetFromExport(targetNameFromExport(exportName), exportName, info, pkg, "subpath export", true)
	}

	keys := make([]string, 0, len(targetsByExport))
	for key := range targetsByExport {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var targets []migratedTarget
	if target, ok := targetsByExport["."]; ok {
		targets = append(targets, target)
	}
	for _, key := range keys {
		if key != "." {
			targets = append(targets, targetsByExport[key])
		}
	}
	return targets
}

func rootExportValue(exports any) any {
	objectValue, ok := exports.(map[string]any)
	if !ok {
		return exports
	}
	if value, exists := objectValue["."]; exists {
		return value
	}
	return nil
}

func subpathExportValues(exports any) map[string]any {
	objectValue, ok := exports.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	for key, value := range objectValue {
		if strings.HasPrefix(key, ".") {
			out[key] = value
		}
	}
	return out
}

func simpleExportFromValue(value any) (simpleExportInfo, bool) {
	if value == nil {
		return simpleExportInfo{}, false
	}
	if stringValue, ok := value.(string); ok {
		return simpleExportInfo{Runtime: stringValue}, true
	}
	objectValue, ok := value.(map[string]any)
	if !ok {
		return simpleExportInfo{}, false
	}
	info := simpleExportInfo{}
	for _, key := range []string{"types", "typings"} {
		if value, ok := objectValue[key].(string); ok && value != "" {
			info.Types = value
			break
		}
	}
	for _, key := range []string{"default", "import", "require", "module", "node"} {
		if value, ok := objectValue[key].(string); ok && value != "" {
			info.Runtime = value
			break
		}
	}
	return info, info.Runtime != "" || info.Types != ""
}

func targetFromExport(name string, exportName string, info simpleExportInfo, pkg packageJSONModel, description string, needsTODO bool) migratedTarget {
	runtime := canonicalMigrationPackagePath(firstNonEmpty(info.Runtime, pkg.Module, pkg.Main, "dist/index.js"))
	types := canonicalMigrationPackagePath(firstNonEmpty(info.Types, migrationTypesField(pkg), "dist/index.d.ts"))
	return migratedTarget{
		Name:        name,
		Export:      exportName,
		Entry:       guessMigrationEntry(runtime, exportName),
		Runtime:     runtime,
		Types:       types,
		NeedsTODO:   needsTODO,
		Description: description,
	}
}

func canonicalMigrationPackagePath(path string) string {
	for strings.HasPrefix(path, "./") {
		path = strings.TrimPrefix(path, "./")
	}
	return path
}

func targetNameFromExport(exportName string) string {
	trimmed := strings.TrimPrefix(exportName, "./")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" || trimmed == "." {
		return "core"
	}
	return packageNameToIdentifier(trimmed)
}

func guessMigrationEntry(runtime string, exportName string) string {
	cleaned := strings.TrimPrefix(runtime, "./")
	if after, ok := strings.CutPrefix(cleaned, "dist/"); ok {
		cleaned = after
	}
	cleaned = strings.TrimSuffix(cleaned, ".cjs")
	cleaned = strings.TrimSuffix(cleaned, ".mjs")
	cleaned = strings.TrimSuffix(cleaned, ".js")
	if cleaned == "" || cleaned == "." {
		cleaned = strings.TrimPrefix(exportName, "./")
	}
	if cleaned == "" || cleaned == "." {
		cleaned = "index"
	}
	return "src/" + cleaned + ".ts"
}

func migrationTypesField(pkg packageJSONModel) string {
	return firstNonEmpty(pkg.Types, pkg.Typings)
}

func inferMigrationPublishInclude(pkg packageJSONModel, draft *migrationDraft) []string {
	if len(pkg.Files) > 0 {
		return append([]string{}, pkg.Files...)
	}
	return []string{"dist/**", "README.md", "LICENSE"}
}

func findMigrationLifecycleScripts(scripts map[string]string) []string {
	var names []string
	for name := range scripts {
		if lifecycleScriptNames[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func analyzePackageScripts(scripts map[string]string) []scriptAnalysis {
	var analyses []scriptAnalysis
	for _, name := range sortedMapKeys(scripts) {
		analysis := analyzePackageScript(name, scripts[name])
		analyses = append(analyses, analysis)
	}
	return analyses
}

func analyzePackageScript(name string, command string) scriptAnalysis {
	analysis := scriptAnalysis{
		Name:       name,
		Command:    command,
		Category:   "unknown",
		Confidence: "low",
		Action:     "review manually; not migrated",
	}

	argv, simple := splitSimpleShellCommand(command)
	analysis.Argv = argv
	if !simple {
		analysis.NeedsReview = true
		analysis.ReviewNotes = append(analysis.ReviewNotes, "shell-composite or shell metacharacters detected")
	}
	if hasEnvPrefix(argv) || hasCrossEnvPrefix(argv) {
		analysis.NeedsReview = true
		analysis.ReviewNotes = append(analysis.ReviewNotes, "environment prefix detected; review RunTarget environment separately")
	}

	commandArgv := scriptCommandArgv(argv)
	commandName := ""
	if len(commandArgv) > 0 {
		commandName = commandArgv[0]
	}

	analysis.Category, analysis.Confidence, analysis.Rationale = classifyScript(name, command, commandArgv)
	analysis.Action = actionForScriptCategory(analysis.Category, analysis.NeedsReview)
	if analysis.Category == "runtime-target-candidate" {
		analysis.Suggestion = suggestRunTargetForScript(name, commandArgv, simple, analysis.ReviewNotes)
		if analysis.Suggestion == nil {
			analysis.NeedsReview = true
			analysis.ReviewNotes = append(analysis.ReviewNotes, "command argv could not be inferred safely")
		}
	}
	if commandName == "" && command != "" {
		analysis.NeedsReview = true
	}
	return analysis
}

func classifyScript(name string, command string, argv []string) (string, string, string) {
	lowerName := strings.ToLower(name)
	commandText := strings.ToLower(command)
	commandName := ""
	secondArg := ""
	if len(argv) > 0 {
		commandName = strings.ToLower(argv[0])
	}
	if len(argv) > 1 {
		secondArg = strings.ToLower(argv[1])
	}

	if lifecycleScriptNames[name] {
		return "lifecycle", "high", "package.json lifecycle script name"
	}
	if isBuildScript(lowerName, commandName, secondArg, commandText) {
		return "build", "high", "build-oriented script name or command"
	}
	if isTestScript(lowerName, commandName, secondArg, commandText) {
		return "test", "high", "test-oriented script name or command"
	}
	if isLintScript(lowerName, commandName, commandText) {
		return "lint", "high", "lint-oriented script name or command"
	}
	if isFormatScript(lowerName, commandName, commandText) {
		return "format", "high", "format-oriented script name or command"
	}
	if isPackageScript(lowerName, commandName, commandText) {
		return "package/publish", "medium", "package or release-oriented script"
	}
	if isRuntimeTargetCandidate(lowerName, commandName, secondArg, commandText) {
		confidence := "medium"
		if commandLooksRuntime(commandName, secondArg, commandText) {
			confidence = "high"
		}
		return "runtime-target-candidate", confidence, "long-running runtime or dev-server command pattern"
	}
	if isMaintenanceScript(lowerName, commandName, commandText) {
		return "maintenance", "medium", "maintenance-oriented script name or command"
	}
	return "unknown", "low", "no conservative RunTarget classification matched"
}

func isBuildScript(name string, commandName string, secondArg string, commandText string) bool {
	if name == "build" || strings.HasPrefix(name, "build:") || strings.HasSuffix(name, ":build") {
		return true
	}
	if commandName == "tsc" || commandName == "tsup" || commandName == "rollup" {
		return true
	}
	if commandName == "webpack" || commandName == "esbuild" {
		return true
	}
	if commandName == "vite" && secondArg == "build" {
		return true
	}
	if commandName == "next" && secondArg == "build" {
		return true
	}
	return strings.Contains(commandText, " build")
}

func isTestScript(name string, commandName string, secondArg string, commandText string) bool {
	if name == "test" || strings.HasPrefix(name, "test:") || strings.Contains(name, ":test") {
		return true
	}
	if commandName == "vitest" || commandName == "jest" || commandName == "playwright" {
		return true
	}
	if commandName == "node" && secondArg == "--test" {
		return true
	}
	return strings.Contains(commandText, " test")
}

func isLintScript(name string, commandName string, commandText string) bool {
	if name == "lint" || strings.HasPrefix(name, "lint:") || strings.Contains(name, ":lint") {
		return true
	}
	return commandName == "eslint" || strings.Contains(commandText, " lint")
}

func isFormatScript(name string, commandName string, commandText string) bool {
	if name == "format" || strings.HasPrefix(name, "format:") || strings.Contains(name, ":format") {
		return true
	}
	return commandName == "prettier" || strings.Contains(commandText, " format")
}

func isPackageScript(name string, commandName string, commandText string) bool {
	if name == "pack" || name == "release" || strings.Contains(name, "publish") {
		return true
	}
	return commandName == "changeset" || strings.Contains(commandText, " changeset") || strings.Contains(commandText, " npm publish")
}

func isRuntimeTargetCandidate(name string, commandName string, secondArg string, commandText string) bool {
	if commandLooksRuntime(commandName, secondArg, commandText) {
		return true
	}
	return name == "dev" || name == "start" || name == "serve" || name == "preview" || name == "storybook" || name == "docs"
}

func commandLooksRuntime(commandName string, secondArg string, commandText string) bool {
	switch commandName {
	case "vite":
		return secondArg != "build"
	case "next", "astro", "remix", "nuxt":
		return secondArg == "dev" || secondArg == "start"
	case "svelte-kit":
		return secondArg == "dev"
	case "storybook":
		return true
	case "vitepress":
		return secondArg == "dev"
	case "docusaurus":
		return secondArg == "start"
	case "node":
		return secondArg != "" && secondArg != "--test"
	default:
		return strings.Contains(commandText, "storybook dev") || strings.Contains(commandText, "vite preview")
	}
}

func isMaintenanceScript(name string, commandName string, commandText string) bool {
	if name == "clean" || name == "typecheck" || name == "check" || name == "generate" || name == "codegen" || name == "docs:build" {
		return true
	}
	return commandName == "rimraf" || strings.Contains(commandText, " typecheck") || strings.Contains(commandText, " codegen")
}

func actionForScriptCategory(category string, needsReview bool) string {
	if category == "runtime-target-candidate" {
		if needsReview {
			return "review as possible RunTarget; shell/env syntax prevents automatic argv confidence"
		}
		return "review suggested RunTarget before enabling"
	}
	if category == "build" || category == "test" || category == "lint" || category == "format" {
		return "keep as non-RunTarget evidence; map to TSPack checks only when explicit support exists"
	}
	if category == "lifecycle" {
		return "security review; not executed by tspack migrate"
	}
	return "review manually; not migrated"
}

func suggestRunTargetForScript(name string, argv []string, simple bool, reviewNotes []string) *runTargetSuggestion {
	if len(argv) == 0 {
		return nil
	}
	suggestion := &runTargetSuggestion{
		Name:       name,
		Command:    append([]string{}, argv...),
		Cwd:        "workspace",
		Confidence: "medium",
		Notes:      []string{"verify command argv, cwd, url, and readiness before enabling"},
	}
	if !simple {
		suggestion.Command = nil
		suggestion.Confidence = "low"
	}
	suggestion.Notes = append(suggestion.Notes, reviewNotes...)

	commandName := strings.ToLower(argv[0])
	secondArg := ""
	if len(argv) > 1 {
		secondArg = strings.ToLower(argv[1])
	}
	port := inferPortFromArgv(argv)
	if port == "" {
		switch {
		case commandName == "vite" && secondArg != "build":
			port = "5173"
		case commandName == "next" && secondArg == "dev":
			port = "3000"
		case commandName == "storybook" || strings.Contains(strings.Join(argv, " "), "storybook"):
			port = "6006"
		}
	}
	if port != "" {
		suggestion.URL = "http://127.0.0.1:" + port
		suggestion.Ready = "http /"
	} else {
		suggestion.Ready = "TODO"
		suggestion.Notes = append(suggestion.Notes, "readiness is unknown without executing the script")
	}
	if commandLooksRuntime(commandName, secondArg, strings.ToLower(strings.Join(argv, " "))) && simple {
		suggestion.Confidence = "high"
	}
	return suggestion
}

func inferPortFromArgv(argv []string) string {
	for index, arg := range argv {
		if arg == "--port" || arg == "-p" {
			if index+1 < len(argv) && isDecimalPort(argv[index+1]) {
				return argv[index+1]
			}
		}
		if strings.HasPrefix(arg, "--port=") {
			value := strings.TrimPrefix(arg, "--port=")
			if isDecimalPort(value) {
				return value
			}
		}
	}
	return ""
}

func isDecimalPort(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func splitSimpleShellCommand(command string) ([]string, bool) {
	var argv []string
	var current strings.Builder
	quote := rune(0)
	escaped := false
	simple := true
	for _, ch := range command {
		if escaped {
			current.WriteRune(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			} else {
				current.WriteRune(ch)
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if ch == '&' || ch == '|' || ch == ';' || ch == '>' || ch == '<' || ch == '`' {
			simple = false
		}
		if ch == '$' {
			simple = false
		}
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			if current.Len() > 0 {
				argv = append(argv, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(ch)
	}
	if quote != 0 || escaped {
		simple = false
	}
	if current.Len() > 0 {
		argv = append(argv, current.String())
	}
	return argv, simple
}

func scriptCommandArgv(argv []string) []string {
	index := 0
	if index < len(argv) && argv[index] == "cross-env" {
		index++
	}
	for index < len(argv) && isEnvAssignment(argv[index]) {
		index++
	}
	if index >= len(argv) {
		return nil
	}
	return append([]string{}, argv[index:]...)
}

func hasEnvPrefix(argv []string) bool {
	return len(argv) > 0 && isEnvAssignment(argv[0])
}

func hasCrossEnvPrefix(argv []string) bool {
	return len(argv) > 0 && argv[0] == "cross-env"
}

func isEnvAssignment(value string) bool {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 || parts[0] == "" {
		return false
	}
	for index, ch := range parts[0] {
		if index == 0 && ch >= '0' && ch <= '9' {
			return false
		}
		if isEnvNameRune(ch) {
			continue
		}
		return false
	}
	return true
}

func isEnvNameRune(ch rune) bool {
	if ch == '_' {
		return true
	}
	if ch >= 'A' && ch <= 'Z' {
		return true
	}
	if ch >= 'a' && ch <= 'z' {
		return true
	}
	return ch >= '0' && ch <= '9'
}

func renderMigrationManifest(draft *migrationDraft) string {
	countMigrationTodos(draft)
	var builder strings.Builder
	builder.WriteString("import {\n")
	builder.WriteString("  define,\n")
	builder.WriteString("  defineDeps,\n")
	builder.WriteString("  dep,\n")
	builder.WriteString("  npm,\n")
	builder.WriteString("  peer,\n")
	builder.WriteString("  tool,\n")
	builder.WriteString("  Package,\n")
	builder.WriteString("  Policies,\n")
	builder.WriteString("  Publish,\n")
	builder.WriteString("  Targets,\n")
	builder.WriteString("  Tools,\n")
	builder.WriteString("  Workspace,\n")
	builder.WriteString("  type BoundaryPolicy,\n")
	builder.WriteString("  type TypePolicy,\n")
	builder.WriteString("} from \"tspack/manifest\";\n\n")
	builder.WriteString("/**\n")
	builder.WriteString(" * Generated by `tspack migrate`.\n")
	builder.WriteString(" *\n")
	builder.WriteString(" * This manifest is a mechanical migration draft.\n")
	builder.WriteString(" * Review all MIGRATION_TODO_* comments before treating it as authoritative.\n")
	builder.WriteString(" */\n\n")

	if len(sourceMissingPackages(draft.SourceEvidence)) > 0 {
		builder.WriteString("// MIGRATION_TODO_DEP_CLASSIFICATION:\n")
		builder.WriteString("// Source scan observed undeclared external imports: ")
		builder.WriteString(formatSourcePackageList(sourceMissingPackages(draft.SourceEvidence)))
		builder.WriteString(". Add dependencies or verify aliases/internal resolution.\n\n")
	}

	builder.WriteString("const deps = defineDeps({\n")
	for _, dep := range draft.Dependencies {
		sourcePkg, hasSourceEvidence := sourceEvidenceForDependency(draft, dep.PackageName)
		if dep.NeedsTODO || sourceDependencyNeedsTODO(sourcePkg, hasSourceEvidence) {
			builder.WriteString("  // MIGRATION_TODO_DEP_CLASSIFICATION:\n")
			if hasSourceEvidence && dep.SourceField == "devDependencies" && sourcePackageHasRuntimeUse(sourcePkg) {
				builder.WriteString("  // Source scan found runtime imports while package.json declares this as a devDependency.\n")
				builder.WriteString("  // Review whether this should be runtime dep, peer, or tool-only.\n")
			} else if dep.SourceField == "optionalDependencies" {
				builder.WriteString("  // optionalDependency semantics require review; TSPack dependency intent is explicit.\n")
			} else if dep.SourceField == "devDependencies" {
				builder.WriteString("  // devDependency classified as tool mechanically; verify if this should be test/build-only policy.\n")
			} else {
				builder.WriteString("  // This dependency was classified mechanically from package.json.\n")
				builder.WriteString("  // Verify whether it belongs to a specific target.\n")
			}
		}
		if hasSourceEvidence {
			builder.WriteString("  // Source evidence: ")
			builder.WriteString(sourceObservedUsage(sourcePkg))
			builder.WriteString(" imports found in ")
			builder.WriteString(sourceFileListForComment(sourcePkg.Files))
			builder.WriteString(".\n")
			if sourcePackageIsTypeOnly(sourcePkg) {
				builder.WriteString("  // MIGRATION_TODO_TYPES: Source scan only found type-only imports; verify type dependency intent.\n")
			}
		}
		builder.WriteString("  ")
		builder.WriteString(dep.Key)
		builder.WriteString(": ")
		builder.WriteString(renderDependencyCall(dep))
		builder.WriteString(",\n")
	}
	builder.WriteString("});\n\n")

	builder.WriteString("// MIGRATION_TODO_BOUNDARIES:\n")
	builder.WriteString("// Boundaries use TSPack strict defaults. Verify package-specific target and source boundaries.\n")
	builder.WriteString("const boundaries = {\n")
	builder.WriteString("  undeclaredImports: \"error\",\n")
	builder.WriteString("  phantomDependencies: \"error\",\n")
	builder.WriteString("  crossTargetImports: \"error\",\n")
	builder.WriteString("} satisfies BoundaryPolicy;\n\n")

	builder.WriteString("// MIGRATION_TODO_TYPES:\n")
	builder.WriteString("// Type policy is strict by default. Verify declaration output and public type leakage expectations.\n")
	builder.WriteString("const types = {\n")
	if draft.Kind == "app" {
		builder.WriteString("  declarations: \"optional\",\n")
		builder.WriteString("  missingTypes: \"ignore\",\n")
		builder.WriteString("  publicTypeLeakage: \"warn\",\n")
	} else {
		builder.WriteString("  declarations: \"required\",\n")
		builder.WriteString("  missingTypes: \"error\",\n")
		builder.WriteString("  publicTypeLeakage: \"error\",\n")
	}
	builder.WriteString("  typeOnlyRuntimeLeakage: \"error\",\n")
	builder.WriteString("} satisfies TypePolicy;\n\n")

	if len(draft.Package.Scripts) > 0 {
		builder.WriteString("// MIGRATION_TODO_RUN_TARGETS:\n")
		builder.WriteString("// package.json scripts are not migrated automatically.\n")
		builder.WriteString("// TSPack RunTargets are runtime targets, not arbitrary scripts.\n")
		builder.WriteString("// Review ")
		builder.WriteString(strconv.Itoa(countScriptAnalysesByCategory(draft.ScriptAnalyses, "runtime-target-candidate")))
		builder.WriteString(" runtime candidate(s) and ")
		builder.WriteString(strconv.Itoa(countScriptAnalysesNeedingReview(draft.ScriptAnalyses)))
		builder.WriteString(" shell/env/unknown script(s) in tspack-migration.md.\n\n")
	}
	if len(draft.LifecycleScripts) > 0 {
		builder.WriteString("// MIGRATION_TODO_SECURITY:\n")
		builder.WriteString("// lifecycle scripts are executable capabilities; review before acknowledging any capability.\n")
		builder.WriteString("// `tspack migrate` does not execute package.json scripts.\n\n")
	}

	builder.WriteString("export default define(\n")
	builder.WriteString("  <Workspace name=")
	builder.WriteString(quoteTSString(draft.WorkspaceName))
	builder.WriteString(">\n")
	builder.WriteString("    <Package\n")
	builder.WriteString("      name=")
	builder.WriteString(quoteTSString(defaultString(draft.Package.Name, "migrated-package")))
	builder.WriteString("\n")
	builder.WriteString("      version=")
	builder.WriteString(quoteTSString(defaultString(draft.Package.Version, "0.0.0")))
	builder.WriteString("\n")
	if draft.Package.License != "" {
		builder.WriteString("      license=")
		builder.WriteString(quoteTSString(draft.Package.License))
		builder.WriteString("\n")
	}
	builder.WriteString("      kind=")
	builder.WriteString(quoteTSString(draft.Kind))
	builder.WriteString("\n")
	builder.WriteString("      dependencies={{ values: [")
	builder.WriteString(joinDependencyRefs(draft.Dependencies, ""))
	builder.WriteString("] }}\n")
	builder.WriteString("    >\n")
	builder.WriteString("      <Policies types={types} boundaries={boundaries} />\n\n")
	builder.WriteString("      <Targets\n")
	builder.WriteString("        rows={[\n")
	for _, target := range draft.Targets {
		if target.NeedsTODO {
			builder.WriteString("          // MIGRATION_TODO_TARGETS:\n")
			builder.WriteString("          // Target was inferred from package.json ")
			builder.WriteString(target.Description)
			builder.WriteString(".\n")
			builder.WriteString("          // Verify entry/runtime/types and target name.\n")
		}
		builder.WriteString("          {\n")
		builder.WriteString("            name: ")
		builder.WriteString(quoteTSString(target.Name))
		builder.WriteString(",\n")
		builder.WriteString("            export: ")
		builder.WriteString(quoteTSString(target.Export))
		builder.WriteString(",\n")
		builder.WriteString("            entry: ")
		builder.WriteString(quoteTSString(target.Entry))
		builder.WriteString(",\n")
		builder.WriteString("            runtime: ")
		builder.WriteString(quoteTSString(target.Runtime))
		builder.WriteString(",\n")
		builder.WriteString("            types: ")
		builder.WriteString(quoteTSString(target.Types))
		builder.WriteString(",\n")
		builder.WriteString("            deps: [")
		builder.WriteString(joinDependencyIdentityRefs(draft.Dependencies, "dep"))
		builder.WriteString("],\n")
		builder.WriteString("            peers: [")
		builder.WriteString(joinDependencyIdentityRefs(draft.Dependencies, "peer"))
		builder.WriteString("],\n")
		builder.WriteString("            optional: false,\n")
		builder.WriteString("          },\n")
	}
	for _, exportName := range draft.SubpathTodos {
		builder.WriteString("          // MIGRATION_TODO_TARGETS: unsupported or complex subpath export ")
		builder.WriteString(quoteTSString(exportName))
		builder.WriteString(" was reported but not converted.\n")
	}
	builder.WriteString("        ]}\n")
	builder.WriteString("      />\n\n")
	builder.WriteString("      <Tools values={[")
	builder.WriteString(joinDependencyRefs(draft.Dependencies, "tool"))
	builder.WriteString("]} />\n\n")
	builder.WriteString("      {/* MIGRATION_TODO_PUBLISH:\n")
	builder.WriteString("          Publish include was inferred from package.json files or a conservative default.\n")
	builder.WriteString("          Verify package contents with `tspack pack --dry-run`. */}\n")
	builder.WriteString("      <Publish\n")
	builder.WriteString("        include={")
	builder.WriteString(renderStringArray(draft.PublishInclude))
	builder.WriteString("}\n")
	builder.WriteString("        exclude={[]}\n")
	builder.WriteString("      />\n")
	builder.WriteString("    </Package>\n")
	builder.WriteString("  </Workspace>,\n")
	builder.WriteString(");\n")
	return builder.String()
}

func renderDependencyCall(dep migratedDependency) string {
	source := "npm(" + quoteTSString(dep.PackageName) + ", " + quoteTSString(dep.Range) + ")"
	options := renderDependencyOptions(dep)
	if options != "" {
		return dep.Kind + "(" + source + ", " + options + ")"
	}
	return dep.Kind + "(" + source + ")"
}

func renderDependencyOptions(dep migratedDependency) string {
	var options []string
	if dep.Key != dep.PackageName {
		options = append(options, "key: "+quoteTSString(dep.PackageName))
	}
	if dep.Kind == "peer" && dep.OptionalPeer {
		options = append(options, "optional: true")
	}
	if len(options) == 0 {
		return ""
	}
	return "{ " + strings.Join(options, ", ") + " }"
}

func joinDependencyRefs(deps []migratedDependency, kind string) string {
	var refs []string
	for _, dep := range deps {
		if kind == "" || dep.Kind == kind {
			refs = append(refs, "deps."+dep.Key)
		}
	}
	return strings.Join(refs, ", ")
}

func joinDependencyIdentityRefs(deps []migratedDependency, kind string) string {
	var refs []string
	for _, dep := range deps {
		if kind == "" || dep.Kind == kind {
			refs = append(refs, quoteTSString(dep.PackageName))
		}
	}
	return strings.Join(refs, ", ")
}

func renderMigrationReport(draft *migrationDraft) string {
	countMigrationTodos(draft)
	var builder strings.Builder
	builder.WriteString("# TSPack Migration Report\n\n")
	builder.WriteString("## Inputs\n")
	builder.WriteString("- root: `")
	builder.WriteString(draft.Config.root)
	builder.WriteString("`\n")
	builder.WriteString("- package.json path: `")
	builder.WriteString(draft.Config.packageJSONPath)
	builder.WriteString("`\n")
	builder.WriteString("- package-lock evidence: ")
	builder.WriteString(migrationLockInputSummary(draft.LockEvidence))
	builder.WriteString("\n")
	builder.WriteString("- source scan: ")
	builder.WriteString(migrationSourceInputSummary(draft.SourceEvidence))
	builder.WriteString("\n")
	builder.WriteString("- generated manifest path: `")
	builder.WriteString(draft.Config.outManifestPath)
	builder.WriteString("`\n")
	builder.WriteString("- generated report path: `")
	builder.WriteString(draft.Config.outReportPath)
	builder.WriteString("`\n\n")

	builder.WriteString("## Summary\n")
	builder.WriteString("- package: `")
	builder.WriteString(packageSummary(draft.Package))
	builder.WriteString("`\n")
	builder.WriteString("- inferred kind: `")
	builder.WriteString(draft.Kind)
	builder.WriteString("`\n")
	builder.WriteString(fmt.Sprintf("- dependency counts: %d runtime, %d peer, %d tool\n", countDepsByKind(draft.Dependencies, "dep"), countDepsByKind(draft.Dependencies, "peer"), countDepsByKind(draft.Dependencies, "tool")))
	builder.WriteString(fmt.Sprintf("- generated target count: %d\n", len(draft.Targets)))
	builder.WriteString(fmt.Sprintf("- TODO count: %d\n", draft.TotalTodos))
	if len(draft.Package.InvalidFields) > 0 {
		builder.WriteString("- unsupported package shapes were skipped with TODOs: ")
		builder.WriteString(strings.Join(draft.Package.InvalidFields, "; "))
		builder.WriteString("\n")
	}
	builder.WriteString("\n")

	builder.WriteString("## Mechanical mappings\n")
	builder.WriteString("| package.json field | TSPack draft mapping |\n")
	builder.WriteString("|---|---|\n")
	builder.WriteString("| `package.name` | `Package.name` |\n")
	builder.WriteString("| `package.version` | `Package.version` |\n")
	builder.WriteString("| `package.license` | `Package.license` |\n")
	builder.WriteString("| `dependencies` | `dep(npm(...))` unless also declared as peer |\n")
	builder.WriteString("| `peerDependencies` | `peer(npm(...))` |\n")
	builder.WriteString("| `devDependencies` | `tool(npm(...))` |\n")
	builder.WriteString("| `main` / `module` / `types` / `exports` | `Targets` rows with TODO review |\n")
	builder.WriteString("| `files` | `Publish include` |\n\n")

	if len(draft.Dependencies) > 0 {
		builder.WriteString("### Dependency rows\n")
		builder.WriteString("| key | package | range | kind | source |\n")
		builder.WriteString("|---|---|---|---|---|\n")
		for _, dep := range draft.Dependencies {
			builder.WriteString("| `")
			builder.WriteString(dep.Key)
			builder.WriteString("` | `")
			builder.WriteString(dep.PackageName)
			builder.WriteString("` | `")
			builder.WriteString(dep.Range)
			builder.WriteString("` | `")
			builder.WriteString(dep.Kind)
			builder.WriteString("` | `")
			builder.WriteString(dep.SourceField)
			builder.WriteString("` |\n")
		}
		builder.WriteString("\n")
	}
	if len(draft.DuplicatePeerDeps) > 0 {
		builder.WriteString("Duplicate dependency/peer declarations preferred peer classification for: `")
		builder.WriteString(strings.Join(draft.DuplicatePeerDeps, "`, `"))
		builder.WriteString("`.\n\n")
	}
	if len(draft.IdentifierCollisions) > 0 {
		builder.WriteString("Identifier collisions were resolved deterministically:\n")
		for _, collision := range draft.IdentifierCollisions {
			builder.WriteString("- ")
			builder.WriteString(collision)
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	renderPackageLockEvidenceSection(&builder, draft.LockEvidence)
	renderSourceImportEvidenceSection(&builder, draft)
	renderMigrationValidationSection(&builder, draft)
	builder.WriteString("## TODOs for human/LLM review\n")
	for _, todo := range migrationTodoOrder {
		builder.WriteString("### ")
		builder.WriteString(todo)
		builder.WriteString("\n")
		for _, message := range todoMessagesForReport(todo, draft) {
			builder.WriteString("- ")
			builder.WriteString(message)
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	renderScriptSuggestionsSection(&builder, draft)

	if len(draft.LifecycleScripts) > 0 || len(draft.LockEvidence.LifecycleScripts) > 0 {
		builder.WriteString("## Security\n")
		if len(draft.LifecycleScripts) > 0 {
			builder.WriteString("package.json lifecycle scripts detected and not executed: `")
			builder.WriteString(strings.Join(draft.LifecycleScripts, "`, `"))
			builder.WriteString("`. Review before acknowledging capabilities.\n")
		}
		if len(draft.LockEvidence.LifecycleScripts) > 0 {
			builder.WriteString(fmt.Sprintf("Lock evidence detected %d dependency lifecycle script capabilities. TSPack will not execute them by default.\n", len(draft.LockEvidence.LifecycleScripts)))
		}
		builder.WriteString("\n")
	}

	builder.WriteString("## Suggested next steps\n")
	if !draft.Config.check {
		builder.WriteString("- Run: `tspack migrate --check`.\n")
	}
	builder.WriteString("- Review `")
	builder.WriteString(filepath.Base(draft.Config.outManifestPath))
	builder.WriteString("`.\n")
	builder.WriteString("- Resolve `MIGRATION_TODO_*` comments.\n")
	builder.WriteString("- Run: `tspack check --manifest ")
	builder.WriteString(filepath.Base(draft.Config.outManifestPath))
	builder.WriteString("`.\n")
	builder.WriteString("- Run: `tspack update --manifest ")
	builder.WriteString(filepath.Base(draft.Config.outManifestPath))
	builder.WriteString("`.\n")
	builder.WriteString("- Run: `tspack pack --dry-run --manifest ")
	builder.WriteString(filepath.Base(draft.Config.outManifestPath))
	builder.WriteString("`.\n")
	builder.WriteString("\nThis report does not claim migration is complete. It is a mechanical draft for human/LLM review.\n")
	return builder.String()
}

func renderMigrationValidationSection(builder *strings.Builder, draft *migrationDraft) {
	builder.WriteString("## Validation\n\n")
	if !draft.Validation.Ran {
		builder.WriteString("Status: not run\n\n")
		builder.WriteString("- Manifest frontend: not run\n")
		builder.WriteString("- Manifest IR validation: not run\n")
		builder.WriteString(fmt.Sprintf("- Remaining TODOs: %d\n\n", draft.TotalTodos))
		builder.WriteString("Run `tspack migrate --check` to validate the generated draft. Validation means the draft is structurally valid, not semantically complete.\n\n")
		return
	}

	status := "failed"
	if draft.Validation.Passed {
		status = "passed"
	}
	builder.WriteString("Status: ")
	builder.WriteString(status)
	builder.WriteString("\n\n")
	builder.WriteString("- Manifest frontend: ")
	builder.WriteString(draft.Validation.ManifestFrontend)
	builder.WriteString("\n")
	appendValidationDiagnostics(builder, draft.Validation.FrontendDiagnostics)
	builder.WriteString("- Manifest IR validation: ")
	builder.WriteString(draft.Validation.ManifestIR)
	builder.WriteString("\n")
	appendValidationDiagnostics(builder, draft.Validation.IRDiagnostics)
	builder.WriteString(fmt.Sprintf("- Remaining TODOs: %d\n\n", draft.Validation.RemainingTodos))
	builder.WriteString("This means the draft is structurally valid, not semantically complete. Review MIGRATION_TODO_* comments before using it as authoritative.\n\n")
}

func appendValidationDiagnostics(builder *strings.Builder, diagnostics []diag.Diagnostic) {
	for _, diagnostic := range diagnostics {
		message := strings.TrimSpace(diagnostic.Message)
		if message == "" {
			message = string(diagnostic.Severity)
		}
		builder.WriteString("  - `")
		builder.WriteString(diagnostic.Code)
		builder.WriteString("`: ")
		builder.WriteString(escapeMarkdownTable(message))
		builder.WriteString("\n")
	}
}

func renderScriptSuggestionsSection(builder *strings.Builder, draft *migrationDraft) {
	builder.WriteString("## Scripts and RunTarget suggestions\n\n")
	builder.WriteString("`tspack migrate` did not execute scripts and did not convert npm scripts into active RunTargets. RunTargets describe declared runtime processes, not arbitrary build/test/lint/package commands.\n\n")

	builder.WriteString("### Scripts not migrated\n\n")
	if len(draft.ScriptAnalyses) == 0 {
		builder.WriteString("No package.json scripts were present.\n\n")
	} else {
		builder.WriteString("| script | category | command | action |\n")
		builder.WriteString("|---|---|---|---|\n")
		for _, analysis := range draft.ScriptAnalyses {
			builder.WriteString("| `")
			builder.WriteString(escapeMarkdownTable(analysis.Name))
			builder.WriteString("` | `")
			builder.WriteString(analysis.Category)
			builder.WriteString("` | `")
			builder.WriteString(escapeMarkdownTable(analysis.Command))
			builder.WriteString("` | ")
			builder.WriteString(escapeMarkdownTable(analysis.Action))
			builder.WriteString(" |\n")
		}
		builder.WriteString("\nRaw script evidence:\n")
		for _, analysis := range draft.ScriptAnalyses {
			builder.WriteString("- `")
			builder.WriteString(analysis.Name)
			builder.WriteString("`: `")
			builder.WriteString(analysis.Command)
			builder.WriteString("`\n")
		}
		builder.WriteString("\n")
	}

	builder.WriteString("### Suggested RunTarget candidates\n\n")
	candidates := runtimeScriptCandidates(draft.ScriptAnalyses)
	if len(candidates) == 0 {
		builder.WriteString("No likely long-running runtime/dev-server scripts were classified as RunTarget candidates.\n\n")
	} else {
		builder.WriteString("| script | suggested target | confidence | command argv | url | ready | notes |\n")
		builder.WriteString("|---|---|---|---|---|---|---|\n")
		for _, analysis := range candidates {
			suggestion := analysis.Suggestion
			argv := "TODO review command string"
			url := "TODO"
			ready := "TODO"
			notes := []string{"review cwd/url/readiness"}
			name := analysis.Name
			confidence := analysis.Confidence
			if suggestion != nil {
				name = suggestion.Name
				confidence = suggestion.Confidence
				if len(suggestion.Command) > 0 {
					argv = renderJSONLikeStringArray(suggestion.Command)
				}
				if suggestion.URL != "" {
					url = suggestion.URL
				}
				if suggestion.Ready != "" {
					ready = suggestion.Ready
				}
				notes = suggestion.Notes
			}
			builder.WriteString("| `")
			builder.WriteString(escapeMarkdownTable(analysis.Name))
			builder.WriteString("` | `")
			builder.WriteString(escapeMarkdownTable(name))
			builder.WriteString("` | `")
			builder.WriteString(confidence)
			builder.WriteString("` | `")
			builder.WriteString(escapeMarkdownTable(argv))
			builder.WriteString("` | `")
			builder.WriteString(escapeMarkdownTable(url))
			builder.WriteString("` | `")
			builder.WriteString(escapeMarkdownTable(ready))
			builder.WriteString("` | ")
			builder.WriteString(escapeMarkdownTable(strings.Join(uniqueStrings(notes), "; ")))
			builder.WriteString(" |\n")
		}
		builder.WriteString("\n")
	}

	builder.WriteString("### Non-RunTarget scripts\n\n")
	nonRuntime := nonRuntimeScriptAnalyses(draft.ScriptAnalyses)
	if len(nonRuntime) == 0 {
		builder.WriteString("No build/test/lint/format/package/maintenance scripts were classified.\n\n")
	} else {
		builder.WriteString("Build/test/lint/format/package scripts are not RunTargets. Use TSPack check/format/lint/test surfaces where appropriate, and keep external tooling until an explicit TSPack command exists.\n\n")
		for _, analysis := range nonRuntime {
			builder.WriteString("- `")
			builder.WriteString(analysis.Name)
			builder.WriteString("` (`")
			builder.WriteString(analysis.Category)
			builder.WriteString("`): `")
			builder.WriteString(analysis.Command)
			builder.WriteString("`\n")
		}
		builder.WriteString("\n")
	}

	builder.WriteString("### Shell/Env review\n\n")
	reviewScripts := reviewScriptAnalyses(draft.ScriptAnalyses)
	if len(reviewScripts) == 0 {
		builder.WriteString("No shell-composite or environment-prefix script review items were detected.\n\n")
	} else {
		for _, analysis := range reviewScripts {
			builder.WriteString("- `")
			builder.WriteString(analysis.Name)
			builder.WriteString("`: `")
			builder.WriteString(analysis.Command)
			builder.WriteString("` — ")
			builder.WriteString(strings.Join(uniqueStrings(analysis.ReviewNotes), "; "))
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
}

func runtimeScriptCandidates(analyses []scriptAnalysis) []scriptAnalysis {
	var out []scriptAnalysis
	for _, analysis := range analyses {
		if analysis.Category == "runtime-target-candidate" {
			out = append(out, analysis)
		}
	}
	return out
}

func nonRuntimeScriptAnalyses(analyses []scriptAnalysis) []scriptAnalysis {
	var out []scriptAnalysis
	for _, analysis := range analyses {
		if analysis.Category != "runtime-target-candidate" && analysis.Category != "unknown" && analysis.Category != "lifecycle" {
			out = append(out, analysis)
		}
	}
	return out
}

func reviewScriptAnalyses(analyses []scriptAnalysis) []scriptAnalysis {
	var out []scriptAnalysis
	for _, analysis := range analyses {
		if analysis.NeedsReview || analysis.Category == "unknown" {
			out = append(out, analysis)
		}
	}
	return out
}

func countScriptAnalysesByCategory(analyses []scriptAnalysis, category string) int {
	count := 0
	for _, analysis := range analyses {
		if analysis.Category == category {
			count++
		}
	}
	return count
}

func countScriptAnalysesNeedingReview(analyses []scriptAnalysis) int {
	return len(reviewScriptAnalyses(analyses))
}

func countRunTargetTodos(analyses []scriptAnalysis) int {
	if len(analyses) == 0 {
		return 0
	}
	count := countScriptAnalysesByCategory(analyses, "runtime-target-candidate")
	count += countScriptAnalysesByCategory(analyses, "unknown")
	count += countScriptAnalysesNeedingReview(analyses)
	if count == 0 {
		return 1
	}
	return count
}

func runTargetTodoMessages(draft *migrationDraft) []string {
	if len(draft.ScriptAnalyses) == 0 {
		return []string{"No package.json scripts were present."}
	}
	messages := []string{
		fmt.Sprintf("package.json scripts are classified below but not migrated. RunTargets describe runtime processes, not arbitrary build/test/lint scripts. Runtime candidates: %d. Shell/env/unknown review items: %d.", countScriptAnalysesByCategory(draft.ScriptAnalyses, "runtime-target-candidate"), countScriptAnalysesNeedingReview(draft.ScriptAnalyses)),
	}
	if countScriptAnalysesByCategory(draft.ScriptAnalyses, "runtime-target-candidate") > 0 {
		messages = append(messages, "Review suggested RunTarget command argv, cwd, url, and readiness before enabling any target.")
	}
	return messages
}

func renderJSONLikeStringArray(values []string) string {
	var parts []string
	for _, value := range values {
		parts = append(parts, strconv.Quote(value))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func escapeMarkdownTable(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func countMigrationTodos(draft *migrationDraft) {
	counts := map[string]int{}
	counts[migrationTodoTargets] = 0
	for _, target := range draft.Targets {
		if target.NeedsTODO {
			counts[migrationTodoTargets]++
		}
	}
	counts[migrationTodoTargets] += len(draft.SubpathTodos)
	counts[migrationTodoDepClassification] = 0
	for _, dep := range draft.Dependencies {
		if dep.NeedsTODO {
			counts[migrationTodoDepClassification]++
		}
	}
	if len(draft.DuplicatePeerDeps) > 0 || len(draft.IdentifierCollisions) > 0 || len(draft.Package.InvalidFields) > 0 {
		counts[migrationTodoDepClassification]++
	}
	if len(draft.LockEvidence.LifecycleScripts) > 0 || len(draft.LockEvidence.Binaries) > 0 || hasLargeFanoutEvidence(draft.LockEvidence.Fanout) {
		counts[migrationTodoDepClassification]++
	}
	counts[migrationTodoDepClassification] += sourceScanDevRuntimeMismatchCount(draft.SourceEvidence)
	counts[migrationTodoDepClassification] += sourceScanMissingDeclarationCount(draft.SourceEvidence)
	counts[migrationTodoRunTargets] = countRunTargetTodos(draft.ScriptAnalyses)
	counts[migrationTodoPublish] = 1
	counts[migrationTodoBoundaries] = 1
	counts[migrationTodoTypes] = 1 + sourceScanTypeOnlyCandidateCount(draft.SourceEvidence)
	counts[migrationTodoSecurity] = boolToCount(len(draft.LifecycleScripts) > 0 || len(draft.LockEvidence.LifecycleScripts) > 0)
	if counts[migrationTodoTargets] == 0 {
		counts[migrationTodoTargets] = 1
	}
	draft.TodoCounts = counts
	total := 0
	for _, todo := range migrationTodoOrder {
		total += counts[todo]
	}
	draft.TotalTodos = total
}

func todoMessagesForReport(todo string, draft *migrationDraft) []string {
	switch todo {
	case migrationTodoTargets:
		messages := []string{"Targets were inferred from package.json main/types/exports. Verify target names, source entries, runtime outputs, and declaration outputs."}
		for _, exportName := range draft.SubpathTodos {
			messages = append(messages, "Complex subpath export `"+exportName+"` was reported but not converted.")
		}
		for _, hint := range sourceTargetHints(draft) {
			messages = append(messages, hint)
		}
		return messages
	case migrationTodoDepClassification:
		messages := []string{"Dependency kind and target scope were classified mechanically from package.json fields."}
		if len(draft.DuplicatePeerDeps) > 0 {
			messages = append(messages, "Dependencies also present in peerDependencies were kept as peers: `"+strings.Join(draft.DuplicatePeerDeps, "`, `")+"`.")
		}
		if len(draft.IdentifierCollisions) > 0 {
			messages = append(messages, "Identifier collisions were resolved with numeric suffixes.")
		}
		if len(draft.Package.InvalidFields) > 0 {
			messages = append(messages, "Some malformed dependency/script fields were skipped: "+strings.Join(draft.Package.InvalidFields, "; ")+".")
		}
		if hasLargeFanoutEvidence(draft.LockEvidence.Fanout) {
			messages = append(messages, "Lock evidence shows large transitive fanout for one or more direct dependencies. Review dependency classification before treating tool/runtime boundaries as final.")
		}
		if len(draft.LockEvidence.Binaries) > 0 || len(draft.LockEvidence.LifecycleScripts) > 0 {
			messages = append(messages, "Lock evidence includes binary or lifecycle capabilities. Review whether these packages are tooling, runtime dependencies, or security-sensitive setup dependencies.")
		}
		if packages := sourceRuntimeDevPackages(draft.SourceEvidence); len(packages) > 0 {
			messages = append(messages, "Source scan found runtime imports declared only in devDependencies: "+formatSourcePackageList(packages)+". Review whether each is runtime, peer, or tool-only.")
		}
		if packages := sourceMissingPackages(draft.SourceEvidence); len(packages) > 0 {
			messages = append(messages, "Source scan found imported packages missing from direct package.json declarations: "+formatSourcePackageList(packages)+". Add dependencies or verify alias/internal resolution.")
		}
		return messages
	case migrationTodoRunTargets:
		return runTargetTodoMessages(draft)
	case migrationTodoPublish:
		return []string{"Publish include was inferred from package.json files or a conservative default. Verify with `tspack pack --dry-run`."}
	case migrationTodoBoundaries:
		return []string{"Strict boundary defaults were emitted. Review target/source-specific policy before relying on them."}
	case migrationTodoTypes:
		messages := []string{"Type policy was generated from package kind, not from source analysis. Verify declarations and type leakage expectations."}
		if len(draft.LockEvidence.TypePackageNames) > 0 {
			messages = append(messages, "Lock evidence includes @types packages: `"+strings.Join(draft.LockEvidence.TypePackageNames, "`, `")+"`.")
		}
		if packages := sourceTypeOnlyPackages(draft.SourceEvidence); len(packages) > 0 {
			messages = append(messages, "Source scan observed only type-only imports for: "+formatSourcePackageList(packages)+". Review whether these can be treated as type-only dependencies.")
		}
		return messages
	case migrationTodoSecurity:
		var messages []string
		if len(draft.LifecycleScripts) > 0 {
			messages = append(messages, "package.json lifecycle scripts were detected and not executed: `"+strings.Join(draft.LifecycleScripts, "`, `")+"`.")
		}
		if len(draft.LockEvidence.LifecycleScripts) > 0 {
			messages = append(messages, fmt.Sprintf("Lock evidence detected %d dependency lifecycle script capabilities. TSPack will not execute them by default and migrate did not execute them.", len(draft.LockEvidence.LifecycleScripts)))
		}
		if len(messages) > 0 {
			return messages
		}
		return []string{"No package.json or lockfile lifecycle scripts were detected. Continue to review dependency lifecycle capabilities during update/check."}
	default:
		return []string{"Review required."}
	}
}

func printMigrationDryRun(draft migrationDraft) {
	fmt.Println("Migration draft planned")
	fmt.Println()
	fmt.Println("Inputs:")
	fmt.Println("  package.json: " + draft.Config.packageJSONPath)
	fmt.Println()
	fmt.Println("Would write:")
	fmt.Println("  manifest: " + draft.Config.outManifestPath)
	fmt.Println("  report: " + draft.Config.outReportPath)
	fmt.Println()
	fmt.Println("Summary:")
	fmt.Println("  package: " + packageSummary(draft.Package))
	fmt.Println("  inferred kind: " + draft.Kind)
	fmt.Printf("  dependencies: %d runtime, %d peer, %d tool\n", countDepsByKind(draft.Dependencies, "dep"), countDepsByKind(draft.Dependencies, "peer"), countDepsByKind(draft.Dependencies, "tool"))
	fmt.Printf("  targets: %d inferred\n", len(draft.Targets))
	fmt.Println("  scripts:")
	fmt.Printf("    total: %d\n", len(draft.Package.Scripts))
	fmt.Printf("    runtime target candidates: %d\n", countScriptAnalysesByCategory(draft.ScriptAnalyses, "runtime-target-candidate"))
	fmt.Printf("    shell/complex: %d\n", countScriptAnalysesNeedingReview(draft.ScriptAnalyses))
	fmt.Printf("    lifecycle: %d\n", countScriptAnalysesByCategory(draft.ScriptAnalyses, "lifecycle"))
	fmt.Printf("  TODOs: %d\n", draft.TotalTodos)
	if draft.Validation.Ran {
		fmt.Println()
		printMigrationValidationSummary(draft.Validation)
	}
	fmt.Println()
	printMigrationLockDryRun(draft.LockEvidence)
	fmt.Println()
	printMigrationSourceDryRun(draft.SourceEvidence)
	fmt.Println()
	fmt.Println("Run with --write to create migration files.")
}

func printMigrationWriteSummary(draft migrationDraft) {
	fmt.Println("Generated:")
	fmt.Println("  " + draft.Config.outManifestPath)
	fmt.Println("  " + draft.Config.outReportPath)
	fmt.Println()
	fmt.Println("Next:")
	fmt.Println("  Review MIGRATION_TODO_* comments.")
	if draft.Validation.Ran {
		fmt.Println()
		printMigrationValidationSummary(draft.Validation)
	}
	fmt.Println("  Run tspack check --manifest " + draft.Config.outManifestPath)
}

func printMigrationValidationSummary(result migrationValidationResult) {
	if !result.Ran {
		return
	}
	fmt.Println("Validation:")
	fmt.Println("  manifest frontend: " + result.ManifestFrontend)
	fmt.Println("  manifest IR: " + result.ManifestIR)
	fmt.Printf("  TODOs: %d\n", result.RemainingTodos)
	if result.Passed {
		fmt.Println("  result: structurally valid draft")
	} else {
		fmt.Println("  result: generated draft needs manual repair")
	}
}

func printMigrationDiagnostic(diagnostic migrationDiagnostic) {
	fmt.Fprintln(os.Stderr, diagnostic.Code+": "+diagnostic.Message)
	for _, detail := range diagnostic.Details {
		fmt.Fprintln(os.Stderr, "  "+detail)
	}
	for _, fix := range diagnostic.Fixes {
		fmt.Fprintln(os.Stderr, "  suggested fix: "+fix)
	}
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedAnyMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func countDepsByKind(deps []migratedDependency, kind string) int {
	count := 0
	for _, dep := range deps {
		if dep.Kind == kind {
			count++
		}
	}
	return count
}

func boolToCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func packageSummary(pkg packageJSONModel) string {
	name := defaultString(pkg.Name, "<unnamed>")
	version := defaultString(pkg.Version, "<unversioned>")
	return name + "@" + version
}

func quoteTSString(value string) string {
	return strconv.Quote(value)
}

func renderStringArray(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, quoteTSString(value))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
