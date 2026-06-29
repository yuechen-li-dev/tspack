package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/adoption"
)

func runAdoptCommand(args []string) {
	root := "."
	reportRequested := false
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
		case "--json":
			jsonOutput = true
		default:
			fmt.Fprintf(os.Stderr, "unknown adopt flag: %s\n", args[i])
			os.Exit(1)
		}
	}
	if !reportRequested {
		fmt.Fprintln(os.Stderr, "TSPACK_ADOPT_REPORT_REQUIRED: adopt currently supports read-only --report only")
		os.Exit(1)
	}
	obs, err := adoption.Observe(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	report := adoption.BuildReport(obs)
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
	fmt.Println("Notes:")
	for _, warning := range report.Warnings {
		fmt.Printf("  - %s\n", warning)
	}
}

func emptyAsDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
