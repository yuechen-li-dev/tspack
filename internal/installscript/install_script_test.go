package installscript

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallScriptInstallsFromFakeReleaseServer(t *testing.T) {
	repoRoot := findRepoRoot(t)
	archiveBytes := buildArchive(t, "tspack-linux-amd64/tspack", "#!/usr/bin/env sh\necho fake tspack\n")
	archiveHash := sha256.Sum256(archiveBytes)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/repos/test/tspack/releases/latest":
			_, _ = w.Write([]byte(`{"tag_name":"v0.0.0-test"}`))
		case "/test/tspack/releases/download/v0.0.0-test/tspack-linux-amd64.tar.gz":
			_, _ = w.Write(archiveBytes)
		case "/test/tspack/releases/download/v0.0.0-test/checksums.txt":
			_, _ = fmt.Fprintf(w, "%x  tspack-linux-amd64.tar.gz\n", archiveHash)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	installDir := t.TempDir()
	cmd := installCommand(t, repoRoot, installDir)
	cmd.Env = append(cmd.Env,
		"TSPACK_REPO=test/tspack",
		"TSPACK_API_BASE="+server.URL+"/api",
		"TSPACK_GITHUB_BASE="+server.URL,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install script failed: %v\n%s", err, output)
	}

	installedPath := filepath.Join(installDir, "tspack")
	info, err := os.Stat(installedPath)
	if err != nil {
		t.Fatalf("installed binary missing: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Fatalf("installed binary is not executable: mode %s", info.Mode())
	}
}

func TestInstallScriptRejectsChecksumMismatch(t *testing.T) {
	repoRoot := findRepoRoot(t)
	archiveBytes := buildArchive(t, "tspack-linux-amd64/tspack", "#!/usr/bin/env sh\necho fake tspack\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/test/tspack/releases/download/v0.0.0-test/tspack-linux-amd64.tar.gz":
			_, _ = w.Write(archiveBytes)
		case "/test/tspack/releases/download/v0.0.0-test/checksums.txt":
			_, _ = w.Write([]byte(strings.Repeat("0", 64) + "  tspack-linux-amd64.tar.gz\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cmd := installCommand(t, repoRoot, t.TempDir())
	cmd.Env = append(cmd.Env,
		"TSPACK_VERSION=v0.0.0-test",
		"TSPACK_REPO=test/tspack",
		"TSPACK_GITHUB_BASE="+server.URL,
	)

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("install script succeeded with checksum mismatch:\n%s", output)
	}
	if !strings.Contains(string(output), "Checksum mismatch") {
		t.Fatalf("missing checksum mismatch message:\n%s", output)
	}
}

func TestInstallScriptDryRunPlatformMapping(t *testing.T) {
	repoRoot := findRepoRoot(t)

	cases := []struct {
		name     string
		osName   string
		archName string
		want     string
	}{
		{name: "linux amd64", osName: "Linux", archName: "x86_64", want: "artifact: tspack-linux-amd64.tar.gz"},
		{name: "linux arm64", osName: "Linux", archName: "aarch64", want: "artifact: tspack-linux-arm64.tar.gz"},
		{name: "darwin amd64", osName: "Darwin", archName: "x86_64", want: "artifact: tspack-darwin-amd64.tar.gz"},
		{name: "darwin arm64", osName: "Darwin", archName: "arm64", want: "artifact: tspack-darwin-arm64.tar.gz"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("sh", filepath.Join(repoRoot, "scripts", "install.sh"))
			cmd.Dir = repoRoot
			cmd.Env = append(os.Environ(),
				"TSPACK_INSTALL_DRY_RUN=1",
				"TSPACK_VERSION=v0.0.0-test",
				"TSPACK_TEST_OS="+tc.osName,
				"TSPACK_TEST_ARCH="+tc.archName,
			)

			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("dry run failed: %v\n%s", err, output)
			}
			if !strings.Contains(string(output), tc.want) {
				t.Fatalf("dry run output missing %q:\n%s", tc.want, output)
			}
		})
	}
}

func TestInstallScriptRejectsUnsupportedPlatform(t *testing.T) {
	repoRoot := findRepoRoot(t)
	cmd := exec.Command("sh", filepath.Join(repoRoot, "scripts", "install.sh"))
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"TSPACK_INSTALL_DRY_RUN=1",
		"TSPACK_VERSION=v0.0.0-test",
		"TSPACK_TEST_OS=Plan9",
		"TSPACK_TEST_ARCH=x86_64",
	)

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("unsupported platform succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), "Unsupported operating system: Plan9") {
		t.Fatalf("missing unsupported OS message:\n%s", output)
	}
}

func installCommand(t *testing.T, repoRoot string, installDir string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sh", filepath.Join(repoRoot, "scripts", "install.sh"))
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"TSPACK_INSTALL_DIR="+installDir,
		"TSPACK_TEST_OS=Linux",
		"TSPACK_TEST_ARCH=x86_64",
	)
	return cmd
}

func buildArchive(t *testing.T, binaryPath string, binaryContents string) []byte {
	t.Helper()

	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)

	contentBytes := []byte(binaryContents)
	header := &tar.Header{
		Name: binaryPath,
		Mode: 0755,
		Size: int64(len(contentBytes)),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tarWriter.Write(contentBytes); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	return buffer.Bytes()
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if _, err := os.Stat(filepath.Join(repoRoot, "scripts", "install.sh")); err != nil {
		t.Fatalf("repo root detection failed: %v", err)
	}
	return repoRoot
}
