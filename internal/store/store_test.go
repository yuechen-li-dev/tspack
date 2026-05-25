package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
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
