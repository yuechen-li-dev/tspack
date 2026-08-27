package adoption

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/yuechen-li-dev/tspack/internal/authoring"
)

type Observation struct {
	Root                 string            `json:"root"`
	PackageJSONPath      string            `json:"packageJsonPath"`
	Name                 string            `json:"name,omitempty"`
	Version              string            `json:"version,omitempty"`
	Type                 string            `json:"type,omitempty"`
	Private              *bool             `json:"private,omitempty"`
	PackageManager       string            `json:"packageManager,omitempty"`
	Workspaces           WorkspaceValue    `json:"workspaces,omitempty"`
	Dependencies         map[string]string `json:"dependencies,omitempty"`
	DevDependencies      map[string]string `json:"devDependencies,omitempty"`
	PeerDependencies     map[string]string `json:"peerDependencies,omitempty"`
	OptionalDependencies map[string]string `json:"optionalDependencies,omitempty"`
	Scripts              map[string]string `json:"scripts,omitempty"`
	Exports              any               `json:"exports,omitempty"`
	Main                 string            `json:"main,omitempty"`
	Module               string            `json:"module,omitempty"`
	Types                string            `json:"types,omitempty"`
	Lockfiles            []Lockfile        `json:"lockfiles"`
	ManifestExists       bool              `json:"manifestExists"`
	LockfileExists       bool              `json:"lockfileExists"`
}

type WorkspaceValue struct {
	Kind     string   `json:"kind,omitempty"`
	Packages []string `json:"packages,omitempty"`
}

type Lockfile struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	PackageManager string `json:"packageManager"`
}

type Report struct {
	Root                  string                    `json:"root"`
	PackageName           string                    `json:"packageName,omitempty"`
	Version               string                    `json:"version,omitempty"`
	DependencyCounts      map[string]int            `json:"dependencyCounts"`
	Scripts               []string                  `json:"scripts"`
	Lockfiles             []Lockfile                `json:"lockfiles"`
	ManifestExists        bool                      `json:"manifestExists"`
	LockfileExists        bool                      `json:"lockfileExists"`
	SuggestedAdoptionMode string                    `json:"suggestedAdoptionMode"`
	Warnings              []string                  `json:"warnings"`
	PackageAnnotations    []PackageAnnotation       `json:"packageAnnotations,omitempty"`
	DependencyAuthoring   *authoring.TapeResolution `json:"dependencyAuthoring,omitempty"`
	Observation           Observation               `json:"observation"`
}

type packageJSON struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Type                 string            `json:"type"`
	Private              *bool             `json:"private"`
	PackageManager       string            `json:"packageManager"`
	Workspaces           json.RawMessage   `json:"workspaces"`
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	Scripts              map[string]string `json:"scripts"`
	Exports              any               `json:"exports"`
	Main                 string            `json:"main"`
	Module               string            `json:"module"`
	Types                string            `json:"types"`
}

func Observe(root string) (Observation, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Observation{}, fmt.Errorf("resolve adoption root: %w", err)
	}
	pkgPath := filepath.Join(absRoot, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Observation{}, fmt.Errorf("TSPACK_ADOPT_PACKAGE_JSON_MISSING: package.json was not found at %s", pkgPath)
		}
		return Observation{}, fmt.Errorf("read package.json: %w", err)
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return Observation{}, fmt.Errorf("TSPACK_ADOPT_PACKAGE_JSON_MALFORMED: package.json is not valid JSON: %w", err)
	}
	obs := Observation{
		Root:                 absRoot,
		PackageJSONPath:      pkgPath,
		Name:                 pkg.Name,
		Version:              pkg.Version,
		Type:                 pkg.Type,
		Private:              pkg.Private,
		PackageManager:       pkg.PackageManager,
		Workspaces:           parseWorkspaces(pkg.Workspaces),
		Dependencies:         copyMap(pkg.Dependencies),
		DevDependencies:      copyMap(pkg.DevDependencies),
		PeerDependencies:     copyMap(pkg.PeerDependencies),
		OptionalDependencies: copyMap(pkg.OptionalDependencies),
		Scripts:              copyMap(pkg.Scripts),
		Exports:              pkg.Exports,
		Main:                 pkg.Main,
		Module:               pkg.Module,
		Types:                pkg.Types,
		Lockfiles:            detectLockfiles(absRoot),
		ManifestExists:       fileExists(filepath.Join(absRoot, "manifest.tsx")),
		LockfileExists:       fileExists(filepath.Join(absRoot, "ts-lock.toml")),
	}
	return obs, nil
}

