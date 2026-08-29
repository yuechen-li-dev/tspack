package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/project"
	"github.com/yuechen-li-dev/tspack/internal/workflow"
	"gopkg.in/yaml.v3"
)

type workflowCommandOptions struct {
	Root        string
	Identity    string
	JSON        bool
	Concurrency int
	Provider    string
	Output      string
	Check       bool
}

func runWorkflowCommand(args []string) {
	if len(args) < 2 {
		failWorkflowArgs("expected list, inspect, run, or export")
	}
	subcommand := args[1]
	options := parseWorkflowOptions(subcommand, args[2:])
	workspace := openWorkspace(options.Root)
	manifestPath := filepath.Join(workspace.Root, "manifest.tsx")
	ir := workspace.LoadManifest(manifestPath)

	switch subcommand {
	case "list":
		runWorkflowList(ir.Workflows, options.JSON)
	case "inspect":
		declaration := findWorkflowOrFail(ir, options.Identity)
		flow, err := workflow.BuildFlow(*declaration)
		if err != nil {
			failWorkflow("TSPACK_WORKFLOW_FLOW_INVALID", err.Error())
		}
		renderWorkflowFlow(flow, options.JSON)
	case "run":
		if options.Provider != "" && options.Provider != "github" {
			failWorkflow("TSPACK_WORKFLOW_PROVIDER_UNSUPPORTED", "local runner does not recognize provider "+options.Provider)
		}
		declaration := findWorkflowOrFail(ir, options.Identity)
		flow, err := workflow.BuildFlow(*declaration)
		if err != nil {
			failWorkflow("TSPACK_WORKFLOW_FLOW_INVALID", err.Error())
		}
		projectOptions := project.DefaultOptions(workspace.Root)
		projectOptions.ManifestPath = manifestPath
		projectOptions.FrontendCLIPath = manifestFrontendCLIPath()
		buildOutput := io.Writer(os.Stdout)
		if options.JSON {
			buildOutput = io.Discard
		}
		executor := workflow.Executor{
			Root:     workspace.Root,
			Manifest: ir,
			Native: workflow.ProjectOperations{
				Options:       projectOptions,
				BuildExecutor: cliBuildTargetExecutor{Output: buildOutput},
				Quiet:         options.JSON,
			},
			Context: workflow.ExecutionContext{
				IsCI:     options.Provider != "",
				Provider: options.Provider,
			},
			Concurrency: options.Concurrency,
			Events:      workflowEventRenderer(options.JSON),
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		result := executor.RunFlow(ctx, flow)
		if options.JSON {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"type": "result", "result": result})
		}
		if result.State != workflow.StateSucceeded {
			exit(1)
		}
	case "export":
		if options.Provider != "github" {
			failWorkflow("TSPACK_WORKFLOW_PROVIDER_UNSUPPORTED", "export currently supports only github")
		}
		declaration := findWorkflowOrFail(ir, options.Identity)
		flow, err := workflow.BuildFlow(*declaration)
		if err != nil {
			failWorkflow("TSPACK_WORKFLOW_FLOW_INVALID", err.Error())
		}
		exportWorkflowGitHub(workspace.Root, flow, options)
	default:
		failWorkflowArgs("unknown workflow subcommand: " + subcommand)
	}
}

func parseWorkflowOptions(subcommand string, args []string) workflowCommandOptions {
	options := workflowCommandOptions{Root: ".", Concurrency: 4}
	if subcommand == "export" && len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		options.Provider = args[0]
		args = args[1:]
	}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--root":
			options.Root = workflowFlagValue(args, &index, "--root")
		case "--json":
			options.JSON = true
		case "--jobs":
			value := workflowFlagValue(args, &index, "--jobs")
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed <= 0 || parsed > workflow.MaxForEachConcurrency {
				failWorkflowArgs(fmt.Sprintf("--jobs must be between 1 and %d", workflow.MaxForEachConcurrency))
			}
			options.Concurrency = parsed
		case "--ci-provider":
			options.Provider = workflowFlagValue(args, &index, "--ci-provider")
		case "--out":
			options.Output = workflowFlagValue(args, &index, "--out")
		case "--check":
			options.Check = true
		default:
			if strings.HasPrefix(args[index], "-") || options.Identity != "" {
				failWorkflowArgs("unexpected workflow argument: " + args[index])
			}
			options.Identity = args[index]
		}
	}
	if subcommand != "list" && options.Identity == "" {
		failWorkflowArgs(subcommand + " requires a workflow identity")
	}
	return options
}

