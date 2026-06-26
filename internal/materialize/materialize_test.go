package materialize

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/store"
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

func TestRootBinMaterializationAndStrictness(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	tool := putPkgWithPackageJSON(t, s, "npm:tool@1.0.0", "tool", `{"name":"tool","bin":{"tool":"bin/tool.js"}}`, []fileSpec{{path: "bin/tool.js", content: "#!/usr/bin/env node\nconsole.log('tool')\n", mode: 0o755}})
	transitive := putPkgWithPackageJSON(t, s, "npm:transitive@1.0.0", "transitive", `{"name":"transitive","bin":{"hidden":"bin/hidden.js"}}`, []fileSpec{{path: "bin/hidden.js", content: "#!/usr/bin/env node\n", mode: 0o755}})
	lf := &lockfile.Lockfile{Packages: []lockfile.Package{
		{ID: "npm:tool@1.0.0", Name: "tool", Hash: tool},
		{ID: "npm:transitive@1.0.0", Name: "transitive", Hash: transitive},
	}, Edges: []lockfile.Edge{{From: "app:tool", To: "npm:tool@1.0.0", Kind: "tool"}, {From: "npm:tool@1.0.0", To: "npm:transitive@1.0.0", Kind: "runtime"}}}
	res := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeCopy}})
	if len(res.Diagnostics) > 0 {
		t.Fatalf("diags: %#v", res.Diagnostics)
	}
	mustExist(t, filepath.Join(ws, "node_modules", ".bin", "tool"))
	if _, err := os.Stat(filepath.Join(ws, "node_modules", ".bin", "hidden")); err == nil {
		t.Fatal("transitive-only bin should not be root-exposed")
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(filepath.Join(ws, "node_modules", "tool", "bin", "tool.js"))
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm()&0o111 == 0 {
			t.Fatalf("expected executable mode, got %o", st.Mode().Perm())
		}
	}
}

func TestBiomeStyleBinMaterializationAndStrictness(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	biomePackageJSON := `{"name":"@biomejs/biome","bin":{"biome":"bin/biome"}}`
	hiddenPackageJSON := `{"name":"@example/hidden-biome","bin":{"biome":"bin/biome"}}`
	biome := putPkgWithPackageJSON(
		t,
		s,
		"npm:@biomejs/biome@1.9.4",
		"@biomejs/biome",
		biomePackageJSON,
		[]fileSpec{{path: "bin/biome", content: "#!/bin/sh\necho biome\n", mode: 0o755}},
	)
	hidden := putPkgWithPackageJSON(
		t,
		s,
		"npm:@example/hidden-biome@1.0.0",
		"@example/hidden-biome",
		hiddenPackageJSON,
		[]fileSpec{{path: "bin/biome", content: "#!/bin/sh\necho hidden\n", mode: 0o755}},
	)
	lf := &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:@biomejs/biome@1.9.4", Name: "@biomejs/biome", Hash: biome},
			{ID: "npm:@example/hidden-biome@1.0.0", Name: "@example/hidden-biome", Hash: hidden},
		},
		Edges: []lockfile.Edge{
			{From: "app:tool", To: "npm:@biomejs/biome@1.9.4", Kind: "tool"},
			{From: "npm:@biomejs/biome@1.9.4", To: "npm:@example/hidden-biome@1.0.0", Kind: "runtime"},
		},
	}

	res := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeCopy}})
	if len(res.Diagnostics) > 0 {
		t.Fatalf("diags: %#v", res.Diagnostics)
	}

	directBin := filepath.Join(ws, "node_modules", "@biomejs", "biome", "bin", "biome")
	mustExist(t, directBin)
	if runtime.GOOS != "windows" {
		st, err := os.Stat(directBin)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm()&0o111 == 0 {
			t.Fatalf("expected Biome direct bin to be executable, got %o", st.Mode().Perm())
		}
	}
	rootBin := filepath.Join(ws, "node_modules", ".bin", "biome")
	mustExist(t, rootBin)
	if target, err := filepath.EvalSymlinks(rootBin); err == nil && target != directBin {
		t.Fatalf("expected root Biome bin to resolve to direct package bin %q, got %q", directBin, target)
	}
	transitiveBin := filepath.Join(ws, "node_modules", "@biomejs", "biome", "node_modules", "@example", "hidden-biome", "bin", "biome")
	mustExist(t, transitiveBin)
	if target, err := filepath.EvalSymlinks(rootBin); err == nil && target == transitiveBin {
		t.Fatal("root Biome bin should not resolve to transitive Biome-like bin")
	}
}

