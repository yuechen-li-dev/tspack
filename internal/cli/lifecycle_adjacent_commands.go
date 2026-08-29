package cli

import (
	"context"
	"errors"
	"fmt"
	compatplan "github.com/yuechen-li-dev/tspack/internal/compat"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/nodecmd"
	"github.com/yuechen-li-dev/tspack/internal/npmbridge"
	"github.com/yuechen-li-dev/tspack/internal/project"
	"github.com/yuechen-li-dev/tspack/internal/testcmd"
	"github.com/yuechen-li-dev/tspack/internal/version"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func runNpmCommand(args []string) {
	root := "."
	npmArgs := []string{}

	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--root" {
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "--root requires a value")
				exit(2)
			}
			root = args[i]
			continue
		}
		npmArgs = append(npmArgs, arg)
	}

	if len(npmArgs) == 0 {
		fmt.Fprintln(os.Stderr, "usage: tspack npm <npm-args...>")
		fmt.Fprintln(os.Stderr, "examples: install, ci, update, exec vite -- --version")
		exit(2)
	}

	root = resolveWorkspaceRoot(root)
	result, err := npmbridge.Run(npmbridge.Options{
		Cwd:    root,
		Args:   npmArgs,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	if err != nil {
		var notFound npmbridge.NotFoundError
		if errors.As(err, &notFound) {
			fmt.Fprintf(os.Stderr, "TSPACK_NPM_NOT_FOUND: %v\n", notFound)
			exit(127)
		}
		fmt.Fprintf(os.Stderr, "TSPACK_NPM_FAILED: %v\n", err)
		exit(1)
	}
	if result.ExitCode != 0 {
		exit(result.ExitCode)
	}
	fmt.Fprintln(os.Stderr, "TSPack: npm completed. Run `tspack adopt --report` to inspect the package.json-native project state.")
}

func runCompatCommand(args []string) {
	if len(args) >= 2 && (args[1] == "--help" || args[1] == "-h" || args[1] == "help") {
		printCommandHelp("compat")
		return
	}
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: tspack compat list|diff|write [--root .]")
		exit(2)
	}
	subcommand := args[1]
	root := "."
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--root":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "--root requires a value")
				exit(2)
			}
			root = args[i]
		default:
			fmt.Fprintf(os.Stderr, "unknown compat flag: %s\n", args[i])
			exit(2)
		}
	}
	if subcommand != "list" && subcommand != "diff" && subcommand != "write" {
		fmt.Fprintf(os.Stderr, "unknown compat subcommand: %s\n", subcommand)
		exit(2)
	}
	root = resolveWorkspaceRoot(root)
	ir := loadManifestPathForRun(root, filepath.Join(root, "manifest.tsx"))
	statuses, err := compatplan.Plan(root, ir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TSPACK_COMPAT_FAILED: %v\n", err)
		exit(1)
	}
	if len(statuses) == 0 {
		fmt.Println("No compatibility files declared.")
		return
	}
	hasDrift := false
	for _, status := range statuses {
		if status.State != compatplan.StateClean {
			hasDrift = true
		}
	}
	switch subcommand {
	case "list":
		for _, status := range statuses {
			fmt.Printf("%s %s %s\n", status.State, status.Format, status.Path)
		}
	case "diff":
		for _, status := range statuses {
			fmt.Printf("%s %s\n", status.State, status.Path)
			if status.State == compatplan.StateMissing {
				fmt.Printf("--- %s (missing)\n+++ %s (desired)\n%s", status.Path, status.Path, string(status.Desired))
			}
			if status.State == compatplan.StateDrifted {
				fmt.Printf("--- %s (existing sha256 %s)\n+++ %s (desired sha256 %s)\n", status.Path, status.ExistingHash, status.Path, status.DesiredHash)
			}
		}
		if hasDrift {
			exit(1)
		}
	case "write":
		if err := compatplan.Write(root, statuses); err != nil {
			fmt.Fprintf(os.Stderr, "TSPACK_COMPAT_WRITE_FAILED: %v\n", err)
			exit(1)
		}
		for _, status := range statuses {
			if status.State == compatplan.StateClean {
				fmt.Printf("up-to-date %s\n", status.Path)
			} else {
				fmt.Printf("written %s\n", status.Path)
			}
		}
	}
}

