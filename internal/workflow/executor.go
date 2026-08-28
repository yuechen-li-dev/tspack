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
)

type State string

const (
	StatePending   State = "pending"
	StateRunning   State = "running"
	StateSucceeded State = "succeeded"
	StateFailed    State = "failed"
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
)

type Event struct {
	Kind     EventKind `json:"kind"`
	Workflow string    `json:"workflow"`
	Job      string    `json:"job,omitempty"`
	Step     string    `json:"step,omitempty"`
	Stream   string    `json:"stream,omitempty"`
	Output   string    `json:"output,omitempty"`
	State    State     `json:"state,omitempty"`
	Duration int64     `json:"durationMs,omitempty"`
	Code     string    `json:"code,omitempty"`
	Severity string    `json:"severity,omitempty"`
	Message  string    `json:"message,omitempty"`
	Details  []string  `json:"details,omitempty"`
}

type JobResult struct {
	Identity   string       `json:"identity"`
	State      State        `json:"state"`
	Error      string       `json:"error,omitempty"`
	DurationMs int64        `json:"durationMs"`
	Steps      []StepResult `json:"steps,omitempty"`
}

type StepResult struct {
	Identity   string `json:"identity"`
	State      State  `json:"state"`
	ExitCode   *int   `json:"exitCode,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"durationMs"`
}

type Result struct {
	Workflow string      `json:"workflow"`
	State    State       `json:"state"`
	Jobs     []JobResult `json:"jobs"`
}

type EventSink func(Event)

type NativeOperations interface {
	Run(context.Context, string, []string) ([]diag.Diagnostic, error)
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
	Options project.Options
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
	if executor.Concurrency <= 0 {
		executor.Concurrency = runtime.NumCPU()
		if executor.Concurrency > 4 {
			executor.Concurrency = 4
		}
	}
	if executor.Environment == nil {
		executor.Environment = os.LookupEnv
	}
	result := Result{Workflow: plan.Workflow, State: StateRunning}
	executor.emit(Event{Kind: EventWorkflowStarted, Workflow: plan.Workflow, State: StateRunning})

	states := make(map[string]State, len(plan.Jobs))
	for _, job := range plan.Jobs {
		states[job.Identity] = StatePending
	}

	type completedJob struct {
		result JobResult
	}
	completed := make(chan completedJob, executor.Concurrency)
	running := 0
	results := map[string]JobResult{}

	for len(results) < len(plan.Jobs) {
		madeProgress := false
		for _, planned := range plan.Jobs {
			if states[planned.Identity] != StatePending {
				continue
			}
			if ctx.Err() != nil {
				states[planned.Identity] = StateCancelled
				results[planned.Identity] = JobResult{Identity: planned.Identity, State: StateCancelled}
				madeProgress = true
				continue
			}
			if dependencyFailed(planned.Needs, states) {
				states[planned.Identity] = StateBlocked
				jobResult := JobResult{Identity: planned.Identity, State: StateBlocked, Error: "required job did not succeed"}
				results[planned.Identity] = jobResult
				executor.emit(Event{Kind: EventJobCompleted, Workflow: plan.Workflow, Job: planned.Identity, State: StateBlocked})
				madeProgress = true
				continue
			}
			if running >= executor.Concurrency || !dependenciesSucceeded(planned.Needs, states) {
				continue
			}
			states[planned.Identity] = StateRunning
			running++
			madeProgress = true
			go func(job PlanJob) {
				completed <- completedJob{result: executor.runJob(ctx, plan.Workflow, job)}
			}(planned)
		}
		if len(results) == len(plan.Jobs) {
			break
		}
		if running == 0 {
			if !madeProgress {
				for _, planned := range plan.Jobs {
					if states[planned.Identity] == StatePending {
						states[planned.Identity] = StateBlocked
						results[planned.Identity] = JobResult{Identity: planned.Identity, State: StateBlocked, Error: "scheduler made no progress"}
					}
				}
			}
			continue
		}
		finished := <-completed
		running--
		states[finished.result.Identity] = finished.result.State
		results[finished.result.Identity] = finished.result
	}

	result.State = StateSucceeded
	for _, planned := range plan.Jobs {
		jobResult := results[planned.Identity]
		result.Jobs = append(result.Jobs, jobResult)
		if jobResult.State == StateCancelled {
			result.State = StateCancelled
		} else if result.State != StateCancelled && (jobResult.State == StateFailed || jobResult.State == StateBlocked) {
			result.State = StateFailed
		}
	}
	executor.emit(Event{Kind: EventWorkflowCompleted, Workflow: plan.Workflow, State: result.State})
	return result
}

