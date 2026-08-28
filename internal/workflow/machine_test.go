package workflow

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func TestFlowAuthoringLowersSequenceAndParallelToExplicitMachine(t *testing.T) {
	declaration := manifest.Workflow{
		Identity: "CI",
		Triggers: []manifest.WorkflowTrigger{{Kind: "manual"}},
		Flow: &manifest.WorkflowFlowNode{
			Kind: "sequence",
			Children: []manifest.WorkflowFlowNode{
				flowEffect("sync"),
				{
					Kind: "parallel",
					Children: []manifest.WorkflowFlowNode{
						{Kind: "branch", Identity: "test", Children: []manifest.WorkflowFlowNode{flowEffect("test")}},
						{Kind: "branch", Identity: "build", Children: []manifest.WorkflowFlowNode{flowEffect("build")}},
					},
				},
				flowEffect("audit"),
			},
		},
	}
	flow, err := BuildFlow(declaration)
	if err != nil {
		t.Fatal(err)
	}
	if flow.SchemaVersion != FlowSchemaVersion || len(ValidateFlow(flow)) != 0 {
		t.Fatalf("flow=%+v diagnostics=%v", flow, ValidateFlow(flow))
	}
	if countNodeKind(flow, NodeFork) != 1 || countNodeKind(flow, NodeJoin) != 1 {
		t.Fatalf("expected one explicit fork/join: %+v", flow.Nodes)
	}
	for _, node := range flow.Nodes {
		if node.Kind != NodeEffect {
			continue
		}
		seen := map[MachineEventKind]bool{}
		for _, transition := range flow.Transitions {
			if transition.From == node.Identity {
				seen[transition.Event] = true
			}
		}
		for _, event := range []MachineEventKind{MachineEffectSucceeded, MachineEffectFailed, MachineCancelRequested, MachineTimeout} {
			if !seen[event] {
				t.Fatalf("%s does not explicitly handle %s", node.Identity, event)
			}
		}
	}
}

func TestSimulatorParallelJoinIsIndependentOfCompletionOrder(t *testing.T) {
	flow := parallelFixture(t)
	left := effectIdentity(flow, "test")
	right := effectIdentity(flow, "build")

	run := func(order []string) Snapshot {
		snapshot := NewSnapshot(flow, 100, 100)
		snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineStart})
		for _, identity := range []string{left, right} {
			snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectStarted, Node: identity})
		}
		for _, identity := range order {
			snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectSucceeded, Node: identity})
		}
		return snapshot
	}
	first := run([]string{left, right})
	second := run([]string{right, left})
	if first.Status != StateSucceeded || second.Status != StateSucceeded {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	for identity, state := range first.Nodes {
		if second.Nodes[identity].State != state.State {
			t.Fatalf("completion order changed %s: %s vs %s", identity, state.State, second.Nodes[identity].State)
		}
	}
}

func TestSimulatorFailureHandlingTimeoutCancellationAndTrace(t *testing.T) {
	builder := newFlowBuilder("Handled", nil)
	report := builder.compileEffect(PlanStep{Operation: "process", Name: "Report", Command: []string{"report"}}, builder.flow.Terminals.Succeeded)
	build := builder.compileEffect(PlanStep{Operation: "build", Name: "Build"}, builder.flow.Terminals.Succeeded)
	builder.addTransition(builder.flow.Entry, MachineContinue, build)
	for index := range builder.flow.Transitions {
		transition := &builder.flow.Transitions[index]
		if transition.From == build && transition.Event == MachineEffectFailed {
			transition.To = report
		}
	}
	flow := builder.flow
	if diagnostics := ValidateFlow(flow); len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}

	snapshot := NewSnapshot(flow, 100, 8)
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineStart})
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectStarted, Node: build})
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectFailed, Node: build, Error: "controlled"})
	if snapshot.Status != StateRunning || !containsIdentity(snapshot.Active, report) {
		t.Fatalf("custom failure transition did not activate report: %+v", snapshot)
	}
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectStarted, Node: report})
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectSucceeded, Node: report})
	if snapshot.Status != StateSucceeded {
		t.Fatalf("handled failure status=%s", snapshot.Status)
	}
	if snapshot.Trace[2].SelectedTransition == "" || snapshot.Trace[2].ResultKind != "failed" {
		t.Fatalf("failure trace=%+v", snapshot.Trace[2])
	}

	timed := NewSnapshot(flow, 100, 8)
	timed = mustStep(t, flow, timed, MachineEvent{Kind: MachineStart})
	timed = mustStep(t, flow, timed, MachineEvent{Kind: MachineEffectStarted, Node: build})
	timed = mustStep(t, flow, timed, MachineEvent{Kind: MachineTimeout, Node: build, Error: "deadline"})
	if timed.Status != StateTimedOut {
		t.Fatalf("timeout status=%s", timed.Status)
	}

	cancelled := NewSnapshot(flow, 100, 8)
	cancelled = mustStep(t, flow, cancelled, MachineEvent{Kind: MachineStart})
	cancelled = mustStep(t, flow, cancelled, MachineEvent{Kind: MachineEffectStarted, Node: build})
	cancelled = mustStep(t, flow, cancelled, MachineEvent{Kind: MachineCancelRequested, Node: build})
	if cancelled.Status != StateCancelled {
		t.Fatalf("cancel status=%s", cancelled.Status)
	}
}