func printVersion() {
	fmt.Printf("tspack %s\n", version.Version)
	fmt.Printf("commit %s\n", version.Commit)
	fmt.Printf("built %s\n", version.Date)
}

func printLegacyHelp() {
	fmt.Println("tspack - TypeScript-first project/package manager")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  tspack help")
	fmt.Println("  tspack --version")
	fmt.Println("  tspack check [--root .] [--json] [--format] [--explain <file>] [--show-conflicts] [--show-lifecycle]")
	fmt.Println("  tspack update [query] [--root .] [--dry-run] [--json] [--quiet]")
	fmt.Println("  tspack sync [--root .] [--clean] [--force]")
	fmt.Println("  tspack pack [--root .] [--out dir] [--package name] [--dry-run] [--verify]")
	fmt.Println("  tspack why [--reverse] <query> [--root .] [--package name]")
	fmt.Println("  tspack outdated [--root .] [--json]")
	fmt.Println("  tspack how <diagnostic-code> [--json]")
	fmt.Println("  tspack how --list [--json]")
	fmt.Println("  tspack test [--root .] [-xtest] [-vitest] [--run target] [--run-ready-timeout seconds] [--env KEY=VALUE] [--list] [--filter text] [--compact] [--batch] [--update-snapshots] [--watch] [--xtest-bridge path]")
	fmt.Println("  tspack artifact [--root .] [--out path] [--list] [--filter text] [--json]")
	fmt.Println("  tspack bench [--root .] [--list] [--filter text] [--json]")
	fmt.Println("  tspack doom [--root .] [--list] [--filter text] [--json] [--out path]")
	fmt.Println("  tspack run [target] [--root .] [--manifest path] [--ready-timeout seconds] [--env KEY=VALUE] [--once] [--preflight-only]")
	fmt.Println("  tspack scenario <scenario.json> --run <RunTarget> [--root .] [--ready-timeout seconds]")
	fmt.Println("  tspack npm <npm-args...> [--root .]")
	fmt.Println("  tspack format [paths...] [--root .] [--check]")
	fmt.Println("  tspack lint [paths...] [--root .] [--fix] [--unsafe]")
	fmt.Println("  tspack inspect <url> [experimental] [--run target] [--env KEY=VALUE] [--url <url>] [--browser auto|vscode|playwright-chromium|chromium|browser-path|host-path|cdp] [--host-path path] [--browser-path path] [--cdp endpoint] [--list-targets] [--target index-or-id] [--target-url substring] [--viewport WxH] [--selector css] [--point x,y] [--json] [--out file] [--text file] [--bundle] [--bundle-output file] [--watch] [--watch-debounce milliseconds] [--verbose]")
	fmt.Println("  tspack doctor [format|run|runtime|inspect|security] [--root .] [--json]")
	fmt.Println("  tspack init --kind <library|app> --name <package-name> [--version <version>] [--license <license>] [--force] [--dry-run]")
	fmt.Println("  tspack migrate [--check] [--write] [--root .] [--package-json path] [--package-lock path] [--no-lock-evidence] [--scan-source] [--no-source-scan] [--out-manifest path] [--out-report path] [--force]")
	fmt.Println()
	fmt.Println("Check flags:")
	fmt.Println("  --format           Run read-only format check as part of check")
	fmt.Println("  --show-conflicts   Show individual version conflict diagnostics instead of summary")
	fmt.Println("  --show-lifecycle   Show individual lifecycle script diagnostics instead of summary")
}

func printCheckHelp() {
	printCommandHelp("check")
}

func printInspectHelp() {
	printCommandHelp("inspect")
}

