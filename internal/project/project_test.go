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
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/lockfile"
	"github.com/tspack/tspack/internal/resolver"
	"github.com/tspack/tspack/internal/store"
)

type fakeClient struct {
	meta             map[string]*resolver.PackageMetadata
	metaErr          map[string]error
	tar              map[string][]byte
	metaCalls        []string
	tarCalls         []string
	packageMetaCalls int
}

func (f *fakeClient) PackageMetadata(_ context.Context, name string) (*resolver.PackageMetadata, error) {
	f.metaCalls = append(f.metaCalls, name)
	f.packageMetaCalls++
	if f.metaErr != nil {
		if err, ok := f.metaErr[name]; ok {
			return nil, err
		}
	}
	m, ok := f.meta[name]
	if !ok {
		return nil, errors.New("not found")
	}
	return m, nil
}
func (f *fakeClient) Tarball(_ context.Context, url string) ([]byte, error) {
	f.tarCalls = append(f.tarCalls, url)
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

func TestUpdateDryRunExistingLockNoChangesLeavesBytesUntouched(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, simpleIR())
	opts.ResolverClient = buildRegistry()
	first := Update(opts)
	if hasErrors(first.Diagnostics) {
		t.Fatalf("initial update failed: %#v", first.Diagnostics)
	}
	before, _ := os.ReadFile(opts.LockfilePath)
	dry := UpdateDryRun(opts)
	if hasErrors(dry.Diagnostics) {
		t.Fatalf("dry-run failed: %#v", dry.Diagnostics)
	}
	after, _ := os.ReadFile(opts.LockfilePath)
	if !bytes.Equal(before, after) {
		t.Fatalf("dry-run mutated lockfile")
	}
	if dry.LockDiff == nil || len(dry.LockDiff.PackagesAdded)+len(dry.LockDiff.PackagesRemoved)+len(dry.LockDiff.PackagesChanged) != 0 {
		t.Fatalf("expected no diff for dry-run")
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

func TestCheckWarnsOnLifecycleCapability(t *testing.T) {
	dir := t.TempDir()
	irPath := writeIR(t, dir, simpleIR())
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Packages: []lockfile.Package{{ID: "npm:dep-a@1.0.0", Name: "dep-a", Version: "1.0.0", Source: "npm", Integrity: "x", Capabilities: []lockfile.Capability{{Kind: "lifecycle-script", Detail: "postinstall"}}}},
		Targets:  []lockfile.Target{{Package: "app", Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts"}},
	}
	b, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(filepath.Join(dir, "ts-lock.toml"), b, 0o644)
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = irPath
	res := Check(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("check failed: %#v", res.Diagnostics)
	}
	if !hasErrCode(res.Diagnostics, "TSPACK_CAPABILITY_LIFECYCLE_SCRIPT_PRESENT") {
		t.Fatalf("expected lifecycle capability warning")
	}
}

func TestSyncDoesNotExecuteLifecycleScripts(t *testing.T) {
	dir := t.TempDir()
	irPath := writeIR(t, dir, simpleIR())
	st, _ := store.Open(filepath.Join(dir, ".tspack", "store"))
	marker := filepath.Join(dir, "marker.txt")
	depRoot := t.TempDir()
	packageJSON := "{\"name\":\"dep-a\",\"version\":\"1.0.0\",\"scripts\":{\"postinstall\":\"sh -c 'echo bad > " + filepath.ToSlash(marker) + "'\"}}"
	_ = os.WriteFile(filepath.Join(depRoot, "package.json"), []byte(packageJSON), 0o644)
	ref, diags := st.PutArtifact(store.Artifact{ID: "npm:dep-a@1.0.0", Name: "dep-a", Version: "1.0.0", Source: "npm", Kind: store.ArtifactPathTree, RootDir: depRoot})
	if len(diags) > 0 {
		t.Fatalf("unexpected put artifact diagnostics: %#v", diags)
	}
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Packages: []lockfile.Package{{ID: "npm:dep-a@1.0.0", Name: "dep-a", Version: "1.0.0", Source: "npm", Hash: ref.Hash, Capabilities: []lockfile.Capability{{Kind: "lifecycle-script", Detail: "postinstall"}}}},
		Edges:    []lockfile.Edge{{From: "app:target:core", To: "npm:dep-a@1.0.0", Kind: "runtime"}},
		Targets:  []lockfile.Target{{Package: "app", Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts"}},
	}
	b, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(filepath.Join(dir, "ts-lock.toml"), b, 0o644)
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = irPath
	res := Sync(opts, false)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("sync failed: %#v", res.Diagnostics)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("marker file exists; lifecycle script was executed")
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

func TestOutdatedWantedAndLatestAvailable(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	pkgs := ir["packages"].([]map[string]any)
	pkgs[0]["dependencies"] = []map[string]any{
		{"kind": "peer", "name": "react", "source": map[string]any{"kind": "npm", "package": "react", "range": ">=18 <20"}},
	}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"}, Packages: []lockfile.Package{{ID: "npm:react@18.2.0", Name: "react", Source: "npm", Version: "18.2.0", Hash: "sha256:dummy"}}}
	b, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(opts.LockfilePath, b, 0o644)
	opts.ResolverClient = &fakeClient{meta: map[string]*resolver.PackageMetadata{"react": {Name: "react", DistTags: map[string]string{"latest": "20.0.0"}, Versions: map[string]resolver.PackageVersion{"18.2.0": {Version: "18.2.0"}, "19.2.0": {Version: "19.2.0"}, "20.0.0": {Version: "20.0.0"}}}}}
	res := Outdated(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("outdated failed: %#v", res.Diagnostics)
	}
	if got := res.Outdated.Dependencies[0].Status; got != "wanted_available" {
		t.Fatalf("expected wanted_available, got %s", got)
	}
}

func TestOutdatedMissingLockWarning(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	pkgs := ir["packages"].([]map[string]any)
	pkgs[0]["dependencies"] = []map[string]any{
		{"kind": "dep", "name": "left-pad", "source": map[string]any{"kind": "npm", "package": "left-pad", "range": "^1.0.0"}},
	}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	opts.ResolverClient = &fakeClient{meta: map[string]*resolver.PackageMetadata{"left-pad": {Name: "left-pad", DistTags: map[string]string{"latest": "1.3.0"}, Versions: map[string]resolver.PackageVersion{"1.0.0": {Version: "1.0.0"}, "1.3.0": {Version: "1.3.0"}}}}}
	res := Outdated(opts)
	if !hasErrCode(res.Diagnostics, "TSPACK_OUTDATED_LOCKFILE_MISSING") {
		t.Fatalf("expected lockfile warning")
	}
	if status := res.Outdated.Dependencies[0].Status; status != "missing_lock" {
		t.Fatalf("expected missing_lock, got %s", status)
	}
	if opts.ResolverClient.(*fakeClient).packageMetaCalls != 1 {
		t.Fatalf("expected metadata fetch for missing lock")
	}
}

func TestOutdatedCurrentStatusAndNoDiagnostics(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	pkgs := ir["packages"].([]map[string]any)
	pkgs[0]["dependencies"] = []map[string]any{
		{"kind": "dep", "name": "left-pad", "source": map[string]any{"kind": "npm", "package": "left-pad", "range": "^1.0.0"}},
	}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"}, Packages: []lockfile.Package{{ID: "npm:left-pad@1.0.0", Name: "left-pad", Source: "npm", Version: "1.0.0", Hash: "sha256:dummy"}}}
	lockBytes, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(opts.LockfilePath, lockBytes, 0o644)
	client := &fakeClient{meta: map[string]*resolver.PackageMetadata{"left-pad": {Name: "left-pad", DistTags: map[string]string{"latest": "1.0.0"}, Versions: map[string]resolver.PackageVersion{"1.0.0": {Version: "1.0.0"}}}}}
	opts.ResolverClient = client
	res := Outdated(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("outdated failed: %#v", res.Diagnostics)
	}
	if len(res.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got %#v", res.Diagnostics)
	}
	dep := res.Outdated.Dependencies[0]
	if dep.Status != "current" {
		t.Fatalf("expected current, got %s", dep.Status)
	}
	if res.Outdated.Summary.Current != 1 || res.Outdated.Summary.Outdated != 0 {
		t.Fatalf("unexpected summary: %#v", res.Outdated.Summary)
	}
	if len(client.tarCalls) != 0 {
		t.Fatalf("outdated should not fetch tarballs")
	}
	lockAfter, _ := os.ReadFile(opts.LockfilePath)
	if !bytes.Equal(lockBytes, lockAfter) {
		t.Fatalf("outdated mutated lockfile")
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("outdated created node_modules")
	}
	if _, err := os.Stat(filepath.Join(dir, ".tspack", "store")); !os.IsNotExist(err) {
		t.Fatalf("outdated created store")
	}
}

func TestOutdatedLatestOutsideRange(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	pkgs := ir["packages"].([]map[string]any)
	pkgs[0]["dependencies"] = []map[string]any{
		{"kind": "dep", "name": "dep-a", "source": map[string]any{"kind": "npm", "package": "dep-a", "range": "^1.0.0"}},
	}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"}, Packages: []lockfile.Package{{ID: "npm:dep-a@1.5.0", Name: "dep-a", Source: "npm", Version: "1.5.0", Hash: "sha256:dummy"}}}
	lockBytes, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(opts.LockfilePath, lockBytes, 0o644)
	opts.ResolverClient = &fakeClient{meta: map[string]*resolver.PackageMetadata{"dep-a": {Name: "dep-a", DistTags: map[string]string{"latest": "2.0.0"}, Versions: map[string]resolver.PackageVersion{"1.4.0": {Version: "1.4.0"}, "1.5.0": {Version: "1.5.0"}, "2.0.0": {Version: "2.0.0"}}}}}
	res := Outdated(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("outdated failed: %#v", res.Diagnostics)
	}
	dep := res.Outdated.Dependencies[0]
	if dep.Status != "latest_available" || dep.Wanted != "1.5.0" || dep.Latest != "2.0.0" {
		t.Fatalf("unexpected dependency: %#v", dep)
	}
}

func TestOutdatedSkipsNonNPMSourcesWithoutMetadataFetch(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	pkgs := ir["packages"].([]map[string]any)
	pkgs[0]["dependencies"] = []map[string]any{
		{"kind": "workspace", "name": "core", "source": map[string]any{"kind": "workspace", "package": "core"}},
	}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	client := &fakeClient{meta: map[string]*resolver.PackageMetadata{}}
	opts.ResolverClient = client
	res := Outdated(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("outdated failed: %#v", res.Diagnostics)
	}
	if client.packageMetaCalls != 0 {
		t.Fatalf("expected no registry fetch for non-npm dependency")
	}
	dep := res.Outdated.Dependencies[0]
	if dep.Status != "not_applicable" {
		t.Fatalf("expected not_applicable, got %s", dep.Status)
	}
	if !hasErrCode(res.Diagnostics, "TSPACK_OUTDATED_UNSUPPORTED_SOURCE") {
		t.Fatalf("expected unsupported source warning")
	}
}

func TestOutdatedMetadataFailureAndMultipleLockedVersions(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	pkgs := ir["packages"].([]map[string]any)
	pkgs[0]["dependencies"] = []map[string]any{
		{"kind": "dep", "name": "dep-a", "source": map[string]any{"kind": "npm", "package": "dep-a", "range": "^1.0.0"}},
	}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	lf := &lockfile.Lockfile{
		Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Packages: []lockfile.Package{
			{ID: "npm:dep-a@1.0.0", Name: "dep-a", Source: "npm", Version: "1.0.0", Hash: "sha256:a"},
			{ID: "npm:dep-a@1.1.0", Name: "dep-a", Source: "npm", Version: "1.1.0", Hash: "sha256:b"},
		},
	}
	lockBytes, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(opts.LockfilePath, lockBytes, 0o644)
	client := &fakeClient{
		meta:    map[string]*resolver.PackageMetadata{},
		metaErr: map[string]error{"dep-a": errors.New("metadata request failed: status=500 package=dep-a")},
	}
	opts.ResolverClient = client
	res := Outdated(opts)
	if !hasErrors(res.Diagnostics) {
		t.Fatalf("expected metadata error")
	}
	dep := res.Outdated.Dependencies[0]
	if dep.Status != "error" {
		t.Fatalf("expected error status, got %s", dep.Status)
	}
	if !reflect.DeepEqual(dep.Current, []string{"1.0.0", "1.1.0"}) {
		t.Fatalf("unexpected current versions: %#v", dep.Current)
	}
	if !hasErrCode(res.Diagnostics, "TSPACK_OUTDATED_METADATA_FETCH_FAILED") {
		t.Fatalf("expected metadata fetch diagnostic")
	}
	if len(res.Diagnostics) == 0 || len(res.Diagnostics[0].Details) == 0 {
		t.Fatalf("expected diagnostic details with package identity")
	}
}

func TestOutdatedRootDirFromDifferentCWD(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	ir := simpleIR()
	pkgs := ir["packages"].([]map[string]any)
	pkgs[0]["dependencies"] = []map[string]any{
		{"kind": "dep", "name": "dep-a", "source": map[string]any{"kind": "npm", "package": "dep-a", "range": "^1.0.0"}},
	}
	opts := DefaultOptions(root)
	opts.ManifestIRPath = writeIR(t, root, ir)
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"}, Packages: []lockfile.Package{{ID: "npm:dep-a@1.0.0", Name: "dep-a", Source: "npm", Version: "1.0.0", Hash: "sha256:dummy"}}}
	lockBytes, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(opts.LockfilePath, lockBytes, 0o644)
	client := &fakeClient{meta: map[string]*resolver.PackageMetadata{"dep-a": {Name: "dep-a", DistTags: map[string]string{"latest": "1.0.0"}, Versions: map[string]resolver.PackageVersion{"1.0.0": {Version: "1.0.0"}}}}}
	opts.ResolverClient = client
	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	_ = os.Chdir(other)
	res := Outdated(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("outdated failed: %#v", res.Diagnostics)
	}
	if client.packageMetaCalls != 1 || len(client.metaCalls) != 1 || client.metaCalls[0] != "dep-a" {
		t.Fatalf("unexpected registry lookup calls: %#v", client.metaCalls)
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

func TestPackDryRunAndDeterministic(t *testing.T) {
	dir := t.TempDir()
	irPath := writeIR(t, dir, simpleIR())
	_ = os.WriteFile(filepath.Join(dir, "src", "index.ts"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("readme"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "dist", "index.js"), []byte("export const x=1\n"), 0o644)
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = irPath
	r := Pack(opts, PackOptions{DryRun: true})
	if hasErrors(r.Diagnostics) {
		t.Fatalf("dry run failed: %#v", r.Diagnostics)
	}
	if len(r.PackResult.Preview) == 0 {
		t.Fatalf("expected preview")
	}
	if _, err := os.Stat(filepath.Join(dir, "tspack-artifacts")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote artifact")
	}

	r1 := Pack(opts, PackOptions{})
	r2 := Pack(opts, PackOptions{})
	if hasErrors(r1.Diagnostics) || hasErrors(r2.Diagnostics) {
		t.Fatalf("pack failed")
	}
	b1, _ := os.ReadFile(r1.PackResult.Artifacts[0].Path)
	b2, _ := os.ReadFile(r2.PackResult.Artifacts[0].Path)
	if !bytes.Equal(b1, b2) {
		t.Fatalf("nondeterministic")
	}
}

func TestPackEdgeCasesAndIntegration(t *testing.T) {
	t.Run("basic archive contents", func(t *testing.T) {
		dir := t.TempDir()
		ir := simpleIR()
		pkg := ir["packages"].([]map[string]any)[0]
		pkg["targets"] = []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "dist/index.js", "types": "dist/index.d.ts", "deps": []string{}, "peers": []string{}}}
		pkg["publish"] = map[string]any{"include": []string{"dist/**", "README.md", "LICENSE"}, "exclude": []string{"src/**"}}
		irPath := writeIR(t, dir, ir)
		_ = os.WriteFile(filepath.Join(dir, "dist", "index.js"), []byte("export {}\n"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("r\n"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "LICENSE"), []byte("l\n"), 0o644)
		r := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{})
		if hasErrors(r.Diagnostics) {
			t.Fatalf("pack failed: %#v", r.Diagnostics)
		}
		entries := readEntries(t, r.PackResult.Artifacts[0].Path)
		mustContain(t, entries, "package/package.json", "package/dist/index.js", "package/dist/index.d.ts", "package/README.md", "package/LICENSE")
		mustNotContain(t, entries, "package/src/index.ts")
	})

	t.Run("missing runtime and missing types", func(t *testing.T) {
		dir := t.TempDir()
		ir := simpleIR()
		ir["packages"].([]map[string]any)[0]["targets"] = []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "dist/missing.js", "types": "dist/missing.d.ts", "deps": []string{}, "peers": []string{}}}
		irPath := writeIR(t, dir, ir)
		r := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{})
		if !hasErrCode(r.Diagnostics, "TSPACK_TYPE_MISSING_OUTPUT") {
			t.Fatalf("missing diagnostics: %#v", r.Diagnostics)
		}
		if r.PackResult != nil && len(r.PackResult.Artifacts) > 0 {
			t.Fatalf("unexpected artifact")
		}
	})

	t.Run("workspace all and selector", func(t *testing.T) {
		dir := t.TempDir()
		ir := map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{
			{"name": "pkg-a", "version": "1.0.0", "root": "packages/a", "kind": "library", "dependencies": []map[string]any{}, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "dist/index.js", "types": "dist/index.d.ts", "deps": []string{}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}},
			{"name": "pkg-b", "version": "2.0.0", "root": "packages/b", "kind": "library", "dependencies": []map[string]any{}, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "dist/index.js", "types": "dist/index.d.ts", "deps": []string{}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}},
		}}
		for _, p := range []string{"packages/a/src", "packages/a/dist", "packages/b/src", "packages/b/dist"} {
			_ = os.MkdirAll(filepath.Join(dir, p), 0o755)
		}
		_ = os.WriteFile(filepath.Join(dir, "packages/a/src/index.ts"), []byte("a"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "packages/a/dist/index.js"), []byte("a"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "packages/a/dist/index.d.ts"), []byte("a"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "packages/b/src/index.ts"), []byte("b"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "packages/b/dist/index.js"), []byte("b"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "packages/b/dist/index.d.ts"), []byte("b"), 0o644)
		irPath := writeIR(t, dir, ir)
		r := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{})
		if hasErrors(r.Diagnostics) || len(r.PackResult.Artifacts) != 2 {
			t.Fatalf("expected two artifacts %#v %#v", r.Diagnostics, r.PackResult)
		}
		rs := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{PackageName: "pkg-a"})
		if hasErrors(rs.Diagnostics) || len(rs.PackResult.Artifacts) != 1 {
			t.Fatalf("selector failed")
		}
		rm := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{PackageName: "missing"})
		if !hasErrCode(rm.Diagnostics, "TSPACK_PACK_PACKAGE_NOT_FOUND") {
			t.Fatalf("expected not found")
		}
	})
}

