package testcmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestScanWatchFilesIncludesRelevantExtensions(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "src/a.ts", "")
	writeTestFile(t, root, "src/b.tsx", "")
	writeTestFile(t, root, "src/c.js", "")
	writeTestFile(t, root, "src/d.jsx", "")
	writeTestFile(t, root, "src/e.css", "")

	state, err := ScanWatchFiles(root)
	if err != nil {
		t.Fatalf("scan watch files: %v", err)
	}

	paths := sortedMapKeys(state)
	expected := []string{"src/a.ts", "src/b.tsx", "src/c.js", "src/d.jsx"}
	if !reflect.DeepEqual(paths, expected) {
		t.Fatalf("expected watched paths %v, got %v", expected, paths)
	}
}

func TestScanWatchFilesIgnoresGeneratedAndVendorDirs(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "src/kept.ts", "")
	ignoredDirs := []string{"node_modules", ".git", ".tspack", "dist", "build", "coverage", "tspack-artifacts", "tmp", "temp"}
	for _, dir := range ignoredDirs {
		writeTestFile(t, root, filepath.Join(dir, "ignored.ts"), "")
	}

	state, err := ScanWatchFiles(root)
	if err != nil {
		t.Fatalf("scan watch files: %v", err)
	}

	paths := sortedMapKeys(state)
	expected := []string{"src/kept.ts"}
	if !reflect.DeepEqual(paths, expected) {
		t.Fatalf("expected watched paths %v, got %v", expected, paths)
	}
}

func TestDiffWatchFilesDetectsAddModifyDeleteDeterministically(t *testing.T) {
	oldTime := time.Unix(10, 0)
	newTime := time.Unix(20, 0)
	previous := map[string]FileStamp{
		"src/deleted.ts":  {ModTime: oldTime, Size: 1},
		"src/modified.ts": {ModTime: oldTime, Size: 1},
		"src/same.ts":     {ModTime: oldTime, Size: 1},
	}
	current := map[string]FileStamp{
		"src/added.ts":    {ModTime: newTime, Size: 1},
		"src/modified.ts": {ModTime: newTime, Size: 1},
		"src/same.ts":     {ModTime: oldTime, Size: 1},
	}

	changed := DiffWatchFiles(previous, current)
	expected := []string{"src/added.ts", "src/deleted.ts", "src/modified.ts"}
	if !reflect.DeepEqual(changed, expected) {
		t.Fatalf("expected changed paths %v, got %v", expected, changed)
	}
}

func TestWatchRejectsListMode(t *testing.T) {
	result := Run(Options{RootDir: t.TempDir(), UseXTest: true, Watch: true, List: true})
	assertDiagnosticCode(t, result, "TSPACK_TEST_WATCH_INVALID_MODE")
}

func TestWatchRejectsJSONMode(t *testing.T) {
	result := Run(Options{RootDir: t.TempDir(), UseXTest: true, Watch: true, JSON: true})
	assertDiagnosticCode(t, result, "TSPACK_TEST_WATCH_INVALID_MODE")
}

func TestWatchRejectsVitestBackend(t *testing.T) {
	result := Run(Options{RootDir: t.TempDir(), UseVitest: true, Watch: true})
	assertDiagnosticCode(t, result, "TSPACK_TEST_WATCH_UNSUPPORTED_BACKEND")
}

func TestWatchRerunsOnFileChangeAndForwardsNativeFlags(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "src/demo.xtest.tsx", "export const value = 1\n")
	bridge := writeWatchBridge(t, root)
	invocationsPath := filepath.Join(root, "invocations.jsonl")
	lockPath := filepath.Join(root, "bridge.lock")
	overlapPath := filepath.Join(root, "overlap.txt")

	t.Setenv("TSPACK_TEST_WATCH_MAX_RUNS", "2")
	t.Setenv("TSPACK_WATCH_TEST_INVOCATIONS", invocationsPath)
	t.Setenv("TSPACK_WATCH_TEST_LOCK", lockPath)
	t.Setenv("TSPACK_WATCH_TEST_OVERLAP", overlapPath)

	resultChannel := make(chan Result, 1)
	go func() {
		resultChannel <- Run(Options{
			RootDir:     root,
			UseXTest:    true,
			Watch:       true,
			Filter:      "cx",
			Compact:     true,
			XTestBridge: bridge,
		})
	}()

	waitForInvocationCount(t, invocationsPath, 1)
	writeTestFile(t, root, "src/demo.xtest.tsx", "export const value = 2\n")

	select {
	case result := <-resultChannel:
		if result.ExitCode != 0 || len(result.Diagnostics) != 0 {
			t.Fatalf("expected successful watch result, got %#v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watch run did not exit after second invocation")
	}

	invocations := readInvocations(t, invocationsPath)
	if len(invocations) != 2 {
		t.Fatalf("expected two bridge invocations, got %d: %#v", len(invocations), invocations)
	}
	for _, args := range invocations {
		joined := strings.Join(args, "\x00")
		for _, required := range []string{"test", "--root", root, "--filter", "cx", "--compact"} {
			if !strings.Contains(joined, required) {
				t.Fatalf("expected invocation %v to contain %q", args, required)
			}
		}
	}
	if _, err := os.Stat(overlapPath); err == nil {
		t.Fatalf("bridge reported overlapping runs in %s", overlapPath)
	}
}

