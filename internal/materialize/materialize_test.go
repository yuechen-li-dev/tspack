package materialize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/store"
)

func TestMaterializeStrictLayout(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	left := putPkg(t, s, "npm:left-pad@1.2.0", "left-pad")
	depA := putPkg(t, s, "npm:dep-a@1.0.0", "dep-a")
	typescript := putPkg(t, s, "npm:typescript@5.0.0", "typescript")
	scope := putPkg(t, s, "npm:@scope/pkg@1.0.0", "@scope/pkg")
	lf := &lockfile.Lockfile{Packages: []lockfile.Package{
		{ID: "npm:left-pad@1.2.0", Name: "left-pad", Source: "npm", Hash: left},
		{ID: "npm:dep-a@1.0.0", Name: "dep-a", Source: "npm", Hash: depA},
		{ID: "npm:typescript@5.0.0", Name: "typescript", Source: "npm", Hash: typescript},
		{ID: "npm:@scope/pkg@1.0.0", Name: "@scope/pkg", Source: "npm", Hash: scope},
	}, Edges: []lockfile.Edge{
		{From: "app:target:core", To: "npm:dep-a@1.0.0", Kind: "runtime"},
		{From: "npm:dep-a@1.0.0", To: "npm:left-pad@1.2.0", Kind: "runtime"},
		{From: "app:tool", To: "npm:typescript@5.0.0", Kind: "tool"},
		{From: "app:target:core", To: "npm:@scope/pkg@1.0.0", Kind: "runtime"},
	}}
	res := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeCopy}})
	if len(res.Diagnostics) > 0 {
		t.Fatalf("diags: %#v", res.Diagnostics)
	}
	mustExist(t, filepath.Join(ws, "node_modules", "dep-a", "package.json"))
	mustExist(t, filepath.Join(ws, "node_modules", "dep-a", "node_modules", "left-pad", "package.json"))
	mustExist(t, filepath.Join(ws, "node_modules", "typescript", "package.json"))
	mustExist(t, filepath.Join(ws, "node_modules", "@scope", "pkg", "package.json"))
	if _, err := os.Stat(filepath.Join(ws, "node_modules", "left-pad", "package.json")); err == nil {
		t.Fatal("phantom root dep should not exist")
	}
}

func TestMaterializeDirectDependencyRootEdge(t *testing.T) {
	workspace := t.TempDir()
	contentStore, _ := store.Open(t.TempDir())
	packageHash := putPkgWithPackageJSON(
		t,
		contentStore,
		"jsr:@luca/flag@1.0.1",
		"@luca/flag",
		`{"name":"@jsr/luca__flag","version":"1.0.1","exports":"./mod.js"}`,
		[]fileSpec{{path: "mod.js", content: "export const flag = true;\n", mode: 0o644}},
	)
	locked := &lockfile.Lockfile{
		Packages: []lockfile.Package{{
			ID:      "jsr:@luca/flag@1.0.1",
			Name:    "@luca/flag",
			Version: "1.0.1",
			Source:  "jsr",
			Hash:    packageHash,
		}},
		Edges: []lockfile.Edge{{
			From: "app:dependency",
			To:   "jsr:@luca/flag@1.0.1",
			Kind: "runtime",
		}},
	}

	result := NodeModulesMaterializer{}.Materialize(context.Background(), Request{
		WorkspaceRoot: workspace,
		Lock:          locked,
		Store:         contentStore,
		Options:       Options{LinkMode: LinkModeCopy},
	})
	if len(result.Diagnostics) > 0 {
		t.Fatalf("direct dependency materialization diagnostics: %#v", result.Diagnostics)
	}
	mustExist(t, filepath.Join(workspace, "node_modules", "@jsr", "luca__flag", "package.json"))
}