func DefaultOptionsWithIR(dir, irPath string) Options {
	o := DefaultOptions(dir)
	o.ManifestIRPath = irPath
	return o
}
func readEntries(t *testing.T, path string) []string {
	t.Helper()
	b, _ := os.ReadFile(path)
	gr, _ := gzip.NewReader(bytes.NewReader(b))
	tr := tar.NewReader(gr)
	out := []string{}
	for {
		h, e := tr.Next()
		if e != nil {
			break
		}
		if !h.ModTime.Equal((h.ModTime).UTC()) {
		}
		out = append(out, h.Name)
	}
	_ = gr.Close()
	return out
}
func mustContain(t *testing.T, entries []string, vals ...string) {
	t.Helper()
	m := map[string]bool{}
	for _, e := range entries {
		m[e] = true
	}
	for _, v := range vals {
		if !m[v] {
			t.Fatalf("missing %s in %v", v, entries)
		}
	}
}
func mustNotContain(t *testing.T, entries []string, val string) {
	t.Helper()
	for _, e := range entries {
		if e == val {
			t.Fatalf("unexpected %s", val)
		}
	}
}

func TestPackMutationGuaranteesAndGeneratedPackageJSON(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	pkg := ir["packages"].([]map[string]any)[0]
	pkg["license"] = "MIT"
	pkg["dependencies"] = []map[string]any{
		{"key": "react-dom", "kind": "peer", "source": map[string]any{"kind": "npm", "package": "react-dom", "range": ">=18 <20"}},
		{"key": "react", "kind": "peer", "source": map[string]any{"kind": "npm", "package": "react", "range": ">=18 <20"}},
	}
	pkg["targets"] = []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "dist/index.js", "types": "dist/index.d.ts", "deps": []string{}, "peers": []string{"react-dom", "react"}}}
	pkg["publish"] = map[string]any{"include": []string{"dist/**", "README.md"}, "exclude": []string{}}
	irPath := writeIR(t, dir, ir)
	_ = os.WriteFile(filepath.Join(dir, "dist", "index.js"), []byte("export const x = 1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "dist", "index.d.ts"), []byte("export declare const x: number;\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("readme\n"), 0o644)
	lockPath := filepath.Join(dir, "ts-lock.toml")
	lf := &lockfile.Lockfile{
		Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Targets: []lockfile.Target{{
			Package: "app",
			Name:    "core",
			Export:  ".",
			Entry:   "src/index.ts",
			Runtime: "dist/index.js",
			Types:   "dist/index.d.ts",
		}},
	}
	before, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(lockPath, before, 0o644)

	r := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{})
	if hasErrors(r.Diagnostics) {
		t.Fatalf("pack failed: %#v", r.Diagnostics)
	}
	after, _ := os.ReadFile(lockPath)
	if !bytes.Equal(before, after) {
		t.Fatalf("lockfile mutated")
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("node_modules created")
	}
	if _, err := os.Stat(filepath.Join(dir, ".tspack", "store")); !os.IsNotExist(err) {
		t.Fatalf("store created")
	}

	artifactBytes, _ := os.ReadFile(r.PackResult.Artifacts[0].Path)
	gz, _ := gzip.NewReader(bytes.NewReader(artifactBytes))
	tr := tar.NewReader(gz)
	var packageJSON []byte
	for {
		h, e := tr.Next()
		if e != nil {
			break
		}
		if h.Name == "package/package.json" {
			packageJSON, _ = io.ReadAll(tr)
		}
	}
	_ = gz.Close()
	if len(packageJSON) == 0 {
		t.Fatalf("missing generated package.json")
	}
	var parsed map[string]any
	_ = json.Unmarshal(packageJSON, &parsed)
	if parsed["name"] != "app" || parsed["version"] != "1.0.0" {
		t.Fatalf("missing name/version: %s", string(packageJSON))
	}
	if parsed["license"] != "MIT" || parsed["main"] != "./dist/index.js" || parsed["types"] != "./dist/index.d.ts" {
		t.Fatalf("missing generated package metadata: %s", string(packageJSON))
	}
	peers := parsed["peerDependencies"].(map[string]any)
	if peers["react"] != ">=18 <20" || peers["react-dom"] != ">=18 <20" {
		t.Fatalf("missing generated package peers: %s", string(packageJSON))
	}
	exports, ok := parsed["exports"].(map[string]any)
	if !ok || exports["."] == nil {
		t.Fatalf("missing exports: %s", string(packageJSON))
	}
}

