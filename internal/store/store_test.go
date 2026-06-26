package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPutTarballAndGetVerify(t *testing.T) {
	d := t.TempDir()
	s, _ := Open(d)
	tgz := makeTarball(t, map[string]string{"package/package.json": `{"name":"a","version":"1.0.0"}`})
	ref, diags := s.PutArtifact(Artifact{ID: "npm:a@1.0.0", Kind: ArtifactNPMTarball, Bytes: tgz, Name: "a", Version: "1.0.0"})
	if len(diags) > 0 {
		t.Fatalf("diags: %v", diags)
	}
	if !s.Has(ref.Hash) {
		t.Fatalf("expected has")
	}
	if _, err := os.Stat(filepath.Join(ref.ExtractedPath, "package.json")); err != nil {
		t.Fatal(err)
	}
	if got := s.Verify(ref.Hash); len(got) > 0 {
		t.Fatalf("verify: %v", got)
	}
}

func TestDedupAndMismatch(t *testing.T) {
	d := t.TempDir()
	s, _ := Open(d)
	tgz := makeTarball(t, map[string]string{"package/package.json": "{}"})
	ref1, _ := s.PutArtifact(Artifact{Kind: ArtifactNPMTarball, Bytes: tgz})
	ref2, _ := s.PutArtifact(Artifact{Kind: ArtifactNPMTarball, Bytes: tgz})
	if ref1.StorePath != ref2.StorePath {
		t.Fatal("not deduped")
	}
	_, diags := s.PutArtifact(Artifact{Kind: ArtifactNPMTarball, Bytes: tgz, Hash: "sha256:deadbeef"})
	if len(diags) == 0 || diags[0].Code != "TSPACK_STORE_HASH_MISMATCH" {
		t.Fatal(diags)
	}
}

func TestDirectoryArtifactAndStability(t *testing.T) {
	base := t.TempDir()
	a := filepath.Join(base, "a")
	b := filepath.Join(base, "b")
	_ = os.MkdirAll(filepath.Join(a, ".git"), 0o755)
	_ = os.MkdirAll(filepath.Join(a, "node_modules"), 0o755)
	_ = os.WriteFile(filepath.Join(a, "z.txt"), []byte("z"), 0o644)
	_ = os.WriteFile(filepath.Join(a, "a.txt"), []byte("a"), 0o644)
	_ = os.WriteFile(filepath.Join(a, ".git", "x"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(a, "node_modules", "x"), []byte("x"), 0o644)
	_ = os.MkdirAll(b, 0o755)
	_ = os.WriteFile(filepath.Join(b, "a.txt"), []byte("a"), 0o644)
	_ = os.WriteFile(filepath.Join(b, "z.txt"), []byte("z"), 0o644)
	s, _ := Open(t.TempDir())
	ref1, d1 := s.PutArtifact(Artifact{Kind: ArtifactPathTree, RootDir: a})
	ref2, d2 := s.PutArtifact(Artifact{Kind: ArtifactPathTree, RootDir: b})
	if len(d1)+len(d2) > 0 {
		t.Fatal(d1, d2)
	}
	if ref1.Hash != ref2.Hash {
		t.Fatalf("expected stable hash")
	}
	if _, err := os.Stat(filepath.Join(ref1.ExtractedPath, ".git", "x")); !os.IsNotExist(err) {
		t.Fatalf(".git should be skipped")
	}
}

func TestTraversalRejected(t *testing.T) {
	s, _ := Open(t.TempDir())
	tgz := makeTarball(t, map[string]string{"../../evil.txt": "x"})
	_, diags := s.PutArtifact(Artifact{Kind: ArtifactNPMTarball, Bytes: tgz})
	if len(diags) == 0 || diags[0].Code != "TSPACK_STORE_TARBALL_PATH_TRAVERSAL" {
		t.Fatal(diags)
	}
}

func TestExecutableModePreserved(t *testing.T) {
	s, _ := Open(t.TempDir())
	tgz := makeTarballWithMode(t, map[string]fileSpec{"package/bin/tool": {content: "#!/bin/sh\necho ok\n", mode: 0o755}})
	ref, diags := s.PutArtifact(Artifact{Kind: ArtifactNPMTarball, Bytes: tgz})
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	st, err := os.Stat(filepath.Join(ref.ExtractedPath, "bin", "tool"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm()&0o111 == 0 {
		t.Fatalf("expected executable bit, got %o", st.Mode().Perm())
	}
}

func TestMetadataDeterministic(t *testing.T) {
	s, _ := Open(t.TempDir())
	tgz := makeTarball(t, map[string]string{"package/package.json": `{"name":"a","version":"1.0.0"}`})
	ref, _ := s.PutArtifact(Artifact{Kind: ArtifactNPMTarball, Bytes: tgz, Metadata: PackageMetadata{Capabilities: nil}})
	b1, _ := os.ReadFile(ref.MetadataPath)
	var m PackageMetadata
	_ = json.Unmarshal(b1, &m)
	ref2, _ := s.PutArtifact(Artifact{Kind: ArtifactNPMTarball, Bytes: tgz, Metadata: m})
	b2, _ := os.ReadFile(ref2.MetadataPath)
	if string(b1) != string(b2) {
		t.Fatal("metadata changed")
	}
}

func makeTarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for p, c := range files {
		_ = tw.WriteHeader(&tar.Header{Name: p, Mode: 0o644, Size: int64(len(c))})
		_, _ = tw.Write([]byte(c))
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

type fileSpec struct {
	content string
	mode    int64
}

func makeTarballWithMode(t *testing.T, files map[string]fileSpec) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for p, c := range files {
		_ = tw.WriteHeader(&tar.Header{Name: p, Mode: c.mode, Size: int64(len(c.content))})
		_, _ = tw.Write([]byte(c.content))
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func TestPathTreeArtifactSkipsInternalDirsAndKeepsDist(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, ".tspack", "store"))
	mustMkdir(t, filepath.Join(root, "tspack-artifacts"))
	mustMkdir(t, filepath.Join(root, "dist"))
	mustWrite(t, filepath.Join(root, ".tspack", "store", "sentinel.txt"), "internal")
	mustWrite(t, filepath.Join(root, "tspack-artifacts", "sentinel.txt"), "internal")
	mustWrite(t, filepath.Join(root, "ts-lock.toml"), "# generated lockfile\n")
	mustWrite(t, filepath.Join(root, "dist", "index.js"), "export const value = 1;\n")
	mustWrite(t, filepath.Join(root, "src.ts"), "export const source = true;\n")

	s, _ := Open(filepath.Join(root, ".tspack", "store"))
	ref, diags := s.PutArtifact(Artifact{Kind: ArtifactPathTree, RootDir: root})
	if len(diags) > 0 {
		t.Fatalf("diags: %v", diags)
	}

	mustExist(t, filepath.Join(ref.ExtractedPath, "src.ts"))
	mustExist(t, filepath.Join(ref.ExtractedPath, "dist", "index.js"))
	mustNotExist(t, filepath.Join(ref.ExtractedPath, ".tspack"))
	mustNotExist(t, filepath.Join(ref.ExtractedPath, "tspack-artifacts"))
	mustNotExist(t, filepath.Join(ref.ExtractedPath, "ts-lock.toml"))
}

func TestPathTreeArtifactHashIgnoresGeneratedLockfile(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "package.json"), `{"name":"local","version":"1.0.0"}`)
	mustWrite(t, filepath.Join(root, "ts-lock.toml"), "# first generated lockfile\n")

	first, err := hashDirectory(root)
	if err != nil {
		t.Fatalf("hash first directory: %v", err)
	}

	mustWrite(t, filepath.Join(root, "ts-lock.toml"), "# second generated lockfile\n")
	second, err := hashDirectory(root)
	if err != nil {
		t.Fatalf("hash second directory: %v", err)
	}

	if first != second {
		t.Fatalf("expected lockfile changes to be ignored, got %q then %q", first, second)
	}
}

func TestCopyTreeRejectsSourceAsDestination(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "package.json"), `{"name":"local","version":"1.0.0"}`)

	diags := copyTree(root, root)
	if len(diags) == 0 || diags[0].Code != "TSPACK_STORE_SELF_COPY_DETECTED" {
		t.Fatalf("expected self-copy diagnostic, got %v", diags)
	}

	mustExist(t, filepath.Join(root, "package.json"))
}

