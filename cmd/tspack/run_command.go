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
	root := "."
	manifestPath := ""
	manifestExplicit := false
	timeoutSeconds := 30
	once := false
	targetArg := ""
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--root":
			if i+1 >= len(args) {
				failRun("TSPACK_RUN_INVALID_TARGET", "--root requires a value")
			}
			i++
			root = args[i]
			if !manifestExplicit {
				manifestPath = filepath.Join(root, "manifest.tsx")
			}
		case "--manifest":
			if i+1 >= len(args) {
				failRun("TSPACK_RUN_INVALID_TARGET", "--manifest requires a value")
			}
			i++
			manifestPath = args[i]
			manifestExplicit = true
		case "--ready-timeout":
			if i+1 >= len(args) {
				failRun("TSPACK_RUN_INVALID_TIMEOUT", "--ready-timeout requires a value")
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				failRun("TSPACK_RUN_INVALID_TIMEOUT", "ready-timeout must be positive seconds")
			}
			timeoutSeconds = n
		case "--once":
			once = true
		default:
			if len(a) > 0 && a[0] == '-' {
				failRun("TSPACK_RUN_INVALID_TARGET", "unknown flag: "+a)
			}
			if targetArg != "" {
				failRun("TSPACK_RUN_INVALID_TARGET", "too many target arguments")
			}
			targetArg = a
		}
	}
	if manifestPath == "" {
		manifestPath = filepath.Join(root, "manifest.tsx")
	}
	ir := loadManifestPathForRun(root, manifestPath)
	rt := selectRunTarget(ir, targetArg)
	fmt.Fprintf(os.Stderr, "Starting run target %q\n", rt.Name)
	fmt.Fprintf(os.Stderr, "Runtime: %s\n", rt.Runtime)
	fmt.Fprintf(os.Stderr, "Command: %s\n", bytes.Join(stringSliceBytes(rt.Command), []byte(" ")))
	readyURL := readinessURL(rt)
	fmt.Fprintf(os.Stderr, "Waiting for: %s\n", readyURL)
	session, readyErr := startRunTarget(root, rt, time.Duration(timeoutSeconds)*time.Second, os.Stdout, os.Stderr)
	if readyErr != nil {
		failRun(readyErr.code, readyErr.msg)
	}
	fmt.Fprintf(os.Stderr, "Ready: %s\n", session.URL)
	if once {
		_ = session.Stop()
		return
	}
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() { <-sig; _ = session.Stop() }()
	<-session.waitCh
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

func selectRunTarget(ir *manifest.ManifestIR, name string) manifest.RunTarget {
	all := []manifest.RunTarget{}
	for _, p := range ir.Packages {
		all = append(all, p.RunTargets...)
	}
	if len(all) == 0 {
		failRun("TSPACK_RUN_TARGET_MISSING", "no run targets declared")
	}
	if name != "" {
		for _, t := range all {
			if t.Name == name {
				return t
			}
		}
		failRun("TSPACK_RUN_TARGET_NOT_FOUND", name)
	}
	for _, t := range all {
		if t.Name == "dev" {
			return t
		}
	}
	if len(all) == 1 {
		return all[0]
	}
	failRun("TSPACK_RUN_TARGET_AMBIGUOUS", "multiple run targets; pass target name")
	return manifest.RunTarget{}
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
