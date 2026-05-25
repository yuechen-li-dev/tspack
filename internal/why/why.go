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
	SourceKind, SourcePackage                           string
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
			e.DeclaredBy = append(e.DeclaredBy, DeclarationReason{PackageName: p.Name, DependencyKey: d.Key, Kind: string(d.Kind), Optional: d.Optional, SourceKind: d.Source.Kind, SourcePackage: d.Source.Package})
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
			addLockDetails(&e, lf, d.Source.Package)
			out.Explanations = append(out.Explanations, e)
		}
		if t, ok := p.Target(q); ok {
			e := Explanation{Query: q, PackageName: p.Name, TargetName: t.Name, MatchType: "target", Kind: "target"}
			e.DeclaredBy = append(e.DeclaredBy, DeclarationReason{PackageName: p.Name, Scope: "target", TargetName: t.Name})
			for _, d := range append(t.RuntimeDeps, t.PeerDeps...) {
				e.DeclaredBy = append(e.DeclaredBy, DeclarationReason{PackageName: p.Name, Scope: "target", TargetName: t.Name, DependencyKey: d.Key, Kind: string(d.Kind), Optional: d.Optional, SourceKind: d.Source.Kind, SourcePackage: d.Source.Package})
			}
			if lf != nil {
				from := p.Name + ":target:" + t.Name
				for _, edge := range lf.Edges {
					if edge.From == from {
						e.LockEdges = append(e.LockEdges, LockEdgeRef(edge))
					}
				}
			}
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
		details = append(details, "  tspack why "+matchingLockIDs[0])
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
	matches := []string{}
	for _, pkg := range lf.Packages {
		if pkg.Name == query {
			matches = append(matches, pkg.ID)
		}
	}
	sort.Strings(matches)
	return matches
}

func addLockDetails(e *Explanation, lf *lockfile.Lockfile, packageName string) {
	if lf == nil || packageName == "" {
		return
	}
	for _, lp := range lf.Packages {
		if lp.Name == packageName {
			e.LockPackages = append(e.LockPackages, LockPackageRef{ID: lp.ID, Name: lp.Name, Version: lp.Version, Source: lp.Source, Hash: lp.Hash})
		}
	}
	for _, edge := range lf.Edges {
		for _, lp := range e.LockPackages {
			if edge.To == lp.ID || edge.From == lp.ID {
				e.LockEdges = append(e.LockEdges, LockEdgeRef(edge))
			}
		}
	}
	dedupeLockEdges(e)
}

func dedupeLockEdges(e *Explanation) {
	if len(e.LockEdges) < 2 {
		return
	}
	seen := map[string]bool{}
	deduped := make([]LockEdgeRef, 0, len(e.LockEdges))
	for _, edge := range e.LockEdges {
		key := edge.From + "|" + edge.To + "|" + edge.Kind
		if edge.Optional {
			key += "|optional"
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, edge)
	}
	e.LockEdges = deduped
}
