package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tspack/tspack/internal/lockfile"
	"github.com/tspack/tspack/internal/manifest"
)

func writeManifestStub(t *testing.T, repo string) {
	t.Helper()
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const out={ok:true,ir:{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"library",dependencies:[],targets:[{name:"core",export:".",entry:"src/index.ts",runtime:"dist/index.js",types:"dist/index.d.ts",deps:[],peers:[]}],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{types:{},boundaries:{}},runTargets:[{name:"dev",runtime:"node",url:"http://127.0.0.1:5173",command:["node","server.js"]}]}]},diagnostics:[]};
process.stdout.write(JSON.stringify(out));`
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })
}

func TestDoctorHelpAndJson(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeManifestStub(t, repo)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, "security"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "security", "ack-postinstall.valid.xtest.tsx"), []byte("// fixture is evidence only\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "security", "ack-postinstall.report.json"), []byte(`{"ok":true,"violations":[]}`), 0o644)
	_ = os.WriteFile(filepath.Join(root, "ts-lock.toml"), []byte("[lock]\nformat=1\n"), 0o644)

	cmd := exec.Command("go", "run", "./cmd/tspack", "help")
	cmd.Dir = repo
	b, _ := cmd.CombinedOutput()
	if !strings.Contains(string(b), "tspack doctor") {
		t.Fatalf("help missing doctor")
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "doctor", "--root", root)
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, string(b))
	}
	for _, s := range []string{"Project", "Format/Lint", "Run", "Inspect", "Summary"} {
		if !strings.Contains(string(b), s) {
			t.Fatalf("missing section %s", s)
		}
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "doctor", "--root", root, "--json")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor json failed: %v\n%s", err, string(b))
	}
	var parsed map[string]any
	if e := json.Unmarshal(b, &parsed); e != nil {
		t.Fatalf("invalid json: %v", e)
	}
	if !strings.HasSuffix(string(b), "\n") {
		t.Fatalf("doctor json output must end with newline")
	}
	if !strings.Contains(string(b), "\n  \"root\"") {
		t.Fatalf("doctor json must use two-space indentation")
	}
}

func TestDoctorInvalidScope(t *testing.T) {
	repo := filepath.Join("..", "..")
	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "badscope")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_DOCTOR_INVALID_SCOPE") {
		t.Fatalf("expected invalid scope: %v\n%s", err, string(b))
	}
}

func TestDoctorFormatMissingBiomeExitsNonzero(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "format", "--root", root)
	cmd.Dir = repo
	cmd.Env = []string{"PATH=/definitely-missing"}
	b, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected nonzero exit for missing biome: %s", string(b))
	}
}

func TestDoctorFormatReportsDefaultBiomeConfigSource(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	backend := filepath.Join(root, "node_modules", ".bin", "biome")
	if err := os.MkdirAll(filepath.Dir(backend), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backend, []byte("#!/bin/sh\necho 1.0.0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	before := tempBiomeConfigFiles(t)
	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "format", "--root", root, "--json")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+doctorPathWithNodeOnly(t))
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor format json failed: %v\n%s", err, string(b))
	}
	after := tempBiomeConfigFiles(t)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("doctor must not create temp Biome configs\nbefore: %#v\nafter: %#v", before, after)
	}

	var report DoctorReport
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatalf("invalid doctor json: %v\n%s", err, string(b))
	}
	config := flattenDoctorChecks(report)["config"]
	if config.Details["configSource"] != "tspack-default" {
		t.Fatalf("expected tspack-default config source: %#v", config.Details)
	}
	if config.Details["defaultStyle"] != defaultBiomeStyleSummary {
		t.Fatalf("expected default style summary: %#v", config.Details)
	}
	if _, ok := config.Details["configPath"]; ok {
		t.Fatalf("default doctor details should not report a temp config path: %#v", config.Details)
	}
}

func TestDoctorFormatReportsProjectBiomeConfigSource(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	backend := filepath.Join(root, "node_modules", ".bin", "biome")
	if err := os.MkdirAll(filepath.Dir(backend), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backend, []byte("#!/bin/sh\necho 1.0.0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "biome.jsonc")
	if err := os.WriteFile(configPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "format", "--root", root, "--json")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+doctorPathWithNodeOnly(t))
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor format json failed: %v\n%s", err, string(b))
	}
	var report DoctorReport
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatalf("invalid doctor json: %v\n%s", err, string(b))
	}
	config := flattenDoctorChecks(report)["config"]
	if config.Details["configSource"] != "project" {
		t.Fatalf("expected project config source: %#v", config.Details)
	}
	if config.Details["configPath"] != configPath {
		t.Fatalf("expected project config path: %#v", config.Details)
	}
	if _, ok := config.Details["defaultStyle"]; ok {
		t.Fatalf("project config details should not include default style: %#v", config.Details)
	}
}

func TestDoctorFormatReportsDirectPackageBiomeBackend(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	backend := filepath.Join(root, "node_modules", "@biomejs", "biome", "bin", "biome")
	if err := os.MkdirAll(filepath.Dir(backend), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backend, []byte("#!/bin/sh\necho 1.0.0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "format", "--root", root, "--json")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+t.TempDir())
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor format json failed: %v\n%s", err, string(b))
	}
	var report DoctorReport
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatalf("invalid doctor json: %v\n%s", err, string(b))
	}
	checks := flattenDoctorChecks(report)
	biome := checks["biome"]
	if biome.Details["selectedPath"] != backend {
		t.Fatalf("expected selected direct package backend: %#v", biome.Details)
	}
	if biome.Details["source"] != "direct-package" {
		t.Fatalf("expected direct-package source: %#v", biome.Details)
	}
	if biome.Details["directPackagePath"] != backend {
		t.Fatalf("expected direct package path detail: %#v", biome.Details)
	}
}

func TestDoctorRuntimeReportsSelectedNodejsProfile(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeManifestStubWithIR(t, repo, `{format:1,workspace:{name:"ws",runtime:"nodejs"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{}}]}`)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)

	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "runtime", "--root", root, "--json")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor runtime json failed: %v\n%s", err, string(b))
	}
	var report DoctorReport
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatalf("invalid doctor runtime json: %v\n%s", err, string(b))
	}
	check := flattenDoctorChecks(report)["runtime profile"]
	if check.Details["selected"] != "nodejs" || check.Details["executable"] != "node" || check.Details["lifecycleOwner"] != "tspack" {
		t.Fatalf("unexpected runtime profile details: %#v", check.Details)
	}
	if check.Details["packageManagerDelegated"] != false {
		t.Fatalf("runtime profile must not delegate package managers: %#v", check.Details)
	}
}

func TestDoctorRuntimeTextReportsOwnershipDetails(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()

	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "runtime", "--root", root)
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor runtime text failed: %v\n%s", err, string(b))
	}
	out := string(b)
	for _, expected := range []string{
		"Runtime profile",
		"message: manifest.tsx missing; defaulting runtime profile to nodejs",
		"selected: nodejs",
		"packageManagerDelegated: false",
		"ownershipNote: TSPack owns package resolution, lockfiles, sync/materialization, check, pack, and lifecycle policy; runtime profile does not delegate those to npm/bun/deno.",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("doctor runtime text missing %q:\n%s", expected, out)
		}
	}
}

func TestDoctorRuntimeReportsOmittedAndExplicitNodejsEquivalently(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)

	omitted := doctorRuntimeDetailsForIR(t, repo, root, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{}}]}`)
	explicit := doctorRuntimeDetailsForIR(t, repo, root, `{format:1,workspace:{name:"ws",runtime:"nodejs"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{}}]}`)

	if !reflect.DeepEqual(omitted, explicit) {
		t.Fatalf("doctor runtime nodejs details differ:\nomitted=%#v\nexplicit=%#v", omitted, explicit)
	}
	if explicit["selected"] != "nodejs" || explicit["executable"] != "node" {
		t.Fatalf("unexpected explicit nodejs details: %#v", explicit)
	}
	if explicit["lifecycleOwner"] != "tspack" || explicit["packageManagerDelegated"] != false {
		t.Fatalf("unexpected ownership details: %#v", explicit)
	}
}

