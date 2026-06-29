package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve repo root: %v\n", err)
		os.Exit(1)
	}
	currentCLI := filepath.Join(repo, "manifest-frontend", "dist", "cli.js")
	legacyCLI := filepath.Join(repo, "manifest-frontend", "dist", "src", "cli.js")
	bridgeCLI := legacyCLI
	if _, err := os.Stat(bridgeCLI); err != nil {
		if currentData, readErr := os.ReadFile(currentCLI); readErr == nil {
			_ = os.MkdirAll(filepath.Dir(legacyCLI), 0o755)
			_ = os.WriteFile(legacyCLI, currentData, 0o755)
		}
	}
	if _, err := os.Stat(bridgeCLI); err != nil {
		bridgeCLI = currentCLI
	}
	bridgeDir := filepath.Dir(bridgeCLI)
	_ = os.Setenv("TSPACK_MANIFEST_FRONTEND_CLI", bridgeCLI)
	_ = os.Setenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR", bridgeDir)
	os.Exit(m.Run())
}
