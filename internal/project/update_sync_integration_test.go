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
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/resolver"
	"github.com/yuechen-li-dev/tspack/internal/store"
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

	s1 := Sync(opts, false, false)
	if hasErrors(s1.Diagnostics) {
		t.Fatalf("sync failed: %#v", s1.Diagnostics)
	}
	mustExist(t, filepath.Join(root, "node_modules", "dep-a", "package.json"))
	storeRef := findPackageStoreRef(t, opts.StoreRoot, lf, "dep-a")
	storeInfo, err := os.Stat(filepath.Join(storeRef.ExtractedPath, "package.json"))
	if err != nil {
		t.Fatalf("stat store package.json: %v", err)
	}
	materializedInfo, err := os.Stat(filepath.Join(root, "node_modules", "dep-a", "package.json"))
	if err != nil {
		t.Fatalf("stat materialized package.json: %v", err)
	}
	if !os.SameFile(storeInfo, materializedInfo) {
		t.Log("hardlinks unavailable in this environment; sync fell back to copying package files")
	}

	before, _ := os.ReadFile(opts.LockfilePath)
	s2 := Sync(opts, false, false)
	if hasErrors(s2.Diagnostics) {
		t.Fatalf("repeat sync failed: %#v", s2.Diagnostics)
	}
	after, _ := os.ReadFile(opts.LockfilePath)
	if !bytes.Equal(before, after) {
		t.Fatalf("sync mutated lockfile")
	}
}

func TestDeclaredMirrorFallbackProvenanceThenOfflineStoreSync(t *testing.T) {
	root := t.TempDir()
	artifact := fakeRegistryTarball(t, "dep-a", "1.0.0", nil)
	var primaryRequests atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryRequests.Add(1)
		http.Error(w, "mirror unavailable", http.StatusServiceUnavailable)
	}))
	defer primary.Close()
	var fallbackRequests atomic.Int32
	var fallback *httptest.Server
	fallback = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackRequests.Add(1)
		switch r.URL.Path {
		case "/dep-a":
			_ = json.NewEncoder(w).Encode(resolver.PackageMetadata{Name: "dep-a", Versions: map[string]resolver.PackageVersion{"1.0.0": {Name: "dep-a", Version: "1.0.0", Dist: resolver.PackageDist{Tarball: fallback.URL + "/dep-a.tgz", Integrity: fakeIntegrity(artifact)}}}})
		case "/dep-a.tgz":
			_, _ = w.Write(artifact)
		default:
			http.NotFound(w, r)
		}
	}))
	defer fallback.Close()

	ir := minimalRegistryPolicyIR(map[string]any{
		"allowedSources":   []string{"npm"},
		"requireIntegrity": true,
		"sources": []map[string]any{{
			"kind":      "npm",
			"endpoints": []map[string]any{{"url": primary.URL}, {"url": fallback.URL}},
		}},
	})
	irPath := writeIR(t, root, ir)
	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	update := Update(opts)
	if hasErrors(update.Diagnostics) {
		t.Fatalf("mirror fallback update failed: %#v", update.Diagnostics)
	}
	locked, _, err := lockfile.LoadFile(opts.LockfilePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(locked.Packages) != 1 || locked.Packages[0].MetadataEndpoint != fallback.URL || locked.Packages[0].RegistryEndpoint != fallback.URL || locked.Packages[0].Source != resolver.SourceNPM {
		t.Fatalf("semantic identity/provenance mismatch: %#v", locked.Packages)
	}
	if primaryRequests.Load() != 1 || fallbackRequests.Load() != 2 {
		t.Fatalf("unexpected deterministic request counts primary=%d fallback=%d", primaryRequests.Load(), fallbackRequests.Load())
	}
	outdated := Outdated(opts)
	if hasErrors(outdated.Diagnostics) {
		t.Fatalf("policy-aware outdated failed: %#v", outdated.Diagnostics)
	}
	if primaryRequests.Load() != 2 || fallbackRequests.Load() != 3 {
		t.Fatalf("outdated did not reuse ordered endpoint policy primary=%d fallback=%d", primaryRequests.Load(), fallbackRequests.Load())
	}

	ir["registryPolicy"].(map[string]any)["offline"] = true
	writeIR(t, root, ir)
	primary.Close()
	fallback.Close()
	syncResult := Sync(opts, false, false)
	if hasErrors(syncResult.Diagnostics) {
		t.Fatalf("offline store sync failed: %#v", syncResult.Diagnostics)
	}
	mustExist(t, filepath.Join(root, "node_modules", "dep-a", "package.json"))
}

