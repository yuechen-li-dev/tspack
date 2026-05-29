package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tspack/tspack/internal/check"
	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/how"
	"github.com/tspack/tspack/internal/manifest"
	"github.com/tspack/tspack/internal/project"
	"github.com/tspack/tspack/internal/testcmd"
)

type CheckJSONReport struct {
	Command      string                `json:"command"`
	OK           bool                  `json:"ok"`
	Root         string                `json:"root"`
	ManifestPath string                `json:"manifestPath,omitempty"`
	LockfilePath string                `json:"lockfilePath,omitempty"`
	Summary      CheckJSONSummary      `json:"summary"`
	Diagnostics  []CheckJSONDiagnostic `json:"diagnostics"`
}

type CheckJSONSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`
	Total    int `json:"total"`
}

type CheckJSONDiagnostic struct {
	Code     string      `json:"code"`
	Severity string      `json:"severity"`
	Message  string      `json:"message"`
	File     string      `json:"file,omitempty"`
	Details  interface{} `json:"details,omitempty"`
	Fixes    interface{} `json:"fixes,omitempty"`
}
type UpdateDryRunJSONReport struct {
	Command     string                         `json:"command"`
	DryRun      bool                           `json:"dryRun"`
	OK          bool                           `json:"ok"`
	Root        string                         `json:"root"`
	Targeted    bool                           `json:"targeted,omitempty"`
	Query       string                         `json:"query,omitempty"`
	Selected    []project.UpdateSelectedTarget `json:"selected,omitempty"`
	Summary     UpdateDryRunSummary            `json:"summary"`
	Changes     UpdateDryRunChanges            `json:"changes"`
	Diagnostics []CheckJSONDiagnostic          `json:"diagnostics"`
}
type UpdateDryRunSummary struct {
	Added     int `json:"added"`
	Removed   int `json:"removed"`
	Changed   int `json:"changed"`
	Unchanged int `json:"unchanged"`
}
type UpdateDryRunChanges struct {
	Added   []lockDiffPackage       `json:"added"`
	Removed []lockDiffPackage       `json:"removed"`
	Changed []lockDiffPackageChange `json:"changed"`
}
type lockDiffPackage struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"`
}
type lockDiffPackageChange struct {
	From lockDiffPackage `json:"from"`
	To   lockDiffPackage `json:"to"`
}

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
	if args[0] == "check" || args[0] == "update" || args[0] == "sync" || args[0] == "pack" || args[0] == "why" || args[0] == "outdated" {
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
	if args[0] == "run" {
		runRunCommand(args)
		return
	}

	if args[0] == "format" {
		runFormatCommand(args)
		return
	}

	if args[0] == "lint" {
		runLintCommand(args)
		return
	}

	if args[0] == "inspect" {
		runInspectCommand(args)
		return
	}
	if args[0] == "how" {
		runHowCommand(args)
		return
	}
	if args[0] == "doctor" {
		runDoctorCommand(args)
		return
	}
	if args[0] == "init" {
		runInitCommand(args)
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
	fmt.Println("  tspack check [--root .] [--json] [--explain <file>]")
	fmt.Println("  tspack update [query] [--root .] [--dry-run] [--json] [--quiet]")
	fmt.Println("  tspack sync [--root .] [--clean]")
	fmt.Println("  tspack pack [--root .] [--out dir] [--package name] [--dry-run]")
	fmt.Println("  tspack why <query> [--root .] [--package name]")
	fmt.Println("  tspack outdated [--root .] [--json]")
	fmt.Println("  tspack how <diagnostic-code> [--json]")
	fmt.Println("  tspack how --list [--json]")
	fmt.Println("  tspack test [--root .] [-xtest] [-vitest] [--list] [--filter text] [--compact] [--batch] [--update-snapshots] [--watch] [--xtest-bridge path]")
	fmt.Println("  tspack artifact [--root .] [--out path] [--list] [--filter text] [--json]")
	fmt.Println("  tspack bench [--root .] [--list] [--filter text] [--json]")
	fmt.Println("  tspack doom [--root .] [--list] [--filter text] [--json] [--out path]")
	fmt.Println("  tspack run [target] [--root .] [--manifest path] [--ready-timeout seconds] [--env KEY=VALUE] [--once]")
	fmt.Println("  tspack format [paths...] [--root .] [--check]")
	fmt.Println("  tspack lint [paths...] [--root .] [--fix]")
	fmt.Println("  tspack inspect <url> [experimental] [--run target] [--env KEY=VALUE] [--url <url>] [--browser auto|vscode|playwright-chromium|chromium|browser-path|host-path|cdp] [--host-path path] [--browser-path path] [--cdp endpoint] [--list-targets] [--target index-or-id] [--target-url substring] [--viewport WxH] [--selector css] [--point x,y] [--json] [--out file] [--text file]")
	fmt.Println("  tspack doctor [format|run|inspect] [--root .] [--json]")
	fmt.Println("  tspack init --kind <library|app> --name <package-name> [--version <version>] [--license <license>] [--force] [--dry-run]")
}

func runInspectCommand(args []string) {
	root := "."
	runTarget := ""
	runReadyTimeout := 30
	runEnv := runEnvOverlay{}
	positionalTarget := ""
	bridgeArgs := []string{}
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--root":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "TSPACK_INSPECT_INVALID_TARGET_OPTIONS: --root requires a value")
				os.Exit(1)
			}
			i++
			root = args[i]
			bridgeArgs = append(bridgeArgs, "--root", root)
		case "--run":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				fmt.Fprintln(os.Stderr, "TSPACK_INSPECT_RUN_TARGET_MISSING: --run requires a target name")
				os.Exit(1)
			}
			i++
			runTarget = args[i]
		case "--run-ready-timeout":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "TSPACK_INSPECT_INVALID_TARGET_OPTIONS: --run-ready-timeout requires a value")
				os.Exit(1)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				fmt.Fprintln(os.Stderr, "TSPACK_INSPECT_INVALID_TARGET_OPTIONS: --run-ready-timeout must be positive seconds")
				os.Exit(1)
			}
			runReadyTimeout = n
		case "--env":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(os.Stderr, "TSPACK_RUN_INVALID_ENV: --env requires KEY=VALUE")
				os.Exit(1)
			}
			i++
			var envErr *runErr
			runEnv, envErr = runEnv.WithAssignment(args[i])
			if envErr != nil {
				fmt.Fprintf(os.Stderr, "%s: %s\n", envErr.code, envErr.msg)
				os.Exit(1)
			}
		default:
			if !strings.HasPrefix(a, "-") && positionalTarget == "" {
				positionalTarget = a
			}
			bridgeArgs = append(bridgeArgs, a)
		}
	}
	if runTarget != "" && positionalTarget != "" && (strings.HasPrefix(positionalTarget, "http://") || strings.HasPrefix(positionalTarget, "https://")) {
		fmt.Fprintln(os.Stderr, "TSPACK_INSPECT_INVALID_TARGET_OPTIONS: cannot combine URL target with --run")
		os.Exit(1)
	}
	if runTarget == "" && positionalTarget != "" && !strings.HasPrefix(positionalTarget, "http://") && !strings.HasPrefix(positionalTarget, "https://") {
		runTarget = positionalTarget
		bridgeArgs = append([]string{}, bridgeArgs[1:]...)
	}
	if runTarget == "" && len(runEnv.Keys) > 0 {
		fmt.Fprintln(os.Stderr, "TSPACK_INSPECT_INVALID_TARGET_OPTIONS: --env requires --run or a run target name")
		os.Exit(1)
	}

	bridge := filepath.Join("manifest-frontend", "dist", "src", "inspect-cli.js")
	if _, err := os.Stat(bridge); err != nil {
		fmt.Fprintln(os.Stderr, "TSPACK_INSPECT_BRIDGE_MISSING: inspect bridge not found")
		os.Exit(1)
	}
	nodeArgs := []string{bridge, "inspect"}
	exitCode := 0
	exitMessage := ""
	if runTarget != "" {
		workspaceRoot := resolveWorkspaceRoot(root)
		manifestPath := filepath.Join(workspaceRoot, "manifest.tsx")
		ir := loadManifestPathForRun(workspaceRoot, manifestPath)
		ref, ok := findRunTargetRefByName(workspaceRoot, manifestPath, ir, runTarget)
		if !ok {
			fmt.Fprintf(os.Stderr, "TSPACK_INSPECT_RUN_TARGET_NOT_FOUND: %s\n", runTarget)
			os.Exit(1)
		}
		cwdPath, cwdErr := resolveRunTargetCwd(ref)
		if cwdErr != nil {
			fmt.Fprintf(os.Stderr, "%s: %s\n", cwdErr.code, cwdErr.msg)
			os.Exit(1)
		}
		rt := ref.Target
		fmt.Fprintf(os.Stderr, "Starting run target %q...\n", runTarget)
		fmt.Fprintf(os.Stderr, "Cwd: %s (%s)\n", effectiveRunTargetCwd(rt), cwdPath)
		if len(runEnv.Keys) > 0 {
			fmt.Fprintf(os.Stderr, "Env: %s\n", strings.Join(runEnv.Keys, ", "))
		}
		session, readyErr := startRunTargetInDir(workspaceRoot, cwdPath, rt, time.Duration(runReadyTimeout)*time.Second, os.Stderr, os.Stderr, runEnv)
		if readyErr != nil {
			code := "TSPACK_INSPECT_RUN_START_FAILED"
			switch readyErr.code {
			case "TSPACK_RUN_READY_TIMEOUT":
				code = "TSPACK_INSPECT_RUN_READY_TIMEOUT"
			case "TSPACK_RUN_PROCESS_EXITED_EARLY":
				code = "TSPACK_INSPECT_RUN_EXITED_EARLY"
			}
			fmt.Fprintf(os.Stderr, "%s: %s\n", code, readyErr.msg)
			os.Exit(1)
		}
		defer func() {
			if err := session.Stop(); err != nil {
				fmt.Fprintf(os.Stderr, "TSPACK_INSPECT_RUN_SHUTDOWN_FAILED: %v\n", err)
			}
			fmt.Fprintf(os.Stderr, "Stopped run target %q.\n", runTarget)
			if exitCode != 0 {
				if exitMessage != "" {
					fmt.Fprintln(os.Stderr, exitMessage)
				}
				os.Exit(exitCode)
			}
		}()
		fmt.Fprintf(os.Stderr, "Ready: %s\n", session.URL)
		fmt.Fprintf(os.Stderr, "Inspecting: %s\n", session.URL)
		nodeArgs = append(nodeArgs, session.URL)
		nodeArgs = append(nodeArgs, bridgeArgs...)
	} else {
		nodeArgs = append(nodeArgs, args[1:]...)
	}
	cmd := exec.Command("node", nodeArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			if runTarget != "" {
				return
			}
			os.Exit(exitCode)
		}
		exitMessage = fmt.Sprintf("TSPACK_INSPECT_FAILED: %v", err)
		exitCode = 1
		if runTarget != "" {
			return
		}
		fmt.Fprintln(os.Stderr, exitMessage)
		os.Exit(exitCode)
	}
}