func TestPackFailsWhenCheckFailsAndWritesNoArtifact(t *testing.T) {
	dir := t.TempDir()
	irPath := writeIR(t, dir, simpleIR())
	_ = os.Remove(filepath.Join(dir, "src", "index.ts"))
	_ = os.WriteFile(filepath.Join(dir, "dist", "index.js"), []byte("export const x = 1\n"), 0o644)
	outDir := filepath.Join(dir, "out")
	r := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: outDir})
	if !hasErrCode(r.Diagnostics, "TSPACK_IMPORT_PARSE_ERROR") {
		t.Fatalf("expected propagated check error: %#v", r.Diagnostics)
	}
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Fatalf("unexpected output dir created")
	}
}

func TestWhyDoesNotMutateLockfile(t *testing.T) {
	dir := t.TempDir()
	irPath := writeIR(t, dir, simpleIR())
	lockPath := filepath.Join(dir, "ts-lock.toml")
	_ = os.WriteFile(lockPath, []byte("[lock]\nformat=1\ntool=\"tspack\"\n"), 0o644)
	before, _ := os.ReadFile(lockPath)
	r := Why(DefaultOptionsWithIR(dir, irPath), WhyOptions{Query: "core"})
	if r.WhyResult == nil {
		t.Fatalf("missing why result")
	}
	after, _ := os.ReadFile(lockPath)
	if !bytes.Equal(before, after) {
		t.Fatalf("lockfile mutated")
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("node_modules should not exist")
	}
}

