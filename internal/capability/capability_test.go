package capability

import (
	"reflect"
	"testing"

	"github.com/tspack/tspack/internal/lockfile"
)

func TestFromPackageJSONScriptsLifecycleOnlySorted(t *testing.T) {
	scripts := map[string]string{
		"test":        "vitest",
		"postinstall": "node postinstall.js",
		"prepare":     "node prepare.js",
		"install":     "node install.js",
		"build":       "tsc -p .",
		"prepack":     "node prepack.js",
		"lint":        "eslint .",
	}

	got := FromPackageJSONScripts(scripts)
	want := []lockfile.Capability{
		{Kind: "lifecycle-script", Detail: "install"},
		{Kind: "lifecycle-script", Detail: "postinstall"},
		{Kind: "lifecycle-script", Detail: "prepack"},
		{Kind: "lifecycle-script", Detail: "prepare"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected capabilities: got=%#v want=%#v", got, want)
	}
}

func TestFromPackageJSONScriptsAllLifecycleScripts(t *testing.T) {
	scripts := map[string]string{
		"preinstall":  "x",
		"install":     "x",
		"postinstall": "x",
		"prepublish":  "x",
		"prepare":     "x",
		"prepack":     "x",
		"postpack":    "x",
	}
	got := FromPackageJSONScripts(scripts)
	if len(got) != 7 {
		t.Fatalf("expected 7 lifecycle capabilities, got %d", len(got))
	}
}
