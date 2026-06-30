package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/adoption"
	"github.com/yuechen-li-dev/tspack/internal/npmobserve"
)

func runAdoptCommand(args []string) {
	root := "."
	reportRequested := false
	securityRequested := false
	suggestPackage := ""
	jsonOutput := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--root":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "TSPACK_ADOPT_ROOT_REQUIRED: --root requires a path")
				os.Exit(1)
			}
			root = args[i]
		case "--report":
			reportRequested = true
		case "--security":
			securityRequested = true
		case "--suggest-package":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "TSPACK_ADOPT_SUGGEST_PACKAGE_REQUIRED: --suggest-package requires a package root")
				os.Exit(1)
			}
			suggestPackage = args[i]
		case "--json":
			jsonOutput = true
		default:
			fmt.Fprintf(os.Stderr, "unknown adopt flag: %s\n", args[i])
			os.Exit(1)
		}
	}
	requestedCount := 0
	if reportRequested {
		requestedCount++
	}
	if securityRequested {
		requestedCount++
	}
	if suggestPackage != "" {
		requestedCount++
	}
	if requestedCount > 1 {
		fmt.Fprintln(os.Stderr, "TSPACK_ADOPT_INVALID_ARGS: --report, --security, and --suggest-package cannot be combined")
		os.Exit(1)
	}
	if requestedCount == 0 {
		fmt.Fprintln(os.Stderr, "TSPACK_ADOPT_INVALID_ARGS: adopt requires --report, --security, or --suggest-package")
		os.Exit(1)
	}
	if securityRequested {
		runAdoptSecurity(root, jsonOutput)
		return
	}
	if suggestPackage != "" {
		runAdoptSuggestPackage(root, suggestPackage)
		return
	}
	obs, err := adoption.Observe(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	annotations, err := adoption.DiscoverPackageAnnotations(obs.Root, manifestFrontendCLIPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	report := adoption.BuildReportWithAnnotations(obs, annotations)
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "TSPACK_ADOPT_JSON_ENCODE_FAILED: %v\n", err)
			os.Exit(1)
		}
		return
	}
	printAdoptionReport(report)
}

func runAdoptSuggestPackage(root string, packageRoot string) {
	suggestion, err := adoption.SuggestPackageAnnotation(root, packageRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if suggestion.ExistingManifestKind != "" {
		fmt.Fprintf(os.Stderr, "TSPACK_ADOPT_SUGGEST_PACKAGE_MANIFEST_EXISTS: %s already exists as a %s; not overwriting.\n", suggestion.ManifestPath, suggestion.ExistingManifestKind)
	}
	fmt.Print(suggestion.Content)
}

func runAdoptSecurity(root string, jsonOutput bool) {
	report, err := npmobserve.Observe(npmobserve.Options{Root: root})
	if err != nil {
		fmt.Fprintf(os.Stderr, "TSPACK_ADOPT_SECURITY_FAILED: %v\n", err)
		os.Exit(1)
	}
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "TSPACK_ADOPT_SECURITY_JSON_ENCODE_FAILED: %v\n", err)
			os.Exit(1)
		}
		return
	}
	printAdoptSecurityReport(report)
}

