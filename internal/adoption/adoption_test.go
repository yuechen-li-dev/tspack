package adoption

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestObservePackageJSONFieldsAndLockfiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{
  "name": "existing-app",
  "version": "1.2.3",
  "type": "module",
  "private": true,
  "packageManager": "npm@10.0.0",
  "workspaces": ["packages/*"],
  "main": "dist/index.js",
  "module": "dist/index.mjs",
  "types": "dist/index.d.ts",
  "scripts": {"build": "vite build", "typecheck": "tsc --noEmit"},
  "dependencies": {"react": "^19.0.0"},
  "devDependencies": {"vite": "^7.0.0"},
  "peerDependencies": {"react-dom": "^19.0.0"},
  "optionalDependencies": {"fsevents": "^2.3.3"}
}`)
	writeFile(t, filepath.Join(root, "package-lock.json"), "{}")

	obs, err := Observe(root)
	if err != nil {
		t.Fatalf("Observe returned error: %v", err)
	}
	if obs.Name != "existing-app" || obs.Version != "1.2.3" || obs.Type != "module" {
		t.Fatalf("unexpected package identity: %#v", obs)
	}
	if obs.Private == nil || !*obs.Private {
		t.Fatalf("expected private package to be observed")
	}
	if obs.Workspaces.Kind != "array" || len(obs.Workspaces.Packages) != 1 {
		t.Fatalf("expected workspace array to be observed: %#v", obs.Workspaces)
	}
	if obs.Dependencies["react"] == "" || obs.DevDependencies["vite"] == "" || obs.PeerDependencies["react-dom"] == "" || obs.OptionalDependencies["fsevents"] == "" {
		t.Fatalf("dependency sections were not observed: %#v", obs)
	}
	if obs.Scripts["build"] == "" || obs.Scripts["typecheck"] == "" {
		t.Fatalf("scripts were not observed: %#v", obs.Scripts)
	}
	if len(obs.Lockfiles) != 1 || obs.Lockfiles[0].Name != "package-lock.json" {
		t.Fatalf("lockfile was not detected: %#v", obs.Lockfiles)
	}
}

func TestObserveWorkspaceObject(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"workspaces":{"packages":["apps/*","packages/*"]}}`)
	obs, err := Observe(root)
	if err != nil {
		t.Fatalf("Observe returned error: %v", err)
	}
	if obs.Workspaces.Kind != "object" || len(obs.Workspaces.Packages) != 2 {
		t.Fatalf("workspace object was not observed: %#v", obs.Workspaces)
	}
}

func TestObserveMissingAndMalformedPackageJSONDiagnostics(t *testing.T) {
	_, err := Observe(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "TSPACK_ADOPT_PACKAGE_JSON_MISSING") {
		t.Fatalf("expected missing package.json diagnostic, got %v", err)
	}
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{not json`)
	_, err = Observe(root)
	if err == nil || !strings.Contains(err.Error(), "TSPACK_ADOPT_PACKAGE_JSON_MALFORMED") {
		t.Fatalf("expected malformed package.json diagnostic, got %v", err)
	}
}

func TestBuildReportPackageJSONOnlyWarnings(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"demo","scripts":{"dev":"vite"},"dependencies":{"react":"^19.0.0"}}`)
	obs, err := Observe(root)
	if err != nil {
		t.Fatalf("Observe returned error: %v", err)
	}
	report := BuildReport(obs)
	if report.SuggestedAdoptionMode != "package-json-only" {
		t.Fatalf("unexpected adoption mode: %s", report.SuggestedAdoptionMode)
	}
	if report.DependencyCounts["dependencies"] != 1 || len(report.Scripts) != 1 {
		t.Fatalf("report did not summarize package.json: %#v", report)
	}
	joined := strings.Join(report.Warnings, "\n")
	if !strings.Contains(joined, "not TSPack RunTargets") || !strings.Contains(joined, "no manifest.tsx yet") {
		t.Fatalf("expected read-only adoption warnings, got %q", joined)
	}
	if fileExists(filepath.Join(root, "manifest.tsx")) || fileExists(filepath.Join(root, "ts-lock.toml")) {
		t.Fatalf("report should not write TSPack files")
	}
}

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func TestBuildReportWithPackageAnnotations(t *testing.T) {
	obs := Observation{Root: t.TempDir(), Name: "root"}
	annotations := []PackageAnnotation{{
		Root:             "packages/ui",
		PackageName:      "@acme/ui",
		DependencyCounts: map[string]int{"dep": 1, "peer": 1, "tool": 1},
		Warnings:         []string{"annotation peer(react) differs from package.json section dependencies"},
	}}
	report := BuildReportWithAnnotations(obs, annotations)
	if report.SuggestedAdoptionMode != "observe-with-package-annotations" {
		t.Fatalf("unexpected mode: %s", report.SuggestedAdoptionMode)
	}
	if len(report.PackageAnnotations) != 1 || report.PackageAnnotations[0].DependencyCounts["peer"] != 1 {
		t.Fatalf("annotation metadata missing: %#v", report.PackageAnnotations)
	}
}

