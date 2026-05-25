package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type checkJSONReport struct {
	Command     string                `json:"command"`
	OK          bool                  `json:"ok"`
	Summary     checkJSONSummary      `json:"summary"`
	Diagnostics []checkJSONDiagnostic `json:"diagnostics"`
}

type checkJSONSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`
	Total    int `json:"total"`
}

type checkJSONDiagnostic struct {
	Code     string   `json:"code"`
	Severity string   `json:"severity"`
	Message  string   `json:"message"`
	Details  []string `json:"details"`
}
type updateDryRunJSONReport struct {
	Command string `json:"command"`
	DryRun  bool   `json:"dryRun"`
	OK      bool   `json:"ok"`
	Summary struct {
		Added     int `json:"added"`
		Removed   int `json:"removed"`
		Changed   int `json:"changed"`
		Unchanged int `json:"unchanged"`
	} `json:"summary"`
}

func buildTspackBinary(t *testing.T, repo string) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "tspack-test-bin")
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/tspack")
	buildCmd.Dir = repo
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build tspack binary failed: %v\n%s", err, string(out))
	}
	return binPath
}

func reservePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestCLIPackSmokeAndDryRun(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const out={ok:true,ir:{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"library",dependencies:[],targets:[{name:"core",export:".",entry:"src/index.ts",runtime:"dist/index.js",types:"dist/index.d.ts",deps:[],peers:[]}],tools:[],boundaries:[],publish:{include:["dist/**","README.md"],exclude:["src/**"]},policies:{types:{},boundaries:{}}}]},diagnostics:[]};
process.stdout.write(JSON.stringify(out));`
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })

	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "src"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, "dist"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "src", "index.ts"), []byte("export const x = 1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "dist", "index.js"), []byte("export const x = 1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "dist", "index.d.ts"), []byte("export declare const x: number\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "README.md"), []byte("readme\n"), 0o644)

	outDir := filepath.Join(root, "out")
	cmd := exec.Command("go", "run", "./cmd/tspack", "pack", "--root", root, "--out", outDir)
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pack failed: %v\n%s", err, string(b))
	}
	if !strings.Contains(string(b), "packed app@1.0.0") {
		t.Fatalf("expected packed output, got: %s", string(b))
	}
	if _, err = os.Stat(filepath.Join(outDir, "app-1.0.0.tgz")); err != nil {
		t.Fatalf("expected artifact: %v", err)
	}

	dryDir := filepath.Join(root, "dry")
	cmd = exec.Command("go", "run", "./cmd/tspack", "pack", "--root", root, "--out", dryDir, "--dry-run")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run failed: %v\n%s", err, string(b))
	}
	if _, err = os.Stat(filepath.Join(dryDir, "app-1.0.0.tgz")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote artifact")
	}
	if !strings.Contains(string(b), "package/dist/index.js") {
		t.Fatalf("expected preview output, got: %s", string(b))
	}

	help := exec.Command("go", "run", "./cmd/tspack", "help")
	help.Dir = repo
	hb, err := help.CombinedOutput()
	if err != nil || !strings.Contains(string(hb), "tspack pack") {
		t.Fatalf("help missing pack: %v\n%s", err, string(hb))
	}
}

