package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindManifestFrontendBridgeUsesCanonicalOverride(t *testing.T) {
	t.Setenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_CLI", "")

	overridePath := filepath.Join(t.TempDir(), "custom", "cli.js")
	writeBridgeFile(t, overridePath)
	t.Setenv("TSPACK_MANIFEST_FRONTEND", overridePath)

	resolution := findManifestFrontendBridge("cli.js")
	if resolution.Path != overridePath {
		t.Fatalf("expected override path %q, got %#v", overridePath, resolution)
	}
	if resolution.OverrideEnv != "TSPACK_MANIFEST_FRONTEND" {
		t.Fatalf("expected canonical override env, got %#v", resolution)
	}
}

func TestFindManifestFrontendBridgeAcceptsBridgeDirOverride(t *testing.T) {
	t.Setenv("TSPACK_MANIFEST_FRONTEND", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_CLI", "")

	overrideDir := filepath.Join(t.TempDir(), "bridges")
	bridgePath := filepath.Join(overrideDir, "inspect-cli.js")
	writeBridgeFile(t, bridgePath)
	t.Setenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR", overrideDir)

	resolution := findManifestFrontendBridge("inspect-cli.js")
	if resolution.Path != bridgePath {
		t.Fatalf("expected override dir bridge %q, got %#v", bridgePath, resolution)
	}
}

func TestFindManifestFrontendBridgeMissingListsSearchedPathsWithoutProjectRootLeak(t *testing.T) {
	t.Setenv("TSPACK_MANIFEST_FRONTEND", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_CLI", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR", "")

	projectRoot := t.TempDir()
	withWorkingDirectory(t, projectRoot)

	resolution := findManifestFrontendBridge("cli.js")
	if resolution.Path != "" {
		t.Fatalf("expected missing bridge, got %#v", resolution)
	}

	projectCandidate := filepath.Join(projectRoot, "manifest-frontend", "dist", "cli.js")
	joined := strings.Join(resolution.SearchedPaths, "\n")
	if strings.Contains(joined, projectCandidate) {
		t.Fatalf("project root candidate should not be searched: %#v", resolution.SearchedPaths)
	}
	if !strings.Contains(joined, "manifest-frontend") {
		t.Fatalf("expected manifest frontend candidates in %#v", resolution.SearchedPaths)
	}
}

func writeBridgeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir bridge dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("#!/usr/bin/env node\n"), 0o755); err != nil {
		t.Fatalf("write bridge: %v", err)
	}
}

func withWorkingDirectory(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previous)
	})
}
