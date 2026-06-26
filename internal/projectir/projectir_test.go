package projectir

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/ecosystem"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func TestTypeScriptNPMProjectPackageFixture(t *testing.T) {
	descriptor := ecosystem.TypeScriptNPMBackend()
	pkg := ProjectPackage{
		Name:      "@acme/web",
		Version:   "1.0.0",
		Ecosystem: descriptor.Ecosystem,
		Backend:   descriptor.ID,
		Runtime: RuntimeSpec{
			Family:         ecosystem.RuntimeFamilyJavaScript,
			Implementation: ecosystem.RuntimeImplementationNodeJS,
			Environment:    ecosystem.EnvironmentNodeModules,
			ExecutionMode:  ecosystem.ExecutionPackageBuild,
		},
		Dependencies: []Dependency{
			{
				Name:        "react",
				Intent:      DependencyRuntime,
				Source:      SourceRef{Ecosystem: ecosystem.EcosystemNPM, Kind: "npm", Name: "react"},
				Range:       "^18.2.0",
				RangeScheme: ecosystem.RangeSchemeNpmSemver,
			},
		},
		Tools: []Dependency{
			{
				Name:        "typescript",
				Intent:      DependencyTool,
				Source:      SourceRef{Ecosystem: ecosystem.EcosystemNPM, Kind: "npm", Name: "typescript"},
				Range:       "^5.0.0",
				RangeScheme: ecosystem.RangeSchemeNpmSemver,
			},
		},
		BackendData: NpmPackageData{
			Targets: []NpmTarget{
				{Name: "lib", Export: ".", Entry: "src/index.ts", Runtime: "node", Types: "src/index.ts"},
			},
		},
	}

	if descriptor.Status != ecosystem.BackendProduction {
		t.Fatalf("TypeScript/npm descriptor status = %q", descriptor.Status)
	}
	if pkg.Ecosystem != ecosystem.EcosystemNPM || pkg.Backend != ecosystem.BackendTypeScriptNPM {
		t.Fatalf("unexpected npm package backend identity: %#v", pkg)
	}
	if pkg.Runtime.Family != ecosystem.RuntimeFamilyJavaScript {
		t.Fatalf("runtime family = %q", pkg.Runtime.Family)
	}
	if pkg.Runtime.Implementation != ecosystem.RuntimeImplementationNodeJS {
		t.Fatalf("runtime implementation = %q", pkg.Runtime.Implementation)
	}
	if pkg.Tools[0].RangeScheme != ecosystem.RangeSchemeNpmSemver {
		t.Fatalf("tool range scheme = %q", pkg.Tools[0].RangeScheme)
	}
	if pkg.BackendData.BackendKind() != ecosystem.BackendTypeScriptNPM {
		t.Fatalf("backend data kind = %q", pkg.BackendData.BackendKind())
	}
}

