package check

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tspack/tspack/internal/graph"
	"github.com/tspack/tspack/internal/manifest"
)

func TestExplainReachabilityRulesAndImportDecisions(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "src", "index.ts"), `import "./button.js";`)
	writeFile(t, filepath.Join(root, "src", "button.tsx"), `
import React from "react";
import "react-dom";
import ts from "typescript";
import "./style-helper.js";
`)
	writeFile(t, filepath.Join(root, "src", "style-helper.ts"), `export const color = "red";`)

	g := explainTestGraph(t, []manifest.BoundaryRule{{From: "src/**", DenyDeps: []string{"react-dom"}}})
	res := Explain(ExplainOptions{RootDir: root, Graph: g, File: "src/button.tsx"})

	if len(res.ReachableFrom) != 1 {
		t.Fatalf("expected one reachable target, got %#v", res.ReachableFrom)
	}
	if res.ReachableFrom[0].Target != "core" {
		t.Fatalf("unexpected target: %#v", res.ReachableFrom[0])
	}
	wantPath := []string{"src/index.ts", "src/button.tsx"}
	if !equalStrings(res.ReachableFrom[0].Path, wantPath) {
		t.Fatalf("path = %#v, want %#v", res.ReachableFrom[0].Path, wantPath)
	}
	if len(res.MatchedRules) != 1 || res.MatchedRules[0].From != "src/**" || !equalStrings(res.MatchedRules[0].DenyDeps, []string{"react-dom"}) {
		t.Fatalf("matched rules = %#v", res.MatchedRules)
	}

	imports := importsBySpecifier(res.Imports)
	assertDecision(t, imports["react"], "allowed", "")
	assertDecision(t, imports["react-dom"], "denied", "TSPACK_BOUNDARY_EXPLICIT_DENY")
	assertDecision(t, imports["typescript"], "denied", "TSPACK_BOUNDARY_TOOL_RUNTIME_IMPORT")
	if imports["./style-helper.js"].Kind != "relative" || imports["./style-helper.js"].Resolved != "src/style-helper.ts" {
		t.Fatalf("relative import = %#v", imports["./style-helper.js"])
	}
}

func TestExplainExactFileRule(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "src", "index.ts"), `import "./button.js";`)
	writeFile(t, filepath.Join(root, "src", "button.tsx"), `import "react-dom";`)
	g := explainTestGraph(t, []manifest.BoundaryRule{{From: "src/button.tsx", DenyDeps: []string{"react-dom"}}})
	res := Explain(ExplainOptions{RootDir: root, Graph: g, File: "src/button.tsx"})
	if len(res.MatchedRules) != 1 || res.MatchedRules[0].From != "src/button.tsx" {
		t.Fatalf("matched rules = %#v", res.MatchedRules)
	}
}

func TestExplainUnreachableFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "src", "index.ts"), `export const used = true;`)
	writeFile(t, filepath.Join(root, "src", "unused.ts"), `import "react";`)
	g := explainTestGraph(t, nil)
	res := Explain(ExplainOptions{RootDir: root, Graph: g, File: "src/unused.ts"})
	if len(res.ReachableFrom) != 0 {
		t.Fatalf("expected unreachable file, got %#v", res.ReachableFrom)
	}
	if len(res.Imports) != 1 || res.Imports[0].Decision != "unknown" {
		t.Fatalf("imports = %#v", res.Imports)
	}
	foundNote := false
	for _, note := range res.Notes {
		if note == "file is not reachable from any declared target entry." {
			foundNote = true
		}
	}
	if !foundNote {
		t.Fatalf("missing unreachable note: %#v", res.Notes)
	}
}