func minimalRegistryPolicyIR(policy map[string]any) map[string]any {
	return map[string]any{
		"format":         1,
		"workspace":      map[string]any{"name": "ws"},
		"registryPolicy": policy,
		"packages": []map[string]any{{
			"name": "app", "version": "1.0.0", "kind": "library",
			"dependencies": []map[string]any{{"key": "dep-a", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "dep-a", "range": "1.0.0"}}},
			"targets":      []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{"dep-a"}, "peers": []string{}}},
			"tools":        []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}},
		}},
	}
}

func TestMixedNPMAndJSRUpdateThenOfflineSyncSupportsNode(t *testing.T) {
	root := t.TempDir()
	irPath := writeIR(t, root, map[string]any{
		"format":    1,
		"workspace": map[string]any{"name": "ws"},
		"packages": []map[string]any{{
			"name": "app", "version": "1.0.0", "kind": "library",
			"dependencies": []map[string]any{
				{"key": "npmLeaf", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "npm-leaf", "range": "1.0.0"}},
				{"key": "jsrLeaf", "kind": "dep", "source": map[string]any{"kind": "jsr", "package": "@demo/leaf", "range": "1.0.0"}},
			},
			"targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{"npmLeaf", "jsrLeaf"}, "peers": []string{}}},
			"tools":   []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}},
		}},
	})

	npmTarball := fakeRegistryTarball(t, "npm-leaf", "1.0.0", nil)
	jsrTarball := fakeJSRCompatibilityTarball(t, "@jsr/demo__leaf", "1.0.0")
	npmMux := http.NewServeMux()
	npmServer := httptest.NewServer(npmMux)
	npmMux.HandleFunc("/npm-leaf", func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(resolver.PackageMetadata{Name: "npm-leaf", Versions: map[string]resolver.PackageVersion{"1.0.0": {Name: "npm-leaf", Version: "1.0.0", Dist: resolver.PackageDist{Tarball: npmServer.URL + "/npm-leaf.tgz", Integrity: fakeIntegrity(npmTarball)}}}})
	})
	npmMux.HandleFunc("/npm-leaf.tgz", func(writer http.ResponseWriter, request *http.Request) { _, _ = writer.Write(npmTarball) })

	var jsrServer *httptest.Server
	jsrServer = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/@jsr/demo__leaf":
			_ = json.NewEncoder(writer).Encode(resolver.PackageMetadata{Name: "@jsr/demo__leaf", Versions: map[string]resolver.PackageVersion{"1.0.0": {Name: "@jsr/demo__leaf", Version: "1.0.0", Dist: resolver.PackageDist{Tarball: jsrServer.URL + "/leaf.tgz", Integrity: fakeIntegrity(jsrTarball)}}}})
		case "/leaf.tgz":
			_, _ = writer.Write(jsrTarball)
		default:
			http.NotFound(writer, request)
		}
	}))

	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	opts.ResolverBackends = resolver.BackendRegistry{
		resolver.SourceNPM: resolver.NewNPMBackend(resolver.NewHTTPRegistryClient(npmServer.URL)),
		resolver.SourceJSR: resolver.NewJSRBackend(resolver.NewHTTPRegistryClient(jsrServer.URL)),
	}
	update := Update(opts)
	if hasErrors(update.Diagnostics) {
		t.Fatalf("mixed update failed: %#v", update.Diagnostics)
	}
	locked, _, err := lockfile.LoadFile(opts.LockfilePath)
	if err != nil {
		t.Fatal(err)
	}
	if !lockHasPackage(locked, "npm:npm-leaf@1.0.0") || !lockHasPackage(locked, "jsr:@demo/leaf@1.0.0") {
		t.Fatalf("mixed lock lacks source-qualified packages: %#v", locked.Packages)
	}

	// Closing both registries proves sync consumes lock/store truth without a registry or Deno.
	npmServer.Close()
	jsrServer.Close()
	syncResult := Sync(opts, false, false)
	if hasErrors(syncResult.Diagnostics) {
		t.Fatalf("offline mixed sync failed: %#v", syncResult.Diagnostics)
	}
	mustExist(t, filepath.Join(root, "node_modules", "@jsr", "demo__leaf", "index.js"))

	// A real Node process is necessary here to prove the generated compatibility package is consumable.
	command := exec.Command("node", "--input-type=module", "--eval", "import { answer } from '@jsr/demo__leaf'; if (answer !== 42) process.exit(3);")
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Node could not consume materialized JSR package: %v\n%s", err, output)
	}
}

