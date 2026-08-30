package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/patchapply"
	"github.com/yuechen-li-dev/tspack/internal/store"
)

func TestDeclaredPatchCreatesDistinctDeterministicRealization(t *testing.T) {
	root := t.TempDir()
	patchOne := "patches/one.patch"
	patchTwo := "renamed/two.patch"
	contents := []byte("--- a/index.js\n+++ b/index.js\n@@ -1,1 +1,1 @@\n-raw\n+patched\n")
	writeProjectTestFile(t, filepath.Join(root, filepath.FromSlash(patchOne)), contents)
	writeProjectTestFile(t, filepath.Join(root, filepath.FromSlash(patchTwo)), contents)

	makeGraph := func(patchPath string) *graph.WorkspaceGraph {
		dependency := &graph.DependencyNode{Source: manifest.Source{Kind: "npm", Package: "demo", Range: "^1.0.0"}, Patch: &manifest.PatchDeclaration{Path: patchPath, Version: "1.0.0"}}
		pkg := &graph.PackageNode{Dependencies: []*graph.DependencyNode{dependency}}
		return &graph.WorkspaceGraph{Packages: []*graph.PackageNode{pkg}}
	}
	makeLock := func() *lockfile.Lockfile {
		return &lockfile.Lockfile{Packages: []lockfile.Package{{ID: "npm:demo@1.0.0", Source: "npm", Name: "demo", Version: "1.0.0", Hash: "sha256:source"}}, Edges: []lockfile.Edge{{From: "app:target:core", To: "npm:demo@1.0.0"}}}
	}

	first := makeLock()
	if diagnostics := applyDeclaredPatches(root, makeGraph(patchOne), first, nil); len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	second := makeLock()
	if diagnostics := applyDeclaredPatches(root, makeGraph(patchTwo), second, nil); len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	if first.Packages[0].RealizationID != second.Packages[0].RealizationID {
		t.Fatal("patch path changed content-based realization identity")
	}
	if first.Packages[0].ID == first.Packages[0].SourceID || first.Edges[0].To != first.Packages[0].ID {
		t.Fatal("patched realization did not remain distinct and connected")
	}
}

func TestRawAndDifferentPatchedTreesDoNotAliasInStore(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeProjectTestFile(t, filepath.Join(source, "package.json"), []byte(`{"name":"demo","version":"1.0.0"}`))
	writeProjectTestFile(t, filepath.Join(source, "index.js"), []byte("raw\n"))
	st, err := store.Open(filepath.Join(root, ".tspack", "store"))
	if err != nil {
		t.Fatal(err)
	}
	raw, diagnostics := st.PutArtifact(store.Artifact{ID: "npm:demo@1.0.0", Source: "npm", Name: "demo", Version: "1.0.0", Kind: store.ArtifactPathTree, RootDir: source})
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}

	materialize := func(name string, replacement string) string {
		patchPath := "patches/" + name + ".patch"
		patchBytes := []byte("--- a/index.js\n+++ b/index.js\n@@ -1,1 +1,1 @@\n-raw\n+" + replacement + "\n")
		writeProjectTestFile(t, filepath.Join(root, filepath.FromSlash(patchPath)), patchBytes)
		pkg := lockfile.Package{ID: "npm:demo@1.0.0#patch=" + name, SourceID: "npm:demo@1.0.0", SourceHash: raw.Hash, RealizationID: "npm:demo@1.0.0#patch=" + name, Source: "npm", Name: "demo", Version: "1.0.0", Patch: &lockfile.Patch{Path: patchPath, SHA256: patchapply.Digest(patchBytes), Algorithm: patchapply.Algorithm}}
		hash, patchDiagnostics := populatePatchedPackage(root, st, pkg)
		if len(patchDiagnostics) > 0 {
			t.Fatal(patchDiagnostics)
		}
		return hash
	}
	firstHash := materialize("one", "first")
	secondHash := materialize("two", "second")
	if raw.Hash == firstHash || raw.Hash == secondHash || firstHash == secondHash {
		t.Fatalf("store identities aliased: raw=%s first=%s second=%s", raw.Hash, firstHash, secondHash)
	}
}

func writeProjectTestFile(t *testing.T, filename string, contents []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, contents, 0o644); err != nil {
		t.Fatal(err)
	}
}
