package cli

import (
	"encoding/json"
	"fmt"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/npmobserve"
	"github.com/yuechen-li-dev/tspack/internal/project"
	"github.com/yuechen-li-dev/tspack/internal/why"
	"os"
	"strings"
)

func runWhyCommand(args []string) {
	paths := newLifecycleProjectPaths()
	jsonOutput := false
	positionals := []string{}
	options := project.WhyOptions{}
	for index := 1; index < len(args); index++ {
		if paths.consume(args, &index) {
			continue
		}
		switch args[index] {
		case "--json":
			jsonOutput = true
		case "--reverse":
			options.Reverse = true
		case "--package":
			options.PackageName = lifecycleFlagValue(args, &index, "--package")
		default:
			if strings.HasPrefix(args[index], "-") {
				failUnknownLifecycleFlag("why", args[index])
			}
			positionals = append(positionals, args[index])
		}
	}
	if options.Reverse && len(positionals) == 0 {
		fmt.Fprintln(os.Stderr, "TSPACK_WHY_QUERY_REQUIRED: reverse why requires exactly one query")
		os.Exit(1)
	}
	if options.Reverse && len(positionals) > 1 {
		fmt.Fprintln(os.Stderr, "TSPACK_WHY_INVALID_ARGS: reverse why requires exactly one query")
		os.Exit(1)
	}
	if len(positionals) > 0 {
		options.Query = positionals[0]
	}
	if shouldUseObservedNPMWhy(paths.Options, options) {
		observed, err := npmobserve.Explain(paths.Options.RootDir, options.Query)
		if err == nil {
			if jsonOutput {
				printObservedNPMWhyJSON(observed)
			} else {
				printObservedNPMWhy(observed)
			}
			return
		}
		result := project.Result{Diagnostics: []diag.Diagnostic{{Code: "TSPACK_OBSERVED_NPM_WHY_FAILED", Severity: diag.SeverityError, Message: err.Error()}}}
		renderWhyResult(paths.Options, options, result, jsonOutput)
		return
	}
	operation := project.RunWhy(project.WhyRequest{Project: paths.Options, Options: options})
	result := project.Result{Diagnostics: operation.Diagnostics, WhyResult: operation.Explanation}
	renderWhyResult(paths.Options, options, result, jsonOutput)
}

func renderWhyResult(projectOptions project.Options, whyOptions project.WhyOptions, result project.Result, jsonOutput bool) {
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(buildWhyJSONReport(projectOptions, whyOptions, result)); err != nil {
			fmt.Fprintf(os.Stderr, "TSPACK_WHY_JSON_ENCODE_FAILED: %v\n", err)
			os.Exit(1)
		}
		exitForDiagnostics(result.Diagnostics)
		return
	}
	renderHumanDiagnostics(os.Stderr, result.Diagnostics, checkRenderOptions{})
	if result.WhyResult != nil && whyOptions.Reverse {
		printReverseWhyResult(whyOptions, result.WhyResult)
	}
	if result.WhyResult != nil && !whyOptions.Reverse {
		renderWhyExplanation(result.WhyResult)
	}
	exitForDiagnostics(result.Diagnostics)
}

func renderWhyExplanation(result *why.Result) {
	for _, explanation := range result.Explanations {
		if explanation.MatchType == "dependency" {
			fmt.Printf("%s declared in package %q as %s\n", explanation.DependencyKey, explanation.PackageName, explanation.Kind)
		}
		if explanation.MatchType == "lock-package" {
			for _, lockPackage := range explanation.LockPackages {
				fmt.Printf("lock package %s\n", lockPackage.ID)
			}
		}
		if explanation.TargetName != "" {
			fmt.Printf("target %s in package %s\n", explanation.TargetName, explanation.PackageName)
		}
		if len(explanation.ReachableFrom) > 0 {
			fmt.Println("reachable from:")
			for _, reference := range explanation.ReachableFrom {
				fmt.Printf("  %s:target:%s\n", reference.PackageName, reference.TargetName)
			}
		}
		if len(explanation.NotReachableFrom) > 0 {
			fmt.Println("not reachable from:")
			for _, reference := range explanation.NotReachableFrom {
				fmt.Printf("  %s:target:%s\n", reference.PackageName, reference.TargetName)
			}
		}
		printWhyCapabilities(explanation.LockPackages)
		if len(explanation.LockEdges) > 0 {
			fmt.Println("lock edges:")
			for _, edge := range explanation.LockEdges {
				fmt.Printf("  %s -> %s %s\n", edge.From, edge.To, edge.Kind)
			}
		}
	}
}

