package authoring

import (
	"encoding/json"
	"errors"
	"math/rand"
	"reflect"
	"testing"
)

func TestTapePreservesOrderedShadowChainAndReversedConceptOrder(t *testing.T) {
	declarations := []DependencyDeclaration{
		testDeclaration("concept-a", "^1", LayerConcept, 0, OriginConcept),
		testDeclaration("concept-b", "^2", LayerConcept, 1, OriginConcept),
		testDeclaration("user", "^3", LayerExplicit, 0, OriginExplicitUserOperation),
	}

	resolution := Build(declarations)
	if len(resolution.Entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(resolution.Entries))
	}
	for index, wantID := range []string{"concept-a", "concept-b", "user"} {
		if resolution.Entries[index].Declaration.ID != wantID {
			t.Fatalf("entry[%d].id = %q, want %q", index, resolution.Entries[index].Declaration.ID, wantID)
		}
	}
	if resolution.Entries[0].ShadowedBy == nil || resolution.Entries[0].ShadowedBy.Position != 1 {
		t.Fatalf("first shadow = %#v, want position 1", resolution.Entries[0].ShadowedBy)
	}
	if resolution.Entries[1].ShadowedBy == nil || resolution.Entries[1].ShadowedBy.Position != 2 {
		t.Fatalf("second shadow = %#v, want position 2", resolution.Entries[1].ShadowedBy)
	}
	if !resolution.Entries[2].Effective || resolution.Effective[0].Source.Range != "^3" {
		t.Fatalf("effective result = %#v", resolution)
	}

	reversed := Build([]DependencyDeclaration{
		testDeclaration("concept-b", "^2", LayerConcept, 0, OriginConcept),
		testDeclaration("concept-a", "^1", LayerConcept, 1, OriginConcept),
	})
	if got := reversed.Effective[0].Source.Range; got != "^1" {
		t.Fatalf("reversed concept winner range = %q, want ^1", got)
	}
}

func TestTapeDistinguishesPackageSources(t *testing.T) {
	npm := testDeclaration("npm-foo", "^1", LayerProject, 0, OriginProjectManifest)
	npm.Key = "npmFoo"
	other := testDeclaration("other-foo", "v1", LayerProject, 0, OriginProjectManifest)
	other.Key = "otherFoo"
	other.Source.Kind = "other"
	other.Identity = PackageIdentity{Source: "other", Name: "foo"}

	resolution := Build([]DependencyDeclaration{npm, other})
	if len(resolution.Effective) != 2 {
		t.Fatalf("effective dependencies = %#v, want two distinct source identities", resolution.Effective)
	}
	if HasFatalConflicts(resolution) {
		t.Fatalf("unexpected source identity conflict: %#v", resolution.Conflicts)
	}
}

func TestTapePreservesDistinctExplicitAliasesForSamePackage(t *testing.T) {
	first := testDeclaration("first", "^1", LayerProject, 0, OriginProjectManifest)
	first.Key = "foo-runtime"
	second := testDeclaration("second", "^1", LayerProject, 0, OriginProjectManifest)
	second.Key = "foo-build"
	second.Order = 1

	resolution := Build([]DependencyDeclaration{first, second})
	if len(resolution.Effective) != 2 {
		t.Fatalf("effective aliases = %#v, want both declarations", resolution.Effective)
	}
	if resolution.Entries[0].ShadowedBy != nil || resolution.Entries[1].ShadowedBy != nil {
		t.Fatalf("distinct aliases must not shadow: %#v", resolution.Entries)
	}
}

func TestTapeReportsCrossSourceEffectiveKeyCollision(t *testing.T) {
	npm := testDeclaration("npm-foo", "^1", LayerProject, 0, OriginProjectManifest)
	other := testDeclaration("other-foo", "v1", LayerProject, 0, OriginProjectManifest)
	other.Source.Kind = "other"
	other.Identity = PackageIdentity{Source: "other", Name: "foo"}

	resolution := Build([]DependencyDeclaration{npm, other})
	if !HasFatalConflicts(resolution) {
		t.Fatalf("expected fatal effective-key collision, got %#v", resolution.Conflicts)
	}
	if got := resolution.Conflicts[len(resolution.Conflicts)-1].Outcome; got != OutcomeKeyCollision {
		t.Fatalf("conflict outcome = %q", got)
	}
}

func TestRemoveUnshadowsPreviousDeclaration(t *testing.T) {
	ir := PackageIR{Declarations: []DependencyDeclaration{
		testDeclaration("concept-a", "^1", LayerConcept, 0, OriginConcept),
		testDeclaration("concept-b", "^2", LayerConcept, 1, OriginConcept),
		testDeclaration("user", "^3", LayerExplicit, 0, OriginExplicitUserOperation),
	}}

	result, err := Remove(ir, DeclarationSelector{ID: "user"})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.After.Effective[0].Source.Range; got != "^2" {
		t.Fatalf("winner after removing user = %q, want ^2", got)
	}
	if !hasChange(result.Changes, ChangeUnshadowed, "concept-b") {
		t.Fatalf("changes do not report concept-b unshadow: %#v", result.Changes)
	}

	remaining := PackageIR{Declarations: declarationsFromEntries(result.After.Entries)}
	result, err = Remove(remaining, DeclarationSelector{ID: "concept-b"})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.After.Effective[0].Source.Range; got != "^1" {
		t.Fatalf("winner after removing concept-b = %q, want ^1", got)
	}
}

