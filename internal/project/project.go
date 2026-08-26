package project

import (
	"github.com/yuechen-li-dev/tspack/internal/check"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/perf"
	"github.com/yuechen-li-dev/tspack/internal/resolver"
	"github.com/yuechen-li-dev/tspack/internal/why"
	"io"
	"path/filepath"
)

type Options struct {
	RootDir, ManifestPath, LockfilePath, StoreRoot string
	ManifestIRPath                                 string
	FrontendCLIPath                                string
	ResolverClient                                 resolver.NPMRegistryClient
	Progress                                       Progress
	Perf                                           *perf.Session
	PerfWriter                                     io.Writer
}
type Result struct {
	Diagnostics  []diag.Diagnostic
	LockDiff     *lockfile.Diff
	DryRun       *UpdateDryRunResult
	UpdateTarget *UpdateTargetResult
	PackResult   *PackResult
	WhyResult    *why.Result
	Outdated     *OutdatedResult
	Explain      *check.ExplainResult
}
type UpdateDryRunResult struct {
	Changed bool
	Summary UpdateDiffSummary
}
type UpdateDiffSummary struct {
	Added, Removed, Changed, Unchanged int
}

type UpdateOptions struct {
	Query string
}

type UpdateTargetResult struct {
	Targeted       bool
	Query          string
	Selected       []UpdateSelectedTarget
	DirectPackages []string
}

type UpdateSelectedTarget struct {
	Package string
	Key     string
	Name    string
	Source  string
}

type WhyOptions struct {
	Query       string
	PackageName string
	Reverse     bool
}

type PackOptions struct {
	OutputDir   string
	PackageName string
	DryRun      bool
	Verify      bool
}
type PackResult struct {
	Artifacts []PackArtifact
	Preview   []PackFile
}
type PackArtifact struct {
	PackageName string
	Version     string
	Path        string
	Hash        string
	Size        int64
	Verified    bool
}
type PackFile struct {
	PackageName string
	SourcePath  string
	ArchivePath string
	Size        int64
	Reason      string
}

func DefaultOptions(root string) Options {
	if root == "" {
		root = "."
	}
	return Options{RootDir: root, ManifestPath: filepath.Join(root, "manifest.tsx"), LockfilePath: filepath.Join(root, "ts-lock.toml"), StoreRoot: filepath.Join(root, ".tspack", "store")}
}
