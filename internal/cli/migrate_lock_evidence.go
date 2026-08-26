package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type packageLockEvidenceStatus string

const (
	packageLockEvidenceFound   packageLockEvidenceStatus = "found"
	packageLockEvidenceMissing packageLockEvidenceStatus = "missing"
	packageLockEvidenceSkipped packageLockEvidenceStatus = "skipped"
	packageLockEvidenceInvalid packageLockEvidenceStatus = "invalid"
)

type packageLockEvidence struct {
	Path                 string
	Status               packageLockEvidenceStatus
	StatusReason         string
	Version              int
	PackageCount         int
	Direct               []directLockEvidence
	Fanout               []fanoutEvidence
	LifecycleScripts     []lifecycleScriptEvidence
	Binaries             []binaryEvidence
	PeerPackages         []peerPackageEvidence
	PlatformPackages     []platformPackageEvidence
	DuplicateVersions    []duplicateVersionEvidence
	MissingDirect        []string
	RootUndeclaredDirect []string
	UnsupportedVersion   bool
	Warnings             []string
	TypePackageNames     []string
}

type directLockEvidence struct {
	Name          string
	DeclaredRange string
	Resolved      string
	Kind          string
	LockPath      string
	Integrity     string
	ResolvedURL   string
	Found         bool
}

type fanoutEvidence struct {
	Name           string
	LockPath       string
	ReachableCount int
	TopDeps        []string
	Large          bool
}

type lifecycleScriptEvidence struct {
	Name       string
	Version    string
	LockPath   string
	ScriptName string
	Command    string
	Direct     bool
}

type binaryEvidence struct {
	Name     string
	Version  string
	LockPath string
	Bins     []string
	Direct   bool
}

type peerPackageEvidence struct {
	Name                 string
	Version              string
	LockPath             string
	PeerDependencies     []string
	PeerDependenciesMeta []string
	Direct               bool
}

type platformPackageEvidence struct {
	Name     string
	Version  string
	LockPath string
	Reasons  []string
	Direct   bool
}

type duplicateVersionEvidence struct {
	Name     string
	Versions []string
	Paths    []string
}

type packageLockModel struct {
	LockfileVersion int                         `json:"lockfileVersion"`
	Packages        map[string]packageLockEntry `json:"packages"`
	Dependencies    map[string]packageLockEntry `json:"dependencies"`
}

type packageLockEntry struct {
	Name                 string                        `json:"name"`
	Version              string                        `json:"version"`
	Resolved             string                        `json:"resolved"`
	Integrity            string                        `json:"integrity"`
	Dependencies         map[string]string             `json:"dependencies"`
	OptionalDependencies map[string]string             `json:"optionalDependencies"`
	PeerDependencies     map[string]string             `json:"peerDependencies"`
	PeerDependenciesMeta map[string]peerDependencyMeta `json:"peerDependenciesMeta"`
	Bin                  any                           `json:"bin"`
	Scripts              map[string]string             `json:"scripts"`
	Dev                  bool                          `json:"dev"`
	Optional             bool                          `json:"optional"`
	Peer                 bool                          `json:"peer"`
	Bundled              any                           `json:"bundled"`
	Engines              map[string]any                `json:"engines"`
	OS                   []string                      `json:"os"`
	CPU                  []string                      `json:"cpu"`
}

