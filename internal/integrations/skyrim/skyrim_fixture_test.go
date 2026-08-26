package skyrim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDiscoverSkyrimSaveCandidatesIsStableAndRedacted(t *testing.T) {
	directory := t.TempDir()
	writeFixtureTestFile(t, filepath.Join(directory, "Save2_Test.ess"), "manual")
	writeFixtureTestFile(t, filepath.Join(directory, "Save2_Test.skse"), "sidecar")
	writeFixtureTestFile(t, filepath.Join(directory, "Autosave1_Test.ess"), "auto")
	first, err := discoverSkyrimSaveCandidates(directory)
	if err != nil {
		t.Fatal(err)
	}
	second, err := discoverSkyrimSaveCandidates(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || first[0].CandidateID != second[0].CandidateID || first[1].CandidateID != second[1].CandidateID {
		t.Fatalf("candidate inventory was not deterministic: %#v %#v", first, second)
	}
	manual, found := findCandidateByManualClassification(first)
	if !found || !manual.SkseSidecarPresent || !manual.LikelyManualSave {
		t.Fatalf("manual paired candidate was not classified: %#v", manual)
	}
	encoded, err := json.Marshal(manual)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), directory) || strings.Contains(string(encoded), "Save2_Test") {
		t.Fatalf("candidate JSON exposed local path or filename: %s", encoded)
	}
	filtered := filterSkyrimSaveCandidates(first, skyrimSaveListOptions{manualOnly: true, requireSidecar: true})
	if len(filtered) != 1 || filtered[0].CandidateID != manual.CandidateID {
		t.Fatalf("safe filters did not retain exactly the paired manual candidate: %#v", filtered)
	}
}

func TestDiscoverSkyrimSaveCandidatesReportsMissingSidecar(t *testing.T) {
	directory := t.TempDir()
	writeFixtureTestFile(t, filepath.Join(directory, "Save3_Test.ess"), "manual")
	candidates, err := discoverSkyrimSaveCandidates(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].SkseSidecarPresent {
		t.Fatalf("missing sidecar was not reported: %#v", candidates)
	}
}

func TestPlanSkyrimFixtureRejectsSourceEqualsDestination(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, fixtureFilename("ed-m2b2d"))
	writeFixtureTestFile(t, path, "save")
	hash, err := hashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = planSkyrimFixture("host", directory, "profile", skyrimHostProfile{}, "ed-m2b2d", skyrimSaveCandidate{CandidateID: "candidate", EssPresent: true, SourceHashEss: hash, essPath: path})
	if err == nil {
		t.Fatal("expected source equals destination rejection")
	}
}

func TestCreateSkyrimFixtureCopiesPairAndPreservesSource(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "saves")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixtureProfile(t, root, directory)
	sourceEss := filepath.Join(directory, "Save9_Source.ess")
	sourceSkse := filepath.Join(directory, "Save9_Source.skse")
	writeFixtureTestFile(t, sourceEss, "source ess")
	writeFixtureTestFile(t, sourceSkse, "source skse")
	candidates, err := discoverSkyrimSaveCandidates(directory)
	if err != nil {
		t.Fatal(err)
	}
	candidate := candidates[0]
	profile, err := loadSkyrimProfile(root, "skyrim-dev")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := planSkyrimFixture("skyrim-dev", directory, "profile", profile, "ed-m2b2d", candidate)
	if err != nil || plan.Action != "create" || plan.FixtureFilename != "MarionetteFixture-ed-m2b2d.ess" {
		t.Fatalf("unexpected initial fixture plan: %#v %v", plan, err)
	}
	report, err := createSkyrimFixture(root, "skyrim-dev", directory, profile, plan, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if !report.FixtureProfileMappingWritten || report.SourceEss.SHA256 != report.SourceEssAfter.SHA256 || report.SourceSkse.SHA256 != report.SourceSkseAfter.SHA256 {
		t.Fatalf("source immutability proof failed: %#v", report)
	}
	updated, err := loadSkyrimProfile(root, "skyrim-dev")
	if err != nil {
		t.Fatal(err)
	}
	fixture, found := updated.TestSaves["ed-m2b2d"]
	if !found || !fixture.Disposable || !fixture.ReadOnly || !fixture.SidecarPresent {
		t.Fatalf("fixture mapping was not persisted safely: %#v", updated)
	}
	inspection, err := inspectSkyrimFixture("skyrim-dev", directory, updated, "ed-m2b2d")
	if err != nil || !inspection.HashesMatch || !inspection.SourceProvenanceKnown {
		t.Fatalf("fixture inspection failed: %#v %v", inspection, err)
	}
	secondPlan, err := planSkyrimFixture("skyrim-dev", directory, "profile", updated, "ed-m2b2d", candidate)
	if err != nil || secondPlan.Action != "unchanged" {
		t.Fatalf("unchanged fixture recreation was not idempotent: %#v %v", secondPlan, err)
	}
}

