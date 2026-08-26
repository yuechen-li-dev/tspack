package cli

import (
	"encoding/json"
	"fmt"
	"github.com/yuechen-li-dev/tspack/internal/capability"
	"github.com/yuechen-li-dev/tspack/internal/check"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/project"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func runCheckCommand(args []string) {
	if len(args) > 1 && (args[1] == "--help" || args[1] == "-h") {
		printCheckHelp()
		return
	}
	paths := newLifecycleProjectPaths()
	jsonOutput := false
	format := false
	showConflicts := false
	showLifecycle := false
	explainFile := ""
	positionals := []string{}
	for index := 1; index < len(args); index++ {
		if paths.consume(args, &index) {
			continue
		}
		switch args[index] {
		case "--json":
			jsonOutput = true
		case "--format":
			format = true
		case "--show-conflicts":
			showConflicts = true
		case "--show-lifecycle":
			showLifecycle = true
		case "--explain":
			if explainFile != "" || index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				fmt.Fprintln(os.Stderr, "TSPACK_CHECK_EXPLAIN_FILE_REQUIRED: --explain requires exactly one file path")
				os.Exit(1)
			}
			explainFile = lifecycleFlagValue(args, &index, "--explain")
		default:
			if strings.HasPrefix(args[index], "-") {
				failUnknownLifecycleFlag("check", args[index])
			}
			positionals = append(positionals, args[index])
		}
	}
	if explainFile != "" && len(positionals) > 0 {
		fmt.Fprintln(os.Stderr, "TSPACK_CHECK_EXPLAIN_FILE_REQUIRED: --explain requires exactly one file path")
		os.Exit(1)
	}
	operation := project.RunCheck(project.CheckRequest{Project: paths.Options, ExplainFile: explainFile})
	result := project.Result{Diagnostics: operation.Diagnostics, Explain: operation.Explanation}
	if explainFile != "" {
		renderCheckExplanation(result, jsonOutput)
		return
	}
	if format {
		formatResult := runCheckFormatValidation(paths.Options.RootDir, jsonOutput)
		result.Diagnostics = append(result.Diagnostics, formatResult.Diagnostics...)
	}
	if jsonOutput {
		writeCheckJSON(paths.Options, result)
		return
	}
	renderHumanDiagnostics(os.Stderr, result.Diagnostics, checkRenderOptions{ShowConflicts: showConflicts, ShowLifecycle: showLifecycle})
	exitForDiagnostics(result.Diagnostics)
}

func renderCheckExplanation(result project.Result, jsonOutput bool) {
	if result.Explain != nil && jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result.Explain); err != nil {
			fmt.Fprintf(os.Stderr, "TSPACK_CHECK_EXPLAIN_FAILED: %v\n", err)
			os.Exit(1)
		}
		exitForDiagnostics(result.Diagnostics)
		return
	}
	if result.Explain != nil {
		printCheckExplain(result.Explain)
		exitForDiagnostics(result.Diagnostics)
		return
	}
	for _, diagnostic := range result.Diagnostics {
		fmt.Fprintf(os.Stderr, "%s: %s\n", diagnostic.Code, diagnostic.Message)
	}
	os.Exit(1)
}

func writeCheckJSON(options project.Options, result project.Result) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(buildCheckJSONReport(options, result)); err != nil {
		fmt.Fprintf(os.Stderr, "TSPACK_CHECK_JSON_ENCODE_FAILED: %v\n", err)
		os.Exit(1)
	}
	exitForDiagnostics(result.Diagnostics)
}

