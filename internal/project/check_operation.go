package project

import (
	"github.com/yuechen-li-dev/tspack/internal/check"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/resolver"
	"github.com/yuechen-li-dev/tspack/internal/securityevidence"
	"os"
	"path/filepath"
	"strings"
)

func Check(opts Options) Result {
	ir, g, out := loadManifestAndGraph(opts)
	if ir != nil {
		out = append(out, securityevidence.Diagnostics(opts.RootDir, ir.Security.AcknowledgedCapabilities)...)
	}
	out = append(out, check.CheckPackage(check.CheckOptions{RootDir: opts.RootDir, Graph: g}).Diagnostics...)
	if _, err := os.Stat(opts.LockfilePath); err == nil {
		lf, d, e := lockfile.LoadFile(opts.LockfilePath)
		if e != nil {
			out = append(out, errDiag("TSPACK_CHECK_FAILED", "failed to read lockfile", e.Error()))
		} else {
			out = append(out, d...)
			out = append(out, lockfile.CheckGraphConsistency(g, lf).Diagnostics...)
			out = append(out, lockfile.CheckVersionConflicts(lf).Diagnostics...)
			out = append(out, lockfile.CheckRequirements(lf).Diagnostics...)
			out = append(out, sourcePolicyLockDiagnostics(ir, lf)...)
			out = append(out, lifecycleCapabilityDiagnostics(lf, lifecycleAcknowledgementSet(ir), lifecycleCategoryAcknowledgements(ir))...)
		}
	} else if os.IsNotExist(err) {
		out = append(out, diag.Diagnostic{Code: "TSPACK_CHECK_LOCKFILE_MISSING", Severity: diag.SeverityWarning, Message: "lockfile is missing"})
	}
	diag.SortDiagnostics(out)
	return Result{Diagnostics: out}
}

func sourcePolicyLockDiagnostics(ir *manifest.ManifestIR, lf *lockfile.Lockfile) []diag.Diagnostic {
	if ir == nil || lf == nil || !hasDeclaredSourcePolicy(ir) {
		return nil
	}
	policy := resolverSourcePolicy(ir)
	declaredEndpoints := map[string]map[string]bool{}
	for _, source := range ir.RegistryPolicy.Sources {
		declaredEndpoints[source.Kind] = map[string]bool{}
		for _, endpoint := range source.Endpoints {
			declaredEndpoints[source.Kind][strings.TrimRight(endpoint.URL, "/")] = true
		}
	}
	var out []diag.Diagnostic
	for _, pkg := range lf.Packages {
		if pkg.Source != resolver.SourceNPM && pkg.Source != resolver.SourceJSR {
			continue
		}
		if !policy.Allows(pkg.Source) {
			out = append(out, diag.Diagnostic{Code: "TSPACK_SOURCE_POLICY_DENIED", Severity: diag.SeverityError, Message: "lockfile contains a package from a denied semantic source", Details: []string{"package: " + pkg.ID, "policy origin: manifest RegistryPolicy", "Run `tspack update` after allowing the source or removing the dependency."}})
			continue
		}
		if policy.RequireAuditCoverage && pkg.Source == resolver.SourceJSR {
			out = append(out, diag.Diagnostic{Code: "TSPACK_REGISTRY_TRUST_FAILED", Severity: diag.SeverityError, Message: "source policy requires vulnerability audit coverage unavailable for JSR", Details: []string{"package: " + pkg.ID, "coverage: unsupported-ecosystem", "OSV has no JSR ecosystem mapping; allow partial coverage or remove the JSR package."}})
		}
		allowed, endpointRestricted := declaredEndpoints[pkg.Source]
		if !endpointRestricted {
			continue
		}
		if pkg.RegistryEndpoint == "" {
			out = append(out, diag.Diagnostic{Code: "TSPACK_REGISTRY_PROVENANCE_MISSING", Severity: diag.SeverityWarning, Message: "locked registry package predates endpoint provenance", Details: []string{"package: " + pkg.ID, "Run `tspack update` to validate the package through the declared endpoint policy."}})
			continue
		}
		if !allowed[strings.TrimRight(pkg.RegistryEndpoint, "/")] {
			out = append(out, diag.Diagnostic{Code: "TSPACK_REGISTRY_ENDPOINT_DENIED", Severity: diag.SeverityError, Message: "locked package provenance uses an endpoint denied by current policy", Details: []string{"package: " + pkg.ID, "endpoint: " + resolver.RedactURL(pkg.RegistryEndpoint)}})
		}
	}
	return out
}

func CheckExplain(opts Options, requestedFile string) Result {
	rootAbs, err := filepath.Abs(opts.RootDir)
	if err != nil {
		return Result{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_CHECK_EXPLAIN_FAILED", "failed to resolve project root", err.Error())}}
	}
	requestPath := requestedFile
	if !filepath.IsAbs(requestPath) {
		requestPath = filepath.Join(rootAbs, requestPath)
	}
	fileAbs, err := filepath.Abs(requestPath)
	if err != nil {
		return Result{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_CHECK_EXPLAIN_FAILED", "failed to resolve explain file", err.Error())}}
	}
	rel, err := filepath.Rel(rootAbs, fileAbs)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return Result{Diagnostics: []diag.Diagnostic{{Code: "TSPACK_CHECK_EXPLAIN_FILE_OUTSIDE_ROOT", Severity: diag.SeverityError, Message: "explain file is outside the project root", File: requestedFile}}}
	}
	st, err := os.Stat(fileAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{Diagnostics: []diag.Diagnostic{{Code: "TSPACK_CHECK_EXPLAIN_FILE_NOT_FOUND", Severity: diag.SeverityError, Message: "explain file does not exist", File: filepath.ToSlash(rel)}}}
		}
		return Result{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_CHECK_EXPLAIN_FAILED", "failed to stat explain file", err.Error())}}
	}
	if st.IsDir() {
		return Result{Diagnostics: []diag.Diagnostic{{Code: "TSPACK_CHECK_EXPLAIN_UNSUPPORTED_FILE", Severity: diag.SeverityError, Message: "explain path must be a supported source file", File: filepath.ToSlash(rel)}}}
	}
	if !isExplainSourceFile(fileAbs) {
		return Result{Diagnostics: []diag.Diagnostic{{Code: "TSPACK_CHECK_EXPLAIN_UNSUPPORTED_FILE", Severity: diag.SeverityError, Message: "explain file must be .ts, .tsx, .js, or .jsx", File: filepath.ToSlash(rel)}}}
	}
	_, g, out := loadManifestAndGraph(opts)
	if hasErrors(out) {
		diag.SortDiagnostics(out)
		return Result{Diagnostics: out}
	}
	explain := check.Explain(check.ExplainOptions{RootDir: rootAbs, Graph: g, File: filepath.ToSlash(rel)})
	return Result{Diagnostics: out, Explain: &explain}
}

func isExplainSourceFile(path string) bool {
	switch filepath.Ext(path) {
	case ".ts", ".tsx", ".js", ".jsx":
		return true
	default:
		return false
	}
}
