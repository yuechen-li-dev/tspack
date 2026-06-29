package compat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func TestRenderJSONStableKeyOrder(t *testing.T) {
	rendered, err := RenderJSON(json.RawMessage(`{"z":1,"a":{"b":2,"a":1},"items":[{"d":4,"c":3}]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"a\": {\n    \"a\": 1,\n    \"b\": 2\n  },\n  \"items\": [\n    {\n      \"c\": 3,\n      \"d\": 4\n    }\n  ],\n  \"z\": 1\n}\n"
	if string(rendered) != want {
		t.Fatalf("unexpected render:\n%s", rendered)
	}
}

func TestPlanAndWrite(t *testing.T) {
	root := t.TempDir()
	ir := &manifest.ManifestIR{CompatFiles: []manifest.CompatFile{{Path: "tsconfig.tspack.json", Format: "json", Value: json.RawMessage(`{"compilerOptions":{"moduleResolution":"Bundler"}}`)}}}
	statuses, err := Plan(root, ir)
	if err != nil {
		t.Fatal(err)
	}
	if statuses[0].State != StateMissing {
		t.Fatalf("state = %s, want missing", statuses[0].State)
	}
	if err := Write(root, statuses); err != nil {
		t.Fatal(err)
	}
	statuses, err = Plan(root, ir)
	if err != nil {
		t.Fatal(err)
	}
	if statuses[0].State != StateClean {
		t.Fatalf("state = %s, want clean", statuses[0].State)
	}
	if err := os.WriteFile(filepath.Join(root, "tsconfig.tspack.json"), []byte(`{"drift":true}\n`), 0o644); err != nil {
		t.Fatal(err)
	}
	statuses, err = Plan(root, ir)
	if err != nil {
		t.Fatal(err)
	}
	if statuses[0].State != StateDrifted || !strings.HasPrefix(statuses[0].DesiredHash, "") {
		t.Fatalf("state = %s, want drifted", statuses[0].State)
	}
}
