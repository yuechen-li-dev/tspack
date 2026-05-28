package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tspack/tspack/internal/manifest"
)

func TestCheckExplainValidationDiagnostics(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, filepath.Join(root, "README.md"), "# demo\n")
	writeProjectFile(t, filepath.Join(root, "outside.ts"), "export {};\n")
	manifestPath := writeExplainManifestIR(t, root)
	opts := DefaultOptions(root)
	opts.ManifestIRPath = manifestPath

	cases := []struct {
		name string
		file string
		code string
	}{
		{name: "missing", file: "src/missing.ts", code: "TSPACK_CHECK_EXPLAIN_FILE_NOT_FOUND"},
		{name: "outside", file: "../outside.ts", code: "TSPACK_CHECK_EXPLAIN_FILE_OUTSIDE_ROOT"},
		{name: "unsupported", file: "README.md", code: "TSPACK_CHECK_EXPLAIN_UNSUPPORTED_FILE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := CheckExplain(opts, tc.file)
			if len(res.Diagnostics) == 0 || res.Diagnostics[0].Code != tc.code {
				t.Fatalf("diagnostics = %#v, want %s", res.Diagnostics, tc.code)
			}
			if res.Explain != nil {
				t.Fatalf("expected no explain result for validation error: %#v", res.Explain)
			}
		})
	}
}

func TestCheckExplainJSONMarshalShape(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, filepath.Join(root, "src", "index.ts"), `import "react";`)
	manifestPath := writeExplainManifestIR(t, root)
	opts := DefaultOptions(root)
	opts.ManifestIRPath = manifestPath
	res := CheckExplain(opts, "src/index.ts")
	if len(res.Diagnostics) != 0 || res.Explain == nil {
		t.Fatalf("result diagnostics=%#v explain=%#v", res.Diagnostics, res.Explain)
	}
	data, err := json.MarshalIndent(res.Explain, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"command": "check"`, `"mode": "explain"`, `"reachableFrom"`, `"matchedRules"`, `"imports"`, `"diagnostics"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("JSON missing %s: %s", want, text)
		}
	}
}

func writeExplainManifestIR(t *testing.T, root string) string {
	t.Helper()
	ir := manifest.ManifestIR{
		Format:    1,
		Workspace: manifest.Workspace{Name: "ws"},
		Packages: []manifest.Package{{
			Name:         "pkg",
			Version:      "1.0.0",
			Kind:         "library",
			Dependencies: []manifest.DependencyIntent{{Kind: "peer", Source: manifest.Source{Kind: "npm", Package: "react", Range: "^19"}}},
			Targets:      []manifest.Target{{Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "dist/index.js", Types: "dist/index.d.ts", Peers: []string{"react"}}},
			Publish:      manifest.PublishPolicy{Include: []string{"dist/**"}, Exclude: []string{"src/**"}},
		}},
	}
	data, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "manifest.ir.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeProjectFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
