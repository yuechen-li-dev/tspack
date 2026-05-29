package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
	for _, name := range []string{"bun", "deno"} {
		check := checks[name]
		if check.Status == "warning" {
			t.Fatalf("%s should not warn while reserved: %#v", name, check)
		}
		if check.Status != "not_applicable" {
			t.Fatalf("%s should be not_applicable while reserved: %#v", name, check)
		}
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "doctor", "run", "--root", root)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor run text failed: %v\n%s", err, string(b))
	}
	text := string(b)
	for _, expected := range []string{"runtimeAvailable: true", "commandFirstToken: node", "cwd: package", "cwdPath: " + root, "packageRoot: " + root, "readyKind: http", "readyPath: /", "reserved runtime backend; not implemented yet"} {
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
	items := strings.Index(text, "items: [\"one\",\"two\"]")
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

func writeManifestStubWithIR(t *testing.T, repo string, irJSON string) {
	t.Helper()
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := "#!/usr/bin/env node\nconst out={ok:true,ir:" + irJSON + ",diagnostics:[]};process.stdout.write(JSON.stringify(out));"
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })
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
