package manifest

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/pathutil"
)

var workflowIdentityPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

func validateWorkflows(add func(string, string, ...string), ir *ManifestIR) {
	packageNames := map[string]struct{}{}
	for _, declaredPackage := range ir.Packages {
		packageNames[declaredPackage.Name] = struct{}{}
	}

	seenWorkflows := map[string]struct{}{}
	for workflowIndex := range ir.Workflows {
		workflow := &ir.Workflows[workflowIndex]
		prefix := fmt.Sprintf("workflows[%d]", workflowIndex)
		if !workflowIdentityPattern.MatchString(workflow.Identity) {
			add("TSPACK_WORKFLOW_IDENTITY_INVALID", prefix+".identity must be a stable identifier")
		}
		if _, exists := seenWorkflows[workflow.Identity]; exists {
			add("TSPACK_WORKFLOW_IDENTITY_DUPLICATE", "duplicate workflow identity: "+workflow.Identity)
		}
		seenWorkflows[workflow.Identity] = struct{}{}

		if len(workflow.Triggers) == 0 {
			add("TSPACK_WORKFLOW_TRIGGER_REQUIRED", prefix+".triggers must not be empty")
		}
		seenTriggers := map[string]struct{}{}
		for triggerIndex, trigger := range workflow.Triggers {
			triggerPrefix := fmt.Sprintf("%s.triggers[%d]", prefix, triggerIndex)
			if trigger.Kind != "manual" && trigger.Kind != "push" && trigger.Kind != "pullRequest" {
				add("TSPACK_WORKFLOW_TRIGGER_INVALID", triggerPrefix+".kind must be manual, push, or pullRequest")
			}
			if _, exists := seenTriggers[trigger.Kind]; exists {
				add("TSPACK_WORKFLOW_TRIGGER_DUPLICATE", prefix+" repeats trigger kind "+trigger.Kind)
			}
			seenTriggers[trigger.Kind] = struct{}{}
			if trigger.Kind == "manual" && (len(trigger.Branches) > 0 || len(trigger.Paths) > 0) {
				add("TSPACK_WORKFLOW_TRIGGER_FILTER_INVALID", triggerPrefix+" manual triggers do not accept branch or path filters")
			}
			for _, branch := range trigger.Branches {
				if strings.TrimSpace(branch) == "" {
					add("TSPACK_WORKFLOW_TRIGGER_FILTER_INVALID", triggerPrefix+".branches must contain non-empty portable branch patterns")
				}
			}
			for _, pathPattern := range trigger.Paths {
				if !pathutil.IsSafeRelativeGlob(pathPattern) && !pathutil.IsSafePackageFilePath(pathPattern) {
					add("TSPACK_WORKFLOW_TRIGGER_FILTER_INVALID", triggerPrefix+".paths must contain safe workspace-relative paths or globs")
				}
			}
		}

		if workflow.Flow != nil && len(workflow.Jobs) > 0 {
			add("TSPACK_WORKFLOW_AUTHORING_AMBIGUOUS", prefix+" must declare either flow or legacy jobs, not both")
		}
		if workflow.Flow == nil && len(workflow.Jobs) == 0 {
			add("TSPACK_WORKFLOW_FLOW_REQUIRED", prefix+" must declare flow or legacy jobs")
		}
		if workflow.Flow != nil {
			validateWorkflowFlowNode(add, prefix+".flow", workflow.Flow, packageNames, "currentHost")
		} else {
			validateWorkflowJobs(add, prefix, workflow, packageNames)
		}
	}
}

