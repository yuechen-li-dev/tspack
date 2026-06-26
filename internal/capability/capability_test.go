package capability

import (
	"reflect"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/lockfile"
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
		"publish":        "x",
		"postpublish":    "x",
	}
	got := FromPackageJSONScripts(scripts)
	if len(got) != 10 {
		t.Fatalf("expected 10 lifecycle capabilities, got %d", len(got))
	}
}

func TestClassifyLifecycleScript(t *testing.T) {
	tests := []struct {
		name                string
		wantCategory        string
		wantConsumerInstall bool
	}{
		{name: "preinstall", wantCategory: LifecycleCategoryConsumerInstall, wantConsumerInstall: true},
		{name: "install", wantCategory: LifecycleCategoryConsumerInstall, wantConsumerInstall: true},
		{name: "postinstall", wantCategory: LifecycleCategoryConsumerInstall, wantConsumerInstall: true},
		{name: "prepublishOnly", wantCategory: LifecycleCategoryMaintainerPublish, wantConsumerInstall: false},
		{name: "prepublish", wantCategory: LifecycleCategoryMaintainerPublish, wantConsumerInstall: false},
		{name: "prepare", wantCategory: LifecycleCategoryMaintainerPublish, wantConsumerInstall: false},
		{name: "prepack", wantCategory: LifecycleCategoryMaintainerPublish, wantConsumerInstall: false},
		{name: "postpack", wantCategory: LifecycleCategoryMaintainerPublish, wantConsumerInstall: false},
		{name: "publish", wantCategory: LifecycleCategoryMaintainerPublish, wantConsumerInstall: false},
		{name: "postpublish", wantCategory: LifecycleCategoryMaintainerPublish, wantConsumerInstall: false},
		{name: "weirdLifecycleMaybe", wantCategory: LifecycleCategoryOther, wantConsumerInstall: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyLifecycleScript(tt.name)
			if got.ScriptName != tt.name {
				t.Fatalf("ScriptName got %q want %q", got.ScriptName, tt.name)
			}
			if got.LifecycleCategory != tt.wantCategory {
				t.Fatalf("LifecycleCategory got %q want %q", got.LifecycleCategory, tt.wantCategory)
			}
			if got.ConsumerInstallTime != tt.wantConsumerInstall {
				t.Fatalf("ConsumerInstallTime got %t want %t", got.ConsumerInstallTime, tt.wantConsumerInstall)
			}
		})
	}
}
