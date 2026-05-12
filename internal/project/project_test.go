package project

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/lockfile"
	"github.com/tspack/tspack/internal/resolver"
	"github.com/tspack/tspack/internal/store"
)

type fakeClient struct {
	meta map[string]*resolver.PackageMetadata
	tar  map[string][]byte
}

func (f *fakeClient) PackageMetadata(_ context.Context, name string) (*resolver.PackageMetadata, error) {
	m, ok := f.meta[name]
	if !ok {
		return nil, errors.New("not found")
	}
	return m, nil
}
func (f *fakeClient) Tarball(_ context.Context, url string) ([]byte, error) {
	b, ok := f.tar[url]
	if !ok {
		return nil, errors.New("not found")
	}
	return b, nil
}

func TestCheckDoesNotMutateLockfile(t *testing.T) {
	dir := t.TempDir()
	irPath := writeIR(t, dir, simpleIR())
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"}, Targets: []lockfile.Target{{Package: "app", Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts"}}}
	b, _ := lockfile.Marshal(lf)
	lockPath := filepath.Join(dir, "ts-lock.toml")
	_ = os.WriteFile(lockPath, b, 0o644)
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = irPath
	res := Check(opts)
	if hasErrCode(res.Diagnostics, "TSPACK_CHECK_FAILED") {
		t.Fatalf("unexpected failure: %#v", res.Diagnostics)
	}
	after, _ := os.ReadFile(lockPath)
	if !bytes.Equal(b, after) {
		t.Fatalf("lockfile mutated")
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("node_modules should not exist")
	}
}

func TestCheckMissingLockfileWarning(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, simpleIR())
	res := Check(opts)
	if !hasErrCode(res.Diagnostics, "TSPACK_CHECK_LOCKFILE_MISSING") {
		t.Fatalf("missing warning")
	}
	if _, err := os.Stat(filepath.Join(dir, "ts-lock.toml")); !os.IsNotExist(err) {
		t.Fatalf("check created lockfile")
	}
}

func TestUpdateDeterministicAndNoNodeModules(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	ir["packages"].([]map[string]any)[0]["dependencies"] = []map[string]any{{"key": "dep-a", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "dep-a", "range": "1.0.0"}}}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	opts.ResolverClient = buildRegistry()
	r1 := Update(opts)
	if hasErrors(r1.Diagnostics) {
		t.Fatalf("update failed: %#v", r1.Diagnostics)
	}
	b1, _ := os.ReadFile(opts.LockfilePath)
	r2 := Update(opts)
	if hasErrors(r2.Diagnostics) {
		t.Fatalf("update2 failed: %#v", r2.Diagnostics)
	}
	b2, _ := os.ReadFile(opts.LockfilePath)
	if !bytes.Equal(b1, b2) {
		t.Fatalf("nondeterministic lockfile")
	}
	if r2.LockDiff == nil || len(r2.LockDiff.PackagesAdded)+len(r2.LockDiff.PackagesRemoved)+len(r2.LockDiff.PackagesChanged) != 0 {
		t.Fatalf("expected empty package diff")
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("update created node_modules")
	}
}

func TestSyncMutationGuardAndMaterialization(t *testing.T) {
	dir := t.TempDir()
	irPath := writeIR(t, dir, simpleIR())
	st, _ := store.Open(filepath.Join(dir, ".tspack", "store"))
	aHash := putPkg(t, st, "dep-a", "1.0.0")
	lHash := putPkg(t, st, "left-pad", "1.2.0")
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Packages: []lockfile.Package{{ID: "npm:dep-a@1.0.0", Name: "dep-a", Version: "1.0.0", Source: "npm", Hash: aHash}, {ID: "npm:left-pad@1.2.0", Name: "left-pad", Version: "1.2.0", Source: "npm", Hash: lHash}},
		Edges:    []lockfile.Edge{{From: "app:target:core", To: "npm:dep-a@1.0.0", Kind: "runtime"}, {From: "npm:dep-a@1.0.0", To: "npm:left-pad@1.2.0", Kind: "runtime"}},
		Targets:  []lockfile.Target{{Package: "app", Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts"}},
	}
	lb, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(filepath.Join(dir, "ts-lock.toml"), lb, 0o644)
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = irPath
	res := Sync(opts, false)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("sync failed: %#v", res.Diagnostics)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "ts-lock.toml"))
	if !bytes.Equal(lb, after) {
		t.Fatalf("lock mutated")
	}
	mustExist(t, filepath.Join(dir, "node_modules", "dep-a", "package.json"))
	mustExist(t, filepath.Join(dir, "node_modules", "dep-a", "node_modules", "left-pad", "package.json"))
	if _, err := os.Stat(filepath.Join(dir, "node_modules", "left-pad")); !os.IsNotExist(err) {
		t.Fatalf("phantom root transitive present")
	}
}

