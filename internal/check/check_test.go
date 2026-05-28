package check

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/tspack/tspack/internal/boundary"
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
