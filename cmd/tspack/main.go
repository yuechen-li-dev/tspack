package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/capability"
	"github.com/yuechen-li-dev/tspack/internal/check"
	compatplan "github.com/yuechen-li-dev/tspack/internal/compat"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/how"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/nodecmd"
	"github.com/yuechen-li-dev/tspack/internal/npmbridge"
	"github.com/yuechen-li-dev/tspack/internal/npmobserve"
	"github.com/yuechen-li-dev/tspack/internal/project"
	"github.com/yuechen-li-dev/tspack/internal/testcmd"
	"github.com/yuechen-li-dev/tspack/internal/version"
	"github.com/yuechen-li-dev/tspack/internal/why"
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
	Code                   string      `json:"code"`
	Severity               string      `json:"severity"`
	Message                string      `json:"message"`
	File                   string      `json:"file,omitempty"`
	LifecycleScriptName    string      `json:"lifecycleScriptName,omitempty"`
	LifecycleCategory      string      `json:"lifecycleCategory,omitempty"`
	ConsumerInstallTime    *bool       `json:"consumerInstallTime,omitempty"`
	Acknowledged           *bool       `json:"acknowledged,omitempty"`
	AcknowledgmentKind     *string     `json:"acknowledgmentKind,omitempty"`
	AcknowledgedByCategory string      `json:"acknowledgedByCategory,omitempty"`
	Details                interface{} `json:"details,omitempty"`
	Fixes                  interface{} `json:"fixes,omitempty"`
}

type WhyJSONReport struct {
	Command      string               `json:"command"`
	Mode         string               `json:"mode,omitempty"`
	Query        string               `json:"query"`
	Package      *string              `json:"package"`
	OK           bool                 `json:"ok"`
	Root         string               `json:"root"`
	ManifestPath string               `json:"manifestPath,omitempty"`
	LockfilePath string               `json:"lockfilePath,omitempty"`
	Summary      WhyJSONSummary       `json:"summary"`
	Explanations []WhyJSONExplanation `json:"explanations"`
	LockPackages []WhyJSONLockPackage `json:"lockPackages,omitempty"`
	Reverse      []WhyJSONReversePath `json:"reverse,omitempty"`
	Notes        []string             `json:"notes,omitempty"`
	Diagnostics  []WhyJSONDiagnostic  `json:"diagnostics"`
}

type WhyJSONSummary struct {
	Explanations int `json:"explanations"`
	LockPackages int `json:"lockPackages,omitempty"`
	ReversePaths int `json:"reversePaths,omitempty"`
	Diagnostics  int `json:"diagnostics"`
	Warnings     int `json:"warnings"`
	Errors       int `json:"errors"`
}

type WhyJSONExplanation struct {
	Kind                string                `json:"kind"`
	PackageName         string                `json:"package,omitempty"`
	DependencyKey       string                `json:"dependencyKey,omitempty"`
	DependencyKind      string                `json:"dependencyKind,omitempty"`
	ExternalPackageName string                `json:"externalPackageName,omitempty"`
	TargetName          string                `json:"targetName,omitempty"`
	Optional            bool                  `json:"optional,omitempty"`
	Source              *WhyJSONSource        `json:"source,omitempty"`
	DeclaredBy          []WhyJSONDeclaration  `json:"declaredBy,omitempty"`
	ReachableFrom       []WhyJSONReachability `json:"reachableFrom,omitempty"`
	NotReachableFrom    []WhyJSONReachability `json:"notReachableFrom,omitempty"`
	LockPackages        []WhyJSONLockPackage  `json:"lockPackages,omitempty"`
	LockEdges           []WhyJSONLockEdge     `json:"lockEdges,omitempty"`
	DirectProject       *bool                 `json:"directProject,omitempty"`
}

type WhyJSONSource struct {
	Kind    string `json:"kind,omitempty"`
	Package string `json:"package,omitempty"`
	Range   string `json:"range,omitempty"`
}

type WhyJSONDeclaration struct {
	PackageName   string         `json:"package,omitempty"`
	Scope         string         `json:"scope,omitempty"`
	TargetName    string         `json:"targetName,omitempty"`
	DependencyKey string         `json:"dependencyKey,omitempty"`
	Kind          string         `json:"kind,omitempty"`
	Optional      bool           `json:"optional,omitempty"`
	Source        *WhyJSONSource `json:"source,omitempty"`
}

type WhyJSONReachability struct {
	PackageName string `json:"package"`
	TargetName  string `json:"target"`
	Reason      string `json:"reason"`
	Ref         string `json:"ref"`
}

type WhyJSONLockPackage struct {
	ID           string              `json:"id"`
	Name         string              `json:"name,omitempty"`
	Version      string              `json:"version,omitempty"`
	Source       string              `json:"source,omitempty"`
	Hash         string              `json:"hash,omitempty"`
	Capabilities []WhyJSONCapability `json:"capabilities,omitempty"`
}

type WhyJSONCapability struct {
	Kind                  string `json:"kind"`
	Script                string `json:"script,omitempty"`
	Command               string `json:"command,omitempty"`
	Execution             string `json:"execution,omitempty"`
	LifecycleCategory     string `json:"lifecycleCategory,omitempty"`
	ConsumerInstallTime   bool   `json:"consumerInstallTime"`
	Acknowledged          bool   `json:"acknowledged"`
	AcknowledgementReason string `json:"acknowledgementReason,omitempty"`
	BehaviorFixture       string `json:"behaviorFixture,omitempty"`
	BehaviorFixtureStatus string `json:"behaviorFixtureStatus,omitempty"`
	BehaviorReport        string `json:"behaviorReport,omitempty"`
	BehaviorReportStatus  string `json:"behaviorReportStatus,omitempty"`
}

type WhyJSONLockEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Kind     string `json:"kind"`
	Optional bool   `json:"optional"`
}

type WhyJSONReversePath struct {
	LockPackage string            `json:"lockPackage"`
	Root        string            `json:"root"`
	Path        []string          `json:"path"`
	Edges       []WhyJSONLockEdge `json:"edges"`
}

type WhyJSONDiagnostic struct {
	Code     string   `json:"code"`
	Severity string   `json:"severity"`
	Message  string   `json:"message"`
	File     string   `json:"file,omitempty"`
	Details  []string `json:"details"`
	Fixes    []string `json:"fixes,omitempty"`
}

type UpdateDryRunJSONReport struct {
	Command     string                         `json:"command"`
	DryRun      UpdateDryRunJSONState          `json:"dryRun"`
	OK          bool                           `json:"ok"`
	Root        string                         `json:"root"`
	Changed     bool                           `json:"changed"`
	Targeted    bool                           `json:"targeted,omitempty"`
	Query       string                         `json:"query,omitempty"`
	Selected    []project.UpdateSelectedTarget `json:"selected,omitempty"`
	Summary     UpdateDryRunSummary            `json:"summary"`
	Changes     UpdateDryRunChanges            `json:"changes"`
	Diagnostics []CheckJSONDiagnostic          `json:"diagnostics"`
}

type PolicyUpdateDryRunJSONReport struct {
	Command     string                `json:"command"`
	DryRun      UpdateDryRunJSONState `json:"dryRun"`
	OK          bool                  `json:"ok"`
	Root        string                `json:"root"`
	PolicyPlan  PolicyUpdatePlanJSON  `json:"policyPlan"`
	Diagnostics []CheckJSONDiagnostic `json:"diagnostics"`
}

type PolicyUpdatePlanJSON struct {
	PolicyPresent          bool                    `json:"policyPresent"`
	WouldUpdate            bool                    `json:"wouldUpdate"`
	WouldApply             bool                    `json:"wouldApply"`
	SecurityGatesEvaluated bool                    `json:"securityGatesEvaluated"`
	SecurityGateStatus     string                  `json:"securityGateStatus"`
	Summary                PolicyUpdatePlanSummary `json:"summary"`
	Allowed                []PolicyUpdateCandidate `json:"allowed"`
	Blocked                []PolicyUpdateCandidate `json:"blocked"`
	Unclassified           []PolicyUpdateCandidate `json:"unclassified"`
	NotApplicable          []PolicyUpdateCandidate `json:"notApplicable"`
	Noop                   []PolicyUpdateCandidate `json:"noop"`
}

