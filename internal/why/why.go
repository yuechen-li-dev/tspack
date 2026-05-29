package why

import (
	"sort"
	"strings"

	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/graph"
	"github.com/tspack/tspack/internal/lockfile"
)

type Options struct{ Query, PackageName string }
type Result struct {
	Diagnostics  []diag.Diagnostic
	Explanations []Explanation
}
type Explanation struct {
	Query, PackageName, DependencyKey, ExternalPackageName, TargetName, Kind string
	Optional                                                                 bool
	MatchType                                                                string
	DeclaredBy                                                               []DeclarationReason
	ReachableFrom, NotReachableFrom                                          []ReachabilityRef
	LockPackages                                                             []LockPackageRef
	LockEdges                                                                []LockEdgeRef
	DirectProject                                                            *bool
}
type DeclarationReason struct {
	PackageName, Scope, TargetName, DependencyKey, Kind string
	Optional                                            bool
	SourceKind, SourcePackage, SourceRange              string
}
type ReachabilityRef struct{ PackageName, TargetName, Reason string }
type LockPackageRef struct{ ID, Name, Version, Source, Hash string }
type LockEdgeRef struct {
	From, To, Kind string
	Optional       bool
}

func Analyze(g *graph.WorkspaceGraph, lf *lockfile.Lockfile, opts Options) Result { /*same*/
	out := Result{}
	if strings.TrimSpace(opts.Query) == "" {
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_WHY_QUERY_REQUIRED", Severity: diag.SeverityError, Message: "why query is required"})
		return out
	}
	if g == nil {
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_WHY_GRAPH_INVALID", Severity: diag.SeverityError, Message: "workspace graph is invalid"})
		return out
	}
	q := opts.Query
	pkgs := g.AllPackages()
	if opts.PackageName != "" {
		p, ok := g.Package(opts.PackageName)
		if !ok {
			out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_WHY_PACKAGE_NOT_FOUND", Severity: diag.SeverityError, Message: "package not found", Details: []string{opts.PackageName}})
			return out
		}
		pkgs = []*graph.PackageNode{p}
	}
	for _, p := range pkgs {
		for _, d := range p.AllDependencies() {
			if d.Key != q && d.Source.Package != q {
				continue
			}
			e := Explanation{Query: q, PackageName: p.Name, DependencyKey: d.Key, ExternalPackageName: d.Source.Package, Kind: string(d.Kind), Optional: d.Optional, MatchType: "dependency"}
			e.DeclaredBy = append(e.DeclaredBy, DeclarationReason{PackageName: p.Name, DependencyKey: d.Key, Kind: string(d.Kind), Optional: d.Optional, SourceKind: d.Source.Kind, SourcePackage: d.Source.Package, SourceRange: d.Source.Range})
			r := p.DependencyReachability(d.Key)
			for _, t := range r.RuntimeTargets {
				e.ReachableFrom = append(e.ReachableFrom, ReachabilityRef{PackageName: p.Name, TargetName: t.Name, Reason: "runtime"})
			}
			for _, t := range r.PeerTargets {
				e.ReachableFrom = append(e.ReachableFrom, ReachabilityRef{PackageName: p.Name, TargetName: t.Name, Reason: "peer"})
			}
			for _, t := range p.AllTargets() {
				if !t.AllowsDependencyKey(d.Key) {
					e.NotReachableFrom = append(e.NotReachableFrom, ReachabilityRef{PackageName: p.Name, TargetName: t.Name, Reason: "not-allowed"})
				}
			}
			addDependencyLockDetails(&e, lf, p, d.Source.Package)
			out.Explanations = append(out.Explanations, e)
		}
		if t, ok := p.Target(q); ok {
			e := Explanation{Query: q, PackageName: p.Name, TargetName: t.Name, MatchType: "target", Kind: "target"}
			e.DeclaredBy = append(e.DeclaredBy, DeclarationReason{PackageName: p.Name, Scope: "target", TargetName: t.Name})
			for _, d := range append(t.RuntimeDeps, t.PeerDeps...) {
				e.DeclaredBy = append(e.DeclaredBy, DeclarationReason{PackageName: p.Name, Scope: "target", TargetName: t.Name, DependencyKey: d.Key, Kind: string(d.Kind), Optional: d.Optional, SourceKind: d.Source.Kind, SourcePackage: d.Source.Package, SourceRange: d.Source.Range})
			}
			addTargetLockDetails(&e, lf, p.Name, t.Name)
			out.Explanations = append(out.Explanations, e)
		}
	}
	if lf != nil && strings.Contains(q, ":") && strings.Contains(q, "@") {
		for _, lp := range lf.Packages {
			if lp.ID != q {
				continue
			}
			direct := false
			e := Explanation{Query: q, MatchType: "lock-package"}
			e.LockPackages = append(e.LockPackages, LockPackageRef{ID: lp.ID, Name: lp.Name, Version: lp.Version, Source: lp.Source, Hash: lp.Hash})
			for _, edge := range lf.Edges {
				if edge.To == q || edge.From == q {
					e.LockEdges = append(e.LockEdges, LockEdgeRef(edge))
				}
				if edge.To == q && strings.Contains(edge.From, ":target:") {
					direct = true
				}
			}
			dedupeLockEdges(&e)
			e.DirectProject = &direct
			out.Explanations = append(out.Explanations, e)
		}
	}
	if len(out.Explanations) == 0 {
		out.Diagnostics = append(out.Diagnostics, buildWhyNotFoundDiagnostic(q, lf))
	}
	sort.SliceStable(out.Explanations, func(i, j int) bool {
		a, b := out.Explanations[i], out.Explanations[j]
		if a.PackageName != b.PackageName {
			return a.PackageName < b.PackageName
		}
		if a.MatchType != b.MatchType {
			return a.MatchType < b.MatchType
		}
		if a.DependencyKey != b.DependencyKey {
			return a.DependencyKey < b.DependencyKey
		}
		return a.TargetName < b.TargetName
	})
	diag.SortDiagnostics(out.Diagnostics)
	return out
}