func lockHasPackage(lock *lockfile.Lockfile, id string) bool {
	for _, pkg := range lock.Packages {
		if pkg.ID == id {
			return true
		}
	}
	return false
}

func fakeJSRCompatibilityTarball(t *testing.T, name string, version string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	files := map[string][]byte{
		"package/package.json": []byte(`{"name":"` + name + `","version":"` + version + `","type":"module","exports":"./index.js","types":"./index.d.ts"}`),
		"package/index.js":     []byte("export const answer = 42;\n"),
		"package/index.d.ts":   []byte("export declare const answer: 42;\n"),
	}
	for _, fileName := range []string{"package/package.json", "package/index.js", "package/index.d.ts"} {
		body := files[fileName]
		if err := tarWriter.WriteHeader(&tar.Header{Name: fileName, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
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

func TestUpdateDryRunNoLockfileReportsAddAndDoesNotMutate(t *testing.T) {
	root := t.TempDir()
	irPath := writeIR(t, root, map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{{"name": "app", "version": "1.0.0", "kind": "library", "dependencies": []map[string]any{{"key": "dep-a", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "dep-a", "range": "1.0.0"}}}, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{"dep-a"}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}}}})
	registry := newFakeRegistryServer(t)
	defer registry.Close()

	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	opts.ResolverClient = resolver.NewHTTPRegistryClient(registry.URL)
	res := UpdateDryRun(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("dry-run failed: %#v", res.Diagnostics)
	}
	if res.LockDiff == nil || len(res.LockDiff.PackagesAdded) == 0 {
		t.Fatalf("expected dry-run added packages: %#v", res.LockDiff)
	}
	if _, err := os.Stat(opts.LockfilePath); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not create lockfile")
	}
	if _, err := os.Stat(opts.StoreRoot); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not create store root")
	}
	if _, err := os.Stat(filepath.Join(root, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not create node_modules")
	}
}

