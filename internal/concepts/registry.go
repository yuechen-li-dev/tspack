package concepts

import "sort"

type Registry interface {
	Lookup(name string) (Fragment, bool)
	BuiltinNames() []string
}

type MapRegistry struct{ fragments map[string]Fragment }

func NewRegistry(fragments []Fragment) *MapRegistry {
	m := map[string]Fragment{}
	for _, fragment := range fragments {
		m[fragment.Name] = fragment
	}
	return &MapRegistry{fragments: m}
}
func (r *MapRegistry) Lookup(name string) (Fragment, bool) { f, ok := r.fragments[name]; return f, ok }
func (r *MapRegistry) BuiltinNames() []string {
	names := make([]string, 0, len(r.fragments))
	for n := range r.fragments {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

var Builtins = NewRegistry(builtinFragments())

func Lookup(name string) (Fragment, bool) { return Builtins.Lookup(name) }
func MustLookup(name string) Fragment {
	f, ok := Lookup(name)
	if !ok {
		panic("unknown built-in concept: " + name)
	}
	return f
}
func BuiltinNames() []string { return Builtins.BuiltinNames() }

func dep(name, rng string) DependencyContribution {
	return DependencyContribution{Name: name, Range: rng, Source: "npm"}
}
func builtinFragments() []Fragment {
	return []Fragment{
		{Name: "tspack.workspace", Description: "TSPack workspace shell", Provides: []string{"tspack.workspace"}, Manifest: ManifestContributions{Workspace: &WorkspaceContribution{Name: "workspace"}, Concepts: []string{"tspack.workspace"}}},
		{Name: "tspack.manifestBoundary", Provides: []string{"tspack.manifestBoundary"}, Requires: []string{"tspack.workspace"}, Manifest: ManifestContributions{Tools: []DependencyContribution{dep("@biomejs/biome", "^1.9.4")}, Concepts: []string{"tspack.manifestBoundary"}}},
		{Name: "tspack.securityPolicy", Provides: []string{"tspack.securityPolicy"}, Manifest: ManifestContributions{SecurityPolicy: []PolicyContribution{{Subject: "npm", Action: "acknowledge", Range: "baseline"}}, Concepts: []string{"tspack.securityPolicy"}}},
		{Name: "tspack.updatePolicy", Provides: []string{"tspack.updatePolicy"}, Manifest: ManifestContributions{UpdatePolicy: []PolicyContribution{{Subject: "dependencies", Action: "pin", Range: "minor"}}, Concepts: []string{"tspack.updatePolicy"}}},
		{Name: "typescript.app", Provides: []string{"typescript.app"}, CompatibleKinds: []string{"app"}, Manifest: ManifestContributions{Tools: []DependencyContribution{dep("typescript", "^5.0.0")}, Concepts: []string{"typescript.app"}}},
		{Name: "vite.app", Provides: []string{"vite.app"}, Requires: []string{"typescript.app"}, CompatibleKinds: []string{"app"}, Manifest: ManifestContributions{Tools: []DependencyContribution{dep("vite", "^5.0.0")}, RunTargets: []RunTargetContribution{{Name: "dev", Command: "vite"}, {Name: "build", Command: "vite build"}, {Name: "preview", Command: "vite preview"}}, Concepts: []string{"vite.app"}}},
		{Name: "react.app", Provides: []string{"react.app"}, Requires: []string{"typescript.app"}, CompatibleKinds: []string{"app"}, Manifest: ManifestContributions{Dependencies: []DependencyContribution{dep("react", "^18.2.0"), dep("react-dom", "^18.2.0")}, Tools: []DependencyContribution{dep("@types/react", "^18.2.0"), dep("@types/react-dom", "^18.2.0")}, Concepts: []string{"react.app"}}},
		{Name: "browser.spa", Provides: []string{"browser.spa"}, CompatibleKinds: []string{"app"}, Manifest: ManifestContributions{Concepts: []string{"browser.spa"}}},
		{Name: "browser.static", Provides: []string{"browser.static"}, Requires: []string{"vite.app"}, CompatibleKinds: []string{"app"}, Manifest: ManifestContributions{Targets: []TargetContribution{{Name: "app", Export: ".", Entry: "src/main.ts", Runtime: "dist/main.js", Types: "dist/main.d.ts"}}, Concepts: []string{"browser.static"}}},
		{Name: "typescript.library", Provides: []string{"typescript.library"}, CompatibleKinds: []string{"library"}, Manifest: ManifestContributions{Tools: []DependencyContribution{dep("typescript", "^5.0.0")}, Concepts: []string{"typescript.library"}}},
		{Name: "vite.library", Provides: []string{"vite.library"}, Requires: []string{"typescript.library"}, CompatibleKinds: []string{"library"}, Manifest: ManifestContributions{Tools: []DependencyContribution{dep("vite", "^5.0.0")}, RunTargets: []RunTargetContribution{{Name: "build", Command: "vite build"}}, Concepts: []string{"vite.library"}}},
		{Name: "package.peerDependencies", Provides: []string{"package.peerDependencies"}, CompatibleKinds: []string{"library"}, Manifest: ManifestContributions{Concepts: []string{"package.peerDependencies"}}},
		{Name: "react.library", Provides: []string{"react.library"}, Requires: []string{"typescript.library", "package.peerDependencies"}, CompatibleKinds: []string{"library"}, Manifest: ManifestContributions{Peers: []DependencyContribution{dep("react", "^18.2.0"), dep("react-dom", "^18.2.0")}, Tools: []DependencyContribution{dep("@types/react", "^18.2.0"), dep("@types/react-dom", "^18.2.0")}, Concepts: []string{"react.library"}}},
		{Name: "package.exports", Provides: []string{"package.exports"}, CompatibleKinds: []string{"library"}, Projections: ProjectionContributions{Objects: map[string]map[string]string{"package.exports": {".": "./dist/index.js"}}}, Manifest: ManifestContributions{Concepts: []string{"package.exports"}}},
		{Name: "tspack.pack", Provides: []string{"tspack.pack"}, CompatibleKinds: []string{"library"}, Manifest: ManifestContributions{Pack: &PackContribution{Format: "npm", Artifact: "dist"}, Concepts: []string{"tspack.pack"}}},
	}
}