func TestWhyMissingAndInvalidLockfileAndDeterminism(t *testing.T) {
	dir := t.TempDir()
	irPath := writeIR(t, dir, simpleIR())
	opts := DefaultOptionsWithIR(dir, irPath)

	missing := Why(opts, WhyOptions{Query: "core"})
	if missing.WhyResult == nil {
		t.Fatalf("missing why result")
	}
	if !hasErrCode(missing.Diagnostics, "TSPACK_WHY_LOCKFILE_MISSING") {
		t.Fatalf("expected missing lock warning")
	}
	if hasErrors(missing.Diagnostics) {
		t.Fatalf("missing lock should not fail: %#v", missing.Diagnostics)
	}

	_ = os.WriteFile(opts.LockfilePath, []byte("bad"), 0o644)
	invalid := Why(opts, WhyOptions{Query: "core"})
	if invalid.WhyResult == nil {
		t.Fatalf("invalid lock should still provide why result")
	}
	if !hasErrCode(invalid.Diagnostics, "TSPACK_LOCK_INVALID_TOML") {
		t.Fatalf("expected invalid lock diagnostics")
	}

	validLock := []byte("[lock]\nformat=1\ntool=\"tspack\"\n")
	_ = os.WriteFile(opts.LockfilePath, validLock, 0o644)
	r1 := Why(opts, WhyOptions{Query: "core"})
	r2 := Why(opts, WhyOptions{Query: "core"})
	if !reflect.DeepEqual(r1.WhyResult, r2.WhyResult) {
		t.Fatalf("non-deterministic why result")
	}
	if _, err := os.Stat(filepath.Join(dir, ".tspack", "store")); !os.IsNotExist(err) {
		t.Fatalf("store should not be created")
	}
}

