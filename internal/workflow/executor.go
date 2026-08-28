package workflow

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/project"
	"github.com/yuechen-li-dev/tspack/internal/testcmd"
)

type State string

const (
	StatePending   State = "pending"
	StateWaiting   State = "waiting"
	StateRunning   State = "running"
	StateSucceeded State = "succeeded"
	StateFailed    State = "failed"
	StateTimedOut  State = "timedOut"
	StateSkipped   State = "skipped"
	StateBlocked   State = "blocked"
	StateCancelled State = "cancelled"
)

type EventKind string

const (
	EventWorkflowStarted   EventKind = "workflowStarted"
	EventJobStarted        EventKind = "jobStarted"
	EventStepStarted       EventKind = "stepStarted"
	EventStepOutput        EventKind = "stepOutput"
	EventStepDiagnostic    EventKind = "stepDiagnostic"
	EventStepCompleted     EventKind = "stepCompleted"
	EventJobCompleted      EventKind = "jobCompleted"
	EventWorkflowCompleted EventKind = "workflowCompleted"
	EventMachineStepped    EventKind = "machineStepped"
)

type Event struct {
	Kind     EventKind     `json:"kind"`
	Workflow string        `json:"workflow"`
	Job      string        `json:"job,omitempty"`
	Step     string        `json:"step,omitempty"`
	Stream   string        `json:"stream,omitempty"`
	Output   string        `json:"output,omitempty"`
	State    State         `json:"state,omitempty"`
	Duration int64         `json:"durationMs,omitempty"`
	Code     string        `json:"code,omitempty"`
	Severity string        `json:"severity,omitempty"`
	Message  string        `json:"message,omitempty"`
	Details  []string      `json:"details,omitempty"`
	Result   *NativeResult `json:"result,omitempty"`
	Trace    *StepTrace    `json:"trace,omitempty"`
}

type JobResult struct {
	Identity   string       `json:"identity"`
	State      State        `json:"state"`
	Error      string       `json:"error,omitempty"`
	DurationMs int64        `json:"durationMs"`
	Steps      []StepResult `json:"steps,omitempty"`
}

type StepResult struct {
	Identity   string        `json:"identity"`
	State      State         `json:"state"`
	ExitCode   *int          `json:"exitCode,omitempty"`
	Error      string        `json:"error,omitempty"`
	DurationMs int64         `json:"durationMs"`
	Result     *NativeResult `json:"result,omitempty"`
}

type Result struct {
	Workflow string       `json:"workflow"`
	State    State        `json:"state"`
	Jobs     []JobResult  `json:"jobs"`
	Effects  []StepResult `json:"effects,omitempty"`
	Snapshot *Snapshot    `json:"snapshot,omitempty"`
}

type EventSink func(Event)

type NativeOperations interface {
	Run(context.Context, string, []string) ([]diag.Diagnostic, error)
}

type StructuredNativeOperations interface {
	RunStep(context.Context, PlanStep) (NativeResult, error)
}

type NativeResult struct {
	Build *project.BuildOperationResult `json:"build,omitempty"`
	Test  *project.TestOperationResult  `json:"test,omitempty"`
	Audit *project.AuditOperationResult `json:"audit,omitempty"`
}

type Executor struct {
	Root        string
	Manifest    *manifest.ManifestIR
	Native      NativeOperations
	Context     ExecutionContext
	Concurrency int
	Environment func(string) (string, bool)
	Events      EventSink
}

type ExecutionContext struct {
	IsCI     bool   `json:"isCI"`
	Provider string `json:"provider,omitempty"`
}

type ProjectOperations struct {
	Options       project.Options
	BuildExecutor project.BuildTargetExecutor
}

func (operations ProjectOperations) RunStep(ctx context.Context, step PlanStep) (NativeResult, error) {
	var result NativeResult
	var diagnostics []diag.Diagnostic
	var err error
	switch step.Operation {
	case "build":
		buildResult := project.RunBuild(ctx, project.BuildRequest{Project: operations.Options, Packages: step.Packages, Targets: step.Targets, Executor: operations.BuildExecutor})
		result.Build = &buildResult
		diagnostics = buildResult.Diagnostics
	case "test":
		testResult := project.RunTest(ctx, project.TestRequest{Project: operations.Options, Options: testcmd.Options{RootDir: operations.Options.RootDir, Filter: step.Filter, CaptureStructured: true}})
		result.Test = &testResult
		diagnostics = testResult.Diagnostics
		if testResult.ExitCode != 0 && !hasErrorDiagnostics(diagnostics) {
			err = errors.New("TSPACK_TEST_FAILED: one or more tests failed")
		}
	case "audit":
		auditResult := project.RunAudit(ctx, project.AuditRequest{Project: operations.Options, AuditLevel: step.AuditLevel, RequireCoverage: step.RequireCoverage})
		result.Audit = &auditResult
		diagnostics = auditResult.Diagnostics
	default:
		diagnostics, err = operations.Run(ctx, step.Operation, step.Packages)
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == diag.SeverityError {
			return result, fmt.Errorf("%s: %s", diagnostic.Code, diagnostic.Message)
		}
	}
	return result, err
}

