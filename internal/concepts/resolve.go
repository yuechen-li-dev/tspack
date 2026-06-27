package concepts

import (
	"fmt"
	"strings"
)

type ResolveResult struct {
	Fragments []Fragment
	Inserted  []string
}

func Resolve(list []string, kind string) (ResolveResult, error) {
	return ResolveWithRegistry(Builtins, list, kind)
}

func ResolveWithRegistry(registry Registry, requested []string, kind string) (ResolveResult, error) {
	listed := map[string]bool{}
	fragments := make([]Fragment, 0, len(requested))
	for _, name := range requested {
		fragment, ok := registry.Lookup(name)
		if !ok {
			return ResolveResult{}, DiagnosticError{Kind: "unknown", Message: fmt.Sprintf("unknown concept %q", name)}
		}
		if listed[name] {
			continue
		}
		listed[name] = true
		fragments = append(fragments, fragment)
	}

	for _, fragment := range fragments {
		if len(fragment.CompatibleKinds) > 0 && kind != "" && !contains(fragment.CompatibleKinds, kind) {
			return ResolveResult{}, DiagnosticError{Kind: "incompatible_kind", Message: fmt.Sprintf("concept %q is incompatible with kind %q", fragment.Name, kind)}
		}
		for _, expected := range fragment.Requires {
			if !listed[expected] {
				return ResolveResult{}, DiagnosticError{Kind: "missing_expected", Message: fmt.Sprintf("concept %q expects %q, but it is not listed", fragment.Name, expected)}
			}
		}
		for _, group := range fragment.RequiresAnyOf {
			if !anyListed(group, listed) {
				return ResolveResult{}, DiagnosticError{Kind: "expects_any_of", Message: fmt.Sprintf("concept %q expects one of %s, but none are listed", fragment.Name, strings.Join(group, ", "))}
			}
		}
		for _, other := range fragment.Conflicts {
			if listed[other] {
				return ResolveResult{}, Conflict{Path: "concepts", ConceptA: fragment.Name, ConceptB: other, Reason: "declared conflict"}
			}
		}
	}

	return ResolveResult{Fragments: fragments, Inserted: nil}, nil
}

func anyListed(group []string, listed map[string]bool) bool {
	for _, name := range group {
		if listed[name] {
			return true
		}
	}
	return false
}

func cyclePath(stack []string, name string) string {
	parts := append([]string{}, stack...)
	parts = append(parts, name)
	return strings.Join(parts, " -> ")
}
func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
