package boundary

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/importscan"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

type Options struct {
	RootDir string
	Graph   *graph.WorkspaceGraph
}

type TransitiveRuleMatch struct {
	Rule manifest.BoundaryRule
	Seed string
	Path []string
}

func Check(opts Options) []diag.Diagnostic {
	if opts.Graph == nil {
		return []diag.Diagnostic{{Code: "TSPACK_BOUNDARY_INTERNAL_ERROR", Severity: diag.SeverityError, Message: "nil graph"}}
	}
	var out []diag.Diagnostic
	for _, p := range opts.Graph.AllPackages() {
		transitiveMatches := BuildTransitiveRuleMatches(opts.RootDir, p)
		typeTransitiveMatches := BuildTransitiveTypeRuleMatches(opts.RootDir, p)
		for _, t := range p.AllTargets() {
			out = append(out, checkTarget(opts.RootDir, p, t, transitiveMatches)...)
			out = append(out, checkTargetTypeBoundaries(opts.RootDir, p, t, typeTransitiveMatches)...)
		}
	}
	diag.SortDiagnostics(out)
	return out
}

func checkTarget(root string, p *graph.PackageNode, t *graph.TargetNode, transitiveMatches map[string][]TransitiveRuleMatch) []diag.Diagnostic {
	pkgRoot := p.Root
	if pkgRoot == "" {
		pkgRoot = "."
	}
	entry := filepath.Clean(filepath.Join(root, pkgRoot, t.Entry))
	q := []string{entry}
	parents := map[string]string{}
	seen := map[string]bool{}
	var out []diag.Diagnostic
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		imps, diags := importscan.ScanFile(cur)
		out = append(out, diags...)
		importscan.SortImports(imps)
		for _, imp := range imps {
			if imp.Kind != importscan.ImportKindRuntime {
				continue
			}
			switch imp.SpecifierKind {
			case importscan.SpecifierRelativeInternal:
				next, ok := importscan.ResolveRelative(cur, imp.Specifier)
				if !ok {
					out = append(out, diag.Diagnostic{Code: "TSPACK_IMPORT_UNRESOLVED_RELATIVE", Severity: diag.SeverityError, Message: "relative import could not be resolved", File: cur, Details: []string{t.Name, imp.Specifier}})
					continue
				}
				if _, ok := parents[next]; !ok {
					parents[next] = cur
				}
				q = append(q, next)
			case importscan.SpecifierExternalPackage:
				matches := transitiveMatches[filepath.Clean(cur)]
				out = append(out, validateExternal(root, t, p, cur, imp.Package, buildPath(parents, entry, cur, imp.Package), imp.Specifier, matches)...)
			case importscan.SpecifierNodeBuiltin:
				// M4 policy: classify and ignore builtin checks.
			}
		}
	}
	return out
}

func checkTargetTypeBoundaries(root string, p *graph.PackageNode, t *graph.TargetNode, transitiveMatches map[string][]TransitiveRuleMatch) []diag.Diagnostic {
	pkgRoot := p.Root
	if pkgRoot == "" {
		pkgRoot = "."
	}
	entry := filepath.Clean(filepath.Join(root, pkgRoot, t.Entry))
	q := []string{entry}
	parents := map[string]string{}
	seen := map[string]bool{}
	var out []diag.Diagnostic
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		imps, _ := importscan.ScanFile(cur)
		importscan.SortImports(imps)
		for _, imp := range imps {
			switch imp.SpecifierKind {
			case importscan.SpecifierRelativeInternal:
				if imp.Kind != importscan.ImportKindRuntime && imp.Kind != importscan.ImportKindTypeOnly {
					continue
				}
				next, ok := importscan.ResolveRelative(cur, imp.Specifier)
				if !ok {
					continue
				}
				if _, ok := parents[next]; !ok {
					parents[next] = cur
				}
				q = append(q, next)
			case importscan.SpecifierExternalPackage:
				if imp.Kind != importscan.ImportKindTypeOnly {
					continue
				}
				matches := transitiveMatches[filepath.Clean(cur)]
				out = append(out, validateTypeExternal(root, t, p, cur, imp.Package, buildPath(parents, entry, cur, imp.Package), imp.Specifier, matches)...)
			}
		}
	}
	return out
}

