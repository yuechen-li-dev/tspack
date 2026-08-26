package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/project"
)

func runUpdateCommand(args []string) {
	paths := newLifecycleProjectPaths()
	dryRun := false
	policy := false
	quiet := false
	jsonOutput := false
	query := ""
	for index := 1; index < len(args); index++ {
		if paths.consume(args, &index) {
			continue
		}
		switch args[index] {
		case "--dry-run":
			dryRun = true
		case "--policy":
			policy = true
		case "--quiet":
			quiet = true
		case "--json":
			jsonOutput = true
		default:
			if strings.HasPrefix(args[index], "-") {
				failUnknownLifecycleFlag("update", args[index])
			}
			if query != "" {
				fmt.Fprintln(os.Stderr, "update accepts at most one query")
				exit(1)
			}
			query = args[index]
		}
	}
	if policy && !dryRun {
		fmt.Fprintln(os.Stderr, "TSPACK_UPDATE_POLICY_REQUIRES_DRY_RUN: policy-driven mutation is not implemented yet; use --dry-run")
		exit(1)
	}
	if policy && query != "" {
		fmt.Fprintln(os.Stderr, "TSPACK_UPDATE_POLICY_TARGET_UNSUPPORTED: targeted policy planning is not implemented in M50b; use workspace policy dry-run")
		exit(1)
	}
	if !policy && !quiet && !jsonOutput {
		paths.Options.Progress = project.Progress{Enabled: true, Writer: os.Stderr}
	}
	if policy {
		plan := project.RunPolicyPlan(project.PolicyPlanRequest{Project: paths.Options})
		result := project.Result{Diagnostics: plan.Diagnostics, Outdated: plan.Outdated}
		renderPolicyUpdate(paths.Options, result, jsonOutput)
		return
	}
	operation := project.RunUpdate(project.UpdateRequest{Project: paths.Options, Query: query, DryRun: dryRun})
	result := project.Result{
		Diagnostics:  operation.Diagnostics,
		LockDiff:     operation.LockDiff,
		DryRun:       operation.DryRun,
		UpdateTarget: operation.Target,
	}
	if dryRun && jsonOutput {
		writeUpdateJSON(paths.Options, result)
		return
	}
	renderHumanDiagnostics(os.Stderr, result.Diagnostics, checkRenderOptions{})
	if result.LockDiff != nil {
		if dryRun {
			printUpdateDryRunPlan(result)
		} else {
			fmt.Printf("lockfile diff: +%d -%d\n", len(result.LockDiff.PackagesAdded), len(result.LockDiff.PackagesRemoved))
			printUpdateAttribution(result)
		}
	}
	exitForDiagnostics(result.Diagnostics)
}

func renderPolicyUpdate(options project.Options, result project.Result, jsonOutput bool) {
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(buildPolicyUpdateDryRunJSONReport(options, result)); err != nil {
			fmt.Fprintf(os.Stderr, "TSPACK_UPDATE_JSON_ENCODE_FAILED: %v\n", err)
			exit(1)
		}
		exitForDiagnostics(result.Diagnostics)
		return
	}
	renderHumanDiagnostics(os.Stderr, result.Diagnostics, checkRenderOptions{})
	printPolicyUpdateDryRunPlan(result)
	exitForDiagnostics(result.Diagnostics)
}

func writeUpdateJSON(options project.Options, result project.Result) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(buildUpdateDryRunJSONReport(options, result)); err != nil {
		fmt.Fprintf(os.Stderr, "TSPACK_UPDATE_JSON_ENCODE_FAILED: %v\n", err)
		exit(1)
	}
	exitForDiagnostics(result.Diagnostics)
}

func printUpdateDryRunPlan(result project.Result) {
	fmt.Println("TSPack update dry run")
	fmt.Println()
	if result.UpdateTarget != nil && result.UpdateTarget.Targeted {
		fmt.Println("Target:")
		fmt.Printf("  %s\n", result.UpdateTarget.Query)
		fmt.Println()
	}
	fmt.Println("Lockfile changes:")
	diff := result.LockDiff
	if diff == nil || (len(diff.PackagesAdded) == 0 && len(diff.PackagesRemoved) == 0 && len(diff.PackagesChanged) == 0) {
		fmt.Println("  none")
		fmt.Println()
		fmt.Println("No files were written.")
		return
	}
	if len(diff.PackagesAdded) > 0 {
		fmt.Println("  added:")
		for _, pkg := range diff.PackagesAdded {
			fmt.Printf("    %s (%s)\n", pkg.ID, updatePackageKind(result, pkg.Name))
		}
	}
	if len(diff.PackagesChanged) > 0 {
		fmt.Println("  changed:")
		for _, ch := range diff.PackagesChanged {
			fmt.Printf("    %s -> %s (%s)\n", ch.Old.ID, ch.New.ID, updatePackageKind(result, ch.New.Name))
		}
	}
	if len(diff.PackagesRemoved) > 0 {
		fmt.Println("  removed:")
		for _, pkg := range diff.PackagesRemoved {
			fmt.Printf("    %s (%s)\n", pkg.ID, updatePackageKind(result, pkg.Name))
		}
	}
	fmt.Println()
	fmt.Println("No files were written.")
}

