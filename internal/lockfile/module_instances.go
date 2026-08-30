package lockfile

import (
	"sort"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/packageidentity"
)

// RebuildModuleInstances derives peer-qualified module instances by traversing
// the authoritative dependency graph from each root consumer. Immutable store
// realizations remain shared while effective peer environments may diverge.
func RebuildModuleInstances(lock *Lockfile) {
	if lock == nil {
		return
	}
	packages := map[string]Package{}
	for _, pkg := range lock.Packages {
		packages[pkg.ID] = pkg
	}
	rootEdges := moduleRootEdges(lock.Edges, packages)
	rootEnvironments := moduleRootEnvironments(rootEdges, lock, packages)
	builder := moduleInstanceBuilder{
		packages:         packages,
		peerRequirements: peerRequirementsByPackage(lock.Requirements),
		edgesByPackage:   packageEdgesByFrom(lock.Edges, packages),
		instances:        map[string]*ModuleInstance{},
		visiting:         map[string]bool{},
	}
	rootInstances := []RootModuleInstance{}
	for _, edge := range rootEdges {
		environment := rootEnvironments[moduleRootConsumer(edge.From)]
		instanceID := builder.instantiate(edge.To, environment)
		if instanceID != "" {
			rootInstances = append(rootInstances, RootModuleInstance{From: edge.From, Reference: moduleReference(edge.Reference, packages[edge.To]), Kind: edge.Kind, InstanceID: instanceID})
		}
	}
	fallbackEnvironment := rootEnvironments[""]
	if fallbackEnvironment == nil {
		fallbackEnvironment = moduleEnvironment{}
	}
	for _, pkg := range lock.Packages {
		if !builder.hasPackageInstance(pkg.ID) {
			builder.instantiate(pkg.ID, fallbackEnvironment)
		}
	}
	instances := make([]ModuleInstance, 0, len(builder.instances))
	for _, instance := range builder.instances {
		instances = append(instances, *instance)
	}
	lock.Instances = instances
	lock.RootInstances = rootInstances
}

func moduleRootEnvironments(rootEdges []Edge, lock *Lockfile, packages map[string]Package) map[string]moduleEnvironment {
	environments := map[string]moduleEnvironment{}
	for _, edge := range rootEdges {
		consumer := moduleRootConsumer(edge.From)
		environment := environments[consumer]
		if environment == nil {
			environment = moduleEnvironment{}
			environments[consumer] = environment
		}
		environment.bind(moduleReference(edge.Reference, packages[edge.To]), packages[edge.To])
	}
	if len(environments) == 0 {
		environments[""] = moduleEnvironment{}
	}

	peerEdges := moduleRootPeerEdges(lock.Edges, packages)
	for _, environment := range environments {
		for _, edge := range peerEdges {
			environment.bindIfAbsent(moduleReference(edge.Reference, packages[edge.To]), packages[edge.To])
		}
		for _, requirement := range lock.Requirements {
			if requirement.Kind != "peer" || requirement.SelectedVersion == "" {
				continue
			}
			for _, pkg := range lock.Packages {
				if pkg.Source == requirement.TargetSource && pkg.Name == requirement.TargetName && pkg.Version == requirement.SelectedVersion {
					environment.bindIfAbsent(requirement.Reference, pkg)
					break
				}
			}
		}
	}
	return environments
}

func (builder *moduleInstanceBuilder) hasPackageInstance(packageID string) bool {
	for _, instance := range builder.instances {
		if instance.PackageID == packageID {
			return true
		}
	}
	return false
}

func moduleRootConsumer(from string) string {
	for _, separator := range []string{":target:", ":test:", ":dependency", ":tool"} {
		if index := strings.Index(from, separator); index >= 0 {
			return from[:index]
		}
	}
	return from
}

type moduleEnvironment map[string]string

func (environment moduleEnvironment) clone() moduleEnvironment {
	out := moduleEnvironment{}
	for key, value := range environment {
		out[key] = value
	}
	return out
}