func doctorRuntimeDetailsForIR(t *testing.T, repo string, root string, irJSON string) map[string]any {
	t.Helper()
	writeManifestStubWithIR(t, repo, irJSON)
	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "runtime", "--root", root, "--json")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor runtime json failed: %v\n%s", err, string(b))
	}
	var report DoctorReport
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatalf("invalid doctor runtime json: %v\n%s", err, string(b))
	}
	return flattenDoctorChecks(report)["runtime profile"].Details
}

func TestDoctorRuntimeReportsSelectedBunWithoutDenoNoise(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeManifestStubWithIR(t, repo, `{format:1,workspace:{name:"ws",runtime:"bun"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{}}]}`)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)

	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "runtime", "--root", root, "--json")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+doctorPathWithNodeOnly(t))
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor runtime json failed: %v\n%s", err, string(b))
	}
	var report DoctorReport
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatalf("invalid doctor runtime json: %v\n%s", err, string(b))
	}
	checks := flattenDoctorChecks(report)
	check := checks["runtime profile"]
	if check.Details["selected"] != "bun" || check.Details["executable"] != "bun" || check.Details["status"] != "experimental" {
		t.Fatalf("unexpected bun runtime profile details: %#v", check.Details)
	}
	if _, ok := checks["deno"]; ok {
		t.Fatalf("non-selected deno should not create warning noise: %#v", checks["deno"])
	}
}

