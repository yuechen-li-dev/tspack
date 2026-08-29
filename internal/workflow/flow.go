package workflow

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

const (
	FlowSchemaVersion          = 3
	DefaultStepLimit           = 10_000
	DefaultTraceLimit          = 256
	DefaultForEachLimit        = 256
	DefaultForEachConcurrency  = 4
	MaxForEachConcurrency      = 32
	DefaultPredicateDepthLimit = 8
	DefaultFinallyNestingLimit = 8
	DefaultCleanupTimeout      = 30 * time.Second
)

type NodeKind string

const (
	NodeEntry      NodeKind = "entry"
	NodeEffect     NodeKind = "effect"
	NodeFork       NodeKind = "fork"
	NodeJoin       NodeKind = "join"
	NodeBranchExit NodeKind = "branchExit"
	NodeMatch      NodeKind = "match"
	NodeIterator   NodeKind = "iterator"
	NodePredicate  NodeKind = "predicate"
	NodeTerminal   NodeKind = "terminal"
)

type MachineEventKind string

const (
	MachineStart           MachineEventKind = "start"
	MachineContinue        MachineEventKind = "continue"
	MachineEffectStarted   MachineEventKind = "effectStarted"
	MachineEffectSucceeded MachineEventKind = "effectSucceeded"
	MachineEffectFailed    MachineEventKind = "effectFailed"
	MachineCancelRequested MachineEventKind = "cancelRequested"
	MachineTimeout         MachineEventKind = "timeout"
)

type TerminalKind string

const (
	TerminalSucceeded TerminalKind = "succeeded"
	TerminalFailed    TerminalKind = "failed"
	TerminalCancelled TerminalKind = "cancelled"
	TerminalTimedOut  TerminalKind = "timedOut"
)

type Flow struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Identity      string                `json:"identity"`
	Triggers      []Trigger             `json:"triggers"`
	Entry         string                `json:"entry"`
	Terminals     FlowTerminals         `json:"terminals"`
	Regions       []ExecutionRegion     `json:"regions"`
	Values        []ValueDefinition     `json:"values,omitempty"`
	Aggregates    []AggregateDefinition `json:"aggregates,omitempty"`
	Nodes         []FlowNode            `json:"nodes"`
	Transitions   []Transition          `json:"transitions"`
}

type AggregateDefinition struct {
	Identity      string   `json:"identity"`
	ResultType    string   `json:"resultType"`
	Elements      []string `json:"elements"`
	Mode          string   `json:"mode"`
	Concurrency   int      `json:"concurrency"`
	FailurePolicy string   `json:"failurePolicy"`
}

type AggregateValue struct {
	Identity string             `json:"identity"`
	Elements []AggregateElement `json:"elements"`
}

type AggregateElement struct {
	Index   int          `json:"index"`
	Value   string       `json:"value"`
	Outcome TerminalKind `json:"outcome,omitempty"`
}

type Predicate struct {
	Kind     string      `json:"kind"`
	Input    *ValueRef   `json:"input,omitempty"`
	Number   *float64    `json:"number,omitempty"`
	Boolean  *bool       `json:"boolean,omitempty"`
	String   string      `json:"string,omitempty"`
	Children []Predicate `json:"children,omitempty"`
}

type ValueCategory string

const (
	ValueControl           ValueCategory = "control"
	ValueSmallSerialized   ValueCategory = "smallSerialized"
	ValueArtifactReference ValueCategory = "artifactReference"
	ValueRegionLocal       ValueCategory = "regionLocal"
	ValuePlacement         ValueCategory = "placement"
)

type ValueRef struct {
	Identity   string        `json:"identity"`
	Source     string        `json:"source"`
	ResultType string        `json:"resultType"`
	FieldPath  []string      `json:"fieldPath,omitempty"`
	Category   ValueCategory `json:"category"`
}

type ValueDefinition struct {
	ValueRef
	Producer    string `json:"producer,omitempty"`
	Region      string `json:"region,omitempty"`
	Transported bool   `json:"transported,omitempty"`
}

type IteratorCursor struct {
	SourceIdentity string         `json:"sourceIdentity"`
	Index          int            `json:"index"`
	Count          int            `json:"count"`
	Binding        string         `json:"binding"`
	Value          IterationValue `json:"value"`
	Mode           string         `json:"mode"`
	FailurePolicy  string         `json:"failurePolicy"`
	Concurrency    int            `json:"concurrency"`
	Aggregate      string         `json:"aggregate,omitempty"`
}

type IterationValue struct {
	Kind    string   `json:"kind"`
	String  string   `json:"string,omitempty"`
	Number  *float64 `json:"number,omitempty"`
	Boolean *bool    `json:"boolean,omitempty"`
}

type FlowTerminals struct {
	Succeeded string `json:"succeeded"`
	Failed    string `json:"failed"`
	Cancelled string `json:"cancelled"`
	TimedOut  string `json:"timedOut"`
}

type ExecutionRegion struct {
	Identity    string        `json:"identity"`
	Platform    string        `json:"platform"`
	Environment []Environment `json:"environment,omitempty"`
}

type FlowNode struct {
	Identity     string          `json:"identity"`
	Kind         NodeKind        `json:"kind"`
	Region       string          `json:"region,omitempty"`
	Branch       string          `json:"branch,omitempty"`
	Effect       *PlanStep       `json:"effect,omitempty"`
	Value        string          `json:"value,omitempty"`
	Source       string          `json:"source,omitempty"`
	Iterator     *IteratorCursor `json:"iterator,omitempty"`
	Predicate    *Predicate      `json:"predicate,omitempty"`
	Cleanup      bool            `json:"cleanup,omitempty"`
	CleanupCause TerminalKind    `json:"cleanupCause,omitempty"`
	Targets      []string        `json:"targets,omitempty"`
	WaitFor      []string        `json:"waitFor,omitempty"`
	Terminal     TerminalKind    `json:"terminal,omitempty"`
	LegacyJob    string          `json:"-"`
	LegacyRank   int             `json:"-"`
	EffectRank   int             `json:"-"`
}

type Transition struct {
	Identity string           `json:"identity"`
	From     string           `json:"from"`
	Event    MachineEventKind `json:"event"`
	Guard    string           `json:"guard,omitempty"`
	To       string           `json:"to"`
}

type MachineEvent struct {
	Kind   MachineEventKind `json:"kind"`
	Node   string           `json:"node,omitempty"`
	Result *NativeResult    `json:"result,omitempty"`
	Error  string           `json:"error,omitempty"`
}

