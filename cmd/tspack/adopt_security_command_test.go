package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdoptSecurityFixtureHuman(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := filepath.Join(repo, "internal", "npmobserve", "testdata", "lifecycle-project")

	cmd := exec.Command(bin, "adopt", "--security", "--root", root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("adopt security failed: %v\n%s", err, string(out))
	}
	text := string(out)
	for _, expected := range []string{
		"Observed npm lifecycle/security report",
		"Root package lifecycle scripts",
		"Dependency lifecycle scripts",
		"direct-hook@1.0.0 postinstall",
		"root -> parent -> transitive-hook",
		"package-lock.json did not expose dependency lifecycle script details.",
		"Adoption note:",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("security report missing %q:\n%s", expected, text)
		}
	}
}

func TestAdoptSecurityFixtureJSON(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := filepath.Join(repo, "internal", "npmobserve", "testdata", "lock-scripts-project")

	cmd := exec.Command(bin, "adopt", "--security", "--json", "--root", root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("adopt security json failed: %v\n%s", err, string(out))
	}

	var report struct {
		SourceKind           string `json:"sourceKind"`
		LockfilePresent      bool   `json:"lockfilePresent"`
		NodeModulesInspected bool   `json:"nodeModulesInspected"`
		LifecycleScripts     []struct {
			PackageName string `json:"packageName"`
			Source      string `json:"source"`
		} `json:"lifecycleScripts"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("decode report json: %v\n%s", err, string(out))
	}
	if report.SourceKind != "observed-npm" || !report.LockfilePresent || report.NodeModulesInspected {
		t.Fatalf("unexpected report metadata: %#v", report)
	}
	if len(report.LifecycleScripts) != 2 {
		t.Fatalf("expected two dependency lifecycle scripts, got %#v", report.LifecycleScripts)
	}
	for _, script := range report.LifecycleScripts {
		if script.Source != "package-lock" {
			t.Fatalf("expected package-lock source, got %#v", report.LifecycleScripts)
		}
	}
}

func TestAdoptSecurityDogfoodProjectNoWrites(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := copyDogfoodProject(t, repo)

	cmd := exec.Command(bin, "adopt", "--security", "--root", root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("adopt security dogfood failed: %v\n%s", err, string(out))
	}

	text := string(out)
	if !strings.Contains(text, "Observed npm lifecycle/security report") {
		t.Fatalf("unexpected output:\n%s", text)
	}
	if !strings.Contains(text, "package-lock.json:") {
		t.Fatalf("expected package-lock source:\n%s", text)
	}
	assertNoGeneratedAdoptionFiles(t, root)
}