func (environment moduleEnvironment) bind(reference string, pkg Package) {
	if strings.TrimSpace(reference) != "" {
		environment[reference] = pkg.ID
	}
	if _, exists := environment[pkg.Name]; !exists {
		environment[pkg.Name] = pkg.ID
	}
}

func (environment moduleEnvironment) bindIfAbsent(reference string, pkg Package) {
	if strings.TrimSpace(reference) != "" {
		if _, exists := environment[reference]; !exists {
			environment[reference] = pkg.ID
		}
	}
	if _, exists := environment[pkg.Name]; !exists {
		environment[pkg.Name] = pkg.ID
	}
}

type moduleInstanceBuilder struct {
	packages         map[string]Package
	peerRequirements map[string][]Requirement
	edgesByPackage   map[string][]Edge
	instances        map[string]*ModuleInstance
	visiting         map[string]bool
}

func (builder *moduleInstanceBuilder) instantiate(packageID string, environment moduleEnvironment) string {
	pkg, ok := builder.packages[packageID]
	if !ok {
		return ""
	}
	bindings := builder.effectivePeerBindings(packageID, environment)
	identityBindings := make([]packageidentity.PeerBinding, 0, len(bindings))
	for _, binding := range bindings {
		identityBindings = append(identityBindings, packageidentity.PeerBinding{Reference: binding.Reference, Source: binding.Source, Name: binding.Name, RealizationID: binding.RealizationID, Optional: binding.Optional, State: binding.State})
	}
	identity := packageidentity.NewModuleInstance(pkg.ID, identityBindings)
	if _, exists := builder.instances[identity.ID]; exists {
		return identity.ID
	}
	instance := &ModuleInstance{ID: identity.ID, PackageID: pkg.ID, PeerContextID: identity.PeerContext.ID, Peers: bindings}
	builder.instances[identity.ID] = instance
	if builder.visiting[identity.ID] {
		return identity.ID
	}
	builder.visiting[identity.ID] = true
	defer delete(builder.visiting, identity.ID)

	childEnvironment := environment.clone()
	for _, edge := range builder.edgesByPackage[packageID] {
		childEnvironment.bind(moduleReference(edge.Reference, builder.packages[edge.To]), builder.packages[edge.To])
	}
	for _, binding := range instance.Peers {
		if binding.State == packageidentity.PeerBindingPresent {
			childEnvironment.bind(binding.Reference, builder.packages[binding.RealizationID])
		}
	}
	for _, edge := range builder.edgesByPackage[packageID] {
		dependencyInstanceID := builder.instantiate(edge.To, childEnvironment)
		if dependencyInstanceID != "" {
			instance.Dependencies = append(instance.Dependencies, InstanceDependency{Reference: moduleReference(edge.Reference, builder.packages[edge.To]), Kind: edge.Kind, InstanceID: dependencyInstanceID})
		}
	}
	for index := range instance.Peers {
		binding := &instance.Peers[index]
		if binding.State == packageidentity.PeerBindingPresent {
			binding.InstanceID = builder.instantiate(binding.RealizationID, environment)
		}
	}
	return identity.ID
}

func (builder *moduleInstanceBuilder) effectivePeerBindings(packageID string, environment moduleEnvironment) []PeerBinding {
	bindings := []packageidentity.PeerBinding{}
	for _, requirement := range builder.peerRequirements[packageID] {
		binding := packageidentity.PeerBinding{Reference: requirement.Reference, Source: requirement.TargetSource, Name: requirement.TargetName, Optional: requirement.Optional, State: packageidentity.PeerBindingAbsent}
		providerID := environment[requirement.Reference]
		if providerID == "" {
			providerID = environment[requirement.TargetName]
		}
		if provider, ok := builder.packages[providerID]; ok && provider.Source == requirement.TargetSource && provider.Name == requirement.TargetName {
			binding.RealizationID = provider.ID
			binding.State = packageidentity.PeerBindingPresent
		}
		bindings = append(bindings, binding)
	}
	canonical := packageidentity.CanonicalPeerBindings(bindings)
	out := make([]PeerBinding, 0, len(canonical))
	for _, binding := range canonical {
		out = append(out, PeerBinding{Reference: binding.Reference, Source: binding.Source, Name: binding.Name, RealizationID: binding.RealizationID, State: binding.State, Optional: binding.Optional})
	}
	return out
}

