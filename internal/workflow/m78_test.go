package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/project"
)

type parallelProbeOperations struct {
	active          atomic.Int32
	maximum         atomic.Int32
	completed       atomic.Int32
	secondCompleted atomic.Bool
	rollingWindow   atomic.Bool
}

type transportFlowOperations struct {
	packPath string
}

func (operations *transportFlowOperations) Run(context.Context, string, []string) ([]diag.Diagnostic, error) {
	return nil, nil
}

func (operations *transportFlowOperations) RunStep(_ context.Context, step PlanStep) (NativeResult, error) {
	switch step.Operation {
	case "build":
		return NativeResult{Build: &project.BuildOperationResult{Artifacts: []project.BuildArtifact{{Package: "app", Target: "browser", Kind: "javaScript", Path: "dist/app.js"}}}}, nil
	case "pack":
		if len(step.ResolvedInputs) == 1 && step.ResolvedInputs[0].Result != nil && step.ResolvedInputs[0].Result.Build != nil {
			operations.packPath = step.ResolvedInputs[0].Result.Build.Artifacts[0].Path
		}
	}
	return NativeResult{}, nil
}

func (operations *parallelProbeOperations) Run(context.Context, string, []string) ([]diag.Diagnostic, error) {
	return nil, nil
}

func (operations *parallelProbeOperations) RunStep(ctx context.Context, step PlanStep) (NativeResult, error) {
	active := operations.active.Add(1)
	defer operations.active.Add(-1)
	for {
		maximum := operations.maximum.Load()
		if active <= maximum || operations.maximum.CompareAndSwap(maximum, active) {
			break
		}
	}
	delay := 25 * time.Millisecond
	if strings.HasSuffix(step.ResultIdentity, "/0") {
		delay = 5 * time.Millisecond
	}
	if strings.HasSuffix(step.ResultIdentity, "/1") {
		delay = 75 * time.Millisecond
	}
	if strings.HasSuffix(step.ResultIdentity, "/2") && !operations.secondCompleted.Load() {
		operations.rollingWindow.Store(true)
	}
	select {
	case <-time.After(delay):
		if strings.HasSuffix(step.ResultIdentity, "/1") {
			operations.secondCompleted.Store(true)
		}
		operations.completed.Add(1)
		return NativeResult{Test: &project.TestOperationResult{Passed: 1}}, nil
	case <-ctx.Done():
		return NativeResult{}, ctx.Err()
	}
}

func TestParallelForEachCollectAllIsBoundedAndSourceOrdered(t *testing.T) {
	declaration := forEachCountFixture(3)
	declaration.Flow.Mode = "parallel"
	declaration.Flow.Concurrency = 2
	declaration.Flow.FailurePolicy = "collectAll"
	flow, err := BuildFlow(declaration)
	if err != nil {
		t.Fatal(err)
	}
	if flow.SchemaVersion != FlowSchemaVersion || len(flow.Aggregates) != 1 {
		t.Fatalf("schema=%d aggregates=%+v", flow.SchemaVersion, flow.Aggregates)
	}
	if flow.Aggregates[0].ResultType != "iterationOutcome<test>" || flow.Aggregates[0].Concurrency != 2 {
		t.Fatalf("aggregate=%+v", flow.Aggregates[0])
	}

	snapshot := NewSnapshot(flow, 100, 100)
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineStart})
	active := activeEffectNodes(flow, snapshot)
	if len(active) != 2 {
		t.Fatalf("first wave active=%d, want 2", len(active))
	}
	for _, node := range active {
		snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectStarted, Node: node.Identity})
	}
	// Physical completion order is deliberately reversed.
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectFailed, Node: active[1].Identity, Error: "boom"})
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectSucceeded, Node: active[0].Identity, Result: &NativeResult{Test: &project.TestOperationResult{Passed: 1}}})
	active = activeEffectNodes(flow, snapshot)
	if len(active) != 1 {
		t.Fatalf("second wave active=%d, want 1", len(active))
	}
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectStarted, Node: active[0].Identity})
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectSucceeded, Node: active[0].Identity, Result: &NativeResult{Test: &project.TestOperationResult{Passed: 1}}})
	if snapshot.Status != StateSucceeded {
		t.Fatalf("status=%s", snapshot.Status)
	}
	aggregate := snapshot.Aggregates[flow.Aggregates[0].Identity]
	want := []TerminalKind{TerminalSucceeded, TerminalFailed, TerminalSucceeded}
	for index, element := range aggregate.Elements {
		if element.Index != index || element.Outcome != want[index] {
			t.Fatalf("aggregate=%+v", aggregate)
		}
	}
}