func TestCLIWhySmoke(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const out={ok:true,ir:{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"library",dependencies:[{key:"vue",kind:"peer",optional:true,source:{kind:"npm",package:"vue",range:"^3.4.0"}},{key:"react",kind:"peer",source:{kind:"npm",package:"react",range:"^19.1.0"}}],targets:[{name:"core",export:".",entry:"src/index.ts",runtime:"src/index.ts",types:"dist/index.d.ts",deps:[],peers:[]},{name:"vue",export:"./vue",entry:"src/vue.ts",runtime:"src/vue.ts",types:"dist/vue.d.ts",deps:[],peers:["vue"]},{name:"react",export:"./react",entry:"src/react.ts",runtime:"src/react.ts",types:"dist/react.d.ts",deps:[],peers:["react"]}],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{types:{},boundaries:{}}}]},diagnostics:[]};
process.stdout.write(JSON.stringify(out));`
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })

	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "src"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, "dist"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "src", "index.ts"), []byte("x\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "src", "vue.ts"), []byte("x\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "src", "react.ts"), []byte("x\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "dist", "index.d.ts"), []byte("x\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "dist", "vue.d.ts"), []byte("x\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "dist", "react.d.ts"), []byte("x\n"), 0o644)
	lock := "[lock]\nformat=1\ntool=\"tspack\"\n[[package]]\nid=\"npm:vue@3.4.0\"\nname=\"vue\"\nversion=\"3.4.0\"\nsource=\"npm\"\nhash=\"h\"\n[[package]]\nid=\"npm:dep-a@1.0.0\"\nname=\"dep-a\"\nversion=\"1.0.0\"\nsource=\"npm\"\nhash=\"h\"\n[[package]]\nid=\"npm:left-pad@1.2.0\"\nname=\"left-pad\"\nversion=\"1.2.0\"\nsource=\"npm\"\nhash=\"h\"\n[[package]]\nid=\"npm:left-pad@1.3.0\"\nname=\"left-pad\"\nversion=\"1.3.0\"\nsource=\"npm\"\nhash=\"h\"\n[[edge]]\nfrom=\"app:target:vue\"\nto=\"npm:vue@3.4.0\"\nkind=\"peer\"\noptional=true\n[[edge]]\nfrom=\"npm:dep-a@1.0.0\"\nto=\"npm:left-pad@1.2.0\"\nkind=\"runtime\"\n[[target]]\npackage=\"app\"\nname=\"core\"\nexport=\".\"\nentry=\"src/index.ts\"\nruntime=\"src/index.ts\"\ntypes=\"dist/index.d.ts\"\n[[target]]\npackage=\"app\"\nname=\"react\"\nexport=\"./react\"\nentry=\"src/react.ts\"\nruntime=\"src/react.ts\"\ntypes=\"dist/react.d.ts\"\n[[target]]\npackage=\"app\"\nname=\"vue\"\nexport=\"./vue\"\nentry=\"src/vue.ts\"\nruntime=\"src/vue.ts\"\ntypes=\"dist/vue.d.ts\"\n"
	_ = os.WriteFile(filepath.Join(root, "ts-lock.toml"), []byte(lock), 0o644)

	cmd := exec.Command("go", "run", "./cmd/tspack", "why", "vue", "--root", root, "--lockfile", filepath.Join(root, "ts-lock.toml"))
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("why vue failed: %v\n%s", err, string(b))
	}
	o := string(b)
	if !strings.Contains(o, "vue") || !strings.Contains(o, "reachable from") || !strings.Contains(o, "not reachable from") {
		t.Fatalf("unexpected why vue output: %s", o)
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "why", "npm:left-pad@1.2.0", "--root", root, "--lockfile", filepath.Join(root, "ts-lock.toml"))
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("why left-pad failed: %v\n%s", err, string(b))
	}
	o = string(b)
	if !strings.Contains(o, "left-pad") || !strings.Contains(o, "npm:dep-a@1.0.0") {
		t.Fatalf("expected transitive edge details: %s", o)
	}
	if strings.Count(o, "npm:dep-a@1.0.0 -> npm:left-pad@1.2.0 runtime") != 1 {
		t.Fatalf("expected deduped lock edge output: %s", o)
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "why", "left-pad", "--root", root, "--lockfile", filepath.Join(root, "ts-lock.toml"))
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("why left-pad should be not found and fail: %s", string(b))
	}
	o = string(b)
	if !strings.Contains(o, "TSPACK_WHY_NOT_FOUND: why query not found: left-pad") {
		t.Fatalf("expected not-found message with query, got: %s", o)
	}
	if !strings.Contains(o, "matching lock packages exist:") || !strings.Contains(o, "npm:left-pad@1.2.0") || !strings.Contains(o, "npm:left-pad@1.3.0") {
		t.Fatalf("expected lock package suggestions: %s", o)
	}
	if !strings.Contains(o, "tspack why npm:left-pad@1.2.0") {
		t.Fatalf("expected concrete suggestion command: %s", o)
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "why")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero for missing query, got success: %s", string(b))
	}
	if !strings.Contains(string(b), "TSPACK_WHY_QUERY_REQUIRED") {
		t.Fatalf("missing query diagnostic not surfaced: %s", string(b))
	}
}

func TestCLIHelpAndUnsupportedCommands(t *testing.T) {
	repo := filepath.Join("..", "..")
	help := exec.Command("go", "run", "./cmd/tspack", "help")
	help.Dir = repo
	b, err := help.CombinedOutput()
	if err != nil {
		t.Fatalf("help failed: %v\n%s", err, string(b))
	}
	text := string(b)
	for _, cmd := range []string{"check", "update", "sync", "pack", "why", "outdated", "how", "format", "lint", "run", "test", "artifact", "bench", "doom", "inspect", "--version", "help"} {
		if !strings.Contains(text, cmd) {
			t.Fatalf("help missing %s: %s", cmd, text)
		}
	}

	if !strings.Contains(text, "inspect <url> [experimental]") {
		t.Fatalf("help must mark inspect experimental: %s", text)
	}
	if !strings.Contains(text, "run [target]") {
		t.Fatalf("help missing run usage: %s", text)
	}
	for _, unsupported := range []string{"build", "dev", "publish", "add", "remove", "install"} {
		if strings.Contains(text, "tspack "+unsupported) {
			t.Fatalf("help unexpectedly advertises unsupported command %s", unsupported)
		}
	}

	for _, c := range []string{"build", "publish"} {
		cmd := exec.Command("go", "run", "./cmd/tspack", c)
		cmd.Dir = repo
		ob, e := cmd.CombinedOutput()
		if e == nil || !strings.Contains(string(ob), "unknown command") {
			t.Fatalf("expected deterministic unknown command for %s: %v\n%s", c, e, string(ob))
		}
	}
}

func TestCLIUpdateDryRunUnknownFlagFailsDeterministically(t *testing.T) {
	repo := filepath.Join("..", "..")
	cmd := exec.Command("go", "run", "./cmd/tspack", "update", "--dry-run", "--unknown")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failure for unknown flag: %s", string(b))
	}
	if !strings.Contains(string(b), "unknown update flag: --unknown") {
		t.Fatalf("unexpected output: %s", string(b))
	}
}

func TestCLIUpdateDryRunJSON(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const out={ok:true,ir:{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"library",dependencies:[],targets:[{name:"core",export:".",entry:"src/index.ts",runtime:"src/index.ts",types:"dist/index.d.ts",deps:[],peers:[]}],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{types:{},boundaries:{}}}]},diagnostics:[]};
process.stdout.write(JSON.stringify(out));`
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })

	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "src"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, "dist"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "src", "index.ts"), []byte("export const x = 1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "dist", "index.d.ts"), []byte("export declare const x: number\n"), 0o644)

	cmd := exec.Command("go", "run", "./cmd/tspack", "update", "--root", root, "--dry-run", "--json")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("update dry-run --json failed: %v\n%s", err, string(b))
	}
	var report updateDryRunJSONReport
	if e := json.Unmarshal(b, &report); e != nil {
		t.Fatalf("invalid json output: %v\n%s", e, string(b))
	}
	if report.Command != "update" || !report.DryRun || !report.OK {
		t.Fatalf("unexpected json report: %#v", report)
	}
	if report.Summary.Added != 0 || report.Summary.Changed != 0 || report.Summary.Removed != 0 {
		t.Fatalf("expected empty change summary: %#v", report.Summary)
	}
}

func TestCLIRootRecomputesDefaultManifestPathAndRespectsExplicitManifest(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("abs repo: %v", err)
	}
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const expected = process.env.TSPACK_EXPECT_MANIFEST;
const got = process.argv[2];
if (expected && got !== expected) {
  process.stdout.write(JSON.stringify({ok:false,ir:null,diagnostics:[{code:"TSPACK_TEST_UNEXPECTED_MANIFEST_PATH",severity:"error",message:"expected="+expected+" got="+got}]}));
  process.exit(0);
}
const out={ok:true,ir:{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"library",dependencies:[],targets:[{name:"core",export:".",entry:"src/index.ts",runtime:"dist/index.js",types:"dist/index.d.ts",deps:[],peers:[]}],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{types:{},boundaries:{}}}]},diagnostics:[]};
process.stdout.write(JSON.stringify(out));`
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })

	fixtureRoot := t.TempDir()
	_ = os.MkdirAll(filepath.Join(fixtureRoot, "src"), 0o755)
	_ = os.MkdirAll(filepath.Join(fixtureRoot, "dist"), 0o755)
	_ = os.WriteFile(filepath.Join(fixtureRoot, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(fixtureRoot, "src", "index.ts"), []byte("x\n"), 0o644)
	_ = os.WriteFile(filepath.Join(fixtureRoot, "dist", "index.js"), []byte("x\n"), 0o644)
	_ = os.WriteFile(filepath.Join(fixtureRoot, "dist", "index.d.ts"), []byte("x\n"), 0o644)

	binPath := filepath.Join(t.TempDir(), "tspack-test-bin")
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/tspack")
	buildCmd.Dir = repo
	if buildOut, buildErr := buildCmd.CombinedOutput(); buildErr != nil {
		t.Fatalf("build tspack binary failed: %v\n%s", buildErr, string(buildOut))
	}

	cmd := exec.Command(binPath, "check", "--root", fixtureRoot)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "TSPACK_EXPECT_MANIFEST="+filepath.Join(fixtureRoot, "manifest.tsx"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("check with root default manifest failed: %v\n%s", err, string(out))
	}

	explicit := filepath.Join(fixtureRoot, "explicit-manifest.tsx")
	_ = os.WriteFile(explicit, []byte("export default {}\n"), 0o644)
	cmd = exec.Command(binPath, "check", "--root", fixtureRoot, "--manifest", explicit)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "TSPACK_EXPECT_MANIFEST="+explicit)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("check with explicit manifest failed: %v\n%s", err, string(out))
	}
}

func TestDocsCommandsInventoryIncludesCurrentSurface(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "commands.md"))
	if err != nil {
		t.Fatalf("read commands doc: %v", err)
	}
	text := string(doc)
	for _, cmd := range []string{"check", "update", "sync", "why", "outdated", "pack", "format", "lint", "run", "test", "artifact", "bench", "doom", "inspect"} {
		if !strings.Contains(text, "`tspack "+cmd) {
			t.Fatalf("commands doc missing %s", cmd)
		}
	}
	for _, phrase := range []string{"core", "native", "experimental"} {
		if !strings.Contains(text, phrase) {
			t.Fatalf("commands doc missing grouping/stability phrase %q", phrase)
		}
	}
}

