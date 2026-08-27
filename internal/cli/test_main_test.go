package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/cli/clitest"
	"github.com/yuechen-li-dev/tspack/internal/manifestfrontend"
)

var testTspackBinary string
var testFixtureBridgeDir string

func runTestApp(t testing.TB, args ...string) clitest.Result {
	t.Helper()
	return clitest.RunApp(t, NewDefaultApp(), args...)
}

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

	tempDirectory, err := os.MkdirTemp("", "tspack-cli-tests-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create CLI test directory: %v\n", err)
		os.Exit(1)
	}
	testFixtureBridgeDir = filepath.Join(tempDirectory, "manifest-frontend-fixture")
	testTspackBinary = filepath.Join(tempDirectory, "tspack-test-bin")
	if runtime.GOOS == "windows" {
		testTspackBinary += ".exe"
	}
	buildCommand := exec.Command("go", "build", "-o", testTspackBinary, "./cmd/tspack")
	buildCommand.Dir = repo
	if output, buildErr := buildCommand.CombinedOutput(); buildErr != nil {
		fmt.Fprintf(os.Stderr, "build shared CLI test binary: %v\n%s", buildErr, output)
		_ = os.RemoveAll(tempDirectory)
		os.Exit(1)
	}

	exitCode := m.Run()
	manifestfrontend.CloseWorkers()
	_ = os.RemoveAll(tempDirectory)
	os.Exit(exitCode)
}
