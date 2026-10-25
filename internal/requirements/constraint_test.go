package requirements

import (
	"testing"

	semver "github.com/Masterminds/semver/v3"
)

func TestRegistryConstraintAlternatives(t *testing.T) {
	for _, test := range []struct {
		name, source, expression, version string
		invalid, satisfies                bool
	}{
		{name: "npm loose peer", source: "npm", expression: ">=3.0.0 || insiders", version: "3.4.17", satisfies: true},
		{name: "reverse alternatives", source: "npm", expression: "insiders || >=3.0.0", version: "3.4.17", satisfies: true},
		{name: "lower bound preserved", source: "npm", expression: ">=3.0.0 || insiders", version: "2.9.0"},
		{name: "prerelease excluded", source: "npm", expression: ">=3.0.0 || insiders", version: "4.0.0-beta.1"},
		{name: "valid prerelease", source: "npm", expression: "insiders || 4.0.0-beta.1", version: "4.0.0-beta.1", satisfies: true},
		{name: "all invalid", source: "npm", expression: "insiders || latest", invalid: true},
		{name: "tag alone", source: "npm", expression: "insiders", invalid: true},
		{name: "jsr remains strict", source: "jsr", expression: ">=3.0.0 || insiders", invalid: true},
		{name: "valid jsr union", source: "jsr", expression: "^2.0.0 || ^3.0.0", version: "3.4.17", satisfies: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			constraint, err := ParseConstraint(test.source, test.expression)
			if (err != nil) != test.invalid {
				t.Fatalf("ParseConstraint(%q, %q): %v", test.source, test.expression, err)
			}
			if test.invalid {
				return
			}
			if got := constraint.Check(semver.MustParse(test.version)); got != test.satisfies {
				t.Fatalf("%q satisfies %q = %t, want %t", test.version, test.expression, got, test.satisfies)
			}
		})
	}
}

func TestLooseNPMConstraintClassificationPreservesAuthoredEvidence(t *testing.T) {
	requirement := PackageRequirement{Constraint: ">=3.0.0 || insiders", Kind: KindPeer}
	requirement.Target.Source = "npm"
	requirement.Target.Name = "tailwindcss"
	tape := Build([]PackageRequirement{requirement}).Classify(map[string]string{SlotKey(requirement): "3.4.17"})
	if entry := tape.Entries[0]; entry.Status != StatusControlling || entry.Requirement.Constraint != requirement.Constraint {
		t.Fatalf("classification lost accepted range or authored evidence: %#v", entry)
	}
}