func TestBuildReportModelsPackageJSONAsObservedAuthoritativeTapeLayer(t *testing.T) {
	obs := Observation{
		Name:            "demo",
		Dependencies:    map[string]string{"react": "^19"},
		DevDependencies: map[string]string{"vite": "^7"},
	}
	annotations := []PackageAnnotation{{
		Root:           ".",
		ManifestPath:   "package.manifest.tsx",
		AnnotationName: "demo",
		Dependencies: []AnnotatedDep{{
			Key:                "react",
			Name:               "react",
			Kind:               "peer",
			Range:              "^18",
			PackageJSONSection: "dependencies",
			PackageJSONRange:   "^19",
		}},
	}}

	report := BuildReportWithAnnotations(obs, annotations)
	if report.DependencyAuthoring == nil {
		t.Fatal("dependency authoring tape is missing")
	}
	resolution := report.DependencyAuthoring
	if len(resolution.Entries) != 4 {
		t.Fatalf("tape entries = %#v", resolution.Entries)
	}
	annotationEntry := resolution.Entries[0]
	if annotationEntry.Declaration.Origin.Kind != "package-manifest" || annotationEntry.Declaration.Editability != "derived" {
		t.Fatalf("annotation declaration classification = %#v", annotationEntry.Declaration)
	}
	var reactEntryFound bool
	for _, entry := range resolution.Entries {
		if entry.Declaration.Identity.Name != "react" || !entry.Effective {
			continue
		}
		reactEntryFound = true
		if entry.Declaration.Origin.Kind != "compatibility" || entry.Declaration.Authority != "observed" || entry.Declaration.Editability != "observed" {
			t.Fatalf("effective package.json declaration classification = %#v", entry.Declaration)
		}
		if entry.Declaration.Constraint != "^19" {
			t.Fatalf("effective package.json constraint = %q", entry.Declaration.Constraint)
		}
	}
	if !reactEntryFound {
		t.Fatalf("effective react observation missing: %#v", resolution)
	}
}

func TestValidateAnnotatedDependencyWarnings(t *testing.T) {
	warnings := validateAnnotatedDependency(AnnotatedDep{Name: "react", Kind: "peer", Range: "^19.0.0", PackageJSONSection: "dependencies", PackageJSONRange: "^18.0.0"})
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "peer(react)") || !strings.Contains(joined, "range for react") {
		t.Fatalf("expected peer and range warnings, got %q", joined)
	}
	missing := validateAnnotatedDependency(AnnotatedDep{Name: "missing", Kind: "tool"})
	if len(missing) != 1 || !strings.Contains(missing[0], "not present") {
		t.Fatalf("expected missing warning, got %#v", missing)
	}
}
