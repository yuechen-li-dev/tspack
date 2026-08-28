package workflow

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func TestMatchResultLowersTypedProjectionAndSelectsEveryOutcome(t *testing.T) {
	declaration := matchFixture()
	flow, err := BuildFlow(declaration)
	if err != nil {
		t.Fatal(err)
	}
	if flow.SchemaVersion != 2 || countNodeKind(flow, NodeMatch) != 1 {
		t.Fatalf("schema=%d matchNodes=%d", flow.SchemaVersion, countNodeKind(flow, NodeMatch))
	}
	if !hasValueDefinition(flow, "artifacts", ValueArtifactReference) {
		t.Fatalf("artifact projection is absent: %+v", flow.Values)
	}

	cases := []struct {
		event MachineEventKind
		want  string
	}{
		{MachineEffectSucceeded, "pack-success"},
		{MachineEffectFailed, "report-failure"},
		{MachineCancelRequested, "report-cancel"},
		{MachineTimeout, "report-timeout"},
	}
	for _, testCase := range cases {
		t.Run(string(testCase.event), func(t *testing.T) {
			snapshot := NewSnapshot(flow, 100, 100)
			snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineStart})
			build := effectIdentity(flow, "build")
			snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectStarted, Node: build})
			snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: testCase.event, Node: build, Error: "controlled"})
			active := activeEffectNodes(flow, snapshot)
			if len(active) != 1 || active[0].Effect.Name != testCase.want {
				t.Fatalf("active=%+v snapshot=%+v", active, snapshot)
			}
			if snapshot.Trace[len(snapshot.Trace)-1].ValueIdentity == "" {
				t.Fatalf("trace does not identify the matched value: %+v", snapshot.Trace)
			}
		})
	}
}

func TestMatchResultSupportsBuildTestAndAuditResults(t *testing.T) {
	for _, operation := range []string{"build", "test", "audit"} {
		t.Run(operation, func(t *testing.T) {
			sourceIdentity := "effect/" + operation
			source := manifest.WorkflowValueRef{Identity: sourceIdentity, Source: sourceIdentity, ResultType: operation, Category: "regionLocal"}
			arms := []manifest.WorkflowMatchArm{}
			for _, outcome := range []string{"succeeded", "failed", "cancelled", "timedOut"} {
				arms = append(arms, manifest.WorkflowMatchArm{Kind: outcome, Flow: flowEffect("sync")})
			}
			declaration := manifest.Workflow{Identity: "Match" + operation, Flow: &manifest.WorkflowFlowNode{
				Kind:   "match",
				Source: &source,
				Effect: &manifest.WorkflowStep{Operation: operation, ResultIdentity: sourceIdentity},
				Arms:   arms,
			}}
			flow, err := BuildFlow(declaration)
			if err != nil || countNodeKind(flow, NodeMatch) != 1 {
				t.Fatalf("flow=%+v error=%v", flow, err)
			}
		})
	}
}

func TestRealExecutorCarriesBuildValueThroughMatchProjection(t *testing.T) {
	flow, err := BuildFlow(matchFixture())
	if err != nil {
		t.Fatal(err)
	}
	result := (&Executor{Native: structuredLifecycleOperations{}}).RunFlow(context.Background(), flow)
	if result.State != StateSucceeded || result.Snapshot == nil {
		t.Fatalf("result=%+v", result)
	}
	if _, exists := result.Snapshot.Values["value/match/effect/build"]; !exists {
		t.Fatalf("values=%+v", result.Snapshot.Values)
	}
	if countExecutedOperation(flow, *result.Snapshot, "build") != 1 || countExecutedOperation(flow, *result.Snapshot, "pack") != 1 {
		t.Fatalf("effects=%+v", result.Effects)
	}
}

type projectionAwareOperations struct {
	packReceivedArtifacts bool
}

func (operations *projectionAwareOperations) Run(context.Context, string, []string) ([]diag.Diagnostic, error) {
	return nil, nil
}

func (operations *projectionAwareOperations) RunStep(_ context.Context, step PlanStep) (NativeResult, error) {
	if step.Operation == "build" {
		return structuredLifecycleOperations{}.RunStep(context.Background(), step)
	}
	if step.Operation == "pack" && len(step.ResolvedInputs) == 1 {
		input := step.ResolvedInputs[0]
		operations.packReceivedArtifacts = len(input.Reference.FieldPath) == 1 &&
			input.Reference.FieldPath[0] == "artifacts" &&
			input.Result != nil &&
			input.Result.Build != nil &&
			len(input.Result.Build.Artifacts) == 1
	}
	return NativeResult{}, nil
}

