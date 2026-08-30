package lockfile

import (
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/packageidentity"
	"github.com/yuechen-li-dev/tspack/internal/pathutil"
)

const (
	FormatVersion = 1
	ToolName      = "tspack"
)

type Lockfile struct {
	Lock          LockHeader           `toml:"lock"`
	Packages      []Package            `toml:"package,omitempty"`
	Instances     []ModuleInstance     `toml:"module_instance,omitempty"`
	RootInstances []RootModuleInstance `toml:"root_module_instance,omitempty"`
	Edges         []Edge               `toml:"edge,omitempty"`
	Requirements  []Requirement        `toml:"requirement,omitempty"`
	Targets       []Target             `toml:"target,omitempty"`
}

type ModuleInstance struct {
	ID            string               `toml:"ID" json:"id"`
	PackageID     string               `toml:"PackageID" json:"packageId"`
	PeerContextID string               `toml:"PeerContextID" json:"peerContextId"`
	Peers         []PeerBinding        `toml:"peer,omitempty" json:"peers,omitempty"`
	Dependencies  []InstanceDependency `toml:"dependency,omitempty" json:"dependencies,omitempty"`
}

type InstanceDependency struct {
	Reference  string `toml:"Reference" json:"reference"`
	Kind       string `toml:"Kind" json:"kind"`
	InstanceID string `toml:"InstanceID" json:"instanceId"`
}

type RootModuleInstance struct {
	From       string `toml:"From" json:"from"`
	Reference  string `toml:"Reference" json:"reference"`
	Kind       string `toml:"Kind" json:"kind"`
	InstanceID string `toml:"InstanceID" json:"instanceId"`
}

type PeerBinding struct {
	Reference     string `toml:"Reference" json:"reference"`
	Source        string `toml:"Source" json:"source"`
	Name          string `toml:"Name" json:"name"`
	RealizationID string `toml:"RealizationID,omitempty" json:"realizationIdentity,omitempty"`
	InstanceID    string `toml:"InstanceID,omitempty" json:"moduleInstanceIdentity,omitempty"`
	State         string `toml:"State" json:"state"`
	Optional      bool   `toml:"Optional,omitempty" json:"optional,omitempty"`
}
type LockHeader struct {
	Format            int    `toml:"format"`
	Tool, GeneratedBy string `toml:"tool,omitempty" toml:"generated_by,omitempty"`
}

type Package struct {
	ID, Name, Version, Source, Integrity, Repo, Rev, TreeHash, Path, Workspace, Hash string
	SourceID                                                                         string       `toml:"SourceID,omitempty"`
	SourceHash                                                                       string       `toml:"SourceHash,omitempty"`
	RealizationID                                                                    string       `toml:"RealizationID,omitempty"`
	RegistryEndpoint                                                                 string       `toml:"registry_endpoint,omitempty"`
	MetadataEndpoint                                                                 string       `toml:"metadata_endpoint,omitempty"`
	ArtifactHost                                                                     string       `toml:"artifact_host,omitempty"`
	Capabilities                                                                     []Capability `toml:"capability,omitempty"`
	Patch                                                                            *Patch       `toml:"patch,omitempty"`
}
type Patch struct {
	Path      string `toml:"path" json:"path"`
	SHA256    string `toml:"sha256" json:"sha256"`
	Algorithm string `toml:"algorithm" json:"algorithm"`
}
type Capability struct {
	Kind    string `toml:"kind"`
	Script  string `toml:"script,omitempty"`
	Command string `toml:"command,omitempty"`
	Detail  string `toml:"detail,omitempty"`
}
type Edge struct {
	From, To, Kind string
	Optional       bool
	Reference      string
}
type Requirement struct {
	ID, Scope, TargetSource, TargetName, Reference, Constraint, Kind string
	RequiringPackage, PackageID, DependencyKey, OriginSource         string
	Order                                                            int
	Optional, Controlling                                            bool
	ShadowedBy, Status, SelectedVersion                              string
}
type Target struct{ Package, Name, Export, Entry, Runtime, Types string }

type ConsistencyResult struct{ Diagnostics []diag.Diagnostic }

