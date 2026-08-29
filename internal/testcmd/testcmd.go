package testcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/bridge"
	"github.com/yuechen-li-dev/tspack/internal/diag"
)

type Options struct {
	RootDir           string
	UseXTest          bool
	UseVitest         bool
	List              bool
	Filter            string
	Compact           bool
	XTestBridge       string
	Watch             bool
	JSON              bool
	UpdateSnapshots   bool
	Batch             bool
	Environment       []string
	CaptureStructured bool
	Quiet             bool
	VitestCwd         string
	VitestConfig      string
	VitestFiles       []string
	VitestProject     string
}

type Result struct {
	Diagnostics []diag.Diagnostic
	ExitCode    int
	Summary     Summary
	Tests       []TestEvidence
}

type Summary struct {
	Passed     int     `json:"passed"`
	Failed     int     `json:"failed"`
	Skipped    int     `json:"skipped"`
	DurationMs float64 `json:"durationMs,omitempty"`
}

type TestEvidence struct {
	ID         string  `json:"id"`
	Name       string  `json:"name,omitempty"`
	Status     string  `json:"status"`
	DurationMs float64 `json:"durationMs,omitempty"`
	Failure    any     `json:"failure,omitempty"`
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
		if len(selected) == 1 && selected[0] == "vitest" && ambiguousWorkspaceVitestRoot(opts.RootDir) {
			return Result{
				Diagnostics: []diag.Diagnostic{{
					Code:     "TSPACK_TEST_AMBIGUOUS_WORKSPACE_VITEST",
					Severity: diag.SeverityError,
					Message:  "refusing to auto-run Vitest across an unconfigured package-manager workspace root",
					Details: []string{
						"workspace root: " + opts.RootDir,
						"Vitest was discovered in node_modules, but no root Vite/Vitest config declares repository-level test intent.",
					},
					Fixes: []string{
						"Run `tspack test --vitest` only if an unconfigured root Vitest run is intentional.",
						"Otherwise declare package-scoped TSPack test intent before running from the workspace root.",
					},
				}},
				ExitCode: 1,
			}
		}
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
			runVitestContext(ctx, opts, &result)
		}
	}
	if hasErrors(result.Diagnostics) || result.ExitCode != 0 {
		result.ExitCode = 1
	}
	return result
}

func ambiguousWorkspaceVitestRoot(root string) bool {
	if _, err := os.Stat(filepath.Join(root, "pnpm-workspace.yaml")); err != nil {
		return false
	}
	for _, name := range []string{
		"vitest.config.ts",
		"vitest.config.mts",
		"vitest.config.cts",
		"vitest.config.js",
		"vitest.config.mjs",
		"vitest.config.cjs",
		"vite.config.ts",
		"vite.config.mts",
		"vite.config.cts",
		"vite.config.js",
		"vite.config.mjs",
		"vite.config.cjs",
	} {
		if info, err := os.Stat(filepath.Join(root, name)); err == nil && !info.IsDir() {
			return false
		}
	}
	return true
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
			if path != root && ignoredTestDiscoveryDir(name) {
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
	case "node_modules", ".git", ".tspack", "dist", "fixtures", "tspack-artifacts":
		return true
	default:
		return false
	}
}

func vitestAvailable(root string) bool {
	_, err := os.Stat(projectManagedBin(root, "vitest"))
	return err == nil
}