func TestRealParallelForEachRespectsDeclaredConcurrency(t *testing.T) {
	declaration := forEachCountFixture(5)
	declaration.Flow.Mode = "parallel"
	declaration.Flow.Concurrency = 2
	declaration.Flow.FailurePolicy = "collectAll"
	flow, err := BuildFlow(declaration)
	if err != nil {
		t.Fatal(err)
	}
	operations := &parallelProbeOperations{}
	result := (&Executor{Native: operations, Concurrency: 8}).RunFlow(context.Background(), flow)
	if result.State != StateSucceeded || operations.completed.Load() != 5 || operations.maximum.Load() != 2 || !operations.rollingWindow.Load() {
		t.Fatalf("state=%s completed=%d maximum=%d rolling=%t", result.State, operations.completed.Load(), operations.maximum.Load(), operations.rollingWindow.Load())
	}
}

func TestParallelForEachTimeoutPoliciesAreDeterministic(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		policy      string
		wantStatus  State
		wantPending bool
	}{
		{name: "fail fast", policy: "failFast", wantStatus: StateTimedOut, wantPending: false},
		{name: "collect all", policy: "collectAll", wantStatus: StateRunning, wantPending: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			declaration := forEachCountFixture(3)
			declaration.Flow.Mode = "parallel"
			declaration.Flow.Concurrency = 2
			declaration.Flow.FailurePolicy = testCase.policy
			flow, err := BuildFlow(declaration)
			if err != nil {
				t.Fatal(err)
			}
			snapshot := NewSnapshot(flow, 100, 100)
			snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineStart})
			active := activeEffectNodes(flow, snapshot)
			for _, node := range active {
				snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectStarted, Node: node.Identity})
			}
			snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineTimeout, Node: active[0].Identity, Error: "deadline"})
			if testCase.policy == "collectAll" {
				snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectSucceeded, Node: active[1].Identity})
			}
			if snapshot.Status != testCase.wantStatus {
				t.Fatalf("status=%s", snapshot.Status)
			}
			if (len(activeEffectNodes(flow, snapshot)) == 1) != testCase.wantPending {
				t.Fatalf("active=%+v", activeEffectNodes(flow, snapshot))
			}
		})
	}
}

func TestParallelForEachCancellationPreservesObservedAggregatePrefix(t *testing.T) {
	declaration := forEachCountFixture(3)
	declaration.Flow.Mode = "parallel"
	declaration.Flow.Concurrency = 2
	declaration.Flow.FailurePolicy = "collectAll"
	flow, err := BuildFlow(declaration)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := NewSnapshot(flow, 100, 100)
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineStart})
	active := activeEffectNodes(flow, snapshot)
	for _, node := range active {
		snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectStarted, Node: node.Identity})
	}
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectSucceeded, Node: active[0].Identity})
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineCancelRequested, Error: "cancelled"})
	if snapshot.Status != StateCancelled {
		t.Fatalf("status=%s", snapshot.Status)
	}
	aggregate := snapshot.Aggregates[flow.Aggregates[0].Identity]
	if aggregate.Elements[0].Outcome != TerminalSucceeded || aggregate.Elements[1].Outcome != "" || aggregate.Elements[2].Outcome != "" {
		t.Fatalf("aggregate=%+v", aggregate)
	}
}

