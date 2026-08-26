package project

import (
	"github.com/yuechen-li-dev/tspack/internal/check"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/why"
)

// CheckRequest contains the explicit inputs for the read-only project check.
type CheckRequest struct {
	Project     Options
	ExplainFile string
}

// CheckResult is the semantic output of a check, independent of CLI rendering.
type CheckResult struct {
	Diagnostics []diag.Diagnostic
	Explanation *check.ExplainResult
}

func RunCheck(request CheckRequest) CheckResult {
	var result Result
	if request.ExplainFile != "" {
		result = CheckExplain(request.Project, request.ExplainFile)
	} else {
		result = Check(request.Project)
	}
	return CheckResult{
		Diagnostics: result.Diagnostics,
		Explanation: result.Explain,
	}
}

// UpdateRequest makes both mutation authority and targeted selection explicit.
type UpdateRequest struct {
	Project Options
	Query   string
	DryRun  bool
}

// UpdateResult describes resolution and lockfile changes without presentation data.
type UpdateResult struct {
	Diagnostics []diag.Diagnostic
	LockDiff    *lockfile.Diff
	DryRun      *UpdateDryRunResult
	Target      *UpdateTargetResult
}

func RunUpdate(request UpdateRequest) UpdateResult {
	options := UpdateOptions{Query: request.Query}
	var result Result
	if request.DryRun {
		result = UpdateDryRunWithOptions(request.Project, options)
	} else {
		result = UpdateWithOptions(request.Project, options)
	}
	return UpdateResult{
		Diagnostics: result.Diagnostics,
		LockDiff:    result.LockDiff,
		DryRun:      result.DryRun,
		Target:      result.UpdateTarget,
	}
}

// SyncRequest describes materialization from lockfile truth. Sync never writes the lockfile.
type SyncRequest struct {
	Project Options
	Clean   bool
	Force   bool
}

type SyncResult struct {
	Diagnostics []diag.Diagnostic
}

func RunSync(request SyncRequest) SyncResult {
	result := Sync(request.Project, request.Clean, request.Force)
	return SyncResult{Diagnostics: result.Diagnostics}
}

// PackRequest describes artifact creation. Pack does not resolve or mutate dependencies.
type PackRequest struct {
	Project Options
	Options PackOptions
}

type PackOperationResult struct {
	Diagnostics []diag.Diagnostic
	Pack        *PackResult
}

func RunPack(request PackRequest) PackOperationResult {
	result := Pack(request.Project, request.Options)
	return PackOperationResult{
		Diagnostics: result.Diagnostics,
		Pack:        result.PackResult,
	}
}

// WhyRequest describes a read-only dependency explanation.
type WhyRequest struct {
	Project Options
	Options WhyOptions
}

type WhyOperationResult struct {
	Diagnostics []diag.Diagnostic
	Explanation *why.Result
}

func RunWhy(request WhyRequest) WhyOperationResult {
	result := Why(request.Project, request.Options)
	return WhyOperationResult{
		Diagnostics: result.Diagnostics,
		Explanation: result.WhyResult,
	}
}

// OutdatedRequest describes read-only package-version observation.
type OutdatedRequest struct {
	Project Options
}

type OutdatedOperationResult struct {
	Diagnostics []diag.Diagnostic
	Outdated    *OutdatedResult
}

func RunOutdated(request OutdatedRequest) OutdatedOperationResult {
	result := Outdated(request.Project)
	return OutdatedOperationResult{
		Diagnostics: result.Diagnostics,
		Outdated:    result.Outdated,
	}
}

// PolicyPlanRequest describes a read-only update-policy planning operation.
type PolicyPlanRequest struct {
	Project Options
}

// PolicyPlanCandidate couples observed version movement with the security gate
// that applies before a policy-authorized update could be admitted.
type PolicyPlanCandidate struct {
	Dependency      OutdatedDependency
	SecurityGate    PolicySecurityGateResult
	EffectiveAction string
}

type PolicyPlanResult struct {
	Diagnostics   []diag.Diagnostic
	PolicyPresent bool
	Security      manifest.Security
	Outdated      *OutdatedResult
	Candidates    []PolicyPlanCandidate
}

func RunPolicyPlan(request PolicyPlanRequest) PolicyPlanResult {
	observed := RunOutdated(OutdatedRequest{Project: request.Project})
	result := PolicyPlanResult{
		Diagnostics: observed.Diagnostics,
		Outdated:    observed.Outdated,
	}
	if observed.Outdated == nil {
		return result
	}
	result.PolicyPresent = observed.Outdated.HasPolicy
	result.Security = observed.Outdated.Security
	for _, dependency := range observed.Outdated.Groups {
		gate := EvaluatePolicySecurityGate(dependency, result.Security)
		result.Candidates = append(result.Candidates, PolicyPlanCandidate{
			Dependency:      dependency,
			SecurityGate:    gate,
			EffectiveAction: policyPlanEffectiveAction(dependency.PolicyStatus, gate.Status),
		})
	}
	return result
}

func policyPlanEffectiveAction(policyStatus string, securityStatus string) string {
	if policyStatus != policyStatusAllowed {
		return "skip"
	}
	switch securityStatus {
	case "passed":
		return "update"
	case "review_required":
		return "review"
	case "blocked":
		return "blocked"
	default:
		return "skip"
	}
}
