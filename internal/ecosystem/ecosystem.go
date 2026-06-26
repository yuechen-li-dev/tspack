// Package ecosystem names the project-package ecosystem and backend schemes that
// shared orchestration code may need to distinguish.
//
// This is intentionally vocabulary and descriptor metadata only. It is not a
// plugin system and does not expose experimental ecosystems as supported
// manifest or lockfile behavior.
package ecosystem

type EcosystemID string

const (
	EcosystemNPM EcosystemID = "npm"

	// EcosystemPyPI is reserved vocabulary for future design work only.
	// Production manifest and lockfile validation must not accept PyPI sources
	// until a real backend owns that behavior.
	EcosystemPyPI EcosystemID = "pypi"
)

type BackendKind string

const (
	BackendTypeScriptNPM BackendKind = "typescript-npm"

	// BackendPythonPyPI is reserved vocabulary for future design work only.
	BackendPythonPyPI BackendKind = "python-pypi"
)

type VersionScheme string

const (
	VersionSchemeNpmSemver VersionScheme = "npmSemver"

	// VersionSchemePEP440 is reserved vocabulary for future design work only.
	VersionSchemePEP440 VersionScheme = "pep440"
)

type RangeScheme string

const (
	RangeSchemeNpmSemver RangeScheme = "npmSemverRange"

	// RangeSchemePEP440 is reserved vocabulary for future design work only.
	RangeSchemePEP440 RangeScheme = "pep440Specifier"
)

type RuntimeFamily string

const (
	RuntimeFamilyJavaScript RuntimeFamily = "javascript"

	// RuntimeFamilyPython is reserved vocabulary for Python-compatible runtimes
	// and DSLs. It is not a manifest runtime value.
	RuntimeFamilyPython RuntimeFamily = "python-family"
)

type RuntimeImplementation string

const (
	RuntimeImplementationNodeJS RuntimeImplementation = "nodejs"
	RuntimeImplementationBun    RuntimeImplementation = "bun"
	RuntimeImplementationDeno   RuntimeImplementation = "deno"

	// Python-family implementation names are reserved vocabulary only.
	RuntimeImplementationCPython RuntimeImplementation = "cpython"
	RuntimeImplementationPyPy    RuntimeImplementation = "pypy"
	RuntimeImplementationMojo    RuntimeImplementation = "mojo"
	RuntimeImplementationTriton  RuntimeImplementation = "triton"
)

type EnvironmentKind string

const (
	EnvironmentNodeModules EnvironmentKind = "nodeModules"

	// Python-family environment kinds are reserved vocabulary only. Future
	// backends may use venv-like layouts internally, but venv should not become
	// the user-facing project abstraction.
	EnvironmentUVManaged         EnvironmentKind = "uvManaged"
	EnvironmentPythonVenv        EnvironmentKind = "pythonVenv"
	EnvironmentSystemInterpreter EnvironmentKind = "systemInterpreter"
	EnvironmentHermetic          EnvironmentKind = "hermetic"
)

type ExecutionMode string

const (
	ExecutionInterpreted ExecutionMode = "interpreted"

	// Reserved execution/build modes for Python-family and native package work.
	ExecutionNativeExtension ExecutionMode = "nativeExtension"
	ExecutionJIT             ExecutionMode = "jit"
	ExecutionStagedGPU       ExecutionMode = "stagedGpu"
	ExecutionPackageBuild    ExecutionMode = "packageBuild"
)

type BackendStatus string

const (
	BackendProduction   BackendStatus = "production"
	BackendExperimental BackendStatus = "experimental"
	BackendReserved     BackendStatus = "reserved"
)

type BackendDescriptor struct {
	ID            BackendKind
	Ecosystem     EcosystemID
	VersionScheme VersionScheme
	RangeScheme   RangeScheme
	Status        BackendStatus
}

func TypeScriptNPMBackend() BackendDescriptor {
	return BackendDescriptor{
		ID:            BackendTypeScriptNPM,
		Ecosystem:     EcosystemNPM,
		VersionScheme: VersionSchemeNpmSemver,
		RangeScheme:   RangeSchemeNpmSemver,
		Status:        BackendProduction,
	}
}

func ReservedPythonPyPIBackend() BackendDescriptor {
	return BackendDescriptor{
		ID:            BackendPythonPyPI,
		Ecosystem:     EcosystemPyPI,
		VersionScheme: VersionSchemePEP440,
		RangeScheme:   RangeSchemePEP440,
		Status:        BackendReserved,
	}
}

func DescriptorForPackageSource(sourceKind string) (BackendDescriptor, bool) {
	if sourceKind == string(EcosystemNPM) {
		return TypeScriptNPMBackend(), true
	}
	return BackendDescriptor{}, false
}