type VersionConflictResult struct{ Diagnostics []diag.Diagnostic }

type RequirementCheckResult struct{ Diagnostics []diag.Diagnostic }

type Diff struct {
	PackagesAdded, PackagesRemoved         []Package
	PackagesChanged                        []PackageChange
	InstancesAdded, InstancesRemoved       []ModuleInstance
	InstancesChanged                       []ModuleInstanceChange
	RequirementsAdded, RequirementsRemoved []Requirement
	RequirementsChanged                    []RequirementChange
	EdgesAdded, EdgesRemoved               []Edge
	TargetsAdded, TargetsRemoved           []Target
	TargetsChanged                         []TargetChange
}
type PackageChange struct{ Old, New Package }
type ModuleInstanceChange struct{ Old, New ModuleInstance }
type RequirementChange struct{ Old, New Requirement }
type TargetChange struct{ Old, New Target }

func Marshal(lf *Lockfile) ([]byte, error) {
	b, e := toml.Marshal(normalize(lf))
	if e != nil {
		return nil, e
	}
	if len(b) == 0 || b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	return b, nil
}
func Parse(filename string, data []byte) (*Lockfile, []diag.Diagnostic) {
	lf := &Lockfile{}
	if err := toml.Unmarshal(data, lf); err != nil {
		return &Lockfile{}, []diag.Diagnostic{{Code: "TSPACK_LOCK_INVALID_TOML", Severity: diag.SeverityError, Message: "invalid lockfile TOML", File: filename, Details: []string{err.Error()}}}
	}
	return normalize(lf), validate(filename, lf)
}
func LoadFile(path string) (*Lockfile, []diag.Diagnostic, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, nil, e
	}
	lf, d := Parse(path, b)
	return lf, d, nil
}

func CheckGraphConsistency(g *graph.WorkspaceGraph, lf *Lockfile) ConsistencyResult {
	out := ConsistencyResult{}
	if g == nil || lf == nil {
		return out
	}
	lm := map[string]Target{}
	for _, t := range lf.Targets {
		lm[t.Package+"::"+t.Name] = t
	}
	for _, p := range g.AllPackages() {
		for _, t := range p.AllTargets() {
			k := p.Name + "::" + t.Name
			lt, ok := lm[k]
			if !ok {
				out.Diagnostics = append(out.Diagnostics, errD("TSPACK_LOCK_STALE_TARGET_MISSING", "graph target missing in lockfile", p.Name, t.Name))
				continue
			}
			if lt.Export != t.Export || lt.Entry != t.Entry || lt.Runtime != t.Runtime || lt.Types != t.Types {
				out.Diagnostics = append(out.Diagnostics, errD("TSPACK_LOCK_STALE_TARGET_MISMATCH", "graph target differs from lockfile target", p.Name, t.Name))
			}
		}
		for _, testTarget := range p.AllTestTargets() {
			targetIdentity := p.Name + ":test:" + testTarget.Name
			expectedRequirements := map[string]struct{}{}
			for _, requirement := range testTarget.Requirements {
				expectedRequirements[requirement.Key] = struct{}{}
			}
			lockedRequirements := map[string]struct{}{}
			for _, edge := range lf.Edges {
				if edge.From == targetIdentity && edge.Kind == "test" {
					lockedRequirements[edge.Reference] = struct{}{}
				}
			}
			for requirement := range expectedRequirements {
				if _, ok := lockedRequirements[requirement]; !ok {
					out.Diagnostics = append(out.Diagnostics, errD("TSPACK_LOCK_STALE_TEST_REQUIREMENT_MISSING", "test target requirement missing in lockfile", targetIdentity, requirement))
				}
			}
			for requirement := range lockedRequirements {
				if _, ok := expectedRequirements[requirement]; !ok {
					out.Diagnostics = append(out.Diagnostics, errD("TSPACK_LOCK_STALE_TEST_REQUIREMENT_UNEXPECTED", "lockfile contains a requirement no longer declared by the test target", targetIdentity, requirement))
				}
			}
		}
	}
	for _, t := range lf.Targets {
		p, ok := g.Package(t.Package)
		if !ok {
			out.Diagnostics = append(out.Diagnostics, errD("TSPACK_LOCK_UNKNOWN_TARGET", "lockfile target package not present in graph", t.Package, t.Name))
			continue
		}
		if _, ok := p.Target(t.Name); !ok {
			out.Diagnostics = append(out.Diagnostics, errD("TSPACK_LOCK_UNKNOWN_TARGET", "lockfile target not present in graph", t.Package, t.Name))
		}
	}
	diag.SortDiagnostics(out.Diagnostics)
	return out
}

