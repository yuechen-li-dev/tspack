// Package projectir contains small ecosystem-aware project/package IR shapes.
//
// The package is intentionally not wired into manifest parsing, resolution,
// sync, materialization, lockfile writing, templates, or CLI product behavior.
// It exists so internal tests can construct current TypeScript/npm package
// intent and reserved future Python-family/PyPI package intent without forcing
// Python-specific fields into npm target data.
package projectir

import "github.com/yuechen-li-dev/tspack/internal/ecosystem"

type ProjectPackage struct {
	Name           string
	Version        string
	Ecosystem      ecosystem.EcosystemID
	Backend        ecosystem.BackendKind
	Runtime        RuntimeSpec
	Dependencies   []Dependency
	Tools          []Dependency
	OptionalGroups []OptionalGroup
	RunTargets     []RunTargetRef
	BackendData    BackendData
}

type RuntimeSpec struct {
	Family         ecosystem.RuntimeFamily
	Implementation ecosystem.RuntimeImplementation
	Environment    ecosystem.EnvironmentKind
	ExecutionMode  ecosystem.ExecutionMode
}

type BackendData interface {
	BackendKind() ecosystem.BackendKind
}

type NpmPackageData struct {
	Targets []NpmTarget
}

func (NpmPackageData) BackendKind() ecosystem.BackendKind {
	return ecosystem.BackendTypeScriptNPM
}

type NpmTarget struct {
	Name    string
	Export  string
	Entry   string
	Runtime string
	Types   string
}

type PythonPackageData struct {
	ImportRoots    []string
	RequiresPython string
	EntryPoints    []PythonEntryPoint
	BuildSystem    *PythonBuildSystem
	Artifacts      []PythonArtifactIntent
}

func (PythonPackageData) BackendKind() ecosystem.BackendKind {
	return ecosystem.BackendPythonPyPI
}

type PythonEntryPoint struct {
	Name       string
	ModulePath string
	Attribute  string
}

type PythonBuildSystem struct {
	Requires     []Dependency
	BuildBackend string
}

type PythonArtifactIntent struct {
	Kind string
	Name string
}

type DependencyIntent string

const (
	DependencyRuntime  DependencyIntent = "runtime"
	DependencyTool     DependencyIntent = "tool"
	DependencyOptional DependencyIntent = "optional"
)

type Dependency struct {
	Name          string
	Intent        DependencyIntent
	Source        SourceRef
	Range         string
	RangeScheme   ecosystem.RangeScheme
	OptionalGroup string
	Extras        []string
	Markers       string
}

type SourceRef struct {
	Ecosystem ecosystem.EcosystemID
	Kind      string
	Name      string
	Index     string
}

type OptionalGroup struct {
	Name         string
	Dependencies []Dependency
}

type RunTargetRef struct {
	Name string
}
