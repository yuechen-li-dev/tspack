package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/lockfile"
)

func TestInspectLockedPackagesExposesSourceAndPatchedRealization(t *testing.T) {
	root := t.TempDir()
	digest := "sha256:" + strings.Repeat("a", 64)
	realizationID := "npm:demo@1.0.0#patch=unified-text-v1-exact." + strings.Repeat("a", 64)
	locked := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: lockfile.FormatVersion, Tool: lockfile.ToolName}, Packages: []lockfile.Package{{
		ID: realizationID, Name: "demo", Version: "1.0.0", Source: "npm", Integrity: "sha512-test", Hash: "sha256:tree",
		SourceID: "npm:demo@1.0.0", SourceHash: "sha256:archive", RealizationID: realizationID,
		Patch: &lockfile.Patch{Path: "patches/demo.patch", SHA256: digest, Algorithm: "unified-text-v1-exact"},
	}}}
	contents, err := lockfile.Marshal(locked)
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(root, "ts-lock.toml")
	if err := os.WriteFile(lockPath, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	result := InspectLockedPackages(Options{LockfilePath: lockPath})
	if len(result.Diagnostics) > 0 || len(result.Packages) != 1 {
		t.Fatalf("result = %#v", result)
	}
	pkg := result.Packages[0]
	if pkg.SourceID == pkg.RealizationID || pkg.Patch == nil || pkg.Patch.SHA256 != digest {
		t.Fatalf("package provenance = %#v", pkg)
	}
}
