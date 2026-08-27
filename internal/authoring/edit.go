package authoring

import "fmt"

type DeclarationSelector struct {
	ID           string
	Identity     *PackageIdentity
	Kind         DependencyKind
	OriginKind   OriginKind
	OriginName   string
	SourcePath   string
	EditableOnly bool
}

type ChangeKind string

const (
	ChangeAdded      ChangeKind = "added"
	ChangeRemoved    ChangeKind = "removed"
	ChangeChanged    ChangeKind = "changed"
	ChangeShadowed   ChangeKind = "shadowed"
	ChangeUnshadowed ChangeKind = "unshadowed"
)

type AuthoringChange struct {
	Kind        ChangeKind             `json:"kind"`
	Declaration DependencyDeclaration  `json:"declaration"`
	Previous    *DependencyDeclaration `json:"previous,omitempty"`
}

type EditResult struct {
	Before  TapeResolution    `json:"before"`
	After   TapeResolution    `json:"after"`
	Changes []AuthoringChange `json:"changes"`
}

type AmbiguousRemovalError struct {
	Matches []DependencyDeclaration
}

func (err AmbiguousRemovalError) Error() string {
	return fmt.Sprintf("dependency removal is ambiguous: %d declarations match", len(err.Matches))
}

type DeclarationNotFoundError struct{}

func (DeclarationNotFoundError) Error() string {
	return "dependency declaration was not found"
}

func Add(ir PackageIR, declaration DependencyDeclaration) EditResult {
	before := Build(ir.Declarations)
	afterDeclarations := append([]DependencyDeclaration(nil), ir.Declarations...)
	if declaration.Layer == "" {
		declaration.Layer = LayerExplicit
	}
	if declaration.Origin.Kind == "" {
		declaration.Origin = DeclarationOrigin{Kind: OriginExplicitUserOperation, Name: "dependency edit"}
	}
	if declaration.Authority == "" {
		declaration.Authority = AuthorityOwned
	}
	if declaration.Editability == "" {
		declaration.Editability = EditabilityEditable
	}
	declaration.Order = nextOrder(afterDeclarations, declaration.Layer, declaration.LayerOrder)
	normalizeDeclaration(&declaration, len(afterDeclarations))
	afterDeclarations = append(afterDeclarations, declaration)
	after := Build(afterDeclarations)
	return buildEditResult(before, after, []AuthoringChange{{Kind: ChangeAdded, Declaration: declaration}})
}

func Remove(ir PackageIR, selector DeclarationSelector) (EditResult, error) {
	before := Build(ir.Declarations)
	matches := matchingDeclarationIndexes(ir.Declarations, selector)
	if len(matches) == 0 {
		return EditResult{}, DeclarationNotFoundError{}
	}
	if len(matches) > 1 {
		declarations := make([]DependencyDeclaration, 0, len(matches))
		for _, index := range matches {
			declarations = append(declarations, ir.Declarations[index])
		}
		return EditResult{}, AmbiguousRemovalError{Matches: declarations}
	}

	removed := ir.Declarations[matches[0]]
	afterDeclarations := make([]DependencyDeclaration, 0, len(ir.Declarations)-1)
	afterDeclarations = append(afterDeclarations, ir.Declarations[:matches[0]]...)
	afterDeclarations = append(afterDeclarations, ir.Declarations[matches[0]+1:]...)
	after := Build(afterDeclarations)
	return buildEditResult(before, after, []AuthoringChange{{Kind: ChangeRemoved, Declaration: removed}}), nil
}

func Replace(ir PackageIR, selector DeclarationSelector, replacement DependencyDeclaration) (EditResult, error) {
	matches := matchingDeclarationIndexes(ir.Declarations, selector)
	if len(matches) == 0 {
		return EditResult{}, DeclarationNotFoundError{}
	}
	if len(matches) > 1 {
		declarations := make([]DependencyDeclaration, 0, len(matches))
		for _, index := range matches {
			declarations = append(declarations, ir.Declarations[index])
		}
		return EditResult{}, AmbiguousRemovalError{Matches: declarations}
	}

	before := Build(ir.Declarations)
	previous := ir.Declarations[matches[0]]
	if replacement.ID == "" {
		replacement.ID = previous.ID
	}
	if replacement.Order == 0 {
		replacement.Order = previous.Order
	}
	afterDeclarations := append([]DependencyDeclaration(nil), ir.Declarations...)
	afterDeclarations[matches[0]] = replacement
	after := Build(afterDeclarations)
	return buildEditResult(before, after, []AuthoringChange{{
		Kind:        ChangeChanged,
		Declaration: replacement,
		Previous:    &previous,
	}}), nil
}

