package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/project"
	"github.com/tspack/tspack/internal/testcmd"
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
	if args[0] == "check" || args[0] == "update" || args[0] == "sync" || args[0] == "pack" || args[0] == "why" {
		runCommand(args)
		return
	}
	if args[0] == "test" {
		runTestCommand(args)
		return
	}
	if args[0] == "artifact" {
		runArtifactCommand(args)
		return
	}
	if args[0] == "bench" {
		runBenchCommand(args)
		return
	}
	if args[0] == "doom" {
		runDoomCommand(args)
		return
	}
	if args[0] == "inspect" {
		runInspectCommand(args)
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
	fmt.Println("  tspack pack [--root .] [--out dir] [--package name] [--dry-run]")
	fmt.Println("  tspack why <query> [--root .] [--package name]")
	fmt.Println("  tspack test [--root .] [-xtest] [-vitest] [--list] [--filter text]")
	fmt.Println("  tspack artifact [--root .] [--out path] [--list] [--filter text] [--json]")
	fmt.Println("  tspack bench [--root .] [--list] [--filter text] [--json]")
	fmt.Println("  tspack doom [--root .] [--list] [--filter text] [--json] [--out path]")
	fmt.Println("  tspack inspect <url> [--url <url>] [--browser chromium] [--viewport WxH] [--selector css] [--point x,y] [--json] [--out file] [--text file]")
}

func runInspectCommand(args []string) {
	bridge := filepath.Join("manifest-frontend", "dist", "src", "inspect-cli.js")
	if _, err := os.Stat(bridge); err != nil {
		fmt.Fprintln(os.Stderr, "TSPACK_INSPECT_BRIDGE_MISSING: inspect bridge not found")
		os.Exit(1)
	}
	nodeArgs := []string{bridge, "inspect"}
	nodeArgs = append(nodeArgs, args[1:]...)
	cmd := exec.Command("node", nodeArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "TSPACK_INSPECT_FAILED: %v\n", err)
		os.Exit(1)
	}
}

func runDoomCommand(args []string) {
	root := "."
	out := ""
	list := false
	filter := ""
	jsonOut := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--root":
			i++
			root = args[i]
		case "--out":
			i++
			out = args[i]
		case "--list":
			list = true
		case "--filter":
			i++
			filter = args[i]
		case "--json":
			jsonOut = true
		default:
			fmt.Fprintf(os.Stderr, "unknown doom flag: %s\n", args[i])
			os.Exit(1)
		}
	}
	bridge := filepath.Join("manifest-frontend", "dist", "src", "native-test-cli.js")
	if _, err := os.Stat(bridge); err != nil {
		fmt.Fprintln(os.Stderr, "TSPACK_DOOM_BRIDGE_MISSING: native doom bridge not found")
		os.Exit(1)
	}
	nodeArgs := []string{bridge, "doom", "--root", root}
	if out != "" {
		nodeArgs = append(nodeArgs, "--out", out)
	}
	if list {
		nodeArgs = append(nodeArgs, "--list")
	}
	if filter != "" {
		nodeArgs = append(nodeArgs, "--filter", filter)
	}
	if jsonOut {
		nodeArgs = append(nodeArgs, "--json")
	}
	cmd := exec.Command("node", nodeArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "TSPACK_DOOM_FAILED: %v\n", err)
		os.Exit(1)
	}
}

func runBenchCommand(args []string) {
	root := "."
	list := false
	filter := ""
	jsonOut := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--root":
			i++
			root = args[i]
		case "--list":
			list = true
		case "--filter":
			i++
			filter = args[i]
		case "--json":
			jsonOut = true
		default:
			fmt.Fprintf(os.Stderr, "unknown bench flag: %s\n", args[i])
			os.Exit(1)
		}
	}
	bridge := filepath.Join("manifest-frontend", "dist", "src", "native-test-cli.js")
	if _, err := os.Stat(bridge); err != nil {
		fmt.Fprintln(os.Stderr, "TSPACK_BENCH_BRIDGE_MISSING: native benchmark bridge not found")
		os.Exit(1)
	}
	nodeArgs := []string{bridge, "bench", "--root", root}
	if list {
		nodeArgs = append(nodeArgs, "--list")
	}
	if filter != "" {
		nodeArgs = append(nodeArgs, "--filter", filter)
	}
	if jsonOut {
		nodeArgs = append(nodeArgs, "--json")
	}
	cmd := exec.Command("node", nodeArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "TSPACK_BENCH_FAILED: %v\n", err)
		os.Exit(1)
	}
}

