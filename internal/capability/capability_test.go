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
		{Kind: "lifecycleScript", Script: "install", Command: "node install.js"},
		{Kind: "lifecycleScript", Script: "postinstall", Command: "node postinstall.js"},
		{Kind: "lifecycleScript", Script: "prepack", Command: "node prepack.js"},
		{Kind: "lifecycleScript", Script: "prepare", Command: "node prepare.js"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected capabilities: got=%#v want=%#v", got, want)
	}
}

func TestFromPackageJSONScriptsAllLifecycleScripts(t *testing.T) {
	scripts := map[string]string{
		"preinstall":     "x",
		"install":        "x",
		"postinstall":    "x",
		"prepublish":     "x",
		"prepare":        "x",
		"prepack":        "x",
		"postpack":       "x",
		"prepublishOnly": "x",
		"postpublish":    "x",
	}
	got := FromPackageJSONScripts(scripts)
	if len(got) != 9 {
		t.Fatalf("expected 9 lifecycle capabilities, got %d", len(got))
	}
}
