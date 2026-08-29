package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type pnpmWorkspaceFile struct {
	Packages []string                     `yaml:"packages"`
	Catalog  map[string]string            `yaml:"catalog"`
	Catalogs map[string]map[string]string `yaml:"catalogs"`
}

type pnpmWorkspaceMigration struct {
	ConfigPath                  string
	Includes                    []string
	Excludes                    []string
	Packages                    []pnpmMigratedPackage
	SkippedPackageRoots         []string
	WorkspaceReferences         int
	ResolvedWorkspaceReferences int
	CatalogReferences           int
	ResolvedCatalogReferences   int
	AliasReferences             int
	PathReferences              int
	Unresolved                  []pnpmMigrationUnresolved
	IdentityAdjustments         []pnpmMigrationIdentityAdjustment
}

type pnpmMigratedPackage struct {
	Root         string
	ManifestPath string
	OriginalName string
	Draft        migrationDraft
}

type pnpmMigrationIdentityAdjustment struct {
	PackageRoot  string
	OriginalName string
	DraftName    string
}

type pnpmMigrationUnresolved struct {
	PackageRoot string
	Dependency  string
	Value       string
	Reason      string
}

func buildPnpmWorkspaceMigrationDraft(cfg migrateConfig, rootPackage packageJSONModel, workspacePath string) (migrationDraft, *migrationDiagnostic) {
	workspaceFile, diagnostic := loadPnpmWorkspaceFile(workspacePath)
	if diagnostic != nil {
		return migrationDraft{}, diagnostic
	}

	workspace := &pnpmWorkspaceMigration{ConfigPath: workspacePath}
	for _, pattern := range workspaceFile.Packages {
		if strings.HasPrefix(pattern, "!") {
			workspace.Excludes = append(workspace.Excludes, strings.TrimPrefix(pattern, "!"))
		} else {
			workspace.Includes = append(workspace.Includes, pattern)
		}
	}

	packageRoots, skippedRoots, discoveryDiagnostic := discoverPnpmPackageRoots(cfg.root, workspaceFile.Packages)
	if discoveryDiagnostic != nil {
		return migrationDraft{}, discoveryDiagnostic
	}
	workspace.SkippedPackageRoots = skippedRoots

	packageModels := make(map[string]packageJSONModel, len(packageRoots))
	packageRootsByName := map[string][]string{}
	for _, packageRoot := range packageRoots {
		packageCfg := cfg
		packageCfg.packageJSONPath = filepath.Join(cfg.root, filepath.FromSlash(packageRoot), "package.json")
		if packageRoot == "." {
			packageCfg.packageJSONPath = cfg.packageJSONPath
		}
		pkg, packageDiagnostic := loadPackageJSONForMigration(packageCfg)
		if packageDiagnostic != nil {
			return migrationDraft{}, packageDiagnostic
		}
		if strings.TrimSpace(pkg.Name) == "" {
			return migrationDraft{}, &migrationDiagnostic{
				Code:    "TSPACK_MIGRATE_WORKSPACE_PACKAGE_NAME_MISSING",
				Message: "a pnpm workspace package has no stable package identity",
				Details: []string{"packageRoot: " + packageRoot, "packageJsonPath: " + packageCfg.packageJSONPath},
				Fixes:   []string{"Add a package.json name or exclude this directory from the pnpm workspace."},
			}
		}
		packageModels[packageRoot] = pkg
		packageRootsByName[pkg.Name] = append(packageRootsByName[pkg.Name], packageRoot)
	}
	draft := migrationDraft{
		Config:            cfg,
		Package:           rootPackage,
		Kind:              inferMigrationKind(rootPackage),
		WorkspaceName:     workspaceNameFromPackage(defaultString(rootPackage.Name, "migrated")),
		TodoCounts:        map[string]int{},
		PnpmWorkspacePath: workspacePath,
		PnpmWorkspace:     workspace,
	}

	for _, packageRoot := range packageRoots {
		pkg := packageModels[packageRoot]
		originalName := pkg.Name
		if roots := packageRootsByName[originalName]; len(roots) > 1 {
			pkg.Name = migrationIdentityForDuplicatePackage(originalName, packageRoot)
			workspace.IdentityAdjustments = append(workspace.IdentityAdjustments, pnpmMigrationIdentityAdjustment{
				PackageRoot:  packageRoot,
				OriginalName: originalName,
				DraftName:    pkg.Name,
			})
		}
		packageDraft := migrationDraft{
			Config:        cfg,
			Package:       pkg,
			Kind:          inferMigrationKind(pkg),
			WorkspaceName: draft.WorkspaceName,
			TodoCounts:    map[string]int{},
		}
		packageDraft.Dependencies = migratePnpmPackageDependencies(
			pkg,
			packageRoot,
			packageRootsByName,
			workspaceFile,
			workspace,
			&packageDraft,
		)
		packageDraft.Targets = inferMigrationTargets(pkg, packageDraft.Kind, &packageDraft)
		packageDraft.PublishInclude = inferMigrationPublishInclude(pkg, &packageDraft)
		packageDraft.LifecycleScripts = findMigrationLifecycleScripts(pkg.Scripts)
		packageDraft.ScriptAnalyses = analyzePackageScripts(pkg.Scripts)
		countMigrationTodos(&packageDraft)
		draft.TotalTodos += packageDraft.TotalTodos
		for todo, count := range packageDraft.TodoCounts {
			draft.TodoCounts[todo] += count
		}

		manifestPath := filepath.ToSlash(filepath.Join(filepath.FromSlash(packageRoot), ".tspack-migration", "package.manifest.tsx"))
		if packageRoot == "." {
			manifestPath = ".tspack-migration/package.manifest.tsx"
		}
		content := renderPnpmPackageManifest(packageRoot, &packageDraft)
		workspace.Packages = append(workspace.Packages, pnpmMigratedPackage{
			Root:         packageRoot,
			ManifestPath: manifestPath,
			OriginalName: originalName,
			Draft:        packageDraft,
		})
		draft.PackageManifests = append(draft.PackageManifests, plannedFile{
			path:    filepath.Join(cfg.root, filepath.FromSlash(manifestPath)),
			content: content,
		})
	}

	draft.Manifest = renderPnpmWorkspaceManifest(&draft)
	draft.Report = renderPnpmWorkspaceMigrationReport(&draft)
	draft.Diagnostics = pnpmWorkspaceMigrationDiagnostics(workspace)
	return draft, nil
}

