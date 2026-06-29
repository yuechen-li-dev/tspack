package npmobserve

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/capability"
)

const (
	SourceRootPackageJSON   = "root-package-json"
	SourcePackageLock       = "package-lock"
	SourceInstalledPackage  = "installed-package-json"
	PresenceRoot            = "root"
	PresenceDirect          = "direct"
	PresenceTransitive      = "transitive"
	SectionDependencies     = "dependencies"
	SectionDevDependencies  = "devDependencies"
	SectionOptionalDeps     = "optionalDependencies"
	SectionPeerDependencies = "peerDependencies"
)

type Options struct {
	Root string
}

type Report struct {
	RootDir              string              `json:"root"`
	SourceKind           string              `json:"sourceKind"`
	PackageJSONPath      string              `json:"packageJsonPath"`
	LockfilePath         string              `json:"lockfilePath,omitempty"`
	LockfilePresent      bool                `json:"lockfilePresent"`
	NodeModulesPresent   bool                `json:"nodeModulesPresent"`
	NodeModulesInspected bool                `json:"nodeModulesInspected"`
	MetadataSources      []string            `json:"metadataSources"`
	RootScripts          []LifecycleScript   `json:"rootScripts"`
	DependencyScripts    []LifecycleScript   `json:"lifecycleScripts"`
	Summary              Summary             `json:"summary"`
	CapabilityWarnings   []CapabilityWarning `json:"capabilityWarnings"`
	Limitations          []string            `json:"limitations,omitempty"`
	Notes                []string            `json:"notes,omitempty"`
}

type Summary struct {
	LifecycleScripts     int `json:"lifecycleScripts"`
	InstallTimeHooks     int `json:"installTimeHooks"`
	DirectHooks          int `json:"directHooks"`
	TransitiveHooks      int `json:"transitiveHooks"`
	OptionalHooks        int `json:"optionalHooks"`
	RootLifecycleScripts int `json:"rootLifecycleScripts"`
}

type CapabilityWarning struct {
	PackageName         string     `json:"packageName"`
	Version             string     `json:"version,omitempty"`
	Location            string     `json:"location"`
	Phase               string     `json:"phase"`
	Command             string     `json:"command"`
	CapabilityID        string     `json:"capabilityID"`
	Title               string     `json:"title"`
	AttentionLevel      string     `json:"attentionLevel"`
	Reason              string     `json:"reason"`
	Meaning             string     `json:"meaning"`
	RecommendedNextStep string     `json:"recommendedNextStep,omitempty"`
	Presence            string     `json:"presence"`
	Source              string     `json:"source"`
	Flags               []string   `json:"flags,omitempty"`
	Chains              [][]string `json:"chains,omitempty"`
	Tags                []string   `json:"tags"`
}

type LifecycleScript struct {
	PackageName         string     `json:"packageName"`
	Version             string     `json:"version,omitempty"`
	Location            string     `json:"location"`
	Presence            string     `json:"presence"`
	DependencySections  []string   `json:"packageJsonSections,omitempty"`
	Phase               string     `json:"phase"`
	Command             string     `json:"command"`
	Source              string     `json:"source"`
	LifecycleCategory   string     `json:"lifecycleCategory"`
	ConsumerInstallTime bool       `json:"consumerInstallTime"`
	Optional            bool       `json:"optional,omitempty"`
	Dev                 bool       `json:"dev,omitempty"`
	Peer                bool       `json:"peer,omitempty"`
	WhyChains           [][]string `json:"chains,omitempty"`
	Notes               []string   `json:"notes,omitempty"`
}

type packageJSONModel struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	Scripts              map[string]string `json:"scripts"`
}

type packageLockModel struct {
	LockfileVersion int                         `json:"lockfileVersion"`
	Packages        map[string]packageLockEntry `json:"packages"`
}