type NodeSnapshot struct {
	State  State         `json:"state"`
	Result *NativeResult `json:"result,omitempty"`
	Error  string        `json:"error,omitempty"`
}

type Snapshot struct {
	Status          State                     `json:"status"`
	Nodes           map[string]NodeSnapshot   `json:"nodes"`
	Active          []string                  `json:"active,omitempty"`
	Values          map[string]NativeResult   `json:"values,omitempty"`
	Outcomes        map[string]TerminalKind   `json:"outcomes,omitempty"`
	Iterators       map[string]IteratorCursor `json:"iterators,omitempty"`
	Aggregates      map[string]AggregateValue `json:"aggregates,omitempty"`
	CleanupFailures []CleanupFailure          `json:"cleanupFailures,omitempty"`
	StepCount       int                       `json:"stepCount"`
	StepLimit       int                       `json:"stepLimit"`
	Trace           []StepTrace               `json:"trace,omitempty"`
	TraceLimit      int                       `json:"traceLimit"`
	DroppedTrace    int                       `json:"droppedTrace"`
}

type CleanupFailure struct {
	Cause TerminalKind `json:"cause"`
	Node  string       `json:"node"`
	Error string       `json:"error"`
}

type StepTrace struct {
	Step                int          `json:"step"`
	StateBefore         []string     `json:"stateBefore"`
	Event               MachineEvent `json:"event"`
	EligibleTransitions []string     `json:"eligibleTransitions,omitempty"`
	SelectedTransition  string       `json:"selectedTransition,omitempty"`
	Effect              string       `json:"effect,omitempty"`
	ResultKind          string       `json:"resultKind,omitempty"`
	ValueIdentity       string       `json:"valueIdentity,omitempty"`
	StateAfter          []string     `json:"stateAfter"`
}

type flowBuilder struct {
	flow            Flow
	nextIdentity    int
	currentRegion   string
	currentBranch   string
	failureTarget   string
	cancelTarget    string
	timeoutTarget   string
	cleanupCause    TerminalKind
	finallyDepth    int
	forEachDepth    int
	effects         map[string]string
	authoredEffects map[string]int
	matchTargets    map[string]string
}

func BuildFlow(declaration manifest.Workflow) (Flow, error) {
	if declaration.Flow == nil {
		return BuildFlowFromPlan(BuildPlan(declaration))
	}

	builder := newFlowBuilder(declaration.Identity, declaration.Triggers)
	builder.authoredEffects = collectAuthoredEffects(*declaration.Flow)
	rootEntry, err := builder.compileAuthoringNode(*declaration.Flow, builder.flow.Terminals.Succeeded)
	if err != nil {
		return Flow{}, err
	}
	builder.addTransition(builder.flow.Entry, MachineContinue, rootEntry)
	if diagnostics := ValidateFlow(builder.flow); len(diagnostics) > 0 {
		return Flow{}, fmt.Errorf("%s", strings.Join(diagnostics, "; "))
	}
	return builder.flow, nil
}

func BuildFlowFromPlan(plan Plan) (Flow, error) {
	builder := newFlowBuilder(plan.Workflow, nil)
	builder.flow.Triggers = append([]Trigger(nil), plan.Triggers...)

	exitByJob := map[string]string{}
	for _, job := range plan.Jobs {
		exitByJob[job.Identity] = builder.addNode("job-"+job.Identity+"-exit", NodeBranchExit)
	}

	join := builder.addNode("legacy-join", NodeJoin)
	fork := builder.addNode("legacy-fork", NodeFork)
	builder.addTransition(builder.flow.Entry, MachineContinue, fork)
	builder.addTransition(join, MachineContinue, builder.flow.Terminals.Succeeded)

	for jobIndex, job := range plan.Jobs {
		region := "region/" + job.Identity
		builder.addRegion(region, job.Platform, job.Environment)
		previousRegion := builder.currentRegion
		previousBranch := builder.currentBranch
		builder.currentRegion = region
		builder.currentBranch = job.Identity

		continuation := exitByJob[job.Identity]
		entry := continuation
		for stepIndex := len(job.Steps) - 1; stepIndex >= 0; stepIndex-- {
			entry = builder.compileEffect(job.Steps[stepIndex], continuation)
			continuation = entry
			node := builder.node(entry)
			node.LegacyJob = job.Identity
			node.LegacyRank = jobIndex
			node.EffectRank = stepIndex
		}
		waitFor := make([]string, 0, len(job.Needs))
		for _, dependency := range job.Needs {
			waitFor = append(waitFor, exitByJob[dependency])
		}
		sort.Strings(waitFor)
		builder.node(entry).WaitFor = waitFor
		builder.node(exitByJob[job.Identity]).LegacyJob = job.Identity
		builder.node(exitByJob[job.Identity]).LegacyRank = jobIndex
		builder.node(fork).Targets = append(builder.node(fork).Targets, entry)
		builder.node(join).WaitFor = append(builder.node(join).WaitFor, exitByJob[job.Identity])
		builder.currentRegion = previousRegion
		builder.currentBranch = previousBranch
	}
	builder.node(fork).Targets = append(builder.node(fork).Targets, join)
	sort.Strings(builder.node(fork).Targets)
	sort.Strings(builder.node(join).WaitFor)
	if diagnostics := ValidateFlow(builder.flow); len(diagnostics) > 0 {
		return Flow{}, fmt.Errorf("%s", strings.Join(diagnostics, "; "))
	}
	return builder.flow, nil
}

func newFlowBuilder(identity string, triggers []manifest.WorkflowTrigger) *flowBuilder {
	builder := &flowBuilder{
		flow: Flow{
			SchemaVersion: FlowSchemaVersion,
			Identity:      identity,
			Entry:         "entry",
			Terminals: FlowTerminals{
				Succeeded: "terminal/succeeded",
				Failed:    "terminal/failed",
				Cancelled: "terminal/cancelled",
				TimedOut:  "terminal/timedOut",
			},
		},
		currentRegion:   "region/default",
		effects:         map[string]string{},
		authoredEffects: map[string]int{},
		matchTargets:    map[string]string{},
	}
	builder.failureTarget = builder.flow.Terminals.Failed
	builder.cancelTarget = builder.flow.Terminals.Cancelled
	builder.timeoutTarget = builder.flow.Terminals.TimedOut
	for _, trigger := range triggers {
		builder.flow.Triggers = append(builder.flow.Triggers, Trigger{
			Kind:     trigger.Kind,
			Branches: sortedCopy(trigger.Branches),
			Paths:    sortedCopy(trigger.Paths),
		})
	}
	builder.addRegion("region/default", "currentHost", nil)
	builder.flow.Nodes = append(builder.flow.Nodes,
		FlowNode{Identity: builder.flow.Entry, Kind: NodeEntry},
		FlowNode{Identity: builder.flow.Terminals.Succeeded, Kind: NodeTerminal, Terminal: TerminalSucceeded},
		FlowNode{Identity: builder.flow.Terminals.Failed, Kind: NodeTerminal, Terminal: TerminalFailed},
		FlowNode{Identity: builder.flow.Terminals.Cancelled, Kind: NodeTerminal, Terminal: TerminalCancelled},
		FlowNode{Identity: builder.flow.Terminals.TimedOut, Kind: NodeTerminal, Terminal: TerminalTimedOut},
	)
	return builder
}