func TestRealEffectInvocationReceivesResolvedTypedProjection(t *testing.T) {
	flow, err := BuildFlow(matchFixture())
	if err != nil {
		t.Fatal(err)
	}
	operations := &projectionAwareOperations{}
	result := (&Executor{Native: operations}).RunFlow(context.Background(), flow)
	if result.State != StateSucceeded || !operations.packReceivedArtifacts {
		t.Fatalf("result=%+v projectionReceived=%v", result, operations.packReceivedArtifacts)
	}
}

func TestFinallyRunsCleanupForSuccessFailureCancellationAndRecordsCleanupFailure(t *testing.T) {
	flow, err := BuildFlow(finallyFixture())
	if err != nil {
		t.Fatal(err)
	}

	run := func(bodyEvent MachineEventKind, cleanupEvent MachineEventKind) Snapshot {
		snapshot := NewSnapshot(flow, 100, 100)
		snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineStart})
		body := activeEffectNodes(flow, snapshot)[0]
		snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectStarted, Node: body.Identity})
		snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: bodyEvent, Node: body.Identity, Error: "body"})
		cleanup := activeEffectNodes(flow, snapshot)
		if len(cleanup) != 1 || !cleanup[0].Cleanup || cleanup[0].Effect.Operation != "audit" {
			t.Fatalf("cleanup=%+v snapshot=%+v", cleanup, snapshot)
		}
		snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectStarted, Node: cleanup[0].Identity})
		return mustStep(t, flow, snapshot, MachineEvent{Kind: cleanupEvent, Node: cleanup[0].Identity, Error: "cleanup"})
	}

	if snapshot := run(MachineEffectSucceeded, MachineEffectSucceeded); snapshot.Status != StateSucceeded {
		t.Fatalf("success status=%s", snapshot.Status)
	}
	if snapshot := run(MachineEffectFailed, MachineEffectSucceeded); snapshot.Status != StateFailed {
		t.Fatalf("failure status=%s", snapshot.Status)
	}
	if snapshot := run(MachineCancelRequested, MachineEffectSucceeded); snapshot.Status != StateCancelled {
		t.Fatalf("cancel status=%s", snapshot.Status)
	}
	if snapshot := run(MachineTimeout, MachineEffectSucceeded); snapshot.Status != StateTimedOut {
		t.Fatalf("timeout status=%s", snapshot.Status)
	}
	if snapshot := run(MachineEffectSucceeded, MachineTimeout); snapshot.Status != StateTimedOut {
		t.Fatalf("cleanup timeout status=%s", snapshot.Status)
	}
	cleanupFailed := run(MachineEffectFailed, MachineEffectFailed)
	if cleanupFailed.Status != StateFailed || len(cleanupFailed.CleanupFailures) != 1 || cleanupFailed.CleanupFailures[0].Cause != TerminalFailed {
		t.Fatalf("combined failure=%+v", cleanupFailed)
	}
	cancelCleanupFailed := run(MachineCancelRequested, MachineEffectFailed)
	if cancelCleanupFailed.Status != StateFailed || cancelCleanupFailed.CleanupFailures[0].Cause != TerminalCancelled {
		t.Fatalf("cancel plus cleanup failure=%+v", cancelCleanupFailed)
	}
}

func TestNestedFinallyIsCompositional(t *testing.T) {
	inner := manifest.WorkflowFlowNode{
		Kind:    "finally",
		Body:    &manifest.WorkflowFlowNode{Kind: "effect", Effect: &manifest.WorkflowStep{Operation: "test", ResultIdentity: "effect/test"}},
		Cleanup: &manifest.WorkflowFlowNode{Kind: "effect", Effect: &manifest.WorkflowStep{Operation: "audit", ResultIdentity: "effect/audit"}},
	}
	declaration := manifest.Workflow{
		Identity: "NestedFinally",
		Flow: &manifest.WorkflowFlowNode{
			Kind:    "finally",
			Body:    &inner,
			Cleanup: &manifest.WorkflowFlowNode{Kind: "effect", Effect: &manifest.WorkflowStep{Operation: "build", ResultIdentity: "effect/build"}},
		},
	}
	flow, err := BuildFlow(declaration)
	if err != nil {
		t.Fatal(err)
	}
	result := (&Executor{Native: structuredLifecycleOperations{}}).RunFlow(context.Background(), flow)
	if result.State != StateSucceeded || result.Snapshot == nil {
		t.Fatalf("result=%+v", result)
	}
	cleanupCount := 0
	for _, node := range flow.Nodes {
		if node.Cleanup && result.Snapshot.Nodes[node.Identity].State == StateSucceeded {
			cleanupCount++
		}
	}
	if cleanupCount != 2 {
		t.Fatalf("executed cleanup count=%d", cleanupCount)
	}
}

