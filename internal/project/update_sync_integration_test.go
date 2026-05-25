package project

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/tspack/tspack/internal/lockfile"
	"github.com/tspack/tspack/internal/resolver"
)

func TestUpdateThenSyncWithFakeRegistryPopulatesStore(t *testing.T) {
	root := t.TempDir()
	irPath := writeIR(t, root, map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{{"name": "app", "version": "1.0.0", "kind": "library", "dependencies": []map[string]any{{"key": "dep-a", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "dep-a", "range": "1.0.0"}}}, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{"dep-a"}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}}}})

	registry := newFakeRegistryServer(t)
	defer registry.Close()

	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	opts.ResolverClient = resolver.NewHTTPRegistryClient(registry.URL)

	up := Update(opts)
	if hasErrors(up.Diagnostics) {
		t.Fatalf("update failed: %#v", up.Diagnostics)
	}
	if _, err := os.Stat(opts.LockfilePath); err != nil {
		t.Fatalf("expected lockfile: %v", err)
	}
	lf, _, err := lockfile.LoadFile(opts.LockfilePath)
	if err != nil {
		t.Fatalf("load lock: %v", err)
	}
	if len(lf.Packages) == 0 || lf.Packages[0].Hash == "" {
		t.Fatalf("expected package hash in lock: %#v", lf.Packages)
	}
	if _, err := os.Stat(filepath.Join(opts.StoreRoot, "metadata")); err != nil {
		t.Fatalf("expected populated store metadata tree: %v", err)
	}

	s1 := Sync(opts, false)
	if hasErrors(s1.Diagnostics) {
		t.Fatalf("sync failed: %#v", s1.Diagnostics)
	}
	mustExist(t, filepath.Join(root, "node_modules", "dep-a", "package.json"))

	before, _ := os.ReadFile(opts.LockfilePath)
	s2 := Sync(opts, false)
	if hasErrors(s2.Diagnostics) {
		t.Fatalf("repeat sync failed: %#v", s2.Diagnostics)
	}
	after, _ := os.ReadFile(opts.LockfilePath)
	if !bytes.Equal(before, after) {
		t.Fatalf("sync mutated lockfile")
	}
}

func TestUpdateFailureDoesNotWriteLockfile(t *testing.T) {
	root := t.TempDir()
	irPath := writeIR(t, root, map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{{"name": "app", "version": "1.0.0", "kind": "library", "dependencies": []map[string]any{{"key": "dep-a", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "dep-a", "range": "1.0.0"}}}, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{"dep-a"}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}}}})

	mux := http.NewServeMux()
	bad := httptest.NewServer(mux)
	defer bad.Close()
	mux.HandleFunc("/dep-a", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(resolver.PackageMetadata{Name: "dep-a", Versions: map[string]resolver.PackageVersion{"1.0.0": {Name: "dep-a", Version: "1.0.0", Dist: resolver.PackageDist{Tarball: bad.URL + "/dep-a-1.0.0.tgz"}}}})
	})

	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	opts.ResolverClient = resolver.NewHTTPRegistryClient(bad.URL)

	res := Update(opts)
	if !hasErrors(res.Diagnostics) {
		t.Fatalf("expected update failure")
	}
	if _, err := os.Stat(opts.LockfilePath); !os.IsNotExist(err) {
		t.Fatalf("lockfile should not be written on failure")
	}
}

func TestUpdatePathDependencyResolvesFromRootWhenCWDDiffers(t *testing.T) {
	root := t.TempDir()
	depRoot := filepath.Join(root, "vendor", "dep")
	_ = os.MkdirAll(depRoot, 0o755)
	_ = os.WriteFile(filepath.Join(depRoot, "package.json"), []byte(`{"name":"dep-local","version":"1.0.0"}`), 0o644)
	irPath := writeIR(t, root, map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{{"name": "app", "version": "1.0.0", "root": ".", "kind": "library", "dependencies": []map[string]any{{"key": "dep-local", "kind": "dep", "source": map[string]any{"kind": "path", "path": "vendor/dep"}}}, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{"dep-local"}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}}}})

	prev, _ := os.Getwd()
	other := t.TempDir()
	_ = os.Chdir(other)
	t.Cleanup(func() { _ = os.Chdir(prev) })

	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	res := Update(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("update failed from different cwd: %#v", res.Diagnostics)
	}
	lf, _, err := lockfile.LoadFile(opts.LockfilePath)
	if err != nil {
		t.Fatalf("load lock: %v", err)
	}
	if len(lf.Packages) != 1 || lf.Packages[0].Path != "vendor/dep" {
		t.Fatalf("unexpected lock packages: %#v", lf.Packages)
	}
}

func newFakeRegistryServer(t *testing.T) *httptest.Server {
	t.Helper()
	depTar := fakeRegistryTarball(t, "dep-a", "1.0.0", map[string]string{"left-pad": "1.0.0"})
	leftPadTar := fakeRegistryTarball(t, "left-pad", "1.0.0", nil)
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	mux.HandleFunc("/dep-a", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(resolver.PackageMetadata{Name: "dep-a", Versions: map[string]resolver.PackageVersion{"1.0.0": {Name: "dep-a", Version: "1.0.0", Dependencies: map[string]string{"left-pad": "1.0.0"}, Dist: resolver.PackageDist{Tarball: server.URL + "/dep-a-1.0.0.tgz", Integrity: fakeIntegrity(depTar)}}}})
	})
	mux.HandleFunc("/left-pad", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(resolver.PackageMetadata{Name: "left-pad", Versions: map[string]resolver.PackageVersion{"1.0.0": {Name: "left-pad", Version: "1.0.0", Dist: resolver.PackageDist{Tarball: server.URL + "/left-pad-1.0.0.tgz", Integrity: fakeIntegrity(leftPadTar)}}}})
	})
	mux.HandleFunc("/dep-a-1.0.0.tgz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(depTar) })
	mux.HandleFunc("/left-pad-1.0.0.tgz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(leftPadTar) })
	return server
}

func fakeRegistryTarball(t *testing.T, name, version string, deps map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	pj := map[string]any{"name": name, "version": version}
	if deps != nil {
		pj["dependencies"] = deps
	}
	b, _ := json.Marshal(pj)
	_ = tw.WriteHeader(&tar.Header{Name: "package/package.json", Mode: 0o644, Size: int64(len(b))})
	_, _ = tw.Write(b)
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func fakeIntegrity(body []byte) string {
	s := sha512.Sum512(body)
	return "sha512-" + base64.StdEncoding.EncodeToString(s[:])
}
