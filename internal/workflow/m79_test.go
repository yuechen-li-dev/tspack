package workflow

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/project"
)

type aggregateConsumptionOperations struct {
	mutex           sync.Mutex
	testCompletions []string
	reports         []string
	delayCompletion bool
}

func (operations *aggregateConsumptionOperations) Run(context.Context, string, []string) ([]diag.Diagnostic, error) {
	return nil, nil
}

func (operations *aggregateConsumptionOperations) RunStep(_ context.Context, step PlanStep) (NativeResult, error) {
	if step.Operation == "test" {
		if operations.delayCompletion {
			if step.Name == "test-0" {
				time.Sleep(40 * time.Millisecond)
			} else {
				time.Sleep(5 * time.Millisecond)
			}
		}
		operations.mutex.Lock()
		operations.testCompletions = append(operations.testCompletions, step.Name)
		operations.mutex.Unlock()
		return NativeResult{Test: &project.TestOperationResult{Passed: 1}}, nil
	}
	if step.Operation == "audit" {
		operations.mutex.Lock()
		operations.reports = append(operations.reports, step.Name)
		operations.mutex.Unlock()
	}
	return NativeResult{}, nil
}

func TestM79RealExecutorConsumesCollectedOutcomesInSourceOrder(t *testing.T) {
	declaration := aggregateConsumptionFixture()
	flow, err := BuildFlow(declaration)
	if err != nil {
		t.Fatal(err)
	}
	if flow.SchemaVersion != FlowSchemaVersion || flow.Expansion.PlannedIterations != 4 {
		t.Fatalf("schema=%d expansion=%+v", flow.SchemaVersion, flow.Expansion)
	}
	operations := &aggregateConsumptionOperations{delayCompletion: true}
	result := (&Executor{Native: operations, Concurrency: 8}).RunFlow(context.Background(), flow)
	if result.State != StateSucceeded {
		t.Fatalf("state=%s", result.State)
	}
	if len(operations.testCompletions) != 2 || operations.testCompletions[0] != "test-1" || operations.testCompletions[1] != "test-0" {
		t.Fatalf("physical completions=%v", operations.testCompletions)
	}
	if len(operations.reports) != 2 || operations.reports[0] != "report-0-succeeded" || operations.reports[1] != "report-1-succeeded" {
		t.Fatalf("second-pass reports=%v", operations.reports)
	}
	if len(flow.Aggregates) != 1 || flow.Aggregates[0].Elements[0] != "value/m79executor/result/0" || flow.Aggregates[0].Elements[1] != "value/m79executor/result/1" {
		t.Fatalf("aggregate=%+v", flow.Aggregates)
	}
}

func TestM79NestedCancellationAndTimeoutKeepCursorHierarchyCoherent(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		event      MachineEventKind
		wantStatus State
	}{
		{name: "cancellation", event: MachineCancelRequested, wantStatus: StateCancelled},
		{name: "timeout", event: MachineTimeout, wantStatus: StateTimedOut},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			flow, err := BuildFlow(nestedCountFixture(2, 2))
			if err != nil {
				t.Fatal(err)
			}
			snapshot := NewSnapshot(flow, 100, 100)
			snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineStart})
			active := activeEffectNodes(flow, snapshot)
			if len(active) != 1 {
				t.Fatalf("active=%+v", active)
			}
			snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectStarted, Node: active[0].Identity})
			event := MachineEvent{Kind: testCase.event, Node: active[0].Identity, Error: testCase.name}
			if testCase.event == MachineCancelRequested {
				event.Node = ""
			}
			snapshot = mustStep(t, flow, snapshot, event)
			if snapshot.Status != testCase.wantStatus || len(snapshot.Active) != 0 {
				t.Fatalf("status=%s active=%v", snapshot.Status, snapshot.Active)
			}
			paths := map[string]bool{}
			for _, cursor := range snapshot.Iterators {
				paths[cursor.Path] = true
			}
			if !paths["item[0]"] || !paths["item[0]/inner-0[0]"] {
				t.Fatalf("cursor paths=%v", paths)
			}
		})
	}
}

func aggregateConsumptionFixture() manifest.Workflow {
	return aggregateConsumptionFixtureCount(2)
}