func TestDoctorRuntimeReportsSelectedBunAvailableWithStub(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeManifestStubWithIR(t, repo, `{format:1,workspace:{name:"ws",runtime:"bun"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{}}]}`)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	binDir := t.TempDir()
	bunPath := filepath.Join(binDir, "bun")
	if err := os.WriteFile(bunPath, []byte("#!/bin/sh\necho bun-stub\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "runtime", "--root", root, "--json")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+doctorPathWithNodeOnly(t))
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor runtime json failed: %v\n%s", err, string(b))
	}
	var report DoctorReport
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatalf("invalid doctor runtime json: %v\n%s", err, string(b))
	}
	check := flattenDoctorChecks(report)["runtime profile"]
	if check.Details["selected"] != "bun" || check.Details["executable"] != "bun" || check.Details["available"] != true {
		t.Fatalf("unexpected available bun runtime profile details: %#v", check.Details)
	}
	if check.Details["packageManagerDelegated"] != false || check.Details["lifecycleOwner"] != "tspack" {
		t.Fatalf("unexpected bun ownership details: %#v", check.Details)
	}
}

func TestDoctorRuntimeReportsSelectedDeno(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeManifestStubWithIR(t, repo, `{format:1,workspace:{name:"ws",runtime:"deno"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{}}]}`)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)

	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "runtime", "--root", root, "--json")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+doctorPathWithNodeOnly(t))
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor runtime json failed: %v\n%s", err, string(b))
	}
	var report DoctorReport
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatalf("invalid doctor runtime json: %v\n%s", err, string(b))
	}
	checks := flattenDoctorChecks(report)
	check := checks["runtime profile"]
	if check.Details["selected"] != "deno" || check.Details["executable"] != "deno" || check.Details["available"] != false || check.Details["status"] != "experimental" {
		t.Fatalf("unexpected deno runtime profile details: %#v", check.Details)
	}
	if check.Details["packageManagerDelegated"] != false || check.Details["lifecycleOwner"] != "tspack" {
		t.Fatalf("unexpected deno ownership details: %#v", check.Details)
	}
	if _, ok := checks["bun"]; ok {
		t.Fatalf("non-selected bun should not create warning noise: %#v", checks["bun"])
	}
}

func TestDoctorRuntimeReportsSelectedDenoAvailableWithStub(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeManifestStubWithIR(t, repo, `{format:1,workspace:{name:"ws",runtime:"deno"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{}}]}`)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	binDir := t.TempDir()
	denoPath := filepath.Join(binDir, "deno")
	if err := os.WriteFile(denoPath, []byte("#!/bin/sh\necho deno-stub\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "runtime", "--root", root, "--json")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+doctorPathWithNodeOnly(t))
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor runtime json failed: %v\n%s", err, string(b))
	}
	var report DoctorReport
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatalf("invalid doctor runtime json: %v\n%s", err, string(b))
	}
	check := flattenDoctorChecks(report)["runtime profile"]
	if check.Details["selected"] != "deno" || check.Details["executable"] != "deno" || check.Details["available"] != true {
		t.Fatalf("unexpected available deno runtime profile details: %#v", check.Details)
	}
}

