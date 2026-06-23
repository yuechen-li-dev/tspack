package bridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveFilesystemPrefersCurrentDistPath(t *testing.T) {
	t.Setenv("TSPACK_MANIFEST_FRONTEND_CLI", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR", "")
	root := t.TempDir()
	withWorkingDirectory(t, root)

	current := filepath.Join("manifest-frontend", "dist", "cli.js")
	legacy := filepath.Join("manifest-frontend", "dist", "src", "cli.js")
	writeTestBridge(t, current, "current")
	writeTestBridge(t, legacy, "legacy")

	resolution := ResolveFilesystem("cli.js")
	if resolution.Path != current {
		t.Fatalf("expected current bridge %q, got %#v", current, resolution)
	}
}

func TestResolveFilesystemReportsBuildInstructions(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)

	resolution := ResolveFilesystem("native-test-cli.js")
	message := MissingMessage("TSPACK_TEST_XTEST_BRIDGE_MISSING", "native xTest bridge", resolution)
	if !strings.Contains(message, "cd manifest-frontend && npm run build") {
		t.Fatalf("missing build instruction in %q", message)
	}
	if !strings.Contains(message, "./scripts/build-release.sh") {
		t.Fatalf("missing release build instruction in %q", message)
	}
}

func writeTestBridge(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir bridge dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
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
	t.Cleanup(func() { _ = os.Chdir(previous) })
}
