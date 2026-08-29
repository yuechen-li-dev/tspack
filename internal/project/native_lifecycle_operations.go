package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	Package      string `json:"package"`
	Target       string `json:"target"`
	Kind         string `json:"kind"`
	Role         string `json:"role,omitempty"`
	Path         string `json:"path"`
	Identity     string `json:"identity,omitempty"`
	ContentHash  string `json:"contentHash,omitempty"`
	OriginRegion string `json:"originRegion,omitempty"`
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
	configuredTargetCount := 0
	for _, pkg := range selectedPackages {
		configuredTargetCount += len(pkg.Targets)
	}
	if configuredTargetCount == 0 {
		details := []string{}
		for _, pkg := range selectedPackages {
			details = append(details, "selected package: "+pkg.Name+" (0 build targets)")
		}
		result.Diagnostics = append(result.Diagnostics, errDiag(
			"TSPACK_BUILD_NO_TARGETS",
			"no build targets are configured for the selected package set",
			append(details, "Declare a compiler target in manifest.tsx or select a package with a declared build contract.")...,
		))
		return result
	}
	plannedTargets, selectionErr := orderWorkspaceBuildTargets(ir, selectedPackages, request.Targets)
	if selectionErr != nil {
		result.Diagnostics = append(result.Diagnostics, errDiag("TSPACK_BUILD_TARGET_NOT_FOUND", selectionErr.Error()))
		return result
	}
	for _, planned := range plannedTargets {
		pkg := planned.Package
		target := planned.Target
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
	return result
}

type plannedBuildTarget struct {
	Package *manifest.Package
	Target  manifest.Target
}

func orderWorkspaceBuildTargets(ir *manifest.ManifestIR, selectedPackages []*manifest.Package, requested []string) ([]plannedBuildTarget, error) {
	byIdentity := map[string]plannedBuildTarget{}
	for packageIndex := range ir.Packages {
		pkg := &ir.Packages[packageIndex]
		for _, target := range pkg.Targets {
			byIdentity[pkg.Name+":"+target.Name] = plannedBuildTarget{Package: pkg, Target: target}
		}
	}

	roots := []string{}
	for _, pkg := range selectedPackages {
		if len(requested) == 0 {
			for _, target := range pkg.Targets {
				roots = append(roots, pkg.Name+":"+target.Name)
			}
			continue
		}
		for _, name := range requested {
			identity := pkg.Name + ":" + name
			if _, ok := byIdentity[identity]; !ok {
				return nil, fmt.Errorf("unknown build target %s for package %s%s", name, pkg.Name, buildTargetSuggestion(name, pkg.Targets))
			}
			roots = append(roots, identity)
		}
	}

	state := map[string]int{}
	ordered := []plannedBuildTarget{}
	var visit func(string) error
	visit = func(identity string) error {
		if state[identity] == 2 {
			return nil
		}
		if state[identity] == 1 {
			return fmt.Errorf("build target dependency cycle includes %s", identity)
		}
		planned, ok := byIdentity[identity]
		if !ok {
			return fmt.Errorf("unknown build target dependency %s", identity)
		}
		state[identity] = 1
		for _, reference := range planned.Target.DependsOn {
			dependencyPackage, dependencyTarget := manifest.ResolveBuildTargetReference(planned.Package.Name, reference)
			if err := visit(dependencyPackage + ":" + dependencyTarget); err != nil {
				return err
			}
		}
		state[identity] = 2
		ordered = append(ordered, planned)
		return nil
	}
	for _, identity := range roots {
		if err := visit(identity); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func buildTargetSuggestion(requested string, targets []manifest.Target) string {
	type candidate struct {
		name     string
		distance int
	}
	candidates := []candidate{}
	for _, target := range targets {
		distance := editDistance(requested, target.Name)
		if distance <= 3 || strings.Contains(target.Name, requested) || strings.Contains(requested, target.Name) {
			candidates = append(candidates, candidate{name: target.Name, distance: distance})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].distance != candidates[j].distance {
			return candidates[i].distance < candidates[j].distance
		}
		return candidates[i].name < candidates[j].name
	})
	if len(candidates) > 3 {
		candidates = candidates[:3]
	}
	if len(candidates) == 0 {
		return ""
	}
	names := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		names = append(names, candidate.name)
	}
	return "; did you mean " + strings.Join(names, ", ") + "?"
}

