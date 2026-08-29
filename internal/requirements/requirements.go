// Package requirements defines deterministic shared-environment package
// requirements. It deliberately does not resolve package versions: it orders
// evidence, selects one controlling requirement per source-qualified slot, and
// classifies every entry once the resolver supplies the selected version.
package requirements

import (
	"fmt"
	"sort"
	"strings"

	semver "github.com/Masterminds/semver/v3"
	"github.com/yuechen-li-dev/tspack/internal/packageidentity"
)

type Kind string

const (
	KindTransitiveRuntime  Kind = "transitive-runtime"
	KindTransitiveOptional Kind = "transitive-optional"
	KindPeer               Kind = "peer"
	KindPackageExplicit    Kind = "package-explicit"
	KindProjectExplicit    Kind = "project-explicit"
	KindTargetExplicit     Kind = "target-explicit"
	KindOverride           Kind = "override"
)

type Scope struct {
	Environment string `json:"environment"`
}

func (scope Scope) Key() string {
	if strings.TrimSpace(scope.Environment) == "" {
		return "workspace"
	}
	return scope.Environment
}

type Origin struct {
	RequiringPackage string `json:"requiringPackage,omitempty"`
	PackageID        string `json:"packageId,omitempty"`
	DependencyKey    string `json:"dependencyKey,omitempty"`
	Source           string `json:"source,omitempty"`
}

type PackageRequirement struct {
	ID         string                          `json:"id"`
	Target     packageidentity.PackageIdentity `json:"target"`
	Reference  string                          `json:"reference,omitempty"`
	Constraint string                          `json:"constraint"`
	Kind       Kind                            `json:"kind"`
	Optional   bool                            `json:"optional,omitempty"`
	Origin     Origin                          `json:"origin"`
	Scope      Scope                           `json:"scope"`
	Order      int                             `json:"order"`
}

type Status string

const (
	StatusPending                Status = "pending"
	StatusControlling            Status = "controlling"
	StatusSatisfied              Status = "satisfied"
	StatusShadowedCompatible     Status = "shadowed-compatible"
	StatusOverriddenIncompatible Status = "overridden-incompatible"
	StatusOptionalUnsatisfied    Status = "optional-unsatisfied"
	StatusInvalid                Status = "invalid"
)

type TapeRef struct {
	Position int    `json:"position"`
	ID       string `json:"id"`
}

type Entry struct {
	Requirement     PackageRequirement `json:"requirement"`
	Position        int                `json:"position"`
	Controlling     bool               `json:"controlling"`
	ShadowedBy      *TapeRef           `json:"shadowedBy,omitempty"`
	Status          Status             `json:"status"`
	SelectedVersion string             `json:"selectedVersion,omitempty"`
}

type Tape struct {
	Entries []Entry `json:"entries"`
}

func Build(input []PackageRequirement) Tape {
	ordered := append([]PackageRequirement(nil), input...)
	for index := range ordered {
		normalize(&ordered[index], index)
	}
	sort.SliceStable(ordered, func(left, right int) bool {
		leftRank := kindRank(ordered[left].Kind)
		rightRank := kindRank(ordered[right].Kind)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if ordered[left].Order != ordered[right].Order {
			return ordered[left].Order < ordered[right].Order
		}
		return ordered[left].ID < ordered[right].ID
	})

	tape := Tape{Entries: make([]Entry, len(ordered))}
	latestBySlot := map[string]int{}
	for position, requirement := range ordered {
		tape.Entries[position] = Entry{
			Requirement: requirement,
			Position:    position,
			Controlling: true,
			Status:      StatusPending,
		}
		slot := SlotKey(requirement)
		if earlier, ok := latestBySlot[slot]; ok {
			controller := TapeRef{Position: position, ID: requirement.ID}
			tape.Entries[earlier].Controlling = false
			tape.Entries[earlier].ShadowedBy = &controller
		}
		latestBySlot[slot] = position
	}
	return tape
}

func (tape Tape) Controllers() []PackageRequirement {
	out := []PackageRequirement{}
	for _, entry := range tape.Entries {
		if entry.Controlling {
			out = append(out, entry.Requirement)
		}
	}
	return out
}

func (tape Tape) Classify(selected map[string]string) Tape {
	out := Tape{Entries: append([]Entry(nil), tape.Entries...)}
	for index := range out.Entries {
		entry := &out.Entries[index]
		version, present := selected[SlotKey(entry.Requirement)]
		entry.SelectedVersion = version
		constraint, err := semver.NewConstraint(entry.Requirement.Constraint)
		if err != nil {
			entry.Status = StatusInvalid
			continue
		}
		if !present || version == "" {
			if entry.Requirement.Optional {
				entry.Status = StatusOptionalUnsatisfied
			} else {
				entry.Status = StatusPending
			}
			continue
		}
		selectedVersion, err := semver.NewVersion(version)
		if err != nil {
			entry.Status = StatusInvalid
			continue
		}
		compatible := constraint.Check(selectedVersion)
		if entry.Controlling {
			if compatible {
				entry.Status = StatusControlling
			} else {
				entry.Status = StatusInvalid
			}
			continue
		}
		if compatible {
			entry.Status = StatusShadowedCompatible
			continue
		}
		if entry.Requirement.Optional {
			entry.Status = StatusOptionalUnsatisfied
			continue
		}
		entry.Status = StatusOverriddenIncompatible
	}
	return out
}

func SlotKey(requirement PackageRequirement) string {
	return requirement.Scope.Key() + "|" + requirement.Target.Key()
}

func normalize(requirement *PackageRequirement, fallbackOrder int) {
	if requirement.Scope.Environment == "" {
		requirement.Scope.Environment = "workspace"
	}
	if requirement.Reference == "" {
		requirement.Reference = requirement.Target.Name
	}
	if requirement.ID == "" {
		requirement.ID = fmt.Sprintf("requirement-%06d", fallbackOrder)
	}
}

func kindRank(kind Kind) int {
	switch kind {
	case KindTransitiveRuntime:
		return 10
	case KindTransitiveOptional:
		return 20
	case KindPeer:
		return 30
	case KindPackageExplicit:
		return 40
	case KindTargetExplicit:
		return 45
	case KindProjectExplicit:
		return 50
	case KindOverride:
		return 60
	default:
		return 0
	}
}
