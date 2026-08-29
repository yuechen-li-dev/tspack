package typesurface

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/importscan"
)

type CheckOptions struct {
	RootDir string
	Graph   *graph.WorkspaceGraph
}
type CheckResult struct{ Diagnostics []diag.Diagnostic }
type policy struct{ decl, missing, leak string }

func CheckTypeSurfaces(opts CheckOptions) CheckResult { /* ... */
	if opts.Graph == nil {
		return CheckResult{Diagnostics: []diag.Diagnostic{{Code: "TSPACK_TYPE_INTERNAL_ERROR", Severity: diag.SeverityError, Message: "nil graph"}}}
	}
	var out []diag.Diagnostic
	for _, p := range opts.Graph.AllPackages() {
		pol := readPolicy(p)
		for _, t := range p.AllTargets() {
			out = append(out, checkTarget(opts.RootDir, p, t, pol)...)
		}
	}
	diag.SortDiagnostics(out)
	return CheckResult{Diagnostics: out}
}
func readPolicy(p *graph.PackageNode) policy {
	pol := policy{decl: "optional", missing: "error", leak: "error"}
	if p.Kind == "library" {
		pol.decl = "required"
	}
	if v := p.Policies.Types["declarations"]; v != "" {
		pol.decl = v
	}
	if v := p.Policies.Types["missingTypes"]; v != "" {
		pol.missing = v
	}
	if v := p.Policies.Types["publicTypeLeakage"]; v != "" {
		pol.leak = v
	}
	return pol
}
func checkTarget(root string, p *graph.PackageNode, t *graph.TargetNode, pol policy) []diag.Diagnostic {
	var out []diag.Diagnostic
	types := strings.TrimSpace(t.Types)
	if types == "" && pol.decl == "required" && pol.missing != "ignore" {
		return []diag.Diagnostic{mk("TSPACK_TYPE_MISSING_OUTPUT", pol.missing, "target missing declarations output", t.Entry, t, "")}
	}
	if types == "" {
		return nil
	}
	if filepath.IsAbs(types) || strings.Contains(types, "..") {
		if pol.missing != "ignore" {
			out = append(out, mk("TSPACK_TYPE_INVALID_OUTPUT_PATH", pol.missing, "invalid type output path", types, t, ""))
		}
		return out
	}
	pkgRoot := p.Root
	if pkgRoot == "" {
		pkgRoot = "."
	}
	entry := filepath.Clean(filepath.Join(root, pkgRoot, types))
	if _, err := os.Stat(entry); err != nil {
		if os.IsNotExist(err) {
			if pol.missing != "ignore" {
				out = append(out, mk("TSPACK_TYPE_MISSING_OUTPUT", pol.missing, "declared type output missing", entry, t, ""))
			}
		} else if pol.missing != "ignore" {
			out = append(out, mk("TSPACK_TYPE_UNREADABLE_OUTPUT", pol.missing, "declared type output unreadable", entry, t, err.Error()))
		}
		return out
	}
	if pol.leak == "ignore" {
		return out
	}
	return append(out, walkTypes(p, t, entry, pol.leak)...)
}
func walkTypes(p *graph.PackageNode, t *graph.TargetNode, entry, severity string) []diag.Diagnostic {
	var out []diag.Diagnostic
	q := []string{entry}
	seen := map[string]bool{}
	parent := map[string]string{}
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		imps, diags := importscan.ScanFile(cur)
		out = append(out, diags...)
		for _, imp := range imps {
			if imp.SpecifierKind == importscan.SpecifierNodeBuiltin || imp.Kind == importscan.ImportKindUnknownDynamic {
				continue
			}
			if imp.SpecifierKind == importscan.SpecifierRelativeInternal {
				next, ok := resolveDTSRelative(cur, imp.Specifier)
				if !ok {
					out = append(out, mkWithPath("TSPACK_TYPE_UNRESOLVED_RELATIVE", severity, "unresolved relative type reference", cur, t, imp.Specifier, buildPath(parent, entry, cur, imp.Specifier)))
					continue
				}
				if _, ok := parent[next]; !ok {
					parent[next] = cur
				}
				q = append(q, next)
				continue
			}
			if imp.SpecifierKind == importscan.SpecifierExternalPackage {
				out = append(out, validateExternal(p, t, cur, imp.Package, severity, buildPath(parent, entry, cur, imp.Package))...)
			}
		}
	}
	return out
}
func validateExternal(p *graph.PackageNode, t *graph.TargetNode, file, pkg, severity string, path []string) []diag.Diagnostic {
	if isTool(p, pkg) {
		return []diag.Diagnostic{mkWithPath("TSPACK_TYPE_TOOL_REFERENCE", severity, "tool dependency in public types", file, t, pkg, path)}
	}
	if t.AllowsExternalPackageName(pkg) {
		return nil
	}
	others := p.TargetsAllowingExternalPackageName(pkg)
	if len(others) == 0 {
		return []diag.Diagnostic{mkWithPath("TSPACK_TYPE_UNDECLARED_REFERENCE", severity, "undeclared external type reference", file, t, pkg, path)}
	}
	for _, o := range others {
		for _, d := range o.OptionalPeerDeps {
			if d.Source.Package == pkg {
				return []diag.Diagnostic{mkWithPath("TSPACK_TYPE_OPTIONAL_PEER_LEAK", severity, "optional peer leaked in public types", file, t, pkg, path)}
			}
		}
	}
	return []diag.Diagnostic{mkWithPath("TSPACK_TYPE_PEER_SCOPE_VIOLATION", severity, "target type surface does not allow external package", file, t, pkg, path)}
}
func resolveDTSRelative(base, spec string) (string, bool) {
	cand := filepath.Clean(filepath.Join(filepath.Dir(base), spec))
	tries := []string{}
	ext := filepath.Ext(cand)
	if ext == ".d.ts" || ext == ".d.mts" || ext == ".d.cts" {
		tries = append(tries, cand)
	} else {
		if ext == ".js" || ext == ".mjs" || ext == ".cjs" {
			withoutRuntimeExtension := strings.TrimSuffix(cand, ext)
			tries = append(tries, withoutRuntimeExtension+".d.ts", withoutRuntimeExtension+".d.mts", withoutRuntimeExtension+".d.cts")
		}
		tries = append(tries, cand+".d.ts", cand+".d.mts", cand+".d.cts", filepath.Join(cand, "index.d.ts"), filepath.Join(cand, "index.d.mts"), filepath.Join(cand, "index.d.cts"))
	}
	for _, p := range tries {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, true
		}
	}
	return "", false
}
func buildPath(parents map[string]string, entry, file, last string) []string {
	p := []string{last}
	for cur := file; ; {
		p = append([]string{filepath.ToSlash(cur)}, p...)
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
func mk(code, severity, msg, file string, t *graph.TargetNode, extra string) diag.Diagnostic {
	return mkWithPath(code, severity, msg, file, t, extra, nil)
}
func mkWithPath(code, severity, msg, file string, t *graph.TargetNode, pkg string, path []string) diag.Diagnostic {
	s := diag.SeverityError
	if severity == "warn" {
		s = diag.SeverityWarning
	}
	d := []string{"package=" + t.Package.Name, "target=" + t.Name}
	if pkg != "" {
		d = append(d, "import="+pkg)
	}
	if len(path) > 0 {
		d = append(d, "path="+strings.Join(path, " -> "))
	}
	return diag.Diagnostic{Code: code, Severity: s, Message: msg, File: file, Details: d}
}
func isTool(p *graph.PackageNode, pkg string) bool {
	for _, d := range p.ToolDependencies() {
		if d.Source.Package == pkg {
			return true
		}
	}
	return false
}
func PathFromDetails(d diag.Diagnostic) []string {
	for _, x := range d.Details {
		if strings.HasPrefix(x, "path=") {
			return strings.Split(strings.TrimPrefix(x, "path="), " -> ")
		}
	}
	return nil
}
func SortStrings(xs []string) { sort.Strings(xs) }
