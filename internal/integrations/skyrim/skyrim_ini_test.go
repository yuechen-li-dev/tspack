package skyrim

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func TestSkyrimINIPlanStagesDeclaredAlwaysActiveOverrideWithoutMutatingSource(t *testing.T) {
	root := t.TempDir()
	saveDirectory := filepath.Join(root, "Documents", "Saves")
	if err := os.MkdirAll(saveDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	iniPath := filepath.Join(filepath.Dir(saveDirectory), "Skyrim.ini")
	original := []byte("; preserve this comment\r\n[General]\r\nsLanguage=ENGLISH\r\nbAlwaysActive=0\r\n[Display]\r\niPresentInterval=1\r\n")
	if err := os.WriteFile(iniPath, original, 0o644); err != nil {
		t.Fatal(err)
	}
	profile := skyrimHostProfile{
		SaveDirectory: saveDirectory,
		INIOverrides: map[string]any{
			skyrimAlwaysActiveOverridePath: true,
		},
	}
	target := skyrimTargetRef{Target: manifestSkyrimTargetForINIOverrideTests()}
	first, err := buildSkyrimINIPlan(root, target, profile, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := buildSkyrimINIPlan(root, target, profile, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.PathSource != "save_directory_parent" || first.EffectiveSHA256 != second.EffectiveSHA256 || first.SourceValue() != false {
		t.Fatalf("unexpected deterministic dry-run plan: %#v %#v", first, second)
	}
	if _, err := os.Stat(first.StagedPath); !os.IsNotExist(err) {
		t.Fatal("dry-run wrote a staged INI")
	}
	if after, _ := os.ReadFile(iniPath); !bytes.Equal(after, original) {
		t.Fatal("INI planning mutated the original file")
	}
	staged, err := buildSkyrimINIPlan(root, target, profile, true)
	if err != nil {
		t.Fatal(err)
	}
	stagedBytes, err := os.ReadFile(staged.StagedPath)
	if err != nil || !strings.Contains(string(stagedBytes), "bAlwaysActive=1\r\n") || !strings.Contains(string(stagedBytes), "iPresentInterval=1\r\n") {
		t.Fatalf("staged INI did not preserve unrelated content: %q %v", stagedBytes, err)
	}
}

func TestSkyrimAlwaysActiveOverrideRejectsMalformedOrAmbiguousINI(t *testing.T) {
	tests := map[string]string{
		"missing General": "[Display]\niPresentInterval=1\n",
		"duplicate key":   "[General]\nbAlwaysActive=0\nbAlwaysActive=1\n",
		"malformed":       "[General]\nnot an assignment\n",
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			if _, _, err := applySkyrimAlwaysActiveOverride([]byte(source), true); err == nil {
				t.Fatal("expected INI rejection")
			}
		})
	}
}

func TestSkyrimAlwaysActiveOverrideAddsMissingKeyAndLeavesTrueUnchanged(t *testing.T) {
	missing, sourceValue, err := applySkyrimAlwaysActiveOverride([]byte("[General]\nsLanguage=ENGLISH\n[Display]\niPresentInterval=1\n"), true)
	if err != nil || sourceValue != nil || string(missing) != "[General]\nsLanguage=ENGLISH\nbAlwaysActive=1\n[Display]\niPresentInterval=1\n" {
		t.Fatalf("missing key result=%q source=%#v err=%v", missing, sourceValue, err)
	}
	alreadyTrue, sourceValue, err := applySkyrimAlwaysActiveOverride([]byte("[General]\nbAlwaysActive=1\n"), true)
	if err != nil || sourceValue != true || string(alreadyTrue) != "[General]\nbAlwaysActive=1\n" {
		t.Fatalf("existing true result=%q source=%#v err=%v", alreadyTrue, sourceValue, err)
	}
}

func TestSkyrimINIOverridesRejectUnknownAndWrongTypes(t *testing.T) {
	fields := manifestSkyrimTargetForINIOverrideTests().INIOverrideFields
	for name, values := range map[string]map[string]any{
		"unknown":      {"Display.iPresentInterval": true},
		"wrong type":   {skyrimAlwaysActiveOverridePath: "true"},
		"two settings": {skyrimAlwaysActiveOverridePath: true, "General.other": false},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateSkyrimINIOverrides(fields, values); err == nil {
				t.Fatal("expected rejected INI override")
			}
		})
	}
	if err := validateSkyrimINIOverrides([]manifest.SkyrimINIOverride{{Section: "Display", Key: "iPresentInterval", Type: "boolean"}}, map[string]any{skyrimAlwaysActiveOverridePath: true}); err == nil {
		t.Fatal("non-Skyrim allowlist unexpectedly accepted")
	}
}

func TestSkyrimINIPathPrefersProfileAndRejectsMissing(t *testing.T) {
	root := t.TempDir()
	iniPath := filepath.Join(root, "profile.ini")
	if err := os.WriteFile(iniPath, []byte("[General]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, source, err := resolveSkyrimINIPath(skyrimHostProfile{INIPath: iniPath})
	if err != nil || path != iniPath || source != "profile" {
		t.Fatalf("profile INI resolution path=%q source=%q err=%v", path, source, err)
	}
	if _, _, err := resolveSkyrimINIPath(skyrimHostProfile{INIPath: filepath.Join(root, "missing.ini")}); err == nil {
		t.Fatal("missing profile INI was accepted")
	}
}

func TestSkyrimINIRestorationReturnsOriginalBytes(t *testing.T) {
	root := t.TempDir()
	saveDirectory := filepath.Join(root, "My Games", "Saves")
	if err := os.MkdirAll(saveDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	iniPath := filepath.Join(filepath.Dir(saveDirectory), "Skyrim.ini")
	original := []byte("[General]\r\nbAlwaysActive=0\r\n")
	if err := os.WriteFile(iniPath, original, 0o644); err != nil {
		t.Fatal(err)
	}
	iniPlan, err := buildSkyrimINIPlan(root, skyrimTargetRef{Target: manifestSkyrimTargetForINIOverrideTests()}, skyrimHostProfile{SaveDirectory: saveDirectory, INIOverrides: map[string]any{skyrimAlwaysActiveOverridePath: true}}, true)
	if err != nil {
		t.Fatal(err)
	}
	plan := skyrimPlanReport{
		ManifestSHA256: "ini-restoration",
		INI:            iniPlan,
		Files: []skyrimFilePlan{{
			Kind:               "ini",
			Source:             iniPath,
			MaterializedSource: iniPlan.StagedPath,
			Destination:        iniPath,
			SourceSHA256:       iniPlan.EffectiveSHA256,
			CurrentSHA256:      iniPlan.SourceSHA256,
			Action:             "replace",
		}},
	}
	report, err := materializeSkyrimPlan(root, plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := restoreSkyrimINI(plan, report); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(iniPath)
	if err != nil || !bytes.Equal(restored, original) {
		t.Fatalf("INI restoration was not byte exact: %q %v", restored, err)
	}
}

func manifestSkyrimTargetForINIOverrideTests() manifest.SkyrimTarget {
	return manifest.SkyrimTarget{
		INIOverrideFields: []manifest.SkyrimINIOverride{{
			Section: "General",
			Key:     "bAlwaysActive",
			Type:    "boolean",
		}},
	}
}

func (plan skyrimINIPlan) SourceValue() any {
	if len(plan.AppliedOverrides) == 0 {
		return nil
	}
	return plan.AppliedOverrides[0].SourceValue
}
