package why

import (
	"sort"
	"strings"

	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/graph"
	"github.com/tspack/tspack/internal/lockfile"
	"github.com/tspack/tspack/internal/manifest"
)

type Options struct {
	Query                    string
	PackageName              string
	Reverse                  bool
	AcknowledgedCapabilities []manifest.AcknowledgedCapability
}
type Result struct {
	Diagnostics  []diag.Diagnostic
	Explanations []Explanation
	LockPackages []LockPackageRef
	ReversePaths []ReversePath
	Notes        []string
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
type LockPackageRef struct {
	ID, Name, Version, Source, Hash string
	Capabilities                    []CapabilityRef
}

type CapabilityRef struct {
	Kind, Script, Command string
	Execution             string
	Acknowledged          bool
	AcknowledgementReason string
}
type LockEdgeRef struct {
	From, To, Kind string
	Optional       bool
}

type ReversePath struct {
	LockPackage string
	Root        string
	Path        []string
	Edges       []LockEdgeRef
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
	if opts.Reverse {
		return analyzeReverse(g, lf, opts)
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
			addDependencyLockDetails(&e, lf, p, d.Source.Package, opts.AcknowledgedCapabilities)
			out.Explanations = append(out.Explanations, e)
		}
		if t, ok := p.Target(q); ok {
			e := Explanation{Query: q, PackageName: p.Name, TargetName: t.Name, MatchType: "target", Kind: "target"}
			e.DeclaredBy = append(e.DeclaredBy, DeclarationReason{PackageName: p.Name, Scope: "target", TargetName: t.Name})
			for _, d := range append(t.RuntimeDeps, t.PeerDeps...) {
				e.DeclaredBy = append(e.DeclaredBy, DeclarationReason{PackageName: p.Name, Scope: "target", TargetName: t.Name, DependencyKey: d.Key, Kind: string(d.Kind), Optional: d.Optional, SourceKind: d.Source.Kind, SourcePackage: d.Source.Package, SourceRange: d.Source.Range})
			}
			addTargetLockDetails(&e, lf, p.Name, t.Name, opts.AcknowledgedCapabilities)
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
			e.LockPackages = append(e.LockPackages, lockPackageRef(lp, opts.AcknowledgedCapabilities))
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

func addDependencyLockDetails(e *Explanation, lf *lockfile.Lockfile, pkg *graph.PackageNode, packageName string, acknowledgements []manifest.AcknowledgedCapability) {
	if lf == nil || pkg == nil || packageName == "" {
		return
	}

	matchingPackageIDs := lockPackageIDsByName(lf, packageName)
	if len(matchingPackageIDs) == 0 {
		return
	}

	rootFromValues := dependencyRootFromValues(e, pkg)
	rootEdges := lockEdgesFromRootsToPackages(lf, rootFromValues, matchingPackageIDs)
	addScopedLockDetails(e, lf, rootEdges, acknowledgements)
}

func addTargetLockDetails(e *Explanation, lf *lockfile.Lockfile, packageName string, targetName string, acknowledgements []manifest.AcknowledgedCapability) {
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
	addScopedLockDetails(e, lf, rootEdges, acknowledgements)
}

func addScopedLockDetails(e *Explanation, lf *lockfile.Lockfile, rootEdges []lockfile.Edge, acknowledgements []manifest.AcknowledgedCapability) {
	if len(rootEdges) == 0 {
		return
	}

	sortLockfileEdges(rootEdges)
	packageByID := lockPackageRefsByID(lf, acknowledgements)
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

func lockPackageRefsByID(lf *lockfile.Lockfile, acknowledgements []manifest.AcknowledgedCapability) map[string]LockPackageRef {
	refs := map[string]LockPackageRef{}
	for _, pkg := range lf.Packages {
		refs[pkg.ID] = lockPackageRef(pkg, acknowledgements)
	}
	return refs
}

func lockPackageRef(pkg lockfile.Package, acknowledgements []manifest.AcknowledgedCapability) LockPackageRef {
	ref := LockPackageRef{ID: pkg.ID, Name: pkg.Name, Version: pkg.Version, Source: pkg.Source, Hash: pkg.Hash}
	for _, capability := range pkg.Capabilities {
		if capability.Kind != "lifecycleScript" && capability.Kind != "lifecycle-script" {
			continue
		}
		script := capability.Script
		if script == "" {
			script = capability.Detail
		}
		acknowledged, reason := lifecycleAcknowledgementForCapability(pkg.ID, script, capability.Command, acknowledgements)
		ref.Capabilities = append(ref.Capabilities, CapabilityRef{
			Kind:                  "lifecycleScript",
			Script:                script,
			Command:               capability.Command,
			Execution:             "blocked",
			Acknowledged:          acknowledged,
			AcknowledgementReason: reason,
		})
	}
	sort.SliceStable(ref.Capabilities, func(i, j int) bool {
		if ref.Capabilities[i].Kind != ref.Capabilities[j].Kind {
			return ref.Capabilities[i].Kind < ref.Capabilities[j].Kind
		}
		if ref.Capabilities[i].Script != ref.Capabilities[j].Script {
			return ref.Capabilities[i].Script < ref.Capabilities[j].Script
		}
		return ref.Capabilities[i].Command < ref.Capabilities[j].Command
	})
	return ref
}

func lifecycleAcknowledgementForCapability(packageID string, script string, command string, acknowledgements []manifest.AcknowledgedCapability) (bool, string) {
	for _, acknowledgement := range acknowledgements {
		if acknowledgement.Package != packageID {
			continue
		}
		if acknowledgement.Kind != "lifecycleScript" {
			continue
		}
		if acknowledgement.Script != script {
			continue
		}
		if acknowledgement.Command != command {
			continue
		}
		return true, acknowledgement.Reason
	}
	return false, ""
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

func analyzeReverse(g *graph.WorkspaceGraph, lf *lockfile.Lockfile, opts Options) Result {
	out := Result{}
	query := strings.TrimSpace(opts.Query)
	if lf == nil {
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{
			Code:     "TSPACK_WHY_LOCKFILE_MISSING",
			Severity: diag.SeverityError,
			Message:  "lockfile is missing",
			Details:  []string{"reverse why requires a lockfile; run tspack update"},
		})
		return out
	}

	if opts.PackageName != "" {
		if _, ok := g.Package(opts.PackageName); !ok {
			out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_WHY_PACKAGE_NOT_FOUND", Severity: diag.SeverityError, Message: "package not found", Details: []string{opts.PackageName}})
			return out
		}
	}

	matchedPackages := reverseQueryLockPackages(g, lf, query)
	if len(matchedPackages) == 0 {
		out.Diagnostics = append(out.Diagnostics, buildReverseWhyNotFoundDiagnostic(query, lf))
		diag.SortDiagnostics(out.Diagnostics)
		return out
	}

	for _, lockPackage := range matchedPackages {
		out.LockPackages = append(out.LockPackages, lockPackageRef(lockPackage, opts.AcknowledgedCapabilities))
	}
	sortLockPackages(out.LockPackages)

	lockPackageIDs := lockPackageIDSet(lf)
	incomingEdges := incomingEdgesByTarget(lf)
	seenPaths := map[string]bool{}
	for _, lockPackage := range out.LockPackages {
		paths := reversePathsForLockPackage(lockPackage.ID, incomingEdges, lockPackageIDs, opts.PackageName)
		for _, path := range paths {
			key := reversePathKey(path)
			if seenPaths[key] {
				continue
			}
			seenPaths[key] = true
			out.ReversePaths = append(out.ReversePaths, path)
		}
	}
	sortReversePaths(out.ReversePaths)

	if opts.PackageName != "" && len(out.ReversePaths) == 0 {
		out.Notes = append(out.Notes, "package filter matched no roots")
	}

	diag.SortDiagnostics(out.Diagnostics)
	return out
}

func reverseQueryLockPackages(g *graph.WorkspaceGraph, lf *lockfile.Lockfile, query string) []lockfile.Package {
	if lf == nil {
		return nil
	}

	matchesByID := []lockfile.Package{}
	for _, lockPackage := range lf.Packages {
		if lockPackage.ID == query {
			matchesByID = append(matchesByID, lockPackage)
		}
	}
	if len(matchesByID) > 0 {
		sortLockfilePackages(matchesByID)
		return matchesByID
	}

	packageName := query
	if strings.HasPrefix(query, "npm:") {
		packageName = strings.TrimPrefix(query, "npm:")
	}

	matchesByName := lockPackagesByName(lf, packageName)
	if len(matchesByName) > 0 {
		return matchesByName
	}

	if g == nil {
		return nil
	}

	declaredPackageNames := map[string]bool{}
	for _, pkg := range g.AllPackages() {
		for _, dep := range pkg.AllDependencies() {
			if dep.Key == query && dep.Source.Kind == "npm" && dep.Source.Package != "" {
				declaredPackageNames[dep.Source.Package] = true
			}
		}
	}
	if len(declaredPackageNames) != 1 {
		return nil
	}

	declaredPackageName := ""
	for name := range declaredPackageNames {
		declaredPackageName = name
	}
	return lockPackagesByName(lf, declaredPackageName)
}

func lockPackagesByName(lf *lockfile.Lockfile, packageName string) []lockfile.Package {
	matches := []lockfile.Package{}
	for _, lockPackage := range lf.Packages {
		if lockPackage.Name == packageName {
			matches = append(matches, lockPackage)
		}
	}
	sortLockfilePackages(matches)
	return matches
}

func sortLockfilePackages(packages []lockfile.Package) {
	sort.SliceStable(packages, func(i, j int) bool {
		return packages[i].ID < packages[j].ID
	})
}

func lockPackageIDSet(lf *lockfile.Lockfile) map[string]bool {
	ids := map[string]bool{}
	for _, lockPackage := range lf.Packages {
		ids[lockPackage.ID] = true
	}
	return ids
}

func incomingEdgesByTarget(lf *lockfile.Lockfile) map[string][]lockfile.Edge {
	incoming := map[string][]lockfile.Edge{}
	for _, edge := range lf.Edges {
		incoming[edge.To] = append(incoming[edge.To], edge)
	}
	for target := range incoming {
		sortLockfileEdges(incoming[target])
	}
	return incoming
}

func reversePathsForLockPackage(lockPackageID string, incomingEdges map[string][]lockfile.Edge, lockPackageIDs map[string]bool, packageFilter string) []ReversePath {
	paths := []ReversePath{}
	seenPaths := map[string]bool{}
	visited := map[string]bool{lockPackageID: true}
	walkReversePaths(lockPackageID, lockPackageID, nil, incomingEdges, lockPackageIDs, packageFilter, visited, seenPaths, &paths)
	sortReversePaths(paths)
	return paths
}

func walkReversePaths(queryLockPackage string, current string, edgePath []LockEdgeRef, incomingEdges map[string][]lockfile.Edge, lockPackageIDs map[string]bool, packageFilter string, visited map[string]bool, seenPaths map[string]bool, paths *[]ReversePath) {
	for _, incoming := range incomingEdges[current] {
		edge := LockEdgeRef(incoming)
		nextEdgePath := prependLockEdge(edge, edgePath)
		if !lockPackageIDs[incoming.From] {
			root := incoming.From
			if packageFilter != "" && !rootBelongsToPackage(root, packageFilter) {
				continue
			}
			path := ReversePath{
				LockPackage: queryLockPackage,
				Root:        root,
				Path:        nodesFromEdges(nextEdgePath),
				Edges:       nextEdgePath,
			}
			key := reversePathKey(path)
			if seenPaths[key] {
				continue
			}
			seenPaths[key] = true
			*paths = append(*paths, path)
			continue
		}

		if visited[incoming.From] {
			continue
		}
		visited[incoming.From] = true
		walkReversePaths(queryLockPackage, incoming.From, nextEdgePath, incomingEdges, lockPackageIDs, packageFilter, visited, seenPaths, paths)
		delete(visited, incoming.From)
	}
}

func prependLockEdge(edge LockEdgeRef, edges []LockEdgeRef) []LockEdgeRef {
	out := make([]LockEdgeRef, 0, len(edges)+1)
	out = append(out, edge)
	out = append(out, edges...)
	return out
}

func nodesFromEdges(edges []LockEdgeRef) []string {
	if len(edges) == 0 {
		return nil
	}
	nodes := make([]string, 0, len(edges)+1)
	nodes = append(nodes, edges[0].From)
	for _, edge := range edges {
		nodes = append(nodes, edge.To)
	}
	return nodes
}

func rootBelongsToPackage(root string, packageName string) bool {
	return strings.HasPrefix(root, packageName+":")
}

func reversePathKey(path ReversePath) string {
	return path.LockPackage + "|" + strings.Join(path.Path, "|")
}

func sortReversePaths(paths []ReversePath) {
	sort.SliceStable(paths, func(i, j int) bool {
		a := paths[i]
		b := paths[j]
		if a.LockPackage != b.LockPackage {
			return a.LockPackage < b.LockPackage
		}
		if a.Root != b.Root {
			return a.Root < b.Root
		}
		return strings.Join(a.Path, "\x00") < strings.Join(b.Path, "\x00")
	})
}

func buildReverseWhyNotFoundDiagnostic(query string, lf *lockfile.Lockfile) diag.Diagnostic {
	details := []string{
		"no lock package matched reverse query \"" + query + "\"",
		"try `tspack why <declared-dep>` for manifest declarations",
	}

	matchingLockIDs := matchingLockPackageIDs(query, lf)
	if len(matchingLockIDs) > 0 {
		details = append(details, "matching lock packages exist:")
		for _, id := range matchingLockIDs {
			details = append(details, "  "+id)
		}
	}

	return diag.Diagnostic{
		Code:     "TSPACK_WHY_NOT_FOUND",
		Severity: diag.SeverityError,
		Message:  "why reverse query not found: " + query,
		Details:  details,
	}
}
