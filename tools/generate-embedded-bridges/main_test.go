package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteFileIfChangedPreservesUnchangedTimestamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generated.go")
	contents := []byte("package generated\n")
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}

	originalTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, originalTime, originalTime); err != nil {
		t.Fatal(err)
	}
	if err := writeFileIfChanged(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(originalTime) {
		t.Fatalf("unchanged generated file timestamp changed: got %v, want %v", info.ModTime(), originalTime)
	}
}

func TestWriteFileIfChangedWritesNewContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generated.go")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeFileIfChanged(path, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "new\n" {
		t.Fatalf("generated contents = %q", contents)
	}
}