func deriveCheckFormatPaths(root string) []string {
	paths := map[string]bool{}
	addPath := func(path string) {
		cleaned := filepath.ToSlash(filepath.Clean(path))
		if cleaned == "." || cleaned == "" || strings.HasPrefix(cleaned, "../") || isGeneratedFormatPath(cleaned) {
			return
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(cleaned))); err == nil {
			paths[cleaned] = true
		}
	}
	addParentDir := func(path string) {
		dir := filepath.ToSlash(filepath.Dir(path))
		if dir == "." {
			addPath(path)
			return
		}
		addPath(dir)
	}

	addPath("manifest.tsx")
	addPath("package.json")
	addPath("src")

	lockPath := filepath.Join(root, "ts-lock.toml")
	if lf, _, err := lockfile.LoadFile(lockPath); err == nil {
		for _, pkg := range lf.Packages {
			if pkg.Path != "" {
				addPath(filepath.ToSlash(filepath.Join(pkg.Path, "src")))
				addPath(filepath.ToSlash(filepath.Join(pkg.Path, "package.json")))
			}
		}
		for _, target := range lf.Targets {
			if target.Entry != "" {
				addParentDir(target.Entry)
			}
			if target.Types != "" && !isGeneratedFormatPath(target.Types) {
				addParentDir(target.Types)
			}
		}
	}

	out := make([]string, 0, len(paths))
	for path := range paths {
		out = append(out, path)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return []string{"."}
	}
	return out
}

func isGeneratedFormatPath(path string) bool {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	first, _, _ := strings.Cut(cleaned, "/")
	switch first {
	case ".tspack", "node_modules", "dist", "tspack-artifacts", "coverage", ".git", "build", ".turbo", ".vite":
		return true
	default:
		return false
	}
}

func runCheckFormatValidation(root string, jsonOutput bool) biomeCommandResult {
	options := biomeCommandOptions{
		Command:                  "format",
		Root:                     root,
		Paths:                    deriveCheckFormatPaths(root),
		UseCheck:                 true,
		CaptureOutput:            jsonOutput,
		PrintDefaultConfigStatus: !jsonOutput,
	}
	result := runBiomeCommandWithOptions(options)
	for i, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "TSPACK_BIOME_BACKEND_NOT_FOUND" {
			result.Diagnostics[i] = newCheckFormatBackendMissingDiagnostic(diagnostic)
		}
	}
	return result
}

func newCheckFormatBackendMissingDiagnostic(underlying diag.Diagnostic) diag.Diagnostic {
	details := []string{
		"Install/configure the formatter backend or add the configured tool dependency.",
		"current backend: biome",
		"underlying: " + underlying.Code,
	}
	details = append(details, underlying.Details...)
	return diag.Diagnostic{
		Code:     "TSPACK_FORMAT_BACKEND_MISSING",
		Severity: diag.SeverityError,
		Message:  "format backend is not available",
		Details:  details,
	}
}

type checkRenderOptions struct {
	ShowConflicts bool
	ShowLifecycle bool
}

func renderHumanDiagnostics(out *os.File, diagnostics []diag.Diagnostic, options checkRenderOptions) {
	conflicts := []diag.Diagnostic{}
	lifecycle := []diag.Diagnostic{}
	other := []diag.Diagnostic{}
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "TSPACK_LOCK_VERSION_CONFLICT" && diagnostic.Severity != diag.SeverityError {
			conflicts = append(conflicts, diagnostic)
			continue
		}
		if diagnostic.Code == "TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT" && diagnostic.Severity != diag.SeverityError {
			lifecycle = append(lifecycle, diagnostic)
			continue
		}
		other = append(other, diagnostic)
	}

	rendered := append([]diag.Diagnostic{}, other...)
	if options.ShowConflicts || len(conflicts) < 2 {
		rendered = append(rendered, conflicts...)
	} else {
		rendered = append(rendered, versionConflictSummaryDiagnostic(conflicts))
	}
	if options.ShowLifecycle {
		rendered = append(rendered, lifecycle...)
	} else {
		unacknowledgedLifecycle := unacknowledgedLifecycleDiagnostics(lifecycle)
		if len(unacknowledgedLifecycle) == 0 && len(lifecycle) > 0 {
			rendered = append(rendered, lifecycleAllAcknowledgedSummaryDiagnostic(lifecycle))
		} else if len(unacknowledgedLifecycle) < 2 {
			rendered = append(rendered, unacknowledgedLifecycle...)
			if len(categoryAcknowledgedLifecycleDiagnostics(lifecycle)) > 0 {
				rendered = append(rendered, lifecycleSummaryDiagnostic(lifecycle))
			}
		} else {
			rendered = append(rendered, lifecycleSummaryDiagnostic(lifecycle))
		}
	}
	diag.SortDiagnostics(rendered)
	for _, diagnostic := range rendered {
		printHumanDiagnostic(out, diagnostic)
	}
}

