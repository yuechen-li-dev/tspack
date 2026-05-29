package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tspack/tspack/internal/manifest"
)

type runTargetRef struct {
	PackageName string
	Target      manifest.RunTarget
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
	ID      string                  `json:"id"`
	Package string                  `json:"package"`
	Name    string                  `json:"name"`
	Runtime string                  `json:"runtime"`
	Command []string                `json:"command"`
	URL     string                  `json:"url"`
	Ready   *manifest.RunReadyCheck `json:"ready,omitempty"`
}

type RunTargetSession struct {
	Target   manifest.RunTarget
	Cmd      *exec.Cmd
	URL      string
	ReadyURL string
	waitCh   chan error
}

func (s *RunTargetSession) Stop() error {
	if s == nil || s.Cmd == nil {
		return nil
	}
	if err := terminate(s.Cmd); err != nil {
		return err
	}
	if s.waitCh != nil {
		<-s.waitCh
	}
	return nil
}

func startRunTarget(root string, target manifest.RunTarget, timeout time.Duration, stdout io.Writer, stderr io.Writer) (*RunTargetSession, *runErr) {
	resolved := target
	if resolved.Runtime == "node" {
		resolved.Command = resolveNodeLocalCommand(root, resolved.Command)
	}
	cmd := exec.Command(resolved.Command[0], resolved.Command[1:]...)
	cmd.Dir = root
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if resolved.Runtime == "node" {
		prependNodeModulesBin(cmd, root)
	}
	readyURL := readinessURL(resolved)
	if err := cmd.Start(); err != nil {
		return nil, &runErr{code: "TSPACK_RUN_PROCESS_START_FAILED", msg: err.Error()}
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	if readyErr := waitReady(waitCh, readyURL, timeout); readyErr != nil {
		_ = terminate(cmd)
		return nil, readyErr
	}
	return &RunTargetSession{Target: resolved, Cmd: cmd, URL: resolved.URL, ReadyURL: readyURL, waitCh: waitCh}, nil
}

func runRunCommand(args []string) {
	opts := parseRunCommandOptions(args)
	if opts.ManifestPath == "" {
		opts.ManifestPath = filepath.Join(opts.Root, "manifest.tsx")
	}
	ir := loadManifestPathForRun(opts.Root, opts.ManifestPath)
	if opts.List {
		renderRunTargetList(opts.Root, ir, opts.PackageName, opts.JSON)
		return
	}
	selected := selectRunTarget(ir, opts.PackageName, opts.TargetArg)
	rt := selected.Target
	fmt.Fprintf(os.Stderr, "Starting run target %q\n", selected.ID())
	fmt.Fprintf(os.Stderr, "Package: %s\n", selected.PackageName)
	fmt.Fprintf(os.Stderr, "Runtime: %s\n", rt.Runtime)
	fmt.Fprintf(os.Stderr, "Command: %s\n", bytes.Join(stringSliceBytes(rt.Command), []byte(" ")))
	readyURL := readinessURL(rt)
	fmt.Fprintf(os.Stderr, "Waiting for: %s\n", readyURL)
	session, readyErr := startRunTarget(opts.Root, rt, time.Duration(opts.TimeoutSeconds)*time.Second, os.Stdout, os.Stderr)
	if readyErr != nil {
		failRun(readyErr.code, readyErr.msg)
	}
	fmt.Fprintf(os.Stderr, "Ready: %s\n", session.URL)
	if opts.Once {
		_ = session.Stop()
		return
	}
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() { <-sig; _ = session.Stop() }()
	<-session.waitCh
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
	return opts
}

type runErr struct{ code, msg string }

func waitReady(waitCh <-chan error, readyURL string, timeout time.Duration) *runErr {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 1 * time.Second}
	for {
		select {
		case <-waitCh:
			return &runErr{"TSPACK_RUN_PROCESS_EXITED_EARLY", "process exited before ready"}
		default:
		}
		resp, err := client.Get(readyURL)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode <= 399 {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return &runErr{"TSPACK_RUN_READY_TIMEOUT", "ready check timed out"}
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func terminate(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	t := time.NewTimer(2 * time.Second)
	defer t.Stop()
	done := make(chan struct{})
	go func() { _, _ = cmd.Process.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-t.C:
		return cmd.Process.Kill()
	}
}

func failRun(code, msg string) { fmt.Fprintln(os.Stderr, code+": "+msg); os.Exit(1) }

func loadManifestForRun(root string) *manifest.ManifestIR {
	return loadManifestPathForRun(root, filepath.Join(root, "manifest.tsx"))
}

func loadManifestPathForRun(root string, manifestPath string) *manifest.ManifestIR {
	_ = root
	cliPath := filepath.Join("manifest-frontend", "dist", "src", "cli.js")
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
	return ir
}

func selectRunTarget(ir *manifest.ManifestIR, packageName string, targetName string) runTargetRef {
	if packageName != "" {
		return selectRunTargetInPackage(ir, packageName, targetName)
	}
	all := collectRunTargets(ir, "")
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

func selectRunTargetInPackage(ir *manifest.ManifestIR, packageName string, targetName string) runTargetRef {
	pkg, ok := findRunPackage(ir, packageName)
	if !ok {
		failRun("TSPACK_RUN_PACKAGE_NOT_FOUND", "package "+packageName+" not found; known packages: "+strings.Join(knownRunPackages(ir), ", "))
	}
	refs := packageRunTargetRefs(pkg)
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

func renderRunTargetList(root string, ir *manifest.ManifestIR, packageName string, jsonOutput bool) {
	refs := collectRunTargets(ir, packageName)
	if packageName != "" {
		if _, ok := findRunPackage(ir, packageName); !ok {
			failRun("TSPACK_RUN_PACKAGE_NOT_FOUND", "package "+packageName+" not found; known packages: "+strings.Join(knownRunPackages(ir), ", "))
		}
	}
	if jsonOutput {
		renderRunTargetListJSON(root, packageName, refs)
		return
	}
	renderRunTargetListText(refs)
}

func renderRunTargetListText(refs []runTargetRef) {
	fmt.Fprintln(os.Stdout, "Run targets")
	if len(refs) == 0 {
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "  (none)")
		return
	}
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
		fmt.Fprintf(os.Stdout, "      runtime: %s\n", ref.Target.Runtime)
		fmt.Fprintf(os.Stdout, "      command: %s\n", strings.Join(ref.Target.Command, " "))
		fmt.Fprintf(os.Stdout, "      url: %s\n", ref.Target.URL)
		if ref.Target.Ready != nil {
			fmt.Fprintf(os.Stdout, "      ready: %s %s\n", ref.Target.Ready.Kind, ref.Target.Ready.Path)
		} else {
			fmt.Fprintln(os.Stdout, "      ready: url")
		}
	}
}

func renderRunTargetListJSON(root string, packageName string, refs []runTargetRef) {
	var packageValue *string
	if packageName != "" {
		value := packageName
		packageValue = &value
	}
	targets := make([]runListTargetJSON, 0, len(refs))
	for _, ref := range refs {
		targets = append(targets, runListTargetJSON{
			ID:      ref.ID(),
			Package: ref.PackageName,
			Name:    ref.Target.Name,
			Runtime: ref.Target.Runtime,
			Command: ref.Target.Command,
			URL:     ref.Target.URL,
			Ready:   ref.Target.Ready,
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

func collectRunTargets(ir *manifest.ManifestIR, packageName string) []runTargetRef {
	refs := []runTargetRef{}
	for pi := range ir.Packages {
		pkg := &ir.Packages[pi]
		if packageName != "" && pkg.Name != packageName {
			continue
		}
		refs = append(refs, packageRunTargetRefs(pkg)...)
	}
	return refs
}

func packageRunTargetRefs(pkg *manifest.Package) []runTargetRef {
	refs := make([]runTargetRef, 0, len(pkg.RunTargets))
	for _, target := range pkg.RunTargets {
		refs = append(refs, runTargetRef{PackageName: pkg.Name, Target: target})
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

func readinessURL(rt manifest.RunTarget) string {
	if rt.Ready == nil {
		return rt.URL
	}
	u, _ := url.Parse(rt.URL)
	u.Path = rt.Ready.Path
	return u.String()
}

func prependNodeModulesBin(cmd *exec.Cmd, root string) {
	bin := filepath.Join(root, "node_modules", ".bin")
	pathValue := os.Getenv("PATH")
	newPath := "PATH=" + bin + string(os.PathListSeparator) + pathValue
	env := os.Environ()
	filtered := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if strings.HasPrefix(entry, "PATH=") {
			continue
		}
		filtered = append(filtered, entry)
	}
	filtered = append(filtered, newPath)
	cmd.Env = filtered
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
	local := filepath.Join(root, "node_modules", ".bin", name)
	if info, err := os.Stat(local); err == nil && !info.IsDir() {
		resolved := make([]string, len(command))
		copy(resolved, command)
		resolved[0] = local
		return resolved
	}
	return command
}
