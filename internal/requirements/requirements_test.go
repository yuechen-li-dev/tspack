package requirements

import (
	"fmt"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/packageidentity"
)

func TestTapeUsesExplicitPrecedenceAndClassifiesLosers(t *testing.T) {
	identity := packageidentity.PackageIdentity{Source: "npm", Name: "react"}
	tape := Build([]PackageRequirement{
		{ID: "project", Target: identity, Constraint: "^19.1", Kind: KindProjectExplicit, Order: 0},
		{ID: "new-widget", Target: identity, Constraint: "^19", Kind: KindPeer, Order: 20},
		{ID: "old-widget", Target: identity, Constraint: "^18", Kind: KindPeer, Order: 10},
	})
	controllers := tape.Controllers()
	if len(controllers) != 1 || controllers[0].ID != "project" {
		t.Fatalf("controllers = %#v, want project", controllers)
	}

	classified := tape.Classify(map[string]string{"workspace|npm:react": "19.1.0"})
	statuses := map[string]Status{}
	for _, entry := range classified.Entries {
		statuses[entry.Requirement.ID] = entry.Status
	}
	if statuses["old-widget"] != StatusOverriddenIncompatible {
		t.Fatalf("old-widget status = %q", statuses["old-widget"])
	}
	if statuses["new-widget"] != StatusShadowedCompatible {
		t.Fatalf("new-widget status = %q", statuses["new-widget"])
	}
	if statuses["project"] != StatusControlling {
		t.Fatalf("project status = %q", statuses["project"])
	}
}

func TestTapeLaterEntryWinsWithinLayerWithoutInputTraversalPolicy(t *testing.T) {
	identity := packageidentity.PackageIdentity{Source: "npm", Name: "react"}
	input := []PackageRequirement{
		{ID: "later", Target: identity, Constraint: "^19", Kind: KindPeer, Order: 20},
		{ID: "earlier", Target: identity, Constraint: "^18", Kind: KindPeer, Order: 10},
	}
	if got := Build(input).Controllers()[0].ID; got != "later" {
		t.Fatalf("controller = %q, want later", got)
	}
	input[0], input[1] = input[1], input[0]
	if got := Build(input).Controllers()[0].ID; got != "later" {
		t.Fatalf("controller after reversed discovery = %q, want later", got)
	}
}

func TestTapeKeepsSourcesInDistinctSlots(t *testing.T) {
	tape := Build([]PackageRequirement{
		{ID: "npm", Target: packageidentity.PackageIdentity{Source: "npm", Name: "foo"}, Constraint: "1.0.0", Kind: KindPeer},
		{ID: "jsr", Target: packageidentity.PackageIdentity{Source: "jsr", Name: "foo"}, Constraint: "1.0.0", Kind: KindPeer},
	})
	if len(tape.Controllers()) != 2 {
		t.Fatalf("controllers = %#v, want distinct source-qualified slots", tape.Controllers())
	}
}

func BenchmarkTape(b *testing.B) {
	for _, count := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("requirements-%d", count), func(b *testing.B) {
			input := make([]PackageRequirement, count)
			for index := range input {
				input[index] = PackageRequirement{
					ID:         fmt.Sprintf("requirement-%d", index),
					Target:     packageidentity.PackageIdentity{Source: "npm", Name: fmt.Sprintf("package-%d", index%100)},
					Constraint: "^1.0.0",
					Kind:       KindPeer,
					Order:      index,
				}
			}
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				tape := Build(input)
				_ = tape.Classify(map[string]string{})
			}
		})
	}
}
