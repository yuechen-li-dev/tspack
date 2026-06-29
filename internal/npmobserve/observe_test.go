package npmobserve

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestObserveDetectsRootAndInstalledLifecycleScripts(t *testing.T) {
	root := filepath.Join("testdata", "lifecycle-project")
	report, err := Observe(Options{Root: root})
	if err != nil {
		t.Fatalf("Observe failed: %v", err)
	}

	if len(report.RootScripts) != 1 {
		t.Fatalf("expected one root lifecycle script, got %#v", report.RootScripts)
	}
	if report.RootScripts[0].Phase != "postinstall" {
		t.Fatalf("expected root postinstall, got %#v", report.RootScripts[0])
	}
	if !report.NodeModulesInspected {
		t.Fatalf("expected node_modules inspection")
	}
	if len(report.DependencyScripts) != 5 {
		t.Fatalf("expected five dependency scripts, got %#v", report.DependencyScripts)
	}

	directHook := findScript(t, report.DependencyScripts, "direct-hook", "postinstall")
	if directHook.Presence != PresenceDirect || directHook.Source != SourceInstalledPackage {
		t.Fatalf("expected direct installed hook, got %#v", directHook)
	}
	if got := strings.Join(directHook.DependencySections, ","); got != SectionDependencies {
		t.Fatalf("expected dependencies section, got %q", got)
	}

	transitiveInstall := findScript(t, report.DependencyScripts, "transitive-hook", "install")
	if transitiveInstall.Presence != PresenceTransitive {
		t.Fatalf("expected transitive presence, got %#v", transitiveInstall)
	}
	if len(transitiveInstall.WhyChains) == 0 || strings.Join(transitiveInstall.WhyChains[0], " -> ") != "root -> parent -> transitive-hook" {
		t.Fatalf("expected why chain through parent, got %#v", transitiveInstall.WhyChains)
	}

	optionalHook := findScript(t, report.DependencyScripts, "optional-hook", "prepare")
	if !optionalHook.Optional {
		t.Fatalf("expected optional hook flag, got %#v", optionalHook)
	}

	if !containsExact(report.Limitations, "package-lock.json did not expose dependency lifecycle script details.") {
		t.Fatalf("expected lock metadata limitation, got %#v", report.Limitations)
	}
}

func TestObserveReportsNoNodeModulesLimitation(t *testing.T) {
	root := filepath.Join("testdata", "no-node-modules")
	report, err := Observe(Options{Root: root})
	if err != nil {
		t.Fatalf("Observe failed: %v", err)
	}

	if report.NodeModulesPresent || report.NodeModulesInspected {
		t.Fatalf("expected no node_modules inspection, got %#v", report)
	}
	if len(report.DependencyScripts) != 0 {
		t.Fatalf("expected no dependency scripts without metadata, got %#v", report.DependencyScripts)
	}
	if !containsExact(report.Limitations, "package-lock.json did not expose dependency lifecycle script details.") {
		t.Fatalf("expected missing lock script limitation, got %#v", report.Limitations)
	}
	if !containsSubstring(report.Notes, "rerun `tspack adopt --security`") {
		t.Fatalf("expected rerun note, got %#v", report.Notes)
	}
}

func TestObserveUsesPackageLockScriptsWhenAvailable(t *testing.T) {
	root := filepath.Join("testdata", "lock-scripts-project")
	report, err := Observe(Options{Root: root})
	if err != nil {
		t.Fatalf("Observe failed: %v", err)
	}

	if report.NodeModulesInspected {
		t.Fatalf("did not expect node_modules inspection")
	}
	if len(report.DependencyScripts) != 2 {
		t.Fatalf("expected two lockfile script records, got %#v", report.DependencyScripts)
	}
	for _, script := range report.DependencyScripts {
		if script.Source != SourcePackageLock {
			t.Fatalf("expected package-lock source, got %#v", script)
		}
	}
}

func findScript(t *testing.T, scripts []LifecycleScript, packageName string, phase string) LifecycleScript {
	t.Helper()
	for _, script := range scripts {
		if script.PackageName == packageName && script.Phase == phase {
			return script
		}
	}
	t.Fatalf("script not found: %s %s in %#v", packageName, phase, scripts)
	return LifecycleScript{}
}

func containsExact(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsSubstring(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}
