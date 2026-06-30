package adoption

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/nodecmd"
)

type PackageAnnotation struct {
	Root             string         `json:"root"`
	ManifestPath     string         `json:"manifestPath"`
	PackageJSONPath  string         `json:"packageJsonPath,omitempty"`
	PackageName      string         `json:"packageName,omitempty"`
	Version          string         `json:"version,omitempty"`
	AnnotationName   string         `json:"annotationName,omitempty"`
	DependencyCounts map[string]int `json:"dependencyCounts"`
	Dependencies     []AnnotatedDep `json:"dependencies,omitempty"`
	Warnings         []string       `json:"warnings,omitempty"`
}

type AnnotatedDep struct {
	Key                string `json:"key,omitempty"`
	Name               string `json:"name"`
	Kind               string `json:"kind"`
	Range              string `json:"range,omitempty"`
	PackageJSONSection string `json:"packageJsonSection,omitempty"`
	PackageJSONRange   string `json:"packageJsonRange,omitempty"`
}

type parsedAnnotationIR struct {
	PackageAnnotations []struct {
		Name         string `json:"name"`
		Dependencies []struct {
			Key    string `json:"key"`
			Kind   string `json:"kind"`
			Source struct {
				Kind    string `json:"kind"`
				Package string `json:"package"`
				Range   string `json:"range"`
			} `json:"source"`
		} `json:"dependencies"`
	} `json:"packageAnnotations"`
}

func DiscoverPackageAnnotations(root string, frontendCLIPath string) ([]PackageAnnotation, error) {
	if frontendCLIPath == "" {
		return nil, nil
	}
	roots := discoverWorkspaceRoots(root)
	if len(roots) == 0 {
		roots = []string{"."}
	}
	annotations := []PackageAnnotation{}
	for _, relRoot := range roots {
		manifestPath := filepath.Join(root, filepath.FromSlash(relRoot), "package.manifest.tsx")
		if !fileExists(manifestPath) {
			continue
		}
		annotation, ok, err := parsePackageAnnotation(root, relRoot, manifestPath, frontendCLIPath)
		if err != nil {
			return nil, err
		}
		if ok {
			annotations = append(annotations, annotation)
		}
	}
	sort.Slice(annotations, func(i, j int) bool { return annotations[i].Root < annotations[j].Root })
	return annotations, nil
}