func validateWorkflowFlowNode(add func(string, string, ...string), prefix string, node *WorkflowFlowNode, packageNames map[string]struct{}, inheritedPlatform string) {
	switch node.Kind {
	case "effect":
		if node.Effect == nil {
			add("TSPACK_WORKFLOW_EFFECT_REQUIRED", prefix+" effect node must contain an effect")
			return
		}
		if len(node.Children) > 0 || node.Identity != "" {
			add("TSPACK_WORKFLOW_FLOW_NODE_INVALID", prefix+" effect node cannot contain children or an identity")
		}
		validateWorkflowStep(add, prefix+".effect", *node.Effect, packageNames)
	case "sequence":
		if len(node.Children) == 0 {
			add("TSPACK_WORKFLOW_SEQUENCE_EMPTY", prefix+" sequence must contain at least one node")
		}
		for index := range node.Children {
			validateWorkflowFlowNode(add, fmt.Sprintf("%s.children[%d]", prefix, index), &node.Children[index], packageNames, inheritedPlatform)
		}
	case "parallel":
		if len(node.Children) < 2 {
			add("TSPACK_WORKFLOW_PARALLEL_TOO_SMALL", prefix+" parallel must contain at least two branches")
		}
		seen := map[string]struct{}{}
		for index := range node.Children {
			child := &node.Children[index]
			childPrefix := fmt.Sprintf("%s.children[%d]", prefix, index)
			if child.Kind != "branch" {
				add("TSPACK_WORKFLOW_PARALLEL_BRANCH_REQUIRED", childPrefix+" must be a Branch declaration")
			}
			if _, exists := seen[child.Identity]; exists {
				add("TSPACK_WORKFLOW_BRANCH_IDENTITY_DUPLICATE", prefix+" repeats branch "+child.Identity)
			}
			seen[child.Identity] = struct{}{}
			validateWorkflowFlowNode(add, childPrefix, child, packageNames, inheritedPlatform)
		}
	case "branch":
		if !workflowIdentityPattern.MatchString(node.Identity) {
			add("TSPACK_WORKFLOW_BRANCH_IDENTITY_INVALID", prefix+".identity must be a stable identifier")
		}
		if len(node.Children) == 0 {
			add("TSPACK_WORKFLOW_BRANCH_EMPTY", prefix+" branch must contain at least one node")
		}
		for index := range node.Children {
			validateWorkflowFlowNode(add, fmt.Sprintf("%s.children[%d]", prefix, index), &node.Children[index], packageNames, inheritedPlatform)
		}
	case "region":
		platform := node.RunsOn
		if platform == "" {
			platform = inheritedPlatform
		}
		if platform != "linux" && platform != "windows" && platform != "macos" && platform != "currentHost" {
			add("TSPACK_WORKFLOW_PLATFORM_INVALID", prefix+".runsOn must be linux, windows, macos, or currentHost")
		}
		validateWorkflowEnvironment(add, prefix+".env", node.Env)
		if len(node.Children) == 0 {
			add("TSPACK_WORKFLOW_REGION_EMPTY", prefix+" region must contain at least one node")
		}
		for index := range node.Children {
			validateWorkflowFlowNode(add, fmt.Sprintf("%s.children[%d]", prefix, index), &node.Children[index], packageNames, platform)
		}
	case "match":
		if node.Source == nil || node.Effect == nil {
			add("TSPACK_WORKFLOW_MATCH_INVALID", prefix+" MatchResult requires a typed source effect")
			return
		}
		validateWorkflowValueRef(add, prefix+".source", *node.Source)
		seen := map[string]bool{}
		for index := range node.Arms {
			arm := &node.Arms[index]
			armPrefix := fmt.Sprintf("%s.arms[%d]", prefix, index)
			if seen[arm.Kind] {
				add("TSPACK_WORKFLOW_MATCH_KIND_DUPLICATE", prefix+" repeats "+arm.Kind)
			}
			seen[arm.Kind] = true
			validateWorkflowFlowNode(add, armPrefix+".flow", &arm.Flow, packageNames, inheritedPlatform)
		}
		for _, kind := range []string{"succeeded", "failed", "cancelled", "timedOut"} {
			if !seen[kind] {
				add("TSPACK_WORKFLOW_MATCH_NON_EXHAUSTIVE", prefix+" is missing "+kind)
			}
		}
	case "finally":
		if node.Body == nil || node.Cleanup == nil {
			add("TSPACK_WORKFLOW_FINALLY_INVALID", prefix+" Finally requires body and cleanup flows")
			return
		}
		validateWorkflowFlowNode(add, prefix+".body", node.Body, packageNames, inheritedPlatform)
		validateWorkflowFlowNode(add, prefix+".cleanup", node.Cleanup, packageNames, inheritedPlatform)
	case "forEach":
		if !workflowIdentityPattern.MatchString(node.Identity) {
			add("TSPACK_WORKFLOW_FOREACH_IDENTITY_INVALID", prefix+".identity must be a stable identifier")
		}
		if len(node.Items) == 0 || len(node.Items) > 256 {
			add("TSPACK_WORKFLOW_FOREACH_LIMIT_INVALID", prefix+" must contain between 1 and 256 finite items")
		}
		mode := node.Mode
		if mode == "" {
			mode = "sequential"
		}
		if mode != "sequential" && mode != "parallel" {
			add("TSPACK_WORKFLOW_FOREACH_MODE_INVALID", prefix+".mode must be sequential or parallel")
		}
		if mode == "sequential" && node.Concurrency != 0 && node.Concurrency != 1 {
			add("TSPACK_WORKFLOW_FOREACH_CONCURRENCY_INVALID", prefix+" sequential mode has concurrency 1")
		}
		if mode == "parallel" && (node.Concurrency < 0 || node.Concurrency > 32) {
			add("TSPACK_WORKFLOW_FOREACH_CONCURRENCY_INVALID", prefix+" parallel concurrency must be omitted or between 1 and 32")
		}
		failurePolicy := node.FailurePolicy
		if failurePolicy == "" {
			failurePolicy = "failFast"
		}
		if failurePolicy != "failFast" && failurePolicy != "collectAll" {
			add("TSPACK_WORKFLOW_FOREACH_FAILURE_POLICY_INVALID", prefix+".failurePolicy must be failFast or collectAll")
		}
		for index := range node.Items {
			item := &node.Items[index]
			if item.Index != index {
				add("TSPACK_WORKFLOW_FOREACH_CURSOR_INVALID", fmt.Sprintf("%s.items[%d] has non-deterministic cursor index %d", prefix, index, item.Index))
			}
			validateWorkflowIterationValue(add, fmt.Sprintf("%s.items[%d].value", prefix, index), item.Value)
			validateWorkflowFlowNode(add, fmt.Sprintf("%s.items[%d].flow", prefix, index), &item.Flow, packageNames, inheritedPlatform)
		}
	case "when":
		if node.Predicate == nil || node.Then == nil {
			add("TSPACK_WORKFLOW_PREDICATE_INVALID", prefix+" When requires a predicate and true branch")
			return
		}
		validateWorkflowPredicate(add, prefix+".predicate", node.Predicate, 1)
		validateWorkflowFlowNode(add, prefix+".then", node.Then, packageNames, inheritedPlatform)
		if node.Else != nil {
			validateWorkflowFlowNode(add, prefix+".else", node.Else, packageNames, inheritedPlatform)
		}
	default:
		add("TSPACK_WORKFLOW_FLOW_NODE_INVALID", prefix+" has unknown flow node kind "+node.Kind)
	}
}