func aggregateConsumptionFixtureCount(count int) manifest.Workflow {
	const aggregateIdentity = "aggregate/runs"
	elements := make([]string, 0, count)
	for index := 0; index < count; index++ {
		elements = append(elements, "result/"+strconv.Itoa(index))
	}
	producerItems := make([]manifest.WorkflowForEachItem, 0, len(elements))
	consumerItems := make([]manifest.WorkflowForEachItem, 0, len(elements))
	for index, element := range elements {
		indexText := strconv.Itoa(index)
		producerItems = append(producerItems, manifest.WorkflowForEachItem{
			Index: index,
			Value: workflowNumberValue(index),
			Flow: manifest.WorkflowFlowNode{Kind: "effect", Effect: &manifest.WorkflowStep{
				Operation:      "test",
				Name:           "test-" + indexText,
				ResultIdentity: element,
			}},
		})
		indexCopy := index
		source := manifest.WorkflowValueRef{
			Identity:   element,
			Source:     element,
			ResultType: "test",
			Category:   "regionLocal",
			Aggregate:  aggregateIdentity,
			Index:      &indexCopy,
		}
		consumerItems = append(consumerItems, manifest.WorkflowForEachItem{
			Index: index,
			Value: manifest.WorkflowIterationValue{Kind: "aggregateElement", Source: &source},
			Flow:  aggregateOutcomeMatch(element, index, source),
		})
	}
	return manifest.Workflow{
		Identity: "M79Executor",
		Flow: &manifest.WorkflowFlowNode{Kind: "sequence", Children: []manifest.WorkflowFlowNode{
			{
				Kind:          "forEach",
				Identity:      "produce",
				Items:         producerItems,
				Mode:          "parallel",
				Concurrency:   min(count, 8),
				FailurePolicy: "collectAll",
				Aggregate: &manifest.WorkflowAggregateRef{
					Identity:   aggregateIdentity,
					ResultType: "test",
					Elements:   elements,
				},
			},
			{
				Kind:          "forEach",
				Identity:      "consume",
				Items:         consumerItems,
				Mode:          "sequential",
				FailurePolicy: "failFast",
			},
		}},
	}
}

func aggregateOutcomeMatch(element string, index int, source manifest.WorkflowValueRef) manifest.WorkflowFlowNode {
	indexText := strconv.Itoa(index)
	arms := make([]manifest.WorkflowMatchArm, 0, 4)
	for _, outcome := range []string{"succeeded", "failed", "cancelled", "timedOut"} {
		arms = append(arms, manifest.WorkflowMatchArm{
			Kind: outcome,
			Flow: manifest.WorkflowFlowNode{Kind: "effect", Effect: &manifest.WorkflowStep{
				Operation: "audit",
				Name:      "report-" + indexText + "-" + outcome,
			}},
		})
	}
	return manifest.WorkflowFlowNode{
		Kind:   "match",
		Source: &source,
		Effect: &manifest.WorkflowStep{Operation: "test", ResultIdentity: element},
		Arms:   arms,
	}
}

func BenchmarkM79AggregateConsumptionLowering(b *testing.B) {
	declaration := aggregateConsumptionFixture()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := BuildFlow(declaration); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkM79Aggregate100SecondPassLowering(b *testing.B) {
	declaration := aggregateConsumptionFixtureCount(100)
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := BuildFlow(declaration); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkM79Aggregate100SecondPassStepping(b *testing.B) {
	flow, err := BuildFlow(aggregateConsumptionFixtureCount(100))
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		operations := &aggregateConsumptionOperations{}
		result := (&Executor{Native: operations, Concurrency: MaxForEachConcurrency}).RunFlow(context.Background(), flow)
		if result.State != StateSucceeded || len(operations.reports) != 100 {
			b.Fatalf("state=%s reports=%d", result.State, len(operations.reports))
		}
	}
}

func BenchmarkM79Aggregate256ReferenceSnapshot(b *testing.B) {
	declaration := forEachCountFixture(256)
	declaration.Flow.Mode = "parallel"
	declaration.Flow.Concurrency = 8
	declaration.Flow.FailurePolicy = "collectAll"
	flow, err := BuildFlow(declaration)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		snapshot := NewSnapshot(flow, DefaultStepLimit, DefaultTraceLimit)
		for _, element := range flow.Aggregates[0].Elements {
			recordAggregateOutcome(flow, &snapshot, element, TerminalSucceeded)
		}
		if len(snapshot.Aggregates[flow.Aggregates[0].Identity].Elements) != 256 {
			b.Fatal("snapshot lost aggregate references")
		}
	}
}

func BenchmarkM79Nested10x10Lowering(b *testing.B) {
	benchmarkNestedLowering(b, 10, 10)
}

func BenchmarkM79Nested32x32Lowering(b *testing.B) {
	benchmarkNestedLowering(b, 32, 32)
}

func BenchmarkM79ExpansionBudgetRejection(b *testing.B) {
	declaration := nestedCountFixture(256, 16)
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := BuildFlow(declaration); err == nil {
			b.Fatal("expected expansion budget diagnostic")
		}
	}
}

func benchmarkNestedLowering(b *testing.B, outerCount int, innerCount int) {
	declaration := nestedCountFixture(outerCount, innerCount)
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := BuildFlow(declaration); err != nil {
			b.Fatal(err)
		}
	}
}

func nestedCountFixture(outerCount int, innerCount int) manifest.Workflow {
	outer := forEachCountFixture(outerCount)
	for index := range outer.Flow.Items {
		inner := forEachCountFixture(innerCount)
		inner.Flow.Identity = "inner-" + strconv.Itoa(index)
		for innerIndex := range inner.Flow.Items {
			inner.Flow.Items[innerIndex].Flow.Effect.ResultIdentity += "/outer/" + strconv.Itoa(index)
		}
		outer.Flow.Items[index].Flow = *inner.Flow
	}
	return outer
}
