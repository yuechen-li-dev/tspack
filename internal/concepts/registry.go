package concepts

import "sort"

type Registry interface {
	Lookup(name string) (Fragment, bool)
	BuiltinNames() []string
}

type MapRegistry struct{ fragments map[string]Fragment }

func NewRegistry(fragments []Fragment) *MapRegistry {
	m := map[string]Fragment{}
	for _, fragment := range fragments {
		m[fragment.Name] = fragment
	}
	return &MapRegistry{fragments: m}
}

func (r *MapRegistry) Lookup(name string) (Fragment, bool) {
	fragment, ok := r.fragments[name]
	return fragment, ok
}

func (r *MapRegistry) BuiltinNames() []string {
	names := make([]string, 0, len(r.fragments))
	for name := range r.fragments {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

var Builtins = NewRegistry(builtinFragments())

func Lookup(name string) (Fragment, bool) {
	return Builtins.Lookup(name)
}

func MustLookup(name string) Fragment {
	fragment, ok := Lookup(name)
	if !ok {
		panic("unknown built-in concept: " + name)
	}
	return fragment
}

func BuiltinNames() []string {
	return Builtins.BuiltinNames()
}

func dep(name, rng string) DependencyContribution {
	return DependencyContribution{Name: name, Range: rng, Source: "npm"}
}