func TestMachineStepAndTraceLimitsAreBounded(t *testing.T) {
	flow := parallelFixture(t)
	snapshot := NewSnapshot(flow, 1, 2)
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineStart})
	if _, _, err := Step(flow, snapshot, MachineEvent{Kind: MachineCancelRequested}); err == nil {
		t.Fatal("expected step limit diagnostic")
	}

	snapshot = NewSnapshot(flow, 100, 2)
	snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineStart})
	for _, identity := range append([]string(nil), snapshot.Active...) {
		snapshot = mustStep(t, flow, snapshot, MachineEvent{Kind: MachineEffectStarted, Node: identity})
	}
	if len(snapshot.Trace) != 2 || snapshot.DroppedTrace == 0 {
		t.Fatalf("trace retention=%d dropped=%d", len(snapshot.Trace), snapshot.DroppedTrace)
	}
}

func TestFlowValidatorRejectsUnknownTargetsAndUnboundedCycles(t *testing.T) {
	flow := parallelFixture(t)
	flow.Transitions[0].To = "missing"
	diagnostics := ValidateFlow(flow)
	if !containsDiagnostic(diagnostics, "TSPACK_WORKFLOW_TRANSITION_TARGET_UNKNOWN") {
		t.Fatalf("diagnostics=%v", diagnostics)
	}

	flow = parallelFixture(t)
	for index := range flow.Transitions {
		if flow.Transitions[index].From == effectIdentity(flow, "test") && flow.Transitions[index].Event == MachineEffectSucceeded {
			flow.Transitions[index].To = flow.Entry
		}
	}
	if diagnostics = ValidateFlow(flow); !containsDiagnostic(diagnostics, "TSPACK_WORKFLOW_CYCLE_UNBOUNDED") {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
}

type realParallelOperations struct {
	active int32
	max    int32
}

func (operations *realParallelOperations) Run(ctx context.Context, _ string, _ []string) ([]diag.Diagnostic, error) {
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
	case <-time.After(30 * time.Millisecond):
		atomic.AddInt32(&operations.active, -1)
		return nil, nil
	}
}

func TestRealExecutorConsumesSameParallelMachine(t *testing.T) {
	flow := parallelFixture(t)
	operations := &realParallelOperations{}
	result := (&Executor{Native: operations, Concurrency: 2}).RunFlow(context.Background(), flow)
	if result.State != StateSucceeded || atomic.LoadInt32(&operations.max) != 2 {
		t.Fatalf("result=%+v max=%d", result, operations.max)
	}
	if result.Snapshot == nil || len(result.Snapshot.Trace) == 0 {
		t.Fatal("real executor did not preserve machine trace")
	}
}

type cancellationOperations struct {
	started chan string
}

