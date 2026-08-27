// Package authoring models dependency declarations before package resolution.
//
// Authoring declarations record what was requested. A dependency tape records
// deterministic precedence and provenance. EffectiveDependency is the narrow
// projection consumed by the existing graph and resolver path.
package authoring

import "strings"

type SourceKind string

const (
	SourceNPM       SourceKind = "npm"
	SourceWorkspace SourceKind = "workspace"
	SourceGit       SourceKind = "git"
	SourcePath      SourceKind = "path"
)

type PackageSource struct {
	Kind    string `json:"kind"`
	Package string `json:"package,omitempty"`
	Range   string `json:"range,omitempty"`
	Ref     string `json:"ref,omitempty"`
	Repo    string `json:"repo,omitempty"`
	Tag     string `json:"tag,omitempty"`
	Rev     string `json:"rev,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Path    string `json:"path,omitempty"`
	Name    string `json:"name,omitempty"`
}

type PackageIdentity struct {
	Source string `json:"source"`
	Name   string `json:"name"`
}

func (source PackageSource) Identity() PackageIdentity {
	name := source.Package
	if name == "" {
		name = source.Name
	}
	if name == "" {
		name = source.Ref
	}
	if name == "" {
		name = source.Path
	}
	if name == "" {
		name = source.Repo
	}
	return PackageIdentity{Source: source.Kind, Name: name}
}

func (identity PackageIdentity) Key() string {
	return identity.Source + ":" + identity.Name
}

func (identity PackageIdentity) Valid() bool {
	return strings.TrimSpace(identity.Source) != "" && strings.TrimSpace(identity.Name) != ""
}

type DependencyKind string

const (
	DependencyRuntime   DependencyKind = "dep"
	DependencyPeer      DependencyKind = "peer"
	DependencyTool      DependencyKind = "tool"
	DependencyType      DependencyKind = "type"
	DependencyTest      DependencyKind = "test"
	DependencyWorkspace DependencyKind = "workspace"
)

type OriginKind string

const (
	OriginProjectManifest       OriginKind = "project-manifest"
	OriginPackageManifest       OriginKind = "package-manifest"
	OriginConcept               OriginKind = "concept"
	OriginTemplate              OriginKind = "template"
	OriginCompatibility         OriginKind = "compatibility"
	OriginGenerated             OriginKind = "generated"
	OriginExplicitUserOperation OriginKind = "explicit-user-operation"
)

type DeclarationOrigin struct {
	Kind       OriginKind `json:"kind"`
	Name       string     `json:"name,omitempty"`
	SourcePath string     `json:"sourcePath,omitempty"`
	Ref        string     `json:"ref,omitempty"`
}

type DeclarationLayer string

const (
	LayerBase          DeclarationLayer = "base"
	LayerConcept       DeclarationLayer = "concept"
	LayerTemplate      DeclarationLayer = "template"
	LayerProject       DeclarationLayer = "project"
	LayerPackage       DeclarationLayer = "package"
	LayerCompatibility DeclarationLayer = "compatibility"
	LayerExplicit      DeclarationLayer = "explicit"
)

type DeclarationAuthority string

const (
	AuthorityOwned     DeclarationAuthority = "owned"
	AuthorityObserved  DeclarationAuthority = "observed"
	AuthorityGenerated DeclarationAuthority = "generated"
)

type DeclarationEditability string

const (
	EditabilityEditable     DeclarationEditability = "editable"
	EditabilityDerived      DeclarationEditability = "derived"
	EditabilityObserved     DeclarationEditability = "observed"
	EditabilityGenerated    DeclarationEditability = "generated"
	EditabilityConceptOwned DeclarationEditability = "concept-owned"
)

type DependencyDeclaration struct {
	ID          string                 `json:"id,omitempty"`
	Key         string                 `json:"key,omitempty"`
	Identity    PackageIdentity        `json:"identity"`
	Source      PackageSource          `json:"source"`
	Constraint  string                 `json:"constraint,omitempty"`
	Kind        DependencyKind         `json:"kind"`
	Optional    bool                   `json:"optional,omitempty"`
	Origin      DeclarationOrigin      `json:"origin"`
	Layer       DeclarationLayer       `json:"layer"`
	LayerOrder  int                    `json:"layerOrder,omitempty"`
	Order       int                    `json:"order"`
	Authority   DeclarationAuthority   `json:"authority"`
	Editability DeclarationEditability `json:"editability"`
}

type PackageIR struct {
	Declarations []DependencyDeclaration `json:"declarations"`
}

type EffectiveDependency struct {
	Key      string        `json:"key,omitempty"`
	Kind     string        `json:"kind"`
	Source   PackageSource `json:"source"`
	Optional bool          `json:"optional,omitempty"`
}

func (declaration DependencyDeclaration) EffectiveDependency() EffectiveDependency {
	source := declaration.Source
	if declaration.Constraint != "" {
		source.Range = declaration.Constraint
	}
	return EffectiveDependency{
		Key:      declaration.Key,
		Kind:     string(declaration.Kind),
		Source:   source,
		Optional: declaration.Optional,
	}
}

func EffectiveIdentity(dependency EffectiveDependency) string {
	if dependency.Key != "" {
		return dependency.Key
	}
	identity := dependency.Source.Identity()
	return identity.Name
}
