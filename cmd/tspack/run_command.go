package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

type runTargetRef struct {
	PackageName   string
	PackageRoot   string
	WorkspaceRoot string
	Target        manifest.RunTarget
}

func (r runTargetRef) ID() string {
	return r.PackageName + ":" + r.Target.Name
}

type runCommandOptions struct {
	Root             string
	ManifestPath     string
	ManifestExplicit bool
	TimeoutSeconds   int
	Once             bool
	List             bool
	JSON             bool
	PackageName      string
	TargetArg        string
	Env              runEnvOverlay
}

type runListOutput struct {
	Command     string              `json:"command"`
	Mode        string              `json:"mode"`
	Root        string              `json:"root"`
	Package     *string             `json:"package"`
	Targets     []runListTargetJSON `json:"targets"`
	Diagnostics []any               `json:"diagnostics"`
}

type runListTargetJSON struct {
	ID               string                  `json:"id"`
	Package          string                  `json:"package"`
	Name             string                  `json:"name"`
	Runtime          string                  `json:"runtime"`
	RuntimeSource    string                  `json:"runtimeSource"`
	ExplicitRuntime  *string                 `json:"explicitRuntime"`
	WorkspaceRuntime *string                 `json:"workspaceRuntime"`
	Command          []string                `json:"command"`
	URL              string                  `json:"url"`
	Cwd              string                  `json:"cwd"`
	CwdPath          string                  `json:"cwdPath,omitempty"`
	Ready            *manifest.RunReadyCheck `json:"ready,omitempty"`
}

type RunTargetSession struct {
	Target           manifest.RunTarget
	Cmd              *exec.Cmd
	URL              string
	ReadyDescription string
	readyCheck       runReadyCheck
	rootPID          int
	waitDone         chan struct{}
	waitErr          error
	cleanupHandle    runTargetCleanupHandle
	cleanupOnce      sync.Once
	cleanupErr       error
}

func (s *RunTargetSession) Stop() error {
	if s == nil || s.Cmd == nil {
		return nil
	}
	if s.rootPID == 0 && s.Cmd.Process != nil {
		s.rootPID = s.Cmd.Process.Pid
	}
	if s.rootPID != 0 {
		_ = signalRunTargetProcessGroup(s.rootPID, syscall.SIGTERM)
	}
	if !s.waitForExit(2 * time.Second) {
		if s.rootPID != 0 {
			if err := signalRunTargetProcessGroup(s.rootPID, syscall.SIGKILL); err != nil {
				return fmt.Errorf("target %q root PID %d forced cleanup failed: %w", s.Target.Name, s.rootPID, err)
			}
		}
		if !s.waitForExit(5 * time.Second) {
			return fmt.Errorf("target %q root PID %d cleanup timed out waiting for process exit", s.Target.Name, s.rootPID)
		}
	}
	return s.closeCleanupHandle()
}

func (s *RunTargetSession) Wait() error {
	if s == nil || s.waitDone == nil {
		return nil
	}
	<-s.waitDone
	return s.waitErr
}