func explainTestGraph(t *testing.T, boundaries []manifest.BoundaryRule) *graph.WorkspaceGraph {
	t.Helper()
	ir := &manifest.ManifestIR{
		Format:    1,
		Workspace: manifest.Workspace{Name: "ws"},
		Packages: []manifest.Package{{
			Name:    "pkg",
			Version: "1.0.0",
			Kind:    "library",
			Dependencies: []manifest.DependencyIntent{
				{Kind: "peer", Source: manifest.Source{Kind: "npm", Package: "react", Range: "^19"}},
				{Kind: "peer", Source: manifest.Source{Kind: "npm", Package: "react-dom", Range: "^19"}},
				{Kind: "tool", Source: manifest.Source{Kind: "npm", Package: "typescript", Range: "^5"}},
			},
			Targets:    []manifest.Target{{Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "dist/index.js", Types: "dist/index.d.ts", Peers: []string{"react"}}},
			Tools:      []string{"typescript"},
			Boundaries: boundaries,
			Publish:    manifest.PublishPolicy{Include: []string{"dist/**"}, Exclude: []string{"src/**"}},
		}},
	}
	g, diags := graph.Build(ir)
	if len(diags) > 0 {
		t.Fatalf("graph diags: %#v", diags)
	}
	return g
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func importsBySpecifier(imports []ExplainImport) map[string]ExplainImport {
	out := map[string]ExplainImport{}
	for _, imp := range imports {
		out[imp.Specifier] = imp
	}
	return out
}

func assertDecision(t *testing.T, imp ExplainImport, decision string, diagnostic string) {
	t.Helper()
	if imp.Decision != decision || imp.Diagnostic != diagnostic {
		t.Fatalf("%s decision=%s diagnostic=%s, want %s/%s", imp.Specifier, imp.Decision, imp.Diagnostic, decision, diagnostic)
	}
}

func equalStrings(a []string, b []string) bool {
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

func TestExplainTransitiveFromMatchedRuleAndDecision(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "src", "index.ts"), `import "./button.js";`)
	writeFile(t, filepath.Join(root, "src", "button.tsx"), `import "react-dom";`)
	g := explainTestGraph(t, []manifest.BoundaryRule{{TransitiveFrom: "src/index.ts", DenyDeps: []string{"react-dom"}}})

	res := Explain(ExplainOptions{RootDir: root, Graph: g, File: "src/button.tsx"})
	if len(res.MatchedRules) != 1 {
		t.Fatalf("matched rules = %#v", res.MatchedRules)
	}
	rule := res.MatchedRules[0]
	if rule.TransitiveFrom != "src/index.ts" || rule.Seed != "src/index.ts" {
		t.Fatalf("matched transitive rule = %#v", rule)
	}
	if !equalStrings(rule.Path, []string{"src/index.ts", "src/button.tsx"}) {
		t.Fatalf("matched transitive path = %#v", rule.Path)
	}

	imports := importsBySpecifier(res.Imports)
	assertDecision(t, imports["react-dom"], "denied", "TSPACK_BOUNDARY_EXPLICIT_DENY")
	if len(imports["react-dom"].Reasons) == 0 || imports["react-dom"].Reasons[0] != "denied by transitive boundary from src/index.ts" {
		t.Fatalf("transitive deny reasons = %#v", imports["react-dom"].Reasons)
	}
}

func TestM33EExplainAllowOnlyViolation(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "src", "index.ts"), `import "./button.js";`)
	writeFile(t, filepath.Join(root, "src", "button.tsx"), `import "react-dom";`)
	g := explainTestGraph(t, []manifest.BoundaryRule{{From: "src/**", AllowOnly: []string{"react"}}})

	res := Explain(ExplainOptions{RootDir: root, Graph: g, File: "src/button.tsx"})
	if len(res.MatchedRules) != 1 || !equalStrings(res.MatchedRules[0].AllowOnly, []string{"react"}) {
		t.Fatalf("matched rules missing allowOnly: %#v", res.MatchedRules)
	}
	imports := importsBySpecifier(res.Imports)
	assertDecision(t, imports["react-dom"], "denied", "TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION")
	if len(imports["react-dom"].Reasons) == 0 || imports["react-dom"].Reasons[0] != "not listed in allowOnly for boundary from src/**" {
		t.Fatalf("allowOnly reasons = %#v", imports["react-dom"].Reasons)
	}
}

func TestM33EExplainTransitiveAllowOnlyViolation(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "src", "index.ts"), `import "./button.js";`)
	writeFile(t, filepath.Join(root, "src", "button.tsx"), `import "react-dom";`)
	g := explainTestGraph(t, []manifest.BoundaryRule{{TransitiveFrom: "src/index.ts", AllowOnly: []string{"react"}}})

	res := Explain(ExplainOptions{RootDir: root, Graph: g, File: "src/button.tsx"})
	if len(res.MatchedRules) != 1 || !equalStrings(res.MatchedRules[0].AllowOnly, []string{"react"}) {
		t.Fatalf("matched transitive rules missing allowOnly: %#v", res.MatchedRules)
	}
	imports := importsBySpecifier(res.Imports)
	assertDecision(t, imports["react-dom"], "denied", "TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION")
	if len(imports["react-dom"].Reasons) == 0 || imports["react-dom"].Reasons[0] != "not listed in allowOnly for transitive boundary from src/index.ts" {
		t.Fatalf("transitive allowOnly reasons = %#v", imports["react-dom"].Reasons)
	}
}

func TestExplainTypeBoundaryDecision(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "src", "index.ts"), `import type { Foo } from "react-dom/client";`)
	g := explainTestGraph(t, []manifest.BoundaryRule{{From: "src/index.ts", DenyTypeDeps: []string{"react-dom"}}})

	res := Explain(ExplainOptions{RootDir: root, Graph: g, File: "src/index.ts"})
	if len(res.MatchedRules) != 1 || !equalStrings(res.MatchedRules[0].DenyTypeDeps, []string{"react-dom"}) {
		t.Fatalf("matched rules = %#v", res.MatchedRules)
	}
	imports := importsBySpecifier(res.Imports)
	imp := imports["react-dom/client"]
	if !imp.TypeOnly || imp.Package != "react-dom" {
		t.Fatalf("type import metadata = %#v", imp)
	}
	assertDecision(t, imp, "denied", "TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY")
}