func migrationIdentityForDuplicatePackage(originalName string, packageRoot string) string {
	suffix := strings.NewReplacer("/", "-", "\\", "-", "_", "-").Replace(packageRoot)
	suffix = strings.Trim(suffix, ".-")
	if suffix == "" {
		suffix = "root"
	}
	return originalName + "--tspack-" + suffix
}

func loadPnpmWorkspaceFile(path string) (pnpmWorkspaceFile, *migrationDiagnostic) {
	content, err := os.ReadFile(path)
	if err != nil {
		return pnpmWorkspaceFile{}, &migrationDiagnostic{
			Code:    "TSPACK_MIGRATE_PNPM_WORKSPACE_INVALID",
			Message: "pnpm workspace configuration could not be read",
			Details: []string{"workspaceConfig: " + path, "error: " + err.Error()},
			Fixes:   []string{"Check pnpm-workspace.yaml permissions and retry."},
		}
	}
	var parsed pnpmWorkspaceFile
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		return pnpmWorkspaceFile{}, &migrationDiagnostic{
			Code:    "TSPACK_MIGRATE_PNPM_WORKSPACE_INVALID",
			Message: "pnpm workspace configuration is not valid YAML",
			Details: []string{"workspaceConfig: " + path, "error: " + err.Error()},
			Fixes:   []string{"Fix pnpm-workspace.yaml syntax and retry."},
		}
	}
	if len(parsed.Packages) == 0 {
		return pnpmWorkspaceFile{}, &migrationDiagnostic{
			Code:    "TSPACK_MIGRATE_PNPM_WORKSPACE_PACKAGES_MISSING",
			Message: "pnpm workspace configuration has no package globs",
			Details: []string{"workspaceConfig: " + path},
			Fixes:   []string{"Add a packages list or migrate the root package without pnpm workspace metadata."},
		}
	}
	return parsed, nil
}

