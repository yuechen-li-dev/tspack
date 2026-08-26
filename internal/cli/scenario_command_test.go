package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveScenarioRunnerPathPrefersExecutableRelativeTool(t *testing.T) {
	temporaryRoot := t.TempDir()
	executablePath := filepath.Join(temporaryRoot, "tspack.exe")
	runnerPath := filepath.Join(temporaryRoot, "tools", "run-browser-scenarios.mjs")
	if err := os.MkdirAll(filepath.Dir(runnerPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runnerPath, []byte("export {};\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveScenarioRunnerPathFrom(executablePath, temporaryRoot)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != runnerPath {
		t.Fatalf("runner path = %q, want %q", resolved, runnerPath)
	}
}

func TestResolveScenarioRunnerPathReportsUnavailableRunner(t *testing.T) {
	temporaryRoot := t.TempDir()
	_, err := resolveScenarioRunnerPathFrom(filepath.Join(temporaryRoot, "tspack.exe"), temporaryRoot)
	if err == nil {
		t.Fatal("expected runner resolution to fail")
	}
}