func TestCheckJSONWarningOnlyLockfileMissing(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const out={ok:true,ir:{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"library",dependencies:[],targets:[{name:"core",export:".",entry:"src/index.ts",runtime:"dist/index.js",types:"dist/index.d.ts",deps:[],peers:[]}],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{types:{},boundaries:{}}}]},diagnostics:[]};
process.stdout.write(JSON.stringify(out));`
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })
	binPath := buildTspackBinary(t, repo)

	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "src"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, "dist"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "src", "index.ts"), []byte("export const x = 1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "dist", "index.js"), []byte("export const x = 1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "dist", "index.d.ts"), []byte("export declare const x: number\n"), 0o644)

	cmd := exec.Command(binPath, "check", "--root", root, "--json")
	cmd.Dir = repo
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("warnings-only check should exit zero: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "TSPACK_CHECK_LOCKFILE_MISSING") {
		t.Fatalf("expected no duplicated human diagnostics on stderr, got: %s", stderr.String())
	}
	if !strings.HasSuffix(stdout.String(), "\n") {
		t.Fatalf("expected trailing newline on JSON output: %q", stdout.String())
	}

	var report checkJSONReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not valid json: %v\n%s", err, stdout.String())
	}
	if report.Command != "check" || !report.OK {
		t.Fatalf("expected command=check and ok=true, got %+v", report)
	}
	if report.Summary.Errors != 0 || report.Summary.Warnings == 0 || report.Summary.Total == 0 {
		t.Fatalf("unexpected summary: %+v", report.Summary)
	}
	lockfileWarningFound := false
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Code == "TSPACK_CHECK_LOCKFILE_MISSING" {
			lockfileWarningFound = true
			if diagnostic.Severity != "warning" {
				t.Fatalf("expected warning severity for lockfile missing, got %+v", diagnostic)
			}
		}
	}
	if !lockfileWarningFound {
		t.Fatalf("expected TSPACK_CHECK_LOCKFILE_MISSING in diagnostics: %+v", report.Diagnostics)
	}
}

func TestCheckJSONInvalidProjectIsNonZero(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const out={ok:true,ir:{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"library",dependencies:[],targets:[{name:"core",export:".",entry:"src/index.ts",runtime:"dist/index.js",types:"dist/index.d.ts",deps:[],peers:[]}],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{types:{},boundaries:{}}}]},diagnostics:[]};
process.stdout.write(JSON.stringify(out));`
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })
	binPath := buildTspackBinary(t, repo)

	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)

	cmd := exec.Command(binPath, "check", "--root", root, "--json")
	cmd.Dir = repo
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected invalid project to exit nonzero")
	}
	if strings.Contains(stderr.String(), "TSPACK_") {
		t.Fatalf("expected no duplicated human diagnostics on stderr, got: %s", stderr.String())
	}
	var report checkJSONReport
	if unmarshalErr := json.Unmarshal(stdout.Bytes(), &report); unmarshalErr != nil {
		t.Fatalf("stdout is not valid json: %v\n%s", unmarshalErr, stdout.String())
	}
	if report.OK {
		t.Fatalf("expected ok=false for invalid project: %+v", report)
	}
	if report.Summary.Errors == 0 {
		t.Fatalf("expected errors > 0 for invalid project: %+v", report.Summary)
	}
}

func TestCLITestNoBackendsAndVitestUnavailable(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()

	cmd := exec.Command("go", "run", "./cmd/tspack", "test", "--root", root)
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero when no backends are present")
	}
	if !strings.Contains(string(b), "TSPACK_TEST_NO_BACKENDS") {
		t.Fatalf("missing no backends diagnostic: %s", string(b))
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "test", "-vitest", "--root", root)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero when vitest unavailable")
	}
	if !strings.Contains(string(b), "TSPACK_TEST_VITEST_NOT_AVAILABLE") {
		t.Fatalf("missing vitest unavailable diagnostic: %s", string(b))
	}
}

func TestCLIArtifactCommand(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	bridge := filepath.Join(frontend, "native-test-cli.js")
	stub := `#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
const args=process.argv.slice(2);
const mode=args[0];
const root=args[args.indexOf('--root')+1];
const outIdx=args.indexOf('--out');
const out=outIdx>=0?args[outIdx+1]:path.join(root,'.tspack','artifacts');
const filterIdx=args.indexOf('--filter');
const filter=filterIdx>=0?args[filterIdx+1]:'';
if(mode!=='artifact'){process.exit(2)}
if(args.includes('--list')){console.log('TSPack artifacts\n\nPASS '+root+'/a.xtest.tsx::suite/artifact/demo\n');process.exit(0)}
if(filter==='no-match'){console.error('TSPACK_ARTIFACT_FILTER_NO_MATCH: none');process.exit(1)}
if(args.includes('--json')){console.log(JSON.stringify({summary:{total:1,passed:1,failed:0,skipped:0,diagnostics:0},artifacts:[{id:'a',name:'demo',status:'passed'}]},null,2));process.exit(0)}
fs.mkdirSync(out,{recursive:true});fs.writeFileSync(path.join(out,'artifact.txt'),'ok');console.log('PASS wrote');`
	_ = os.WriteFile(bridge, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(bridge) })

	root := t.TempDir()
	cmd := exec.Command("go", "run", "./cmd/tspack", "artifact", "--root", root, "--list")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "TSPack artifacts") {
		t.Fatalf("artifact list failed: %v\n%s", err, string(b))
	}

	out := filepath.Join(root, "out")
	cmd = exec.Command("go", "run", "./cmd/tspack", "artifact", "--root", root, "--out", out)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("artifact run failed: %v\n%s", err, string(b))
	}
	if _, err = os.Stat(filepath.Join(out, "artifact.txt")); err != nil {
		t.Fatalf("expected written artifact: %v", err)
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "artifact", "--root", root, "--filter", "no-match")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_ARTIFACT_FILTER_NO_MATCH") {
		t.Fatalf("expected no-match failure: %v\n%s", err, string(b))
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "artifact", "--root", root, "--json")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "\"summary\"") {
		t.Fatalf("expected json output: %v\n%s", err, string(b))
	}
}