func printAdoptionReport(report adoption.Report) {
	fmt.Println("TSPack adoption report")
	fmt.Println()
	fmt.Printf("Root: %s\n", report.Root)
	fmt.Printf("Package: %s", emptyAsDash(report.PackageName))
	if report.Version != "" {
		fmt.Printf("@%s", report.Version)
	}
	fmt.Println()
	fmt.Printf("Suggested adoption mode: %s\n", report.SuggestedAdoptionMode)
	fmt.Println()
	fmt.Println("Package.json dependency counts:")
	fmt.Printf("  dependencies: %d\n", report.DependencyCounts["dependencies"])
	fmt.Printf("  devDependencies: %d\n", report.DependencyCounts["devDependencies"])
	fmt.Printf("  peerDependencies: %d\n", report.DependencyCounts["peerDependencies"])
	fmt.Printf("  optionalDependencies: %d\n", report.DependencyCounts["optionalDependencies"])
	fmt.Println()
	fmt.Println("Scripts found:")
	if len(report.Scripts) == 0 {
		fmt.Println("  none")
	} else {
		fmt.Printf("  %s\n", strings.Join(report.Scripts, ", "))
	}
	fmt.Println()
	fmt.Println("Package manager lockfiles found:")
	if len(report.Lockfiles) == 0 {
		fmt.Println("  none")
	} else {
		for _, lockfile := range report.Lockfiles {
			fmt.Printf("  %s (%s)\n", lockfile.Name, lockfile.PackageManager)
		}
	}
	fmt.Println()
	fmt.Printf("manifest.tsx exists: %t\n", report.ManifestExists)
	fmt.Printf("ts-lock.toml exists: %t\n", report.LockfileExists)
	fmt.Println()
	printPackageAnnotations(report.PackageAnnotations)

	fmt.Println("Notes:")
	for _, warning := range report.Warnings {
		fmt.Printf("  - %s\n", warning)
	}
}

func printPackageAnnotations(annotations []adoption.PackageAnnotation) {
	fmt.Println("Package annotations:")
	if len(annotations) == 0 {
		fmt.Println("  none")
		fmt.Println()
		return
	}
	fmt.Println("  package.json remains authoritative; annotations report semantic intent only")
	for _, annotation := range annotations {
		fmt.Printf("  - %s (%s)\n", annotation.Root, emptyAsDash(annotation.PackageName))
		fmt.Printf("    manifest: %s\n", annotation.ManifestPath)
		fmt.Printf("    classification counts: dep=%d peer=%d tool=%d\n", annotation.DependencyCounts["dep"], annotation.DependencyCounts["peer"], annotation.DependencyCounts["tool"])
		for _, warning := range annotation.Warnings {
			fmt.Printf("    warning: %s\n", warning)
		}
	}
	fmt.Println()
}

func emptyAsDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func printAdoptSecurityReport(report npmobserve.Report) {
	fmt.Println("Observed npm lifecycle/security report")
	fmt.Println()
	fmt.Printf("Root: %s\n", report.RootDir)
	fmt.Println("Mode: read-only observed npm metadata")
	fmt.Println()

	fmt.Println("Sources:")
	fmt.Printf("  package.json: %s\n", report.PackageJSONPath)
	if report.LockfilePresent {
		fmt.Printf("  package-lock.json: %s\n", report.LockfilePath)
	} else {
		fmt.Println("  package-lock.json: not found")
	}
	if report.NodeModulesInspected {
		fmt.Println("  installed package metadata: inspected read-only")
	} else if report.NodeModulesPresent {
		fmt.Println("  installed package metadata: available but not inspected")
	} else {
		fmt.Println("  installed package metadata: node_modules not present")
	}
	fmt.Println()

	fmt.Println("Summary:")
	fmt.Printf("  lifecycle scripts found: %d\n", report.Summary.LifecycleScripts)
	fmt.Printf("  install-time hooks: %d\n", report.Summary.InstallTimeHooks)
	fmt.Printf("  direct hooks: %d\n", report.Summary.DirectHooks)
	fmt.Printf("  transitive hooks: %d\n", report.Summary.TransitiveHooks)
	fmt.Printf("  optional hooks: %d\n", report.Summary.OptionalHooks)
	fmt.Printf("  root lifecycle scripts: %d\n", report.Summary.RootLifecycleScripts)
	if len(report.MetadataSources) > 0 {
		fmt.Printf("  metadata sources: %s\n", strings.Join(report.MetadataSources, ", "))
	}
	fmt.Println()

	dependencyWarnings := filterCapabilityWarnings(report.CapabilityWarnings, false)
	if len(dependencyWarnings) > 0 {
		fmt.Println("Lifecycle capability warnings")
		for _, warning := range dependencyWarnings {
			printCapabilityWarning(warning)
		}
		fmt.Println()
	}

	if len(report.RootScripts) > 0 {
		fmt.Println("Root package lifecycle scripts")
		for _, warning := range filterCapabilityWarnings(report.CapabilityWarnings, true) {
			printCapabilityWarning(warning)
		}
		fmt.Println()
	}

	if len(report.Limitations) > 0 {
		fmt.Println("Metadata limitations:")
		for _, limitation := range report.Limitations {
			fmt.Printf("  - %s\n", limitation)
		}
		fmt.Println()
	}

	if len(report.Notes) > 0 {
		fmt.Println("Next steps:")
		for _, note := range report.Notes {
			fmt.Printf("  - %s\n", note)
		}
		fmt.Println()
	}

	fmt.Println("Adoption note:")
	fmt.Println("  This report is based on observed npm metadata. It is not yet a TSPack manifest security policy decision.")
}

