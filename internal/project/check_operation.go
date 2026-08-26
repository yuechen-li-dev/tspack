package project

import (
	"github.com/yuechen-li-dev/tspack/internal/check"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
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
			out = append(out, lifecycleCapabilityDiagnostics(lf, lifecycleAcknowledgementSet(ir), lifecycleCategoryAcknowledgements(ir))...)
		}
	} else if os.IsNotExist(err) {
		out = append(out, diag.Diagnostic{Code: "TSPACK_CHECK_LOCKFILE_MISSING", Severity: diag.SeverityWarning, Message: "lockfile is missing"})
	}
	diag.SortDiagnostics(out)
	return Result{Diagnostics: out}
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
