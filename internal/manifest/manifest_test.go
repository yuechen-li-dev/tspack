package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/diag"
)

func hasDiagnosticCode(diagnostics []diag.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

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
		{"no packages or compat files", `{"format":1,"workspace":{"name":"mono"},"packages":[]}`, "TSPACK_IR_NO_PACKAGES"},
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

func TestBuildArtifactSetsAndQualifiedDependenciesValidate(t *testing.T) {
	valid := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"base","version":"1.0.0","kind":"library","dependencies":[],"targets":[{"name":"package","compiler":"rollup","export":".","entry":"src/index.ts","runtime":"dist/index.js","types":"dist/index.d.ts","artifacts":[{"name":"index-js","kind":"javaScript","role":"runtimeEntry","path":"dist/index.js"},{"name":"shared-dts","kind":"typeDeclarations","role":"declarationChunk","path":"dist/index.d-*.d.ts"}],"peers":[],"deps":[]}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}},{"name":"app","version":"1.0.0","kind":"app","dependencies":[],"targets":[{"name":"package","export":".","entry":"src/index.ts","runtime":"dist/index.js","types":"dist/index.d.ts","dependsOn":["base:package"],"peers":[],"deps":[]}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":[],"exclude":[]}}]}`
	if _, diagnostics := LoadBytes("valid.json", []byte(valid)); len(diagnostics) != 0 {
		t.Fatalf("diagnostics=%v", diagnostics)
	}

	unknown := strings.Replace(valid, `"base:package"`, `"missing:package"`, 1)
	if _, diagnostics := LoadBytes("unknown.json", []byte(unknown)); !hasDiagnosticCode(diagnostics, "TSPACK_COMPILER_TARGET_DEPENDENCY_UNKNOWN") {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	self := strings.Replace(valid, `"base:package"`, `"package"`, 1)
	if _, diagnostics := LoadBytes("self.json", []byte(self)); !hasDiagnosticCode(diagnostics, "TSPACK_COMPILER_TARGET_DEPENDENCY_CYCLE") {
		t.Fatalf("diagnostics=%v", diagnostics)
	}

	cycle := strings.Replace(valid, `"peers":[],"deps":[]}],"policies"`, `"dependsOn":["app:package"],"peers":[],"deps":[]}],"policies"`, 1)
	if _, diagnostics := LoadBytes("cycle.json", []byte(cycle)); !hasDiagnosticCode(diagnostics, "TSPACK_COMPILER_TARGET_DEPENDENCY_CYCLE") {
		t.Fatalf("diagnostics=%v", diagnostics)
	}

	duplicateArtifact := strings.Replace(valid, `"name":"shared-dts"`, `"name":"index-js"`, 1)
	if _, diagnostics := LoadBytes("duplicate.json", []byte(duplicateArtifact)); !hasDiagnosticCode(diagnostics, "TSPACK_COMPILER_ARTIFACT_DUPLICATE") {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
}

func TestTestTargetRequirementsAndLocalFixturesValidate(t *testing.T) {
	valid := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"tests","version":"1.0.0","kind":"app","dependencies":[{"key":"sinon","kind":"tool","source":{"kind":"npm","package":"sinon","range":"^22.0.0"}},{"key":"http-client","kind":"tool","source":{"kind":"path","path":"projects/http-client"}}],"targets":[],"testTargets":[{"name":"unit","harness":"vitest","sources":["test/unit.test.ts"],"requirements":["sinon","http-client"],"fixtures":[{"name":"http-client","dependency":"http-client","binding":"http-client","mode":"source"}]}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":[],"exclude":[]}}]}`
	if _, diagnostics := LoadBytes("valid.json", []byte(valid)); len(diagnostics) != 0 {
		t.Fatalf("diagnostics=%v", diagnostics)
	}

	cases := []struct {
		name string
		old  string
		new  string
		code string
	}{
		{name: "unknown requirement", old: `"sinon","http-client"`, new: `"missing","http-client"`, code: "TSPACK_TEST_REQUIREMENT_UNKNOWN"},
		{name: "missing fixture requirement", old: `"sinon","http-client"`, new: `"sinon"`, code: "TSPACK_TEST_FIXTURE_REQUIREMENT_MISSING"},
		{name: "invalid fixture binding", old: `"binding":"http-client"`, new: `"binding":"../escape"`, code: "TSPACK_TEST_FIXTURE_BINDING_INVALID"},
		{name: "invalid fixture mode", old: `"mode":"source"`, new: `"mode":"mutable"`, code: "TSPACK_TEST_FIXTURE_MODE_INVALID"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			contents := strings.Replace(valid, testCase.old, testCase.new, 1)
			if _, diagnostics := LoadBytes("invalid.json", []byte(contents)); !hasDiagnosticCode(diagnostics, testCase.code) {
				t.Fatalf("diagnostics=%v", diagnostics)
			}
		})
	}
}

func TestTestTargetBuildPrerequisitesAndBuiltFixturesValidate(t *testing.T) {
	valid := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"runtime","version":"1.0.0","kind":"library","dependencies":[],"targets":[{"name":"package","compiler":"rollup","export":".","entry":"src/index.ts","runtime":"dist/index.js","types":"dist/index.d.ts","artifacts":[{"name":"runtime-js","kind":"javaScript","path":"dist/*.js"}],"peers":[],"deps":[]}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}},{"name":"tests","version":"1.0.0","kind":"app","dependencies":[],"targets":[],"testTargets":[{"name":"unit","harness":"vitest","sources":["test/unit.test.ts"],"dependsOn":["runtime:package"],"builtFixtures":[{"name":"runtime","target":"runtime:package","artifact":"runtime-js","binding":"@demo/runtime"}]}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":[],"exclude":[]}}]}`
	if _, diagnostics := LoadBytes("valid.json", []byte(valid)); len(diagnostics) != 0 {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	cases := []struct {
		name string
		old  string
		new  string
		code string
	}{
		{name: "unknown target", old: `"dependsOn":["runtime:package"]`, new: `"dependsOn":["runtime:missing"]`, code: "TSPACK_TEST_BUILD_DEPENDENCY_UNKNOWN"},
		{name: "missing requirement", old: `"dependsOn":["runtime:package"]`, new: `"dependsOn":[]`, code: "TSPACK_TEST_BUILT_FIXTURE_REQUIREMENT_MISSING"},
		{name: "unknown artifact", old: `"artifact":"runtime-js"`, new: `"artifact":"missing"`, code: "TSPACK_TEST_BUILT_FIXTURE_ARTIFACT_UNKNOWN"},
		{name: "invalid binding", old: `"binding":"@demo/runtime"`, new: `"binding":"../runtime"`, code: "TSPACK_TEST_FIXTURE_BINDING_INVALID"},
		{name: "duplicate build dependency", old: `"dependsOn":["runtime:package"]`, new: `"dependsOn":["runtime:package","runtime:package"]`, code: "TSPACK_TEST_BUILD_DEPENDENCY_DUPLICATE"},
		{name: "duplicate fixture name", old: `"builtFixtures":[{"name":"runtime"`, new: `"fixtures":[{"name":"runtime","producer":"runtime","binding":"runtime","mode":"package"}],"builtFixtures":[{"name":"runtime"`, code: "TSPACK_TEST_FIXTURE_DUPLICATE"},
		{name: "duplicate fixture binding", old: `"builtFixtures":[{"name":"runtime"`, new: `"fixtures":[{"name":"source","producer":"runtime","binding":"@demo/runtime","mode":"package"}],"builtFixtures":[{"name":"runtime"`, code: "TSPACK_TEST_FIXTURE_BINDING_DUPLICATE"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			contents := strings.Replace(valid, testCase.old, testCase.new, 1)
			if _, diagnostics := LoadBytes("invalid.json", []byte(contents)); !hasDiagnosticCode(diagnostics, testCase.code) {
				t.Fatalf("diagnostics=%v", diagnostics)
			}
		})
	}
}

func TestCompilerSelectionDefaultsToTscAndRestrictsTscl(t *testing.T) {
	base := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"app","version":"1.0.0","kind":"app","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":[],"exclude":[]}%s}]}`

	ir, diagnostics := LoadBytes("default.json", []byte(fmt.Sprintf(base, "")))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected default compiler diagnostics: %#v", diagnostics)
	}
	if ir.Packages[0].Compiler != "tsc" {
		t.Fatalf("compiler=%q, want tsc", ir.Packages[0].Compiler)
	}

	_, diagnostics = LoadBytes("invalid.json", []byte(fmt.Sprintf(base, `,"compiler":"bun"`)))
	if !hasDiagnosticCode(diagnostics, "TSPACK_MANIFEST_INVALID_COMPILER") {
		t.Fatalf("missing invalid compiler diagnostic: %#v", diagnostics)
	}

	_, diagnostics = LoadBytes("missing-path.json", []byte(fmt.Sprintf(base, `,"compiler":"tscl"`)))
	if !hasDiagnosticCode(diagnostics, "TSPACK_TSCL_PATH_REQUIRED") {
		t.Fatalf("missing tscl path diagnostic: %#v", diagnostics)
	}
}

func TestCompilerSelectionCanVaryByTarget(t *testing.T) {
	contents := []byte(`{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"app","version":"1.0.0","kind":"app","compiler":"tsc","compilerPath":"tscl","dependencies":[],"targets":[{"name":"web","compiler":"tsc","compilerConfig":"tsconfig.json","export":".","entry":"src/web.ts","runtime":"dist/web.js","types":"","peers":[],"deps":[]},{"name":"domain","compiler":"tscl","compilerConfig":"tsconfig.tsx","export":"./domain","entry":"src/domain.ts","runtime":"dist/domain.js","types":"","peers":[],"deps":[]}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":[],"exclude":[]}}]}`)
	ir, diagnostics := LoadBytes("multi-compiler.json", contents)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	if ir.Packages[0].Targets[0].Compiler != "tsc" || ir.Packages[0].Targets[1].Compiler != "tscl" {
		t.Fatalf("target compiler selection was collapsed: %#v", ir.Packages[0].Targets)
	}
	if ir.Packages[0].Targets[0].Language != "typescript" || ir.Packages[0].Targets[1].Language != "copeland-ts" {
		t.Fatalf("target language identity was not defaulted by compiler: %#v", ir.Packages[0].Targets)
	}
}

func TestScriptCTargetRequiresBoundedInputsAndDefaultsNativeArtifact(t *testing.T) {
	contents := []byte(`{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"app","version":"1.0.0","kind":"app","dependencies":[],"targets":[{"name":"hot","compiler":"scriptc","compilerConfig":"scriptc.json","inputs":["src/hot/**"],"export":"./hot","entry":"src/hot/main.ts","runtime":"dist/hot.exe","types":"","peers":[],"deps":[]}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":[],"exclude":[]}}]}`)
	ir, diagnostics := LoadBytes("scriptc.json", contents)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	target := ir.Packages[0].Targets[0]
	if target.Language != "scriptc" || target.Artifact != "nativeExecutable" {
		t.Fatalf("unexpected ScriptC defaults: %#v", target)
	}
}

func TestPerryTargetRequiresBoundedInputsAndDefaultsNativeArtifact(t *testing.T) {
	contents := []byte(`{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"app","version":"1.0.0","kind":"app","dependencies":[],"targets":[{"name":"hot","compiler":"perry","compilerConfig":"perry.json","inputs":["src/hot/**"],"export":"./hot","entry":"src/hot/main.ts","runtime":"dist/hot.exe","types":"","peers":[],"deps":[]}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":[],"exclude":[]}}]}`)
	ir, diagnostics := LoadBytes("perry.json", contents)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	target := ir.Packages[0].Targets[0]
	if target.Language != "perry-ts" || target.Artifact != "nativeExecutable" {
		t.Fatalf("unexpected Perry defaults: %#v", target)
	}
}

func TestRootCompatOnlyManifestValidates(t *testing.T) {
	_, diags := LoadBytes("x.json", []byte(`{"format":1,"workspace":{"name":"mono"},"compatFiles":[{"format":"json","path":"tsconfig.tspack.json","value":{}}],"packages":[]}`))
	if len(diags) != 0 {
		t.Fatalf("root compat-only manifest should validate: %#v", diags)
	}
}

func TestWorkspaceRuntimeProfiles(t *testing.T) {
	cases := []string{"nodejs", "bun", "deno"}
	for _, runtimeProfile := range cases {
		t.Run(runtimeProfile, func(t *testing.T) {
			j := `{"format":1,"workspace":{"name":"mono","runtime":"` + runtimeProfile + `"},"packages":[{"name":"ok","version":"1.0.0","kind":"app","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
			ir, diags := LoadBytes("x.json", []byte(j))
			if len(diags) != 0 {
				t.Fatalf("unexpected diagnostics: %#v", diags)
			}
			if ir.Workspace.Runtime != runtimeProfile {
				t.Fatalf("runtime=%q", ir.Workspace.Runtime)
			}
		})
	}
}

func TestWorkspaceRuntimeProfileDefaultsToNodejs(t *testing.T) {
	j := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"app","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	ir, diags := LoadBytes("x.json", []byte(j))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	if ir.Workspace.Runtime != "nodejs" {
		t.Fatalf("runtime=%q", ir.Workspace.Runtime)
	}
}

func TestNodejsRuntimeBaselineFixtureEquivalence(t *testing.T) {
	omitted := loadRuntimeBaselineFixture(t, "runtime-baseline-omitted")
	explicit := loadRuntimeBaselineFixture(t, "runtime-baseline-nodejs")

	if omitted.Workspace.Runtime != "nodejs" {
		t.Fatalf("omitted runtime normalized to %q", omitted.Workspace.Runtime)
	}
	if explicit.Workspace.Runtime != "nodejs" {
		t.Fatalf("explicit runtime normalized to %q", explicit.Workspace.Runtime)
	}
	if !reflect.DeepEqual(omitted, explicit) {
		t.Fatalf("omitted and explicit nodejs IR differ:\nomitted=%#v\nexplicit=%#v", omitted, explicit)
	}
}

func loadRuntimeBaselineFixture(t *testing.T, name string) *ManifestIR {
	t.Helper()
	path := filepath.Join("..", "..", "fixtures", "valid", name, "manifest.ir.golden.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ir, diags := LoadBytes(path, b)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics for %s: %#v", name, diags)
	}
	return ir
}

func TestWorkspaceRuntimeProfileRejectsInvalidValues(t *testing.T) {
	j := `{"format":1,"workspace":{"name":"mono","runtime":"npm"},"packages":[{"name":"ok","version":"1.0.0","kind":"app","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	_, diags := LoadBytes("x.json", []byte(j))
	found := false
	for _, d := range diags {
		if d.Code == "TSPACK_MANIFEST_INVALID_RUNTIME_PROFILE" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TSPACK_MANIFEST_INVALID_RUNTIME_PROFILE, got %#v", diags)
	}
}

func TestPackageKindsValidate(t *testing.T) {
	cases := []struct {
		name string
		kind string
	}{
		{name: "library", kind: "library"},
		{name: "app", kind: "app"},
		{name: "tool", kind: "tool"},
		{name: "service", kind: "service"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			j := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"` + tc.kind + `","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
			ir, diags := LoadBytes("x.json", []byte(j))
			if len(diags) != 0 {
				t.Fatalf("unexpected diagnostics: %#v", diags)
			}
			if ir.Packages[0].Kind != tc.kind {
				t.Fatalf("kind was not preserved: got %q", ir.Packages[0].Kind)
			}
		})
	}
}

func TestPackageKindRejectsUnknown(t *testing.T) {
	j := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"worker","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":[],"exclude":[]}}]}`
	_, diags := LoadBytes("x.json", []byte(j))
	if !hasDiagnosticCode(diags, "TSPACK_IR_INVALID_PACKAGE_KIND") {
		t.Fatalf("expected TSPACK_IR_INVALID_PACKAGE_KIND, got %#v", diags)
	}
}

func TestServicePackageRunTargetEnvAndServiceRequirementsValidate(t *testing.T) {
	j := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"api","version":"1.0.0","kind":"service","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":[],"exclude":[]},"runTargets":[{"name":"dev","runtime":"node","command":["tsx","src/server.ts"],"url":"http://127.0.0.1:3000","env":[{"name":"DATABASE_URL","required":true,"secret":true}],"requires":[{"kind":"service","name":"postgres","tcp":"127.0.0.1:5432"}]}]}]}`
	_, diags := LoadBytes("x.json", []byte(j))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
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

func TestScopedToolExplicitKeyReferenceValidates(t *testing.T) {
	j := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[{"key":"@biomejs/biome","kind":"tool","source":{"kind":"npm","package":"@biomejs/biome","range":"^1.9.4"}}],"targets":[{"name":"core","export":".","entry":"src/index.ts","runtime":"dist/index.js","types":"dist/index.d.ts","peers":[],"deps":[]}],"policies":{},"boundaries":[],"tools":["@biomejs/biome"],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	_, diags := LoadBytes("x.json", []byte(j))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
}

func TestJSRDependencySourceValidates(t *testing.T) {
	j := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"app","version":"1.0.0","kind":"library","dependencies":[{"key":"path","kind":"dep","source":{"kind":"jsr","package":"@std/path","range":"^1.1.0"}}],"targets":[{"name":"core","export":".","entry":"src/index.ts","runtime":"dist/index.js","types":"dist/index.d.ts","peers":[],"deps":["path"]}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	_, diagnostics := LoadBytes("jsr.json", []byte(j))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected JSR diagnostics: %#v", diagnostics)
	}
}

func TestUnscopedToolAliasReferenceValidates(t *testing.T) {
	j := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[{"kind":"tool","source":{"kind":"npm","package":"typescript","range":"^5.0.0"}}],"targets":[{"name":"core","export":".","entry":"src/index.ts","runtime":"dist/index.js","types":"dist/index.d.ts","peers":[],"deps":[]}],"policies":{},"boundaries":[],"tools":["typescript"],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	_, diags := LoadBytes("x.json", []byte(j))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
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

func TestRunTargetDirectRuntimeValidation(t *testing.T) {
	for _, runtimeName := range []string{"bun", "deno"} {
		t.Run(runtimeName, func(t *testing.T) {
			j := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"runTargets":[{"name":"dev","runtime":"` + runtimeName + `","command":["server.js"],"url":"http://127.0.0.1:5173"}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
			_, diags := LoadBytes("x.json", []byte(j))
			if len(diags) != 0 {
				t.Fatalf("unexpected diagnostics for %s runtime: %#v", runtimeName, diags)
			}
		})
	}
}

func TestRunTargetInvalidRuntime(t *testing.T) {
	j := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"runTargets":[{"name":"dev","runtime":"npm","command":["server.ts"],"url":"http://127.0.0.1:5173"}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
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

func TestAcknowledgedLifecycleCategoryValidation(t *testing.T) {
	valid := `{"format":1,"workspace":{"name":"mono"},"security":{"acknowledgedLifecycleCategories":[{"category":"maintainer-publish","scripts":["prepare"],"reason":"Maintainer lifecycle scripts are blocked."}]},"packages":[{"name":"p","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	if _, diagnostics := LoadBytes("manifest.json", []byte(valid)); len(diagnostics) > 0 {
		t.Fatalf("valid lifecycle category acknowledgment rejected: %#v", diagnostics)
	}

	invalidCategory := `{"format":1,"workspace":{"name":"mono"},"security":{"acknowledgedLifecycleCategories":[{"category":"maintainer","reason":"bad"}]},"packages":[{"name":"p","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	if _, diagnostics := LoadBytes("manifest.json", []byte(invalidCategory)); !hasDiagnosticCode(diagnostics, "TSPACK_SECURITY_INVALID_ACKNOWLEDGED_LIFECYCLE_CATEGORY") {
		t.Fatalf("expected invalid lifecycle category diagnostic: %#v", diagnostics)
	}

	missingReason := `{"format":1,"workspace":{"name":"mono"},"security":{"acknowledgedLifecycleCategories":[{"category":"maintainer-publish"}]},"packages":[{"name":"p","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	if _, diagnostics := LoadBytes("manifest.json", []byte(missingReason)); !hasDiagnosticCode(diagnostics, "TSPACK_SECURITY_INVALID_ACKNOWLEDGED_LIFECYCLE_CATEGORY") {
		t.Fatalf("expected missing reason diagnostic: %#v", diagnostics)
	}
}

func TestUpdatePolicyValidation(t *testing.T) {
	base := func(policy string) string {
		return `{"format":1,"workspace":{"name":"mono"},"updatePolicy":` + policy + `,"packages":[{"name":"p","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	}
	valid := `{"rows":[{"name":"typescript","kind":"tool","strategy":"rolling","level":"minor","reason":"tooling can roll"},{"name":"react","kind":"dep","strategy":"manual"}]}`
	ir, diags := LoadBytes("valid", []byte(base(valid)))
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	if len(ir.UpdatePolicy.Rows) != 2 || ir.UpdatePolicy.Rows[0].Name != "typescript" {
		t.Fatalf("unexpected update policy IR: %#v", ir.UpdatePolicy)
	}

	cases := []struct {
		name   string
		policy string
		code   string
	}{
		{"invalid strategy", `{"rows":[{"name":"typescript","kind":"tool","strategy":"auto","level":"minor"}]}`, "TSPACK_UPDATE_POLICY_INVALID_STRATEGY"},
		{"invalid level", `{"rows":[{"name":"typescript","kind":"tool","strategy":"rolling","level":"week"}]}`, "TSPACK_UPDATE_POLICY_INVALID_LEVEL"},
		{"rolling missing level", `{"rows":[{"name":"typescript","kind":"tool","strategy":"rolling"}]}`, "TSPACK_UPDATE_POLICY_INVALID_LEVEL"},
		{"manual level", `{"rows":[{"name":"react","kind":"dep","strategy":"manual","level":"minor"}]}`, "TSPACK_UPDATE_POLICY_LEVEL_NOT_ALLOWED"},
		{"duplicate row", `{"rows":[{"name":"react","kind":"dep","strategy":"manual"},{"name":"react","kind":"dep","strategy":"pinned"}]}`, "TSPACK_UPDATE_POLICY_DUPLICATE_ROW"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags := LoadBytes(tc.name, []byte(base(tc.policy)))
			if !hasDiag(diags, tc.code) {
				t.Fatalf("missing %s in %#v", tc.code, diags)
			}
		})
	}
}

func TestManifestRejectsReservedPyPISourceKind(t *testing.T) {
	j := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[{"key":"requests","kind":"dep","source":{"kind":"pypi","package":"requests","range":">=2"}}],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	_, diags := LoadBytes("test.json", []byte(j))
	if !hasDiagnosticCode(diags, "TSPACK_IR_INVALID_SOURCE_KIND") {
		t.Fatalf("expected reserved pypi source to be rejected: %#v", diags)
	}
}

func TestRunTargetEnvValidation(t *testing.T) {
	valid := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"app","dependencies":[],"targets":[],"runTargets":[{"name":"dev","runtime":"system","command":["node"],"url":"http://127.0.0.1:1","env":[{"name":"DATABASE_URL","required":true,"secret":true,"description":"db"},{"name":"PORT","default":"3000"}]}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	if _, diags := LoadBytes("manifest.json", []byte(valid)); len(diags) != 0 {
		t.Fatalf("valid env declarations produced diagnostics: %#v", diags)
	}

	cases := []struct {
		name string
		json string
		code string
	}{
		{"invalid", `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"app","dependencies":[],"targets":[],"runTargets":[{"name":"dev","runtime":"system","command":["node"],"url":"http://127.0.0.1:1","env":[{"name":"1BAD"}]}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`, "TSPACK_MANIFEST_ENV_INVALID"},
		{"duplicate", `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"app","dependencies":[],"targets":[],"runTargets":[{"name":"dev","runtime":"system","command":["node"],"url":"http://127.0.0.1:1","env":[{"name":"PORT"},{"name":"port"}]}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`, "TSPACK_MANIFEST_ENV_DUPLICATE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags := LoadBytes("manifest.json", []byte(tc.json))
			if !hasDiagnosticCode(diags, tc.code) {
				t.Fatalf("expected %s, got %#v", tc.code, diags)
			}
		})
	}
}

func TestRunTargetServiceRequirementValidation(t *testing.T) {
	base := func(requires string) string {
		return `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"app","dependencies":[],"targets":[],"runTargets":[{"name":"dev","runtime":"system","command":["node"],"url":"http://127.0.0.1:1","requires":` + requires + `}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	}
	validCases := []string{
		`[{"kind":"service","name":"postgres","tcp":"127.0.0.1:5432","timeoutMs":1000}]`,
		`[{"kind":"service","name":"api","http":"http://127.0.0.1:8080/health","expectStatus":200,"optional":true}]`,
	}
	for _, input := range validCases {
		if _, diags := LoadBytes("x.json", []byte(base(input))); len(diags) != 0 {
			t.Fatalf("valid service requirement rejected: diags=%v", diags)
		}
	}
	invalidCases := []struct {
		name string
		req  string
		code string
	}{
		{"missing endpoint", `[{"kind":"service","name":"postgres"}]`, "TSPACK_MANIFEST_SERVICE_INVALID"},
		{"both endpoints", `[{"kind":"service","name":"postgres","tcp":"127.0.0.1:5432","http":"http://127.0.0.1:5432"}]`, "TSPACK_MANIFEST_SERVICE_INVALID"},
		{"bad tcp", `[{"kind":"service","name":"postgres","tcp":"127.0.0.1:99999"}]`, "TSPACK_MANIFEST_SERVICE_INVALID"},
		{"bad http", `[{"kind":"service","name":"api","http":"ftp://127.0.0.1/health"}]`, "TSPACK_MANIFEST_SERVICE_INVALID"},
		{"bad status", `[{"kind":"service","name":"api","http":"http://127.0.0.1/health","expectStatus":99}]`, "TSPACK_MANIFEST_SERVICE_INVALID"},
		{"bad timeout", `[{"kind":"service","name":"api","http":"http://127.0.0.1/health","timeoutMs":60001}]`, "TSPACK_MANIFEST_SERVICE_INVALID"},
		{"duplicate", `[{"kind":"service","name":"api","http":"http://127.0.0.1/a"},{"kind":"service","name":"API","http":"http://127.0.0.1/b"}]`, "TSPACK_MANIFEST_SERVICE_DUPLICATE"},
	}
	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags := LoadBytes("x.json", []byte(base(tc.req)))
			if !hasDiagnosticCode(diags, tc.code) {
				t.Fatalf("expected %s, got %v", tc.code, diags)
			}
		})
	}
}

func TestRunTargetURLAllowsEnvPlaceholders(t *testing.T) {
	valid := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"runTargets":[{"name":"dev","runtime":"system","command":["node"],"url":"http://127.0.0.1:${PORT}","ready":{"kind":"http","path":"/health"}}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	_, diags := LoadBytes("x.json", []byte(valid))
	if len(diags) != 0 {
		t.Fatalf("expected env placeholder URL to validate, got %#v", diags)
	}

	invalid := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"runTargets":[{"name":"dev","runtime":"system","command":["node"],"url":"http://127.0.0.1:${BAD-NAME}","ready":{"kind":"http","path":"/health"}}],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`
	_, diags = LoadBytes("x.json", []byte(invalid))
	if !hasDiag(diags, "TSPACK_RUN_INVALID_URL") {
		t.Fatalf("expected invalid placeholder URL diagnostic, got %#v", diags)
	}
}

func TestWorkflowValidationRejectsUnknownNeedsCyclesAndUnsafeEffects(t *testing.T) {
	base := `{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"app","version":"1.0.0","kind":"app","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":[],"exclude":[]}}],"workflows":[%s]}`
	cases := []struct {
		name     string
		workflow string
		code     string
	}{
		{
			name:     "unknown dependency",
			workflow: `{"identity":"CI","triggers":[{"kind":"push"}],"jobs":[{"identity":"test","needs":["missing"],"steps":[{"operation":"check"}]}]}`,
			code:     "TSPACK_WORKFLOW_JOB_DEPENDENCY_UNKNOWN",
		},
		{
			name:     "cycle",
			workflow: `{"identity":"CI","triggers":[{"kind":"manual"}],"jobs":[{"identity":"one","needs":["two"],"steps":[{"operation":"check"}]},{"identity":"two","needs":["one"],"steps":[{"operation":"sync"}]}]}`,
			code:     "TSPACK_WORKFLOW_JOB_DEPENDENCY_CYCLE",
		},
		{
			name:     "empty argv",
			workflow: `{"identity":"CI","triggers":[{"kind":"push"}],"jobs":[{"identity":"test","steps":[{"operation":"process","name":"bad","command":[]}]}]}`,
			code:     "TSPACK_WORKFLOW_PROCESS_COMMAND_REQUIRED",
		},
		{
			name:     "secret value",
			workflow: `{"identity":"CI","triggers":[{"kind":"push"}],"jobs":[{"identity":"test","steps":[{"operation":"process","name":"bad","command":["tool"],"env":[{"name":"TOKEN","value":{"kind":"secret","name":"CI_TOKEN","value":"leak"}}]}]}]}`,
			code:     "TSPACK_WORKFLOW_SECRET_REFERENCE_INVALID",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, diagnostics := LoadBytes("manifest.json", []byte(fmt.Sprintf(base, testCase.workflow)))
			if !hasDiagnosticCode(diagnostics, testCase.code) {
				t.Fatalf("expected %s, got %#v", testCase.code, diagnostics)
			}
		})
	}
}