func TestMaterializeSourceFixtureIntoConsumerPackage(t *testing.T) {
	workspace := t.TempDir()
	contentStore, _ := store.Open(t.TempDir())
	fixtureSource := filepath.Join(workspace, "test", "unit", "projects", "http-client")
	if err := os.MkdirAll(fixtureSource, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureSource, "package.json"), []byte(`{"name":"http-client","source":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fixtureID := "path:test/unit/projects/http-client#fixture"
	fixtureHash := putPkg(t, contentStore, fixtureID, "http-client")
	ir := &manifest.ManifestIR{Format: 1, Workspace: manifest.Workspace{Name: "w"}, Packages: []manifest.Package{{
		Name:    "tests",
		Root:    "test/unit",
		Version: "1.0.0",
		Kind:    "app",
		Dependencies: []manifest.DependencyIntent{{
			Key: "http-client", Kind: "tool", Source: manifest.Source{Kind: "path", Path: "projects/http-client"},
		}},
		TestTargets: []manifest.TestTarget{{
			Name:         "unit",
			Harness:      "vitest",
			Sources:      []string{"test/unit.test.ts"},
			Requirements: []string{"http-client"},
			Fixtures:     []manifest.TestFixture{{Name: "http-client", Dependency: "http-client", Binding: "http-client", Mode: "source"}},
		}},
	}}}
	g, diagnostics := graph.Build(ir)
	if len(diagnostics) > 0 {
		t.Fatalf("graph diagnostics=%v", diagnostics)
	}
	locked := &lockfile.Lockfile{
		Packages: []lockfile.Package{{ID: fixtureID, Name: "http-client", Source: "path", Hash: fixtureHash}},
		Edges:    []lockfile.Edge{{From: "tests:test:unit", To: fixtureID, Kind: "test", Reference: "http-client"}},
	}
	request := Request{WorkspaceRoot: workspace, Graph: g, Lock: locked, Store: contentStore, Options: Options{LinkMode: LinkModeCopy}}
	first := NodeModulesMaterializer{}.Materialize(context.Background(), request)
	if len(first.Diagnostics) > 0 {
		t.Fatalf("first diagnostics=%#v", first.Diagnostics)
	}
	destination := filepath.Join(workspace, "test", "unit", "node_modules", "http-client", "package.json")
	mustExist(t, destination)
	contents, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), `"source":true`) {
		t.Fatalf("fixture did not expose authoritative source: %s", contents)
	}
	firstInfo, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	second := NodeModulesMaterializer{}.Materialize(context.Background(), request)
	if len(second.Diagnostics) > 0 {
		t.Fatalf("second diagnostics=%#v", second.Diagnostics)
	}
	secondInfo, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !firstInfo.ModTime().Equal(secondInfo.ModTime()) {
		t.Fatalf("unchanged fixture was rematerialized: first=%v second=%v", firstInfo.ModTime(), secondInfo.ModTime())
	}
	if err := os.WriteFile(filepath.Join(fixtureSource, "updated.txt"), []byte("updated"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustExist(t, filepath.Join(workspace, "test", "unit", "node_modules", "http-client", "updated.txt"))
}

func TestCleanMaterializeSourceFixtureReplacesDependencyBinding(t *testing.T) {
	workspace := t.TempDir()
	contentStore, _ := store.Open(t.TempDir())
	fixtureSource := filepath.Join(workspace, "test", "unit", "projects", "http-client")
	if err := os.MkdirAll(fixtureSource, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureSource, "package.json"), []byte(`{"name":"http-client","source":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fixtureID := "path:test/unit/projects/http-client#fixture"
	fixtureHash := putPkg(t, contentStore, fixtureID, "http-client")
	ir := &manifest.ManifestIR{Format: 1, Workspace: manifest.Workspace{Name: "w"}, Packages: []manifest.Package{{
		Name: "tests", Root: "test/unit", Version: "1.0.0", Kind: "app",
		Dependencies: []manifest.DependencyIntent{{Key: "http-client", Kind: "tool", Source: manifest.Source{Kind: "path", Path: "projects/http-client"}}},
		TestTargets: []manifest.TestTarget{{
			Name: "unit", Harness: "vitest", Sources: []string{"test/unit.test.ts"}, Requirements: []string{"http-client"},
			Fixtures: []manifest.TestFixture{{Name: "http-client", Dependency: "http-client", Binding: "http-client", Mode: "source"}},
		}},
	}}}
	graphValue, diagnostics := graph.Build(ir)
	if len(diagnostics) > 0 {
		t.Fatalf("graph diagnostics=%v", diagnostics)
	}
	locked := &lockfile.Lockfile{
		Packages: []lockfile.Package{{ID: fixtureID, Name: "http-client", Source: "path", Hash: fixtureHash}},
		Edges:    []lockfile.Edge{{From: "tests:test:unit", To: fixtureID, Kind: "test", Reference: "http-client"}},
	}
	destination := filepath.Join(workspace, "test", "unit", "node_modules", "http-client")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "stale.txt"), []byte("dependency binding"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := Request{
		WorkspaceRoot: workspace,
		Graph:         graphValue,
		Lock:          locked,
		Store:         contentStore,
		Options:       Options{Clean: true, LinkMode: LinkModeCopy},
	}
	result := materializeTestFixtures(request, filepath.Join(workspace, "node_modules"), LinkModeCopy)
	if len(result.Diagnostics) > 0 {
		t.Fatalf("diagnostics=%#v", result.Diagnostics)
	}
	mustExist(t, filepath.Join(destination, "package.json"))
	if _, err := os.Stat(filepath.Join(destination, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("clean fixture projection retained the displaced dependency binding: %v", err)
	}
}

func TestMaterializePackageFixtureUsesLockedStoreContent(t *testing.T) {
	workspace := t.TempDir()
	contentStore, _ := store.Open(t.TempDir())
	fixtureID := "path:test/unit/deps/dep-cjs#fixture"
	fixtureHash := putPkgWithPackageJSON(
		t,
		contentStore,
		fixtureID,
		"@vitest/test-dep-cjs",
		`{"name":"@vitest/test-dep-cjs","locked":true}`,
		nil,
	)
	ir := &manifest.ManifestIR{Format: 1, Workspace: manifest.Workspace{Name: "w"}, Packages: []manifest.Package{{
		Name:    "tests",
		Root:    "test/unit",
		Version: "1.0.0",
		Kind:    "app",
		Dependencies: []manifest.DependencyIntent{{
			Key: "dep-cjs", Kind: "tool", Source: manifest.Source{Kind: "path", Path: "deps/dep-cjs"},
		}},
		TestTargets: []manifest.TestTarget{{
			Name:         "unit",
			Harness:      "vitest",
			Sources:      []string{"test/unit.test.ts"},
			Requirements: []string{"dep-cjs"},
			Fixtures:     []manifest.TestFixture{{Name: "dep-cjs", Dependency: "dep-cjs", Binding: "@vitest/test-dep-cjs", Mode: "package"}},
		}},
	}}}
	g, diagnostics := graph.Build(ir)
	if len(diagnostics) > 0 {
		t.Fatalf("graph diagnostics=%v", diagnostics)
	}
	locked := &lockfile.Lockfile{
		Packages: []lockfile.Package{{ID: fixtureID, Name: "@vitest/test-dep-cjs", Source: "path", Hash: fixtureHash}},
		Edges:    []lockfile.Edge{{From: "tests:test:unit", To: fixtureID, Kind: "test", Reference: "dep-cjs"}},
	}
	result := NodeModulesMaterializer{}.Materialize(context.Background(), Request{
		WorkspaceRoot: workspace,
		Graph:         g,
		Lock:          locked,
		Store:         contentStore,
		Options:       Options{LinkMode: LinkModeCopy},
	})
	if len(result.Diagnostics) > 0 {
		t.Fatalf("diagnostics=%#v", result.Diagnostics)
	}
	destination := filepath.Join(workspace, "test", "unit", "node_modules", "@vitest", "test-dep-cjs", "package.json")
	contents, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), `"locked":true`) {
		t.Fatalf("package fixture did not use locked store content: %s", contents)
	}
}

func TestMaterializeSourceFixtureRejectsSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	consumerRoot := filepath.Join(workspace, "test", "unit")
	if err := os.MkdirAll(filepath.Join(consumerRoot, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	fixtureSource := filepath.Join(consumerRoot, "projects", "escaped")
	if err := createWorkspaceDirectoryLink(outside, fixtureSource); err != nil {
		t.Skipf("directory links are unavailable: %v", err)
	}
	if existingPathContainedByWorkspace(workspace, fixtureSource) {
		t.Fatalf("escaped fixture source was incorrectly contained in the workspace")
	}
	contentStore, _ := store.Open(t.TempDir())
	fixtureID := "path:test/unit/projects/escaped#fixture"
	fixtureHash := putPkg(t, contentStore, fixtureID, "escaped")
	ir := &manifest.ManifestIR{Format: 1, Workspace: manifest.Workspace{Name: "w"}, Packages: []manifest.Package{{
		Name: "tests", Root: "test/unit", Version: "1.0.0", Kind: "app",
		Dependencies: []manifest.DependencyIntent{{Key: "escaped", Kind: "tool", Source: manifest.Source{Kind: "path", Path: "projects/escaped"}}},
		TestTargets:  []manifest.TestTarget{{Name: "unit", Harness: "vitest", Sources: []string{"test/unit.test.ts"}, Requirements: []string{"escaped"}, Fixtures: []manifest.TestFixture{{Name: "escaped", Dependency: "escaped", Binding: "escaped", Mode: "source"}}}},
	}}}
	g, diagnostics := graph.Build(ir)
	if len(diagnostics) > 0 {
		t.Fatalf("graph diagnostics=%v", diagnostics)
	}
	locked := &lockfile.Lockfile{Packages: []lockfile.Package{{ID: fixtureID, Name: "escaped", Source: "path", Hash: fixtureHash}}, Edges: []lockfile.Edge{{From: "tests:test:unit", To: fixtureID, Kind: "test", Reference: "escaped"}}}
	result := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: workspace, Graph: g, Lock: locked, Store: contentStore, Options: Options{LinkMode: LinkModeCopy}})
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "TSPACK_TEST_FIXTURE_PATH_ESCAPES_WORKSPACE" {
		t.Fatalf("diagnostics=%#v", result.Diagnostics)
	}
}

func TestMaterializeNPMAliasAtReferenceName(t *testing.T) {
	workspace := t.TempDir()
	contentStore, _ := store.Open(t.TempDir())
	parentHash := putPkg(t, contentStore, "npm:parent@1.0.0", "parent")
	barHash := putPkg(t, contentStore, "npm:bar@2.1.0", "bar")
	locked := &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:parent@1.0.0", Name: "parent", Source: "npm", Hash: parentHash},
			{ID: "npm:bar@2.1.0", Name: "bar", Source: "npm", Hash: barHash},
		},
		Edges: []lockfile.Edge{
			{From: "app:target:core", To: "npm:parent@1.0.0", Kind: "runtime"},
			{From: "npm:parent@1.0.0", To: "npm:bar@2.1.0", Kind: "runtime", Reference: "foo"},
		},
	}
	result := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: workspace, Lock: locked, Store: contentStore, Options: Options{LinkMode: LinkModeCopy}})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("alias materialization diagnostics: %#v", result.Diagnostics)
	}
	mustExist(t, filepath.Join(workspace, "node_modules", "parent", "node_modules", "foo", "package.json"))
	if _, err := os.Stat(filepath.Join(workspace, "node_modules", "parent", "node_modules", "bar")); !os.IsNotExist(err) {
		t.Fatalf("semantic target should materialize under alias reference, stat error = %v", err)
	}
}

