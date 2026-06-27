package concepts

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuiltinRegistry(t *testing.T) {
	names := BuiltinNames()
	expected := []string{"browser.spa", "browser.static", "package.exports", "package.peerDependencies", "react.app", "react.library", "tspack.manifestBoundary", "tspack.pack", "tspack.securityPolicy", "tspack.updatePolicy", "tspack.workspace", "typescript.app", "typescript.library", "vite.app", "vite.library"}
	if !reflect.DeepEqual(names, expected) {
		t.Fatalf("names mismatch\nwant %#v\ngot  %#v", expected, names)
	}
	if _, ok := Lookup("vite.app"); !ok {
		t.Fatal("expected vite.app lookup to work")
	}
}

func TestResolveRequiredInsertedBeforeDependent(t *testing.T) {
	result, err := Resolve([]string{"vite.app"}, "app")
	if err != nil {
		t.Fatal(err)
	}
	if got := fragmentNames(result.Fragments); !reflect.DeepEqual(got, []string{"typescript.app", "vite.app"}) {
		t.Fatalf("unexpected order: %#v", got)
	}
}

func TestResolveUnknownConflictCycleAndKind(t *testing.T) {
	if _, err := Resolve([]string{"missing.concept"}, "app"); err == nil || !strings.Contains(err.Error(), "unknown concept") {
		t.Fatalf("expected unknown error, got %v", err)
	}
	registry := NewRegistry([]Fragment{{Name: "a", Conflicts: []string{"b"}}, {Name: "b"}, {Name: "c", Requires: []string{"d"}}, {Name: "d", Requires: []string{"c"}}, {Name: "lib", CompatibleKinds: []string{"library"}}})
	if _, err := ResolveWithRegistry(registry, []string{"a", "b"}, ""); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("expected conflict, got %v", err)
	}
	if _, err := ResolveWithRegistry(registry, []string{"c"}, ""); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle, got %v", err)
	}
	if _, err := ResolveWithRegistry(registry, []string{"lib"}, "app"); err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("expected incompatible kind, got %v", err)
	}
}

func TestMergeBuiltinAppFragments(t *testing.T) {
	ir := mustBuild(t, []string{"typescript.app", "vite.app", "react.app"}, "app")
	assertDep(t, ir.Manifest.Tools, "typescript")
	assertDep(t, ir.Manifest.Tools, "vite")
	assertDep(t, ir.Manifest.Dependencies, "react")
	assertRunTarget(t, ir.Manifest.RunTargets, "dev")
	assertRunTarget(t, ir.Manifest.RunTargets, "build")
}

func TestMergeDuplicateAndConflicts(t *testing.T) {
	duplicate := []Fragment{{Name: "a", Manifest: ManifestContributions{Dependencies: []DependencyContribution{dep("x", "^1")}}}, {Name: "b", Manifest: ManifestContributions{Dependencies: []DependencyContribution{dep("x", "^1")}}}}
	if _, err := Merge(duplicate); err != nil {
		t.Fatalf("identical duplicate should merge: %v", err)
	}
	cases := [][]Fragment{
		{{Name: "a", Manifest: ManifestContributions{Dependencies: []DependencyContribution{dep("x", "^1")}}}, {Name: "b", Manifest: ManifestContributions{Dependencies: []DependencyContribution{dep("x", "^2")}}}},
		{{Name: "a", Manifest: ManifestContributions{Dependencies: []DependencyContribution{dep("react", "^18")}}}, {Name: "b", Manifest: ManifestContributions{Peers: []DependencyContribution{dep("react", "^18")}}}},
		{{Name: "a", Manifest: ManifestContributions{RunTargets: []RunTargetContribution{{Name: "build", Command: "a"}}}}, {Name: "b", Manifest: ManifestContributions{RunTargets: []RunTargetContribution{{Name: "build", Command: "b"}}}}},
		{{Name: "a", Manifest: ManifestContributions{Env: []EnvContribution{{Name: "PORT", Default: "3000"}}}}, {Name: "b", Manifest: ManifestContributions{Env: []EnvContribution{{Name: "PORT", Default: "4000"}}}}},
		{{Name: "a", Manifest: ManifestContributions{Services: []ServiceContribution{{Name: "db", Endpoint: "a"}}}}, {Name: "b", Manifest: ManifestContributions{Services: []ServiceContribution{{Name: "db", Endpoint: "b"}}}}},
		{{Name: "a", Files: []FileContribution{{Path: "src/main.ts", Content: "a"}}}, {Name: "b", Files: []FileContribution{{Path: "src/main.ts", Content: "b"}}}},
		{{Name: "a", Slots: []SlotContribution{{Name: "app.entry", Mode: SlotSingleOwner, Owner: "a"}}}, {Name: "b", Slots: []SlotContribution{{Name: "app.entry", Mode: SlotSingleOwner, Owner: "b"}}}},
		{{Name: "a", Manifest: ManifestContributions{UpdatePolicy: []PolicyContribution{{Subject: "react", Action: "pin", Range: "minor"}}}}, {Name: "b", Manifest: ManifestContributions{UpdatePolicy: []PolicyContribution{{Subject: "react", Action: "float", Range: "major"}}}}},
	}
	for _, fragments := range cases {
		if _, err := Merge(fragments); err == nil {
			t.Fatalf("expected conflict for %#v", fragments)
		}
	}
}

