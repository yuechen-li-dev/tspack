package skyrim

import (
	"encoding/json"
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

func TestDominatusSkyrimManagedControllerCommandIsExplicit(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "Controller.csproj")
	if err := os.WriteFile(project, []byte("<Project />"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURELIAN_MARIONETTE_PROJECT", project)
	command, err := managedControllerCommand(root, filepath.Join("build", "transport.json"))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(command, " ")
	for _, required := range []string{"dotnet", "--project", project, "world-host", "--config", "transport.json"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing %q: %s", required, joined)
		}
	}
	t.Setenv("AURELIAN_MARIONETTE_PROJECT", filepath.Join(root, "missing.csproj"))
	if _, err := managedControllerCommand(root, "transport.json"); err == nil {
		t.Fatal("missing explicit project silently fell back")
	}
}

func TestWorldHostDiscoveryUsesCurrentCopelandProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("AURELIAN_MARIONETTE_PROJECT", "")
	project := filepath.Join(home, "source", "repos", "Copeland", "src", "Skyrim", "Aurelian.Marionette", "Aurelian.Marionette.csproj")
	if err := os.MkdirAll(filepath.Dir(project), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(project, []byte("<Project />"), 0o644); err != nil {
		t.Fatal(err)
	}
	command, err := managedControllerCommand(t.TempDir(), "transport.json")
	if err != nil {
		t.Fatal(err)
	}
	if command[3] != project {
		t.Fatalf("project=%q want=%q", command[3], project)
	}
}

func TestDominatusSkyrimTransportConfigIncludesScopedCheckpointDirectory(t *testing.T) {
	root := t.TempDir()
	profile := skyrimHostProfile{RuntimeOverrides: map[string]map[string]any{
		"MarionetteSSE": {
			"eternal_dragonborn.development.presenter.profile": "fixture",
			"eternal_dragonborn.development.presenter.token":   "fixture-token",
		},
	}}
	path, err := writeSkyrimSessionBootstrapTransportConfig(root, profile, false, false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "build", "skyrim", "checkpoints")
	if config["checkpointDirectory"] != want {
		t.Fatalf("transport config checkpoint directory=%v want=%s", config["checkpointDirectory"], want)
	}
	if config["startupSaveId"] != nil || config["enableNpcNumberProof"] != false || config["enablePlaytestShell"] != false || config["enableOwnedNpcProof"] != false {
		t.Fatalf("ordinary host unexpectedly enabled a proof or save load: %#v", config)
	}
}

func TestWorldHostNamedSaveKeepsFilenameOutOfControllerConfig(t *testing.T) {
	const prefix = "eternal_dragonborn.development.presenter."
	profile := skyrimHostProfile{
		TestSaves: map[string]skyrimTestSave{
			"ordinary-world": {Filename: "Ordinary World.ess", Disposable: true, ReadOnly: true},
		},
		RuntimeOverrides: map[string]map[string]any{
			"MarionetteSSE": {prefix + "profile": "test", prefix + "token": "local-token"},
		},
	}
	effective, err := withDominatusSkyrimExperiment(profile, "test")
	if err != nil {
		t.Fatal(err)
	}
	effective, err = withSkyrimNamedTestSave(effective, "ordinary-world")
	if err != nil {
		t.Fatal(err)
	}
	values := effective.RuntimeOverrides["MarionetteSSE"]
	values[prefix+"allow_npc_value_changes"] = true
	values[prefix+"allow_playtest_input"] = true
	if err := applySkyrimFixtureRuntimeOverrides(effective, values); err != nil {
		t.Fatal(err)
	}
	if values[prefix+"development_session_ed_m2b2d"] != "Ordinary World.ess" {
		t.Fatal("named save was not resolved through the local registry")
	}
	path, err := writeSkyrimSessionBootstrapTransportConfig(t.TempDir(), effective, false, false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config["startupSaveId"] != "ordinary-world" || config["enableNpcNumberProof"] != true || config["enablePlaytestShell"] != true || strings.Contains(string(data), "Ordinary World.ess") {
		t.Fatalf("controller must receive only the symbolic save ID: %#v", config)
	}
	if _, changed := profile.RuntimeOverrides["MarionetteSSE"][prefix+"allow_session_bootstrap"]; changed {
		t.Fatal("named save overlay mutated the original profile")
	}
	for _, id := range []string{"../ordinary-world", "missing"} {
		if _, err := withSkyrimNamedTestSave(profile, id); err == nil {
			t.Fatalf("invalid save ID accepted: %q", id)
		}
	}
	profile.TestSaves["ordinary-world"] = skyrimTestSave{Filename: "Ordinary World.ess", Disposable: true}
	if _, err := withSkyrimNamedTestSave(profile, "ordinary-world"); err == nil {
		t.Fatal("writable save accepted")
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
	source := "[eternal_dragonborn.development.presenter]\nenabled = false\nallow_semantic_actuation = false\nallow_host_request_evaluation = false\nprofile = \"safe\"\n"
	if err := os.WriteFile(configPath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	target := skyrimTargetRef{PackageRoot: root, Target: manifestSkyrimTargetForOverrideTests()}
	profile := skyrimHostProfile{RuntimeOverrides: map[string]map[string]any{
		"MarionetteSSE": {
			"host": "skyrim-dev",
			"eternal_dragonborn.development.presenter.enabled":                       true,
			"eternal_dragonborn.development.presenter.allow_semantic_actuation":      false,
			"eternal_dragonborn.development.presenter.allow_host_request_evaluation": true,
			"eternal_dragonborn.development.presenter.profile":                       "skyrim-dev",
			"eternal_dragonborn.development.presenter.token":                         "test-token-012345",
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
	if first.EffectiveSHA256 != second.EffectiveSHA256 || len(first.AppliedOverrides) != 5 {
		t.Fatalf("runtime config was not deterministic: %#v %#v", first, second)
	}
	if _, err := os.Stat(first.StagedPath); !os.IsNotExist(err) {
		t.Fatal("dry-run runtime config planning wrote a staged file")
	}
	if first.AppliedOverrides[4].EffectiveValue != "<redacted>" {
		t.Fatalf("secret runtime override was not redacted: %#v", first.AppliedOverrides[4])
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
			{Path: "eternal_dragonborn.development.presenter.allow_host_request_evaluation", Type: "boolean"},
			{Path: "eternal_dragonborn.development.presenter.profile", Type: "string"},
			{Path: "eternal_dragonborn.development.presenter.token", Type: "string", Secret: true},
		},
	}
}

func TestOwnedNpcProofKeepsIndependentCapabilitiesAndComposesItsOwnNumberProof(t *testing.T) {
	const prefix = "eternal_dragonborn.development.presenter."
	values := map[string]any{prefix + "profile": "test", prefix + "token": "test", prefix + "allow_npc_value_changes": true, prefix + "allow_owned_npc_spawn": true}
	profile := skyrimHostProfile{RuntimeOverrides: map[string]map[string]any{"MarionetteSSE": values}}
	path, err := writeSkyrimSessionBootstrapTransportConfig(t.TempDir(), profile, false, false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config["enableOwnedNpcProof"] != true || config["enableNpcNumberProof"] != false {
		t.Fatalf("proof composition: %#v", config)
	}
}

func TestOwnedMovementProofUsesOnlyItsOwnBehaviorAndScopedCapability(t *testing.T) {
	const prefix = "eternal_dragonborn.development.presenter."
	values := map[string]any{prefix + "profile": "test", prefix + "token": "test", prefix + "allow_owned_npc_spawn": true, prefix + "allow_owned_npc_movement": true}
	profile := skyrimHostProfile{RuntimeOverrides: map[string]map[string]any{"MarionetteSSE": values}}
	path, err := writeSkyrimSessionBootstrapTransportConfig(t.TempDir(), profile, false, false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config["enableOwnedMovementProof"] != true || config["enableOwnedNpcProof"] != false || config["enableNpcNumberProof"] != false {
		t.Fatalf("movement proof composition: %#v", config)
	}
}

func TestOwnedNpcAgentDoesNotComposeEarlierProofs(t *testing.T) {
	const prefix = "eternal_dragonborn.development.presenter."
	values := map[string]any{
		prefix + "profile": "test", prefix + "token": "test",
		prefix + "allow_owned_npc_spawn": true, prefix + "allow_owned_npc_movement": true,
	}
	profile := skyrimHostProfile{RuntimeOverrides: map[string]map[string]any{"MarionetteSSE": values}}
	path, err := writeSkyrimSessionBootstrapTransportConfig(t.TempDir(), profile, true, true)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config["enablePatrolTrial"] != true || config["enableOwnedNpcAgent"] != true || config["enableOwnedMovementProof"] != false ||
		config["enableOwnedNpcProof"] != false || config["enableNpcNumberProof"] != false {
		t.Fatalf("agent composition: %#v", config)
	}
}
