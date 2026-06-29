package npmbridge

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLocateUsesPathFakeNpm(t *testing.T) {
	binDir := t.TempDir()
	writeFakeExecutable(t, filepath.Join(binDir, fakeNpmName()), "#!/bin/sh\nexit 0\n")
	t.Setenv(OverrideEnv, "")
	t.Setenv("PATH", binDir)

	path, err := Locate()
	if err != nil {
		t.Fatalf("Locate failed: %v", err)
	}
	if filepath.Dir(path) != binDir {
		t.Fatalf("Locate returned %q, want directory %q", path, binDir)
	}
}

func TestLocateUsesOverride(t *testing.T) {
	binDir := t.TempDir()
	fake := filepath.Join(binDir, fakeNpmName())
	writeFakeExecutable(t, fake, "#!/bin/sh\nexit 0\n")
	t.Setenv(OverrideEnv, fake)
	t.Setenv("PATH", t.TempDir())

	path, err := Locate()
	if err != nil {
		t.Fatalf("Locate override failed: %v", err)
	}
	if path != fake {
		t.Fatalf("Locate returned %q, want override %q", path, fake)
	}
}

func TestRunPassesArgsCwdAndPreservesExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake npm runner is POSIX-only")
	}
	binDir := t.TempDir()
	projectDir := t.TempDir()
	record := filepath.Join(t.TempDir(), "record.txt")
	fake := filepath.Join(binDir, fakeNpmName())
	writeFakeExecutable(t, fake, "#!/bin/sh\nprintf 'cwd=%s\\n' \"$(pwd)\" > \"$TSPACK_NPM_RECORD\"\nprintf 'args=%s\\n' \"$*\" >> \"$TSPACK_NPM_RECORD\"\nexit 7\n")
	t.Setenv(OverrideEnv, fake)
	t.Setenv("TSPACK_NPM_RECORD", record)

	result, err := Run(Options{Cwd: projectDir, Args: []string{"install", "-D", "vite"}})
	if err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("ExitCode = %d, want 7", result.ExitCode)
	}
	content := readFile(t, record)
	if !strings.Contains(content, "cwd="+projectDir) || !strings.Contains(content, "args=install -D vite") {
		t.Fatalf("record did not contain cwd and args:\n%s", content)
	}
}

func TestLocateMissingReportsNotFound(t *testing.T) {
	t.Setenv(OverrideEnv, "")
	t.Setenv("PATH", t.TempDir())
	_, err := Locate()
	if err == nil {
		t.Fatal("Locate succeeded unexpectedly")
	}
	if _, ok := err.(NotFoundError); !ok {
		t.Fatalf("error type = %T, want NotFoundError", err)
	}
}

func fakeNpmName() string {
	if runtime.GOOS == "windows" {
		return "npm.cmd"
	}
	return "npm"
}

func writeFakeExecutable(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
