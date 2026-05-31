package check

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/tspack/tspack/internal/boundary"
	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/graph"
	"github.com/tspack/tspack/internal/manifest"
)

func loadGraph(t *testing.T, fixture string) *graph.WorkspaceGraph {
	b, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	ir, diags := manifest.LoadBytes(fixture, b)
	if len(diags) > 0 {
		t.Fatalf("manifest diags: %#v", diags)
	}
	g, diags := graph.Build(ir)
	if len(diags) > 0 {
		t.Fatalf("graph diags: %#v", diags)
	}
	return g
}

func TestM4Fixtures(t *testing.T) {
	pass := []string{"../../fixtures/valid/m4-basic/manifest.ir.golden.json", "../../fixtures/valid/m4-react-vue-isolated/manifest.ir.golden.json"}
	for _, f := range pass {
		res := CheckRuntimeBoundaries(CheckOptions{RootDir: "../..", Graph: loadGraph(t, f)})
		if len(res.Diagnostics) != 0 {
			t.Fatalf("%s diags=%#v", f, res.Diagnostics)
		}
	}
}

func TestM4Invalids(t *testing.T) {
	cases := map[string]string{
		"../../fixtures/invalid/vue-leaks-core/manifest.ir.golden.json":           "TSPACK_BOUNDARY_OPTIONAL_PEER_LEAK",
		"../../fixtures/invalid/tool-imported-at-runtime/manifest.ir.golden.json": "TSPACK_BOUNDARY_TOOL_RUNTIME_IMPORT",
		"../../fixtures/invalid/undeclared-import/manifest.ir.golden.json":        "TSPACK_BOUNDARY_UNDECLARED_IMPORT",
		"../../fixtures/invalid/explicit-deny/manifest.ir.golden.json":            "TSPACK_BOUNDARY_EXPLICIT_DENY",
	}
	for f, code := range cases {
		res := CheckRuntimeBoundaries(CheckOptions{RootDir: "../..", Graph: loadGraph(t, f)})
		ok := false
		for _, d := range res.Diagnostics {
			if d.Code == code {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("%s expected %s got %#v", f, code, res.Diagnostics)
		}
	}
}

func TestLeakPath(t *testing.T) {
	res := CheckRuntimeBoundaries(CheckOptions{RootDir: "../..", Graph: loadGraph(t, "../../fixtures/invalid/vue-leaks-core/manifest.ir.golden.json")})
	for _, d := range res.Diagnostics {
		if d.Code == "TSPACK_BOUNDARY_OPTIONAL_PEER_LEAK" {
			p := strings.Join(boundary.PathFromDetails(d), "|")
			if !strings.Contains(p, "src/index.ts") || !strings.Contains(p, "src/text/index.ts") || !strings.Contains(p, "src/text/vue/index.ts") || !strings.Contains(p, "vue") {
				t.Fatalf("path %s", p)
			}
			return
		}
	}
	t.Fatal("missing leak diag")
}

func TestDeterministicDiagnostics(t *testing.T) {
	g := loadGraph(t, "../../fixtures/invalid/vue-leaks-core/manifest.ir.golden.json")
	a := CheckRuntimeBoundaries(CheckOptions{RootDir: "../..", Graph: g}).Diagnostics
	b := CheckRuntimeBoundaries(CheckOptions{RootDir: "../..", Graph: g}).Diagnostics
	if !reflect.DeepEqual(a, b) {
		t.Fatal("nondeterministic")
	}
}

func TestCheckPackageIntegrationAndTypeOnlyRuntime(t *testing.T) {
	res := CheckPackage(CheckOptions{RootDir: "../..", Graph: loadGraph(t, "../../fixtures/invalid/m5-type-only-runtime-import/manifest.ir.golden.json")})
	codes := map[string]bool{}
	for _, d := range res.Diagnostics {
		codes[d.Code] = true
	}
	if !codes["TSPACK_BOUNDARY_TYPE_ONLY_RUNTIME_IMPORT"] {
		t.Fatalf("missing type-only runtime diag: %#v", res.Diagnostics)
	}
}

func TestM6BSplitWorkspaceRuntimeAndTypesPass(t *testing.T) {
	graph := loadGraph(t, "../../fixtures/valid/m6b-workspace-split/manifest.ir.golden.json")
	rootDir := "../../fixtures/valid/m6b-workspace-split"

	runtimeRes := CheckRuntimeBoundaries(CheckOptions{RootDir: rootDir, Graph: graph})
	if len(runtimeRes.Diagnostics) != 0 {
		t.Fatalf("runtime diags=%#v", runtimeRes.Diagnostics)
	}

	typeRes := CheckTypeSurfaces(CheckOptions{RootDir: rootDir, Graph: graph})
	if len(typeRes.Diagnostics) != 0 {
		t.Fatalf("type diags=%#v", typeRes.Diagnostics)
	}
}

func TestSinglePackageFixtureStillPasses(t *testing.T) {
	g := loadGraph(t, "../../fixtures/valid/m5-types-basic/manifest.ir.golden.json")
	res := CheckPackage(CheckOptions{RootDir: "../..", Graph: g})
	if len(res.Diagnostics) != 0 {
		t.Fatalf("single-package fixture produced diagnostics: %#v", res.Diagnostics)
	}
}

func TestM33AWorkspaceDependencyAndTsEsmAliasFixturePasses(t *testing.T) {
	fixture := "../../fixtures/valid/m33a-workspace-ts-esm/manifest.ir.golden.json"
	res := CheckRuntimeBoundaries(CheckOptions{RootDir: "../../fixtures/valid/m33a-workspace-ts-esm", Graph: loadGraph(t, fixture)})
	if len(res.Diagnostics) != 0 {
		t.Fatalf("runtime diags=%#v", res.Diagnostics)
	}
}

func TestNodejsRuntimeBaselineCheckEquivalence(t *testing.T) {
	fixtures := []string{
		"runtime-baseline-omitted",
		"runtime-baseline-nodejs",
	}

	var first []diag.Diagnostic
	for _, fixtureName := range fixtures {
		t.Run(fixtureName, func(t *testing.T) {
			fixturePath := "../../fixtures/valid/" + fixtureName + "/manifest.ir.golden.json"
			rootDir := "../../fixtures/valid/" + fixtureName
			res := CheckRuntimeBoundaries(CheckOptions{RootDir: rootDir, Graph: loadGraph(t, fixturePath)})
			if len(res.Diagnostics) != 0 {
				t.Fatalf("runtime baseline check diagnostics=%#v", res.Diagnostics)
			}
			if first == nil {
				first = append([]diag.Diagnostic(nil), res.Diagnostics...)
				return
			}
			if !reflect.DeepEqual(first, res.Diagnostics) {
				t.Fatalf("diagnostics differ: first=%#v current=%#v", first, res.Diagnostics)
			}
		})
	}
}

func TestM33ATsEsmAliasTraceFindsTransitiveViolation(t *testing.T) {
	fixture := "../../fixtures/invalid/m33a-ts-esm-alias-violation/manifest.ir.golden.json"
	res := CheckRuntimeBoundaries(CheckOptions{RootDir: "../../fixtures/invalid/m33a-ts-esm-alias-violation", Graph: loadGraph(t, fixture)})
	for _, d := range res.Diagnostics {
		if d.Code != "TSPACK_BOUNDARY_UNDECLARED_IMPORT" {
			continue
		}

		path := strings.Join(boundary.PathFromDetails(d), "|")
		if !strings.Contains(path, "src/index.ts") {
			t.Fatalf("path is missing index.ts: %s", path)
		}
		if !strings.Contains(path, "src/button.tsx") {
			t.Fatalf("path is missing button.tsx: %s", path)
		}
		if !strings.Contains(path, "forbidden-dep") {
			t.Fatalf("path is missing forbidden-dep: %s", path)
		}
		return
	}
	t.Fatalf("missing undeclared import diagnostic: %#v", res.Diagnostics)
}

func buildM33BBoundaryGraph(t *testing.T, boundaryFrom string) *graph.WorkspaceGraph {
	t.Helper()

	ir := &manifest.ManifestIR{
		Format: 1,
		Workspace: manifest.Workspace{
			Name: "ws",
		},
		Packages: []manifest.Package{
			{
				Name:    "app",
				Version: "1.0.0",
				Kind:    "library",
				Dependencies: []manifest.DependencyIntent{
					{
						Key:  "react-dom",
						Kind: "runtime",
						Source: manifest.Source{
							Kind:    "npm",
							Package: "react-dom",
							Range:   "^19.0.0",
						},
					},
				},
				Targets: []manifest.Target{
					{
						Name:    "core",
						Export:  ".",
						Entry:   "src/index.ts",
						Runtime: "dist/index.js",
						Types:   "dist/index.d.ts",
						Deps:    []string{"react-dom"},
					},
				},
				Boundaries: []manifest.BoundaryRule{
					{
						From:     boundaryFrom,
						DenyDeps: []string{"react-dom"},
					},
				},
			},
		},
	}

	g, diags := graph.Build(ir)
	if len(diags) != 0 {
		t.Fatalf("graph diags=%#v", diags)
	}
	return g
}

func writeM33BTransitiveSourceGraph(t *testing.T, root string) {
	t.Helper()

	if err := os.MkdirAll(root+"/src", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/src/index.ts", []byte(`import "./button.js";
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/src/button.tsx", []byte(`import "react-dom";
`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestM33BExactFileFromDoesNotApplyTransitively(t *testing.T) {
	root := t.TempDir()
	writeM33BTransitiveSourceGraph(t, root)

	res := CheckRuntimeBoundaries(CheckOptions{RootDir: root, Graph: buildM33BBoundaryGraph(t, "src/index.ts")})
	for _, d := range res.Diagnostics {
		if d.Code == "TSPACK_BOUNDARY_EXPLICIT_DENY" {
			t.Fatalf("exact-file from matched transitive import file unexpectedly: %#v", res.Diagnostics)
		}
	}
	if len(res.Diagnostics) != 0 {
		t.Fatalf("expected clean fixture apart from exact-file deny behavior, got %#v", res.Diagnostics)
	}
}

func TestM33BGlobFromAppliesToPhysicalImportingFile(t *testing.T) {
	root := t.TempDir()
	writeM33BTransitiveSourceGraph(t, root)

	res := CheckRuntimeBoundaries(CheckOptions{RootDir: root, Graph: buildM33BBoundaryGraph(t, "src/**")})
	for _, d := range res.Diagnostics {
		if d.Code != "TSPACK_BOUNDARY_EXPLICIT_DENY" {
			continue
		}

		path := strings.Join(boundary.PathFromDetails(d), "|")
		if !strings.Contains(path, "src/index.ts") {
			t.Fatalf("path is missing entry file: %s", path)
		}
		if !strings.Contains(path, "src/button.tsx") {
			t.Fatalf("path is missing physical importing file: %s", path)
		}
		if !strings.Contains(path, "react-dom") {
			t.Fatalf("path is missing denied dependency: %s", path)
		}
		return
	}
	t.Fatalf("missing explicit deny diagnostic: %#v", res.Diagnostics)
}

func TestM33BExactFileFromMatchesDirectImport(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/src", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/src/index.ts", []byte(`import "react-dom";
`), 0o644); err != nil {
		t.Fatal(err)
	}

	res := CheckRuntimeBoundaries(CheckOptions{RootDir: root, Graph: buildM33BBoundaryGraph(t, "src/index.ts")})
	for _, d := range res.Diagnostics {
		if d.Code == "TSPACK_BOUNDARY_EXPLICIT_DENY" {
			return
		}
	}
	t.Fatalf("missing exact-file explicit deny diagnostic: %#v", res.Diagnostics)
}

func TestM33AWorkspaceToolDependencyStillRejectedAtRuntime(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/src", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/src/index.ts", []byte(`import "@prisma-ui/build-tool";`), 0o644); err != nil {
		t.Fatal(err)
	}

	ir := &manifest.ManifestIR{
		Format: 1,
		Workspace: manifest.Workspace{
			Name: "mono",
		},
		Packages: []manifest.Package{
			{
				Name:    "demo",
				Version: "0.1.0",
				Kind:    "app",
				Dependencies: []manifest.DependencyIntent{
					{
						Key:  "build-tool",
						Kind: "tool",
						Source: manifest.Source{
							Kind: "workspace",
							Name: "@prisma-ui/build-tool",
						},
					},
				},
				Targets: []manifest.Target{
					{
						Name:    "app",
						Export:  ".",
						Entry:   "src/index.ts",
						Runtime: "dist/index.js",
						Types:   "dist/index.d.ts",
					},
				},
				Tools: []string{"build-tool"},
			},
		},
	}
	g, diags := graph.Build(ir)
	if len(diags) != 0 {
		t.Fatalf("graph diags=%#v", diags)
	}

	res := CheckRuntimeBoundaries(CheckOptions{RootDir: root, Graph: g})
	for _, d := range res.Diagnostics {
		if d.Code == "TSPACK_BOUNDARY_TOOL_RUNTIME_IMPORT" {
			return
		}
	}
	t.Fatalf("missing tool runtime diagnostic: %#v", res.Diagnostics)
}

func buildM33DTransitiveBoundaryGraph(t *testing.T, boundary manifest.BoundaryRule) *graph.WorkspaceGraph {
	t.Helper()

	ir := &manifest.ManifestIR{
		Format:    1,
		Workspace: manifest.Workspace{Name: "ws"},
		Packages: []manifest.Package{
			{
				Name:    "app",
				Version: "1.0.0",
				Kind:    "library",
				Dependencies: []manifest.DependencyIntent{
					{
						Key:  "react-dom",
						Kind: "runtime",
						Source: manifest.Source{
							Kind:    "npm",
							Package: "react-dom",
							Range:   "^19.0.0",
						},
					},
				},
				Targets: []manifest.Target{
					{
						Name:    "core",
						Export:  ".",
						Entry:   "src/index.ts",
						Runtime: "dist/index.js",
						Types:   "dist/index.d.ts",
						Deps:    []string{"react-dom"},
					},
				},
				Boundaries: []manifest.BoundaryRule{boundary},
			},
		},
	}

	g, diags := graph.Build(ir)
	if len(diags) != 0 {
		t.Fatalf("graph diags=%#v", diags)
	}
	return g
}

func TestM33DTransitiveFromExactFileDeniesReachableImport(t *testing.T) {
	root := t.TempDir()
	writeM33BTransitiveSourceGraph(t, root)

	res := CheckRuntimeBoundaries(CheckOptions{RootDir: root, Graph: buildM33DTransitiveBoundaryGraph(t, manifest.BoundaryRule{TransitiveFrom: "src/index.ts", DenyDeps: []string{"react-dom"}})})
	for _, d := range res.Diagnostics {
		if d.Code != "TSPACK_BOUNDARY_EXPLICIT_DENY" {
			continue
		}
		details := strings.Join(d.Details, "|")
		path := strings.Join(boundary.PathFromDetails(d), "|")
		if !strings.Contains(details, "transitiveFrom=src/index.ts") {
			t.Fatalf("details missing transitiveFrom: %#v", d.Details)
		}
		if !strings.Contains(details, "seed=") || !strings.Contains(details, "src/index.ts") {
			t.Fatalf("details missing seed: %#v", d.Details)
		}
		if !strings.Contains(path, "src/index.ts") || !strings.Contains(path, "src/button.tsx") || !strings.Contains(path, "react-dom") {
			t.Fatalf("path missing expected chain: %s", path)
		}
		return
	}
	t.Fatalf("missing transitive explicit deny diagnostic: %#v", res.Diagnostics)
}

func TestM33DTransitiveFromIncludesSeedFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/src", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/src/index.ts", []byte(`import "react-dom";
`), 0o644); err != nil {
		t.Fatal(err)
	}

	res := CheckRuntimeBoundaries(CheckOptions{RootDir: root, Graph: buildM33DTransitiveBoundaryGraph(t, manifest.BoundaryRule{TransitiveFrom: "src/index.ts", DenyDeps: []string{"react-dom"}})})
	for _, d := range res.Diagnostics {
		if d.Code == "TSPACK_BOUNDARY_EXPLICIT_DENY" {
			return
		}
	}
	t.Fatalf("missing seed-file transitive deny: %#v", res.Diagnostics)
}

func TestM33DTransitiveFromReportsMultiHopPath(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/src", 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"src/index.ts": `import "./a.js";
`,
		"src/a.ts": `import "./b.js";
`,
		"src/b.ts": `import "react-dom";
`,
	}
	for name, content := range files {
		if err := os.WriteFile(root+"/"+name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	res := CheckRuntimeBoundaries(CheckOptions{RootDir: root, Graph: buildM33DTransitiveBoundaryGraph(t, manifest.BoundaryRule{TransitiveFrom: "src/index.ts", DenyDeps: []string{"react-dom"}})})
	for _, d := range res.Diagnostics {
		if d.Code != "TSPACK_BOUNDARY_EXPLICIT_DENY" {
			continue
		}
		path := strings.Join(boundary.PathFromDetails(d), "|")
		for _, want := range []string{"src/index.ts", "src/a.ts", "src/b.ts", "react-dom"} {
			if !strings.Contains(path, want) {
				t.Fatalf("path %q missing %s", path, want)
			}
		}
		return
	}
	t.Fatalf("missing multi-hop transitive deny: %#v", res.Diagnostics)
}

func TestM33DTransitiveFromCycleSafe(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/src", 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"src/index.ts": `import "./a.js";
`,
		"src/a.ts": `import "./b.js";
`,
		"src/b.ts": `import "./a.js";
import "react-dom";
`,
	}
	for name, content := range files {
		if err := os.WriteFile(root+"/"+name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	res := CheckRuntimeBoundaries(CheckOptions{RootDir: root, Graph: buildM33DTransitiveBoundaryGraph(t, manifest.BoundaryRule{TransitiveFrom: "src/a.ts", DenyDeps: []string{"react-dom"}})})
	for _, d := range res.Diagnostics {
		if d.Code == "TSPACK_BOUNDARY_EXPLICIT_DENY" {
			return
		}
	}
	t.Fatalf("missing cycle transitive deny: %#v", res.Diagnostics)
}

func TestM33DTransitiveFromGlobCanSeedNestedFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/src/nested", 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"src/index.ts": `import "./nested/button.js";
`,
		"src/nested/button.ts": `import "react-dom";
`,
	}
	for name, content := range files {
		if err := os.WriteFile(root+"/"+name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	res := CheckRuntimeBoundaries(CheckOptions{RootDir: root, Graph: buildM33DTransitiveBoundaryGraph(t, manifest.BoundaryRule{TransitiveFrom: "src/**", DenyDeps: []string{"react-dom"}})})
	for _, d := range res.Diagnostics {
		if d.Code == "TSPACK_BOUNDARY_EXPLICIT_DENY" {
			return
		}
	}
	t.Fatalf("missing glob transitive deny: %#v", res.Diagnostics)
}

func buildM33EAllowOnlyGraph(t *testing.T, boundaryRules []manifest.BoundaryRule, targetDeps []string) *graph.WorkspaceGraph {
	t.Helper()

	ir := &manifest.ManifestIR{
		Format:    1,
		Workspace: manifest.Workspace{Name: "ws"},
		Packages: []manifest.Package{
			{
				Name:    "app",
				Version: "1.0.0",
				Kind:    "library",
				Dependencies: []manifest.DependencyIntent{
					{
						Key:  "react",
						Kind: "runtime",
						Source: manifest.Source{
							Kind:    "npm",
							Package: "react",
							Range:   "^19.0.0",
						},
					},
					{
						Key:  "react-dom",
						Kind: "runtime",
						Source: manifest.Source{
							Kind:    "npm",
							Package: "react-dom",
							Range:   "^19.0.0",
						},
					},
					{
						Key:  "typescript",
						Kind: "tool",
						Source: manifest.Source{
							Kind:    "npm",
							Package: "typescript",
							Range:   "^5.9.0",
						},
					},
				},
				Targets: []manifest.Target{
					{
						Name:    "core",
						Export:  ".",
						Entry:   "src/index.ts",
						Runtime: "dist/index.js",
						Types:   "dist/index.d.ts",
						Deps:    targetDeps,
					},
				},
				Tools:      []string{"typescript"},
				Boundaries: boundaryRules,
			},
		},
	}
	g, diags := graph.Build(ir)
	if len(diags) != 0 {
		t.Fatalf("graph diags=%#v", diags)
	}
	return g
}

func writeM33EFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		fullPath := root + "/" + name
		if err := os.MkdirAll(fullPath[:strings.LastIndex(fullPath, "/")], 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func findDiagnosticByCode(diags []diag.Diagnostic, code string) (diag.Diagnostic, bool) {
	for _, d := range diags {
		if d.Code == code {
			return d, true
		}
	}
	return diag.Diagnostic{}, false
}

func TestM33EFromAllowOnlyAllowsListedExternalAndRelativeImports(t *testing.T) {
	root := t.TempDir()
	writeM33EFiles(t, root, map[string]string{
		"src/index.ts": `import "react";
import "./local.js";
`,
		"src/local.ts": `export const local = true;
`,
	})

	graph := buildM33EAllowOnlyGraph(t, []manifest.BoundaryRule{{From: "src/**", AllowOnly: []string{"react"}}}, []string{"react", "react-dom"})
	res := CheckRuntimeBoundaries(CheckOptions{RootDir: root, Graph: graph})
	if len(res.Diagnostics) != 0 {
		t.Fatalf("allowOnly produced unexpected diagnostics: %#v", res.Diagnostics)
	}
}

func TestM33EFromAllowOnlyDeniesUnlistedExternal(t *testing.T) {
	root := t.TempDir()
	writeM33EFiles(t, root, map[string]string{
		"src/index.ts": `import "react-dom";
`,
	})

	graph := buildM33EAllowOnlyGraph(t, []manifest.BoundaryRule{{From: "src/**", AllowOnly: []string{"react"}}}, []string{"react", "react-dom"})
	res := CheckRuntimeBoundaries(CheckOptions{RootDir: root, Graph: graph})
	diagnostic, ok := findDiagnosticByCode(res.Diagnostics, "TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION")
	if !ok {
		t.Fatalf("missing allowOnly violation: %#v", res.Diagnostics)
	}
	details := strings.Join(diagnostic.Details, "|")
	for _, want := range []string{"from=src/**", "allowOnly=react", "react-dom"} {
		if !strings.Contains(details, want) {
			t.Fatalf("details %q missing %s", details, want)
		}
	}
}

func TestM33EFromEmptyAllowOnlyDeniesExternalButAllowsRelative(t *testing.T) {
	root := t.TempDir()
	writeM33EFiles(t, root, map[string]string{
		"src/index.ts": `import "./local.js";
import "react";
`,
		"src/local.ts": `export const local = true;
`,
	})

	graph := buildM33EAllowOnlyGraph(t, []manifest.BoundaryRule{{From: "src/**", AllowOnly: []string{}}}, []string{"react"})
	res := CheckRuntimeBoundaries(CheckOptions{RootDir: root, Graph: graph})
	if _, ok := findDiagnosticByCode(res.Diagnostics, "TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION"); !ok {
		t.Fatalf("missing empty allowOnly violation: %#v", res.Diagnostics)
	}
	for _, diagnostic := range res.Diagnostics {
		if strings.Contains(strings.Join(diagnostic.Details, "|"), "./local.js") {
			t.Fatalf("relative import should not be denied by allowOnly: %#v", res.Diagnostics)
		}
	}
}

func TestM33ETransitiveFromAllowOnlyDeniesReachableImport(t *testing.T) {
	root := t.TempDir()
	writeM33EFiles(t, root, map[string]string{
		"src/index.ts": `import "./button.js";
`,
		"src/button.tsx": `import "react-dom";
`,
	})

	graph := buildM33EAllowOnlyGraph(t, []manifest.BoundaryRule{{TransitiveFrom: "src/index.ts", AllowOnly: []string{"react"}}}, []string{"react", "react-dom"})
	res := CheckRuntimeBoundaries(CheckOptions{RootDir: root, Graph: graph})
	diagnostic, ok := findDiagnosticByCode(res.Diagnostics, "TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION")
	if !ok {
		t.Fatalf("missing transitive allowOnly violation: %#v", res.Diagnostics)
	}
	details := strings.Join(diagnostic.Details, "|")
	for _, want := range []string{"transitiveFrom=src/index.ts", "seed=src/index.ts", "allowOnly=react"} {
		if !strings.Contains(details, want) {
			t.Fatalf("details %q missing %s", details, want)
		}
	}
	path := strings.Join(boundary.PathFromDetails(diagnostic), "|")
	for _, want := range []string{"src/index.ts", "src/button.tsx", "react-dom"} {
		if !strings.Contains(path, want) {
			t.Fatalf("path %q missing %s", path, want)
		}
	}
}

func TestM33ETransitiveFromAllowOnlyDeniesSeedImport(t *testing.T) {
	root := t.TempDir()
	writeM33EFiles(t, root, map[string]string{
		"src/index.ts": `import "react-dom";
`,
	})

	graph := buildM33EAllowOnlyGraph(t, []manifest.BoundaryRule{{TransitiveFrom: "src/index.ts", AllowOnly: []string{"react"}}}, []string{"react", "react-dom"})
	res := CheckRuntimeBoundaries(CheckOptions{RootDir: root, Graph: graph})
	if _, ok := findDiagnosticByCode(res.Diagnostics, "TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION"); !ok {
		t.Fatalf("missing seed allowOnly violation: %#v", res.Diagnostics)
	}
}

func TestM33EExplicitDenyWinsOverAllowOnly(t *testing.T) {
	root := t.TempDir()
	writeM33EFiles(t, root, map[string]string{
		"src/index.ts": `import "react-dom";
`,
	})

	graph := buildM33EAllowOnlyGraph(t, []manifest.BoundaryRule{{From: "src/**", DenyDeps: []string{"react-dom"}, AllowOnly: []string{"react-dom"}}}, []string{"react-dom"})
	res := CheckRuntimeBoundaries(CheckOptions{RootDir: root, Graph: graph})
	if _, ok := findDiagnosticByCode(res.Diagnostics, "TSPACK_BOUNDARY_EXPLICIT_DENY"); !ok {
		t.Fatalf("missing explicit deny diagnostic: %#v", res.Diagnostics)
	}
	if _, ok := findDiagnosticByCode(res.Diagnostics, "TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION"); ok {
		t.Fatalf("explicit deny should be enough without allowOnly violation: %#v", res.Diagnostics)
	}
}

func TestM33EAllowOnlyDoesNotDeclareDependency(t *testing.T) {
	root := t.TempDir()
	writeM33EFiles(t, root, map[string]string{
		"src/index.ts": `import "react";
`,
	})

	graph := buildM33EAllowOnlyGraph(t, []manifest.BoundaryRule{{From: "src/**", AllowOnly: []string{"react"}}}, []string{})
	res := CheckRuntimeBoundaries(CheckOptions{RootDir: root, Graph: graph})
	if _, ok := findDiagnosticByCode(res.Diagnostics, "TSPACK_BOUNDARY_UNDECLARED_IMPORT"); !ok {
		t.Fatalf("missing undeclared import diagnostic: %#v", res.Diagnostics)
	}
	if _, ok := findDiagnosticByCode(res.Diagnostics, "TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION"); ok {
		t.Fatalf("react is listed in allowOnly and should not get allowOnly violation: %#v", res.Diagnostics)
	}
}

func TestM33EMultipleAllowOnlyRulesAreStrict(t *testing.T) {
	root := t.TempDir()
	writeM33EFiles(t, root, map[string]string{
		"src/index.ts": `import "react-dom";
`,
	})

	graph := buildM33EAllowOnlyGraph(t, []manifest.BoundaryRule{
		{From: "src/**", AllowOnly: []string{"react"}},
		{From: "src/index.ts", AllowOnly: []string{"react-dom"}},
	}, []string{"react", "react-dom"})
	res := CheckRuntimeBoundaries(CheckOptions{RootDir: root, Graph: graph})
	if _, ok := findDiagnosticByCode(res.Diagnostics, "TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION"); !ok {
		t.Fatalf("missing stricter allowOnly violation: %#v", res.Diagnostics)
	}
}

func TestM33EToolRuntimeWinsOverAllowOnly(t *testing.T) {
	root := t.TempDir()
	writeM33EFiles(t, root, map[string]string{
		"src/index.ts": `import "typescript";
`,
	})

	graph := buildM33EAllowOnlyGraph(t, []manifest.BoundaryRule{{From: "src/**", AllowOnly: []string{"typescript"}}}, []string{})
	res := CheckRuntimeBoundaries(CheckOptions{RootDir: root, Graph: graph})
	if _, ok := findDiagnosticByCode(res.Diagnostics, "TSPACK_BOUNDARY_TOOL_RUNTIME_IMPORT"); !ok {
		t.Fatalf("missing tool runtime diagnostic: %#v", res.Diagnostics)
	}
	if _, ok := findDiagnosticByCode(res.Diagnostics, "TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION"); ok {
		t.Fatalf("tool runtime should return before allowOnly: %#v", res.Diagnostics)
	}
}

func TestRuntimeSwitchCheckStability(t *testing.T) {
	fixtures := []string{
		"runtime-switch-nodejs",
		"runtime-switch-bun",
		"runtime-switch-deno",
	}

	var first []diag.Diagnostic
	for _, fixtureName := range fixtures {
		t.Run(fixtureName, func(t *testing.T) {
			fixturePath := "../../fixtures/valid/" + fixtureName + "/manifest.ir.golden.json"
			rootDir := "../../fixtures/valid/" + fixtureName
			res := CheckRuntimeBoundaries(CheckOptions{RootDir: rootDir, Graph: loadGraph(t, fixturePath)})
			if len(res.Diagnostics) != 0 {
				t.Fatalf("runtime switch check diagnostics=%#v", res.Diagnostics)
			}
			if first == nil {
				first = append([]diag.Diagnostic(nil), res.Diagnostics...)
				return
			}
			if !reflect.DeepEqual(first, res.Diagnostics) {
				t.Fatalf("diagnostics differ: first=%#v current=%#v", first, res.Diagnostics)
			}
		})
	}
}
