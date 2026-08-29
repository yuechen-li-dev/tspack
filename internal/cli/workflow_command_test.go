package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/cli/clitest"
	"github.com/yuechen-li-dev/tspack/internal/workflow"
	"gopkg.in/yaml.v3"
)

func TestWorkflowCLIListInspectRunAndExportUseOneManifestPlan(t *testing.T) {
	repository := repoRootForMigrateTest(t)
	sourceRoot := filepath.Join(repository, "fixtures", "workflow-m74")
	workspace := clitest.TempWorkspace(t)
	for _, name := range []string{"manifest.tsx", "ts-lock.toml"} {
		contents, err := os.ReadFile(filepath.Join(sourceRoot, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(workspace.Root, name), contents, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("M74_TOKEN", "cli-test-secret")

	listed := runTestApp(t, "workflow", "list", "--root", workspace.Root, "--json")
	clitest.AssertExit(t, listed, 0)
	if !strings.Contains(listed.Stdout, `"CI"`) {
		t.Fatalf("list output=%s", listed.Stdout)
	}

	inspected := runTestApp(t, "workflow", "inspect", "CI", "--root", workspace.Root, "--json")
	clitest.AssertExit(t, inspected, 0)
	var flow workflow.Flow
	if err := json.Unmarshal([]byte(inspected.Stdout), &flow); err != nil {
		t.Fatalf("decode inspect flow: %v\n%s", err, inspected.Stdout)
	}
	if flow.Identity != "CI" || flow.SchemaVersion != workflow.FlowSchemaVersion || len(flow.Nodes) == 0 {
		t.Fatalf("unexpected flow: %#v", flow)
	}

	run := runTestApp(t, "workflow", "run", "CI", "--root", workspace.Root, "--jobs", "2")
	clitest.AssertExit(t, run, 0)
	if !strings.Contains(run.Stdout, "fixture-ok") || strings.Contains(run.Stdout, "cli-test-secret") {
		t.Fatalf("run output did not preserve redaction contract:\n%s", run.Stdout)
	}

	exported := runTestApp(t, "workflow", "export", "github", "CI", "--root", workspace.Root)
	clitest.AssertExit(t, exported, 0)
	providerPath := filepath.Join(workspace.Root, filepath.FromSlash(workflow.GitHubPath("CI")))
	providerContents, err := os.ReadFile(providerPath)
	if err != nil {
		t.Fatal(err)
	}
	var providerDocument yaml.Node
	if err := yaml.Unmarshal(providerContents, &providerDocument); err != nil {
		t.Fatalf("provider YAML invalid: %v", err)
	}
	if strings.Contains(string(providerContents), "cli-test-secret") || !strings.Contains(string(providerContents), "${{ secrets.M74_TOKEN }}") {
		t.Fatalf("provider secret containment failed:\n%s", providerContents)
	}

	driftCheck := runTestApp(t, "workflow", "export", "github", "CI", "--root", workspace.Root, "--check")
	clitest.AssertExit(t, driftCheck, 0)
}

func TestRenderWorkflowEffectSelectionShowsSemanticTargetIdentity(t *testing.T) {
	build := workflow.PlanStep{
		Operation: "build",
		Packages:  []string{"@vitest/expect"},
		Targets:   []string{"package"},
	}
	if got := renderWorkflowEffectSelection(build); got != "@vitest/expect:package" {
		t.Fatalf("build selection=%q", got)
	}
	test := workflow.PlanStep{
		Operation: "test",
		Packages:  []string{"@vitest/test-unit"},
		Target:    "basic-threads",
	}
	if got := renderWorkflowEffectSelection(test); got != "@vitest/test-unit:basic-threads" {
		t.Fatalf("test selection=%q", got)
	}
}

func TestWorkflowM77FixtureInspectExposesValueAndControlFlow(t *testing.T) {
	repository := repoRootForMigrateTest(t)
	root := filepath.Join(repository, "fixtures", "workflow-m77")

	inspected := runTestApp(t, "workflow", "inspect", "CI", "--root", root, "--json")
	clitest.AssertExit(t, inspected, 0)
	var flow workflow.Flow
	if err := json.Unmarshal([]byte(inspected.Stdout), &flow); err != nil {
		t.Fatalf("decode M77 inspect flow: %v\n%s", err, inspected.Stdout)
	}
	if flow.SchemaVersion != workflow.FlowSchemaVersion || len(flow.Values) == 0 {
		t.Fatalf("M77 flow=%#v", flow)
	}
	if countCLIFlowNodes(flow, workflow.NodeMatch) != 1 || countCLIFlowNodes(flow, workflow.NodeIterator) != 2 {
		t.Fatalf("M77 nodes=%#v", flow.Nodes)
	}

	human := runTestApp(t, "workflow", "inspect", "CI", "--root", root)
	clitest.AssertExit(t, human, 0)
	for _, expected := range []string{"match value/ci/", "foreach suite cursor", "cleanup(", "consumes:", "Values:"} {
		if !strings.Contains(human.Stdout, expected) {
			t.Fatalf("human inspect missing %q:\n%s", expected, human.Stdout)
		}
	}
}

func TestWorkflowM78FixtureInspectExposesFanOutTransportAndFacts(t *testing.T) {
	repository := repoRootForMigrateTest(t)
	root := filepath.Join(repository, "fixtures", "workflow-m78")

	inspected := runTestApp(t, "workflow", "inspect", "M78", "--root", root, "--json")
	clitest.AssertExit(t, inspected, 0)
	var flow workflow.Flow
	if err := json.Unmarshal([]byte(inspected.Stdout), &flow); err != nil {
		t.Fatalf("decode M78 inspect flow: %v\n%s", err, inspected.Stdout)
	}
	if flow.SchemaVersion != workflow.FlowSchemaVersion || len(flow.Aggregates) != 1 || flow.Aggregates[0].Concurrency != 2 || flow.Aggregates[0].FailurePolicy != "collectAll" {
		t.Fatalf("M78 flow=%#v", flow)
	}
	if countCLIFlowNodes(flow, workflow.NodePredicate) != 1 || countCLIFlowNodes(flow, workflow.NodeIterator) != 2 {
		t.Fatalf("M78 nodes=%#v", flow.Nodes)
	}

	human := runTestApp(t, "workflow", "inspect", "M78", "--root", root)
	clitest.AssertExit(t, human, 0)
	for _, expected := range []string{"parallel, concurrency 2, collectAll", "transport target: windows", "when value/m78/", "Aggregates:", "iterationOutcome<test>"} {
		if !strings.Contains(human.Stdout, expected) {
			t.Fatalf("human inspect missing %q:\n%s", expected, human.Stdout)
		}
	}
}

func TestWorkflowM79FixtureInspectExposesAggregateConsumptionAndNestedPaths(t *testing.T) {
	repository := repoRootForMigrateTest(t)
	root := filepath.Join(repository, "fixtures", "workflow-m79")

	inspected := runTestApp(t, "workflow", "inspect", "M79", "--root", root, "--json")
	clitest.AssertExit(t, inspected, 0)
	var flow workflow.Flow
	if err := json.Unmarshal([]byte(inspected.Stdout), &flow); err != nil {
		t.Fatalf("decode M79 inspect flow: %v\n%s", err, inspected.Stdout)
	}
	if flow.SchemaVersion != workflow.FlowSchemaVersion || flow.Expansion.PlannedIterations != 10 || flow.Expansion.Limit != workflow.DefaultExpansionBudget {
		t.Fatalf("M79 expansion=%+v schema=%d", flow.Expansion, flow.SchemaVersion)
	}
	if len(flow.Aggregates) != 1 || !flow.Aggregates[0].Complete || flow.Aggregates[0].ResultType != "iterationOutcome<test>" {
		t.Fatalf("M79 aggregates=%+v", flow.Aggregates)
	}
	projections := 0
	aggregateSources := 0
	nestedPath := false
	for _, node := range flow.Nodes {
		if node.Projection != nil {
			projections++
		}
		if node.Iterator != nil {
			if node.Iterator.SourceAggregate != "" {
				aggregateSources++
			}
			if strings.Contains(node.Iterator.Path, "/") {
				nestedPath = true
			}
		}
	}
	if projections != 3 || aggregateSources != 2 || !nestedPath {
		t.Fatalf("projections=%d aggregateSources=%d nestedPath=%t", projections, aggregateSources, nestedPath)
	}

	human := runTestApp(t, "workflow", "inspect", "M79", "--root", root)
	clitest.AssertExit(t, human, 0)
	for _, expected := range []string{"Expansion: 10/4096", "global concurrency ceiling 32", "consumes value/m79/aggregate", "path platform-config[", "complete"} {
		if !strings.Contains(human.Stdout, expected) {
			t.Fatalf("human inspect missing %q:\n%s", expected, human.Stdout)
		}
	}
}

func countCLIFlowNodes(flow workflow.Flow, kind workflow.NodeKind) int {
	count := 0
	for _, node := range flow.Nodes {
		if node.Kind == kind {
			count++
		}
	}
	return count
}