func TestForEachExposesSequentialCursorAndDeterministicSourceOrder(t *testing.T) {
	flow, err := BuildFlow(forEachFixture())
	if err != nil {
		t.Fatal(err)
	}
	if countNodeKind(flow, NodeIterator) != 3 {
		t.Fatalf("iterator nodes=%d", countNodeKind(flow, NodeIterator))
	}
	snapshot := NewSnapshot(flow, 100, 100)
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineStart})
	for wantIndex := 0; wantIndex < 3; wantIndex++ {
		active := activeEffectNodes(flow, snapshot)
		if len(active) != 1 || active[0].Effect.Name != []string{"linux", "windows", "currentHost"}[wantIndex] {
			t.Fatalf("iteration %d active=%+v", wantIndex, active)
		}
		snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectStarted, Node: active[0].Identity})
		snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectSucceeded, Node: active[0].Identity})
	}
	if snapshot.Status != StateSucceeded || len(snapshot.Iterators) != 3 {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	for _, cursor := range snapshot.Iterators {
		if cursor.Count != 3 || cursor.Mode != "sequential" || cursor.FailurePolicy != "failFast" {
			t.Fatalf("cursor=%+v", cursor)
		}
	}
}

func TestValueRegionLawRejectsArtifactUseWithoutTransport(t *testing.T) {
	ref := manifest.WorkflowValueRef{Identity: "effect/build.artifacts", Source: "effect/build", ResultType: "build", FieldPath: []string{"artifacts"}, Category: "artifactReference"}
	declaration := manifest.Workflow{
		Identity: "CrossRegion",
		Flow: &manifest.WorkflowFlowNode{Kind: "sequence", Children: []manifest.WorkflowFlowNode{
			{Kind: "region", RunsOn: "linux", Children: []manifest.WorkflowFlowNode{{Kind: "effect", Effect: &manifest.WorkflowStep{Operation: "build", ResultIdentity: "effect/build"}}}},
			{Kind: "region", RunsOn: "windows", Children: []manifest.WorkflowFlowNode{{Kind: "effect", Effect: &manifest.WorkflowStep{Operation: "pack", ResultIdentity: "effect/pack", Inputs: []manifest.WorkflowValueRef{ref}}}}},
		}},
	}
	_, err := BuildFlow(declaration)
	if err == nil || !strings.Contains(err.Error(), "TSPACK_WORKFLOW_VALUE_REGION_ILLEGAL") {
		t.Fatalf("error=%v", err)
	}
}

type cleanupAfterCancelOperations struct {
	cleanupRan bool
	started    chan struct{}
}

func (operations *cleanupAfterCancelOperations) Run(_ context.Context, _ string, _ []string) ([]diag.Diagnostic, error) {
	return nil, nil
}

func (operations *cleanupAfterCancelOperations) RunStep(ctx context.Context, step PlanStep) (NativeResult, error) {
	if step.Operation == "test" {
		operations.started <- struct{}{}
		<-ctx.Done()
		return NativeResult{}, ctx.Err()
	}
	operations.cleanupRan = true
	return NativeResult{}, nil
}

func TestRealExecutorRunsFinallyCleanupWithBoundedUncancelledContext(t *testing.T) {
	flow, err := BuildFlow(finallyFixture())
	if err != nil {
		t.Fatal(err)
	}
	operations := &cleanupAfterCancelOperations{started: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Result, 1)
	go func() {
		done <- (&Executor{Native: operations}).RunFlow(ctx, flow)
	}()
	<-operations.started
	cancel()
	result := <-done
	if result.State != StateCancelled || !operations.cleanupRan {
		t.Fatalf("result=%+v cleanupRan=%v", result, operations.cleanupRan)
	}
}

