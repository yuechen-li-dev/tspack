package graph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

type DependencyKind string

const (
	// DependencyKindRuntime indicates a dependency that must be available to runtime code.
	DependencyKindRuntime DependencyKind = "runtime"
	// DependencyKindDep is general authoring intent and is treated as runtime-equivalent in graph usage.
	DependencyKindDep DependencyKind = "dep"
	// DependencyKindPeer indicates a consumer-provided runtime dependency scoped to target peers.
	DependencyKindPeer DependencyKind = "peer"
	// DependencyKindTool indicates build/dev tooling dependencies excluded from runtime target deps.
	DependencyKindTool DependencyKind = "tool"
	// DependencyKindType indicates type-only dependencies not runtime-visible.
	DependencyKindType DependencyKind = "type"
	// DependencyKindTest indicates test-only metadata, reserved in v1 and not runtime-visible.
	DependencyKindTest DependencyKind = "test"
	// DependencyKindWorkspace indicates an intra-workspace dependency reference.
	DependencyKindWorkspace DependencyKind = "workspace"
)

type WorkspaceGraph struct {
	WorkspaceName  string
	Packages       []*PackageNode
	PackagesByName map[string]*PackageNode
}

type PackageNode struct {
	Name              string
	Version           string
	Root              string
	License           string
	Kind              string
	Dependencies      []*DependencyNode
	DependenciesByKey map[string]*DependencyNode
	Targets           []*TargetNode
	TargetsByName     map[string]*TargetNode
	TargetsByExport   map[string]*TargetNode
	Tools             []*DependencyNode
	Boundaries        []manifest.BoundaryRule
	Publish           manifest.PublishPolicy
	Policies          manifest.Policies
}

type TargetNode struct {
	Package          *PackageNode
	Name             string
	Export           string
	Entry            string
	Runtime          string
	Types            string
	Optional         bool
	RuntimeDeps      []*DependencyNode
	PeerDeps         []*DependencyNode
	OptionalPeerDeps []*DependencyNode
	TypeDeps         []*DependencyNode
}

type DependencyNode struct {
	Package  *PackageNode
	Key      string
	Kind     DependencyKind
	Source   manifest.Source
	Optional bool
}

type DependencyReachability struct {
	Dependency     *DependencyNode
	RuntimeTargets []*TargetNode
	PeerTargets    []*TargetNode
	ToolOnly       bool
	OptionalOnly   bool
}