func (operations ProjectOperations) Run(ctx context.Context, operation string, packages []string) ([]diag.Diagnostic, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(packages) > 0 && operation != "pack" {
		return nil, fmt.Errorf("TSPACK_WORKFLOW_TARGETING_UNSUPPORTED: %s does not yet expose package selection", operation)
	}
	var diagnostics []diag.Diagnostic
	switch operation {
	case "sync":
		diagnostics = project.RunSync(project.SyncRequest{Project: operations.Options}).Diagnostics
	case "check":
		diagnostics = project.RunCheck(project.CheckRequest{Project: operations.Options}).Diagnostics
	case "pack":
		if len(packages) > 1 {
			return nil, errors.New("TSPACK_WORKFLOW_TARGETING_UNSUPPORTED: pack accepts at most one package")
		}
		options := project.PackOptions{}
		if len(packages) == 1 {
			options.PackageName = packages[0]
		}
		diagnostics = project.RunPack(project.PackRequest{Project: operations.Options, Options: options}).Diagnostics
	default:
		return nil, fmt.Errorf("TSPACK_WORKFLOW_STEP_UNSUPPORTED: %s", operation)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == diag.SeverityError {
			return diagnostics, fmt.Errorf("%s: %s", diagnostic.Code, diagnostic.Message)
		}
	}
	return diagnostics, nil
}

func (executor *Executor) Run(ctx context.Context, plan Plan) Result {
	flow, err := BuildFlowFromPlan(plan)
	if err != nil {
		return Result{Workflow: plan.Workflow, State: StateFailed}
	}
	return executor.RunFlow(ctx, flow)
}

func processExitCode(err error) *int {
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return nil
	}
	exitCode := exitError.ExitCode()
	return &exitCode
}

func (executor *Executor) runStep(parent context.Context, workflowIdentity string, job PlanJob, step PlanStep) (*NativeResult, error) {
	ctx := parent
	cancel := func() {}
	if step.TimeoutSeconds > 0 {
		ctx, cancel = context.WithTimeout(parent, time.Duration(step.TimeoutSeconds)*time.Second)
	}
	defer cancel()
	if step.Operation == "sync" || step.Operation == "check" || step.Operation == "build" || step.Operation == "test" || step.Operation == "pack" || step.Operation == "audit" {
		if executor.Native == nil {
			return nil, errors.New("TSPACK_WORKFLOW_NATIVE_EXECUTOR_MISSING: native lifecycle operations are unavailable")
		}
		var nativeResult NativeResult
		var diagnostics []diag.Diagnostic
		var err error
		if structured, ok := executor.Native.(StructuredNativeOperations); ok {
			nativeResult, err = structured.RunStep(ctx, step)
			diagnostics = nativeDiagnostics(nativeResult)
		} else {
			diagnostics, err = executor.Native.Run(ctx, step.Operation, step.Packages)
		}
		for _, diagnostic := range diagnostics {
			executor.emit(Event{
				Kind:     EventStepDiagnostic,
				Workflow: workflowIdentity,
				Job:      job.Identity,
				Step:     step.Identity,
				Code:     diagnostic.Code,
				Severity: string(diagnostic.Severity),
				Message:  diagnostic.Message,
				Details:  append([]string(nil), diagnostic.Details...),
			})
		}
		return &nativeResult, err
	}

	environment, secrets, err := executor.resolveEnvironment(job.Environment, step.Environment)
	if err != nil {
		return nil, err
	}
	cwd, err := executor.resolveCwd(step.Cwd)
	if err != nil {
		return nil, err
	}
	var command *exec.Cmd
	switch step.Operation {
	case "process":
		command = exec.CommandContext(ctx, step.Command[0], step.Command[1:]...)
	case "shellScript":
		shell := step.Shell
		if shell == "" {
			if runtime.GOOS == "windows" {
				shell = "powershell"
			} else {
				shell = "sh"
			}
		}
		if shell == "powershell" {
			command = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", step.Script)
		} else {
			command = exec.CommandContext(ctx, "sh", "-c", step.Script)
		}
	default:
		return nil, fmt.Errorf("TSPACK_WORKFLOW_STEP_UNSUPPORTED: %s", step.Operation)
	}
	command.Dir = cwd
	command.Env = append(os.Environ(), environment...)
	configureWorkflowProcess(command)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("TSPACK_WORKFLOW_PROCESS_START_FAILED: %w", err)
	}
	cleanup, err := attachWorkflowCleanup(command)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, fmt.Errorf("TSPACK_WORKFLOW_PROCESS_OWNERSHIP_FAILED: %w", err)
	}
	var outputWait sync.WaitGroup
	outputWait.Add(2)
	go executor.streamOutput(&outputWait, stdout, workflowIdentity, job.Identity, step.Identity, "stdout", secrets)
	go executor.streamOutput(&outputWait, stderr, workflowIdentity, job.Identity, step.Identity, "stderr", secrets)
	err = command.Wait()
	outputWait.Wait()
	if cleanupErr := cleanupWorkflowProcessTree(command.Process.Pid, cleanup); cleanupErr != nil && err == nil {
		err = fmt.Errorf("TSPACK_WORKFLOW_PROCESS_CLEANUP_FAILED: %w", cleanupErr)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("TSPACK_WORKFLOW_PROCESS_FAILED: %w", err)
	}
	return nil, nil
}

