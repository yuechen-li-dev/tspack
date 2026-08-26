package project

import (
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/why"
	"os"
)

func Why(opts Options, whyOpts WhyOptions) Result {
	ir, g, out := loadManifestAndGraph(opts)
	if hasErrors(out) {
		return Result{Diagnostics: out}
	}
	var lf *lockfile.Lockfile
	lockfileMissing := false
	if _, err := os.Stat(opts.LockfilePath); err == nil {
		parsed, d, e := lockfile.LoadFile(opts.LockfilePath)
		if e != nil {
			out = append(out, errDiag("TSPACK_WHY_LOCKFILE_INVALID", "failed to read lockfile", e.Error()))
		} else {
			lf = parsed
			out = append(out, d...)
			out = append(out, lockfile.CheckGraphConsistency(g, lf).Diagnostics...)
		}
	} else if os.IsNotExist(err) {
		severity := diag.SeverityWarning
		message := "lockfile is missing"
		details := []string(nil)
		if whyOpts.Reverse {
			severity = diag.SeverityError
			details = []string{"reverse why requires a lockfile; run tspack update"}
		}
		out = append(out, diag.Diagnostic{Code: "TSPACK_WHY_LOCKFILE_MISSING", Severity: severity, Message: message, Details: details})
		lockfileMissing = true
	}
	wr := why.Result{}
	if whyOpts.Reverse && lockfileMissing {
		wr = why.Result{}
	} else {
		wr = why.Analyze(g, lf, why.Options{Query: whyOpts.Query, PackageName: whyOpts.PackageName, Reverse: whyOpts.Reverse, RootDir: opts.RootDir, AcknowledgedCapabilities: ir.Security.AcknowledgedCapabilities})
		out = append(out, wr.Diagnostics...)
	}
	diag.SortDiagnostics(out)
	return Result{Diagnostics: out, WhyResult: &wr}
}