func validateExternal(root string, t *graph.TargetNode, p *graph.PackageNode, file, pkg string, path []string, spec string, transitiveMatches []TransitiveRuleMatch) []diag.Diagnostic {
	var out []diag.Diagnostic
	detail := []string{"target=" + t.Name, "package=" + pkg, "import=" + spec, "path=" + strings.Join(path, " -> ")}
	if IsTool(p, pkg) {
		out = append(out, diag.Diagnostic{Code: "TSPACK_BOUNDARY_TOOL_RUNTIME_IMPORT", Severity: diag.SeverityError, Message: "tool dependency imported at runtime", File: file, Details: detail})
		return out
	}
	deniedByExplicitRule := false
	if DeniedByBoundary(p, file, pkg) {
		deniedByExplicitRule = true
		out = append(out, diag.Diagnostic{Code: "TSPACK_BOUNDARY_EXPLICIT_DENY", Severity: diag.SeverityError, Message: "import denied by explicit boundary", File: file, Details: detail})
	}
	for _, match := range transitiveMatches {
		if !ruleDeniesPackage(match.Rule, pkg) {
			continue
		}
		deniedByExplicitRule = true
		transitivePath := relPathList(root, match.Path)
		transitivePath = append(transitivePath, pkg)
		transitiveDetail := []string{
			"target=" + t.Name,
			"package=" + pkg,
			"import=" + spec,
			"boundary=transitiveFrom " + match.Rule.TransitiveFrom,
			"transitiveFrom=" + match.Rule.TransitiveFrom,
			"seed=" + relPath(root, match.Seed),
			"path=" + strings.Join(transitivePath, " -> "),
		}
		out = append(out, diag.Diagnostic{Code: "TSPACK_BOUNDARY_EXPLICIT_DENY", Severity: diag.SeverityError, Message: "import denied by explicit transitive boundary", File: file, Details: transitiveDetail})
	}
	if !deniedByExplicitRule {
		out = append(out, allowOnlyViolations(root, t, p, file, pkg, path, spec, transitiveMatches)...)
	}
	if AllowViolation(p, file, pkg, t.AllowsExternalPackageName(pkg)) || transitiveAllowViolation(transitiveMatches, pkg, t.AllowsExternalPackageName(pkg)) {
		out = append(out, diag.Diagnostic{Code: "TSPACK_BOUNDARY_EXPLICIT_ALLOW_VIOLATION", Severity: diag.SeverityError, Message: "import not present in explicit allow list", File: file, Details: detail})
	}
	if t.AllowsExternalPackageName(pkg) {
		for _, d := range t.RuntimeDeps {
			if d.MatchesExternalPackageName(pkg) && d.Kind == graph.DependencyKindType {
				out = append(out, diag.Diagnostic{Code: "TSPACK_BOUNDARY_TYPE_ONLY_RUNTIME_IMPORT", Severity: diag.SeverityError, Message: "type-only dependency imported at runtime", File: file, Details: detail})
				return out
			}
		}
		return out
	}
	others := p.TargetsAllowingExternalPackageName(pkg)
	if len(others) == 0 {
		out = append(out, diag.Diagnostic{Code: "TSPACK_BOUNDARY_UNDECLARED_IMPORT", Severity: diag.SeverityError, Message: "undeclared runtime import", File: file, Details: detail})
		return out
	}
	for _, o := range others {
		for _, d := range o.OptionalPeerDeps {
			if d.MatchesExternalPackageName(pkg) {
				out = append(out, diag.Diagnostic{Code: "TSPACK_BOUNDARY_OPTIONAL_PEER_LEAK", Severity: diag.SeverityError, Message: "optional peer leaked across target boundary", File: file, Details: detail})
				return out
			}
		}
	}
	out = append(out, diag.Diagnostic{Code: "TSPACK_BOUNDARY_PEER_SCOPE_VIOLATION", Severity: diag.SeverityError, Message: "target does not allow external package", File: file, Details: detail})
	return out
}