func runArtifactCommand(args []string) {
	root := "."
	out := ""
	list := false
	filter := ""
	jsonOut := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--root":
			i++
			root = args[i]
		case "--out":
			i++
			out = args[i]
		case "--list":
			list = true
		case "--filter":
			i++
			filter = args[i]
		case "--json":
			jsonOut = true
		default:
			fmt.Fprintf(os.Stderr, "unknown artifact flag: %s\n", args[i])
			os.Exit(1)
		}
	}
	bridge := filepath.Join("manifest-frontend", "dist", "src", "native-test-cli.js")
	if _, err := os.Stat(bridge); err != nil {
		fmt.Fprintln(os.Stderr, "TSPACK_ARTIFACT_BRIDGE_MISSING: native artifact bridge not found")
		os.Exit(1)
	}
	nodeArgs := []string{bridge, "artifact", "--root", root}
	if out != "" {
		nodeArgs = append(nodeArgs, "--out", out)
	}
	if list {
		nodeArgs = append(nodeArgs, "--list")
	}
	if filter != "" {
		nodeArgs = append(nodeArgs, "--filter", filter)
	}
	if jsonOut {
		nodeArgs = append(nodeArgs, "--json")
	}
	cmd := exec.Command("node", nodeArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "TSPACK_ARTIFACT_FAILED: %v\n", err)
		os.Exit(1)
	}
}

func runTestCommand(args []string) {
	opts := testcmd.Options{RootDir: "."}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--root":
			i++
			opts.RootDir = args[i]
		case "-xtest", "--xtest":
			opts.UseXTest = true
		case "-vitest", "--vitest":
			opts.UseVitest = true
		case "--list":
			opts.List = true
		case "--filter":
			i++
			opts.Filter = args[i]
		default:
			fmt.Fprintf(os.Stderr, "unknown test flag: %s\n", args[i])
			os.Exit(1)
		}
	}
	result := testcmd.Run(opts)
	for _, d := range result.Diagnostics {
		fmt.Fprintf(os.Stderr, "%s: %s\n", d.Code, d.Message)
	}
	if result.ExitCode != 0 {
		os.Exit(result.ExitCode)
	}
}

func runCommand(args []string) {
	cmd := args[0]
	opts := project.DefaultOptions(".")
	clean := false
	packOpts := project.PackOptions{}
	whyOpts := project.WhyOptions{}
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
		case "--out":
			i++
			packOpts.OutputDir = args[i]
		case "--dry-run":
			packOpts.DryRun = true
		case "--package":
			i++
			packOpts.PackageName = args[i]
			whyOpts.PackageName = args[i]
		}
	}
	var result project.Result
	if cmd == "why" && len(args) > 1 {
		whyOpts.Query = args[1]
	}
	switch cmd {
	case "check":
		result = project.Check(opts)
	case "update":
		result = project.Update(opts)
	case "sync":
		result = project.Sync(opts, clean)
	case "pack":
		result = project.Pack(opts, packOpts)
	case "why":
		result = project.Why(opts, whyOpts)
	}
	for _, d := range result.Diagnostics {
		fmt.Fprintf(os.Stderr, "%s: %s\n", d.Code, d.Message)
	}
	if result.LockDiff != nil {
		fmt.Printf("lockfile diff: +%d -%d\n", len(result.LockDiff.PackagesAdded), len(result.LockDiff.PackagesRemoved))
	}
	if result.WhyResult != nil {
		for _, e := range result.WhyResult.Explanations {
			if e.MatchType == "dependency" {
				fmt.Printf("%s declared in package %q as %s\n", e.DependencyKey, e.PackageName, e.Kind)
			}
			if e.MatchType == "lock-package" {
				for _, lp := range e.LockPackages {
					fmt.Printf("lock package %s\n", lp.ID)
				}
			}
			if e.TargetName != "" {
				fmt.Printf("target %s in package %s\n", e.TargetName, e.PackageName)
			}
			if len(e.ReachableFrom) > 0 {
				fmt.Println("reachable from:")
				for _, r := range e.ReachableFrom {
					fmt.Printf("  %s:target:%s\n", r.PackageName, r.TargetName)
				}
			}
			if len(e.NotReachableFrom) > 0 {
				fmt.Println("not reachable from:")
				for _, r := range e.NotReachableFrom {
					fmt.Printf("  %s:target:%s\n", r.PackageName, r.TargetName)
				}
			}

			if len(e.LockEdges) > 0 {
				fmt.Println("lock edges:")
				for _, edge := range e.LockEdges {
					fmt.Printf("  %s -> %s %s\n", edge.From, edge.To, edge.Kind)
				}
			}
		}
	}
	if result.PackResult != nil {
		for _, a := range result.PackResult.Artifacts {
			fmt.Printf("packed %s@%s -> %s (%s)\n", a.PackageName, a.Version, a.Path, a.Hash)
		}
		if packOpts.DryRun {
			for _, f := range result.PackResult.Preview {
				fmt.Printf("%s %s <- %s\n", f.PackageName, f.ArchivePath, f.SourcePath)
			}
		}
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