func printHumanDiagnostic(out *os.File, diagnostic diag.Diagnostic) {
	fmt.Fprintf(out, "%s: %s\n", diagnostic.Code, diagnostic.Message)
	for _, detail := range diagnostic.Details {
		if detail == diagnostic.Message {
			continue
		}
		fmt.Fprintf(out, "  %s\n", detail)
	}
}

func versionConflictSummaryDiagnostic(conflicts []diag.Diagnostic) diag.Diagnostic {
	examples := versionConflictExamples(conflicts, 3)
	return diag.Diagnostic{
		Code:     "TSPACK_LOCK_VERSION_CONFLICT",
		Severity: diag.SeverityWarning,
		Message:  fmt.Sprintf("Version conflicts: %d packages have multiple resolved versions.", len(conflicts)),
		Details: []string{
			"Examples: " + strings.Join(examples, ", "),
			"Run `tspack check --show-conflicts` for full conflict diagnostics.",
		},
	}
}

func unacknowledgedLifecycleDiagnostics(lifecycle []diag.Diagnostic) []diag.Diagnostic {
	out := []diag.Diagnostic{}
	for _, diagnostic := range lifecycle {
		if lifecycleDiagnosticDetail(diagnostic, "acknowledgmentKind") == "lifecycle-category" {
			continue
		}
		out = append(out, diagnostic)
	}
	return out
}

func categoryAcknowledgedLifecycleDiagnostics(lifecycle []diag.Diagnostic) []diag.Diagnostic {
	out := []diag.Diagnostic{}
	for _, diagnostic := range lifecycle {
		if lifecycleDiagnosticDetail(diagnostic, "acknowledgmentKind") == "lifecycle-category" {
			out = append(out, diagnostic)
		}
	}
	return out
}

func lifecycleAllAcknowledgedSummaryDiagnostic(lifecycle []diag.Diagnostic) diag.Diagnostic {
	counts := lifecycleCategoryCounts(lifecycle)
	message := fmt.Sprintf("Lifecycle scripts: %d scripts acknowledged by category policy; execution remains blocked.", len(lifecycle))
	if counts[capability.LifecycleCategoryMaintainerPublish] == len(lifecycle) {
		message = fmt.Sprintf("Lifecycle scripts: %d maintainer-side scripts acknowledged by category policy; execution remains blocked.", len(lifecycle))
	}
	return diag.Diagnostic{
		Code:     "TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT",
		Severity: diag.SeverityInfo,
		Message:  message,
		Details: []string{
			"Run `tspack check --show-lifecycle` for full script and acknowledgment details.",
			"Run `tspack doctor security` for audit details.",
		},
	}
}
func lifecycleSummaryDiagnostic(lifecycle []diag.Diagnostic) diag.Diagnostic {
	counts := lifecycleCategoryCounts(lifecycle)
	consumerExamples := lifecycleExamplesByCategory(lifecycle, capability.LifecycleCategoryConsumerInstall, 2)
	maintainerExamples := lifecycleExamplesByCategory(lifecycle, capability.LifecycleCategoryMaintainerPublish, 3)
	otherExamples := lifecycleExamplesByCategory(lifecycle, capability.LifecycleCategoryOther, 3)
	details := []string{}
	if len(consumerExamples) > 0 {
		details = append(details, "Consumer examples: "+strings.Join(consumerExamples, ", "))
	}
	if len(maintainerExamples) > 0 {
		details = append(details, "Maintainer examples: "+strings.Join(maintainerExamples, ", "))
	}
	if len(otherExamples) > 0 {
		details = append(details, "Other examples: "+strings.Join(otherExamples, ", "))
	}
	if counts[capability.LifecycleCategoryConsumerInstall] == 0 && counts[capability.LifecycleCategoryMaintainerPublish] > 0 && counts[capability.LifecycleCategoryOther] == 0 {
		details = append(details, "These do not run during normal consumer install in npm-style workflows.")
	}
	details = append(details,
		"Run `tspack check --show-lifecycle` for full script and pull-chain details.",
		"Run `tspack doctor security` for policy posture.",
	)
	message := lifecycleSummaryMessage(counts)
	categoryAcknowledgedCount := len(categoryAcknowledgedLifecycleDiagnostics(lifecycle))
	unacknowledgedCount := len(unacknowledgedLifecycleDiagnostics(lifecycle))
	if categoryAcknowledgedCount > 0 {
		details = append([]string{fmt.Sprintf("%d lifecycle scripts acknowledged by lifecycle category policy.", categoryAcknowledgedCount)}, details...)
	}
	if unacknowledgedCount > 0 {
		details = append([]string{fmt.Sprintf("%d lifecycle scripts remain unacknowledged.", unacknowledgedCount)}, details...)
	}
	return diag.Diagnostic{
		Code:     "TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT",
		Severity: diag.SeverityWarning,
		Message:  message,
		Details:  details,
	}
}