func TestCLIDoomCommand(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	bridge := filepath.Join(frontend, "native-test-cli.js")
	stub := `#!/usr/bin/env node
const args=process.argv.slice(2);
if(args[0]!=='doom'){process.exit(2)}
if(args.includes('--list')){console.log('TSPack doom\n\nPASS demo.prophecy.tsx::suite/prophecy/x\n');process.exit(0)}
const filterIdx=args.indexOf('--filter'); const filter=filterIdx>=0?args[filterIdx+1]:'';
if(filter==='none'){console.error('TSPACK_DOOM_FILTER_NO_MATCH: none');process.exit(1)}
if(args.includes('--json')){console.log(JSON.stringify({summary:{total:1,passed:1,failed:0,skipped:0,diagnostics:0},prophecies:[{id:'x',name:'x',status:'passed'}]},null,2));process.exit(0)}
console.log('PASS demo.prophecy.tsx::suite/prophecy/x');`
	_ = os.WriteFile(bridge, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(bridge) })

	root := t.TempDir()
	cmd := exec.Command("go", "run", "./cmd/tspack", "doom", "--root", root, "--list")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "TSPack doom") {
		t.Fatalf("doom list failed: %v\n%s", err, string(b))
	}
	cmd = exec.Command("go", "run", "./cmd/tspack", "doom", "--root", root)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "PASS") {
		t.Fatalf("doom run failed: %v\n%s", err, string(b))
	}
	cmd = exec.Command("go", "run", "./cmd/tspack", "doom", "--root", root, "--filter", "none")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_DOOM_FILTER_NO_MATCH") {
		t.Fatalf("expected doom no-match failure: %v\n%s", err, string(b))
	}
	cmd = exec.Command("go", "run", "./cmd/tspack", "doom", "--root", root, "--json")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "\"prophecies\"") {
		t.Fatalf("expected doom json output: %v\n%s", err, string(b))
	}
}

func TestCLIDoomBridgeMissing(t *testing.T) {
	repo := filepath.Join("..", "..")
	bridge := filepath.Join(repo, "manifest-frontend", "dist", "src", "native-test-cli.js")
	backup := bridge + ".bak"
	if _, err := os.Stat(bridge); err == nil {
		_ = os.Rename(bridge, backup)
		defer func() { _ = os.Rename(backup, bridge) }()
	}
	cmd := exec.Command("go", "run", "./cmd/tspack", "doom", "--root", t.TempDir())
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_DOOM_BRIDGE_MISSING") {
		t.Fatalf("expected bridge missing diagnostic: %v\n%s", err, string(b))
	}
}

func TestCLIInspectCommandRouting(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	bridge := filepath.Join(frontend, "inspect-cli.js")
	stub := `#!/usr/bin/env node
const args=process.argv.slice(2);
if(!args.includes('http://example.test')){console.error('missing-url');process.exit(1)}
if(!args.includes('--cdp') || !args.includes('http://127.0.0.1:9222')){console.error('missing-cdp');process.exit(1)}
if(!args.includes('--host-path') || !args.includes('/tmp/host')){console.error('missing-host-path');process.exit(1)}
if(!args.includes('--browser-path') || !args.includes('/tmp/browser')){console.error('missing-browser-path');process.exit(1)}
if(!args.includes('--list-targets') || !args.includes('--target') || !args.includes('0') || !args.includes('--target-url') || !args.includes('localhost:5173')){console.error('missing-target-flags');process.exit(1)}
if(args.includes('--json')){console.log('{"ok":true}');process.exit(0)}
console.log(args.join(' '));`
	_ = os.WriteFile(bridge, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(bridge) })

	cmd := exec.Command("go", "run", "./cmd/tspack", "inspect", "http://example.test", "--json", "--cdp", "http://127.0.0.1:9222", "--host-path", "/tmp/host", "--browser-path", "/tmp/browser", "--list-targets", "--target", "0", "--target-url", "localhost:5173")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "{\"ok\":true}") {
		t.Fatalf("inspect routing failed: %v\n%s", err, string(b))
	}
}

func TestCLIInspectRunTargetByName(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	server := `const http=require('http'); const p=5221; http.createServer((_,res)=>{res.statusCode=200;res.end('ok')}).listen(p,'127.0.0.1'); setInterval(()=>{},1000);`
	_ = os.WriteFile(filepath.Join(root, "server.js"), []byte(server), 0o644)
	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","server.js"],url:"http://127.0.0.1:5221",ready:{kind:"http",path:"/"}}]}]}`)

	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	bridge := filepath.Join(frontend, "inspect-cli.js")
	stub := `#!/usr/bin/env node
const args=process.argv.slice(2);
if(args[1] !== 'http://127.0.0.1:5221'){console.error('missing-run-url');process.exit(1)}
console.log('{"ok":true}');`
	_ = os.WriteFile(bridge, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(bridge) })
	cmd := exec.Command("go", "run", "./cmd/tspack", "inspect", "dev", "--root", root, "--json")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "{\"ok\":true}") {
		t.Fatalf("inspect run target failed: %v\n%s", err, string(b))
	}
}

func TestCLIInspectRunTargetConflict(t *testing.T) {
	repo := filepath.Join("..", "..")
	cmd := exec.Command("go", "run", "./cmd/tspack", "inspect", "http://localhost:1234", "--run", "dev")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_INSPECT_INVALID_TARGET_OPTIONS") {
		t.Fatalf("expected conflict failure: %v\n%s", err, string(b))
	}
}

func TestCLIInspectRunFlagExplicit(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	port := reservePort(t)
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	server := fmt.Sprintf(`const http=require('http'); http.createServer((_,res)=>{res.statusCode=200;res.end('ok')}).listen(%d,'127.0.0.1'); setInterval(()=>{},1000);`, port)
	_ = os.WriteFile(filepath.Join(root, "server.js"), []byte(server), 0o644)
	writeRunFrontendStub(t, fmt.Sprintf(`{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","server.js"],url:"http://127.0.0.1:%d",ready:{kind:"http",path:"/"}}]}]}`, port))
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	bridge := filepath.Join(frontend, "inspect-cli.js")
	stub := fmt.Sprintf(`#!/usr/bin/env node
const args=process.argv.slice(2);
if (args[1] !== 'http://127.0.0.1:%d') { process.exit(2); }
console.log('{"ok":true}');`, port)
	_ = os.WriteFile(bridge, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(bridge) })
	cmd := exec.Command("go", "run", "./cmd/tspack", "inspect", "--run", "dev", "--root", root, "--json")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), `{"ok":true}`) || !strings.Contains(string(b), `Stopped run target "dev".`) {
		t.Fatalf("inspect --run failed: %v\n%s", err, string(b))
	}
}

func TestCLIInspectTargetRequiredStillEnforced(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	bridge := filepath.Join(frontend, "inspect-cli.js")
	_ = os.WriteFile(bridge, []byte("#!/usr/bin/env node\nprocess.stderr.write('TSPACK_INSPECT_TARGET_REQUIRED\\n'); process.exit(1)\n"), 0o755)
	t.Cleanup(func() { _ = os.Remove(bridge) })
	cmd := exec.Command("go", "run", "./cmd/tspack", "inspect")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_INSPECT_TARGET_REQUIRED") {
		t.Fatalf("expected inspect target required: %v\n%s", err, string(b))
	}
}

func TestCLIInspectRunTargetNotFound(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","x.js"],url:"http://127.0.0.1:1"}]}]}`)
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	bridge := filepath.Join(frontend, "inspect-cli.js")
	_ = os.WriteFile(bridge, []byte("#!/usr/bin/env node\nprocess.exit(0)\n"), 0o755)
	t.Cleanup(func() { _ = os.Remove(bridge) })
	cmd := exec.Command("go", "run", "./cmd/tspack", "inspect", "nope", "--root", root)
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_INSPECT_RUN_TARGET_NOT_FOUND") {
		t.Fatalf("expected run target not found: %v\n%s", err, string(b))
	}
}