func CheckVersionConflicts(lf *Lockfile) VersionConflictResult {
	out := VersionConflictResult{}
	if lf == nil {
		return out
	}

	type groupKey struct {
		Source string
		Name   string
	}

	versionsByGroup := map[groupKey]map[string][]string{}
	for _, pkg := range lf.Packages {
		if pkg.Source == "" || pkg.Name == "" || pkg.Version == "" {
			continue
		}

		key := groupKey{Source: pkg.Source, Name: pkg.Name}
		versions, ok := versionsByGroup[key]
		if !ok {
			versions = map[string][]string{}
			versionsByGroup[key] = versions
		}
		versions[pkg.Version] = append(versions[pkg.Version], pkg.ID)
	}

	keys := make([]groupKey, 0, len(versionsByGroup))
	for key, versions := range versionsByGroup {
		if len(versions) > 1 {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Name != keys[j].Name {
			return keys[i].Name < keys[j].Name
		}
		return keys[i].Source < keys[j].Source
	})

	for _, key := range keys {
		versions := versionsByGroup[key]
		versionNames := make([]string, 0, len(versions))
		for version := range versions {
			versionNames = append(versionNames, version)
		}
		sort.Strings(versionNames)

		details := []string{
			"source: " + key.Source,
			"versions:",
		}
		for _, version := range versionNames {
			packageIDs := append([]string(nil), versions[version]...)
			sort.Strings(packageIDs)
			for _, packageID := range packageIDs {
				details = append(details, "  "+version+" -> "+packageID)
			}
		}
		details = append(details, "This can be valid, but may indicate duplicated runtime dependencies or peer version drift.")

		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{
			Code:     "TSPACK_LOCK_VERSION_CONFLICT",
			Severity: diag.SeverityWarning,
			Message:  "package \"" + key.Name + "\" appears at multiple versions",
			Details:  details,
		})
	}

	return out
}

func CheckRequirements(lf *Lockfile) RequirementCheckResult {
	out := RequirementCheckResult{}
	if lf == nil {
		return out
	}
	for _, requirement := range lf.Requirements {
		if requirement.Status != "overridden-incompatible" {
			continue
		}
		code := "TSPACK_REQUIREMENT_OVERRIDDEN"
		if requirement.Kind == "peer" {
			code = "TSPACK_PEER_REQUIREMENT_OVERRIDDEN"
		}
		origin := requirement.PackageID
		if origin == "" {
			origin = requirement.RequiringPackage
		}
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{
			Code:     code,
			Severity: diag.SeverityWarning,
			Message:  "selected package version overrides an incompatible requirement",
			Details: []string{
				"origin: " + origin,
				"target: " + requirement.TargetSource + ":" + requirement.TargetName,
				"constraint: " + requirement.Constraint,
				"selectedVersion: " + requirement.SelectedVersion,
				"controllingRequirement: " + requirement.ShadowedBy,
			},
		})
	}
	diag.SortDiagnostics(out.Diagnostics)
	return out
}