func (s *RunTargetSession) waitForExit(timeout time.Duration) bool {
	if s == nil || s.waitDone == nil {
		return true
	}
	select {
	case <-s.waitDone:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (s *RunTargetSession) closeCleanupHandle() error {
	if s == nil {
		return nil
	}
	s.cleanupOnce.Do(func() {
		s.cleanupErr = cleanupExitedRunTargetProcessTree(s.rootPID, s.cleanupHandle)
		s.cleanupHandle = nil
	})
	return s.cleanupErr
}

func startRunTarget(root string, target manifest.RunTarget, timeout time.Duration, stdout io.Writer, stderr io.Writer) (*RunTargetSession, *runErr) {
	return startRunTargetInDir(root, root, target, timeout, stdout, stderr, runEnvOverlay{})
}

func startRunTargetInDir(root string, cwdPath string, target manifest.RunTarget, timeout time.Duration, stdout io.Writer, stderr io.Writer, envOverlay runEnvOverlay) (*RunTargetSession, *runErr) {
	session, launchErr := launchRunTargetInDir(root, cwdPath, target, stdout, stderr, envOverlay)
	if launchErr != nil {
		return nil, launchErr
	}
	if runTargetModeFor(target) == runTargetModeFinite {
		waitErr := session.Wait()
		_ = session.closeCleanupHandle()
		if waitErr == nil {
			return nil, &runErr{code: "TSPACK_RUN_PROCESS_EXITED_EARLY", msg: "finite target exited before readiness could be observed"}
		}
		return nil, &runErr{code: "TSPACK_RUN_PROCESS_EXITED_EARLY", msg: waitErr.Error()}
	}
	readyCheck := session.readyCheck
	if readyErr := waitReady(session, readyCheck, timeout); readyErr != nil {
		if readyErr.code != "TSPACK_RUN_PROCESS_EXITED_EARLY" {
			_ = session.Stop()
		} else {
			_ = session.closeCleanupHandle()
		}
		return nil, readyErr
	}
	session.URL = target.URL
	session.ReadyDescription = readyCheck.readyDescription()
	return session, nil
}

func launchRunTargetInDir(root string, cwdPath string, target manifest.RunTarget, stdout io.Writer, stderr io.Writer, envOverlay runEnvOverlay) (*RunTargetSession, *runErr) {
	resolved := target
	launchCommand, launchErr := resolveRunTargetLaunchCommand(root, resolved)
	if launchErr != nil {
		return nil, launchErr
	}
	cmd := exec.Command(launchCommand[0], launchCommand[1:]...)
	cmd.Dir = cwdPath
	configureRunTargetProcess(cmd)
	var readyCheck runReadyCheck
	var stdoutMatcher *outputMatcher
	var stderrMatcher *outputMatcher
	if runTargetModeFor(resolved) == runTargetModeServer {
		readyCheck = newReadyCheck(resolved)
		stdoutMatcher, stderrMatcher = readyCheck.outputMatchers()
	}
	var stdoutPipe io.ReadCloser
	var stderrPipe io.ReadCloser
	var pipeErr error
	if stdoutMatcher != nil {
		stdoutPipe, pipeErr = cmd.StdoutPipe()
		if pipeErr != nil {
			return nil, &runErr{code: "TSPACK_RUN_PROCESS_START_FAILED", msg: pipeErr.Error()}
		}
	} else {
		cmd.Stdout = stdout
	}
	if stderrMatcher != nil {
		stderrPipe, pipeErr = cmd.StderrPipe()
		if pipeErr != nil {
			return nil, &runErr{code: "TSPACK_RUN_PROCESS_START_FAILED", msg: pipeErr.Error()}
		}
	} else {
		cmd.Stderr = stderr
	}
	projectToolBins := projectToolBinDirs(root)
	cmd.Env = buildRunCommandEnv(resolved.Runtime, root, envOverlay)
	if err := cmd.Start(); err != nil {
		if isExecutableNotFoundErr(err) {
			return nil, &runErr{
				code: "TSPACK_RUN_TOOL_NOT_FOUND",
				msg:  missingRunToolMessage(launchCommand[0], resolved.Name, projectToolBins, err),
			}
		}
		return nil, &runErr{code: "TSPACK_RUN_PROCESS_START_FAILED", msg: err.Error()}
	}
	cleanupHandle, cleanupErr := attachRunTargetCleanup(cmd)
	if cleanupErr != nil {
		_ = signalRunTargetProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
		return nil, &runErr{code: "TSPACK_RUN_PROCESS_START_FAILED", msg: cleanupErr.Error()}
	}
	session := &RunTargetSession{
		Target:        resolved,
		Cmd:           cmd,
		readyCheck:    readyCheck,
		rootPID:       cmd.Process.Pid,
		waitDone:      make(chan struct{}),
		cleanupHandle: cleanupHandle,
	}
	var copyWG sync.WaitGroup
	copyOutputPipe(stdoutPipe, stdout, stdoutMatcher, &copyWG)
	copyOutputPipe(stderrPipe, stderr, stderrMatcher, &copyWG)
	go func() {
		waitErr := cmd.Wait()
		copyWG.Wait()
		session.waitErr = waitErr
		close(session.waitDone)
	}()
	return session, nil
}

func runRunCommand(args []string) {
	opts := parseRunCommandOptions(args)
	if opts.ManifestPath == "" {
		opts.ManifestPath = filepath.Join(opts.Root, "manifest.tsx")
	}
	workspaceRoot := resolveWorkspaceRoot(opts.Root)
	ir := loadManifestPathForRun(workspaceRoot, opts.ManifestPath)
	if opts.List {
		renderRunTargetList(workspaceRoot, opts.ManifestPath, ir, opts.PackageName, opts.JSON)
		return
	}
	selected := selectRunTarget(workspaceRoot, opts.ManifestPath, ir, opts.PackageName, opts.TargetArg)
	rt := selected.Target
	cwdPolicy := effectiveRunTargetCwd(rt)
	cwdPath, cwdErr := resolveRunTargetCwd(selected)
	if cwdErr != nil {
		failRun(cwdErr.code, cwdErr.msg)
	}
	fmt.Fprintf(os.Stderr, "Starting run target %q\n", selected.ID())
	fmt.Fprintf(os.Stderr, "Package: %s\n", selected.PackageName)
	resolvedRuntime := resolveRunTargetRuntime(rt, workspaceRuntimeForRunTargets(ir))
	rt.Runtime = resolvedRuntime.Runtime
	fmt.Fprintf(os.Stderr, "Runtime: %s (%s)\n", rt.Runtime, resolvedRuntime.Source)
	fmt.Fprintf(os.Stderr, "Command: %s\n", formatRunTargetCommand(rt))
	fmt.Fprintf(os.Stderr, "Cwd: %s (%s)\n", cwdPolicy, cwdPath)
	if len(opts.Env.Keys) > 0 {
		fmt.Fprintf(os.Stderr, "Env: %s\n", strings.Join(opts.Env.Keys, ", "))
	}
	if runTargetModeFor(rt) == runTargetModeFinite {
		fmt.Fprintln(os.Stderr, "Waiting for: process exit")
		session, launchErr := launchRunTargetInDir(workspaceRoot, cwdPath, rt, os.Stdout, os.Stderr, opts.Env)
		if launchErr != nil {
			failRun(launchErr.code, launchErr.msg)
		}
		waitErr := session.Wait()
		cleanupErr := session.closeCleanupHandle()
		if waitErr != nil {
			failRun(runProcessExitCode(waitErr), runProcessExitMessage(waitErr))
		}
		if cleanupErr != nil {
			failRun("TSPACK_RUN_CLEANUP_FAILED", fmt.Sprintf("target %q root PID %d cleanup failed after exit: %v", session.Target.Name, session.rootPID, cleanupErr))
		}
		fmt.Fprintln(os.Stderr, "Completed: exit code 0")
		return
	}

	readyCheck := newReadyCheck(rt)
	fmt.Fprintf(os.Stderr, "Waiting for: %s\n", readyCheck.waitingDescription())
	session, readyErr := startRunTargetInDir(workspaceRoot, cwdPath, rt, time.Duration(opts.TimeoutSeconds)*time.Second, os.Stdout, os.Stderr, opts.Env)
	if readyErr != nil {
		failRun(readyErr.code, readyErr.msg)
	}
	fmt.Fprintf(os.Stderr, "Ready: %s\n", session.ReadyDescription)
	if session.URL != "" && session.ReadyDescription != session.URL {
		fmt.Fprintf(os.Stderr, "URL: %s\n", session.URL)
	}
	if opts.Once {
		if err := session.Stop(); err != nil {
			failRun("TSPACK_RUN_CLEANUP_FAILED", err.Error()+"; hint: stop stale Vite/Node/esbuild processes and retry")
		}
		return
	}
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() { <-sig; _ = session.Stop() }()
	waitErr := session.Wait()
	cleanupErr := session.closeCleanupHandle()
	if waitErr != nil {
		failRun(runProcessExitCode(waitErr), runProcessExitMessage(waitErr))
	}
	if cleanupErr != nil {
		failRun("TSPACK_RUN_CLEANUP_FAILED", fmt.Sprintf("target %q root PID %d cleanup failed after exit: %v", session.Target.Name, session.rootPID, cleanupErr))
	}
}

func parseRunCommandOptions(args []string) runCommandOptions {
	opts := runCommandOptions{Root: ".", TimeoutSeconds: 30}
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--root":
			if i+1 >= len(args) {
				failRun("TSPACK_RUN_INVALID_ARGS", "--root requires a value")
			}
			i++
			opts.Root = args[i]
			if !opts.ManifestExplicit {
				opts.ManifestPath = filepath.Join(opts.Root, "manifest.tsx")
			}
		case "--manifest":
			if i+1 >= len(args) {
				failRun("TSPACK_RUN_INVALID_ARGS", "--manifest requires a value")
			}
			i++
			opts.ManifestPath = args[i]
			opts.ManifestExplicit = true
		case "--ready-timeout":
			if i+1 >= len(args) {
				failRun("TSPACK_RUN_INVALID_TIMEOUT", "--ready-timeout requires a value")
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				failRun("TSPACK_RUN_INVALID_TIMEOUT", "ready-timeout must be positive seconds")
			}
			opts.TimeoutSeconds = n
		case "--once":
			opts.Once = true
		case "--list":
			opts.List = true
		case "--json":
			opts.JSON = true
		case "--package":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				failRun("TSPACK_RUN_INVALID_ARGS", "--package requires a value")
			}
			i++
			opts.PackageName = args[i]
		case "--env":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				failRun("TSPACK_RUN_INVALID_ENV", "--env requires KEY=VALUE")
			}
			i++
			var envErr *runErr
			opts.Env, envErr = opts.Env.WithAssignment(args[i])
			if envErr != nil {
				failRun(envErr.code, envErr.msg)
			}
		default:
			if len(a) > 0 && a[0] == '-' {
				failRun("TSPACK_RUN_INVALID_ARGS", "unknown flag: "+a)
			}
			if opts.TargetArg != "" {
				failRun("TSPACK_RUN_INVALID_ARGS", "too many target arguments")
			}
			opts.TargetArg = a
		}
	}
	if opts.List && opts.TargetArg != "" {
		failRun("TSPACK_RUN_INVALID_ARGS", "--list cannot be combined with a target argument")
	}
	if opts.List && opts.Once {
		failRun("TSPACK_RUN_INVALID_ARGS", "--list cannot be combined with --once")
	}
	if opts.List && len(opts.Env.Keys) > 0 {
		failRun("TSPACK_RUN_INVALID_ARGS", "--list cannot be combined with --env")
	}
	return opts
}