func findRunTargetRefByName(root string, manifestPath string, ir *manifest.ManifestIR, name string) (runTargetRef, bool) {
	for _, ref := range collectRunTargets(root, manifestPath, ir, "") {
		if ref.Target.Name == name {
			return ref, true
		}
	}
	return runTargetRef{}, false
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

func nextTestFlagValue(args []string, index *int, flag string) (string, bool) {
	if *index+1 >= len(args) {
		fmt.Fprintf(os.Stderr, "missing value for test flag: %s\n", flag)
		os.Exit(1)
		return "", false
	}
	*index = *index + 1
	return args[*index], true
}

func runTestCommand(args []string) {
	opts := testcmd.Options{RootDir: "."}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--root":
			value, ok := nextTestFlagValue(args, &i, "--root")
			if !ok {
				return
			}
			opts.RootDir = value
		case "-xtest", "--xtest":
			opts.UseXTest = true
		case "-vitest", "--vitest":
			opts.UseVitest = true
		case "--list":
			opts.List = true
		case "--compact":
			opts.Compact = true
		case "--batch":
			opts.Batch = true
		case "--watch":
			opts.Watch = true
		case "--update-snapshots":
			opts.UpdateSnapshots = true
		case "--json":
			opts.JSON = true
		case "--filter":
			value, ok := nextTestFlagValue(args, &i, "--filter")
			if !ok {
				return
			}
			opts.Filter = value
		case "--xtest-bridge", "--bridge":
			value, ok := nextTestFlagValue(args, &i, args[i])
			if !ok {
				return
			}
			opts.XTestBridge = value
		default:
			fmt.Fprintf(os.Stderr, "unknown test flag: %s\n", args[i])
			os.Exit(1)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	result := testcmd.RunContext(ctx, opts)
	for _, d := range result.Diagnostics {
		fmt.Fprintf(os.Stderr, "%s: %s\n", d.Code, d.Message)
		for _, detail := range d.Details {
			if detail == d.Message {
				continue
			}
			fmt.Fprintf(os.Stderr, "  %s\n", detail)
		}
	}
	if result.ExitCode != 0 {
		os.Exit(result.ExitCode)
	}
}

func runCommand(args []string) {
	cmd := args[0]
	opts := project.DefaultOptions(".")
	manifestExplicit := false
	lockfileExplicit := false
	storeExplicit := false
	jsonOutput := false
	explainFile := ""
	explainSet := false
	checkPositionals := []string{}
	clean := false
	updateDryRun := false
	updateQuiet := false
	updateQuery := ""
	packOpts := project.PackOptions{}
	whyOpts := project.WhyOptions{}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--root":
			i++
			opts.RootDir = args[i]
			if !manifestExplicit {
				opts.ManifestPath = filepath.Join(opts.RootDir, "manifest.tsx")
			}
			if !lockfileExplicit {
				opts.LockfilePath = filepath.Join(opts.RootDir, "ts-lock.toml")
			}
			if !storeExplicit {
				opts.StoreRoot = filepath.Join(opts.RootDir, ".tspack", "store")
			}
		case "--manifest":
			i++
			opts.ManifestPath = args[i]
			manifestExplicit = true
		case "--lockfile":
			i++
			opts.LockfilePath = args[i]
			lockfileExplicit = true
		case "--store":
			i++
			opts.StoreRoot = args[i]
			storeExplicit = true
		case "--clean":
			clean = true
		case "--out":
			i++
			packOpts.OutputDir = args[i]
		case "--quiet":
			if cmd == "update" {
				updateQuiet = true
				continue
			}
			fmt.Fprintf(os.Stderr, "unknown %s flag: --quiet\n", cmd)
			os.Exit(1)
		case "--dry-run":
			if cmd == "pack" {
				packOpts.DryRun = true
				continue
			}
			if cmd == "update" {
				updateDryRun = true
				continue
			}
			fmt.Fprintf(os.Stderr, "unknown %s flag: --dry-run\n", cmd)
			os.Exit(1)
		case "--package":
			i++
			packOpts.PackageName = args[i]
			whyOpts.PackageName = args[i]
		case "--json":
			jsonOutput = true
		case "--explain":
			if cmd != "check" {
				fmt.Fprintf(os.Stderr, "unknown %s flag: --explain\n", cmd)
				os.Exit(1)
			}
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				fmt.Fprintln(os.Stderr, "TSPACK_CHECK_EXPLAIN_FILE_REQUIRED: --explain requires exactly one file path")
				os.Exit(1)
			}
			i++
			if explainSet {
				fmt.Fprintln(os.Stderr, "TSPACK_CHECK_EXPLAIN_FILE_REQUIRED: --explain requires exactly one file path")
				os.Exit(1)
			}
			explainSet = true
			explainFile = args[i]
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(os.Stderr, "unknown %s flag: %s\n", cmd, args[i])
				os.Exit(1)
			}
			if cmd == "check" {
				checkPositionals = append(checkPositionals, args[i])
			}
			if cmd == "update" {
				if updateQuery != "" {
					fmt.Fprintln(os.Stderr, "update accepts at most one query")
					os.Exit(1)
				}
				updateQuery = args[i]
			}
		}
	}
	var result project.Result
	if cmd == "why" && len(args) > 1 {
		whyOpts.Query = args[1]
	}
	if cmd == "check" && explainSet && len(checkPositionals) > 0 {
		fmt.Fprintln(os.Stderr, "TSPACK_CHECK_EXPLAIN_FILE_REQUIRED: --explain requires exactly one file path")
		os.Exit(1)
	}
	if cmd == "check" && explainSet {
		result = project.CheckExplain(opts, explainFile)
		if jsonOutput && result.Explain != nil {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(result.Explain); err != nil {
				fmt.Fprintf(os.Stderr, "TSPACK_CHECK_EXPLAIN_FAILED: %v\n", err)
				os.Exit(1)
			}
			if hasErrors(result.Diagnostics) {
				os.Exit(1)
			}
			return
		}
		if result.Explain != nil {
			printCheckExplain(result.Explain)
			if hasErrors(result.Diagnostics) {
				os.Exit(1)
			}
			return
		}
		for _, d := range result.Diagnostics {
			fmt.Fprintf(os.Stderr, "%s: %s\n", d.Code, d.Message)
		}
		os.Exit(1)
	}
	if cmd == "update" && !updateQuiet && !jsonOutput {
		opts.Progress = project.Progress{Enabled: true, Writer: os.Stderr}
	}
	updateOptions := project.UpdateOptions{Query: updateQuery}
	switch cmd {
	case "check":
		result = project.Check(opts)
	case "update":
		if updateDryRun {
			result = project.UpdateDryRunWithOptions(opts, updateOptions)
		} else {
			result = project.UpdateWithOptions(opts, updateOptions)
		}
	case "sync":
		result = project.Sync(opts, clean)
	case "pack":
		result = project.Pack(opts, packOpts)
	case "why":
		result = project.Why(opts, whyOpts)
	case "outdated":
		result = project.Outdated(opts)
	}
	if cmd == "outdated" && jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		report := map[string]any{
			"command":      "outdated",
			"ok":           !hasErrors(result.Diagnostics),
			"root":         opts.RootDir,
			"summary":      result.Outdated.Summary,
			"dependencies": result.Outdated.Dependencies,
			"diagnostics":  buildCheckJSONReport(opts, result).Diagnostics,
		}
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "TSPACK_OUTDATED_JSON_ENCODE_FAILED: %v\n", err)
			os.Exit(1)
		}
		if hasErrors(result.Diagnostics) {
			os.Exit(1)
		}
		return
	}
	if cmd == "update" && updateDryRun && jsonOutput {
		report := buildUpdateDryRunJSONReport(opts, result)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "TSPACK_UPDATE_JSON_ENCODE_FAILED: %v\n", err)
			os.Exit(1)
		}
		if hasErrors(result.Diagnostics) {
			os.Exit(1)
		}
		return
	}
	if cmd == "check" && jsonOutput {
		report := buildCheckJSONReport(opts, result)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "TSPACK_CHECK_JSON_ENCODE_FAILED: %v\n", err)
			os.Exit(1)
		}
		if hasErrors(result.Diagnostics) {
			os.Exit(1)
		}
		return
	}
	for _, d := range result.Diagnostics {
		fmt.Fprintf(os.Stderr, "%s: %s\n", d.Code, d.Message)
		for _, detail := range d.Details {
			if detail == d.Message {
				continue
			}
			fmt.Fprintf(os.Stderr, "  %s\n", detail)
		}
	}
	if result.LockDiff != nil {
		if cmd == "update" && updateDryRun {
			printUpdateDryRunPlan(result)
		} else {
			fmt.Printf("lockfile diff: +%d -%d\n", len(result.LockDiff.PackagesAdded), len(result.LockDiff.PackagesRemoved))
		}
	}
	if result.WhyResult != nil {
		printedLockEdges := map[string]bool{}
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
				lines := make([]string, 0, len(e.LockEdges))
				for _, edge := range e.LockEdges {
					key := edge.From + "|" + edge.To + "|" + edge.Kind
					if edge.Optional {
						key += "|optional"
					}
					if printedLockEdges[key] {
						continue
					}
					printedLockEdges[key] = true
					lines = append(lines, fmt.Sprintf("  %s -> %s %s", edge.From, edge.To, edge.Kind))
				}
				if len(lines) > 0 {
					fmt.Println("lock edges:")
					for _, line := range lines {
						fmt.Println(line)
					}
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
	if result.Outdated != nil {
		fmt.Println("TSPack outdated")
		fmt.Println()
		for _, dep := range result.Outdated.Dependencies {
			fmt.Println(dep.Name)
			fmt.Printf("  kind: %s\n", dep.Kind)
			fmt.Printf("  requested: %s\n", dep.Requested)
			if len(dep.Current) == 0 {
				fmt.Println("  current: -")
			} else {
				fmt.Printf("  current: %s\n", strings.Join(dep.Current, ", "))
			}
			fmt.Printf("  wanted: %s\n", dep.Wanted)
			fmt.Printf("  latest: %s\n", dep.Latest)
			fmt.Printf("  status: %s\n", strings.ReplaceAll(dep.Status, "_", " "))
		}
		fmt.Println()
		fmt.Println("Summary:")
		fmt.Printf("  current: %d\n", result.Outdated.Summary.Current)
		fmt.Printf("  outdated: %d\n", result.Outdated.Summary.Outdated)
		fmt.Printf("  skipped: %d\n", result.Outdated.Summary.Skipped)
		fmt.Printf("  errors: %d\n", result.Outdated.Summary.Errors)
	}
	if hasErrors(result.Diagnostics) {
		if cmd == "pack" {
			fmt.Fprintln(os.Stderr, "pack failed; no artifacts were written")
		}
		os.Exit(1)
	}
}

func printUpdateDryRunPlan(result project.Result) {
	fmt.Println("TSPack update dry run")
	fmt.Println()
	if result.UpdateTarget != nil && result.UpdateTarget.Targeted {
		fmt.Println("Target:")
		fmt.Printf("  %s\n", result.UpdateTarget.Query)
		fmt.Println()
	}
	fmt.Println("Lockfile changes:")
	diff := result.LockDiff
	if diff == nil || (len(diff.PackagesAdded) == 0 && len(diff.PackagesRemoved) == 0 && len(diff.PackagesChanged) == 0) {
		fmt.Println("  none")
		fmt.Println()
		fmt.Println("No files were written.")
		return
	}
	if len(diff.PackagesAdded) > 0 {
		fmt.Println("  added:")
		for _, pkg := range diff.PackagesAdded {
			fmt.Printf("    %s\n", pkg.ID)
		}
	}
	if len(diff.PackagesChanged) > 0 {
		fmt.Println("  changed:")
		for _, ch := range diff.PackagesChanged {
			fmt.Printf("    %s -> %s\n", ch.Old.ID, ch.New.ID)
		}
	}
	if len(diff.PackagesRemoved) > 0 {
		fmt.Println("  removed:")
		for _, pkg := range diff.PackagesRemoved {
			fmt.Printf("    %s\n", pkg.ID)
		}
	}
	fmt.Println()
	fmt.Println("No files were written.")
}

func buildUpdateDryRunJSONReport(opts project.Options, result project.Result) UpdateDryRunJSONReport {
	report := UpdateDryRunJSONReport{
		Command: "update",
		DryRun:  true,
		OK:      !hasErrors(result.Diagnostics),
		Root:    opts.RootDir,
	}
	if result.UpdateTarget != nil {
		report.Targeted = result.UpdateTarget.Targeted
		report.Query = result.UpdateTarget.Query
		report.Selected = result.UpdateTarget.Selected
	}
	if result.DryRun != nil {
		report.Summary = UpdateDryRunSummary(result.DryRun.Summary)
	}
	if result.LockDiff != nil {
		for _, pkg := range result.LockDiff.PackagesAdded {
			report.Changes.Added = append(report.Changes.Added, lockDiffPackage{ID: pkg.ID, Name: pkg.Name, Version: pkg.Version, Source: pkg.Source})
		}
		for _, pkg := range result.LockDiff.PackagesRemoved {
			report.Changes.Removed = append(report.Changes.Removed, lockDiffPackage{ID: pkg.ID, Name: pkg.Name, Version: pkg.Version, Source: pkg.Source})
		}
		for _, ch := range result.LockDiff.PackagesChanged {
			report.Changes.Changed = append(report.Changes.Changed, lockDiffPackageChange{
				From: lockDiffPackage{ID: ch.Old.ID, Name: ch.Old.Name, Version: ch.Old.Version, Source: ch.Old.Source},
				To:   lockDiffPackage{ID: ch.New.ID, Name: ch.New.Name, Version: ch.New.Version, Source: ch.New.Source},
			})
		}
	}
	diags := append([]diag.Diagnostic(nil), result.Diagnostics...)
	diag.SortDiagnostics(diags)
	for _, d := range diags {
		report.Diagnostics = append(report.Diagnostics, CheckJSONDiagnostic{
			Code:     d.Code,
			Severity: string(d.Severity),
			Message:  d.Message,
			Details:  d.Details,
		})
	}
	return report
}

func buildCheckJSONReport(opts project.Options, result project.Result) CheckJSONReport {
	diags := append([]diag.Diagnostic(nil), result.Diagnostics...)
	diag.SortDiagnostics(diags)
	summary := CheckJSONSummary{}
	jsonDiagnostics := make([]CheckJSONDiagnostic, 0, len(diags))
	for _, d := range diags {
		switch d.Severity {
		case diag.SeverityError:
			summary.Errors++
		case diag.SeverityWarning:
			summary.Warnings++
		default:
			summary.Info++
		}
		summary.Total++
		jd := CheckJSONDiagnostic{
			Code:     d.Code,
			Severity: string(d.Severity),
			Message:  d.Message,
		}
		if d.File != "" {
			jd.File = d.File
		}
		if len(d.Details) > 0 {
			jd.Details = d.Details
		}
		if len(d.Fixes) > 0 {
			jd.Fixes = d.Fixes
		}
		jsonDiagnostics = append(jsonDiagnostics, jd)
	}
	return CheckJSONReport{
		Command:      "check",
		OK:           summary.Errors == 0,
		Root:         opts.RootDir,
		ManifestPath: opts.ManifestPath,
		LockfilePath: opts.LockfilePath,
		Summary:      summary,
		Diagnostics:  jsonDiagnostics,
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

func runHowCommand(args []string) {
	list := false
	jsonOutput := false
	positionals := []string{}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--list":
			list = true
		case "--json":
			jsonOutput = true
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(os.Stderr, "TSPACK_HOW_INVALID_ARGS: unknown flag %s\n", args[i])
				os.Exit(1)
			}
			positionals = append(positionals, args[i])
		}
	}
	if list {
		items := how.List()
		if jsonOutput {
			type listEntry struct {
				Code  string `json:"code"`
				Title string `json:"title"`
			}
			resp := struct {
				Codes []listEntry `json:"codes"`
			}{Codes: make([]listEntry, 0, len(items))}
			for _, item := range items {
				resp.Codes = append(resp.Codes, listEntry{Code: item.Code, Title: item.Title})
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(resp)
			return
		}
		fmt.Println("Known diagnostic help entries:")
		fmt.Println()
		for _, item := range items {
			fmt.Printf("  %-40s %s\n", item.Code, item.Title)
		}
		return
	}
	if len(positionals) == 0 {
		fmt.Fprintln(os.Stderr, "TSPACK_HOW_CODE_REQUIRED: diagnostic code is required (or use --list)")
		os.Exit(1)
	}
	if len(positionals) > 1 {
		fmt.Fprintln(os.Stderr, "TSPACK_HOW_INVALID_ARGS: expected exactly one diagnostic code")
		os.Exit(1)
	}
	entry, ok := how.Lookup(positionals[0])
	if !ok {
		fmt.Fprintf(os.Stderr, "TSPACK_HOW_CODE_NOT_FOUND: unknown diagnostic code %s (run: tspack how --list)\n", positionals[0])
		os.Exit(1)
	}
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(entry)
		return
	}
	fmt.Println(entry.Code)
	fmt.Println()
	fmt.Println(entry.Title)
	fmt.Println()
	fmt.Println("What it means:")
	fmt.Printf("  %s\n", entry.Summary)
	fmt.Println()
	fmt.Println("Why TSPack cares:")
	fmt.Printf("  %s\n", entry.Why)
	if len(entry.CommonCauses) > 0 {
		fmt.Println()
		fmt.Println("Common causes:")
		for _, cause := range entry.CommonCauses {
			fmt.Printf("  - %s\n", cause)
		}
	}
	if len(entry.Fixes) > 0 {
		fmt.Println()
		fmt.Println("How to fix:")
		for _, fix := range entry.Fixes {
			fmt.Printf("  - %s\n", fix)
		}
	}
	if len(entry.BadExamples) > 0 {
		fmt.Println()
		fmt.Println("Bad examples:")
		for _, e := range entry.BadExamples {
			fmt.Printf("  %s:\n%s\n", e.Label, e.Text)
		}
	}
	if len(entry.GoodExamples) > 0 {
		fmt.Println()
		fmt.Println("Good examples:")
		for _, e := range entry.GoodExamples {
			fmt.Printf("  %s:\n%s\n", e.Label, e.Text)
		}
	}
}