func lifecycleSummaryMessage(counts map[string]int) string {
	parts := []string{}
	if counts[capability.LifecycleCategoryConsumerInstall] > 0 {
		parts = append(parts, fmt.Sprintf("%d consumer install-time scripts", counts[capability.LifecycleCategoryConsumerInstall]))
	}
	if counts[capability.LifecycleCategoryMaintainerPublish] > 0 {
		parts = append(parts, fmt.Sprintf("%d maintainer-side scripts", counts[capability.LifecycleCategoryMaintainerPublish]))
	}
	if counts[capability.LifecycleCategoryOther] > 0 {
		parts = append(parts, fmt.Sprintf("%d other lifecycle scripts", counts[capability.LifecycleCategoryOther]))
	}
	return "Lifecycle scripts: " + strings.Join(parts, " and ") + " found; execution is blocked by policy."
}

func versionConflictExamples(conflicts []diag.Diagnostic, limit int) []string {
	examples := []string{}
	for _, conflict := range sortedDiagnosticsForExamples(conflicts) {
		name := strings.TrimPrefix(conflict.Message, "package \"")
		if index := strings.Index(name, "\" appears at multiple versions"); index >= 0 {
			name = name[:index]
		}
		versions := []string{}
		for _, detail := range conflict.Details {
			trimmed := strings.TrimSpace(detail)
			if !strings.Contains(trimmed, " -> ") {
				continue
			}
			version := strings.SplitN(trimmed, " -> ", 2)[0]
			versions = append(versions, version)
		}
		examples = append(examples, fmt.Sprintf("%s (%s)", name, strings.Join(uniqueSorted(versions), ", ")))
		if len(examples) >= limit {
			return examples
		}
	}
	return examples
}

func lifecycleExamples(lifecycle []diag.Diagnostic, limit int) []string {
	examples := []string{}
	for _, diagnostic := range sortedDiagnosticsForExamples(lifecycle) {
		examples = append(examples, lifecyclePackageAndScript(diagnostic))
		if len(examples) >= limit {
			return examples
		}
	}
	return examples
}

func lifecycleExamplesByCategory(lifecycle []diag.Diagnostic, category string, limit int) []string {
	examples := []string{}
	for _, diagnostic := range sortedDiagnosticsForExamples(lifecycle) {
		if lifecycleDiagnosticDetail(diagnostic, "lifecycleCategory") != category {
			continue
		}
		examples = append(examples, lifecyclePackageAndScript(diagnostic))
		if len(examples) >= limit {
			return examples
		}
	}
	return examples
}

