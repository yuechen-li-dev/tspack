package check

import (
	"github.com/yuechen-li-dev/tspack/internal/boundary"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/typesurface"
)

type CheckOptions struct {
	RootDir            string
	Graph              *graph.WorkspaceGraph
	AllowMissingOutput bool
}

type CheckResult struct{ Diagnostics []diag.Diagnostic }

func CheckRuntimeBoundaries(opts CheckOptions) CheckResult {
	return CheckResult{Diagnostics: boundary.Check(boundary.Options{RootDir: opts.RootDir, Graph: opts.Graph})}
}
func CheckTypeSurfaces(opts CheckOptions) CheckResult {
	return CheckResult{Diagnostics: typesurface.CheckTypeSurfaces(typesurface.CheckOptions{
		RootDir:            opts.RootDir,
		Graph:              opts.Graph,
		AllowMissingOutput: opts.AllowMissingOutput,
	}).Diagnostics}
}
func CheckPackage(opts CheckOptions) CheckResult {
	r := CheckRuntimeBoundaries(opts).Diagnostics
	t := CheckTypeSurfaces(opts).Diagnostics
	all := append(r, t...)
	diag.SortDiagnostics(all)
	return CheckResult{Diagnostics: all}
}
