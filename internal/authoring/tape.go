package authoring

import (
	"fmt"
	"sort"
)

type TapeRef struct {
	Position int    `json:"position"`
	ID       string `json:"id"`
}

type TapeEntry struct {
	Declaration DependencyDeclaration `json:"declaration"`
	Position    int                   `json:"position"`
	Effective   bool                  `json:"effective"`
	ShadowedBy  *TapeRef              `json:"shadowedBy,omitempty"`
}

type ConflictOutcome string

const (
	OutcomeDuplicate          ConflictOutcome = "duplicate"
	OutcomeConstraintOverride ConflictOutcome = "constraint-override"
	OutcomeKindOverride       ConflictOutcome = "kind-override"
	OutcomeOptionalOverride   ConflictOutcome = "optional-override"
	OutcomeKeyCollision       ConflictOutcome = "key-collision"
)

type AuthoringConflict struct {
	Outcome ConflictOutcome `json:"outcome"`
	Earlier TapeRef         `json:"earlier"`
	Later   TapeRef         `json:"later"`
	Fatal   bool            `json:"fatal,omitempty"`
	Message string          `json:"message"`
}

type TapeResolution struct {
	Entries   []TapeEntry           `json:"entries"`
	Effective []EffectiveDependency `json:"effective"`
	Conflicts []AuthoringConflict   `json:"conflicts,omitempty"`
}

func Build(declarations []DependencyDeclaration) TapeResolution {
	ordered := append([]DependencyDeclaration(nil), declarations...)
	for index := range ordered {
		normalizeDeclaration(&ordered[index], index)
	}

	sort.SliceStable(ordered, func(left, right int) bool {
		leftRank := layerRank(ordered[left].Layer)
		rightRank := layerRank(ordered[right].Layer)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if ordered[left].LayerOrder != ordered[right].LayerOrder {
			return ordered[left].LayerOrder < ordered[right].LayerOrder
		}
		if ordered[left].Order != ordered[right].Order {
			return ordered[left].Order < ordered[right].Order
		}
		return ordered[left].ID < ordered[right].ID
	})

	resolution := TapeResolution{
		Entries: make([]TapeEntry, len(ordered)),
	}
	latestByIdentity := map[string]int{}
	latestByEffectiveKey := map[string]int{}
	for position, declaration := range ordered {
		entry := TapeEntry{
			Declaration: declaration,
			Position:    position,
			Effective:   true,
		}
		resolution.Entries[position] = entry

		identityKey := declarationSlotKey(declaration)
		if earlierPosition, ok := latestByIdentity[identityKey]; ok {
			laterRef := tapeRef(resolution.Entries[position])
			resolution.Entries[earlierPosition].Effective = false
			resolution.Entries[earlierPosition].ShadowedBy = &laterRef
			resolution.Conflicts = append(
				resolution.Conflicts,
				classifyShadow(resolution.Entries[earlierPosition], resolution.Entries[position]),
			)
		}
		latestByIdentity[identityKey] = position

		effectiveKey := EffectiveIdentity(declaration.EffectiveDependency())
		if earlierPosition, ok := latestByEffectiveKey[effectiveKey]; ok {
			earlierIdentity := resolution.Entries[earlierPosition].Declaration.Identity
			if earlierIdentity != declaration.Identity {
				resolution.Conflicts = append(resolution.Conflicts, AuthoringConflict{
					Outcome: OutcomeKeyCollision,
					Earlier: tapeRef(resolution.Entries[earlierPosition]),
					Later:   tapeRef(resolution.Entries[position]),
					Fatal:   true,
					Message: fmt.Sprintf("effective dependency key %q names both %s and %s", effectiveKey, earlierIdentity.Key(), declaration.Identity.Key()),
				})
			}
		}
		latestByEffectiveKey[effectiveKey] = position
	}

	for _, entry := range resolution.Entries {
		if entry.Effective {
			resolution.Effective = append(resolution.Effective, entry.Declaration.EffectiveDependency())
		}
	}
	return resolution
}

func declarationSlotKey(declaration DependencyDeclaration) string {
	effectiveKey := EffectiveIdentity(declaration.EffectiveDependency())
	return declaration.Identity.Key() + "|" + effectiveKey
}

func normalizeDeclaration(declaration *DependencyDeclaration, fallbackOrder int) {
	if !declaration.Identity.Valid() {
		declaration.Identity = declaration.Source.Identity()
	}
	if declaration.Constraint == "" {
		declaration.Constraint = declaration.Source.Range
	}
	if declaration.Layer == "" {
		declaration.Layer = LayerBase
	}
	if declaration.Authority == "" {
		declaration.Authority = AuthorityOwned
	}
	if declaration.Editability == "" {
		declaration.Editability = EditabilityDerived
	}
	if declaration.ID == "" {
		declaration.ID = fmt.Sprintf("declaration-%04d", fallbackOrder)
	}
}

func layerRank(layer DeclarationLayer) int {
	switch layer {
	case LayerBase:
		return 0
	case LayerConcept:
		return 10
	case LayerTemplate:
		return 20
	case LayerProject:
		return 30
	case LayerPackage:
		return 40
	case LayerCompatibility:
		return 50
	case LayerExplicit:
		return 60
	default:
		return 25
	}
}

func tapeRef(entry TapeEntry) TapeRef {
	return TapeRef{Position: entry.Position, ID: entry.Declaration.ID}
}

func classifyShadow(earlier TapeEntry, later TapeEntry) AuthoringConflict {
	outcome := OutcomeDuplicate
	message := "later equivalent declaration shadows earlier declaration"
	if earlier.Declaration.Kind != later.Declaration.Kind {
		outcome = OutcomeKindOverride
		message = fmt.Sprintf("dependency kind changes from %s to %s", earlier.Declaration.Kind, later.Declaration.Kind)
	} else if earlier.Declaration.Optional != later.Declaration.Optional {
		outcome = OutcomeOptionalOverride
		message = fmt.Sprintf("optional semantics change from %t to %t", earlier.Declaration.Optional, later.Declaration.Optional)
	} else if earlier.Declaration.Constraint != later.Declaration.Constraint {
		outcome = OutcomeConstraintOverride
		message = fmt.Sprintf("constraint changes from %q to %q", earlier.Declaration.Constraint, later.Declaration.Constraint)
	}
	return AuthoringConflict{
		Outcome: outcome,
		Earlier: tapeRef(earlier),
		Later:   tapeRef(later),
		Message: message,
	}
}

func HasFatalConflicts(resolution TapeResolution) bool {
	for _, conflict := range resolution.Conflicts {
		if conflict.Fatal {
			return true
		}
	}
	return false
}