func peerRequirementsByPackage(requirements []Requirement) map[string][]Requirement {
	out := map[string][]Requirement{}
	for _, requirement := range requirements {
		if requirement.Kind == "peer" && requirement.PackageID != "" {
			out[requirement.PackageID] = append(out[requirement.PackageID], requirement)
		}
	}
	for packageID := range out {
		sort.SliceStable(out[packageID], func(left, right int) bool {
			leftRequirement := out[packageID][left]
			rightRequirement := out[packageID][right]
			return leftRequirement.Reference+"\x00"+leftRequirement.TargetSource+"\x00"+leftRequirement.TargetName < rightRequirement.Reference+"\x00"+rightRequirement.TargetSource+"\x00"+rightRequirement.TargetName
		})
	}
	return out
}

func packageEdgesByFrom(edges []Edge, packages map[string]Package) map[string][]Edge {
	out := map[string][]Edge{}
	for _, edge := range edges {
		if _, packageFrom := packages[edge.From]; !packageFrom || edge.Kind == "peer" || edge.Kind == "optionalPeer" {
			continue
		}
		if _, packageTo := packages[edge.To]; packageTo {
			out[edge.From] = append(out[edge.From], edge)
		}
	}
	for packageID := range out {
		sort.SliceStable(out[packageID], func(left, right int) bool {
			leftEdge := out[packageID][left]
			rightEdge := out[packageID][right]
			return leftEdge.Reference+"\x00"+leftEdge.To+"\x00"+leftEdge.Kind < rightEdge.Reference+"\x00"+rightEdge.To+"\x00"+rightEdge.Kind
		})
	}
	return out
}

func moduleRootEdges(edges []Edge, packages map[string]Package) []Edge {
	out := []Edge{}
	for _, edge := range edges {
		if _, packageFrom := packages[edge.From]; packageFrom {
			continue
		}
		if _, packageTo := packages[edge.To]; !packageTo || edge.Kind == "peer" || edge.Kind == "optionalPeer" || strings.HasPrefix(edge.From, "workspace:peer:") {
			continue
		}
		out = append(out, edge)
	}
	sort.SliceStable(out, func(left, right int) bool {
		leftEdge := out[left]
		rightEdge := out[right]
		return leftEdge.From+"\x00"+leftEdge.Reference+"\x00"+leftEdge.To+"\x00"+leftEdge.Kind < rightEdge.From+"\x00"+rightEdge.Reference+"\x00"+rightEdge.To+"\x00"+rightEdge.Kind
	})
	return out
}

func moduleRootPeerEdges(edges []Edge, packages map[string]Package) []Edge {
	out := []Edge{}
	for _, edge := range edges {
		if !strings.HasPrefix(edge.From, "workspace:peer:") {
			continue
		}
		if _, ok := packages[edge.To]; ok {
			out = append(out, edge)
		}
	}
	sort.SliceStable(out, func(left, right int) bool {
		return out[left].From+"\x00"+out[left].To < out[right].From+"\x00"+out[right].To
	})
	return out
}

func moduleReference(reference string, pkg Package) string {
	if pkg.Source != "jsr" && strings.TrimSpace(reference) != "" {
		return reference
	}
	usage, err := packageidentity.NodeUsage(packageidentity.PackageIdentity{Source: pkg.Source, Name: pkg.Name})
	if err == nil {
		return usage.MaterializedAs.Name
	}
	return pkg.Name
}