func DiffLockfiles(old, next *Lockfile) Diff {
	d := Diff{}
	o, n := normalize(old), normalize(next)
	om, nm := map[string]Package{}, map[string]Package{}
	for _, p := range o.Packages {
		om[p.ID] = p
	}
	for _, p := range n.Packages {
		nm[p.ID] = p
	}
	for id, p := range nm {
		if op, ok := om[id]; !ok {
			d.PackagesAdded = append(d.PackagesAdded, p)
		} else if !reflect.DeepEqual(op, p) {
			d.PackagesChanged = append(d.PackagesChanged, PackageChange{op, p})
		}
	}
	for id, p := range om {
		if _, ok := nm[id]; !ok {
			d.PackagesRemoved = append(d.PackagesRemoved, p)
		}
	}
	omInstances := map[string]ModuleInstance{}
	nmInstances := map[string]ModuleInstance{}
	for _, instance := range o.Instances {
		omInstances[instance.ID] = instance
	}
	for _, instance := range n.Instances {
		nmInstances[instance.ID] = instance
	}
	for id, instance := range nmInstances {
		previous, exists := omInstances[id]
		if !exists {
			d.InstancesAdded = append(d.InstancesAdded, instance)
		} else if !reflect.DeepEqual(previous, instance) {
			d.InstancesChanged = append(d.InstancesChanged, ModuleInstanceChange{Old: previous, New: instance})
		}
	}
	for id, instance := range omInstances {
		if _, exists := nmInstances[id]; !exists {
			d.InstancesRemoved = append(d.InstancesRemoved, instance)
		}
	}
	omRequirements := map[string]Requirement{}
	nmRequirements := map[string]Requirement{}
	for _, requirement := range o.Requirements {
		omRequirements[requirement.ID] = requirement
	}
	for _, requirement := range n.Requirements {
		nmRequirements[requirement.ID] = requirement
	}
	for id, requirement := range nmRequirements {
		oldRequirement, exists := omRequirements[id]
		if !exists {
			d.RequirementsAdded = append(d.RequirementsAdded, requirement)
		} else if !reflect.DeepEqual(oldRequirement, requirement) {
			d.RequirementsChanged = append(d.RequirementsChanged, RequirementChange{Old: oldRequirement, New: requirement})
		}
	}
	for id, requirement := range omRequirements {
		if _, exists := nmRequirements[id]; !exists {
			d.RequirementsRemoved = append(d.RequirementsRemoved, requirement)
		}
	}
	kE := func(e Edge) string {
		return e.From + "|" + e.To + "|" + e.Kind + "|" + boolStr(e.Optional) + "|" + e.Reference
	}
	oe, ne := map[string]Edge{}, map[string]Edge{}
	for _, e := range o.Edges {
		oe[kE(e)] = e
	}
	for _, e := range n.Edges {
		ne[kE(e)] = e
	}
	for k, e := range ne {
		if _, ok := oe[k]; !ok {
			d.EdgesAdded = append(d.EdgesAdded, e)
		}
	}
	for k, e := range oe {
		if _, ok := ne[k]; !ok {
			d.EdgesRemoved = append(d.EdgesRemoved, e)
		}
	}
	kT := func(t Target) string { return t.Package + "::" + t.Name }
	ot, nt := map[string]Target{}, map[string]Target{}
	for _, t := range o.Targets {
		ot[kT(t)] = t
	}
	for _, t := range n.Targets {
		nt[kT(t)] = t
	}
	for k, t := range nt {
		if otv, ok := ot[k]; !ok {
			d.TargetsAdded = append(d.TargetsAdded, t)
		} else if otv != t {
			d.TargetsChanged = append(d.TargetsChanged, TargetChange{otv, t})
		}
	}
	for k, t := range ot {
		if _, ok := nt[k]; !ok {
			d.TargetsRemoved = append(d.TargetsRemoved, t)
		}
	}
	sort.SliceStable(d.PackagesAdded, func(i, j int) bool { return d.PackagesAdded[i].ID < d.PackagesAdded[j].ID })
	sort.SliceStable(d.PackagesRemoved, func(i, j int) bool { return d.PackagesRemoved[i].ID < d.PackagesRemoved[j].ID })
	sort.SliceStable(d.PackagesChanged, func(i, j int) bool { return d.PackagesChanged[i].Old.ID < d.PackagesChanged[j].Old.ID })
	sort.SliceStable(d.InstancesAdded, func(i, j int) bool { return d.InstancesAdded[i].ID < d.InstancesAdded[j].ID })
	sort.SliceStable(d.InstancesRemoved, func(i, j int) bool { return d.InstancesRemoved[i].ID < d.InstancesRemoved[j].ID })
	sort.SliceStable(d.InstancesChanged, func(i, j int) bool { return d.InstancesChanged[i].Old.ID < d.InstancesChanged[j].Old.ID })
	sort.SliceStable(d.RequirementsAdded, func(i, j int) bool { return d.RequirementsAdded[i].ID < d.RequirementsAdded[j].ID })
	sort.SliceStable(d.RequirementsRemoved, func(i, j int) bool { return d.RequirementsRemoved[i].ID < d.RequirementsRemoved[j].ID })
	sort.SliceStable(d.RequirementsChanged, func(i, j int) bool { return d.RequirementsChanged[i].Old.ID < d.RequirementsChanged[j].Old.ID })
	sort.SliceStable(d.EdgesAdded, func(i, j int) bool { return kE(d.EdgesAdded[i]) < kE(d.EdgesAdded[j]) })
	sort.SliceStable(d.EdgesRemoved, func(i, j int) bool { return kE(d.EdgesRemoved[i]) < kE(d.EdgesRemoved[j]) })
	sort.SliceStable(d.TargetsAdded, func(i, j int) bool { return kT(d.TargetsAdded[i]) < kT(d.TargetsAdded[j]) })
	sort.SliceStable(d.TargetsRemoved, func(i, j int) bool { return kT(d.TargetsRemoved[i]) < kT(d.TargetsRemoved[j]) })
	sort.SliceStable(d.TargetsChanged, func(i, j int) bool { return kT(d.TargetsChanged[i].Old) < kT(d.TargetsChanged[j].Old) })
	return d
}

