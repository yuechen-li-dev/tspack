package testcmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/bridge"
	"github.com/yuechen-li-dev/tspack/internal/diag"
)

type Options struct {
	RootDir         string
	UseXTest        bool
	UseVitest       bool
	List            bool
	Filter          string
	Compact         bool
	XTestBridge     string
	Watch           bool
	JSON            bool
	UpdateSnapshots bool
	Batch           bool
}

type Result struct {
	Diagnostics []diag.Diagnostic
	ExitCode    int
}

type BridgeResolution struct {
	Path          string
	SearchedPaths []string
	CWD           string
	Executable    string
}

func Run(opts Options) Result {
	return RunContext(context.Background(), opts)
}

func RunContext(ctx context.Context, opts Options) Result {
	if opts.Watch {
		return runWatch(ctx, opts, os.Stderr)
	}

	selected := selectedBackends(opts)
	if len(selected) == 0 {
		selected = autoDetectBackends(opts.RootDir)
		if opts.Batch && containsString(selected, "xtest") {
			selected = []string{"xtest"}
		}
	}
	if len(selected) == 0 {
		return Result{Diagnostics: []diag.Diagnostic{{Code: "TSPACK_TEST_NO_BACKENDS", Severity: diag.SeverityError, Message: "no test backends discovered"}}, ExitCode: 1}
	}

	result := Result{}
	for _, backend := range selected {
		switch backend {
		case "xtest":
			runXTestContext(ctx, opts, &result)
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
			if ignoredTestDiscoveryDir(name) {
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

func ignoredTestDiscoveryDir(name string) bool {
	switch name {
	case "node_modules", ".git", ".tspack", "dist", "tspack-artifacts":
		return true
	default:
		return false
	}
}

func vitestAvailable(root string) bool {
	_, err := os.Stat(filepath.Join(root, "node_modules", ".bin", "vitest"))
	return err == nil
}

func runXTest(opts Options, result *Result) {
	runXTestContext(context.Background(), opts, result)
}

func runXTestContext(ctx context.Context, opts Options, result *Result) {
	resolution := ResolveXTestBridge(opts.XTestBridge)
	if resolution.Path == "" {
		result.Diagnostics = append(result.Diagnostics, missingBridgeDiagnostic(resolution))
		result.ExitCode = 1
		return
	}

	args := []string{resolution.Path, "test", "--root", opts.RootDir}
	if opts.List {
		args = append(args, "--list")
	}
	if opts.JSON {
		args = append(args, "--json")
	}
	if opts.Filter != "" {
		args = append(args, "--filter", opts.Filter)
	}
	if opts.Compact && !opts.List {
		args = append(args, "--compact")
	}
	if opts.UpdateSnapshots && !opts.List {
		args = append(args, "--update-snapshots")
	}
	if opts.Batch && !opts.List {
		args = append(args, "--batch")
	}
	cmd := exec.CommandContext(ctx, "node", args...)
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

func ResolveXTestBridge(explicitPath string) BridgeResolution {
	cwd, _ := os.Getwd()
	executable, _ := os.Executable()
	resolution := BridgeResolution{CWD: cwd, Executable: executable}

	if embedded := bridge.Resolve("native-test-cli.js"); embedded.Path != "" && explicitPath == "" && os.Getenv("TSPACK_XTEST_BRIDGE") == "" {
		resolution.Path = embedded.Path
		return resolution
	}

	if explicitPath != "" {
		resolution.SearchedPaths = append(resolution.SearchedPaths, explicitPath)
		if fileExists(explicitPath) {
			resolution.Path = explicitPath
		}
		return resolution
	}

	if envPath := os.Getenv("TSPACK_XTEST_BRIDGE"); envPath != "" {
		resolution.SearchedPaths = append(resolution.SearchedPaths, envPath)
		if fileExists(envPath) {
			resolution.Path = envPath
			return resolution
		}
	}

	for _, candidate := range defaultBridgeCandidates(cwd, executable) {
		if candidate == "" || containsString(resolution.SearchedPaths, candidate) {
			continue
		}
		resolution.SearchedPaths = append(resolution.SearchedPaths, candidate)
		if fileExists(candidate) {
			resolution.Path = candidate
			return resolution
		}
	}

	return resolution
}

func defaultBridgeCandidates(cwd string, executable string) []string {
	var candidates []string
	if executable != "" {
		execDir := filepath.Dir(executable)
		candidates = append(candidates, manifestFrontendBridgeCandidates(execDir, "native-test-cli.js")...)
		candidates = append(candidates, manifestFrontendBridgeCandidates(filepath.Join(execDir, ".."), "native-test-cli.js")...)
		candidates = append(candidates, manifestFrontendSharedBridgeCandidates(execDir, "native-test-cli.js")...)
	}
	if sourceRepo := sourceRepositoryRoot(); sourceRepo != "" {
		candidates = append(candidates, manifestFrontendBridgeCandidates(sourceRepo, "native-test-cli.js")...)
	}
	if cwd != "" {
		candidates = append(candidates, manifestFrontendBridgeCandidates(cwd, "native-test-cli.js")...)
	}
	return candidates
}

func manifestFrontendBridgeCandidates(root string, bridgeName string) []string {
	return []string{
		filepath.Join(root, "manifest-frontend", "dist", bridgeName),
		filepath.Join(root, "manifest-frontend", "dist", "src", bridgeName),
	}
}

func manifestFrontendSharedBridgeCandidates(execDir string, bridgeName string) []string {
	return []string{
		filepath.Join(execDir, "..", "share", "tspack", "manifest-frontend", "dist", bridgeName),
		filepath.Join(execDir, "..", "share", "tspack", "manifest-frontend", "dist", "src", bridgeName),
	}
}

func sourceRepositoryRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	dir := filepath.Dir(file)
	for {
		if fileExists(filepath.Join(dir, "go.mod")) && dirContainsManifestFrontend(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func dirContainsManifestFrontend(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "manifest-frontend"))
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func missingBridgeDiagnostic(resolution BridgeResolution) diag.Diagnostic {
	details := bridge.BuildNeededDetails()
	if len(resolution.SearchedPaths) > 0 {
		details = append(details, "searched paths:")
		for _, searched := range resolution.SearchedPaths {
			details = append(details, "  "+searched)
		}
	}
	if resolution.CWD != "" {
		details = append(details, "cwd: "+resolution.CWD)
	}
	if resolution.Executable != "" {
		details = append(details, "executable: "+resolution.Executable)
	}
	return diag.Diagnostic{Code: "TSPACK_TEST_XTEST_BRIDGE_MISSING", Severity: diag.SeverityError, Message: "native xTest bridge not found", Details: details}
}

func runVitest(opts Options, result *Result) {
	if opts.Batch {
		result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_TEST_BATCH_UNSUPPORTED_BACKEND", Severity: diag.SeverityError, Message: "batch execution only applies to native xTest"})
		result.ExitCode = 1
		return
	}
	if opts.Compact {
		result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_TEST_COMPACT_UNSUPPORTED_BACKEND", Severity: diag.SeverityWarning, Message: "compact output only applies to native xTest; Vitest output is unchanged"})
	}
	if opts.UpdateSnapshots {
		result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_SNAPSHOT_UNSUPPORTED_BACKEND", Severity: diag.SeverityError, Message: "snapshot updates only apply to native xTest"})
		result.ExitCode = 1
		return
	}
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
