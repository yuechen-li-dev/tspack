package cli

import (
	"context"
	"encoding/json"
	"fmt"
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
		plan := workflow.BuildPlan(*declaration)
		renderWorkflowPlan(plan, options.JSON)
	case "run":
		if options.Provider != "" && options.Provider != "github" {
			failWorkflow("TSPACK_WORKFLOW_PROVIDER_UNSUPPORTED", "local runner does not recognize provider "+options.Provider)
		}
		declaration := findWorkflowOrFail(ir, options.Identity)
		plan := workflow.BuildPlan(*declaration)
		projectOptions := project.DefaultOptions(workspace.Root)
		projectOptions.ManifestPath = manifestPath
		projectOptions.FrontendCLIPath = manifestFrontendCLIPath()
		executor := workflow.Executor{
			Root:     workspace.Root,
			Manifest: ir,
			Native:   workflow.ProjectOperations{Options: projectOptions},
			Context: workflow.ExecutionContext{
				IsCI:     options.Provider != "",
				Provider: options.Provider,
			},
			Concurrency: options.Concurrency,
			Events:      workflowEventRenderer(options.JSON),
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		result := executor.Run(ctx, plan)
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
		plan := workflow.BuildPlan(*declaration)
		exportWorkflowGitHub(workspace.Root, plan, options)
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
			if err != nil || parsed <= 0 || parsed > 64 {
				failWorkflowArgs("--jobs must be between 1 and 64")
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

func renderWorkflowPlan(plan workflow.Plan, jsonOutput bool) {
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(plan)
		return
	}
	fmt.Printf("Workflow %s\n\n", plan.Workflow)
	fmt.Println("Triggers:")
	for _, trigger := range plan.Triggers {
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
	fmt.Println("\nJobs:")
	for _, job := range plan.Jobs {
		fmt.Printf("  %s [%s]\n", job.Identity, job.Platform)
		if len(job.Needs) > 0 {
			fmt.Printf("    needs: %s\n", strings.Join(job.Needs, ", "))
		}
		for _, environment := range job.Environment {
			if environment.Kind == "secret" {
				fmt.Printf("    env: %s <- secret(%s)\n", environment.Name, environment.Secret)
			} else {
				fmt.Printf("    env: %s <- plain\n", environment.Name)
			}
		}
		for _, step := range job.Steps {
			fmt.Printf("    %s (%s)\n", step.Name, step.Operation)
			if step.Operation == "process" {
				fmt.Printf("      argv: %s\n", strings.Join(step.Command, " "))
			}
			if step.Operation == "shellScript" {
				fmt.Printf("      shell: %s\n", step.Shell)
			}
			if step.Cwd != "" {
				fmt.Printf("      cwd: %s\n", step.Cwd)
			}
			for _, environment := range step.Environment {
				if environment.Kind == "secret" {
					fmt.Printf("      env: %s <- secret(%s)\n", environment.Name, environment.Secret)
				} else {
					fmt.Printf("      env: %s <- plain\n", environment.Name)
				}
			}
			if len(step.Capabilities) > 0 {
				fmt.Printf("      capabilities: %s\n", strings.Join(step.Capabilities, ", "))
			}
		}
	}
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
		case workflow.EventJobCompleted:
			fmt.Printf("[%s] %s (%dms)\n", event.Job, event.State, event.Duration)
		case workflow.EventWorkflowCompleted:
			fmt.Printf("Workflow %s %s\n", event.Workflow, event.State)
		}
	}
}

func exportWorkflowGitHub(root string, plan workflow.Plan, options workflowCommandOptions) {
	contents, err := workflow.ExportGitHub(plan)
	if err != nil {
		failWorkflow("TSPACK_WORKFLOW_PROVIDER_UNSUPPORTED", err.Error())
	}
	var parsed yaml.Node
	if err := yaml.Unmarshal(contents, &parsed); err != nil {
		failWorkflow("TSPACK_WORKFLOW_PROVIDER_OUTPUT_INVALID", err.Error())
	}
	relativePath := options.Output
	if relativePath == "" {
		relativePath = workflow.GitHubPath(plan.Workflow)
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
