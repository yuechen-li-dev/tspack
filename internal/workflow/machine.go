package workflow

import (
	"fmt"
	"sort"
)

func NewSnapshot(flow Flow, stepLimit int, traceLimit int) Snapshot {
	if stepLimit <= 0 {
		stepLimit = DefaultStepLimit
	}
	if traceLimit <= 0 {
		traceLimit = DefaultTraceLimit
	}
	nodes := make(map[string]NodeSnapshot, len(flow.Nodes))
	for _, node := range flow.Nodes {
		nodes[node.Identity] = NodeSnapshot{State: StatePending}
	}
	return Snapshot{
		Status:     StatePending,
		Nodes:      nodes,
		Values:     map[string]NativeResult{},
		Outcomes:   map[string]TerminalKind{},
		Iterators:  map[string]IteratorCursor{},
		StepLimit:  stepLimit,
		TraceLimit: traceLimit,
	}
}

func Step(flow Flow, current Snapshot, event MachineEvent) (Snapshot, StepTrace, error) {
	if current.StepLimit <= 0 {
		current.StepLimit = DefaultStepLimit
	}
	if current.TraceLimit <= 0 {
		current.TraceLimit = DefaultTraceLimit
	}
	if current.StepCount >= current.StepLimit {
		return current, StepTrace{}, fmt.Errorf("TSPACK_WORKFLOW_STEP_LIMIT_EXCEEDED: machine exceeded %d steps", current.StepLimit)
	}

	next := cloneSnapshot(current)
	trace := StepTrace{
		Step:        next.StepCount + 1,
		StateBefore: append([]string(nil), current.Active...),
		Event:       event,
	}
	next.StepCount++

	switch event.Kind {
	case MachineStart:
		if next.Status != StatePending {
			return current, StepTrace{}, fmt.Errorf("TSPACK_WORKFLOW_EVENT_INVALID: start requires a pending machine")
		}
		next.Status = StateRunning
		activateNode(flow, &next, flow.Entry)
	case MachineEffectStarted:
		node, snapshot, err := requireEffectState(flow, &next, event.Node, StateRunning, StatePending)
		if err != nil {
			return current, StepTrace{}, err
		}
		if snapshot.State != StatePending {
			return current, StepTrace{}, fmt.Errorf("TSPACK_WORKFLOW_EVENT_INVALID: %s is not ready to start", event.Node)
		}
		snapshot.State = StateRunning
		next.Nodes[event.Node] = *snapshot
		trace.Effect = node.Effect.Operation
	case MachineEffectSucceeded, MachineEffectFailed, MachineTimeout, MachineCancelRequested:
		if event.Kind == MachineCancelRequested && event.Node == "" {
			for identity, nodeSnapshot := range next.Nodes {
				node, exists := flowNode(flow, identity)
				if !exists || node.Kind == NodeTerminal {
					continue
				}
				if nodeSnapshot.State == StateRunning || nodeSnapshot.State == StatePending || nodeSnapshot.State == StateWaiting {
					nodeSnapshot.State = StateCancelled
					next.Nodes[identity] = nodeSnapshot
				}
			}
			next.Active = nil
			activateNode(flow, &next, flow.Terminals.Cancelled)
			break
		}
		node, snapshot, err := requireEffectState(flow, &next, event.Node, StateRunning)
		if err != nil {
			return current, StepTrace{}, err
		}
		trace.Effect = node.Effect.Operation
		switch event.Kind {
		case MachineEffectSucceeded:
			snapshot.State = StateSucceeded
			trace.ResultKind = "succeeded"
		case MachineEffectFailed:
			snapshot.State = StateFailed
			snapshot.Error = event.Error
			trace.ResultKind = "failed"
		case MachineTimeout:
			snapshot.State = StateTimedOut
			snapshot.Error = event.Error
			trace.ResultKind = "timedOut"
		case MachineCancelRequested:
			snapshot.State = StateCancelled
			snapshot.Error = event.Error
			trace.ResultKind = "cancelled"
		}
		if event.Result != nil {
			resultCopy := *event.Result
			snapshot.Result = &resultCopy
			if node.Value != "" {
				next.Values[node.Value] = resultCopy
			}
		}
		if node.Value != "" {
			next.Outcomes[node.Value] = TerminalKind(trace.ResultKind)
			trace.ValueIdentity = node.Value
		}
		if node.Cleanup && event.Kind != MachineEffectSucceeded {
			next.CleanupFailures = append(next.CleanupFailures, CleanupFailure{Cause: node.CleanupCause, Node: node.Identity, Error: event.Error})
		}
		next.Nodes[event.Node] = *snapshot
		removeActive(&next, event.Node)
		transition, eligible, err := selectTransition(flow, event.Node, event.Kind)
		trace.EligibleTransitions = eligible
		if err != nil {
			return current, StepTrace{}, err
		}
		trace.SelectedTransition = transition.Identity
		activateNode(flow, &next, transition.To)
	default:
		return current, StepTrace{}, fmt.Errorf("TSPACK_WORKFLOW_EVENT_INVALID: unsupported event %s", event.Kind)
	}

	if err := settleControl(flow, &next); err != nil {
		return current, StepTrace{}, err
	}
	trace.StateAfter = append([]string(nil), next.Active...)
	appendTrace(&next, trace)
	return next, trace, nil
}

