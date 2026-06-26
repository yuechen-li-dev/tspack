package check

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/boundary"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/importscan"
)

type ExplainOptions struct {
	RootDir string
	Graph   *graph.WorkspaceGraph
	File    string
}

type ExplainResult struct {
	Command       string                    `json:"command"`
	Mode          string                    `json:"mode"`
	Root          string                    `json:"root"`
	File          string                    `json:"file"`
	ReachableFrom []ExplainReachability     `json:"reachableFrom"`
	MatchedRules  []ExplainBoundaryRule     `json:"matchedRules"`
	Imports       []ExplainImport           `json:"imports"`
	Diagnostics   []CheckJSONDiagnosticLike `json:"diagnostics"`
	Notes         []string                  `json:"notes,omitempty"`
}

type CheckJSONDiagnosticLike struct {
	Code     string   `json:"code"`
	Severity string   `json:"severity"`
	Message  string   `json:"message"`
	File     string   `json:"file,omitempty"`
	Details  []string `json:"details,omitempty"`
}

type ExplainReachability struct {
	Target string   `json:"target"`
	Path   []string `json:"path"`
}

type ExplainBoundaryRule struct {
	From           string   `json:"from,omitempty"`
	TransitiveFrom string   `json:"transitiveFrom,omitempty"`
	Seed           string   `json:"seed,omitempty"`
	Path           []string `json:"path,omitempty"`
	AllowDeps      []string `json:"allowDeps,omitempty"`
	DenyDeps       []string `json:"denyDeps,omitempty"`
	DenyTypeDeps   []string `json:"denyTypeDeps,omitempty"`
	AllowOnly      []string `json:"allowOnly,omitempty"`
}

type ExplainImport struct {
	Specifier  string                  `json:"specifier,omitempty"`
	Package    string                  `json:"package,omitempty"`
	Kind       string                  `json:"kind"`
	TypeOnly   bool                    `json:"typeOnly,omitempty"`
	Resolved   string                  `json:"resolved,omitempty"`
	Decision   string                  `json:"decision,omitempty"`
	Diagnostic string                  `json:"diagnostic,omitempty"`
	Reasons    []string                `json:"reasons,omitempty"`
	Targets    []ExplainTargetDecision `json:"targets,omitempty"`
}

type ExplainTargetDecision struct {
	Target     string   `json:"target"`
	Decision   string   `json:"decision"`
	Diagnostic string   `json:"diagnostic,omitempty"`
	Reasons    []string `json:"reasons,omitempty"`
}

func Explain(opts ExplainOptions) ExplainResult {
	res := ExplainResult{Command: "check", Mode: "explain", Root: opts.RootDir, File: filepath.ToSlash(opts.File)}
	res.Notes = append(res.Notes, "boundary `from` matches the file containing the import statement.")
	if opts.Graph == nil {
		res.Diagnostics = append(res.Diagnostics, explainDiag("TSPACK_CHECK_EXPLAIN_FAILED", "nil graph", ""))
		return res
	}
	absFile := filepath.Clean(filepath.Join(opts.RootDir, opts.File))
	res.ReachableFrom = findReachability(opts.RootDir, opts.Graph, absFile)
	transitiveMatches := findTransitiveRuleMatches(opts.RootDir, opts.Graph, absFile)
	typeTransitiveMatches := findTransitiveTypeRuleMatches(opts.RootDir, opts.Graph, absFile)
	res.MatchedRules = findMatchedRules(opts.RootDir, opts.Graph, absFile, transitiveMatches, typeTransitiveMatches)
	res.Imports = explainImports(opts.RootDir, opts.Graph, absFile, res.ReachableFrom, transitiveMatches, typeTransitiveMatches)
	_, scanDiags := importscan.ScanFile(absFile)
	for _, scanDiag := range scanDiags {
		res.Diagnostics = append(res.Diagnostics, explainDiagnostic(scanDiag))
	}
	if len(res.ReachableFrom) == 0 {
		res.Notes = append(res.Notes, "file is not reachable from any declared target entry.")
	}
	return res
}

