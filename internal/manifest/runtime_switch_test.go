package manifest

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRuntimeSwitchManifestsDifferOnlyByWorkspaceRuntimeLine(t *testing.T) {
	nodejs := readRuntimeSwitchManifest(t, "nodejs")
	bun := readRuntimeSwitchManifest(t, "bun")
	deno := readRuntimeSwitchManifest(t, "deno")

	normalizedNodejs := normalizeRuntimeSwitchManifest(nodejs)
	normalizedBun := normalizeRuntimeSwitchManifest(bun)
	normalizedDeno := normalizeRuntimeSwitchManifest(deno)

	if normalizedNodejs != normalizedBun {
		t.Fatalf("nodejs and bun runtime switch manifests differ beyond runtime line")
	}
	if normalizedNodejs != normalizedDeno {
		t.Fatalf("nodejs and deno runtime switch manifests differ beyond runtime line")
	}
}

func TestRuntimeSwitchIRDiffersOnlyByWorkspaceRuntime(t *testing.T) {
	profiles := []string{"nodejs", "bun", "deno"}
	loaded := make(map[string]*ManifestIR)

	for _, profile := range profiles {
		ir := loadRuntimeSwitchFixture(t, profile)
		if ir.Workspace.Runtime != profile {
			t.Fatalf("%s fixture normalized to runtime %q", profile, ir.Workspace.Runtime)
		}
		loaded[profile] = ir
	}

	baseline := cloneRuntimeSwitchIRWithRuntime(loaded["nodejs"], "<RUNTIME>")
	for _, profile := range []string{"bun", "deno"} {
		current := cloneRuntimeSwitchIRWithRuntime(loaded[profile], "<RUNTIME>")
		if !reflect.DeepEqual(baseline, current) {
			t.Fatalf("nodejs and %s IR differ beyond workspace runtime:\nnodejs=%#v\n%s=%#v", profile, baseline, profile, current)
		}
	}
}

func readRuntimeSwitchManifest(t *testing.T, profile string) string {
	t.Helper()
	path := filepath.Join("..", "..", "fixtures", "valid", "runtime-switch-"+profile, "manifest.tsx")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func normalizeRuntimeSwitchManifest(contents string) string {
	contents = strings.ReplaceAll(contents, `runtime="nodejs"`, `runtime="<RUNTIME>"`)
	contents = strings.ReplaceAll(contents, `runtime="bun"`, `runtime="<RUNTIME>"`)
	contents = strings.ReplaceAll(contents, `runtime="deno"`, `runtime="<RUNTIME>"`)
	return contents
}

func loadRuntimeSwitchFixture(t *testing.T, profile string) *ManifestIR {
	t.Helper()
	path := filepath.Join("..", "..", "fixtures", "valid", "runtime-switch-"+profile, "manifest.ir.golden.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ir, diags := LoadBytes(path, data)
	if len(diags) != 0 {
		t.Fatalf("manifest diagnostics for %s: %#v", profile, diags)
	}
	return ir
}

func cloneRuntimeSwitchIRWithRuntime(ir *ManifestIR, runtime string) *ManifestIR {
	clone := *ir
	workspace := clone.Workspace
	workspace.Runtime = runtime
	clone.Workspace = workspace
	return &clone
}