func TestMaterializeRootNPMAliasSurvivesPruning(t *testing.T) {
	workspace := t.TempDir()
	contentStore, _ := store.Open(t.TempDir())
	reactIsHash := putPkg(t, contentStore, "npm:react-is@18.3.1", "react-is")
	locked := &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:react-is@18.3.1", Name: "react-is", Source: "npm", Hash: reactIsHash},
		},
		Edges: []lockfile.Edge{
			{From: "app:target:package", To: "npm:react-is@18.3.1", Kind: "tool", Reference: "react-is-18"},
		},
	}

	result := NodeModulesMaterializer{}.Materialize(context.Background(), Request{
		WorkspaceRoot: workspace,
		Lock:          locked,
		Store:         contentStore,
		Options:       Options{LinkMode: LinkModeCopy},
	})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("root alias materialization diagnostics: %#v", result.Diagnostics)
	}
	mustExist(t, filepath.Join(workspace, "node_modules", "react-is-18", "package.json"))
}

func TestMaterializeMultipleAliasesForOneSemanticTarget(t *testing.T) {
	workspace := t.TempDir()
	contentStore, _ := store.Open(t.TempDir())
	parentHash := putPkg(t, contentStore, "npm:parent@1.0.0", "parent")
	barHash := putPkg(t, contentStore, "npm:bar@2.1.0", "bar")
	locked := &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:parent@1.0.0", Name: "parent", Source: "npm", Hash: parentHash},
			{ID: "npm:bar@2.1.0", Name: "bar", Source: "npm", Hash: barHash},
		},
		Edges: []lockfile.Edge{
			{From: "app:target:core", To: "npm:parent@1.0.0", Kind: "runtime"},
			{From: "npm:parent@1.0.0", To: "npm:bar@2.1.0", Kind: "runtime", Reference: "foo"},
			{From: "npm:parent@1.0.0", To: "npm:bar@2.1.0", Kind: "runtime", Reference: "baz"},
		},
	}
	result := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: workspace, Lock: locked, Store: contentStore, Options: Options{LinkMode: LinkModeCopy}})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("multiple alias diagnostics: %#v", result.Diagnostics)
	}
	mustExist(t, filepath.Join(workspace, "node_modules", "parent", "node_modules", "foo", "package.json"))
	mustExist(t, filepath.Join(workspace, "node_modules", "parent", "node_modules", "baz", "package.json"))
}

func TestMaterializeRejectsAliasReferenceCollisionBeforeMutation(t *testing.T) {
	workspace := t.TempDir()
	contentStore, _ := store.Open(t.TempDir())
	firstHash := putPkg(t, contentStore, "npm:first@1.0.0", "first")
	secondHash := putPkg(t, contentStore, "npm:second@1.0.0", "second")
	locked := &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:first@1.0.0", Name: "first", Source: "npm", Hash: firstHash},
			{ID: "npm:second@1.0.0", Name: "second", Source: "npm", Hash: secondHash},
		},
		Edges: []lockfile.Edge{
			{From: "app:target:core", To: "npm:first@1.0.0", Kind: "runtime", Reference: "shared"},
			{From: "app:target:core", To: "npm:second@1.0.0", Kind: "runtime", Reference: "shared"},
		},
	}
	result := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: workspace, Lock: locked, Store: contentStore, Options: Options{LinkMode: LinkModeCopy}})
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "TSPACK_ALIAS_REFERENCE_COLLISION" {
		t.Fatalf("alias collision diagnostics: %#v", result.Diagnostics)
	}
	if _, err := os.Stat(filepath.Join(workspace, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("alias collision mutated filesystem, stat error = %v", err)
	}
}

func TestMaterializeRegistryNameCollisionUsesDistinctNodeNames(t *testing.T) {
	workspace := t.TempDir()
	contentStore, _ := store.Open(t.TempDir())
	npmHash := putPkgWithPackageJSON(
		t,
		contentStore,
		"npm:@std/path@1.0.0",
		"@std/path",
		`{"name":"@std/path","version":"1.0.0","exports":"./index.js"}`,
		[]fileSpec{{path: "index.js", content: "export const source = 'npm';\n", mode: 0o644}},
	)
	jsrHash := putPkgWithPackageJSON(
		t,
		contentStore,
		"jsr:@std/path@1.1.6",
		"@std/path",
		`{"name":"@jsr/std__path","version":"1.1.6","exports":"./mod.js"}`,
		[]fileSpec{{path: "mod.js", content: "export const separator = '/';\n", mode: 0o644}},
	)
	lf := &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:@std/path@1.0.0", Name: "@std/path", Version: "1.0.0", Source: "npm", Hash: npmHash},
			{ID: "jsr:@std/path@1.1.6", Name: "@std/path", Version: "1.1.6", Source: "jsr", Hash: jsrHash},
		},
		Edges: []lockfile.Edge{
			{From: "app:target:core", To: "npm:@std/path@1.0.0", Kind: "runtime"},
			{From: "app:target:core", To: "jsr:@std/path@1.1.6", Kind: "runtime"},
		},
	}

	result := NodeModulesMaterializer{}.Materialize(context.Background(), Request{
		WorkspaceRoot: workspace,
		Lock:          lf,
		Store:         contentStore,
		Options:       Options{LinkMode: LinkModeCopy},
	})
	if len(result.Diagnostics) > 0 {
		t.Fatalf("JSR materialization diagnostics: %#v", result.Diagnostics)
	}
	mustExist(t, filepath.Join(workspace, "node_modules", "@jsr", "std__path", "package.json"))
	mustExist(t, filepath.Join(workspace, "node_modules", "@std", "path", "package.json"))
}

func TestMaterializeRejectsSourceQualifiedPackagesWithSameDestination(t *testing.T) {
	workspace := t.TempDir()
	contentStore, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	npmHash := putPkgWithPackageJSON(
		t,
		contentStore,
		"npm:@jsr/scope__pkg@1.0.0",
		"@jsr/scope__pkg",
		`{"name":"@jsr/scope__pkg","version":"1.0.0"}`,
		nil,
	)
	jsrHash := putPkgWithPackageJSON(
		t,
		contentStore,
		"jsr:@scope/pkg@1.0.0",
		"@scope/pkg",
		`{"name":"@jsr/scope__pkg","version":"1.0.0"}`,
		nil,
	)
	locked := &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:@jsr/scope__pkg@1.0.0", Name: "@jsr/scope__pkg", Source: "npm", Hash: npmHash},
			{ID: "jsr:@scope/pkg@1.0.0", Name: "@scope/pkg", Source: "jsr", Hash: jsrHash},
		},
		Edges: []lockfile.Edge{
			{From: "app:target:core", To: "npm:@jsr/scope__pkg@1.0.0", Kind: "runtime"},
			{From: "app:target:core", To: "jsr:@scope/pkg@1.0.0", Kind: "runtime"},
		},
	}

	result := NodeModulesMaterializer{}.Materialize(context.Background(), Request{
		WorkspaceRoot: workspace,
		Lock:          locked,
		Store:         contentStore,
		Options:       Options{LinkMode: LinkModeCopy},
	})

	if len(result.Diagnostics) != 1 {
		t.Fatalf("collision diagnostics = %#v", result.Diagnostics)
	}
	diagnostic := result.Diagnostics[0]
	if diagnostic.Code != "TSPACK_MATERIALIZE_IMPORT_COLLISION" {
		t.Fatalf("collision diagnostic = %#v", diagnostic)
	}
	wantDetails := []string{
		"destination: " + filepath.ToSlash(filepath.Join(workspace, "node_modules", "@jsr", "scope__pkg")),
		"source-qualified packages:",
		"  jsr:@scope/pkg@1.0.0",
		"  npm:@jsr/scope__pkg@1.0.0",
		"Compatibility/materialization names must be unique within each node_modules directory.",
	}
	if !reflect.DeepEqual(diagnostic.Details, wantDetails) {
		t.Fatalf("collision details = %#v, want %#v", diagnostic.Details, wantDetails)
	}
	reversedLock := &lockfile.Lockfile{
		Packages: []lockfile.Package{locked.Packages[1], locked.Packages[0]},
		Edges:    []lockfile.Edge{locked.Edges[1], locked.Edges[0]},
	}
	reversedResult := NodeModulesMaterializer{}.Materialize(context.Background(), Request{
		WorkspaceRoot: workspace,
		Lock:          reversedLock,
		Store:         contentStore,
		Options:       Options{LinkMode: LinkModeCopy},
	})
	if !reflect.DeepEqual(result.Diagnostics, reversedResult.Diagnostics) {
		t.Fatalf("collision diagnostics depend on lock order:\nfirst: %#v\nreversed: %#v", result.Diagnostics, reversedResult.Diagnostics)
	}
	if _, err := os.Stat(filepath.Join(workspace, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("collision should be detected before filesystem mutation, stat error = %v", err)
	}
}

