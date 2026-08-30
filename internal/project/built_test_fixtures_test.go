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

	"github.com/yuechen-li-dev/tspack/internal/authoring"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/materialize"
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

func TestCopiedRegularFileModePreservesExecutableSemantics(t *testing.T) {
	if got := copiedRegularFileMode(0o644); got != 0o644 {
		t.Fatalf("non-executable mode=%#o", got)
	}
	if got := copiedRegularFileMode(0o755); got != 0o755 {
		t.Fatalf("executable mode=%#o", got)
	}
	if got := copiedRegularFileMode(0o711); got != 0o755 {
		t.Fatalf("normalized executable mode=%#o", got)
	}
}

func TestRealizeBuiltTestFixtureReplacesOnlyAuthoritativeModuleInstanceProjection(t *testing.T) {
	root := t.TempDir()
	producerRoot := filepath.Join(root, "packages", "runtime")
	consumerRoot := filepath.Join(root, "tests")
	artifactPath := filepath.Join(producerRoot, "dist", "index.js")
	writeBuiltFixtureFile(t, filepath.Join(producerRoot, "package.json"), `{"name":"@demo/runtime","type":"module","exports":"./dist/index.js"}`)
	writeBuiltFixtureFile(t, artifactPath, "export const value = 42\n")
	hash := testFileHash(t, artifactPath)
	instanceID := "npm:@demo/runtime@1.0.0#peers=none"
	digest := sha256.Sum256([]byte(instanceID))
	instancePackage := filepath.Join(root, "node_modules", ".tspack-instances", hex.EncodeToString(digest[:]), "node_modules", "@demo", "runtime")
	writeBuiltFixtureFile(t, filepath.Join(instancePackage, "package.json"), `{"name":"@demo/runtime","version":"1.0.0"}`)
	destination := filepath.Join(consumerRoot, "node_modules", "@demo", "runtime")
	if err := materialize.LinkPackageDirectory(instancePackage, destination); err != nil {
		t.Fatal(err)
	}
	authoritativeLock := &lockfile.Lockfile{
		Packages: []lockfile.Package{{ID: "npm:@demo/runtime@1.0.0", Name: "@demo/runtime"}},
		Instances: []lockfile.ModuleInstance{{
			ID:        instanceID,
			PackageID: "npm:@demo/runtime@1.0.0",
		}},
		RootInstances: []lockfile.RootModuleInstance{{
			From:       "tests:test:unit",
			Reference:  "@demo/runtime",
			Kind:       "tool",
			InstanceID: instanceID,
		}},
	}
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
			Artifacts: []BuildArtifact{{
				Package:     "@demo/runtime",
				Target:      "package",
				Path:        artifactPath,
				Identity:    "@demo/runtime:package:runtime",
				ContentHash: hash,
			}},
		}},
		NewBuildCoordinator(),
		authoritativeLock,
	)
	if len(diagnostics) != 0 || len(results) != 1 {
		t.Fatalf("results=%+v diagnostics=%v", results, diagnostics)
	}
	if !ownedBuiltFixture(destination) {
		t.Fatalf("expected authoritative projection to be replaced by a marked built fixture")
	}
}