func (builder *flowBuilder) compileAuthoringNode(node manifest.WorkflowFlowNode, continuation string) (string, error) {
	switch node.Kind {
	case "effect":
		if node.Effect == nil {
			return "", errorsForFlowNode(node.Kind, "effect is missing")
		}
		return builder.compileEffect(planStepFromManifest(*node.Effect, builder.nextIdentity+1, "flow"), continuation), nil
	case "sequence", "branch":
		next := continuation
		previousBranch := builder.currentBranch
		if node.Kind == "branch" {
			builder.currentBranch = node.Identity
		}
		for index := len(node.Children) - 1; index >= 0; index-- {
			entry, err := builder.compileAuthoringNode(node.Children[index], next)
			if err != nil {
				return "", err
			}
			next = entry
		}
		builder.currentBranch = previousBranch
		return next, nil
	case "parallel":
		join := builder.addNode("join", NodeJoin)
		builder.addTransition(join, MachineContinue, continuation)
		fork := builder.addNode("fork", NodeFork)
		for _, branch := range node.Children {
			exit := builder.addNode("branch-"+branch.Identity+"-exit", NodeBranchExit)
			builder.node(exit).Branch = branch.Identity
			entry, err := builder.compileAuthoringNode(branch, exit)
			if err != nil {
				return "", err
			}
			builder.node(fork).Targets = append(builder.node(fork).Targets, entry)
			builder.node(join).WaitFor = append(builder.node(join).WaitFor, exit)
		}
		builder.node(fork).Targets = append(builder.node(fork).Targets, join)
		return fork, nil
	case "region":
		region := builder.addNode("region", NodeEntry)
		regionIdentity := "region/" + node.RunsOn
		if len(node.Env) > 0 {
			regionIdentity = "region/" + strings.TrimPrefix(region, "node/")
		}
		builder.addRegion(regionIdentity, node.RunsOn, normalizeEnvironment(node.Env))
		previous := builder.currentRegion
		builder.currentRegion = regionIdentity
		next := continuation
		for index := len(node.Children) - 1; index >= 0; index-- {
			entry, err := builder.compileAuthoringNode(node.Children[index], next)
			if err != nil {
				return "", err
			}
			next = entry
		}
		builder.currentRegion = previous
		builder.addTransition(region, MachineContinue, next)
		return region, nil
	case "match":
		if node.Source == nil || node.Effect == nil {
			return "", errorsForFlowNode(node.Kind, "source effect and result reference are required")
		}
		match := builder.addNode("match-"+node.Source.ResultType, NodeMatch)
		builder.node(match).Source = builder.valueIdentity(node.Source.Source)
		seen := map[string]bool{}
		for _, arm := range node.Arms {
			if seen[arm.Kind] {
				return "", fmt.Errorf("TSPACK_WORKFLOW_MATCH_KIND_DUPLICATE: MatchResult repeats %s", arm.Kind)
			}
			seen[arm.Kind] = true
			entry, err := builder.compileAuthoringNode(arm.Flow, continuation)
			if err != nil {
				return "", err
			}
			builder.addTransitionGuard(match, MachineContinue, arm.Kind, entry)
		}
		for _, kind := range []string{"succeeded", "failed", "cancelled", "timedOut"} {
			if !seen[kind] {
				return "", fmt.Errorf("TSPACK_WORKFLOW_MATCH_NON_EXHAUSTIVE: MatchResult is missing %s", kind)
			}
		}
		if _, exists := builder.matchTargets[node.Effect.ResultIdentity]; exists {
			return "", fmt.Errorf("TSPACK_WORKFLOW_MATCH_SOURCE_DUPLICATE: value %s is matched more than once", node.Effect.ResultIdentity)
		}
		builder.matchTargets[node.Effect.ResultIdentity] = match
		if builder.authoredEffects[node.Effect.ResultIdentity] > 0 {
			return match, nil
		}
		return builder.compileEffect(planStepFromManifest(*node.Effect, builder.nextIdentity+1, "flow"), match), nil
	case "finally":
		if node.Body == nil || node.Cleanup == nil {
			return "", errorsForFlowNode(node.Kind, "body and cleanup are required")
		}
		if builder.finallyDepth >= DefaultFinallyNestingLimit {
			return "", fmt.Errorf("TSPACK_WORKFLOW_FINALLY_LIMIT_EXCEEDED: Finally nesting exceeds %d", DefaultFinallyNestingLimit)
		}
		builder.finallyDepth++
		defer func() { builder.finallyDepth-- }()

		outerFailure := builder.failureTarget
		outerCancel := builder.cancelTarget
		outerTimeout := builder.timeoutTarget
		successCleanup, err := builder.compileCleanup(*node.Cleanup, continuation, TerminalSucceeded)
		if err != nil {
			return "", err
		}
		failedCleanup, err := builder.compileCleanup(*node.Cleanup, outerFailure, TerminalFailed)
		if err != nil {
			return "", err
		}
		cancelledCleanup, err := builder.compileCleanup(*node.Cleanup, outerCancel, TerminalCancelled)
		if err != nil {
			return "", err
		}
		timedOutCleanup, err := builder.compileCleanup(*node.Cleanup, outerTimeout, TerminalTimedOut)
		if err != nil {
			return "", err
		}

		previousFailure := builder.failureTarget
		previousCancel := builder.cancelTarget
		previousTimeout := builder.timeoutTarget
		builder.failureTarget = failedCleanup
		builder.cancelTarget = cancelledCleanup
		builder.timeoutTarget = timedOutCleanup
		entry, bodyErr := builder.compileAuthoringNode(*node.Body, successCleanup)
		builder.failureTarget = previousFailure
		builder.cancelTarget = previousCancel
		builder.timeoutTarget = previousTimeout
		return entry, bodyErr
	case "forEach":
		return builder.compileForEach(node, continuation)
	case "when":
		if node.Predicate == nil || node.Then == nil {
			return "", errorsForFlowNode(node.Kind, "predicate and true branch are required")
		}
		predicate, err := predicateFromManifest(*node.Predicate)
		if err != nil {
			return "", err
		}
		builder.normalizePredicate(&predicate)
		falseEntry := continuation
		if node.Else != nil {
			falseEntry, err = builder.compileAuthoringNode(*node.Else, continuation)
			if err != nil {
				return "", err
			}
		}
		trueEntry, err := builder.compileAuthoringNode(*node.Then, continuation)
		if err != nil {
			return "", err
		}
		condition := builder.addNode("when", NodePredicate)
		builder.node(condition).Predicate = &predicate
		builder.addTransitionGuard(condition, MachineContinue, "true", trueEntry)
		builder.addTransitionGuard(condition, MachineContinue, "false", falseEntry)
		return condition, nil
	default:
		return "", errorsForFlowNode(node.Kind, "unsupported authoring node")
	}
}

