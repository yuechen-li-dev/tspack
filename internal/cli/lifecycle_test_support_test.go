package cli

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type checkJSONReport struct {
	Command     string                `json:"command"`
	OK          bool                  `json:"ok"`
	Summary     checkJSONSummary      `json:"summary"`
	Diagnostics []checkJSONDiagnostic `json:"diagnostics"`
}

type checkJSONSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`
	Total    int `json:"total"`
}

type checkJSONDiagnostic struct {
	Code                string   `json:"code"`
	Severity            string   `json:"severity"`
	Message             string   `json:"message"`
	LifecycleScriptName string   `json:"lifecycleScriptName"`
	LifecycleCategory   string   `json:"lifecycleCategory"`
	ConsumerInstallTime *bool    `json:"consumerInstallTime"`
	Details             []string `json:"details"`
}
type updateDryRunJSONReport struct {
	Command string `json:"command"`
	DryRun  struct {
		Enabled bool `json:"enabled"`
		Changed bool `json:"changed"`
		Summary struct {
			Added     int `json:"added"`
			Removed   int `json:"removed"`
			Changed   int `json:"changed"`
			Unchanged int `json:"unchanged"`
		} `json:"summary"`
	} `json:"dryRun"`
	OK      bool `json:"ok"`
	Summary struct {
		Added     int `json:"added"`
		Removed   int `json:"removed"`
		Changed   int `json:"changed"`
		Unchanged int `json:"unchanged"`
	} `json:"summary"`
}

func buildTspackBinary(t *testing.T, repo string) string {
	t.Helper()
	if testTspackBinary == "" {
		t.Fatal("shared tspack test binary was not initialized")
	}
	return testTspackBinary
}

func reservePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func portEventuallyClosed(t *testing.T, port int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 150*time.Millisecond)
		if err != nil {
			return
		}
		_ = conn.Close()
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("port %d was still accepting connections after %s", port, timeout)
}

func testExecutablePath(path string) string {
	if runtime.GOOS == "windows" && filepath.Ext(path) == "" {
		return path + ".cmd"
	}
	return path
}

func writeNodeBackedExecutable(t *testing.T, path string, script string) string {
	t.Helper()

	actualPath := testExecutablePath(path)
	if err := os.MkdirAll(filepath.Dir(actualPath), 0o755); err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS != "windows" {
		if err := os.WriteFile(actualPath, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		return actualPath
	}

	jsPath := strings.TrimSuffix(actualPath, ".cmd") + ".js"
	if err := os.WriteFile(jsPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	wrapper := fmt.Sprintf("@echo off\r\nnode %s %%*\r\n", windowsBatchQuote(jsPath))
	if err := os.WriteFile(actualPath, []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	return actualPath
}

func copyFile(t *testing.T, src string, dst string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	if err := out.Chmod(0o755); err != nil {
		t.Fatal(err)
	}
}

func testManifestFrontendBridgeDir(t *testing.T) string {
	t.Helper()
	if existing := os.Getenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR"); existing != "" {
		if !strings.Contains(filepath.ToSlash(existing), "manifest-frontend/dist") {
			return existing
		}
	}
	frontend := filepath.Join(t.TempDir(), "manifest-frontend-bridges")
	if err := os.MkdirAll(frontend, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TSPACK_MANIFEST_FRONTEND", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_CLI", "")
	t.Setenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR", frontend)
	return frontend
}
