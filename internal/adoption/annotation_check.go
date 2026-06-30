package adoption

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
)

type AnnotationCheckReport struct {
	Root                string                   `json:"root"`
	PackagesChecked     int                      `json:"packagesChecked"`
	AnnotationManifests int                      `json:"annotationManifests"`
	Summary             AnnotationCheckSummary   `json:"summary"`
	Packages            []AnnotationCheckPackage `json:"packages"`
	Findings            []AnnotationCheckFinding `json:"findings"`
	HasErrors           bool                     `json:"hasErrors"`
	HasWarnings         bool                     `json:"hasWarnings"`
	HasNotices          bool                     `json:"hasNotices"`
}

type AnnotationCheckSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Notices  int `json:"notices"`
}

type AnnotationCheckPackage struct {
	PackageRoot            string                   `json:"packageRoot"`
	PackageJSONPath        string                   `json:"packageJsonPath"`
	PackageName            string                   `json:"packageName,omitempty"`
	AnnotationManifestPath string                   `json:"annotationManifestPath"`
	AnnotationKind         string                   `json:"annotationKind"`
	DependencyCounts       map[string]int           `json:"dependencyCounts"`
	Findings               []AnnotationCheckFinding `json:"findings"`
}

type AnnotationCheckFinding struct {
	Code               string `json:"code"`
	Severity           string `json:"severity"`
	PackageName        string `json:"packageName,omitempty"`
	PackageRoot        string `json:"packageRoot"`
	DependencyName     string `json:"dependencyName,omitempty"`
	AnnotationKind     string `json:"annotationKind,omitempty"`
	PackageJSONSection string `json:"packageJsonSection,omitempty"`
	AnnotationRange    string `json:"annotationRange,omitempty"`
	PackageJSONRange   string `json:"packageJsonRange,omitempty"`
	Message            string `json:"message"`
}

func CheckPackageAnnotations(root string, frontendCLIPath string) (AnnotationCheckReport, error) {
	obs, err := Observe(root)
	if err != nil {
		return AnnotationCheckReport{}, err
	}
	annotations, err := DiscoverPackageAnnotations(obs.Root, frontendCLIPath)
	if err != nil {
		return AnnotationCheckReport{}, err
	}

	report := AnnotationCheckReport{Root: obs.Root}
	for _, annotation := range annotations {
		checkedPackage := checkOneAnnotationPackage(obs.Root, annotation)
		report.Packages = append(report.Packages, checkedPackage)
		report.Findings = append(report.Findings, checkedPackage.Findings...)
	}

	report.PackagesChecked = len(report.Packages)
	report.AnnotationManifests = len(report.Packages)
	sort.Slice(report.Findings, func(i, j int) bool {
		return findingSortKey(report.Findings[i]) < findingSortKey(report.Findings[j])
	})

	for _, finding := range report.Findings {
		switch finding.Severity {
		case "error":
			report.Summary.Errors++
		case "warning":
			report.Summary.Warnings++
		case "notice":
			report.Summary.Notices++
		}
	}

	report.HasErrors = report.Summary.Errors > 0
	report.HasWarnings = report.Summary.Warnings > 0
	report.HasNotices = report.Summary.Notices > 0
	return report, nil
}

func checkOneAnnotationPackage(root string, annotation PackageAnnotation) AnnotationCheckPackage {
	checkedPackage := AnnotationCheckPackage{
		PackageRoot:            annotation.Root,
		PackageJSONPath:        annotation.PackageJSONPath,
		PackageName:            annotation.PackageName,
		AnnotationManifestPath: annotation.ManifestPath,
		AnnotationKind:         "package-annotations",
		DependencyCounts:       copyIntMap(annotation.DependencyCounts),
	}

	pkgPath := filepath.Join(root, filepath.FromSlash(annotation.PackageJSONPath))
	pkg, readWarnings := readAnnotationPackageJSON(pkgPath)
	if len(readWarnings) > 0 {
		for _, warning := range readWarnings {
			checkedPackage.Findings = append(checkedPackage.Findings, AnnotationCheckFinding{
				Code:        "package-json-missing/malformed",
				Severity:    "error",
				PackageName: annotation.PackageName,
				PackageRoot: annotation.Root,
				Message:     warning,
			})
		}
		return checkedPackage
	}

	annotatedNames := map[string]bool{}
	for _, dep := range annotation.Dependencies {
		annotatedNames[dep.Name] = true
		checkedPackage.Findings = append(checkedPackage.Findings, checkAnnotatedDependency(annotation, dep)...)
	}

	for _, dep := range packageJSONDependencyEntries(pkg) {
		if annotatedNames[dep.name] {
			continue
		}
		checkedPackage.Findings = append(checkedPackage.Findings, AnnotationCheckFinding{
			Code:               "unannotated-package-json-dependency",
			Severity:           "notice",
			PackageName:        annotation.PackageName,
			PackageRoot:        annotation.Root,
			DependencyName:     dep.name,
			PackageJSONSection: dep.section,
			PackageJSONRange:   dep.versionRange,
			Message:            fmt.Sprintf("%s is in %s but not annotated", dep.name, dep.section),
		})
	}

	sort.Slice(checkedPackage.Findings, func(i, j int) bool {
		return findingSortKey(checkedPackage.Findings[i]) < findingSortKey(checkedPackage.Findings[j])
	})
	return checkedPackage
}