func editDistance(left string, right string) int {
	previous := make([]int, len(right)+1)
	for index := range previous {
		previous[index] = index
	}
	for leftIndex, leftRune := range []rune(left) {
		current := make([]int, len([]rune(right))+1)
		current[0] = leftIndex + 1
		for rightIndex, rightRune := range []rune(right) {
			cost := 0
			if leftRune != rightRune {
				cost = 1
			}
			deletion := previous[rightIndex+1] + 1
			insertion := current[rightIndex] + 1
			replacement := previous[rightIndex] + cost
			current[rightIndex+1] = min(deletion, insertion, replacement)
		}
		previous = current
	}
	return previous[len(previous)-1]
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
	Package string
	Target  string
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
	if request.Package != "" || request.Target != "" {
		ir, diagnostics := loadManifestIR(request.Project)
		if hasErrors(diagnostics) {
			return TestOperationResult{Diagnostics: diagnostics, ExitCode: 1}
		}
		pkg := selectTestPackage(ir, request.Package)
		if pkg == nil {
			return TestOperationResult{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_TEST_PACKAGE_NOT_FOUND", "no package matched the requested test selection", "package: "+request.Package)}, ExitCode: 1}
		}
		target := selectTestTarget(pkg, request.Target)
		if target == nil {
			details := []string{"selected package: " + pkg.Name, "Declare TestTargets in the package manifest or select a configured target."}
			if suggestion := testTargetSuggestion(request.Target, pkg.TestTargets); suggestion != "" {
				details = append(details, suggestion)
			}
			return TestOperationResult{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_TEST_NO_TARGETS", "no test target matched the requested package selection", details...)}, ExitCode: 1}
		}
		if target.Harness != "vitest" {
			return TestOperationResult{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_TEST_HARNESS_UNSUPPORTED", "unsupported declared test harness: "+target.Harness)}, ExitCode: 1}
		}
		options.UseVitest = true
		options.CaptureStructured = true
		options.VitestCwd = filepath.Join(request.Project.RootDir, filepath.FromSlash(pkg.Root))
		options.VitestConfig = target.Config
		options.VitestFiles = append([]string{}, target.Sources...)
		options.VitestProject = target.Project
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

func selectTestPackage(ir *manifest.ManifestIR, requested string) *manifest.Package {
	for index := range ir.Packages {
		if ir.Packages[index].Name == requested {
			return &ir.Packages[index]
		}
	}
	return nil
}

func selectTestTarget(pkg *manifest.Package, requested string) *manifest.TestTarget {
	for index := range pkg.TestTargets {
		if pkg.TestTargets[index].Name == requested {
			return &pkg.TestTargets[index]
		}
	}
	return nil
}

func testTargetSuggestion(requested string, targets []manifest.TestTarget) string {
	type candidate struct {
		name     string
		distance int
	}
	candidates := []candidate{}
	for _, target := range targets {
		distance := editDistance(requested, target.Name)
		if distance <= 3 || strings.Contains(target.Name, requested) || strings.Contains(requested, target.Name) {
			candidates = append(candidates, candidate{name: target.Name, distance: distance})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].distance != candidates[j].distance {
			return candidates[i].distance < candidates[j].distance
		}
		return candidates[i].name < candidates[j].name
	})
	if len(candidates) > 3 {
		candidates = candidates[:3]
	}
	if len(candidates) == 0 {
		return ""
	}
	names := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		names = append(names, candidate.name)
	}
	return "did you mean: " + strings.Join(names, ", ")
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