func parsePackageAnnotation(root string, relRoot string, manifestPath string, frontendCLIPath string) (PackageAnnotation, bool, error) {
	cmd, err := nodecmd.Command(frontendCLIPath, manifestPath)
	if err != nil {
		if nodecmd.IsNotFound(err) {
			return PackageAnnotation{}, false, fmt.Errorf("%s: %s", nodecmd.DiagnosticCode, nodecmd.MessageBody())
		}
		return PackageAnnotation{}, false, err
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return PackageAnnotation{}, false, fmt.Errorf("TSPACK_ADOPT_PACKAGE_ANNOTATION_PARSE_FAILED: %s: %v: %s", manifestPath, err, strings.TrimSpace(stderr.String()))
	}
	var parsed struct {
		OK bool            `json:"ok"`
		IR json.RawMessage `json:"ir"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		return PackageAnnotation{}, false, fmt.Errorf("TSPACK_ADOPT_PACKAGE_ANNOTATION_INVALID_FRONTEND_JSON: %w", err)
	}
	var ir parsedAnnotationIR
	if err := json.Unmarshal(parsed.IR, &ir); err != nil {
		return PackageAnnotation{}, false, fmt.Errorf("TSPACK_ADOPT_PACKAGE_ANNOTATION_INVALID_IR: %w", err)
	}
	if len(ir.PackageAnnotations) == 0 {
		return PackageAnnotation{}, false, nil
	}
	pkgPath := filepath.Join(root, filepath.FromSlash(relRoot), "package.json")
	pkg, warnings := readAnnotationPackageJSON(pkgPath)
	annotation := PackageAnnotation{
		Root:             filepath.ToSlash(relRoot),
		ManifestPath:     filepath.ToSlash(mustRel(root, manifestPath)),
		PackageJSONPath:  filepath.ToSlash(mustRel(root, pkgPath)),
		PackageName:      pkg.Name,
		Version:          pkg.Version,
		AnnotationName:   ir.PackageAnnotations[0].Name,
		DependencyCounts: map[string]int{"dep": 0, "peer": 0, "tool": 0},
		Warnings:         warnings,
	}
	if annotation.Root == "" {
		annotation.Root = "."
	}
	if annotation.AnnotationName != "" && pkg.Name != "" && annotation.AnnotationName != pkg.Name {
		annotation.Warnings = append(annotation.Warnings, fmt.Sprintf("annotation name %q differs from package.json name %q", annotation.AnnotationName, pkg.Name))
	}
	for _, dep := range ir.PackageAnnotations[0].Dependencies {
		name := dep.Source.Package
		if name == "" {
			name = dep.Key
		}
		section, declaredRange := packageJSONSectionFor(pkg, name)
		item := AnnotatedDep{Key: dep.Key, Name: name, Kind: dep.Kind, Range: dep.Source.Range, PackageJSONSection: section, PackageJSONRange: declaredRange}
		annotation.Dependencies = append(annotation.Dependencies, item)
		annotation.DependencyCounts[dep.Kind]++
		annotation.Warnings = append(annotation.Warnings, validateAnnotatedDependency(item)...)
	}
	sort.Strings(annotation.Warnings)
	return annotation, true, nil
}

func readAnnotationPackageJSON(path string) (packageJSON, []string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return packageJSON{}, []string{"package.json is missing; package annotations are compared against package.json during incremental adoption"}
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return packageJSON{}, []string{"package.json is malformed; package annotations could not be compared"}
	}
	return pkg, nil
}

func packageJSONSectionFor(pkg packageJSON, name string) (string, string) {
	sections := []struct {
		name string
		deps map[string]string
	}{
		{"dependencies", pkg.Dependencies},
		{"devDependencies", pkg.DevDependencies},
		{"peerDependencies", pkg.PeerDependencies},
		{"optionalDependencies", pkg.OptionalDependencies},
	}
	for _, section := range sections {
		if value, ok := section.deps[name]; ok {
			return section.name, value
		}
	}
	return "", ""
}

func validateAnnotatedDependency(dep AnnotatedDep) []string {
	if dep.PackageJSONSection == "" {
		return []string{fmt.Sprintf("annotation %s(%s) is not present in package.json dependency fields", dep.Kind, dep.Name)}
	}
	warnings := []string{}
	if dep.Kind == "peer" && dep.PackageJSONSection != "peerDependencies" {
		warnings = append(warnings, fmt.Sprintf("annotation peer(%s) differs from package.json section %s", dep.Name, dep.PackageJSONSection))
	}
	if dep.Kind == "tool" && dep.PackageJSONSection != "devDependencies" {
		warnings = append(warnings, fmt.Sprintf("annotation tool(%s) differs from package.json section %s", dep.Name, dep.PackageJSONSection))
	}
	if dep.Kind == "dep" && dep.PackageJSONSection == "devDependencies" {
		warnings = append(warnings, fmt.Sprintf("annotation dep(%s) differs from package.json section devDependencies", dep.Name))
	}
	if dep.Range != "" && dep.PackageJSONRange != "" && dep.Range != dep.PackageJSONRange {
		warnings = append(warnings, fmt.Sprintf("annotation range for %s is %s but package.json declares %s", dep.Name, dep.Range, dep.PackageJSONRange))
	}
	return warnings
}

func discoverWorkspaceRoots(root string) []string {
	obs, err := Observe(root)
	if err != nil {
		return []string{}
	}
	roots := []string{"."}
	for _, pattern := range obs.Workspaces.Packages {
		roots = append(roots, expandSimpleWorkspacePattern(root, pattern)...)
	}
	return uniqueStrings(roots)
}

func expandSimpleWorkspacePattern(root string, pattern string) []string {
	if !strings.HasSuffix(pattern, "/*") || strings.Contains(strings.TrimSuffix(pattern, "/*"), "*") {
		return nil
	}
	base := strings.TrimSuffix(pattern, "/*")
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(base)))
	if err != nil {
		return nil
	}
	out := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(base, entry.Name()))
		if fileExists(filepath.Join(root, filepath.FromSlash(rel), "package.json")) {
			out = append(out, rel)
		}
	}
	return out
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func mustRel(root string, target string) string {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return target
	}
	return rel
}