func TestDoctorAllIncludesRuntimeProfileSection(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeManifestStubWithIR(t, repo, `{format:1,workspace:{name:"ws",runtime:"nodejs"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{}}]}`)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)

	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "--root", root, "--json")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor all json failed: %v\n%s", err, string(b))
	}
	var report DoctorReport
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatalf("invalid doctor all json: %v\n%s", err, string(b))
	}
	found := false
	for _, section := range report.Sections {
		if section.Name == "Runtime profile" {
			found = true
		}
	}
	if !found {
		t.Fatalf("doctor all missing Runtime profile section: %#v", report.Sections)
	}
}

func TestDoctorRunSystemRuntimeAndReservedRuntimeSignal(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeManifestStubWithIR(t, repo, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",cwd:"package",url:"http://127.0.0.1:5173",command:["node","server.js"],ready:{kind:"http",path:"/"}}]}]}`)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)

	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "run", "--root", root, "--json")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor run json failed: %v\n%s", err, string(b))
	}
	var report DoctorReport
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatalf("invalid doctor json: %v\n%s", err, string(b))
	}
	checks := flattenDoctorChecks(report)
	runTarget := checks["runTarget:app:dev"]
	if runTarget.Details["id"] != "app:dev" {
		t.Fatalf("missing package-qualified run target id: %#v", runTarget.Details)
	}
	if runTarget.Details["runtimeAvailable"] != true {
		t.Fatalf("system runtimeAvailable should be true: %#v", runTarget.Details)
	}
	if runTarget.Details["cwd"] != "package" {
		t.Fatalf("missing cwd detail: %#v", runTarget.Details)
	}
	if runTarget.Details["cwdPath"] != root {
		t.Fatalf("missing cwdPath detail: %#v", runTarget.Details)
	}
	if runTarget.Details["packageRoot"] != root {
		t.Fatalf("missing packageRoot detail: %#v", runTarget.Details)
	}
	if runTarget.Details["commandFirstToken"] != "node" {
		t.Fatalf("missing commandFirstToken detail: %#v", runTarget.Details)
	}
	if _, ok := runTarget.Details["commandAvailable"]; !ok {
		t.Fatalf("missing commandAvailable detail: %#v", runTarget.Details)
	}
	denoCheck := checks["deno"]
	if denoCheck.Name != "deno" {
		t.Fatalf("doctor run should report Deno runtime availability")
	}
	if _, ok := checks["bun"]; !ok {
		t.Fatalf("doctor run should report Bun runtime availability")
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "doctor", "run", "--root", root)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor run text failed: %v\n%s", err, string(b))
	}
	text := string(b)
	for _, expected := range []string{"runtimeAvailable: true", "commandFirstToken: node", "cwd: package", "cwdPath: " + root, "packageRoot: " + root, "readyKind: http", "readyPath: /", "deno"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("doctor text missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "system not found") || strings.Contains(text, "runtimeAvailable: false") {
		t.Fatalf("doctor text reported system unavailable:\n%s", text)
	}
}

func TestDoctorTextRendersSortedDetailsAndRuntimeVersion(t *testing.T) {
	check := DoctorCheck{
		Name:    "details",
		Status:  "ok",
		Message: "details found",
		Details: map[string]any{
			"zeta":  true,
			"alpha": "first",
			"items": []string{"one", "two"},
		},
	}
	report := DoctorReport{
		Root:     "/tmp/project",
		Sections: []DoctorSection{{Name: "Run", Checks: []DoctorCheck{check}}},
		Summary:  DoctorSummary{Ok: 1},
	}
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	printDoctorText(report)
	_ = w.Close()
	os.Stdout = old
	data, _ := io.ReadAll(r)
	text := string(data)
	alpha := strings.Index(text, "alpha: first")
	items := strings.Index(text, "items:\n      one\n      two")
	zeta := strings.Index(text, "zeta: true")
	if alpha < 0 || items < 0 || zeta < 0 || !(alpha < items && items < zeta) {
		t.Fatalf("details not sorted/rendered deterministically:\n%s", text)
	}

	nodeCheck := runtimeCheck("node", "--version")
	if nodeCheck.Status == "ok" {
		if nodeCheck.Details["path"] == "" || nodeCheck.Details["version"] == "" {
			t.Fatalf("node runtime detail missing path/version: %#v", nodeCheck.Details)
		}
	}
}