func TestCopyTreeSkipsDestinationInsideSource(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "nested", "dest")
	mustWrite(t, filepath.Join(root, "package.json"), `{"name":"local","version":"1.0.0"}`)
	mustWrite(t, filepath.Join(dest, "old.txt"), "old")

	diags := copyTree(root, dest)
	if len(diags) > 0 {
		t.Fatalf("diags: %v", diags)
	}

	mustExist(t, filepath.Join(dest, "package.json"))
	mustNotExist(t, filepath.Join(dest, "nested", "dest", "old.txt"))
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be absent, got %v", path, err)
	}
}

func TestPutArtifactConcurrentDuplicateTargetSafe(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "package.json"), `{"name":"dup","version":"1.0.0"}`)

	const workers = 8
	errs := make(chan []diag.Diagnostic, workers)
	refs := make(chan StoreRef, workers)
	for i := 0; i < workers; i++ {
		go func() {
			ref, diags := s.PutArtifact(Artifact{ID: "path:dup", Name: "dup", Version: "1.0.0", Source: "path", Kind: ArtifactPathTree, RootDir: root, Metadata: PackageMetadata{Name: "dup", Version: "1.0.0", Source: "path", PackageID: "path:dup"}})
			errs <- diags
			refs <- ref
		}()
	}
	var hash string
	for i := 0; i < workers; i++ {
		if diags := <-errs; len(diags) > 0 {
			t.Fatalf("put artifact failed: %#v", diags)
		}
		ref := <-refs
		if hash == "" {
			hash = ref.Hash
		}
		if ref.Hash != hash {
			t.Fatalf("hash mismatch: %q != %q", ref.Hash, hash)
		}
	}
	if diags := s.Verify(hash); len(diags) > 0 {
		t.Fatalf("verify failed: %#v", diags)
	}
}