func lifecycleCategoryCounts(lifecycle []diag.Diagnostic) map[string]int {
	counts := map[string]int{
		capability.LifecycleCategoryConsumerInstall:   0,
		capability.LifecycleCategoryMaintainerPublish: 0,
		capability.LifecycleCategoryOther:             0,
	}
	for _, diagnostic := range lifecycle {
		category := lifecycleDiagnosticDetail(diagnostic, "lifecycleCategory")
		if category == "" {
			category = capability.LifecycleCategoryOther
		}
		counts[category]++
	}
	return counts
}

func lifecyclePackageAndScript(diagnostic diag.Diagnostic) string {
	script := lifecycleDiagnosticDetail(diagnostic, "lifecycleScriptName")
	if script == "" {
		script = lifecycleDiagnosticDetail(diagnostic, "script")
	}
	if script == "" {
		return lifecyclePackageName(diagnostic)
	}
	return lifecyclePackageName(diagnostic) + " " + script
}

func lifecycleDiagnosticDetail(diagnostic diag.Diagnostic, key string) string {
	prefix := key + ": "
	for _, detail := range diagnostic.Details {
		trimmed := strings.TrimSpace(detail)
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimPrefix(trimmed, prefix)
		}
	}
	return ""
}

func lifecyclePackageName(diagnostic diag.Diagnostic) string {
	for _, detail := range diagnostic.Details {
		trimmed := strings.TrimSpace(detail)
		if after, ok := strings.CutPrefix(trimmed, "package: "); ok {
			packageID := after
			if after, ok := strings.CutPrefix(packageID, "npm:"); ok {
				packageID = after
			}
			if at := strings.LastIndex(packageID, "@"); at > 0 {
				return packageID[:at]
			}
			return packageID
		}
	}
	return diagnostic.Message
}

func sortedDiagnosticsForExamples(diagnostics []diag.Diagnostic) []diag.Diagnostic {
	out := append([]diag.Diagnostic(nil), diagnostics...)
	diag.SortDiagnostics(out)
	return out
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	unique := []string{}
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
}

