package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
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

func TestSkyrimRuntimeConfigAppliesDeclaredTypedOverridesDeterministically(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "MarionetteSSE.toml")
	source := "[eternal_dragonborn.development.presenter]\nenabled = false\nallow_semantic_actuation = false\nprofile = \"safe\"\n"
	if err := os.WriteFile(configPath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	target := skyrimTargetRef{PackageRoot: root, Target: manifestSkyrimTargetForOverrideTests()}
	profile := skyrimHostProfile{RuntimeOverrides: map[string]map[string]any{
		"MarionetteSSE": {
			"host": "skyrim-dev",
			"eternal_dragonborn.development.presenter.enabled":                  true,
			"eternal_dragonborn.development.presenter.allow_semantic_actuation": false,
			"eternal_dragonborn.development.presenter.profile":                  "skyrim-dev",
			"eternal_dragonborn.development.presenter.token":                    "test-token-012345",
		},
	}}
	first, err := buildSkyrimRuntimeConfig(root, target, profile, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := buildSkyrimRuntimeConfig(root, target, profile, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.EffectiveSHA256 != second.EffectiveSHA256 || len(first.AppliedOverrides) != 4 {
		t.Fatalf("runtime config was not deterministic: %#v %#v", first, second)
	}
	if _, err := os.Stat(first.StagedPath); !os.IsNotExist(err) {
		t.Fatal("dry-run runtime config planning wrote a staged file")
	}
	if first.AppliedOverrides[3].EffectiveValue != "<redacted>" {
		t.Fatalf("secret runtime override was not redacted: %#v", first.AppliedOverrides[3])
	}
	staged, err := buildSkyrimRuntimeConfig(root, target, profile, true)
	if err != nil {
		t.Fatal(err)
	}
	stagedHash, err := hashFile(staged.StagedPath)
	if err != nil || stagedHash != staged.EffectiveSHA256 {
		t.Fatalf("staged runtime config hash mismatch: %s %v", stagedHash, err)
	}
	stagedText, err := os.ReadFile(staged.StagedPath)
	if err != nil || !strings.Contains(string(stagedText), "profile = \"skyrim-dev\"") || !strings.Contains(string(stagedText), "enabled = true") {
		t.Fatalf("staged config did not preserve compatible TOML scalar syntax: %q %v", stagedText, err)
	}
	committed, err := os.ReadFile(configPath)
	if err != nil || string(committed) != source {
		t.Fatal("runtime config generation mutated the committed source")
	}
}

func TestSkyrimRuntimeConfigRejectsUndeclaredAndMismatchedOverrides(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "MarionetteSSE.toml"), []byte("[eternal_dragonborn.development.presenter]\nenabled = false\nallow_semantic_actuation = false\nprofile = \"safe\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := skyrimTargetRef{PackageRoot: root, Target: manifestSkyrimTargetForOverrideTests()}
	for name, values := range map[string]map[string]any{
		"undeclared": {"host": "skyrim-dev", "eternal_dragonborn.development.presenter.other": true},
		"wrong type": {"host": "skyrim-dev", "eternal_dragonborn.development.presenter.enabled": "true"},
		"wrong host": {"host": "another-host", "eternal_dragonborn.development.presenter.enabled": true},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := buildSkyrimRuntimeConfig(root, target, skyrimHostProfile{RuntimeOverrides: map[string]map[string]any{"MarionetteSSE": values}}, false)
			if err == nil {
				t.Fatal("expected rejected runtime override")
			}
		})
	}
}

func TestSkyrimRuntimeConfigRestorationReturnsDeployedConfigToCommittedSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "MarionetteSSE.toml")
	staged := filepath.Join(root, "build", "skyrim", "runtime", "effective.toml")
	destination := filepath.Join(root, "game", "Data", "SKSE", "Plugins", "MarionetteSSE.toml")
	if err := os.MkdirAll(filepath.Dir(staged), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("enabled = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("enabled = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("previous = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	effectiveHash, _ := hashFile(staged)
	plan := skyrimPlanReport{
		ManifestSHA256: "restore-test",
		RuntimeConfig: skyrimRuntimeConfigPlan{
			SourcePath:         source,
			RestorationPlanned: true,
		},
		Files: []skyrimFilePlan{{
			Kind:               "config",
			Source:             source,
			MaterializedSource: staged,
			Destination:        destination,
			SourceSHA256:       effectiveHash,
			Action:             "replace",
		}},
	}
	if _, err := materializeSkyrimPlan(root, plan); err != nil {
		t.Fatal(err)
	}
	if deployed, _ := os.ReadFile(destination); string(deployed) != "enabled = true\n" {
		t.Fatalf("effective config was not deployed: %q", deployed)
	}
	if err := restoreSkyrimRuntimeConfig(root, plan); err != nil {
		t.Fatal(err)
	}
	if restored, _ := os.ReadFile(destination); string(restored) != "enabled = false\n" {
		t.Fatalf("committed safe config was not restored: %q", restored)
	}
}

func manifestSkyrimTargetForOverrideTests() manifest.SkyrimTarget {
	return manifest.SkyrimTarget{
		Host:                  "skyrim-dev",
		RuntimeConfig:         "MarionetteSSE.toml",
		RuntimeOverrideTarget: "MarionetteSSE",
		RuntimeOverrideFields: []manifest.SkyrimRuntimeOverride{
			{Path: "eternal_dragonborn.development.presenter.enabled", Type: "boolean"},
			{Path: "eternal_dragonborn.development.presenter.allow_semantic_actuation", Type: "boolean"},
			{Path: "eternal_dragonborn.development.presenter.profile", Type: "string"},
			{Path: "eternal_dragonborn.development.presenter.token", Type: "string", Secret: true},
		},
	}
}