func TestRootBinMaterializationAndStrictness(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	tool := putPkgWithPackageJSON(t, s, "npm:tool@1.0.0", "tool", `{"name":"tool","bin":{"tool":"bin/tool.js"}}`, []fileSpec{{path: "bin/tool.js", content: "#!/usr/bin/env node\nconsole.log('tool')\n", mode: 0o755}})
	transitive := putPkgWithPackageJSON(t, s, "npm:transitive@1.0.0", "transitive", `{"name":"transitive","bin":{"hidden":"bin/hidden.js"}}`, []fileSpec{{path: "bin/hidden.js", content: "#!/usr/bin/env node\n", mode: 0o755}})
	lf := &lockfile.Lockfile{Packages: []lockfile.Package{
		{ID: "npm:tool@1.0.0", Name: "tool", Hash: tool},
		{ID: "npm:transitive@1.0.0", Name: "transitive", Hash: transitive},
	}, Edges: []lockfile.Edge{{From: "app:tool", To: "npm:tool@1.0.0", Kind: "tool"}, {From: "npm:tool@1.0.0", To: "npm:transitive@1.0.0", Kind: "runtime"}}}
	res := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeCopy}})
	if len(res.Diagnostics) > 0 {
		t.Fatalf("diags: %#v", res.Diagnostics)
	}
	rootBin := filepath.Join(ws, "node_modules", ".bin", "tool")
	if runtime.GOOS == "windows" {
		rootBin += ".cmd"
	}
	mustExist(t, rootBin)
	if _, err := os.Stat(filepath.Join(ws, "node_modules", ".bin", "hidden")); err == nil {
		t.Fatal("transitive-only bin should not be root-exposed")
	}
	if runtime.GOOS == "windows" {
		content, err := os.ReadFile(rootBin)
		if err != nil {
			t.Fatal(err)
		}
		text := string(content)
		for _, want := range []string{"@ECHO off", `"%~dp0\..\tool\bin\tool.js"`, "%*"} {
			if !strings.Contains(text, want) {
				t.Fatalf("windows shim missing %q:\n%s", want, text)
			}
		}
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(filepath.Join(ws, "node_modules", "tool", "bin", "tool.js"))
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm()&0o111 == 0 {
			t.Fatalf("expected executable mode, got %o", st.Mode().Perm())
		}
	}
}

func TestCollectRootEdgesIncludesNativeTestToolRequirements(t *testing.T) {
	edges := []lockfile.Edge{
		{From: "app:test:unit", To: "npm:vitest@4.0.0", Kind: "tool"},
		{From: "npm:parent@1.0.0", To: "npm:transitive@1.0.0", Kind: "runtime"},
	}
	rootEdges := collectRootEdges(edges)
	if len(rootEdges) != 1 || rootEdges[0].From != "app:test:unit" {
		t.Fatalf("root edges=%#v", rootEdges)
	}
}

func TestCollectRootEdgesPreservesAliasesAndSkipsProviderPeers(t *testing.T) {
	edges := []lockfile.Edge{
		{From: "pkg:target:package", To: "npm:react-is@18.3.1", Kind: "tool", Reference: "react-is-18"},
		{From: "pkg:target:package", To: "npm:vite@8.0.0", Kind: "peer", Reference: "vite"},
	}

	rootEdges := collectRootEdges(edges)
	if len(rootEdges) != 1 {
		t.Fatalf("root edges = %#v", rootEdges)
	}
	if rootEdges[0].Reference != "react-is-18" {
		t.Fatalf("alias reference = %q", rootEdges[0].Reference)
	}
}

func TestBiomeStyleBinMaterializationAndStrictness(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	biomePackageJSON := `{"name":"@biomejs/biome","bin":{"biome":"bin/biome"}}`
	hiddenPackageJSON := `{"name":"@example/hidden-biome","bin":{"biome":"bin/biome"}}`
	biome := putPkgWithPackageJSON(
		t,
		s,
		"npm:@biomejs/biome@1.9.4",
		"@biomejs/biome",
		biomePackageJSON,
		[]fileSpec{{path: "bin/biome", content: "#!/bin/sh\necho biome\n", mode: 0o755}},
	)
	hidden := putPkgWithPackageJSON(
		t,
		s,
		"npm:@example/hidden-biome@1.0.0",
		"@example/hidden-biome",
		hiddenPackageJSON,
		[]fileSpec{{path: "bin/biome", content: "#!/bin/sh\necho hidden\n", mode: 0o755}},
	)
	lf := &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:@biomejs/biome@1.9.4", Name: "@biomejs/biome", Hash: biome},
			{ID: "npm:@example/hidden-biome@1.0.0", Name: "@example/hidden-biome", Hash: hidden},
		},
		Edges: []lockfile.Edge{
			{From: "app:tool", To: "npm:@biomejs/biome@1.9.4", Kind: "tool"},
			{From: "npm:@biomejs/biome@1.9.4", To: "npm:@example/hidden-biome@1.0.0", Kind: "runtime"},
		},
	}

	res := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeCopy}})
	if len(res.Diagnostics) > 0 {
		t.Fatalf("diags: %#v", res.Diagnostics)
	}

	directBin := filepath.Join(ws, "node_modules", "@biomejs", "biome", "bin", "biome")
	mustExist(t, directBin)
	if runtime.GOOS != "windows" {
		st, err := os.Stat(directBin)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm()&0o111 == 0 {
			t.Fatalf("expected Biome direct bin to be executable, got %o", st.Mode().Perm())
		}
	}
	rootBin := filepath.Join(ws, "node_modules", ".bin", "biome")
	if runtime.GOOS == "windows" {
		rootBin += ".cmd"
	}
	mustExist(t, rootBin)
	if runtime.GOOS == "windows" {
		content, err := os.ReadFile(rootBin)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), `..\@biomejs\biome\bin\biome`) {
			t.Fatalf("expected windows shim to point at direct bin, got:\n%s", string(content))
		}
	} else if target, err := filepath.EvalSymlinks(rootBin); err == nil && target != directBin {
		t.Fatalf("expected root Biome bin to resolve to direct package bin %q, got %q", directBin, target)
	}
	transitiveBin := filepath.Join(ws, "node_modules", "@biomejs", "biome", "node_modules", "@example", "hidden-biome", "bin", "biome")
	mustExist(t, transitiveBin)
	if target, err := filepath.EvalSymlinks(rootBin); err == nil && target == transitiveBin {
		t.Fatal("root Biome bin should not resolve to transitive Biome-like bin")
	}
}