func workflowFlagValue(args []string, index *int, flag string) string {
	*index++
	if *index >= len(args) {
		failWorkflowArgs(flag + " requires a value")
	}
	return args[*index]
}

func runWorkflowList(declarations []manifest.Workflow, jsonOutput bool) {
	if jsonOutput {
		identities := make([]string, 0, len(declarations))
		for _, declaration := range declarations {
			identities = append(identities, declaration.Identity)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"workflows": identities})
		return
	}
	if len(declarations) == 0 {
		fmt.Println("No workflows declared.")
		return
	}
	for _, declaration := range declarations {
		fmt.Println(declaration.Identity)
	}
}

func findWorkflowOrFail(ir *manifest.ManifestIR, identity string) *manifest.Workflow {
	declaration, err := workflow.Find(ir, identity)
	if err != nil {
		failWorkflow("TSPACK_WORKFLOW_NOT_FOUND", err.Error())
	}
	return declaration
}

func renderWorkflowFlow(flow workflow.Flow, jsonOutput bool) {
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(flow)
		return
	}
	fmt.Printf("Workflow %s\n\n", flow.Identity)
	fmt.Println("Triggers:")
	for _, trigger := range flow.Triggers {
		filters := []string{}
		if len(trigger.Branches) > 0 {
			filters = append(filters, "branches="+strings.Join(trigger.Branches, ","))
		}
		if len(trigger.Paths) > 0 {
			filters = append(filters, "paths="+strings.Join(trigger.Paths, ","))
		}
		if len(filters) > 0 {
			fmt.Printf("  %s (%s)\n", trigger.Kind, strings.Join(filters, "; "))
		} else {
			fmt.Printf("  %s\n", trigger.Kind)
		}
	}
	fmt.Printf("\nFlow (schema %d):\n", flow.SchemaVersion)
	fmt.Printf("Expansion: %d/%d planned iterations; global concurrency ceiling %d\n", flow.Expansion.PlannedIterations, flow.Expansion.Limit, flow.Expansion.MaxConcurrency)
	renderFlowProgram(flow, flow.Entry, "  ", map[string]bool{})
	if len(flow.Values) > 0 {
		fmt.Println("\nValues:")
		for _, value := range flow.Values {
			if len(value.FieldPath) > 0 {
				fmt.Printf("  %s = %s.%s [%s]\n", value.Identity, value.Source, strings.Join(value.FieldPath, "."), value.Category)
			} else {
				fmt.Printf("  %s <- %s [%s]\n", value.Identity, value.Producer, value.Category)
			}
		}
	}
	if len(flow.Aggregates) > 0 {
		fmt.Println("\nAggregates:")
		for _, aggregate := range flow.Aggregates {
			completeness := "partial"
			if aggregate.Complete {
				completeness = "complete"
			}
			fmt.Printf("  %s: %s[%d] (%s, concurrency %d, %s, %s)\n", aggregate.Identity, aggregate.ResultType, len(aggregate.Elements), aggregate.Mode, aggregate.Concurrency, aggregate.FailurePolicy, completeness)
			for index, element := range aggregate.Elements {
				fmt.Printf("    [%d] -> %s\n", index, element)
			}
		}
	}
	fmt.Println("\nTransitions:")
	for _, transition := range flow.Transitions {
		guard := ""
		if transition.Guard != "" && transition.Guard != string(transition.Event) {
			guard = " [" + transition.Guard + "]"
		}
		fmt.Printf("  %s --%s%s--> %s\n", transition.From, transition.Event, guard, transition.To)
	}
}