func TestReservedPythonPyPIProjectPackageFixtureIsInternalOnly(t *testing.T) {
	descriptor := ecosystem.ReservedPythonPyPIBackend()
	pkg := ProjectPackage{
		Name:      "acme-api",
		Version:   "0.1.0",
		Ecosystem: descriptor.Ecosystem,
		Backend:   descriptor.ID,
		Runtime: RuntimeSpec{
			Family:         ecosystem.RuntimeFamilyPython,
			Implementation: ecosystem.RuntimeImplementationCPython,
			Environment:    ecosystem.EnvironmentUVManaged,
			ExecutionMode:  ecosystem.ExecutionInterpreted,
		},
		Dependencies: []Dependency{
			{
				Name:        "fastapi",
				Intent:      DependencyRuntime,
				Source:      SourceRef{Ecosystem: ecosystem.EcosystemPyPI, Kind: "pypi", Name: "fastapi", Index: "https://pypi.org/simple"},
				Range:       ">=0.115,<0.116",
				RangeScheme: ecosystem.RangeSchemePEP440,
				Extras:      []string{"standard"},
				Markers:     "python_version >= '3.11'",
			},
		},
		Tools: []Dependency{
			{
				Name:        "ruff",
				Intent:      DependencyTool,
				Source:      SourceRef{Ecosystem: ecosystem.EcosystemPyPI, Kind: "pypi", Name: "ruff"},
				Range:       ">=0.8,<0.9",
				RangeScheme: ecosystem.RangeSchemePEP440,
			},
		},
		OptionalGroups: []OptionalGroup{
			{
				Name: "dev",
				Dependencies: []Dependency{
					{
						Name:        "pytest",
						Intent:      DependencyOptional,
						Source:      SourceRef{Ecosystem: ecosystem.EcosystemPyPI, Kind: "pypi", Name: "pytest"},
						Range:       ">=8,<9",
						RangeScheme: ecosystem.RangeSchemePEP440,
					},
				},
			},
		},
		BackendData: PythonPackageData{
			ImportRoots:    []string{"acme_api"},
			RequiresPython: ">=3.11",
			EntryPoints: []PythonEntryPoint{
				{Name: "acme-api", ModulePath: "acme_api.cli", Attribute: "main"},
			},
			BuildSystem: &PythonBuildSystem{BuildBackend: "hatchling.build"},
		},
	}

	if descriptor.Status == ecosystem.BackendProduction {
		t.Fatalf("reserved Python/PyPI descriptor must not be production")
	}
	if pkg.Ecosystem != ecosystem.EcosystemPyPI || pkg.Backend != ecosystem.BackendPythonPyPI {
		t.Fatalf("unexpected Python package backend identity: %#v", pkg)
	}
	if pkg.Runtime.Family != ecosystem.RuntimeFamilyPython {
		t.Fatalf("runtime family = %q", pkg.Runtime.Family)
	}
	if pkg.Runtime.Family == "python" {
		t.Fatalf("Python-family fixture must not collapse to flat python runtime")
	}
	if pkg.Dependencies[0].RangeScheme != ecosystem.RangeSchemePEP440 {
		t.Fatalf("dependency range scheme = %q", pkg.Dependencies[0].RangeScheme)
	}
	if pkg.BackendData.BackendKind() != ecosystem.BackendPythonPyPI {
		t.Fatalf("backend data kind = %q", pkg.BackendData.BackendKind())
	}
}

func TestProjectIRPyPIFixtureDoesNotChangeProductGuardrails(t *testing.T) {
	manifestJSON := []byte(`{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[{"key":"fastapi","kind":"dep","source":{"kind":"pypi","package":"fastapi","range":">=0.115,<0.116"}}],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`)
	_, manifestDiagnostics := manifest.LoadBytes("manifest.json", manifestJSON)
	if len(manifestDiagnostics) == 0 {
		t.Fatalf("manifest validation accepted reserved pypi source")
	}

	lockfileBytes := []byte("[lock]\nformat = 1\n\n[[package]]\nid = 'pypi:fastapi@0.115.6'\nname = 'fastapi'\nsource = 'pypi'\nversion = '0.115.6'\n")
	_, lockfileDiagnostics := lockfile.Parse("ts-lock.toml", lockfileBytes)
	if len(lockfileDiagnostics) == 0 {
		t.Fatalf("lockfile validation accepted reserved pypi source")
	}

	if _, ok := ecosystem.DescriptorForPackageSource("pypi"); ok {
		t.Fatalf("pypi source must not map to a production package-source descriptor")
	}
}

func TestProjectIRPackageOwnershipDoesNotAdvertisePythonInCLIHelp(t *testing.T) {
	command := exec.Command("go", "run", "../../cmd/tspack", "help", "all")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("help command failed: %v\n%s", err, string(output))
	}

	help := strings.ToLower(string(output))
	for _, term := range []string{"python", "pypi", "uv"} {
		if strings.Contains(help, term) {
			t.Fatalf("CLI help advertises reserved %q support:\n%s", term, string(output))
		}
	}
}