func doctorPathWithNodeOnly(t *testing.T) string {
	t.Helper()
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("node is required for doctor runtime test: %v", err)
	}
	dir := t.TempDir()
	linkPath := filepath.Join(dir, "node")
	if err := os.Symlink(nodePath, linkPath); err != nil {
		t.Fatalf("failed to create node symlink: %v", err)
	}
	return dir
}

func writeManifestStubWithIR(t *testing.T, repo string, irJSON string) {
	t.Helper()
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := "#!/usr/bin/env node\nconst out={ok:true,ir:" + irJSON + ",diagnostics:[]};process.stdout.write(JSON.stringify(out));"
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })
}

func tempBiomeConfigFiles(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(os.TempDir(), "tspack-biome-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func flattenDoctorChecks(report DoctorReport) map[string]DoctorCheck {
	checks := map[string]DoctorCheck{}
	for _, section := range report.Sections {
		for _, check := range section.Checks {
			checks[check.Name] = check
		}
	}
	return checks
}

func TestDoctorRunReportsReadyKindSpecificDetails(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeManifestStubWithIR(t, repo, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"db",runtime:"system",command:["node","db.js"],ready:{kind:"tcp",port:5432}},{name:"web",runtime:"system",command:["node","web.js"],url:"http://127.0.0.1:5173",ready:{kind:"stdout-match",pattern:"Local:",stream:"stderr"}}]}]}`)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)

	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "run", "--root", root, "--json")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor run json failed: %v\n%s", err, string(b))
	}
	var report DoctorReport
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatalf("invalid doctor json: %v\n%s", err, string(b))
	}
	checks := flattenDoctorChecks(report)
	db := checks["runTarget:app:db"]
	if db.Details["readyKind"] != "tcp" || db.Details["readyHost"] != "127.0.0.1" || db.Details["readyPort"] != float64(5432) {
		t.Fatalf("missing tcp ready details: %#v", db.Details)
	}
	web := checks["runTarget:app:web"]
	if web.Details["readyKind"] != "stdout-match" || web.Details["readyPattern"] != "Local:" || web.Details["readyStream"] != "stderr" {
		t.Fatalf("missing stdout-match ready details: %#v", web.Details)
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "doctor", "run", "--root", root)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor run text failed: %v\n%s", err, string(b))
	}
	text := string(b)
	for _, expected := range []string{"readyKind: tcp", "readyHost: 127.0.0.1", "readyPort: 5432", "readyKind: stdout-match", "readyPattern: Local:", "readyStream: stderr"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("doctor text missing %q:\n%s", expected, text)
		}
	}
}

func TestDoctorSecurityNoLifecycleCapabilities(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeManifestStubWithIR(t, repo, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{}}]}`)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "ts-lock.toml"), []byte("[lock]\nformat=1\ntool=\"tspack\"\n"), 0o644)

	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "security", "--root", root)
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor security text failed: %v\n%s", err, string(b))
	}
	text := string(b)
	for _, expected := range []string{"Security", "lifecycle summary: ok", "no lifecycle script capabilities recorded", "totalLifecycleCapabilities: 0", `"consumerInstall":0`, `"maintainerPublish":0`, `"other":0`, "lifecycle execution posture: ok"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("doctor security text missing %q:\n%s", expected, text)
		}
	}
	cmd = exec.Command("go", "run", "./cmd/tspack", "doctor", "security", "--root", root, "--json")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor security json failed: %v\n%s", err, string(b))
	}
	var report DoctorReport
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatalf("invalid doctor security json: %v\n%s", err, string(b))
	}
	checks := flattenDoctorChecks(report)
	summary := checks["lifecycle summary"]
	if summary.Status != "ok" || summary.Details["totalLifecycleCapabilities"] != float64(0) || summary.Details["unusedAcknowledgments"] != float64(0) {
		t.Fatalf("unexpected zero-capability summary: %#v", summary)
	}
}

