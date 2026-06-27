package concepts

import "fmt"

type Fragment struct {
	Name            string
	Description     string
	Provides        []string
	Requires        []string
	RequiresAnyOf   [][]string
	Optional        []string
	Conflicts       []string
	CompatibleKinds []string
	Variables       []VariableContribution
	Manifest        ManifestContributions
	Files           []FileContribution
	Projections     ProjectionContributions
	Slots           []SlotContribution
	Warnings        []string
}

type VariableContribution struct {
	Name        string
	Description string
	Default     string
	Allowed     []string
	Source      string
}

type ManifestContributions struct {
	Workspace      *WorkspaceContribution
	Package        *PackageContribution
	Dependencies   []DependencyContribution
	Tools          []DependencyContribution
	Peers          []DependencyContribution
	Targets        []TargetContribution
	RunTargets     []RunTargetContribution
	Env            []EnvContribution
	Services       []ServiceContribution
	UpdatePolicy   []PolicyContribution
	SecurityPolicy []PolicyContribution
	Pack           *PackContribution
	Concepts       []string
}

type WorkspaceContribution struct{ Name string }
type PackageContribution struct{ Name, Kind string }
type PackContribution struct{ Format, Artifact string }
type TargetContribution struct{ Name, Path string }

type DependencyContribution struct{ Name, Range, Source string }

type RunTargetContribution struct{ Name, Command, Cwd string }

type EnvContribution struct {
	Name, Default, Description string
	Required, Secret           bool
}

type ServiceContribution struct {
	Name, Protocol, Endpoint string
	Optional                 bool
}

type PolicyContribution struct{ Subject, Action, Range string }

type FileContribution struct {
	Path, Content string
	Rendered      bool
}

type ProjectionContributions struct{ Objects map[string]map[string]string }

type SlotMode string

const (
	SlotSingleOwner   SlotMode = "singleOwner"
	SlotAppendOrdered SlotMode = "appendOrdered"
	SlotMapByKey      SlotMode = "mapByKey"
)

type SlotContribution struct {
	Name    string
	Mode    SlotMode
	Owner   string
	Values  []string
	Entries map[string]string
}

type MergedConceptIR struct {
	Concepts    []string
	Variables   []VariableContribution
	Manifest    ManifestContributions
	Files       []FileContribution
	Projections ProjectionContributions
	Slots       []SlotContribution
	Warnings    []string
}

type Conflict struct{ Path, ConceptA, ConceptB, Reason string }

func (c Conflict) Error() string {
	return fmt.Sprintf("concept conflict at %s between %s and %s: %s", c.Path, c.ConceptA, c.ConceptB, c.Reason)
}

type DiagnosticError struct {
	Kind, Message string
	Conflicts     []Conflict
}

func (e DiagnosticError) Error() string { return e.Message }
