package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/project"
	"github.com/yuechen-li-dev/tspack/internal/testcmd"
	"gopkg.in/yaml.v3"
)

func TestBuildPlanExpandsMatrixDeterministicallyAndConnectsAllNeeds(t *testing.T) {
	declaration := manifest.Workflow{
		Identity: "CI",
		Triggers: []manifest.WorkflowTrigger{{Kind: "push", Branches: []string{"release", "main"}}},
		Jobs: []manifest.WorkflowJob{
			{
				Identity: "test",
				RunsOn:   "currentHost",
				Matrix: map[string][]any{
					"mode":  {"debug", "release"},
					"shard": {float64(1), float64(2)},
				},
				Steps: []manifest.WorkflowStep{{Operation: "check"}},
			},
			{Identity: "pack", Needs: []string{"test"}, RunsOn: "currentHost", Steps: []manifest.WorkflowStep{{Operation: "pack"}}},
		},
	}

	first := BuildPlan(declaration)
	second := BuildPlan(declaration)
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("plan is not deterministic:\n%s\n%s", firstJSON, secondJSON)
	}
	if len(first.Jobs) != 5 {
		t.Fatalf("jobs=%d want 5", len(first.Jobs))
	}
	if len(first.Jobs[4].Needs) != 4 {
		t.Fatalf("fan-in needs=%v", first.Jobs[4].Needs)
	}
	if first.Jobs[0].Identity != `test[mode=7:"debug",shard=1:1]` {
		t.Fatalf("first identity=%q", first.Jobs[0].Identity)
	}
}

type structuredLifecycleOperations struct{}

func (structuredLifecycleOperations) Run(context.Context, string, []string) ([]diag.Diagnostic, error) {
	return nil, nil
}

type semanticFailureOperations struct {
	failing string
}

func (operations semanticFailureOperations) Run(context.Context, string, []string) ([]diag.Diagnostic, error) {
	return nil, nil
}

func (operations semanticFailureOperations) RunStep(_ context.Context, step PlanStep) (NativeResult, error) {
	if step.Operation != operations.failing {
		return NativeResult{}, nil
	}
	switch step.Operation {
	case "build":
		result := project.BuildOperationResult{Diagnostics: []diag.Diagnostic{{Code: "TSPACK_COMPILER_BUILD_FAILED", Severity: diag.SeverityError, Message: "controlled build failure"}}}
		return NativeResult{Build: &result}, errors.New("TSPACK_COMPILER_BUILD_FAILED: controlled build failure")
	case "test":
		result := project.TestOperationResult{
			ExitCode: 1,
			Failed:   1,
			Tests: []testcmd.TestEvidence{{
				ID:      "suite/failing fact",
				Status:  "failed",
				Failure: map[string]any{"code": "TSPACK_TEST_ASSERTION_FAILED"},
			}},
		}
		return NativeResult{Test: &result}, errors.New("TSPACK_TEST_FAILED: one or more tests failed")
	case "audit":
		result := project.AuditOperationResult{Failing: 1, Diagnostics: []diag.Diagnostic{{Code: "TSPACK_AUDIT_POLICY_REJECTED", Severity: diag.SeverityError, Message: "controlled audit failure"}}}
		return NativeResult{Audit: &result}, errors.New("TSPACK_AUDIT_POLICY_REJECTED: controlled audit failure")
	default:
		return NativeResult{}, nil
	}
}

func (structuredLifecycleOperations) RunStep(_ context.Context, step PlanStep) (NativeResult, error) {
	switch step.Operation {
	case "build":
		return NativeResult{Build: &project.BuildOperationResult{Artifacts: []project.BuildArtifact{{Package: "app", Target: "browser", Kind: "javaScript", Path: "dist/app.js"}}}}, nil
	case "test":
		return NativeResult{Test: &project.TestOperationResult{Passed: 3, Skipped: 1}}, nil
	case "audit":
		return NativeResult{Audit: &project.AuditOperationResult{Source: "OSV.dev", AuditLevel: "high"}}, nil
	default:
		return NativeResult{}, nil
	}
}