func discoverPnpmPackageRoots(root string, patterns []string) ([]string, []string, *migrationDiagnostic) {
	selected := map[string]bool{".": true}
	for _, rawPattern := range patterns {
		pattern := filepath.ToSlash(strings.TrimSpace(rawPattern))
		if pattern == "" || strings.HasPrefix(pattern, "!") {
			continue
		}
		matches, err := matchWorkspaceDirectories(root, pattern)
		if err != nil {
			return nil, nil, &migrationDiagnostic{
				Code:    "TSPACK_MIGRATE_PNPM_WORKSPACE_GLOB_INVALID",
				Message: "pnpm workspace package glob is invalid",
				Details: []string{"pattern: " + rawPattern, "error: " + err.Error()},
				Fixes:   []string{"Fix the workspace glob and retry."},
			}
		}
		for _, match := range matches {
			selected[match] = true
		}
	}
	for _, rawPattern := range patterns {
		if !strings.HasPrefix(rawPattern, "!") {
			continue
		}
		pattern := filepath.ToSlash(strings.TrimSpace(strings.TrimPrefix(rawPattern, "!")))
		matches, err := matchWorkspaceDirectories(root, pattern)
		if err != nil {
			return nil, nil, &migrationDiagnostic{
				Code:    "TSPACK_MIGRATE_PNPM_WORKSPACE_GLOB_INVALID",
				Message: "pnpm workspace exclusion glob is invalid",
				Details: []string{"pattern: " + rawPattern, "error: " + err.Error()},
				Fixes:   []string{"Fix the workspace exclusion and retry."},
			}
		}
		for _, match := range matches {
			if match != "." {
				delete(selected, match)
			}
		}
	}

	var roots []string
	var skipped []string
	for packageRoot := range selected {
		packageJSONPath := filepath.Join(root, filepath.FromSlash(packageRoot), "package.json")
		info, err := os.Stat(packageJSONPath)
		if err != nil || info.IsDir() {
			skipped = append(skipped, packageRoot)
			continue
		}
		roots = append(roots, packageRoot)
	}
	sort.Slice(roots, func(i, j int) bool {
		if roots[i] == "." {
			return true
		}
		if roots[j] == "." {
			return false
		}
		return roots[i] < roots[j]
	})
	sort.Strings(skipped)
	return roots, skipped, nil
}

func matchWorkspaceDirectories(root string, pattern string) ([]string, error) {
	if strings.Contains(pattern, "..") || filepath.IsAbs(pattern) {
		return nil, fmt.Errorf("pattern must be workspace-relative and cannot traverse")
	}
	if strings.Contains(pattern, "**") {
		return matchWorkspaceDirectoriesRecursive(root, pattern)
	}
	matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
	if err != nil {
		return nil, err
	}
	var roots []string
	for _, match := range matches {
		info, statErr := os.Stat(match)
		if statErr != nil || !info.IsDir() {
			continue
		}
		relative, relErr := filepath.Rel(root, match)
		if relErr != nil {
			return nil, relErr
		}
		roots = append(roots, filepath.ToSlash(relative))
	}
	sort.Strings(roots)
	return roots, nil
}

func matchWorkspaceDirectoriesRecursive(root string, pattern string) ([]string, error) {
	prefix := strings.Split(pattern, "**")[0]
	walkRoot := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(prefix, "/")))
	if _, err := os.Stat(walkRoot); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var roots []string
	err := filepath.WalkDir(walkRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if doublestarWorkspaceMatch(pattern, relative) {
			roots = append(roots, relative)
		}
		return nil
	})
	sort.Strings(roots)
	return roots, err
}