func findReachability(root string, g *graph.WorkspaceGraph, absFile string) []ExplainReachability {
	out := []ExplainReachability{}
	for _, p := range g.AllPackages() {
		pkgRoot := p.Root
		if pkgRoot == "" {
			pkgRoot = "."
		}
		for _, t := range p.AllTargets() {
			entry := filepath.Clean(filepath.Join(root, pkgRoot, t.Entry))
			if path, ok := firstPathTo(entry, absFile); ok {
				out = append(out, ExplainReachability{Target: t.Name, Path: relPathList(root, path)})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Target != out[j].Target {
			return out[i].Target < out[j].Target
		}
		return strings.Join(out[i].Path, "\x00") < strings.Join(out[j].Path, "\x00")
	})
	return out
}

func firstPathTo(entry string, target string) ([]string, bool) {
	q := []string{entry}
	parents := map[string]string{}
	seen := map[string]bool{}
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		if cur == target {
			return buildFilePath(parents, entry, target), true
		}
		imports, _ := importscan.ScanFile(cur)
		importscan.SortImports(imports)
		for _, imp := range imports {
			if imp.Kind != importscan.ImportKindRuntime || imp.SpecifierKind != importscan.SpecifierRelativeInternal {
				continue
			}
			next, ok := importscan.ResolveRelative(cur, imp.Specifier)
			if !ok {
				continue
			}
			if _, exists := parents[next]; !exists {
				parents[next] = cur
			}
			q = append(q, next)
		}
	}
	return nil, false
}

func buildFilePath(parents map[string]string, entry string, target string) []string {
	out := []string{target}
	cur := target
	for cur != entry {
		parent, ok := parents[cur]
		if !ok {
			break
		}
		out = append([]string{parent}, out...)
		cur = parent
	}
	return out
}

func findMatchedRules(root string, g *graph.WorkspaceGraph, absFile string, transitiveMatches []boundary.TransitiveRuleMatch, typeTransitiveMatches []boundary.TransitiveRuleMatch) []ExplainBoundaryRule {
	out := []ExplainBoundaryRule{}
	for _, p := range g.AllPackages() {
		for _, rule := range p.Boundaries {
			if rule.From != "" && boundary.MatchFrom(rule.From, filepath.ToSlash(absFile)) {
				out = append(out, ExplainBoundaryRule{From: rule.From, AllowDeps: append([]string(nil), rule.AllowDeps...), DenyDeps: append([]string(nil), rule.DenyDeps...), DenyTypeDeps: append([]string(nil), rule.DenyTypeDeps...), AllowOnly: copyStringSlice(rule.AllowOnly)})
			}
		}
	}
	for _, match := range transitiveMatches {
		out = append(out, explainRuleFromTransitiveMatch(root, match))
	}
	for _, match := range typeTransitiveMatches {
		if len(match.Rule.DenyTypeDeps) == 0 {
			continue
		}
		out = append(out, explainRuleFromTransitiveMatch(root, match))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		if out[i].TransitiveFrom != out[j].TransitiveFrom {
			return out[i].TransitiveFrom < out[j].TransitiveFrom
		}
		return out[i].Seed < out[j].Seed
	})
	return dedupeExplainBoundaryRules(out)
}

func dedupeExplainBoundaryRules(rules []ExplainBoundaryRule) []ExplainBoundaryRule {
	seen := map[string]bool{}
	out := []ExplainBoundaryRule{}
	for _, rule := range rules {
		key := strings.Join([]string{
			rule.From,
			rule.TransitiveFrom,
			rule.Seed,
			strings.Join(rule.Path, "|"),
			strings.Join(rule.AllowDeps, "|"),
			strings.Join(rule.DenyDeps, "|"),
			strings.Join(rule.DenyTypeDeps, "|"),
			strings.Join(rule.AllowOnly, "|"),
		}, "||")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, rule)
	}
	return out
}

