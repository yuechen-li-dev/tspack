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
	if len(report.DependencyScripts) != 6 {
		t.Fatalf("expected six dependency scripts, got %#v", report.DependencyScripts)
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

	directWarning := findWarning(t, report.CapabilityWarnings, "direct-hook", "postinstall")
	if directWarning.CapabilityID != "install-time-code-execution" || !containsExact(directWarning.Tags, "direct-dependency-install-hook") {
		t.Fatalf("expected direct install-time capability warning, got %#v", directWarning)
	}

	transitiveWarning := findWarning(t, report.CapabilityWarnings, "transitive-hook", "install")
	if transitiveWarning.CapabilityID != "install-time-code-execution" || !containsExact(transitiveWarning.Tags, "transitive-dependency-install-hook") {
		t.Fatalf("expected transitive install-time capability warning, got %#v", transitiveWarning)
	}
	if len(transitiveWarning.Chains) == 0 || strings.Join(transitiveWarning.Chains[0], " -> ") != "root -> parent -> transitive-hook" {
		t.Fatalf("expected transitive warning why chain, got %#v", transitiveWarning.Chains)
	}

	optionalWarning := findWarning(t, report.CapabilityWarnings, "optional-hook", "prepare")
	if !containsExact(optionalWarning.Tags, "optional-dependency-install-hook") || !containsExact(optionalWarning.Flags, "optional") {
		t.Fatalf("expected optional install hook warning, got %#v", optionalWarning)
	}

	rootWarning := findWarning(t, report.CapabilityWarnings, "fixture-app", "postinstall")
	if rootWarning.CapabilityID != "root-install-lifecycle" || rootWarning.Presence != PresenceRoot {
		t.Fatalf("expected root lifecycle capability, got %#v", rootWarning)
	}

	if report.Summary.InstallTimeHooks != 6 || report.Summary.DirectHooks != 3 || report.Summary.TransitiveHooks != 2 || report.Summary.OptionalHooks != 1 || report.Summary.RootLifecycleScripts != 1 {
		t.Fatalf("unexpected lifecycle summary: %#v", report.Summary)
	}

	if !containsExact(report.Limitations, "package-lock.json did not expose dependency lifecycle script details.") {
		t.Fatalf("expected lock metadata limitation, got %#v", report.Limitations)
	}
}

func TestObserveClassifiesPublishPackLifecycle(t *testing.T) {
	root := filepath.Join("testdata", "lifecycle-project")
	report, err := Observe(Options{Root: root})
	if err != nil {
		t.Fatalf("Observe failed: %v", err)
	}

	warning := findWarning(t, report.CapabilityWarnings, "dev-hook", "prepack")
	if warning.CapabilityID != "publish-or-pack-lifecycle" || warning.AttentionLevel != "info" {
		t.Fatalf("expected publish/pack classification, got %#v", warning)
	}
	if !strings.Contains(warning.Meaning, "pack or publish") {
		t.Fatalf("expected publish/pack meaning, got %q", warning.Meaning)
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

func findWarning(t *testing.T, warnings []CapabilityWarning, packageName string, phase string) CapabilityWarning {
	t.Helper()
	for _, warning := range warnings {
		if warning.PackageName == packageName && warning.Phase == phase {
			return warning
		}
	}
	t.Fatalf("warning not found: %s %s in %#v", packageName, phase, warnings)
	return CapabilityWarning{}
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