func projectManagedBin(root string, name string) string {
	path := filepath.Join(root, "node_modules", ".bin", name)
	if fileExists(path) {
		return path
	}
	if runtime.GOOS == "windows" {
		path += ".cmd"
	}
	return path
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
	cmd.Env = append(os.Environ(), opts.Environment...)
	var structured bytes.Buffer
	if opts.CaptureStructured {
		if !opts.JSON {
			args = append(args, "--json")
			cmd = exec.CommandContext(ctx, "node", args...)
			cmd.Env = append(os.Environ(), opts.Environment...)
		}
		cmd.Stdout = &structured
	} else {
		cmd.Stdout = os.Stdout
	}
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if opts.CaptureStructured && structured.Len() > 0 {
		var report struct {
			Summary     Summary           `json:"summary"`
			Tests       []TestEvidence    `json:"tests"`
			Diagnostics []diag.Diagnostic `json:"diagnostics"`
		}
		if decodeErr := json.Unmarshal(structured.Bytes(), &report); decodeErr != nil {
			result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_TEST_RESULT_INVALID", Severity: diag.SeverityError, Message: "native xTest returned invalid structured results", Details: []string{decodeErr.Error()}})
			result.ExitCode = 1
		} else {
			result.Summary = report.Summary
			result.Tests = append(result.Tests, report.Tests...)
			result.Diagnostics = append(result.Diagnostics, report.Diagnostics...)
		}
	}
	if err != nil {
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

func runVitestContext(ctx context.Context, opts Options, result *Result) {
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
	args = append(args, opts.VitestFiles...)
	if opts.VitestConfig != "" {
		args = append(args, "--config", opts.VitestConfig)
	}
	if opts.VitestProject != "" {
		args = append(args, "--project", opts.VitestProject)
	}
	if opts.Filter != "" {
		args = append(args, "-t", opts.Filter)
	}
	bin, err := resolvedProjectManagedBin(opts.RootDir, "vitest")
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_TEST_VITEST_FAILED_TO_START", Severity: diag.SeverityError, Message: "failed to resolve the Vitest executable", Details: []string{err.Error()}})
		result.ExitCode = 1
		return
	}
	var reportPath string
	if opts.CaptureStructured {
		reportFile, err := os.CreateTemp("", "tspack-vitest-report-*.json")
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_TEST_RESULT_IO", Severity: diag.SeverityError, Message: err.Error()})
			result.ExitCode = 1
			return
		}
		reportPath = reportFile.Name()
		_ = reportFile.Close()
		defer os.Remove(reportPath)
		args = append(args, "--reporter=json", "--outputFile="+reportPath)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), opts.Environment...)
	cmd.Dir = opts.VitestCwd
	if cmd.Dir == "" {
		cmd.Dir = opts.RootDir
	}
	if opts.Quiet {
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			result.ExitCode = 1
		} else {
			result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_TEST_VITEST_FAILED_TO_START", Severity: diag.SeverityError, Message: "failed to start vitest", Details: []string{fmt.Sprintf("%v", err)}})
			result.ExitCode = 1
			return
		}
	}
	if opts.CaptureStructured {
		parseVitestReport(reportPath, result)
	}
}

func resolvedProjectManagedBin(root string, name string) (string, error) {
	bin := projectManagedBin(root, name)
	if filepath.IsAbs(bin) {
		return bin, nil
	}
	return filepath.Abs(bin)
}

func parseVitestReport(path string, result *Result) {
	contents, err := os.ReadFile(path)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_TEST_RESULT_INVALID", Severity: diag.SeverityError, Message: "Vitest did not produce its structured report", Details: []string{err.Error()}})
		result.ExitCode = 1
		return
	}
	var report struct {
		Start   float64 `json:"startTime"`
		Total   int     `json:"numTotalTests"`
		Passed  int     `json:"numPassedTests"`
		Failed  int     `json:"numFailedTests"`
		Skipped int     `json:"numPendingTests"`
		Results []struct {
			Name       string  `json:"name"`
			End        float64 `json:"endTime"`
			Assertions []struct {
				FullName        string   `json:"fullName"`
				Status          string   `json:"status"`
				Duration        float64  `json:"duration"`
				FailureMessages []string `json:"failureMessages"`
			} `json:"assertionResults"`
		} `json:"testResults"`
	}
	if err := json.Unmarshal(contents, &report); err != nil {
		result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_TEST_RESULT_INVALID", Severity: diag.SeverityError, Message: "Vitest returned an invalid structured report", Details: []string{err.Error()}})
		result.ExitCode = 1
		return
	}
	result.Summary.Passed += report.Passed
	result.Summary.Failed += report.Failed
	result.Summary.Skipped += report.Skipped
	latestEnd := report.Start
	for _, suite := range report.Results {
		if suite.End > latestEnd {
			latestEnd = suite.End
		}
		for _, assertion := range suite.Assertions {
			evidence := TestEvidence{ID: suite.Name + "::" + assertion.FullName, Name: assertion.FullName, Status: assertion.Status, DurationMs: assertion.Duration}
			if len(assertion.FailureMessages) > 0 {
				evidence.Failure = assertion.FailureMessages
			}
			result.Tests = append(result.Tests, evidence)
		}
	}
	if report.Start > 0 && latestEnd >= report.Start {
		result.Summary.DurationMs += latestEnd - report.Start
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