func TestDoctorSecurityLifecycleAcknowledgementStates(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeManifestStubWithIR(t, repo, `{format:1,workspace:{name:"ws"},security:{acknowledgedCapabilities:[{package:"npm:ack@1.0.0",kind:"lifecycleScript",script:"postinstall",command:"node install.js",reason:"Known lifecycle capability; execution remains blocked.",behaviorFixture:"security/ack-postinstall.valid.xtest.tsx",behaviorReport:"security/ack-postinstall.report.json"},{package:"npm:stale@1.0.0",kind:"lifecycleScript",script:"postinstall",command:"node old.js",reason:"Expected package install hook."},{package:"npm:unused@1.0.0",kind:"lifecycleScript",script:"postinstall",command:"node gone.js",reason:"No longer present."}]},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{}}]}`)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, "security"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "security", "ack-postinstall.valid.xtest.tsx"), []byte("// fixture is evidence only\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "security", "ack-postinstall.report.json"), []byte(`{"ok":true,"violations":[]}`), 0o644)
	lockText := `[lock]
format = 1
tool = "tspack"

[[package]]
id = "npm:unack@1.0.0"
name = "unack"
version = "1.0.0"
source = "npm"
integrity = "sha512-test"
  [[package.capability]]
  kind = "lifecycleScript"
  script = "postinstall"
  command = "node build.js"

[[package]]
id = "npm:ack@1.0.0"
name = "ack"
version = "1.0.0"
source = "npm"
integrity = "sha512-test"
  [[package.capability]]
  kind = "lifecycleScript"
  script = "postinstall"
  command = "node install.js"

[[package]]
id = "npm:stale@1.0.0"
name = "stale"
version = "1.0.0"
source = "npm"
integrity = "sha512-test"
  [[package.capability]]
  kind = "lifecycleScript"
  script = "postinstall"
  command = "node new.js"

[[edge]]
from = "app:target:app"
to = "npm:unack@1.0.0"
kind = "runtime"

[[edge]]
from = "app:target:app"
to = "npm:ack@1.0.0"
kind = "runtime"

[[edge]]
from = "app:target:app"
to = "npm:stale@1.0.0"
kind = "runtime"
`
	_ = os.WriteFile(filepath.Join(root, "ts-lock.toml"), []byte(lockText), 0o644)

	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "security", "--root", root, "--json")
	cmd.Dir = repo
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("warnings-only doctor security should exit 0: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("doctor security --json wrote stderr: %q", stderr.String())
	}
	firstOutput := stdout.String()
	cmd = exec.Command("go", "run", "./cmd/tspack", "doctor", "security", "--root", root, "--json")
	cmd.Dir = repo
	b, err := cmd.Output()
	if err != nil {
		t.Fatalf("repeat doctor security json failed: %v", err)
	}
	if firstOutput != string(b) {
		t.Fatalf("doctor security json not deterministic:\nfirst=%s\nsecond=%s", firstOutput, string(b))
	}

	var report DoctorReport
	if err := json.Unmarshal([]byte(firstOutput), &report); err != nil {
		t.Fatalf("invalid doctor security json: %v\n%s", err, firstOutput)
	}
	checks := flattenDoctorChecks(report)
	summary := checks["lifecycle summary"]
	if summary.Status != "warning" || summary.Details["totalLifecycleCapabilities"] != float64(3) || summary.Details["acknowledged"] != float64(1) || summary.Details["unacknowledged"] != float64(1) || summary.Details["staleAcknowledgments"] != float64(1) || summary.Details["unusedAcknowledgments"] != float64(1) {
		t.Fatalf("unexpected lifecycle summary: %#v", summary)
	}
	categories, ok := summary.Details["lifecycleCategories"].(map[string]any)
	if !ok || categories["consumerInstall"] != float64(3) || categories["maintainerPublish"] != float64(0) || categories["other"] != float64(0) {
		t.Fatalf("unexpected lifecycle categories: %#v", summary.Details["lifecycleCategories"])
	}
	unack := checks["lifecycle npm:unack@1.0.0 postinstall"]
	if unack.Status != "warning" || unack.Details["execution"] != "blocked" || unack.Details["acknowledged"] != false || unack.Details["command"] != "node build.js" || unack.Details["lifecycleCategory"] != "consumer-install" || unack.Details["consumerInstallTime"] != true {
		t.Fatalf("unexpected unacknowledged lifecycle check: %#v", unack)
	}
	pulledBy, ok := unack.Details["pulledBy"].([]any)
	if !ok || len(pulledBy) != 1 || pulledBy[0] != "app:target:app -> npm:unack@1.0.0" {
		t.Fatalf("missing pulled-by path: %#v", unack.Details)
	}
	ack := checks["lifecycle npm:ack@1.0.0 postinstall"]
	if ack.Status != "ok" || ack.Details["acknowledged"] != true || ack.Details["reason"] == "" || ack.Details["lifecycleCategory"] != "consumer-install" {
		t.Fatalf("unexpected acknowledged lifecycle check: %#v", ack)
	}
	if ack.Details["behaviorFixture"] != "security/ack-postinstall.valid.xtest.tsx" || ack.Details["behaviorFixtureStatus"] != "present" {
		t.Fatalf("missing fixture evidence status: %#v", ack)
	}
	evidenceSummary := checks["behavior evidence summary"]
	if evidenceSummary.Status != "ok" || evidenceSummary.Details["behaviorFixturesPresent"] != float64(1) || evidenceSummary.Details["behaviorReportsPresent"] != float64(1) {
		t.Fatalf("unexpected evidence summary: %#v", evidenceSummary)
	}
	stale := checks["lifecycle npm:stale@1.0.0 postinstall"]
	if stale.Status != "warning" || stale.Details["acknowledged"] != false || stale.Details["stale"] != true || stale.Details["acknowledgedCommand"] != "node old.js" || stale.Details["actualCommand"] != "node new.js" {
		t.Fatalf("unexpected stale lifecycle check: %#v", stale)
	}
	unused := checks["unused acknowledgement npm:unused@1.0.0 postinstall"]
	if unused.Status != "warning" || unused.Details["command"] != "node gone.js" || unused.Details["reason"] == "" {
		t.Fatalf("unexpected unused acknowledgement check: %#v", unused)
	}
}