func TestExecutorPreservesStructuredNativeLifecycleResults(t *testing.T) {
	plan := Plan{Workflow: "CI", Jobs: []PlanJob{{Identity: "ci", Platform: "currentHost", Steps: []PlanStep{
		{Identity: "ci/step-01-test", Operation: "test"},
		{Identity: "ci/step-02-build", Operation: "build"},
		{Identity: "ci/step-03-audit", Operation: "audit"},
	}}}}
	result := (&Executor{Native: structuredLifecycleOperations{}}).Run(context.Background(), plan)
	if result.State != StateSucceeded {
		t.Fatalf("result=%+v", result)
	}
	steps := result.Jobs[0].Steps
	if steps[0].Result == nil || steps[0].Result.Test == nil || steps[0].Result.Test.Passed != 3 {
		t.Fatalf("test result=%+v", steps[0].Result)
	}
	if steps[1].Result == nil || len(steps[1].Result.Build.Artifacts) != 1 {
		t.Fatalf("build result=%+v", steps[1].Result)
	}
	if steps[2].Result == nil || steps[2].Result.Audit.Source != "OSV.dev" {
		t.Fatalf("audit result=%+v", steps[2].Result)
	}
}

func TestNativeBuildFailureBlocksDependentsAndPreservesSemanticDiagnostic(t *testing.T) {
	events := []Event{}
	plan := Plan{Workflow: "Failure", Jobs: []PlanJob{
		{Identity: "build", Platform: "currentHost", Steps: []PlanStep{{Identity: "build/step-01-build", Operation: "build"}}},
		{Identity: "dependent", Platform: "currentHost", Needs: []string{"build"}, Steps: []PlanStep{{Identity: "dependent/step-01-test", Operation: "test"}}},
		{Identity: "independent", Platform: "currentHost", Steps: []PlanStep{{Identity: "independent/step-01-check", Operation: "check"}}},
	}}
	result := (&Executor{Native: semanticFailureOperations{failing: "build"}, Concurrency: 1, Events: func(event Event) { events = append(events, event) }}).Run(context.Background(), plan)
	if result.Jobs[0].State != StateFailed || result.Jobs[1].State != StateBlocked || result.Jobs[2].State != StateSucceeded {
		t.Fatalf("jobs=%+v", result.Jobs)
	}
	found := false
	for _, event := range events {
		if event.Kind == EventStepDiagnostic && event.Code == "TSPACK_COMPILER_BUILD_FAILED" {
			found = true
		}
	}
	if !found {
		t.Fatalf("events=%+v", events)
	}
}

func TestNativeTestAndAuditFailuresPreserveTypedEvidence(t *testing.T) {
	for _, operation := range []string{"test", "audit"} {
		t.Run(operation, func(t *testing.T) {
			plan := Plan{Workflow: "Failure", Jobs: []PlanJob{{Identity: operation, Platform: "currentHost", Steps: []PlanStep{{Identity: operation + "/step-01-" + operation, Operation: operation}}}}}
			result := (&Executor{Native: semanticFailureOperations{failing: operation}}).Run(context.Background(), plan)
			step := result.Jobs[0].Steps[0]
			if step.State != StateFailed || step.Result == nil {
				t.Fatalf("step=%+v", step)
			}
			if operation == "test" && (step.Result.Test == nil || step.Result.Test.Tests[0].ID != "suite/failing fact") {
				t.Fatalf("test=%+v", step.Result.Test)
			}
			if operation == "audit" && (step.Result.Audit == nil || step.Result.Audit.Failing != 1) {
				t.Fatalf("audit=%+v", step.Result.Audit)
			}
		})
	}
}

type concurrencyOperations struct {
	active int32
	max    int32
}

func (operations *concurrencyOperations) Run(ctx context.Context, _ string, _ []string) ([]diag.Diagnostic, error) {
	active := atomic.AddInt32(&operations.active, 1)
	for {
		maximum := atomic.LoadInt32(&operations.max)
		if active <= maximum || atomic.CompareAndSwapInt32(&operations.max, maximum, active) {
			break
		}
	}
	select {
	case <-ctx.Done():
		atomic.AddInt32(&operations.active, -1)
		return nil, ctx.Err()
	case <-time.After(20 * time.Millisecond):
		atomic.AddInt32(&operations.active, -1)
		return nil, nil
	}
}