func TestBinConflictDiagnostic(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	a := putPkgWithPackageJSON(t, s, "npm:a@1.0.0", "a", `{"name":"a","bin":{"same":"bin/a.js"}}`, []fileSpec{{path: "bin/a.js", content: "", mode: 0o755}})
	b := putPkgWithPackageJSON(t, s, "npm:b@1.0.0", "b", `{"name":"b","bin":{"same":"bin/b.js"}}`, []fileSpec{{path: "bin/b.js", content: "", mode: 0o755}})
	lf := &lockfile.Lockfile{Packages: []lockfile.Package{{ID: "npm:a@1.0.0", Name: "a", Hash: a}, {ID: "npm:b@1.0.0", Name: "b", Hash: b}}, Edges: []lockfile.Edge{{From: "app:tool", To: "npm:a@1.0.0", Kind: "tool"}, {From: "app:tool", To: "npm:b@1.0.0", Kind: "tool"}}}
	res := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeCopy}})
	assertCode(t, res, "TSPACK_MATERIALIZE_BIN_CONFLICT")
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

func TestMaterializeCircularDependenciesStayBounded(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	aHash := putPkg(t, s, "npm:a@1.0.0", "a")
	bHash := putPkg(t, s, "npm:b@1.0.0", "b")
	lf := &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:a@1.0.0", Name: "a", Hash: aHash},
			{ID: "npm:b@1.0.0", Name: "b", Hash: bHash},
		},
		Edges: []lockfile.Edge{
			{From: "app:target:core", To: "npm:a@1.0.0", Kind: "runtime"},
			{From: "npm:a@1.0.0", To: "npm:b@1.0.0", Kind: "runtime"},
			{From: "npm:b@1.0.0", To: "npm:a@1.0.0", Kind: "runtime"},
		},
	}

	res := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeCopy}})
	if len(res.Diagnostics) > 0 {
		t.Fatalf("diags: %#v", res.Diagnostics)
	}

	nm := filepath.Join(ws, "node_modules")
	mustExist(t, filepath.Join(nm, "a", "package.json"))
	mustExist(t, filepath.Join(nm, "a", "node_modules", "b", "package.json"))
	mustExist(t, filepath.Join(nm, "a", "node_modules", "b", "node_modules", "a", "package.json"))
	assertNoRepeatedPackagePattern(t, nm, []string{"a", "b", "a", "b"})
	assertMaxNodeModulesDepth(t, nm, 3)
}

