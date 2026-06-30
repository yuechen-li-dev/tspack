package adoption

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

type PackageSuggestion struct {
	PackageRoot          string
	PackageJSONPath      string
	ManifestPath         string
	ExistingManifestKind string
	Dependencies         []SuggestedDependency
	Notes                []string
	Content              string
}

type SuggestedDependency struct {
	Key     string
	Name    string
	Range   string
	Kind    string
	Section string
}

func SuggestPackageAnnotation(root string, packageRoot string) (PackageSuggestion, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return PackageSuggestion{}, fmt.Errorf("resolve adoption root: %w", err)
	}
	absPackageRoot := packageRoot
	if !filepath.IsAbs(absPackageRoot) {
		absPackageRoot = filepath.Join(absRoot, filepath.FromSlash(packageRoot))
	}
	absPackageRoot, err = filepath.Abs(absPackageRoot)
	if err != nil {
		return PackageSuggestion{}, fmt.Errorf("resolve package root: %w", err)
	}
	pkgPath := filepath.Join(absPackageRoot, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return PackageSuggestion{}, fmt.Errorf("TSPACK_ADOPT_SUGGEST_PACKAGE_JSON_MISSING: package.json was not found at %s", pkgPath)
		}
		return PackageSuggestion{}, fmt.Errorf("read package.json for suggestion: %w", err)
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return PackageSuggestion{}, fmt.Errorf("TSPACK_ADOPT_SUGGEST_PACKAGE_JSON_MALFORMED: package.json is not valid JSON: %w", err)
	}
	deps, notes := suggestedDependencies(pkg)
	assignSuggestedKeys(deps)
	relRoot := mustRel(absRoot, absPackageRoot)
	if relRoot == "" {
		relRoot = "."
	}
	manifestPath := filepath.Join(absPackageRoot, "package.manifest.tsx")
	suggestion := PackageSuggestion{PackageRoot: filepath.ToSlash(relRoot), PackageJSONPath: filepath.ToSlash(mustRel(absRoot, pkgPath)), ManifestPath: filepath.ToSlash(mustRel(absRoot, manifestPath)), ExistingManifestKind: existingManifestKind(manifestPath), Dependencies: deps, Notes: notes}
	suggestion.Content = renderPackageAnnotationSuggestion(suggestion)
	return suggestion, nil
}

func suggestedDependencies(pkg packageJSON) ([]SuggestedDependency, []string) {
	deps := []SuggestedDependency{}
	notes := []string{}
	addSection := func(section string, values map[string]string, kind string) {
		for name, depRange := range values {
			suggestedKind := kind
			if section != "peerDependencies" && isObviousToolPackage(name) {
				suggestedKind = "tool"
				notes = append(notes, fmt.Sprintf("%s is declared in %s but looks like TypeScript/build tooling; suggested as tool(...).", name, section))
			}
			deps = append(deps, SuggestedDependency{Name: name, Range: depRange, Kind: suggestedKind, Section: section})
		}
	}
	addSection("dependencies", pkg.Dependencies, "dep")
	addSection("optionalDependencies", pkg.OptionalDependencies, "dep")
	addSection("peerDependencies", pkg.PeerDependencies, "peer")
	for name, depRange := range pkg.DevDependencies {
		deps = append(deps, SuggestedDependency{Name: name, Range: depRange, Kind: "tool", Section: "devDependencies"})
		if name == "typescript" {
			notes = append(notes, "typescript is a devDependency and suggested as tool(...).")
		}
	}
	if _, ok := pkg.Dependencies["react"]; ok && packageLooksLikeLibrary(pkg) {
		notes = append(notes, "react is in dependencies. If this package is a library, consider peer(...) instead.")
	}
	for name := range pkg.OptionalDependencies {
		notes = append(notes, fmt.Sprintf("%s is in optionalDependencies and is suggested as dep(...); review optional runtime behavior manually.", name))
	}
	sort.Slice(deps, func(i, j int) bool {
		if deps[i].Kind != deps[j].Kind {
			return kindRank(deps[i].Kind) < kindRank(deps[j].Kind)
		}
		return deps[i].Name < deps[j].Name
	})
	sort.Strings(notes)
	return deps, notes
}

