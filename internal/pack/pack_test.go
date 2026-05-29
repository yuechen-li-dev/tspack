package pack

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/graph"
	"github.com/tspack/tspack/internal/manifest"
)

func TestPackUnsafeEmptyAndSymlink(t *testing.T) {
	pkg := &graph.PackageNode{Name: "app", Version: "1.0.0", Publish: manifest.PublishPolicy{Include: []string{"../evil"}}}
	r := Pack(t.TempDir(), pkg, Options{DryRun: true})
	if !hasCode(r.Diagnostics, "TSPACK_PACK_INVALID_PUBLISH_PATH") {
		t.Fatalf("expected invalid path")
	}

	d := t.TempDir()
	pkg = &graph.PackageNode{Name: "app", Version: "1.0.0", Publish: manifest.PublishPolicy{Include: []string{"nope/**"}}}
	r = Pack(d, pkg, Options{DryRun: true})
	if !hasCode(r.Diagnostics, "TSPACK_PACK_INCLUDE_MATCHED_NOTHING") {
		t.Fatalf("expected empty include error")
	}
	if !hasSeverity(r.Diagnostics, "TSPACK_PACK_INCLUDE_MATCHED_NOTHING", diag.SeverityError) {
		t.Fatalf("expected empty include diagnostic to be an error: %#v", r.Diagnostics)
	}

	d2 := t.TempDir()
	_ = os.MkdirAll(filepath.Join(d2, "dist"), 0o755)
	_ = os.WriteFile(filepath.Join(d2, "dist", "real.js"), []byte("x"), 0o644)
	if err := os.Symlink(filepath.Join(d2, "dist", "real.js"), filepath.Join(d2, "dist", "link.js")); err == nil {
		pkg = &graph.PackageNode{Name: "app", Version: "1.0.0", Publish: manifest.PublishPolicy{Include: []string{"dist/**"}}}
		r = Pack(d2, pkg, Options{DryRun: true})
		if !hasCode(r.Diagnostics, "TSPACK_PACK_SYMLINK_UNSUPPORTED") {
			t.Fatalf("expected symlink unsupported: %#v", r.Diagnostics)
		}
	}
}

func TestGeneratedPackageJSONMetadata(t *testing.T) {
	react := &graph.DependencyNode{
		Key:  "react",
		Kind: graph.DependencyKindPeer,
		Source: manifest.Source{
			Kind:    "npm",
			Package: "react",
			Range:   ">=18 <20",
		},
	}
	reactDOM := &graph.DependencyNode{
		Key:  "react-dom",
		Kind: graph.DependencyKindPeer,
		Source: manifest.Source{
			Kind:    "npm",
			Package: "react-dom",
			Range:   ">=18 <20",
		},
		Optional: true,
	}
	localPeer := &graph.DependencyNode{
		Key:  "local-peer",
		Kind: graph.DependencyKindPeer,
		Source: manifest.Source{
			Kind: "path",
			Path: "../local-peer",
		},
	}
	pkg := &graph.PackageNode{
		Name:    "pkg",
		Version: "1.2.3",
		License: "MIT",
		Targets: []*graph.TargetNode{
			{
				Name:     "core",
				Export:   ".",
				Runtime:  "dist/index.js",
				Types:    "dist/index.d.ts",
				PeerDeps: []*graph.DependencyNode{reactDOM, react, localPeer},
			},
		},
	}

	packageJSON := generatedPackageJSON(pkg)
	var parsed map[string]any
	if err := json.Unmarshal(packageJSON, &parsed); err != nil {
		t.Fatalf("package.json is not valid JSON: %v", err)
	}
	if parsed["license"] != "MIT" {
		t.Fatalf("missing license: %s", string(packageJSON))
	}
	if parsed["main"] != "./dist/index.js" {
		t.Fatalf("missing normalized main: %s", string(packageJSON))
	}
	if parsed["types"] != "./dist/index.d.ts" {
		t.Fatalf("missing normalized types: %s", string(packageJSON))
	}

	peers := parsed["peerDependencies"].(map[string]any)
	if peers["react"] != ">=18 <20" || peers["react-dom"] != ">=18 <20" {
		t.Fatalf("missing npm peers: %s", string(packageJSON))
	}
	if _, ok := peers["local-peer"]; ok {
		t.Fatalf("path peer should not be emitted as npm peer: %s", string(packageJSON))
	}
	meta := parsed["peerDependenciesMeta"].(map[string]any)
	reactDOMMeta := meta["react-dom"].(map[string]any)
	if reactDOMMeta["optional"] != true {
		t.Fatalf("missing optional peer metadata: %s", string(packageJSON))
	}
	if !bytes.Contains(packageJSON, []byte("\n    \"react\": \">=18 <20\",\n    \"react-dom\": \">=18 <20\"")) {
		t.Fatalf("peerDependencies are not deterministically sorted: %s", string(packageJSON))
	}
}