func TestCLIInspectRunTimeoutAndExitedEarly(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "hang.js"), []byte("setInterval(()=>{},1000)\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "exit.js"), []byte("process.exit(0)\n"), 0o644)
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	bridge := filepath.Join(frontend, "inspect-cli.js")
	_ = os.WriteFile(bridge, []byte("#!/usr/bin/env node\nconsole.log('{\"ok\":true}')\n"), 0o755)
	t.Cleanup(func() { _ = os.Remove(bridge) })

	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","hang.js"],url:"http://127.0.0.1:5233",ready:{kind:"http",path:"/"}}]}]}`)
	cmd := exec.Command("go", "run", "./cmd/tspack", "inspect", "--run", "dev", "--root", root, "--run-ready-timeout", "1")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_INSPECT_RUN_READY_TIMEOUT") {
		t.Fatalf("expected inspect run timeout: %v\n%s", err, string(b))
	}

	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","exit.js"],url:"http://127.0.0.1:5234",ready:{kind:"http",path:"/"}}]}]}`)
	cmd = exec.Command("go", "run", "./cmd/tspack", "inspect", "--run", "dev", "--root", root, "--run-ready-timeout", "2")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_INSPECT_RUN_EXITED_EARLY") {
		t.Fatalf("expected inspect run exited early: %v\n%s", err, string(b))
	}
}

func TestCLIInspectRunBridgeFailureStillStopsTargetAndJsonClean(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	port := reservePort(t)
	marker := filepath.Join(root, "run-marker.txt")
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	server := fmt.Sprintf(`const fs=require('fs'); const http=require('http'); fs.writeFileSync(%q,'started');
const srv=http.createServer((_,res)=>{res.statusCode=200;res.end('ok')}); srv.listen(%d,'127.0.0.1'); process.on('SIGTERM',()=>{fs.writeFileSync(%q,'stopped'); srv.close(()=>process.exit(0));}); setInterval(()=>{},1000);`, marker, port, marker)
	_ = os.WriteFile(filepath.Join(root, "server.js"), []byte(server), 0o644)
	writeRunFrontendStub(t, fmt.Sprintf(`{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","server.js"],url:"http://127.0.0.1:%d",ready:{kind:"http",path:"/"}}]}]}`, port))
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	bridge := filepath.Join(frontend, "inspect-cli.js")
	stub := `#!/usr/bin/env node
console.error("bridge-failed");
process.exit(7);`
	_ = os.WriteFile(bridge, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(bridge) })
	cmd := exec.Command("go", "run", "./cmd/tspack", "inspect", "--run", "dev", "--root", root, "--json")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "bridge-failed") || !strings.Contains(string(out), `Stopped run target "dev".`) {
		t.Fatalf("expected bridge failure with shutdown: %v\n%s", err, string(out))
	}
	b, readErr := os.ReadFile(marker)
	if readErr != nil || strings.TrimSpace(string(b)) != "stopped" {
		t.Fatalf("expected stopped marker: err=%v marker=%q output=%s", readErr, string(b), string(out))
	}
}

func TestCLIInspectRunPassesThroughFlagsAndMutationContractAndNoNpmInference(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	port := reservePort(t)
	manifestPath := filepath.Join(root, "manifest.tsx")
	lockPath := filepath.Join(root, "ts-lock.toml")
	_ = os.WriteFile(manifestPath, []byte("export default {}\n"), 0o644)
	_ = os.WriteFile(lockPath, []byte("[lock]\nformat=1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":{"dev":"node no-run-target.js"}}`), 0o644)
	server := fmt.Sprintf(`const http=require('http'); http.createServer((_,res)=>{res.statusCode=200;res.end('ok')}).listen(%d,'127.0.0.1'); setInterval(()=>{},1000);`, port)
	_ = os.WriteFile(filepath.Join(root, "server.js"), []byte(server), 0o644)
	_ = os.MkdirAll(filepath.Join(root, "node_modules", ".bin"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "node_modules", ".bin", "dev-server"), []byte(fmt.Sprintf("#!/bin/sh\nexec node %q\n", filepath.Join(root, "server.js"))), 0o755)
	argsPath := filepath.Join(root, "bridge-args.json")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	bridge := filepath.Join(frontend, "inspect-cli.js")
	stub := fmt.Sprintf(`#!/usr/bin/env node
import fs from 'node:fs';
fs.writeFileSync(%q, JSON.stringify(process.argv.slice(2)));
console.log('{"ok":true}');`, argsPath)
	_ = os.WriteFile(bridge, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(bridge) })
	writeRunFrontendStub(t, fmt.Sprintf(`{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"node",command:["dev-server"],url:"http://127.0.0.1:%d",ready:{kind:"http",path:"/"}}]}]}`, port))
	beforeManifest, _ := os.ReadFile(manifestPath)
	beforeLock, _ := os.ReadFile(lockPath)
	outPath := filepath.Join(root, "inspect.json")
	textPath := filepath.Join(root, "inspect.txt")
	cmd := exec.Command("go", "run", "./cmd/tspack", "inspect", "--run", "dev", "--root", root, "--json", "--out", outPath, "--text", textPath, "--selector", "#root", "--point", "320,148", "--viewport", "1024x768", "--cdp", "http://127.0.0.1:9222", "--host-path", "/tmp/host")
	cmd.Dir = repo
	var stdoutBuf strings.Builder
	var stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()
	stdout := []byte(stdoutBuf.String())
	if err != nil {
		t.Fatalf("inspect run with passthrough flags failed: %v\nstdout=%s\nstderr=%s", err, stdoutBuf.String(), stderrBuf.String())
	}
	var payload map[string]any
	if unmarshalErr := json.Unmarshal(stdout, &payload); unmarshalErr != nil || payload["ok"] != true {
		t.Fatalf("stdout not clean json: err=%v output=%s", unmarshalErr, string(stdout))
	}
	if !strings.Contains(stderrBuf.String(), "Starting run target") || strings.Contains(stdoutBuf.String(), "Starting run target") {
		t.Fatalf("expected progress logs on stderr only; stdout=%q stderr=%q", stdoutBuf.String(), stderrBuf.String())
	}
	argsRaw, readErr := os.ReadFile(argsPath)
	if readErr != nil {
		t.Fatalf("missing bridge args: %v", readErr)
	}
	argsText := string(argsRaw)
	for _, expected := range []string{"--out", outPath, "--text", textPath, "--selector", "#root", "--point", "320,148", "--viewport", "1024x768", "--cdp", "http://127.0.0.1:9222", "--host-path", "/tmp/host"} {
		if !strings.Contains(argsText, expected) {
			t.Fatalf("missing bridge passthrough %q in %s", expected, argsText)
		}
	}
	if !strings.Contains(argsText, fmt.Sprintf("http://127.0.0.1:%d", port)) {
		t.Fatalf("missing resolved run url in bridge args: %s", argsText)
	}
	afterManifest, _ := os.ReadFile(manifestPath)
	afterLock, _ := os.ReadFile(lockPath)
	if string(beforeManifest) != string(afterManifest) || string(beforeLock) != string(afterLock) {
		t.Fatalf("inspect run mutated manifest or lock")
	}
	if _, statErr := os.Stat(filepath.Join(root, "node_modules")); statErr != nil {
		t.Fatalf("expected existing node_modules for local bin fixture: %v", statErr)
	}

	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[]}]} `)
	cmd = exec.Command("go", "run", "./cmd/tspack", "inspect", "dev", "--root", root)
	cmd.Dir = repo
	bad, runErr := cmd.CombinedOutput()
	if runErr == nil || !strings.Contains(string(bad), "TSPACK_INSPECT_RUN_TARGET_NOT_FOUND") {
		t.Fatalf("expected no npm script inference failure: %v\n%s", runErr, string(bad))
	}
}