func BuildReport(obs Observation) Report {
	return BuildReportWithAnnotations(obs, nil)
}

func BuildReportWithAnnotations(obs Observation, annotations []PackageAnnotation) Report {
	mode := "package-json-only"
	if obs.ManifestExists {
		mode = "observe"
	}
	if len(annotations) > 0 {
		mode = "observe-with-package-annotations"
	}
	counts := map[string]int{
		"dependencies":         len(obs.Dependencies),
		"devDependencies":      len(obs.DevDependencies),
		"peerDependencies":     len(obs.PeerDependencies),
		"optionalDependencies": len(obs.OptionalDependencies),
	}
	warnings := []string{
		"package.json scripts are observed only; they are not TSPack RunTargets",
		"package.json dependencies are not fully classified as TSPack dep/tool/peer declarations beyond package.json section signal",
	}
	if !obs.ManifestExists {
		warnings = append(warnings, "no manifest.tsx yet; TSPack is observing the package.json substrate only")
	}
	if !obs.LockfileExists {
		warnings = append(warnings, "no ts-lock.toml yet; TSPack has not generated a lockfile for this project")
	}
	dependencyAuthoring := buildDependencyAuthoring(obs, annotations)
	return Report{
		Root:                  obs.Root,
		PackageName:           obs.Name,
		Version:               obs.Version,
		DependencyCounts:      counts,
		Scripts:               sortedKeys(obs.Scripts),
		Lockfiles:             obs.Lockfiles,
		ManifestExists:        obs.ManifestExists,
		LockfileExists:        obs.LockfileExists,
		SuggestedAdoptionMode: mode,
		Warnings:              warnings,
		PackageAnnotations:    annotations,
		DependencyAuthoring:   &dependencyAuthoring,
		Observation:           obs,
	}
}

