package importscan

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/diag"
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
	reImportFrom              = regexp.MustCompile(`(?m)\bimport\s+(type\s+)?([^;\n]*?)\sfrom\s*["']([^"']+)["']`)
	reImportSide              = regexp.MustCompile(`(?m)\bimport[\t ]*["']([^"'\r\n]+)["']`)
	reExportFrom              = regexp.MustCompile(`(?m)\bexport\s+(type\s+)?(\*|\{[^}]*\})\s*from\s*["']([^"']+)["']`)
	reRequireLiteral          = regexp.MustCompile(`(?m)\brequire\s*\(\s*["']([^"']+)["']\s*\)`)
	reRequireAny              = regexp.MustCompile(`(?m)\brequire\s*\(`)
	reImportLiteral           = regexp.MustCompile(`(?m)\bimport\s*\(\s*["']([^"']+)["']\s*\)`)
	reImportDynamicAny        = regexp.MustCompile(`(?m)\bimport\s*\(`)
	reImportDynamicNonLiteral = regexp.MustCompile(`(?m)\bimport\s*\(\s*[^\s"'\)]`)
	reNamedTypeSpecifier      = regexp.MustCompile(`(?:^|[,\{])\s*type\s+[A-Za-z_$]`)
)

func ScanFile(path string) ([]Import, []diag.Diagnostic) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, []diag.Diagnostic{{Code: "TSPACK_IMPORT_PARSE_ERROR", Severity: diag.SeverityError, Message: "failed to read source file", File: path, Details: []string{err.Error()}}}
	}
	s := string(b)
	declarationFile := isDeclarationFile(path)
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
		clause := m[2]
		kind := ImportKindRuntime
		if declarationFile || strings.TrimSpace(m[1]) != "" || namedClauseIsTypeOnly(clause) {
			kind = ImportKindTypeOnly
		}
		add(m[3], kind)
		if kind == ImportKindRuntime && reNamedTypeSpecifier.MatchString(clause) {
			add(m[3], ImportKindTypeOnly)
		}
	}
	for _, m := range reImportSide.FindAllStringSubmatch(s, -1) {
		kind := ImportKindRuntime
		if declarationFile {
			kind = ImportKindTypeOnly
		}
		add(m[1], kind)
	}
	for _, m := range reExportFrom.FindAllStringSubmatch(s, -1) {
		clause := m[2]
		kind := ImportKindRuntime
		if declarationFile || strings.TrimSpace(m[1]) != "" || namedClauseIsTypeOnly(clause) {
			kind = ImportKindTypeOnly
		}
		add(m[3], kind)
		if kind == ImportKindRuntime && reNamedTypeSpecifier.MatchString(clause) {
			add(m[3], ImportKindTypeOnly)
		}
	}
	for _, m := range reRequireLiteral.FindAllStringSubmatch(s, -1) {
		add(m[1], ImportKindRuntime)
	}
	for _, m := range reImportLiteral.FindAllStringSubmatch(s, -1) {
		kind := ImportKindRuntime
		if declarationFile {
			kind = ImportKindTypeOnly
		}
		add(m[1], kind)
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

func namedClauseIsTypeOnly(clause string) bool {
	trimmed := strings.TrimSpace(clause)
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return false
	}
	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "{"), "}"))
	if body == "" {
		return false
	}
	for _, part := range strings.Split(body, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if !strings.HasPrefix(name, "type ") {
			return false
		}
	}
	return true
}

func isDeclarationFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, ".d.ts") || strings.HasSuffix(base, ".d.mts") || strings.HasSuffix(base, ".d.cts")
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
	candidate := filepath.Clean(filepath.Join(base, spec))
	tries := relativeResolutionCandidates(candidate)
	for _, path := range tries {
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			return filepath.Clean(path), true
		}
	}
	return "", false
}

func relativeResolutionCandidates(candidate string) []string {
	exts := []string{".ts", ".tsx", ".js", ".jsx", ".mts", ".cts"}
	ext := filepath.Ext(candidate)
	if ext == ".js" {
		return []string{
			candidate,
			strings.TrimSuffix(candidate, ext) + ".ts",
			strings.TrimSuffix(candidate, ext) + ".tsx",
			strings.TrimSuffix(candidate, ext) + ".jsx",
		}
	}
	if ext == ".jsx" {
		return []string{
			candidate,
			strings.TrimSuffix(candidate, ext) + ".tsx",
			strings.TrimSuffix(candidate, ext) + ".ts",
			strings.TrimSuffix(candidate, ext) + ".js",
		}
	}
	if ext != "" {
		return []string{candidate}
	}

	tries := []string{}
	for _, candidateExt := range exts {
		tries = append(tries, candidate+candidateExt)
	}
	for _, candidateExt := range exts {
		tries = append(tries, filepath.Join(candidate, "index"+candidateExt))
	}
	return tries
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