func validate(file string, lf *Lockfile) []diag.Diagnostic {
	var out []diag.Diagnostic
	if lf.Lock.Format == 0 {
		out = append(out, errD("TSPACK_LOCK_MISSING_HEADER", "missing [lock] header"))
	} else if lf.Lock.Format != FormatVersion {
		out = append(out, errD("TSPACK_LOCK_UNSUPPORTED_FORMAT", "unsupported lockfile format", "expected 1"))
	}
	if lf.Lock.Tool != "" && lf.Lock.Tool != ToolName {
		out = append(out, errD("TSPACK_LOCK_INVALID_TOOL", "invalid lock tool", lf.Lock.Tool))
	}
	ids := map[string]struct{}{}
	for _, p := range lf.Packages {
		if p.ID == "" || p.Name == "" || p.Source == "" {
			out = append(out, errD("TSPACK_LOCK_INVALID_PACKAGE", "package requires id/name/source", p.ID))
			continue
		}
		if _, ok := ids[p.ID]; ok {
			out = append(out, errD("TSPACK_LOCK_DUPLICATE_PACKAGE", "duplicate package id", p.ID))
		}
		ids[p.ID] = struct{}{}
		// Registry packages share the existing version plus integrity/content-hash
		// contract; source-qualified IDs keep registry identities distinct.
		switch p.Source {
		case "npm", "jsr":
			if p.Version == "" || (p.Integrity == "" && p.Hash == "") {
				out = append(out, errD("TSPACK_LOCK_INVALID_PACKAGE", "registry package requires version and integrity/hash", p.ID))
			}
			expectedID := p.Source + ":" + p.Name + "@" + p.Version
			validID := p.ID == expectedID
			if p.Patch != nil {
				expectedRealizationID := expectedID + "#patch=" + p.Patch.Algorithm + "." + strings.TrimPrefix(p.Patch.SHA256, "sha256:")
				validID = p.SourceID == expectedID && p.RealizationID == p.ID && p.ID == expectedRealizationID
				if !pathutil.IsSafePackageFilePath(p.Patch.Path) ||
					!validSHA256(p.Patch.SHA256) ||
					p.Patch.Algorithm == "" ||
					p.SourceHash == "" ||
					p.Hash == "" {
					out = append(out, errD(
						"TSPACK_LOCK_INVALID_PATCH",
						"patched registry package requires safe path, SHA-256 digest, algorithm, source hash, and realization hash",
						p.ID,
					))
				}
			}
			if !validID {
				out = append(out, errD(
					"TSPACK_LOCK_PACKAGE_IDENTITY_MISMATCH",
					"registry lock package ID does not match its source-qualified semantic identity",
					"package: "+p.ID,
					"expected: "+expectedID,
				))
			}
		case "git":
			if p.Repo == "" || p.Rev == "" || (p.TreeHash == "" && p.Hash == "") {
				out = append(out, errD("TSPACK_LOCK_INVALID_PACKAGE", "git package requires repo/rev/tree_hash/hash", p.ID))
			}
		case "path":
			if !pathutil.IsSafePackageFilePath(p.Path) {
				out = append(out, errD("TSPACK_LOCK_INVALID_PATH", "path package path must be safe relative", p.ID))
			}
		case "workspace":
			if p.Workspace == "" && p.Path == "" {
				out = append(out, errD("TSPACK_LOCK_INVALID_PACKAGE", "workspace package missing identifiers", p.ID))
			}
		default:
			out = append(out, errD("TSPACK_LOCK_INVALID_SOURCE", "invalid package source", p.Source, p.ID))
		}
	}
	seenInstances := map[string]struct{}{}
	for _, instance := range lf.Instances {
		if instance.ID == "" || instance.PackageID == "" || instance.PeerContextID == "" {
			out = append(out, errD("TSPACK_LOCK_INVALID_MODULE_INSTANCE", "module instance requires id, package realization, and peer context identity", instance.ID))
			continue
		}
		if _, ok := ids[instance.PackageID]; !ok {
			out = append(out, errD("TSPACK_LOCK_MODULE_INSTANCE_UNKNOWN_PACKAGE", "module instance refers to an unknown package realization", instance.ID, instance.PackageID))
		}
		if _, exists := seenInstances[instance.ID]; exists {
			out = append(out, errD("TSPACK_LOCK_DUPLICATE_MODULE_INSTANCE", "duplicate module instance identity", instance.ID))
		}
		seenInstances[instance.ID] = struct{}{}
		bindings := make([]packageidentity.PeerBinding, 0, len(instance.Peers))
		for _, peer := range instance.Peers {
			bindings = append(bindings, packageidentity.PeerBinding{Reference: peer.Reference, Source: peer.Source, Name: peer.Name, RealizationID: peer.RealizationID, Optional: peer.Optional, State: peer.State})
			if peer.Reference == "" || peer.Source == "" || peer.Name == "" || (peer.State != packageidentity.PeerBindingPresent && peer.State != packageidentity.PeerBindingAbsent) {
				out = append(out, errD("TSPACK_LOCK_INVALID_PEER_BINDING", "module-instance peer binding is incomplete", instance.ID, peer.Reference))
			}
			if peer.State == packageidentity.PeerBindingPresent {
				if _, ok := ids[peer.RealizationID]; !ok {
					out = append(out, errD("TSPACK_LOCK_PEER_BINDING_UNKNOWN_PACKAGE", "present peer binding refers to an unknown realization", instance.ID, peer.RealizationID))
				}
			}
		}
		expected := packageidentity.NewModuleInstance(instance.PackageID, bindings)
		if expected.ID != instance.ID || expected.PeerContext.ID != instance.PeerContextID {
			out = append(out, errD("TSPACK_LOCK_MODULE_INSTANCE_IDENTITY_MISMATCH", "module instance identity does not match its realization and canonical peer context", instance.ID, "expected: "+expected.ID))
		}
	}
	seenE := map[string]struct{}{}
	for _, e := range lf.Edges {
		if e.From == "" || e.To == "" || e.Kind == "" {
			out = append(out, errD("TSPACK_LOCK_INVALID_EDGE", "edge requires from/to/kind"))
			continue
		}
		if !validEdgeKind(e.Kind) {
			out = append(out, errD("TSPACK_LOCK_INVALID_EDGE", "invalid edge kind", e.Kind))
		}
		if _, ok := ids[e.To]; !ok {
			out = append(out, errD("TSPACK_LOCK_UNKNOWN_PACKAGE_REF", "edge to unknown package", e.To))
		}
		k := e.From + "|" + e.To + "|" + e.Kind + "|" + boolStr(e.Optional) + "|" + e.Reference
		if _, ok := seenE[k]; ok {
			out = append(out, errD("TSPACK_LOCK_DUPLICATE_EDGE", "duplicate edge", k))
		}
		seenE[k] = struct{}{}
	}
	seenRequirements := map[string]struct{}{}
	for _, requirement := range lf.Requirements {
		if requirement.ID == "" || requirement.Scope == "" || requirement.TargetSource == "" || requirement.TargetName == "" || requirement.Constraint == "" || requirement.Kind == "" || requirement.Status == "" {
			out = append(out, errD("TSPACK_LOCK_INVALID_REQUIREMENT", "requirement requires id/scope/target/constraint/kind/status", requirement.ID))
			continue
		}
		if requirement.TargetSource != "npm" && requirement.TargetSource != "jsr" {
			out = append(out, errD("TSPACK_LOCK_INVALID_REQUIREMENT", "requirement target source is not supported", requirement.ID, requirement.TargetSource))
		}
		if !validRequirementKind(requirement.Kind) || !validRequirementStatus(requirement.Status) {
			out = append(out, errD("TSPACK_LOCK_INVALID_REQUIREMENT", "requirement kind or status is invalid", requirement.ID, requirement.Kind, requirement.Status))
		}
		if _, exists := seenRequirements[requirement.ID]; exists {
			out = append(out, errD("TSPACK_LOCK_DUPLICATE_REQUIREMENT", "duplicate requirement id", requirement.ID))
		}
		seenRequirements[requirement.ID] = struct{}{}
	}
	tk, ek := map[string]struct{}{}, map[string]struct{}{}
	for _, t := range lf.Targets {
		if t.Package == "" || t.Name == "" || t.Export == "" || t.Entry == "" || t.Runtime == "" {
			out = append(out, errD("TSPACK_LOCK_INVALID_TARGET", "target requires package/name/export/entry/runtime", t.Package, t.Name))
			continue
		}
		if t.Export != "." && !strings.HasPrefix(t.Export, "./") {
			out = append(out, errD("TSPACK_LOCK_INVALID_TARGET", "target export must be . or ./ prefixed", t.Package, t.Name))
		}
		if !pathutil.IsSafePackageFilePath(t.Entry) || !pathutil.IsSafePackageFilePath(t.Runtime) || (t.Types != "" && !pathutil.IsSafePackageFilePath(t.Types)) {
			out = append(out, errD("TSPACK_LOCK_INVALID_PATH", "target paths must be safe relative", t.Package, t.Name))
		}
		k := t.Package + "::" + t.Name
		if _, ok := tk[k]; ok {
			out = append(out, errD("TSPACK_LOCK_DUPLICATE_TARGET", "duplicate target", k))
		}
		tk[k] = struct{}{}
		ex := t.Package + "::" + t.Export
		if _, ok := ek[ex]; ok {
			out = append(out, errD("TSPACK_LOCK_DUPLICATE_EXPORT", "duplicate export", ex))
		}
		ek[ex] = struct{}{}
	}
	diag.SortDiagnostics(out)
	return out
}