func explainRuleFromTransitiveMatch(root string, match boundary.TransitiveRuleMatch) ExplainBoundaryRule {
	return ExplainBoundaryRule{
		TransitiveFrom: match.Rule.TransitiveFrom,
		Seed:           relPath(root, match.Seed),
		Path:           relPathList(root, match.Path),
		AllowDeps:      append([]string(nil), match.Rule.AllowDeps...),
		DenyDeps:       append([]string(nil), match.Rule.DenyDeps...),
		DenyTypeDeps:   append([]string(nil), match.Rule.DenyTypeDeps...),
		AllowOnly:      copyStringSlice(match.Rule.AllowOnly),
	}
}

func findTransitiveRuleMatches(root string, g *graph.WorkspaceGraph, absFile string) []boundary.TransitiveRuleMatch {
	out := []boundary.TransitiveRuleMatch{}
	for _, p := range g.AllPackages() {
		matchesByFile := boundary.BuildTransitiveRuleMatches(root, p)
		out = append(out, matchesByFile[filepath.Clean(absFile)]...)
	}
	return out
}

func findTransitiveTypeRuleMatches(root string, g *graph.WorkspaceGraph, absFile string) []boundary.TransitiveRuleMatch {
	out := []boundary.TransitiveRuleMatch{}
	for _, p := range g.AllPackages() {
		matchesByFile := boundary.BuildTransitiveTypeRuleMatches(root, p)
		out = append(out, matchesByFile[filepath.Clean(absFile)]...)
	}
	return out
}

func explainImports(root string, g *graph.WorkspaceGraph, absFile string, reach []ExplainReachability, transitiveMatches []boundary.TransitiveRuleMatch, typeTransitiveMatches []boundary.TransitiveRuleMatch) []ExplainImport {
	imports, diags := importscan.ScanFile(absFile)
	_ = diags
	importscan.SortImports(imports)
	out := []ExplainImport{}
	for _, imp := range imports {
		typeOnly := imp.Kind == importscan.ImportKindTypeOnly
		switch imp.SpecifierKind {
		case importscan.SpecifierRelativeInternal:
			item := ExplainImport{Specifier: imp.Specifier, Kind: "relative", TypeOnly: typeOnly}
			if resolved, ok := importscan.ResolveRelative(absFile, imp.Specifier); ok {
				item.Resolved = relPath(root, resolved)
			} else {
				item.Decision = "unknown"
				item.Reasons = append(item.Reasons, "relative import could not be resolved")
			}
			out = append(out, item)
		case importscan.SpecifierExternalPackage:
			out = append(out, explainExternal(g, absFile, imp, reach, transitiveMatches, typeTransitiveMatches))
		}
	}
	return out
}