func loadPackageLockEvidence(cfg migrateConfig, deps []migratedDependency) (packageLockEvidence, *migrationDiagnostic) {
	if cfg.noLockEvidence {
		return packageLockEvidence{
			Path:         cfg.packageLockPath,
			Status:       packageLockEvidenceSkipped,
			StatusReason: "skipped by --no-lock-evidence",
		}, nil
	}

	path, explicit, missingDiagnostic := resolvePackageLockPath(cfg)
	if missingDiagnostic != nil {
		if explicit {
			return packageLockEvidence{}, missingDiagnostic
		}
		return packageLockEvidence{Status: packageLockEvidenceMissing, StatusReason: "not found"}, nil
	}
	if path == "" {
		return packageLockEvidence{Status: packageLockEvidenceMissing, StatusReason: "not found"}, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		invalid := packageLockInvalidDiagnostic(cfg, path, "read failed: "+err.Error(), 0)
		if explicit {
			return packageLockEvidence{}, &invalid
		}
		return packageLockEvidence{
			Path:         path,
			Status:       packageLockEvidenceInvalid,
			StatusReason: "read failed: " + err.Error(),
			Warnings:     []string{invalid.Code + ": " + invalid.Message},
		}, nil
	}

	model, parseErr, version := parsePackageLock(content)
	if parseErr != nil {
		invalid := packageLockInvalidDiagnostic(cfg, path, parseErr.Error(), version)
		if explicit {
			return packageLockEvidence{}, &invalid
		}
		return packageLockEvidence{
			Path:         path,
			Status:       packageLockEvidenceInvalid,
			StatusReason: parseErr.Error(),
			Version:      version,
			Warnings:     []string{invalid.Code + ": " + invalid.Message},
		}, nil
	}

	evidence := buildPackageLockEvidence(path, model, deps)
	if evidence.UnsupportedVersion {
		evidence.Warnings = append(evidence.Warnings, fmt.Sprintf("TSPACK_MIGRATE_PACKAGE_LOCK_UNSUPPORTED_VERSION: lockfileVersion %d is not npm v2/v3; evidence is best effort", evidence.Version))
	}
	return evidence, nil
}

func resolvePackageLockPath(cfg migrateConfig) (string, bool, *migrationDiagnostic) {
	if cfg.packageLockPath != "" {
		if _, err := os.Stat(cfg.packageLockPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				diagnostic := packageLockMissingDiagnostic(cfg, cfg.packageLockPath)
				return "", true, &diagnostic
			}
			diagnostic := packageLockInvalidDiagnostic(cfg, cfg.packageLockPath, "stat failed: "+err.Error(), 0)
			return "", true, &diagnostic
		}
		return cfg.packageLockPath, true, nil
	}

	candidate := filepath.Join(cfg.root, "package-lock.json")
	if _, err := os.Stat(candidate); err != nil {
		return "", false, nil
	}
	return candidate, false, nil
}

func parsePackageLock(content []byte) (packageLockModel, error, int) {
	var raw map[string]any
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return packageLockModel{}, err, 0
	}

	version := jsonNumberToInt(raw["lockfileVersion"])
	if packages, ok := raw["packages"].(map[string]any); !ok || len(packages) == 0 {
		return packageLockModel{}, errors.New("package-lock packages object is missing or empty"), version
	}

	var model packageLockModel
	decoder = json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	if err := decoder.Decode(&model); err != nil {
		return packageLockModel{}, err, version
	}
	return model, nil, version
}