func doublestarWorkspaceMatch(pattern string, value string) bool {
	parts := strings.Split(pattern, "**")
	if len(parts) != 2 {
		return false
	}
	prefix := strings.TrimSuffix(parts[0], "/")
	suffix := strings.TrimPrefix(parts[1], "/")
	if prefix != "" && value != prefix && !strings.HasPrefix(value, prefix+"/") {
		return false
	}
	if suffix == "" {
		return true
	}
	matched, err := filepath.Match(filepath.FromSlash(suffix), filepath.Base(filepath.FromSlash(value)))
	return err == nil && matched
}

func migratePnpmPackageDependencies(
	pkg packageJSONModel,
	packageRoot string,
	packageRootsByName map[string][]string,
	workspaceFile pnpmWorkspaceFile,
	workspace *pnpmWorkspaceMigration,
	draft *migrationDraft,
) []migratedDependency {
	usedKeys := map[string]string{}
	peerNames := map[string]bool{}
	var dependencies []migratedDependency
	appendSection := func(values map[string]string, kind string, sourceField string) {
		for _, name := range sortedMapKeys(values) {
			if sourceField != "peerDependencies" && peerNames[name] {
				draft.DuplicatePeerDeps = append(draft.DuplicatePeerDeps, name)
				duplicate := migratedDependency{
					PackageName: name,
					Range:       values[name],
				}
				resolvePnpmDependency(&duplicate, packageRoot, packageRootsByName, workspaceFile, workspace)
				continue
			}
			dependency := migratedDependency{
				Key:           migrationIdentifierForPackage(name, usedKeys, draft),
				PackageName:   name,
				Range:         values[name],
				OriginalRange: values[name],
				Kind:          kind,
				SourceField:   sourceField,
				SourceKind:    "npm",
				SourceName:    name,
				KnownTool:     knownMigrationTools[name],
			}
			if sourceField == "peerDependencies" {
				dependency.OptionalPeer = pkg.PeerDependenciesMeta[name].Optional
			} else if sourceField == "optionalDependencies" {
				dependency.NeedsTODO = true
			}
			resolvePnpmDependency(&dependency, packageRoot, packageRootsByName, workspaceFile, workspace)
			if sourceField == "devDependencies" && !dependency.KnownTool && dependency.SourceKind != "workspace" {
				dependency.NeedsTODO = true
			}
			dependencies = append(dependencies, dependency)
		}
	}
	for name := range pkg.PeerDependencies {
		peerNames[name] = true
	}
	appendSection(pkg.PeerDependencies, "peer", "peerDependencies")
	appendSection(pkg.Dependencies, "dep", "dependencies")
	appendSection(pkg.OptionalDependencies, "dep", "optionalDependencies")
	appendSection(pkg.DevDependencies, "tool", "devDependencies")
	return dependencies
}