func printWhyCapabilities(lockPackages []why.LockPackageRef) {
	printedHeader := false
	for _, lockPackage := range lockPackages {
		for _, capability := range lockPackage.Capabilities {
			if !printedHeader {
				fmt.Println("capabilities:")
				printedHeader = true
			}
			fmt.Printf("  %s %s: %s\n", capability.Kind, capability.Script, capability.Command)
			fmt.Printf("    lifecycleCategory: %s\n", capability.LifecycleCategory)
			fmt.Printf("    consumerInstallTime: %t\n", capability.ConsumerInstallTime)
			fmt.Println("    execution: blocked by default")
			if capability.Acknowledged {
				fmt.Println("    acknowledged: true")
				if capability.AcknowledgementReason != "" {
					fmt.Printf("    reason: %s\n", capability.AcknowledgementReason)
				}
				if capability.BehaviorFixture != "" {
					fmt.Printf("    behaviorFixture: %s (%s)\n", capability.BehaviorFixture, capability.BehaviorFixtureStatus)
				}
				if capability.BehaviorReport != "" {
					fmt.Printf("    behaviorReport: %s (%s)\n", capability.BehaviorReport, capability.BehaviorReportStatus)
				}
			} else {
				fmt.Println("    acknowledged: false")
			}
		}
	}
}

func printReverseWhyResult(whyOpts project.WhyOptions, result *why.Result) {
	fmt.Printf("Reverse why: %s\n", whyOpts.Query)
	fmt.Println()

	if len(result.LockPackages) > 1 {
		fmt.Println("Matching lock packages:")
		for _, lockPackage := range result.LockPackages {
			fmt.Printf("  %s\n", lockPackage.ID)
		}
		fmt.Println()
	}

	pathsByLockPackage := map[string][]why.ReversePath{}
	for _, path := range result.ReversePaths {
		pathsByLockPackage[path.LockPackage] = append(pathsByLockPackage[path.LockPackage], path)
	}

	for _, lockPackage := range result.LockPackages {
		paths := pathsByLockPackage[lockPackage.ID]
		if len(paths) == 0 {
			if whyOpts.PackageName != "" {
				fmt.Printf("No reverse paths from package %s.\n", whyOpts.PackageName)
			} else {
				fmt.Printf("No reverse paths found for %s.\n", lockPackage.ID)
			}
			fmt.Println()
			continue
		}

		fmt.Printf("%s is pulled in by:\n", lockPackage.ID)
		printWhyCapabilities([]why.LockPackageRef{lockPackage})
		fmt.Println()
		for _, path := range paths {
			fmt.Printf("  %s\n", path.Root)
			fmt.Println("    path:")
			for index, node := range path.Path {
				if index == 0 {
					fmt.Printf("      %s\n", node)
				} else {
					fmt.Printf("      -> %s\n", node)
				}
			}
			fmt.Println()
		}
	}
}
func shouldUseObservedNPMWhy(opts project.Options, whyOpts project.WhyOptions) bool {
	if whyOpts.Reverse || whyOpts.PackageName != "" || strings.TrimSpace(whyOpts.Query) == "" {
		return false
	}
	if !npmobserve.HasPackageJSON(opts.RootDir) {
		return false
	}
	if _, err := os.Stat(opts.LockfilePath); err == nil {
		return false
	}
	return true
}