func (builder *flowBuilder) compileForEach(node manifest.WorkflowFlowNode, continuation string) (string, error) {
	if len(node.Items) == 0 || len(node.Items) > DefaultForEachLimit {
		return "", fmt.Errorf("TSPACK_WORKFLOW_FOREACH_LIMIT_INVALID: ForEach %s requires 1..%d finite items", node.Identity, DefaultForEachLimit)
	}
	if builder.forEachDepth > 0 {
		return "", errorsForFlowNode("forEach", "nested ForEach is not supported in schema v3")
	}
	mode := node.Mode
	if mode == "" {
		mode = "sequential"
	}
	if mode != "sequential" && mode != "parallel" {
		return "", fmt.Errorf("TSPACK_WORKFLOW_FOREACH_MODE_INVALID: ForEach %s mode must be sequential or parallel", node.Identity)
	}
	failurePolicy := node.FailurePolicy
	if failurePolicy == "" {
		failurePolicy = "failFast"
	}
	if failurePolicy != "failFast" && failurePolicy != "collectAll" {
		return "", fmt.Errorf("TSPACK_WORKFLOW_FOREACH_FAILURE_POLICY_INVALID: ForEach %s failure policy must be failFast or collectAll", node.Identity)
	}
	concurrency := 1
	if mode == "parallel" {
		concurrency = node.Concurrency
		if concurrency == 0 {
			concurrency = DefaultForEachConcurrency
		}
		if concurrency < 1 || concurrency > MaxForEachConcurrency {
			return "", fmt.Errorf("TSPACK_WORKFLOW_FOREACH_CONCURRENCY_INVALID: ForEach %s concurrency must be 1..%d", node.Identity, MaxForEachConcurrency)
		}
	}

	aggregate, err := builder.addForEachAggregate(node, mode, concurrency, failurePolicy)
	if err != nil {
		return "", err
	}
	builder.forEachDepth++
	defer func() { builder.forEachDepth-- }()

	if mode == "sequential" {
		next := continuation
		for index := len(node.Items) - 1; index >= 0; index-- {
			entry, compileErr := builder.compileForEachItem(node, node.Items[index], next, mode, concurrency, failurePolicy, aggregate.Identity)
			if compileErr != nil {
				return "", compileErr
			}
			next = entry
		}
		return next, nil
	}

	join := builder.addNode("foreach-"+node.Identity+"-join", NodeJoin)
	builder.addTransition(join, MachineContinue, continuation)
	fork := builder.addNode("foreach-"+node.Identity+"-fork", NodeFork)
	exits := make([]string, len(node.Items))
	entries := make([]string, len(node.Items))
	for index, item := range node.Items {
		exits[index] = builder.addNode(fmt.Sprintf("foreach-%s-%03d-exit", node.Identity, item.Index), NodeBranchExit)
		builder.node(join).WaitFor = append(builder.node(join).WaitFor, exits[index])
	}
	for index, item := range node.Items {
		entry, compileErr := builder.compileForEachItem(node, item, exits[index], mode, concurrency, failurePolicy, aggregate.Identity)
		if compileErr != nil {
			return "", compileErr
		}
		entries[index] = entry
	}
	for index, exit := range exits {
		nextIndex := index + concurrency
		if nextIndex < len(entries) {
			builder.addTransition(exit, MachineContinue, entries[nextIndex])
		}
	}
	for index := 0; index < concurrency && index < len(entries); index++ {
		builder.node(fork).Targets = append(builder.node(fork).Targets, entries[index])
	}
	builder.node(fork).Targets = append(builder.node(fork).Targets, join)
	return fork, nil
}

func (builder *flowBuilder) compileForEachItem(node manifest.WorkflowFlowNode, item manifest.WorkflowForEachItem, continuation string, mode string, concurrency int, failurePolicy string, aggregate string) (string, error) {
	previousFailure := builder.failureTarget
	previousCancel := builder.cancelTarget
	previousTimeout := builder.timeoutTarget
	if failurePolicy == "collectAll" {
		builder.failureTarget = continuation
		builder.cancelTarget = continuation
		builder.timeoutTarget = continuation
	}
	entry, err := builder.compileAuthoringNode(item.Flow, continuation)
	builder.failureTarget = previousFailure
	builder.cancelTarget = previousCancel
	builder.timeoutTarget = previousTimeout
	if err != nil {
		return "", err
	}
	cursor := builder.addNode(fmt.Sprintf("foreach-%s-%03d", node.Identity, item.Index), NodeIterator)
	binding := builder.valueIdentity(fmt.Sprintf("foreach/%s/%03d", node.Identity, item.Index))
	builder.node(cursor).Iterator = &IteratorCursor{
		SourceIdentity: node.Identity,
		Index:          item.Index,
		Count:          len(node.Items),
		Binding:        binding,
		Value: IterationValue{
			Kind:    item.Value.Kind,
			String:  item.Value.String,
			Number:  item.Value.Number,
			Boolean: item.Value.Boolean,
		},
		Mode:          mode,
		FailurePolicy: failurePolicy,
		Concurrency:   concurrency,
		Aggregate:     aggregate,
	}
	category := ValueControl
	if item.Value.Kind == "platform" {
		category = ValuePlacement
	}
	builder.flow.Values = append(builder.flow.Values, ValueDefinition{
		ValueRef: ValueRef{Identity: binding, Source: binding, ResultType: "iterationBinding", Category: category},
		Producer: cursor,
		Region:   builder.currentRegion,
	})
	builder.addTransition(cursor, MachineContinue, entry)
	return cursor, nil
}