func Build(ir *manifest.ManifestIR) (*WorkspaceGraph, []diag.Diagnostic) {
	g := &WorkspaceGraph{PackagesByName: map[string]*PackageNode{}}
	var out []diag.Diagnostic
	add := func(code, msg string, details ...string) {
		out = append(out, diag.Diagnostic{Code: code, Severity: diag.SeverityError, Message: msg, Details: details})
	}
	if ir == nil {
		add("TSPACK_GRAPH_INTERNAL_ERROR", "nil manifest IR")
		diag.SortDiagnostics(out)
		return g, out
	}
	g.WorkspaceName = ir.Workspace.Name
	for _, p := range ir.Packages {
		if _, ok := g.PackagesByName[p.Name]; ok {
			add("TSPACK_GRAPH_DUPLICATE_PACKAGE", "duplicate package", p.Name)
			continue
		}
		pn := &PackageNode{Name: p.Name, Version: p.Version, Root: p.Root, License: p.License, Kind: p.Kind, DependenciesByKey: map[string]*DependencyNode{}, TargetsByName: map[string]*TargetNode{}, TargetsByExport: map[string]*TargetNode{}, Boundaries: append([]manifest.BoundaryRule(nil), p.Boundaries...), Publish: p.Publish, Policies: p.Policies}
		g.PackagesByName[p.Name] = pn
		g.Packages = append(g.Packages, pn)

		for _, d := range p.Dependencies {
			key := manifest.DependencyIdentity(d)
			if key == "" {
				add("TSPACK_GRAPH_DUPLICATE_DEPENDENCY", "dependency has empty key", p.Name)
				continue
			}
			if _, ok := pn.DependenciesByKey[key]; ok {
				add("TSPACK_GRAPH_DUPLICATE_DEPENDENCY", "duplicate dependency", p.Name, key)
				continue
			}
			dk := DependencyKind(d.Kind)
			if !validKind(dk) {
				add("TSPACK_GRAPH_INVALID_DEPENDENCY_KIND", "invalid dependency kind", p.Name, key, d.Kind)
				continue
			}
			dn := &DependencyNode{Package: pn, Key: key, Kind: dk, Source: d.Source, Optional: d.Optional}
			pn.DependenciesByKey[key] = dn
			pn.Dependencies = append(pn.Dependencies, dn)
		}

		for _, t := range p.Targets {
			dup := false
			if _, ok := pn.TargetsByName[t.Name]; ok {
				add("TSPACK_GRAPH_DUPLICATE_TARGET", "duplicate target", p.Name, t.Name)
				dup = true
			}
			if _, ok := pn.TargetsByExport[t.Export]; ok {
				add("TSPACK_GRAPH_DUPLICATE_EXPORT", "duplicate export", p.Name, t.Export)
				dup = true
			}
			if dup {
				continue
			}
			tn := &TargetNode{Package: pn, Name: t.Name, Export: t.Export, Entry: t.Entry, Runtime: t.Runtime, Types: t.Types, Optional: t.Optional}
			pn.TargetsByName[t.Name] = tn
			pn.TargetsByExport[t.Export] = tn
			pn.Targets = append(pn.Targets, tn)

			for _, ref := range t.Deps {
				dn, ok := pn.DependenciesByKey[ref]
				if !ok {
					add("TSPACK_GRAPH_UNKNOWN_DEPENDENCY_REF", "unknown target dependency ref", p.Name, t.Name, ref)
					continue
				}
				if dn.Kind == DependencyKindTool {
					add("TSPACK_GRAPH_INVALID_TARGET_REF", "target deps references tool dependency", p.Name, t.Name, ref)
					continue
				}
				tn.RuntimeDeps = append(tn.RuntimeDeps, dn)
				if dn.Kind == DependencyKindType {
					tn.TypeDeps = append(tn.TypeDeps, dn)
				}
			}
			for _, ref := range t.Peers {
				dn, ok := pn.DependenciesByKey[ref]
				if !ok {
					add("TSPACK_GRAPH_UNKNOWN_DEPENDENCY_REF", "unknown target peer ref", p.Name, t.Name, ref)
					continue
				}
				if dn.Kind != DependencyKindPeer {
					add("TSPACK_GRAPH_INVALID_TARGET_REF", "target peers must reference peer dependencies", p.Name, t.Name, ref)
					continue
				}
				tn.PeerDeps = append(tn.PeerDeps, dn)
				if dn.Optional {
					tn.OptionalPeerDeps = append(tn.OptionalPeerDeps, dn)
				}
			}
		}

		for _, tk := range p.Tools {
			dn, ok := pn.DependenciesByKey[tk]
			if !ok {
				add("TSPACK_GRAPH_UNKNOWN_DEPENDENCY_REF", "unknown tool dependency ref", p.Name, tk)
				continue
			}
			if dn.Kind != DependencyKindTool {
				add("TSPACK_GRAPH_INVALID_DEPENDENCY_KIND", "tool list must reference tool dependency", p.Name, tk)
				continue
			}
			pn.Tools = append(pn.Tools, dn)
		}

		sort.SliceStable(pn.Dependencies, func(i, j int) bool { return pn.Dependencies[i].Key < pn.Dependencies[j].Key })
		sort.SliceStable(pn.Targets, func(i, j int) bool { return pn.Targets[i].Name < pn.Targets[j].Name })
		sort.SliceStable(pn.Tools, func(i, j int) bool { return pn.Tools[i].Key < pn.Tools[j].Key })
		for _, t := range pn.Targets {
			sort.SliceStable(t.RuntimeDeps, func(i, j int) bool { return t.RuntimeDeps[i].Key < t.RuntimeDeps[j].Key })
			sort.SliceStable(t.PeerDeps, func(i, j int) bool { return t.PeerDeps[i].Key < t.PeerDeps[j].Key })
			sort.SliceStable(t.OptionalPeerDeps, func(i, j int) bool { return t.OptionalPeerDeps[i].Key < t.OptionalPeerDeps[j].Key })
			sort.SliceStable(t.TypeDeps, func(i, j int) bool { return t.TypeDeps[i].Key < t.TypeDeps[j].Key })
		}
	}
	sort.SliceStable(g.Packages, func(i, j int) bool { return g.Packages[i].Name < g.Packages[j].Name })
	diag.SortDiagnostics(out)
	return g, out
}

func validKind(k DependencyKind) bool {
	switch k {
	case DependencyKindRuntime, DependencyKindDep, DependencyKindPeer, DependencyKindTool, DependencyKindType, DependencyKindTest, DependencyKindWorkspace:
		return true
	}
	return false
}

