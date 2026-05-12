package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/project"
)

const version = "tspack 0.0.0-dev"

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printHelp()
		return
	}

	if args[0] == "--version" || args[0] == "version" || args[0] == "-v" {
		fmt.Println(version)
		return
	}
	if args[0] == "check" || args[0] == "update" || args[0] == "sync" {
		runCommand(args)
		return
	}

	fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
	printHelp()
	os.Exit(1)
}

func printHelp() {
	fmt.Println("tspack - TypeScript-first package manager (M0 scaffold)")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  tspack help")
	fmt.Println("  tspack --version")
	fmt.Println("  tspack check [--root .]")
	fmt.Println("  tspack update [--root .]")
	fmt.Println("  tspack sync [--root .] [--clean]")
}

func runCommand(args []string) {
	cmd := args[0]
	opts := project.DefaultOptions(".")
	clean := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--root":
			i++
			opts.RootDir = args[i]
			if opts.ManifestPath == "./manifest.tsx" {
				opts.ManifestPath = filepath.Join(opts.RootDir, "manifest.tsx")
				opts.LockfilePath = filepath.Join(opts.RootDir, "ts-lock.toml")
				opts.StoreRoot = filepath.Join(opts.RootDir, ".tspack", "store")
			}
		case "--manifest":
			i++
			opts.ManifestPath = args[i]
		case "--lockfile":
			i++
			opts.LockfilePath = args[i]
		case "--store":
			i++
			opts.StoreRoot = args[i]
		case "--clean":
			clean = true
		}
	}
	var result project.Result
	switch cmd {
	case "check":
		result = project.Check(opts)
	case "update":
		result = project.Update(opts)
	case "sync":
		result = project.Sync(opts, clean)
	}
	for _, d := range result.Diagnostics {
		fmt.Fprintf(os.Stderr, "%s: %s\n", d.Code, d.Message)
	}
	if result.LockDiff != nil {
		fmt.Printf("lockfile diff: +%d -%d\n", len(result.LockDiff.PackagesAdded), len(result.LockDiff.PackagesRemoved))
	}
	if hasErrors(result.Diagnostics) {
		os.Exit(1)
	}
}

func hasErrors(diags []diag.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return true
		}
	}
	return false
}
