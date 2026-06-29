package bridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveFilesystemPrefersExecutableRelativeDevPath(t *testing.T) {
	t.Setenv("TSPACK_MANIFEST_FRONTEND", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_CLI", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR", "")

	repo := t.TempDir()
	projectRoot := t.TempDir()
	executablePath := filepath.Join(repo, "dist", "tspack.exe")
	expected := filepath.Join(repo, "manifest-frontend", "dist", "cli.js")
	legacy := filepath.Join(repo, "manifest-frontend", "dist", "src", "cli.js")
	writeTestBridge(t, expected, "current")
	writeTestBridge(t, legacy, "legacy")

	resolution := ResolveWithOptions("cli.js", ResolveOptions{
		CWD:            projectRoot,
		ExecutablePath: executablePath,
		ProjectRoot:    projectRoot,
		SourceRepoRoot: repo,
	})
	if resolution.Path != expected {
		t.Fatalf("expected executable-relative bridge %q, got %#v", expected, resolution)
	}
	if resolution.SelectedSource != "executable-parent" {
		t.Fatalf("expected executable-parent source, got %#v", resolution)
	}
}

func TestResolveFilesystemUsesCanonicalWhenLegacyMissing(t *testing.T) {
	t.Setenv("TSPACK_MANIFEST_FRONTEND", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_CLI", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR", "")

	repo := t.TempDir()
	projectRoot := t.TempDir()
	executablePath := filepath.Join(repo, "dist", "tspack.exe")
	canonical := filepath.Join(repo, "manifest-frontend", "dist", "cli.js")
	writeTestBridge(t, canonical, "canonical")

	resolution := ResolveWithOptions("cli.js", ResolveOptions{
		CWD:            projectRoot,
		ExecutablePath: executablePath,
		ProjectRoot:    projectRoot,
		SourceRepoRoot: repo,
	})
	if resolution.Path != canonical {
		t.Fatalf("expected canonical bridge %q, got %#v", canonical, resolution)
	}
}

func TestResolveFilesystemFallsBackToLegacyWhenCanonicalMissing(t *testing.T) {
	t.Setenv("TSPACK_MANIFEST_FRONTEND", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_CLI", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR", "")

	repo := t.TempDir()
	projectRoot := t.TempDir()
	executablePath := filepath.Join(repo, "dist", "tspack.exe")
	legacy := filepath.Join(repo, "manifest-frontend", "dist", "src", "cli.js")
	writeTestBridge(t, legacy, "legacy")

	resolution := ResolveWithOptions("cli.js", ResolveOptions{
		CWD:            projectRoot,
		ExecutablePath: executablePath,
		ProjectRoot:    projectRoot,
		SourceRepoRoot: repo,
	})
	if resolution.Path != legacy {
		t.Fatalf("expected legacy fallback bridge %q, got %#v", legacy, resolution)
	}
}

func TestResolveFilesystemMissingListsCanonicalAndLegacyCandidates(t *testing.T) {
	t.Setenv("TSPACK_MANIFEST_FRONTEND", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_CLI", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR", "")

	repo := t.TempDir()
	projectRoot := t.TempDir()
	executablePath := filepath.Join(repo, "dist", "tspack.exe")
	canonical := filepath.Join(repo, "manifest-frontend", "dist", "cli.js")
	legacy := filepath.Join(repo, "manifest-frontend", "dist", "src", "cli.js")

	resolution := ResolveWithOptions("cli.js", ResolveOptions{
		CWD:            projectRoot,
		ExecutablePath: executablePath,
		ProjectRoot:    projectRoot,
		SourceRepoRoot: repo,
	})
	if resolution.Path != "" {
		t.Fatalf("expected missing bridge, got %#v", resolution)
	}
	joined := strings.Join(resolution.SearchedPaths, "\n")
	for _, expected := range []string{canonical, legacy} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing searched path %q in %#v", expected, resolution.SearchedPaths)
		}
	}
}

func TestResolveFilesystemDoesNotUseProjectRootAsFrontendCandidate(t *testing.T) {
	t.Setenv("TSPACK_MANIFEST_FRONTEND", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_CLI", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR", "")

	repo := t.TempDir()
	projectRoot := t.TempDir()
	executablePath := filepath.Join(repo, "dist", "tspack.exe")
	projectCandidate := filepath.Join(projectRoot, "manifest-frontend", "dist", "cli.js")

	resolution := ResolveWithOptions("cli.js", ResolveOptions{
		CWD:            projectRoot,
		ExecutablePath: executablePath,
		ProjectRoot:    projectRoot,
		SourceRepoRoot: repo,
	})
	if resolution.Path != "" {
		t.Fatalf("expected missing bridge, got %#v", resolution)
	}
	joined := strings.Join(resolution.SearchedPaths, "\n")
	if strings.Contains(joined, projectCandidate) {
		t.Fatalf("project root candidate should not be searched: %#v", resolution.SearchedPaths)
	}
}

func TestResolveFilesystemOverrideWins(t *testing.T) {
	t.Setenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR", "")
	overridePath := filepath.Join(t.TempDir(), "custom", "cli.js")
	writeTestBridge(t, overridePath, "override")
	t.Setenv("TSPACK_MANIFEST_FRONTEND", overridePath)
	t.Setenv("TSPACK_MANIFEST_FRONTEND_CLI", filepath.Join(t.TempDir(), "legacy", "cli.js"))

	resolution := ResolveWithOptions("cli.js", ResolveOptions{})
	if resolution.Path != overridePath {
		t.Fatalf("expected override bridge %q, got %#v", overridePath, resolution)
	}
	if resolution.OverrideEnv != "TSPACK_MANIFEST_FRONTEND" {
		t.Fatalf("expected canonical override env, got %#v", resolution)
	}
}

func TestResolveFilesystemMissingOverrideStopsSearch(t *testing.T) {
	t.Setenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR", "")
	overridePath := filepath.Join(t.TempDir(), "missing", "cli.js")
	t.Setenv("TSPACK_MANIFEST_FRONTEND", overridePath)
	t.Setenv("TSPACK_MANIFEST_FRONTEND_CLI", "")

	resolution := ResolveWithOptions("cli.js", ResolveOptions{
		CWD:            t.TempDir(),
		ExecutablePath: filepath.Join(t.TempDir(), "dist", "tspack.exe"),
		SourceRepoRoot: t.TempDir(),
	})
	if resolution.Path != "" {
		t.Fatalf("expected missing override to fail, got %#v", resolution)
	}
	if len(resolution.SearchedPaths) != 1 || resolution.SearchedPaths[0] != overridePath {
		t.Fatalf("expected missing override to short-circuit search, got %#v", resolution.SearchedPaths)
	}
}

func TestResolveFilesystemReportsBuildInstructions(t *testing.T) {
	resolution := ResolveWithOptions("native-test-cli.js", ResolveOptions{
		CWD:            t.TempDir(),
		ExecutablePath: filepath.Join(t.TempDir(), "dist", "tspack.exe"),
		SourceRepoRoot: t.TempDir(),
	})
	message := MissingMessage("TSPACK_TEST_XTEST_BRIDGE_MISSING", "native xTest bridge", resolution)
	for _, expected := range []string{
		"npm --prefix manifest-frontend run build",
		"./scripts/build-release.sh",
		"TSPACK_MANIFEST_FRONTEND=<path-to-cli.js>",
	} {
		if !strings.Contains(message, expected) {
			t.Fatalf("missing build instruction %q in %q", expected, message)
		}
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
