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
	tsconfigStatus := findStatusByPath(t, statuses, "tsconfig.tspack.json")
	if tsconfigStatus.State != StateMissing {
		t.Fatalf("state = %s, want missing", tsconfigStatus.State)
	}
	if err := Write(root, statuses); err != nil {
		t.Fatal(err)
	}
	statuses, err = Plan(root, ir)
	if err != nil {
		t.Fatal(err)
	}
	tsconfigStatus = findStatusByPath(t, statuses, "tsconfig.tspack.json")
	if tsconfigStatus.State != StateClean {
		t.Fatalf("state = %s, want clean", tsconfigStatus.State)
	}
	if err := os.WriteFile(filepath.Join(root, "tsconfig.tspack.json"), []byte(`{"drift":true}\n`), 0o644); err != nil {
		t.Fatal(err)
	}
	statuses, err = Plan(root, ir)
	if err != nil {
		t.Fatal(err)
	}
	tsconfigStatus = findStatusByPath(t, statuses, "tsconfig.tspack.json")
	if tsconfigStatus.State != StateDrifted || !strings.HasPrefix(tsconfigStatus.DesiredHash, "") {
		t.Fatalf("state = %s, want drifted", tsconfigStatus.State)
	}
}

func findStatusByPath(t *testing.T, statuses []FileStatus, want string) FileStatus {
	t.Helper()
	for _, status := range statuses {
		if status.Path == want {
			return status
		}
	}
	t.Fatalf("missing status for %s in %#v", want, statuses)
	return FileStatus{}
}