func (executor *Executor) runJob(ctx context.Context, workflowIdentity string, job PlanJob) JobResult {
	started := time.Now()
	result := JobResult{Identity: job.Identity, State: StateRunning}
	executor.emit(Event{Kind: EventJobStarted, Workflow: workflowIdentity, Job: job.Identity, State: StateRunning})
	if !platformCompatible(job.Platform) {
		result.State = StateFailed
		result.Error = "TSPACK_WORKFLOW_PLATFORM_UNAVAILABLE: job requires " + job.Platform
		executor.emit(Event{Kind: EventJobCompleted, Workflow: workflowIdentity, Job: job.Identity, State: result.State})
		return result
	}
	for stepIndex, step := range job.Steps {
		stepStarted := time.Now()
		executor.emit(Event{Kind: EventStepStarted, Workflow: workflowIdentity, Job: job.Identity, Step: step.Identity, State: StateRunning})
		err := executor.runStep(ctx, workflowIdentity, job, step)
		duration := time.Since(stepStarted).Milliseconds()
		state := StateSucceeded
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				state = StateCancelled
			} else {
				state = StateFailed
			}
		}
		stepResult := StepResult{Identity: step.Identity, State: state, DurationMs: duration}
		if err != nil {
			stepResult.Error = err.Error()
			stepResult.ExitCode = processExitCode(err)
		}
		result.Steps = append(result.Steps, stepResult)
		executor.emit(Event{Kind: EventStepCompleted, Workflow: workflowIdentity, Job: job.Identity, Step: step.Identity, State: state, Duration: duration})
		if err != nil {
			for _, skipped := range job.Steps[stepIndex+1:] {
				result.Steps = append(result.Steps, StepResult{Identity: skipped.Identity, State: StateSkipped})
			}
			result.State = state
			result.Error = err.Error()
			result.DurationMs = time.Since(started).Milliseconds()
			executor.emit(Event{Kind: EventJobCompleted, Workflow: workflowIdentity, Job: job.Identity, State: state, Duration: result.DurationMs})
			return result
		}
	}
	result.State = StateSucceeded
	result.DurationMs = time.Since(started).Milliseconds()
	executor.emit(Event{Kind: EventJobCompleted, Workflow: workflowIdentity, Job: job.Identity, State: result.State, Duration: result.DurationMs})
	return result
}

func processExitCode(err error) *int {
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return nil
	}
	exitCode := exitError.ExitCode()
	return &exitCode
}

func (executor *Executor) runStep(parent context.Context, workflowIdentity string, job PlanJob, step PlanStep) error {
	ctx := parent
	cancel := func() {}
	if step.TimeoutSeconds > 0 {
		ctx, cancel = context.WithTimeout(parent, time.Duration(step.TimeoutSeconds)*time.Second)
	}
	defer cancel()
	if step.Operation == "sync" || step.Operation == "check" || step.Operation == "pack" {
		if executor.Native == nil {
			return errors.New("TSPACK_WORKFLOW_NATIVE_EXECUTOR_MISSING: native lifecycle operations are unavailable")
		}
		diagnostics, err := executor.Native.Run(ctx, step.Operation, step.Packages)
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
		return err
	}

	environment, secrets, err := executor.resolveEnvironment(job.Environment, step.Environment)
	if err != nil {
		return err
	}
	cwd, err := executor.resolveCwd(step.Cwd)
	if err != nil {
		return err
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
		return fmt.Errorf("TSPACK_WORKFLOW_STEP_UNSUPPORTED: %s", step.Operation)
	}
	command.Dir = cwd
	command.Env = append(os.Environ(), environment...)
	configureWorkflowProcess(command)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("TSPACK_WORKFLOW_PROCESS_START_FAILED: %w", err)
	}
	cleanup, err := attachWorkflowCleanup(command)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return fmt.Errorf("TSPACK_WORKFLOW_PROCESS_OWNERSHIP_FAILED: %w", err)
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
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("TSPACK_WORKFLOW_PROCESS_FAILED: %w", err)
	}
	return nil
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

func dependenciesSucceeded(needs []string, states map[string]State) bool {
	for _, dependency := range needs {
		if states[dependency] != StateSucceeded {
			return false
		}
	}
	return true
}

func dependencyFailed(needs []string, states map[string]State) bool {
	for _, dependency := range needs {
		state := states[dependency]
		if state == StateFailed || state == StateBlocked || state == StateCancelled || state == StateSkipped {
			return true
		}
	}
	return false
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