func TestCLIInspectBridgeMissing(t *testing.T) {
	repo := filepath.Join("..", "..")
	bridge := filepath.Join(repo, "manifest-frontend", "dist", "src", "inspect-cli.js")
	_ = os.Remove(bridge)
	cmd := exec.Command("go", "run", "./cmd/tspack", "inspect", "http://example.test")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_INSPECT_BRIDGE_MISSING") {
		t.Fatalf("expected bridge missing: %v\n%s", err, string(b))
	}
}

func TestHelpMarksInspectExperimental(t *testing.T) {
	repo := filepath.Join("..", "..")
	cmd := exec.Command("go", "run", "./cmd/tspack", "help")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("help failed: %v\n%s", err, string(b))
	}
	if !strings.Contains(string(b), "tspack inspect <url> [experimental]") {
		t.Fatalf("inspect help not marked experimental:\n%s", string(b))
	}
}

func writeRunFrontendStub(t *testing.T, irJSON string) {
	t.Helper()
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := "#!/usr/bin/env node\nconst out={ok:true,ir:" + irJSON + ",diagnostics:[]};process.stdout.write(JSON.stringify(out));"
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })
}

func TestCLIRunOnceSelectionAndErrors(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	server := `const http=require('http'); const p=Number(process.env.PORT||process.argv[2]||5173); http.createServer((_,res)=>{res.statusCode=200;res.end('ok')}).listen(p,'127.0.0.1'); setInterval(()=>{},1000);`
	_ = os.WriteFile(filepath.Join(root, "server.js"), []byte(server), 0o644)

	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","server.js","5191"],url:"http://127.0.0.1:5191",ready:{kind:"http",path:"/"}},{name:"api",runtime:"system",command:["node","server.js","5192"],url:"http://127.0.0.1:5192"}]}]}`)

	cmd := exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--once")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "Ready: http://127.0.0.1:5191") {
		t.Fatalf("run dev failed: %v\n%s", err, string(b))
	}
	cmd = exec.Command("go", "run", "./cmd/tspack", "run", "api", "--root", root, "--once")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "Ready: http://127.0.0.1:5192") {
		t.Fatalf("run api failed: %v\n%s", err, string(b))
	}
	cmd = exec.Command("go", "run", "./cmd/tspack", "run", "nope", "--root", root, "--once")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_RUN_TARGET_NOT_FOUND") {
		t.Fatalf("expected target not found: %v\n%s", err, string(b))
	}
}

func TestCLIRunTimeoutAndInvalidTimeout(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	server := `setInterval(()=>{},1000);`
	_ = os.WriteFile(filepath.Join(root, "hang.js"), []byte(server), 0o644)
	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","hang.js"],url:"http://127.0.0.1:5199",ready:{kind:"http",path:"/"}}]}]}`)
	cmd := exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--ready-timeout", "1", "--once")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_RUN_READY_TIMEOUT") {
		t.Fatalf("expected timeout: %v\n%s", err, string(b))
	}
	cmd = exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--ready-timeout", "0", "--once")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_RUN_INVALID_TIMEOUT") {
		t.Fatalf("expected invalid timeout: %v\n%s", err, string(b))
	}
}

func TestCLIRunNoShellArgvSemantics(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	script := `const fs=require('fs'); const http=require('http'); const path=require('path');
const out=path.join(process.cwd(),'args.txt'); fs.writeFileSync(out, JSON.stringify(process.argv.slice(2)));
const p=Number(process.argv[2]||5201); http.createServer((_,res)=>{res.statusCode=200;res.end('ok')}).listen(p,'127.0.0.1'); setInterval(()=>{},1000);`
	_ = os.WriteFile(filepath.Join(root, "print-args.js"), []byte(script), 0o644)
	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","print-args.js","5201","hello","&&","echo","BAD"],url:"http://127.0.0.1:5201",ready:{kind:"http",path:"/"}}]}]}`)
	cmd := exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--once")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run failed: %v\n%s", err, string(b))
	}
	argsBytes, err := os.ReadFile(filepath.Join(root, "args.txt"))
	if err != nil {
		t.Fatalf("missing args file: %v", err)
	}
	argsText := string(argsBytes)
	if !strings.Contains(argsText, `"&&"`) || !strings.Contains(argsText, `"echo"`) || !strings.Contains(argsText, `"BAD"`) {
		t.Fatalf("expected literal args, got %s", argsText)
	}
}

func TestCLIRunNodeLocalBinPrecedence(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	pathBin := t.TempDir()
	port := reservePort(t)
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, "node_modules", ".bin"), 0o755)
	markerPath := filepath.Join(root, "which-bin.txt")
	localServer := fmt.Sprintf(`const fs=require('fs'); const http=require('http'); fs.writeFileSync(%q,'local-bin'); http.createServer((_,res)=>{res.statusCode=200;res.end('ok')}).listen(%d,'127.0.0.1'); setInterval(()=>{},1000);`, markerPath, port)
	_ = os.WriteFile(filepath.Join(root, "local-server.js"), []byte(localServer), 0o644)
	local := fmt.Sprintf("#!/bin/sh\nexec node %q\n", filepath.Join(root, "local-server.js"))
	pathScript := fmt.Sprintf("#!/bin/sh\necho path-bin > %q\nexit 1\n", markerPath)
	_ = os.WriteFile(filepath.Join(root, "node_modules", ".bin", "fake-dev-server"), []byte(local), 0o755)
	_ = os.WriteFile(filepath.Join(pathBin, "fake-dev-server"), []byte(pathScript), 0o755)
	writeRunFrontendStub(t, fmt.Sprintf(`{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"node",command:["fake-dev-server"],url:"http://127.0.0.1:%d",ready:{kind:"http",path:"/"}}]}]}`, port))
	cmd := exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--once")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+pathBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run failed: %v\n%s", err, string(b))
	}
	bm, err := os.ReadFile(markerPath)
	if err != nil || strings.TrimSpace(string(bm)) != "local-bin" {
		t.Fatalf("expected local-bin marker, err=%v value=%q\noutput=%s", err, string(bm), string(b))
	}
}

