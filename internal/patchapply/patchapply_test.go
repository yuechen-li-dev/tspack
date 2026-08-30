package patchapply

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaterializeAppliesExactHunkAndCanonicalizesPatchDigest(t *testing.T) {
	source := t.TempDir()
	destination := filepath.Join(t.TempDir(), "patched")
	writeTestFile(t, filepath.Join(source, "dist", "index.js"), "first\nold\nlast\n")
	patchLF := []byte("diff --git a/dist/index.js b/dist/index.js\n--- a/dist/index.js\n+++ b/dist/index.js\n@@ -1,3 +1,3 @@\n first\n-old\n+new\n last\n")
	patchCRLF := []byte(strings.ReplaceAll(string(patchLF), "\n", "\r\n"))
	if Digest(patchLF) != Digest(patchCRLF) {
		t.Fatal("patch digest depends on host line endings")
	}
	if err := Materialize(source, destination, patchCRLF); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(destination, "dist", "index.js"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "first\nnew\nlast\n" {
		t.Fatalf("patched contents = %q", got)
	}
	original, _ := os.ReadFile(filepath.Join(source, "dist", "index.js"))
	if string(original) != "first\nold\nlast\n" {
		t.Fatal("source realization was mutated")
	}
}

func TestMaterializeFailsClosedOnOffsetAndUnsafeOrUnsupportedChanges(t *testing.T) {
	tests := []struct {
		name  string
		patch string
		want  string
	}{
		{name: "offset", patch: "--- a/file.txt\n+++ b/file.txt\n@@ -2,1 +2,1 @@\n-old\n+new\n", want: "mismatch"},
		{name: "traversal", patch: "--- a/../file.txt\n+++ b/../file.txt\n@@ -1,1 +1,1 @@\n-old\n+new\n", want: "unsafe"},
		{name: "windows drive", patch: "--- a/C:/outside.txt\n+++ b/C:/outside.txt\n@@ -1,1 +1,1 @@\n-old\n+new\n", want: "unsafe"},
		{name: "create", patch: "--- /dev/null\n+++ b/new.txt\n@@ -0,0 +1,1 @@\n+new\n", want: "unsupported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := t.TempDir()
			writeTestFile(t, filepath.Join(source, "file.txt"), "old\n")
			err := Materialize(source, filepath.Join(t.TempDir(), "patched"), []byte(test.patch))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestMaterializeRejectsSymlinkTargets(t *testing.T) {
	source := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	writeTestFile(t, outside, "old\n")
	if err := os.Symlink(outside, filepath.Join(source, "file.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	err := Materialize(source, filepath.Join(t.TempDir(), "patched"), []byte("--- a/file.txt\n+++ b/file.txt\n@@ -1,1 +1,1 @@\n-old\n+new\n"))
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error = %v", err)
	}
}

func writeTestFile(t *testing.T, filename string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
