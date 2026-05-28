package boundary

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/graph"
	"github.com/tspack/tspack/internal/manifest"
)

func TestDenyTypeDepsFromExactFile(t *testing.T) {
	root := writeTypeBoundaryFixture(t, map[string]string{
		"src/index.ts": `import type { Foo } from "react-dom";
export const ok = true;
`,
	})
	g := typeBoundaryGraph(t, []manifest.BoundaryRule{{From: "src/index.ts", DenyTypeDeps: []string{"react-dom"}}})

	diags := Check(Options{RootDir: root, Graph: g})
	if !hasDiagnostic(diags, "TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY") {
		t.Fatalf("expected type explicit deny, got %#v", diags)
	}
}

func TestDenyTypeDepsExportTypeAndSubpath(t *testing.T) {
	root := writeTypeBoundaryFixture(t, map[string]string{
		"src/index.ts": `export type { Foo } from "react-dom/client";
`,
	})
	g := typeBoundaryGraph(t, []manifest.BoundaryRule{{From: "src/index.ts", DenyTypeDeps: []string{"react-dom"}}})

	diags := Check(Options{RootDir: root, Graph: g})
	if !hasDiagnostic(diags, "TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY") {
		t.Fatalf("expected subpath type explicit deny, got %#v", diags)
	}
}

func TestDenyTypeDepsDoesNotDenyRuntimeImport(t *testing.T) {
	root := writeTypeBoundaryFixture(t, map[string]string{
		"src/index.ts": `import ReactDOM from "react-dom";
export const ok = ReactDOM;
`,
	})
	g := typeBoundaryGraph(t, []manifest.BoundaryRule{{From: "src/index.ts", DenyTypeDeps: []string{"react-dom"}}})

	diags := Check(Options{RootDir: root, Graph: g})
	if hasDiagnostic(diags, "TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY") {
		t.Fatalf("denyTypeDeps should not emit type diagnostic for runtime import: %#v", diags)
	}
}

func TestDenyTypeDepsTransitiveFromTypeOnlyLocalGraph(t *testing.T) {
	root := writeTypeBoundaryFixture(t, map[string]string{
		"src/index.ts": `export type { Foo } from "./types.js";
`,
		"src/types.ts": `export type { Foo } from "react-dom";
`,
	})
	g := typeBoundaryGraph(t, []manifest.BoundaryRule{{TransitiveFrom: "src/index.ts", DenyTypeDeps: []string{"react-dom"}}})

	diags := Check(Options{RootDir: root, Graph: g})
	if !hasDiagnostic(diags, "TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY") {
		t.Fatalf("expected transitive type explicit deny, got %#v", diags)
	}
	if !diagnosticDetailsContain(diags, "TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY", "path=src/index.ts -> src/types.ts -> react-dom") {
		t.Fatalf("expected transitive path detail, got %#v", diags)
	}
}

func writeTypeBoundaryFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func typeBoundaryGraph(t *testing.T, boundaries []manifest.BoundaryRule) *graph.WorkspaceGraph {
	t.Helper()
	ir := &manifest.ManifestIR{
		Format:    1,
		Workspace: manifest.Workspace{Name: "ws"},
		Packages: []manifest.Package{
			{
				Name:    "app",
				Version: "1.0.0",
				Kind:    "library",
				Dependencies: []manifest.DependencyIntent{
					{Key: "react-dom", Kind: "peer", Source: manifest.Source{Kind: "npm", Package: "react-dom", Range: "^19.0.0"}},
				},
				Targets: []manifest.Target{
					{Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "dist/index.js", Types: "dist/index.d.ts", Peers: []string{"react-dom"}},
				},
				Boundaries: boundaries,
				Publish:    manifest.PublishPolicy{Include: []string{"dist/**"}},
			},
		},
	}
	g, diags := graph.Build(ir)
	if len(diags) > 0 {
		t.Fatalf("graph diagnostics: %#v", diags)
	}
	return g
}

func hasDiagnostic(diags []diag.Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

func diagnosticDetailsContain(diags []diag.Diagnostic, code string, detail string) bool {
	for _, d := range diags {
		if d.Code != code {
			continue
		}
		for _, got := range d.Details {
			if got == detail {
				return true
			}
		}
	}
	return false
}