func resolvePnpmDependency(
	dependency *migratedDependency,
	packageRoot string,
	packageRootsByName map[string][]string,
	workspaceFile pnpmWorkspaceFile,
	workspace *pnpmWorkspaceMigration,
) {
	value := dependency.Range
	switch {
	case strings.HasPrefix(value, "workspace:"):
		workspace.WorkspaceReferences++
		roots := packageRootsByName[dependency.PackageName]
		if len(roots) == 1 {
			dependency.SourceKind = "workspace"
			dependency.SourceName = dependency.PackageName
			dependency.Range = ""
			dependency.Resolution = "resolved-workspace"
			workspace.ResolvedWorkspaceReferences++
			return
		}
		dependency.NeedsTODO = true
		dependency.Resolution = "unresolved-workspace"
		if len(roots) == 0 {
			dependency.UnresolvedReason = "no discovered workspace package has this identity"
		} else {
			dependency.UnresolvedReason = "multiple discovered workspace packages have this identity"
		}
		workspace.Unresolved = append(workspace.Unresolved, pnpmMigrationUnresolved{
			PackageRoot: packageRoot,
			Dependency:  dependency.PackageName,
			Value:       value,
			Reason:      dependency.UnresolvedReason,
		})
	case strings.HasPrefix(value, "catalog:"):
		workspace.CatalogReferences++
		catalogName := strings.TrimPrefix(value, "catalog:")
		catalog := workspaceFile.Catalog
		if catalogName != "" {
			catalog = workspaceFile.Catalogs[catalogName]
		}
		resolvedRange := catalog[dependency.PackageName]
		if strings.TrimSpace(resolvedRange) != "" {
			dependency.Range = resolvedRange
			dependency.Resolution = "resolved-catalog"
			workspace.ResolvedCatalogReferences++
			return
		}
		dependency.NeedsTODO = true
		dependency.Resolution = "unresolved-catalog"
		dependency.UnresolvedReason = "catalog does not define this dependency"
		workspace.Unresolved = append(workspace.Unresolved, pnpmMigrationUnresolved{
			PackageRoot: packageRoot,
			Dependency:  dependency.PackageName,
			Value:       value,
			Reason:      dependency.UnresolvedReason,
		})
	case strings.HasPrefix(value, "npm:"):
		workspace.AliasReferences++
		targetName, targetRange, ok := parseNpmAlias(value)
		if !ok {
			dependency.NeedsTODO = true
			dependency.Resolution = "unresolved-npm-alias"
			dependency.UnresolvedReason = "npm alias is malformed"
			workspace.Unresolved = append(workspace.Unresolved, pnpmMigrationUnresolved{
				PackageRoot: packageRoot,
				Dependency:  dependency.PackageName,
				Value:       value,
				Reason:      dependency.UnresolvedReason,
			})
			return
		}
		dependency.SourceName = targetName
		dependency.Range = targetRange
		dependency.Resolution = "resolved-npm-alias"
	case strings.HasPrefix(value, "file:") || strings.HasPrefix(value, "link:"):
		workspace.PathReferences++
		pathValue := strings.TrimPrefix(strings.TrimPrefix(value, "file:"), "link:")
		workspacePath := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.FromSlash(packageRoot), filepath.FromSlash(pathValue))))
		if workspacePath == ".." || strings.HasPrefix(workspacePath, "../") {
			dependency.NeedsTODO = true
			dependency.Resolution = "unresolved-path"
			dependency.UnresolvedReason = "local path leaves the workspace root"
			workspace.Unresolved = append(workspace.Unresolved, pnpmMigrationUnresolved{
				PackageRoot: packageRoot,
				Dependency:  dependency.PackageName,
				Value:       value,
				Reason:      dependency.UnresolvedReason,
			})
			return
		}
		dependency.SourceKind = "path"
		dependency.SourceName = filepath.ToSlash(filepath.Clean(filepath.FromSlash(pathValue)))
		dependency.Range = ""
		dependency.Resolution = "resolved-path"
	default:
		dependency.Resolution = "authoritative-package-json"
	}
}

func parseNpmAlias(value string) (string, string, bool) {
	value = strings.TrimPrefix(value, "npm:")
	delimiter := strings.LastIndex(value, "@")
	if delimiter <= 0 || delimiter == len(value)-1 {
		return "", "", false
	}
	return value[:delimiter], value[delimiter+1:], true
}

func renderPnpmWorkspaceManifest(draft *migrationDraft) string {
	var builder strings.Builder
	builder.WriteString("import { Packages, Workspace, defineWorkspace } from \"tspack/manifest\";\n\n")
	builder.WriteString("/**\n")
	builder.WriteString(" * Generated by `tspack migrate` from pnpm workspace compatibility metadata.\n")
	builder.WriteString(" * Package roots and identities are high-confidence inferred facts.\n")
	builder.WriteString(" * Review package-local MIGRATION_TODO_* comments before promotion.\n")
	builder.WriteString(" */\n")
	builder.WriteString("export default defineWorkspace(\n")
	builder.WriteString("  <Workspace name=")
	builder.WriteString(quoteTSString(draft.WorkspaceName))
	builder.WriteString(">\n")
	builder.WriteString("    <Packages\n")
	builder.WriteString("      rows={[\n")
	for _, pkg := range draft.PnpmWorkspace.Packages {
		builder.WriteString("        { name: ")
		builder.WriteString(quoteTSString(pkg.Draft.Package.Name))
		builder.WriteString(", root: ")
		builder.WriteString(quoteTSString(pkg.Root))
		builder.WriteString(", manifest: ")
		builder.WriteString(quoteTSString(pkg.ManifestPath))
		builder.WriteString(" },\n")
	}
	builder.WriteString("      ]}\n")
	builder.WriteString("    />\n")
	builder.WriteString("  </Workspace>,\n")
	builder.WriteString(");\n")
	return builder.String()
}