func ChangeConstraint(ir PackageIR, selector DeclarationSelector, constraint string) (EditResult, error) {
	matches := matchingDeclarationIndexes(ir.Declarations, selector)
	if len(matches) == 0 {
		return EditResult{}, DeclarationNotFoundError{}
	}
	if len(matches) > 1 {
		declarations := make([]DependencyDeclaration, 0, len(matches))
		for _, index := range matches {
			declarations = append(declarations, ir.Declarations[index])
		}
		return EditResult{}, AmbiguousRemovalError{Matches: declarations}
	}
	replacement := ir.Declarations[matches[0]]
	replacement.Constraint = constraint
	replacement.Source.Range = constraint
	return Replace(ir, selector, replacement)
}

func matchingDeclarationIndexes(declarations []DependencyDeclaration, selector DeclarationSelector) []int {
	var matches []int
	for index, declaration := range declarations {
		if selector.ID != "" && declaration.ID != selector.ID {
			continue
		}
		identity := declaration.Identity
		if !identity.Valid() {
			identity = declaration.Source.Identity()
		}
		if selector.Identity != nil && identity != *selector.Identity {
			continue
		}
		if selector.Kind != "" && declaration.Kind != selector.Kind {
			continue
		}
		if selector.OriginKind != "" && declaration.Origin.Kind != selector.OriginKind {
			continue
		}
		if selector.OriginName != "" && declaration.Origin.Name != selector.OriginName {
			continue
		}
		if selector.SourcePath != "" && declaration.Origin.SourcePath != selector.SourcePath {
			continue
		}
		if selector.EditableOnly && declaration.Editability != EditabilityEditable {
			continue
		}
		matches = append(matches, index)
	}
	return matches
}

func nextOrder(declarations []DependencyDeclaration, layer DeclarationLayer, layerOrder int) int {
	order := -1
	for _, declaration := range declarations {
		if declaration.Layer == layer && declaration.LayerOrder == layerOrder && declaration.Order > order {
			order = declaration.Order
		}
	}
	return order + 1
}

func buildEditResult(before TapeResolution, after TapeResolution, changes []AuthoringChange) EditResult {
	beforeEffective := effectiveEntriesByIdentity(before)
	afterEffective := effectiveEntriesByIdentity(after)
	for identity, beforeEntry := range beforeEffective {
		afterEntry, stillEffective := afterEffective[identity]
		if !stillEffective {
			continue
		}
		if beforeEntry.Declaration.ID == afterEntry.Declaration.ID {
			continue
		}
		if removesDeclaration(changes, beforeEntry.Declaration.ID) {
			changes = append(changes, AuthoringChange{Kind: ChangeUnshadowed, Declaration: afterEntry.Declaration, Previous: &beforeEntry.Declaration})
		} else {
			changes = append(changes, AuthoringChange{Kind: ChangeShadowed, Declaration: beforeEntry.Declaration, Previous: &afterEntry.Declaration})
		}
	}
	return EditResult{Before: before, After: after, Changes: changes}
}

func removesDeclaration(changes []AuthoringChange, declarationID string) bool {
	for _, change := range changes {
		if change.Kind == ChangeRemoved && change.Declaration.ID == declarationID {
			return true
		}
	}
	return false
}

func effectiveEntriesByIdentity(resolution TapeResolution) map[string]TapeEntry {
	out := map[string]TapeEntry{}
	for _, entry := range resolution.Entries {
		if entry.Effective {
			out[declarationSlotKey(entry.Declaration)] = entry
		}
	}
	return out
}