func TestAppendSlotOrderingAndProjectionConflict(t *testing.T) {
	ir, err := Merge([]Fragment{{Name: "a", Slots: []SlotContribution{{Name: "readme.sections", Mode: SlotAppendOrdered, Values: []string{"a"}}}}, {Name: "b", Slots: []SlotContribution{{Name: "readme.sections", Mode: SlotAppendOrdered, Values: []string{"b"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{ir.Slots[0].Values[0], ir.Slots[1].Values[0]}; !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("slot order mismatch: %#v", got)
	}
	_, err = Merge([]Fragment{{Name: "a", Projections: ProjectionContributions{Objects: map[string]map[string]string{"tsconfig": {"jsx": "react-jsx"}}}}, {Name: "b", Projections: ProjectionContributions{Objects: map[string]map[string]string{"tsconfig": {"jsx": "preserve"}}}}})
	if err == nil {
		t.Fatal("expected projection scalar conflict")
	}
}

func TestCurrentTemplateConceptCompositionsResolve(t *testing.T) {
	static := mustBuild(t, []string{"tspack.workspace", "tspack.manifestBoundary", "tspack.securityPolicy", "tspack.updatePolicy", "typescript.app", "vite.app", "browser.static"}, "app")
	assertDep(t, static.Manifest.Tools, "typescript")
	assertDep(t, static.Manifest.Tools, "vite")
	assertDep(t, static.Manifest.Tools, "@biomejs/biome")
	assertTarget(t, static.Manifest.Targets, "app")
	react := mustBuild(t, []string{"tspack.workspace", "tspack.manifestBoundary", "tspack.securityPolicy", "tspack.updatePolicy", "typescript.app", "vite.app", "react.app", "browser.spa"}, "app")
	assertDep(t, react.Manifest.Dependencies, "react")
	library := mustBuild(t, []string{"tspack.workspace", "tspack.manifestBoundary", "tspack.securityPolicy", "tspack.updatePolicy", "tspack.pack", "typescript.library", "vite.library", "react.library", "package.exports", "package.peerDependencies"}, "library")
	assertDep(t, library.Manifest.Peers, "react")
	if library.Manifest.Pack == nil {
		t.Fatal("expected pack contribution")
	}
	if _, ok := library.Projections.Objects["package.exports"]; !ok {
		t.Fatal("expected package exports projection")
	}
}

func mustBuild(t *testing.T, names []string, kind string) *MergedConceptIR {
	t.Helper()
	ir, err := BuildConceptIR(names, kind, Builtins)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	return ir
}
func fragmentNames(fragments []Fragment) []string {
	names := []string{}
	for _, fragment := range fragments {
		names = append(names, fragment.Name)
	}
	return names
}
func assertDep(t *testing.T, deps []DependencyContribution, name string) {
	t.Helper()
	for _, dep := range deps {
		if dep.Name == name {
			return
		}
	}
	t.Fatalf("missing dependency %s in %#v", name, deps)
}
func assertRunTarget(t *testing.T, targets []RunTargetContribution, name string) {
	t.Helper()
	for _, target := range targets {
		if target.Name == name {
			return
		}
	}
	t.Fatalf("missing run target %s in %#v", name, targets)
}

func assertTarget(t *testing.T, targets []TargetContribution, name string) {
	t.Helper()
	for _, target := range targets {
		if target.Name == name {
			return
		}
	}
	t.Fatalf("missing target %s in %#v", name, targets)
}