type PolicyUpdatePlanSummary struct {
	Allowed         int `json:"allowed"`
	Blocked         int `json:"blocked"`
	Unclassified    int `json:"unclassified"`
	NotApplicable   int `json:"notApplicable"`
	Noop            int `json:"noop"`
	Ready           int `json:"ready"`
	SecurityBlocked int `json:"securityBlocked"`
	ReviewRequired  int `json:"reviewRequired"`
}

type PolicyUpdateCandidate struct {
	Name                    string                                 `json:"name"`
	Kind                    string                                 `json:"kind"`
	Requested               string                                 `json:"requested"`
	Current                 []string                               `json:"current"`
	Wanted                  string                                 `json:"wanted"`
	Latest                  string                                 `json:"latest"`
	Packages                []project.OutdatedPackage              `json:"packages"`
	PackageCount            int                                    `json:"packageCount"`
	PolicyStrategy          string                                 `json:"policyStrategy,omitempty"`
	PolicyLevel             string                                 `json:"policyLevel,omitempty"`
	PolicyStatus            string                                 `json:"policyStatus"`
	PolicyReason            string                                 `json:"policyReason,omitempty"`
	Action                  string                                 `json:"action"`
	EffectiveAction         string                                 `json:"effectiveAction"`
	Message                 string                                 `json:"message"`
	SecurityGateStatus      string                                 `json:"securityGateStatus"`
	SecurityGateReasons     []string                               `json:"securityGateReasons"`
	SecurityGateDiagnostics []project.PolicySecurityGateDiagnostic `json:"securityGateDiagnostics,omitempty"`
}

