// Package clitest provides explicit in-process and process-level CLI runners,
// plus small workspace and assertion helpers shared by CLI tests.
package clitest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type AppRunner interface {
	Run(context.Context, []string) int
}

// RunApp is the default CLI test path. It proves dispatch, parsing, rendering,
// and application wiring without creating another operating-system process.
func RunApp(t testing.TB, app AppRunner, args ...string) Result {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	configured, ok := app.(interface {
		SetIO(io.Reader, io.Writer, io.Writer)
	})
	if !ok {
		t.Fatal("in-process CLI runner does not support injected IO")
	}
	configured.SetIO(bytes.NewReader(nil), &stdout, &stderr)
	exitCode := app.Run(context.Background(), args)
	return Result{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: exitCode}
}

// RunProcess executes the real binary. Use it only when the test's contract is
// process-specific, such as executable dispatch, environment inheritance,
// signals, process trees, file locks, or OS pipe behavior.
func RunProcess(t testing.TB, binary string, args ...string) Result {
	return RunProcessInDir(t, "", binary, args...)
}

func RunProcessInDir(t testing.TB, directory string, binary string, args ...string) Result {
	t.Helper()
	command := exec.Command(binary, args...)
	command.Dir = directory
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	exitCode := 0
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Fatalf("run tspack: %v", err)
		}
		exitCode = exitError.ExitCode()
	}
	return Result{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: exitCode}
}

func RunProcessJSON(t testing.TB, binary string, output any, args ...string) Result {
	t.Helper()
	result := RunProcess(t, binary, args...)
	if err := json.Unmarshal([]byte(result.Stdout), output); err != nil {
		t.Fatalf("decode tspack JSON: %v\nstdout: %s\nstderr: %s", err, result.Stdout, result.Stderr)
	}
	return result
}

type Workspace struct {
	Root string
}

func TempWorkspace(t testing.TB) Workspace {
	t.Helper()
	return Workspace{Root: t.TempDir()}
}

func (workspace Workspace) WriteManifest(t testing.TB, contents string) string {
	t.Helper()
	path := filepath.Join(workspace.Root, "manifest.tsx")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}

func AssertExit(t testing.TB, result Result, expected int) {
	t.Helper()
	if result.ExitCode != expected {
		t.Fatalf("exit code = %d, want %d\nstdout: %s\nstderr: %s", result.ExitCode, expected, result.Stdout, result.Stderr)
	}
}

type TrackedState map[string][]byte

func Snapshot(t testing.TB, root string, paths ...string) TrackedState {
	t.Helper()
	state := TrackedState{}
	for _, relativePath := range paths {
		contents, err := os.ReadFile(filepath.Join(root, relativePath))
		if os.IsNotExist(err) {
			state[relativePath] = nil
			continue
		}
		if err != nil {
			t.Fatalf("snapshot %s: %v", relativePath, err)
		}
		state[relativePath] = contents
	}
	return state
}

func AssertTrackedStateUnchanged(t testing.TB, root string, before TrackedState) {
	t.Helper()
	for relativePath, expected := range before {
		actual, err := os.ReadFile(filepath.Join(root, relativePath))
		if expected == nil && os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("read tracked state %s: %v", relativePath, err)
		}
		if !bytes.Equal(actual, expected) {
			t.Errorf("tracked state changed: %s", relativePath)
		}
	}
}

func (result Result) String() string {
	return fmt.Sprintf("exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
}