func matchFixture() manifest.Workflow {
	source := manifest.WorkflowValueRef{Identity: "effect/build", Source: "effect/build", ResultType: "build", Category: "regionLocal"}
	artifacts := manifest.WorkflowValueRef{Identity: "effect/build.artifacts", Source: "effect/build", ResultType: "build", FieldPath: []string{"artifacts"}, Category: "artifactReference"}
	arm := func(kind string, name string, operation string, inputs []manifest.WorkflowValueRef) manifest.WorkflowMatchArm {
		return manifest.WorkflowMatchArm{Kind: kind, Flow: manifest.WorkflowFlowNode{Kind: "effect", Effect: &manifest.WorkflowStep{Operation: operation, Name: name, Inputs: inputs}}}
	}
	match := manifest.WorkflowFlowNode{
		Kind:   "match",
		Source: &source,
		Effect: &manifest.WorkflowStep{Operation: "build", ResultIdentity: "effect/build"},
		Arms: []manifest.WorkflowMatchArm{
			arm("succeeded", "pack-success", "pack", []manifest.WorkflowValueRef{artifacts}),
			arm("failed", "report-failure", "process", nil),
			arm("cancelled", "report-cancel", "process", nil),
			arm("timedOut", "report-timeout", "process", nil),
		},
	}
	return manifest.Workflow{
		Identity: "Match",
		Flow: &manifest.WorkflowFlowNode{Kind: "sequence", Children: []manifest.WorkflowFlowNode{
			{Kind: "effect", Effect: &manifest.WorkflowStep{Operation: "build", ResultIdentity: "effect/build"}},
			match,
		}},
	}
}

func countExecutedOperation(flow Flow, snapshot Snapshot, operation string) int {
	count := 0
	for _, node := range flow.Nodes {
		if node.Effect != nil && node.Effect.Operation == operation && snapshot.Nodes[node.Identity].State == StateSucceeded {
			count++
		}
	}
	return count
}

func finallyFixture() manifest.Workflow {
	return manifest.Workflow{
		Identity: "Finally",
		Flow: &manifest.WorkflowFlowNode{
			Kind:    "finally",
			Body:    &manifest.WorkflowFlowNode{Kind: "effect", Effect: &manifest.WorkflowStep{Operation: "test", ResultIdentity: "effect/body"}},
			Cleanup: &manifest.WorkflowFlowNode{Kind: "effect", Effect: &manifest.WorkflowStep{Operation: "audit", ResultIdentity: "effect/cleanup"}},
		},
	}
}

func forEachFixture() manifest.Workflow {
	items := []manifest.WorkflowForEachItem{}
	for index, platform := range []string{"linux", "windows", "currentHost"} {
		items = append(items, manifest.WorkflowForEachItem{
			Index: index,
			Value: manifest.WorkflowIterationValue{Kind: "platform", String: platform},
			Flow: manifest.WorkflowFlowNode{Kind: "effect", Effect: &manifest.WorkflowStep{
				Operation:      "test",
				Name:           platform,
				ResultIdentity: "effect/test/" + platform,
			}},
		})
	}
	return manifest.Workflow{Identity: "ForEach", Flow: &manifest.WorkflowFlowNode{Kind: "forEach", Identity: "platform", Items: items}}
}

func hasValueDefinition(flow Flow, field string, category ValueCategory) bool {
	for _, value := range flow.Values {
		if len(value.FieldPath) == 1 && value.FieldPath[0] == field && value.Category == category {
			return true
		}
	}
	return false
}

func BenchmarkM77MatchProjectionLowering(b *testing.B) {
	declaration := matchFixture()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := BuildFlow(declaration); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkM77FinallyLowering(b *testing.B) {
	declaration := finallyFixture()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := BuildFlow(declaration); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkM77ForEach100Lowering(b *testing.B) {
	declaration := forEachCountFixture(100)
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := BuildFlow(declaration); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkM77ForEach1000LimitDiagnostic(b *testing.B) {
	declaration := forEachCountFixture(1000)
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := BuildFlow(declaration); err == nil {
			b.Fatal("expected finite expansion limit diagnostic")
		}
	}
}

func BenchmarkM77IteratorStep(b *testing.B) {
	flow, err := BuildFlow(forEachFixture())
	if err != nil {
		b.Fatal(err)
	}
	snapshot := NewSnapshot(flow, 100, 100)
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		candidate := cloneSnapshot(snapshot)
		if _, _, err := Step(flow, candidate, MachineEvent{Kind: MachineStart}); err != nil {
			b.Fatal(err)
		}
	}
}

func forEachCountFixture(count int) manifest.Workflow {
	items := make([]manifest.WorkflowForEachItem, 0, count)
	for index := 0; index < count; index++ {
		items = append(items, manifest.WorkflowForEachItem{
			Index: index,
			Value: workflowNumberValue(index),
			Flow: manifest.WorkflowFlowNode{Kind: "effect", Effect: &manifest.WorkflowStep{
				Operation:      "test",
				ResultIdentity: "effect/test/" + fmt.Sprint(index),
			}},
		})
	}
	return manifest.Workflow{Identity: "ForEachCount", Flow: &manifest.WorkflowFlowNode{Kind: "forEach", Identity: "item", Items: items}}
}

func workflowNumberValue(value int) manifest.WorkflowIterationValue {
	number := float64(value)
	return manifest.WorkflowIterationValue{Kind: "number", Number: &number}
}
