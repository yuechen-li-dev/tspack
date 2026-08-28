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
	var plan workflow.Plan
	if err := json.Unmarshal([]byte(inspected.Stdout), &plan); err != nil {
		t.Fatalf("decode inspect plan: %v\n%s", err, inspected.Stdout)
	}
	if plan.Workflow != "CI" || len(plan.Jobs) != 2 || plan.Jobs[1].Needs[0] != "validate" {
		t.Fatalf("unexpected plan: %#v", plan)
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