func (operations cancellationOperations) Run(ctx context.Context, operation string, _ []string) ([]diag.Diagnostic, error) {
	operations.started <- operation
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestRealExecutorCancellationBeforeNativeBuildAndParallelEffects(t *testing.T) {
	buildDeclaration := manifest.Workflow{
		Identity: "BuildCancel",
		Triggers: []manifest.WorkflowTrigger{{Kind: "manual"}},
		Flow:     &manifest.WorkflowFlowNode{Kind: "sequence", Children: []manifest.WorkflowFlowNode{flowEffect("build")}},
	}
	buildFlow, err := BuildFlow(buildDeclaration)
	if err != nil {
		t.Fatal(err)
	}

	preCancelled, cancelBefore := context.WithCancel(context.Background())
	cancelBefore()
	before := (&Executor{Native: noopOperations{}}).RunFlow(preCancelled, buildFlow)
	if before.State != StateCancelled || before.Snapshot == nil {
		t.Fatalf("before-work cancellation=%+v", before)
	}

	started := make(chan string, 2)
	buildContext, cancelBuild := context.WithCancel(context.Background())
	buildDone := make(chan Result, 1)
	go func() {
		buildDone <- (&Executor{Native: cancellationOperations{started: started}}).RunFlow(buildContext, buildFlow)
	}()
	if operation := <-started; operation != "build" {
		t.Fatalf("started %s", operation)
	}
	cancelBuild()
	buildResult := <-buildDone
	if buildResult.State != StateCancelled || buildResult.Snapshot.Nodes[effectIdentity(buildFlow, "build")].State != StateCancelled {
		t.Fatalf("native build cancellation=%+v", buildResult)
	}

	parallel := parallelFixture(t)
	parallelContext, cancelParallel := context.WithCancel(context.Background())
	parallelDone := make(chan Result, 1)
	go func() {
		parallelDone <- (&Executor{Native: cancellationOperations{started: started}, Concurrency: 2}).RunFlow(parallelContext, parallel)
	}()
	<-started
	<-started
	cancelParallel()
	parallelResult := <-parallelDone
	if parallelResult.State != StateCancelled {
		t.Fatalf("parallel cancellation=%+v", parallelResult)
	}
}

func parallelFixture(t testing.TB) Flow {
	t.Helper()
	declaration := manifest.Workflow{
		Identity: "Parallel",
		Triggers: []manifest.WorkflowTrigger{{Kind: "manual"}},
		Flow: &manifest.WorkflowFlowNode{
			Kind: "parallel",
			Children: []manifest.WorkflowFlowNode{
				{Kind: "branch", Identity: "left", Children: []manifest.WorkflowFlowNode{flowEffect("test")}},
				{Kind: "branch", Identity: "right", Children: []manifest.WorkflowFlowNode{flowEffect("build")}},
			},
		},
	}
	flow, err := BuildFlow(declaration)
	if err != nil {
		t.Fatal(err)
	}
	return flow
}

func flowEffect(operation string) manifest.WorkflowFlowNode {
	return manifest.WorkflowFlowNode{Kind: "effect", Effect: &manifest.WorkflowStep{Operation: operation}}
}

func effectIdentity(flow Flow, operation string) string {
	for _, node := range flow.Nodes {
		if node.Effect != nil && node.Effect.Operation == operation {
			return node.Identity
		}
	}
	return ""
}

func countNodeKind(flow Flow, kind NodeKind) int {
	count := 0
	for _, node := range flow.Nodes {
		if node.Kind == kind {
			count++
		}
	}
	return count
}

func mustStep(t *testing.T, flow Flow, snapshot Snapshot, event MachineEvent) Snapshot {
	t.Helper()
	next, _, err := Step(flow, snapshot, event)
	if err != nil {
		t.Fatalf("step %+v: %v", event, err)
	}
	return next
}

func containsDiagnostic(diagnostics []string, code string) bool {
	for _, diagnostic := range diagnostics {
		if len(diagnostic) >= len(code) && diagnostic[:len(code)] == code {
			return true
		}
	}
	return false
}

func BenchmarkBuildPreferredFlow(b *testing.B) {
	declaration := manifest.Workflow{
		Identity: "CI",
		Triggers: []manifest.WorkflowTrigger{{Kind: "manual"}},
		Flow: &manifest.WorkflowFlowNode{
			Kind: "sequence",
			Children: []manifest.WorkflowFlowNode{
				flowEffect("sync"),
				flowEffect("check"),
				{
					Kind: "parallel",
					Children: []manifest.WorkflowFlowNode{
						{Kind: "branch", Identity: "test", Children: []manifest.WorkflowFlowNode{flowEffect("test")}},
						{Kind: "branch", Identity: "build", Children: []manifest.WorkflowFlowNode{flowEffect("build")}},
					},
				},
				flowEffect("audit"),
			},
		},
	}
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := BuildFlow(declaration); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidateFlow(b *testing.B) {
	flow := parallelFixture(b)
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		_ = ValidateFlow(flow)
	}
}

func BenchmarkMachineTransition(b *testing.B) {
	flow := parallelFixture(b)
	snapshot := NewSnapshot(flow, DefaultStepLimit, DefaultTraceLimit)
	snapshot, _, _ = Step(flow, snapshot, MachineEvent{Kind: MachineStart})
	identity := effectIdentity(flow, "test")
	snapshot, _, _ = Step(flow, snapshot, MachineEvent{Kind: MachineEffectStarted, Node: identity})
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		candidate := cloneSnapshot(snapshot)
		if _, _, err := Step(flow, candidate, MachineEvent{Kind: MachineEffectSucceeded, Node: identity}); err != nil {
			b.Fatal(err)
		}
	}
}