func printObservedNPMWhyJSON(result npmobserve.ExplainResult) {
	report := map[string]any{
		"query":               result.Query,
		"sourceKind":          "observed-npm",
		"source":              npmobserve.SourceLabel,
		"found":               len(result.Direct) > 0 || len(result.Matches) > 0,
		"direct":              len(result.Direct) > 0,
		"packageJsonSections": observedJSONSections(result.Direct),
		"requestedRange":      observedJSONRequestedRange(result.Direct),
		"matches":             result.Matches,
		"chains":              result.Chains,
		"notes":               result.Notes,
		"lockfilePresent":     result.LockfilePresent,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "TSPACK_OBSERVED_NPM_WHY_JSON_ENCODE_FAILED: %v\n", err)
		os.Exit(1)
	}
}

func observedJSONSections(matches []npmobserve.DirectMatch) []string {
	sections := []string{}
	for _, match := range matches {
		sections = append(sections, match.Section)
	}
	return sections
}

func observedJSONRequestedRange(matches []npmobserve.DirectMatch) string {
	if len(matches) == 0 {
		return ""
	}
	return matches[0].Range
}

func printObservedNPMWhy(result npmobserve.ExplainResult) {
	fmt.Println("source: " + npmobserve.SourceLabel)
	fmt.Println()
	for _, lockfile := range result.UnsupportedLockfiles {
		fmt.Printf("note: %s is present; observed why currently supports npm package-lock only.\n", lockfile)
	}
	if len(result.UnsupportedLockfiles) > 0 {
		fmt.Println()
	}

	if len(result.Direct) > 0 || len(result.Matches) > 0 {
		if len(result.Direct) > 0 {
			fmt.Printf("%s is present in the observed npm project.\n", result.Query)
		} else {
			fmt.Printf("%s is present in the observed npm lockfile.\n", result.Query)
		}
		fmt.Println()
		printObservedSources(result)
		printObservedVersions(result)
		printObservedReasons(result)
		printObservedChains(result)
		printObservedAdoptionNote()
		return
	}

	if result.LockfilePresent {
		fmt.Printf("%s was not found in package.json or package-lock.json.\n", result.Query)
		printObservedAdoptionNote()
		return
	}

	fmt.Printf("%s was not found in package.json.\n", result.Query)
	fmt.Println("No package-lock.json is available, so TSPack cannot explain transitive npm packages yet.")
	fmt.Println("To create npm's lockfile, run:")
	fmt.Println("tspack npm install")
	printObservedAdoptionNote()
}

func printObservedSources(result npmobserve.ExplainResult) {
	fmt.Println("Source:")
	if len(result.Direct) > 0 {
		for _, direct := range result.Direct {
			fmt.Println(direct.Section)
		}
	}
	if result.LockfilePresent {
		fmt.Println("package-lock.json")
	}
	fmt.Println()
}

func printObservedVersions(result npmobserve.ExplainResult) {
	if len(result.Matches) == 0 {
		return
	}
	fmt.Println("Version:")
	for _, match := range result.Matches {
		if match.Version == "" {
			fmt.Printf("%s at %s\n", match.Name, match.Location)
		} else {
			fmt.Printf("%s %s at %s\n", match.Name, match.Version, match.Location)
		}
	}
	fmt.Println()
}

func printObservedReasons(result npmobserve.ExplainResult) {
	if len(result.Direct) == 0 {
		return
	}
	fmt.Println("Reason:")
	for _, direct := range result.Direct {
		section := strings.TrimPrefix(direct.Section, "package.json ")
		fmt.Printf("root package declares %s in %s as %s\n", result.Query, section, direct.Range)
	}
	fmt.Println()
}

func printObservedChains(result npmobserve.ExplainResult) {
	if len(result.Chains) == 0 {
		return
	}
	fmt.Println("Chain:")
	for chainIndex, chain := range result.Chains {
		if chainIndex > 0 {
			fmt.Println()
		}
		for nodeIndex, node := range chain.Nodes {
			prefix := ""
			if nodeIndex > 0 {
				prefix = strings.Repeat("  ", nodeIndex-1) + "└─ "
			}
			if node.Version == "" {
				fmt.Printf("%s%s\n", prefix, node.Name)
			} else {
				fmt.Printf("%s%s %s\n", prefix, node.Name, node.Version)
			}
		}
	}
	fmt.Println()
}