func TestRealizeBuiltTestFixtureComposesNamedArtifactSetsAtomically(t *testing.T) {
	root := t.TempDir()
	producerRoot := filepath.Join(root, "packages", "runtime")
	consumerRoot := filepath.Join(root, "tests")
	runtimePath := filepath.Join(producerRoot, "dist", "index.js")
	chunkPath := filepath.Join(producerRoot, "dist", "chunks", "shared.js")
	writeBuiltFixtureFile(t, filepath.Join(producerRoot, "package.json"), `{"name":"@demo/runtime","type":"module","exports":"./dist/index.js"}`)
	writeBuiltFixtureFile(t, runtimePath, "import './chunks/shared.js'\n")
	writeBuiltFixtureFile(t, chunkPath, "export const value = 42\n")
	ir := builtFixtureTestManifest()
	producerTarget := &ir.Packages[0].Targets[0]
	producerTarget.Artifacts = append(producerTarget.Artifacts, manifest.TargetArtifact{
		Name: "runtime-chunks", Kind: "javaScript", Path: "dist/chunks/*.js",
	})
	fixture := &ir.Packages[1].TestTargets[0].BuiltFixtures[0]
	fixture.Artifact = ""
	fixture.Artifacts = []string{"runtime", "runtime-chunks"}
	buildResults := []BuildTargetResult{{
		Package:   "@demo/runtime",
		Target:    "package",
		Succeeded: true,
		Artifacts: []BuildArtifact{
			{
				Package: "@demo/runtime", Target: "package", Kind: "javaScript", Path: runtimePath,
				Identity: "@demo/runtime:package:runtime", ContentHash: testFileHash(t, runtimePath),
			},
			{
				Package: "@demo/runtime", Target: "package", Kind: "javaScript", Path: chunkPath,
				Identity: "@demo/runtime:package:runtime-chunks", ContentHash: testFileHash(t, chunkPath),
			},
		},
	}}

	results, diagnostics := realizeBuiltTestFixtures(root, ir, &ir.Packages[1], &ir.Packages[1].TestTargets[0], buildResults, NewBuildCoordinator())
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	if len(results) != 1 || len(results[0].ArtifactIdentities) != 2 {
		t.Fatalf("results=%+v", results)
	}
	for _, relative := range []string{filepath.Join("dist", "index.js"), filepath.Join("dist", "chunks", "shared.js")} {
		if _, err := os.Stat(filepath.Join(consumerRoot, "node_modules", "@demo", "runtime", relative)); err != nil {
			t.Fatalf("missing composed fixture file %s: %v", relative, err)
		}
	}
}

func TestRealizeBuiltTestFixtureProjectsRegistryRuntimeDependencies(t *testing.T) {
	root := t.TempDir()
	producerRoot := filepath.Join(root, "packages", "runtime")
	artifactPath := filepath.Join(producerRoot, "dist", "index.js")
	writeBuiltFixtureFile(t, filepath.Join(producerRoot, "package.json"), `{"name":"@demo/runtime","type":"module"}`)
	writeBuiltFixtureFile(t, artifactPath, "export { value } from 'helper'\n")
	instancePackage := filepath.Join(root, "node_modules", ".tspack-instances", "helper-instance", "node_modules", "helper")
	writeBuiltFixtureFile(t, filepath.Join(instancePackage, "package.json"), `{"name":"helper","type":"module"}`)
	writeBuiltFixtureFile(t, filepath.Join(instancePackage, "index.js"), "export const value = 42\n")
	producerDependency := filepath.Join(producerRoot, "node_modules", "helper")
	if err := materialize.LinkPackageDirectory(instancePackage, producerDependency); err != nil {
		t.Fatal(err)
	}
	ir := builtFixtureTestManifest()
	ir.Packages[0].Dependencies = []manifest.DependencyIntent{{
		Kind:   "dep",
		Source: authoring.PackageSource{Kind: "npm", Name: "helper", Range: "1.0.0"},
	}}
	ir.Packages[0].Targets[0].Deps = []string{"helper"}
	buildResults := []BuildTargetResult{{
		Package: "@demo/runtime", Target: "package", Succeeded: true,
		Artifacts: []BuildArtifact{{
			Package: "@demo/runtime", Target: "package", Kind: "javaScript", Path: artifactPath,
			Identity: "@demo/runtime:package:runtime", ContentHash: testFileHash(t, artifactPath),
		}},
	}}

	results, diagnostics := realizeBuiltTestFixtures(root, ir, &ir.Packages[1], &ir.Packages[1].TestTargets[0], buildResults, NewBuildCoordinator())
	if len(diagnostics) != 0 || len(results) != 1 {
		t.Fatalf("results=%+v diagnostics=%v", results, diagnostics)
	}
	dependency := filepath.Join(root, "tests", "node_modules", "@demo", "runtime", "node_modules", "helper", "index.js")
	if contents, err := os.ReadFile(dependency); err != nil || string(contents) != "export const value = 42\n" {
		t.Fatalf("runtime dependency contents=%q err=%v", contents, err)
	}
	dependencyInfo, err := os.Stat(filepath.Dir(dependency))
	if err != nil {
		t.Fatal(err)
	}
	instanceInfo, err := os.Stat(instancePackage)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(dependencyInfo, instanceInfo) {
		t.Fatalf("runtime dependency does not project the exact module instance")
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