func TestFinallyRunsAfterCollectAllParallelFanOut(t *testing.T) {
	body := forEachCountFixture(2).Flow
	body.Mode = "parallel"
	body.Concurrency = 2
	body.FailurePolicy = "collectAll"
	declaration := manifest.Workflow{Identity: "ParallelFinally", Flow: &manifest.WorkflowFlowNode{
		Kind:    "finally",
		Body:    body,
		Cleanup: &manifest.WorkflowFlowNode{Kind: "effect", Effect: &manifest.WorkflowStep{Operation: "audit", Name: "cleanup"}},
	}}
	flow, err := BuildFlow(declaration)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := NewSnapshot(flow, 100, 100)
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineStart})
	active := activeEffectNodes(flow, snapshot)
	for _, node := range active {
		snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectStarted, Node: node.Identity})
	}
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectFailed, Node: active[0].Identity, Error: "failed"})
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectSucceeded, Node: active[1].Identity})
	active = activeEffectNodes(flow, snapshot)
	if len(active) != 1 || active[0].Effect.Name != "cleanup" {
		t.Fatalf("active=%+v", active)
	}
}

func TestWhenEvaluatesTypedSuccessfulResultFact(t *testing.T) {
	failed := manifest.WorkflowValueRef{Identity: "effect/test.failed", Source: "effect/test", ResultType: "test", FieldPath: []string{"failed"}, Category: "control"}
	threshold := float64(0)
	declaration := manifest.Workflow{Identity: "Facts", Flow: &manifest.WorkflowFlowNode{Kind: "sequence", Children: []manifest.WorkflowFlowNode{
		{Kind: "effect", Effect: &manifest.WorkflowStep{Operation: "test", ResultIdentity: "effect/test"}},
		{Kind: "when", Predicate: &manifest.WorkflowPredicate{Kind: "greaterThan", Input: &failed, Number: &threshold}, Then: &manifest.WorkflowFlowNode{Kind: "effect", Effect: &manifest.WorkflowStep{Operation: "audit", Name: "report"}}},
	}}}
	flow, err := BuildFlow(declaration)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := NewSnapshot(flow, 100, 100)
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineStart})
	active := activeEffectNodes(flow, snapshot)
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectStarted, Node: active[0].Identity})
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectSucceeded, Node: active[0].Identity, Result: &NativeResult{Test: &project.TestOperationResult{Failed: 1}}})
	active = activeEffectNodes(flow, snapshot)
	if len(active) != 1 || active[0].Effect.Name != "report" {
		t.Fatalf("active=%+v", active)
	}
}

func TestWhenFalseSkipsTrueBranchAndBooleanCompositionIsTyped(t *testing.T) {
	failed := manifest.WorkflowValueRef{Identity: "effect/test.failed", Source: "effect/test", ResultType: "test", FieldPath: []string{"failed"}, Category: "control"}
	skipped := manifest.WorkflowValueRef{Identity: "effect/test.skipped", Source: "effect/test", ResultType: "test", FieldPath: []string{"skipped"}, Category: "control"}
	zero := float64(0)
	declaration := manifest.Workflow{Identity: "BooleanFacts", Flow: &manifest.WorkflowFlowNode{Kind: "sequence", Children: []manifest.WorkflowFlowNode{
		{Kind: "effect", Effect: &manifest.WorkflowStep{Operation: "test", ResultIdentity: "effect/test"}},
		{Kind: "when", Predicate: &manifest.WorkflowPredicate{Kind: "and", Children: []manifest.WorkflowPredicate{
			{Kind: "greaterThan", Input: &failed, Number: &zero},
			{Kind: "greaterThan", Input: &skipped, Number: &zero},
		}}, Then: &manifest.WorkflowFlowNode{Kind: "effect", Effect: &manifest.WorkflowStep{Operation: "audit", Name: "should-not-run"}}},
	}}}
	flow, err := BuildFlow(declaration)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := NewSnapshot(flow, 100, 100)
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineStart})
	active := activeEffectNodes(flow, snapshot)
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectStarted, Node: active[0].Identity})
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectSucceeded, Node: active[0].Identity, Result: &NativeResult{Test: &project.TestOperationResult{Failed: 1, Skipped: 0}}})
	if snapshot.Status != StateSucceeded || len(activeEffectNodes(flow, snapshot)) != 0 {
		t.Fatalf("snapshot=%+v", snapshot)
	}
}