func runInspectCommand(args []string) {
	for _, arg := range args[1:] {
		if arg == "--help" || arg == "-h" || arg == "help" {
			printInspectHelp()
			return
		}
	}

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
				exit(1)
			}
			i++
			root = args[i]
			bridgeArgs = append(bridgeArgs, "--root", root)
		case "--run":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				fmt.Fprintln(os.Stderr, "TSPACK_INSPECT_RUN_TARGET_MISSING: --run requires a target name")
				exit(1)
			}
			i++
			runTarget = args[i]
		case "--run-ready-timeout":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "TSPACK_INSPECT_INVALID_TARGET_OPTIONS: --run-ready-timeout requires a value")
				exit(1)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				fmt.Fprintln(os.Stderr, "TSPACK_INSPECT_INVALID_TARGET_OPTIONS: --run-ready-timeout must be positive seconds")
				exit(1)
			}
			runReadyTimeout = n
		case "--env":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(os.Stderr, "TSPACK_RUN_INVALID_ENV: --env requires KEY=VALUE")
				exit(1)
			}
			i++
			var envErr *runErr
			runEnv, envErr = runEnv.WithAssignment(args[i])
			if envErr != nil {
				fmt.Fprintf(os.Stderr, "%s: %s\n", envErr.code, envErr.msg)
				exit(1)
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
		exit(1)
	}
	if runTarget == "" && positionalTarget != "" && !strings.HasPrefix(positionalTarget, "http://") && !strings.HasPrefix(positionalTarget, "https://") {
		runTarget = positionalTarget
		bridgeArgs = append([]string{}, bridgeArgs[1:]...)
	}
	if runTarget == "" && len(runEnv.Keys) > 0 {
		fmt.Fprintln(os.Stderr, "TSPACK_INSPECT_INVALID_TARGET_OPTIONS: --env requires --run or a run target name")
		exit(1)
	}
	if runTarget != "" {
		runEnv = withInspectInstrumentationEnv(runEnv)
	}

	bridge := requireManifestFrontendBridge("inspect-cli.js", "TSPACK_INSPECT_BRIDGE_MISSING", "inspect bridge")
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
			exit(1)
		}
		cwdPath, cwdErr := resolveRunTargetCwd(ref)
		if cwdErr != nil {
			fmt.Fprintf(os.Stderr, "%s: %s\n", cwdErr.code, cwdErr.msg)
			exit(1)
		}
		rt := ref.Target
		resolvedRuntime := resolveRunTargetRuntime(rt, workspaceRuntimeForRunTargets(ir))
		rt.Runtime = resolvedRuntime.Runtime
		fmt.Fprintf(os.Stderr, "Starting run target %q...\n", runTarget)
		fmt.Fprintf(os.Stderr, "Cwd: %s (%s)\n", effectiveRunTargetCwd(rt), cwdPath)
		if userEnvKeys := runEnv.UserKeys(); len(userEnvKeys) > 0 {
			fmt.Fprintf(os.Stderr, "Env: %s\n", strings.Join(userEnvKeys, ", "))
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
			exit(1)
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
				exit(exitCode)
			}
		}()
		fmt.Fprintf(os.Stderr, "Ready: %s\n", session.URL)
		fmt.Fprintf(os.Stderr, "Inspecting: %s\n", session.URL)
		nodeArgs = append(nodeArgs, session.URL)
		nodeArgs = append(nodeArgs, bridgeArgs...)
	} else {
		nodeArgs = append(nodeArgs, args[1:]...)
	}
	cmd, err := nodecmd.Command(nodeArgs...)
	if err != nil {
		if nodecmd.IsNotFound(err) {
			exitMessage = nodecmd.Message()
			exitCode = 127
			if runTarget != "" {
				return
			}
			fmt.Fprintln(os.Stderr, exitMessage)
			exit(exitCode)
		}
		exitMessage = fmt.Sprintf("TSPACK_INSPECT_FAILED: %v", err)
		exitCode = 1
		if runTarget != "" {
			return
		}
		fmt.Fprintln(os.Stderr, exitMessage)
		exit(exitCode)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			if runTarget != "" {
				return
			}
			exit(exitCode)
		}
		exitMessage = fmt.Sprintf("TSPACK_INSPECT_FAILED: %v", err)
		exitCode = 1
		if runTarget != "" {
			return
		}
		fmt.Fprintln(os.Stderr, exitMessage)
		exit(exitCode)
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
			exit(1)
		}
	}
	bridge := requireManifestFrontendBridge("native-test-cli.js", "TSPACK_DOOM_BRIDGE_MISSING", "native doom bridge")
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
	cmd, err := nodecmd.Command(nodeArgs...)
	if err != nil {
		if nodecmd.IsNotFound(err) {
			fmt.Fprintln(os.Stderr, nodecmd.Message())
			exit(127)
		}
		fmt.Fprintf(os.Stderr, "TSPACK_DOOM_FAILED: %v\n", err)
		exit(1)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "TSPACK_DOOM_FAILED: %v\n", err)
		exit(1)
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
			exit(1)
		}
	}
	bridge := requireManifestFrontendBridge("native-test-cli.js", "TSPACK_BENCH_BRIDGE_MISSING", "native benchmark bridge")
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
	cmd, err := nodecmd.Command(nodeArgs...)
	if err != nil {
		if nodecmd.IsNotFound(err) {
			fmt.Fprintln(os.Stderr, nodecmd.Message())
			exit(127)
		}
		fmt.Fprintf(os.Stderr, "TSPACK_BENCH_FAILED: %v\n", err)
		exit(1)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "TSPACK_BENCH_FAILED: %v\n", err)
		exit(1)
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
			exit(1)
		}
	}
	bridge := requireManifestFrontendBridge("native-test-cli.js", "TSPACK_ARTIFACT_BRIDGE_MISSING", "native artifact bridge")
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
	cmd, err := nodecmd.Command(nodeArgs...)
	if err != nil {
		if nodecmd.IsNotFound(err) {
			fmt.Fprintln(os.Stderr, nodecmd.Message())
			exit(127)
		}
		fmt.Fprintf(os.Stderr, "TSPACK_ARTIFACT_FAILED: %v\n", err)
		exit(1)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "TSPACK_ARTIFACT_FAILED: %v\n", err)
		exit(1)
	}
}

