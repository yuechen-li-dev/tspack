package project

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/resolver"
)

func TestStoreJobsFromEnv(t *testing.T) {
	t.Setenv("TSPACK_STORE_JOBS", "")
	if jobs, err := storeJobsFromEnv(); err != nil || jobs < 2 || jobs > 8 {
		t.Fatalf("default jobs = %d, %v; want bounded value greater than one", jobs, err)
	}

	t.Setenv("TSPACK_STORE_JOBS", "1")
	if jobs, err := storeJobsFromEnv(); err != nil || jobs != 1 {
		t.Fatalf("jobs=1 parsed as %d, %v", jobs, err)
	}

	t.Setenv("TSPACK_STORE_JOBS", "4")
	if jobs, err := storeJobsFromEnv(); err != nil || jobs != 4 {
		t.Fatalf("jobs=4 parsed as %d, %v", jobs, err)
	}

	t.Setenv("TSPACK_STORE_JOBS", "0")
	if _, err := storeJobsFromEnv(); err == nil {
		t.Fatalf("expected invalid jobs error")
	}
}

func TestUpdateFailsClearlyForInvalidStoreJobs(t *testing.T) {
	root := t.TempDir()
	irPath := writeIR(t, root, map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{{"name": "app", "version": "1.0.0", "kind": "library", "dependencies": []map[string]any{{"key": "dep-a", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "dep-a", "range": "1.0.0"}}}, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{"dep-a"}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}}}})
	registry := newFakeRegistryServer(t)
	defer registry.Close()
	t.Setenv("TSPACK_STORE_JOBS", "nope")

	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	opts.ResolverClient = resolver.NewHTTPRegistryClient(registry.URL)
	res := Update(opts)
	if !hasDiagnosticCode(res.Diagnostics, "TSPACK_UPDATE_STORE_JOBS_INVALID") {
		t.Fatalf("expected invalid jobs diagnostic, got %#v", res.Diagnostics)
	}
}

func TestUpdateLockfileDeterministicAcrossStoreJobs(t *testing.T) {
	firstRoot := t.TempDir()
	firstLock := runFakeRegistryUpdateWithJobs(t, firstRoot, "1")
	secondRoot := t.TempDir()
	secondLock := runFakeRegistryUpdateWithJobs(t, secondRoot, "4")
	if !bytes.Equal(firstLock, secondLock) {
		t.Fatalf("lockfile changed between jobs=1 and jobs=4\n--- jobs=1 ---\n%s\n--- jobs=4 ---\n%s", firstLock, secondLock)
	}
}

func runFakeRegistryUpdateWithJobs(t *testing.T, root string, jobs string) []byte {
	t.Helper()
	irPath := writeIR(t, root, map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{{"name": "app", "version": "1.0.0", "kind": "library", "dependencies": []map[string]any{{"key": "dep-a", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "dep-a", "range": "1.0.0"}}}, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{"dep-a"}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}}}})
	registry := newFakeRegistryServer(t)
	defer registry.Close()
	t.Setenv("TSPACK_STORE_JOBS", jobs)

	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	opts.ResolverClient = resolver.NewHTTPRegistryClient(registry.URL)
	res := Update(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("update failed: %#v", res.Diagnostics)
	}
	lf, _, err := lockfile.LoadFile(opts.LockfilePath)
	if err != nil {
		t.Fatalf("load lock: %v", err)
	}
	if len(lf.Packages) != 2 {
		t.Fatalf("unexpected package set: %#v", lf.Packages)
	}
	if _, err := os.Stat(filepath.Join(opts.StoreRoot, "metadata")); err != nil {
		t.Fatalf("expected store metadata: %v", err)
	}
	b, err := os.ReadFile(opts.LockfilePath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	return b
}

func hasDiagnosticCode(diags []diag.Diagnostic, code string) bool {
	for _, diagnostic := range diags {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