func TestDoctorSecurityMissingLockfileSuppressesUnusedAcknowledgements(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeManifestStubWithIR(t, repo, `{format:1,workspace:{name:"ws"},security:{acknowledgedCapabilities:[{package:"npm:unused@1.0.0",kind:"lifecycleScript",script:"postinstall",command:"node gone.js",reason:"No lock graph yet.",behaviorFixture:"security/missing.valid.xtest.tsx"}]},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{}}]}`)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)

	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "security", "--root", root, "--json")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("missing-lockfile warning should exit 0: %v\n%s", err, string(b))
	}
	var report DoctorReport
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatalf("invalid missing-lockfile json: %v\n%s", err, string(b))
	}
	checks := flattenDoctorChecks(report)
	if checks["security lockfile missing"].Status != "warning" {
		t.Fatalf("missing lockfile warning not reported: %#v", checks)
	}
	if _, ok := checks["unused acknowledgement npm:unused@1.0.0 postinstall"]; ok {
		t.Fatalf("unused acknowledgement should be suppressed without lockfile: %#v", checks)
	}
	if checks["behavior fixture missing npm:unused@1.0.0 postinstall"].Status != "warning" {
		t.Fatalf("missing fixture should be reported without lockfile: %#v", checks)
	}
}

func TestDoctorAllIncludesSecuritySection(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeManifestStub(t, repo)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "ts-lock.toml"), []byte("[lock]\nformat=1\ntool=\"tspack\"\n"), 0o644)

	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "--root", root, "--json")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor all json failed: %v\n%s", err, string(b))
	}
	var report DoctorReport
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatalf("invalid doctor all json: %v\n%s", err, string(b))
	}
	foundSecurity := false
	for _, section := range report.Sections {
		if section.Name == "Security" {
			foundSecurity = true
		}
	}
	if !foundSecurity {
		t.Fatalf("doctor all missing Security section: %#v", report.Sections)
	}
}

