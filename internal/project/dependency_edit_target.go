package project

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func selectDependencyEditTarget(
	packages []manifest.Package,
	requested string,
	rootDirectory string,
	workingDirectory string,
	diagnosticPrefix string,
) (*manifest.Package, []diag.Diagnostic) {
	if len(packages) == 0 {
		npmCommand := "install"
		if strings.HasSuffix(diagnosticPrefix, "REMOVE") {
			npmCommand = "uninstall"
		}
		return nil, []diag.Diagnostic{dependencyEditDiagnostic(
			diagnosticPrefix+"_AUTHORITY_DENIED",
			"no editable native package is available; package.json remains dependency-authoritative",
			"Use: tspack npm "+npmCommand+" <package>",
		)}
	}
	if requested != "" {
		for index := range packages {
			if packages[index].Name == requested {
				return &packages[index], nil
			}
		}
		pathMatches := packagesMatchingDirectory(packages, rootDirectory, requestedDirectory(rootDirectory, workingDirectory, requested), false)
		if len(pathMatches) == 1 {
			return pathMatches[0], nil
		}
		return nil, []diag.Diagnostic{dependencyEditDiagnostic(
			diagnosticPrefix+"_PACKAGE_TARGET_NOT_FOUND",
			"selected package target was not found",
			requested,
			"Available packages: "+strings.Join(packageNames(packages), ", "),
		)}
	}
	if len(packages) == 1 {
		return &packages[0], nil
	}
	currentMatches := packagesMatchingDirectory(packages, rootDirectory, workingDirectory, true)
	if len(currentMatches) == 1 {
		return currentMatches[0], nil
	}
	return nil, []diag.Diagnostic{dependencyEditDiagnostic(
		diagnosticPrefix+"_PACKAGE_TARGET_AMBIGUOUS",
		"several editable packages are available; select one with --package",
		packageNames(packages)...,
	)}
}

func requestedDirectory(rootDirectory string, workingDirectory string, requested string) string {
	if requested == "." && workingDirectory != "" {
		return workingDirectory
	}
	if filepath.IsAbs(requested) {
		return requested
	}
	return filepath.Join(rootDirectory, filepath.FromSlash(requested))
}

func packagesMatchingDirectory(packages []manifest.Package, rootDirectory string, directory string, allowContaining bool) []*manifest.Package {
	if directory == "" {
		return nil
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return nil
	}
	type match struct {
		packageIndex int
		rootLength   int
	}
	matches := []match{}
	for index := range packages {
		packageRoot := packages[index].Root
		if packageRoot == "" {
			packageRoot = "."
		}
		absoluteRoot, err := filepath.Abs(filepath.Join(rootDirectory, filepath.FromSlash(packageRoot)))
		if err != nil {
			continue
		}
		relative, err := filepath.Rel(absoluteRoot, directory)
		if err != nil {
			continue
		}
		exact := relative == "."
		contained := relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
		if exact || allowContaining && contained {
			matches = append(matches, match{packageIndex: index, rootLength: len(absoluteRoot)})
		}
	}
	if allowContaining && len(matches) > 1 {
		sort.SliceStable(matches, func(left int, right int) bool {
			return matches[left].rootLength > matches[right].rootLength
		})
		if matches[0].rootLength > matches[1].rootLength {
			matches = matches[:1]
		}
	}
	result := make([]*manifest.Package, 0, len(matches))
	for _, match := range matches {
		result = append(result, &packages[match.packageIndex])
	}
	return result
}

func dependencyEditDiagnostic(code string, message string, details ...string) diag.Diagnostic {
	return diag.Diagnostic{Code: code, Severity: diag.SeverityError, Message: message, Details: details}
}
