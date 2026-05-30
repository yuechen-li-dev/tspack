package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tspack/tspack/internal/manifest"
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
	ID      string                  `json:"id"`
	Package string                  `json:"package"`
	Name    string                  `json:"name"`
	Runtime string                  `json:"runtime"`
	Command []string                `json:"command"`
	URL     string                  `json:"url"`
	Cwd     string                  `json:"cwd"`
	CwdPath string                  `json:"cwdPath,omitempty"`
	Ready   *manifest.RunReadyCheck `json:"ready,omitempty"`
}

type RunTargetSession struct {
	Target           manifest.RunTarget
	Cmd              *exec.Cmd
	URL              string
	ReadyDescription string
	waitCh           chan error
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
	return startRunTargetInDir(root, root, target, timeout, stdout, stderr, runEnvOverlay{})
}

func startRunTargetInDir(root string, cwdPath string, target manifest.RunTarget, timeout time.Duration, stdout io.Writer, stderr io.Writer, envOverlay runEnvOverlay) (*RunTargetSession, *runErr) {
	resolved := target
	launchCommand, launchErr := resolveRunTargetLaunchCommand(root, resolved)
	if launchErr != nil {
		return nil, launchErr
	}
	cmd := exec.Command(launchCommand[0], launchCommand[1:]...)
	cmd.Dir = cwdPath
	readyCheck := newReadyCheck(resolved)
	stdoutMatcher, stderrMatcher := readyCheck.outputMatchers()
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
	cmd.Env = buildRunCommandEnv(resolved.Runtime, root, envOverlay)
	if err := cmd.Start(); err != nil {
		return nil, &runErr{code: "TSPACK_RUN_PROCESS_START_FAILED", msg: err.Error()}
	}
	var copyWG sync.WaitGroup
	copyOutputPipe(stdoutPipe, stdout, stdoutMatcher, &copyWG)
	copyOutputPipe(stderrPipe, stderr, stderrMatcher, &copyWG)
	waitCh := make(chan error, 1)
	go func() {
		waitErr := cmd.Wait()
		copyWG.Wait()
		waitCh <- waitErr
	}()
	if readyErr := waitReady(waitCh, readyCheck, timeout); readyErr != nil {
		if readyErr.code != "TSPACK_RUN_PROCESS_EXITED_EARLY" {
			_ = terminate(cmd)
			<-waitCh
		}
		return nil, readyErr
	}
	return &RunTargetSession{Target: resolved, Cmd: cmd, URL: resolved.URL, ReadyDescription: readyCheck.readyDescription(), waitCh: waitCh}, nil
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
	fmt.Fprintf(os.Stderr, "Runtime: %s\n", rt.Runtime)
	fmt.Fprintf(os.Stderr, "Command: %s\n", formatRunTargetCommand(rt))
	fmt.Fprintf(os.Stderr, "Cwd: %s (%s)\n", cwdPolicy, cwdPath)
	if len(opts.Env.Keys) > 0 {
		fmt.Fprintf(os.Stderr, "Env: %s\n", strings.Join(opts.Env.Keys, ", "))
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

func waitReady(waitCh <-chan error, readyCheck runReadyCheck, timeout time.Duration) *runErr {
	deadline := time.Now().Add(timeout)
	for {
		select {
		case <-waitCh:
			return &runErr{"TSPACK_RUN_PROCESS_EXITED_EARLY", "process exited before ready"}
		default:
		}
		if readyCheck.ready() {
			return nil
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
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return err
	}
	time.AfterFunc(2*time.Second, func() {
		_ = cmd.Process.Kill()
	})
	return nil
}

func failRun(code, msg string) { fmt.Fprintln(os.Stderr, code+": "+msg); os.Exit(1) }

func loadManifestForRun(root string) *manifest.ManifestIR {
	return loadManifestPathForRun(root, filepath.Join(root, "manifest.tsx"))
}

func manifestFrontendCLIPath() string {
	candidates := []string{
		filepath.Join("manifest-frontend", "dist", "src", "cli.js"),
		filepath.Join("manifest-frontend", "dist", "cli.js"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return candidates[0]
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
	return ir
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
		cwdPath, _ := resolveRunTargetCwd(ref)
		fmt.Fprintf(os.Stdout, "      cwd: %s", effectiveRunTargetCwd(ref.Target))
		if cwdPath != "" {
			fmt.Fprintf(os.Stdout, " (%s)", cwdPath)
		}
		fmt.Fprintln(os.Stdout)
		if ref.Target.Ready != nil {
			fmt.Fprintf(os.Stdout, "      ready: %s\n", formatReadyForList(ref.Target.Ready))
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
			Cwd:     effectiveRunTargetCwd(ref.Target),
			CwdPath: mustRunTargetCwd(ref),
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
	if rt.Ready == nil {
		return &httpReadyCheck{url: rt.URL}
	}
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
	if target.Runtime == "bun" {
		if _, err := exec.LookPath("bun"); err != nil {
			return nil, &runErr{
				code: "TSPACK_RUN_RUNTIME_NOT_FOUND",
				msg:  "runtime: bun; executable: bun; target: " + target.Name + "; hint: install Bun or change the RunTarget runtime",
			}
		}
		command := make([]string, 0, len(target.Command)+1)
		command = append(command, "bun")
		command = append(command, target.Command...)
		return command, nil
	}
	if target.Runtime == "node" {
		return resolveNodeLocalCommand(root, target.Command), nil
	}
	return target.Command, nil
}

func formatRunTargetCommand(target manifest.RunTarget) string {
	parts := target.Command
	if target.Runtime == "bun" {
		parts = make([]string, 0, len(target.Command)+1)
		parts = append(parts, "bun")
		parts = append(parts, target.Command...)
	}
	return string(bytes.Join(stringSliceBytes(parts), []byte(" ")))
}

func buildRunCommandEnv(runtime string, root string, overlay runEnvOverlay) []string {
	env := os.Environ()
	if runtime == "node" {
		env = prependNodeModulesBinToEnv(env, root)
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

func prependNodeModulesBinToEnv(env []string, root string) []string {
	bin := filepath.Join(root, "node_modules", ".bin")
	pathValue := os.Getenv("PATH")
	newPath := "PATH=" + bin + string(os.PathListSeparator) + pathValue
	filtered := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if strings.HasPrefix(entry, "PATH=") {
			continue
		}
		filtered = append(filtered, entry)
	}
	filtered = append(filtered, newPath)
	return filtered
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