func TestExecutorBoundsConcurrencyAndHonorsFanIn(t *testing.T) {
	jobs := []PlanJob{}
	for index := 0; index < 12; index++ {
		identity := "job-" + string(rune('a'+index))
		jobs = append(jobs, PlanJob{Identity: identity, JobIdentity: identity, Platform: "currentHost", Steps: []PlanStep{{Identity: identity + "/step-01-check", Operation: "check"}}})
	}
	needs := []string{}
	for _, job := range jobs {
		needs = append(needs, job.Identity)
	}
	jobs = append(jobs, PlanJob{Identity: "fanin", JobIdentity: "fanin", Platform: "currentHost", Needs: needs, Steps: []PlanStep{{Identity: "fanin/step-01-check", Operation: "check"}}})
	for index := 0; index < 4; index++ {
		identity := "fanout-" + string(rune('a'+index))
		jobs = append(jobs, PlanJob{Identity: identity, JobIdentity: identity, Platform: "currentHost", Needs: []string{"fanin"}, Steps: []PlanStep{{Identity: identity + "/step-01-check", Operation: "check"}}})
	}
	operations := &concurrencyOperations{}
	executor := Executor{Root: t.TempDir(), Manifest: &manifest.ManifestIR{}, Native: operations, Concurrency: 3}

	result := executor.Run(context.Background(), Plan{Workflow: "Stress", Jobs: jobs})

	if result.State != StateSucceeded {
		t.Fatalf("state=%s jobs=%#v", result.State, result.Jobs)
	}
	if maximum := atomic.LoadInt32(&operations.max); maximum != 3 {
		t.Fatalf("max concurrency=%d want 3", maximum)
	}
}

type noopOperations struct{}

func (noopOperations) Run(context.Context, string, []string) ([]diag.Diagnostic, error) {
	return nil, nil
}

func BenchmarkBuildPlanRepresentative(b *testing.B) {
	jobs := make([]manifest.WorkflowJob, 0, 20)
	for index := 0; index < 20; index++ {
		identity := "job-" + string(rune('a'+index))
		jobs = append(jobs, manifest.WorkflowJob{
			Identity: identity,
			RunsOn:   "currentHost",
			Matrix: map[string][]any{
				"mode":  {"debug", "release"},
				"shard": {float64(1), float64(2)},
			},
			Steps: []manifest.WorkflowStep{{Operation: "check"}},
		})
	}
	declaration := manifest.Workflow{Identity: "Benchmark", Triggers: []manifest.WorkflowTrigger{{Kind: "manual"}}, Jobs: jobs}
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		_ = BuildPlan(declaration)
	}
}

func BenchmarkSchedulerNoopRepresentative(b *testing.B) {
	jobs := make([]PlanJob, 0, 40)
	for index := 0; index < 40; index++ {
		identity := "job-" + string(rune('A'+index))
		jobs = append(jobs, PlanJob{
			Identity:    identity,
			JobIdentity: identity,
			Platform:    "currentHost",
			Steps:       []PlanStep{{Identity: identity + "/step-01-check", Operation: "check"}},
		})
	}
	executor := Executor{Root: b.TempDir(), Manifest: &manifest.ManifestIR{}, Native: noopOperations{}, Concurrency: 4}
	plan := Plan{Workflow: "Benchmark", Jobs: jobs}
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		_ = executor.Run(context.Background(), plan)
	}
}