func renderFlowProgram(flow workflow.Flow, identity string, indent string, visited map[string]bool) {
	if visited[identity] {
		fmt.Printf("%s%s (already shown)\n", indent, identity)
		return
	}
	visited[identity] = true
	node, exists := findFlowNode(flow, identity)
	if !exists {
		fmt.Printf("%sunknown %s\n", indent, identity)
		return
	}
	switch node.Kind {
	case workflow.NodeEntry:
		fmt.Printf("%sentry\n", indent)
		if next := flowTransitionTarget(flow, identity, workflow.MachineContinue); next != "" {
			renderFlowProgram(flow, next, indent+"  ", visited)
		}
	case workflow.NodeEffect:
		cleanup := ""
		if node.Cleanup {
			cleanup = " cleanup(" + string(node.CleanupCause) + ")"
		}
		fmt.Printf("%s%s (%s) @ %s => %s%s\n", indent, node.Effect.Name, node.Effect.Operation, node.Region, node.Value, cleanup)
		if selection := renderWorkflowEffectSelection(*node.Effect); selection != "" {
			fmt.Printf("%s  target: %s\n", indent, selection)
		}
		for _, input := range node.Effect.Inputs {
			fmt.Printf("%s  consumes: %s\n", indent, input.Identity)
		}
		if node.Effect.Operation == "process" {
			fmt.Printf("%s  argv: %s\n", indent, strings.Join(node.Effect.Command, " "))
		}
		if node.Effect.Operation == "transfer" {
			fmt.Printf("%s  transport target: %s\n", indent, node.Effect.TransferTarget)
		}
		if next := flowTransitionTarget(flow, identity, workflow.MachineEffectSucceeded); next != "" {
			renderFlowProgram(flow, next, indent, visited)
		}
	case workflow.NodeFork:
		fmt.Printf("%sfork\n", indent)
		join := ""
		for _, target := range node.Targets {
			targetNode, _ := findFlowNode(flow, target)
			if targetNode.Kind == workflow.NodeJoin {
				join = target
				continue
			}
			branch := targetNode.Branch
			if branch == "" {
				branch = target
			}
			fmt.Printf("%s  branch %s:\n", indent, branch)
			renderFlowProgram(flow, target, indent+"    ", visited)
		}
		if join != "" {
			fmt.Printf("%sjoin all\n", indent)
			visited[join] = true
			if next := flowTransitionTarget(flow, join, workflow.MachineContinue); next != "" {
				renderFlowProgram(flow, next, indent, visited)
			}
		}
	case workflow.NodeBranchExit:
		fmt.Printf("%sbranch complete\n", indent)
		if next := flowTransitionTarget(flow, identity, workflow.MachineContinue); next != "" {
			fmt.Printf("%s  admits next source item:\n", indent)
			renderFlowProgram(flow, next, indent+"    ", visited)
		}
	case workflow.NodeJoin:
		fmt.Printf("%sjoin all\n", indent)
		if next := flowTransitionTarget(flow, identity, workflow.MachineContinue); next != "" {
			renderFlowProgram(flow, next, indent, visited)
		}
	case workflow.NodeMatch:
		if node.Projection != nil {
			fmt.Printf("%smatch %s[%d] -> %s:\n", indent, node.Projection.Aggregate, node.Projection.Index, node.Source)
		} else {
			fmt.Printf("%smatch %s:\n", indent, node.Source)
		}
		for _, transition := range flow.Transitions {
			if transition.From == identity && transition.Event == workflow.MachineContinue {
				fmt.Printf("%s  %s:\n", indent, transition.Guard)
				renderFlowProgram(flow, transition.To, indent+"    ", visited)
			}
		}
	case workflow.NodeIterator:
		if node.Iterator == nil {
			fmt.Printf("%siterator (invalid)\n", indent)
			break
		}
		if node.Iterator.SourceAggregate != "" {
			fmt.Printf("%sforeach %s cursor %d/%d consumes %s -> %s (path %s; %s, concurrency %d, %s)\n", indent, node.Iterator.SourceIdentity, node.Iterator.Index+1, node.Iterator.Count, node.Iterator.SourceAggregate, node.Iterator.ElementIdentity, node.Iterator.Path, node.Iterator.Mode, node.Iterator.Concurrency, node.Iterator.FailurePolicy)
		} else {
			fmt.Printf("%sforeach %s cursor %d/%d = %s (path %s; %s, concurrency %d, %s)\n", indent, node.Iterator.SourceIdentity, node.Iterator.Index+1, node.Iterator.Count, renderIterationValue(node.Iterator.Value), node.Iterator.Path, node.Iterator.Mode, node.Iterator.Concurrency, node.Iterator.FailurePolicy)
		}
		if next := flowTransitionTarget(flow, identity, workflow.MachineContinue); next != "" {
			renderFlowProgram(flow, next, indent+"  ", visited)
		}
	case workflow.NodePredicate:
		fmt.Printf("%swhen %s:\n", indent, renderWorkflowPredicate(node.Predicate))
		for _, transition := range flow.Transitions {
			if transition.From == identity && transition.Event == workflow.MachineContinue {
				fmt.Printf("%s  %s:\n", indent, transition.Guard)
				renderFlowProgram(flow, transition.To, indent+"    ", visited)
			}
		}
	case workflow.NodeTerminal:
		fmt.Printf("%sexit %s\n", indent, node.Terminal)
	default:
		fmt.Printf("%s%s [%s]\n", indent, identity, node.Kind)
	}
}