func (builder *flowBuilder) addForEachAggregate(node manifest.WorkflowFlowNode, mode string, concurrency int, failurePolicy string) (AggregateDefinition, error) {
	aggregate := AggregateDefinition{
		Identity:      builder.valueIdentity("aggregate/" + node.Identity),
		Mode:          mode,
		Concurrency:   concurrency,
		FailurePolicy: failurePolicy,
	}
	if node.Aggregate != nil {
		aggregate.Identity = builder.valueIdentity(node.Aggregate.Identity)
		aggregate.ResultType = node.Aggregate.ResultType
		for _, element := range node.Aggregate.Elements {
			aggregate.Elements = append(aggregate.Elements, builder.valueIdentity(element))
		}
	} else {
		for _, item := range node.Items {
			identity, resultType, ok := singleTypedEffect(item.Flow)
			if !ok {
				return aggregate, nil
			}
			if aggregate.ResultType == "" {
				aggregate.ResultType = resultType
			}
			if aggregate.ResultType != resultType {
				return AggregateDefinition{}, errorsForFlowNode("forEach", "aggregate elements must have one result type")
			}
			aggregate.Elements = append(aggregate.Elements, builder.valueIdentity(identity))
		}
	}
	if len(aggregate.Elements) == len(node.Items) {
		if failurePolicy == "collectAll" {
			aggregate.ResultType = "iterationOutcome<" + aggregate.ResultType + ">"
		}
		builder.flow.Aggregates = append(builder.flow.Aggregates, aggregate)
	}
	return aggregate, nil
}

func singleTypedEffect(node manifest.WorkflowFlowNode) (string, string, bool) {
	if node.Kind == "effect" && node.Effect != nil && node.Effect.ResultIdentity != "" {
		switch node.Effect.Operation {
		case "build", "test", "audit":
			return node.Effect.ResultIdentity, node.Effect.Operation, true
		}
	}
	if node.Kind == "region" && len(node.Children) == 1 {
		return singleTypedEffect(node.Children[0])
	}
	return "", "", false
}

func predicateFromManifest(source manifest.WorkflowPredicate) (Predicate, error) {
	predicate := Predicate{
		Kind:    source.Kind,
		Number:  source.Number,
		Boolean: source.Boolean,
		String:  source.String,
	}
	if source.Input != nil {
		input := valueRefsFromManifest([]manifest.WorkflowValueRef{*source.Input})[0]
		predicate.Input = &input
	}
	for _, child := range source.Children {
		converted, err := predicateFromManifest(child)
		if err != nil {
			return Predicate{}, err
		}
		predicate.Children = append(predicate.Children, converted)
	}
	return predicate, nil
}

func (builder *flowBuilder) normalizePredicate(predicate *Predicate) {
	if predicate.Input != nil {
		predicate.Input.Identity = builder.valueIdentity(predicate.Input.Identity)
		predicate.Input.Source = builder.valueIdentity(predicate.Input.Source)
		builder.addProjection(*predicate.Input)
	}
	for index := range predicate.Children {
		builder.normalizePredicate(&predicate.Children[index])
	}
}

func (builder *flowBuilder) compileCleanup(node manifest.WorkflowFlowNode, continuation string, cause TerminalKind) (string, error) {
	previousFailure := builder.failureTarget
	previousCancel := builder.cancelTarget
	previousTimeout := builder.timeoutTarget
	previousCause := builder.cleanupCause
	builder.failureTarget = builder.flow.Terminals.Failed
	builder.cancelTarget = builder.flow.Terminals.Cancelled
	builder.timeoutTarget = builder.flow.Terminals.TimedOut
	if cause != TerminalSucceeded {
		builder.failureTarget = builder.flow.Terminals.Failed
		builder.timeoutTarget = builder.flow.Terminals.Failed
	}
	builder.cleanupCause = cause
	entry, err := builder.compileAuthoringNode(node, continuation)
	builder.failureTarget = previousFailure
	builder.cancelTarget = previousCancel
	builder.timeoutTarget = previousTimeout
	builder.cleanupCause = previousCause
	return entry, err
}

func errorsForFlowNode(kind string, message string) error {
	return fmt.Errorf("TSPACK_WORKFLOW_FLOW_NODE_INVALID: %s: %s", kind, message)
}

func (builder *flowBuilder) compileEffect(step PlanStep, continuation string) string {
	if builder.cleanupCause != "" && step.ResultIdentity != "" {
		step.ResultIdentity = fmt.Sprintf("%s/cleanup/%s/%03d", step.ResultIdentity, builder.cleanupCause, builder.nextIdentity+1)
	}
	if step.ResultIdentity != "" {
		if _, exists := builder.effects[step.ResultIdentity]; exists {
			return continuation
		}
	}
	identity := builder.addNode(step.Operation, NodeEffect)
	node := builder.node(identity)
	step.Identity = identity
	node.Region = builder.currentRegion
	if step.Operation == "transfer" {
		node.Region = "region/" + step.TransferTarget
		builder.addRegion(node.Region, step.TransferTarget, nil)
	}
	node.Branch = builder.currentBranch
	node.Effect = &step
	node.Cleanup = builder.cleanupCause != ""
	node.CleanupCause = builder.cleanupCause
	if step.ResultIdentity == "" {
		step.ResultIdentity = strings.TrimPrefix(identity, "node/")
	}
	node.Value = builder.valueIdentity(step.ResultIdentity)
	step.Inputs = builder.normalizeInputs(step.Inputs)
	if step.Operation == "transfer" && len(step.Inputs) == 1 {
		step.TransferSourceRegion = builder.valueRegion(step.Inputs[0].Source)
	}
	if original := strings.TrimPrefix(step.ResultIdentity, "value/"); original != "" {
		builder.effects[original] = identity
	}
	producerType := step.Operation
	builder.flow.Values = append(builder.flow.Values, ValueDefinition{
		ValueRef:    ValueRef{Identity: node.Value, Source: node.Value, ResultType: producerType, Category: ValueRegionLocal},
		Producer:    identity,
		Region:      node.Region,
		Transported: step.Operation == "transfer",
	})
	for _, input := range step.Inputs {
		if input.Identity == input.Source {
			continue
		}
		builder.addProjection(input)
	}
	matchTarget := builder.matchTargets[step.ResultIdentity]
	if matchTarget != "" {
		builder.addTransition(identity, MachineEffectSucceeded, continuation)
		builder.addTransition(identity, MachineEffectFailed, continuation)
		builder.addTransition(identity, MachineCancelRequested, continuation)
		builder.addTransition(identity, MachineTimeout, continuation)
	} else {
		builder.addTransition(identity, MachineEffectSucceeded, continuation)
		builder.addTransition(identity, MachineEffectFailed, builder.failureTarget)
		builder.addTransition(identity, MachineCancelRequested, builder.cancelTarget)
		builder.addTransition(identity, MachineTimeout, builder.timeoutTarget)
	}
	return identity
}