func (g *WorkspaceGraph) Package(name string) (*PackageNode, bool) {
	p, ok := g.PackagesByName[name]
	return p, ok
}
func (g *WorkspaceGraph) AllPackages() []*PackageNode {
	return append([]*PackageNode(nil), g.Packages...)
}
func (p *PackageNode) Dependency(key string) (*DependencyNode, bool) {
	d, ok := p.DependenciesByKey[key]
	return d, ok
}
func (p *PackageNode) Target(name string) (*TargetNode, bool) {
	t, ok := p.TargetsByName[name]
	return t, ok
}
func (p *PackageNode) TargetByExport(e string) (*TargetNode, bool) {
	t, ok := p.TargetsByExport[e]
	return t, ok
}
func (p *PackageNode) AllDependencies() []*DependencyNode {
	return append([]*DependencyNode(nil), p.Dependencies...)
}
func (p *PackageNode) AllTargets() []*TargetNode { return append([]*TargetNode(nil), p.Targets...) }
func (p *PackageNode) ToolDependencies() []*DependencyNode {
	return append([]*DependencyNode(nil), p.Tools...)
}
func (t *TargetNode) AllowedRuntimeDependencies() []*DependencyNode {
	return append([]*DependencyNode(nil), t.RuntimeDeps...)
}
func (t *TargetNode) AllowedPeerDependencies() []*DependencyNode {
	return append([]*DependencyNode(nil), t.PeerDeps...)
}
func (t *TargetNode) AllowedDependencyKeys() []string {
	m := map[string]struct{}{}
	for _, d := range t.RuntimeDeps {
		m[d.Key] = struct{}{}
	}
	for _, d := range t.PeerDeps {
		m[d.Key] = struct{}{}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
func (t *TargetNode) AllowsDependencyKey(key string) bool {
	for _, k := range t.AllowedDependencyKeys() {
		if k == key {
			return true
		}
	}
	return false
}
func (t *TargetNode) AllowsExternalPackageName(name string) bool {
	for _, d := range append(t.RuntimeDeps, t.PeerDeps...) {
		if d.MatchesExternalPackageName(name) {
			return true
		}
	}
	return false
}

func (d *DependencyNode) MatchesExternalPackageName(name string) bool {
	for _, identifier := range d.ExternalPackageIdentifiers() {
		if identifier == name {
			return true
		}
	}
	return false
}

func (d *DependencyNode) ExternalPackageIdentifiers() []string {
	identifiers := []string{}
	seen := map[string]struct{}{}
	add := func(identifier string) {
		if identifier == "" {
			return
		}
		if _, ok := seen[identifier]; ok {
			return
		}
		seen[identifier] = struct{}{}
		identifiers = append(identifiers, identifier)
	}

	add(d.Key)
	add(d.Source.Name)
	add(d.Source.Package)
	if d.Source.Kind == "jsr" && strings.HasPrefix(d.Source.Package, "@") {
		parts := strings.Split(strings.TrimPrefix(d.Source.Package, "@"), "/")
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			add("@jsr/" + parts[0] + "__" + parts[1])
		}
	}
	return identifiers
}

func (p *PackageNode) TargetsAllowingDependencyKey(key string) []*TargetNode {
	out := []*TargetNode{}
	for _, t := range p.Targets {
		if t.AllowsDependencyKey(key) {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func (p *PackageNode) TargetsAllowingExternalPackageName(name string) []*TargetNode {
	out := []*TargetNode{}
	for _, t := range p.Targets {
		if t.AllowsExternalPackageName(name) {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func (p *PackageNode) DependencyReachability(key string) DependencyReachability {
	r := DependencyReachability{}
	d, ok := p.DependenciesByKey[key]
	if !ok {
		return r
	}
	r.Dependency = d
	for _, t := range p.Targets {
		for _, x := range t.RuntimeDeps {
			if x.Key == key {
				r.RuntimeTargets = append(r.RuntimeTargets, t)
				break
			}
		}
		for _, x := range t.PeerDeps {
			if x.Key == key {
				r.PeerTargets = append(r.PeerTargets, t)
				break
			}
		}
	}
	r.ToolOnly = d.Kind == DependencyKindTool && len(r.RuntimeTargets) == 0 && len(r.PeerTargets) == 0
	if d.Optional {
		r.OptionalOnly = len(r.RuntimeTargets) == 0 && len(r.PeerTargets) > 0
	}
	sort.SliceStable(r.RuntimeTargets, func(i, j int) bool { return r.RuntimeTargets[i].Name < r.RuntimeTargets[j].Name })
	sort.SliceStable(r.PeerTargets, func(i, j int) bool { return r.PeerTargets[i].Name < r.PeerTargets[j].Name })
	return r
}

func (d *DependencyNode) String() string { return fmt.Sprintf("%s(%s)", d.Key, d.Kind) }