func buildWhyNotFoundDiagnostic(query string, lf *lockfile.Lockfile) diag.Diagnostic {
	details := []string{
		"no declared dependency key or target matched \"" + query + "\"",
	}

	matchingLockIDs := matchingLockPackageIDs(query, lf)
	if len(matchingLockIDs) > 0 {
		details = append(details, "matching lock packages exist:")
		for _, id := range matchingLockIDs {
			details = append(details, "  "+id)
		}
		details = append(details, "try:")
		for _, id := range matchingLockIDs {
			details = append(details, "  tspack why "+id)
		}
	}

	return diag.Diagnostic{
		Code:     "TSPACK_WHY_NOT_FOUND",
		Severity: diag.SeverityError,
		Message:  "why query not found: " + query,
		Details:  details,
	}
}

func matchingLockPackageIDs(query string, lf *lockfile.Lockfile) []string {
	if lf == nil {
		return nil
	}

	packageName := query
	if strings.HasPrefix(query, "npm:") && !lockPackageIDExists(query, lf) {
		packageName = strings.TrimPrefix(query, "npm:")
	}

	matches := []string{}
	for _, pkg := range lf.Packages {
		if pkg.Name == packageName {
			matches = append(matches, pkg.ID)
		}
	}
	sort.Strings(matches)
	return matches
}

func lockPackageIDExists(query string, lf *lockfile.Lockfile) bool {
	for _, pkg := range lf.Packages {
		if pkg.ID == query {
			return true
		}
	}
	return false
}

func addDependencyLockDetails(e *Explanation, lf *lockfile.Lockfile, pkg *graph.PackageNode, packageName string) {
	if lf == nil || pkg == nil || packageName == "" {
		return
	}

	matchingPackageIDs := lockPackageIDsByName(lf, packageName)
	if len(matchingPackageIDs) == 0 {
		return
	}

	rootFromValues := dependencyRootFromValues(e, pkg)
	rootEdges := lockEdgesFromRootsToPackages(lf, rootFromValues, matchingPackageIDs)
	addScopedLockDetails(e, lf, rootEdges)
}

func addTargetLockDetails(e *Explanation, lf *lockfile.Lockfile, packageName string, targetName string) {
	if lf == nil || packageName == "" || targetName == "" {
		return
	}

	from := packageName + ":target:" + targetName
	rootEdges := []lockfile.Edge{}
	for _, edge := range lf.Edges {
		if edge.From == from {
			rootEdges = append(rootEdges, edge)
		}
	}
	addScopedLockDetails(e, lf, rootEdges)
}

func addScopedLockDetails(e *Explanation, lf *lockfile.Lockfile, rootEdges []lockfile.Edge) {
	if len(rootEdges) == 0 {
		return
	}

	sortLockfileEdges(rootEdges)
	packageByID := lockPackageRefsByID(lf)
	seenPackages := map[string]bool{}

	for _, edge := range rootEdges {
		if pkg, ok := packageByID[edge.To]; ok && !seenPackages[pkg.ID] {
			e.LockPackages = append(e.LockPackages, pkg)
			seenPackages[pkg.ID] = true
		}
	}

	for _, edge := range reachableLockEdges(lf, rootEdges) {
		e.LockEdges = append(e.LockEdges, LockEdgeRef(edge))
		if pkg, ok := packageByID[edge.To]; ok && !seenPackages[pkg.ID] {
			e.LockPackages = append(e.LockPackages, pkg)
			seenPackages[pkg.ID] = true
		}
	}

	dedupeLockEdges(e)
	sortLockPackages(e.LockPackages)
}

