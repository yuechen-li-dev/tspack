package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func TestRealizeBuiltTestFixtureUsesQualifiedBuildResultAndReusesVerifiedProjection(t *testing.T) {
	root := t.TempDir()
	producerRoot := filepath.Join(root, "packages", "runtime")
	consumerRoot := filepath.Join(root, "tests")
	artifactPath := filepath.Join(producerRoot, "dist", "index.js")
	writeBuiltFixtureFile(t, filepath.Join(producerRoot, "package.json"), `{"name":"@demo/runtime","type":"module","exports":"./dist/index.js"}`)
	writeBuiltFixtureFile(t, artifactPath, "export const value = 42\n")
	hash := testFileHash(t, artifactPath)
	ir := builtFixtureTestManifest()
	consumer := &ir.Packages[1]
	target := &consumer.TestTargets[0]
	buildResults := []BuildTargetResult{{
		Package:   "@demo/runtime",
		Target:    "package",
		Succeeded: true,
		Artifacts: []BuildArtifact{{
			Package:     "@demo/runtime",
			Target:      "package",
			Kind:        "javaScript",
			Path:        artifactPath,
			Identity:    "@demo/runtime:package:runtime",
			ContentHash: hash,
		}},
	}}
	coordinator := NewBuildCoordinator()

	results, diagnostics := realizeBuiltTestFixtures(root, ir, consumer, target, buildResults, coordinator)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	if len(results) != 1 || results[0].Reused {
		t.Fatalf("results=%+v", results)
	}
	realizedArtifact := filepath.Join(consumerRoot, "node_modules", "@demo", "runtime", "dist", "index.js")
	if contents, err := os.ReadFile(realizedArtifact); err != nil || string(contents) != "export const value = 42\n" {
		t.Fatalf("contents=%q err=%v", contents, err)
	}

	results, diagnostics = realizeBuiltTestFixtures(root, ir, consumer, target, buildResults, coordinator)
	if len(diagnostics) != 0 || len(results) != 1 || !results[0].Reused {
		t.Fatalf("results=%+v diagnostics=%v", results, diagnostics)
	}
	sharedConsumer := *target
	sharedConsumer.Name = "unit-shared"
	results, diagnostics = realizeBuiltTestFixtures(root, ir, consumer, &sharedConsumer, buildResults, coordinator)
	if len(diagnostics) != 0 || len(results) != 1 || !results[0].Reused {
		t.Fatalf("shared results=%+v diagnostics=%v", results, diagnostics)
	}
}

func TestRealizeBuiltTestFixtureRejectsStaleArtifactContent(t *testing.T) {
	root := t.TempDir()
	producerRoot := filepath.Join(root, "packages", "runtime")
	artifactPath := filepath.Join(producerRoot, "dist", "index.js")
	writeBuiltFixtureFile(t, filepath.Join(producerRoot, "package.json"), `{"name":"@demo/runtime"}`)
	writeBuiltFixtureFile(t, artifactPath, "stale\n")
	ir := builtFixtureTestManifest()
	results, diagnostics := realizeBuiltTestFixtures(root, ir, &ir.Packages[1], &ir.Packages[1].TestTargets[0], []BuildTargetResult{{
		Package:   "@demo/runtime",
		Target:    "package",
		Succeeded: true,
		Artifacts: []BuildArtifact{{
			Package:     "@demo/runtime",
			Target:      "package",
			Path:        artifactPath,
			Identity:    "@demo/runtime:package:runtime",
			ContentHash: strings.Repeat("0", 64),
		}},
	}}, NewBuildCoordinator())
	if len(results) != 0 || !hasDiagnosticCodeForBuiltFixtureTests(diagnostics, "TSPACK_TEST_BUILT_FIXTURE_VERIFICATION_FAILED") {
		t.Fatalf("results=%+v diagnostics=%v", results, diagnostics)
	}
}

func TestRealizeBuiltTestFixtureRejectsMissingQualifiedArtifact(t *testing.T) {
	root := t.TempDir()
	ir := builtFixtureTestManifest()
	results, diagnostics := realizeBuiltTestFixtures(
		root,
		ir,
		&ir.Packages[1],
		&ir.Packages[1].TestTargets[0],
		[]BuildTargetResult{{
			Package:   "@demo/runtime",
			Target:    "package",
			Succeeded: true,
		}},
		NewBuildCoordinator(),
	)
	if len(results) != 0 || !hasDiagnosticCodeForBuiltFixtureTests(diagnostics, "TSPACK_TEST_BUILT_FIXTURE_ARTIFACT_MISSING") {
		t.Fatalf("results=%+v diagnostics=%v", results, diagnostics)
	}
	if len(diagnostics[0].Details) < 4 || diagnostics[0].Details[0] != "consumer target: tests:test:unit" {
		t.Fatalf("diagnostics=%+v", diagnostics)
	}
}