func renderPnpmPackageManifest(packageRoot string, draft *migrationDraft) string {
	var builder strings.Builder
	builder.WriteString("import {\n")
	builder.WriteString("  Package,\n")
	builder.WriteString("  Publish,\n")
	builder.WriteString("  defineDeps,\n")
	builder.WriteString("  definePackage,\n")
	builder.WriteString("  dep,\n")
	builder.WriteString("  npm,\n")
	builder.WriteString("  path,\n")
	builder.WriteString("  peer,\n")
	builder.WriteString("  tool,\n")
	builder.WriteString("  workspace,\n")
	builder.WriteString("} from \"tspack/manifest\";\n\n")
	builder.WriteString("/**\n")
	builder.WriteString(" * Generated migration draft for ")
	builder.WriteString(packageRoot)
	builder.WriteString("/package.json.\n")
	builder.WriteString(" * Identity and package.json dependency sections are compatibility-derived facts.\n")
	builder.WriteString(" * MIGRATION_TODO_* comments mark unresolved or manually authored semantics.\n")
	builder.WriteString(" */\n")
	if len(draft.Dependencies) > 0 {
		builder.WriteString("const deps = defineDeps({\n")
		for _, dependency := range draft.Dependencies {
			if dependency.NeedsTODO {
				builder.WriteString("  // MIGRATION_TODO_DEP_CLASSIFICATION: ")
				if dependency.UnresolvedReason != "" {
					builder.WriteString(dependency.UnresolvedReason)
					builder.WriteString("; original value ")
					builder.WriteString(quoteTSString(dependency.OriginalRange))
				} else if dependency.SourceField == "optionalDependencies" {
					builder.WriteString("TSPack has no optional runtime dependency intent; review this compatibility-derived dep")
				} else {
					builder.WriteString("mechanically classified from ")
					builder.WriteString(dependency.SourceField)
				}
				builder.WriteString(".\n")
			}
			builder.WriteString("  ")
			builder.WriteString(dependency.Key)
			builder.WriteString(": ")
			builder.WriteString(renderDependencyCall(dependency))
			builder.WriteString(",\n")
		}
		builder.WriteString("});\n\n")
	}
	builder.WriteString("// MIGRATION_TODO_TARGETS: Author build, test, run, and publish intent from repository evidence.\n")
	builder.WriteString("export default definePackage(\n")
	builder.WriteString("  <Package\n")
	builder.WriteString("    name=")
	builder.WriteString(quoteTSString(draft.Package.Name))
	builder.WriteString("\n")
	builder.WriteString("    version=")
	builder.WriteString(quoteTSString(defaultString(draft.Package.Version, "0.0.0")))
	builder.WriteString("\n")
	if draft.Package.License != "" {
		builder.WriteString("    license=")
		builder.WriteString(quoteTSString(draft.Package.License))
		builder.WriteString("\n")
	}
	builder.WriteString("    kind=")
	builder.WriteString(quoteTSString(draft.Kind))
	builder.WriteString("\n")
	if len(draft.Dependencies) > 0 {
		builder.WriteString("    dependencies={{ values: [")
		builder.WriteString(joinDependencyRefs(draft.Dependencies, ""))
		builder.WriteString("] }}\n")
	}
	if draft.Kind == "library" {
		builder.WriteString("  >\n")
		builder.WriteString("    {/* MIGRATION_TODO_PUBLISH: compatibility-derived include; verify with tspack pack --dry-run. */}\n")
		builder.WriteString("    <Publish include={")
		builder.WriteString(renderStringArray(draft.PublishInclude))
		builder.WriteString("} exclude={[]} />\n")
		builder.WriteString("  </Package>,\n")
	} else {
		builder.WriteString("  />,\n")
	}
	builder.WriteString(");\n")
	return builder.String()
}