func printObservedAdoptionNote() {
	fmt.Println("Adoption note:")
	fmt.Println("This explanation is from observed npm metadata. It is not a TSPack manifest dependency classification yet.")
}

func buildWhyJSONReport(opts project.Options, whyOpts project.WhyOptions, result project.Result) WhyJSONReport {
	packageFilter := (*string)(nil)
	if whyOpts.PackageName != "" {
		name := whyOpts.PackageName
		packageFilter = &name
	}

	explanations := []WhyJSONExplanation{}
	lockPackages := []WhyJSONLockPackage{}
	reversePaths := []WhyJSONReversePath{}
	notes := []string{}
	if result.WhyResult != nil {
		for _, explanation := range result.WhyResult.Explanations {
			explanations = append(explanations, buildWhyJSONExplanation(explanation))
		}
		for _, lockPackage := range result.WhyResult.LockPackages {
			lockPackages = append(lockPackages, buildWhyJSONLockPackage(lockPackage))
		}
		for _, reversePath := range result.WhyResult.ReversePaths {
			reversePaths = append(reversePaths, buildWhyJSONReversePath(reversePath))
		}
		notes = append(notes, result.WhyResult.Notes...)
	}

	diagnostics := buildWhyJSONDiagnostics(result.Diagnostics)
	summary := WhyJSONSummary{
		Explanations: len(explanations),
		LockPackages: len(lockPackages),
		ReversePaths: len(reversePaths),
		Diagnostics:  len(diagnostics),
	}
	for _, diagnostic := range diagnostics {
		switch diagnostic.Severity {
		case string(diag.SeverityError):
			summary.Errors++
		case string(diag.SeverityWarning):
			summary.Warnings++
		}
	}

	mode := ""
	if whyOpts.Reverse {
		mode = "reverse"
	}

	return WhyJSONReport{
		Command:      "why",
		Mode:         mode,
		Query:        whyOpts.Query,
		Package:      packageFilter,
		OK:           summary.Errors == 0,
		Root:         opts.RootDir,
		ManifestPath: opts.ManifestPath,
		LockfilePath: opts.LockfilePath,
		Summary:      summary,
		Explanations: explanations,
		LockPackages: lockPackages,
		Reverse:      reversePaths,
		Notes:        notes,
		Diagnostics:  diagnostics,
	}
}

func buildWhyJSONLockPackage(lockPackage why.LockPackageRef) WhyJSONLockPackage {
	jsonPackage := WhyJSONLockPackage{
		ID:      lockPackage.ID,
		Name:    lockPackage.Name,
		Version: lockPackage.Version,
		Source:  lockPackage.Source,
		Hash:    lockPackage.Hash,
	}
	for _, capability := range lockPackage.Capabilities {
		jsonPackage.Capabilities = append(jsonPackage.Capabilities, WhyJSONCapability{
			Kind:                  capability.Kind,
			Script:                capability.Script,
			Command:               capability.Command,
			Execution:             capability.Execution,
			LifecycleCategory:     capability.LifecycleCategory,
			ConsumerInstallTime:   capability.ConsumerInstallTime,
			Acknowledged:          capability.Acknowledged,
			AcknowledgementReason: capability.AcknowledgementReason,
			BehaviorFixture:       capability.BehaviorFixture,
			BehaviorFixtureStatus: capability.BehaviorFixtureStatus,
			BehaviorReport:        capability.BehaviorReport,
			BehaviorReportStatus:  capability.BehaviorReportStatus,
		})
	}
	return jsonPackage
}

func buildWhyJSONReversePath(reversePath why.ReversePath) WhyJSONReversePath {
	jsonPath := WhyJSONReversePath{
		LockPackage: reversePath.LockPackage,
		Root:        reversePath.Root,
		Path:        append([]string(nil), reversePath.Path...),
	}
	for _, edge := range reversePath.Edges {
		jsonPath.Edges = append(jsonPath.Edges, WhyJSONLockEdge{
			From:     edge.From,
			To:       edge.To,
			Kind:     edge.Kind,
			Optional: edge.Optional,
		})
	}
	return jsonPath
}