func TestRuntimeSwitchDoctorRuntimeReportsSelectedProfile(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		profile    string
		executable string
	}{
		{profile: "nodejs", executable: "node"},
		{profile: "bun", executable: "bun"},
		{profile: "deno", executable: "deno"},
	}

	for _, tc := range cases {
		t.Run(tc.profile, func(t *testing.T) {
			irJSON := readRuntimeSwitchFixtureIRJSON(t, repo, tc.profile)
			writeManifestStubWithIR(t, repo, irJSON)
			cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "runtime", "--root", root, "--json")
			cmd.Dir = repo
			cmd.Env = append(os.Environ(), "PATH="+doctorPathWithNodeOnly(t))
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("doctor runtime json failed: %v\n%s", err, string(output))
			}
			var report DoctorReport
			if err := json.Unmarshal(output, &report); err != nil {
				t.Fatalf("invalid doctor runtime json: %v\n%s", err, string(output))
			}
			check := flattenDoctorChecks(report)["runtime profile"]
			if check.Details["selected"] != tc.profile {
				t.Fatalf("selected runtime profile = %#v, want %q", check.Details["selected"], tc.profile)
			}
			if check.Details["executable"] != tc.executable {
				t.Fatalf("runtime executable = %#v, want %q", check.Details["executable"], tc.executable)
			}
			if check.Details["lifecycleOwner"] != "tspack" || check.Details["packageManagerDelegated"] != false {
				t.Fatalf("unexpected ownership details: %#v", check.Details)
			}
		})
	}
}

func readRuntimeSwitchFixtureIRJSON(t *testing.T, repo string, profile string) string {
	t.Helper()
	path := filepath.Join(repo, "fixtures", "valid", "runtime-switch-"+profile, "manifest.ir.golden.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestDoctorLifecycleCategoryAcknowledgementStates(t *testing.T) {
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Packages: []lockfile.Package{{ID: "npm:dep-a@1.0.0", Name: "dep-a", Version: "1.0.0", Source: "npm", Integrity: "x", Capabilities: []lockfile.Capability{
			{Kind: "lifecycleScript", Script: "prepare", Command: "node prepare.js"},
			{Kind: "lifecycleScript", Script: "postinstall", Command: "node install.js"},
		}}},
	}
	checks := doctorLifecycleSecurityChecks(t.TempDir(), lf, nil, []manifest.AcknowledgedLifecycleCategory{{
		Category: "maintainer-publish",
		Reason:   "Maintainer lifecycle scripts remain blocked.",
	}})
	byName := map[string]DoctorCheck{}
	for _, check := range checks {
		byName[check.Name] = check
	}
	summary := byName["lifecycle summary"]
	if summary.Details["categoryAcknowledgedCapabilities"] != 1 || summary.Details["unacknowledgedCapabilities"] != 1 {
		t.Fatalf("unexpected lifecycle category summary: %#v", summary)
	}
	prepare := byName["lifecycle npm:dep-a@1.0.0 prepare"]
	if prepare.Status != "ok" || prepare.Details["acknowledgmentKind"] != "lifecycle-category" || prepare.Details["acknowledgedByCategory"] != "maintainer-publish" {
		t.Fatalf("unexpected category acknowledged capability: %#v", prepare)
	}
	postinstall := byName["lifecycle npm:dep-a@1.0.0 postinstall"]
	if postinstall.Status != "warning" || postinstall.Details["acknowledged"] != false || postinstall.Details["lifecycleCategory"] != "consumer-install" {
		t.Fatalf("consumer install lifecycle should remain unacknowledged: %#v", postinstall)
	}
	category := byName["lifecycle category acknowledgement maintainer-publish"]
	if category.Details["matchedCapabilities"] != 1 {
		t.Fatalf("category acknowledgement missing matched count: %#v", category)
	}
}