func collectAuthoredEffects(node manifest.WorkflowFlowNode) map[string]int {
	counts := map[string]int{}
	var visit func(manifest.WorkflowFlowNode)
	visit = func(current manifest.WorkflowFlowNode) {
		if current.Kind == "effect" && current.Effect != nil && current.Effect.ResultIdentity != "" {
			counts[current.Effect.ResultIdentity]++
		}
		for _, child := range current.Children {
			visit(child)
		}
		if current.Body != nil {
			visit(*current.Body)
		}
		if current.Cleanup != nil {
			visit(*current.Cleanup)
		}
		for _, arm := range current.Arms {
			visit(arm.Flow)
		}
		for _, item := range current.Items {
			visit(item.Flow)
		}
	}
	visit(node)
	return counts
}

func planStepFromManifest(step manifest.WorkflowStep, ordinal int, prefix string) PlanStep {
	name := step.Name
	if name == "" {
		name = operationDisplayName(step.Operation)
	}
	capabilities := sortedCopy(step.Capabilities)
	if len(capabilities) == 0 {
		capabilities = nativeCapabilities(step.Operation)
	}
	return PlanStep{
		Identity:        fmt.Sprintf("%s/step-%02d-%s", prefix, ordinal, step.Operation),
		ResultIdentity:  step.ResultIdentity,
		Inputs:          valueRefsFromManifest(step.Inputs),
		Name:            name,
		Operation:       step.Operation,
		Packages:        sortedCopy(step.Packages),
		Targets:         sortedCopy(step.Targets),
		Filter:          step.Filter,
		AuditLevel:      step.AuditLevel,
		RequireCoverage: step.RequireCoverage,
		Command:         append([]string(nil), step.Command...),
		Script:          step.Script,
		Shell:           step.Shell,
		Cwd:             step.Cwd,
		Capabilities:    capabilities,
		Environment:     normalizeEnvironment(step.Env),
		TimeoutSeconds:  step.TimeoutSeconds,
		TransferTarget:  step.TransferTarget,
	}
}

func valueRefsFromManifest(values []manifest.WorkflowValueRef) []ValueRef {
	refs := make([]ValueRef, 0, len(values))
	for _, value := range values {
		refs = append(refs, ValueRef{
			Identity:   value.Identity,
			Source:     value.Source,
			ResultType: value.ResultType,
			FieldPath:  append([]string(nil), value.FieldPath...),
			Category:   ValueCategory(value.Category),
		})
	}
	return refs
}

func (builder *flowBuilder) valueIdentity(source string) string {
	if strings.HasPrefix(source, "value/") {
		return source
	}
	return "value/" + sanitizeFlowIdentity(builder.flow.Identity) + "/" + strings.TrimPrefix(source, "/")
}

func (builder *flowBuilder) normalizeInputs(inputs []ValueRef) []ValueRef {
	normalized := make([]ValueRef, 0, len(inputs))
	for _, input := range inputs {
		input.Identity = builder.valueIdentity(input.Identity)
		input.Source = builder.valueIdentity(input.Source)
		normalized = append(normalized, input)
	}
	return normalized
}

func (builder *flowBuilder) addProjection(reference ValueRef) {
	for _, value := range builder.flow.Values {
		if value.Identity == reference.Identity {
			return
		}
	}
	builder.flow.Values = append(builder.flow.Values, ValueDefinition{ValueRef: reference})
}

func (builder *flowBuilder) valueRegion(identity string) string {
	for _, value := range builder.flow.Values {
		if value.Identity == identity {
			return value.Region
		}
	}
	return ""
}

func (builder *flowBuilder) addNode(label string, kind NodeKind) string {
	builder.nextIdentity++
	identity := fmt.Sprintf("node/%03d-%s", builder.nextIdentity, sanitizeFlowIdentity(label))
	builder.flow.Nodes = append(builder.flow.Nodes, FlowNode{Identity: identity, Kind: kind})
	return identity
}

func (builder *flowBuilder) addTransition(from string, event MachineEventKind, to string) {
	builder.addTransitionGuard(from, event, string(event), to)
}

func (builder *flowBuilder) addTransitionGuard(from string, event MachineEventKind, guard string, to string) {
	identity := fmt.Sprintf("transition/%03d", len(builder.flow.Transitions)+1)
	builder.flow.Transitions = append(builder.flow.Transitions, Transition{
		Identity: identity,
		From:     from,
		Event:    event,
		Guard:    guard,
		To:       to,
	})
}

func (builder *flowBuilder) addRegion(identity string, platform string, environment []Environment) {
	if platform == "" {
		platform = "currentHost"
	}
	for _, region := range builder.flow.Regions {
		if region.Identity == identity {
			return
		}
	}
	builder.flow.Regions = append(builder.flow.Regions, ExecutionRegion{
		Identity:    identity,
		Platform:    platform,
		Environment: append([]Environment(nil), environment...),
	})
}

func (builder *flowBuilder) node(identity string) *FlowNode {
	for index := range builder.flow.Nodes {
		if builder.flow.Nodes[index].Identity == identity {
			return &builder.flow.Nodes[index]
		}
	}
	panic("flow builder referenced an unknown node: " + identity)
}

func sanitizeFlowIdentity(value string) string {
	var builder strings.Builder
	for _, character := range value {
		if character >= 'A' && character <= 'Z' {
			character += 'a' - 'A'
		}
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' {
			builder.WriteRune(character)
		} else {
			builder.WriteRune('-')
		}
	}
	return strings.Trim(builder.String(), "-")
}