func renderPnpmWorkspaceMigrationReport(draft *migrationDraft) string {
	workspace := draft.PnpmWorkspace
	var builder strings.Builder
	builder.WriteString("# TSPack Migration Report\n\n")
	builder.WriteString("## Inputs\n\n")
	builder.WriteString("- root: `" + draft.Config.root + "`\n")
	builder.WriteString("- workspace config: `" + workspace.ConfigPath + "`\n")
	builder.WriteString("- generated root draft: `" + draft.Config.outManifestPath + "`\n\n")
	builder.WriteString("## Workspace summary\n\n")
	builder.WriteString(fmt.Sprintf("- workspace packages discovered: %d\n", len(workspace.Packages)))
	builder.WriteString(fmt.Sprintf("- workspace references resolved/unresolved: %d/%d resolved, %d unresolved\n", workspace.ResolvedWorkspaceReferences, workspace.WorkspaceReferences, workspace.WorkspaceReferences-workspace.ResolvedWorkspaceReferences))
	builder.WriteString(fmt.Sprintf("- catalog references resolved/unresolved: %d/%d resolved, %d unresolved\n", workspace.ResolvedCatalogReferences, workspace.CatalogReferences, workspace.CatalogReferences-workspace.ResolvedCatalogReferences))
	builder.WriteString(fmt.Sprintf("- npm aliases recovered: %d\n", workspace.AliasReferences))
	builder.WriteString(fmt.Sprintf("- local path/link references recovered: %d\n", workspace.PathReferences))
	builder.WriteString(fmt.Sprintf("- packages requiring manual semantic completion: %d\n", len(workspace.Packages)))
	builder.WriteString("\nHigh-confidence inferred facts: workspace roots, package identities, package versions, dependency section kinds, unambiguous workspace identities, and catalog versions.\n\n")
	builder.WriteString("## Workspace globs\n\n")
	for _, pattern := range workspace.Includes {
		builder.WriteString("- include `" + pattern + "`\n")
	}
	for _, pattern := range workspace.Excludes {
		builder.WriteString("- exclude `" + pattern + "`\n")
	}
	if len(workspace.SkippedPackageRoots) > 0 {
		builder.WriteString("\nMatched directories without package.json were validated and skipped:\n")
		for _, root := range workspace.SkippedPackageRoots {
			builder.WriteString("- `" + root + "`\n")
		}
	}
	builder.WriteString("\n## Generated package contracts\n\n")
	for _, pkg := range workspace.Packages {
		builder.WriteString("- `" + pkg.Draft.Package.Name + "` — root `" + pkg.Root + "`, draft `" + pkg.ManifestPath + "`\n")
	}
	builder.WriteString("\n## Unresolved facts and TODOs\n\n")
	if len(workspace.IdentityAdjustments) > 0 {
		builder.WriteString("TSPack package identities must be unique. These duplicate compatibility identities received deterministic draft-only identities and require semantic review:\n")
		for _, adjustment := range workspace.IdentityAdjustments {
			builder.WriteString("- `" + adjustment.PackageRoot + "`: package.json `" + adjustment.OriginalName + "` -> draft `" + adjustment.DraftName + "`\n")
		}
		builder.WriteString("\n")
	}
	if len(workspace.Unresolved) == 0 {
		builder.WriteString("No workspace or catalog references were unresolved. Package-local semantic TODOs remain.\n")
	} else {
		for _, unresolved := range workspace.Unresolved {
			builder.WriteString("- `" + unresolved.PackageRoot + "`: `" + unresolved.Dependency + "` = `" + unresolved.Value + "` — " + unresolved.Reason + "\n")
		}
	}
	builder.WriteString("\n## Unsupported constructs\n\n")
	builder.WriteString("- pnpm overrides, patches, lifecycle allowlists, release/CI policy, and arbitrary scripts remain compatibility evidence; they are not imported as TSPack semantic truth.\n")
	builder.WriteString("- optional runtime dependencies are emitted as reviewable deps because TSPack currently has optional semantics only for peers.\n")
	builder.WriteString("- targets, compiler configuration, build topology, and test topology require repository-aware semantic authoring.\n")
	builder.WriteString("\n## Validation\n\n")
	if !draft.Validation.Ran {
		builder.WriteString("Status: not run. Run `tspack migrate --check` to validate the composed root and package drafts without promoting them.\n")
	} else if draft.Validation.Passed {
		builder.WriteString("Status: passed. The composed draft passed the manifest frontend and Go IR validation.\n")
		builder.WriteString(fmt.Sprintf("- Remaining conservative TODO units: %d\n", draft.Validation.RemainingTodos))
	} else {
		builder.WriteString("Status: failed. No migration outputs are written by `--write --check` when structural validation fails.\n")
		builder.WriteString("- manifest frontend: `" + draft.Validation.ManifestFrontend + "`\n")
		builder.WriteString("- manifest IR: `" + draft.Validation.ManifestIR + "`\n")
	}
	builder.WriteString("\n## Suggested next steps\n\n")
	builder.WriteString("1. Review `manifest.migrated.tsx` and each `.tspack-migration/package.manifest.tsx` draft.\n")
	builder.WriteString(fmt.Sprintf("2. Resolve %d workspace/catalog TODOs plus package-local semantic TODOs.\n", len(workspace.Unresolved)))
	builder.WriteString("3. Promote the reviewed root to `manifest.tsx` and package drafts to package-local `package.manifest.tsx`; lifecycle commands intentionally reject the root draft filename.\n")
	builder.WriteString("4. Run `tspack check`.\n")
	builder.WriteString("5. Run `tspack update`, then `tspack sync`, as appropriate for native dependency realization.\n")
	builder.WriteString("\nThis is a mechanical migration draft, not a claim of complete project semantics.\n")
	return builder.String()
}

