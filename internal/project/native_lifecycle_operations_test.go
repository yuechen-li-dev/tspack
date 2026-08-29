package project

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/audit"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/testcmd"
)

type recordingBuildExecutor struct {
	targets []string
}

type blockingBuildExecutor struct {
	started chan struct{}
}

func (executor blockingBuildExecutor) BuildTarget(ctx context.Context, request BuildTargetRequest) BuildTargetResult {
	close(executor.started)
	<-ctx.Done()
	return BuildTargetResult{
		Package: request.Package.Name,
		Target:  request.Target.Name,
		Diagnostics: []diag.Diagnostic{{
			Code:     "TSPACK_BUILD_CANCELLED",
			Severity: diag.SeverityError,
			Message:  ctx.Err().Error(),
		}},
	}
}

func (executor *recordingBuildExecutor) BuildTarget(_ context.Context, request BuildTargetRequest) BuildTargetResult {
	executor.targets = append(executor.targets, request.Package.Name+":"+request.Target.Name)
	artifact := BuildArtifact{Package: request.Package.Name, Target: request.Target.Name, Kind: "javaScript", Path: request.Target.Runtime}
	return BuildTargetResult{Succeeded: true, Artifacts: []BuildArtifact{artifact}}
}

func TestRunBuildSelectsTargetsAndReturnsTypedArtifactsWithoutCLI(t *testing.T) {
	root := t.TempDir()
	irPath := filepath.Join(root, "ir.json")
	ir := map[string]any{
		"format":    1,
		"workspace": map[string]any{"name": "demo"},
		"packages": []any{map[string]any{
			"name": "app", "version": "1.0.0", "kind": "app",
			"targets": []any{map[string]any{"name": "browser", "export": ".", "compiler": "tsc", "entry": "src.ts", "runtime": "dist/app.js"}},
		}},
	}
	contents, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(irPath, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	executor := &recordingBuildExecutor{}
	result := RunBuild(context.Background(), BuildRequest{
		Project:  Options{RootDir: root, ManifestIRPath: irPath},
		Packages: []string{"app"},
		Targets:  []string{"browser"},
		Executor: executor,
	})
	if hasErrors(result.Diagnostics) {
		t.Fatalf("diagnostics=%v", result.Diagnostics)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Path != "dist/app.js" {
		t.Fatalf("artifacts=%v", result.Artifacts)
	}
	if len(executor.targets) != 1 || executor.targets[0] != "app:browser" {
		t.Fatalf("targets=%v", executor.targets)
	}
}

func TestRunBuildPlansQualifiedPrerequisiteOnceBeforeDownstream(t *testing.T) {
	root := t.TempDir()
	irPath := filepath.Join(root, "ir.json")
	ir := map[string]any{
		"format":    1,
		"workspace": map[string]any{"name": "demo"},
		"packages": []any{
			map[string]any{
				"name": "base", "version": "1.0.0", "kind": "library",
				"targets": []any{map[string]any{"name": "package", "export": ".", "compiler": "tsc", "entry": "src.ts", "runtime": "dist/base.js", "types": "dist/base.d.ts"}},
				"publish": map[string]any{"include": []string{"dist/**"}},
			},
			map[string]any{
				"name": "app", "version": "1.0.0", "kind": "app",
				"targets": []any{map[string]any{"name": "package", "export": ".", "compiler": "tsc", "entry": "src.ts", "runtime": "dist/app.js", "types": "dist/app.d.ts", "dependsOn": []string{"base:package"}}},
			},
		},
	}
	contents, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(irPath, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	executor := &recordingBuildExecutor{}
	result := RunBuild(context.Background(), BuildRequest{
		Project:  Options{RootDir: root, ManifestIRPath: irPath},
		Packages: []string{"app"},
		Targets:  []string{"package"},
		Executor: executor,
	})
	if hasErrors(result.Diagnostics) {
		t.Fatalf("diagnostics=%v", result.Diagnostics)
	}
	want := []string{"base:package", "app:package"}
	if !reflect.DeepEqual(executor.targets, want) {
		t.Fatalf("targets=%v want=%v", executor.targets, want)
	}
}

func TestRunBuildRejectsSelectedPackagesWithoutTargets(t *testing.T) {
	root := t.TempDir()
	irPath := filepath.Join(root, "ir.json")
	contents := []byte(`{"format":1,"workspace":{"name":"demo"},"packages":[{"name":"app","version":"1.0.0","kind":"app","targets":[]}]}`)
	if err := os.WriteFile(irPath, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	result := RunBuild(context.Background(), BuildRequest{
		Project:  Options{RootDir: root, ManifestIRPath: irPath},
		Packages: []string{"app"},
		Executor: &recordingBuildExecutor{},
	})
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "TSPACK_BUILD_NO_TARGETS" {
		t.Fatalf("diagnostics=%v", result.Diagnostics)
	}
	if len(result.Diagnostics[0].Details) == 0 || result.Diagnostics[0].Details[0] != "selected package: app (0 build targets)" {
		t.Fatalf("details=%v", result.Diagnostics[0].Details)
	}
}

func TestRunBuildHonorsCancellationBeforeTarget(t *testing.T) {
	root := t.TempDir()
	irPath := filepath.Join(root, "ir.json")
	contents := []byte(`{"format":1,"workspace":{"name":"demo"},"packages":[{"name":"app","version":"1.0.0","kind":"app","targets":[{"name":"browser","export":".","compiler":"tsc","entry":"src.ts","runtime":"dist/app.js"}]}]}`)
	if err := os.WriteFile(irPath, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := RunBuild(ctx, BuildRequest{Project: Options{RootDir: root, ManifestIRPath: irPath}, Executor: &recordingBuildExecutor{}})
	if len(result.Diagnostics) == 0 || result.Diagnostics[len(result.Diagnostics)-1].Code != "TSPACK_BUILD_CANCELLED" {
		t.Fatalf("diagnostics=%v", result.Diagnostics)
	}
}

func TestRunBuildPropagatesCancellationIntoTargetExecutor(t *testing.T) {
	root := t.TempDir()
	irPath := filepath.Join(root, "ir.json")
	contents := []byte(`{"format":1,"workspace":{"name":"demo"},"packages":[{"name":"app","version":"1.0.0","kind":"app","targets":[{"name":"browser","export":".","compiler":"tsc","entry":"src.ts","runtime":"dist/app.js"}]}]}`)
	if err := os.WriteFile(irPath, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	completed := make(chan BuildOperationResult, 1)
	go func() {
		completed <- RunBuild(ctx, BuildRequest{Project: Options{RootDir: root, ManifestIRPath: irPath}, Executor: blockingBuildExecutor{started: started}})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("build target did not start")
	}
	cancel()
	select {
	case result := <-completed:
		if len(result.Diagnostics) == 0 || result.Diagnostics[len(result.Diagnostics)-1].Code != "TSPACK_BUILD_CANCELLED" {
			t.Fatalf("diagnostics=%v", result.Diagnostics)
		}
	case <-time.After(time.Second):
		t.Fatal("build target did not observe cancellation")
	}
}

type emptyAuditClient struct{}

func (emptyAuditClient) QueryBatch(_ context.Context, queries []audit.Query) ([]audit.QueryResult, error) {
	return make([]audit.QueryResult, len(queries)), nil
}

func (emptyAuditClient) Get(_ context.Context, _ string) (audit.Vulnerability, error) {
	return audit.Vulnerability{}, nil
}

func TestRunAuditPreservesCoverageWithoutCLI(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, "ts-lock.toml")
	locked := lockfile.Lockfile{
		Lock: lockfile.LockHeader{Format: lockfile.FormatVersion, Tool: lockfile.ToolName},
		Packages: []lockfile.Package{{
			ID:      "npm:left-pad@1.3.0",
			Source:  "npm",
			Name:    "left-pad",
			Version: "1.3.0",
			Hash:    "test-hash",
		}},
	}
	contents, err := lockfile.Marshal(&locked)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	result := RunAudit(context.Background(), AuditRequest{Project: DefaultOptions(root), Client: emptyAuditClient{}})
	if hasErrors(result.Diagnostics) {
		t.Fatalf("diagnostics=%v", result.Diagnostics)
	}
	if !result.Report.CoverageComplete || len(result.Report.Coverage) != 1 {
		t.Fatalf("report=%+v", result.Report)
	}
}

func TestRunTestIsCallableWithoutCLI(t *testing.T) {
	result := RunTest(context.Background(), TestRequest{Project: DefaultOptions(t.TempDir())})
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "TSPACK_TEST_NO_BACKENDS" {
		t.Fatalf("diagnostics=%v", result.Diagnostics)
	}
}

func TestFailedTestDiagnosticsPreserveTargetIdentityAndFailure(t *testing.T) {
	diagnostics := failedTestDiagnostics("@scope/tests", "fixture-family", []testcmd.TestEvidence{{
		ID:      "test/fixture.test.ts::reports context",
		Status:  "failed",
		Failure: []string{"AssertionError: expected fixture value"},
	}})
	if len(diagnostics) != 1 || diagnostics[0].Code != "TSPACK_TEST_ASSERTION_FAILED" {
		t.Fatalf("diagnostics=%#v", diagnostics)
	}
	details := strings.Join(diagnostics[0].Details, "\n")
	for _, expected := range []string{"@scope/tests:test:fixture-family", "test/fixture.test.ts::reports context", "AssertionError: expected fixture value"} {
		if !strings.Contains(details, expected) {
			t.Fatalf("details=%q, want %q", details, expected)
		}
	}
}
