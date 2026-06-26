package ecosystem

import "testing"

func TestTypeScriptNPMBackendDescriptor(t *testing.T) {
	descriptor := TypeScriptNPMBackend()

	if descriptor.ID != BackendTypeScriptNPM {
		t.Fatalf("ID = %q", descriptor.ID)
	}
	if descriptor.Ecosystem != EcosystemNPM {
		t.Fatalf("Ecosystem = %q", descriptor.Ecosystem)
	}
	if descriptor.VersionScheme != VersionSchemeNpmSemver {
		t.Fatalf("VersionScheme = %q", descriptor.VersionScheme)
	}
	if descriptor.RangeScheme != RangeSchemeNpmSemver {
		t.Fatalf("RangeScheme = %q", descriptor.RangeScheme)
	}
	if descriptor.Status != BackendProduction {
		t.Fatalf("Status = %q", descriptor.Status)
	}
}

func TestReservedPythonPyPIBackendDescriptorIsNotProduction(t *testing.T) {
	descriptor := ReservedPythonPyPIBackend()

	if descriptor.ID != BackendPythonPyPI {
		t.Fatalf("ID = %q", descriptor.ID)
	}
	if descriptor.Ecosystem != EcosystemPyPI {
		t.Fatalf("Ecosystem = %q", descriptor.Ecosystem)
	}
	if descriptor.Status == BackendProduction {
		t.Fatalf("reserved backend must not be production")
	}
}

func TestDescriptorForPackageSourceOnlyMapsProductionNPM(t *testing.T) {
	descriptor, ok := DescriptorForPackageSource("npm")
	if !ok {
		t.Fatalf("expected npm source to map to a descriptor")
	}
	if descriptor.ID != BackendTypeScriptNPM {
		t.Fatalf("ID = %q", descriptor.ID)
	}

	_, ok = DescriptorForPackageSource("pypi")
	if ok {
		t.Fatalf("pypi source must not map to a production descriptor")
	}
}

func TestReservedPythonFamilyVocabularyIsSeparateFromPyPI(t *testing.T) {
	if RuntimeFamilyPython != "python-family" {
		t.Fatalf("RuntimeFamilyPython = %q", RuntimeFamilyPython)
	}
	if RuntimeImplementationCPython != "cpython" {
		t.Fatalf("RuntimeImplementationCPython = %q", RuntimeImplementationCPython)
	}
	if RuntimeImplementationPyPy != "pypy" {
		t.Fatalf("RuntimeImplementationPyPy = %q", RuntimeImplementationPyPy)
	}
	if EnvironmentUVManaged != "uvManaged" {
		t.Fatalf("EnvironmentUVManaged = %q", EnvironmentUVManaged)
	}
	if ExecutionStagedGPU != "stagedGpu" {
		t.Fatalf("ExecutionStagedGPU = %q", ExecutionStagedGPU)
	}
}
