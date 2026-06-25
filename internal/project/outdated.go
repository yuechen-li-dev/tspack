package project

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	semver "github.com/Masterminds/semver/v3"
	"github.com/tspack/tspack/internal/check"
	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/graph"
	"github.com/tspack/tspack/internal/lockfile"
	"github.com/tspack/tspack/internal/resolver"
)

type OutdatedResult struct {
	Summary      OutdatedSummary
	Dependencies []OutdatedDependency
	Groups       []OutdatedDependency
}

type OutdatedSummary struct {
	Current  int
	Outdated int
	Skipped  int
	Errors   int
	Total    int
}

type OutdatedPackage struct {
	Name string `json:"name"`
	Root string `json:"root,omitempty"`
}

type OutdatedDependency struct {
	Key          string
	Name         string
	Kind         string
	Source       string
	Requested    string
	Current      []string
	Wanted       string
	Latest       string
	Status       string
	Packages     []OutdatedPackage
	PackageCount int
}

func Outdated(opts Options) Result {
	_, g, out := loadManifestAndGraph(opts)
	out = append(out, check.CheckPackage(check.CheckOptions{RootDir: opts.RootDir, Graph: g}).Diagnostics...)
	if hasErrors(out) {
		return Result{Diagnostics: out}
	}
	var lf *lockfile.Lockfile
	if _, err := os.Stat(opts.LockfilePath); err == nil {
		loaded, d, e := lockfile.LoadFile(opts.LockfilePath)
		if e != nil {
			out = append(out, errDiag("TSPACK_OUTDATED_RESOLUTION_FAILED", "failed to read lockfile", e.Error()))
			return Result{Diagnostics: out}
		}
		out = append(out, d...)
		lf = loaded
	} else if os.IsNotExist(err) {
		out = append(out, diag.Diagnostic{Code: "TSPACK_OUTDATED_LOCKFILE_MISSING", Severity: diag.SeverityWarning, Message: "lockfile is missing"})
	}
	client := opts.ResolverClient
	if client == nil {
		client = resolver.NewHTTPRegistryClient("")
	}
	result := buildOutdatedResult(context.Background(), g, lf, client, &out)
	diag.SortDiagnostics(out)
	return Result{Diagnostics: out, Outdated: &result}
}

func buildOutdatedResult(ctx context.Context, g *graph.WorkspaceGraph, lf *lockfile.Lockfile, client resolver.NPMRegistryClient, out *[]diag.Diagnostic) OutdatedResult {
	entries := make([]OutdatedDependency, 0)
	currentByName := map[string][]string{}
	if lf != nil {
		for _, pkg := range lf.Packages {
			if pkg.Source != "npm" || pkg.Name == "" || pkg.Version == "" {
				continue
			}
			currentByName[pkg.Name] = append(currentByName[pkg.Name], pkg.Version)
		}
	}
	for name := range currentByName {
		sort.Strings(currentByName[name])
	}
	for _, pkg := range g.AllPackages() {
		for _, dep := range pkg.AllDependencies() {
			entry := OutdatedDependency{
				Key:          dep.Key,
				Kind:         string(dep.Kind),
				Source:       dep.Source.Kind,
				Status:       "not_applicable",
				Packages:     []OutdatedPackage{{Name: pkg.Name, Root: pkg.Root}},
				PackageCount: 1,
			}
			switch dep.Source.Kind {
			case "npm":
				entry.Name = dep.Source.Package
				entry.Requested = dep.Source.Range
				entry.Current = append([]string(nil), currentByName[entry.Name]...)
				if len(entry.Current) == 0 {
					entry.Status = "missing_lock"
				} else {
					entry.Status = "current"
				}
				meta, err := client.PackageMetadata(ctx, entry.Name)
				if err != nil {
					entry.Status = "error"
					*out = append(*out, diag.Diagnostic{Code: "TSPACK_OUTDATED_METADATA_FETCH_FAILED", Severity: diag.SeverityError, Message: "failed to fetch npm metadata", Details: []string{entry.Key, entry.Name, err.Error()}})
					entries = append(entries, entry)
					continue
				}
				wanted, wErr := selectHighestSatisfying(meta, entry.Requested)
				if wErr != nil {
					entry.Status = "error"
					*out = append(*out, diag.Diagnostic{Code: "TSPACK_OUTDATED_RESOLUTION_FAILED", Severity: diag.SeverityError, Message: "failed to resolve wanted version", Details: []string{entry.Key, entry.Name, wErr.Error()}})
					entries = append(entries, entry)
					continue
				}
				entry.Wanted = wanted
				entry.Latest = latestVersion(meta)
				if len(entry.Current) > 0 {
					current := entry.Current[len(entry.Current)-1]
					if lessThan(current, entry.Wanted) {
						entry.Status = "wanted_available"
					} else if entry.Latest != "" && entry.Latest != entry.Wanted && !lessThan(current, entry.Wanted) {
						entry.Status = "latest_available"
					}
				}
			default:
				entry.Name = dep.Source.Package
				if entry.Name == "" {
					entry.Name = dep.Key
				}
				*out = append(*out, diag.Diagnostic{Code: "TSPACK_OUTDATED_NON_REGISTRY_DEP", Severity: diag.SeverityWarning, Message: "non-registry dependency is not applicable to outdated checks", Details: []string{entry.Key, dep.Source.Kind}})
			}
			entries = append(entries, entry)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Key != entries[j].Key {
			return entries[i].Key < entries[j].Key
		}
		return entries[i].Name < entries[j].Name
	})
	groups := groupOutdatedDependencies(entries)
	summary := summarizeOutdated(entries)
	return OutdatedResult{Summary: summary, Dependencies: entries, Groups: groups}
}

