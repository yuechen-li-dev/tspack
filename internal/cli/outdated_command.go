package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/project"
)

func runOutdatedCommand(args []string) {
	paths := newLifecycleProjectPaths()
	jsonOutput := false
	perPackage := false
	for index := 1; index < len(args); index++ {
		if paths.consume(args, &index) {
			continue
		}
		switch args[index] {
		case "--json":
			jsonOutput = true
		case "--per-package":
			perPackage = true
		default:
			failUnknownLifecycleFlag("outdated", args[index])
		}
	}
	operation := project.RunOutdated(project.OutdatedRequest{Project: paths.Options})
	result := project.Result{Diagnostics: operation.Diagnostics, Outdated: operation.Outdated}
	if jsonOutput {
		writeOutdatedJSON(paths.Options, result, perPackage)
		return
	}
	renderHumanDiagnostics(os.Stderr, result.Diagnostics, checkRenderOptions{})
	renderOutdatedText(result.Outdated, perPackage)
	exitForDiagnostics(result.Diagnostics)
}

func writeOutdatedJSON(options project.Options, result project.Result, perPackage bool) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	report := map[string]any{
		"command":      "outdated",
		"ok":           !hasErrors(result.Diagnostics),
		"root":         options.RootDir,
		"summary":      result.Outdated.Summary,
		"entries":      outdatedJSONEntries(result.Outdated, perPackage),
		"dependencies": result.Outdated.Dependencies,
		"diagnostics":  buildCheckJSONReport(options, result).Diagnostics,
	}
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "TSPACK_OUTDATED_JSON_ENCODE_FAILED: %v\n", err)
		os.Exit(1)
	}
	exitForDiagnostics(result.Diagnostics)
}

func renderOutdatedText(result *project.OutdatedResult, perPackage bool) {
	if result == nil {
		return
	}
	fmt.Println("TSPack outdated")
	fmt.Println()
	for _, dependency := range outdatedHumanEntries(result, perPackage) {
		fmt.Println(dependency.Name)
		fmt.Printf("  kind: %s\n", dependency.Kind)
		fmt.Printf("  requested: %s\n", dependency.Requested)
		if len(dependency.Current) == 0 {
			fmt.Println("  current: -")
		} else {
			fmt.Printf("  current: %s\n", strings.Join(dependency.Current, ", "))
		}
		fmt.Printf("  wanted: %s\n", dependency.Wanted)
		fmt.Printf("  latest: %s\n", dependency.Latest)
		fmt.Printf("  status: %s\n", strings.ReplaceAll(dependency.Status, "_", " "))
		if result.HasPolicy {
			fmt.Printf("  policy: %s", dependency.PolicyStatus)
			if dependency.PolicyStrategy != "" {
				fmt.Printf(" %s", dependency.PolicyStrategy)
			}
			if dependency.PolicyLevel != "" {
				fmt.Printf(":%s", dependency.PolicyLevel)
			}
			fmt.Println()
		}
		if dependency.PackageCount > 0 {
			fmt.Printf("  packages: %d\n", dependency.PackageCount)
			if len(dependency.Packages) > 0 {
				fmt.Printf("  declared by: %s\n", formatOutdatedPackages(dependency.Packages))
			}
		}
	}
	fmt.Println()
	fmt.Println("Summary:")
	fmt.Printf("  current: %d\n", result.Summary.Current)
	fmt.Printf("  outdated: %d\n", result.Summary.Outdated)
	fmt.Printf("  skipped: %d\n", result.Summary.Skipped)
	fmt.Printf("  errors: %d\n", result.Summary.Errors)
}

func outdatedHumanEntries(result *project.OutdatedResult, perPackage bool) []project.OutdatedDependency {
	if result == nil {
		return nil
	}
	if perPackage || len(result.Groups) == 0 {
		return result.Dependencies
	}
	return result.Groups
}

type outdatedJSONEntry struct {
	Name           string                    `json:"name"`
	Kind           string                    `json:"kind"`
	Source         string                    `json:"source,omitempty"`
	Requested      string                    `json:"requested"`
	Current        []string                  `json:"current"`
	Wanted         string                    `json:"wanted"`
	Latest         string                    `json:"latest"`
	Status         string                    `json:"status"`
	Packages       []project.OutdatedPackage `json:"packages"`
	PackageCount   int                       `json:"packageCount"`
	PolicyStrategy string                    `json:"policyStrategy,omitempty"`
	PolicyLevel    string                    `json:"policyLevel,omitempty"`
	PolicyStatus   string                    `json:"policyStatus,omitempty"`
	PolicyReason   string                    `json:"policyReason,omitempty"`
	PolicyMatched  bool                      `json:"policyMatched"`
	PolicyRow      int                       `json:"policyRow,omitempty"`
	PolicyMessage  string                    `json:"policyMessage,omitempty"`
}

func outdatedJSONEntries(result *project.OutdatedResult, perPackage bool) []outdatedJSONEntry {
	dependencies := outdatedHumanEntries(result, perPackage)
	entries := make([]outdatedJSONEntry, 0, len(dependencies))
	for _, dep := range dependencies {
		entry := outdatedJSONEntry{
			Name:           dep.Name,
			Kind:           dep.Kind,
			Source:         dep.Source,
			Requested:      dep.Requested,
			Current:        dep.Current,
			Wanted:         dep.Wanted,
			Latest:         dep.Latest,
			Status:         dep.Status,
			Packages:       dep.Packages,
			PackageCount:   dep.PackageCount,
			PolicyStrategy: dep.PolicyStrategy,
			PolicyLevel:    dep.PolicyLevel,
			PolicyStatus:   dep.PolicyStatus,
			PolicyReason:   dep.PolicyReason,
			PolicyMatched:  dep.PolicyMatched,
			PolicyRow:      dep.PolicyRow,
			PolicyMessage:  dep.PolicyMessage,
		}
		entries = append(entries, entry)
	}
	return entries
}

func formatOutdatedPackages(packages []project.OutdatedPackage) string {
	names := make([]string, 0, len(packages))
	for _, pkg := range packages {
		if pkg.Name != "" {
			names = append(names, pkg.Name)
			continue
		}
		names = append(names, pkg.Root)
	}
	return strings.Join(names, ", ")
}