func pnpmWorkspaceMigrationDiagnostics(workspace *pnpmWorkspaceMigration) []migrationDiagnostic {
	diagnostics := []migrationDiagnostic{{
		Code:    "TSPACK_MIGRATE_PNPM_WORKSPACE_IMPORTED",
		Message: "pnpm workspace compatibility metadata was lowered into a reviewable TSPack workspace draft",
		Details: []string{
			fmt.Sprintf("packages: %d", len(workspace.Packages)),
			fmt.Sprintf("workspaceReferences: %d/%d resolved", workspace.ResolvedWorkspaceReferences, workspace.WorkspaceReferences),
			fmt.Sprintf("catalogReferences: %d/%d resolved", workspace.ResolvedCatalogReferences, workspace.CatalogReferences),
		},
		Fixes: []string{"Review package-local semantic TODOs before promoting the draft."},
	}}
	if len(workspace.Unresolved) > 0 {
		diagnostics = append(diagnostics, migrationDiagnostic{
			Code:    "TSPACK_MIGRATE_PNPM_REFERENCES_UNRESOLVED",
			Message: "some pnpm workspace or catalog references could not be resolved safely",
			Details: []string{fmt.Sprintf("unresolved: %d", len(workspace.Unresolved))},
			Fixes:   []string{"Review the stable TODOs in the generated package drafts and migration report."},
		})
	}
	if len(workspace.IdentityAdjustments) > 0 {
		diagnostics = append(diagnostics, migrationDiagnostic{
			Code:    "TSPACK_MIGRATE_WORKSPACE_PACKAGE_IDENTITIES_ADJUSTED",
			Message: "duplicate package.json identities received deterministic draft-only TSPack identities",
			Details: []string{fmt.Sprintf("adjustedPackages: %d", len(workspace.IdentityAdjustments))},
			Fixes:   []string{"Review the identity mappings in tspack-migration.md before promotion."},
		})
	}
	return diagnostics
}
