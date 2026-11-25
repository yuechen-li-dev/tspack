package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/lockfile"
)

func TestSyncPreservesManifestFailureBeforeValidatingLockedPatches(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.FrontendCLIPath = filepath.Join(dir, "missing-frontend.js")
	locked := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"}}
	contents, err := lockfile.Marshal(locked)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(opts.LockfilePath, contents, 0o644); err != nil {
		t.Fatal(err)
	}

	result := Sync(opts, false, false)
	if !hasErrCode(result.Diagnostics, "TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED") {
		t.Fatalf("expected original manifest failure, got %#v", result.Diagnostics)
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("failed manifest must not materialize dependencies: %v", err)
	}
}
