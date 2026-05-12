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
	if err != nil { t.Fatal(err) }
	ir, diags := manifest.LoadBytes(fixture, b)
	if len(diags) > 0 { t.Fatalf("manifest diags: %#v", diags) }
	g, diags := graph.Build(ir)
	if len(diags) > 0 { t.Fatalf("graph diags: %#v", diags) }
	return g
}

func TestM4Fixtures(t *testing.T) {
	pass := []string{"../../fixtures/valid/m4-basic/manifest.ir.golden.json", "../../fixtures/valid/m4-react-vue-isolated/manifest.ir.golden.json"}
	for _, f := range pass {
		res := CheckRuntimeBoundaries(CheckOptions{RootDir: "../..", Graph: loadGraph(t, f)})
		if len(res.Diagnostics) != 0 { t.Fatalf("%s diags=%#v", f, res.Diagnostics) }
	}
}

func TestM4Invalids(t *testing.T) {
	cases := map[string]string{
		"../../fixtures/invalid/vue-leaks-core/manifest.ir.golden.json": "TSPACK_BOUNDARY_OPTIONAL_PEER_LEAK",
		"../../fixtures/invalid/tool-imported-at-runtime/manifest.ir.golden.json": "TSPACK_BOUNDARY_TOOL_RUNTIME_IMPORT",
		"../../fixtures/invalid/undeclared-import/manifest.ir.golden.json": "TSPACK_BOUNDARY_UNDECLARED_IMPORT",
		"../../fixtures/invalid/explicit-deny/manifest.ir.golden.json": "TSPACK_BOUNDARY_EXPLICIT_DENY",
	}
	for f, code := range cases {
		res := CheckRuntimeBoundaries(CheckOptions{RootDir: "../..", Graph: loadGraph(t, f)})
		ok := false
		for _, d := range res.Diagnostics { if d.Code == code { ok = true } }
		if !ok { t.Fatalf("%s expected %s got %#v", f, code, res.Diagnostics) }
	}
}

func TestLeakPath(t *testing.T) {
	res := CheckRuntimeBoundaries(CheckOptions{RootDir: "../..", Graph: loadGraph(t, "../../fixtures/invalid/vue-leaks-core/manifest.ir.golden.json")})
	for _, d := range res.Diagnostics {
		if d.Code == "TSPACK_BOUNDARY_OPTIONAL_PEER_LEAK" {
			p := strings.Join(boundary.PathFromDetails(d), "|")
			if !strings.Contains(p, "src/index.ts") || !strings.Contains(p, "src/text/index.ts") || !strings.Contains(p, "src/text/vue/index.ts") || !strings.Contains(p, "vue") { t.Fatalf("path %s", p) }
			return
		}
	}
	t.Fatal("missing leak diag")
}

func TestDeterministicDiagnostics(t *testing.T) {
	g := loadGraph(t, "../../fixtures/invalid/vue-leaks-core/manifest.ir.golden.json")
	a := CheckRuntimeBoundaries(CheckOptions{RootDir: "../..", Graph: g}).Diagnostics
	b := CheckRuntimeBoundaries(CheckOptions{RootDir: "../..", Graph: g}).Diagnostics
	if !reflect.DeepEqual(a, b) { t.Fatal("nondeterministic") }
}
