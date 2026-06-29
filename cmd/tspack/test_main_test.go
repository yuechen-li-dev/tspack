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
	bridgeDir := filepath.Join(repo, "manifest-frontend", "dist")
	_ = os.Unsetenv("TSPACK_MANIFEST_FRONTEND")
	_ = os.Unsetenv("TSPACK_MANIFEST_FRONTEND_CLI")
	_ = os.Setenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR", bridgeDir)
	os.Exit(m.Run())
}