type runErr struct{ code, msg string }

func waitReady(session *RunTargetSession, readyCheck runReadyCheck, timeout time.Duration) *runErr {
	deadline := time.Now().Add(timeout)
	for {
		if readyCheck.ready() {
			return nil
		}
		select {
		case <-session.waitDone:
			if readyCheck.ready() {
				return nil
			}
			return &runErr{"TSPACK_RUN_PROCESS_EXITED_EARLY", runProcessExitedBeforeReadyMessage(session.waitErr)}
		default:
		}
		if time.Now().After(deadline) {
			return &runErr{"TSPACK_RUN_READY_TIMEOUT", "ready check timed out"}
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func failRun(code, msg string) { fmt.Fprintln(os.Stderr, code+": "+msg); os.Exit(1) }

func loadManifestForRun(root string) *manifest.ManifestIR {
	return loadManifestPathForRun(root, filepath.Join(root, "manifest.tsx"))
}

func loadManifestPathForRun(root string, manifestPath string) *manifest.ManifestIR {
	_ = root
	cliPath := manifestFrontendCLIPath()
	cmd := exec.Command("node", cliPath, manifestPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		failRun("TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED", err.Error()+": "+stderr.String())
	}
	var parsed struct {
		Ok          bool `json:"ok"`
		IR          any  `json:"ir"`
		Diagnostics any  `json:"diagnostics"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil || !parsed.Ok {
		failRun("TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED", "failed to parse manifest frontend output")
	}
	irBytes, _ := json.Marshal(parsed.IR)
	ir, diags := manifest.LoadBytes(manifestPath, irBytes)
	if len(diags) > 0 {
		failRun(diags[0].Code, diags[0].Message)
	}
	if !placeholderManifestForFrontendStub(manifestPath) {
		ir.Workspace.RuntimeSpecified = workspaceRuntimeDeclaredInManifest(manifestPath)
	}
	return ir
}

func placeholderManifestForFrontendStub(manifestPath string) bool {
	contents, err := os.ReadFile(manifestPath)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(contents)) == "export default {}"
}

func workspaceRuntimeDeclaredInManifest(manifestPath string) bool {
	contents, err := os.ReadFile(manifestPath)
	if err != nil {
		return false
	}
	text := string(contents)
	return strings.Contains(text, "<Workspace") && strings.Contains(text, "runtime=")
}

func selectRunTarget(root string, manifestPath string, ir *manifest.ManifestIR, packageName string, targetName string) runTargetRef {
	if packageName != "" {
		return selectRunTargetInPackage(root, manifestPath, ir, packageName, targetName)
	}
	all := collectRunTargets(root, manifestPath, ir, "")
	if len(all) == 0 {
		failRun("TSPACK_RUN_TARGET_MISSING", "no run targets declared")
	}
	if targetName != "" {
		matches := matchingRunTargets(all, targetName)
		if len(matches) == 1 {
			return matches[0]
		}
		if len(matches) > 1 {
			failRun("TSPACK_RUN_TARGET_AMBIGUOUS", ambiguousTargetMessage(targetName, matches))
		}
		failRun("TSPACK_RUN_TARGET_NOT_FOUND", "target "+targetName+" not found; known targets: "+strings.Join(runTargetIDs(all), ", "))
	}
	devTargets := matchingRunTargets(all, "dev")
	if len(devTargets) == 1 {
		return devTargets[0]
	}
	if len(devTargets) > 1 {
		failRun("TSPACK_RUN_TARGET_AMBIGUOUS", ambiguousTargetMessage("dev", devTargets))
	}
	if len(all) == 1 {
		return all[0]
	}
	failRun("TSPACK_RUN_TARGET_AMBIGUOUS", "multiple run targets; pass target name or use --package <name>; candidates: "+strings.Join(runTargetIDs(all), ", "))
	return runTargetRef{}
}

func selectRunTargetInPackage(root string, manifestPath string, ir *manifest.ManifestIR, packageName string, targetName string) runTargetRef {
	pkg, ok := findRunPackage(ir, packageName)
	if !ok {
		failRun("TSPACK_RUN_PACKAGE_NOT_FOUND", "package "+packageName+" not found; known packages: "+strings.Join(knownRunPackages(ir), ", "))
	}
	refs := packageRunTargetRefs(root, manifestPath, ir, pkg)
	if len(refs) == 0 {
		failRun("TSPACK_RUN_TARGET_MISSING", "package "+packageName+" declares no run targets")
	}
	if targetName != "" {
		for _, ref := range refs {
			if ref.Target.Name == targetName {
				return ref
			}
		}
		failRun("TSPACK_RUN_TARGET_NOT_FOUND", "target "+targetName+" not found in package "+packageName+"; known targets: "+strings.Join(runTargetNames(refs), ", "))
	}
	for _, ref := range refs {
		if ref.Target.Name == "dev" {
			return ref
		}
	}
	if len(refs) == 1 {
		return refs[0]
	}
	failRun("TSPACK_RUN_TARGET_AMBIGUOUS", "package "+packageName+" has multiple run targets; pass target name; known targets: "+strings.Join(runTargetNames(refs), ", "))
	return runTargetRef{}
}

func renderRunTargetList(root string, manifestPath string, ir *manifest.ManifestIR, packageName string, jsonOutput bool) {
	refs := collectRunTargets(root, manifestPath, ir, packageName)
	if packageName != "" {
		if _, ok := findRunPackage(ir, packageName); !ok {
			failRun("TSPACK_RUN_PACKAGE_NOT_FOUND", "package "+packageName+" not found; known packages: "+strings.Join(knownRunPackages(ir), ", "))
		}
	}
	if jsonOutput {
		renderRunTargetListJSON(root, packageName, refs, workspaceRuntimeForRunTargets(ir))
		return
	}
	renderRunTargetListText(refs, workspaceRuntimeForRunTargets(ir))
}

func renderRunTargetListText(refs []runTargetRef, workspaceRuntime string) {
	fmt.Fprintln(os.Stdout, "Run targets")
	if len(refs) == 0 {
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "  (none)")
		return
	}
	renderRunTargetRuntimeNotes(workspaceRuntime)
	currentPackage := ""
	for _, ref := range refs {
		if ref.PackageName != currentPackage {
			if currentPackage != "" {
				fmt.Fprintln(os.Stdout)
			}
			currentPackage = ref.PackageName
			fmt.Fprintln(os.Stdout)
			fmt.Fprintf(os.Stdout, "  %s\n", ref.PackageName)
		}
		fmt.Fprintf(os.Stdout, "    %s\n", ref.Target.Name)
		resolvedRuntime := resolveRunTargetRuntime(ref.Target, workspaceRuntime)
		fmt.Fprintf(os.Stdout, "      runtime: %s (%s)\n", resolvedRuntime.Runtime, resolvedRuntime.Source)
		fmt.Fprintf(os.Stdout, "      command: %s\n", strings.Join(ref.Target.Command, " "))
		fmt.Fprintf(os.Stdout, "      url: %s\n", ref.Target.URL)
		cwdPath, _ := resolveRunTargetCwd(ref)
		fmt.Fprintf(os.Stdout, "      cwd: %s", effectiveRunTargetCwd(ref.Target))
		if cwdPath != "" {
			fmt.Fprintf(os.Stdout, " (%s)", cwdPath)
		}
		fmt.Fprintln(os.Stdout)
		if ref.Target.Ready != nil {
			fmt.Fprintf(os.Stdout, "      ready: %s\n", formatReadyForList(ref.Target.Ready))
		} else {
			fmt.Fprintln(os.Stdout, "      ready: none (finite target)")
		}
	}
}

func renderRunTargetRuntimeNotes(workspaceRuntime string) {
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Runtime notes:")
	profile := effectiveWorkspaceRuntimeProfileValue(workspaceRuntime)
	source := ""
	if workspaceRuntime == "" {
		source = " (default)"
	}
	fmt.Fprintf(os.Stdout, "  Workspace runtime profile: %s%s\n", profile, source)
	fmt.Fprintln(os.Stdout, "  node: resolves bare commands from project tool bins first; does not prepend node to script paths.")
	fmt.Fprintln(os.Stdout, "  system: runs commands directly without node-local tool resolution.")
	fmt.Fprintln(os.Stdout, "  explicit RunTarget runtime overrides the workspace runtime profile.")
	fmt.Fprintln(os.Stdout, "  unspecified RunTarget runtime inherits the workspace runtime profile.")
}

func effectiveWorkspaceRuntimeProfile(ir *manifest.ManifestIR) string {
	return effectiveWorkspaceRuntimeProfileValue(ir.Workspace.Runtime)
}

func workspaceRuntimeForRunTargets(ir *manifest.ManifestIR) string {
	if ir.Workspace.RuntimeSpecified {
		return ir.Workspace.Runtime
	}
	return ""
}

func effectiveWorkspaceRuntimeProfileValue(workspaceRuntime string) string {
	if workspaceRuntime != "" {
		return workspaceRuntime
	}
	return "nodejs"
}

type resolvedRunTargetRuntime struct {
	Runtime          string
	Source           string
	ExplicitRuntime  *string
	WorkspaceRuntime *string
}

func resolveRunTargetRuntime(target manifest.RunTarget, workspaceRuntime string) resolvedRunTargetRuntime {
	if target.Runtime != "" {
		explicit := target.Runtime
		workspace := optionalRuntimeValue(workspaceRuntime)
		return resolvedRunTargetRuntime{Runtime: target.Runtime, Source: "explicit", ExplicitRuntime: &explicit, WorkspaceRuntime: workspace}
	}
	if workspaceRuntime != "" {
		workspace := workspaceRuntime
		return resolvedRunTargetRuntime{Runtime: workspaceRuntime, Source: "workspace", WorkspaceRuntime: &workspace}
	}
	return resolvedRunTargetRuntime{Runtime: "nodejs", Source: "default"}
}

func optionalRuntimeValue(runtime string) *string {
	if runtime == "" {
		return nil
	}
	return &runtime
}

func renderRunTargetListJSON(root string, packageName string, refs []runTargetRef, workspaceRuntime string) {
	var packageValue *string
	if packageName != "" {
		value := packageName
		packageValue = &value
	}
	targets := make([]runListTargetJSON, 0, len(refs))
	for _, ref := range refs {
		resolvedRuntime := resolveRunTargetRuntime(ref.Target, workspaceRuntime)
		targets = append(targets, runListTargetJSON{
			ID:               ref.ID(),
			Package:          ref.PackageName,
			Name:             ref.Target.Name,
			Runtime:          resolvedRuntime.Runtime,
			RuntimeSource:    resolvedRuntime.Source,
			ExplicitRuntime:  resolvedRuntime.ExplicitRuntime,
			WorkspaceRuntime: resolvedRuntime.WorkspaceRuntime,
			Command:          ref.Target.Command,
			URL:              ref.Target.URL,
			Cwd:              effectiveRunTargetCwd(ref.Target),
			CwdPath:          mustRunTargetCwd(ref),
			Ready:            ref.Target.Ready,
		})
	}
	payload := runListOutput{
		Command:     "run",
		Mode:        "list",
		Root:        root,
		Package:     packageValue,
		Targets:     targets,
		Diagnostics: []any{},
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		failRun("TSPACK_RUN_INVALID_ARGS", "failed to encode run target list")
	}
	fmt.Fprintln(os.Stdout, string(encoded))
}

func collectRunTargets(root string, manifestPath string, ir *manifest.ManifestIR, packageName string) []runTargetRef {
	refs := []runTargetRef{}
	for pi := range ir.Packages {
		pkg := &ir.Packages[pi]
		if packageName != "" && pkg.Name != packageName {
			continue
		}
		refs = append(refs, packageRunTargetRefs(root, manifestPath, ir, pkg)...)
	}
	return refs
}

func packageRunTargetRefs(root string, manifestPath string, ir *manifest.ManifestIR, pkg *manifest.Package) []runTargetRef {
	refs := make([]runTargetRef, 0, len(pkg.RunTargets))
	packageRoot := resolvePackageRoot(root, manifestPath, ir, pkg)
	for _, target := range pkg.RunTargets {
		refs = append(refs, runTargetRef{
			PackageName:   pkg.Name,
			PackageRoot:   packageRoot,
			WorkspaceRoot: root,
			Target:        target,
		})
	}
	return refs
}

func findRunPackage(ir *manifest.ManifestIR, packageName string) (*manifest.Package, bool) {
	for pi := range ir.Packages {
		pkg := &ir.Packages[pi]
		if pkg.Name == packageName {
			return pkg, true
		}
	}
	return nil, false
}

func matchingRunTargets(refs []runTargetRef, name string) []runTargetRef {
	matches := []runTargetRef{}
	for _, ref := range refs {
		if ref.Target.Name == name {
			matches = append(matches, ref)
		}
	}
	return matches
}

func knownRunPackages(ir *manifest.ManifestIR) []string {
	packages := make([]string, 0, len(ir.Packages))
	for _, pkg := range ir.Packages {
		packages = append(packages, pkg.Name)
	}
	return packages
}

func runTargetIDs(refs []runTargetRef) []string {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, ref.ID())
	}
	return ids
}

func runTargetNames(refs []runTargetRef) []string {
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		names = append(names, ref.Target.Name)
	}
	return names
}

func ambiguousTargetMessage(name string, refs []runTargetRef) string {
	return "target " + name + " is ambiguous; candidates: " + strings.Join(runTargetIDs(refs), ", ") + "; hint: use --package <name> to select one"
}

type runReadyCheck interface {
	ready() bool
	waitingDescription() string
	readyDescription() string
	outputMatchers() (*outputMatcher, *outputMatcher)
}

type httpReadyCheck struct {
	url string
}

func (c *httpReadyCheck) ready() bool {
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(c.url)
	if err != nil {
		return false
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode <= 399
}

func (c *httpReadyCheck) waitingDescription() string {
	return "http " + c.url
}

func (c *httpReadyCheck) readyDescription() string {
	return c.url
}

func (c *httpReadyCheck) outputMatchers() (*outputMatcher, *outputMatcher) {
	return nil, nil
}

type tcpReadyCheck struct {
	host string
	port int
}

func (c *tcpReadyCheck) ready() bool {
	conn, err := net.DialTimeout("tcp", c.address(), 1*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (c *tcpReadyCheck) waitingDescription() string {
	return "tcp " + c.address()
}

func (c *tcpReadyCheck) readyDescription() string {
	return "tcp " + c.address()
}

func (c *tcpReadyCheck) outputMatchers() (*outputMatcher, *outputMatcher) {
	return nil, nil
}

func (c *tcpReadyCheck) address() string {
	return net.JoinHostPort(c.host, strconv.Itoa(c.port))
}

type stdoutMatchReadyCheck struct {
	pattern       string
	stream        string
	sharedMatcher *outputMatcher
}

func (c *stdoutMatchReadyCheck) ready() bool {
	return c.sharedMatcher.matched()
}

func (c *stdoutMatchReadyCheck) waitingDescription() string {
	return fmt.Sprintf("stdout-match %q on %s", c.pattern, c.stream)
}

func (c *stdoutMatchReadyCheck) readyDescription() string {
	return fmt.Sprintf("matched %q", c.pattern)
}

func (c *stdoutMatchReadyCheck) outputMatchers() (*outputMatcher, *outputMatcher) {
	switch c.stream {
	case "stdout":
		return c.sharedMatcher, nil
	case "stderr":
		return nil, c.sharedMatcher
	default:
		return c.sharedMatcher, c.sharedMatcher
	}
}

type outputMatcher struct {
	pattern []byte
	mu      sync.Mutex
	tail    []byte
	seen    bool
}

func newOutputMatcher(pattern string) *outputMatcher {
	return &outputMatcher{pattern: []byte(pattern)}
}

func (m *outputMatcher) observe(chunk []byte) {
	if m == nil || len(chunk) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.seen {
		return
	}
	combined := make([]byte, 0, len(m.tail)+len(chunk))
	combined = append(combined, m.tail...)
	combined = append(combined, chunk...)
	if bytes.Contains(combined, m.pattern) {
		m.seen = true
		return
	}
	keep := len(m.pattern) - 1
	if keep <= 0 {
		m.tail = nil
		return
	}
	if len(combined) <= keep {
		m.tail = append(m.tail[:0], combined...)
		return
	}
	m.tail = append(m.tail[:0], combined[len(combined)-keep:]...)
}

func (m *outputMatcher) matched() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.seen
}

func copyOutputPipe(pipe io.ReadCloser, destination io.Writer, matcher *outputMatcher, wg *sync.WaitGroup) {
	if pipe == nil {
		return
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer pipe.Close()
		buffer := make([]byte, 4096)
		for {
			n, err := pipe.Read(buffer)
			if n > 0 {
				chunk := buffer[:n]
				_, _ = destination.Write(chunk)
				matcher.observe(chunk)
			}
			if err != nil {
				return
			}
		}
	}()
}

func newReadyCheck(rt manifest.RunTarget) runReadyCheck {
	switch rt.Ready.Kind {
	case "tcp":
		host := rt.Ready.Host
		if host == "" {
			host = "127.0.0.1"
		}
		return &tcpReadyCheck{host: host, port: rt.Ready.Port}
	case "stdout-match":
		stream := rt.Ready.Stream
		if stream == "" {
			stream = "both"
		}
		return &stdoutMatchReadyCheck{
			pattern:       rt.Ready.Pattern,
			stream:        stream,
			sharedMatcher: newOutputMatcher(rt.Ready.Pattern),
		}
	default:
		return &httpReadyCheck{url: readinessURL(rt)}
	}
}

func formatReadyForList(ready *manifest.RunReadyCheck) string {
	if ready == nil {
		return "none (finite target)"
	}
	switch ready.Kind {
	case "http":
		return "http " + ready.Path
	case "tcp":
		host := ready.Host
		if host == "" {
			host = "127.0.0.1"
		}
		return "tcp " + net.JoinHostPort(host, strconv.Itoa(ready.Port))
	case "stdout-match":
		stream := ready.Stream
		if stream == "" {
			stream = "both"
		}
		return fmt.Sprintf("stdout-match %q on %s", ready.Pattern, stream)
	default:
		return ready.Kind
	}
}

func readinessURL(rt manifest.RunTarget) string {
	if rt.Ready == nil {
		return rt.URL
	}
	u, _ := url.Parse(rt.URL)
	u.Path = rt.Ready.Path
	return u.String()
}

type runEnvOverlay struct {
	Values map[string]string
	Keys   []string
}

func (overlay runEnvOverlay) WithAssignment(assignment string) (runEnvOverlay, *runErr) {
	key, value, ok := strings.Cut(assignment, "=")
	if !ok {
		return overlay, &runErr{code: "TSPACK_RUN_INVALID_ENV", msg: "--env must use KEY=VALUE"}
	}
	if !isValidRunEnvKey(key) {
		return overlay, &runErr{code: "TSPACK_RUN_INVALID_ENV", msg: "invalid --env key: " + key}
	}
	if overlay.Values == nil {
		overlay.Values = map[string]string{}
	}
	if _, exists := overlay.Values[key]; !exists {
		overlay.Keys = append(overlay.Keys, key)
	}
	overlay.Values[key] = value
	return overlay, nil
}

func isValidRunEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for index, r := range key {
		if index == 0 {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_' {
				continue
			}
			return false
		}
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func resolveRunTargetLaunchCommand(root string, target manifest.RunTarget) ([]string, *runErr) {
	if executable, ok := directRuntimeExecutable(target.Runtime); ok {
		if _, err := exec.LookPath(executable); err != nil {
			return nil, &runErr{
				code: "TSPACK_RUN_RUNTIME_NOT_FOUND",
				msg:  missingRuntimeMessage(target.Runtime, executable, target.Name),
			}
		}
		return prependRunTargetExecutable(executable, target.Command), nil
	}
	if target.Runtime == "node" || target.Runtime == "nodejs" {
		return resolveNodeLocalCommand(root, target.Command), nil
	}
	return target.Command, nil
}

func directRuntimeExecutable(runtime string) (string, bool) {
	switch runtime {
	case "bun":
		return "bun", true
	case "deno":
		return "deno", true
	default:
		return "", false
	}
}

func missingRuntimeMessage(runtime string, executable string, targetName string) string {
	runtimeName := runtime
	if runtime == "bun" {
		runtimeName = "Bun"
	}
	if runtime == "deno" {
		runtimeName = "Deno"
	}
	return "runtime: " + runtime + "; executable: " + executable + "; target: " + targetName + "; hint: install " + runtimeName + " or change the RunTarget runtime"
}

func prependRunTargetExecutable(executable string, argv []string) []string {
	command := make([]string, 0, len(argv)+1)
	command = append(command, executable)
	command = append(command, argv...)
	return command
}

func formatRunTargetCommand(target manifest.RunTarget) string {
	parts := target.Command
	if executable, ok := directRuntimeExecutable(target.Runtime); ok {
		parts = prependRunTargetExecutable(executable, target.Command)
	}
	return string(bytes.Join(stringSliceBytes(parts), []byte(" ")))
}

func buildRunCommandEnv(runtime string, root string, overlay runEnvOverlay) []string {
	env := os.Environ()
	if runtime == "node" || runtime == "nodejs" {
		env = prependProjectToolBinsToEnv(env, projectToolBinDirs(root))
	}
	return overlayRunEnv(env, overlay)
}

func overlayRunEnv(env []string, overlay runEnvOverlay) []string {
	if len(overlay.Values) == 0 {
		result := make([]string, len(env))
		copy(result, env)
		return result
	}
	filtered := make([]string, 0, len(env)+len(overlay.Keys))
	for _, entry := range env {
		key, _, hasEquals := strings.Cut(entry, "=")
		if hasEquals {
			if _, overridden := overlay.Values[key]; overridden {
				continue
			}
		}
		filtered = append(filtered, entry)
	}
	for _, key := range overlay.Keys {
		filtered = append(filtered, key+"="+overlay.Values[key])
	}
	return filtered
}

func projectToolBinDirs(root string) []string {
	// sync owns compatibility tool materialization in one project-local bin dir.
	// run prepends that bin dir before the host PATH for node-backed RunTargets.
	return []string{filepath.Join(root, "node_modules", ".bin")}
}

func prependProjectToolBinsToEnv(env []string, binDirs []string) []string {
	if len(binDirs) == 0 {
		result := make([]string, len(env))
		copy(result, env)
		return result
	}
	pathKey := "PATH"
	pathValue := ""
	filtered := make([]string, 0, len(env)+1)
	for _, entry := range env {
		key, value, hasEquals := strings.Cut(entry, "=")
		if !hasEquals {
			filtered = append(filtered, entry)
			continue
		}
		if isPathEnvKey(key) {
			pathKey = key
			pathValue = value
			continue
		}
		filtered = append(filtered, entry)
	}
	prefixes := make([]string, 0, len(binDirs))
	for _, dir := range binDirs {
		if dir == "" {
			continue
		}
		prefixes = append(prefixes, dir)
	}
	newPathValue := strings.Join(prefixes, string(os.PathListSeparator))
	if pathValue != "" {
		newPathValue += string(os.PathListSeparator) + pathValue
	}
	filtered = append(filtered, pathKey+"="+newPathValue)
	return filtered
}

func isPathEnvKey(key string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(key, "PATH")
	}
	return key == "PATH"
}

func stringSliceBytes(values []string) [][]byte {
	result := make([][]byte, 0, len(values))
	for _, value := range values {
		result = append(result, []byte(value))
	}
	return result
}

func resolveNodeLocalCommand(root string, command []string) []string {
	if len(command) == 0 {
		return command
	}
	name := command[0]
	if strings.ContainsRune(name, os.PathSeparator) {
		return command
	}
	for _, binDir := range projectToolBinDirs(root) {
		if local, ok := resolveCommandInDir(binDir, name); ok {
			resolved := make([]string, len(command))
			copy(resolved, command)
			resolved[0] = local
			return resolved
		}
	}
	return command
}

func resolveCommandInDir(dir string, name string) (string, bool) {
	for _, candidate := range executableCandidatesInDir(dir, name) {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

func executableCandidatesInDir(dir string, name string) []string {
	candidates := []string{filepath.Join(dir, name)}
	if runtime.GOOS != "windows" {
		return candidates
	}
	for _, ext := range strings.Split(strings.ToLower(os.Getenv("PATHEXT")), ";") {
		ext = strings.TrimSpace(ext)
		if ext == "" {
			continue
		}
		candidates = append(candidates, filepath.Join(dir, name+ext))
	}
	if len(candidates) == 1 {
		candidates = append(candidates,
			filepath.Join(dir, name+".cmd"),
			filepath.Join(dir, name+".exe"),
			filepath.Join(dir, name+".bat"),
		)
	}
	return candidates
}

func isExecutableNotFoundErr(err error) bool {
	var execErr *exec.Error
	return errors.As(err, &execErr) || errors.Is(err, exec.ErrNotFound)
}

func missingRunToolMessage(executable string, targetName string, projectToolBins []string, err error) string {
	lines := []string{
		fmt.Sprintf("Could not find executable %q for run target %q.", executable, targetName),
		"",
		"TSPack looked in:",
	}
	for _, dir := range projectToolBins {
		lines = append(lines, "  "+dir)
	}
	lines = append(lines,
		"  PATH",
		"",
		"Try:",
		"  tspack update",
		"  tspack sync",
		"",
		"Underlying error:",
		"  "+err.Error(),
	)
	return strings.Join(lines, "\n")
}

func resolveWorkspaceRoot(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		return root
	}
	return abs
}

func effectiveRunTargetCwd(target manifest.RunTarget) string {
	if target.Cwd == "package" {
		return "package"
	}
	return "workspace"
}

func resolveRunTargetCwd(ref runTargetRef) (string, *runErr) {
	policy := effectiveRunTargetCwd(ref.Target)
	if policy == "workspace" {
		return ref.WorkspaceRoot, nil
	}
	if ref.PackageRoot == "" {
		return "", &runErr{
			code: "TSPACK_RUN_PACKAGE_ROOT_UNKNOWN",
			msg:  "package root is unknown for " + ref.ID(),
		}
	}
	return ref.PackageRoot, nil
}

func mustRunTargetCwd(ref runTargetRef) string {
	cwdPath, err := resolveRunTargetCwd(ref)
	if err != nil {
		return ""
	}
	return cwdPath
}

func resolvePackageRoot(root string, manifestPath string, ir *manifest.ManifestIR, pkg *manifest.Package) string {
	if pkg.Root != "" {
		return filepath.Join(root, filepath.FromSlash(pkg.Root))
	}
	if len(ir.Packages) == 1 {
		if filepath.Base(manifestPath) == "package.manifest.tsx" {
			absManifest, err := filepath.Abs(manifestPath)
			if err == nil {
				return filepath.Dir(absManifest)
			}
			return filepath.Dir(manifestPath)
		}
		return root
	}
	return ""
}

type runTargetMode string

const (
	runTargetModeFinite runTargetMode = "finite"
	runTargetModeServer runTargetMode = "server"
)

func runTargetModeFor(target manifest.RunTarget) runTargetMode {
	if target.Ready != nil {
		return runTargetModeServer
	}
	return runTargetModeFinite
}

type runTargetCleanupHandle interface {
	Close() error
}

func runProcessExitedBeforeReadyMessage(waitErr error) string {
	if waitErr == nil {
		return "process exited before ready"
	}
	if exitErr, ok := waitErr.(*exec.ExitError); ok {
		return fmt.Sprintf("process exited with code %d before ready", exitErr.ExitCode())
	}
	return fmt.Sprintf("process exited before ready: %v", waitErr)
}

func runProcessExitCode(waitErr error) string {
	if waitErr == nil {
		return "TSPACK_RUN_PROCESS_EXIT_FAILED"
	}
	if _, ok := waitErr.(*exec.ExitError); ok {
		return "TSPACK_RUN_PROCESS_EXITED"
	}
	return "TSPACK_RUN_PROCESS_EXIT_FAILED"
}

func runProcessExitMessage(waitErr error) string {
	if waitErr == nil {
		return "process exited unexpectedly"
	}
	if exitErr, ok := waitErr.(*exec.ExitError); ok {
		return fmt.Sprintf("process exited with code %d", exitErr.ExitCode())
	}
	return waitErr.Error()
}