func TestPlanSkyrimFixtureRequiresReplacementForChangedSource(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "Save9_Source.ess")
	writeFixtureTestFile(t, source, "old source")
	candidate, err := discoverSkyrimSaveCandidates(directory)
	if err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(directory, fixtureFilename("ed-m2b2d"))
	writeFixtureTestFile(t, fixturePath, "different fixture")
	plan, err := planSkyrimFixture("host", directory, "profile", skyrimHostProfile{}, "ed-m2b2d", candidate[0])
	if err != nil || plan.Action != "replace" {
		t.Fatalf("changed fixture did not require replacement: %#v %v", plan, err)
	}
}

func TestResolveSkyrimSaveDirectoryUsesConfiguredPathAndRejectsMissing(t *testing.T) {
	directory := t.TempDir()
	resolved, source, err := resolveSkyrimSaveDirectory(skyrimHostProfile{SaveDirectory: directory})
	if err != nil || resolved != directory || source != "profile" {
		t.Fatalf("configured save directory was not selected: %q %q %v", resolved, source, err)
	}
	if _, _, err := resolveSkyrimSaveDirectory(skyrimHostProfile{SaveDirectory: filepath.Join(directory, "missing")}); err == nil {
		t.Fatal("missing configured save directory was accepted")
	}
}

func TestFixtureDryRunPlanDoesNotWrite(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "Save1_Source.ess")
	writeFixtureTestFile(t, source, "source")
	candidates, err := discoverSkyrimSaveCandidates(directory)
	if err != nil {
		t.Fatal(err)
	}
	_, err = planSkyrimFixture("host", directory, "profile", skyrimHostProfile{}, "ed-m2b2d", candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, fixtureFilename("ed-m2b2d"))); !os.IsNotExist(err) {
		t.Fatal("fixture planning wrote a destination file")
	}
}

func TestSessionBootstrapUsesOnlyProvisionedFixtureMapping(t *testing.T) {
	profile := skyrimHostProfile{
		TestSaves: map[string]skyrimTestSave{
			"ed-m2b2d": {Filename: "MarionetteFixture-ed-m2b2d.ess", Disposable: true, ReadOnly: true, SidecarPresent: true},
		},
		RuntimeOverrides: map[string]map[string]any{
			"MarionetteSSE": {"host": "skyrim-dev", "eternal_dragonborn.development.presenter.token": "local-token"},
		},
	}
	effective, err := withSkyrimSessionBootstrap(profile, "skyrim-dev")
	if err != nil {
		t.Fatal(err)
	}
	values := effective.RuntimeOverrides["MarionetteSSE"]
	if values["eternal_dragonborn.development.presenter.allow_session_bootstrap"] != true || values["eternal_dragonborn.development.presenter.allow_semantic_actuation"] != false || values["eternal_dragonborn.development.presenter.allow_host_request_evaluation"] != false {
		t.Fatalf("session bootstrap did not constrain effective capabilities: %#v", values)
	}
	if _, ok := profile.RuntimeOverrides["MarionetteSSE"]["eternal_dragonborn.development.presenter.allow_session_bootstrap"]; ok {
		t.Fatal("run-scoped bootstrap mutated the host profile")
	}
	if err := applySkyrimFixtureRuntimeOverrides(effective, values); err != nil {
		t.Fatal(err)
	}
	if values["eternal_dragonborn.development.presenter.development_session_ed_m2b2d"] != "MarionetteFixture-ed-m2b2d.ess" || values["eternal_dragonborn.development.presenter.development_session_ed_m2b2d_read_only"] != true {
		t.Fatalf("fixture mapping was not the runtime filename authority: %#v", values)
	}
}

