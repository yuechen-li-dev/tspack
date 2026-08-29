package workflow

import (
	"context"
	"errors"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"
)

type completedEffect struct {
	node     FlowNode
	result   *NativeResult
	err      error
	duration int64
}

func (executor *Executor) RunFlow(ctx context.Context, flow Flow) Result {
	if diagnostics := ValidateFlow(flow); len(diagnostics) > 0 {
		return Result{Workflow: flow.Identity, State: StateFailed}
	}
	if executor.Concurrency <= 0 {
		executor.Concurrency = runtime.NumCPU()
		if executor.Concurrency > 4 {
			executor.Concurrency = 4
		}
	}
	if executor.Environment == nil {
		executor.Environment = os.LookupEnv
	}

	executor.emit(Event{Kind: EventWorkflowStarted, Workflow: flow.Identity, State: StateRunning})
	snapshot := NewSnapshot(flow, DefaultStepLimit, DefaultTraceLimit)
	var err error
	snapshot, _, err = executor.applyMachineEvent(flow, snapshot, MachineEvent{Kind: MachineStart})
	if err != nil {
		return executor.machineResult(flow, snapshot, StateFailed)
	}

	completionChannel := make(chan completedEffect, len(flow.Nodes))
	semaphore := make(chan struct{}, executor.Concurrency)
	running := map[string]FlowNode{}
	for snapshot.Status == StateRunning || len(running) > 0 {
		active := activeEffectNodes(flow, snapshot)
		if ctx.Err() != nil && len(running) == 0 && !containsCleanupEffect(active) {
			snapshot, _, _ = executor.applyMachineEvent(flow, snapshot, MachineEvent{Kind: MachineCancelRequested, Error: ctx.Err().Error()})
			break
		}
		if snapshot.Status == StateRunning {
			for _, node := range active {
				if ctx.Err() != nil && !node.Cleanup {
					continue
				}
				snapshot, _, err = executor.applyMachineEvent(flow, snapshot, MachineEvent{Kind: MachineEffectStarted, Node: node.Identity})
				if err != nil {
					snapshot.Status = StateFailed
					break
				}
				running[node.Identity] = node
				executor.emit(Event{Kind: EventStepStarted, Workflow: flow.Identity, Job: effectRegionLabel(node), Step: node.Identity, State: StateRunning})
				if node.Effect.Operation == "transfer" {
					executor.emit(Event{Kind: EventArtifactTransferStarted, Workflow: flow.Identity, Job: effectRegionLabel(node), Step: node.Identity, State: StateRunning})
				}
				go func(effectNode FlowNode, launchSnapshot Snapshot) {
					if effectNode.Cleanup {
						semaphore <- struct{}{}
					} else {
						select {
						case semaphore <- struct{}{}:
						case <-ctx.Done():
							completionChannel <- completedEffect{node: effectNode, err: ctx.Err()}
							return
						}
					}
					defer func() { <-semaphore }()
					completionChannel <- executor.runOneFlowEffect(ctx, flow, effectNode, launchSnapshot)
				}(node, snapshot)
			}
		}
		if len(running) == 0 {
			if snapshot.Status == StateRunning {
				snapshot.Status = StateFailed
			}
			break
		}
		completed := []completedEffect{<-completionChannel}
		for {
			select {
			case completion := <-completionChannel:
				completed = append(completed, completion)
			default:
				sort.Slice(completed, func(left, right int) bool {
					return completed[left].node.Identity < completed[right].node.Identity
				})
				goto applyCompletions
			}
		}
	applyCompletions:
		for _, completion := range completed {
			delete(running, completion.node.Identity)
			eventKind := MachineEffectSucceeded
			state := StateSucceeded
			if completion.err != nil {
				if ctx.Err() != nil && !completion.node.Cleanup {
					eventKind = MachineCancelRequested
					state = StateCancelled
				} else if errors.Is(completion.err, context.DeadlineExceeded) {
					eventKind = MachineTimeout
					state = StateTimedOut
				} else if errors.Is(completion.err, context.Canceled) {
					eventKind = MachineCancelRequested
					state = StateCancelled
				} else {
					eventKind = MachineEffectFailed
					state = StateFailed
				}
			}
			if eventKind == MachineCancelRequested && snapshot.Status == StateRunning {
				snapshot, _, _ = executor.applyMachineEvent(flow, snapshot, MachineEvent{Kind: MachineCancelRequested, Node: completion.node.Identity, Result: completion.result, Error: completion.err.Error()})
			} else if snapshot.Nodes[completion.node.Identity].State == StateRunning {
				machineEvent := MachineEvent{Kind: eventKind, Node: completion.node.Identity, Result: completion.result}
				if completion.err != nil {
					machineEvent.Error = completion.err.Error()
				}
				snapshot, _, err = executor.applyMachineEvent(flow, snapshot, machineEvent)
				if err != nil {
					snapshot.Status = StateFailed
				}
			}
			executor.emit(Event{
				Kind:     EventStepCompleted,
				Workflow: flow.Identity,
				Job:      effectRegionLabel(completion.node),
				Step:     completion.node.Identity,
				State:    state,
				Duration: completion.duration,
				Result:   completion.result,
			})
			if completion.node.Effect.Operation == "transfer" {
				kind := EventArtifactTransferCompleted
				if completion.err != nil {
					kind = EventArtifactTransferFailed
				}
				executor.emit(Event{Kind: kind, Workflow: flow.Identity, Job: effectRegionLabel(completion.node), Step: completion.node.Identity, State: state, Duration: completion.duration, Result: completion.result})
			}
		}
	}

	result := executor.machineResult(flow, snapshot, snapshot.Status)
	executor.emit(Event{Kind: EventWorkflowCompleted, Workflow: flow.Identity, State: result.State})
	return result
}