func TestMaterializeSharedDependencyDeterministicAndBounded(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	aHash := putPkg(t, s, "npm:a@1.0.0", "a")
	bHash := putPkg(t, s, "npm:b@1.0.0", "b")
	cHash := putPkg(t, s, "npm:c@1.0.0", "c")
	lf := &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:a@1.0.0", Name: "a", Hash: aHash},
			{ID: "npm:b@1.0.0", Name: "b", Hash: bHash},
			{ID: "npm:c@1.0.0", Name: "c", Hash: cHash},
		},
		Edges: []lockfile.Edge{
			{From: "app:target:core", To: "npm:b@1.0.0", Kind: "runtime"},
			{From: "app:target:core", To: "npm:a@1.0.0", Kind: "runtime"},
			{From: "npm:a@1.0.0", To: "npm:c@1.0.0", Kind: "runtime"},
			{From: "npm:b@1.0.0", To: "npm:c@1.0.0", Kind: "runtime"},
		},
	}

	m := NodeModulesMaterializer{}
	first := m.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeCopy}})
	if len(first.Diagnostics) > 0 {
		t.Fatalf("first diags: %#v", first.Diagnostics)
	}
	firstTree := collectMaterializedPaths(t, filepath.Join(ws, "node_modules"))

	second := m.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{Clean: true, LinkMode: LinkModeCopy}})
	if len(second.Diagnostics) > 0 {
		t.Fatalf("second diags: %#v", second.Diagnostics)
	}
	secondTree := collectMaterializedPaths(t, filepath.Join(ws, "node_modules"))

	if len(firstTree) != len(secondTree) {
		t.Fatalf("tree path count changed: first=%d second=%d", len(firstTree), len(secondTree))
	}
	for i := range firstTree {
		if firstTree[i] != secondTree[i] {
			t.Fatalf("tree path changed at %d: %q != %q", i, firstTree[i], secondTree[i])
		}
	}

	nm := filepath.Join(ws, "node_modules")
	mustExist(t, filepath.Join(nm, "a", "node_modules", "c", "package.json"))
	mustExist(t, filepath.Join(nm, "b", "node_modules", "c", "package.json"))
	assertMaxNodeModulesDepth(t, nm, 2)
}

func TestMaterializePathDepthGuardReportsClearDiagnostic(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	packages := make([]lockfile.Package, 0, maxMaterializeDependencyDepth+2)
	edges := []lockfile.Edge{{From: "app:target:core", To: "npm:p00@1.0.0", Kind: "runtime"}}
	for i := 0; i < maxMaterializeDependencyDepth+2; i++ {
		name := fmt.Sprintf("p%02d", i)
		id := fmt.Sprintf("npm:%s@1.0.0", name)
		hash := putPkg(t, s, id, name)
		packages = append(packages, lockfile.Package{ID: id, Name: name, Hash: hash})
		if i > 0 {
			from := fmt.Sprintf("npm:p%02d@1.0.0", i-1)
			edges = append(edges, lockfile.Edge{From: from, To: id, Kind: "runtime"})
		}
	}
	lf := &lockfile.Lockfile{Packages: packages, Edges: edges}

	res := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeCopy}})
	assertCode(t, res, "TSPACK_MATERIALIZE_PATH_DEPTH_EXCEEDED")
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

func collectMaterializedPaths(t *testing.T, root string) []string {
	t.Helper()
	paths := []string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func assertNoRepeatedPackagePattern(t *testing.T, root string, pattern []string) {
	t.Helper()
	paths := collectMaterializedPaths(t, root)
	needle := strings.Join(pattern, "/node_modules/")
	for _, path := range paths {
		if strings.Contains(path, needle) {
			t.Fatalf("found repeated package pattern %q in %q", needle, path)
		}
	}
}

func assertMaxNodeModulesDepth(t *testing.T, root string, maxDepth int) {
	t.Helper()
	paths := collectMaterializedPaths(t, root)
	for _, path := range paths {
		depth := strings.Count(path, "node_modules") + 1
		if path == "." {
			depth = 0
		}
		if depth > maxDepth {
			t.Fatalf("materialized path depth %d exceeds max %d: %s", depth, maxDepth, path)
		}
		if len(path) >= 240 {
			t.Fatalf("materialized relative path should stay comfortably below Windows MAX_PATH: len=%d path=%s", len(path), path)
		}
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

type fileSpec struct {
	path    string
	content string
	mode    os.FileMode
}

func putPkgWithPackageJSON(t *testing.T, s *store.Store, id, name, packageJSON string, files []fileSpec) string {
	t.Helper()
	d := t.TempDir()
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "package.json"), []byte(packageJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		full := filepath.Join(d, file.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(file.content), file.mode); err != nil {
			t.Fatal(err)
		}
	}
	ref, diags := s.PutArtifact(store.Artifact{ID: id, Name: name, Kind: store.ArtifactPathTree, RootDir: d})
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	return ref.Hash
}
