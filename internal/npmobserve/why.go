package npmobserve

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const SourceLabel = "observed npm package.json/package-lock"

type PackageJSON struct {
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
}

type Lockfile struct {
	LockfileVersion int                    `json:"lockfileVersion"`
	Packages        map[string]LockPackage `json:"packages"`
	Dependencies    map[string]LegacyDep   `json:"dependencies"`
}

type LockPackage struct {
	Version              string            `json:"version"`
	Resolved             string            `json:"resolved"`
	Integrity            string            `json:"integrity"`
	Dependencies         map[string]string `json:"dependencies"`
	Dev                  bool              `json:"dev"`
	Optional             bool              `json:"optional"`
	Peer                 bool              `json:"peer"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
}

type LegacyDep struct {
	Version      string               `json:"version"`
	Dependencies map[string]LegacyDep `json:"dependencies"`
	Dev          bool                 `json:"dev"`
	Optional     bool                 `json:"optional"`
	Peer         bool                 `json:"peer"`
}

type ExplainResult struct {
	Query                string
	Root                 string
	PackageJSONPresent   bool
	LockfilePresent      bool
	UnsupportedLockfiles []string
	Direct               []DirectMatch
	Matches              []PackageMatch
	Chains               []Chain
	Notes                []string
}

type DirectMatch struct {
	Section string
	Range   string
}

type PackageMatch struct {
	Name     string
	Version  string
	Location string
	Flags    []string
}

type Chain struct {
	Nodes []PackageMatch
}

type node struct {
	Name         string
	Version      string
	Location     string
	Dependencies []string
	Flags        []string
}

func Explain(root string, query string) (ExplainResult, error) {
	result := ExplainResult{Query: query, Root: root}
	pkgPath := filepath.Join(root, "package.json")
	pkg, err := readPackageJSON(pkgPath)
	if err != nil {
		return result, err
	}
	result.PackageJSONPresent = true
	result.Direct = directMatches(pkg, query)
	result.UnsupportedLockfiles = unsupportedLockfiles(root)

	lockPath := filepath.Join(root, "package-lock.json")
	lock, err := readLockfile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.Notes = append(result.Notes, "No package-lock.json is available, so TSPack cannot explain transitive npm packages yet.")
			return result, nil
		}
		return result, err
	}
	result.LockfilePresent = true

	nodes := buildNodes(lock)
	result.Matches = matchesByName(nodes, query)
	result.Chains = findChains(pkg, nodes, query, 5)
	return result, nil
}

func HasPackageJSON(root string) bool {
	_, err := os.Stat(filepath.Join(root, "package.json"))
	return err == nil
}

func readPackageJSON(path string) (PackageJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PackageJSON{}, err
	}
	var pkg PackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return PackageJSON{}, fmt.Errorf("package.json is not valid JSON: %w", err)
	}
	return pkg, nil
}

func readLockfile(path string) (Lockfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Lockfile{}, err
	}
	var lock Lockfile
	if err := json.Unmarshal(data, &lock); err != nil {
		return Lockfile{}, fmt.Errorf("package-lock.json is not valid JSON: %w", err)
	}
	return lock, nil
}

func directMatches(pkg PackageJSON, query string) []DirectMatch {
	sections := []struct {
		Name string
		Deps map[string]string
	}{
		{"dependencies", pkg.Dependencies},
		{"devDependencies", pkg.DevDependencies},
		{"peerDependencies", pkg.PeerDependencies},
		{"optionalDependencies", pkg.OptionalDependencies},
	}
	matches := []DirectMatch{}
	for _, section := range sections {
		if requestedRange, ok := section.Deps[query]; ok {
			matches = append(matches, DirectMatch{Section: "package.json " + section.Name, Range: requestedRange})
		}
	}
	return matches
}

func unsupportedLockfiles(root string) []string {
	candidates := []string{
		"pnpm-lock.yaml",
		"yarn.lock",
		"bun.lock",
		"bun." + "lockb",
	}
	found := []string{}
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(root, candidate)); err == nil {
			found = append(found, candidate)
		}
	}
	return found
}

func buildNodes(lock Lockfile) map[string]node {
	nodes := map[string]node{}
	for location, entry := range lock.Packages {
		if location == "" {
			continue
		}
		name := nameFromLocation(location)
		if name == "" {
			continue
		}
		nodes[location] = node{
			Name:         name,
			Version:      entry.Version,
			Location:     location,
			Dependencies: observedLockDependencies(entry),
			Flags:        flags(entry.Dev, entry.Optional, entry.Peer),
		}
	}
	if len(nodes) == 0 {
		for name, entry := range lock.Dependencies {
			location := "node_modules/" + name
			nodes[location] = node{
				Name:         name,
				Version:      entry.Version,
				Location:     location,
				Dependencies: sortedKeysLegacy(entry.Dependencies),
				Flags:        flags(entry.Dev, entry.Optional, entry.Peer),
			}
		}
	}
	return nodes
}

func nameFromLocation(location string) string {
	parts := strings.Split(location, "node_modules/")
	return strings.Trim(strings.TrimSuffix(parts[len(parts)-1], "/"), " ")
}

func matchesByName(nodes map[string]node, query string) []PackageMatch {
	matches := []PackageMatch{}
	for _, n := range nodes {
		if n.Name == query {
			matches = append(matches, packageMatch(n))
		}
	}
	sortMatches(matches)
	return matches
}

func findChains(pkg PackageJSON, nodes map[string]node, query string, limit int) []Chain {
	type item struct {
		Location string
		Path     []string
	}
	queue := []item{}
	seen := map[string]bool{}
	for _, dep := range directDependencyNames(pkg) {
		for _, location := range locationsForName(nodes, dep) {
			queue = append(queue, item{Location: location, Path: []string{location}})
		}
	}
	chains := []Chain{}
	for len(queue) > 0 && len(chains) < limit {
		current := queue[0]
		queue = queue[1:]
		if seen[current.Location+"|"+strings.Join(current.Path, ">")] {
			continue
		}
		seen[current.Location+"|"+strings.Join(current.Path, ">")] = true
		currentNode := nodes[current.Location]
		if currentNode.Name == query {
			chains = append(chains, chainFromPath(nodes, current.Path))
			continue
		}
		for _, depName := range currentNode.Dependencies {
			for _, next := range locationsForName(nodes, depName) {
				if contains(current.Path, next) {
					continue
				}
				queue = append(queue, item{Location: next, Path: append(append([]string{}, current.Path...), next)})
			}
		}
	}
	return chains
}

func directDependencyNames(pkg PackageJSON) []string {
	set := map[string]bool{}
	for _, deps := range []map[string]string{pkg.Dependencies, pkg.DevDependencies, pkg.PeerDependencies, pkg.OptionalDependencies} {
		for name := range deps {
			set[name] = true
		}
	}
	return sortedSet(set)
}

func locationsForName(nodes map[string]node, name string) []string {
	locations := []string{}
	for location, n := range nodes {
		if n.Name == name {
			locations = append(locations, location)
		}
	}
	sort.Strings(locations)
	return locations
}

func chainFromPath(nodes map[string]node, path []string) Chain {
	chain := Chain{Nodes: []PackageMatch{{Name: "root", Location: ""}}}
	for _, location := range path {
		chain.Nodes = append(chain.Nodes, packageMatch(nodes[location]))
	}
	return chain
}

func packageMatch(n node) PackageMatch {
	return PackageMatch{
		Name:     n.Name,
		Version:  n.Version,
		Location: n.Location,
		Flags:    append([]string{}, n.Flags...),
	}
}

func flags(dev bool, optional bool, peer bool) []string {
	out := []string{}
	if dev {
		out = append(out, "dev")
	}
	if optional {
		out = append(out, "optional")
	}
	if peer {
		out = append(out, "peer")
	}
	return out
}

func observedLockDependencies(entry LockPackage) []string {
	set := map[string]bool{}
	for key := range entry.Dependencies {
		set[key] = true
	}
	for key := range entry.OptionalDependencies {
		set[key] = true
	}
	for key := range entry.PeerDependencies {
		set[key] = true
	}
	return sortedSet(set)
}

func sortedKeys(values map[string]string) []string {
	keys := []string{}
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeysLegacy(values map[string]LegacyDep) []string {
	keys := []string{}
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedSet(values map[string]bool) []string {
	keys := []string{}
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortMatches(matches []PackageMatch) {
	sort.SliceStable(matches, func(i int, j int) bool {
		if matches[i].Name != matches[j].Name {
			return matches[i].Name < matches[j].Name
		}
		if matches[i].Version != matches[j].Version {
			return matches[i].Version < matches[j].Version
		}
		return matches[i].Location < matches[j].Location
	})
}

func contains(values []string, query string) bool {
	for _, value := range values {
		if value == query {
			return true
		}
	}
	return false
}
