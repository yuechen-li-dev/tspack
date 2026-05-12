package importscan

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tspack/tspack/internal/diag"
)

type ImportKind string

type SpecifierKind string

const (
	ImportKindRuntime        ImportKind = "runtime"
	ImportKindTypeOnly       ImportKind = "type-only"
	ImportKindUnknownDynamic ImportKind = "unknown-dynamic"

	SpecifierRelativeInternal SpecifierKind = "relative-internal"
	SpecifierExternalPackage  SpecifierKind = "external-package"
	SpecifierNodeBuiltin      SpecifierKind = "node-builtin"
	SpecifierUnknown          SpecifierKind = "unknown"
)

type Import struct {
	Specifier     string
	Package       string
	Kind          ImportKind
	SpecifierKind SpecifierKind
}

var (
	reImportFrom      = regexp.MustCompile(`(?m)\bimport\s+(type\s+)?[^;\n]*?\sfrom\s*["']([^"']+)["']`)
	reImportSide      = regexp.MustCompile(`(?m)\bimport\s*["']([^"']+)["']`)
	reExportFrom      = regexp.MustCompile(`(?m)\bexport\s+(type\s+)?(?:\*|\{[^}]*\})\s*from\s*["']([^"']+)["']`)
	reRequireLiteral  = regexp.MustCompile(`(?m)\brequire\s*\(\s*["']([^"']+)["']\s*\)`)
	reRequireAny      = regexp.MustCompile(`(?m)\brequire\s*\(`)
	reImportLiteral   = regexp.MustCompile(`(?m)\bimport\s*\(\s*["']([^"']+)["']\s*\)`)
	reImportDynamicAny = regexp.MustCompile(`(?m)\bimport\s*\(`)
	reImportDynamicNonLiteral = regexp.MustCompile(`(?m)\bimport\s*\(\s*[^\s"'\)]`)
)

func ScanFile(path string) ([]Import, []diag.Diagnostic) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, []diag.Diagnostic{{Code: "TSPACK_IMPORT_PARSE_ERROR", Severity: diag.SeverityError, Message: "failed to read source file", File: path, Details: []string{err.Error()}}}
	}
	s := string(b)
	out := []Import{}
	diags := []diag.Diagnostic{}
	seen := map[string]bool{}
	add := func(spec string, kind ImportKind) {
		k, pkg := classifySpecifier(spec)
		key := string(kind) + "|" + spec
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, Import{Specifier: spec, Kind: kind, SpecifierKind: k, Package: pkg})
	}
	for _, m := range reImportFrom.FindAllStringSubmatch(s, -1) {
		kind := ImportKindRuntime
		if strings.TrimSpace(m[1]) != "" {
			kind = ImportKindTypeOnly
		}
		add(m[2], kind)
	}
	for _, m := range reImportSide.FindAllStringSubmatch(s, -1) {
		add(m[1], ImportKindRuntime)
	}
	for _, m := range reExportFrom.FindAllStringSubmatch(s, -1) {
		kind := ImportKindRuntime
		if strings.TrimSpace(m[1]) != "" {
			kind = ImportKindTypeOnly
		}
		add(m[2], kind)
	}
	for _, m := range reRequireLiteral.FindAllStringSubmatch(s, -1) {
		add(m[1], ImportKindRuntime)
	}
	for _, m := range reImportLiteral.FindAllStringSubmatch(s, -1) {
		add(m[1], ImportKindRuntime)
	}
	if regexp.MustCompile(`(?m)\brequire\s*\(\s*[^\s"'\)]`).MatchString(s) {
		diags = append(diags, diag.Diagnostic{Code: "TSPACK_IMPORT_UNSUPPORTED_DYNAMIC", Severity: diag.SeverityWarning, Message: "non-literal require is unsupported", File: path})
		out = append(out, Import{Kind: ImportKindUnknownDynamic, SpecifierKind: SpecifierUnknown})
	}
	if reImportDynamicNonLiteral.MatchString(s) {
		diags = append(diags, diag.Diagnostic{Code: "TSPACK_IMPORT_UNSUPPORTED_DYNAMIC", Severity: diag.SeverityWarning, Message: "non-literal dynamic import is unsupported", File: path})
		out = append(out, Import{Kind: ImportKindUnknownDynamic, SpecifierKind: SpecifierUnknown})
	}
	return out, diags
}

func classifySpecifier(spec string) (SpecifierKind, string) {
	if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") {
		return SpecifierRelativeInternal, ""
	}
	if strings.HasPrefix(spec, "#") || spec == "" {
		return SpecifierUnknown, ""
	}
	if strings.HasPrefix(spec, "node:") {
		return SpecifierNodeBuiltin, spec
	}
	if builtins[spec] {
		return SpecifierNodeBuiltin, spec
	}
	if pkg, ok := ExternalPackageName(spec); ok {
		return SpecifierExternalPackage, pkg
	}
	return SpecifierUnknown, ""
}

func ExternalPackageName(specifier string) (string, bool) {
	if specifier == "" || strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../") || strings.HasPrefix(specifier, "/") || strings.HasPrefix(specifier, "#") {
		return "", false
	}
	if strings.HasPrefix(specifier, "@") {
		parts := strings.Split(specifier, "/")
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return "", false
		}
		return parts[0] + "/" + parts[1], true
	}
	parts := strings.Split(specifier, "/")
	if parts[0] == "" {
		return "", false
	}
	return parts[0], true
}

func ResolveRelative(baseFile, spec string) (string, bool) {
	base := filepath.Dir(baseFile)
	cand := filepath.Clean(filepath.Join(base, spec))
	exts := []string{".ts", ".tsx", ".js", ".jsx", ".mts", ".cts"}
	tries := []string{}
	if filepath.Ext(cand) != "" {
		tries = append(tries, cand)
	} else {
		for _, e := range exts {
			tries = append(tries, cand+e)
		}
		for _, e := range exts {
			tries = append(tries, filepath.Join(cand, "index"+e))
		}
	}
	for _, p := range tries {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return filepath.Clean(p), true
		}
	}
	return "", false
}

func SortImports(imps []Import) {
	sort.SliceStable(imps, func(i, j int) bool {
		if imps[i].Specifier != imps[j].Specifier {
			return imps[i].Specifier < imps[j].Specifier
		}
		return imps[i].Kind < imps[j].Kind
	})
}

var builtins = map[string]bool{"fs": true, "path": true, "url": true, "os": true, "crypto": true, "stream": true, "http": true, "https": true, "zlib": true, "util": true, "events": true, "buffer": true}