func TestUpdateHTTPPathAvoidsDoubleTarballFetchAndExtraMetadataFetch(t *testing.T) {
	root := t.TempDir()
	irPath := writeIR(t, root, map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{{"name": "app", "version": "1.0.0", "kind": "library", "dependencies": []map[string]any{{"key": "dep-a-a", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "dep-a", "range": "1.0.0"}}, {"key": "dep-a-b", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "dep-a", "range": "1.0.0"}}}, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{"dep-a-a", "dep-a-b"}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}}}})

	depTar := fakeRegistryTarball(t, "dep-a", "1.0.0", map[string]string{"left-pad": "1.0.0"})
	leftPadTar := fakeRegistryTarball(t, "left-pad", "1.0.0", nil)
	metadataHits := map[string]int{}
	tarballHits := map[string]int{}
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	mux.HandleFunc("/dep-a", func(w http.ResponseWriter, r *http.Request) {
		metadataHits["dep-a"]++
		_ = json.NewEncoder(w).Encode(resolver.PackageMetadata{Name: "dep-a", Versions: map[string]resolver.PackageVersion{"1.0.0": {Name: "dep-a", Version: "1.0.0", Dependencies: map[string]string{"left-pad": "1.0.0"}, Dist: resolver.PackageDist{Tarball: server.URL + "/dep-a-1.0.0.tgz", Integrity: fakeIntegrity(depTar)}}}})
	})
	mux.HandleFunc("/left-pad", func(w http.ResponseWriter, r *http.Request) {
		metadataHits["left-pad"]++
		_ = json.NewEncoder(w).Encode(resolver.PackageMetadata{Name: "left-pad", Versions: map[string]resolver.PackageVersion{"1.0.0": {Name: "left-pad", Version: "1.0.0", Dist: resolver.PackageDist{Tarball: server.URL + "/left-pad-1.0.0.tgz", Integrity: fakeIntegrity(leftPadTar)}}}})
	})
	mux.HandleFunc("/dep-a-1.0.0.tgz", func(w http.ResponseWriter, r *http.Request) {
		tarballHits["dep-a"]++
		_, _ = w.Write(depTar)
	})
	mux.HandleFunc("/left-pad-1.0.0.tgz", func(w http.ResponseWriter, r *http.Request) {
		tarballHits["left-pad"]++
		_, _ = w.Write(leftPadTar)
	})

	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	opts.ResolverClient = resolver.NewHTTPRegistryClient(server.URL)
	res := Update(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("update failed: %#v", res.Diagnostics)
	}

	if metadataHits["dep-a"] != 1 || metadataHits["left-pad"] != 1 {
		t.Fatalf("unexpected metadata hits: %#v", metadataHits)
	}
	if tarballHits["dep-a"] != 1 || tarballHits["left-pad"] != 1 {
		t.Fatalf("unexpected tarball hits: %#v", tarballHits)
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

func findPackageStoreRef(t *testing.T, storeRoot string, lf *lockfile.Lockfile, packageName string) store.StoreRef {
	t.Helper()
	var hash string
	for _, pkg := range lf.Packages {
		if pkg.Name == packageName {
			hash = pkg.Hash
			break
		}
	}
	if hash == "" {
		t.Fatalf("missing package hash for %s in lockfile", packageName)
	}
	st, err := store.Open(storeRoot)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ref, diags := st.Get(hash)
	if len(diags) > 0 {
		t.Fatalf("store get diags: %#v", diags)
	}
	return ref
}

func TestTargetedUpdatePreservesNonSelectedRootDep(t *testing.T) {
	root := t.TempDir()
	irPath := writeIR(t, root, map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{{"name": "app", "version": "1.0.0", "kind": "library", "dependencies": []map[string]any{{"key": "react", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "react", "range": "^18.0.0"}}, {"key": "lodash", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "lodash", "range": "^4.0.0"}}}, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{"react", "lodash"}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}}}})

	client := &fakeClient{meta: map[string]*resolver.PackageMetadata{}, tar: map[string][]byte{}}
	react182 := fakeRegistryTarball(t, "react", "18.2.0", nil)
	react183 := fakeRegistryTarball(t, "react", "18.3.1", nil)
	lodash420 := fakeRegistryTarball(t, "lodash", "4.17.20", nil)
	lodash421 := fakeRegistryTarball(t, "lodash", "4.17.21", nil)
	client.meta["react"] = &resolver.PackageMetadata{Name: "react", Versions: map[string]resolver.PackageVersion{"18.2.0": {Name: "react", Version: "18.2.0", Dist: resolver.PackageDist{Tarball: "react-182", Integrity: fakeIntegrity(react182)}}}}
	client.meta["lodash"] = &resolver.PackageMetadata{Name: "lodash", Versions: map[string]resolver.PackageVersion{"4.17.20": {Name: "lodash", Version: "4.17.20", Dist: resolver.PackageDist{Tarball: "lodash-420", Integrity: fakeIntegrity(lodash420)}}}}
	client.tar["react-182"] = react182
	client.tar["react-183"] = react183
	client.tar["lodash-420"] = lodash420
	client.tar["lodash-421"] = lodash421

	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	opts.ResolverClient = client
	first := Update(opts)
	if hasErrors(first.Diagnostics) {
		t.Fatalf("initial update failed: %#v", first.Diagnostics)
	}
	client.meta["react"].Versions["18.3.1"] = resolver.PackageVersion{Name: "react", Version: "18.3.1", Dist: resolver.PackageDist{Tarball: "react-183", Integrity: fakeIntegrity(react183)}}
	client.meta["lodash"].Versions["4.17.21"] = resolver.PackageVersion{Name: "lodash", Version: "4.17.21", Dist: resolver.PackageDist{Tarball: "lodash-421", Integrity: fakeIntegrity(lodash421)}}
	second := UpdateWithOptions(opts, UpdateOptions{Query: "react"})
	if hasErrors(second.Diagnostics) {
		t.Fatalf("targeted update failed: %#v", second.Diagnostics)
	}
	lf, _, err := lockfile.LoadFile(opts.LockfilePath)
	if err != nil {
		t.Fatalf("load lock: %v", err)
	}
	foundReact := false
	foundLodash := false
	for _, p := range lf.Packages {
		if p.Name == "react" && p.Version == "18.3.1" {
			foundReact = true
		}
		if p.Name == "lodash" && p.Version == "4.17.20" {
			foundLodash = true
		}
	}
	if !foundReact || !foundLodash {
		t.Fatalf("expected react upgraded and lodash pinned; lock=%#v", lf.Packages)
	}
}