func buildWhyJSONExplanation(explanation why.Explanation) WhyJSONExplanation {
	jsonExplanation := WhyJSONExplanation{
		Kind:                explanation.MatchType,
		PackageName:         explanation.PackageName,
		DependencyKey:       explanation.DependencyKey,
		DependencyKind:      explanation.Kind,
		ExternalPackageName: explanation.ExternalPackageName,
		TargetName:          explanation.TargetName,
		Optional:            explanation.Optional,
		DirectProject:       explanation.DirectProject,
	}
	if jsonExplanation.Kind == "" {
		jsonExplanation.Kind = explanation.Kind
	}

	for _, declaration := range explanation.DeclaredBy {
		jsonDeclaration := WhyJSONDeclaration{
			PackageName:   declaration.PackageName,
			Scope:         declaration.Scope,
			TargetName:    declaration.TargetName,
			DependencyKey: declaration.DependencyKey,
			Kind:          declaration.Kind,
			Optional:      declaration.Optional,
		}
		if declaration.SourceKind != "" || declaration.SourcePackage != "" || declaration.SourceRange != "" {
			jsonDeclaration.Source = &WhyJSONSource{
				Kind:    declaration.SourceKind,
				Package: declaration.SourcePackage,
				Range:   declaration.SourceRange,
			}
		}
		jsonExplanation.DeclaredBy = append(jsonExplanation.DeclaredBy, jsonDeclaration)
	}

	jsonExplanation.Source = primaryWhyJSONSource(jsonExplanation.DeclaredBy)
	for _, reachable := range explanation.ReachableFrom {
		jsonExplanation.ReachableFrom = append(jsonExplanation.ReachableFrom, buildWhyJSONReachability(reachable))
	}
	for _, unreachable := range explanation.NotReachableFrom {
		jsonExplanation.NotReachableFrom = append(jsonExplanation.NotReachableFrom, buildWhyJSONReachability(unreachable))
	}
	for _, lockPackage := range explanation.LockPackages {
		jsonExplanation.LockPackages = append(jsonExplanation.LockPackages, buildWhyJSONLockPackage(lockPackage))
	}
	for _, edge := range explanation.LockEdges {
		jsonExplanation.LockEdges = append(jsonExplanation.LockEdges, WhyJSONLockEdge{
			From:     edge.From,
			To:       edge.To,
			Kind:     edge.Kind,
			Optional: edge.Optional,
		})
	}

	return jsonExplanation
}

func primaryWhyJSONSource(declarations []WhyJSONDeclaration) *WhyJSONSource {
	for _, declaration := range declarations {
		if declaration.Source != nil {
			return declaration.Source
		}
	}
	return nil
}

func buildWhyJSONReachability(ref why.ReachabilityRef) WhyJSONReachability {
	return WhyJSONReachability{
		PackageName: ref.PackageName,
		TargetName:  ref.TargetName,
		Reason:      ref.Reason,
		Ref:         ref.PackageName + ":target:" + ref.TargetName,
	}
}

func buildWhyJSONDiagnostics(diags []diag.Diagnostic) []WhyJSONDiagnostic {
	sorted := append([]diag.Diagnostic(nil), diags...)
	diag.SortDiagnostics(sorted)
	jsonDiagnostics := []WhyJSONDiagnostic{}
	for _, diagnostic := range sorted {
		jsonDiagnostic := WhyJSONDiagnostic{
			Code:     diagnostic.Code,
			Severity: string(diagnostic.Severity),
			Message:  diagnostic.Message,
			File:     diagnostic.File,
			Details:  append([]string(nil), diagnostic.Details...),
			Fixes:    append([]string(nil), diagnostic.Fixes...),
		}
		if jsonDiagnostic.Details == nil {
			jsonDiagnostic.Details = []string{}
		}
		jsonDiagnostics = append(jsonDiagnostics, jsonDiagnostic)
	}
	return jsonDiagnostics
}
