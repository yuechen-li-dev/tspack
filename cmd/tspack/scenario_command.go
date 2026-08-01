package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/nodecmd"
)

// runScenarioCommand owns the browser host lifetime for declared JSON browser
// scenarios. The scenario runner itself is a generic Node/Playwright adapter;
// projects only describe viewports, steps, assertions, and artifact locations.
func runScenarioCommand(args []string) {
	root := "."
	scenarioPath := ""
	runTarget := ""
	readyTimeout := 30

	for index := 1; index < len(args); index++ {
		argument := args[index]
		switch argument {
		case "--root":
			index++
			if index >= len(args) {
				failScenarioArgument("--root requires a value")
			}
			root = args[index]
		case "--run":
			index++
			if index >= len(args) {
				failScenarioArgument("--run requires a target name")
			}
			runTarget = args[index]
		case "--ready-timeout":
			index++
			if index >= len(args) {
				failScenarioArgument("--ready-timeout requires seconds")
			}
			if _, err := fmt.Sscanf(args[index], "%d", &readyTimeout); err != nil || readyTimeout <= 0 {
				failScenarioArgument("--ready-timeout must be positive seconds")
			}
		case "--help", "-h", "help":
			printScenarioHelp()
			return
		default:
			if strings.HasPrefix(argument, "-") || scenarioPath != "" {
				failScenarioArgument("expected one scenario file")
			}
			scenarioPath = argument
		}
	}

	if scenarioPath == "" || runTarget == "" {
		printScenarioHelp()
		os.Exit(1)
	}

	workspaceRoot := resolveWorkspaceRoot(root)
	absScenarioPath, err := filepath.Abs(scenarioPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TSPACK_SCENARIO_INVALID_FILE: %v\n", err)
		os.Exit(1)
	}
	if _, err := os.Stat(absScenarioPath); err != nil {
		fmt.Fprintf(os.Stderr, "TSPACK_SCENARIO_FILE_NOT_FOUND: %s\n", absScenarioPath)
		os.Exit(1)
	}

	manifestPath := filepath.Join(workspaceRoot, "manifest.tsx")
	ir := loadManifestPathForRun(workspaceRoot, manifestPath)
	ref, ok := findRunTargetRefByName(workspaceRoot, manifestPath, ir, runTarget)
	if !ok {
		fmt.Fprintf(os.Stderr, "TSPACK_SCENARIO_RUN_TARGET_NOT_FOUND: %s\n", runTarget)
		os.Exit(1)
	}
	cwdPath, cwdErr := resolveRunTargetCwd(ref)
	if cwdErr != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", cwdErr.code, cwdErr.msg)
		os.Exit(1)
	}
	target := ref.Target
	target.Runtime = resolveRunTargetRuntime(target, workspaceRuntimeForRunTargets(ir)).Runtime

	fmt.Fprintf(os.Stderr, "Starting scenario run target %q...\n", runTarget)
	session, readyErr := startRunTargetInDir(workspaceRoot, cwdPath, target, time.Duration(readyTimeout)*time.Second, os.Stderr, os.Stderr, runEnvOverlay{})
	if readyErr != nil {
		fmt.Fprintf(os.Stderr, "TSPACK_SCENARIO_RUN_START_FAILED: %s\n", readyErr.msg)
		os.Exit(1)
	}
	exitCode := 0
	defer func() {
		if err := session.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "TSPACK_SCENARIO_RUN_SHUTDOWN_FAILED: %v\n", err)
			exitCode = 1
		}
		fmt.Fprintf(os.Stderr, "Stopped scenario run target %q.\n", runTarget)
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}()

	runnerPath, pathErr := resolveScenarioRunnerPath()
	if pathErr != nil {
		fmt.Fprintf(os.Stderr, "TSPACK_SCENARIO_RUNNER_UNAVAILABLE: %v\n", pathErr)
		os.Exit(1)
	}
	command, commandErr := nodecmd.Command(runnerPath, "--url", session.URL, "--scenario", absScenarioPath)
	if commandErr != nil {
		fmt.Fprintf(os.Stderr, "TSPACK_SCENARIO_NODE_UNAVAILABLE: %v\n", commandErr)
		exitCode = 1
		return
	}
	command.Dir = workspaceRoot
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
			return
		}
		fmt.Fprintf(os.Stderr, "TSPACK_SCENARIO_FAILED: %v\n", err)
		exitCode = 1
	}
}

func resolveScenarioRunnerPath() (string, error) {
	executablePath, executableErr := os.Executable()
	workingDirectory, workingDirectoryErr := os.Getwd()

	return resolveScenarioRunnerPathFrom(firstNonEmptyScenarioPath(executablePath, executableErr), firstNonEmptyScenarioPath(workingDirectory, workingDirectoryErr))
}

func resolveScenarioRunnerPathFrom(executablePath string, workingDirectory string) (string, error) {
	candidates := make([]string, 0, 3)
	if executablePath != "" {
		executableDirectory := filepath.Dir(executablePath)
		candidates = append(candidates,
			filepath.Join(executableDirectory, "tools", "run-browser-scenarios.mjs"),
			filepath.Join(executableDirectory, "..", "tools", "run-browser-scenarios.mjs"),
		)
	}
	if workingDirectory != "" {
		candidates = append(candidates, filepath.Join(workingDirectory, "tools", "run-browser-scenarios.mjs"))
	}

	for _, candidate := range candidates {
		absoluteCandidate, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if info, err := os.Stat(absoluteCandidate); err == nil && !info.IsDir() {
			return absoluteCandidate, nil
		}
	}

	return "", fmt.Errorf("could not find tools/run-browser-scenarios.mjs; searched %s", strings.Join(candidates, ", "))
}

func firstNonEmptyScenarioPath(value string, err error) string {
	if err != nil {
		return ""
	}
	return value
}

func failScenarioArgument(message string) {
	fmt.Fprintf(os.Stderr, "TSPACK_SCENARIO_INVALID_ARGS: %s\n", message)
	os.Exit(1)
}

func printScenarioHelp() {
	fmt.Println("Usage: tspack scenario <scenario.json> --run <RunTarget> [--root .] [--ready-timeout seconds]")
	fmt.Println("The JSON declaration names viewports, navigation steps, bounded assertions, screenshots, and an artifact directory.")
	fmt.Println("topmost-at-point requires integer x/y and one expected CSS selector; it uses browser hit testing without arbitrary script execution.")
}