func updatePackageKind(result project.Result, packageName string) string {
	if result.UpdateTarget != nil {
		for _, name := range result.UpdateTarget.DirectPackages {
			if name == packageName {
				return "direct"
			}
		}
	}
	return "transitive"
}

func printUpdateAttribution(result project.Result) {
	if result.LockDiff == nil {
		return
	}
	direct, transitive := 0, 0
	count := func(name string) {
		if updatePackageKind(result, name) == "direct" {
			direct++
		} else {
			transitive++
		}
	}
	for _, pkg := range result.LockDiff.PackagesAdded {
		count(pkg.Name)
	}
	for _, pkg := range result.LockDiff.PackagesRemoved {
		count(pkg.Name)
	}
	for _, change := range result.LockDiff.PackagesChanged {
		count(change.New.Name)
	}
	fmt.Printf("change attribution: %d direct, %d transitive closure\n", direct, transitive)
	if result.UpdateTarget != nil && !result.UpdateTarget.Targeted && direct+transitive >= 10 {
		fmt.Println("hint: preview or limit broad changes with `tspack update <dependency> --dry-run`.")
	}
}

func buildPolicyUpdateDryRunJSONReport(opts project.Options, result project.Result) PolicyUpdateDryRunJSONReport {
	plan := buildPolicyUpdatePlan(result.Outdated)
	report := PolicyUpdateDryRunJSONReport{
		Command: "update",
		DryRun: UpdateDryRunJSONState{
			Enabled: true,
			Changed: false,
			Summary: UpdateDryRunSummary{},
		},
		OK:         !hasErrors(result.Diagnostics),
		Root:       opts.RootDir,
		PolicyPlan: plan,
	}
	report.Diagnostics = updateDiagnosticsJSON(result.Diagnostics)
	return report
}

func buildPolicyUpdatePlan(outdated *project.OutdatedResult) PolicyUpdatePlanJSON {
	plan := PolicyUpdatePlanJSON{
		SecurityGatesEvaluated: true,
		SecurityGateStatus:     "not_applicable",
		Allowed:                []PolicyUpdateCandidate{},
		Blocked:                []PolicyUpdateCandidate{},
		Unclassified:           []PolicyUpdateCandidate{},
		NotApplicable:          []PolicyUpdateCandidate{},
		Noop:                   []PolicyUpdateCandidate{},
	}
	if outdated == nil {
		return plan
	}
	plan.PolicyPresent = outdated.HasPolicy
	for _, dep := range outdatedHumanEntries(outdated, false) {
		candidate := policyUpdateCandidate(dep)
		applyPolicySecurityGate(&candidate, dep, outdated.Security)
		switch candidate.PolicyStatus {
		case "allowed":
			plan.Allowed = append(plan.Allowed, candidate)
		case "blocked-manual", "pinned", "outside-policy-level":
			plan.Blocked = append(plan.Blocked, candidate)
		case "not-applicable":
			plan.NotApplicable = append(plan.NotApplicable, candidate)
		case "unclassified":
			plan.Unclassified = append(plan.Unclassified, candidate)
		default:
			if dep.Status == "current" {
				candidate.PolicyStatus = "current"
				candidate.Action = "noop"
				candidate.Message = "dependency is already current"
				plan.Noop = append(plan.Noop, candidate)
			}
		}
	}
	plan.Summary = summarizePolicyUpdatePlan(plan)
	plan.WouldUpdate = plan.Summary.Allowed > 0
	plan.WouldApply = plan.Summary.Ready > 0
	plan.SecurityGateStatus = summarizePolicySecurityGateStatus(plan)
	return plan
}

func policyUpdateCandidate(dep project.OutdatedDependency) PolicyUpdateCandidate {
	status := dep.PolicyStatus
	action := "noop"
	message := dep.PolicyMessage
	switch status {
	case "allowed":
		action = "update"
	case "blocked-manual":
		action = "manual"
	case "pinned":
		action = "pinned"
	case "outside-policy-level":
		action = "outside-policy"
	case "unclassified":
		action = "unclassified"
	case "not-applicable":
		action = "not-applicable"
	default:
		if dep.Status == "current" {
			status = "current"
			message = "dependency is already current"
		}
	}
	if message == "" {
		message = status
	}
	return PolicyUpdateCandidate{
		Name:               dep.Name,
		Kind:               dep.Kind,
		Requested:          dep.Requested,
		Current:            dep.Current,
		Wanted:             dep.Wanted,
		Latest:             dep.Latest,
		Packages:           dep.Packages,
		PackageCount:       dep.PackageCount,
		PolicyStrategy:     dep.PolicyStrategy,
		PolicyLevel:        dep.PolicyLevel,
		PolicyStatus:       status,
		PolicyReason:       dep.PolicyReason,
		Action:             action,
		Message:            message,
		SecurityGateStatus: "not_applicable",
		EffectiveAction:    action,
	}
}