func renderWorkflowEffectSelection(step workflow.PlanStep) string {
	if step.Operation == "test" && step.Target != "" && len(step.Packages) == 1 {
		return step.Packages[0] + ":" + step.Target
	}
	if step.Operation != "build" || len(step.Packages) == 0 || len(step.Targets) == 0 {
		return ""
	}
	selections := make([]string, 0, len(step.Packages)*len(step.Targets))
	for _, packageName := range step.Packages {
		for _, target := range step.Targets {
			selections = append(selections, packageName+":"+target)
		}
	}
	return strings.Join(selections, ", ")
}

func renderWorkflowPredicate(predicate *workflow.Predicate) string {
	if predicate == nil {
		return "<invalid>"
	}
	switch predicate.Kind {
	case "greaterThan", "lessThan":
		operator := ">"
		if predicate.Kind == "lessThan" {
			operator = "<"
		}
		if predicate.Input != nil && predicate.Number != nil {
			return fmt.Sprintf("%s %s %v", predicate.Input.Identity, operator, *predicate.Number)
		}
	case "notEmpty", "isEmpty":
		if predicate.Input != nil {
			return predicate.Kind + "(" + predicate.Input.Identity + ")"
		}
	case "and", "or":
		parts := make([]string, 0, len(predicate.Children))
		for index := range predicate.Children {
			parts = append(parts, renderWorkflowPredicate(&predicate.Children[index]))
		}
		return "(" + strings.Join(parts, " "+predicate.Kind+" ") + ")"
	case "not":
		if len(predicate.Children) == 1 {
			return "not(" + renderWorkflowPredicate(&predicate.Children[0]) + ")"
		}
	}
	return "<invalid>"
}

func renderIterationValue(value workflow.IterationValue) string {
	switch value.Kind {
	case "string", "platform":
		return value.String
	case "number":
		if value.Number != nil {
			return fmt.Sprint(*value.Number)
		}
	case "boolean":
		if value.Boolean != nil {
			return fmt.Sprint(*value.Boolean)
		}
	}
	return "<invalid>"
}

func findFlowNode(flow workflow.Flow, identity string) (workflow.FlowNode, bool) {
	for _, node := range flow.Nodes {
		if node.Identity == identity {
			return node, true
		}
	}
	return workflow.FlowNode{}, false
}

func flowTransitionTarget(flow workflow.Flow, from string, event workflow.MachineEventKind) string {
	for _, transition := range flow.Transitions {
		if transition.From == from && transition.Event == event {
			return transition.To
		}
	}
	return ""
}

