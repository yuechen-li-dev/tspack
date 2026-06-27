package concepts

import (
	"fmt"
	"sort"
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
	requestedIndex := map[string]int{}
	for index, name := range requested {
		if _, ok := registry.Lookup(name); !ok {
			return ResolveResult{}, DiagnosticError{Kind: "unknown", Message: fmt.Sprintf("unknown concept %q", name)}
		}
		if _, exists := requestedIndex[name]; !exists {
			requestedIndex[name] = index
		}
	}
	seen := map[string]bool{}
	resolving := map[string]bool{}
	stack := []string{}
	inserted := []string{}
	var include func(string, bool) error
	include = func(name string, explicit bool) error {
		if resolving[name] {
			return DiagnosticError{Kind: "cycle", Message: "concept cycle: " + cyclePath(stack, name)}
		}
		if seen[name] {
			return nil
		}
		fragment, ok := registry.Lookup(name)
		if !ok {
			return DiagnosticError{Kind: "missing_required", Message: fmt.Sprintf("missing required concept %q", name)}
		}
		if !explicit && !seen[name] {
			inserted = append(inserted, name)
		}
		seen[name] = true
		resolving[name] = true
		stack = append(stack, name)
		for _, required := range fragment.Requires {
			if err := include(required, false); err != nil {
				return err
			}
		}
		for _, group := range fragment.RequiresAnyOf {
			if anySeen(group, seen) {
				continue
			}
			insertedAny := false
			for _, candidate := range group {
				if _, ok := registry.Lookup(candidate); ok {
					if err := include(candidate, false); err != nil {
						return err
					}
					insertedAny = true
					break
				}
			}
			if !insertedAny {
				return DiagnosticError{Kind: "requires_any_of", Message: fmt.Sprintf("concept %q requires one of %s", name, strings.Join(group, ", "))}
			}
		}
		stack = stack[:len(stack)-1]
		resolving[name] = false
		return nil
	}
	for _, name := range requested {
		if err := include(name, true); err != nil {
			return ResolveResult{}, err
		}
	}
	fragments := map[string]Fragment{}
	for name := range seen {
		fragment, _ := registry.Lookup(name)
		fragments[name] = fragment
	}
	for _, fragment := range fragments {
		if len(fragment.CompatibleKinds) > 0 && kind != "" && !contains(fragment.CompatibleKinds, kind) {
			return ResolveResult{}, DiagnosticError{Kind: "incompatible_kind", Message: fmt.Sprintf("concept %q is incompatible with kind %q", fragment.Name, kind)}
		}
		for _, other := range fragment.Conflicts {
			if seen[other] {
				return ResolveResult{}, Conflict{Path: "concepts", ConceptA: fragment.Name, ConceptB: other, Reason: "declared conflict"}
			}
		}
	}
	ordered, err := topologicalOrder(fragments, requestedIndex)
	if err != nil {
		return ResolveResult{}, err
	}
	return ResolveResult{Fragments: ordered, Inserted: inserted}, nil
}

func anySeen(group []string, seen map[string]bool) bool {
	for _, name := range group {
		if seen[name] {
			return true
		}
	}
	return false
}

func topologicalOrder(fragments map[string]Fragment, requestedIndex map[string]int) ([]Fragment, error) {
	visiting := map[string]bool{}
	visited := map[string]bool{}
	output := []Fragment{}
	stack := []string{}
	names := make([]string, 0, len(fragments))
	for name := range fragments {
		names = append(names, name)
	}
	sort.SliceStable(names, func(i, j int) bool {
		return orderRank(names[i], requestedIndex) < orderRank(names[j], requestedIndex) || (orderRank(names[i], requestedIndex) == orderRank(names[j], requestedIndex) && names[i] < names[j])
	})
	var visit func(string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		if visiting[name] {
			return DiagnosticError{Kind: "cycle", Message: "concept cycle: " + cyclePath(stack, name)}
		}
		visiting[name] = true
		stack = append(stack, name)
		fragment := fragments[name]
		required := append([]string{}, fragment.Requires...)
		sort.SliceStable(required, func(i, j int) bool {
			return orderRank(required[i], requestedIndex) < orderRank(required[j], requestedIndex) || (orderRank(required[i], requestedIndex) == orderRank(required[j], requestedIndex) && required[i] < required[j])
		})
		for _, dep := range required {
			if _, ok := fragments[dep]; ok {
				if err := visit(dep); err != nil {
					return err
				}
			}
		}
		stack = stack[:len(stack)-1]
		visiting[name] = false
		visited[name] = true
		output = append(output, fragment)
		return nil
	}
	for _, name := range names {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return output, nil
}
func orderRank(name string, requestedIndex map[string]int) int {
	if i, ok := requestedIndex[name]; ok {
		return i
	}
	return 1000000
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