func buildCheckJSONReport(opts project.Options, result project.Result) CheckJSONReport {
	diags := append([]diag.Diagnostic(nil), result.Diagnostics...)
	diag.SortDiagnostics(diags)
	summary := CheckJSONSummary{}
	jsonDiagnostics := make([]CheckJSONDiagnostic, 0, len(diags))
	for _, d := range diags {
		switch d.Severity {
		case diag.SeverityError:
			summary.Errors++
		case diag.SeverityWarning:
			summary.Warnings++
		default:
			summary.Info++
		}
		summary.Total++
		jd := CheckJSONDiagnostic{
			Code:     d.Code,
			Severity: string(d.Severity),
			Message:  d.Message,
		}
		if d.File != "" {
			jd.File = d.File
		}
		if d.Code == "TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT" {
			jd.LifecycleScriptName = lifecycleDiagnosticDetail(d, "lifecycleScriptName")
			jd.LifecycleCategory = lifecycleDiagnosticDetail(d, "lifecycleCategory")
			consumerInstallTime := lifecycleDiagnosticDetail(d, "consumerInstallTime") == "true"
			jd.ConsumerInstallTime = &consumerInstallTime
			acknowledged := lifecycleDiagnosticDetail(d, "acknowledged") == "true"
			jd.Acknowledged = &acknowledged
			acknowledgmentKind := lifecycleDiagnosticDetail(d, "acknowledgmentKind")
			if acknowledgmentKind != "" && acknowledgmentKind != "null" {
				jd.AcknowledgmentKind = &acknowledgmentKind
			}
			jd.AcknowledgedByCategory = lifecycleDiagnosticDetail(d, "acknowledgedByCategory")
		}
		if len(d.Details) > 0 {
			jd.Details = d.Details
		}
		if len(d.Fixes) > 0 {
			jd.Fixes = d.Fixes
		}
		jsonDiagnostics = append(jsonDiagnostics, jd)
	}
	return CheckJSONReport{
		Command:      "check",
		OK:           summary.Errors == 0,
		Root:         opts.RootDir,
		ManifestPath: opts.ManifestPath,
		LockfilePath: opts.LockfilePath,
		Summary:      summary,
		Diagnostics:  jsonDiagnostics,
	}
}
func printCheckExplain(explain *check.ExplainResult) {
	fmt.Printf("Boundary explanation for %s\n\n", explain.File)
	fmt.Println("Reachable from targets:")
	if len(explain.ReachableFrom) == 0 {
		fmt.Println("  none")
	} else {
		for _, r := range explain.ReachableFrom {
			fmt.Printf("  %s\n", r.Target)
			fmt.Printf("    path: %s\n", strings.Join(r.Path, " -> "))
		}
	}
	fmt.Println()
	fmt.Println("Matched boundary rules:")
	if len(explain.MatchedRules) == 0 {
		fmt.Println("  none")
	} else {
		for _, rule := range explain.MatchedRules {
			if rule.TransitiveFrom != "" {
				fmt.Printf("  transitiveFrom: %s\n", rule.TransitiveFrom)
				fmt.Printf("    seed: %s\n", rule.Seed)
				if len(rule.Path) > 0 {
					fmt.Printf("    path: %s\n", strings.Join(rule.Path, " -> "))
				}
			} else {
				fmt.Printf("  from: %s\n", rule.From)
			}
			if len(rule.AllowDeps) > 0 {
				fmt.Printf("    allowDeps: %s\n", strings.Join(rule.AllowDeps, ", "))
			}
			if len(rule.DenyDeps) > 0 {
				fmt.Printf("    denyDeps: %s\n", strings.Join(rule.DenyDeps, ", "))
			}
			if len(rule.DenyTypeDeps) > 0 {
				fmt.Printf("    denyTypeDeps: %s\n", strings.Join(rule.DenyTypeDeps, ", "))
			}
			if rule.AllowOnly != nil {
				fmt.Printf("    allowOnly: %s\n", strings.Join(rule.AllowOnly, ", "))
			}
		}
	}
	fmt.Println()
	fmt.Println("External imports:")
	hasExternal := false
	for _, imp := range explain.Imports {
		if imp.Kind != "external" || imp.TypeOnly {
			continue
		}
		hasExternal = true
		fmt.Printf("  %s\n", imp.Specifier)
		fmt.Printf("    decision: %s\n", imp.Decision)
		for _, reason := range imp.Reasons {
			fmt.Printf("    reason: %s\n", reason)
		}
		if imp.Diagnostic != "" {
			fmt.Printf("    diagnostic: %s\n", imp.Diagnostic)
		}
	}
	if !hasExternal {
		fmt.Println("  none")
	}
	fmt.Println()
	fmt.Println("Type imports:")
	hasTypeExternal := false
	for _, imp := range explain.Imports {
		if imp.Kind != "external" || !imp.TypeOnly {
			continue
		}
		hasTypeExternal = true
		fmt.Printf("  %s\n", imp.Specifier)
		fmt.Printf("    decision: %s\n", imp.Decision)
		for _, reason := range imp.Reasons {
			fmt.Printf("    reason: %s\n", reason)
		}
		if imp.Diagnostic != "" {
			fmt.Printf("    diagnostic: %s\n", imp.Diagnostic)
		}
	}
	if !hasTypeExternal {
		fmt.Println("  none")
	}
	fmt.Println()
	fmt.Println("Relative imports:")
	hasRelative := false
	for _, imp := range explain.Imports {
		if imp.Kind != "relative" {
			continue
		}
		hasRelative = true
		fmt.Printf("  %s\n", imp.Specifier)
		if imp.Resolved != "" {
			fmt.Printf("    resolved: %s\n", imp.Resolved)
		} else {
			fmt.Println("    resolved: unresolved")
		}
	}
	if !hasRelative {
		fmt.Println("  none")
	}
	if len(explain.Notes) > 0 {
		fmt.Println()
		fmt.Println("Notes:")
		for _, note := range explain.Notes {
			fmt.Printf("  %s\n", note)
		}
	}
}
