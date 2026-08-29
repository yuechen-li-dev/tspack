package testcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveXTestBridgeExplicitPath(t *testing.T) {
	root := t.TempDir()
	bridge := filepath.Join(root, "bridge.js")
	if err := os.WriteFile(bridge, []byte(""), 0o644); err != nil {
		t.Fatalf("write bridge: %v", err)
	}

	resolution := ResolveXTestBridge(bridge)
	if resolution.Path != bridge {
		t.Fatalf("expected explicit bridge path %q, got %#v", bridge, resolution)
	}
}

func TestResolveXTestBridgeMissingIncludesSearchContext(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.js")
	resolution := ResolveXTestBridge(missing)
	if resolution.Path != "" {
		t.Fatalf("expected missing bridge, got %#v", resolution)
	}

	diagnostic := missingBridgeDiagnostic(resolution)
	joined := strings.Join(diagnostic.Details, "\n")
	if !strings.Contains(joined, missing) {
		t.Fatalf("expected missing path in details, got %q", joined)
	}
	if !strings.Contains(joined, "cwd:") || !strings.Contains(joined, "executable:") {
		t.Fatalf("expected cwd and executable details, got %q", joined)
	}
}

func TestRunXTestForwardsUpdateSnapshotsOnlyForRunMode(t *testing.T) {
	root := t.TempDir()
	bridge := filepath.Join(root, "bridge.js")
	recordPath := filepath.Join(root, "args.txt")
	script := "#!/usr/bin/env node\n" +
		"import fs from 'node:fs';\n" +
		"fs.appendFileSync(" + quoteJS(recordPath) + ", process.argv.slice(2).join('\\t') + '\\n');\n" +
		"console.log('Native xTest results');\n" +
		"console.log('');\n" +
		"console.log('Summary:');\n" +
		"console.log('  total: 0');\n" +
		"console.log('  passed: 0');\n" +
		"console.log('  failed: 0');\n" +
		"console.log('  skipped: 0');\n"
	if err := os.WriteFile(bridge, []byte(script), 0o755); err != nil {
		t.Fatalf("write bridge: %v", err)
	}

	runXTest(Options{RootDir: root, XTestBridge: bridge, UpdateSnapshots: true}, &Result{})
	runXTest(Options{RootDir: root, XTestBridge: bridge, UpdateSnapshots: true, List: true}, &Result{})

	recorded, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(recorded)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two invocations, got %q", string(recorded))
	}
	if !strings.Contains(lines[0], "--update-snapshots") {
		t.Fatalf("run invocation did not include --update-snapshots: %q", lines[0])
	}
	if strings.Contains(lines[1], "--update-snapshots") {
		t.Fatalf("list invocation should not include --update-snapshots: %q", lines[1])
	}
}

func TestHasXTestsIgnoresGeneratedTSPackInternals(t *testing.T) {
	root := t.TempDir()
	mustWriteTestFile(t, filepath.Join(root, ".tspack", "store", "copied.xtest.tsx"))
	mustWriteTestFile(t, filepath.Join(root, "tspack-artifacts", "copied.xtest.tsx"))

	if hasXTests(root) {
		t.Fatalf("expected generated TSPack internals to be ignored")
	}

	mustWriteTestFile(t, filepath.Join(root, "tests", "real.xtest.tsx"))
	if !hasXTests(root) {
		t.Fatalf("expected root tests to be discovered")
	}
}