func validateTypeExternal(root string, t *graph.TargetNode, p *graph.PackageNode, file, pkg string, path []string, spec string, transitiveMatches []TransitiveRuleMatch) []diag.Diagnostic {
	var out []diag.Diagnostic
	for _, rule := range p.Boundaries {
		if rule.From == "" || !MatchFrom(rule.From, filepath.ToSlash(file)) {
			continue
		}
		if !ruleDeniesTypePackage(rule, pkg) {
			continue
		}
		detail := []string{
			"target=" + t.Name,
			"package=" + pkg,
			"import=" + spec,
			"boundary=from " + rule.From,
			"from=" + rule.From,
			"denyTypeDeps=" + strings.Join(dedupeStrings(rule.DenyTypeDeps), ","),
			"path=" + strings.Join(path, " -> "),
		}
		out = append(out, diag.Diagnostic{Code: "TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY", Severity: diag.SeverityError, Message: "type import denied by explicit boundary", File: file, Details: detail})
	}
	for _, match := range transitiveMatches {
		if !ruleDeniesTypePackage(match.Rule, pkg) {
			continue
		}
		transitivePath := relPathList(root, match.Path)
		transitivePath = append(transitivePath, pkg)
		detail := []string{
			"target=" + t.Name,
			"package=" + pkg,
			"import=" + spec,
			"boundary=transitiveFrom " + match.Rule.TransitiveFrom,
			"transitiveFrom=" + match.Rule.TransitiveFrom,
			"seed=" + relPath(root, match.Seed),
			"denyTypeDeps=" + strings.Join(dedupeStrings(match.Rule.DenyTypeDeps), ","),
			"path=" + strings.Join(transitivePath, " -> "),
		}
		out = append(out, diag.Diagnostic{Code: "TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY", Severity: diag.SeverityError, Message: "type import denied by explicit transitive boundary", File: file, Details: detail})
	}
	return out
}

func allowOnlyViolations(root string, t *graph.TargetNode, p *graph.PackageNode, file, pkg string, path []string, spec string, transitiveMatches []TransitiveRuleMatch) []diag.Diagnostic {
	out := []diag.Diagnostic{}
	for _, rule := range p.Boundaries {
		if rule.From == "" || !ruleHasAllowOnly(rule) || !MatchFrom(rule.From, filepath.ToSlash(file)) {
			continue
		}
		if ruleAllowsOnlyPackage(rule, pkg) {
			continue
		}
		detail := []string{
			"target=" + t.Name,
			"package=" + pkg,
			"import=" + spec,
			"boundary=from " + rule.From,
			"from=" + rule.From,
			"allowOnly=" + strings.Join(dedupeStrings(rule.AllowOnly), ","),
			"path=" + strings.Join(path, " -> "),
		}
		out = append(out, diag.Diagnostic{Code: "TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION", Severity: diag.SeverityError, Message: "import not listed in allowOnly boundary", File: file, Details: detail})
	}
	for _, match := range transitiveMatches {
		if !ruleHasAllowOnly(match.Rule) || ruleAllowsOnlyPackage(match.Rule, pkg) {
			continue
		}
		transitivePath := relPathList(root, match.Path)
		transitivePath = append(transitivePath, pkg)
		detail := []string{
			"target=" + t.Name,
			"package=" + pkg,
			"import=" + spec,
			"boundary=transitiveFrom " + match.Rule.TransitiveFrom,
			"transitiveFrom=" + match.Rule.TransitiveFrom,
			"seed=" + relPath(root, match.Seed),
			"allowOnly=" + strings.Join(dedupeStrings(match.Rule.AllowOnly), ","),
			"path=" + strings.Join(transitivePath, " -> "),
		}
		out = append(out, diag.Diagnostic{Code: "TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION", Severity: diag.SeverityError, Message: "import not listed in transitive allowOnly boundary", File: file, Details: detail})
	}
	return out
}

func AllowOnlyViolation(p *graph.PackageNode, file, pkg string) bool {
	for _, rule := range p.Boundaries {
		if rule.From == "" || !ruleHasAllowOnly(rule) || !MatchFrom(rule.From, filepath.ToSlash(file)) {
			continue
		}
		if !ruleAllowsOnlyPackage(rule, pkg) {
			return true
		}
	}
	return false
}

func TransitiveAllowOnlyViolation(matches []TransitiveRuleMatch, pkg string) bool {
	for _, match := range matches {
		if !ruleHasAllowOnly(match.Rule) {
			continue
		}
		if !ruleAllowsOnlyPackage(match.Rule, pkg) {
			return true
		}
	}
	return false
}