func dependencyRootFromValues(e *Explanation, pkg *graph.PackageNode) []string {
	rootValues := []string{}
	seen := map[string]bool{}

	for _, ref := range e.ReachableFrom {
		from := ref.PackageName + ":target:" + ref.TargetName
		if seen[from] {
			continue
		}
		seen[from] = true
		rootValues = append(rootValues, from)
	}

	if e.Kind == "tool" {
		from := pkg.Name + ":tool"
		if !seen[from] {
			rootValues = append(rootValues, from)
		}
	}

	sort.Strings(rootValues)
	return rootValues
}

func lockEdgesFromRootsToPackages(lf *lockfile.Lockfile, rootFromValues []string, packageIDs map[string]bool) []lockfile.Edge {
	rootFromSet := map[string]bool{}
	for _, from := range rootFromValues {
		rootFromSet[from] = true
	}

	rootEdges := []lockfile.Edge{}
	for _, edge := range lf.Edges {
		if rootFromSet[edge.From] && packageIDs[edge.To] {
			rootEdges = append(rootEdges, edge)
		}
	}
	sortLockfileEdges(rootEdges)
	return rootEdges
}

func lockPackageIDsByName(lf *lockfile.Lockfile, packageName string) map[string]bool {
	packageIDs := map[string]bool{}
	for _, pkg := range lf.Packages {
		if pkg.Name == packageName {
			packageIDs[pkg.ID] = true
		}
	}
	return packageIDs
}

func lockPackageRefsByID(lf *lockfile.Lockfile) map[string]LockPackageRef {
	refs := map[string]LockPackageRef{}
	for _, pkg := range lf.Packages {
		refs[pkg.ID] = LockPackageRef{ID: pkg.ID, Name: pkg.Name, Version: pkg.Version, Source: pkg.Source, Hash: pkg.Hash}
	}
	return refs
}

func reachableLockEdges(lf *lockfile.Lockfile, rootEdges []lockfile.Edge) []lockfile.Edge {
	edgesByFrom := map[string][]lockfile.Edge{}
	for _, edge := range lf.Edges {
		edgesByFrom[edge.From] = append(edgesByFrom[edge.From], edge)
	}
	for from := range edgesByFrom {
		sortLockfileEdges(edgesByFrom[from])
	}

	result := []lockfile.Edge{}
	seenEdges := map[string]bool{}
	visitedPackages := map[string]bool{}
	queue := []string{}

	for _, edge := range rootEdges {
		appendEdgeOnce(&result, seenEdges, edge)
		if !visitedPackages[edge.To] {
			visitedPackages[edge.To] = true
			queue = append(queue, edge.To)
		}
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, edge := range edgesByFrom[current] {
			appendEdgeOnce(&result, seenEdges, edge)
			if !visitedPackages[edge.To] {
				visitedPackages[edge.To] = true
				queue = append(queue, edge.To)
			}
		}
	}

	return result
}

func appendEdgeOnce(result *[]lockfile.Edge, seen map[string]bool, edge lockfile.Edge) {
	key := lockEdgeKey(LockEdgeRef(edge))
	if seen[key] {
		return
	}
	seen[key] = true
	*result = append(*result, edge)
}

func sortLockfileEdges(edges []lockfile.Edge) {
	sort.SliceStable(edges, func(i, j int) bool {
		return lockEdgeKey(LockEdgeRef(edges[i])) < lockEdgeKey(LockEdgeRef(edges[j]))
	})
}

func sortLockPackages(packages []LockPackageRef) {
	sort.SliceStable(packages, func(i, j int) bool {
		return packages[i].ID < packages[j].ID
	})
}

func dedupeLockEdges(e *Explanation) {
	if len(e.LockEdges) < 2 {
		return
	}
	seen := map[string]bool{}
	deduped := make([]LockEdgeRef, 0, len(e.LockEdges))
	for _, edge := range e.LockEdges {
		key := lockEdgeKey(edge)
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, edge)
	}
	e.LockEdges = deduped
}

func lockEdgeKey(edge LockEdgeRef) string {
	key := edge.From + "|" + edge.To + "|" + edge.Kind
	if edge.Optional {
		key += "|optional"
	}
	return key
}
