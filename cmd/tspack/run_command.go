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

func runRunCommand(args []string) {
	root := "."
	timeoutSeconds := 30
	once := false
	targetArg := ""
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--root":
			i++
			root = args[i]
		case "--ready-timeout":
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
	ir := loadManifestForRun(root)
	rt := selectRunTarget(ir, targetArg)
	if rt.Runtime == "node" {
		rt.Command = resolveNodeLocalCommand(root, rt.Command)
	}
	cmd := exec.Command(rt.Command[0], rt.Command[1:]...)
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if rt.Runtime == "node" {
		prependNodeModulesBin(cmd, root)
	}
	fmt.Printf("Starting run target %q\n", rt.Name)
	fmt.Printf("Runtime: %s\n", rt.Runtime)
	fmt.Printf("Command: %s\n", bytes.Join(stringSliceBytes(rt.Command), []byte(" ")))
	readyURL := readinessURL(rt)
	fmt.Printf("Waiting for: %s\n", readyURL)
	if err := cmd.Start(); err != nil {
		failRun("TSPACK_RUN_PROCESS_START_FAILED", err.Error())
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	readyErr := waitReady(waitCh, readyURL, time.Duration(timeoutSeconds)*time.Second)
	if readyErr != nil {
		_ = terminate(cmd)
		failRun(readyErr.code, readyErr.msg)
	}
	fmt.Printf("Ready: %s\n", rt.URL)
	if once {
		_ = terminate(cmd)
		<-waitCh
		return
	}
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() { <-sig; _ = terminate(cmd) }()
	<-waitCh
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
	cliPath := filepath.Join("manifest-frontend", "dist", "src", "cli.js")
	manifestPath := filepath.Join(root, "manifest.tsx")
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