func mustWriteTestFile(t *testing.T, filePath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir test file dir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("export default null\n"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}

func TestVitestUpdateSnapshotsUnsupported(t *testing.T) {
	result := Run(Options{RootDir: t.TempDir(), UseVitest: true, UpdateSnapshots: true})
	if result.ExitCode == 0 {
		t.Fatalf("expected unsupported backend failure")
	}
	if len(result.Diagnostics) == 0 || result.Diagnostics[0].Code != "TSPACK_SNAPSHOT_UNSUPPORTED_BACKEND" {
		t.Fatalf("expected snapshot unsupported diagnostic, got %#v", result.Diagnostics)
	}
}

func quoteJS(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func TestHasXTestsIgnoresNestedFixturesButAllowsExplicitFixtureRoot(t *testing.T) {
	root := t.TempDir()
	fixtureRoot := filepath.Join(root, "fixtures", "controlled")
	mustWriteTestFile(t, filepath.Join(fixtureRoot, "controlled.xtest.tsx"))
	if hasXTests(root) {
		t.Fatal("repository discovery must ignore controlled fixtures")
	}
	if !hasXTests(fixtureRoot) {
		t.Fatal("an explicitly selected fixture root must remain runnable")
	}
}

func TestRunXTestForwardsBatchOnlyForRunMode(t *testing.T) {
	root := t.TempDir()
	bridge := filepath.Join(root, "bridge.js")
	recordPath := filepath.Join(root, "batch-args.txt")
	script := "#!/usr/bin/env node\n" +
		"import fs from 'node:fs';\n" +
		"fs.appendFileSync(" + quoteJS(recordPath) + ", process.argv.slice(2).join('\\t') + '\\n');\n" +
		"console.log('Native xTest results');\n" +
		"console.log('');\n" +
		"console.log('Summary:');\n" +
		"console.log('  total: 0');\n" +
		"console.log('  passed: 0');\n" +
		"console.log('  failed: 0');\n" +
		"console.log('  skipped: 0');\n"
	if err := os.WriteFile(bridge, []byte(script), 0o755); err != nil {
		t.Fatalf("write bridge: %v", err)
	}

	runXTest(Options{RootDir: root, XTestBridge: bridge, Batch: true, Filter: "cx", Compact: true, UpdateSnapshots: true}, &Result{})
	runXTest(Options{RootDir: root, XTestBridge: bridge, Batch: true, List: true}, &Result{})

	recorded, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(recorded)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two invocations, got %q", string(recorded))
	}
	for _, required := range []string{"--batch", "--filter\tcx", "--compact", "--update-snapshots"} {
		if !strings.Contains(lines[0], required) {
			t.Fatalf("run invocation missing %q: %q", required, lines[0])
		}
	}
	if strings.Contains(lines[1], "--batch") {
		t.Fatalf("list invocation should not include --batch: %q", lines[1])
	}
}

func TestVitestBatchUnsupported(t *testing.T) {
	result := Run(Options{RootDir: t.TempDir(), UseVitest: true, Batch: true})
	if result.ExitCode == 0 {
		t.Fatalf("expected unsupported backend failure")
	}
	if len(result.Diagnostics) == 0 || result.Diagnostics[0].Code != "TSPACK_TEST_BATCH_UNSUPPORTED_BACKEND" {
		t.Fatalf("expected batch unsupported diagnostic, got %#v", result.Diagnostics)
	}
}

func TestAutoDetectedVitestRefusesUnconfiguredPnpmWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	mustWriteTestFile(t, filepath.Join(root, "node_modules", ".bin", "vitest"))
	if err := os.WriteFile(filepath.Join(root, "pnpm-workspace.yaml"), []byte("packages:\n  - packages/*\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := Run(Options{RootDir: root})
	if result.ExitCode == 0 {
		t.Fatalf("expected ambiguous workspace failure")
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "TSPACK_TEST_AMBIGUOUS_WORKSPACE_VITEST" {
		t.Fatalf("expected workspace Vitest diagnostic, got %#v", result.Diagnostics)
	}
}

func TestWorkspaceVitestAutodetectionAcceptsRootConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pnpm-workspace.yaml"), []byte("packages:\n  - packages/*\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "vitest.config.ts"), []byte("export default {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ambiguousWorkspaceVitestRoot(root) {
		t.Fatalf("root Vitest config should make workspace intent explicit")
	}
}

func TestParseVitestReportPreservesFailedAssertionEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vitest-report.json")
	contents := []byte(`{"numTotalTests":1,"numPassedTests":0,"numFailedTests":1,"numPendingTests":0,"startTime":100,"testResults":[{"name":"test/failing.test.ts","endTime":125,"assertionResults":[{"fullName":"suite fails clearly","status":"failed","duration":4,"failureMessages":["expected 1 to be 2"]}]}]}`)
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	result := Result{ExitCode: 1}

	parseVitestReport(path, &result)

	if result.Summary.Failed != 1 || result.Summary.DurationMs != 25 {
		t.Fatalf("summary=%+v", result.Summary)
	}
	if len(result.Tests) != 1 || result.Tests[0].Status != "failed" || result.Tests[0].Failure == nil {
		t.Fatalf("tests=%+v", result.Tests)
	}
}

func TestDefaultXTestBridgeCandidatesPreferCurrentDistPath(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "manifest-frontend", "dist", "native-test-cli.js")
	legacy := filepath.Join(root, "manifest-frontend", "dist", "src", "native-test-cli.js")
	candidates := defaultBridgeCandidates(root, "")

	currentIndex := indexOfString(candidates, current)
	legacyIndex := indexOfString(candidates, legacy)
	if currentIndex < 0 || legacyIndex < 0 {
		t.Fatalf("expected current and legacy candidates, got %#v", candidates)
	}
	if currentIndex > legacyIndex {
		t.Fatalf("expected current dist path before legacy dist/src path, got %#v", candidates)
	}
}

func TestDefaultXTestBridgeCandidatesAcceptLegacyDistSrcPath(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "manifest-frontend", "dist", "src", "native-test-cli.js")
	candidates := defaultBridgeCandidates(root, "")
	if indexOfString(candidates, legacy) < 0 {
		t.Fatalf("expected legacy bridge candidate %q in %#v", legacy, candidates)
	}
}

func indexOfString(values []string, target string) int {
	for index, value := range values {
		if value == target {
			return index
		}
	}
	return -1
}
