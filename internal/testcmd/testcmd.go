package testcmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tspack/tspack/internal/diag"
)

type Options struct {
	RootDir     string
	UseXTest    bool
	UseVitest   bool
	List        bool
	Filter      string
	XTestBridge string
}

type Result struct {
	Diagnostics []diag.Diagnostic
	ExitCode    int
}

func Run(opts Options) Result {
	selected := selectedBackends(opts)
	if len(selected) == 0 {
		selected = autoDetectBackends(opts.RootDir)
	}
	if len(selected) == 0 {
		return Result{Diagnostics: []diag.Diagnostic{{Code: "TSPACK_TEST_NO_BACKENDS", Severity: diag.SeverityError, Message: "no test backends discovered"}}, ExitCode: 1}
	}

	result := Result{}
	for _, backend := range selected {
		switch backend {
		case "xtest":
			runXTest(opts, &result)
		case "vitest":
			runVitest(opts, &result)
		}
	}
	if hasErrors(result.Diagnostics) || result.ExitCode != 0 {
		result.ExitCode = 1
	}
	return result
}

func selectedBackends(opts Options) []string {
	var backends []string
	if opts.UseXTest {
		backends = append(backends, "xtest")
	}
	if opts.UseVitest {
		backends = append(backends, "vitest")
	}
	return backends
}

func autoDetectBackends(root string) []string {
	var backends []string
	if hasXTests(root) {
		backends = append(backends, "xtest")
	}
	if vitestAvailable(root) {
		backends = append(backends, "vitest")
	}
	return backends
}

func hasXTests(root string) bool {
	found := false
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == "node_modules" || name == ".git" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".xtest.tsx") {
			found = true
			return fmt.Errorf("found")
		}
		return nil
	})
	return found
}

func vitestAvailable(root string) bool {
	_, err := os.Stat(filepath.Join(root, "node_modules", ".bin", "vitest"))
	return err == nil
}

func runXTest(opts Options, result *Result) {
	bridge := opts.XTestBridge
	if bridge == "" {
		bridge = filepath.Join("manifest-frontend", "dist", "src", "native-test-cli.js")
	}
	if _, err := os.Stat(bridge); err != nil {
		result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_TEST_XTEST_BRIDGE_MISSING", Severity: diag.SeverityError, Message: "native xTest bridge not found", Details: []string{bridge}})
		result.ExitCode = 1
		return
	}

	args := []string{bridge, "test", "--root", opts.RootDir}
	if opts.List {
		args = append(args, "--list")
	}
	if opts.Filter != "" {
		args = append(args, "--filter", opts.Filter)
	}
	cmd := exec.Command("node", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			result.ExitCode = 1
			return
		}
		result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_TEST_XTEST_FAILED", Severity: diag.SeverityError, Message: "xTest backend failed", Details: []string{err.Error()}})
		result.ExitCode = 1
	}
}

func runVitest(opts Options, result *Result) {
	if opts.List {
		result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_TEST_BACKEND_LIST_UNSUPPORTED", Severity: diag.SeverityWarning, Message: "Vitest list mode is not supported in M18"})
		return
	}
	if !vitestAvailable(opts.RootDir) {
		result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_TEST_VITEST_NOT_AVAILABLE", Severity: diag.SeverityError, Message: "vitest is not available"})
		result.ExitCode = 1
		return
	}
	args := []string{"run"}
	if opts.Filter != "" {
		args = append(args, "-t", opts.Filter)
	}
	bin := filepath.Join(opts.RootDir, "node_modules", ".bin", "vitest")
	cmd := exec.Command(bin, args...)
	cmd.Dir = opts.RootDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			result.ExitCode = 1
			return
		}
		result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_TEST_VITEST_FAILED_TO_START", Severity: diag.SeverityError, Message: "failed to start vitest", Details: []string{fmt.Sprintf("%v", err)}})
		result.ExitCode = 1
	}
}

func hasErrors(diags []diag.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return true
		}
	}
	return false
}
