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
	locked := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: lockfile.FormatVersion, Tool: lockfile.ToolName}, Packages: []lockfile.Package{
		{
			ID: realizationID, Name: "demo", Version: "1.0.0", Source: "npm", Integrity: "sha512-test", Hash: "sha256:tree",
			SourceID: "npm:demo@1.0.0", SourceHash: "sha256:archive", RealizationID: realizationID,
			Patch: &lockfile.Patch{Path: "patches/demo.patch", SHA256: digest, Algorithm: "unified-text-v1-exact"},
		},
		{ID: "npm:react@19.0.0", Name: "react", Version: "19.0.0", Source: "npm", Hash: "sha256:react"},
	}, Requirements: []lockfile.Requirement{{ID: "peer-demo-react", Scope: "workspace", Kind: "peer", PackageID: realizationID, TargetSource: "npm", TargetName: "react", Reference: "react", Constraint: "^19.0.0", SelectedVersion: "19.0.0", Status: "controlling", Controlling: true}}}
	lockfile.RebuildModuleInstances(locked)
	contents, err := lockfile.Marshal(locked)
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(root, "ts-lock.toml")
	if err := os.WriteFile(lockPath, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	result := InspectLockedPackages(Options{LockfilePath: lockPath})
	if len(result.Diagnostics) > 0 || len(result.Packages) != 2 {
		t.Fatalf("result = %#v", result)
	}
	var pkg LockedPackageSummary
	for _, candidate := range result.Packages {
		if candidate.Name == "demo" {
			pkg = candidate
		}
	}
	if pkg.SourceID == pkg.RealizationID || pkg.Patch == nil || pkg.Patch.SHA256 != digest {
		t.Fatalf("package provenance = %#v", pkg)
	}
	if len(pkg.Instances) != 1 || len(pkg.Instances[0].Peers) != 1 || pkg.Instances[0].Peers[0].RealizationID != "npm:react@19.0.0" {
		t.Fatalf("module instance provenance = %#v", pkg.Instances)
	}
}