func TestWatchContextCancellationStopsLoop(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "src/demo.xtest.tsx", "export const value = 1\n")
	bridge := writeWatchBridge(t, root)
	invocationsPath := filepath.Join(root, "invocations.jsonl")
	lockPath := filepath.Join(root, "bridge.lock")
	overlapPath := filepath.Join(root, "overlap.txt")

	t.Setenv("TSPACK_WATCH_TEST_INVOCATIONS", invocationsPath)
	t.Setenv("TSPACK_WATCH_TEST_LOCK", lockPath)
	t.Setenv("TSPACK_WATCH_TEST_OVERLAP", overlapPath)

	ctx, cancel := context.WithCancel(context.Background())
	resultChannel := make(chan Result, 1)
	go func() {
		resultChannel <- RunContext(ctx, Options{RootDir: root, UseXTest: true, Watch: true, XTestBridge: bridge})
	}()

	waitForInvocationCount(t, invocationsPath, 1)
	cancel()

	select {
	case result := <-resultChannel:
		if result.ExitCode != 0 || len(result.Diagnostics) != 0 {
			t.Fatalf("expected clean cancellation, got %#v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watch run did not stop after context cancellation")
	}
}

func writeTestFile(t *testing.T, root string, relativePath string, contents string) {
	t.Helper()
	path := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func sortedMapKeys(state map[string]FileStamp) []string {
	paths := make([]string, 0, len(state))
	for path := range state {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func assertDiagnosticCode(t *testing.T, result Result, code string) {
	t.Helper()
	if result.ExitCode == 0 {
		t.Fatalf("expected nonzero exit code for %s", code)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != code {
		t.Fatalf("expected diagnostic %s, got %#v", code, result.Diagnostics)
	}
}

func writeWatchBridge(t *testing.T, root string) string {
	t.Helper()
	bridge := filepath.Join(root, "bridge.js")
	script := `const fs = require('fs');
const invocations = process.env.TSPACK_WATCH_TEST_INVOCATIONS;
const lock = process.env.TSPACK_WATCH_TEST_LOCK;
const overlap = process.env.TSPACK_WATCH_TEST_OVERLAP;
if (fs.existsSync(lock)) {
  fs.writeFileSync(overlap, 'overlap');
}
fs.writeFileSync(lock, String(process.pid));
fs.appendFileSync(invocations, JSON.stringify(process.argv.slice(2)) + '\n');
setTimeout(() => {
  fs.rmSync(lock, { force: true });
  process.exit(0);
}, 150);
`
	if err := os.WriteFile(bridge, []byte(script), 0o644); err != nil {
		t.Fatalf("write bridge: %v", err)
	}
	return bridge
}

func waitForInvocationCount(t *testing.T, path string, expected int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(readInvocationsAllowMissing(t, path)) >= expected {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d invocations, got %d", expected, len(readInvocationsAllowMissing(t, path)))
}

func readInvocations(t *testing.T, path string) [][]string {
	t.Helper()
	invocations := readInvocationsAllowMissing(t, path)
	if invocations == nil {
		t.Fatalf("expected invocation file %s", path)
	}
	return invocations
}

func readInvocationsAllowMissing(t *testing.T, path string) [][]string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read invocations: %v", err)
	}
	trimmed := strings.TrimSpace(string(contents))
	if trimmed == "" {
		return nil
	}
	lines := strings.Split(trimmed, "\n")
	invocations := make([][]string, 0, len(lines))
	for _, line := range lines {
		var args []string
		if err := json.Unmarshal([]byte(line), &args); err != nil {
			t.Fatalf("parse invocation line %q: %v", line, err)
		}
		invocations = append(invocations, args)
	}
	return invocations
}