type UpdateDryRunJSONState struct {
	Enabled bool                `json:"enabled"`
	Changed bool                `json:"changed"`
	Summary UpdateDryRunSummary `json:"summary"`
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

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printDefaultHelp()
		return
	}
	if args[0] == "help" {
		topic := ""
		if len(args) > 1 {
			topic = args[1]
		}
		if !printHelpTopic(topic) {
			fmt.Fprintf(os.Stderr, "unknown help topic: %s\n\n", topic)
			printDefaultHelp()
			os.Exit(1)
		}
		return
	}

	if len(args) > 1 && (args[1] == "--help" || args[1] == "-h" || args[1] == "help") {
		if printHelpTopic(args[0]) {
			return
		}
	}

	if args[0] == "--version" || args[0] == "version" || args[0] == "-v" {
		printVersion()
		return
	}
	if args[0] == "check" || args[0] == "update" || args[0] == "sync" || args[0] == "pack" || args[0] == "why" || args[0] == "outdated" {
		runCommand(args)
		return
	}
	if args[0] == "build" {
		runBuildCommand(args)
		return
	}
	if args[0] == "npm" {
		runNpmCommand(args)
		return
	}
	if args[0] == "compat" {
		runCompatCommand(args)
		return
	}
	if args[0] == "adopt" {
		runAdoptCommand(args)
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
	if args[0] == "materialize-tree" {
		runMaterializeTreeCommand(args)
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
	if args[0] == "scenario" {
		runScenarioCommand(args)
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
	if args[0] == "migrate" {
		runMigrateCommand(args)
		return
	}

	fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
	printDefaultHelp()
	os.Exit(1)
}

func runNpmCommand(args []string) {
	root := "."
	npmArgs := []string{}

	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--root" {
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "--root requires a value")
				os.Exit(2)
			}
			root = args[i]
			continue
		}
		npmArgs = append(npmArgs, arg)
	}

	if len(npmArgs) == 0 {
		fmt.Fprintln(os.Stderr, "usage: tspack npm <npm-args...>")
		fmt.Fprintln(os.Stderr, "examples: install, ci, update, exec vite -- --version")
		os.Exit(2)
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
			os.Exit(127)
		}
		fmt.Fprintf(os.Stderr, "TSPACK_NPM_FAILED: %v\n", err)
		os.Exit(1)
	}
	if result.ExitCode != 0 {
		os.Exit(result.ExitCode)
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
		os.Exit(2)
	}
	subcommand := args[1]
	root := "."
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--root":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "--root requires a value")
				os.Exit(2)
			}
			root = args[i]
		default:
			fmt.Fprintf(os.Stderr, "unknown compat flag: %s\n", args[i])
			os.Exit(2)
		}
	}
	if subcommand != "list" && subcommand != "diff" && subcommand != "write" {
		fmt.Fprintf(os.Stderr, "unknown compat subcommand: %s\n", subcommand)
		os.Exit(2)
	}
	root = resolveWorkspaceRoot(root)
	ir := loadManifestPathForRun(root, filepath.Join(root, "manifest.tsx"))
	statuses, err := compatplan.Plan(root, ir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TSPACK_COMPAT_FAILED: %v\n", err)
		os.Exit(1)
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
			os.Exit(1)
		}
	case "write":
		if err := compatplan.Write(root, statuses); err != nil {
			fmt.Fprintf(os.Stderr, "TSPACK_COMPAT_WRITE_FAILED: %v\n", err)
			os.Exit(1)
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
	fmt.Println("  tspack test [--root .] [-xtest] [-vitest] [--list] [--filter text] [--compact] [--batch] [--update-snapshots] [--watch] [--xtest-bridge path]")
	fmt.Println("  tspack artifact [--root .] [--out path] [--list] [--filter text] [--json]")
	fmt.Println("  tspack bench [--root .] [--list] [--filter text] [--json]")
	fmt.Println("  tspack doom [--root .] [--list] [--filter text] [--json] [--out path]")
	fmt.Println("  tspack run [target] [--root .] [--manifest path] [--ready-timeout seconds] [--env KEY=VALUE] [--once] [--preflight-only]")
	fmt.Println("  tspack scenario <scenario.json> --run <RunTarget> [--root .] [--ready-timeout seconds]")
	fmt.Println("  tspack npm <npm-args...> [--root .]")
	fmt.Println("  tspack format [paths...] [--root .] [--check]")
	fmt.Println("  tspack lint [paths...] [--root .] [--fix] [--unsafe]")
	fmt.Println("  tspack inspect <url> [experimental] [--run target] [--env KEY=VALUE] [--url <url>] [--browser auto|vscode|playwright-chromium|chromium|browser-path|host-path|cdp] [--host-path path] [--browser-path path] [--cdp endpoint] [--list-targets] [--target index-or-id] [--target-url substring] [--viewport WxH] [--selector css] [--point x,y] [--json] [--out file] [--text file]")
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
			os.Exit(1)
		}
		cwdPath, cwdErr := resolveRunTargetCwd(ref)
		if cwdErr != nil {
			fmt.Fprintf(os.Stderr, "%s: %s\n", cwdErr.code, cwdErr.msg)
			os.Exit(1)
		}
		rt := ref.Target
		resolvedRuntime := resolveRunTargetRuntime(rt, workspaceRuntimeForRunTargets(ir))
		rt.Runtime = resolvedRuntime.Runtime
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
	cmd, err := nodecmd.Command(nodeArgs...)
	if err != nil {
		if nodecmd.IsNotFound(err) {
			exitMessage = nodecmd.Message()
			exitCode = 127
			if runTarget != "" {
				return
			}
			fmt.Fprintln(os.Stderr, exitMessage)
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
			os.Exit(127)
		}
		fmt.Fprintf(os.Stderr, "TSPACK_DOOM_FAILED: %v\n", err)
		os.Exit(1)
	}
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
			os.Exit(127)
		}
		fmt.Fprintf(os.Stderr, "TSPACK_BENCH_FAILED: %v\n", err)
		os.Exit(1)
	}
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
			os.Exit(127)
		}
		fmt.Fprintf(os.Stderr, "TSPACK_ARTIFACT_FAILED: %v\n", err)
		os.Exit(1)
	}
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
	opts.PerfWriter = os.Stderr
	manifestExplicit := false
	lockfileExplicit := false
	storeExplicit := false
	jsonOutput := false
	explainFile := ""
	explainSet := false
	checkPositionals := []string{}
	checkFormat := false
	showConflicts := false
	showLifecycle := false
	clean := false
	force := false
	updateDryRun := false
	updatePolicy := false
	updateQuiet := false
	outdatedPerPackage := false
	updateQuery := ""
	packOpts := project.PackOptions{}
	whyOpts := project.WhyOptions{}
	whyPositionals := []string{}
	if cmd == "check" && len(args) > 1 && (args[1] == "--help" || args[1] == "-h") {
		printCheckHelp()
		return
	}
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
		case "--force":
			if cmd != "sync" {
				fmt.Fprintf(os.Stderr, "unknown %s flag: --force\n", cmd)
				os.Exit(1)
			}
			force = true
		case "--out":
			i++
			packOpts.OutputDir = args[i]
		case "--policy":
			if cmd != "update" {
				fmt.Fprintf(os.Stderr, "unknown %s flag: --policy\n", cmd)
				os.Exit(1)
			}
			updatePolicy = true
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
		case "--verify":
			if cmd == "pack" {
				packOpts.Verify = true
				continue
			}
			fmt.Fprintf(os.Stderr, "unknown %s flag: --verify\n", cmd)
			os.Exit(1)
		case "--package":
			i++
			packOpts.PackageName = args[i]
			whyOpts.PackageName = args[i]
		case "--reverse":
			if cmd != "why" {
				fmt.Fprintf(os.Stderr, "unknown %s flag: --reverse\n", cmd)
				os.Exit(1)
			}
			whyOpts.Reverse = true
		case "--json":
			jsonOutput = true
		case "--per-package":
			if cmd != "outdated" {
				fmt.Fprintf(os.Stderr, "unknown %s flag: --per-package\n", cmd)
				os.Exit(1)
			}
			outdatedPerPackage = true
		case "--show-conflicts":
			if cmd != "check" {
				fmt.Fprintf(os.Stderr, "unknown %s flag: --show-conflicts\n", cmd)
				os.Exit(1)
			}
			showConflicts = true
		case "--show-lifecycle":
			if cmd != "check" {
				fmt.Fprintf(os.Stderr, "unknown %s flag: --show-lifecycle\n", cmd)
				os.Exit(1)
			}
			showLifecycle = true
		case "--format":
			if cmd != "check" {
				fmt.Fprintf(os.Stderr, "unknown %s flag: --format\n", cmd)
				os.Exit(1)
			}
			checkFormat = true
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
			if cmd == "why" {
				whyPositionals = append(whyPositionals, args[i])
			}
		}
	}
	var result project.Result
	if cmd == "why" {
		if whyOpts.Reverse {
			if len(whyPositionals) == 0 {
				fmt.Fprintln(os.Stderr, "TSPACK_WHY_QUERY_REQUIRED: reverse why requires exactly one query")
				os.Exit(1)
			}
			if len(whyPositionals) > 1 {
				fmt.Fprintln(os.Stderr, "TSPACK_WHY_INVALID_ARGS: reverse why requires exactly one query")
				os.Exit(1)
			}
		}
		if len(whyPositionals) > 0 {
			whyOpts.Query = whyPositionals[0]
		}
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
	if cmd == "update" && updatePolicy && !updateDryRun {
		fmt.Fprintln(os.Stderr, "TSPACK_UPDATE_POLICY_REQUIRES_DRY_RUN: policy-driven mutation is not implemented yet; use --dry-run")
		os.Exit(1)
	}
	if cmd == "update" && updatePolicy && updateQuery != "" {
		fmt.Fprintln(os.Stderr, "TSPACK_UPDATE_POLICY_TARGET_UNSUPPORTED: targeted policy planning is not implemented in M50b; use workspace policy dry-run")
		os.Exit(1)
	}
	if cmd == "update" && !updatePolicy && !updateQuiet && !jsonOutput {
		opts.Progress = project.Progress{Enabled: true, Writer: os.Stderr}
	}
	if cmd == "sync" && !jsonOutput {
		opts.Progress = project.Progress{Enabled: true, Writer: os.Stderr}
	}
	updateOptions := project.UpdateOptions{Query: updateQuery}
	switch cmd {
	case "check":
		result = project.Check(opts)
		if checkFormat {
			formatResult := runCheckFormatValidation(opts.RootDir, jsonOutput)
			result.Diagnostics = append(result.Diagnostics, formatResult.Diagnostics...)
		}
	case "update":
		if updatePolicy {
			result = project.Outdated(opts)
		} else if updateDryRun {
			result = project.UpdateDryRunWithOptions(opts, updateOptions)
		} else {
			result = project.UpdateWithOptions(opts, updateOptions)
		}
	case "sync":
		result = project.Sync(opts, clean, force)
	case "pack":
		result = project.Pack(opts, packOpts)
	case "why":
		if shouldUseObservedNPMWhy(opts, whyOpts) {
			observed, err := npmobserve.Explain(opts.RootDir, whyOpts.Query)
			if err != nil {
				result = project.Result{
					Diagnostics: []diag.Diagnostic{
						{
							Code:     "TSPACK_OBSERVED_NPM_WHY_FAILED",
							Severity: diag.SeverityError,
							Message:  err.Error(),
						},
					},
				}
			} else {
				if jsonOutput {
					printObservedNPMWhyJSON(observed)
				} else {
					printObservedNPMWhy(observed)
				}
				return
			}
		} else {
			result = project.Why(opts, whyOpts)
		}
	case "outdated":
		result = project.Outdated(opts)
	}
	if cmd == "why" && jsonOutput {
		report := buildWhyJSONReport(opts, whyOpts, result)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "TSPACK_WHY_JSON_ENCODE_FAILED: %v\n", err)
			os.Exit(1)
		}
		if hasErrors(result.Diagnostics) {
			os.Exit(1)
		}
		return
	}
	if cmd == "outdated" && jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		report := map[string]any{
			"command":      "outdated",
			"ok":           !hasErrors(result.Diagnostics),
			"root":         opts.RootDir,
			"summary":      result.Outdated.Summary,
			"entries":      outdatedJSONEntries(result.Outdated, outdatedPerPackage),
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
	if cmd == "update" && updatePolicy && updateDryRun && jsonOutput {
		report := buildPolicyUpdateDryRunJSONReport(opts, result)
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
	renderHumanDiagnostics(os.Stderr, result.Diagnostics, checkRenderOptions{ShowConflicts: showConflicts, ShowLifecycle: showLifecycle})
	if cmd == "update" && updatePolicy && updateDryRun {
		printPolicyUpdateDryRunPlan(result)
	}
	if result.LockDiff != nil {
		if cmd == "update" && updateDryRun {
			printUpdateDryRunPlan(result)
		} else {
			fmt.Printf("lockfile diff: +%d -%d\n", len(result.LockDiff.PackagesAdded), len(result.LockDiff.PackagesRemoved))
		}
	}
	if result.WhyResult != nil && whyOpts.Reverse {
		printReverseWhyResult(whyOpts, result.WhyResult)
	}
	if result.WhyResult != nil && !whyOpts.Reverse {
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

			printWhyCapabilities(e.LockPackages)

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
			if a.Verified {
				fmt.Printf("Verified package artifact: %s\n", a.Path)
			}
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
		for _, dep := range outdatedHumanEntries(result.Outdated, outdatedPerPackage) {
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
			if result.Outdated.HasPolicy {
				fmt.Printf("  policy: %s", dep.PolicyStatus)
				if dep.PolicyStrategy != "" {
					fmt.Printf(" %s", dep.PolicyStrategy)
				}
				if dep.PolicyLevel != "" {
					fmt.Printf(":%s", dep.PolicyLevel)
				}
				fmt.Println()
			}
			if dep.PackageCount > 0 {
				fmt.Printf("  packages: %d\n", dep.PackageCount)
				if len(dep.Packages) > 0 {
					fmt.Printf("  declared by: %s\n", formatOutdatedPackages(dep.Packages))
				}
			}
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

func printWhyCapabilities(lockPackages []why.LockPackageRef) {
	printedHeader := false
	for _, lockPackage := range lockPackages {
		for _, capability := range lockPackage.Capabilities {
			if !printedHeader {
				fmt.Println("capabilities:")
				printedHeader = true
			}
			fmt.Printf("  %s %s: %s\n", capability.Kind, capability.Script, capability.Command)
			fmt.Printf("    lifecycleCategory: %s\n", capability.LifecycleCategory)
			fmt.Printf("    consumerInstallTime: %t\n", capability.ConsumerInstallTime)
			fmt.Println("    execution: blocked by default")
			if capability.Acknowledged {
				fmt.Println("    acknowledged: true")
				if capability.AcknowledgementReason != "" {
					fmt.Printf("    reason: %s\n", capability.AcknowledgementReason)
				}
				if capability.BehaviorFixture != "" {
					fmt.Printf("    behaviorFixture: %s (%s)\n", capability.BehaviorFixture, capability.BehaviorFixtureStatus)
				}
				if capability.BehaviorReport != "" {
					fmt.Printf("    behaviorReport: %s (%s)\n", capability.BehaviorReport, capability.BehaviorReportStatus)
				}
			} else {
				fmt.Println("    acknowledged: false")
			}
		}
	}
}

func printReverseWhyResult(whyOpts project.WhyOptions, result *why.Result) {
	fmt.Printf("Reverse why: %s\n", whyOpts.Query)
	fmt.Println()

	if len(result.LockPackages) > 1 {
		fmt.Println("Matching lock packages:")
		for _, lockPackage := range result.LockPackages {
			fmt.Printf("  %s\n", lockPackage.ID)
		}
		fmt.Println()
	}

	pathsByLockPackage := map[string][]why.ReversePath{}
	for _, path := range result.ReversePaths {
		pathsByLockPackage[path.LockPackage] = append(pathsByLockPackage[path.LockPackage], path)
	}

	for _, lockPackage := range result.LockPackages {
		paths := pathsByLockPackage[lockPackage.ID]
		if len(paths) == 0 {
			if whyOpts.PackageName != "" {
				fmt.Printf("No reverse paths from package %s.\n", whyOpts.PackageName)
			} else {
				fmt.Printf("No reverse paths found for %s.\n", lockPackage.ID)
			}
			fmt.Println()
			continue
		}

		fmt.Printf("%s is pulled in by:\n", lockPackage.ID)
		printWhyCapabilities([]why.LockPackageRef{lockPackage})
		fmt.Println()
		for _, path := range paths {
			fmt.Printf("  %s\n", path.Root)
			fmt.Println("    path:")
			for index, node := range path.Path {
				if index == 0 {
					fmt.Printf("      %s\n", node)
				} else {
					fmt.Printf("      -> %s\n", node)
				}
			}
			fmt.Println()
		}
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

func buildPolicyUpdateDryRunJSONReport(opts project.Options, result project.Result) PolicyUpdateDryRunJSONReport {
	plan := buildPolicyUpdatePlan(result.Outdated)
	report := PolicyUpdateDryRunJSONReport{
		Command: "update",
		DryRun: UpdateDryRunJSONState{
			Enabled: true,
			Changed: false,
			Summary: UpdateDryRunSummary{},
		},
		OK:         !hasErrors(result.Diagnostics),
		Root:       opts.RootDir,
		PolicyPlan: plan,
	}
	report.Diagnostics = updateDiagnosticsJSON(result.Diagnostics)
	return report
}

func buildPolicyUpdatePlan(outdated *project.OutdatedResult) PolicyUpdatePlanJSON {
	plan := PolicyUpdatePlanJSON{
		SecurityGatesEvaluated: true,
		SecurityGateStatus:     "not_applicable",
		Allowed:                []PolicyUpdateCandidate{},
		Blocked:                []PolicyUpdateCandidate{},
		Unclassified:           []PolicyUpdateCandidate{},
		NotApplicable:          []PolicyUpdateCandidate{},
		Noop:                   []PolicyUpdateCandidate{},
	}
	if outdated == nil {
		return plan
	}
	plan.PolicyPresent = outdated.HasPolicy
	for _, dep := range outdatedHumanEntries(outdated, false) {
		candidate := policyUpdateCandidate(dep)
		applyPolicySecurityGate(&candidate, dep, outdated.Security)
		switch candidate.PolicyStatus {
		case "allowed":
			plan.Allowed = append(plan.Allowed, candidate)
		case "blocked-manual", "pinned", "outside-policy-level":
			plan.Blocked = append(plan.Blocked, candidate)
		case "not-applicable":
			plan.NotApplicable = append(plan.NotApplicable, candidate)
		case "unclassified":
			plan.Unclassified = append(plan.Unclassified, candidate)
		default:
			if dep.Status == "current" {
				candidate.PolicyStatus = "current"
				candidate.Action = "noop"
				candidate.Message = "dependency is already current"
				plan.Noop = append(plan.Noop, candidate)
			}
		}
	}
	plan.Summary = summarizePolicyUpdatePlan(plan)
	plan.WouldUpdate = plan.Summary.Allowed > 0
	plan.WouldApply = plan.Summary.Ready > 0
	plan.SecurityGateStatus = summarizePolicySecurityGateStatus(plan)
	return plan
}

func policyUpdateCandidate(dep project.OutdatedDependency) PolicyUpdateCandidate {
	status := dep.PolicyStatus
	action := "noop"
	message := dep.PolicyMessage
	switch status {
	case "allowed":
		action = "update"
	case "blocked-manual":
		action = "manual"
	case "pinned":
		action = "pinned"
	case "outside-policy-level":
		action = "outside-policy"
	case "unclassified":
		action = "unclassified"
	case "not-applicable":
		action = "not-applicable"
	default:
		if dep.Status == "current" {
			status = "current"
			message = "dependency is already current"
		}
	}
	if message == "" {
		message = status
	}
	return PolicyUpdateCandidate{
		Name:               dep.Name,
		Kind:               dep.Kind,
		Requested:          dep.Requested,
		Current:            dep.Current,
		Wanted:             dep.Wanted,
		Latest:             dep.Latest,
		Packages:           dep.Packages,
		PackageCount:       dep.PackageCount,
		PolicyStrategy:     dep.PolicyStrategy,
		PolicyLevel:        dep.PolicyLevel,
		PolicyStatus:       status,
		PolicyReason:       dep.PolicyReason,
		Action:             action,
		Message:            message,
		SecurityGateStatus: "not_applicable",
		EffectiveAction:    action,
	}
}

func applyPolicySecurityGate(candidate *PolicyUpdateCandidate, dep project.OutdatedDependency, security manifest.Security) {
	gate := project.EvaluatePolicySecurityGate(dep, security)
	candidate.SecurityGateStatus = gate.Status
	candidate.SecurityGateReasons = gate.Reasons
	candidate.SecurityGateDiagnostics = gate.Diagnostics
	candidate.EffectiveAction = policyEffectiveAction(candidate.PolicyStatus, gate.Status)
	if candidate.PolicyStatus == "allowed" && len(gate.Reasons) > 0 {
		candidate.Message = candidate.Message + ", security: " + strings.Join(gate.Reasons, "; ")
	}
}

func policyEffectiveAction(policyStatus string, securityStatus string) string {
	if policyStatus != "allowed" {
		return "skip"
	}
	switch securityStatus {
	case "passed":
		return "update"
	case "review_required":
		return "review"
	case "blocked":
		return "blocked"
	default:
		return "skip"
	}
}

func summarizePolicyUpdatePlan(plan PolicyUpdatePlanJSON) PolicyUpdatePlanSummary {
	summary := PolicyUpdatePlanSummary{
		Allowed:       len(plan.Allowed),
		Blocked:       len(plan.Blocked),
		Unclassified:  len(plan.Unclassified),
		NotApplicable: len(plan.NotApplicable),
		Noop:          len(plan.Noop),
	}
	for _, candidate := range plan.Allowed {
		switch candidate.SecurityGateStatus {
		case "passed":
			summary.Ready++
		case "blocked":
			summary.SecurityBlocked++
		case "review_required":
			summary.ReviewRequired++
		}
	}
	return summary
}

func summarizePolicySecurityGateStatus(plan PolicyUpdatePlanJSON) string {
	statuses := map[string]bool{}
	for _, candidates := range [][]PolicyUpdateCandidate{
		plan.Allowed,
		plan.Blocked,
		plan.Unclassified,
		plan.NotApplicable,
		plan.Noop,
	} {
		for _, candidate := range candidates {
			statuses[candidate.SecurityGateStatus] = true
		}
	}
	if len(statuses) == 0 {
		return "not_applicable"
	}
	if len(statuses) == 1 {
		for status := range statuses {
			return status
		}
	}
	return "mixed"
}

func updateDiagnosticsJSON(diags []diag.Diagnostic) []CheckJSONDiagnostic {
	sorted := append([]diag.Diagnostic(nil), diags...)
	diag.SortDiagnostics(sorted)
	out := make([]CheckJSONDiagnostic, 0, len(sorted))
	for _, d := range sorted {
		out = append(out, CheckJSONDiagnostic{Code: d.Code, Severity: string(d.Severity), Message: d.Message, Details: d.Details})
	}
	return out
}

func printPolicyUpdateDryRunPlan(result project.Result) {
	plan := buildPolicyUpdatePlan(result.Outdated)
	fmt.Println("Policy update plan (dry run)")
	fmt.Println("No lockfile changes will be written.")
	fmt.Println()
	if result.Outdated != nil && !result.Outdated.HasPolicy {
		fmt.Println("No update policy declared.")
		fmt.Println("Candidates are unclassified; use <UpdatePolicy> to declare rolling/manual/pinned intent.")
		fmt.Println()
	}
	if plan.Summary.Allowed == 0 && plan.Summary.Blocked == 0 && plan.Summary.Unclassified == 0 && plan.Summary.NotApplicable == 0 {
		fmt.Println("No policy-eligible updates found.")
		fmt.Println("security gates: evaluated")
		fmt.Println("lifecycle execution remains blocked")
		fmt.Println("lockfile written: no")
		return
	}
	printPolicyCandidates("Ready:", filterPolicyCandidatesByEffectiveAction(plan.Allowed, "update"))
	printPolicyCandidates("Needs review:", filterPolicyCandidatesByEffectiveAction(plan.Allowed, "review"))
	printPolicyCandidates("Blocked by security:", filterPolicyCandidatesByEffectiveAction(plan.Allowed, "blocked"))
	printPolicyCandidates("Blocked by policy:", plan.Blocked)
	printPolicyCandidates("Unclassified:", plan.Unclassified)
	printPolicyCandidates("Not applicable:", plan.NotApplicable)
	fmt.Println("Summary:")
	fmt.Printf("version-policy allowed: %d\n", plan.Summary.Allowed)
	fmt.Printf("ready: %d\n", plan.Summary.Ready)
	fmt.Printf("review required: %d\n", plan.Summary.ReviewRequired)
	fmt.Printf("security blocked: %d\n", plan.Summary.SecurityBlocked)
	fmt.Printf("policy blocked: %d\n", plan.Summary.Blocked)
	fmt.Printf("unclassified: %d\n", plan.Summary.Unclassified)
	fmt.Printf("not applicable: %d\n", plan.Summary.NotApplicable)
	fmt.Println("security gates: evaluated")
	fmt.Println("security model: current TSPack lifecycle/acknowledgment model")
	fmt.Println("lifecycle execution remains blocked")
	fmt.Println("lockfile written: no")
}

func filterPolicyCandidatesByEffectiveAction(candidates []PolicyUpdateCandidate, action string) []PolicyUpdateCandidate {
	filtered := []PolicyUpdateCandidate{}
	for _, candidate := range candidates {
		if candidate.EffectiveAction == action {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func printPolicyCandidates(title string, candidates []PolicyUpdateCandidate) {
	if len(candidates) == 0 {
		return
	}
	fmt.Println(title)
	for _, candidate := range candidates {
		fmt.Printf("%s %s %s -> %s", candidate.Name, candidate.Kind, formatPolicyVersion(candidate.Current), candidate.Latest)
		if candidate.PolicyStrategy != "" {
			fmt.Printf(" %s", candidate.PolicyStrategy)
			if candidate.PolicyLevel != "" {
				fmt.Printf(":%s", candidate.PolicyLevel)
			}
		}
		fmt.Printf(" packages: %d", candidate.PackageCount)
		if candidate.Message != "" {
			fmt.Printf(" %s", candidate.Message)
		}
		fmt.Printf("\n  security: %s", strings.ReplaceAll(candidate.SecurityGateStatus, "_", " "))
		if len(candidate.SecurityGateReasons) > 0 {
			fmt.Printf(" — %s", strings.Join(candidate.SecurityGateReasons, "; "))
		}
		fmt.Println()
	}
	fmt.Println()
}

func formatPolicyVersion(versions []string) string {
	if len(versions) == 0 {
		return "-"
	}
	return strings.Join(versions, ",")
}

func buildUpdateDryRunJSONReport(opts project.Options, result project.Result) UpdateDryRunJSONReport {
	report := UpdateDryRunJSONReport{
		Command: "update",
		DryRun:  UpdateDryRunJSONState{Enabled: true},
		OK:      !hasErrors(result.Diagnostics),
		Root:    opts.RootDir,
	}
	if result.UpdateTarget != nil {
		report.Targeted = result.UpdateTarget.Targeted
		report.Query = result.UpdateTarget.Query
		report.Selected = result.UpdateTarget.Selected
	}
	if result.DryRun != nil {
		report.Changed = result.DryRun.Changed
		report.Summary = UpdateDryRunSummary(result.DryRun.Summary)
		report.DryRun.Changed = result.DryRun.Changed
		report.DryRun.Summary = UpdateDryRunSummary(result.DryRun.Summary)
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

func deriveCheckFormatPaths(root string) []string {
	paths := map[string]bool{}
	addPath := func(path string) {
		cleaned := filepath.ToSlash(filepath.Clean(path))
		if cleaned == "." || cleaned == "" || strings.HasPrefix(cleaned, "../") || isGeneratedFormatPath(cleaned) {
			return
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(cleaned))); err == nil {
			paths[cleaned] = true
		}
	}
	addParentDir := func(path string) {
		dir := filepath.ToSlash(filepath.Dir(path))
		if dir == "." {
			addPath(path)
			return
		}
		addPath(dir)
	}

	addPath("manifest.tsx")
	addPath("package.json")
	addPath("src")

	lockPath := filepath.Join(root, "ts-lock.toml")
	if lf, _, err := lockfile.LoadFile(lockPath); err == nil {
		for _, pkg := range lf.Packages {
			if pkg.Path != "" {
				addPath(filepath.ToSlash(filepath.Join(pkg.Path, "src")))
				addPath(filepath.ToSlash(filepath.Join(pkg.Path, "package.json")))
			}
		}
		for _, target := range lf.Targets {
			if target.Entry != "" {
				addParentDir(target.Entry)
			}
			if target.Types != "" && !isGeneratedFormatPath(target.Types) {
				addParentDir(target.Types)
			}
		}
	}

	out := make([]string, 0, len(paths))
	for path := range paths {
		out = append(out, path)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return []string{"."}
	}
	return out
}

func isGeneratedFormatPath(path string) bool {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	first, _, _ := strings.Cut(cleaned, "/")
	switch first {
	case ".tspack", "node_modules", "dist", "tspack-artifacts", "coverage", ".git", "build", ".turbo", ".vite":
		return true
	default:
		return false
	}
}

func runCheckFormatValidation(root string, jsonOutput bool) biomeCommandResult {
	options := biomeCommandOptions{
		Command:                  "format",
		Root:                     root,
		Paths:                    deriveCheckFormatPaths(root),
		UseCheck:                 true,
		CaptureOutput:            jsonOutput,
		PrintDefaultConfigStatus: !jsonOutput,
	}
	result := runBiomeCommandWithOptions(options)
	for i, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "TSPACK_BIOME_BACKEND_NOT_FOUND" {
			result.Diagnostics[i] = newCheckFormatBackendMissingDiagnostic(diagnostic)
		}
	}
	return result
}

func newCheckFormatBackendMissingDiagnostic(underlying diag.Diagnostic) diag.Diagnostic {
	details := []string{
		"Install/configure the formatter backend or add the configured tool dependency.",
		"current backend: biome",
		"underlying: " + underlying.Code,
	}
	details = append(details, underlying.Details...)
	return diag.Diagnostic{
		Code:     "TSPACK_FORMAT_BACKEND_MISSING",
		Severity: diag.SeverityError,
		Message:  "format backend is not available",
		Details:  details,
	}
}

type checkRenderOptions struct {
	ShowConflicts bool
	ShowLifecycle bool
}

func renderHumanDiagnostics(out *os.File, diagnostics []diag.Diagnostic, options checkRenderOptions) {
	conflicts := []diag.Diagnostic{}
	lifecycle := []diag.Diagnostic{}
	other := []diag.Diagnostic{}
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "TSPACK_LOCK_VERSION_CONFLICT" && diagnostic.Severity != diag.SeverityError {
			conflicts = append(conflicts, diagnostic)
			continue
		}
		if diagnostic.Code == "TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT" && diagnostic.Severity != diag.SeverityError {
			lifecycle = append(lifecycle, diagnostic)
			continue
		}
		other = append(other, diagnostic)
	}

	rendered := append([]diag.Diagnostic{}, other...)
	if options.ShowConflicts || len(conflicts) < 2 {
		rendered = append(rendered, conflicts...)
	} else {
		rendered = append(rendered, versionConflictSummaryDiagnostic(conflicts))
	}
	if options.ShowLifecycle {
		rendered = append(rendered, lifecycle...)
	} else {
		unacknowledgedLifecycle := unacknowledgedLifecycleDiagnostics(lifecycle)
		if len(unacknowledgedLifecycle) == 0 && len(lifecycle) > 0 {
			rendered = append(rendered, lifecycleAllAcknowledgedSummaryDiagnostic(lifecycle))
		} else if len(unacknowledgedLifecycle) < 2 {
			rendered = append(rendered, unacknowledgedLifecycle...)
			if len(categoryAcknowledgedLifecycleDiagnostics(lifecycle)) > 0 {
				rendered = append(rendered, lifecycleSummaryDiagnostic(lifecycle))
			}
		} else {
			rendered = append(rendered, lifecycleSummaryDiagnostic(lifecycle))
		}
	}
	diag.SortDiagnostics(rendered)
	for _, diagnostic := range rendered {
		printHumanDiagnostic(out, diagnostic)
	}
}

func printHumanDiagnostic(out *os.File, diagnostic diag.Diagnostic) {
	fmt.Fprintf(out, "%s: %s\n", diagnostic.Code, diagnostic.Message)
	for _, detail := range diagnostic.Details {
		if detail == diagnostic.Message {
			continue
		}
		fmt.Fprintf(out, "  %s\n", detail)
	}
}

func versionConflictSummaryDiagnostic(conflicts []diag.Diagnostic) diag.Diagnostic {
	examples := versionConflictExamples(conflicts, 3)
	return diag.Diagnostic{
		Code:     "TSPACK_LOCK_VERSION_CONFLICT",
		Severity: diag.SeverityWarning,
		Message:  fmt.Sprintf("Version conflicts: %d packages have multiple resolved versions.", len(conflicts)),
		Details: []string{
			"Examples: " + strings.Join(examples, ", "),
			"Run `tspack check --show-conflicts` for full conflict diagnostics.",
		},
	}
}

func unacknowledgedLifecycleDiagnostics(lifecycle []diag.Diagnostic) []diag.Diagnostic {
	out := []diag.Diagnostic{}
	for _, diagnostic := range lifecycle {
		if lifecycleDiagnosticDetail(diagnostic, "acknowledgmentKind") == "lifecycle-category" {
			continue
		}
		out = append(out, diagnostic)
	}
	return out
}

func categoryAcknowledgedLifecycleDiagnostics(lifecycle []diag.Diagnostic) []diag.Diagnostic {
	out := []diag.Diagnostic{}
	for _, diagnostic := range lifecycle {
		if lifecycleDiagnosticDetail(diagnostic, "acknowledgmentKind") == "lifecycle-category" {
			out = append(out, diagnostic)
		}
	}
	return out
}

func lifecycleAllAcknowledgedSummaryDiagnostic(lifecycle []diag.Diagnostic) diag.Diagnostic {
	counts := lifecycleCategoryCounts(lifecycle)
	message := fmt.Sprintf("Lifecycle scripts: %d scripts acknowledged by category policy; execution remains blocked.", len(lifecycle))
	if counts[capability.LifecycleCategoryMaintainerPublish] == len(lifecycle) {
		message = fmt.Sprintf("Lifecycle scripts: %d maintainer-side scripts acknowledged by category policy; execution remains blocked.", len(lifecycle))
	}
	return diag.Diagnostic{
		Code:     "TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT",
		Severity: diag.SeverityInfo,
		Message:  message,
		Details: []string{
			"Run `tspack check --show-lifecycle` for full script and acknowledgment details.",
			"Run `tspack doctor security` for audit details.",
		},
	}
}
func lifecycleSummaryDiagnostic(lifecycle []diag.Diagnostic) diag.Diagnostic {
	counts := lifecycleCategoryCounts(lifecycle)
	consumerExamples := lifecycleExamplesByCategory(lifecycle, capability.LifecycleCategoryConsumerInstall, 2)
	maintainerExamples := lifecycleExamplesByCategory(lifecycle, capability.LifecycleCategoryMaintainerPublish, 3)
	otherExamples := lifecycleExamplesByCategory(lifecycle, capability.LifecycleCategoryOther, 3)
	details := []string{}
	if len(consumerExamples) > 0 {
		details = append(details, "Consumer examples: "+strings.Join(consumerExamples, ", "))
	}
	if len(maintainerExamples) > 0 {
		details = append(details, "Maintainer examples: "+strings.Join(maintainerExamples, ", "))
	}
	if len(otherExamples) > 0 {
		details = append(details, "Other examples: "+strings.Join(otherExamples, ", "))
	}
	if counts[capability.LifecycleCategoryConsumerInstall] == 0 && counts[capability.LifecycleCategoryMaintainerPublish] > 0 && counts[capability.LifecycleCategoryOther] == 0 {
		details = append(details, "These do not run during normal consumer install in npm-style workflows.")
	}
	details = append(details,
		"Run `tspack check --show-lifecycle` for full script and pull-chain details.",
		"Run `tspack doctor security` for policy posture.",
	)
	message := lifecycleSummaryMessage(counts)
	categoryAcknowledgedCount := len(categoryAcknowledgedLifecycleDiagnostics(lifecycle))
	unacknowledgedCount := len(unacknowledgedLifecycleDiagnostics(lifecycle))
	if categoryAcknowledgedCount > 0 {
		details = append([]string{fmt.Sprintf("%d lifecycle scripts acknowledged by lifecycle category policy.", categoryAcknowledgedCount)}, details...)
	}
	if unacknowledgedCount > 0 {
		details = append([]string{fmt.Sprintf("%d lifecycle scripts remain unacknowledged.", unacknowledgedCount)}, details...)
	}
	return diag.Diagnostic{
		Code:     "TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT",
		Severity: diag.SeverityWarning,
		Message:  message,
		Details:  details,
	}
}

func lifecycleSummaryMessage(counts map[string]int) string {
	parts := []string{}
	if counts[capability.LifecycleCategoryConsumerInstall] > 0 {
		parts = append(parts, fmt.Sprintf("%d consumer install-time scripts", counts[capability.LifecycleCategoryConsumerInstall]))
	}
	if counts[capability.LifecycleCategoryMaintainerPublish] > 0 {
		parts = append(parts, fmt.Sprintf("%d maintainer-side scripts", counts[capability.LifecycleCategoryMaintainerPublish]))
	}
	if counts[capability.LifecycleCategoryOther] > 0 {
		parts = append(parts, fmt.Sprintf("%d other lifecycle scripts", counts[capability.LifecycleCategoryOther]))
	}
	return "Lifecycle scripts: " + strings.Join(parts, " and ") + " found; execution is blocked by policy."
}

func versionConflictExamples(conflicts []diag.Diagnostic, limit int) []string {
	examples := []string{}
	for _, conflict := range sortedDiagnosticsForExamples(conflicts) {
		name := strings.TrimPrefix(conflict.Message, "package \"")
		if index := strings.Index(name, "\" appears at multiple versions"); index >= 0 {
			name = name[:index]
		}
		versions := []string{}
		for _, detail := range conflict.Details {
			trimmed := strings.TrimSpace(detail)
			if !strings.Contains(trimmed, " -> ") {
				continue
			}
			version := strings.SplitN(trimmed, " -> ", 2)[0]
			versions = append(versions, version)
		}
		examples = append(examples, fmt.Sprintf("%s (%s)", name, strings.Join(uniqueSorted(versions), ", ")))
		if len(examples) >= limit {
			return examples
		}
	}
	return examples
}

func lifecycleExamples(lifecycle []diag.Diagnostic, limit int) []string {
	examples := []string{}
	for _, diagnostic := range sortedDiagnosticsForExamples(lifecycle) {
		examples = append(examples, lifecyclePackageAndScript(diagnostic))
		if len(examples) >= limit {
			return examples
		}
	}
	return examples
}

func lifecycleExamplesByCategory(lifecycle []diag.Diagnostic, category string, limit int) []string {
	examples := []string{}
	for _, diagnostic := range sortedDiagnosticsForExamples(lifecycle) {
		if lifecycleDiagnosticDetail(diagnostic, "lifecycleCategory") != category {
			continue
		}
		examples = append(examples, lifecyclePackageAndScript(diagnostic))
		if len(examples) >= limit {
			return examples
		}
	}
	return examples
}

func lifecycleCategoryCounts(lifecycle []diag.Diagnostic) map[string]int {
	counts := map[string]int{
		capability.LifecycleCategoryConsumerInstall:   0,
		capability.LifecycleCategoryMaintainerPublish: 0,
		capability.LifecycleCategoryOther:             0,
	}
	for _, diagnostic := range lifecycle {
		category := lifecycleDiagnosticDetail(diagnostic, "lifecycleCategory")
		if category == "" {
			category = capability.LifecycleCategoryOther
		}
		counts[category]++
	}
	return counts
}

func lifecyclePackageAndScript(diagnostic diag.Diagnostic) string {
	script := lifecycleDiagnosticDetail(diagnostic, "lifecycleScriptName")
	if script == "" {
		script = lifecycleDiagnosticDetail(diagnostic, "script")
	}
	if script == "" {
		return lifecyclePackageName(diagnostic)
	}
	return lifecyclePackageName(diagnostic) + " " + script
}

func lifecycleDiagnosticDetail(diagnostic diag.Diagnostic, key string) string {
	prefix := key + ": "
	for _, detail := range diagnostic.Details {
		trimmed := strings.TrimSpace(detail)
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimPrefix(trimmed, prefix)
		}
	}
	return ""
}

func lifecyclePackageName(diagnostic diag.Diagnostic) string {
	for _, detail := range diagnostic.Details {
		trimmed := strings.TrimSpace(detail)
		if after, ok := strings.CutPrefix(trimmed, "package: "); ok {
			packageID := after
			if after, ok := strings.CutPrefix(packageID, "npm:"); ok {
				packageID = after
			}
			if at := strings.LastIndex(packageID, "@"); at > 0 {
				return packageID[:at]
			}
			return packageID
		}
	}
	return diagnostic.Message
}

func sortedDiagnosticsForExamples(diagnostics []diag.Diagnostic) []diag.Diagnostic {
	out := append([]diag.Diagnostic(nil), diagnostics...)
	diag.SortDiagnostics(out)
	return out
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	unique := []string{}
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
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
		if d.Code == "TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT" {
			jd.LifecycleScriptName = lifecycleDiagnosticDetail(d, "lifecycleScriptName")
			jd.LifecycleCategory = lifecycleDiagnosticDetail(d, "lifecycleCategory")
			consumerInstallTime := lifecycleDiagnosticDetail(d, "consumerInstallTime") == "true"
			jd.ConsumerInstallTime = &consumerInstallTime
			acknowledged := lifecycleDiagnosticDetail(d, "acknowledged") == "true"
			jd.Acknowledged = &acknowledged
			acknowledgmentKind := lifecycleDiagnosticDetail(d, "acknowledgmentKind")
			if acknowledgmentKind != "" && acknowledgmentKind != "null" {
				jd.AcknowledgmentKind = &acknowledgmentKind
			}
			jd.AcknowledgedByCategory = lifecycleDiagnosticDetail(d, "acknowledgedByCategory")
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

func shouldUseObservedNPMWhy(opts project.Options, whyOpts project.WhyOptions) bool {
	if whyOpts.Reverse || whyOpts.PackageName != "" || strings.TrimSpace(whyOpts.Query) == "" {
		return false
	}
	if !npmobserve.HasPackageJSON(opts.RootDir) {
		return false
	}
	if _, err := os.Stat(opts.LockfilePath); err == nil {
		return false
	}
	return true
}

func printObservedNPMWhyJSON(result npmobserve.ExplainResult) {
	report := map[string]any{
		"query":               result.Query,
		"sourceKind":          "observed-npm",
		"source":              npmobserve.SourceLabel,
		"found":               len(result.Direct) > 0 || len(result.Matches) > 0,
		"direct":              len(result.Direct) > 0,
		"packageJsonSections": observedJSONSections(result.Direct),
		"requestedRange":      observedJSONRequestedRange(result.Direct),
		"matches":             result.Matches,
		"chains":              result.Chains,
		"notes":               result.Notes,
		"lockfilePresent":     result.LockfilePresent,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "TSPACK_OBSERVED_NPM_WHY_JSON_ENCODE_FAILED: %v\n", err)
		os.Exit(1)
	}
}

func observedJSONSections(matches []npmobserve.DirectMatch) []string {
	sections := []string{}
	for _, match := range matches {
		sections = append(sections, match.Section)
	}
	return sections
}

func observedJSONRequestedRange(matches []npmobserve.DirectMatch) string {
	if len(matches) == 0 {
		return ""
	}
	return matches[0].Range
}

func printObservedNPMWhy(result npmobserve.ExplainResult) {
	fmt.Println("source: " + npmobserve.SourceLabel)
	fmt.Println()
	for _, lockfile := range result.UnsupportedLockfiles {
		fmt.Printf("note: %s is present; observed why currently supports npm package-lock only.\n", lockfile)
	}
	if len(result.UnsupportedLockfiles) > 0 {
		fmt.Println()
	}

	if len(result.Direct) > 0 || len(result.Matches) > 0 {
		if len(result.Direct) > 0 {
			fmt.Printf("%s is present in the observed npm project.\n", result.Query)
		} else {
			fmt.Printf("%s is present in the observed npm lockfile.\n", result.Query)
		}
		fmt.Println()
		printObservedSources(result)
		printObservedVersions(result)
		printObservedReasons(result)
		printObservedChains(result)
		printObservedAdoptionNote()
		return
	}

	if result.LockfilePresent {
		fmt.Printf("%s was not found in package.json or package-lock.json.\n", result.Query)
		printObservedAdoptionNote()
		return
	}

	fmt.Printf("%s was not found in package.json.\n", result.Query)
	fmt.Println("No package-lock.json is available, so TSPack cannot explain transitive npm packages yet.")
	fmt.Println("To create npm's lockfile, run:")
	fmt.Println("tspack npm install")
	printObservedAdoptionNote()
}

func printObservedSources(result npmobserve.ExplainResult) {
	fmt.Println("Source:")
	if len(result.Direct) > 0 {
		for _, direct := range result.Direct {
			fmt.Println(direct.Section)
		}
	}
	if result.LockfilePresent {
		fmt.Println("package-lock.json")
	}
	fmt.Println()
}

func printObservedVersions(result npmobserve.ExplainResult) {
	if len(result.Matches) == 0 {
		return
	}
	fmt.Println("Version:")
	for _, match := range result.Matches {
		if match.Version == "" {
			fmt.Printf("%s at %s\n", match.Name, match.Location)
		} else {
			fmt.Printf("%s %s at %s\n", match.Name, match.Version, match.Location)
		}
	}
	fmt.Println()
}

func printObservedReasons(result npmobserve.ExplainResult) {
	if len(result.Direct) == 0 {
		return
	}
	fmt.Println("Reason:")
	for _, direct := range result.Direct {
		section := strings.TrimPrefix(direct.Section, "package.json ")
		fmt.Printf("root package declares %s in %s as %s\n", result.Query, section, direct.Range)
	}
	fmt.Println()
}

func printObservedChains(result npmobserve.ExplainResult) {
	if len(result.Chains) == 0 {
		return
	}
	fmt.Println("Chain:")
	for chainIndex, chain := range result.Chains {
		if chainIndex > 0 {
			fmt.Println()
		}
		for nodeIndex, node := range chain.Nodes {
			prefix := ""
			if nodeIndex > 0 {
				prefix = strings.Repeat("  ", nodeIndex-1) + "└─ "
			}
			if node.Version == "" {
				fmt.Printf("%s%s\n", prefix, node.Name)
			} else {
				fmt.Printf("%s%s %s\n", prefix, node.Name, node.Version)
			}
		}
	}
	fmt.Println()
}

func printObservedAdoptionNote() {
	fmt.Println("Adoption note:")
	fmt.Println("This explanation is from observed npm metadata. It is not a TSPack manifest dependency classification yet.")
}

func buildWhyJSONReport(opts project.Options, whyOpts project.WhyOptions, result project.Result) WhyJSONReport {
	packageFilter := (*string)(nil)
	if whyOpts.PackageName != "" {
		name := whyOpts.PackageName
		packageFilter = &name
	}

	explanations := []WhyJSONExplanation{}
	lockPackages := []WhyJSONLockPackage{}
	reversePaths := []WhyJSONReversePath{}
	notes := []string{}
	if result.WhyResult != nil {
		for _, explanation := range result.WhyResult.Explanations {
			explanations = append(explanations, buildWhyJSONExplanation(explanation))
		}
		for _, lockPackage := range result.WhyResult.LockPackages {
			lockPackages = append(lockPackages, buildWhyJSONLockPackage(lockPackage))
		}
		for _, reversePath := range result.WhyResult.ReversePaths {
			reversePaths = append(reversePaths, buildWhyJSONReversePath(reversePath))
		}
		notes = append(notes, result.WhyResult.Notes...)
	}

	diagnostics := buildWhyJSONDiagnostics(result.Diagnostics)
	summary := WhyJSONSummary{
		Explanations: len(explanations),
		LockPackages: len(lockPackages),
		ReversePaths: len(reversePaths),
		Diagnostics:  len(diagnostics),
	}
	for _, diagnostic := range diagnostics {
		switch diagnostic.Severity {
		case string(diag.SeverityError):
			summary.Errors++
		case string(diag.SeverityWarning):
			summary.Warnings++
		}
	}

	mode := ""
	if whyOpts.Reverse {
		mode = "reverse"
	}

	return WhyJSONReport{
		Command:      "why",
		Mode:         mode,
		Query:        whyOpts.Query,
		Package:      packageFilter,
		OK:           summary.Errors == 0,
		Root:         opts.RootDir,
		ManifestPath: opts.ManifestPath,
		LockfilePath: opts.LockfilePath,
		Summary:      summary,
		Explanations: explanations,
		LockPackages: lockPackages,
		Reverse:      reversePaths,
		Notes:        notes,
		Diagnostics:  diagnostics,
	}
}

func buildWhyJSONLockPackage(lockPackage why.LockPackageRef) WhyJSONLockPackage {
	jsonPackage := WhyJSONLockPackage{
		ID:      lockPackage.ID,
		Name:    lockPackage.Name,
		Version: lockPackage.Version,
		Source:  lockPackage.Source,
		Hash:    lockPackage.Hash,
	}
	for _, capability := range lockPackage.Capabilities {
		jsonPackage.Capabilities = append(jsonPackage.Capabilities, WhyJSONCapability{
			Kind:                  capability.Kind,
			Script:                capability.Script,
			Command:               capability.Command,
			Execution:             capability.Execution,
			LifecycleCategory:     capability.LifecycleCategory,
			ConsumerInstallTime:   capability.ConsumerInstallTime,
			Acknowledged:          capability.Acknowledged,
			AcknowledgementReason: capability.AcknowledgementReason,
			BehaviorFixture:       capability.BehaviorFixture,
			BehaviorFixtureStatus: capability.BehaviorFixtureStatus,
			BehaviorReport:        capability.BehaviorReport,
			BehaviorReportStatus:  capability.BehaviorReportStatus,
		})
	}
	return jsonPackage
}

func buildWhyJSONReversePath(reversePath why.ReversePath) WhyJSONReversePath {
	jsonPath := WhyJSONReversePath{
		LockPackage: reversePath.LockPackage,
		Root:        reversePath.Root,
		Path:        append([]string(nil), reversePath.Path...),
	}
	for _, edge := range reversePath.Edges {
		jsonPath.Edges = append(jsonPath.Edges, WhyJSONLockEdge{
			From:     edge.From,
			To:       edge.To,
			Kind:     edge.Kind,
			Optional: edge.Optional,
		})
	}
	return jsonPath
}

func buildWhyJSONExplanation(explanation why.Explanation) WhyJSONExplanation {
	jsonExplanation := WhyJSONExplanation{
		Kind:                explanation.MatchType,
		PackageName:         explanation.PackageName,
		DependencyKey:       explanation.DependencyKey,
		DependencyKind:      explanation.Kind,
		ExternalPackageName: explanation.ExternalPackageName,
		TargetName:          explanation.TargetName,
		Optional:            explanation.Optional,
		DirectProject:       explanation.DirectProject,
	}
	if jsonExplanation.Kind == "" {
		jsonExplanation.Kind = explanation.Kind
	}

	for _, declaration := range explanation.DeclaredBy {
		jsonDeclaration := WhyJSONDeclaration{
			PackageName:   declaration.PackageName,
			Scope:         declaration.Scope,
			TargetName:    declaration.TargetName,
			DependencyKey: declaration.DependencyKey,
			Kind:          declaration.Kind,
			Optional:      declaration.Optional,
		}
		if declaration.SourceKind != "" || declaration.SourcePackage != "" || declaration.SourceRange != "" {
			jsonDeclaration.Source = &WhyJSONSource{
				Kind:    declaration.SourceKind,
				Package: declaration.SourcePackage,
				Range:   declaration.SourceRange,
			}
		}
		jsonExplanation.DeclaredBy = append(jsonExplanation.DeclaredBy, jsonDeclaration)
	}

	jsonExplanation.Source = primaryWhyJSONSource(jsonExplanation.DeclaredBy)
	for _, reachable := range explanation.ReachableFrom {
		jsonExplanation.ReachableFrom = append(jsonExplanation.ReachableFrom, buildWhyJSONReachability(reachable))
	}
	for _, unreachable := range explanation.NotReachableFrom {
		jsonExplanation.NotReachableFrom = append(jsonExplanation.NotReachableFrom, buildWhyJSONReachability(unreachable))
	}
	for _, lockPackage := range explanation.LockPackages {
		jsonExplanation.LockPackages = append(jsonExplanation.LockPackages, buildWhyJSONLockPackage(lockPackage))
	}
	for _, edge := range explanation.LockEdges {
		jsonExplanation.LockEdges = append(jsonExplanation.LockEdges, WhyJSONLockEdge{
			From:     edge.From,
			To:       edge.To,
			Kind:     edge.Kind,
			Optional: edge.Optional,
		})
	}

	return jsonExplanation
}

func primaryWhyJSONSource(declarations []WhyJSONDeclaration) *WhyJSONSource {
	for _, declaration := range declarations {
		if declaration.Source != nil {
			return declaration.Source
		}
	}
	return nil
}

func buildWhyJSONReachability(ref why.ReachabilityRef) WhyJSONReachability {
	return WhyJSONReachability{
		PackageName: ref.PackageName,
		TargetName:  ref.TargetName,
		Reason:      ref.Reason,
		Ref:         ref.PackageName + ":target:" + ref.TargetName,
	}
}

func buildWhyJSONDiagnostics(diags []diag.Diagnostic) []WhyJSONDiagnostic {
	sorted := append([]diag.Diagnostic(nil), diags...)
	diag.SortDiagnostics(sorted)
	jsonDiagnostics := []WhyJSONDiagnostic{}
	for _, diagnostic := range sorted {
		jsonDiagnostic := WhyJSONDiagnostic{
			Code:     diagnostic.Code,
			Severity: string(diagnostic.Severity),
			Message:  diagnostic.Message,
			File:     diagnostic.File,
			Details:  append([]string(nil), diagnostic.Details...),
			Fixes:    append([]string(nil), diagnostic.Fixes...),
		}
		if jsonDiagnostic.Details == nil {
			jsonDiagnostic.Details = []string{}
		}
		jsonDiagnostics = append(jsonDiagnostics, jsonDiagnostic)
	}
	return jsonDiagnostics
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
