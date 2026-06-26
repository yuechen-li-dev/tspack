package typesurface

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func loadGraph(t *testing.T, fixture string) *graph.WorkspaceGraph {
	b, _ := os.ReadFile(fixture)
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
func has(diags []string, x string) bool {
	for _, d := range diags {
		if d == x {
			return true
		}
	}
	return false
}

func TestM5TypeFixtures(t *testing.T) {
	res := CheckTypeSurfaces(CheckOptions{RootDir: "../..", Graph: loadGraph(t, "../../fixtures/valid/m5-types-basic/manifest.ir.golden.json")})
	if len(res.Diagnostics) != 0 {
		t.Fatalf("unexpected diags: %#v", res.Diagnostics)
	}
	cases := map[string]string{
		"../../fixtures/invalid/m5-missing-types/manifest.ir.golden.json":              "TSPACK_TYPE_MISSING_OUTPUT",
		"../../fixtures/invalid/m5-core-types-leak-vue/manifest.ir.golden.json":        "TSPACK_TYPE_OPTIONAL_PEER_LEAK",
		"../../fixtures/invalid/m5-types-tool-reference/manifest.ir.golden.json":       "TSPACK_TYPE_TOOL_REFERENCE",
		"../../fixtures/invalid/m5-types-undeclared-reference/manifest.ir.golden.json": "TSPACK_TYPE_UNDECLARED_REFERENCE",
		"../../fixtures/invalid/m5-types-unresolved-relative/manifest.ir.golden.json":  "TSPACK_TYPE_UNRESOLVED_RELATIVE",
	}
	for f, code := range cases {
		res := CheckTypeSurfaces(CheckOptions{RootDir: "../..", Graph: loadGraph(t, f)})
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

func TestLeakPathAndDeterminism(t *testing.T) {
	g := loadGraph(t, "../../fixtures/invalid/m5-core-types-leak-vue/manifest.ir.golden.json")
	a := CheckTypeSurfaces(CheckOptions{RootDir: "../..", Graph: g}).Diagnostics
	b := CheckTypeSurfaces(CheckOptions{RootDir: "../..", Graph: g}).Diagnostics
	if !reflect.DeepEqual(a, b) {
		t.Fatal("nondeterministic")
	}
	for _, d := range a {
		if d.Code == "TSPACK_TYPE_OPTIONAL_PEER_LEAK" {
			p := strings.Join(PathFromDetails(d), "|")
			if !strings.Contains(p, "dist/index.d.ts") || !strings.Contains(p, "dist/text/index.d.ts") || !strings.Contains(p, "dist/text/vue/index.d.ts") || !strings.Contains(p, "vue") {
				t.Fatal(p)
			}
			return
		}
	}
	t.Fatal("missing leak")
}