func TestV1GoldenPathCommandLoop(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	opts.ResolverClient = buildRegistry()

	check1 := Check(opts)
	if hasErrors(check1.Diagnostics) && !hasErrCode(check1.Diagnostics, "TSPACK_CHECK_LOCKFILE_MISSING") {
		t.Fatalf("initial check unexpected errors: %#v", check1.Diagnostics)
	}
	if _, err := os.Stat(opts.LockfilePath); !os.IsNotExist(err) {
		t.Fatalf("check should not create lockfile")
	}

	up := Update(opts)
	if hasErrors(up.Diagnostics) {
		t.Fatalf("update failed: %#v", up.Diagnostics)
	}
	lockAfterUpdate, _ := os.ReadFile(opts.LockfilePath)

	syncRes := Sync(opts, false)
	if hasErrors(syncRes.Diagnostics) {
		t.Fatalf("sync failed: %#v", syncRes.Diagnostics)
	}
	lockAfterSync, _ := os.ReadFile(opts.LockfilePath)
	if !bytes.Equal(lockAfterUpdate, lockAfterSync) {
		t.Fatalf("sync mutated lockfile")
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules", ".tspack-materialized")); err != nil {
		t.Fatalf("sync should materialize node_modules marker: %v", err)
	}

	whyRes := Why(opts, WhyOptions{Query: "core"})
	if hasErrors(whyRes.Diagnostics) || whyRes.WhyResult == nil || len(whyRes.WhyResult.Explanations) == 0 {
		t.Fatalf("why failed: %#v", whyRes.Diagnostics)
	}
	lockAfterWhy, _ := os.ReadFile(opts.LockfilePath)
	if !bytes.Equal(lockAfterSync, lockAfterWhy) {
		t.Fatalf("why mutated lockfile")
	}

	if _, err := os.Stat(filepath.Join(dir, "tspack-artifacts")); !os.IsNotExist(err) {
		t.Fatalf("pack output dir should not pre-exist")
	}
	pack1 := Pack(opts, PackOptions{OutputDir: filepath.Join(dir, "out")})
	if hasErrors(pack1.Diagnostics) || pack1.PackResult == nil || len(pack1.PackResult.Artifacts) != 1 {
		t.Fatalf("pack1 failed: %#v", pack1.Diagnostics)
	}
	pack2 := Pack(opts, PackOptions{OutputDir: filepath.Join(dir, "out2")})
	if hasErrors(pack2.Diagnostics) || pack2.PackResult == nil || len(pack2.PackResult.Artifacts) != 1 {
		t.Fatalf("pack2 failed: %#v", pack2.Diagnostics)
	}
	if pack1.PackResult.Artifacts[0].Hash != pack2.PackResult.Artifacts[0].Hash {
		t.Fatalf("pack hash not stable: %s vs %s", pack1.PackResult.Artifacts[0].Hash, pack2.PackResult.Artifacts[0].Hash)
	}
	lockAfterPack, _ := os.ReadFile(opts.LockfilePath)
	if !bytes.Equal(lockAfterWhy, lockAfterPack) {
		t.Fatalf("pack mutated lockfile")
	}
}

