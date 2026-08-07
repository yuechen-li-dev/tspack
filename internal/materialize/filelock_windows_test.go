//go:build windows

package materialize

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
)

func TestIsTransientMaterializeLockErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "access denied", err: syscall.Errno(5), want: true},
		{name: "sharing violation", err: syscall.Errno(32), want: true},
		{name: "lock violation", err: syscall.Errno(33), want: true},
		{name: "directory not empty", err: syscall.Errno(145), want: true},
		{name: "wrapped path error", err: &os.PathError{Op: "remove", Path: "x", Err: syscall.Errno(32)}, want: true},
		{name: "not exists", err: syscall.Errno(2), want: false},
	}
	for _, tc := range cases {
		if got := isTransientMaterializeLockErr(tc.err); got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestRetryMaterializeFileOpRetriesTransientWindowsLock(t *testing.T) {
	attempts := 0
	err := retryMaterializeFileOp("remove", "node_modules\\vite\\esbuild.exe", func() error {
		attempts++
		if attempts < 4 {
			return syscall.Errno(32)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected retry success, got %v", err)
	}
	if attempts != 4 {
		t.Fatalf("expected 4 attempts, got %d", attempts)
	}
}

func TestRetryMaterializeFileOpReturnsLockedErrorAfterBoundedRetries(t *testing.T) {
	err := retryMaterializeFileOp("remove", "node_modules\\vite\\esbuild.exe", func() error {
		return syscall.Errno(32)
	})
	var locked *materializeFileLockError
	if !errors.As(err, &locked) {
		t.Fatalf("expected materializeFileLockError, got %T %v", err, err)
	}
	if locked.Op != "remove" {
		t.Fatalf("expected remove op, got %q", locked.Op)
	}
	if locked.Path != "node_modules\\vite\\esbuild.exe" {
		t.Fatalf("unexpected path: %q", locked.Path)
	}
	if locked.Attempts < 2 {
		t.Fatalf("expected multiple attempts, got %d", locked.Attempts)
	}
}

func TestMaterializeDiagnosticFromLockedError(t *testing.T) {
	previousOwners := materializeLockOwners
	materializeLockOwners = func(string) []materializeLockOwner {
		return []materializeLockOwner{{PID: 4242, Name: "esbuild.exe"}}
	}
	t.Cleanup(func() { materializeLockOwners = previousOwners })
	diag := materializeDiagnosticFromError(
		&materializeFileLockError{
			Op:       "remove",
			Path:     "node_modules\\vite\\node_modules\\esbuild\\esbuild.exe",
			Attempts: 9,
			Err:      syscall.Errno(32),
		},
		"npm:vite@5.4.21",
		"node_modules\\vite",
		"vite",
	)
	if diag.Code != "TSPACK_MATERIALIZE_FILE_LOCKED" {
		t.Fatalf("unexpected diagnostic code: %q", diag.Code)
	}
	if diag.File == "" || !strings.Contains(diag.File, "esbuild.exe") {
		t.Fatalf("expected locked file path in File, got %q", diag.File)
	}
	if len(diag.Fixes) == 0 {
		t.Fatal("expected actionable fixes")
	}
	if !strings.Contains(strings.Join(diag.Details, "\n"), "lockOwner=esbuild.exe (pid=4242)") {
		t.Fatalf("expected lock owner details, got %#v", diag.Details)
	}
}
