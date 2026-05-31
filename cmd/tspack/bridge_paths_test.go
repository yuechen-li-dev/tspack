package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindManifestFrontendBridgePrefersCurrentDistPath(t *testing.T) {
	t.Setenv("TSPACK_MANIFEST_FRONTEND_CLI", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR", "")
	for _, bridgeName := range []string{"cli.js", "inspect-cli.js", "native-test-cli.js"} {
		t.Run(bridgeName, func(t *testing.T) {
			root := t.TempDir()
			withWorkingDirectory(t, root)
			current := filepath.Join("manifest-frontend", "dist", bridgeName)
			legacy := filepath.Join("manifest-frontend", "dist", "src", bridgeName)
			writeBridgeFile(t, current)
			writeBridgeFile(t, legacy)

			resolution := findManifestFrontendBridge(bridgeName)
			if resolution.Path != current {
				t.Fatalf("expected current dist path %q, got %#v", current, resolution)
			}
		})
	}
}

func TestFindManifestFrontendBridgeAcceptsLegacyDistSrcPath(t *testing.T) {
	t.Setenv("TSPACK_MANIFEST_FRONTEND_CLI", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR", "")
	for _, bridgeName := range []string{"cli.js", "inspect-cli.js", "native-test-cli.js"} {
		t.Run(bridgeName, func(t *testing.T) {
			root := t.TempDir()
			withWorkingDirectory(t, root)
			legacy := filepath.Join("manifest-frontend", "dist", "src", bridgeName)
			writeBridgeFile(t, legacy)

			resolution := findManifestFrontendBridge(bridgeName)
			if resolution.Path != legacy {
				t.Fatalf("expected legacy dist/src path %q, got %#v", legacy, resolution)
			}
		})
	}
}

func TestFindManifestFrontendBridgeMissingListsSearchedPaths(t *testing.T) {
	t.Setenv("TSPACK_MANIFEST_FRONTEND_CLI", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR", "")
	for _, bridgeName := range []string{"cli.js", "inspect-cli.js", "native-test-cli.js"} {
		t.Run(bridgeName, func(t *testing.T) {
			root := t.TempDir()
			withWorkingDirectory(t, root)

			resolution := findManifestFrontendBridge(bridgeName)
			if resolution.Path != "" {
				t.Fatalf("expected missing bridge, got %#v", resolution)
			}
			joined := strings.Join(resolution.SearchedPaths, "\n")
			for _, expected := range []string{
				filepath.Join("manifest-frontend", "dist", bridgeName),
				filepath.Join("manifest-frontend", "dist", "src", bridgeName),
			} {
				if !strings.Contains(joined, expected) {
					t.Fatalf("expected searched path %q in %#v", expected, resolution.SearchedPaths)
				}
			}
		})
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

func repoRootForBridgePathTest(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return cwd
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			t.Fatalf("could not find repository root")
		}
		cwd = parent
	}
}