func TestDominatusSkyrimOverlayEnablesOnlyBoundedExperimentGates(t *testing.T) {
	profile := skyrimHostProfile{
		TestSaves: map[string]skyrimTestSave{
			"ed-m2b2d": {Filename: "MarionetteFixture-ed-m2b2d.ess", Disposable: true, ReadOnly: true},
		},
		RuntimeOverrides: map[string]map[string]any{
			"MarionetteSSE": {
				"host": "skyrim-dev",
				"eternal_dragonborn.development.presenter.profile": "skyrim-dev",
				"eternal_dragonborn.development.presenter.token":   "local-token",
			},
		},
	}
	effective, err := withDominatusSkyrimExperiment(profile, "skyrim-dev")
	if err != nil {
		t.Fatal(err)
	}
	values := effective.RuntimeOverrides["MarionetteSSE"]
	for _, path := range []string{
		"eternal_dragonborn.development.presenter.enabled",
		"eternal_dragonborn.development.presenter.allow_session_bootstrap",
		"eternal_dragonborn.development.presenter.allow_semantic_actuation",
		"eternal_dragonborn.development.presenter.allow_host_request_evaluation",
		"eternal_dragonborn.development.presenter.allow_host_fixture_query",
	} {
		if values[path] != true {
			t.Fatalf("expected %s enabled in run-scoped experiment: %#v", path, values)
		}
	}
	if _, ok := profile.RuntimeOverrides["MarionetteSSE"]["eternal_dragonborn.development.presenter.enabled"]; ok {
		t.Fatal("experiment overlay mutated the host profile")
	}
	if effective.INIOverrides[skyrimAlwaysActiveOverridePath] != true {
		t.Fatalf("experiment must keep Skyrim active while unfocused: %#v", effective.INIOverrides)
	}
}

func TestDominatusSkyrimRunOptionIsExplicit(t *testing.T) {
	opts := parseSkyrimRunOptions([]string{"run", "skyrim", "--dominatus-skyrim", "--dry-run"})
	if !opts.dominatusSkyrim || !opts.dryRun || opts.sessionBootstrap {
		t.Fatalf("unexpected Dominatus Skyrim options: %#v", opts)
	}
}

func TestSessionBootstrapWritesIgnoredControllerConfigFromHostValues(t *testing.T) {
	root := t.TempDir()
	profile := skyrimHostProfile{RuntimeOverrides: map[string]map[string]any{
		"MarionetteSSE": {
			"eternal_dragonborn.development.presenter.profile": "skyrim-dev",
			"eternal_dragonborn.development.presenter.token":   "local-token",
		},
	}}
	path, err := writeSkyrimSessionBootstrapTransportConfig(root, profile)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "tspack-session-bootstrap") || !strings.Contains(string(data), "local-token") {
		t.Fatalf("controller config was not materialized: %q %v", data, err)
	}
}

func findCandidateByManualClassification(candidates []skyrimSaveCandidate) (skyrimSaveCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.LikelyManualSave {
			return candidate, true
		}
	}
	return skyrimSaveCandidate{}, false
}

func writeFixtureTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeFixtureProfile(t *testing.T, root string, saveDirectory string) {
	t.Helper()
	profileDirectory := filepath.Join(root, ".tspack")
	if err := os.MkdirAll(profileDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	profile := "[hosts.skyrim-dev]\n" +
		"gameRoot = " + tomlFixtureString(root) + "\n" +
		"dataDirectory = " + tomlFixtureString(root) + "\n" +
		"skseLauncher = " + tomlFixtureString(filepath.Join(root, "skse64_loader.exe")) + "\n" +
		"pluginState = " + tomlFixtureString(filepath.Join(root, "plugins.txt")) + "\n" +
		"runtimeLogDirectory = " + tomlFixtureString(root) + "\n" +
		"runtimeVersion = \"test\"\n" +
		"saveDirectory = " + tomlFixtureString(saveDirectory) + "\n"
	writeFixtureTestFile(t, filepath.Join(profileDirectory, "skyrim-hosts.toml"), profile)
}

func tomlFixtureString(value string) string {
	return "\"" + strings.ReplaceAll(value, "\\", "\\\\") + "\""
}

func TestSnapshotSkyrimFixtureFileRetainsTimestamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Save.ess")
	writeFixtureTestFile(t, path, "save")
	before := time.Now().Add(-time.Second).UTC()
	snapshot := snapshotSkyrimFixtureFile(path)
	if !snapshot.Present || snapshot.ModifiedTime.Before(before) {
		t.Fatalf("file snapshot did not retain metadata: %#v", snapshot)
	}
}