func groupOutdatedDependencies(entries []OutdatedDependency) []OutdatedDependency {
	byKey := map[string]int{}
	groups := make([]OutdatedDependency, 0, len(entries))
	for _, entry := range entries {
		key := outdatedGroupKey(entry)
		index, ok := byKey[key]
		if !ok {
			group := entry
			group.Key = entry.Name
			group.Packages = append([]OutdatedPackage(nil), entry.Packages...)
			group.PackageCount = len(group.Packages)
			byKey[key] = len(groups)
			groups = append(groups, group)
			continue
		}
		groups[index].Packages = append(groups[index].Packages, entry.Packages...)
		groups[index].PackageCount = len(groups[index].Packages)
	}
	for i := range groups {
		sort.SliceStable(groups[i].Packages, func(a, b int) bool {
			if groups[i].Packages[a].Name != groups[i].Packages[b].Name {
				return groups[i].Packages[a].Name < groups[i].Packages[b].Name
			}
			return groups[i].Packages[a].Root < groups[i].Packages[b].Root
		})
	}
	sort.SliceStable(groups, func(i, j int) bool {
		return outdatedGroupKey(groups[i]) < outdatedGroupKey(groups[j])
	})
	return groups
}

func outdatedGroupKey(entry OutdatedDependency) string {
	return strings.Join([]string{
		entry.Source,
		entry.Name,
		entry.Kind,
		entry.Requested,
		strings.Join(entry.Current, ","),
		entry.Wanted,
		entry.Latest,
		entry.Status,
	}, "\x00")
}

func summarizeOutdated(entries []OutdatedDependency) OutdatedSummary {
	s := OutdatedSummary{Total: len(entries)}
	for _, e := range entries {
		switch e.Status {
		case "current":
			s.Current++
		case "wanted_available", "latest_available":
			s.Outdated++
		case "not_applicable":
			s.Skipped++
		case "error":
			s.Errors++
		}
	}
	return s
}

func selectHighestSatisfying(meta *resolver.PackageMetadata, rng string) (string, error) {
	constraint, err := semver.NewConstraint(rng)
	if err != nil {
		return "", err
	}
	versions := make([]*semver.Version, 0, len(meta.Versions))
	for v := range meta.Versions {
		sv, parseErr := semver.NewVersion(v)
		if parseErr != nil {
			continue
		}
		versions = append(versions, sv)
	}
	sort.SliceStable(versions, func(i, j int) bool { return versions[i].GreaterThan(versions[j]) })
	for _, v := range versions {
		if constraint.Check(v) {
			return v.Original(), nil
		}
	}
	return "", fmt.Errorf("no versions satisfy %q", rng)
}

func latestVersion(meta *resolver.PackageMetadata) string {
	if meta == nil {
		return ""
	}
	if latest, ok := meta.DistTags["latest"]; ok && latest != "" {
		return latest
	}
	versions := make([]*semver.Version, 0, len(meta.Versions))
	for v := range meta.Versions {
		sv, err := semver.NewVersion(v)
		if err != nil {
			continue
		}
		versions = append(versions, sv)
	}
	if len(versions) == 0 {
		return ""
	}
	sort.SliceStable(versions, func(i, j int) bool { return versions[i].GreaterThan(versions[j]) })
	return versions[0].Original()
}

func lessThan(a, b string) bool {
	av, aErr := semver.NewVersion(a)
	bv, bErr := semver.NewVersion(b)
	if aErr != nil || bErr != nil {
		return strings.Compare(a, b) < 0
	}
	return av.LessThan(bv)
}