func TestSyncMissingAndStaleLockfile(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, simpleIR())
	if !hasErrCode(Sync(opts, false).Diagnostics, "TSPACK_SYNC_LOCKFILE_MISSING") {
		t.Fatalf("missing expected code")
	}
	_ = os.WriteFile(opts.LockfilePath, []byte("bad"), 0o644)
	before, _ := os.ReadFile(opts.LockfilePath)
	res := Sync(opts, false)
	if !hasErrCode(res.Diagnostics, "TSPACK_SYNC_LOCKFILE_STALE") && !hasErrCode(res.Diagnostics, "TSPACK_LOCK_INVALID_TOML") {
		t.Fatalf("expected stale lock diagnostics")
	}
	after, _ := os.ReadFile(opts.LockfilePath)
	if !bytes.Equal(before, after) {
		t.Fatalf("stale lock mutated")
	}
}

func TestFrontendBridgeMissingCLI(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.ManifestPath = filepath.Join(dir, "manifest.tsx")
	_ = os.WriteFile(opts.ManifestPath, []byte("export default {}"), 0o644)
	res := Check(opts)
	if !hasErrCode(res.Diagnostics, "TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED") {
		t.Fatalf("expected frontend failure")
	}
}

func writeIR(t *testing.T, dir string, ir map[string]any) string {
	t.Helper()
	_ = os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	_ = os.MkdirAll(filepath.Join(dir, "dist"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "src", "index.ts"), []byte("export const x = 1;\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "dist", "index.d.ts"), []byte("export declare const x: number;\n"), 0o644)
	b, _ := json.Marshal(ir)
	p := filepath.Join(dir, "manifest.ir.json")
	_ = os.WriteFile(p, b, 0o644)
	return p
}
func simpleIR() map[string]any {
	return map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{{"name": "app", "version": "1.0.0", "kind": "library", "dependencies": []map[string]any{}, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}}}}
}
func hasErrCode(diags []diag.Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}
func mustExist(t *testing.T, p string) {
	t.Helper()
	if _, e := os.Stat(p); e != nil {
		t.Fatalf("missing %s", p)
	}
}

func buildRegistry() *fakeClient {
	tarballs := map[string][]byte{}
	mk := func(name, version string, deps map[string]string) resolver.PackageVersion {
		tgz := tarball(name, version, deps)
		u := "https://example.invalid/" + name + "-" + version + ".tgz"
		tarballs[u] = tgz
		sum := sha512sum(tgz)
		return resolver.PackageVersion{Name: name, Version: version, Dependencies: deps, Dist: resolver.PackageDist{Tarball: u, Integrity: "sha512-" + base64.StdEncoding.EncodeToString(sum)}}
	}
	meta := map[string]*resolver.PackageMetadata{"dep-a": {Name: "dep-a", Versions: map[string]resolver.PackageVersion{"1.0.0": mk("dep-a", "1.0.0", map[string]string{"left-pad": "1.2.0"})}}, "left-pad": {Name: "left-pad", Versions: map[string]resolver.PackageVersion{"1.2.0": mk("left-pad", "1.2.0", nil)}}}
	return &fakeClient{meta: meta, tar: tarballs}
}
func sha512sum(b []byte) []byte { h := sha512.Sum512(b); return h[:] }
func tarball(name, version string, deps map[string]string) []byte {
	pj := map[string]any{"name": name, "version": version}
	if len(deps) > 0 {
		pj["dependencies"] = deps
	}
	jb, _ := json.Marshal(pj)
	buf := bytes.NewBuffer(nil)
	gz := gzip.NewWriter(buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "package/package.json", Mode: 0o644, Size: int64(len(jb))})
	_, _ = tw.Write(jb)
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}
func putPkg(t *testing.T, st *store.Store, name, version string) string {
	d := t.TempDir()
	_ = os.WriteFile(filepath.Join(d, "package.json"), []byte("{\"name\":\""+name+"\",\"version\":\""+version+"\"}"), 0o644)
	ref, diags := st.PutArtifact(store.Artifact{ID: "npm:" + name + "@" + version, Name: name, Version: version, Source: "npm", Kind: store.ArtifactPathTree, RootDir: d})
	if len(diags) > 0 {
		t.Fatalf("put artifact diags: %#v", diags)
	}
	return ref.Hash
}