func validateWorkflowPredicate(add func(string, string, ...string), prefix string, predicate *WorkflowPredicate, depth int) {
	if depth > 8 {
		add("TSPACK_WORKFLOW_PREDICATE_LIMIT_EXCEEDED", prefix+" exceeds depth 8")
		return
	}
	switch predicate.Kind {
	case "greaterThan", "lessThan":
		if predicate.Input == nil || predicate.Number == nil || len(predicate.Children) != 0 {
			add("TSPACK_WORKFLOW_PREDICATE_INVALID", prefix+" numeric comparison requires one input and number")
			return
		}
		validateWorkflowValueRef(add, prefix+".input", *predicate.Input)
		if predicate.Input.Category != "control" {
			add("TSPACK_WORKFLOW_PREDICATE_TYPE_INVALID", prefix+" numeric comparison requires a control value")
		}
	case "notEmpty", "isEmpty":
		if predicate.Input == nil || len(predicate.Children) != 0 {
			add("TSPACK_WORKFLOW_PREDICATE_INVALID", prefix+" collection predicate requires one input")
			return
		}
		validateWorkflowValueRef(add, prefix+".input", *predicate.Input)
		if predicate.Input.Category != "artifactReference" && predicate.Input.Category != "smallSerialized" {
			add("TSPACK_WORKFLOW_PREDICATE_TYPE_INVALID", prefix+" collection predicate requires safe collection metadata")
		}
	case "and", "or":
		if len(predicate.Children) < 2 || len(predicate.Children) > 8 {
			add("TSPACK_WORKFLOW_PREDICATE_INVALID", prefix+" boolean composition requires 2..8 children")
		}
	case "not":
		if len(predicate.Children) != 1 {
			add("TSPACK_WORKFLOW_PREDICATE_INVALID", prefix+" Not requires one child")
		}
	default:
		add("TSPACK_WORKFLOW_PREDICATE_INVALID", prefix+" has unknown predicate kind "+predicate.Kind)
	}
	for index := range predicate.Children {
		validateWorkflowPredicate(add, fmt.Sprintf("%s.children[%d]", prefix, index), &predicate.Children[index], depth+1)
	}
}