func applyPolicySecurityGate(candidate *PolicyUpdateCandidate, dep project.OutdatedDependency, security manifest.Security) {
	gate := project.EvaluatePolicySecurityGate(dep, security)
	candidate.SecurityGateStatus = gate.Status
	candidate.SecurityGateReasons = gate.Reasons
	candidate.SecurityGateDiagnostics = gate.Diagnostics
	candidate.EffectiveAction = policyEffectiveAction(candidate.PolicyStatus, gate.Status)
	if candidate.PolicyStatus == "allowed" && len(gate.Reasons) > 0 {
		candidate.Message = candidate.Message + ", security: " + strings.Join(gate.Reasons, "; ")
	}
}

func policyEffectiveAction(policyStatus string, securityStatus string) string {
	if policyStatus != "allowed" {
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

func summarizePolicyUpdatePlan(plan PolicyUpdatePlanJSON) PolicyUpdatePlanSummary {
	summary := PolicyUpdatePlanSummary{
		Allowed:       len(plan.Allowed),
		Blocked:       len(plan.Blocked),
		Unclassified:  len(plan.Unclassified),
		NotApplicable: len(plan.NotApplicable),
		Noop:          len(plan.Noop),
	}
	for _, candidate := range plan.Allowed {
		switch candidate.SecurityGateStatus {
		case "passed":
			summary.Ready++
		case "blocked":
			summary.SecurityBlocked++
		case "review_required":
			summary.ReviewRequired++
		}
	}
	return summary
}

func summarizePolicySecurityGateStatus(plan PolicyUpdatePlanJSON) string {
	statuses := map[string]bool{}
	for _, candidates := range [][]PolicyUpdateCandidate{
		plan.Allowed,
		plan.Blocked,
		plan.Unclassified,
		plan.NotApplicable,
		plan.Noop,
	} {
		for _, candidate := range candidates {
			statuses[candidate.SecurityGateStatus] = true
		}
	}
	if len(statuses) == 0 {
		return "not_applicable"
	}
	if len(statuses) == 1 {
		for status := range statuses {
			return status
		}
	}
	return "mixed"
}

func updateDiagnosticsJSON(diags []diag.Diagnostic) []CheckJSONDiagnostic {
	sorted := append([]diag.Diagnostic(nil), diags...)
	diag.SortDiagnostics(sorted)
	out := make([]CheckJSONDiagnostic, 0, len(sorted))
	for _, d := range sorted {
		out = append(out, CheckJSONDiagnostic{Code: d.Code, Severity: string(d.Severity), Message: d.Message, Details: d.Details})
	}
	return out
}

func printPolicyUpdateDryRunPlan(result project.Result) {
	plan := buildPolicyUpdatePlan(result.Outdated)
	fmt.Println("Policy update plan (dry run)")
	fmt.Println("No lockfile changes will be written.")
	fmt.Println()
	if result.Outdated != nil && !result.Outdated.HasPolicy {
		fmt.Println("No update policy declared.")
		fmt.Println("Candidates are unclassified; use <UpdatePolicy> to declare rolling/manual/pinned intent.")
		fmt.Println()
	}
	if plan.Summary.Allowed == 0 && plan.Summary.Blocked == 0 && plan.Summary.Unclassified == 0 && plan.Summary.NotApplicable == 0 {
		fmt.Println("No policy-eligible updates found.")
		fmt.Println("security gates: evaluated")
		fmt.Println("lifecycle execution remains blocked")
		fmt.Println("lockfile written: no")
		return
	}
	printPolicyCandidates("Ready:", filterPolicyCandidatesByEffectiveAction(plan.Allowed, "update"))
	printPolicyCandidates("Needs review:", filterPolicyCandidatesByEffectiveAction(plan.Allowed, "review"))
	printPolicyCandidates("Blocked by security:", filterPolicyCandidatesByEffectiveAction(plan.Allowed, "blocked"))
	printPolicyCandidates("Blocked by policy:", plan.Blocked)
	printPolicyCandidates("Unclassified:", plan.Unclassified)
	printPolicyCandidates("Not applicable:", plan.NotApplicable)
	fmt.Println("Summary:")
	fmt.Printf("version-policy allowed: %d\n", plan.Summary.Allowed)
	fmt.Printf("ready: %d\n", plan.Summary.Ready)
	fmt.Printf("review required: %d\n", plan.Summary.ReviewRequired)
	fmt.Printf("security blocked: %d\n", plan.Summary.SecurityBlocked)
	fmt.Printf("policy blocked: %d\n", plan.Summary.Blocked)
	fmt.Printf("unclassified: %d\n", plan.Summary.Unclassified)
	fmt.Printf("not applicable: %d\n", plan.Summary.NotApplicable)
	fmt.Println("security gates: evaluated")
	fmt.Println("security model: current TSPack lifecycle/acknowledgment model")
	fmt.Println("lifecycle execution remains blocked")
	fmt.Println("lockfile written: no")
}

func filterPolicyCandidatesByEffectiveAction(candidates []PolicyUpdateCandidate, action string) []PolicyUpdateCandidate {
	filtered := []PolicyUpdateCandidate{}
	for _, candidate := range candidates {
		if candidate.EffectiveAction == action {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func printPolicyCandidates(title string, candidates []PolicyUpdateCandidate) {
	if len(candidates) == 0 {
		return
	}
	fmt.Println(title)
	for _, candidate := range candidates {
		fmt.Printf("%s %s %s -> %s", candidate.Name, candidate.Kind, formatPolicyVersion(candidate.Current), candidate.Latest)
		if candidate.PolicyStrategy != "" {
			fmt.Printf(" %s", candidate.PolicyStrategy)
			if candidate.PolicyLevel != "" {
				fmt.Printf(":%s", candidate.PolicyLevel)
			}
		}
		fmt.Printf(" packages: %d", candidate.PackageCount)
		if candidate.Message != "" {
			fmt.Printf(" %s", candidate.Message)
		}
		fmt.Printf("\n  security: %s", strings.ReplaceAll(candidate.SecurityGateStatus, "_", " "))
		if len(candidate.SecurityGateReasons) > 0 {
			fmt.Printf(" — %s", strings.Join(candidate.SecurityGateReasons, "; "))
		}
		fmt.Println()
	}
	fmt.Println()
}

func formatPolicyVersion(versions []string) string {
	if len(versions) == 0 {
		return "-"
	}
	return strings.Join(versions, ",")
}

func buildUpdateDryRunJSONReport(opts project.Options, result project.Result) UpdateDryRunJSONReport {
	report := UpdateDryRunJSONReport{
		Command: "update",
		DryRun:  UpdateDryRunJSONState{Enabled: true},
		OK:      !hasErrors(result.Diagnostics),
		Root:    opts.RootDir,
	}
	if result.UpdateTarget != nil {
		report.Targeted = result.UpdateTarget.Targeted
		report.Query = result.UpdateTarget.Query
		report.Selected = result.UpdateTarget.Selected
	}
	if result.DryRun != nil {
		report.Changed = result.DryRun.Changed
		report.Summary = UpdateDryRunSummary(result.DryRun.Summary)
		report.DryRun.Changed = result.DryRun.Changed
		report.DryRun.Summary = UpdateDryRunSummary(result.DryRun.Summary)
	}
	if result.LockDiff != nil {
		for _, pkg := range result.LockDiff.PackagesAdded {
			report.Changes.Added = append(report.Changes.Added, lockDiffPackage{ID: pkg.ID, Name: pkg.Name, Version: pkg.Version, Source: pkg.Source, Direct: updatePackageKind(result, pkg.Name) == "direct"})
		}
		for _, pkg := range result.LockDiff.PackagesRemoved {
			report.Changes.Removed = append(report.Changes.Removed, lockDiffPackage{ID: pkg.ID, Name: pkg.Name, Version: pkg.Version, Source: pkg.Source, Direct: updatePackageKind(result, pkg.Name) == "direct"})
		}
		for _, ch := range result.LockDiff.PackagesChanged {
			report.Changes.Changed = append(report.Changes.Changed, lockDiffPackageChange{
				From: lockDiffPackage{ID: ch.Old.ID, Name: ch.Old.Name, Version: ch.Old.Version, Source: ch.Old.Source, Direct: updatePackageKind(result, ch.Old.Name) == "direct"},
				To:   lockDiffPackage{ID: ch.New.ID, Name: ch.New.Name, Version: ch.New.Version, Source: ch.New.Source, Direct: updatePackageKind(result, ch.New.Name) == "direct"},
			})
		}
	}
	diags := append([]diag.Diagnostic(nil), result.Diagnostics...)
	diag.SortDiagnostics(diags)
	for _, d := range diags {
		report.Diagnostics = append(report.Diagnostics, CheckJSONDiagnostic{
			Code:     d.Code,
			Severity: string(d.Severity),
			Message:  d.Message,
			Details:  d.Details,
		})
	}
	return report
}
