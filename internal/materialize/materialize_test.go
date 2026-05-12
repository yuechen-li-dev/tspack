package materialize

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tspack/tspack/internal/lockfile"
	"github.com/tspack/tspack/internal/store"
)

func TestMaterializeStrictLayout(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	left := putPkg(t, s, "npm:left-pad@1.2.0", "left-pad")
	depA := putPkg(t, s, "npm:dep-a@1.0.0", "dep-a")
	typescript := putPkg(t, s, "npm:typescript@5.0.0", "typescript")
	scope := putPkg(t, s, "npm:@scope/pkg@1.0.0", "@scope/pkg")
	lf := &lockfile.Lockfile{Packages: []lockfile.Package{
		{ID: "npm:left-pad@1.2.0", Name: "left-pad", Source: "npm", Hash: left},
		{ID: "npm:dep-a@1.0.0", Name: "dep-a", Source: "npm", Hash: depA},
		{ID: "npm:typescript@5.0.0", Name: "typescript", Source: "npm", Hash: typescript},
		{ID: "npm:@scope/pkg@1.0.0", Name: "@scope/pkg", Source: "npm", Hash: scope},
	}, Edges: []lockfile.Edge{
		{From: "app:target:core", To: "npm:dep-a@1.0.0", Kind: "runtime"},
		{From: "npm:dep-a@1.0.0", To: "npm:left-pad@1.2.0", Kind: "runtime"},
		{From: "app:tool", To: "npm:typescript@5.0.0", Kind: "tool"},
		{From: "app:target:core", To: "npm:@scope/pkg@1.0.0", Kind: "runtime"},
	}}
	res := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeCopy}})
	if len(res.Diagnostics) > 0 {
		t.Fatalf("diags: %#v", res.Diagnostics)
	}
	mustExist(t, filepath.Join(ws, "node_modules", "dep-a", "package.json"))
	mustExist(t, filepath.Join(ws, "node_modules", "dep-a", "node_modules", "left-pad", "package.json"))
	mustExist(t, filepath.Join(ws, "node_modules", "typescript", "package.json"))
	mustExist(t, filepath.Join(ws, "node_modules", "@scope", "pkg", "package.json"))
	if _, err := os.Stat(filepath.Join(ws, "node_modules", "left-pad", "package.json")); err == nil {
		t.Fatal("phantom root dep should not exist")
	}
}

func TestDiagnosticsAndCleanSafety(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	okHash := putPkg(t, s, "npm:ok@1.0.0", "ok")
	lf := &lockfile.Lockfile{Packages: []lockfile.Package{
		{ID: "npm:ok@1.0.0", Name: "ok", Source: "npm", Hash: okHash},
		{ID: "npm:nohash@1", Name: "nohash", Source: "npm"},
		{ID: "npm:bad@1", Name: "../evil", Source: "npm", Hash: okHash},
		{ID: "npm:missing@1", Name: "missing", Source: "npm", Hash: "sha256:deadbeef"},
	}, Edges: []lockfile.Edge{
		{From: "app:target:core", To: "npm:ok@1.0.0", Kind: "runtime"},
		{From: "app:target:core", To: "npm:nohash@1", Kind: "runtime"},
		{From: "app:target:core", To: "npm:bad@1", Kind: "runtime"},
		{From: "app:target:core", To: "npm:missing@1", Kind: "runtime"},
		{From: "app:target:core", To: "npm:unknown@1", Kind: "runtime"},
	}}
	res := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeCopy}})
	assertCode(t, res, "TSPACK_MATERIALIZE_PACKAGE_HASH_MISSING")
	assertCode(t, res, "TSPACK_MATERIALIZE_INVALID_PACKAGE_NAME")
	assertCode(t, res, "TSPACK_MATERIALIZE_MISSING_STORE_ARTIFACT")
	assertCode(t, res, "TSPACK_MATERIALIZE_EDGE_UNKNOWN_PACKAGE")

	nm := filepath.Join(ws, "node_modules")
	_ = os.Remove(filepath.Join(nm, markerFile))
	cleanRes := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: &lockfile.Lockfile{}, Store: s, Options: Options{Clean: true, LinkMode: LinkModeCopy}})
	assertCode(t, cleanRes, "TSPACK_MATERIALIZE_CLEAN_REFUSED")
}

func TestDeterministicAndLinkMode(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	h := putPkg(t, s, "workspace:@scope/pkg", "@scope/pkg")
	lf := &lockfile.Lockfile{Packages: []lockfile.Package{{ID: "workspace:@scope/pkg", Name: "@scope/pkg", Source: "workspace", Hash: h}}, Edges: []lockfile.Edge{{From: "app:target:core", To: "workspace:@scope/pkg", Kind: "runtime"}}}
	m := NodeModulesMaterializer{}
	first := m.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeAuto}})
	second := m.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{Clean: true, LinkMode: LinkModeCopy}})
	if len(first.Diagnostics)+len(second.Diagnostics) > 0 {
		t.Fatalf("diags: %#v %#v", first.Diagnostics, second.Diagnostics)
	}
	if len(first.Written) != len(second.Written) || first.Written[0].Path != second.Written[0].Path {
		t.Fatal("non-deterministic written paths")
	}
	bad := m.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeSymlink}})
	assertCode(t, bad, "TSPACK_MATERIALIZE_UNSUPPORTED_LINK_MODE")
}

func putPkg(t *testing.T, s *store.Store, id, name string) string {
	t.Helper()
	d := t.TempDir()
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "package.json"), []byte("{\"name\":\""+name+"\"}"), 0o644); err != nil {
		t.Fatal(err)
	}
	ref, diags := s.PutArtifact(store.Artifact{ID: id, Name: name, Kind: store.ArtifactPathTree, RootDir: d})
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	return ref.Hash
}

func mustExist(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
}

func assertCode(t *testing.T, res Result, code string) {
	t.Helper()
	for _, d := range res.Diagnostics {
		if d.Code == code {
			return
		}
	}
	t.Fatalf("missing diagnostic %s: %#v", code, res.Diagnostics)
}