func buildPackageLockEvidence(path string, model packageLockModel, deps []migratedDependency) packageLockEvidence {
	evidence := packageLockEvidence{
		Path:               path,
		Status:             packageLockEvidenceFound,
		Version:            model.LockfileVersion,
		PackageCount:       len(model.Packages),
		UnsupportedVersion: model.LockfileVersion != 2 && model.LockfileVersion != 3,
	}

	directByName := directDependencyMap(deps)
	entriesByName := packageLockEntriesByName(model.Packages)
	paths := sortedPackageLockPaths(model.Packages)

	for _, dep := range deps {
		entryPath, entry, found := findDirectPackageLockEntry(dep.PackageName, model.Packages, entriesByName)
		direct := directLockEvidence{
			Name:          dep.PackageName,
			DeclaredRange: dep.Range,
			Kind:          dep.Kind,
			Found:         found,
		}
		if found {
			direct.Resolved = entry.Version
			direct.LockPath = entryPath
			direct.Integrity = shortIntegrity(entry.Integrity)
			direct.ResolvedURL = entry.Resolved
		} else {
			evidence.MissingDirect = append(evidence.MissingDirect, dep.PackageName)
		}
		evidence.Direct = append(evidence.Direct, direct)
	}

	for _, pathKey := range paths {
		if pathKey == "" {
			continue
		}
		entry := model.Packages[pathKey]
		name := packageLockEntryName(pathKey, entry)
		if name == "" {
			continue
		}
		direct := directByName[name]
		evidence.LifecycleScripts = append(evidence.LifecycleScripts, lifecycleEvidenceForEntry(name, pathKey, entry, direct)...)
		if binary := binaryEvidenceForEntry(name, pathKey, entry, direct); len(binary.Bins) > 0 {
			evidence.Binaries = append(evidence.Binaries, binary)
		}
		if peer := peerEvidenceForEntry(name, pathKey, entry, direct); len(peer.PeerDependencies) > 0 || len(peer.PeerDependenciesMeta) > 0 {
			evidence.PeerPackages = append(evidence.PeerPackages, peer)
		}
		if platform := platformEvidenceForEntry(name, pathKey, entry, direct); len(platform.Reasons) > 0 {
			evidence.PlatformPackages = append(evidence.PlatformPackages, platform)
		}
		if strings.HasPrefix(name, "@types/") {
			evidence.TypePackageNames = append(evidence.TypePackageNames, name)
		}
	}

	evidence.DuplicateVersions = duplicateVersionEvidenceForPackages(model.Packages)
	evidence.RootUndeclaredDirect = undeclaredRootLockDependencies(model, directByName)
	evidence.Fanout = fanoutEvidenceForDirectPackages(model.Packages, evidence.Direct, entriesByName)
	evidence.TypePackageNames = uniqueSortedStrings(evidence.TypePackageNames)
	return evidence
}

func directDependencyMap(deps []migratedDependency) map[string]bool {
	out := map[string]bool{}
	for _, dep := range deps {
		out[dep.PackageName] = true
	}
	return out
}

func packageLockEntriesByName(packages map[string]packageLockEntry) map[string][]string {
	out := map[string][]string{}
	for path, entry := range packages {
		name := packageLockEntryName(path, entry)
		if name == "" {
			continue
		}
		out[name] = append(out[name], path)
	}
	for name := range out {
		sort.Strings(out[name])
	}
	return out
}

