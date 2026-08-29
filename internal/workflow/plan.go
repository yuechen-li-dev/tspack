package workflow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

type Plan struct {
	Workflow string    `json:"workflow"`
	Triggers []Trigger `json:"triggers"`
	Jobs     []PlanJob `json:"jobs"`
}

type Trigger struct {
	Kind     string   `json:"kind"`
	Branches []string `json:"branches,omitempty"`
	Paths    []string `json:"paths,omitempty"`
}

type PlanJob struct {
	Identity       string         `json:"identity"`
	JobIdentity    string         `json:"jobIdentity"`
	Needs          []string       `json:"needs,omitempty"`
	Platform       string         `json:"platform"`
	Matrix         map[string]any `json:"matrix,omitempty"`
	Environment    []Environment  `json:"environment,omitempty"`
	Steps          []PlanStep     `json:"steps"`
	DeclarationOrd int            `json:"-"`
}

type PlanStep struct {
	Identity             string          `json:"identity"`
	ResultIdentity       string          `json:"resultIdentity,omitempty"`
	Inputs               []ValueRef      `json:"inputs,omitempty"`
	ResolvedInputs       []ResolvedValue `json:"-"`
	Name                 string          `json:"name"`
	Operation            string          `json:"operation"`
	Packages             []string        `json:"packages,omitempty"`
	Targets              []string        `json:"targets,omitempty"`
	Filter               string          `json:"filter,omitempty"`
	AuditLevel           string          `json:"auditLevel,omitempty"`
	RequireCoverage      bool            `json:"requireCoverage,omitempty"`
	Command              []string        `json:"command,omitempty"`
	Script               string          `json:"script,omitempty"`
	Shell                string          `json:"shell,omitempty"`
	Cwd                  string          `json:"cwd,omitempty"`
	Capabilities         []string        `json:"capabilities,omitempty"`
	Environment          []Environment   `json:"environment,omitempty"`
	TimeoutSeconds       int             `json:"timeoutSeconds,omitempty"`
	TransferTarget       string          `json:"transferTarget,omitempty"`
	TransferSourceRegion string          `json:"transferSourceRegion,omitempty"`
}

type ResolvedValue struct {
	Reference ValueRef
	Result    *NativeResult
}

type Environment struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Value  string `json:"value,omitempty"`
	Secret string `json:"secret,omitempty"`
}

func Find(ir *manifest.ManifestIR, identity string) (*manifest.Workflow, error) {
	for index := range ir.Workflows {
		if ir.Workflows[index].Identity == identity {
			return &ir.Workflows[index], nil
		}
	}
	return nil, fmt.Errorf("workflow %q was not declared", identity)
}

func BuildPlan(declaration manifest.Workflow) Plan {
	plan := Plan{Workflow: declaration.Identity}
	for _, trigger := range declaration.Triggers {
		plan.Triggers = append(plan.Triggers, Trigger{
			Kind:     trigger.Kind,
			Branches: sortedCopy(trigger.Branches),
			Paths:    sortedCopy(trigger.Paths),
		})
	}
	instancesByJob := map[string][]string{}
	for jobIndex, job := range declaration.Jobs {
		combinations := expandMatrix(job.Matrix)
		for _, combination := range combinations {
			identity := matrixIdentity(job.Identity, combination)
			instancesByJob[job.Identity] = append(instancesByJob[job.Identity], identity)
			planned := PlanJob{
				Identity:       identity,
				JobIdentity:    job.Identity,
				Platform:       job.RunsOn,
				Matrix:         combination,
				Environment:    normalizeEnvironment(job.Env),
				DeclarationOrd: jobIndex,
			}
			for stepIndex, step := range job.Steps {
				name := step.Name
				if name == "" {
					name = operationDisplayName(step.Operation)
				}
				capabilities := sortedCopy(step.Capabilities)
				if len(capabilities) == 0 {
					capabilities = nativeCapabilities(step.Operation)
				}
				planned.Steps = append(planned.Steps, PlanStep{
					Identity:        fmt.Sprintf("%s/step-%02d-%s", identity, stepIndex+1, step.Operation),
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
				})
			}
			plan.Jobs = append(plan.Jobs, planned)
		}
	}
	for index := range plan.Jobs {
		declarationJob := declaration.Jobs[plan.Jobs[index].DeclarationOrd]
		for _, dependency := range declarationJob.Needs {
			plan.Jobs[index].Needs = append(plan.Jobs[index].Needs, instancesByJob[dependency]...)
		}
		sort.Strings(plan.Jobs[index].Needs)
	}
	return plan
}

func nativeCapabilities(operation string) []string {
	switch operation {
	case "sync":
		return []string{"network", "workspaceWrite"}
	case "check":
		return []string{"workspaceRead"}
	case "test":
		return []string{"process", "workspaceRead"}
	case "build":
		return []string{"process", "workspaceRead", "workspaceWrite"}
	case "pack":
		return []string{"workspaceRead", "workspaceWrite"}
	case "transfer":
		return []string{"workspaceRead", "workspaceWrite"}
	case "audit":
		return []string{"network", "workspaceRead"}
	default:
		return nil
	}
}

func expandMatrix(matrix map[string][]any) []map[string]any {
	if len(matrix) == 0 {
		return []map[string]any{{}}
	}
	axes := make([]string, 0, len(matrix))
	for axis := range matrix {
		axes = append(axes, axis)
	}
	sort.Strings(axes)
	combinations := []map[string]any{{}}
	for _, axis := range axes {
		next := make([]map[string]any, 0, len(combinations)*len(matrix[axis]))
		for _, combination := range combinations {
			for _, value := range matrix[axis] {
				entry := make(map[string]any, len(combination)+1)
				for key, existing := range combination {
					entry[key] = existing
				}
				entry[axis] = value
				next = append(next, entry)
			}
		}
		combinations = next
	}
	return combinations
}

func matrixIdentity(job string, values map[string]any) string {
	if len(values) == 0 {
		return job
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		encoded, err := json.Marshal(values[key])
		if err != nil {
			encoded = []byte(fmt.Sprintf("%v", values[key]))
		}
		parts = append(parts, fmt.Sprintf("%s=%d:%s", key, len(encoded), encoded))
	}
	return job + "[" + strings.Join(parts, ",") + "]"
}

func normalizeEnvironment(values []manifest.WorkflowEnvironment) []Environment {
	out := make([]Environment, 0, len(values))
	for _, value := range values {
		out = append(out, Environment{
			Name:   value.Name,
			Kind:   value.Value.Kind,
			Value:  value.Value.Value,
			Secret: value.Value.Name,
		})
	}
	sort.Slice(out, func(left, right int) bool {
		return out[left].Name < out[right].Name
	})
	return out
}

func operationDisplayName(operation string) string {
	switch operation {
	case "shellScript":
		return "Shell script"
	default:
		if operation == "" {
			return "Step"
		}
		return strings.ToUpper(operation[:1]) + operation[1:]
	}
}

func sortedCopy(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