type failingPrerequisiteExecutor struct{}

func (failingPrerequisiteExecutor) BuildTarget(_ context.Context, request BuildTargetRequest) BuildTargetResult {
	return BuildTargetResult{
		Package: request.Package.Name,
		Target:  request.Target.Name,
		Diagnostics: []diag.Diagnostic{{
			Code:     "TSPACK_COMPILER_BUILD_FAILED",
			Severity: diag.SeverityError,
			Message:  "controlled producer failure",
		}},
	}
}

func TestRunTestBlocksConsumerWhenBuildPrerequisiteFails(t *testing.T) {
	root := t.TempDir()
	irPath := filepath.Join(root, "ir.json")
	ir := builtFixtureTestManifest()
	contents, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(irPath, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	result := RunTest(context.Background(), TestRequest{
		Project:       Options{RootDir: root, ManifestIRPath: irPath},
		Package:       "tests",
		Target:        "unit",
		BuildExecutor: failingPrerequisiteExecutor{},
	})
	if result.ExitCode != 1 || !hasDiagnosticCodeForBuiltFixtureTests(result.Diagnostics, "TSPACK_COMPILER_BUILD_FAILED") {
		t.Fatalf("result=%+v", result)
	}
	if len(result.Tests) != 0 {
		t.Fatalf("consumer unexpectedly ran: %+v", result.Tests)
	}
	if len(result.Diagnostics[0].Details) == 0 || result.Diagnostics[0].Details[0] != "consumer target: tests:test:unit" {
		t.Fatalf("diagnostics=%+v", result.Diagnostics)
	}
}

type sharedCountingExecutor struct {
	mu    sync.Mutex
	calls int
}

func (executor *sharedCountingExecutor) BuildTarget(_ context.Context, request BuildTargetRequest) BuildTargetResult {
	executor.mu.Lock()
	executor.calls++
	executor.mu.Unlock()
	time.Sleep(20 * time.Millisecond)
	return BuildTargetResult{Package: request.Package.Name, Target: request.Target.Name, Succeeded: true}
}

func TestBuildCoordinatorExecutesSharedProducerOnceForConcurrentConsumers(t *testing.T) {
	base := &sharedCountingExecutor{}
	executor := coordinatedBuildExecutor{base: base, coordinator: NewBuildCoordinator()}
	request := BuildTargetRequest{Package: &manifest.Package{Name: "runtime"}, Target: manifest.Target{Name: "package"}}
	var wait sync.WaitGroup
	wait.Add(2)
	for range 2 {
		go func() {
			defer wait.Done()
			result := executor.BuildTarget(context.Background(), request)
			if !result.Succeeded {
				t.Errorf("result=%+v", result)
			}
		}()
	}
	wait.Wait()
	base.mu.Lock()
	defer base.mu.Unlock()
	if base.calls != 1 {
		t.Fatalf("calls=%d", base.calls)
	}
}

func builtFixtureTestManifest() *manifest.ManifestIR {
	return &manifest.ManifestIR{
		Format:    1,
		Workspace: manifest.Workspace{Name: "demo"},
		Packages: []manifest.Package{
			{
				Name:    "@demo/runtime",
				Root:    "packages/runtime",
				Version: "1.0.0",
				Kind:    "library",
				Publish: manifest.PublishPolicy{Include: []string{"dist/**"}},
				Targets: []manifest.Target{{
					Name:     "package",
					Compiler: "rollup",
					Export:   ".",
					Entry:    "src/index.ts",
					Runtime:  "dist/index.js",
					Types:    "dist/index.d.ts",
					Artifacts: []manifest.TargetArtifact{{
						Name: "runtime", Kind: "javaScript", Path: "dist/*.js",
					}},
				}},
			},
			{
				Name:    "tests",
				Root:    "tests",
				Version: "1.0.0",
				Kind:    "app",
				TestTargets: []manifest.TestTarget{{
					Name:      "unit",
					Harness:   "vitest",
					Sources:   []string{"unit.test.ts"},
					DependsOn: []string{"@demo/runtime:package"},
					BuiltFixtures: []manifest.BuiltArtifactFixture{{
						Name: "runtime", Target: "@demo/runtime:package", Artifact: "runtime", Binding: "@demo/runtime",
					}},
				}},
			},
		},
	}
}

func writeBuiltFixtureFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testFileHash(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(contents)
	return hex.EncodeToString(hash[:])
}

func hasDiagnosticCodeForBuiltFixtureTests(diagnostics []diag.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
