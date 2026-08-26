package project

import (
	"github.com/yuechen-li-dev/tspack/internal/check"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/pack"
	"os"
)

func Pack(opts Options, packOpts PackOptions) Result {
	if packOpts.DryRun && packOpts.Verify {
		return Result{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_PACK_INVALID_ARGS", "--verify requires a produced archive and cannot be combined with --dry-run")}}
	}

	_, g, out := loadManifestAndGraph(opts)
	out = append(out, check.CheckPackage(check.CheckOptions{RootDir: opts.RootDir, Graph: g}).Diagnostics...)
	if _, err := os.Stat(opts.LockfilePath); err == nil {
		lf, d, e := lockfile.LoadFile(opts.LockfilePath)
		if e != nil {
			out = append(out, errDiag("TSPACK_PACK_LOCKFILE_STALE", "failed to read lockfile", e.Error()))
		} else {
			out = append(out, d...)
			out = append(out, lockfile.CheckGraphConsistency(g, lf).Diagnostics...)
		}
	} else if os.IsNotExist(err) {
		out = append(out, diag.Diagnostic{Code: "TSPACK_CHECK_LOCKFILE_MISSING", Severity: diag.SeverityWarning, Message: "lockfile is missing"})
	}
	if hasErrors(out) {
		diag.SortDiagnostics(out)
		return Result{Diagnostics: out}
	}
	pkgs := g.AllPackages()
	if packOpts.PackageName != "" {
		p, ok := g.Package(packOpts.PackageName)
		if !ok {
			return Result{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_PACK_PACKAGE_NOT_FOUND", "package not found", packOpts.PackageName)}}
		}
		pkgs = []*graph.PackageNode{p}
	}
	for _, p := range pkgs {
		if p.Kind == "service" {
			out = append(out, errDiag("TSPACK_PACK_UNSUPPORTED_PACKAGE_KIND", "pack does not support service packages yet", p.Name))
		}
	}
	if hasErrors(out) {
		diag.SortDiagnostics(out)
		return Result{Diagnostics: out}
	}
	pr := &PackResult{}
	plans := []pack.Plan{}
	packOptions := pack.Options{OutputDir: packOpts.OutputDir, DryRun: packOpts.DryRun, Verify: packOpts.Verify}
	for _, p := range pkgs {
		r := pack.PlanPackage(opts.RootDir, p, packOptions)
		out = append(out, r.Diagnostics...)
		plans = append(plans, r.Plans...)
		for _, f := range r.Preview {
			pr.Preview = append(pr.Preview, PackFile(f))
		}
	}
	if hasErrors(out) {
		diag.SortDiagnostics(out)
		return Result{Diagnostics: out, PackResult: pr}
	}
	if !packOpts.DryRun {
		writeResult := pack.WritePlans(plans, packOptions)
		out = append(out, writeResult.Diagnostics...)
		for _, a := range writeResult.Artifacts {
			pr.Artifacts = append(pr.Artifacts, PackArtifact(a))
		}
	}
	diag.SortDiagnostics(out)
	return Result{Diagnostics: out, PackResult: pr}
}
