package nodecmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocateCachesByResolutionInputs(t *testing.T) {
	firstDirectory := t.TempDir()
	secondDirectory := t.TempDir()
	executableName := "node"
	contents := "#!/bin/sh\nexit 0\n"
	if runtime.GOOS == "windows" {
		executableName = "node.cmd"
		contents = "@echo off\r\nexit /b 0\r\n"
	}
	executablePath := filepath.Join(firstDirectory, executableName)
	if err := os.WriteFile(executablePath, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", firstDirectory)
	first, err := Locate()
	if err != nil {
		t.Fatal(err)
	}
	second, err := Locate()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("cached path = %q, want %q", second, first)
	}

	t.Setenv("PATH", secondDirectory)
	if _, err := Locate(); err == nil {
		t.Fatal("changed PATH reused stale Node path")
	}
}