func TestExecutorRedactsSecretsAndCancelsExternalProcess(t *testing.T) {
	if os.Getenv("TSPACK_WORKFLOW_HELPER") == "output" {
		_, _ = os.Stdout.WriteString("secret=" + os.Getenv("TOKEN") + "\n")
		os.Exit(0)
	}
	if os.Getenv("TSPACK_WORKFLOW_HELPER") == "sleep" {
		time.Sleep(10 * time.Second)
		os.Exit(0)
	}
	if os.Getenv("TSPACK_WORKFLOW_HELPER") == "tree-parent" {
		child := exec.Command(os.Args[0], "-test.run=TestExecutorRedactsSecretsAndCancelsExternalProcess")
		child.Env = append(os.Environ(), "TSPACK_WORKFLOW_HELPER=tree-child")
		if err := child.Start(); err != nil {
			os.Exit(3)
		}
		os.Exit(0)
	}
	if os.Getenv("TSPACK_WORKFLOW_HELPER") == "tree-child" {
		time.Sleep(500 * time.Millisecond)
		_ = os.WriteFile(os.Getenv("TSPACK_WORKFLOW_MARKER"), []byte("orphaned"), 0o644)
		os.Exit(0)
	}

	var mutex sync.Mutex
	outputs := []string{}
	executor := Executor{
		Root:     t.TempDir(),
		Manifest: &manifest.ManifestIR{},
		Environment: func(name string) (string, bool) {
			if name == "CI_TOKEN" {
				return "super-secret", true
			}
			return os.LookupEnv(name)
		},
		Events: func(event Event) {
			if event.Kind == EventStepOutput {
				mutex.Lock()
				outputs = append(outputs, event.Output)
				mutex.Unlock()
			}
		},
	}
	outputPlan := Plan{
		Workflow: "Secrets",
		Jobs: []PlanJob{
			{
				Identity: "one",
				Platform: "currentHost",
				Steps: []PlanStep{
					{
						Identity:  "one/step-01-process",
						Operation: "process",
						Command:   []string{os.Args[0], "-test.run=TestExecutorRedactsSecretsAndCancelsExternalProcess"},
						Environment: []Environment{
							{Name: "TSPACK_WORKFLOW_HELPER", Kind: "plain", Value: "output"},
							{Name: "TOKEN", Kind: "secret", Secret: "CI_TOKEN"},
						},
					},
				},
			},
		},
	}
	result := executor.Run(context.Background(), outputPlan)
	if result.State != StateSucceeded || strings.Contains(strings.Join(outputs, "\n"), "super-secret") || !strings.Contains(strings.Join(outputs, "\n"), "[REDACTED]") {
		t.Fatalf("result=%#v outputs=%v", result, outputs)
	}

	cancelPlan := Plan{
		Workflow: "Cancel",
		Jobs: []PlanJob{
			{
				Identity: "one",
				Platform: "currentHost",
				Steps: []PlanStep{
					{
						Identity:  "one/step-01-process",
						Operation: "process",
						Command:   []string{os.Args[0], "-test.run=TestExecutorRedactsSecretsAndCancelsExternalProcess"},
						Environment: []Environment{
							{Name: "TSPACK_WORKFLOW_HELPER", Kind: "plain", Value: "sleep"},
						},
					},
				},
			},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	result = executor.Run(ctx, cancelPlan)
	if result.State != StateCancelled || time.Since(started) > 3*time.Second {
		t.Fatalf("cancel result=%#v duration=%s", result, time.Since(started))
	}

	marker := filepath.Join(t.TempDir(), "orphan-marker")
	treePlan := Plan{
		Workflow: "Tree",
		Jobs: []PlanJob{
			{
				Identity: "one",
				Platform: "currentHost",
				Steps: []PlanStep{
					{
						Identity:  "one/step-01-process",
						Operation: "process",
						Command:   []string{os.Args[0], "-test.run=TestExecutorRedactsSecretsAndCancelsExternalProcess"},
						Environment: []Environment{
							{Name: "TSPACK_WORKFLOW_HELPER", Kind: "plain", Value: "tree-parent"},
							{Name: "TSPACK_WORKFLOW_MARKER", Kind: "plain", Value: marker},
						},
					},
				},
			},
		},
	}
	result = executor.Run(context.Background(), treePlan)
	time.Sleep(700 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("process tree escaped workflow ownership: result=%#v", result)
	}
}

func TestGitHubExportIsDeterministicValidAndContainsOnlySecretReferences(t *testing.T) {
	plan := Plan{
		Workflow: "CI",
		Triggers: []Trigger{{Kind: "pullRequest", Paths: []string{"src/**"}}, {Kind: "manual"}, {Kind: "push", Branches: []string{"main"}}},
		Jobs: []PlanJob{{
			Identity:    "test",
			Platform:    "linux",
			Environment: []Environment{{Name: "TOKEN", Kind: "secret", Secret: "CI_TOKEN"}},
			Steps:       []PlanStep{{Identity: "test/step-01-check", Operation: "check"}},
		}},
	}
	first, err := ExportGitHub(plan)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := ExportGitHub(plan)
	if string(first) != string(second) {
		t.Fatal("GitHub output is not deterministic")
	}
	var document yaml.Node
	if err := yaml.Unmarshal(first, &document); err != nil {
		t.Fatalf("generated YAML is invalid: %v\n%s", err, first)
	}
	text := string(first)
	for _, expected := range []string{"workflow_dispatch", "pull_request", "actions/checkout@v4", GitHubSetupAction, "tspack workflow run CI --ci-provider github", "${{ secrets.CI_TOKEN }}"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %q in:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "super-secret") {
		t.Fatal("generated provider file contains a secret value")
	}
}
