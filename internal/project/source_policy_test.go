package project

import (
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func TestSourcePolicyLockDiagnosticsRejectDeniedSourceAndEndpoint(t *testing.T) {
	ir := &manifest.ManifestIR{RegistryPolicy: manifest.RegistryPolicy{
		AllowedSources: []string{"npm"},
		Sources: []manifest.RegistrySourcePolicy{{
			Kind:      "npm",
			Endpoints: []manifest.RegistryEndpointPolicy{{URL: "https://npm.company.example"}},
		}},
	}}
	lf := &lockfile.Lockfile{Packages: []lockfile.Package{
		{ID: "jsr:@std/path@1.0.0", Source: "jsr", Name: "@std/path", Version: "1.0.0"},
		{ID: "npm:react@19.0.0", Source: "npm", Name: "react", Version: "19.0.0", RegistryEndpoint: "https://registry.npmjs.org"},
	}}
	diagnostics := sourcePolicyLockDiagnostics(ir, lf)
	if !containsSeverityCode(diagnostics, diag.SeverityError, "TSPACK_SOURCE_POLICY_DENIED") {
		t.Fatalf("missing denied-source diagnostic: %#v", diagnostics)
	}
	if !containsSeverityCode(diagnostics, diag.SeverityError, "TSPACK_REGISTRY_ENDPOINT_DENIED") {
		t.Fatalf("missing denied-endpoint diagnostic: %#v", diagnostics)
	}
}

func TestSourcePolicyLockDiagnosticsReportsUnsupportedAuditCoverage(t *testing.T) {
	ir := &manifest.ManifestIR{RegistryPolicy: manifest.RegistryPolicy{
		AllowedSources:       []string{"jsr"},
		RequireAuditCoverage: true,
	}}
	lf := &lockfile.Lockfile{Packages: []lockfile.Package{{ID: "jsr:@std/path@1.0.0", Source: "jsr", Name: "@std/path", Version: "1.0.0"}}}
	diagnostics := sourcePolicyLockDiagnostics(ir, lf)
	if !containsSeverityCode(diagnostics, diag.SeverityError, "TSPACK_REGISTRY_TRUST_FAILED") {
		t.Fatalf("missing audit-coverage trust diagnostic: %#v", diagnostics)
	}
}

func containsSeverityCode(diagnostics []diag.Diagnostic, severity diag.Severity, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == severity && diagnostic.Code == code {
			return true
		}
	}
	return false
}