func explainExternal(g *graph.WorkspaceGraph, absFile string, imp importscan.Import, reach []ExplainReachability, transitiveMatches []boundary.TransitiveRuleMatch, typeTransitiveMatches []boundary.TransitiveRuleMatch) ExplainImport {
	item := ExplainImport{Specifier: imp.Specifier, Package: imp.Package, Kind: "external", TypeOnly: imp.Kind == importscan.ImportKindTypeOnly}
	if imp.Kind == importscan.ImportKindTypeOnly {
		return explainTypeExternal(g, absFile, item, imp.Package, typeTransitiveMatches)
	}
	if imp.Kind != importscan.ImportKindRuntime {
		item.Decision = "unknown"
		item.Reasons = append(item.Reasons, "dynamic import could not be classified")
		return item
	}
	if len(reach) == 0 {
		if ruleDeniesAny(g, absFile, imp.Package) {
			item.Decision = "denied"
			item.Diagnostic = "TSPACK_BOUNDARY_EXPLICIT_DENY"
			item.Reasons = append(item.Reasons, "denied by boundary rule matching this file")
			return item
		}
		if boundary.DeniedByTransitiveBoundary(transitiveMatches, imp.Package) {
			item.Decision = "denied"
			item.Diagnostic = "TSPACK_BOUNDARY_EXPLICIT_DENY"
			item.Reasons = append(item.Reasons, transitiveDenyReason(transitiveMatches))
			return item
		}
		if ruleAllowOnlyViolationAny(g, absFile, imp.Package) {
			item.Decision = "denied"
			item.Diagnostic = "TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION"
			item.Reasons = append(item.Reasons, ruleAllowOnlyReasonAny(g, absFile, imp.Package))
			return item
		}
		if boundary.TransitiveAllowOnlyViolation(transitiveMatches, imp.Package) {
			item.Decision = "denied"
			item.Diagnostic = "TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION"
			item.Reasons = append(item.Reasons, boundary.TransitiveAllowOnlyReason(transitiveMatches, imp.Package))
			return item
		}
		item.Decision = "unknown"
		item.Reasons = append(item.Reasons, "file is not reachable from any target, so target-scoped allowances could not be evaluated")
		return item
	}
	item.Decision = "allowed"
	for _, r := range reach {
		p, t := targetByName(g, r.Target)
		if p == nil || t == nil {
			continue
		}
		td := decideForTarget(p, t, absFile, imp.Package, transitiveMatches)
		item.Targets = append(item.Targets, td)
		item.Reasons = append(item.Reasons, td.Reasons...)
		if td.Decision == "denied" {
			item.Decision = "denied"
			if item.Diagnostic == "" {
				item.Diagnostic = td.Diagnostic
			}
		}
	}
	item.Reasons = dedupeStrings(item.Reasons)
	return item
}

func explainTypeExternal(g *graph.WorkspaceGraph, absFile string, item ExplainImport, pkg string, typeTransitiveMatches []boundary.TransitiveRuleMatch) ExplainImport {
	if ruleDeniesTypeAny(g, absFile, pkg) {
		item.Decision = "denied"
		item.Diagnostic = "TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY"
		item.Reasons = append(item.Reasons, "denied by denyTypeDeps in boundary matching this file")
		return item
	}
	if boundary.DeniedByTransitiveTypeBoundary(typeTransitiveMatches, pkg) {
		item.Decision = "denied"
		item.Diagnostic = "TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY"
		item.Reasons = append(item.Reasons, transitiveTypeDenyReason(typeTransitiveMatches))
		return item
	}
	item.Decision = "allowed"
	item.Reasons = append(item.Reasons, "type import not denied by matching type boundary rules")
	return item
}