func TestArtifactTransferStagesAndIntegrityProtectsArtifact(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dist", "app.js"), []byte("const answer = 42;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	executor := &Executor{Root: root}
	step := PlanStep{
		Operation:      "transfer",
		TransferTarget: "windows",
		ResolvedInputs: []ResolvedValue{{Result: &NativeResult{Build: &project.BuildOperationResult{Artifacts: []project.BuildArtifact{{Package: "app", Target: "browser", Kind: "javaScript", Path: "dist/app.js"}}}}}},
	}
	result, err := executor.runArtifactTransfer(context.Background(), step)
	if err != nil {
		t.Fatal(err)
	}
	artifact := result.Build.Artifacts[0]
	if artifact.Path == "dist/app.js" || !strings.HasPrefix(artifact.ContentHash, "sha256:") {
		t.Fatalf("artifact=%+v", artifact)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(artifact.Path))); err != nil {
		t.Fatal(err)
	}
}

func TestRealFlowTransfersArtifactBeforeCrossRegionConsumption(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dist", "app.js"), []byte("artifact"), 0o644); err != nil {
		t.Fatal(err)
	}
	buildArtifacts := manifest.WorkflowValueRef{Identity: "effect/build.artifacts", Source: "effect/build", ResultType: "build", FieldPath: []string{"artifacts"}, Category: "artifactReference"}
	portableArtifacts := manifest.WorkflowValueRef{Identity: "effect/transfer.artifacts", Source: "effect/transfer", ResultType: "transfer", FieldPath: []string{"artifacts"}, Category: "artifactReference"}
	declaration := manifest.Workflow{Identity: "Transport", Flow: &manifest.WorkflowFlowNode{Kind: "sequence", Children: []manifest.WorkflowFlowNode{
		{Kind: "effect", Effect: &manifest.WorkflowStep{Operation: "build", ResultIdentity: "effect/build"}},
		{Kind: "effect", Effect: &manifest.WorkflowStep{Operation: "transfer", ResultIdentity: "effect/transfer", TransferTarget: "currentHost", Inputs: []manifest.WorkflowValueRef{buildArtifacts}}},
		{Kind: "region", RunsOn: "currentHost", Env: []manifest.WorkflowEnvironment{{Name: "REGION_TEST", Value: manifest.WorkflowValue{Kind: "plain", Value: "1"}}}, Children: []manifest.WorkflowFlowNode{{Kind: "effect", Effect: &manifest.WorkflowStep{Operation: "pack", Inputs: []manifest.WorkflowValueRef{portableArtifacts}}}}},
	}}}
	flow, err := BuildFlow(declaration)
	if err != nil {
		t.Fatal(err)
	}
	operations := &transportFlowOperations{}
	result := (&Executor{Root: root, Native: operations}).RunFlow(context.Background(), flow)
	if result.State != StateSucceeded || operations.packPath == "" || operations.packPath == "dist/app.js" {
		t.Fatalf("state=%s packPath=%q", result.State, operations.packPath)
	}
	if !strings.Contains(operations.packPath, "workflow-transport") {
		t.Fatalf("packPath=%q", operations.packPath)
	}
}

func TestArtifactTransferRejectsWorkspaceEscape(t *testing.T) {
	executor := &Executor{Root: t.TempDir()}
	step := PlanStep{
		Operation:      "transfer",
		TransferTarget: "currentHost",
		ResolvedInputs: []ResolvedValue{{Result: &NativeResult{Build: &project.BuildOperationResult{Artifacts: []project.BuildArtifact{{Path: "../secret.txt"}}}}}},
	}
	_, err := executor.runArtifactTransfer(context.Background(), step)
	if err == nil || !strings.Contains(err.Error(), "TRANSFER_PATH_INVALID") {
		t.Fatalf("error=%v", err)
	}
}