func (executor *Executor) runOneFlowEffect(ctx context.Context, flow Flow, effectNode FlowNode, snapshot Snapshot) completedEffect {
	effectStep := *effectNode.Effect
	for _, input := range effectNode.Effect.Inputs {
		value, exists := snapshot.Values[input.Source]
		if !exists {
			return completedEffect{node: effectNode, err: errors.New("TSPACK_WORKFLOW_VALUE_UNAVAILABLE: " + input.Identity + " is not available to " + effectNode.Identity)}
		}
		valueCopy := value
		effectStep.ResolvedInputs = append(effectStep.ResolvedInputs, ResolvedValue{Reference: input, Result: &valueCopy})
	}
	effectContext := ctx
	cancelCleanup := func() {}
	if effectNode.Cleanup {
		effectContext, cancelCleanup = context.WithTimeout(context.WithoutCancel(ctx), DefaultCleanupTimeout)
	}
	defer cancelCleanup()
	started := time.Now()
	region := findRegion(flow, effectNode.Region)
	job := PlanJob{
		Identity:    effectRegionLabel(effectNode),
		Platform:    region.Platform,
		Environment: region.Environment,
	}
	if !platformCompatible(job.Platform) {
		return completedEffect{node: effectNode, err: errors.New("TSPACK_WORKFLOW_PLATFORM_UNAVAILABLE: region requires " + job.Platform)}
	}
	result, err := executor.runStep(effectContext, flow.Identity, job, effectStep)
	return completedEffect{node: effectNode, result: result, err: err, duration: time.Since(started).Milliseconds()}
}

func (executor *Executor) applyMachineEvent(flow Flow, snapshot Snapshot, event MachineEvent) (Snapshot, StepTrace, error) {
	next, trace, err := Step(flow, snapshot, event)
	if err == nil {
		executor.emit(Event{Kind: EventMachineStepped, Workflow: flow.Identity, Trace: &trace})
	}
	return next, trace, err
}