func isObviousToolPackage(name string) bool {
	if strings.HasPrefix(name, "@types/") {
		return true
	}
	toolNames := map[string]bool{
		"typescript":           true,
		"vite":                 true,
		"@vitejs/plugin-react": true,
		"tsx":                  true,
		"vitest":               true,
		"playwright":           true,
		"eslint":               true,
		"prettier":             true,
		"@biomejs/biome":       true,
		"rollup":               true,
		"esbuild":              true,
		"tsup":                 true,
		"webpack":              true,
		"babel":                true,
		"swc":                  true,
	}
	if toolNames[name] {
		return true
	}
	return strings.HasPrefix(name, "@babel/") || strings.HasPrefix(name, "@swc/")
}

func packageLooksLikeLibrary(pkg packageJSON) bool {
	if pkg.Exports != nil || pkg.Types != "" || pkg.Main != "" || pkg.Module != "" {
		return true
	}
	for script := range pkg.Scripts {
		if script == "dev" || script == "start" {
			return false
		}
	}
	return false
}

func kindRank(kind string) int {
	switch kind {
	case "dep":
		return 0
	case "peer":
		return 1
	case "tool":
		return 2
	default:
		return 3
	}
}

func assignSuggestedKeys(deps []SuggestedDependency) {
	counts := map[string]int{}
	for i := range deps {
		base := packageNameToIdentifier(deps[i].Name)
		counts[base]++
		if counts[base] == 1 {
			deps[i].Key = base
		} else {
			deps[i].Key = fmt.Sprintf("%s%d", base, counts[base])
		}
	}
}

func packageNameToIdentifier(name string) string {
	trimmed := strings.TrimPrefix(name, "@")
	parts := regexp.MustCompile(`[^A-Za-z0-9]+`).Split(trimmed, -1)
	out := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		lower := strings.ToLower(part[:1]) + part[1:]
		if out == "" {
			out = lower
		} else {
			out += strings.ToUpper(lower[:1]) + lower[1:]
		}
	}
	if out == "" {
		out = "dep"
	}
	first, _ := utf8FirstRune(out)
	if !unicode.IsLetter(first) && first != '_' {
		out = "dep" + strings.ToUpper(out[:1]) + out[1:]
	}
	return out
}

func utf8FirstRune(value string) (rune, bool) {
	for _, r := range value {
		return r, true
	}
	return 0, false
}

func existingManifestKind(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := string(data)
	if strings.Contains(text, "definePackage") {
		return "full package manifest"
	}
	if strings.Contains(text, "annotatePackage") {
		return "package annotation manifest"
	}
	return "package manifest"
}

func renderPackageAnnotationSuggestion(s PackageSuggestion) string {
	imports := []string{"PackageAnnotations", "annotatePackage", "defineDeps"}
	used := map[string]bool{"npm": len(s.Dependencies) > 0}
	for _, dep := range s.Dependencies {
		used[dep.Kind] = true
	}
	for _, name := range []string{"dep", "npm", "peer", "tool"} {
		if used[name] {
			imports = append(imports, name)
		}
	}
	var b strings.Builder
	b.WriteString("// Suggested by `tspack adopt --suggest-package`.\n")
	b.WriteString("// Review before committing. package.json remains authoritative.\n")
	if len(s.Notes) > 0 {
		b.WriteString("// Notes:\n")
		for _, note := range s.Notes {
			b.WriteString("// - " + note + "\n")
		}
	}
	b.WriteString("import {\n")
	for _, item := range imports {
		b.WriteString("  " + item + ",\n")
	}
	b.WriteString("} from \"tspack/manifest\";\n\n")
	b.WriteString("const deps = defineDeps({\n")
	for _, dep := range s.Dependencies {
		b.WriteString(fmt.Sprintf("  %s: %s(npm(%q, %q)),\n", dep.Key, dep.Kind, dep.Name, dep.Range))
	}
	b.WriteString("});\n\n")
	b.WriteString("export default annotatePackage(\n")
	if len(s.Dependencies) == 0 {
		b.WriteString("  <PackageAnnotations />")
	} else {
		b.WriteString("  <PackageAnnotations dependencies={{ values: [\n")
		for _, dep := range s.Dependencies {
			b.WriteString("    deps." + dep.Key + ",\n")
		}
		b.WriteString("  ] }} />")
	}
	b.WriteString(",\n);\n")
	return b.String()
}