func ValidateFlow(flow Flow) []string {
	diagnostics := []string{}
	add := func(code string, message string) {
		diagnostics = append(diagnostics, code+": "+message)
	}
	if flow.SchemaVersion != FlowSchemaVersion {
		add("TSPACK_WORKFLOW_SCHEMA_UNSUPPORTED", fmt.Sprintf("schemaVersion must be %d", FlowSchemaVersion))
	}
	nodes := map[string]FlowNode{}
	for _, node := range flow.Nodes {
		if node.Identity == "" {
			add("TSPACK_WORKFLOW_NODE_IDENTITY_INVALID", "node identity must not be empty")
		}
		if _, exists := nodes[node.Identity]; exists {
			add("TSPACK_WORKFLOW_NODE_IDENTITY_DUPLICATE", "duplicate node "+node.Identity)
		}
		nodes[node.Identity] = node
		if node.Kind == NodeEffect && node.Effect == nil {
			add("TSPACK_WORKFLOW_EFFECT_REQUIRED", node.Identity+" has no effect")
		}
		if node.Kind == NodeJoin && len(node.WaitFor) == 0 {
			add("TSPACK_WORKFLOW_JOIN_INVALID", node.Identity+" has no branches to join")
		}
		if node.Kind == NodePredicate && node.Predicate == nil {
			add("TSPACK_WORKFLOW_PREDICATE_INVALID", node.Identity+" has no predicate")
		}
	}
	if _, exists := nodes[flow.Entry]; !exists {
		add("TSPACK_WORKFLOW_ENTRY_UNKNOWN", "entry node does not exist")
	}
	regions := map[string]struct{}{}
	for _, region := range flow.Regions {
		if _, exists := regions[region.Identity]; exists {
			add("TSPACK_WORKFLOW_REGION_DUPLICATE", "duplicate region "+region.Identity)
		}
		regions[region.Identity] = struct{}{}
	}
	transitionKeys := map[string]struct{}{}
	transitionCases := map[string]struct{}{}
	for _, transition := range flow.Transitions {
		if _, exists := transitionKeys[transition.Identity]; exists {
			add("TSPACK_WORKFLOW_TRANSITION_DUPLICATE", "duplicate transition "+transition.Identity)
		}
		transitionKeys[transition.Identity] = struct{}{}
		if _, exists := nodes[transition.From]; !exists {
			add("TSPACK_WORKFLOW_TRANSITION_SOURCE_UNKNOWN", transition.Identity+" has unknown source "+transition.From)
		}
		if _, exists := nodes[transition.To]; !exists {
			add("TSPACK_WORKFLOW_TRANSITION_TARGET_UNKNOWN", transition.Identity+" has unknown target "+transition.To)
		}
		caseKey := transition.From + "\x00" + string(transition.Event)
		if source, exists := nodes[transition.From]; exists && (source.Kind == NodeMatch || source.Kind == NodePredicate) {
			caseKey += "\x00" + transition.Guard
		}
		if _, exists := transitionCases[caseKey]; exists {
			add("TSPACK_WORKFLOW_TRANSITION_AMBIGUOUS", transition.From+" handles "+string(transition.Event)+" more than once")
		}
		transitionCases[caseKey] = struct{}{}
	}
	for _, node := range flow.Nodes {
		if node.Region != "" {
			if _, exists := regions[node.Region]; !exists {
				add("TSPACK_WORKFLOW_REGION_UNKNOWN", node.Identity+" selects unknown region "+node.Region)
			}
		}
		for _, reference := range append(append([]string(nil), node.Targets...), node.WaitFor...) {
			if _, exists := nodes[reference]; !exists {
				add("TSPACK_WORKFLOW_NODE_REFERENCE_UNKNOWN", node.Identity+" references unknown node "+reference)
			}
		}
		if node.Kind == NodeEffect {
			for _, event := range []MachineEventKind{MachineEffectSucceeded, MachineEffectFailed, MachineCancelRequested, MachineTimeout} {
				if _, exists := transitionCases[node.Identity+"\x00"+string(event)]; !exists {
					add("TSPACK_WORKFLOW_RESULT_UNHANDLED", node.Identity+" does not handle "+string(event))
				}
			}
		}
		if node.Kind == NodeMatch {
			for _, outcome := range []string{"succeeded", "failed", "cancelled", "timedOut"} {
				if _, exists := transitionCases[node.Identity+"\x00"+string(MachineContinue)+"\x00"+outcome]; !exists {
					add("TSPACK_WORKFLOW_MATCH_NON_EXHAUSTIVE", node.Identity+" does not handle "+outcome)
				}
			}
		}
		if node.Kind == NodePredicate {
			for _, outcome := range []string{"true", "false"} {
				if _, exists := transitionCases[node.Identity+"\x00"+string(MachineContinue)+"\x00"+outcome]; !exists {
					add("TSPACK_WORKFLOW_PREDICATE_NON_EXHAUSTIVE", node.Identity+" does not handle "+outcome)
				}
			}
		}
	}
	validateFlowValues(add, flow, nodes)
	adjacency := map[string][]string{}
	for _, node := range flow.Nodes {
		adjacency[node.Identity] = append(adjacency[node.Identity], node.Targets...)
		if node.Kind == NodeJoin {
			for _, dependency := range node.WaitFor {
				adjacency[dependency] = append(adjacency[dependency], node.Identity)
			}
		}
	}
	for _, transition := range flow.Transitions {
		adjacency[transition.From] = append(adjacency[transition.From], transition.To)
	}
	reachable := map[string]bool{}
	var markReachable func(string)
	markReachable = func(identity string) {
		if reachable[identity] {
			return
		}
		reachable[identity] = true
		for _, target := range adjacency[identity] {
			markReachable(target)
		}
	}
	markReachable(flow.Entry)
	for _, node := range flow.Nodes {
		if !reachable[node.Identity] && node.Kind != NodeTerminal && !node.Cleanup {
			add("TSPACK_WORKFLOW_NODE_UNREACHABLE", node.Identity+" cannot be reached from entry")
		}
	}
	visitState := map[string]int{}
	var visit func(string) bool
	visit = func(identity string) bool {
		if visitState[identity] == 1 {
			return true
		}
		if visitState[identity] == 2 {
			return false
		}
		visitState[identity] = 1
		for _, target := range adjacency[identity] {
			if visit(target) {
				return true
			}
		}
		visitState[identity] = 2
		return false
	}
	if visit(flow.Entry) {
		add("TSPACK_WORKFLOW_CYCLE_UNBOUNDED", "runtime cycles are not supported until an explicit bounded loop construct exists")
	}
	sort.Strings(diagnostics)
	return diagnostics
}