func TestMaterializeStatsObserverTracksHardlinksAndCopyFallbacks(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	pkgHash := putPkg(t, s, "npm:left-pad@1.2.0", "left-pad")
	lf := &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:left-pad@1.2.0", Name: "left-pad", Source: "npm", Hash: pkgHash},
		},
		Edges: []lockfile.Edge{
			{From: "app:target:core", To: "npm:left-pad@1.2.0", Kind: "runtime"},
		},
	}

	var observer statsCapture
	originalLink := materializeLink
	materializeLink = func(oldname string, newname string) error {
		return errors.New("hardlinks disabled for stats test")
	}
	t.Cleanup(func() {
		materializeLink = originalLink
	})

	res := NodeModulesMaterializer{}.Materialize(context.Background(), Request{
		WorkspaceRoot: ws,
		Lock:          lf,
		Store:         s,
		Options: Options{
			LinkMode: LinkModeHardlink,
			Stats:    &observer,
		},
	})
	if len(res.Diagnostics) > 0 {
		t.Fatalf("diags: %#v", res.Diagnostics)
	}
	if observer.packages == 0 || observer.files == 0 || observer.directories == 0 {
		t.Fatalf("expected package/file/directory counts, got %+v", observer)
	}
	if observer.copyCount == 0 {
		t.Fatalf("expected copy fallback count, got %+v", observer)
	}
	if observer.hardlinkCount != 0 {
		t.Fatalf("expected no successful hardlinks when link fails, got %+v", observer)
	}
	if observer.logicalBytes == 0 || observer.copiedBytes == 0 {
		t.Fatalf("expected byte accounting, got %+v", observer)
	}
}

type statsCapture struct {
	mu            sync.Mutex
	packages      int
	files         int
	directories   int
	hardlinkCount int
	copyCount     int
	logicalBytes  int64
	copiedBytes   int64
	noop          bool
	noopPackages  int
	noopFiles     int
}

func (s *statsCapture) RecordMaterializedPackage(pkg lockfile.Package) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.packages++
}

func (s *statsCapture) RecordMaterializedDirectory(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.directories++
}

func (s *statsCapture) RecordMaterializedFile(path string, size int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files++
	s.logicalBytes += size
}

func (s *statsCapture) RecordHardlink(path string, size int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hardlinkCount++
}

func (s *statsCapture) RecordCopy(path string, size int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.copyCount++
	s.copiedBytes += size
}

func (s *statsCapture) RecordMaterializationMarkerHit()      {}
func (s *statsCapture) RecordMaterializationMarkerMiss()     {}
func (s *statsCapture) RecordMaterializationMarkerMismatch() {}
func (s *statsCapture) RecordMaterializationMarkerCorrupt()  {}
func (s *statsCapture) RecordForcedMaterialization()         {}
func (s *statsCapture) RecordMaterializationMarkerWrite()    {}

func (s *statsCapture) RecordMaterializationNoop(packages int, files int, directories int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.noop = true
	s.noopPackages = packages
	s.noopFiles = files
}

func TestBinConflictDiagnostic(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	a := putPkgWithPackageJSON(t, s, "npm:a@1.0.0", "a", `{"name":"a","bin":{"same":"bin/a.js"}}`, []fileSpec{{path: "bin/a.js", content: "", mode: 0o755}})
	b := putPkgWithPackageJSON(t, s, "npm:b@1.0.0", "b", `{"name":"b","bin":{"same":"bin/b.js"}}`, []fileSpec{{path: "bin/b.js", content: "", mode: 0o755}})
	lf := &lockfile.Lockfile{Packages: []lockfile.Package{{ID: "npm:a@1.0.0", Name: "a", Hash: a}, {ID: "npm:b@1.0.0", Name: "b", Hash: b}}, Edges: []lockfile.Edge{{From: "app:tool", To: "npm:a@1.0.0", Kind: "tool"}, {From: "app:tool", To: "npm:b@1.0.0", Kind: "tool"}}}
	res := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeCopy}})
	assertCode(t, res, "TSPACK_MATERIALIZE_BIN_CONFLICT")
}

func TestDiagnosticsAndCleanSafety(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	okHash := putPkg(t, s, "npm:ok@1.0.0", "ok")
	lf := &lockfile.Lockfile{Packages: []lockfile.Package{
		{ID: "npm:ok@1.0.0", Name: "ok", Source: "npm", Hash: okHash},
		{ID: "npm:nohash@1", Name: "nohash", Source: "npm"},
		{ID: "npm:bad@1", Name: "../evil", Source: "npm", Hash: okHash},
		{ID: "npm:missing@1", Name: "missing", Source: "npm", Hash: "sha256:deadbeef"},
	}, Edges: []lockfile.Edge{
		{From: "app:target:core", To: "npm:ok@1.0.0", Kind: "runtime"},
		{From: "app:target:core", To: "npm:nohash@1", Kind: "runtime"},
		{From: "app:target:core", To: "npm:bad@1", Kind: "runtime"},
		{From: "app:target:core", To: "npm:missing@1", Kind: "runtime"},
		{From: "app:target:core", To: "npm:unknown@1", Kind: "runtime"},
	}}
	res := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeCopy}})
	assertCode(t, res, "TSPACK_MATERIALIZE_PACKAGE_HASH_MISSING")
	assertCode(t, res, "TSPACK_MATERIALIZE_INVALID_PACKAGE_NAME")
	assertCode(t, res, "TSPACK_MATERIALIZE_MISSING_STORE_ARTIFACT")
	assertCode(t, res, "TSPACK_MATERIALIZE_EDGE_UNKNOWN_PACKAGE")

	nm := filepath.Join(ws, "node_modules")
	_ = os.Remove(filepath.Join(nm, markerFile))
	cleanRes := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: &lockfile.Lockfile{}, Store: s, Options: Options{Clean: true, LinkMode: LinkModeCopy}})
	assertCode(t, cleanRes, "TSPACK_MATERIALIZE_CLEAN_REFUSED")
}

func TestDeterministicAndLinkMode(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	h := putPkg(t, s, "workspace:@scope/pkg", "@scope/pkg")
	lf := &lockfile.Lockfile{Packages: []lockfile.Package{{ID: "workspace:@scope/pkg", Name: "@scope/pkg", Source: "workspace", Hash: h}}, Edges: []lockfile.Edge{{From: "app:target:core", To: "workspace:@scope/pkg", Kind: "runtime"}}}
	m := NodeModulesMaterializer{}
	first := m.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeAuto}})
	second := m.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{Clean: true, LinkMode: LinkModeCopy}})
	if len(first.Diagnostics)+len(second.Diagnostics) > 0 {
		t.Fatalf("diags: %#v %#v", first.Diagnostics, second.Diagnostics)
	}
	if len(first.Written) != len(second.Written) || first.Written[0].Path != second.Written[0].Path {
		t.Fatal("non-deterministic written paths")
	}
	bad := m.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeSymlink}})
	assertCode(t, bad, "TSPACK_MATERIALIZE_UNSUPPORTED_LINK_MODE")
}

