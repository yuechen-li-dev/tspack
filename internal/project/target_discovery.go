package project

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

// TargetDiscoveryRequest selects the semantic target projection exposed to
// users and tooling. Kind is empty, build, or test.
type TargetDiscoveryRequest struct {
	Project Options
	Kind    string
}

type TargetDiscoveryResult struct {
	Targets     []DiscoveredTarget `json:"targets"`
	Diagnostics []diag.Diagnostic  `json:"diagnostics,omitempty"`
}

type DiscoveredTarget struct {
	Identity        string                  `json:"identity"`
	Kind            string                  `json:"kind"`
	Package         string                  `json:"package"`
	PublicationName string                  `json:"publicationName,omitempty"`
	Root            string                  `json:"root"`
	Name            string                  `json:"name"`
	Tool            string                  `json:"tool"`
	Artifacts       []DiscoveredArtifact    `json:"artifacts,omitempty"`
	Prerequisites   []string                `json:"prerequisites,omitempty"`
	Sources         []string                `json:"sources,omitempty"`
	HarnessProject  string                  `json:"harnessProject,omitempty"`
	Requirements    []DiscoveredRequirement `json:"requirements,omitempty"`
	Fixtures        []DiscoveredFixture     `json:"fixtures,omitempty"`
}

type DiscoveredRequirement struct {
	Identity string `json:"identity"`
	Source   string `json:"source"`
	Producer string `json:"producer"`
}

type DiscoveredFixture struct {
	Identity     string `json:"identity"`
	Producer     string `json:"producer"`
	Binding      string `json:"binding"`
	Mode         string `json:"mode"`
	RealizedPath string `json:"realizedPath"`
}

type DiscoveredArtifact struct {
	Identity string `json:"identity"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Role     string `json:"role,omitempty"`
	Path     string `json:"path"`
}

func DiscoverTargets(request TargetDiscoveryRequest) TargetDiscoveryResult {
	result := TargetDiscoveryResult{Targets: []DiscoveredTarget{}}
	if request.Kind != "" && request.Kind != "build" && request.Kind != "test" {
		result.Diagnostics = append(result.Diagnostics, errDiag("TSPACK_TARGET_DISCOVERY_KIND_INVALID", "target kind must be build or test"))
		return result
	}
	ir, diagnostics := loadManifestIR(request.Project)
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	if hasErrors(result.Diagnostics) {
		return result
	}
	for _, pkg := range ir.Packages {
		if request.Kind == "" || request.Kind == "build" {
			for _, target := range pkg.Targets {
				result.Targets = append(result.Targets, discoverBuildTarget(pkg, target))
			}
		}
		if request.Kind == "" || request.Kind == "test" {
			for _, target := range pkg.TestTargets {
				result.Targets = append(result.Targets, discoverTestTarget(pkg, target))
			}
		}
	}
	sort.Slice(result.Targets, func(i, j int) bool {
		return result.Targets[i].Identity < result.Targets[j].Identity
	})
	return result
}

func discoverBuildTarget(pkg manifest.Package, target manifest.Target) DiscoveredTarget {
	artifacts := target.Artifacts
	if len(artifacts) == 0 {
		artifacts = []manifest.TargetArtifact{
			{Name: "javaScript", Kind: "javaScript", Role: "runtimeEntry", Path: target.Runtime},
			{Name: "typeDeclarations", Kind: "typeDeclarations", Role: "typeDeclaration", Path: target.Types},
		}
	}
	discoveredArtifacts := []DiscoveredArtifact{}
	for _, artifact := range artifacts {
		if artifact.Path == "" {
			continue
		}
		discoveredArtifacts = append(discoveredArtifacts, DiscoveredArtifact{
			Identity: pkg.Name + ":" + target.Name + ":" + artifact.Name,
			Name:     artifact.Name,
			Kind:     artifact.Kind,
			Role:     artifact.Role,
			Path:     artifact.Path,
		})
	}
	sort.Slice(discoveredArtifacts, func(i, j int) bool {
		return discoveredArtifacts[i].Identity < discoveredArtifacts[j].Identity
	})
	prerequisites := make([]string, 0, len(target.DependsOn))
	for _, reference := range target.DependsOn {
		dependencyPackage, dependencyTarget := manifest.ResolveBuildTargetReference(pkg.Name, reference)
		prerequisites = append(prerequisites, dependencyPackage+":"+dependencyTarget)
	}
	sort.Strings(prerequisites)
	return DiscoveredTarget{
		Identity:        "build:" + pkg.Name + ":" + target.Name,
		Kind:            "build",
		Package:         pkg.Name,
		PublicationName: pkg.PublicationName,
		Root:            packageDisplayRoot(pkg.Root),
		Name:            target.Name,
		Tool:            target.Compiler,
		Artifacts:       discoveredArtifacts,
		Prerequisites:   prerequisites,
	}
}

func discoverTestTarget(pkg manifest.Package, target manifest.TestTarget) DiscoveredTarget {
	sources := append([]string{}, target.Sources...)
	sort.Strings(sources)
	dependencies := map[string]manifest.DependencyIntent{}
	for _, dependency := range pkg.Dependencies {
		dependencies[manifest.DependencyIdentity(dependency)] = dependency
	}
	requirements := make([]DiscoveredRequirement, 0, len(target.Requirements))
	for _, identity := range target.Requirements {
		dependency := dependencies[identity]
		requirements = append(requirements, DiscoveredRequirement{
			Identity: identity,
			Source:   dependency.Source.Kind,
			Producer: dependencySourceIdentity(dependency.Source),
		})
	}
	sort.Slice(requirements, func(i, j int) bool { return requirements[i].Identity < requirements[j].Identity })
	fixtures := make([]DiscoveredFixture, 0, len(target.Fixtures))
	for _, fixture := range target.Fixtures {
		dependency := dependencies[fixture.Dependency]
		fixtures = append(fixtures, DiscoveredFixture{
			Identity:     fixture.Name,
			Producer:     dependencySourceIdentity(dependency.Source),
			Binding:      fixture.Binding,
			Mode:         fixture.Mode,
			RealizedPath: filepath.ToSlash(filepath.Join(packageDisplayRoot(pkg.Root), "node_modules", filepath.FromSlash(fixture.Binding))),
		})
	}
	sort.Slice(fixtures, func(i, j int) bool { return fixtures[i].Identity < fixtures[j].Identity })
	return DiscoveredTarget{
		Identity:        "test:" + pkg.Name + ":" + target.Name,
		Kind:            "test",
		Package:         pkg.Name,
		PublicationName: pkg.PublicationName,
		Root:            packageDisplayRoot(pkg.Root),
		Name:            target.Name,
		Tool:            target.Harness,
		Sources:         sources,
		HarnessProject:  target.Project,
		Requirements:    requirements,
		Fixtures:        fixtures,
	}
}

func dependencySourceIdentity(source manifest.Source) string {
	if source.Kind == "workspace" {
		if source.Name != "" {
			return "workspace:" + source.Name
		}
		return "workspace:" + source.Package
	}
	if source.Kind == "path" {
		return "path:" + filepath.ToSlash(source.Path)
	}
	return source.Kind + ":" + source.Package
}

func packageDisplayRoot(root string) string {
	if strings.TrimSpace(root) == "" {
		return "."
	}
	return root
}