func TestRemoveUnshadowsEquivalentDeclarationByProvenance(t *testing.T) {
	concept := testDeclaration("concept", "^4", LayerConcept, 0, OriginConcept)
	explicit := testDeclaration("explicit", "^4", LayerExplicit, 0, OriginExplicitUserOperation)

	result, err := Remove(
		PackageIR{Declarations: []DependencyDeclaration{concept, explicit}},
		DeclarationSelector{ID: "explicit", EditableOnly: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !hasChange(result.Changes, ChangeUnshadowed, "concept") {
		t.Fatalf("equivalent declaration provenance was not unshadowed: %#v", result.Changes)
	}
	if len(result.After.Effective) != 1 || result.After.Effective[0].Source.Range != "^4" {
		t.Fatalf("effective value changed while provenance unshadowed: %#v", result.After.Effective)
	}
}

func TestAddDefaultsToExplicitEditableUserLayer(t *testing.T) {
	concept := testDeclaration("concept", "^1", LayerConcept, 0, OriginConcept)
	added := DependencyDeclaration{
		ID:         "added",
		Identity:   PackageIdentity{Source: "npm", Name: "foo"},
		Source:     PackageSource{Kind: "npm", Package: "foo", Range: "^2"},
		Constraint: "^2",
		Kind:       DependencyRuntime,
	}

	result := Add(PackageIR{Declarations: []DependencyDeclaration{concept}}, added)
	winner := result.After.Entries[len(result.After.Entries)-1].Declaration
	if winner.Layer != LayerExplicit || winner.Origin.Kind != OriginExplicitUserOperation {
		t.Fatalf("added declaration provenance = %#v", winner)
	}
	if winner.Authority != AuthorityOwned || winner.Editability != EditabilityEditable {
		t.Fatalf("added declaration editability = %#v", winner)
	}
	if !hasChange(result.Changes, ChangeShadowed, "concept") {
		t.Fatalf("add changes = %#v, want concept shadow explanation", result.Changes)
	}
}

func TestRemoveSurfacesAmbiguityAndEditability(t *testing.T) {
	first := testDeclaration("one", "^1", LayerProject, 0, OriginProjectManifest)
	second := first
	second.ID = "two"
	second.Kind = DependencyTool
	second.Editability = EditabilityObserved
	ir := PackageIR{Declarations: []DependencyDeclaration{first, second}}
	identity := PackageIdentity{Source: "npm", Name: "foo"}

	_, err := Remove(ir, DeclarationSelector{Identity: &identity})
	var ambiguous AmbiguousRemovalError
	if !errors.As(err, &ambiguous) || len(ambiguous.Matches) != 2 {
		t.Fatalf("remove error = %#v, want two-way ambiguity", err)
	}

	result, err := Remove(ir, DeclarationSelector{Identity: &identity, EditableOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.After.Entries) != 1 || result.After.Entries[0].Declaration.ID != "two" {
		t.Fatalf("editable-only removal result = %#v", result.After.Entries)
	}
}

func TestTapeIsDeterministicAcrossInputMapIteration(t *testing.T) {
	base := map[string]DependencyDeclaration{
		"a": testDeclaration("a", "^1", LayerConcept, 0, OriginConcept),
		"b": testDeclaration("b", "^2", LayerConcept, 1, OriginConcept),
		"c": testDeclaration("c", "^3", LayerExplicit, 0, OriginExplicitUserOperation),
	}
	var expected []byte
	for iteration := 0; iteration < 50; iteration++ {
		keys := []string{"a", "b", "c"}
		rand.New(rand.NewSource(int64(iteration))).Shuffle(len(keys), func(left, right int) {
			keys[left], keys[right] = keys[right], keys[left]
		})
		declarations := make([]DependencyDeclaration, 0, len(keys))
		for _, key := range keys {
			declarations = append(declarations, base[key])
		}
		encoded, err := json.Marshal(Build(declarations))
		if err != nil {
			t.Fatal(err)
		}
		if expected == nil {
			expected = encoded
			continue
		}
		if !reflect.DeepEqual(encoded, expected) {
			t.Fatalf("iteration %d produced nondeterministic tape\nfirst: %s\n got: %s", iteration, expected, encoded)
		}
	}
}

func BenchmarkBuildDependencyTape(b *testing.B) {
	declarations := make([]DependencyDeclaration, 0, 100)
	for index := 0; index < 100; index++ {
		declaration := testDeclaration("", "^1", LayerProject, 0, OriginProjectManifest)
		declaration.ID = "dependency"
		declaration.Key = "dependency"
		declaration.Source.Package = "dependency"
		declaration.Identity.Name = "dependency"
		declaration.Order = index
		declarations = append(declarations, declaration)
	}
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		_ = Build(declarations)
	}
}

func testDeclaration(id string, constraint string, layer DeclarationLayer, order int, origin OriginKind) DependencyDeclaration {
	return DependencyDeclaration{
		ID:          id,
		Identity:    PackageIdentity{Source: "npm", Name: "foo"},
		Source:      PackageSource{Kind: "npm", Package: "foo", Range: constraint},
		Constraint:  constraint,
		Kind:        DependencyRuntime,
		Origin:      DeclarationOrigin{Kind: origin, Name: id},
		Layer:       layer,
		LayerOrder:  order,
		Order:       0,
		Authority:   AuthorityOwned,
		Editability: EditabilityEditable,
	}
}

func declarationsFromEntries(entries []TapeEntry) []DependencyDeclaration {
	declarations := make([]DependencyDeclaration, 0, len(entries))
	for _, entry := range entries {
		declarations = append(declarations, entry.Declaration)
	}
	return declarations
}

func hasChange(changes []AuthoringChange, kind ChangeKind, id string) bool {
	for _, change := range changes {
		if change.Kind == kind && change.Declaration.ID == id {
			return true
		}
	}
	return false
}
