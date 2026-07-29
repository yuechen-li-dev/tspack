package concepts

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestBuiltinRegistry(t *testing.T) {
	names := BuiltinNames()
	expected := append([]string(nil), generatedBuiltinNames...)
	if !sort.StringsAreSorted(expected) {
		sort.Strings(expected)
	}
	if !reflect.DeepEqual(names, expected) {
		t.Fatalf("names mismatch\nwant %#v\ngot  %#v", expected, names)
	}
	if _, ok := Lookup("vite.app"); !ok {
		t.Fatal("expected vite.app lookup to work")
	}
}

func TestGeneratedBuiltinCompositionContracts(t *testing.T) {
	for _, expected := range generatedBuiltinCompositionCases {
		t.Run(expected.Name, func(t *testing.T) {
			ir, err := BuildConceptIR(expected.Concepts, expected.Kind, Builtins)
			if err != nil {
				t.Fatalf("BuildConceptIR: %v", err)
			}
			for _, name := range expected.Dependencies {
				assertDep(t, ir.Manifest.Dependencies, name)
			}
			for _, name := range expected.Tools {
				assertDep(t, ir.Manifest.Tools, name)
			}
			for _, name := range expected.Peers {
				assertDep(t, ir.Manifest.Peers, name)
			}
			for _, name := range expected.Targets {
				assertTarget(t, ir.Manifest.Targets, name)
			}
			for _, name := range expected.RunTargets {
				assertRunTarget(t, ir.Manifest.RunTargets, name)
			}
			if (ir.Manifest.Pack != nil) != expected.HasPack {
				t.Fatalf("pack contribution = %t, want %t", ir.Manifest.Pack != nil, expected.HasPack)
			}
			for _, projection := range expected.Projections {
				parts := strings.SplitN(projection, ":", 2)
				if len(parts) != 2 || ir.Projections.Objects[parts[0]][parts[1]] == "" {
					t.Fatalf("missing projection %q in %#v", projection, ir.Projections.Objects)
				}
			}
		})
	}
}

func TestResolveDoesNotInsertExpectedConcepts(t *testing.T) {
	if _, err := Resolve([]string{"vite.app"}, "app"); err == nil || !strings.Contains(err.Error(), "expects \"typescript.app\"") {
		t.Fatalf("expected missing expected concept error, got %v", err)
	}
	result, err := Resolve([]string{"vite.app", "typescript.app"}, "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Inserted) != 0 {
		t.Fatalf("expected no inserted concepts, got %#v", result.Inserted)
	}
	if got := fragmentNames(result.Fragments); !reflect.DeepEqual(got, []string{"vite.app", "typescript.app"}) {
		t.Fatalf("expected listed order to be preserved, got %#v", got)
	}
}

func TestResolveUnknownConflictCycleAndKind(t *testing.T) {
	if _, err := Resolve([]string{"missing.concept"}, "app"); err == nil || !strings.Contains(err.Error(), "unknown concept") {
		t.Fatalf("expected unknown error, got %v", err)
	}
	registry := NewRegistry([]Fragment{{Name: "a", Conflicts: []string{"b"}}, {Name: "b"}, {Name: "c", RequiresAnyOf: [][]string{{"d", "e"}}}, {Name: "d"}, {Name: "lib", CompatibleKinds: []string{"library"}}})
	if _, err := ResolveWithRegistry(registry, []string{"a", "b"}, ""); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("expected conflict, got %v", err)
	}
	if _, err := ResolveWithRegistry(registry, []string{"c"}, ""); err == nil || !strings.Contains(err.Error(), "expects one of") {
		t.Fatalf("expected any-of validation error, got %v", err)
	}
	if _, err := ResolveWithRegistry(registry, []string{"lib"}, "app"); err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("expected incompatible kind, got %v", err)
	}
}

func TestMergeBuiltinAppFragments(t *testing.T) {
	ir := mustBuild(t, []string{"react.app", "browser.spa", "vite.app", "typescript.app"}, "app")
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

func TestConflictDiagnosticIncludesPriorityAndPath(t *testing.T) {
	_, err := Merge([]Fragment{
		{Name: "higher", Manifest: ManifestContributions{Dependencies: []DependencyContribution{dep("x", "^1")}}},
		{Name: "lower", Manifest: ManifestContributions{Dependencies: []DependencyContribution{dep("x", "^2")}}},
	})
	if err == nil {
		t.Fatal("expected conflict")
	}
	conflict, ok := err.(Conflict)
	if !ok {
		t.Fatalf("expected Conflict, got %T", err)
	}
	if conflict.Path != "manifest.dependencies.x" {
		t.Fatalf("unexpected conflict path: %s", conflict.Path)
	}
	if conflict.HigherPriorityConcept != "higher" || conflict.LowerPriorityConcept != "lower" {
		t.Fatalf("unexpected priority metadata: %#v", conflict)
	}
	if !strings.Contains(conflict.Error(), "cannot be resolved by priority") {
		t.Fatalf("expected priority failure reason, got %q", conflict.Error())
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

func TestReactLibraryMissingCompanionConceptsFail(t *testing.T) {
	base := []string{
		"react.library",
		"package.peerDependencies",
		"package.exports",
		"tspack.pack",
		"vite.library",
		"typescript.library",
		"tspack.workspace",
		"tspack.manifestBoundary",
		"tspack.updatePolicy",
		"tspack.securityPolicy",
	}
	missing := []string{"package.peerDependencies", "package.exports", "tspack.pack", "vite.library", "typescript.library"}
	for _, removed := range missing {
		stack := removeConcept(base, removed)
		_, err := BuildConceptIR(stack, "library", Builtins)
		if err == nil {
			t.Fatalf("expected missing %s to fail", removed)
		}
		if !strings.Contains(err.Error(), "expects") {
			t.Fatalf("expected explicit companion diagnostic for %s, got %v", removed, err)
		}
	}
}

func TestReactLibraryConflictsWithBrowserSPAAndHasNoAppTarget(t *testing.T) {
	_, err := Resolve([]string{"react.library", "package.peerDependencies", "package.exports", "tspack.pack", "vite.library", "typescript.library", "browser.spa"}, "library")
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("expected react.library/browser.spa conflict, got %v", err)
	}
	ir := mustBuild(t, []string{"react.library", "package.peerDependencies", "package.exports", "tspack.pack", "vite.library", "typescript.library"}, "library")
	for _, target := range ir.Manifest.Targets {
		if target.Name == "app" || target.Entry == "src/main.tsx" {
			t.Fatalf("react.library contributed browser app target: %#v", target)
		}
	}
}

func removeConcept(concepts []string, removed string) []string {
	result := []string{}
	for _, concept := range concepts {
		if concept != removed {
			result = append(result, concept)
		}
	}
	return result
}
