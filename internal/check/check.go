package check

import (
	"github.com/tspack/tspack/internal/boundary"
	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/graph"
)

type CheckOptions struct {
	RootDir string
	Graph   *graph.WorkspaceGraph
}

type CheckResult struct { Diagnostics []diag.Diagnostic }

func CheckRuntimeBoundaries(opts CheckOptions) CheckResult {
	return CheckResult{Diagnostics: boundary.Check(boundary.Options{RootDir: opts.RootDir, Graph: opts.Graph})}
}