func targetedIRWithDeps(deps []map[string]any, depRefs []string) map[string]any {
	return map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{{"name": "app", "version": "1.0.0", "kind": "library", "dependencies": deps, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": depRefs, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}}}}
}

func TestTargetedSelectionKeyNameAndQualifiedName(t *testing.T) {
	deps := []map[string]any{{"key": "react", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "react", "range": "^18"}}, {"key": "lodash", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "lodash", "range": "^4"}}}
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, targetedIRWithDeps(deps, []string{"react", "lodash"}))
	opts.ResolverClient = buildRegistryForTargetedSelection(t)
	for _, q := range []string{"react", "npm:react"} {
		res := UpdateDryRunWithOptions(opts, UpdateOptions{Query: q})
		if hasErrors(res.Diagnostics) {
			t.Fatalf("query %s failed: %#v", q, res.Diagnostics)
		}
		if res.UpdateTarget == nil || len(res.UpdateTarget.Selected) != 1 || res.UpdateTarget.Selected[0].Name != "react" {
			t.Fatalf("query %s selected unexpected target: %#v", q, res.UpdateTarget)
		}
	}
	resByName := UpdateDryRunWithOptions(opts, UpdateOptions{Query: "lodash"})
	if hasErrors(resByName.Diagnostics) {
		t.Fatalf("query by package name failed: %#v", resByName.Diagnostics)
	}
	if resByName.UpdateTarget == nil || len(resByName.UpdateTarget.Selected) != 1 || resByName.UpdateTarget.Selected[0].Name != "lodash" {
		t.Fatalf("query by package name selected unexpected target: %#v", resByName.UpdateTarget)
	}
}