func TestGeneratedPackageJSONOmitsMainWithoutRootExport(t *testing.T) {
	pkg := &graph.PackageNode{
		Name:    "pkg",
		Version: "1.2.3",
		Targets: []*graph.TargetNode{
			{Export: "./feature", Runtime: "dist/feature.js", Types: "dist/feature.d.ts"},
		},
	}

	packageJSON := generatedPackageJSON(pkg)
	var parsed map[string]any
	if err := json.Unmarshal(packageJSON, &parsed); err != nil {
		t.Fatalf("package.json is not valid JSON: %v", err)
	}
	if _, ok := parsed["main"]; ok {
		t.Fatalf("main should not be emitted without root export: %s", string(packageJSON))
	}
}

func TestPackRejectsNonNPMPeerDependencies(t *testing.T) {
	d := t.TempDir()
	_ = os.MkdirAll(filepath.Join(d, "dist"), 0o755)
	_ = os.WriteFile(filepath.Join(d, "dist", "index.js"), []byte("export{}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(d, "dist", "index.d.ts"), []byte("export declare const x: number;\n"), 0o644)
	localPeer := &graph.DependencyNode{
		Key:  "local-peer",
		Kind: graph.DependencyKindPeer,
		Source: manifest.Source{
			Kind: "path",
			Path: "../local-peer",
		},
	}
	pkg := &graph.PackageNode{
		Name:    "pkg",
		Version: "1.2.3",
		Publish: manifest.PublishPolicy{Include: []string{"dist/**"}},
		Targets: []*graph.TargetNode{
			{
				Export:   ".",
				Runtime:  "dist/index.js",
				Types:    "dist/index.d.ts",
				PeerDeps: []*graph.DependencyNode{localPeer},
			},
		},
	}

	r := Pack(d, pkg, Options{})
	if !hasCode(r.Diagnostics, "TSPACK_PACK_UNPUBLISHABLE_PEER_DEPENDENCY") {
		t.Fatalf("expected unpublishable peer diagnostic: %#v", r.Diagnostics)
	}
	if len(r.Artifacts) != 0 {
		t.Fatalf("unexpected artifact for unpublishable peer")
	}
}

func TestPackArchiveMetadataAndGeneratedPackageJSON(t *testing.T) {
	d := t.TempDir()
	_ = os.MkdirAll(filepath.Join(d, "dist"), 0o755)
	_ = os.WriteFile(filepath.Join(d, "dist", "index.js"), []byte("export{}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(d, "dist", "index.d.ts"), []byte("export declare const x: number;\n"), 0o644)
	react := &graph.DependencyNode{Key: "react", Kind: graph.DependencyKindPeer, Source: manifest.Source{Kind: "npm", Package: "react", Range: ">=18 <20"}}
	reactDOM := &graph.DependencyNode{Key: "react-dom", Kind: graph.DependencyKindPeer, Source: manifest.Source{Kind: "npm", Package: "react-dom", Range: ">=18 <20"}}
	pkg := &graph.PackageNode{
		Name:    "pkg",
		Version: "1.2.3",
		License: "MIT",
		Publish: manifest.PublishPolicy{Include: []string{"dist/**"}},
		Targets: []*graph.TargetNode{
			{
				Export:   ".",
				Runtime:  "dist/index.js",
				Types:    "dist/index.d.ts",
				PeerDeps: []*graph.DependencyNode{reactDOM, react},
			},
		},
	}
	r := Pack(d, pkg, Options{})
	if len(r.Artifacts) != 1 {
		t.Fatalf("missing artifact")
	}
	r2 := Pack(d, pkg, Options{})
	if len(r2.Artifacts) != 1 {
		t.Fatalf("missing second artifact")
	}
	if r.Artifacts[0].Hash != r2.Artifacts[0].Hash {
		t.Fatalf("archive hash is not deterministic: %s != %s", r.Artifacts[0].Hash, r2.Artifacts[0].Hash)
	}
	b, _ := os.ReadFile(r.Artifacts[0].Path)
	gz, _ := gzip.NewReader(bytes.NewReader(b))
	tr := tar.NewReader(gz)
	entries := []string{}
	var packageJSON []byte
	for {
		h, e := tr.Next()
		if e != nil {
			break
		}
		entries = append(entries, h.Name)
		if h.ModTime != time.Unix(0, 0) {
			t.Fatalf("non-normalized mtime")
		}
		if h.Name[:8] != "package/" {
			t.Fatalf("missing prefix")
		}
		if h.Name == "package/package.json" {
			packageJSON, _ = io.ReadAll(tr)
		}
	}
	sorted := append([]string{}, entries...)
	sort.Strings(sorted)
	if !equal(entries, sorted) {
		t.Fatalf("entries not sorted: %v", entries)
	}
	_ = gz.Close()
	if !contains(entries, "package/package.json") {
		t.Fatalf("missing generated package.json")
	}

	var parsed map[string]any
	if err := json.Unmarshal(packageJSON, &parsed); err != nil {
		t.Fatalf("package.json is not valid JSON: %v", err)
	}
	if parsed["license"] != "MIT" || parsed["main"] != "./dist/index.js" {
		t.Fatalf("missing package metadata: %s", string(packageJSON))
	}
	peers := parsed["peerDependencies"].(map[string]any)
	if peers["react"] != ">=18 <20" || peers["react-dom"] != ">=18 <20" {
		t.Fatalf("missing peer dependencies: %s", string(packageJSON))
	}
	if parsed["types"] != "./dist/index.d.ts" {
		t.Fatalf("missing types: %s", string(packageJSON))
	}
	if _, ok := parsed["exports"].(map[string]any)["."]; !ok {
		t.Fatalf("missing exports: %s", string(packageJSON))
	}
}

func hasCode(diags []diag.Diagnostic, c string) bool {
	for _, d := range diags {
		if d.Code == c {
			return true
		}
	}
	return false
}
func hasSeverity(diags []diag.Diagnostic, c string, severity diag.Severity) bool {
	for _, d := range diags {
		if d.Code == c && d.Severity == severity {
			return true
		}
	}
	return false
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPackVerifySuccessfulArtifact(t *testing.T) {
	d := t.TempDir()
	_ = os.MkdirAll(filepath.Join(d, "dist"), 0o755)
	_ = os.WriteFile(filepath.Join(d, "dist", "index.js"), []byte("export{}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(d, "dist", "index.d.ts"), []byte("export declare const x: number;\n"), 0o644)

	react := &graph.DependencyNode{Key: "react", Kind: graph.DependencyKindPeer, Source: manifest.Source{Kind: "npm", Package: "react", Range: ">=18 <20"}}
	reactDOM := &graph.DependencyNode{Key: "react-dom", Kind: graph.DependencyKindPeer, Source: manifest.Source{Kind: "npm", Package: "react-dom", Range: ">=18 <20"}, Optional: true}
	pkg := &graph.PackageNode{
		Name:    "pkg",
		Version: "1.2.3",
		License: "MIT",
		Publish: manifest.PublishPolicy{Include: []string{"dist/**"}},
		Targets: []*graph.TargetNode{{
			Export:   ".",
			Runtime:  "dist/index.js",
			Types:    "dist/index.d.ts",
			PeerDeps: []*graph.DependencyNode{reactDOM, react},
		}},
	}

	r := Pack(d, pkg, Options{Verify: true})
	if hasErrors(r.Diagnostics) {
		t.Fatalf("verify pack failed: %#v", r.Diagnostics)
	}
	if len(r.Artifacts) != 1 || !r.Artifacts[0].Verified {
		t.Fatalf("expected one verified artifact: %#v", r.Artifacts)
	}
	mustFileExist(t, r.Artifacts[0].Path)

	r2 := Pack(d, pkg, Options{Verify: true})
	if hasErrors(r2.Diagnostics) || len(r2.Artifacts) != 1 {
		t.Fatalf("second verify pack failed: %#v", r2.Diagnostics)
	}
	if r.Artifacts[0].Hash != r2.Artifacts[0].Hash {
		t.Fatalf("verified archive hash is not deterministic: %s != %s", r.Artifacts[0].Hash, r2.Artifacts[0].Hash)
	}
}

func TestVerifyArchiveDiagnostics(t *testing.T) {
	basePlan := verificationTestPlan()

	t.Run("missing package json", func(t *testing.T) {
		path := writeVerificationArchive(t, basePlan, map[string]string{"dist/index.js": "export{}\n", "dist/index.d.ts": "export{}\n"})
		diagnostics := VerifyArchive(path, basePlan)
		if !hasCode(diagnostics, "TSPACK_PACK_VERIFY_PACKAGE_JSON_MISSING") {
			t.Fatalf("expected missing package.json diagnostic: %#v", diagnostics)
		}
	})

	t.Run("invalid package json", func(t *testing.T) {
		path := writeVerificationArchive(t, basePlan, map[string]string{"package.json": "{", "dist/index.js": "export{}\n", "dist/index.d.ts": "export{}\n"})
		diagnostics := VerifyArchive(path, basePlan)
		if !hasCode(diagnostics, "TSPACK_PACK_VERIFY_PACKAGE_JSON_INVALID") {
			t.Fatalf("expected invalid package.json diagnostic: %#v", diagnostics)
		}
	})

	t.Run("missing main file", func(t *testing.T) {
		packageJSON := verificationPackageJSON("./dist/index.js", true)
		path := writeVerificationArchive(t, basePlan, map[string]string{"package.json": packageJSON, "dist/index.d.ts": "export{}\n"})
		diagnostics := VerifyArchive(path, basePlan)
		if !hasCode(diagnostics, "TSPACK_PACK_VERIFY_MISSING_FILE") || !diagnosticContains(diagnostics, "field=main") {
			t.Fatalf("expected missing main diagnostic: %#v", diagnostics)
		}
	})

	t.Run("missing types file", func(t *testing.T) {
		packageJSON := verificationPackageJSON("./dist/index.js", true)
		path := writeVerificationArchive(t, basePlan, map[string]string{"package.json": packageJSON, "dist/index.js": "export{}\n"})
		diagnostics := VerifyArchive(path, basePlan)
		if !hasCode(diagnostics, "TSPACK_PACK_VERIFY_MISSING_FILE") || !diagnosticContains(diagnostics, "field=types") {
			t.Fatalf("expected missing types diagnostic: %#v", diagnostics)
		}
	})

	t.Run("missing peer dependency metadata", func(t *testing.T) {
		packageJSON := verificationPackageJSON("./dist/index.js", false)
		path := writeVerificationArchive(t, basePlan, map[string]string{"package.json": packageJSON, "dist/index.js": "export{}\n", "dist/index.d.ts": "export{}\n"})
		diagnostics := VerifyArchive(path, basePlan)
		if !hasCode(diagnostics, "TSPACK_PACK_VERIFY_METADATA_MISMATCH") || !diagnosticContains(diagnostics, "field=peerDependencies.react") {
			t.Fatalf("expected peer metadata mismatch: %#v", diagnostics)
		}
	})

	t.Run("invalid package path", func(t *testing.T) {
		packageJSON := verificationPackageJSON("../dist/index.js", true)
		path := writeVerificationArchive(t, basePlan, map[string]string{"package.json": packageJSON, "dist/index.js": "export{}\n", "dist/index.d.ts": "export{}\n"})
		diagnostics := VerifyArchive(path, basePlan)
		if !hasCode(diagnostics, "TSPACK_PACK_VERIFY_INVALID_PACKAGE_PATH") || !diagnosticContains(diagnostics, "field=main") {
			t.Fatalf("expected invalid path diagnostic: %#v", diagnostics)
		}
	})
}

func TestWritePlansVerifyFailureLeavesNoFinalArtifacts(t *testing.T) {
	d := t.TempDir()
	goodPath := filepath.Join(d, "good.tgz")
	badPath := filepath.Join(d, "bad.tgz")
	goodPlan := verificationPlanWithPath(goodPath)
	badPlan := verificationPlanWithPath(badPath)
	badPlan.files["package.json"] = matchedFile{source: "<generated>", archive: "package.json", reason: "test", mode: 0o644, size: 2, data: []byte("{}")}

	result := WritePlans([]Plan{goodPlan, badPlan}, Options{Verify: true})
	if !hasCode(result.Diagnostics, "TSPACK_PACK_VERIFY_METADATA_MISMATCH") {
		t.Fatalf("expected verification failure: %#v", result.Diagnostics)
	}
	if len(result.Artifacts) != 0 {
		t.Fatalf("verification failure should not report artifacts: %#v", result.Artifacts)
	}
	if _, err := os.Stat(goodPath); !os.IsNotExist(err) {
		t.Fatalf("good final artifact should not remain after multi-package verify failure")
	}
	if _, err := os.Stat(badPath); !os.IsNotExist(err) {
		t.Fatalf("bad final artifact should not remain after verify failure")
	}
}

func verificationTestPlan() Plan {
	return Plan{
		PackageName: "pkg",
		Version:     "1.2.3",
		License:     "MIT",
		rootTarget:  &plannedTarget{Export: ".", Runtime: "dist/index.js", Types: "dist/index.d.ts"},
		targets:     []plannedTarget{{Export: ".", Runtime: "dist/index.js", Types: "dist/index.d.ts"}},
		peerDeps:    map[string]string{"react": ">=18 <20"},
	}
}

func verificationPlanWithPath(path string) Plan {
	plan := verificationTestPlan()
	plan.Path = path
	packageJSON := verificationPackageJSON("./dist/index.js", true)
	plan.files = map[string]matchedFile{
		"package.json":    {source: "<generated>", archive: "package.json", reason: "test", mode: 0o644, size: int64(len(packageJSON)), data: []byte(packageJSON)},
		"dist/index.js":   {source: "<generated>", archive: "dist/index.js", reason: "test", mode: 0o644, size: 9, data: []byte("export{}\n")},
		"dist/index.d.ts": {source: "<generated>", archive: "dist/index.d.ts", reason: "test", mode: 0o644, size: 9, data: []byte("export{}\n")},
	}
	plan.keys = []string{"dist/index.d.ts", "dist/index.js", "package.json"}
	return plan
}

func verificationPackageJSON(mainPath string, includePeers bool) string {
	packageJSON := map[string]any{
		"name":    "pkg",
		"version": "1.2.3",
		"license": "MIT",
		"main":    mainPath,
		"types":   "./dist/index.d.ts",
		"exports": map[string]any{
			".": map[string]any{
				"types":   "./dist/index.d.ts",
				"default": "./dist/index.js",
			},
		},
	}
	if includePeers {
		packageJSON["peerDependencies"] = map[string]string{
			"react": ">=18 <20",
		}
	}

	body, _ := json.Marshal(packageJSON)
	return string(body)
}

func writeVerificationArchive(t *testing.T, plan Plan, files map[string]string) string {
	t.Helper()
	plan.files = map[string]matchedFile{}
	plan.keys = make([]string, 0, len(files))
	for name, body := range files {
		plan.files[name] = matchedFile{source: "<generated>", archive: name, reason: "test", mode: 0o644, size: int64(len(body)), data: []byte(body)}
		plan.keys = append(plan.keys, name)
	}
	sort.Strings(plan.keys)
	archive, err := buildArchive(plan)
	if err != nil {
		t.Fatalf("build archive: %v", err)
	}
	path := filepath.Join(t.TempDir(), "artifact.tgz")
	if err := os.WriteFile(path, archive, 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	return path
}

func diagnosticContains(diags []diag.Diagnostic, needle string) bool {
	for _, diagnostic := range diags {
		for _, detail := range diagnostic.Details {
			if detail == needle {
				return true
			}
		}
	}
	return false
}

func mustFileExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
}
