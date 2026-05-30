package main

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
	outManifestPath string
	outReportPath   string
	write           bool
	force           bool
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
	LockfilePath         string
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

func runMigrateCommand(args []string) {
	cfg, parseDiags := parseMigrateArgs(args)
	if len(parseDiags) > 0 {
		for _, diagnostic := range parseDiags {
			printMigrationDiagnostic(diagnostic)
		}
		os.Exit(1)
	}

	draft, diagnostic := buildMigrationDraft(cfg)
	if diagnostic != nil {
		printMigrationDiagnostic(*diagnostic)
		os.Exit(1)
	}

	if !cfg.write {
		printMigrationDryRun(draft)
		return
	}

	outputs := []plannedFile{
		{path: cfg.outManifestPath, content: draft.Manifest},
		{path: cfg.outReportPath, content: draft.Report},
	}

	if !cfg.force {
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
				os.Exit(1)
			}
		}
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
			os.Exit(1)
		}
	}

	printMigrationWriteSummary(draft)
}

func parseMigrateArgs(args []string) (migrateConfig, []migrationDiagnostic) {
	cfg := migrateConfig{root: "."}
	var diags []migrationDiagnostic

	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--write":
			cfg.write = true
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
				Code:    "TSPACK_MIGRATE_UNSUPPORTED_PACKAGE_SHAPE",
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

	cfg.packageJSONPath = filepath.Clean(cfg.packageJSONPath)
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
		Code:    "TSPACK_MIGRATE_UNSUPPORTED_PACKAGE_SHAPE",
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
		LockfilePath:  detectMigrationLockfile(cfg.root),
		TodoCounts:    map[string]int{},
	}
	draft.Dependencies = migrateDependencies(pkg, &draft)
	draft.Targets = inferMigrationTargets(pkg, draft.Kind, &draft)
	draft.PublishInclude = inferMigrationPublishInclude(pkg, &draft)
	draft.LifecycleScripts = findMigrationLifecycleScripts(pkg.Scripts)
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
		runtime := firstNonEmpty(pkg.Module, pkg.Main, "dist/index.js")
		targetsByExport["."] = migratedTarget{
			Name:        "core",
			Export:      ".",
			Entry:       guessMigrationEntry(runtime, "."),
			Runtime:     runtime,
			Types:       firstNonEmpty(migrationTypesField(pkg), "dist/index.d.ts"),
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
	runtime := firstNonEmpty(info.Runtime, pkg.Module, pkg.Main, "dist/index.js")
	return migratedTarget{
		Name:        name,
		Export:      exportName,
		Entry:       guessMigrationEntry(runtime, exportName),
		Runtime:     runtime,
		Types:       firstNonEmpty(info.Types, migrationTypesField(pkg), "dist/index.d.ts"),
		NeedsTODO:   needsTODO,
		Description: description,
	}
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
	if strings.HasPrefix(cleaned, "dist/") {
		cleaned = strings.TrimPrefix(cleaned, "dist/")
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

	builder.WriteString("const deps = defineDeps({\n")
	for _, dep := range draft.Dependencies {
		if dep.NeedsTODO {
			builder.WriteString("  // MIGRATION_TODO_DEP_CLASSIFICATION:\n")
			if dep.SourceField == "optionalDependencies" {
				builder.WriteString("  // optionalDependency semantics require review; TSPack dependency intent is explicit.\n")
			} else if dep.SourceField == "devDependencies" {
				builder.WriteString("  // devDependency classified as tool mechanically; verify if this should be test/build-only policy.\n")
			} else {
				builder.WriteString("  // This dependency was classified mechanically from package.json.\n")
				builder.WriteString("  // Verify whether it belongs to a specific target.\n")
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
		builder.WriteString("// TSPack RunTargets are runtime targets, not arbitrary scripts.\n\n")
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
		builder.WriteString(joinDependencyRefs(draft.Dependencies, "dep"))
		builder.WriteString("],\n")
		builder.WriteString("            peers: [")
		builder.WriteString(joinDependencyRefs(draft.Dependencies, "peer"))
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
	if dep.Kind == "peer" && dep.OptionalPeer {
		return "peer(" + source + ", { optional: true })"
	}
	return dep.Kind + "(" + source + ")"
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

func renderMigrationReport(draft *migrationDraft) string {
	countMigrationTodos(draft)
	var builder strings.Builder
	builder.WriteString("# TSPack Migration Report\n\n")
	builder.WriteString("## Inputs\n")
	builder.WriteString("- root: `" + draft.Config.root + "`\n")
	builder.WriteString("- package.json path: `" + draft.Config.packageJSONPath + "`\n")
	if draft.LockfilePath != "" {
		builder.WriteString("- lockfile path detected but not consumed in M41a: `" + draft.LockfilePath + "`\n")
	} else {
		builder.WriteString("- lockfile path detected but not consumed in M41a: none\n")
	}
	builder.WriteString("- generated manifest path: `" + draft.Config.outManifestPath + "`\n")
	builder.WriteString("- generated report path: `" + draft.Config.outReportPath + "`\n\n")

	builder.WriteString("## Summary\n")
	builder.WriteString("- package: `" + packageSummary(draft.Package) + "`\n")
	builder.WriteString("- inferred kind: `" + draft.Kind + "`\n")
	builder.WriteString(fmt.Sprintf("- dependency counts: %d runtime, %d peer, %d tool\n", countDepsByKind(draft.Dependencies, "dep"), countDepsByKind(draft.Dependencies, "peer"), countDepsByKind(draft.Dependencies, "tool")))
	builder.WriteString(fmt.Sprintf("- generated target count: %d\n", len(draft.Targets)))
	builder.WriteString(fmt.Sprintf("- TODO count: %d\n", draft.TotalTodos))
	if len(draft.Package.InvalidFields) > 0 {
		builder.WriteString("- unsupported package shapes were skipped with TODOs: " + strings.Join(draft.Package.InvalidFields, "; ") + "\n")
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
			builder.WriteString("| `" + dep.Key + "` | `" + dep.PackageName + "` | `" + dep.Range + "` | `" + dep.Kind + "` | `" + dep.SourceField + "` |\n")
		}
		builder.WriteString("\n")
	}
	if len(draft.DuplicatePeerDeps) > 0 {
		builder.WriteString("Duplicate dependency/peer declarations preferred peer classification for: `" + strings.Join(draft.DuplicatePeerDeps, "`, `") + "`.\n\n")
	}
	if len(draft.IdentifierCollisions) > 0 {
		builder.WriteString("Identifier collisions were resolved deterministically:\n")
		for _, collision := range draft.IdentifierCollisions {
			builder.WriteString("- " + collision + "\n")
		}
		builder.WriteString("\n")
	}

	builder.WriteString("## TODOs for human/LLM review\n")
	for _, todo := range migrationTodoOrder {
		builder.WriteString("### " + todo + "\n")
		for _, message := range todoMessagesForReport(todo, draft) {
			builder.WriteString("- " + message + "\n")
		}
		builder.WriteString("\n")
	}

	builder.WriteString("## Scripts not migrated\n")
	if len(draft.Package.Scripts) == 0 {
		builder.WriteString("No package.json scripts were present.\n\n")
	} else {
		for _, name := range sortedMapKeys(draft.Package.Scripts) {
			builder.WriteString("- `" + name + "`: `" + draft.Package.Scripts[name] + "`\n")
		}
		builder.WriteString("\nDev/preview/server scripts may become `<RunTargets>` after review. Build/test/lint scripts should not be blindly converted to RunTargets. No scripts were executed.\n\n")
	}

	if len(draft.LifecycleScripts) > 0 {
		builder.WriteString("## Security\n")
		builder.WriteString("Lifecycle scripts detected and not executed: `" + strings.Join(draft.LifecycleScripts, "`, `") + "`. Review before acknowledging capabilities.\n\n")
	}

	builder.WriteString("## Suggested next steps\n")
	builder.WriteString("- Review `" + filepath.Base(draft.Config.outManifestPath) + "`.\n")
	builder.WriteString("- Resolve `MIGRATION_TODO_*` comments.\n")
	builder.WriteString("- Run: `tspack check --manifest " + filepath.Base(draft.Config.outManifestPath) + "`.\n")
	builder.WriteString("- Run: `tspack update --manifest " + filepath.Base(draft.Config.outManifestPath) + "`.\n")
	builder.WriteString("- Run: `tspack pack --dry-run --manifest " + filepath.Base(draft.Config.outManifestPath) + "`.\n")
	builder.WriteString("\nThis report does not claim migration is complete. It is a mechanical draft for human/LLM review.\n")
	return builder.String()
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
	counts[migrationTodoRunTargets] = boolToCount(len(draft.Package.Scripts) > 0)
	counts[migrationTodoPublish] = 1
	counts[migrationTodoBoundaries] = 1
	counts[migrationTodoTypes] = 1
	counts[migrationTodoSecurity] = boolToCount(len(draft.LifecycleScripts) > 0)
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
		return messages
	case migrationTodoRunTargets:
		return []string{"package.json scripts are listed below but not migrated. RunTargets describe runtime processes, not arbitrary build/test/lint scripts."}
	case migrationTodoPublish:
		return []string{"Publish include was inferred from package.json files or a conservative default. Verify with `tspack pack --dry-run`."}
	case migrationTodoBoundaries:
		return []string{"Strict boundary defaults were emitted. Review target/source-specific policy before relying on them."}
	case migrationTodoTypes:
		return []string{"Type policy was generated from package kind, not from source analysis. Verify declarations and type leakage expectations."}
	case migrationTodoSecurity:
		if len(draft.LifecycleScripts) > 0 {
			return []string{"Lifecycle scripts were detected and not executed: `" + strings.Join(draft.LifecycleScripts, "`, `") + "`."}
		}
		return []string{"No package.json lifecycle scripts were detected. Continue to review dependency lifecycle capabilities during update/check."}
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
	fmt.Printf("  TODOs: %d\n", draft.TotalTodos)
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
	fmt.Println("  Run tspack check --manifest " + draft.Config.outManifestPath)
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

func detectMigrationLockfile(root string) string {
	for _, name := range []string{"package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "yarn.lock"} {
		candidate := filepath.Join(root, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
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
