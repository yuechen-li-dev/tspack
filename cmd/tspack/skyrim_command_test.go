package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSkyrimRunIsExplicitlyOptedIn(t *testing.T) {
	if isSkyrimRunInvocation([]string{"run", "dev"}) {
		t.Fatal("ordinary RunTarget unexpectedly selected Skyrim lifecycle")
	}
	if !isSkyrimRunInvocation([]string{"run", "skyrim"}) {
		t.Fatal("Skyrim target did not select Skyrim lifecycle")
	}
}

func TestNormalizeSkyrimPluginEntriesPreservesUnrelatedOrder(t *testing.T) {
	current := []string{"# header", "*Unofficial Patch.esp", "MarionetteSSE.esp", "*MarionetteSSE.esp", "*MarionetteSSE.Generated.experimental.esp", "*SkyUI_SE.esp"}
	desired, removed := normalizeSkyrimPluginEntries(current, "MarionetteSSE.esp", []string{"MarionetteSSE.Generated.experimental.esp"})
	want := []string{"# header", "*Unofficial Patch.esp", "*MarionetteSSE.esp", "*SkyUI_SE.esp"}
	if !reflect.DeepEqual(desired, want) {
		t.Fatalf("desired=%#v want=%#v", desired, want)
	}
	if !reflect.DeepEqual(removed, []string{"MarionetteSSE.Generated.experimental.esp"}) {
		t.Fatalf("removed=%#v", removed)
	}
}

func TestPlanSkyrimPluginStateRejectsMalformedAndDoesNotWrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "plugins.txt")
	malformed := []byte{'*', 'A', 0, 'B'}
	if err := os.WriteFile(path, malformed, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := planSkyrimPluginState(path, "MarionetteSSE.esp", nil); err == nil {
		t.Fatal("expected malformed state error")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, malformed) {
		t.Fatal("planning changed plugin state")
	}
}

func TestMaterializeSkyrimPlanIsIdempotentAndKeepsBackupOutsideData(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "game", "Data")
	build := filepath.Join(root, "build", "assets")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(build, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(build, "MarionetteSSE.esp")
	destination := filepath.Join(data, "MarionetteSSE.esp")
	state := filepath.Join(root, "plugins.txt")
	_ = os.WriteFile(source, []byte("new"), 0o644)
	_ = os.WriteFile(destination, []byte("old"), 0o644)
	_ = os.WriteFile(state, []byte("*Other.esp\n"), 0o644)
	sourceHash, _ := hashFile(source)
	currentHash, _ := hashFile(destination)
	plan := skyrimPlanReport{
		ManifestSHA256: "manifest",
		Files:          []skyrimFilePlan{{Kind: "esp", Source: source, Destination: destination, SourceSHA256: sourceHash, CurrentSHA256: currentHash, Action: "replace"}},
		PluginState:    skyrimPluginStatePlan{Path: state, CurrentEntries: []string{"*Other.esp"}, DesiredEntries: []string{"*Other.esp", "*MarionetteSSE.esp"}, Changed: true},
	}
	report, err := materializeSkyrimPlan(root, plan)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(report.RollbackPath) == data {
		t.Fatal("rollback directory must not be inside Data")
	}
	if _, err := os.Stat(filepath.Join(data, "MarionetteSSE.esp.backup")); !os.IsNotExist(err) {
		t.Fatal("backup ESP remained in Data")
	}
	deployedHash, _ := hashFile(destination)
	if deployedHash != sourceHash {
		t.Fatal("deployed hash mismatch")
	}
	plan.Files[0].CurrentSHA256 = sourceHash
	plan.Files[0].Action = "unchanged"
	plan.PluginState.Changed = false
	second, err := materializeSkyrimPlan(root, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.ChangedFiles) != 0 || len(second.UnchangedFiles) != 1 {
		t.Fatalf("second deployment was not idempotent: %#v", second)
	}
}

func TestMaterializeSkyrimPlanRollsBackAppliedFilesAfterFailure(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "game", "Data", "MarionetteSSE.esp")
	source := filepath.Join(root, "build", "assets", "MarionetteSSE.esp")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("previous"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("replacement"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceHash, err := hashFile(source)
	if err != nil {
		t.Fatal(err)
	}
	plan := skyrimPlanReport{
		ManifestSHA256: "rollback-test",
		Files: []skyrimFilePlan{
			{Kind: "esp", Source: source, Destination: destination, SourceSHA256: sourceHash, Action: "replace"},
			{Kind: "dll", Source: filepath.Join(root, "missing.dll"), Destination: filepath.Join(root, "game", "Data", "SKSE", "Plugins", "MarionetteSSE.dll"), SourceSHA256: "missing", Action: "replace"},
		},
	}
	if _, err := materializeSkyrimPlan(root, plan); err == nil {
		t.Fatal("expected second file to fail")
	}
	after, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != "previous" {
		t.Fatalf("first file was not rolled back: %q", after)
	}
}