func validateFlowValues(add func(string, string), flow Flow, nodes map[string]FlowNode) {
	definitions := map[string]ValueDefinition{}
	for _, value := range flow.Values {
		if _, exists := definitions[value.Identity]; exists {
			add("TSPACK_WORKFLOW_VALUE_DUPLICATE", "value "+value.Identity+" is defined more than once")
			continue
		}
		definitions[value.Identity] = value
		if value.Source != value.Identity {
			if !validProjection(value.ResultType, value.FieldPath) {
				add("TSPACK_WORKFLOW_PROJECTION_INVALID", value.Identity+" is not a field of "+value.ResultType)
			}
		}
	}
	for _, node := range flow.Nodes {
		if node.Kind == NodeMatch {
			definition, exists := definitions[node.Source]
			if !exists {
				add("TSPACK_WORKFLOW_VALUE_UNKNOWN", node.Identity+" matches unknown value "+node.Source)
			} else if definition.Producer != "" && !flowNodeReaches(flow, definition.Producer, node.Identity) {
				add("TSPACK_WORKFLOW_VALUE_SCOPE_ILLEGAL", node.Source+" is not produced on a path reaching "+node.Identity)
			}
		}
		if node.Kind == NodePredicate && node.Predicate != nil {
			validatePredicateValues(add, flow, node.Identity, node.Predicate, definitions, 1)
		}
		if node.Effect == nil {
			continue
		}
		for _, input := range node.Effect.Inputs {
			definition, exists := definitions[input.Identity]
			if !exists {
				add("TSPACK_WORKFLOW_VALUE_UNKNOWN", node.Identity+" consumes unknown value "+input.Identity)
				continue
			}
			source := definitions[definition.Source]
			if source.Producer != "" && !flowNodeReaches(flow, source.Producer, node.Identity) {
				add("TSPACK_WORKFLOW_VALUE_SCOPE_ILLEGAL", input.Identity+" is not produced on a path reaching "+node.Identity)
			}
			if source.Region != "" && node.Region != "" && source.Region != node.Region {
				if input.Category == ValueArtifactReference || input.Category == ValueRegionLocal {
					transportAvailable := source.Transported && input.Category == ValueArtifactReference && regionsSharePlatform(flow, source.Region, node.Region)
					if (node.Effect.Operation != "transfer" || input.Category != ValueArtifactReference) && !transportAvailable {
						add("TSPACK_WORKFLOW_VALUE_REGION_ILLEGAL", input.Identity+" cannot cross from "+source.Region+" to "+node.Region+" without semantic transport")
					}
				}
			}
		}
	}
	aggregateIdentities := map[string]bool{}
	for _, aggregate := range flow.Aggregates {
		if aggregateIdentities[aggregate.Identity] {
			add("TSPACK_WORKFLOW_AGGREGATE_DUPLICATE", "aggregate "+aggregate.Identity+" is defined more than once")
		}
		aggregateIdentities[aggregate.Identity] = true
		if aggregate.Identity == "" || len(aggregate.Elements) == 0 || len(aggregate.Elements) > DefaultForEachLimit {
			add("TSPACK_WORKFLOW_AGGREGATE_INVALID", "aggregate requires deterministic identity and bounded elements")
		}
		if aggregate.Mode != "sequential" && aggregate.Mode != "parallel" {
			add("TSPACK_WORKFLOW_FOREACH_MODE_INVALID", aggregate.Identity+" has invalid mode "+aggregate.Mode)
		}
		if aggregate.Concurrency < 1 || aggregate.Concurrency > MaxForEachConcurrency || (aggregate.Mode == "sequential" && aggregate.Concurrency != 1) {
			add("TSPACK_WORKFLOW_FOREACH_CONCURRENCY_INVALID", aggregate.Identity+" has invalid concurrency")
		}
		if aggregate.FailurePolicy != "failFast" && aggregate.FailurePolicy != "collectAll" {
			add("TSPACK_WORKFLOW_FOREACH_FAILURE_POLICY_INVALID", aggregate.Identity+" has invalid failure policy")
		}
		for _, element := range aggregate.Elements {
			definition, exists := definitions[element]
			if !exists || definition.Producer == "" {
				add("TSPACK_WORKFLOW_AGGREGATE_ELEMENT_UNKNOWN", aggregate.Identity+" references unknown element "+element)
			}
		}
	}
}

func regionsSharePlatform(flow Flow, leftIdentity string, rightIdentity string) bool {
	left := ""
	right := ""
	for _, region := range flow.Regions {
		if region.Identity == leftIdentity {
			left = region.Platform
		}
		if region.Identity == rightIdentity {
			right = region.Platform
		}
	}
	return left != "" && left == right
}

func validatePredicateValues(add func(string, string), flow Flow, consumer string, predicate *Predicate, definitions map[string]ValueDefinition, depth int) {
	if depth > DefaultPredicateDepthLimit {
		add("TSPACK_WORKFLOW_PREDICATE_LIMIT_EXCEEDED", consumer+" predicate exceeds depth limit")
		return
	}
	if predicate.Input != nil {
		definition, exists := definitions[predicate.Input.Identity]
		if !exists {
			add("TSPACK_WORKFLOW_VALUE_UNKNOWN", consumer+" predicate consumes unknown value "+predicate.Input.Identity)
		} else {
			source := definitions[definition.Source]
			if source.Producer != "" && !flowNodeReaches(flow, source.Producer, consumer) {
				add("TSPACK_WORKFLOW_VALUE_SCOPE_ILLEGAL", predicate.Input.Identity+" is not produced on a path reaching "+consumer)
			}
		}
	}
	for index := range predicate.Children {
		validatePredicateValues(add, flow, consumer, &predicate.Children[index], definitions, depth+1)
	}
}

func flowNodeReaches(flow Flow, from string, target string) bool {
	adjacency := map[string][]string{}
	for _, node := range flow.Nodes {
		adjacency[node.Identity] = append(adjacency[node.Identity], node.Targets...)
		if node.Kind == NodeJoin {
			for _, dependency := range node.WaitFor {
				adjacency[dependency] = append(adjacency[dependency], node.Identity)
			}
		}
	}
	for _, transition := range flow.Transitions {
		adjacency[transition.From] = append(adjacency[transition.From], transition.To)
	}
	seen := map[string]bool{}
	pending := []string{from}
	for len(pending) > 0 {
		identity := pending[0]
		pending = pending[1:]
		if identity == target {
			return true
		}
		if seen[identity] {
			continue
		}
		seen[identity] = true
		pending = append(pending, adjacency[identity]...)
	}
	return false
}

func validProjection(resultType string, path []string) bool {
	if len(path) != 1 {
		return false
	}
	fields := map[string]map[string]bool{
		"build":    {"artifacts": true, "targets": true, "diagnostics": true},
		"transfer": {"artifacts": true, "targets": true, "diagnostics": true},
		"test":     {"passed": true, "failed": true, "skipped": true, "durationMs": true, "tests": true, "diagnostics": true},
		"audit":    {"source": true, "auditLevel": true, "failing": true, "report": true, "diagnostics": true},
	}
	return fields[resultType][path[0]]
}