func requireEffectState(flow Flow, snapshot *Snapshot, identity string, allowed ...State) (FlowNode, *NodeSnapshot, error) {
	node, exists := flowNode(flow, identity)
	if !exists || node.Kind != NodeEffect {
		return FlowNode{}, nil, fmt.Errorf("TSPACK_WORKFLOW_EFFECT_UNKNOWN: %s is not an effect node", identity)
	}
	state, exists := snapshot.Nodes[identity]
	if !exists {
		return FlowNode{}, nil, fmt.Errorf("TSPACK_WORKFLOW_STATE_UNKNOWN: snapshot has no node %s", identity)
	}
	for _, candidate := range allowed {
		if state.State == candidate {
			copy := state
			return node, &copy, nil
		}
	}
	return FlowNode{}, nil, fmt.Errorf("TSPACK_WORKFLOW_EVENT_INVALID: %s is %s", identity, state.State)
}

func selectTransition(flow Flow, from string, event MachineEventKind) (Transition, []string, error) {
	eligible := []string{}
	var selected Transition
	for _, transition := range flow.Transitions {
		if transition.From != from || transition.Event != event {
			continue
		}
		eligible = append(eligible, transition.Identity)
		if selected.Identity == "" {
			selected = transition
		}
	}
	if len(eligible) == 0 {
		return Transition{}, nil, fmt.Errorf("TSPACK_WORKFLOW_RESULT_UNHANDLED: %s has no %s transition", from, event)
	}
	if len(eligible) > 1 {
		return Transition{}, eligible, fmt.Errorf("TSPACK_WORKFLOW_TRANSITION_AMBIGUOUS: %s has multiple %s transitions", from, event)
	}
	return selected, eligible, nil
}

func settleControl(flow Flow, snapshot *Snapshot) error {
	for {
		progress := false
		for _, node := range flow.Nodes {
			state := snapshot.Nodes[node.Identity]
			if state.State == StateWaiting && dependenciesComplete(snapshot, node.WaitFor) {
				state.State = StatePending
				snapshot.Nodes[node.Identity] = state
				addActive(snapshot, node.Identity)
				progress = true
			}
			if state.State != StatePending || !containsIdentity(snapshot.Active, node.Identity) || node.Kind == NodeEffect {
				continue
			}

			removeActive(snapshot, node.Identity)
			state.State = StateSucceeded
			snapshot.Nodes[node.Identity] = state
			progress = true
			switch node.Kind {
			case NodeEntry, NodeJoin, NodeIterator:
				if node.Kind == NodeIterator && node.Iterator != nil {
					snapshot.Iterators[node.Identity] = *node.Iterator
				}
				transition, _, err := selectTransition(flow, node.Identity, MachineContinue)
				if err != nil {
					return err
				}
				activateNode(flow, snapshot, transition.To)
			case NodeMatch:
				outcome, exists := snapshot.Outcomes[node.Source]
				if !exists {
					return fmt.Errorf("TSPACK_WORKFLOW_VALUE_UNKNOWN: MatchResult source %s is unavailable", node.Source)
				}
				transition, _, err := selectGuardedTransition(flow, node.Identity, MachineContinue, string(outcome))
				if err != nil {
					return err
				}
				activateNode(flow, snapshot, transition.To)
			case NodeFork:
				for _, target := range node.Targets {
					activateNode(flow, snapshot, target)
				}
			case NodeBranchExit:
				// Joins observe this explicit completion node through WaitFor.
			case NodeTerminal:
				snapshot.Active = nil
				switch node.Terminal {
				case TerminalSucceeded:
					snapshot.Status = StateSucceeded
				case TerminalFailed:
					snapshot.Status = StateFailed
				case TerminalCancelled:
					snapshot.Status = StateCancelled
				case TerminalTimedOut:
					snapshot.Status = StateTimedOut
				}
				cancelNonterminalNodes(flow, snapshot)
			}
		}
		if !progress {
			break
		}
	}
	sort.Strings(snapshot.Active)
	return nil
}