func validateWorkflowIterationValue(add func(string, string, ...string), prefix string, value WorkflowIterationValue) {
	switch value.Kind {
	case "string":
		if value.Number != nil || value.Boolean != nil {
			add("TSPACK_WORKFLOW_FOREACH_VALUE_INVALID", prefix+" string value has conflicting scalar fields")
		}
	case "platform":
		if value.String != "linux" && value.String != "windows" && value.String != "macos" && value.String != "currentHost" {
			add("TSPACK_WORKFLOW_PLATFORM_INVALID", prefix+" has invalid platform "+value.String)
		}
	case "number":
		if value.Number == nil || value.Boolean != nil || value.String != "" {
			add("TSPACK_WORKFLOW_FOREACH_VALUE_INVALID", prefix+" must contain exactly one number")
		}
	case "boolean":
		if value.Boolean == nil || value.Number != nil || value.String != "" {
			add("TSPACK_WORKFLOW_FOREACH_VALUE_INVALID", prefix+" must contain exactly one boolean")
		}
	default:
		add("TSPACK_WORKFLOW_FOREACH_VALUE_INVALID", prefix+" has unknown finite scalar kind "+value.Kind)
	}
}

func validateWorkflowValueRef(add func(string, string, ...string), prefix string, value WorkflowValueRef) {
	if value.Identity == "" || value.Source == "" || value.ResultType == "" {
		add("TSPACK_WORKFLOW_VALUE_INVALID", prefix+" must contain deterministic identity, source, and result type")
	}
	switch value.Category {
	case "control", "smallSerialized", "artifactReference", "regionLocal", "placement":
	default:
		add("TSPACK_WORKFLOW_VALUE_CATEGORY_INVALID", prefix+" has unknown category "+value.Category)
	}
}

