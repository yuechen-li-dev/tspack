package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tspack/tspack/internal/diag"
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

func TestAppTargetAllowsEmptyTypes(t *testing.T) {
	j := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"app","dependencies":[],"targets":[{"name":"app","export":".","entry":"src/main.ts","runtime":"dist/main.js","types":"","peers":[],"deps":[]}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	_, diags := LoadBytes("x.json", []byte(j))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
}

func TestLibraryTargetRejectsEmptyTypes(t *testing.T) {
	j := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[{"name":"core","export":".","entry":"src/index.ts","runtime":"dist/index.js","types":"","peers":[],"deps":[]}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	_, diags := LoadBytes("x.json", []byte(j))
	found := false
	for _, d := range diags {
		if d.Code == "TSPACK_IR_INVALID_RELATIVE_PATH" {
			found = true
			break
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

func TestRunTargetCwdValidation(t *testing.T) {
	cases := []struct {
		name string
		cwd  string
	}{
		{name: "omitted", cwd: ""},
		{name: "workspace", cwd: `,"cwd":"workspace"`},
		{name: "package", cwd: `,"cwd":"package"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			j := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"runTargets":[{"name":"dev","runtime":"system","command":["node","server.js"],"url":"http://127.0.0.1:5173"` + tc.cwd + `}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
			_, diags := LoadBytes("x.json", []byte(j))
			if len(diags) != 0 {
				t.Fatalf("unexpected diagnostics: %#v", diags)
			}
		})
	}
}

func TestRunTargetInvalidCwd(t *testing.T) {
	j := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"runTargets":[{"name":"dev","runtime":"system","command":["node"],"url":"http://127.0.0.1:5173","cwd":"repo"}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	_, diags := LoadBytes("x.json", []byte(j))
	found := false
	for _, d := range diags {
		if d.Code == "TSPACK_RUN_INVALID_CWD" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TSPACK_RUN_INVALID_CWD, got %#v", diags)
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

func TestBoundaryTransitiveFromValidation(t *testing.T) {
	base := func(boundary string) string {
		return `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"policies":{},"boundaries":[` + boundary + `],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	}

	cases := []struct {
		name     string
		boundary string
		code     string
	}{
		{
			name:     "both scopes",
			boundary: `{"from":"src/index.ts","transitiveFrom":"src/index.ts","denyDeps":["react-dom"]}`,
			code:     "TSPACK_BOUNDARY_INVALID_SCOPE",
		},
		{
			name:     "empty transitiveFrom",
			boundary: `{"transitiveFrom":"","denyDeps":["react-dom"]}`,
			code:     "TSPACK_BOUNDARY_INVALID_TRANSITIVE_FROM",
		},
		{
			name:     "absolute transitiveFrom",
			boundary: `{"transitiveFrom":"/src/index.ts","denyDeps":["react-dom"]}`,
			code:     "TSPACK_BOUNDARY_INVALID_TRANSITIVE_FROM",
		},
		{
			name:     "parent traversal transitiveFrom",
			boundary: `{"transitiveFrom":"../src/index.ts","denyDeps":["react-dom"]}`,
			code:     "TSPACK_BOUNDARY_INVALID_TRANSITIVE_FROM",
		},
		{
			name:     "glob transitiveFrom",
			boundary: `{"transitiveFrom":"src/**","denyDeps":["react-dom"]}`,
			code:     "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags := LoadBytes("x.json", []byte(base(tc.boundary)))
			if tc.code == "" {
				if len(diags) != 0 {
					t.Fatalf("unexpected diagnostics: %#v", diags)
				}
				return
			}
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

func TestBoundaryDenyTypeDepsValidation(t *testing.T) {
	valid := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[{"name":"core","export":".","entry":"src/index.ts","runtime":"dist/index.js","types":"dist/index.d.ts","peers":[],"deps":[]}],"policies":{},"boundaries":[{"from":"src/index.ts","denyTypeDeps":["react-dom"]},{"from":"src/empty.ts","denyTypeDeps":[]}],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	if _, diags := LoadBytes("valid.json", []byte(valid)); len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}

	invalid := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[{"name":"core","export":".","entry":"src/index.ts","runtime":"dist/index.js","types":"dist/index.d.ts","peers":[],"deps":[]}],"policies":{},"boundaries":[{"from":"src/index.ts","denyTypeDeps":[""]}],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	_, diags := LoadBytes("invalid.json", []byte(invalid))
	found := false
	for _, d := range diags {
		if d.Code == "TSPACK_BOUNDARY_INVALID_DENY_TYPE_DEPS" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected TSPACK_BOUNDARY_INVALID_DENY_TYPE_DEPS, got %#v", diags)
	}
}

func TestRunTargetReadyKindValidation(t *testing.T) {
	base := func(ready string) string {
		return `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"runTargets":[{"name":"dev","runtime":"system","command":["node"],"url":"http://127.0.0.1:1","ready":` + ready + `}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	}
	validCases := []struct {
		name  string
		ready string
	}{
		{name: "http", ready: `{"kind":"http","path":"/ready"}`},
		{name: "tcp port", ready: `{"kind":"tcp","port":5432}`},
		{name: "tcp host port", ready: `{"kind":"tcp","host":"127.0.0.1","port":6379}`},
		{name: "stdout pattern", ready: `{"kind":"stdout-match","pattern":"Local:"}`},
		{name: "stdout stream", ready: `{"kind":"stdout-match","pattern":"Local:","stream":"stdout"}`},
		{name: "stderr stream", ready: `{"kind":"stdout-match","pattern":"Local:","stream":"stderr"}`},
		{name: "both stream", ready: `{"kind":"stdout-match","pattern":"Local:","stream":"both"}`},
	}
	for _, tc := range validCases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags := LoadBytes("x.json", []byte(base(tc.ready)))
			if len(diags) != 0 {
				t.Fatalf("unexpected diagnostics: %#v", diags)
			}
		})
	}
	invalidCases := []struct {
		name  string
		ready string
	}{
		{name: "tcp missing port", ready: `{"kind":"tcp"}`},
		{name: "tcp zero port", ready: `{"kind":"tcp","port":0}`},
		{name: "tcp large port", ready: `{"kind":"tcp","port":65536}`},
		{name: "stdout missing pattern", ready: `{"kind":"stdout-match"}`},
		{name: "stdout empty pattern", ready: `{"kind":"stdout-match","pattern":""}`},
		{name: "stdout invalid stream", ready: `{"kind":"stdout-match","pattern":"Local:","stream":"stdin"}`},
	}
	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags := LoadBytes("x.json", []byte(base(tc.ready)))
			found := false
			for _, d := range diags {
				if d.Code == "TSPACK_RUN_INVALID_READY" {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected TSPACK_RUN_INVALID_READY, got %#v", diags)
			}
		})
	}
}

func TestAcknowledgedCapabilityValidation(t *testing.T) {
	valid := `{"format":1,"workspace":{"name":"mono"},"security":{"acknowledgedCapabilities":[{"package":"npm:dep-a@1.0.0","kind":"lifecycleScript","script":"postinstall","command":"node install.js","reason":"Known lifecycle capability; blocked by default."}]},"packages":[{"name":"p","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	ir, diags := LoadBytes("valid", []byte(valid))
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	if len(ir.Security.AcknowledgedCapabilities) != 1 {
		t.Fatalf("expected acknowledgement in IR: %#v", ir.Security)
	}

	cases := []struct {
		name string
		row  string
		code string
	}{
		{"missing reason", `{"package":"npm:dep-a@1.0.0","kind":"lifecycleScript","script":"postinstall","command":"node install.js","reason":""}`, "TSPACK_SECURITY_INVALID_ACKNOWLEDGED_CAPABILITY"},
		{"invalid kind", `{"package":"npm:dep-a@1.0.0","kind":"network","script":"postinstall","command":"node install.js","reason":"x"}`, "TSPACK_SECURITY_INVALID_ACKNOWLEDGED_CAPABILITY"},
		{"invalid script", `{"package":"npm:dep-a@1.0.0","kind":"lifecycleScript","script":"prebad","command":"node install.js","reason":"x"}`, "TSPACK_SECURITY_INVALID_ACKNOWLEDGED_CAPABILITY"},
		{"empty command", `{"package":"npm:dep-a@1.0.0","kind":"lifecycleScript","script":"postinstall","command":"","reason":"x"}`, "TSPACK_SECURITY_INVALID_ACKNOWLEDGED_CAPABILITY"},
		{"absolute fixture", `{"package":"npm:dep-a@1.0.0","kind":"lifecycleScript","script":"postinstall","command":"node install.js","reason":"x","behaviorFixture":"/tmp/probe.xtest.tsx"}`, "TSPACK_SECURITY_INVALID_BEHAVIOR_FIXTURE"},
		{"traversing fixture", `{"package":"npm:dep-a@1.0.0","kind":"lifecycleScript","script":"postinstall","command":"node install.js","reason":"x","behaviorFixture":"../probe.xtest.tsx"}`, "TSPACK_SECURITY_INVALID_BEHAVIOR_FIXTURE"},
		{"wrong fixture extension", `{"package":"npm:dep-a@1.0.0","kind":"lifecycleScript","script":"postinstall","command":"node install.js","reason":"x","behaviorFixture":"security/probe.tsx"}`, "TSPACK_SECURITY_INVALID_BEHAVIOR_FIXTURE"},
		{"traversing report", `{"package":"npm:dep-a@1.0.0","kind":"lifecycleScript","script":"postinstall","command":"node install.js","reason":"x","behaviorReport":"../probe.json"}`, "TSPACK_SECURITY_INVALID_BEHAVIOR_REPORT"},
		{"wrong report extension", `{"package":"npm:dep-a@1.0.0","kind":"lifecycleScript","script":"postinstall","command":"node install.js","reason":"x","behaviorReport":"security/probe.txt"}`, "TSPACK_SECURITY_INVALID_BEHAVIOR_REPORT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jsonText := `{"format":1,"workspace":{"name":"mono"},"security":{"acknowledgedCapabilities":[` + tc.row + `]},"packages":[{"name":"p","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
			_, diags := LoadBytes(tc.name, []byte(jsonText))
			if !hasDiag(diags, tc.code) {
				t.Fatalf("missing %s in %#v", tc.code, diags)
			}
		})
	}

	duplicate := `{"format":1,"workspace":{"name":"mono"},"security":{"acknowledgedCapabilities":[{"package":"npm:dep-a@1.0.0","kind":"lifecycleScript","script":"postinstall","command":"node install.js","reason":"x"},{"package":"npm:dep-a@1.0.0","kind":"lifecycleScript","script":"postinstall","command":"node install.js","reason":"x"}]},"packages":[{"name":"p","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	_, diags = LoadBytes("duplicate", []byte(duplicate))
	if !hasDiag(diags, "TSPACK_SECURITY_DUPLICATE_ACKNOWLEDGED_CAPABILITY") {
		t.Fatalf("missing duplicate diagnostic in %#v", diags)
	}
}

func hasDiag(diags []diag.Diagnostic, code string) bool {
	for _, diagnostic := range diags {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
