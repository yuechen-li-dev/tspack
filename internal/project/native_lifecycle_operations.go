package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yuechen-li-dev/tspack/internal/audit"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/testcmd"
)

// BuildRequest is the application contract shared by CLI and workflow builds.
// Compiler process implementation is supplied below the presentation boundary.
type BuildRequest struct {
	Project                Options
	Packages               []string
	Targets                []string
	PreserveLastSuccessful bool
	Executor               BuildTargetExecutor
}

type BuildTargetRequest struct {
	Project                Options
	Manifest               *manifest.ManifestIR
	Package                *manifest.Package
	Target                 manifest.Target
	PreserveLastSuccessful bool
}

type BuildTargetExecutor interface {
	BuildTarget(context.Context, BuildTargetRequest) BuildTargetResult
}

type BuildTargetResult struct {
	Package     string            `json:"package"`
	Target      string            `json:"target"`
	Compiler    string            `json:"compiler"`
	Succeeded   bool              `json:"succeeded"`
	Artifacts   []BuildArtifact   `json:"artifacts,omitempty"`
	Diagnostics []diag.Diagnostic `json:"diagnostics,omitempty"`
}

type BuildArtifact struct {
	Package string `json:"package"`
	Target  string `json:"target"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
}

type BuildOperationResult struct {
	Diagnostics []diag.Diagnostic   `json:"diagnostics,omitempty"`
	Targets     []BuildTargetResult `json:"targets"`
	Artifacts   []BuildArtifact     `json:"artifacts"`
}

func RunBuild(ctx context.Context, request BuildRequest) BuildOperationResult {
	result := BuildOperationResult{Targets: []BuildTargetResult{}, Artifacts: []BuildArtifact{}}
	if ctx == nil {
		ctx = context.Background()
	}
	if request.Executor == nil {
		result.Diagnostics = append(result.Diagnostics, errDiag("TSPACK_BUILD_EXECUTOR_MISSING", "build target executor is unavailable"))
		return result
	}
	ir, diagnostics := loadManifestIR(request.Project)
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	if hasErrors(result.Diagnostics) {
		return result
	}
	selectedPackages := selectLifecyclePackages(ir, request.Packages)
	if len(selectedPackages) == 0 {
		result.Diagnostics = append(result.Diagnostics, errDiag("TSPACK_BUILD_PACKAGE_NOT_FOUND", "no package matched the requested build selection"))
		return result
	}
	for _, pkg := range selectedPackages {
		selectedTargets, selectionErr := orderLifecycleTargets(pkg, request.Targets)
		if selectionErr != nil {
			result.Diagnostics = append(result.Diagnostics, errDiag("TSPACK_BUILD_TARGET_NOT_FOUND", selectionErr.Error(), "package: "+pkg.Name))
			continue
		}
		for _, target := range selectedTargets {
			if err := ctx.Err(); err != nil {
				result.Diagnostics = append(result.Diagnostics, errDiag("TSPACK_BUILD_CANCELLED", "build was cancelled", err.Error()))
				return result
			}
			targetResult := request.Executor.BuildTarget(ctx, BuildTargetRequest{
				Project:                request.Project,
				Manifest:               ir,
				Package:                pkg,
				Target:                 target,
				PreserveLastSuccessful: request.PreserveLastSuccessful,
			})
			if targetResult.Package == "" {
				targetResult.Package = pkg.Name
			}
			if targetResult.Target == "" {
				targetResult.Target = target.Name
			}
			if targetResult.Compiler == "" {
				targetResult.Compiler = target.Compiler
			}
			result.Targets = append(result.Targets, targetResult)
			result.Artifacts = append(result.Artifacts, targetResult.Artifacts...)
			result.Diagnostics = append(result.Diagnostics, targetResult.Diagnostics...)
			if !targetResult.Succeeded || hasErrors(targetResult.Diagnostics) {
				return result
			}
		}
	}
	return result
}

func selectLifecyclePackages(ir *manifest.ManifestIR, requested []string) []*manifest.Package {
	selected := []*manifest.Package{}
	for index := range ir.Packages {
		pkg := &ir.Packages[index]
		if len(requested) == 0 || containsLifecycleSelection(requested, pkg.Name) {
			selected = append(selected, pkg)
		}
	}
	return selected
}

func orderLifecycleTargets(pkg *manifest.Package, requested []string) ([]manifest.Target, error) {
	byName := map[string]manifest.Target{}
	for _, target := range pkg.Targets {
		byName[target.Name] = target
	}
	selected := map[string]bool{}
	var selectTarget func(string) error
	selectTarget = func(name string) error {
		if selected[name] {
			return nil
		}
		target, ok := byName[name]
		if !ok {
			return fmt.Errorf("unknown compiler target dependency %s", name)
		}
		selected[name] = true
		for _, dependency := range target.DependsOn {
			if err := selectTarget(dependency); err != nil {
				return err
			}
		}
		return nil
	}
	if len(requested) == 0 {
		for _, target := range pkg.Targets {
			if err := selectTarget(target.Name); err != nil {
				return nil, err
			}
		}
	} else {
		for _, name := range requested {
			if err := selectTarget(name); err != nil {
				return nil, err
			}
		}
	}
	state := map[string]int{}
	ordered := []manifest.Target{}
	var visit func(string) error
	visit = func(name string) error {
		if state[name] == 2 || !selected[name] {
			return nil
		}
		if state[name] == 1 {
			return fmt.Errorf("TSPACK_COMPILER_TARGET_DEPENDENCY_CYCLE: cycle includes %s", name)
		}
		state[name] = 1
		target := byName[name]
		for _, dependency := range target.DependsOn {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[name] = 2
		ordered = append(ordered, target)
		return nil
	}
	for _, target := range pkg.Targets {
		if err := visit(target.Name); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func containsLifecycleSelection(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

type TestRequest struct {
	Project Options
	Options testcmd.Options
}

type TestOperationResult struct {
	Diagnostics []diag.Diagnostic      `json:"diagnostics,omitempty"`
	ExitCode    int                    `json:"exitCode"`
	Passed      int                    `json:"passed"`
	Failed      int                    `json:"failed"`
	Skipped     int                    `json:"skipped"`
	DurationMs  float64                `json:"durationMs,omitempty"`
	Tests       []testcmd.TestEvidence `json:"tests,omitempty"`
}

func RunTest(ctx context.Context, request TestRequest) TestOperationResult {
	options := request.Options
	if options.RootDir == "" {
		options.RootDir = request.Project.RootDir
	}
	result := testcmd.RunContext(ctx, options)
	operation := TestOperationResult{
		Diagnostics: result.Diagnostics,
		ExitCode:    result.ExitCode,
		Passed:      result.Summary.Passed,
		Failed:      result.Summary.Failed,
		Skipped:     result.Summary.Skipped,
		DurationMs:  result.Summary.DurationMs,
		Tests:       result.Tests,
	}
	return operation
}

type AuditRequest struct {
	Project         Options
	AuditLevel      string
	RequireCoverage bool
	Client          audit.Client
}

type AuditOperationResult struct {
	Diagnostics []diag.Diagnostic `json:"diagnostics,omitempty"`
	Source      string            `json:"source"`
	AuditLevel  string            `json:"auditLevel"`
	Failing     int               `json:"failing"`
	Report      audit.Report      `json:"report"`
}

func RunAudit(ctx context.Context, request AuditRequest) AuditOperationResult {
	result := AuditOperationResult{Source: "OSV.dev", AuditLevel: request.AuditLevel}
	if result.AuditLevel == "" {
		result.AuditLevel = "any"
	}
	threshold, err := audit.ParseThreshold(result.AuditLevel)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, errDiag("TSPACK_AUDIT_INVALID_ARGS", err.Error()))
		return result
	}
	lockPath := request.Project.LockfilePath
	if lockPath == "" {
		lockPath = filepath.Join(request.Project.RootDir, "ts-lock.toml")
	}
	locked, diagnostics, err := lockfile.LoadFile(lockPath)
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, errDiag("TSPACK_AUDIT_LOCKFILE_FAILED", fmt.Sprintf("failed to read %s: %v", lockPath, err)))
		return result
	}
	if hasErrors(result.Diagnostics) {
		return result
	}
	client := request.Client
	if client == nil {
		client = &audit.HTTPClient{Endpoint: os.Getenv("TSPACK_OSV_API")}
	}
	report, err := audit.Scan(ctx, locked, client)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, errDiag("TSPACK_AUDIT_SERVICE_FAILED", err.Error()))
		return result
	}
	result.Report = report
	for _, finding := range report.Findings {
		if audit.FailsThreshold(finding.Severity, threshold) {
			result.Failing++
		}
	}
	if request.RequireCoverage && !report.CoverageComplete {
		result.Diagnostics = append(result.Diagnostics, errDiag("TSPACK_AUDIT_COVERAGE_REQUIRED", "audit coverage is incomplete"))
	}
	if result.Failing > 0 {
		result.Diagnostics = append(result.Diagnostics, errDiag("TSPACK_AUDIT_POLICY_REJECTED", fmt.Sprintf("%d finding(s) meet the %s threshold", result.Failing, result.AuditLevel)))
	}
	return result
}