func buildDependencyAuthoring(obs Observation, annotations []PackageAnnotation) authoring.TapeResolution {
	var declarations []authoring.DependencyDeclaration
	order := 0

	sortedAnnotations := append([]PackageAnnotation(nil), annotations...)
	sort.SliceStable(sortedAnnotations, func(left, right int) bool {
		if sortedAnnotations[left].Root != sortedAnnotations[right].Root {
			return sortedAnnotations[left].Root < sortedAnnotations[right].Root
		}
		return sortedAnnotations[left].ManifestPath < sortedAnnotations[right].ManifestPath
	})
	for _, annotation := range sortedAnnotations {
		dependencies := append([]AnnotatedDep(nil), annotation.Dependencies...)
		sort.SliceStable(dependencies, func(left, right int) bool {
			if dependencies[left].Name != dependencies[right].Name {
				return dependencies[left].Name < dependencies[right].Name
			}
			return dependencies[left].Kind < dependencies[right].Kind
		})
		for _, dependency := range dependencies {
			constraint := dependency.Range
			if constraint == "" {
				constraint = dependency.PackageJSONRange
			}
			declarations = append(declarations, authoring.DependencyDeclaration{
				ID:          fmt.Sprintf("annotation-%04d", order),
				Key:         dependency.Key,
				Identity:    authoring.PackageIdentity{Source: "npm", Name: dependency.Name},
				Source:      authoring.PackageSource{Kind: "npm", Package: dependency.Name, Range: constraint},
				Constraint:  constraint,
				Kind:        authoring.DependencyKind(dependency.Kind),
				Origin:      authoring.DeclarationOrigin{Kind: authoring.OriginPackageManifest, Name: annotation.AnnotationName, SourcePath: annotation.ManifestPath},
				Layer:       authoring.LayerPackage,
				Order:       order,
				Authority:   authoring.AuthorityOwned,
				Editability: authoring.EditabilityDerived,
			})
			order++
		}
	}
	for _, annotation := range sortedAnnotations {
		dependencies := append([]AnnotatedDep(nil), annotation.Dependencies...)
		sort.SliceStable(dependencies, func(left, right int) bool {
			if dependencies[left].Name != dependencies[right].Name {
				return dependencies[left].Name < dependencies[right].Name
			}
			return dependencies[left].PackageJSONSection < dependencies[right].PackageJSONSection
		})
		for _, dependency := range dependencies {
			kind, optional, observed := packageJSONDependencySemantics(dependency.PackageJSONSection)
			if !observed {
				continue
			}
			declarations = append(declarations, authoring.DependencyDeclaration{
				ID:          fmt.Sprintf("annotated-package-json-%04d", order),
				Key:         dependency.Name,
				Identity:    authoring.PackageIdentity{Source: "npm", Name: dependency.Name},
				Source:      authoring.PackageSource{Kind: "npm", Package: dependency.Name, Range: dependency.PackageJSONRange},
				Constraint:  dependency.PackageJSONRange,
				Kind:        kind,
				Optional:    optional,
				Origin:      authoring.DeclarationOrigin{Kind: authoring.OriginCompatibility, Name: "package.json " + dependency.PackageJSONSection, SourcePath: annotation.PackageJSONPath},
				Layer:       authoring.LayerCompatibility,
				Order:       order,
				Authority:   authoring.AuthorityObserved,
				Editability: authoring.EditabilityObserved,
			})
			order++
		}
	}

	sections := []struct {
		name         string
		dependencies map[string]string
		kind         authoring.DependencyKind
		optional     bool
	}{
		{name: "dependencies", dependencies: obs.Dependencies, kind: authoring.DependencyRuntime},
		{name: "devDependencies", dependencies: obs.DevDependencies, kind: authoring.DependencyTool},
		{name: "peerDependencies", dependencies: obs.PeerDependencies, kind: authoring.DependencyPeer},
		{name: "optionalDependencies", dependencies: obs.OptionalDependencies, kind: authoring.DependencyRuntime, optional: true},
	}
	for sectionOrder, section := range sections {
		for _, name := range sortedKeys(section.dependencies) {
			constraint := section.dependencies[name]
			declarations = append(declarations, authoring.DependencyDeclaration{
				ID:          fmt.Sprintf("package-json-%04d", order),
				Key:         name,
				Identity:    authoring.PackageIdentity{Source: "npm", Name: name},
				Source:      authoring.PackageSource{Kind: "npm", Package: name, Range: constraint},
				Constraint:  constraint,
				Kind:        section.kind,
				Optional:    section.optional,
				Origin:      authoring.DeclarationOrigin{Kind: authoring.OriginCompatibility, Name: "package.json " + section.name, SourcePath: "package.json"},
				Layer:       authoring.LayerCompatibility,
				LayerOrder:  sectionOrder,
				Order:       order,
				Authority:   authoring.AuthorityObserved,
				Editability: authoring.EditabilityObserved,
			})
			order++
		}
	}

	return authoring.Build(declarations)
}

func packageJSONDependencySemantics(section string) (authoring.DependencyKind, bool, bool) {
	switch section {
	case "dependencies":
		return authoring.DependencyRuntime, false, true
	case "devDependencies":
		return authoring.DependencyTool, false, true
	case "peerDependencies":
		return authoring.DependencyPeer, false, true
	case "optionalDependencies":
		return authoring.DependencyRuntime, true, true
	default:
		return "", false, false
	}
}

func parseWorkspaces(raw json.RawMessage) WorkspaceValue {
	if len(raw) == 0 || string(raw) == "null" {
		return WorkspaceValue{}
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return WorkspaceValue{Kind: "array", Packages: list}
	}
	var object struct {
		Packages []string `json:"packages"`
	}
	if err := json.Unmarshal(raw, &object); err == nil {
		return WorkspaceValue{Kind: "object", Packages: object.Packages}
	}
	return WorkspaceValue{Kind: "unknown"}
}

func detectLockfiles(root string) []Lockfile {
	candidates := []Lockfile{
		{Name: "package-lock.json", PackageManager: "npm"},
		{Name: "pnpm-lock.yaml", PackageManager: "pnpm"},
		{Name: "yarn.lock", PackageManager: "yarn"},
		{Name: "bun.lock", PackageManager: "bun"},
		{Name: "bun.lock" + "b", PackageManager: "bun"},
	}
	found := []Lockfile{}
	for _, candidate := range candidates {
		path := filepath.Join(root, candidate.Name)
		if fileExists(path) {
			candidate.Path = path
			found = append(found, candidate)
		}
	}
	return found
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func copyMap(input map[string]string) map[string]string {
	output := map[string]string{}
	for key, value := range input {
		output[key] = value
	}
	return output
}

func sortedKeys(input map[string]string) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