func activateNode(flow Flow, snapshot *Snapshot, identity string) {
	node, exists := flowNode(flow, identity)
	if !exists {
		return
	}
	state := snapshot.Nodes[identity]
	if state.State != StatePending {
		return
	}
	if len(node.WaitFor) > 0 && !dependenciesComplete(snapshot, node.WaitFor) {
		state.State = StateWaiting
		snapshot.Nodes[identity] = state
		return
	}
	addActive(snapshot, identity)
}

func dependenciesComplete(snapshot *Snapshot, dependencies []string) bool {
	for _, dependency := range dependencies {
		if snapshot.Nodes[dependency].State != StateSucceeded {
			return false
		}
	}
	return true
}

func cancelNonterminalNodes(flow Flow, snapshot *Snapshot) {
	if snapshot.Status == StateSucceeded {
		return
	}
	for _, node := range flow.Nodes {
		if node.Kind == NodeTerminal {
			continue
		}
		state := snapshot.Nodes[node.Identity]
		if snapshot.Status == StateCancelled {
			if state.State == StatePending || state.State == StateWaiting || state.State == StateRunning {
				state.State = StateCancelled
				snapshot.Nodes[node.Identity] = state
			}
			continue
		}
		if state.State == StatePending || state.State == StateWaiting {
			state.State = StateBlocked
			snapshot.Nodes[node.Identity] = state
		}
	}
}

func cloneSnapshot(source Snapshot) Snapshot {
	clone := source
	clone.Active = append([]string(nil), source.Active...)
	clone.Nodes = make(map[string]NodeSnapshot, len(source.Nodes))
	for identity, state := range source.Nodes {
		clone.Nodes[identity] = state
	}
	clone.Values = make(map[string]NativeResult, len(source.Values))
	for identity, value := range source.Values {
		clone.Values[identity] = value
	}
	clone.Outcomes = make(map[string]TerminalKind, len(source.Outcomes))
	for identity, outcome := range source.Outcomes {
		clone.Outcomes[identity] = outcome
	}
	clone.Iterators = make(map[string]IteratorCursor, len(source.Iterators))
	for identity, cursor := range source.Iterators {
		clone.Iterators[identity] = cursor
	}
	clone.CleanupFailures = append([]CleanupFailure(nil), source.CleanupFailures...)
	clone.Trace = append([]StepTrace(nil), source.Trace...)
	return clone
}

func selectGuardedTransition(flow Flow, from string, event MachineEventKind, guard string) (Transition, []string, error) {
	eligible := []string{}
	var selected Transition
	for _, transition := range flow.Transitions {
		if transition.From != from || transition.Event != event || transition.Guard != guard {
			continue
		}
		eligible = append(eligible, transition.Identity)
		if selected.Identity == "" {
			selected = transition
		}
	}
	if len(eligible) == 0 {
		return Transition{}, nil, fmt.Errorf("TSPACK_WORKFLOW_MATCH_UNHANDLED: %s has no %s arm", from, guard)
	}
	if len(eligible) > 1 {
		return Transition{}, eligible, fmt.Errorf("TSPACK_WORKFLOW_TRANSITION_AMBIGUOUS: %s has multiple %s arms", from, guard)
	}
	return selected, eligible, nil
}

func appendTrace(snapshot *Snapshot, trace StepTrace) {
	if len(snapshot.Trace) == snapshot.TraceLimit {
		copy(snapshot.Trace, snapshot.Trace[1:])
		snapshot.Trace[len(snapshot.Trace)-1] = trace
		snapshot.DroppedTrace++
		return
	}
	snapshot.Trace = append(snapshot.Trace, trace)
}

func flowNode(flow Flow, identity string) (FlowNode, bool) {
	for _, node := range flow.Nodes {
		if node.Identity == identity {
			return node, true
		}
	}
	return FlowNode{}, false
}

func addActive(snapshot *Snapshot, identity string) {
	if !containsIdentity(snapshot.Active, identity) {
		snapshot.Active = append(snapshot.Active, identity)
	}
}

func removeActive(snapshot *Snapshot, identity string) {
	for index, active := range snapshot.Active {
		if active == identity {
			snapshot.Active = append(snapshot.Active[:index], snapshot.Active[index+1:]...)
			return
		}
	}
}

func containsIdentity(values []string, identity string) bool {
	for _, value := range values {
		if value == identity {
			return true
		}
	}
	return false
}