type packageLockEntry struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Dependencies         map[string]string `json:"dependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	Scripts              map[string]string `json:"scripts"`
	Dev                  bool              `json:"dev"`
	Optional             bool              `json:"optional"`
	Peer                 bool              `json:"peer"`
}

type observedPackage struct {
	Name        string
	Version     string
	LockPath    string
	Sections    []string
	Presence    string
	Optional    bool
	Dev         bool
	Peer        bool
	Chains      [][]string
	LockScripts map[string]string
}

func Observe(opts Options) (Report, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err == nil {
		root = absRoot
	}

	report := Report{
		RootDir:         root,
		SourceKind:      "observed-npm",
		PackageJSONPath: filepath.Join(root, "package.json"),
	}

	rootPkg, err := loadPackageJSON(report.PackageJSONPath)
	if err != nil {
		return Report{}, err
	}

	report.MetadataSources = append(report.MetadataSources, SourceRootPackageJSON)
	report.RootScripts = collectLifecycleScripts(
		rootPkg.Name,
		rootPkg.Version,
		".",
		PresenceRoot,
		nil,
		rootPkg.Scripts,
		SourceRootPackageJSON,
		false,
		false,
		false,
		nil,
	)

	lockPath := filepath.Join(root, "package-lock.json")
	lock, lockPresent, lockErr := loadPackageLock(lockPath)
	report.LockfilePath = lockPath
	report.LockfilePresent = lockPresent

	nodeModulesPath := filepath.Join(root, "node_modules")
	if info, err := os.Stat(nodeModulesPath); err == nil && info.IsDir() {
		report.NodeModulesPresent = true
	}

	if lockErr != nil {
		report.Limitations = append(report.Limitations, "package-lock.json could not be parsed: "+lockErr.Error())
		return finalizeReport(report), nil
	}

	if !lockPresent {
		report.Limitations = append(report.Limitations, "package-lock.json was not found, so dependency lifecycle script metadata could not be observed.")
		if !report.NodeModulesPresent {
			report.Notes = append(report.Notes, "To materialize installed npm package metadata for a follow-up read-only scan, run `tspack npm install` and rerun `tspack adopt --security`.")
		}
		return finalizeReport(report), nil
	}

	report.MetadataSources = append(report.MetadataSources, SourcePackageLock)
	packages := observeLockPackages(rootPkg, lock)

	lockScriptCount := 0
	for _, pkg := range packages {
		if len(pkg.LockScripts) == 0 {
			continue
		}
		lockScriptCount += len(pkg.LockScripts)
		report.DependencyScripts = append(report.DependencyScripts, collectLifecycleScripts(
			pkg.Name,
			pkg.Version,
			pkg.LockPath,
			pkg.Presence,
			pkg.Sections,
			pkg.LockScripts,
			SourcePackageLock,
			pkg.Optional,
			pkg.Dev,
			pkg.Peer,
			pkg.Chains,
		)...)
	}

	if lockScriptCount == 0 {
		report.Limitations = append(report.Limitations, "package-lock.json did not expose dependency lifecycle script details.")
	}

	if report.NodeModulesPresent {
		report.NodeModulesInspected = true
		report.MetadataSources = append(report.MetadataSources, SourceInstalledPackage)
		report.DependencyScripts = append(report.DependencyScripts, inspectInstalledPackageScripts(root, packages, report.DependencyScripts)...)
	} else if lockScriptCount == 0 {
		report.Notes = append(report.Notes, "To inspect installed package metadata read-only, run `tspack npm install` and rerun `tspack adopt --security`.")
	}

	if lockScriptCount == 0 && report.NodeModulesInspected && countDependencyScriptsBySource(report.DependencyScripts, SourceInstalledPackage) == 0 {
		report.Notes = append(report.Notes, "No install-time lifecycle scripts were found in observed installed package metadata.")
	}

	return finalizeReport(report), nil
}

func finalizeReport(report Report) Report {
	report.MetadataSources = uniqueSorted(report.MetadataSources)
	report.Limitations = uniqueSorted(report.Limitations)
	report.Notes = uniqueSorted(report.Notes)
	sortScripts(report.RootScripts)
	sortScripts(report.DependencyScripts)
	report.Summary = summarizeLifecycle(report.RootScripts, report.DependencyScripts)
	report.CapabilityWarnings = buildCapabilityWarnings(report.RootScripts, report.DependencyScripts)
	return report
}

func summarizeLifecycle(rootScripts []LifecycleScript, dependencyScripts []LifecycleScript) Summary {
	summary := Summary{
		LifecycleScripts:     len(rootScripts) + len(dependencyScripts),
		RootLifecycleScripts: len(rootScripts),
	}
	for _, script := range dependencyScripts {
		if isInstallTimePhase(script.Phase) {
			summary.InstallTimeHooks++
		}
		if script.Presence == PresenceDirect && isInstallTimePhase(script.Phase) {
			summary.DirectHooks++
		}
		if script.Presence == PresenceTransitive && isInstallTimePhase(script.Phase) {
			summary.TransitiveHooks++
		}
		if script.Optional && isInstallTimePhase(script.Phase) {
			summary.OptionalHooks++
		}
	}
	for _, script := range rootScripts {
		if isInstallTimePhase(script.Phase) {
			summary.InstallTimeHooks++
		}
	}
	return summary
}

func buildCapabilityWarnings(rootScripts []LifecycleScript, dependencyScripts []LifecycleScript) []CapabilityWarning {
	warnings := []CapabilityWarning{}
	for _, script := range dependencyScripts {
		warnings = append(warnings, classifyCapabilityWarning(script))
	}
	for _, script := range rootScripts {
		warnings = append(warnings, classifyCapabilityWarning(script))
	}
	sort.SliceStable(warnings, func(i, j int) bool {
		left := capabilitySortKey(warnings[i])
		right := capabilitySortKey(warnings[j])
		return left < right
	})
	return warnings
}

func classifyCapabilityWarning(script LifecycleScript) CapabilityWarning {
	warning := CapabilityWarning{
		PackageName:         script.PackageName,
		Version:             script.Version,
		Location:            script.Location,
		Phase:               script.Phase,
		Command:             script.Command,
		AttentionLevel:      "notice",
		Presence:            script.Presence,
		Source:              script.Source,
		Flags:               lifecycleFlags(script),
		Chains:              copyChains(script.WhyChains),
		Tags:                lifecycleTags(script),
		RecommendedNextStep: "Review whether this install-time behavior is expected for the package and chain that pulled it in.",
	}

	if script.Presence == PresenceRoot && isInstallTimePhase(script.Phase) {
		warning.CapabilityID = "root-install-lifecycle"
		warning.Title = "root install lifecycle script"
		warning.Reason = "this project declares an install-time lifecycle script"
		warning.Meaning = "npm may execute this project's lifecycle script during install workflows. This is project-owned behavior, not a dependency finding."
		return warning
	}

	if isPublishOrPackPhase(script.Phase) {
		warning.CapabilityID = "publish-or-pack-lifecycle"
		warning.Title = "publish or pack lifecycle script"
		warning.AttentionLevel = "info"
		warning.Reason = "this lifecycle phase is associated with npm pack or publish workflows"
		warning.Meaning = "npm may execute this script during pack or publish workflows. The prepare phase can also run in some dependency install contexts, especially git-style installs."
		return warning
	}

	warning.CapabilityID = "install-time-code-execution"
	warning.Title = "install-time code execution"
	warning.Reason = installTimeReason(script)
	warning.Meaning = "npm may execute this package's lifecycle script during install or materialization. This is common for some packages, but it is still install-time code execution worth making visible."
	return warning
}

func installTimeReason(script LifecycleScript) string {
	if script.Optional {
		return "optional dependency install hook observed"
	}
	if script.Presence == PresenceDirect {
		return "direct dependency install hook observed"
	}
	if script.Presence == PresenceTransitive {
		return "transitive dependency install hook observed"
	}
	return "install-time lifecycle hook observed"
}

func lifecycleFlags(script LifecycleScript) []string {
	flags := []string{}
	if script.Optional {
		flags = append(flags, "optional")
	}
	if script.Dev {
		flags = append(flags, "dev")
	}
	if script.Peer {
		flags = append(flags, "peer")
	}
	return flags
}

func lifecycleTags(script LifecycleScript) []string {
	tags := []string{script.Presence}
	if isInstallTimePhase(script.Phase) {
		tags = append(tags, "install-time", "install-time-code-execution")
		if script.Presence == PresenceDirect {
			tags = append(tags, "direct-dependency-install-hook")
		}
		if script.Presence == PresenceTransitive {
			tags = append(tags, "transitive-dependency-install-hook")
		}
		if script.Optional {
			tags = append(tags, "optional-dependency-install-hook")
		}
	}
	if isPublishOrPackPhase(script.Phase) {
		tags = append(tags, "publish-time")
	}
	if script.Optional {
		tags = append(tags, "optional")
	}
	if script.Dev {
		tags = append(tags, "dev")
	}
	if script.Peer {
		tags = append(tags, "peer")
	}
	if script.Source == SourceInstalledPackage {
		tags = append(tags, "installed-metadata")
	}
	if script.Presence == PresenceRoot && isInstallTimePhase(script.Phase) {
		tags = append(tags, "root-install-lifecycle")
	}
	if isPublishOrPackPhase(script.Phase) {
		tags = append(tags, "publish-or-pack-lifecycle")
	}
	if script.Source == SourcePackageLock {
		tags = append(tags, "lockfile-metadata")
	}
	return uniqueSorted(tags)
}

func isInstallTimePhase(phase string) bool {
	switch phase {
	case "preinstall", "install", "postinstall", "prepare":
		return true
	default:
		return false
	}
}

func isPublishOrPackPhase(phase string) bool {
	switch phase {
	case "prepack", "postpack", "prepublish", "prepublishOnly":
		return true
	default:
		return false
	}
}

func capabilitySortKey(warning CapabilityWarning) string {
	return attentionSortPrefix(warning.AttentionLevel) + "|" + presenceSortPrefix(warning.Presence) + "|" + warning.PackageName + "|" + warning.Phase
}

func attentionSortPrefix(level string) string {
	switch level {
	case "warning":
		return "0"
	case "notice":
		return "1"
	default:
		return "2"
	}
}

func presenceSortPrefix(presence string) string {
	switch presence {
	case PresenceRoot:
		return "0"
	case PresenceDirect:
		return "1"
	case PresenceTransitive:
		return "2"
	default:
		return "3"
	}
}

func loadPackageJSON(path string) (packageJSONModel, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return packageJSONModel{}, errors.New("package.json was not found")
		}
		return packageJSONModel{}, err
	}
	var model packageJSONModel
	if err := json.Unmarshal(content, &model); err != nil {
		return packageJSONModel{}, err
	}
	return model, nil
}

func loadPackageLock(path string) (packageLockModel, bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return packageLockModel{}, false, nil
		}
		return packageLockModel{}, true, err
	}
	var model packageLockModel
	if err := json.Unmarshal(content, &model); err != nil {
		return packageLockModel{}, true, err
	}
	if len(model.Packages) == 0 {
		return packageLockModel{}, true, errors.New("packages object is missing or empty")
	}
	return model, true, nil
}

func observeLockPackages(rootPkg packageJSONModel, lock packageLockModel) []observedPackage {
	directSections := directDependencySections(rootPkg)
	paths := make([]string, 0, len(lock.Packages))
	for path := range lock.Packages {
		if path == "" {
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)

	parents := map[string][]string{}
	packagesByPath := map[string]observedPackage{}

	for _, path := range paths {
		entry := lock.Packages[path]
		name := entry.Name
		if name == "" {
			name = packageNameFromLockPath(path)
		}
		if name == "" {
			continue
		}
		sections := append([]string(nil), directSections[name]...)
		presence := PresenceTransitive
		if len(sections) > 0 {
			presence = PresenceDirect
		}
		packagesByPath[path] = observedPackage{
			Name:        name,
			Version:     entry.Version,
			LockPath:    path,
			Sections:    sections,
			Presence:    presence,
			Optional:    entry.Optional || hasSection(sections, SectionOptionalDeps),
			Dev:         entry.Dev || hasSection(sections, SectionDevDependencies),
			Peer:        entry.Peer || hasSection(sections, SectionPeerDependencies),
			LockScripts: copyStringMap(entry.Scripts),
		}
	}

	for _, path := range paths {
		entry := lock.Packages[path]
		for depName := range mergedDependencyNames(entry.Dependencies, entry.OptionalDependencies) {
			depPath, ok := resolveDependencyPath(lock.Packages, path, depName)
			if !ok {
				continue
			}
			parents[depPath] = append(parents[depPath], path)
		}
	}

	rootParents := map[string]bool{}
	for depName := range directSections {
		depPath, ok := resolveDependencyPath(lock.Packages, "", depName)
		if !ok {
			continue
		}
		rootParents[depPath] = true
	}

	byPath := make([]observedPackage, 0, len(packagesByPath))
	for _, path := range paths {
		pkg, ok := packagesByPath[path]
		if !ok {
			continue
		}
		pkg.Chains = buildWhyChains(path, packagesByPath, parents, rootParents)
		packagesByPath[path] = pkg
		byPath = append(byPath, pkg)
	}

	sort.SliceStable(byPath, func(i, j int) bool {
		if byPath[i].Name != byPath[j].Name {
			return byPath[i].Name < byPath[j].Name
		}
		return byPath[i].LockPath < byPath[j].LockPath
	})
	return byPath
}

func directDependencySections(rootPkg packageJSONModel) map[string][]string {
	out := map[string][]string{}
	appendSection := func(section string, deps map[string]string) {
		for name := range deps {
			out[name] = append(out[name], section)
		}
	}
	appendSection(SectionDependencies, rootPkg.Dependencies)
	appendSection(SectionDevDependencies, rootPkg.DevDependencies)
	appendSection(SectionOptionalDeps, rootPkg.OptionalDependencies)
	appendSection(SectionPeerDependencies, rootPkg.PeerDependencies)
	for name := range out {
		sort.Strings(out[name])
	}
	return out
}

func buildWhyChains(targetPath string, packagesByPath map[string]observedPackage, parents map[string][]string, rootParents map[string]bool) [][]string {
	if rootParents[targetPath] {
		if pkg, ok := packagesByPath[targetPath]; ok {
			return [][]string{{"root", pkg.Name}}
		}
	}

	type queueItem struct {
		path  string
		chain []string
	}

	queue := []queueItem{{path: targetPath, chain: []string{packageNameForPath(targetPath, packagesByPath)}}}
	seen := map[string]bool{targetPath: true}
	chains := [][]string{}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if rootParents[item.path] {
			chain := append([]string{"root"}, reverseStrings(item.chain)...)
			chains = append(chains, chain)
			continue
		}

		for _, parentPath := range uniqueSorted(parents[item.path]) {
			if seen[parentPath] {
				continue
			}
			seen[parentPath] = true
			parentName := packageNameForPath(parentPath, packagesByPath)
			queue = append(queue, queueItem{
				path:  parentPath,
				chain: append(append([]string(nil), item.chain...), parentName),
			})
		}
	}

	return dedupeChains(chains)
}

func inspectInstalledPackageScripts(root string, packages []observedPackage, existing []LifecycleScript) []LifecycleScript {
	seen := map[string]bool{}
	for _, script := range existing {
		seen[script.Source+"|"+script.Location+"|"+script.Phase] = true
	}

	var out []LifecycleScript
	for _, pkg := range packages {
		packageJSONPath := filepath.Join(root, filepath.FromSlash(pkg.LockPath), "package.json")
		model, err := loadPackageJSON(packageJSONPath)
		if err != nil {
			continue
		}
		scripts := collectLifecycleScripts(
			pkg.Name,
			firstNonEmpty(model.Version, pkg.Version),
			pkg.LockPath,
			pkg.Presence,
			pkg.Sections,
			model.Scripts,
			SourceInstalledPackage,
			pkg.Optional,
			pkg.Dev,
			pkg.Peer,
			pkg.Chains,
		)
		for _, script := range scripts {
			key := script.Source + "|" + script.Location + "|" + script.Phase
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, script)
		}
	}
	return out
}

func collectLifecycleScripts(
	packageName string,
	version string,
	location string,
	presence string,
	sections []string,
	scripts map[string]string,
	source string,
	optional bool,
	dev bool,
	peer bool,
	chains [][]string,
) []LifecycleScript {
	if len(scripts) == 0 {
		return nil
	}

	var out []LifecycleScript
	for _, phase := range orderedLifecyclePhases() {
		command, ok := scripts[phase]
		if !ok {
			continue
		}
		classification := capability.ClassifyLifecycleScript(phase)
		out = append(out, LifecycleScript{
			PackageName:         packageName,
			Version:             version,
			Location:            filepath.ToSlash(location),
			Presence:            presence,
			DependencySections:  append([]string(nil), sections...),
			Phase:               phase,
			Command:             command,
			Source:              source,
			LifecycleCategory:   classification.LifecycleCategory,
			ConsumerInstallTime: classification.ConsumerInstallTime,
			Optional:            optional,
			Dev:                 dev,
			Peer:                peer,
			WhyChains:           copyChains(chains),
			Notes:               buildScriptNotes(presence, optional, classification.ConsumerInstallTime),
		})
	}
	return out
}

func buildScriptNotes(presence string, optional bool, consumerInstall bool) []string {
	notes := []string{}
	if consumerInstall {
		notes = append(notes, "install hook present")
	} else {
		notes = append(notes, "lifecycle script present")
	}
	if presence == PresenceDirect {
		notes = append(notes, "direct dependency")
	} else if presence == PresenceTransitive {
		notes = append(notes, "transitive dependency")
	}
	if optional {
		notes = append(notes, "optional dependency")
	}
	return notes
}

func resolveDependencyPath(packages map[string]packageLockEntry, fromPath string, depName string) (string, bool) {
	candidate := joinNodeModulesPath(fromPath, depName)
	if _, ok := packages[candidate]; ok {
		return candidate, true
	}

	base := fromPath
	for base != "" {
		base = trimOnePackage(base)
		if base == "" {
			break
		}
		candidate = joinNodeModulesPath(base, depName)
		if _, ok := packages[candidate]; ok {
			return candidate, true
		}
	}

	candidate = joinNodeModulesPath("", depName)
	_, ok := packages[candidate]
	return candidate, ok
}

func joinNodeModulesPath(base string, depName string) string {
	if base == "" {
		return "node_modules/" + depName
	}
	return base + "/node_modules/" + depName
}

func trimOnePackage(path string) string {
	parts := strings.Split(path, "/")
	lastNodeModules := -1
	for index, part := range parts {
		if part == "node_modules" {
			lastNodeModules = index
		}
	}
	if lastNodeModules <= 0 {
		return ""
	}
	return strings.Join(parts[:lastNodeModules], "/")
}

func packageNameFromLockPath(path string) string {
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

func packageNameForPath(path string, packagesByPath map[string]observedPackage) string {
	if pkg, ok := packagesByPath[path]; ok && pkg.Name != "" {
		return pkg.Name
	}
	return packageNameFromLockPath(path)
}

func mergedDependencyNames(runtime map[string]string, optional map[string]string) map[string]bool {
	out := map[string]bool{}
	for name := range runtime {
		out[name] = true
	}
	for name := range optional {
		out[name] = true
	}
	return out
}

func countDependencyScriptsBySource(scripts []LifecycleScript, source string) int {
	count := 0
	for _, script := range scripts {
		if script.Source == source {
			count++
		}
	}
	return count
}

func orderedLifecyclePhases() []string {
	return []string{
		"preinstall",
		"install",
		"postinstall",
		"prepack",
		"prepare",
		"postpack",
		"prepublish",
		"prepublishOnly",
		"publish",
		"postpublish",
	}
}

func sortScripts(scripts []LifecycleScript) {
	sort.SliceStable(scripts, func(i, j int) bool {
		if scripts[i].Presence != scripts[j].Presence {
			return scripts[i].Presence < scripts[j].Presence
		}
		if scripts[i].PackageName != scripts[j].PackageName {
			return scripts[i].PackageName < scripts[j].PackageName
		}
		if scripts[i].Location != scripts[j].Location {
			return scripts[i].Location < scripts[j].Location
		}
		return scripts[i].Phase < scripts[j].Phase
	})
}

func hasSection(sections []string, want string) bool {
	for _, section := range sections {
		if section == want {
			return true
		}
	}
	return false
}

func copyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, value := range values {
		out[key] = value
	}
	return out
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

func reverseStrings(values []string) []string {
	out := append([]string(nil), values...)
	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] = out[right], out[left]
	}
	return out
}

func dedupeChains(chains [][]string) [][]string {
	seen := map[string]bool{}
	var out [][]string
	for _, chain := range chains {
		key := strings.Join(chain, "->")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, chain)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) < len(out[j])
		}
		return strings.Join(out[i], "->") < strings.Join(out[j], "->")
	})
	return out
}

func copyChains(chains [][]string) [][]string {
	if len(chains) == 0 {
		return nil
	}
	out := make([][]string, 0, len(chains))
	for _, chain := range chains {
		out = append(out, append([]string(nil), chain...))
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