func TestCLIRunSelectionAndMutationContract(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.tsx")
	lockPath := filepath.Join(root, "ts-lock.toml")
	_ = os.WriteFile(manifestPath, []byte("export default {}\n"), 0o644)
	_ = os.WriteFile(lockPath, []byte("[lock]\nformat=1\n"), 0o644)
	server := `const http=require('http'); const p=Number(process.argv[2]); http.createServer((_,res)=>{res.statusCode=200;res.end('ok')}).listen(p,'127.0.0.1'); setInterval(()=>{},1000);`
	_ = os.WriteFile(filepath.Join(root, "server.js"), []byte(server), 0o644)

	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[]}]} `)
	cmd := exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--once")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_RUN_TARGET_MISSING") {
		t.Fatalf("expected missing target: %v\n%s", err, string(b))
	}

	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"api",runtime:"system",command:["node","server.js","5203"],url:"http://127.0.0.1:5203",ready:{kind:"http",path:"/"}}]}]} `)
	beforeManifest, _ := os.ReadFile(manifestPath)
	beforeLock, _ := os.ReadFile(lockPath)
	cmd = exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--once")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("single-target fallback failed: %v\n%s", err, string(b))
	}
	afterManifest, _ := os.ReadFile(manifestPath)
	afterLock, _ := os.ReadFile(lockPath)
	if string(beforeManifest) != string(afterManifest) || string(beforeLock) != string(afterLock) {
		t.Fatalf("run mutated manifest or lock")
	}
	if _, statErr := os.Stat(filepath.Join(root, "node_modules")); statErr == nil {
		t.Fatalf("run unexpectedly created node_modules")
	}

	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"api",runtime:"system",command:["node","server.js","5204"],url:"http://127.0.0.1:5204"},{name:"docs",runtime:"system",command:["node","server.js","5205"],url:"http://127.0.0.1:5205"}]}]} `)
	cmd = exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--once")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_RUN_TARGET_AMBIGUOUS") {
		t.Fatalf("expected ambiguous target failure: %v\n%s", err, string(b))
	}
}

func TestCLIRunProcessExitedEarly(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "exit.js"), []byte("process.exit(0);\n"), 0o644)
	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","exit.js"],url:"http://127.0.0.1:5210",ready:{kind:"http",path:"/"}}]}]} `)
	cmd := exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--once", "--ready-timeout", "2")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_RUN_PROCESS_EXITED_EARLY") {
		t.Fatalf("expected exited early: %v\n%s", err, string(b))
	}
}

func TestCLIHelpIncludesFormatAndLint(t *testing.T) {
	repo := filepath.Join("..", "..")
	cmd := exec.Command("go", "run", "./cmd/tspack", "help")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("help failed: %v\n%s", err, string(b))
	}
	out := string(b)
	if !strings.Contains(out, "tspack format") || !strings.Contains(out, "tspack lint") {
		t.Fatalf("help missing format/lint: %s", out)
	}
}

func TestCLIBiomeBackendResolutionAndArgs(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	binDir := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(root, "capture.json")
	backend := "#!/usr/bin/env node\nconst fs=require('fs');const p=process.env.TSPACK_CAPTURE;fs.writeFileSync(p,JSON.stringify({argv:process.argv.slice(2),cwd:process.cwd()}));process.stdout.write('LOCAL_BACKEND\\n');"
	if err := os.WriteFile(filepath.Join(binDir, "biome"), []byte(backend), 0o755); err != nil {
		t.Fatal(err)
	}
	pathDir := t.TempDir()
	pathBackend := "#!/usr/bin/env node\nprocess.stdout.write('PATH_BACKEND\\n');"
	if err := os.WriteFile(filepath.Join(pathDir, "biome"), []byte(pathBackend), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", "./cmd/tspack", "format", "src", "tests", "--root", root)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+pathDir+":"+os.Getenv("PATH"), "TSPACK_CAPTURE="+capture)
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("format failed: %v\n%s", err, string(b))
	}
	if !strings.Contains(string(b), "LOCAL_BACKEND") {
		t.Fatalf("expected local backend: %s", string(b))
	}
	if strings.Contains(string(b), "PATH_BACKEND") {
		t.Fatalf("path backend should not run: %s", string(b))
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Argv []string `json:"argv"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got.Argv, " ")
	if !strings.Contains(joined, "format") || !strings.Contains(joined, "--write") || !strings.Contains(joined, "src") || !strings.Contains(joined, "tests") {
		t.Fatalf("unexpected argv: %v", got.Argv)
	}
}

func TestCLIBiomeMissingBackendAndInvalidFlags(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	cmd := exec.Command("go", "run", "./cmd/tspack", "format", "--root", root)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+t.TempDir())
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_BIOME_BACKEND_NOT_FOUND") {
		t.Fatalf("missing backend diagnostic not shown: %v\n%s", err, string(b))
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "format", "--fix", "--root", root)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_FORMAT_INVALID_FLAGS") {
		t.Fatalf("format invalid flags missing: %v\n%s", err, string(b))
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "lint", "--check", "--root", root)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_LINT_INVALID_FLAGS") {
		t.Fatalf("lint invalid flags missing: %v\n%s", err, string(b))
	}
}

func TestCLIHowCommand(t *testing.T) {
	repo := filepath.Join("..", "..")
	cmd := exec.Command("go", "run", "./cmd/tspack", "how")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_HOW_CODE_REQUIRED") {
		t.Fatalf("expected required code diagnostic: %v\n%s", err, string(b))
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "how", "NOPE")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_HOW_CODE_NOT_FOUND") || !strings.Contains(string(b), "tspack how --list") {
		t.Fatalf("expected not found diagnostic: %v\n%s", err, string(b))
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "how", "TSPACK_LOCK_VERSION_CONFLICT")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("how new code failed: %v\n%s", err, string(b))
	}
	text := string(b)
	if !strings.Contains(text, "multiple versions") || !strings.Contains(text, "tspack why") || !strings.Contains(text, "valid") {
		t.Fatalf("expected guidance for conflict diagnostic: %s", text)
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "how", "TSPACK_IR_INVALID_RELATIVE_PATH")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("how known code failed: %v\n%s", err, string(b))
	}
	if !strings.Contains(string(b), "types: \"\"") {
		t.Fatalf("expected app types empty string note: %s", string(b))
	}
}

func TestCLIDiagnosticDetailsPrintedAndJSONStructured(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const out={ok:false,ir:null,diagnostics:[{code:"TSPACK_TEST_DETAIL",severity:"error",message:"manifest invalid",details:["package=dep-a@1.0.0","reason=broken"]}]};
process.stdout.write(JSON.stringify(out));`
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })

	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	bin := buildTspackBinary(t, repo)

	cmd := exec.Command(bin, "check", "--root", root)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected check failure")
	}
	text := string(out)
	if !strings.Contains(text, "TSPACK_TEST_DETAIL: manifest invalid") {
		t.Fatalf("missing primary diagnostic: %s", text)
	}
	if !strings.Contains(text, "  package=dep-a@1.0.0") || !strings.Contains(text, "  reason=broken") {
		t.Fatalf("missing indented details: %s", text)
	}

	jsonCmd := exec.Command(bin, "check", "--root", root, "--json")
	jsonCmd.Dir = repo
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	jsonCmd.Stdout = stdout
	jsonCmd.Stderr = stderr
	jerr := jsonCmd.Run()
	if jerr == nil {
		t.Fatalf("expected json check failure")
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no human diagnostics on stderr in --json mode, got %q", stderr.String())
	}
	var report map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode json output: %v", err)
	}
	diagnostics, ok := report["diagnostics"].([]any)
	if !ok || len(diagnostics) == 0 {
		t.Fatalf("expected diagnostics in json output: %s", stdout.String())
	}
	var found map[string]any
	for _, raw := range diagnostics {
		m, _ := raw.(map[string]any)
		if m["code"] == "TSPACK_TEST_DETAIL" {
			found = m
			break
		}
	}
	if found == nil {
		t.Fatalf("expected TSPACK_TEST_DETAIL in json diagnostics: %#v", diagnostics)
	}
	details, ok := found["details"].([]any)
	if !ok || len(details) != 2 {
		t.Fatalf("expected structured details in json output: %#v", found)
	}
}