func TestMaterializeWorkspacePackageLinksLiveOutputs(t *testing.T) {
	workspace := t.TempDir()
	packageRoot := filepath.Join(workspace, "packages", "utils")
	if err := os.MkdirAll(packageRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "package.json"), []byte(`{"name":"@scope/utils"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	contentStore, _ := store.Open(t.TempDir())
	packageHash := putPkg(t, contentStore, "workspace:@scope/utils", "@scope/utils")
	locked := &lockfile.Lockfile{
		Packages: []lockfile.Package{{
			ID:     "workspace:@scope/utils",
			Name:   "@scope/utils",
			Source: "workspace",
			Path:   "packages/utils",
			Hash:   packageHash,
		}},
		Edges: []lockfile.Edge{{
			From: "app:target:package",
			To:   "workspace:@scope/utils",
			Kind: "runtime",
		}},
	}
	result := NodeModulesMaterializer{}.Materialize(context.Background(), Request{
		WorkspaceRoot: workspace,
		Lock:          locked,
		Store:         contentStore,
		Options:       Options{LinkMode: LinkModeCopy},
	})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("workspace link diagnostics: %#v", result.Diagnostics)
	}
	liveOutput := filepath.Join(packageRoot, "dist", "index.js")
	if err := os.MkdirAll(filepath.Dir(liveOutput), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(liveOutput, []byte("export {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	projectionDiagnostics := ProjectWorkspaceBuildArtifacts(
		workspace,
		packageRoot,
		"@scope/utils",
		[]string{"dist/*.js"},
		[]string{liveOutput},
	)
	if len(projectionDiagnostics) != 0 {
		t.Fatalf("linked workspace projection diagnostics: %#v", projectionDiagnostics)
	}
	materializedOutput := filepath.Join(workspace, "node_modules", "@scope", "utils", "dist", "index.js")
	assertFileContent(t, materializedOutput, "export {}\n")
}

func TestMaterializeFileHardlinksWhenAvailable(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()
	src := filepath.Join(srcDir, "tool.js")
	dest := filepath.Join(destDir, "tool.js")
	if err := os.WriteFile(src, []byte("#!/usr/bin/env node\nconsole.log('hi')\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := materializeFile(src, dest, info, LinkModeHardlink, nil); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, dest, "#!/usr/bin/env node\nconsole.log('hi')\n")
	srcInfo, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	destInfo, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(srcInfo, destInfo) {
		t.Skip("hardlinks unavailable in this environment; content fallback succeeded")
	}
	if runtime.GOOS != "windows" && destInfo.Mode().Perm()&0o111 == 0 {
		t.Fatalf("expected executable mode on hardlinked file, got %o", destInfo.Mode().Perm())
	}
}

func TestMaterializeFileFallsBackToCopyWhenHardlinkFails(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()
	src := filepath.Join(srcDir, "package.json")
	dest := filepath.Join(destDir, "package.json")
	if err := os.WriteFile(src, []byte("{\"name\":\"pkg\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	originalLink := materializeLink
	materializeLink = func(oldname string, newname string) error {
		return errors.New("hardlinks disabled for test")
	}
	t.Cleanup(func() {
		materializeLink = originalLink
	})
	if err := materializeFile(src, dest, info, LinkModeHardlink, nil); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, dest, "{\"name\":\"pkg\"}\n")
	srcInfo, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	destInfo, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(srcInfo, destInfo) {
		t.Fatal("fallback copy should not share file identity with source")
	}
	if runtime.GOOS != "windows" && destInfo.Mode().Perm() != 0o644 {
		t.Fatalf("expected copied mode 0644, got %o", destInfo.Mode().Perm())
	}
}

func TestMaterializePackageFilesHardlinkFromStoreWhenAvailable(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	h := putPkgWithPackageJSON(
		t,
		s,
		"npm:tool@1.0.0",
		"tool",
		`{"name":"tool","bin":{"tool":"bin/tool.js"}}`,
		[]fileSpec{{path: "bin/tool.js", content: "#!/usr/bin/env node\nconsole.log('tool')\n", mode: 0o755}},
	)
	lf := &lockfile.Lockfile{
		Packages: []lockfile.Package{{ID: "npm:tool@1.0.0", Name: "tool", Hash: h}},
		Edges:    []lockfile.Edge{{From: "app:tool", To: "npm:tool@1.0.0", Kind: "tool"}},
	}
	res := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s})
	if len(res.Diagnostics) > 0 {
		t.Fatalf("diags: %#v", res.Diagnostics)
	}
	ref, diags := s.Get(h)
	if len(diags) > 0 {
		t.Fatalf("store get diags: %#v", diags)
	}
	storeFile := filepath.Join(ref.ExtractedPath, "package.json")
	materializedFile := filepath.Join(ws, "node_modules", "tool", "package.json")
	mustExist(t, materializedFile)
	assertFileContent(t, materializedFile, "{\"name\":\"tool\",\"bin\":{\"tool\":\"bin/tool.js\"}}")
	storeInfo, err := os.Stat(storeFile)
	if err != nil {
		t.Fatal(err)
	}
	materializedInfo, err := os.Stat(materializedFile)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(storeInfo, materializedInfo) {
		t.Skip("hardlinks unavailable in this environment; package materialization fell back to copy")
	}
}

func TestMaterializeCanReplaceExistingTreeWithoutClean(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	h := putPkg(t, s, "npm:pkg@1.0.0", "pkg")
	lf := &lockfile.Lockfile{
		Packages: []lockfile.Package{{ID: "npm:pkg@1.0.0", Name: "pkg", Hash: h}},
		Edges:    []lockfile.Edge{{From: "app:target:core", To: "npm:pkg@1.0.0", Kind: "runtime"}},
	}
	m := NodeModulesMaterializer{}
	first := m.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s})
	if len(first.Diagnostics) > 0 {
		t.Fatalf("first diags: %#v", first.Diagnostics)
	}
	second := m.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s})
	if len(second.Diagnostics) > 0 {
		t.Fatalf("second diags: %#v", second.Diagnostics)
	}
	mustExist(t, filepath.Join(ws, "node_modules", "pkg", "package.json"))
}

func TestMaterializeWritesMarkerWithCurrentPlanDigest(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	hash := putPkg(t, s, "npm:pkg@1.0.0", "pkg")
	lf := &lockfile.Lockfile{
		Packages: []lockfile.Package{{ID: "npm:pkg@1.0.0", Name: "pkg", Hash: hash}},
		Edges:    []lockfile.Edge{{From: "app:target:core", To: "npm:pkg@1.0.0", Kind: "runtime"}},
	}

	res := NodeModulesMaterializer{}.Materialize(context.Background(), Request{
		WorkspaceRoot: ws,
		Lock:          lf,
		Store:         s,
		Options:       Options{LinkMode: LinkModeCopy},
	})
	if len(res.Diagnostics) > 0 {
		t.Fatalf("diags: %#v", res.Diagnostics)
	}

	marker := readMarkerFile(t, ws)
	plan := buildMaterializationPlan(lf, filepath.Join(ws, "node_modules"), LinkModeCopy)
	if marker.PlanDigest != plan.digest {
		t.Fatalf("marker digest=%q want %q", marker.PlanDigest, plan.digest)
	}
	if marker.PackageCount != 1 || marker.FileCount == 0 {
		t.Fatalf("unexpected marker counts: %+v", marker)
	}
}

func TestMaterializeSecondRunBecomesNoop(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	hash := putPkg(t, s, "npm:pkg@1.0.0", "pkg")
	lf := &lockfile.Lockfile{
		Packages: []lockfile.Package{{ID: "npm:pkg@1.0.0", Name: "pkg", Hash: hash}},
		Edges:    []lockfile.Edge{{From: "app:target:core", To: "npm:pkg@1.0.0", Kind: "runtime"}},
	}

	first := NodeModulesMaterializer{}.Materialize(context.Background(), Request{
		WorkspaceRoot: ws,
		Lock:          lf,
		Store:         s,
		Options:       Options{LinkMode: LinkModeCopy},
	})
	if len(first.Diagnostics) > 0 {
		t.Fatalf("first diags: %#v", first.Diagnostics)
	}

	observer := &statsCapture{}
	second := NodeModulesMaterializer{}.Materialize(context.Background(), Request{
		WorkspaceRoot: ws,
		Lock:          lf,
		Store:         s,
		Options:       Options{LinkMode: LinkModeCopy, Stats: observer},
	})
	if len(second.Diagnostics) > 0 {
		t.Fatalf("second diags: %#v", second.Diagnostics)
	}
	if !observer.noop {
		t.Fatalf("expected noop marker fast path, got %+v", observer)
	}
	if observer.files != 0 || observer.copyCount != 0 || observer.hardlinkCount != 0 {
		t.Fatalf("expected no file relinking on noop path, got %+v", observer)
	}
}

func TestMaterializeMissingOrCorruptMarkerFallsBackToFullMaterialization(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(t *testing.T, ws string)
	}{
		{
			name: "missing",
			mutate: func(t *testing.T, ws string) {
				if err := os.Remove(materializationMarkerForRoot(ws)); err != nil {
					t.Fatalf("remove marker: %v", err)
				}
			},
		},
		{
			name: "corrupt",
			mutate: func(t *testing.T, ws string) {
				if err := os.WriteFile(materializationMarkerForRoot(ws), []byte("{not-json"), 0o644); err != nil {
					t.Fatalf("write corrupt marker: %v", err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws := t.TempDir()
			s, _ := store.Open(t.TempDir())
			hash := putPkg(t, s, "npm:pkg@1.0.0", "pkg")
			lf := &lockfile.Lockfile{
				Packages: []lockfile.Package{{ID: "npm:pkg@1.0.0", Name: "pkg", Hash: hash}},
				Edges:    []lockfile.Edge{{From: "app:target:core", To: "npm:pkg@1.0.0", Kind: "runtime"}},
			}

			first := NodeModulesMaterializer{}.Materialize(context.Background(), Request{
				WorkspaceRoot: ws,
				Lock:          lf,
				Store:         s,
				Options:       Options{LinkMode: LinkModeCopy},
			})
			if len(first.Diagnostics) > 0 {
				t.Fatalf("first diags: %#v", first.Diagnostics)
			}

			tc.mutate(t, ws)

			observer := &statsCapture{}
			second := NodeModulesMaterializer{}.Materialize(context.Background(), Request{
				WorkspaceRoot: ws,
				Lock:          lf,
				Store:         s,
				Options:       Options{LinkMode: LinkModeCopy, Stats: observer},
			})
			if len(second.Diagnostics) > 0 {
				t.Fatalf("second diags: %#v", second.Diagnostics)
			}
			if observer.noop {
				t.Fatalf("expected full rematerialization when marker is %s", tc.name)
			}
			if observer.files == 0 {
				t.Fatalf("expected package files to be materialized when marker is %s", tc.name)
			}
		})
	}
}

func TestMaterializeForceBypassesMatchingMarker(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	hash := putPkg(t, s, "npm:pkg@1.0.0", "pkg")
	lf := &lockfile.Lockfile{
		Packages: []lockfile.Package{{ID: "npm:pkg@1.0.0", Name: "pkg", Hash: hash}},
		Edges:    []lockfile.Edge{{From: "app:target:core", To: "npm:pkg@1.0.0", Kind: "runtime"}},
	}

	first := NodeModulesMaterializer{}.Materialize(context.Background(), Request{
		WorkspaceRoot: ws,
		Lock:          lf,
		Store:         s,
		Options:       Options{LinkMode: LinkModeCopy},
	})
	if len(first.Diagnostics) > 0 {
		t.Fatalf("first diags: %#v", first.Diagnostics)
	}

	observer := &statsCapture{}
	second := NodeModulesMaterializer{}.Materialize(context.Background(), Request{
		WorkspaceRoot: ws,
		Lock:          lf,
		Store:         s,
		Options:       Options{LinkMode: LinkModeCopy, Force: true, Stats: observer},
	})
	if len(second.Diagnostics) > 0 {
		t.Fatalf("second diags: %#v", second.Diagnostics)
	}
	if observer.noop {
		t.Fatalf("force should bypass noop marker path")
	}
	if observer.files == 0 {
		t.Fatalf("force should rematerialize package files, got %+v", observer)
	}
}

func TestMaterializePlanDigestDeterministicAndModeSensitive(t *testing.T) {
	lfA := &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:b@1.0.0", Name: "b", Hash: "hash-b"},
			{ID: "npm:a@1.0.0", Name: "a", Hash: "hash-a"},
		},
		Edges: []lockfile.Edge{
			{From: "npm:a@1.0.0", To: "npm:b@1.0.0", Kind: "runtime"},
			{From: "app:target:core", To: "npm:a@1.0.0", Kind: "runtime"},
		},
	}
	lfB := &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:a@1.0.0", Name: "a", Hash: "hash-a"},
			{ID: "npm:b@1.0.0", Name: "b", Hash: "hash-b"},
		},
		Edges: []lockfile.Edge{
			{From: "app:target:core", To: "npm:a@1.0.0", Kind: "runtime"},
			{From: "npm:a@1.0.0", To: "npm:b@1.0.0", Kind: "runtime"},
		},
	}

	planA := buildMaterializationPlan(lfA, filepath.Join("workspace", "node_modules"), LinkModeCopy)
	planB := buildMaterializationPlan(lfB, filepath.Join("workspace", "node_modules"), LinkModeCopy)
	if planA.digest != planB.digest {
		t.Fatalf("digest should be order-independent: %q != %q", planA.digest, planB.digest)
	}

	lfChanged := &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:a@1.0.0", Name: "a", Hash: "hash-a"},
			{ID: "npm:b@1.0.0", Name: "b", Hash: "hash-b-changed"},
		},
		Edges: lfB.Edges,
	}
	planChanged := buildMaterializationPlan(lfChanged, filepath.Join("workspace", "node_modules"), LinkModeCopy)
	if planA.digest == planChanged.digest {
		t.Fatalf("digest should change when package hash changes")
	}

	planHardlink := buildMaterializationPlan(lfB, filepath.Join("workspace", "node_modules"), LinkModeHardlink)
	if planA.digest == planHardlink.digest {
		t.Fatalf("digest should change when materialization mode changes")
	}
}

func TestMaterializeRemovesStaleRootPackageBeforeWritingMarker(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	hashA := putPkg(t, s, "npm:a@1.0.0", "a")
	hashB := putPkg(t, s, "npm:b@1.0.0", "b")

	firstLock := &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:a@1.0.0", Name: "a", Hash: hashA},
			{ID: "npm:b@1.0.0", Name: "b", Hash: hashB},
		},
		Edges: []lockfile.Edge{
			{From: "app:target:core", To: "npm:a@1.0.0", Kind: "runtime"},
			{From: "app:target:core", To: "npm:b@1.0.0", Kind: "runtime"},
		},
	}
	secondLock := &lockfile.Lockfile{
		Packages: []lockfile.Package{{ID: "npm:a@1.0.0", Name: "a", Hash: hashA}},
		Edges:    []lockfile.Edge{{From: "app:target:core", To: "npm:a@1.0.0", Kind: "runtime"}},
	}

	first := NodeModulesMaterializer{}.Materialize(context.Background(), Request{
		WorkspaceRoot: ws,
		Lock:          firstLock,
		Store:         s,
		Options:       Options{LinkMode: LinkModeCopy},
	})
	if len(first.Diagnostics) > 0 {
		t.Fatalf("first diags: %#v", first.Diagnostics)
	}

	second := NodeModulesMaterializer{}.Materialize(context.Background(), Request{
		WorkspaceRoot: ws,
		Lock:          secondLock,
		Store:         s,
		Options:       Options{LinkMode: LinkModeCopy},
	})
	if len(second.Diagnostics) > 0 {
		t.Fatalf("second diags: %#v", second.Diagnostics)
	}

	if _, err := os.Stat(filepath.Join(ws, "node_modules", "b")); !os.IsNotExist(err) {
		t.Fatalf("stale root package should be pruned, got err=%v", err)
	}
	marker := readMarkerFile(t, ws)
	plan := buildMaterializationPlan(secondLock, filepath.Join(ws, "node_modules"), LinkModeCopy)
	if marker.PlanDigest != plan.digest {
		t.Fatalf("marker digest=%q want %q after root prune", marker.PlanDigest, plan.digest)
	}
}

func TestMaterializeCircularDependenciesStayBounded(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	aHash := putPkg(t, s, "npm:a@1.0.0", "a")
	bHash := putPkg(t, s, "npm:b@1.0.0", "b")
	lf := &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:a@1.0.0", Name: "a", Hash: aHash},
			{ID: "npm:b@1.0.0", Name: "b", Hash: bHash},
		},
		Edges: []lockfile.Edge{
			{From: "app:target:core", To: "npm:a@1.0.0", Kind: "runtime"},
			{From: "npm:a@1.0.0", To: "npm:b@1.0.0", Kind: "runtime"},
			{From: "npm:b@1.0.0", To: "npm:a@1.0.0", Kind: "runtime"},
		},
	}

	res := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeCopy}})
	if len(res.Diagnostics) > 0 {
		t.Fatalf("diags: %#v", res.Diagnostics)
	}

	nm := filepath.Join(ws, "node_modules")
	mustExist(t, filepath.Join(nm, "a", "package.json"))
	mustExist(t, filepath.Join(nm, "a", "node_modules", "b", "package.json"))
	mustExist(t, filepath.Join(nm, "a", "node_modules", "b", "node_modules", "a", "package.json"))
	assertNoRepeatedPackagePattern(t, nm, []string{"a", "b", "a", "b"})
	assertMaxNodeModulesDepth(t, nm, 3)
}

func TestMaterializeSharedDependencyDeterministicAndBounded(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	aHash := putPkg(t, s, "npm:a@1.0.0", "a")
	bHash := putPkg(t, s, "npm:b@1.0.0", "b")
	cHash := putPkg(t, s, "npm:c@1.0.0", "c")
	lf := &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:a@1.0.0", Name: "a", Hash: aHash},
			{ID: "npm:b@1.0.0", Name: "b", Hash: bHash},
			{ID: "npm:c@1.0.0", Name: "c", Hash: cHash},
		},
		Edges: []lockfile.Edge{
			{From: "app:target:core", To: "npm:b@1.0.0", Kind: "runtime"},
			{From: "app:target:core", To: "npm:a@1.0.0", Kind: "runtime"},
			{From: "npm:a@1.0.0", To: "npm:c@1.0.0", Kind: "runtime"},
			{From: "npm:b@1.0.0", To: "npm:c@1.0.0", Kind: "runtime"},
		},
	}

	m := NodeModulesMaterializer{}
	first := m.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeCopy}})
	if len(first.Diagnostics) > 0 {
		t.Fatalf("first diags: %#v", first.Diagnostics)
	}
	firstTree := collectMaterializedPaths(t, filepath.Join(ws, "node_modules"))

	second := m.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{Clean: true, LinkMode: LinkModeCopy}})
	if len(second.Diagnostics) > 0 {
		t.Fatalf("second diags: %#v", second.Diagnostics)
	}
	secondTree := collectMaterializedPaths(t, filepath.Join(ws, "node_modules"))

	if len(firstTree) != len(secondTree) {
		t.Fatalf("tree path count changed: first=%d second=%d", len(firstTree), len(secondTree))
	}
	for i := range firstTree {
		if firstTree[i] != secondTree[i] {
			t.Fatalf("tree path changed at %d: %q != %q", i, firstTree[i], secondTree[i])
		}
	}

	nm := filepath.Join(ws, "node_modules")
	mustExist(t, filepath.Join(nm, "a", "node_modules", "c", "package.json"))
	mustExist(t, filepath.Join(nm, "b", "node_modules", "c", "package.json"))
	assertMaxNodeModulesDepth(t, nm, 2)
}

func TestMaterializePathDepthGuardReportsClearDiagnostic(t *testing.T) {
	ws := t.TempDir()
	s, _ := store.Open(t.TempDir())
	packages := make([]lockfile.Package, 0, maxMaterializeDependencyDepth+2)
	edges := []lockfile.Edge{{From: "app:target:core", To: "npm:p00@1.0.0", Kind: "runtime"}}
	for i := 0; i < maxMaterializeDependencyDepth+2; i++ {
		name := fmt.Sprintf("p%02d", i)
		id := fmt.Sprintf("npm:%s@1.0.0", name)
		hash := putPkg(t, s, id, name)
		packages = append(packages, lockfile.Package{ID: id, Name: name, Hash: hash})
		if i > 0 {
			from := fmt.Sprintf("npm:p%02d@1.0.0", i-1)
			edges = append(edges, lockfile.Edge{From: from, To: id, Kind: "runtime"})
		}
	}
	lf := &lockfile.Lockfile{Packages: packages, Edges: edges}

	res := NodeModulesMaterializer{}.Materialize(context.Background(), Request{WorkspaceRoot: ws, Lock: lf, Store: s, Options: Options{LinkMode: LinkModeCopy}})
	assertCode(t, res, "TSPACK_MATERIALIZE_PATH_DEPTH_EXCEEDED")
}

func putPkg(t *testing.T, s *store.Store, id, name string) string {
	t.Helper()
	d := t.TempDir()
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "package.json"), []byte("{\"name\":\""+name+"\"}"), 0o644); err != nil {
		t.Fatal(err)
	}
	ref, diags := s.PutArtifact(store.Artifact{ID: id, Name: name, Kind: store.ArtifactPathTree, RootDir: d})
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	return ref.Hash
}

func mustExist(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
}

func collectMaterializedPaths(t *testing.T, root string) []string {
	t.Helper()
	paths := []string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func assertNoRepeatedPackagePattern(t *testing.T, root string, pattern []string) {
	t.Helper()
	paths := collectMaterializedPaths(t, root)
	needle := strings.Join(pattern, "/node_modules/")
	for _, path := range paths {
		if strings.Contains(path, needle) {
			t.Fatalf("found repeated package pattern %q in %q", needle, path)
		}
	}
}

func assertMaxNodeModulesDepth(t *testing.T, root string, maxDepth int) {
	t.Helper()
	paths := collectMaterializedPaths(t, root)
	for _, path := range paths {
		depth := strings.Count(path, "node_modules") + 1
		if path == "." {
			depth = 0
		}
		if depth > maxDepth {
			t.Fatalf("materialized path depth %d exceeds max %d: %s", depth, maxDepth, path)
		}
		if len(path) >= 240 {
			t.Fatalf("materialized relative path should stay comfortably below Windows MAX_PATH: len=%d path=%s", len(path), path)
		}
	}
}

func assertCode(t *testing.T, res Result, code string) {
	t.Helper()
	for _, d := range res.Diagnostics {
		if d.Code == code {
			return
		}
	}
	t.Fatalf("missing diagnostic %s: %#v", code, res.Diagnostics)
}

type fileSpec struct {
	path    string
	content string
	mode    os.FileMode
}

func putPkgWithPackageJSON(t *testing.T, s *store.Store, id, name, packageJSON string, files []fileSpec) string {
	t.Helper()
	d := t.TempDir()
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "package.json"), []byte(packageJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		full := filepath.Join(d, file.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(file.content), file.mode); err != nil {
			t.Fatal(err)
		}
	}
	ref, diags := s.PutArtifact(store.Artifact{ID: id, Name: name, Kind: store.ArtifactPathTree, RootDir: d})
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	return ref.Hash
}

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("unexpected file content for %s:\nwant: %q\ngot:  %q", path, want, string(got))
	}
}

func readMarkerFile(t *testing.T, ws string) materializationMarker {
	t.Helper()
	body, err := os.ReadFile(materializationMarkerForRoot(ws))
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	var marker materializationMarker
	if err := json.Unmarshal(body, &marker); err != nil {
		t.Fatalf("parse marker: %v\n%s", err, string(body))
	}
	return marker
}
