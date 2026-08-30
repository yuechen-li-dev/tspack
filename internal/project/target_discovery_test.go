package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverTargetsProjectsStableBuildAndTestContracts(t *testing.T) {
	root := t.TempDir()
	irPath := filepath.Join(root, "ir.json")
	ir := map[string]any{
		"format":    1,
		"workspace": map[string]any{"name": "demo"},
		"packages": []any{map[string]any{
			"name": "internal-snapshot", "publicationName": "@demo/snapshot", "root": "packages/snapshot", "version": "1.0.0", "kind": "library",
			"dependencies": []any{map[string]any{"key": "fixture", "kind": "tool", "source": map[string]any{"kind": "path", "path": "fixtures/local"}}},
			"targets": []any{map[string]any{
				"name": "package", "export": ".", "compiler": "rollup", "entry": "src/index.ts", "runtime": "dist/index.js", "types": "dist/index.d.ts",
				"artifacts": []any{
					map[string]any{"name": "index-js", "kind": "javaScript", "role": "runtimeEntry", "path": "dist/index.js"},
					map[string]any{"name": "index-dts", "kind": "typeDeclarations", "role": "typeDeclaration", "path": "dist/index.d.ts"},
				},
			}},
			"testTargets": []any{map[string]any{
				"name": "unit", "harness": "vitest", "sources": []string{"test/b.test.ts", "test/a.test.ts"}, "project": "threads",
				"requirements": []string{"fixture"},
				"fixtures":     []any{map[string]any{"name": "local", "dependency": "fixture", "binding": "local-fixture", "mode": "source"}},
				"dependsOn":    []string{"package"},
				"builtFixtures": []any{map[string]any{
					"name": "runtime", "target": "package", "artifact": "index-js", "binding": "@demo/runtime",
				}},
			}},
			"publish": map[string]any{"include": []string{"dist/**"}},
		}},
	}
	contents, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(irPath, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	result := DiscoverTargets(TargetDiscoveryRequest{Project: Options{RootDir: root, ManifestIRPath: irPath}})
	if hasErrors(result.Diagnostics) {
		t.Fatalf("diagnostics=%v", result.Diagnostics)
	}
	if len(result.Targets) != 2 {
		t.Fatalf("targets=%v", result.Targets)
	}
	build := result.Targets[0]
	if build.Identity != "build:internal-snapshot:package" || build.PublicationName != "@demo/snapshot" || build.Root != "packages/snapshot" || len(build.Artifacts) != 2 {
		t.Fatalf("build=%+v", build)
	}
	testTarget := result.Targets[1]
	if testTarget.Identity != "test:internal-snapshot:unit" || testTarget.Sources[0] != "test/a.test.ts" || testTarget.HarnessProject != "threads" {
		t.Fatalf("test=%+v", testTarget)
	}
	if len(testTarget.Requirements) != 1 || testTarget.Requirements[0].Producer != "path:fixtures/local" {
		t.Fatalf("requirements=%+v", testTarget.Requirements)
	}
	if len(testTarget.Fixtures) != 1 || testTarget.Fixtures[0].RealizedPath != "packages/snapshot/node_modules/local-fixture" {
		t.Fatalf("fixtures=%+v", testTarget.Fixtures)
	}
	if len(testTarget.Prerequisites) != 1 || testTarget.Prerequisites[0] != "internal-snapshot:package" {
		t.Fatalf("prerequisites=%+v", testTarget.Prerequisites)
	}
	if len(testTarget.BuiltFixtures) != 1 || testTarget.BuiltFixtures[0].ArtifactIdentity != "internal-snapshot:package:index-js" {
		t.Fatalf("built fixtures=%+v", testTarget.BuiltFixtures)
	}
}