func AllowOnlyReason(p *graph.PackageNode, file, pkg string) string {
	for _, rule := range p.Boundaries {
		if rule.From == "" || !ruleHasAllowOnly(rule) || !MatchFrom(rule.From, filepath.ToSlash(file)) {
			continue
		}
		if !ruleAllowsOnlyPackage(rule, pkg) {
			return "not listed in allowOnly for boundary from " + rule.From
		}
	}
	return "not listed in allowOnly boundary"
}

func TransitiveAllowOnlyReason(matches []TransitiveRuleMatch, pkg string) string {
	for _, match := range matches {
		if !ruleHasAllowOnly(match.Rule) || ruleAllowsOnlyPackage(match.Rule, pkg) {
			continue
		}
		return "not listed in allowOnly for transitive boundary from " + match.Rule.TransitiveFrom
	}
	return "not listed in transitive allowOnly boundary"
}

func ruleHasAllowOnly(rule manifest.BoundaryRule) bool {
	return rule.AllowOnlySpecified || rule.AllowOnly != nil
}

func ruleAllowsOnlyPackage(rule manifest.BoundaryRule, pkg string) bool {
	for _, allowed := range rule.AllowOnly {
		if allowed == pkg {
			return true
		}
	}
	return false
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
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

func buildPath(parents map[string]string, entry, file, pkg string) []string {
	p := []string{pkg}
	cur := file
	for {
		p = append([]string{cur}, p...)
		if cur == entry {
			break
		}
		n, ok := parents[cur]
		if !ok {
			break
		}
		cur = n
	}
	return p
}

func BuildTransitiveRuleMatches(root string, p *graph.PackageNode) map[string][]TransitiveRuleMatch {
	return buildTransitiveRuleMatches(root, p, false)
}

func BuildTransitiveTypeRuleMatches(root string, p *graph.PackageNode) map[string][]TransitiveRuleMatch {
	return buildTransitiveRuleMatches(root, p, true)
}

func buildTransitiveRuleMatches(root string, p *graph.PackageNode, includeTypeOnlyLocalImports bool) map[string][]TransitiveRuleMatch {
	matches := map[string][]TransitiveRuleMatch{}
	hasTransitiveRule := false
	for _, rule := range p.Boundaries {
		if rule.TransitiveFrom != "" {
			hasTransitiveRule = true
			break
		}
	}
	if !hasTransitiveRule {
		return matches
	}

	sourceFiles := packageSourceFiles(root, p)
	for _, rule := range p.Boundaries {
		if rule.TransitiveFrom == "" {
			continue
		}
		seeds := matchingSeeds(root, rule.TransitiveFrom, sourceFiles)
		for _, seed := range seeds {
			paths := reachablePathsFromSeed(seed, includeTypeOnlyLocalImports)
			files := sortedMapKeys(paths)
			for _, file := range files {
				matches[file] = append(matches[file], TransitiveRuleMatch{Rule: rule, Seed: seed, Path: paths[file]})
			}
		}
	}
	for file := range matches {
		sort.SliceStable(matches[file], func(i, j int) bool {
			left := matches[file][i]
			right := matches[file][j]
			if left.Rule.TransitiveFrom != right.Rule.TransitiveFrom {
				return left.Rule.TransitiveFrom < right.Rule.TransitiveFrom
			}
			return left.Seed < right.Seed
		})
	}
	return matches
}

func packageSourceFiles(root string, p *graph.PackageNode) []string {
	pkgRoot := p.Root
	if pkgRoot == "" {
		pkgRoot = "."
	}
	absRoot := filepath.Clean(filepath.Join(root, pkgRoot))
	out := []string{}
	_ = filepath.WalkDir(absRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == "node_modules" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if isSourceFile(path) {
			out = append(out, filepath.Clean(path))
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func isSourceFile(path string) bool {
	switch filepath.Ext(path) {
	case ".ts", ".tsx", ".js", ".jsx", ".mts", ".cts":
		return true
	default:
		return false
	}
}

func matchingSeeds(root string, pattern string, sourceFiles []string) []string {
	out := []string{}
	for _, file := range sourceFiles {
		rel, err := filepath.Rel(root, file)
		if err != nil {
			continue
		}
		if MatchFrom(pattern, filepath.ToSlash(rel)) {
			out = append(out, file)
		}
	}
	sort.Strings(out)
	return out
}

func reachablePathsFromSeed(seed string, includeTypeOnlyLocalImports bool) map[string][]string {
	q := []string{seed}
	parents := map[string]string{}
	seen := map[string]bool{}
	paths := map[string][]string{}
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		paths[cur] = buildFilePath(parents, seed, cur)
		imports, _ := importscan.ScanFile(cur)
		importscan.SortImports(imports)
		for _, imp := range imports {
			if imp.SpecifierKind != importscan.SpecifierRelativeInternal {
				continue
			}
			if imp.Kind != importscan.ImportKindRuntime && (!includeTypeOnlyLocalImports || imp.Kind != importscan.ImportKindTypeOnly) {
				continue
			}
			next, ok := importscan.ResolveRelative(cur, imp.Specifier)
			if !ok {
				continue
			}
			if _, exists := parents[next]; !exists && next != seed {
				parents[next] = cur
			}
			q = append(q, next)
		}
	}
	return paths
}

func sortedMapKeys(values map[string][]string) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func MatchFrom(pattern, file string) bool {
	pattern = filepath.Clean(pattern)
	file = filepath.ToSlash(file)
	p := filepath.ToSlash(pattern)
	if strings.HasSuffix(p, "/**") {
		pref := strings.TrimSuffix(p, "/**")
		return file == pref || strings.HasPrefix(file, pref+"/") || strings.Contains(file, "/"+pref+"/")
	}
	return file == p || strings.HasSuffix(file, "/"+p)
}

func DeniedByBoundary(p *graph.PackageNode, file, pkg string) bool {
	for _, b := range p.Boundaries {
		if b.From == "" || !MatchFrom(b.From, filepath.ToSlash(file)) {
			continue
		}
		if ruleDeniesPackage(b, pkg) {
			return true
		}
	}
	return false
}

func DeniedByTransitiveBoundary(matches []TransitiveRuleMatch, pkg string) bool {
	for _, match := range matches {
		if ruleDeniesPackage(match.Rule, pkg) {
			return true
		}
	}
	return false
}

func DeniedByTypeBoundary(p *graph.PackageNode, file, pkg string) bool {
	for _, b := range p.Boundaries {
		if b.From == "" || !MatchFrom(b.From, filepath.ToSlash(file)) {
			continue
		}
		if ruleDeniesTypePackage(b, pkg) {
			return true
		}
	}
	return false
}

func DeniedByTransitiveTypeBoundary(matches []TransitiveRuleMatch, pkg string) bool {
	for _, match := range matches {
		if ruleDeniesTypePackage(match.Rule, pkg) {
			return true
		}
	}
	return false
}

func ruleDeniesPackage(rule manifest.BoundaryRule, pkg string) bool {
	for _, d := range rule.DenyDeps {
		if d == pkg {
			return true
		}
	}
	return false
}

func ruleDeniesTypePackage(rule manifest.BoundaryRule, pkg string) bool {
	for _, d := range rule.DenyTypeDeps {
		if d == pkg {
			return true
		}
	}
	return false
}

func AllowViolation(p *graph.PackageNode, file, pkg string, targetAllows bool) bool {
	for _, b := range p.Boundaries {
		if b.From == "" || !MatchFrom(b.From, filepath.ToSlash(file)) || len(b.AllowDeps) == 0 {
			continue
		}
		if ruleAllowsPackage(b, pkg) {
			return false
		}
		if targetAllows {
			return true
		}
	}
	return false
}

func transitiveAllowViolation(matches []TransitiveRuleMatch, pkg string, targetAllows bool) bool {
	for _, match := range matches {
		if len(match.Rule.AllowDeps) == 0 {
			continue
		}
		if ruleAllowsPackage(match.Rule, pkg) {
			return false
		}
		if targetAllows {
			return true
		}
	}
	return false
}

func ruleAllowsPackage(rule manifest.BoundaryRule, pkg string) bool {
	for _, a := range rule.AllowDeps {
		if a == pkg {
			return true
		}
	}
	return false
}

func IsTool(p *graph.PackageNode, pkg string) bool {
	for _, d := range p.ToolDependencies() {
		if d.MatchesExternalPackageName(pkg) {
			return true
		}
	}
	return false
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

func PathFromDetails(d diag.Diagnostic) []string { // tests helper
	for _, x := range d.Details {
		if strings.HasPrefix(x, "path=") {
			return strings.Split(strings.TrimPrefix(x, "path="), " -> ")
		}
	}
	return nil
}

func SortStrings(xs []string) { sort.Strings(xs) }