func (executor *Executor) runEffectWave(ctx context.Context, flow Flow, nodes []FlowNode, snapshot Snapshot) []completedEffect {
	completed := make(chan completedEffect, len(nodes))
	semaphore := make(chan struct{}, executor.Concurrency)
	var wait sync.WaitGroup
	for _, node := range nodes {
		wait.Add(1)
		go func(effectNode FlowNode) {
			defer wait.Done()
			effectStep := *effectNode.Effect
			for _, input := range effectNode.Effect.Inputs {
				value, exists := snapshot.Values[input.Source]
				if !exists {
					completed <- completedEffect{node: effectNode, err: errors.New("TSPACK_WORKFLOW_VALUE_UNAVAILABLE: " + input.Identity + " is not available to " + effectNode.Identity)}
					return
				}
				valueCopy := value
				effectStep.ResolvedInputs = append(effectStep.ResolvedInputs, ResolvedValue{Reference: input, Result: &valueCopy})
			}
			effectContext := ctx
			cancelCleanup := func() {}
			if effectNode.Cleanup {
				effectContext, cancelCleanup = context.WithTimeout(context.WithoutCancel(ctx), DefaultCleanupTimeout)
			}
			defer cancelCleanup()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-effectContext.Done():
				completed <- completedEffect{node: effectNode, err: effectContext.Err()}
				return
			}
			started := time.Now()
			region := findRegion(flow, effectNode.Region)
			job := PlanJob{
				Identity:    effectRegionLabel(effectNode),
				Platform:    region.Platform,
				Environment: region.Environment,
			}
			if !platformCompatible(job.Platform) {
				completed <- completedEffect{node: effectNode, err: errors.New("TSPACK_WORKFLOW_PLATFORM_UNAVAILABLE: region requires " + job.Platform)}
				return
			}
			result, err := executor.runStep(effectContext, flow.Identity, job, effectStep)
			completed <- completedEffect{node: effectNode, result: result, err: err, duration: time.Since(started).Milliseconds()}
		}(node)
	}
	wait.Wait()
	close(completed)
	results := make([]completedEffect, 0, len(nodes))
	for result := range completed {
		results = append(results, result)
	}
	sort.Slice(results, func(left, right int) bool {
		return results[left].node.Identity < results[right].node.Identity
	})
	return results
}

func containsCleanupEffect(nodes []FlowNode) bool {
	for _, node := range nodes {
		if node.Cleanup {
			return true
		}
	}
	return false
}

func activeEffectNodes(flow Flow, snapshot Snapshot) []FlowNode {
	active := []FlowNode{}
	for _, identity := range snapshot.Active {
		node, exists := flowNode(flow, identity)
		if exists && node.Kind == NodeEffect && snapshot.Nodes[identity].State == StatePending {
			active = append(active, node)
		}
	}
	sort.Slice(active, func(left, right int) bool {
		return active[left].Identity < active[right].Identity
	})
	return active
}

func findRegion(flow Flow, identity string) ExecutionRegion {
	for _, region := range flow.Regions {
		if region.Identity == identity {
			return region
		}
	}
	return ExecutionRegion{Identity: "region/default", Platform: "currentHost"}
}

func effectRegionLabel(node FlowNode) string {
	if node.LegacyJob != "" {
		return node.LegacyJob
	}
	if node.Branch != "" {
		return node.Branch
	}
	if node.Region != "" {
		return node.Region
	}
	return "flow"
}

func (executor *Executor) machineResult(flow Flow, snapshot Snapshot, state State) Result {
	result := Result{Workflow: flow.Identity, State: state, Snapshot: &snapshot}
	type groupedJob struct {
		identity string
		rank     int
		steps    []rankedStep
	}
	type jobKey struct {
		identity string
		rank     int
	}
	groups := map[jobKey][]rankedStep{}
	for _, node := range flow.Nodes {
		if node.Kind != NodeEffect {
			continue
		}
		nodeState := snapshot.Nodes[node.Identity]
		step := StepResult{Identity: node.Identity, State: nodeState.State, Result: nodeState.Result}
		if nodeState.Error != "" {
			step.Error = nodeState.Error
		}
		result.Effects = append(result.Effects, step)
		key := jobKey{identity: effectRegionLabel(node), rank: node.LegacyRank}
		groups[key] = append(groups[key], rankedStep{rank: node.EffectRank, step: step})
	}
	jobs := []groupedJob{}
	for key, steps := range groups {
		jobs = append(jobs, groupedJob{identity: key.identity, rank: key.rank, steps: steps})
	}
	sort.Slice(jobs, func(left, right int) bool {
		if jobs[left].rank != jobs[right].rank {
			return jobs[left].rank < jobs[right].rank
		}
		return jobs[left].identity < jobs[right].identity
	})
	for _, group := range jobs {
		sort.Slice(group.steps, func(left, right int) bool {
			return group.steps[left].rank < group.steps[right].rank
		})
		job := JobResult{Identity: group.identity, State: StateSucceeded}
		for _, ranked := range group.steps {
			job.Steps = append(job.Steps, ranked.step)
			if ranked.step.State == StateFailed || ranked.step.State == StateTimedOut {
				job.State = StateFailed
			} else if ranked.step.State == StateBlocked && job.State == StateSucceeded {
				job.State = StateBlocked
			} else if ranked.step.State == StateCancelled && job.State == StateSucceeded {
				job.State = StateCancelled
			}
		}
		result.Jobs = append(result.Jobs, job)
	}
	return result
}

type rankedStep struct {
	rank int
	step StepResult
}