func TestUpdateWorkspaceRootDependencyDoesNotCopyTSPackStoreIntoItself(t *testing.T) {
	root := t.TempDir()
	mustWriteProjectFile(t, filepath.Join(root, "src", "app", "index.ts"), "import { label } from '../ui/index';\nconsole.log(label);\n")
	mustWriteProjectFile(t, filepath.Join(root, "src", "ui", "index.ts"), "export const label = 'ui';\n")
	mustWriteProjectFile(t, filepath.Join(root, "dist", "app", "index.d.ts"), "export {};\n")
	mustWriteProjectFile(t, filepath.Join(root, "dist", "ui", "index.d.ts"), "export declare const label: string;\n")
	mustWriteProjectFile(t, filepath.Join(root, "dist", "ui", "index.js"), "export const label = 'ui';\n")
	mustWriteProjectFile(t, filepath.Join(root, ".tspack", "store", "sentinel.txt"), "internal store state")
	mustWriteProjectFile(t, filepath.Join(root, "tspack-artifacts", "sentinel.txt"), "internal artifact state")

	irPath := writeIR(t, root, map[string]any{
		"format":    1,
		"workspace": map[string]any{"name": "single-root-ws"},
		"packages": []map[string]any{
			{
				"name":    "app",
				"version": "1.0.0",
				"root":    ".",
				"kind":    "app",
				"dependencies": []map[string]any{
					{"key": "ui", "kind": "dep", "source": map[string]any{"kind": "workspace", "name": "ui"}},
				},
				"targets": []map[string]any{
					{"name": "app", "export": ".", "entry": "src/app/index.ts", "runtime": "src/app/index.ts", "types": "dist/app/index.d.ts", "deps": []string{"ui"}, "peers": []string{}},
				},
				"tools":      []string{},
				"boundaries": []any{},
				"publish":    map[string]any{"include": []string{"dist/**"}, "exclude": []string{}},
				"policies":   map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}},
			},
			{
				"name":         "ui",
				"version":      "1.0.0",
				"root":         ".",
				"kind":         "library",
				"dependencies": []map[string]any{},
				"targets": []map[string]any{
					{"name": "core", "export": ".", "entry": "src/ui/index.ts", "runtime": "src/ui/index.ts", "types": "dist/ui/index.d.ts", "deps": []string{}, "peers": []string{}},
				},
				"tools":      []string{},
				"boundaries": []any{},
				"publish":    map[string]any{"include": []string{"dist/**"}, "exclude": []string{}},
				"policies":   map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}},
			},
		},
	})

	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	res := Update(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("update failed: %#v", res.Diagnostics)
	}

	lf, _, err := lockfile.LoadFile(opts.LockfilePath)
	if err != nil {
		t.Fatalf("load lock: %v", err)
	}
	if len(lf.Packages) != 1 || lf.Packages[0].Source != "workspace" {
		t.Fatalf("unexpected lock packages: %#v", lf.Packages)
	}

	extractedRoot := filepath.Join(opts.StoreRoot, "extracted")
	mustFindProjectFile(t, extractedRoot, filepath.Join("src", "ui", "index.ts"))
	mustFindProjectFile(t, extractedRoot, filepath.Join("dist", "ui", "index.js"))
	mustNotFindProjectPath(t, extractedRoot, ".tspack")
	mustNotFindProjectPath(t, extractedRoot, "tspack-artifacts")
}

func mustWriteProjectFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustFindProjectFile(t *testing.T, root, rel string) {
	t.Helper()
	found := false
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		pathRel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if pathRel == rel || strings.HasSuffix(pathRel, string(filepath.Separator)+rel) {
			found = true
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", root, walkErr)
	}
	if !found {
		t.Fatalf("expected to find %s under %s", rel, root)
	}
}

func mustNotFindProjectPath(t *testing.T, root, name string) {
	t.Helper()
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == name {
			t.Fatalf("found excluded path %s under %s", path, root)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", root, walkErr)
	}
}