func TestTargetedSelectionNotFoundAndTransitiveOnly(t *testing.T) {
	dir := t.TempDir()
	deps := []map[string]any{{"key": "react", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "react", "range": "^18"}}}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, targetedIRWithDeps(deps, []string{"react"}))
	opts.ResolverClient = buildRegistryForTargetedSelection(t)
	res := UpdateDryRunWithOptions(opts, UpdateOptions{Query: "loose-envify"})
	if !hasErrCode(res.Diagnostics, "TSPACK_UPDATE_TARGET_NOT_FOUND") {
		t.Fatalf("expected not found diagnostic: %#v", res.Diagnostics)
	}
	if !diagnosticHasDetail(res.Diagnostics, "TSPACK_UPDATE_TARGET_NOT_FOUND", "targeted update only updates declared dependencies") {
		t.Fatalf("expected declared-only detail: %#v", res.Diagnostics)
	}
}

func TestTargetedSelectionAmbiguousAndUnsupportedSource(t *testing.T) {
	dir := t.TempDir()
	deps := []map[string]any{{"key": "react", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "react", "range": "^18"}}, {"key": "local-shared", "kind": "dep", "source": map[string]any{"kind": "path", "path": "vendor/shared"}}}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{{"name": "app", "version": "1.0.0", "kind": "library", "dependencies": deps, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{"react", "local-shared"}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}}}})
	opts.ResolverClient = buildRegistryForTargetedSelection(t)
	res := UpdateDryRunWithOptions(opts, UpdateOptions{Query: "local-shared"})
	if !hasErrCode(res.Diagnostics, "TSPACK_UPDATE_TARGET_UNSUPPORTED_SOURCE") {
		t.Fatalf("expected unsupported source diagnostic: %#v", res.Diagnostics)
	}

	dir2 := t.TempDir()
	opts2 := DefaultOptions(dir2)
	app1 := map[string]any{
		"name":         "app",
		"version":      "1.0.0",
		"kind":         "library",
		"dependencies": []map[string]any{{"key": "shared", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "react", "range": "^18"}}},
		"targets":      []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{"shared"}, "peers": []string{}}},
		"tools":        []string{},
		"boundaries":   []any{},
		"publish":      map[string]any{"include": []string{"dist/**"}, "exclude": []string{}},
		"policies":     map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}},
	}
	app2 := map[string]any{
		"name":         "app2",
		"version":      "1.0.0",
		"kind":         "library",
		"dependencies": []map[string]any{{"key": "shared", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "react-dom", "range": "^18"}}},
		"targets":      []map[string]any{{"name": "core", "export": "./2", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{"shared"}, "peers": []string{}}},
		"tools":        []string{},
		"boundaries":   []any{},
		"publish":      map[string]any{"include": []string{"dist/**"}, "exclude": []string{}},
		"policies":     map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}},
	}
	opts2.ManifestIRPath = writeIR(t, dir2, map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{app1, app2}})
	opts2.ResolverClient = buildRegistryForTargetedSelection(t)
	res2 := UpdateDryRunWithOptions(opts2, UpdateOptions{Query: "shared"})
	if !hasErrCode(res2.Diagnostics, "TSPACK_UPDATE_TARGET_AMBIGUOUS") {
		t.Fatalf("expected ambiguous diagnostic: %#v", res2.Diagnostics)
	}
	if !diagnosticHasDetail(res2.Diagnostics, "TSPACK_UPDATE_TARGET_AMBIGUOUS", "query matched multiple declared dependencies") {
		t.Fatalf("expected ambiguous matches detail: %#v", res2.Diagnostics)
	}
}