func workflowEventRenderer(jsonOutput bool) workflow.EventSink {
	var mutex sync.Mutex
	return func(event workflow.Event) {
		mutex.Lock()
		defer mutex.Unlock()
		if jsonOutput {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"type": "event", "event": event})
			return
		}
		switch event.Kind {
		case workflow.EventWorkflowStarted:
			fmt.Printf("Workflow %s started\n", event.Workflow)
		case workflow.EventJobStarted:
			fmt.Printf("[%s] started\n", event.Job)
		case workflow.EventStepStarted:
			fmt.Printf("[%s] %s started\n", event.Job, event.Step)
		case workflow.EventStepOutput:
			fmt.Printf("[%s/%s] %s\n", event.Job, event.Stream, event.Output)
		case workflow.EventStepDiagnostic:
			fmt.Printf("[%s] %s: %s\n", event.Job, event.Code, event.Message)
			for _, detail := range event.Details {
				fmt.Printf("[%s]   %s\n", event.Job, detail)
			}
		case workflow.EventStepCompleted:
			fmt.Printf("[%s] %s %s (%dms)\n", event.Job, event.Step, event.State, event.Duration)
			if event.Result != nil && event.Result.Test != nil {
				fmt.Printf("[%s]   %d passed, %d failed, %d skipped\n", event.Job, event.Result.Test.Passed, event.Result.Test.Failed, event.Result.Test.Skipped)
			}
			if event.Result != nil && event.Result.Build != nil {
				for _, artifact := range event.Result.Build.Artifacts {
					fmt.Printf("[%s]   %s:%s -> %s\n", event.Job, artifact.Package, artifact.Target, artifact.Path)
				}
			}
			if event.Result != nil && event.Result.Audit != nil {
				fmt.Printf("[%s]   %d blocking findings; coverage complete: %t\n", event.Job, event.Result.Audit.Failing, event.Result.Audit.Report.CoverageComplete)
			}
		case workflow.EventArtifactTransferStarted:
			fmt.Printf("[%s] artifact transfer started\n", event.Job)
		case workflow.EventArtifactTransferCompleted:
			fmt.Printf("[%s] artifact transfer completed (%dms)\n", event.Job, event.Duration)
		case workflow.EventArtifactTransferFailed:
			fmt.Printf("[%s] artifact transfer failed\n", event.Job)
		case workflow.EventJobCompleted:
			fmt.Printf("[%s] %s (%dms)\n", event.Job, event.State, event.Duration)
		case workflow.EventWorkflowCompleted:
			fmt.Printf("Workflow %s %s\n", event.Workflow, event.State)
		}
	}
}

func exportWorkflowGitHub(root string, flow workflow.Flow, options workflowCommandOptions) {
	contents, err := workflow.ExportGitHub(flow)
	if err != nil {
		failWorkflow("TSPACK_WORKFLOW_PROVIDER_UNSUPPORTED", err.Error())
	}
	var parsed yaml.Node
	if err := yaml.Unmarshal(contents, &parsed); err != nil {
		failWorkflow("TSPACK_WORKFLOW_PROVIDER_OUTPUT_INVALID", err.Error())
	}
	relativePath := options.Output
	if relativePath == "" {
		relativePath = workflow.GitHubPath(flow.Identity)
	}
	if filepath.IsAbs(relativePath) || strings.HasPrefix(filepath.Clean(relativePath), "..") {
		failWorkflow("TSPACK_WORKFLOW_OUTPUT_PATH_INVALID", "output path must stay within the workspace")
	}
	outputPath := filepath.Join(root, relativePath)
	if options.Check {
		existing, readErr := os.ReadFile(outputPath)
		if readErr != nil || string(existing) != string(contents) {
			failWorkflow("TSPACK_WORKFLOW_PROVIDER_DRIFT", relativePath+" is missing or stale")
		}
		fmt.Printf("Workflow provider artifact is current: %s\n", relativePath)
		return
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		failWorkflow("TSPACK_WORKFLOW_PROVIDER_WRITE_FAILED", err.Error())
	}
	if existing, readErr := os.ReadFile(outputPath); readErr == nil && string(existing) == string(contents) {
		fmt.Printf("Workflow provider artifact unchanged: %s\n", relativePath)
		return
	}
	if err := os.WriteFile(outputPath, contents, 0o644); err != nil {
		failWorkflow("TSPACK_WORKFLOW_PROVIDER_WRITE_FAILED", err.Error())
	}
	fmt.Printf("Wrote workflow provider artifact: %s\n", relativePath)
}

func failWorkflowArgs(message string) {
	fmt.Fprintln(os.Stderr, "TSPACK_WORKFLOW_INVALID_ARGS:", message)
	fmt.Fprintln(os.Stderr, "usage: tspack workflow list|inspect <name>|run <name>|export github <name> [--root path] [--json]")
	exit(2)
}

func failWorkflow(code string, message string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", code, message)
	exit(1)
}
