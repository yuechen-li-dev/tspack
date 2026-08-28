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

		validateWorkflowJobs(add, prefix, workflow, packageNames)
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
	switch step.Operation {
	case "sync", "check", "build", "test", "pack", "audit":
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
