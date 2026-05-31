package main

import (
	"fmt"
	"os"
	"path/filepath"
)

type manifestFrontendBridgeResolution struct {
	Path          string
	SearchedPaths []string
}

func findManifestFrontendBridge(bridgeName string) manifestFrontendBridgeResolution {
	candidates := []string{}
	if overrideDir := os.Getenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR"); overrideDir != "" {
		candidates = append(candidates, filepath.Join(overrideDir, bridgeName))
	}
	if bridgeName == "cli.js" {
		if override := os.Getenv("TSPACK_MANIFEST_FRONTEND_CLI"); override != "" {
			candidates = append(candidates, override)
		}
	}
	candidates = append(candidates,
		filepath.Join("manifest-frontend", "dist", bridgeName),
		filepath.Join("manifest-frontend", "dist", "src", bridgeName),
	)
	resolution := manifestFrontendBridgeResolution{SearchedPaths: candidates}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			resolution.Path = candidate
			return resolution
		}
	}
	return resolution
}

func manifestFrontendCLIPath() string {
	resolution := findManifestFrontendBridge("cli.js")
	if resolution.Path != "" {
		return resolution.Path
	}
	return resolution.SearchedPaths[0]
}

func requireManifestFrontendBridge(bridgeName string, code string, label string) string {
	resolution := findManifestFrontendBridge(bridgeName)
	if resolution.Path != "" {
		return resolution.Path
	}
	fmt.Fprintf(os.Stderr, "%s: %s not found\n", code, label)
	fmt.Fprintln(os.Stderr, "searched paths:")
	for _, searched := range resolution.SearchedPaths {
		fmt.Fprintf(os.Stderr, "  %s\n", searched)
	}
	os.Exit(1)
	return ""
}