func validateWorkflowJobs(add func(string, string, ...string), prefix string, workflow *Workflow, packageNames map[string]struct{}) {
	jobs := map[string]*WorkflowJob{}
	for jobIndex := range workflow.Jobs {
		job := &workflow.Jobs[jobIndex]
		jobPrefix := fmt.Sprintf("%s.jobs[%d]", prefix, jobIndex)
		if !workflowIdentityPattern.MatchString(job.Identity) {
			add("TSPACK_WORKFLOW_JOB_IDENTITY_INVALID", jobPrefix+".identity must be a stable identifier")
		}
		if _, exists := jobs[job.Identity]; exists {
			add("TSPACK_WORKFLOW_JOB_IDENTITY_DUPLICATE", "duplicate workflow job identity: "+job.Identity)
		}
		jobs[job.Identity] = job
		if job.RunsOn == "" {
			job.RunsOn = "currentHost"
		}
		if job.RunsOn != "linux" && job.RunsOn != "windows" && job.RunsOn != "macos" && job.RunsOn != "currentHost" {
			add("TSPACK_WORKFLOW_PLATFORM_INVALID", jobPrefix+".runsOn must be linux, windows, macos, or currentHost")
		}
		if len(job.Steps) == 0 {
			add("TSPACK_WORKFLOW_STEPS_REQUIRED", jobPrefix+".steps must not be empty")
		}
		validateWorkflowEnvironment(add, jobPrefix+".env", job.Env)
		validateWorkflowMatrix(add, jobPrefix, job.Matrix)
		for stepIndex, step := range job.Steps {
			validateWorkflowStep(add, fmt.Sprintf("%s.steps[%d]", jobPrefix, stepIndex), step, packageNames)
		}
	}
	if len(workflow.Jobs) == 0 {
		add("TSPACK_WORKFLOW_JOBS_REQUIRED", prefix+".jobs must not be empty")
	}

	for _, job := range workflow.Jobs {
		seenNeeds := map[string]struct{}{}
		for _, dependency := range job.Needs {
			if dependency == job.Identity {
				add("TSPACK_WORKFLOW_JOB_SELF_DEPENDENCY", "workflow job cannot depend on itself: "+job.Identity)
			}
			if _, exists := jobs[dependency]; !exists {
				add("TSPACK_WORKFLOW_JOB_DEPENDENCY_UNKNOWN", job.Identity+" needs unknown job "+dependency)
			}
			if _, exists := seenNeeds[dependency]; exists {
				add("TSPACK_WORKFLOW_JOB_DEPENDENCY_DUPLICATE", job.Identity+" repeats dependency "+dependency)
			}
			seenNeeds[dependency] = struct{}{}
		}
	}
	if cycle := workflowDependencyCycle(workflow.Jobs); len(cycle) > 0 {
		add("TSPACK_WORKFLOW_JOB_DEPENDENCY_CYCLE", "workflow job dependency cycle: "+strings.Join(cycle, " -> "))
	}
}

