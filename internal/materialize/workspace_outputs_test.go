package materialize

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectWorkspaceBuildArtifactsRefreshesMaterializedOutputs(t *testing.T) {
	workspaceRoot := t.TempDir()
	packageRoot := filepath.Join(workspaceRoot, "packages", "utils")
	materializedRoot := filepath.Join(workspaceRoot, "node_modules", "@scope", "utils")
	if err := os.MkdirAll(filepath.Join(packageRoot, "dist", "chunks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(materializedRoot, "dist", "chunks"), 0o755); err != nil {
		t.Fatal(err)
	}
	currentEntry := filepath.Join(packageRoot, "dist", "index.js")
	currentChunk := filepath.Join(packageRoot, "dist", "chunks", "current.js")
	if err := os.WriteFile(currentEntry, []byte("entry\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(currentChunk, []byte("current\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	staleChunk := filepath.Join(materializedRoot, "dist", "chunks", "stale.js")
	if err := os.WriteFile(staleChunk, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	diagnostics := ProjectWorkspaceBuildArtifacts(
		workspaceRoot,
		packageRoot,
		"@scope/utils",
		[]string{"dist/*.js", "dist/chunks/*.js"},
		[]string{currentEntry, currentChunk},
	)
	if len(diagnostics) != 0 {
		t.Fatalf("projection diagnostics: %#v", diagnostics)
	}
	if _, err := os.Stat(staleChunk); !os.IsNotExist(err) {
		t.Fatalf("stale chunk remains: %v", err)
	}
	assertFileContent(t, filepath.Join(materializedRoot, "dist", "index.js"), "entry\n")
	assertFileContent(t, filepath.Join(materializedRoot, "dist", "chunks", "current.js"), "current\n")
	assertFileContent(t, currentEntry, "entry\n")
}
