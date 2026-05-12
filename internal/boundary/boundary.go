package boundary

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/graph"
	"github.com/tspack/tspack/internal/importscan"
)

type Options struct {
	RootDir string
	Graph   *graph.WorkspaceGraph
}

func Check(opts Options) []diag.Diagnostic {
	if opts.Graph == nil {
		return []diag.Diagnostic{{Code: "TSPACK_BOUNDARY_INTERNAL_ERROR", Severity: diag.SeverityError, Message: "nil graph"}}
	}
	var out []diag.Diagnostic
	for _, p := range opts.Graph.AllPackages() {
		for _, t := range p.AllTargets() {
			out = append(out, checkTarget(opts.RootDir, p, t)...)
		}
	}
	diag.SortDiagnostics(out)
	return out
}

func checkTarget(root string, p *graph.PackageNode, t *graph.TargetNode) []diag.Diagnostic {
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
		if seen[cur] { continue }
		seen[cur] = true
		imps, diags := importscan.ScanFile(cur)
		out = append(out, diags...)
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
				if _, ok := parents[next]; !ok { parents[next] = cur }
				q = append(q, next)
			case importscan.SpecifierExternalPackage:
				out = append(out, validateExternal(t, p, cur, imp.Package, buildPath(parents, entry, cur, imp.Package), imp.Specifier)...)
			case importscan.SpecifierNodeBuiltin:
				// M4 policy: classify and ignore builtin checks.
			}
		}
	}
	return out
}

func validateExternal(t *graph.TargetNode, p *graph.PackageNode, file, pkg string, path []string, spec string) []diag.Diagnostic {
	var out []diag.Diagnostic
	detail := []string{"target=" + t.Name, "package=" + pkg, "import=" + spec, "path=" + strings.Join(path, " -> ")}
	if isTool(p, pkg) {
		out = append(out, diag.Diagnostic{Code: "TSPACK_BOUNDARY_TOOL_RUNTIME_IMPORT", Severity: diag.SeverityError, Message: "tool dependency imported at runtime", File: file, Details: detail})
		return out
	}
	if deniedByBoundary(p, file, pkg) {
		out = append(out, diag.Diagnostic{Code: "TSPACK_BOUNDARY_EXPLICIT_DENY", Severity: diag.SeverityError, Message: "import denied by explicit boundary", File: file, Details: detail})
	}
	if allowViolation(p, file, pkg, t.AllowsExternalPackageName(pkg)) {
		out = append(out, diag.Diagnostic{Code: "TSPACK_BOUNDARY_EXPLICIT_ALLOW_VIOLATION", Severity: diag.SeverityError, Message: "import not present in explicit allow list", File: file, Details: detail})
	}
	if t.AllowsExternalPackageName(pkg) {
		for _, d := range t.RuntimeDeps {
			if d.Source.Package == pkg && d.Kind == graph.DependencyKindType {
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
			if d.Source.Package == pkg {
				out = append(out, diag.Diagnostic{Code: "TSPACK_BOUNDARY_OPTIONAL_PEER_LEAK", Severity: diag.SeverityError, Message: "optional peer leaked across target boundary", File: file, Details: detail})
				return out
			}
		}
	}
	out = append(out, diag.Diagnostic{Code: "TSPACK_BOUNDARY_PEER_SCOPE_VIOLATION", Severity: diag.SeverityError, Message: "target does not allow external package", File: file, Details: detail})
	return out
}

func buildPath(parents map[string]string, entry, file, pkg string) []string {
	p := []string{pkg}
	cur := file
	for {
		p = append([]string{cur}, p...)
		if cur == entry { break }
		n, ok := parents[cur]; if !ok { break }
		cur = n
	}
	return p
}

func matchFrom(pattern, file string) bool {
	pattern = filepath.Clean(pattern)
	file = filepath.ToSlash(file)
	p := filepath.ToSlash(pattern)
	if strings.HasSuffix(p, "/**") {
		pref := strings.TrimSuffix(p, "/**")
		return strings.Contains(file, pref)
	}
	return file == p || strings.HasSuffix(file, "/"+p)
}
func deniedByBoundary(p *graph.PackageNode, file, pkg string) bool {
	for _, b := range p.Boundaries {
		if !matchFrom(b.From, filepath.ToSlash(file)) { continue }
		for _, d := range b.DenyDeps { if d == pkg { return true } }
	}
	return false
}
func allowViolation(p *graph.PackageNode, file, pkg string, targetAllows bool) bool {
	for _, b := range p.Boundaries {
		if !matchFrom(b.From, filepath.ToSlash(file)) || len(b.AllowDeps)==0 { continue }
		for _, a := range b.AllowDeps { if a == pkg { return false } }
		if targetAllows { return true }
	}
	return false
}
func isTool(p *graph.PackageNode, pkg string) bool {
	for _, d := range p.ToolDependencies() { if d.Source.Package == pkg { return true } }
	return false
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
