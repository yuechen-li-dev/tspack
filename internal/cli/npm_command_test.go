package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestNpmCommandDelegatesArgsAndRootWithoutTspackWrites(t *testing.T) {
	skipPosixFakeNpmTestOnWindows(t)
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), "{\"scripts\":{\"build\":\"vite build\"}}\n")
	binDir := t.TempDir()
	record := filepath.Join(t.TempDir(), "npm-record.txt")
	writeFakeNpmForCLI(t, filepath.Join(binDir, fakeNpmExecutableName()), record, 0)

	env := append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := runTspackWithEnv(bin, env, "npm", "install", "-D", "vite", "--root", root)
	if err != nil {
		t.Fatalf("tspack npm failed: %v\n%s", err, out)
	}
	content := readTestFile(t, record)
	if !strings.Contains(content, "cwd="+root) {
		t.Fatalf("fake npm did not run in root %q:\n%s", root, content)
	}
	if !strings.Contains(content, "arg0=install\narg1=-D\narg2=vite\n") {
		t.Fatalf("fake npm did not receive exact args:\n%s", content)
	}
	assertPathDoesNotExist(t, filepath.Join(root, "manifest.tsx"))
	assertPathDoesNotExist(t, filepath.Join(root, "ts-lock.toml"))
}

func TestNpmCommandExecPreservesDoubleDashArgs(t *testing.T) {
	skipPosixFakeNpmTestOnWindows(t)
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := t.TempDir()
	binDir := t.TempDir()
	record := filepath.Join(t.TempDir(), "npm-record.txt")
	writeFakeNpmForCLI(t, filepath.Join(binDir, fakeNpmExecutableName()), record, 0)

	env := append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := runTspackWithEnv(bin, env, "npm", "exec", "vite", "--", "--version", "--root", root)
	if err != nil {
		t.Fatalf("tspack npm exec failed: %v\n%s", err, out)
	}
	content := readTestFile(t, record)
	for _, expected := range []string{"arg0=exec", "arg1=vite", "arg2=--", "arg3=--version"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("missing %q in fake npm record:\n%s", expected, content)
		}
	}
}

func TestNpmCommandPreservesExitCode(t *testing.T) {
	skipPosixFakeNpmTestOnWindows(t)
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := t.TempDir()
	binDir := t.TempDir()
	record := filepath.Join(t.TempDir(), "npm-record.txt")
	writeFakeNpmForCLI(t, filepath.Join(binDir, fakeNpmExecutableName()), record, 9)

	env := append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := runTspackWithEnv(bin, env, "npm", "ci", "--root", root)
	if err == nil {
		t.Fatalf("tspack npm unexpectedly succeeded:\n%s", out)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 9 {
		t.Fatalf("exit code = %v, want 9; output:\n%s", err, out)
	}
}

func TestNpmCommandNoArgsAndMissingNpmDiagnostics(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)

	out, err := runTspackWithEnv(bin, os.Environ(), "npm")
	if err == nil || !strings.Contains(out, "usage: tspack npm <npm-args...>") {
		t.Fatalf("no-args output was not usage failure: err=%v\n%s", err, out)
	}

	env := append(os.Environ(), "PATH="+t.TempDir())
	out, err = runTspackWithEnv(bin, env, "npm", "install")
	if err == nil || !strings.Contains(out, "TSPACK_NPM_NOT_FOUND") {
		t.Fatalf("missing npm output was not diagnostic failure: err=%v\n%s", err, out)
	}
}

func TestRunCommandDoesNotFallbackToPackageJSONScripts(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), "{\"scripts\":{\"build\":\"echo should-not-run\"}}\n")

	out, err := runTspackWithEnv(bin, os.Environ(), "run", "build", "--root", root)
	if err == nil {
		t.Fatalf("tspack run unexpectedly fell back to package.json script:\n%s", out)
	}
	if strings.Contains(out, "should-not-run") {
		t.Fatalf("package.json script appears to have run:\n%s", out)
	}
}

func skipPosixFakeNpmTestOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake npm runner is POSIX-only")
	}
}

func runTspackWithEnv(bin string, env []string, args ...string) (string, error) {
	cmd := newInProcessCommand(args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	writeFileMode(t, path, content, 0o644)
}

func writeFileMode(t *testing.T, path string, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func assertPathDoesNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("unexpected path exists: %s", path)
	}
}

func writeFakeNpmForCLI(t *testing.T, path string, record string, exitCode int) {
	t.Helper()
	content := "#!/bin/sh\n" +
		"printf 'cwd=%s\\n' \"$(pwd)\" > '" + record + "'\n" +
		"i=0\n" +
		"for arg in \"$@\"; do\n" +
		"  printf 'arg%s=%s\\n' \"$i\" \"$arg\" >> '" + record + "'\n" +
		"  i=$((i + 1))\n" +
		"done\n" +
		"exit " + strconv.Itoa(exitCode) + "\n"
	writeFileMode(t, path, content, 0o755)
}

func fakeNpmExecutableName() string {
	if runtime.GOOS == "windows" {
		return "npm.cmd"
	}
	return "npm"
}