func nextTestFlagValue(args []string, index *int, flag string) (string, bool) {
	if *index+1 >= len(args) {
		fmt.Fprintf(os.Stderr, "missing value for test flag: %s\n", flag)
		exit(1)
		return "", false
	}
	*index = *index + 1
	return args[*index], true
}

func runTestCommand(args []string) {
	opts := testcmd.Options{RootDir: "."}
	runTarget := ""
	runReadyTimeout := 30
	runEnv := runEnvOverlay{}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--root":
			value, ok := nextTestFlagValue(args, &i, "--root")
			if !ok {
				return
			}
			opts.RootDir = value
		case "--run":
			value, ok := nextTestFlagValue(args, &i, "--run")
			if !ok {
				return
			}
			runTarget = value
		case "--run-ready-timeout":
			value, ok := nextTestFlagValue(args, &i, "--run-ready-timeout")
			if !ok {
				return
			}
			seconds, err := strconv.Atoi(value)
			if err != nil || seconds <= 0 {
				fmt.Fprintln(os.Stderr, "TSPACK_TEST_RUN_INVALID_OPTIONS: --run-ready-timeout must be positive seconds")
				exit(1)
			}
			runReadyTimeout = seconds
		case "--env":
			value, ok := nextTestFlagValue(args, &i, "--env")
			if !ok {
				return
			}
			var envErr *runErr
			runEnv, envErr = runEnv.WithAssignment(value)
			if envErr != nil {
				fmt.Fprintf(os.Stderr, "%s: %s\n", envErr.code, envErr.msg)
				exit(1)
			}
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
			exit(1)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if runTarget == "" && len(runEnv.Keys) > 0 {
		fmt.Fprintln(os.Stderr, "TSPACK_TEST_RUN_INVALID_OPTIONS: --env requires --run")
		exit(1)
	}
	if runTarget != "" && opts.List {
		fmt.Fprintln(os.Stderr, "TSPACK_TEST_RUN_INVALID_OPTIONS: --run cannot be combined with --list")
		exit(1)
	}
	if runTarget != "" {
		runEnv = withInspectInstrumentationEnv(runEnv)
	}

	var runSession *RunTargetSession
	if runTarget != "" {
		workspaceRoot := resolveWorkspaceRoot(opts.RootDir)
		opts.RootDir = workspaceRoot
		manifestPath := filepath.Join(workspaceRoot, "manifest.tsx")
		ir := loadManifestPathForRun(workspaceRoot, manifestPath)
		ref, ok := findRunTargetRefByName(workspaceRoot, manifestPath, ir, runTarget)
		if !ok {
			fmt.Fprintf(os.Stderr, "TSPACK_TEST_RUN_TARGET_NOT_FOUND: %s\n", runTarget)
			exit(1)
		}
		if strings.TrimSpace(ref.Target.URL) == "" {
			fmt.Fprintf(os.Stderr, "TSPACK_TEST_RUN_TARGET_URL_MISSING: %s\n", runTarget)
			exit(1)
		}
		cwdPath, cwdErr := resolveRunTargetCwd(ref)
		if cwdErr != nil {
			fmt.Fprintf(os.Stderr, "%s: %s\n", cwdErr.code, cwdErr.msg)
			exit(1)
		}
		resolvedRuntime := resolveRunTargetRuntime(
			ref.Target,
			workspaceRuntimeForRunTargets(ir),
		)
		ref.Target.Runtime = resolvedRuntime.Runtime
		fmt.Fprintf(os.Stderr, "Starting test run target %q...\n", runTarget)
		var readyErr *runErr
		runSession, readyErr = startRunTargetInDir(
			workspaceRoot,
			cwdPath,
			ref.Target,
			time.Duration(runReadyTimeout)*time.Second,
			os.Stderr,
			os.Stderr,
			runEnv,
		)
		if readyErr != nil {
			fmt.Fprintf(os.Stderr, "TSPACK_TEST_RUN_TARGET_FAILED: %s\n", readyErr.msg)
			exit(1)
		}
		defer func() {
			if err := runSession.Stop(); err != nil {
				fmt.Fprintf(os.Stderr, "TSPACK_TEST_RUN_SHUTDOWN_FAILED: %v\n", err)
			}
			fmt.Fprintf(os.Stderr, "Stopped test run target %q.\n", runTarget)
		}()
		fmt.Fprintf(os.Stderr, "Test run target ready: %s\n", runSession.URL)
		opts.Environment = append(
			opts.Environment,
			"TSPACK_TEST_RUN_TARGET_URL="+runSession.URL,
		)
	}

	operation := project.RunTest(ctx, project.TestRequest{Project: project.DefaultOptions(opts.RootDir), Options: opts})
	for _, d := range operation.Diagnostics {
		fmt.Fprintf(os.Stderr, "%s: %s\n", d.Code, d.Message)
		for _, detail := range d.Details {
			if detail == d.Message {
				continue
			}
			fmt.Fprintf(os.Stderr, "  %s\n", detail)
		}
		for _, fix := range d.Fixes {
			fmt.Fprintf(os.Stderr, "  suggested fix: %s\n", fix)
		}
	}
	if operation.ExitCode != 0 || hasDiagnosticErrors(operation.Diagnostics) {
		exit(1)
	}
}

func hasDiagnosticErrors(diagnostics []diag.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == diag.SeverityError {
			return true
		}
	}
	return false
}

func withInspectInstrumentationEnv(runEnv runEnvOverlay) runEnvOverlay {
	assignments := []string{"TSPACK_INSPECT_INSTRUMENTATION=1"}
	adapter := findManifestFrontendBridge(filepath.Join("inspect", "source-instrumentation.js"))
	if adapter.Path != "" {
		assignments = append(assignments, "TSPACK_INSPECT_VITE_ADAPTER="+adapter.Path)
	}
	for _, assignment := range assignments {
		updated, assignmentErr := runEnv.WithInternalAssignment(assignment)
		if assignmentErr != nil {
			fmt.Fprintf(os.Stderr, "%s: %s\n", assignmentErr.code, assignmentErr.msg)
			exit(1)
		}
		runEnv = updated
	}
	return runEnv
}