func validateWorkflowStep(add func(string, string, ...string), prefix string, step WorkflowStep, packageNames map[string]struct{}) {
	for index, input := range step.Inputs {
		validateWorkflowValueRef(add, fmt.Sprintf("%s.inputs[%d]", prefix, index), input)
	}
	switch step.Operation {
	case "sync", "check", "build", "test", "pack", "audit", "transfer":
		if len(step.Command) > 0 || step.Script != "" || step.Cwd != "" || len(step.Env) > 0 || len(step.Capabilities) > 0 {
			add("TSPACK_WORKFLOW_NATIVE_STEP_INVALID", prefix+" native operations cannot contain process, shell, cwd, environment, or capability fields")
		}
		if step.Operation != "pack" && step.Operation != "build" && len(step.Packages) > 0 {
			add("TSPACK_WORKFLOW_TARGETING_UNSUPPORTED", prefix+" package targeting is not supported by this operation")
		}
		if step.Operation != "build" && len(step.Targets) > 0 {
			add("TSPACK_WORKFLOW_TARGETING_UNSUPPORTED", prefix+" target selection is supported only by Build")
		}
		if step.Operation != "test" && step.Filter != "" {
			add("TSPACK_WORKFLOW_TEST_FILTER_INVALID", prefix+" filter is supported only by Test")
		}
		if step.Operation != "audit" && (step.AuditLevel != "" || step.RequireCoverage) {
			add("TSPACK_WORKFLOW_AUDIT_OPTIONS_INVALID", prefix+" audit options are supported only by Audit")
		}
		if step.Operation == "audit" {
			switch step.AuditLevel {
			case "", "any", "low", "moderate", "high", "critical":
			default:
				add("TSPACK_WORKFLOW_AUDIT_OPTIONS_INVALID", prefix+" auditLevel must be any, low, moderate, high, or critical")
			}
		}
		if step.Operation == "transfer" {
			if step.TransferTarget != "linux" && step.TransferTarget != "windows" && step.TransferTarget != "macos" && step.TransferTarget != "currentHost" {
				add("TSPACK_WORKFLOW_TRANSFER_TARGET_INVALID", prefix+" transfer target must be linux, windows, macos, or currentHost")
			}
			if len(step.Inputs) != 1 || step.Inputs[0].Category != "artifactReference" {
				add("TSPACK_WORKFLOW_TRANSFER_INPUT_INVALID", prefix+" transfer requires one artifact reference")
			}
		} else if step.TransferTarget != "" {
			add("TSPACK_WORKFLOW_TRANSFER_TARGET_INVALID", prefix+" transferTarget is supported only by transfer")
		}
	case "process":
		if len(step.Command) == 0 {
			add("TSPACK_WORKFLOW_PROCESS_COMMAND_REQUIRED", prefix+".command must contain an executable and argv")
		}
		for _, argument := range step.Command {
			if strings.TrimSpace(argument) == "" {
				add("TSPACK_WORKFLOW_PROCESS_COMMAND_INVALID", prefix+".command entries must not be empty")
			}
		}
		if step.Script != "" || step.Shell != "" {
			add("TSPACK_WORKFLOW_PROCESS_COMMAND_INVALID", prefix+" process steps cannot contain shell fields")
		}
	case "shellScript":
		if strings.TrimSpace(step.Script) == "" {
			add("TSPACK_WORKFLOW_SHELL_SCRIPT_REQUIRED", prefix+".script must not be empty")
		}
		if step.Shell != "" && step.Shell != "sh" && step.Shell != "powershell" {
			add("TSPACK_WORKFLOW_SHELL_INVALID", prefix+".shell must be sh or powershell")
		}
		if len(step.Command) > 0 {
			add("TSPACK_WORKFLOW_SHELL_INVALID", prefix+" shell steps cannot contain process argv")
		}
	default:
		add("TSPACK_WORKFLOW_STEP_UNSUPPORTED", prefix+".operation is not a supported semantic operation")
	}
	if step.TimeoutSeconds < 0 {
		add("TSPACK_WORKFLOW_TIMEOUT_INVALID", prefix+".timeoutSeconds must be positive")
	}
	if step.Cwd != "" && step.Cwd != "workspace" && !strings.HasPrefix(step.Cwd, "package:") {
		add("TSPACK_WORKFLOW_CWD_INVALID", prefix+".cwd must be workspace or package:<identity>")
	}
	if packageName := strings.TrimPrefix(step.Cwd, "package:"); strings.HasPrefix(step.Cwd, "package:") {
		if _, exists := packageNames[packageName]; !exists {
			add("TSPACK_WORKFLOW_PACKAGE_UNKNOWN", prefix+".cwd selects unknown package "+packageName)
		}
	}
	for _, packageName := range step.Packages {
		if _, exists := packageNames[packageName]; !exists {
			add("TSPACK_WORKFLOW_PACKAGE_UNKNOWN", prefix+".packages selects unknown package "+packageName)
		}
	}
	validateWorkflowEnvironment(add, prefix+".env", step.Env)
	if step.Operation == "process" || step.Operation == "shellScript" {
		if !containsWorkflowCapability(step.Capabilities, "process") {
			add("TSPACK_WORKFLOW_CAPABILITY_REQUIRED", prefix+" external execution must declare process capability")
		}
		if !containsWorkflowCapability(step.Capabilities, "workspaceRead") {
			add("TSPACK_WORKFLOW_CAPABILITY_REQUIRED", prefix+" external execution must declare workspaceRead capability")
		}
		if len(step.Env) > 0 && !containsWorkflowCapability(step.Capabilities, "environment") {
			add("TSPACK_WORKFLOW_CAPABILITY_REQUIRED", prefix+" environment declarations require environment capability")
		}
		for _, environment := range step.Env {
			if environment.Value.Kind == "secret" && !containsWorkflowCapability(step.Capabilities, "secrets") {
				add("TSPACK_WORKFLOW_CAPABILITY_REQUIRED", prefix+" secret references require secrets capability")
			}
		}
	}
	for _, capability := range step.Capabilities {
		switch capability {
		case "network", "workspaceRead", "workspaceWrite", "environment", "secrets", "process":
		default:
			add("TSPACK_WORKFLOW_CAPABILITY_INVALID", prefix+" contains unknown capability "+capability)
		}
	}
}

