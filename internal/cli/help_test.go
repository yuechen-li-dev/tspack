package cli

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runTSPackForHelpTest(t *testing.T, args ...string) string {
	t.Helper()
	repo := filepath.Join("..", "..")
	cmd := exec.Command(testTspackBinary, args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tspack %v failed: %v\n%s", args, err, string(out))
	}
	return string(out)
}

func assertContainsAll(t *testing.T, text string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestDefaultHelpIsSemantic(t *testing.T) {
	text := runTSPackForHelpTest(t)
	assertContainsAll(t, text, "TypeScript-first", "Common workflows", "Create a project", "tspack help workflow")
	if strings.Contains(text, "M0 scaffold") {
		t.Fatalf("default help should not mention M0 scaffold:\n%s", text)
	}
}

func TestCategorizedCommandsHelp(t *testing.T) {
	text := runTSPackForHelpTest(t, "help", "commands")
	assertContainsAll(t, text, "Project setup", "Dependency lifecycle", "Validation and diagnostics", "Execution and testing", "Packaging")
}

func TestWorkflowHelp(t *testing.T) {
	text := runTSPackForHelpTest(t, "help", "workflow")
	assertContainsAll(t, text, "init", "update", "sync", "check", "run", "pack", "TSPack project lifecycle")
}

func TestConceptsHelp(t *testing.T) {
	text := runTSPackForHelpTest(t, "help", "concepts")
	assertContainsAll(t, text, "Manifest", "Lockfile", "Templates", "Security", "RunTargets")
}

func TestInitHelpIsSemantic(t *testing.T) {
	text := runTSPackForHelpTest(t, "help", "init")
	assertContainsAll(t, text, "--template static", "--template react", "--template react-library", "Templates do not run commands")
}

func TestInitHelpAlias(t *testing.T) {
	helpTopic := runTSPackForHelpTest(t, "help", "init")
	helpAlias := runTSPackForHelpTest(t, "init", "--help")
	assertContainsAll(t, helpAlias, "tspack init", "Templates do not run commands", "react-library")
	if helpTopic != helpAlias {
		t.Fatalf("init --help should match help init\nhelp init:\n%s\ninit --help:\n%s", helpTopic, helpAlias)
	}
}

func TestCheckHelpAliasMentionsDiagnostics(t *testing.T) {
	text := runTSPackForHelpTest(t, "check", "--help")
	assertContainsAll(t, text, "Validates manifest", "--show-conflicts", "--show-lifecycle", "--format", "tspack how")
}

func TestSyncHelpMentionsHydrationBehavior(t *testing.T) {
	text := runTSPackForHelpTest(t, "help", "sync")
	assertContainsAll(t, text, "Materializes dependencies from ts-lock.toml.", "hydrates missing local store artifacts", "without changing versions or rewriting ts-lock.toml", "hardlink-first writes", "immutable generated output", "skips relinking when node_modules already matches", "--force")
}

func TestExhaustiveHelpStillIncludesFlags(t *testing.T) {
	text := runTSPackForHelpTest(t, "help", "all")
	assertContainsAll(t, text, "Usage:", "--show-conflicts", "--show-lifecycle", "tspack check [--root .]")
}

func TestInitNextStepHints(t *testing.T) {
	repo := filepath.Join("..", "..")
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "static",
			args: []string{"init", "--template", "static", "--name", "hello-static"},
			want: []string{"Created TSPack project: hello-static", "Next:", "tspack update", "tspack sync", "tspack check", "tspack check --format"},
		},
		{
			name: "react",
			args: []string{"init", "--template", "react", "--name", "hello-react"},
			want: []string{"Created TSPack project: hello-react", "Next:", "tspack update", "tspack sync", "tspack run dev"},
		},
		{
			name: "react-library",
			args: []string{"init", "--template", "react-library", "--name", "ui-kit", "--package", "@local/ui-kit"},
			want: []string{"Created TSPack project: ui-kit", "Next:", "tspack update", "tspack sync", "tspack run typecheck", "tspack run build", "tspack run build-types", "tspack pack --verify --package @local/ui-kit"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			args := append([]string{}, tc.args...)
			args = append(args, "--root", root)
			cmd := exec.Command(testTspackBinary, args...)
			cmd.Dir = repo
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("init failed: %v\n%s", err, string(out))
			}
			assertContainsAll(t, string(out), tc.want...)
			if strings.Contains(string(out), "npm install") || strings.Contains(string(out), "pnpm install") {
				t.Fatalf("init output should not imply command execution:\n%s", string(out))
			}
		})
	}
}