func nativeDiagnostics(result NativeResult) []diag.Diagnostic {
	if result.Build != nil {
		return result.Build.Diagnostics
	}
	if result.Test != nil {
		return result.Test.Diagnostics
	}
	if result.Audit != nil {
		return result.Audit.Diagnostics
	}
	return nil
}

func hasErrorDiagnostics(diagnostics []diag.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == diag.SeverityError {
			return true
		}
	}
	return false
}

func (executor *Executor) resolveEnvironment(jobValues []Environment, stepValues []Environment) ([]string, []string, error) {
	byName := map[string]Environment{}
	for _, value := range jobValues {
		byName[value.Name] = value
	}
	for _, value := range stepValues {
		byName[value.Name] = value
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	resolved := make([]string, 0, len(names))
	secrets := []string{}
	for _, name := range names {
		value := byName[name]
		if value.Kind == "secret" {
			secret, exists := executor.Environment(value.Secret)
			if !exists {
				return nil, nil, fmt.Errorf("TSPACK_WORKFLOW_SECRET_MISSING: secret %s required by environment %s", value.Secret, name)
			}
			resolved = append(resolved, name+"="+secret)
			if secret != "" {
				secrets = append(secrets, secret)
			}
			continue
		}
		resolved = append(resolved, name+"="+value.Value)
	}
	return resolved, secrets, nil
}

func (executor *Executor) resolveCwd(requested string) (string, error) {
	if requested == "" || requested == "workspace" {
		return executor.Root, nil
	}
	packageName := strings.TrimPrefix(requested, "package:")
	for _, declaredPackage := range executor.Manifest.Packages {
		if declaredPackage.Name != packageName {
			continue
		}
		candidate := filepath.Clean(filepath.Join(executor.Root, filepath.FromSlash(declaredPackage.Root)))
		relative, err := filepath.Rel(executor.Root, candidate)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", errors.New("TSPACK_WORKFLOW_CWD_ESCAPE: package root escapes workspace")
		}
		return candidate, nil
	}
	return "", fmt.Errorf("TSPACK_WORKFLOW_PACKAGE_UNKNOWN: %s", packageName)
}

func (executor *Executor) streamOutput(wait *sync.WaitGroup, reader io.Reader, workflowIdentity string, jobIdentity string, stepIdentity string, stream string, secrets []string) {
	defer wait.Done()
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		output := scanner.Text()
		for _, secret := range secrets {
			output = strings.ReplaceAll(output, secret, "[REDACTED]")
		}
		executor.emit(Event{Kind: EventStepOutput, Workflow: workflowIdentity, Job: jobIdentity, Step: stepIdentity, Stream: stream, Output: output})
	}
}

func (executor *Executor) emit(event Event) {
	if executor.Events != nil {
		executor.Events(event)
	}
}

func platformCompatible(platform string) bool {
	if platform == "" || platform == "currentHost" {
		return true
	}
	switch runtime.GOOS {
	case "darwin":
		return platform == "macos"
	default:
		return platform == runtime.GOOS
	}
}