func diagnosticHasDetail(diags []diag.Diagnostic, code, needle string) bool {
	for _, d := range diags {
		if d.Code != code {
			continue
		}
		for _, detail := range d.Details {
			if strings.Contains(detail, needle) {
				return true
			}
		}
	}
	return false
}

func buildRegistryForTargetedSelection(t *testing.T) *fakeClient {
	t.Helper()
	tarballs := map[string][]byte{}
	makeVer := func(name, version string, deps map[string]string) resolver.PackageVersion {
		body := tarball(name, version, deps)
		url := "https://example.invalid/" + name + "-" + version + ".tgz"
		tarballs[url] = body
		sum := sha512sum(body)
		return resolver.PackageVersion{Name: name, Version: version, Dependencies: deps, Dist: resolver.PackageDist{Tarball: url, Integrity: "sha512-" + base64.StdEncoding.EncodeToString(sum)}}
	}
	meta := map[string]*resolver.PackageMetadata{
		"react":        {Name: "react", Versions: map[string]resolver.PackageVersion{"18.2.0": makeVer("react", "18.2.0", map[string]string{"loose-envify": "1.4.0"})}},
		"lodash":       {Name: "lodash", Versions: map[string]resolver.PackageVersion{"4.17.20": makeVer("lodash", "4.17.20", nil)}},
		"react-dom":    {Name: "react-dom", Versions: map[string]resolver.PackageVersion{"18.2.0": makeVer("react-dom", "18.2.0", nil)}},
		"loose-envify": {Name: "loose-envify", Versions: map[string]resolver.PackageVersion{"1.4.0": makeVer("loose-envify", "1.4.0", nil)}},
	}
	return &fakeClient{meta: meta, tar: tarballs}
}