func TestNestedSequentialForEachUsesHierarchicalPaths(t *testing.T) {
	inner := forEachCountFixture(1).Flow
	outer := forEachCountFixture(1)
	outer.Flow.Items[0].Flow = *inner
	flow, err := BuildFlow(outer)
	if err != nil {
		t.Fatal(err)
	}
	if flow.Expansion.PlannedIterations != 2 {
		t.Fatalf("expansion=%+v", flow.Expansion)
	}
	paths := []string{}
	for _, node := range flow.Nodes {
		if node.Iterator != nil {
			paths = append(paths, node.Iterator.Path)
		}
	}
	if !containsString(paths, "item[0]") || !containsString(paths, "item[0]/item[0]") {
		t.Fatalf("cursor paths=%v", paths)
	}
}

func TestNestedParallelForEachIsExplicitlyDeferred(t *testing.T) {
	inner := forEachCountFixture(1).Flow
	inner.Mode = "parallel"
	inner.Concurrency = 2
	outer := forEachCountFixture(1)
	outer.Flow.Items[0].Flow = *inner
	_, err := BuildFlow(outer)
	if err == nil || !strings.Contains(err.Error(), "NESTED_PARALLEL_UNSUPPORTED") {
		t.Fatalf("error=%v", err)
	}
}

func TestNestedForEachExpansionBudgetIsExactAndBounded(t *testing.T) {
	outer := forEachCountFixture(DefaultForEachLimit)
	for index := range outer.Flow.Items {
		outer.Flow.Items[index].Flow = *forEachCountFixture(15).Flow
	}
	count, err := plannedIterationCount(*outer.Flow, DefaultExpansionBudget)
	if err != nil {
		t.Fatal(err)
	}
	if count != DefaultExpansionBudget {
		t.Fatalf("expansion=%d", count)
	}
	outer.Flow.Items[0].Flow = *forEachCountFixture(16).Flow
	_, err = plannedIterationCount(*outer.Flow, DefaultExpansionBudget)
	if err == nil || !strings.Contains(err.Error(), "expands to 4097 total iterations; workflow limit is 4096") {
		t.Fatalf("error=%v", err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func BenchmarkM78ParallelForEach100Lowering(b *testing.B) {
	declaration := forEachCountFixture(100)
	declaration.Flow.Mode = "parallel"
	declaration.Flow.Concurrency = 8
	declaration.Flow.FailurePolicy = "collectAll"
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := BuildFlow(declaration); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkM78PredicateEvaluation(b *testing.B) {
	failed := ValueRef{Identity: "value/test.failed", Source: "value/test", ResultType: "test", FieldPath: []string{"failed"}, Category: ValueControl}
	threshold := float64(0)
	predicate := Predicate{Kind: "greaterThan", Input: &failed, Number: &threshold}
	snapshot := Snapshot{Values: map[string]NativeResult{"value/test": {Test: &project.TestOperationResult{Failed: 1}}}}
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if matched, err := evaluatePredicate(&snapshot, &predicate); err != nil || !matched {
			b.Fatalf("matched=%t error=%v", matched, err)
		}
	}
}

func BenchmarkM78CollectAll100Outcomes(b *testing.B) {
	declaration := forEachCountFixture(100)
	declaration.Flow.Mode = "parallel"
	declaration.Flow.Concurrency = 8
	declaration.Flow.FailurePolicy = "collectAll"
	flow, err := BuildFlow(declaration)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		snapshot := NewSnapshot(flow, 1_000, 1)
		for _, element := range flow.Aggregates[0].Elements {
			recordAggregateOutcome(flow, &snapshot, element, TerminalSucceeded)
		}
	}
}

func BenchmarkM78ArtifactTransport1KiB(b *testing.B) {
	root := b.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dist", "artifact.bin"), make([]byte, 1024), 0o644); err != nil {
		b.Fatal(err)
	}
	artifact := project.BuildArtifact{Package: "app", Target: "build", Kind: "binary", Path: "dist/artifact.bin"}
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := transportArtifact(root, "region/default", "currentHost", artifact, 0); err != nil {
			b.Fatal(err)
		}
	}
}