func printCapabilityWarning(warning npmobserve.CapabilityWarning) {
	version := warning.Version
	if version == "" {
		version = "unknown"
	}
	if warning.Presence == npmobserve.PresenceRoot {
		fmt.Printf("%s\n", warning.Phase)
	} else {
		fmt.Printf("%s@%s\n", warning.PackageName, version)
	}
	fmt.Printf("  phase: %s\n", warning.Phase)
	fmt.Printf("  command: %s\n", warning.Command)
	fmt.Printf("  capability: %s\n", warning.Title)
	fmt.Printf("  attention: %s\n", formatWarningAttention(warning))
	fmt.Printf("  source: %s\n", warning.Source)
	fmt.Printf("  presence: %s\n", warning.Presence)
	if warning.Location != "" {
		fmt.Printf("  location: %s\n", warning.Location)
	}
	if len(warning.Chains) > 0 {
		fmt.Println("  why:")
		for _, chain := range warning.Chains {
			fmt.Printf("    %s\n", strings.Join(chain, " -> "))
		}
	}
	fmt.Println("  meaning:")
	fmt.Printf("    %s\n", warning.Meaning)
}

func formatWarningAttention(warning npmobserve.CapabilityWarning) string {
	parts := []string{warning.AttentionLevel}
	if warning.Reason != "" {
		parts = append(parts, warning.Reason)
	}
	if len(warning.Flags) > 0 {
		parts = append(parts, strings.Join(warning.Flags, ","))
	}
	return strings.Join(parts, " — ")
}

func filterCapabilityWarnings(warnings []npmobserve.CapabilityWarning, rootOnly bool) []npmobserve.CapabilityWarning {
	out := []npmobserve.CapabilityWarning{}
	for _, warning := range warnings {
		isRoot := warning.Presence == npmobserve.PresenceRoot
		if isRoot == rootOnly {
			out = append(out, warning)
		}
	}
	return out
}

func formatObservedScriptTitle(script npmobserve.LifecycleScript) string {
	if script.Presence == npmobserve.PresenceRoot {
		return fmt.Sprintf("%s [%s]", script.Phase, script.LifecycleCategory)
	}
	version := script.Version
	if version == "" {
		version = "unknown"
	}
	return fmt.Sprintf("%s@%s %s [%s]", script.PackageName, version, script.Phase, script.LifecycleCategory)
}

func formatObservedPresence(script npmobserve.LifecycleScript) string {
	parts := []string{script.Presence}
	if len(script.DependencySections) > 0 {
		parts = append(parts, strings.Join(script.DependencySections, "+"))
	}
	flags := []string{}
	if script.Optional {
		flags = append(flags, "optional")
	}
	if script.Dev {
		flags = append(flags, "dev")
	}
	if script.Peer {
		flags = append(flags, "peer")
	}
	if len(flags) > 0 {
		parts = append(parts, strings.Join(flags, ","))
	}
	return strings.Join(parts, " ")
}

func countObservedScriptsByPresence(scripts []npmobserve.LifecycleScript, presence string) int {
	count := 0
	for _, script := range scripts {
		if script.Presence == presence {
			count++
		}
	}
	return count
}