func validSHA256(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, character := range strings.TrimPrefix(value, "sha256:") {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func normalize(lf *Lockfile) *Lockfile {
	if lf == nil {
		return &Lockfile{Lock: LockHeader{Format: FormatVersion, Tool: ToolName}}
	}
	n := *lf
	n.Packages = append([]Package(nil), lf.Packages...)
	n.Instances = append([]ModuleInstance(nil), lf.Instances...)
	n.RootInstances = append([]RootModuleInstance(nil), lf.RootInstances...)
	n.Edges = append([]Edge(nil), lf.Edges...)
	n.Requirements = append([]Requirement(nil), lf.Requirements...)
	n.Targets = append([]Target(nil), lf.Targets...)
	for i := range n.Packages {
		n.Packages[i].Capabilities = append([]Capability(nil), n.Packages[i].Capabilities...)
		for capabilityIndex := range n.Packages[i].Capabilities {
			capability := &n.Packages[i].Capabilities[capabilityIndex]
			if capability.Script == "" && capability.Detail != "" {
				capability.Script = capability.Detail
			}
			if capability.Kind == "lifecycle-script" {
				capability.Kind = "lifecycleScript"
			}
			capability.Detail = ""
		}
		sort.SliceStable(n.Packages[i].Capabilities, func(a, b int) bool {
			x, y := n.Packages[i].Capabilities[a], n.Packages[i].Capabilities[b]
			if x.Kind != y.Kind {
				return x.Kind < y.Kind
			}
			if x.Script != y.Script {
				return x.Script < y.Script
			}
			return x.Command < y.Command
		})
	}
	for index := range n.Instances {
		n.Instances[index].Peers = append([]PeerBinding(nil), n.Instances[index].Peers...)
		n.Instances[index].Dependencies = append([]InstanceDependency(nil), n.Instances[index].Dependencies...)
		sort.SliceStable(n.Instances[index].Peers, func(left, right int) bool {
			return peerBindingSortKey(n.Instances[index].Peers[left]) < peerBindingSortKey(n.Instances[index].Peers[right])
		})
		sort.SliceStable(n.Instances[index].Dependencies, func(left, right int) bool {
			leftDependency := n.Instances[index].Dependencies[left]
			rightDependency := n.Instances[index].Dependencies[right]
			return leftDependency.Reference+"\x00"+leftDependency.Kind+"\x00"+leftDependency.InstanceID < rightDependency.Reference+"\x00"+rightDependency.Kind+"\x00"+rightDependency.InstanceID
		})
	}
	sort.SliceStable(n.Packages, func(i, j int) bool { return n.Packages[i].ID < n.Packages[j].ID })
	sort.SliceStable(n.Instances, func(i, j int) bool { return n.Instances[i].ID < n.Instances[j].ID })
	sort.SliceStable(n.RootInstances, func(i, j int) bool {
		left := n.RootInstances[i]
		right := n.RootInstances[j]
		return left.From+"\x00"+left.Reference+"\x00"+left.Kind+"\x00"+left.InstanceID < right.From+"\x00"+right.Reference+"\x00"+right.Kind+"\x00"+right.InstanceID
	})
	sort.SliceStable(n.Edges, func(i, j int) bool {
		a, b := n.Edges[i], n.Edges[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.To != b.To {
			return a.To < b.To
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Reference != b.Reference {
			return a.Reference < b.Reference
		}
		return !a.Optional && b.Optional
	})
	sort.SliceStable(n.Requirements, func(i, j int) bool {
		a, b := n.Requirements[i], n.Requirements[j]
		if a.Scope != b.Scope {
			return a.Scope < b.Scope
		}
		if a.TargetSource != b.TargetSource {
			return a.TargetSource < b.TargetSource
		}
		if a.TargetName != b.TargetName {
			return a.TargetName < b.TargetName
		}
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		return a.ID < b.ID
	})
	sort.SliceStable(n.Targets, func(i, j int) bool {
		a, b := n.Targets[i], n.Targets[j]
		if a.Package != b.Package {
			return a.Package < b.Package
		}
		return a.Name < b.Name
	})
	return &n
}

func peerBindingSortKey(binding PeerBinding) string {
	return binding.Reference + "\x00" + binding.Source + "\x00" + binding.Name + "\x00" + binding.State + "\x00" + binding.RealizationID + "\x00" + binding.InstanceID + "\x00" + boolStr(binding.Optional)
}
func errD(c, m string, details ...string) diag.Diagnostic {
	return diag.Diagnostic{Code: c, Severity: diag.SeverityError, Message: m, Details: details}
}
func validEdgeKind(k string) bool {
	switch k {
	case "runtime", "dep", "peer", "optionalPeer", "tool", "type", "test", "workspace":
		return true
	}
	return false
}

func validRequirementKind(kind string) bool {
	switch kind {
	case "transitive-runtime", "transitive-optional", "peer", "package-explicit", "project-explicit", "target-explicit", "override":
		return true
	}
	return false
}

func validRequirementStatus(status string) bool {
	switch status {
	case "pending", "controlling", "satisfied", "shadowed-compatible", "overridden-incompatible", "optional-unsatisfied", "invalid":
		return true
	}
	return false
}
func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
