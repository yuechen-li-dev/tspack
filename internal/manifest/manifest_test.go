package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidFixtures(t *testing.T) {
	cases := []string{
		"../../fixtures/valid/minimal-library/manifest.ir.golden.json",
		"../../fixtures/valid/machinalayout-like/manifest.ir.golden.json",
		"../../fixtures/valid/git-dep/manifest.ir.golden.json",
		"../../fixtures/valid/m6b-workspace-split/manifest.ir.golden.json",
	}
	for _, c := range cases {
		t.Run(filepath.Base(filepath.Dir(c)), func(t *testing.T) {
			b, err := os.ReadFile(c)
			if err != nil {
				t.Fatal(err)
			}
			ir, diags := LoadBytes(c, b)
			if len(diags) > 0 {
				t.Fatalf("unexpected diagnostics: %#v", diags)
			}
			if ir == nil || ir.Workspace.Name == "" || len(ir.Packages) == 0 {
				t.Fatalf("bad ir")
			}
		})
	}
}

func TestLoadM6BSplitWorkspaceRoots(t *testing.T) {
	b, err := os.ReadFile("../../fixtures/valid/m6b-workspace-split/manifest.ir.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	ir, diags := LoadBytes("m6b", b)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	if ir.Workspace.Name != "m6b-workspace" {
		t.Fatalf("workspace=%q", ir.Workspace.Name)
	}
	if len(ir.Packages) != 2 {
		t.Fatalf("packages=%d", len(ir.Packages))
	}
	roots := map[string]string{}
	for _, p := range ir.Packages {
		roots[p.Name] = p.Root
	}
	if roots["@m6b/core"] != "packages/core" {
		t.Fatalf("core root=%q", roots["@m6b/core"])
	}
	if roots["@m6b/react"] != "packages/react" {
		t.Fatalf("react root=%q", roots["@m6b/react"])
	}
}

func TestInvalidCases(t *testing.T) {
	cases := []struct{ name, json, code string }{
		{"invalid json", `{`, "TSPACK_IR_INVALID_JSON"},
		{"format", `{"format":2,"workspace":{"name":"mono"},"packages":[]}`, "TSPACK_IR_UNSUPPORTED_FORMAT"},
		{"missing workspace", `{"format":1,"workspace":{"name":""},"packages":[{"name":"p","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`, "TSPACK_IR_MISSING_WORKSPACE"},
		{"no packages", `{"format":1,"workspace":{"name":"mono"},"packages":[]}`, "TSPACK_IR_NO_PACKAGES"},
		{"bad package name", `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"bad name","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`, "TSPACK_IR_INVALID_PACKAGE_NAME"},
		{"bad version", `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"banana","kind":"library","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`, "TSPACK_IR_INVALID_PACKAGE_VERSION"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags := LoadBytes("x.json", []byte(tc.json))
			found := false
			for _, d := range diags {
				if d.Code == tc.code {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected code %s got %#v", tc.code, diags)
			}
		})
	}
}

func TestDeterministicDiagnostics(t *testing.T) {
	j := `{"format":2,"workspace":{"name":""},"packages":[]}`
	_, d1 := LoadBytes("x.json", []byte(j))
	_, d2 := LoadBytes("x.json", []byte(j))
	if string(StableDiagnosticsJSON(d1)) != string(StableDiagnosticsJSON(d2)) {
		t.Fatalf("nondeterministic diagnostics")
	}
}

func TestDependencyIdentityDerivationRule(t *testing.T) {
	cases := []struct {
		name string
		dep  DependencyIntent
		want string
	}{
		{name: "explicit key", dep: DependencyIntent{Key: "ts", Source: Source{Package: "typescript"}}, want: "ts"},
		{name: "fallback package", dep: DependencyIntent{Source: Source{Package: "typescript"}}, want: "typescript"},
		{name: "fallback workspace name", dep: DependencyIntent{Source: Source{Name: "core"}}, want: "core"},
		{name: "fallback git ref basename", dep: DependencyIntent{Source: Source{Ref: "github:acme/helper"}}, want: "helper"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := depIdentity(tc.dep)
			if got != tc.want {
				t.Fatalf("depIdentity() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTargetPathsRejectBareDot(t *testing.T) {
	j := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[{"name":"core","export":".","entry":".","runtime":"dist/index.js","types":"dist/index.d.ts","peers":[],"deps":[]}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	_, diags := LoadBytes("x.json", []byte(j))
	found := false
	for _, d := range diags {
		if d.Code == "TSPACK_IR_INVALID_RELATIVE_PATH" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TSPACK_IR_INVALID_RELATIVE_PATH, got %#v", diags)
	}
}

func TestPackageRootAllowsDot(t *testing.T) {
	j := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","root":".","kind":"library","dependencies":[],"targets":[{"name":"core","export":".","entry":"src/index.ts","runtime":"dist/index.js","types":"dist/index.d.ts","peers":[],"deps":[]}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	_, diags := LoadBytes("x.json", []byte(j))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
}

func TestRunTargetValidation(t *testing.T) {
	j := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"runTargets":[{"name":"dev","runtime":"system","command":["node","server.js"],"url":"http://127.0.0.1:5173","ready":{"kind":"http","path":"/"}}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	_, diags := LoadBytes("x.json", []byte(j))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
}

func TestRunTargetInvalidRuntime(t *testing.T) {
	j := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"runTargets":[{"name":"dev","runtime":"bun","command":["node"],"url":"http://127.0.0.1:5173"}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	_, diags := LoadBytes("x.json", []byte(j))
	found := false
	for _, d := range diags {
		if d.Code == "TSPACK_RUN_INVALID_RUNTIME" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TSPACK_RUN_INVALID_RUNTIME, got %#v", diags)
	}
}

func TestRunTargetValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		json string
		code string
	}{
		{"duplicate", `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"runTargets":[{"name":"dev","runtime":"system","command":["node"],"url":"http://127.0.0.1:1"},{"name":"dev","runtime":"system","command":["node"],"url":"http://127.0.0.1:2"}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`, "TSPACK_RUN_DUPLICATE_TARGET"},
		{"empty command", `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"runTargets":[{"name":"dev","runtime":"system","command":[],"url":"http://127.0.0.1:1"}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`, "TSPACK_RUN_INVALID_COMMAND"},
		{"empty argv part", `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"runTargets":[{"name":"dev","runtime":"system","command":["node",""],"url":"http://127.0.0.1:1"}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`, "TSPACK_RUN_INVALID_COMMAND"},
		{"invalid url", `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"runTargets":[{"name":"dev","runtime":"system","command":["node"],"url":"notaurl"}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`, "TSPACK_RUN_INVALID_URL"},
		{"invalid ready kind", `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"runTargets":[{"name":"dev","runtime":"system","command":["node"],"url":"http://127.0.0.1:1","ready":{"kind":"tcp","path":"/"}}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`, "TSPACK_RUN_INVALID_READY"},
		{"invalid ready path", `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"runTargets":[{"name":"dev","runtime":"system","command":["node"],"url":"http://127.0.0.1:1","ready":{"kind":"http","path":"x"}}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`, "TSPACK_RUN_INVALID_READY"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags := LoadBytes("x.json", []byte(tc.json))
			found := false
			for _, d := range diags {
				if d.Code == tc.code {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected %s got %#v", tc.code, diags)
			}
		})
	}
}