func printCheckExplain(explain *check.ExplainResult) {
	fmt.Printf("Boundary explanation for %s\n\n", explain.File)
	fmt.Println("Reachable from targets:")
	if len(explain.ReachableFrom) == 0 {
		fmt.Println("  none")
	} else {
		for _, r := range explain.ReachableFrom {
			fmt.Printf("  %s\n", r.Target)
			fmt.Printf("    path: %s\n", strings.Join(r.Path, " -> "))
		}
	}
	fmt.Println()
	fmt.Println("Matched boundary rules:")
	if len(explain.MatchedRules) == 0 {
		fmt.Println("  none")
	} else {
		for _, rule := range explain.MatchedRules {
			if rule.TransitiveFrom != "" {
				fmt.Printf("  transitiveFrom: %s\n", rule.TransitiveFrom)
				fmt.Printf("    seed: %s\n", rule.Seed)
				if len(rule.Path) > 0 {
					fmt.Printf("    path: %s\n", strings.Join(rule.Path, " -> "))
				}
			} else {
				fmt.Printf("  from: %s\n", rule.From)
			}
			if len(rule.AllowDeps) > 0 {
				fmt.Printf("    allowDeps: %s\n", strings.Join(rule.AllowDeps, ", "))
			}
			if len(rule.DenyDeps) > 0 {
				fmt.Printf("    denyDeps: %s\n", strings.Join(rule.DenyDeps, ", "))
			}
			if len(rule.DenyTypeDeps) > 0 {
				fmt.Printf("    denyTypeDeps: %s\n", strings.Join(rule.DenyTypeDeps, ", "))
			}
			if rule.AllowOnly != nil {
				fmt.Printf("    allowOnly: %s\n", strings.Join(rule.AllowOnly, ", "))
			}
		}
	}
	fmt.Println()
	fmt.Println("External imports:")
	hasExternal := false
	for _, imp := range explain.Imports {
		if imp.Kind != "external" || imp.TypeOnly {
			continue
		}
		hasExternal = true
		fmt.Printf("  %s\n", imp.Specifier)
		fmt.Printf("    decision: %s\n", imp.Decision)
		for _, reason := range imp.Reasons {
			fmt.Printf("    reason: %s\n", reason)
		}
		if imp.Diagnostic != "" {
			fmt.Printf("    diagnostic: %s\n", imp.Diagnostic)
		}
	}
	if !hasExternal {
		fmt.Println("  none")
	}
	fmt.Println()
	fmt.Println("Type imports:")
	hasTypeExternal := false
	for _, imp := range explain.Imports {
		if imp.Kind != "external" || !imp.TypeOnly {
			continue
		}
		hasTypeExternal = true
		fmt.Printf("  %s\n", imp.Specifier)
		fmt.Printf("    decision: %s\n", imp.Decision)
		for _, reason := range imp.Reasons {
			fmt.Printf("    reason: %s\n", reason)
		}
		if imp.Diagnostic != "" {
			fmt.Printf("    diagnostic: %s\n", imp.Diagnostic)
		}
	}
	if !hasTypeExternal {
		fmt.Println("  none")
	}
	fmt.Println()
	fmt.Println("Relative imports:")
	hasRelative := false
	for _, imp := range explain.Imports {
		if imp.Kind != "relative" {
			continue
		}
		hasRelative = true
		fmt.Printf("  %s\n", imp.Specifier)
		if imp.Resolved != "" {
			fmt.Printf("    resolved: %s\n", imp.Resolved)
		} else {
			fmt.Println("    resolved: unresolved")
		}
	}
	if !hasRelative {
		fmt.Println("  none")
	}
	if len(explain.Notes) > 0 {
		fmt.Println()
		fmt.Println("Notes:")
		for _, note := range explain.Notes {
			fmt.Printf("  %s\n", note)
		}
	}
}