func TestCheckVersionConflictTextAndLockfileImmutable(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const out={ok:true,ir:{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"library",dependencies:[],targets:[{name:"core",export:".",entry:"src/index.ts",runtime:"dist/index.js",types:"dist/index.d.ts",deps:[],peers:[]}],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{types:{},boundaries:{}}}]},diagnostics:[]};
process.stdout.write(JSON.stringify(out));`
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })
	binPath := buildTspackBinary(t, repo)

	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "src"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, "dist"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "src", "index.ts"), []byte("export const x = 1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "dist", "index.js"), []byte("export const x = 1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "dist", "index.d.ts"), []byte("export declare const x: number\n"), 0o644)

	lockfilePath := filepath.Join(root, "ts-lock.toml")
	lockBody := "[lock]\nformat = 1\ntool = \"tspack\"\n\n[[package]]\nid = \"npm:react@18.3.1\"\nname = \"react\"\nversion = \"18.3.1\"\nsource = \"npm\"\nintegrity = \"sha512-a\"\n\n[[package]]\nid = \"npm:react@19.2.6\"\nname = \"react\"\nversion = \"19.2.6\"\nsource = \"npm\"\nintegrity = \"sha512-b\"\n\n[[target]]\npackage = \"app\"\nname = \"core\"\nexport = \".\"\nentry = \"src/index.ts\"\nruntime = \"dist/index.js\"\ntypes = \"dist/index.d.ts\"\n"
	_ = os.WriteFile(lockfilePath, []byte(lockBody), 0o644)
	before, _ := os.ReadFile(lockfilePath)

	cmd := exec.Command(binPath, "check", "--root", root)
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("warnings-only check should exit zero: %v\n%s", err, string(b))
	}
	out := string(b)
	if !strings.Contains(out, "TSPACK_LOCK_VERSION_CONFLICT") {
		t.Fatalf("expected conflict warning in text output: %s", out)
	}
	if !strings.Contains(out, "npm:react@18.3.1") || !strings.Contains(out, "npm:react@19.2.6") {
		t.Fatalf("expected both package IDs in output: %s", out)
	}

	after, _ := os.ReadFile(lockfilePath)
	if string(before) != string(after) {
		t.Fatalf("lockfile changed after check")
	}
}

func TestCheckVersionConflictJSON(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const out={ok:true,ir:{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"library",dependencies:[],targets:[{name:"core",export:".",entry:"src/index.ts",runtime:"dist/index.js",types:"dist/index.d.ts",deps:[],peers:[]}],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{types:{},boundaries:{}}}]},diagnostics:[]};
process.stdout.write(JSON.stringify(out));`
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })
	binPath := buildTspackBinary(t, repo)

	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "src"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, "dist"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "src", "index.ts"), []byte("export const x = 1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "dist", "index.js"), []byte("export const x = 1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "dist", "index.d.ts"), []byte("export declare const x: number\n"), 0o644)

	lockfilePath := filepath.Join(root, "ts-lock.toml")
	lockBody := "[lock]\nformat = 1\ntool = \"tspack\"\n\n[[package]]\nid = \"npm:react@18.3.1\"\nname = \"react\"\nversion = \"18.3.1\"\nsource = \"npm\"\nintegrity = \"sha512-a\"\n\n[[package]]\nid = \"npm:react@19.2.6\"\nname = \"react\"\nversion = \"19.2.6\"\nsource = \"npm\"\nintegrity = \"sha512-b\"\n\n[[target]]\npackage = \"app\"\nname = \"core\"\nexport = \".\"\nentry = \"src/index.ts\"\nruntime = \"dist/index.js\"\ntypes = \"dist/index.d.ts\"\n"
	_ = os.WriteFile(lockfilePath, []byte(lockBody), 0o644)

	cmd := exec.Command(binPath, "check", "--root", root, "--json")
	cmd.Dir = repo
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("warnings-only json check should exit zero: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "TSPACK_LOCK_VERSION_CONFLICT") {
		t.Fatalf("expected no duplicated human diagnostics on stderr, got: %s", stderr.String())
	}

	var report checkJSONReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, stdout.String())
	}
	if !report.OK {
		t.Fatalf("expected ok=true for warnings-only report: %+v", report)
	}
	if report.Summary.Warnings == 0 {
		t.Fatalf("expected warnings in summary: %+v", report.Summary)
	}
	found := false
	for _, d := range report.Diagnostics {
		if d.Code == "TSPACK_LOCK_VERSION_CONFLICT" {
			found = true
			detailsText := strings.Join(d.Details, "\n")
			if !strings.Contains(detailsText, "npm:react@18.3.1") || !strings.Contains(detailsText, "npm:react@19.2.6") {
				t.Fatalf("expected both package IDs in details: %#v", d)
			}
		}
	}
	if !found {
		t.Fatalf("expected TSPACK_LOCK_VERSION_CONFLICT in diagnostics: %#v", report.Diagnostics)
	}
}

func TestCLIUpdateTargetedDryRunJSONIncludesTargetFieldsOnlyJSON(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const out={ok:true,ir:{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"library",dependencies:[{key:"react",kind:"dep",source:{kind:"npm",package:"react",range:"18.2.0"}}],targets:[{name:"core",export:".",entry:"src/index.ts",runtime:"src/index.ts",types:"dist/index.d.ts",deps:["react"],peers:[]}],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{types:{},boundaries:{}}}]},diagnostics:[]};
process.stdout.write(JSON.stringify(out));`
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })

	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "src"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, "dist"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "src", "index.ts"), []byte("export const x=1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "dist", "index.d.ts"), []byte("export declare const x:number\n"), 0o644)

	cmd := exec.Command("go", "run", "./cmd/tspack", "update", "react", "--root", root, "--dry-run", "--json")
	cmd.Dir = repo
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("targeted dry-run json failed: %v", err)
	}
	if stderr.String() != "" {
		t.Fatalf("expected no stderr output in json mode, got %q", stderr.String())
	}
	var report map[string]any
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, string(out))
	}
	if report["targeted"] != true || report["query"] != "react" {
		t.Fatalf("missing targeted fields: %#v", report)
	}
	selected, ok := report["selected"].([]any)
	if !ok || len(selected) != 1 {
		t.Fatalf("expected selected target, got %#v", report["selected"])
	}
}