func decideForTarget(p *graph.PackageNode, t *graph.TargetNode, absFile string, pkg string, transitiveMatches []boundary.TransitiveRuleMatch) ExplainTargetDecision {
	res := ExplainTargetDecision{Target: t.Name, Decision: "allowed"}
	if boundary.IsTool(p, pkg) {
		res.Decision = "denied"
		res.Diagnostic = "TSPACK_BOUNDARY_TOOL_RUNTIME_IMPORT"
		res.Reasons = append(res.Reasons, "tool dependencies cannot be imported by runtime source")
		return res
	}
	if boundary.DeniedByBoundary(p, absFile, pkg) {
		res.Decision = "denied"
		res.Diagnostic = "TSPACK_BOUNDARY_EXPLICIT_DENY"
		res.Reasons = append(res.Reasons, "denied by boundary rule matching this file")
		return res
	}
	if boundary.DeniedByTransitiveBoundary(transitiveMatches, pkg) {
		res.Decision = "denied"
		res.Diagnostic = "TSPACK_BOUNDARY_EXPLICIT_DENY"
		res.Reasons = append(res.Reasons, transitiveDenyReason(transitiveMatches))
		return res
	}
	if boundary.AllowOnlyViolation(p, absFile, pkg) {
		res.Decision = "denied"
		res.Diagnostic = "TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION"
		res.Reasons = append(res.Reasons, boundary.AllowOnlyReason(p, absFile, pkg))
		return res
	}
	if boundary.TransitiveAllowOnlyViolation(transitiveMatches, pkg) {
		res.Decision = "denied"
		res.Diagnostic = "TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION"
		res.Reasons = append(res.Reasons, boundary.TransitiveAllowOnlyReason(transitiveMatches, pkg))
		return res
	}
	if t.AllowsExternalPackageName(pkg) {
		res.Reasons = append(res.Reasons, "declared dependency for target "+t.Name)
		return res
	}
	res.Decision = "denied"
	if len(p.TargetsAllowingExternalPackageName(pkg)) == 0 {
		res.Diagnostic = "TSPACK_BOUNDARY_UNDECLARED_IMPORT"
		res.Reasons = append(res.Reasons, "package is not declared for any target")
		return res
	}
	res.Diagnostic = "TSPACK_BOUNDARY_PEER_SCOPE_VIOLATION"
	res.Reasons = append(res.Reasons, "target "+t.Name+" does not allow this package")
	return res
}

func transitiveTypeDenyReason(matches []boundary.TransitiveRuleMatch) string {
	if len(matches) == 0 {
		return "denied by transitive type boundary"
	}
	return "denied by denyTypeDeps in transitive boundary from " + matches[0].Rule.TransitiveFrom
}

func transitiveDenyReason(matches []boundary.TransitiveRuleMatch) string {
	if len(matches) == 0 {
		return "denied by transitive boundary"
	}
	return "denied by transitive boundary from " + matches[0].Rule.TransitiveFrom
}

func ruleDeniesTypeAny(g *graph.WorkspaceGraph, absFile string, pkg string) bool {
	for _, p := range g.AllPackages() {
		if boundary.DeniedByTypeBoundary(p, absFile, pkg) {
			return true
		}
	}
	return false
}

func ruleDeniesAny(g *graph.WorkspaceGraph, absFile string, pkg string) bool {
	for _, p := range g.AllPackages() {
		if boundary.DeniedByBoundary(p, absFile, pkg) {
			return true
		}
	}
	return false
}

func ruleAllowOnlyViolationAny(g *graph.WorkspaceGraph, absFile string, pkg string) bool {
	for _, p := range g.AllPackages() {
		if boundary.AllowOnlyViolation(p, absFile, pkg) {
			return true
		}
	}
	return false
}

func ruleAllowOnlyReasonAny(g *graph.WorkspaceGraph, absFile string, pkg string) string {
	for _, p := range g.AllPackages() {
		if boundary.AllowOnlyViolation(p, absFile, pkg) {
			return boundary.AllowOnlyReason(p, absFile, pkg)
		}
	}
	return "not listed in allowOnly boundary"
}

func targetByName(g *graph.WorkspaceGraph, name string) (*graph.PackageNode, *graph.TargetNode) {
	for _, p := range g.AllPackages() {
		if t, ok := p.Target(name); ok {
			return p, t
		}
	}
	return nil, nil
}

func relPathList(root string, paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		out = append(out, relPath(root, path))
	}
	return out
}

func relPath(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func copyStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string{}, values...)
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range in {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func explainDiag(code string, message string, file string) CheckJSONDiagnosticLike {
	return CheckJSONDiagnosticLike{Code: code, Severity: string(diag.SeverityError), Message: message, File: file}
}

func explainDiagnostic(d diag.Diagnostic) CheckJSONDiagnosticLike {
	return CheckJSONDiagnosticLike{
		Code:     d.Code,
		Severity: string(d.Severity),
		Message:  d.Message,
		File:     d.File,
		Details:  append([]string(nil), d.Details...),
	}
}