func containsWorkflowCapability(capabilities []string, requested string) bool {
	for _, capability := range capabilities {
		if capability == requested {
			return true
		}
	}
	return false
}

func validateWorkflowEnvironment(add func(string, string, ...string), prefix string, environment []WorkflowEnvironment) {
	seen := map[string]struct{}{}
	for index, entry := range environment {
		entryPrefix := fmt.Sprintf("%s[%d]", prefix, index)
		if !envNameRe.MatchString(entry.Name) {
			add("TSPACK_WORKFLOW_ENV_INVALID", entryPrefix+".name is invalid")
		}
		key := strings.ToUpper(entry.Name)
		if _, exists := seen[key]; exists {
			add("TSPACK_WORKFLOW_ENV_DUPLICATE", prefix+" contains duplicate environment name "+entry.Name)
		}
		seen[key] = struct{}{}
		switch entry.Value.Kind {
		case "plain":
			if entry.Value.Name != "" {
				add("TSPACK_WORKFLOW_ENV_VALUE_INVALID", entryPrefix+" plain value cannot name a secret")
			}
		case "secret":
			if !envNameRe.MatchString(entry.Value.Name) || entry.Value.Value != "" {
				add("TSPACK_WORKFLOW_SECRET_REFERENCE_INVALID", entryPrefix+" secret value must contain only a valid secret identity")
			}
		default:
			add("TSPACK_WORKFLOW_ENV_VALUE_INVALID", entryPrefix+".value.kind must be plain or secret")
		}
	}
}

func validateWorkflowMatrix(add func(string, string, ...string), prefix string, matrix map[string][]any) {
	for axis, values := range matrix {
		if !workflowIdentityPattern.MatchString(axis) || len(values) == 0 {
			add("TSPACK_WORKFLOW_MATRIX_INVALID", prefix+".matrix axes need stable identities and non-empty values")
		}
		seen := map[string]struct{}{}
		for _, value := range values {
			key := fmt.Sprintf("%T:%v", value, value)
			switch value.(type) {
			case string, float64, bool:
			default:
				add("TSPACK_WORKFLOW_MATRIX_INVALID", prefix+".matrix values must be strings, numbers, booleans, or semantic platform strings")
			}
			if _, exists := seen[key]; exists {
				add("TSPACK_WORKFLOW_MATRIX_INVALID", prefix+".matrix axis "+axis+" contains a duplicate value")
			}
			seen[key] = struct{}{}
		}
	}
}

func workflowDependencyCycle(jobs []WorkflowJob) []string {
	needs := map[string][]string{}
	identities := make([]string, 0, len(jobs))
	for _, job := range jobs {
		needs[job.Identity] = append([]string(nil), job.Needs...)
		identities = append(identities, job.Identity)
	}
	sort.Strings(identities)
	state := map[string]int{}
	stack := []string{}
	var visit func(string) []string
	visit = func(identity string) []string {
		if state[identity] == 1 {
			for index, entry := range stack {
				if entry == identity {
					return append(append([]string(nil), stack[index:]...), identity)
				}
			}
		}
		if state[identity] == 2 {
			return nil
		}
		state[identity] = 1
		stack = append(stack, identity)
		dependencies := append([]string(nil), needs[identity]...)
		sort.Strings(dependencies)
		for _, dependency := range dependencies {
			if cycle := visit(dependency); len(cycle) > 0 {
				return cycle
			}
		}
		stack = stack[:len(stack)-1]
		state[identity] = 2
		return nil
	}
	for _, identity := range identities {
		if cycle := visit(identity); len(cycle) > 0 {
			return cycle
		}
	}
	return nil
}
