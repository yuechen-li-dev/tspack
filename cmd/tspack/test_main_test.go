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
	legacyCLI := filepath.Join(repo, "manifest-frontend", "dist", "src", "cli.js")
	legacyDir := filepath.Dir(legacyCLI)
	_ = os.Setenv("TSPACK_MANIFEST_FRONTEND_CLI", legacyCLI)
	_ = os.Setenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR", legacyDir)
	os.Exit(m.Run())
}