func checkAnnotatedDependency(annotation PackageAnnotation, dep AnnotatedDep) []AnnotationCheckFinding {
	if dep.PackageJSONSection == "" {
		return []AnnotationCheckFinding{{
			Code:            "missing-in-package-json",
			Severity:        "error",
			PackageName:     annotation.PackageName,
			PackageRoot:     annotation.Root,
			DependencyName:  dep.Name,
			AnnotationKind:  dep.Kind,
			AnnotationRange: dep.Range,
			Message:         fmt.Sprintf("annotation declares %s(%s) but package.json does not list it in a dependency section", dep.Kind, dep.Name),
		}}
	}

	findings := []AnnotationCheckFinding{}
	if !annotationKindMatchesPackageJSONSection(dep.Kind, dep.PackageJSONSection) {
		findings = append(findings, AnnotationCheckFinding{
			Code:               "classification-mismatch",
			Severity:           "warning",
			PackageName:        annotation.PackageName,
			PackageRoot:        annotation.Root,
			DependencyName:     dep.Name,
			AnnotationKind:     dep.Kind,
			PackageJSONSection: dep.PackageJSONSection,
			AnnotationRange:    dep.Range,
			PackageJSONRange:   dep.PackageJSONRange,
			Message:            fmt.Sprintf("annotation says %s(%s) but package.json lists it in %s", dep.Kind, dep.Name, dep.PackageJSONSection),
		})
	}
	if dep.Range != "" && dep.PackageJSONRange != "" && dep.Range != dep.PackageJSONRange {
		findings = append(findings, AnnotationCheckFinding{
			Code:               "range-mismatch",
			Severity:           "warning",
			PackageName:        annotation.PackageName,
			PackageRoot:        annotation.Root,
			DependencyName:     dep.Name,
			AnnotationKind:     dep.Kind,
			PackageJSONSection: dep.PackageJSONSection,
			AnnotationRange:    dep.Range,
			PackageJSONRange:   dep.PackageJSONRange,
			Message:            fmt.Sprintf("annotation range for %s is %s but package.json declares %s", dep.Name, dep.Range, dep.PackageJSONRange),
		})
	}
	return findings
}

func annotationKindMatchesPackageJSONSection(kind string, section string) bool {
	switch kind {
	case "dep":
		return section == "dependencies" || section == "optionalDependencies"
	case "peer":
		return section == "peerDependencies"
	case "tool":
		return section == "devDependencies"
	default:
		return true
	}
}

type dependencyEntry struct {
	name         string
	section      string
	versionRange string
}

func packageJSONDependencyEntries(pkg packageJSON) []dependencyEntry {
	entries := []dependencyEntry{}
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
		for name, versionRange := range section.deps {
			entries = append(entries, dependencyEntry{
				name:         name,
				section:      section.name,
				versionRange: versionRange,
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].section != entries[j].section {
			return entries[i].section < entries[j].section
		}
		return entries[i].name < entries[j].name
	})
	return entries
}

func copyIntMap(input map[string]int) map[string]int {
	output := map[string]int{}
	for key, value := range input {
		output[key] = value
	}
	return output
}

func findingSortKey(finding AnnotationCheckFinding) string {
	encoded, err := json.Marshal([]string{
		finding.PackageRoot,
		finding.Severity,
		finding.Code,
		finding.DependencyName,
	})
	if err != nil {
		return finding.PackageRoot + finding.Severity + finding.Code + finding.DependencyName
	}
	return string(encoded)
}