func sortedPackageLockPaths(packages map[string]packageLockEntry) []string {
	paths := make([]string, 0, len(packages))
	for path := range packages {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func findDirectPackageLockEntry(name string, packages map[string]packageLockEntry, entriesByName map[string][]string) (string, packageLockEntry, bool) {
	directPath := "node_modules/" + name
	if entry, ok := packages[directPath]; ok {
		return directPath, entry, true
	}
	paths := entriesByName[name]
	if len(paths) == 0 {
		return "", packageLockEntry{}, false
	}
	return paths[0], packages[paths[0]], true
}

func packageLockEntryName(path string, entry packageLockEntry) string {
	if entry.Name != "" {
		return entry.Name
	}
	return packageNameFromLockPath(path)
}

func packageNameFromLockPath(path string) string {
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	for index := len(parts) - 1; index >= 0; index-- {
		if parts[index] != "node_modules" {
			continue
		}
		if index+1 >= len(parts) {
			return ""
		}
		name := parts[index+1]
		if strings.HasPrefix(name, "@") && index+2 < len(parts) {
			return name + "/" + parts[index+2]
		}
		return name
	}
	return ""
}

func lifecycleEvidenceForEntry(name string, path string, entry packageLockEntry, direct bool) []lifecycleScriptEvidence {
	var out []lifecycleScriptEvidence
	for _, scriptName := range sortedMapKeys(entry.Scripts) {
		if !lifecycleScriptNames[scriptName] {
			continue
		}
		out = append(out, lifecycleScriptEvidence{
			Name:       name,
			Version:    entry.Version,
			LockPath:   path,
			ScriptName: scriptName,
			Command:    entry.Scripts[scriptName],
			Direct:     direct,
		})
	}
	return out
}

func binaryEvidenceForEntry(name string, path string, entry packageLockEntry, direct bool) binaryEvidence {
	return binaryEvidence{
		Name:     name,
		Version:  entry.Version,
		LockPath: path,
		Bins:     readPackageLockBinNames(entry.Bin),
		Direct:   direct,
	}
}

func readPackageLockBinNames(value any) []string {
	var names []string
	switch typed := value.(type) {
	case string:
		if typed != "" {
			names = append(names, typed)
		}
	case map[string]any:
		for name, pathValue := range typed {
			pathString, _ := pathValue.(string)
			if pathString == "" {
				names = append(names, name)
			} else {
				names = append(names, name+" -> "+pathString)
			}
		}
	}
	sort.Strings(names)
	return names
}

func peerEvidenceForEntry(name string, path string, entry packageLockEntry, direct bool) peerPackageEvidence {
	return peerPackageEvidence{
		Name:                 name,
		Version:              entry.Version,
		LockPath:             path,
		PeerDependencies:     sortedDependencyNames(entry.PeerDependencies),
		PeerDependenciesMeta: sortedPeerMetaNames(entry.PeerDependenciesMeta),
		Direct:               direct,
	}
}

func platformEvidenceForEntry(name string, path string, entry packageLockEntry, direct bool) platformPackageEvidence {
	var reasons []string
	if len(entry.OS) > 0 {
		reasons = append(reasons, "os="+strings.Join(entry.OS, ","))
	}
	if len(entry.CPU) > 0 {
		reasons = append(reasons, "cpu="+strings.Join(entry.CPU, ","))
	}
	for _, marker := range []string{"linux", "darwin", "win32", "x64", "arm64", "musl"} {
		if strings.Contains(strings.ToLower(name), marker) {
			reasons = append(reasons, "name contains "+marker)
		}
	}
	for _, prefix := range []string{"@biomejs/cli-", "@esbuild/", "@rollup/rollup-"} {
		if strings.HasPrefix(name, prefix) {
			reasons = append(reasons, "known native/platform package pattern "+prefix)
		}
	}
	return platformPackageEvidence{
		Name:     name,
		Version:  entry.Version,
		LockPath: path,
		Reasons:  uniqueSortedStrings(reasons),
		Direct:   direct,
	}
}

func duplicateVersionEvidenceForPackages(packages map[string]packageLockEntry) []duplicateVersionEvidence {
	versionsByName := map[string]map[string][]string{}
	for path, entry := range packages {
		name := packageLockEntryName(path, entry)
		if name == "" || entry.Version == "" {
			continue
		}
		if versionsByName[name] == nil {
			versionsByName[name] = map[string][]string{}
		}
		versionsByName[name][entry.Version] = append(versionsByName[name][entry.Version], path)
	}

	var out []duplicateVersionEvidence
	for name, versions := range versionsByName {
		if len(versions) < 2 {
			continue
		}
		var versionNames []string
		var paths []string
		for version, versionPaths := range versions {
			versionNames = append(versionNames, version)
			paths = append(paths, versionPaths...)
		}
		sort.Strings(versionNames)
		sort.Strings(paths)
		out = append(out, duplicateVersionEvidence{Name: name, Versions: versionNames, Paths: paths})
	}
	sort.Slice(out, func(i int, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func undeclaredRootLockDependencies(model packageLockModel, directByName map[string]bool) []string {
	root, ok := model.Packages[""]
	if !ok {
		return nil
	}
	rootNames := map[string]bool{}
	for name := range root.Dependencies {
		rootNames[name] = true
	}
	for name := range root.OptionalDependencies {
		rootNames[name] = true
	}
	for name := range root.PeerDependencies {
		rootNames[name] = true
	}
	var out []string
	for name := range rootNames {
		if !directByName[name] {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func fanoutEvidenceForDirectPackages(packages map[string]packageLockEntry, direct []directLockEvidence, entriesByName map[string][]string) []fanoutEvidence {
	var out []fanoutEvidence
	for _, directEntry := range direct {
		if !directEntry.Found || directEntry.LockPath == "" {
			continue
		}
		visited := map[string]bool{}
		visitPackageLockDependencies(directEntry.LockPath, packages, entriesByName, visited)
		delete(visited, directEntry.LockPath)
		topDeps := directDependencyNamesForLockPath(directEntry.LockPath, packages)
		out = append(out, fanoutEvidence{
			Name:           directEntry.Name,
			LockPath:       directEntry.LockPath,
			ReachableCount: len(visited),
			TopDeps:        topDeps,
			Large:          len(visited) >= 25,
		})
	}
	sort.Slice(out, func(i int, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func visitPackageLockDependencies(path string, packages map[string]packageLockEntry, entriesByName map[string][]string, visited map[string]bool) {
	if visited[path] {
		return
	}
	visited[path] = true
	entry, ok := packages[path]
	if !ok {
		return
	}
	for _, name := range directDependencyNamesForEntry(entry) {
		for _, childPath := range entriesByName[name] {
			visitPackageLockDependencies(childPath, packages, entriesByName, visited)
		}
	}
}

func directDependencyNamesForLockPath(path string, packages map[string]packageLockEntry) []string {
	entry, ok := packages[path]
	if !ok {
		return nil
	}
	return directDependencyNamesForEntry(entry)
}

func directDependencyNamesForEntry(entry packageLockEntry) []string {
	seen := map[string]bool{}
	for name := range entry.Dependencies {
		seen[name] = true
	}
	for name := range entry.OptionalDependencies {
		seen[name] = true
	}
	var names []string
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > 5 {
		return names[:5]
	}
	return names
}

func sortedDependencyNames(values map[string]string) []string {
	var names []string
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedPeerMetaNames(values map[string]peerDependencyMeta) []string {
	var names []string
	for name, meta := range values {
		if meta.Optional {
			names = append(names, name+" optional")
		} else {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func jsonNumberToInt(value any) int {
	switch typed := value.(type) {
	case json.Number:
		intValue, err := typed.Int64()
		if err == nil {
			return int(intValue)
		}
	case float64:
		return int(typed)
	}
	return 0
}

func shortIntegrity(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 24 {
		return value
	}
	return value[:24] + "..."
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		if value != "" {
			seen[value] = true
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func packageLockMissingDiagnostic(cfg migrateConfig, path string) migrationDiagnostic {
	return migrationDiagnostic{
		Code:    "TSPACK_MIGRATE_PACKAGE_LOCK_MISSING",
		Message: "explicit package-lock.json path was not found",
		Details: []string{
			"root: " + cfg.root,
			"packageLockPath: " + path,
		},
		Fixes: []string{"Pass an existing --package-lock path, regenerate package-lock with npm outside tspack, or use --no-lock-evidence."},
	}
}

func packageLockInvalidDiagnostic(cfg migrateConfig, path string, reason string, version int) migrationDiagnostic {
	details := []string{
		"root: " + cfg.root,
		"packageLockPath: " + path,
		"reason: " + reason,
	}
	if version != 0 {
		details = append(details, fmt.Sprintf("lockfileVersion: %d", version))
	}
	return migrationDiagnostic{
		Code:    "TSPACK_MIGRATE_PACKAGE_LOCK_INVALID",
		Message: "package-lock.json evidence could not be parsed",
		Details: details,
		Fixes:   []string{"Regenerate package-lock with npm outside tspack, fix JSON syntax, or use --no-lock-evidence."},
	}
}
