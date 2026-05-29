package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
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

type whyJSONReport struct {
	Command string  `json:"command"`
	Mode    string  `json:"mode"`
	Query   string  `json:"query"`
	Package *string `json:"package"`
	OK      bool    `json:"ok"`
	Summary struct {
		Explanations int `json:"explanations"`
		LockPackages int `json:"lockPackages"`
		ReversePaths int `json:"reversePaths"`
		Diagnostics  int `json:"diagnostics"`
		Warnings     int `json:"warnings"`
		Errors       int `json:"errors"`
	} `json:"summary"`
	Explanations []struct {
		Kind           string `json:"kind"`
		PackageName    string `json:"package"`
		DependencyKey  string `json:"dependencyKey"`
		DependencyKind string `json:"dependencyKind"`
		TargetName     string `json:"targetName"`
		Source         struct {
			Kind    string `json:"kind"`
			Package string `json:"package"`
			Range   string `json:"range"`
		} `json:"source"`
		ReachableFrom []struct {
			Ref string `json:"ref"`
		} `json:"reachableFrom"`
		LockPackages []struct {
			ID string `json:"id"`
		} `json:"lockPackages"`
		LockEdges []struct {
			From     string `json:"from"`
			To       string `json:"to"`
			Kind     string `json:"kind"`
			Optional bool   `json:"optional"`
		} `json:"lockEdges"`
	} `json:"explanations"`
	LockPackages []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Version string `json:"version"`
		Source  string `json:"source"`
	} `json:"lockPackages"`
	Reverse []struct {
		LockPackage string   `json:"lockPackage"`
		Root        string   `json:"root"`
		Path        []string `json:"path"`
		Edges       []struct {
			From     string `json:"from"`
			To       string `json:"to"`
			Kind     string `json:"kind"`
			Optional bool   `json:"optional"`
		} `json:"edges"`
	} `json:"reverse"`
	Notes       []string `json:"notes"`
	Diagnostics []struct {
		Code     string   `json:"code"`
		Severity string   `json:"severity"`
		Message  string   `json:"message"`
		Details  []string `json:"details"`
	} `json:"diagnostics"`
}

func TestCLIWhyJSON(t *testing.T) {
	repo := filepath.Join("..", "..")
	bin := buildTspackBinary(t, repo)
	root := setupWhyJSONWorkspace(t, repo)
	lockfile := filepath.Join(root, "ts-lock.toml")

	runWhyJSON := func(args ...string) (whyJSONReport, string, string, error) {
		fullArgs := append([]string{"why"}, args...)
		fullArgs = append(fullArgs, "--root", root, "--lockfile", lockfile)
		cmd := exec.Command(bin, fullArgs...)
		cmd.Dir = repo
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		var report whyJSONReport
		if unmarshalErr := json.Unmarshal(stdout.Bytes(), &report); unmarshalErr != nil {
			t.Fatalf("stdout should be parseable JSON: %v\nstdout: %s\nstderr: %s", unmarshalErr, stdout.String(), stderr.String())
		}
		return report, stdout.String(), stderr.String(), err
	}

	reactReport, firstBytes, stderrText, err := runWhyJSON("react", "--json")
	if err != nil {
		t.Fatalf("why react --json failed: %v\nstdout: %s\nstderr: %s", err, firstBytes, stderrText)
	}
	if stderrText != "" {
		t.Fatalf("why react --json should not write stderr, got: %s", stderrText)
	}
	if reactReport.Command != "why" || reactReport.Query != "react" || !reactReport.OK {
		t.Fatalf("unexpected report header: %#v", reactReport)
	}
	if reactReport.Summary.Explanations == 0 || len(reactReport.Explanations) == 0 {
		t.Fatalf("expected explanations: %#v", reactReport)
	}
	if !whyJSONHasDependencyExplanation(reactReport, "@acme/components", "react", "peer") {
		t.Fatalf("expected dependency explanation for @acme/components react peer: %#v", reactReport.Explanations)
	}
	if !whyJSONHasLockEdge(reactReport, "@acme/components:target:core", "npm:react@19.2.6", "peer") {
		t.Fatalf("expected structured scoped lock edge: %#v", reactReport.Explanations)
	}

	scopedReport, _, scopedStderr, err := runWhyJSON("react", "--package", "@acme/components", "--json")
	if err != nil {
		t.Fatalf("why react --package --json failed: %v\nstderr: %s", err, scopedStderr)
	}
	if scopedStderr != "" {
		t.Fatalf("package scoped why json should not write stderr, got: %s", scopedStderr)
	}
	if scopedReport.Package == nil || *scopedReport.Package != "@acme/components" {
		t.Fatalf("expected package filter in report: %#v", scopedReport.Package)
	}
	for _, explanation := range scopedReport.Explanations {
		if explanation.PackageName != "@acme/components" {
			t.Fatalf("expected only @acme/components explanations, got %#v", scopedReport.Explanations)
		}
	}

	lockIDReport, _, lockIDStderr, err := runWhyJSON("npm:loose-envify@1.4.0", "--json")
	if err != nil {
		t.Fatalf("why lock ID --json failed: %v\nstderr: %s", err, lockIDStderr)
	}
	if lockIDStderr != "" {
		t.Fatalf("lock ID why json should not write stderr, got: %s", lockIDStderr)
	}
	if !whyJSONHasLockPackage(lockIDReport, "npm:loose-envify@1.4.0") {
		t.Fatalf("expected lock package explanation: %#v", lockIDReport.Explanations)
	}
	if countWhyJSONLockEdge(lockIDReport, "npm:react@19.2.6", "npm:loose-envify@1.4.0", "runtime") != 1 {
		t.Fatalf("expected one inbound dependent edge: %#v", lockIDReport.Explanations)
	}

	notFoundReport, _, notFoundStderr, err := runWhyJSON("loose-envify", "--json")
	if err == nil {
		t.Fatalf("why loose-envify --json should preserve nonzero not-found semantics")
	}
	if notFoundStderr != "" {
		t.Fatalf("not-found why json should not write stderr, got: %s", notFoundStderr)
	}
	if notFoundReport.OK || notFoundReport.Summary.Errors == 0 {
		t.Fatalf("expected not-found report to be not ok: %#v", notFoundReport)
	}
	if !whyJSONHasDiagnosticDetail(notFoundReport, "TSPACK_WHY_NOT_FOUND", "npm:loose-envify@1.4.0") || !whyJSONHasDiagnosticDetail(notFoundReport, "TSPACK_WHY_NOT_FOUND", "tspack why npm:loose-envify@1.4.0") {
		t.Fatalf("expected structured not-found suggestion details: %#v", notFoundReport.Diagnostics)
	}

	reverseReport, reverseBytes, reverseStderr, err := runWhyJSON("--reverse", "loose-envify", "--json")
	if err != nil {
		t.Fatalf("reverse why loose-envify --json failed: %v\nstdout: %s\nstderr: %s", err, reverseBytes, reverseStderr)
	}
	if reverseStderr != "" {
		t.Fatalf("reverse why json should not write stderr, got: %s", reverseStderr)
	}
	if reverseReport.Mode != "reverse" || !reverseReport.OK || reverseReport.Summary.LockPackages != 1 || reverseReport.Summary.ReversePaths != 2 {
		t.Fatalf("unexpected reverse report summary: %#v", reverseReport)
	}
	if !whyJSONHasReversePath(reverseReport, "npm:loose-envify@1.4.0", "@acme/components:target:core") {
		t.Fatalf("expected components reverse path: %#v", reverseReport.Reverse)
	}
	if !whyJSONHasReversePath(reverseReport, "npm:loose-envify@1.4.0", "@acme/demo:target:app") {
		t.Fatalf("expected demo reverse path: %#v", reverseReport.Reverse)
	}

	reverseRepeatReport, reverseRepeatBytes, reverseRepeatStderr, err := runWhyJSON("--reverse", "loose-envify", "--json")
	if err != nil {
		t.Fatalf("repeat reverse why failed: %v\nstderr: %s", err, reverseRepeatStderr)
	}
	if reverseBytes != reverseRepeatBytes || !reflect.DeepEqual(reverseReport, reverseRepeatReport) {
		t.Fatalf("reverse why json should be deterministic\nfirst: %s\nrepeat: %s", reverseBytes, reverseRepeatBytes)
	}

	filteredReverse, _, filteredReverseStderr, err := runWhyJSON("--reverse", "loose-envify", "--package", "@acme/demo", "--json")
	if err != nil {
		t.Fatalf("filtered reverse why failed: %v\nstderr: %s", err, filteredReverseStderr)
	}
	if len(filteredReverse.Reverse) != 1 || filteredReverse.Reverse[0].Root != "@acme/demo:target:app" {
		t.Fatalf("expected only demo reverse path, got %#v", filteredReverse.Reverse)
	}

	if err := os.Remove(lockfile); err != nil {
		t.Fatalf("remove lockfile: %v", err)
	}
	missingLockReport, _, missingLockStderr, err := runWhyJSON("react", "--json")
	if err != nil {
		t.Fatalf("missing-lockfile why react --json should follow warning-only success semantics: %v\nstderr: %s", err, missingLockStderr)
	}
	if missingLockStderr != "" {
		t.Fatalf("missing-lockfile why json should not write stderr, got: %s", missingLockStderr)
	}
	if !missingLockReport.OK || missingLockReport.Summary.Warnings == 0 || missingLockReport.Summary.Explanations == 0 {
		t.Fatalf("expected warning plus manifest explanations: %#v", missingLockReport)
	}
	if !whyJSONHasDiagnostic(missingLockReport, "TSPACK_WHY_LOCKFILE_MISSING", "warning") {
		t.Fatalf("expected missing lockfile warning: %#v", missingLockReport.Diagnostics)
	}

	reverseMissingLockReport, _, reverseMissingLockStderr, err := runWhyJSON("--reverse", "react", "--json")
	if err == nil {
		t.Fatalf("reverse missing-lockfile why should exit nonzero")
	}
	if reverseMissingLockStderr != "" {
		t.Fatalf("reverse missing-lockfile why json should not write stderr, got: %s", reverseMissingLockStderr)
	}
	if reverseMissingLockReport.OK || !whyJSONHasDiagnostic(reverseMissingLockReport, "TSPACK_WHY_LOCKFILE_MISSING", "error") {
		t.Fatalf("expected reverse missing lockfile error report: %#v", reverseMissingLockReport)
	}

	if err := os.WriteFile(lockfile, []byte(whyJSONLockfile()), 0o644); err != nil {
		t.Fatalf("restore lockfile: %v", err)
	}
	repeatReport, repeatBytes, repeatStderr, err := runWhyJSON("react", "--json")
	if err != nil {
		t.Fatalf("repeat why react --json failed: %v\nstderr: %s", err, repeatStderr)
	}
	if firstBytes != repeatBytes || !reflect.DeepEqual(reactReport, repeatReport) {
		t.Fatalf("why json should be deterministic\nfirst: %s\nrepeat: %s", firstBytes, repeatBytes)
	}

	textCmd := exec.Command(bin, "why", "react", "--root", root, "--lockfile", lockfile)
	textCmd.Dir = repo
	var textStdout bytes.Buffer
	var textStderr bytes.Buffer
	textCmd.Stdout = &textStdout
	textCmd.Stderr = &textStderr
	if err := textCmd.Run(); err != nil {
		t.Fatalf("why react text mode failed: %v\nstderr: %s", err, textStderr.String())
	}
	if !strings.Contains(textStdout.String(), "react declared in package") {
		t.Fatalf("expected human text output, got: %s", textStdout.String())
	}
	var textJSON whyJSONReport
	if json.Unmarshal(textStdout.Bytes(), &textJSON) == nil {
		t.Fatalf("text mode should not emit JSON: %s", textStdout.String())
	}
}

func setupWhyJSONWorkspace(t *testing.T, repo string) string {
	t.Helper()
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	if err := os.MkdirAll(frontend, 0o755); err != nil {
		t.Fatalf("create frontend dir: %v", err)
	}
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const out={ok:true,ir:{format:1,workspace:{name:"ws"},packages:[{name:"@acme/components",version:"1.0.0",kind:"library",dependencies:[{key:"react",kind:"peer",source:{kind:"npm",package:"react",range:">=18 <20"}}],targets:[{name:"core",export:".",entry:"src/components.ts",runtime:"src/components.ts",types:"dist/components.d.ts",deps:[],peers:["react"]}],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{types:{},boundaries:{}}},{name:"@acme/demo",version:"1.0.0",kind:"library",dependencies:[{key:"react",kind:"runtime",source:{kind:"npm",package:"react",range:"^18.3.1"}}],targets:[{name:"app",export:"./app",entry:"src/demo.ts",runtime:"src/demo.ts",types:"dist/demo.d.ts",deps:["react"],peers:[]}],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{types:{},boundaries:{}}}]},diagnostics:[]};
process.stdout.write(JSON.stringify(out));`
	if err := os.WriteFile(cliPath, []byte(stub), 0o755); err != nil {
		t.Fatalf("write frontend stub: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(cliPath) })

	root := t.TempDir()
	for _, dir := range []string{"src", "dist"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}
	files := map[string]string{
		"manifest.tsx":         "export default {}\n",
		"src/components.ts":    "x\n",
		"src/demo.ts":          "x\n",
		"dist/components.d.ts": "x\n",
		"dist/demo.d.ts":       "x\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "ts-lock.toml"), []byte(whyJSONLockfile()), 0o644); err != nil {
		t.Fatalf("write lockfile: %v", err)
	}
	return root
}

func whyJSONLockfile() string {
	return "[lock]\nformat=1\ntool=\"tspack\"\n" +
		"[[package]]\nid=\"npm:react@19.2.6\"\nname=\"react\"\nversion=\"19.2.6\"\nsource=\"npm\"\nhash=\"h\"\n" +
		"[[package]]\nid=\"npm:react@18.3.1\"\nname=\"react\"\nversion=\"18.3.1\"\nsource=\"npm\"\nhash=\"h\"\n" +
		"[[package]]\nid=\"npm:loose-envify@1.4.0\"\nname=\"loose-envify\"\nversion=\"1.4.0\"\nsource=\"npm\"\nhash=\"h\"\n" +
		"[[edge]]\nfrom=\"@acme/components:target:core\"\nto=\"npm:react@19.2.6\"\nkind=\"peer\"\noptional=false\n" +
		"[[edge]]\nfrom=\"@acme/demo:target:app\"\nto=\"npm:react@18.3.1\"\nkind=\"runtime\"\noptional=false\n" +
		"[[edge]]\nfrom=\"npm:react@19.2.6\"\nto=\"npm:loose-envify@1.4.0\"\nkind=\"runtime\"\noptional=false\n" +
		"[[edge]]\nfrom=\"npm:react@18.3.1\"\nto=\"npm:loose-envify@1.4.0\"\nkind=\"runtime\"\noptional=false\n" +
		"[[target]]\npackage=\"@acme/components\"\nname=\"core\"\nexport=\".\"\nentry=\"src/components.ts\"\nruntime=\"src/components.ts\"\ntypes=\"dist/components.d.ts\"\n" +
		"[[target]]\npackage=\"@acme/demo\"\nname=\"app\"\nexport=\"./app\"\nentry=\"src/demo.ts\"\nruntime=\"src/demo.ts\"\ntypes=\"dist/demo.d.ts\"\n"
}

func whyJSONHasDependencyExplanation(report whyJSONReport, packageName string, dependencyKey string, dependencyKind string) bool {
	for _, explanation := range report.Explanations {
		if explanation.Kind == "dependency" && explanation.PackageName == packageName && explanation.DependencyKey == dependencyKey && explanation.DependencyKind == dependencyKind {
			return true
		}
	}
	return false
}

func whyJSONHasLockPackage(report whyJSONReport, id string) bool {
	for _, explanation := range report.Explanations {
		for _, lockPackage := range explanation.LockPackages {
			if lockPackage.ID == id {
				return true
			}
		}
	}
	return false
}

func whyJSONHasLockEdge(report whyJSONReport, from string, to string, kind string) bool {
	return countWhyJSONLockEdge(report, from, to, kind) > 0
}

func countWhyJSONLockEdge(report whyJSONReport, from string, to string, kind string) int {
	count := 0
	for _, explanation := range report.Explanations {
		for _, edge := range explanation.LockEdges {
			if edge.From == from && edge.To == to && edge.Kind == kind {
				count++
			}
		}
	}
	return count
}

func whyJSONHasReversePath(report whyJSONReport, lockPackage string, root string) bool {
	for _, path := range report.Reverse {
		if path.LockPackage == lockPackage && path.Root == root {
			return true
		}
	}
	return false
}

func whyJSONHasDiagnostic(report whyJSONReport, code string, severity string) bool {
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Code == code && diagnostic.Severity == severity {
			return true
		}
	}
	return false
}

func whyJSONHasDiagnosticDetail(report whyJSONReport, code string, detailSubstring string) bool {
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Code != code {
			continue
		}
		for _, detail := range diagnostic.Details {
			if strings.Contains(detail, detailSubstring) {
				return true
			}
		}
	}
	return false
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

func TestDocsReleaseGateIncludesPhase3BoundarySmoke(t *testing.T) {
	phase3Doc := filepath.Join("..", "..", "docs", "claude-fooding-phase3.md")
	if _, err := os.Stat(phase3Doc); err != nil {
		t.Fatalf("phase 3 closeout doc missing: %v", err)
	}

	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "release-gate.md"))
	if err != nil {
		t.Fatalf("read release gate doc: %v", err)
	}

	text := string(doc)
	for _, phrase := range []string{
		"`tspack check --explain src/file.ts`",
		"`tspack how TSPACK_BOUNDARY_EXPLICIT_DENY`",
		"`tspack how TSPACK_BOUNDARY_ALLOW_ONLY_VIOLATION`",
		"`tspack how TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY`",
	} {
		if !strings.Contains(text, phrase) {
			t.Fatalf("release gate doc missing phase 3 boundary smoke phrase %q", phrase)
		}
	}
}

func TestDocsReleaseGateIncludesPhase4NativeXTestSmoke(t *testing.T) {
	phase4Doc := filepath.Join("..", "..", "docs", "claude-fooding-phase4.md")
	if _, err := os.Stat(phase4Doc); err != nil {
		t.Fatalf("phase 4 closeout doc missing: %v", err)
	}

	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "release-gate.md"))
	if err != nil {
		t.Fatalf("read release gate doc: %v", err)
	}

	text := string(doc)
	for _, phrase := range []string{
		"`tspack test --batch`",
		"`tspack test --watch`",
		"`tspack test --update-snapshots`",
		"assert.type",
		"snapshot",
	} {
		if !strings.Contains(text, phrase) {
			t.Fatalf("release gate doc missing phase 4 native xTest smoke phrase %q", phrase)
		}
	}
}

func TestDocsReleaseGateIncludesPhase5RunTargetSmoke(t *testing.T) {
	phase5Doc := filepath.Join("..", "..", "docs", "claude-fooding-phase5.md")
	if _, err := os.Stat(phase5Doc); err != nil {
		t.Fatalf("phase 5 closeout doc missing: %v", err)
	}

	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "release-gate.md"))
	if err != nil {
		t.Fatalf("read release gate doc: %v", err)
	}

	text := string(doc)
	for _, phrase := range []string{
		"`tspack run --list`",
		"`tspack run --package <pkg> <target> --once`",
		"stdout-match",
		"--env",
		"`tspack doctor run --json`",
	} {
		if !strings.Contains(text, phrase) {
			t.Fatalf("release gate doc missing phase 5 RunTarget smoke phrase %q", phrase)
		}
	}
}

func TestDocsReleaseGateIncludesPhase6PackWhySmoke(t *testing.T) {
	phase6Doc := filepath.Join("..", "..", "docs", "claude-fooding-phase6.md")
	if _, err := os.Stat(phase6Doc); err != nil {
		t.Fatalf("phase 6 closeout doc missing: %v", err)
	}

	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "release-gate.md"))
	if err != nil {
		t.Fatalf("read release gate doc: %v", err)
	}

	text := string(doc)
	for _, phrase := range []string{
		"`tspack pack --verify`",
		"`tspack why --json`",
		"`tspack why --reverse <name>`",
		"CHANGELOG.md",
		"peerDependencies",
	} {
		if !strings.Contains(text, phrase) {
			t.Fatalf("release gate doc missing phase 6 pack/why smoke phrase %q", phrase)
		}
	}
}

func TestDocsReleaseGateIncludesPhase7SecuritySmoke(t *testing.T) {
	phase7Doc := filepath.Join("..", "..", "docs", "claude-fooding-phase7.md")
	if _, err := os.Stat(phase7Doc); err != nil {
		t.Fatalf("phase 7 closeout doc missing: %v", err)
	}

	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "release-gate.md"))
	if err != nil {
		t.Fatalf("read release gate doc: %v", err)
	}

	text := string(doc)
	for _, phrase := range []string{
		"doctor security",
		"lifecycle.runScript",
		"acknowledgedCapabilities",
		"behaviorFixture",
		"behaviorReport",
		"OS jail",
		"non-execution",
	} {
		if !strings.Contains(text, phrase) {
			t.Fatalf("release gate doc missing phase 7 security smoke phrase %q", phrase)
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

func TestDefaultBiomeConfigContent(t *testing.T) {
	var config map[string]any
	if err := json.Unmarshal(defaultBiomeConfigBytes(), &config); err != nil {
		t.Fatalf("default Biome config must be valid JSON: %v", err)
	}

	assertNestedValue(t, config, true, "formatter", "enabled")
	assertNestedValue(t, config, "tab", "formatter", "indentStyle")
	assertNestedValue(t, config, float64(100), "formatter", "lineWidth")
	assertNestedValue(t, config, true, "organizeImports", "enabled")
	assertNestedValue(t, config, true, "linter", "rules", "recommended")
	assertNestedValue(t, config, "warn", "linter", "rules", "correctness", "noUnusedVariables")
	assertNestedValue(t, config, "warn", "linter", "rules", "correctness", "noUnusedImports")
	assertNestedValue(t, config, "error", "linter", "rules", "style", "useImportType")
	assertNestedValue(t, config, "double", "javascript", "formatter", "quoteStyle")
	assertNestedValue(t, config, "all", "javascript", "formatter", "trailingCommas")
	assertNestedValue(t, config, "always", "javascript", "formatter", "semicolons")
	assertNestedValue(t, config, "always", "javascript", "formatter", "arrowParentheses")
	assertNestedValue(t, config, true, "javascript", "formatter", "bracketSpacing")
}

func TestCLIFormatArgsAndBiomeBinPriority(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	capture := filepath.Join(root, "capture.json")
	localBiome := filepath.Join(root, "node_modules", ".bin", "biome")
	pathBiome := filepath.Join(t.TempDir(), "biome")

	writeBiomeCaptureBackend(t, localBiome, "LOCAL_BACKEND")
	writeBiomeCaptureBackend(t, pathBiome, "PATH_BACKEND")

	output := runTSPackWithBiomeCapture(t, repo, root, []string{"format", "src", "tests", "--root", root}, capture, filepath.Dir(pathBiome))
	if !strings.Contains(output, "LOCAL_BACKEND") {
		t.Fatalf("expected local .bin backend: %s", output)
	}
	if strings.Contains(output, "PATH_BACKEND") {
		t.Fatalf("path backend should not run when .bin exists: %s", output)
	}
	got := readCapturedBiomeArgv(t, capture)
	assertBiomeArgsInclude(t, got, "format", "--write", "src", "tests")
	assertBiomeArgsOmit(t, got, "--check")

	output = runTSPackWithBiomeCapture(t, repo, root, []string{"format", "src", "--check", "--root", root}, capture, filepath.Dir(pathBiome))
	if !strings.Contains(output, "LOCAL_BACKEND") {
		t.Fatalf("expected local .bin backend for check: %s", output)
	}
	got = readCapturedBiomeArgv(t, capture)
	assertBiomeArgsInclude(t, got, "format", "src")
	assertBiomeArgsOmit(t, got, "--check", "--write")
}

func TestCLIBiomeDirectPackageBackendFallback(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	capture := filepath.Join(root, "capture.json")
	directBiome := filepath.Join(root, "node_modules", "@biomejs", "biome", "bin", "biome")
	pathBiome := filepath.Join(t.TempDir(), "biome")

	writeBiomeCaptureBackend(t, directBiome, "DIRECT_BACKEND")
	writeBiomeCaptureBackend(t, pathBiome, "PATH_BACKEND")

	output := runTSPackWithBiomeCapture(t, repo, root, []string{"format", "src", "--check", "--root", root}, capture, filepath.Dir(pathBiome))
	if !strings.Contains(output, "DIRECT_BACKEND") {
		t.Fatalf("expected direct package backend: %s", output)
	}
	if strings.Contains(output, "PATH_BACKEND") {
		t.Fatalf("path backend should not run when direct package backend exists: %s", output)
	}
	got := readCapturedBiomeArgv(t, capture)
	assertBiomeArgsInclude(t, got, "format", "src")
	assertBiomeArgsOmit(t, got, "--check", "--write")
}

func TestCLIBiomePathBackendFallback(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	capture := filepath.Join(root, "capture.json")
	pathBiome := filepath.Join(t.TempDir(), "biome")

	writeBiomeCaptureBackend(t, pathBiome, "PATH_BACKEND")

	output := runTSPackWithBiomeCapture(t, repo, root, []string{"format", "src", "--root", root}, capture, filepath.Dir(pathBiome))
	if !strings.Contains(output, "PATH_BACKEND") {
		t.Fatalf("expected PATH backend: %s", output)
	}
	got := readCapturedBiomeArgv(t, capture)
	assertBiomeArgsInclude(t, got, "format", "--write", "src")
	assertBiomeArgsOmit(t, got, "--check")
}

func TestCLIBiomeDefaultConfigSignalingAndCleanup(t *testing.T) {
	repo := filepath.Join("..", "..")

	cases := []struct {
		name string
		args []string
	}{
		{name: "format check", args: []string{"format", "src", "--check"}},
		{name: "lint", args: []string{"lint", "src"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			capture := filepath.Join(root, "capture.json")
			localBiome := filepath.Join(root, "node_modules", ".bin", "biome")
			writeBiomeConfigCaptureBackend(t, localBiome, capture, "BIOME_STDOUT", "BIOME_STDERR")

			stdout, stderr, err := runTSPackForBiomeSplit(t, repo, root, append(tc.args, "--root", root), "")
			if err != nil {
				t.Fatalf("expected command to succeed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
			}
			if !strings.Contains(stderr, defaultBiomeConfigStatusLine) {
				t.Fatalf("expected default config message on stderr:\n%s", stderr)
			}
			if strings.Contains(stdout, defaultBiomeConfigStatusLine) {
				t.Fatalf("default config message must not be on stdout:\n%s", stdout)
			}

			captured := readCapturedBiomeInvocation(t, capture)
			if captured.ConfigPath == "" {
				t.Fatalf("expected --config-path for default config: %#v", captured)
			}
			if _, err := os.Stat(captured.ConfigPath); !os.IsNotExist(err) {
				t.Fatalf("expected temp config to be removed after command, stat err: %v", err)
			}

			var config map[string]any
			if err := json.Unmarshal([]byte(captured.ConfigJSON), &config); err != nil {
				t.Fatalf("captured config must be valid JSON: %v\n%s", err, captured.ConfigJSON)
			}
			assertNestedValue(t, config, "double", "javascript", "formatter", "quoteStyle")
			assertNestedValue(t, config, true, "organizeImports", "enabled")
		})
	}
}

func TestCLIBiomeProjectConfigSuppressesDefaultSignal(t *testing.T) {
	repo := filepath.Join("..", "..")

	cases := []struct {
		name       string
		configName string
	}{
		{name: "biome json", configName: "biome.json"},
		{name: "biome jsonc", configName: "biome.jsonc"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			capture := filepath.Join(root, "capture.json")
			localBiome := filepath.Join(root, "node_modules", ".bin", "biome")
			writeBiomeConfigCaptureBackend(t, localBiome, capture, "BIOME_STDOUT", "")
			if err := os.WriteFile(filepath.Join(root, tc.configName), []byte("{}\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			stdout, stderr, err := runTSPackForBiomeSplit(t, repo, root, []string{"format", "src", "--check", "--root", root}, "")
			if err != nil {
				t.Fatalf("expected command to succeed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
			}
			if strings.Contains(stderr, defaultBiomeConfigStatusLine) || strings.Contains(stdout, defaultBiomeConfigStatusLine) {
				t.Fatalf("did not expect default config message with project config\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
			}

			captured := readCapturedBiomeInvocation(t, capture)
			if captured.ConfigPath != "" {
				t.Fatalf("project config should be discovered by Biome without temp --config-path: %#v", captured)
			}
		})
	}
}

func TestCLIBiomeLintUnsafeArgForwarding(t *testing.T) {
	repo := filepath.Join("..", "..")

	cases := []struct {
		name            string
		args            []string
		wantBackendArgv []string
		wantOmittedArgv []string
	}{
		{
			name:            "lint fix unsafe default path",
			args:            []string{"lint", "--fix", "--unsafe"},
			wantBackendArgv: []string{"lint", "--write", "--unsafe", "."},
		},
		{
			name:            "lint fix unsafe preserves path",
			args:            []string{"lint", "src", "--fix", "--unsafe"},
			wantBackendArgv: []string{"lint", "--write", "--unsafe", "src"},
		},
		{
			name:            "lint fix omits unsafe",
			args:            []string{"lint", "--fix"},
			wantBackendArgv: []string{"lint", "--write", "."},
			wantOmittedArgv: []string{"--unsafe"},
		},
		{
			name:            "lint check omits unsafe",
			args:            []string{"lint"},
			wantBackendArgv: []string{"lint", "."},
			wantOmittedArgv: []string{"--write", "--unsafe"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			capture := filepath.Join(root, "capture.json")
			localBiome := filepath.Join(root, "node_modules", ".bin", "biome")
			writeBiomeExitBackend(t, localBiome, capture, 0, "BIOME_OK", "")

			output, err := runTSPackForBiome(t, repo, root, append(tc.args, "--root", root), "")
			if err != nil {
				t.Fatalf("expected command to succeed: %v\n%s", err, output)
			}

			got := readCapturedBiomeArgv(t, capture)
			assertBiomeArgsIncludeInOrder(t, got, tc.wantBackendArgv...)
			assertBiomeArgsOmit(t, got, tc.wantOmittedArgv...)
		})
	}
}

func TestCLIBiomeFormatAndLintFailureDiagnostics(t *testing.T) {
	repo := filepath.Join("..", "..")

	cases := []struct {
		name            string
		args            []string
		backendExitCode int
		wantCode        string
		wantText        []string
		wantOmittedCode string
		wantBackendArgv []string
		wantOmittedArgv []string
	}{
		{
			name:            "format check",
			args:            []string{"format", "src", "--check"},
			backendExitCode: 1,
			wantCode:        "TSPACK_FORMAT_CHECK_FAILED",
			wantText: []string{
				"format check failed",
				"Biome format found files that would change.",
				"Run `tspack format` to apply formatting.",
			},
			wantOmittedCode: "TSPACK_BIOME_COMMAND_FAILED",
			wantBackendArgv: []string{"format", "src"},
			wantOmittedArgv: []string{"--write", "--check"},
		},
		{
			name:            "format write",
			args:            []string{"format", "src"},
			backendExitCode: 1,
			wantCode:        "TSPACK_FORMAT_WRITE_FAILED",
			wantText: []string{
				"format failed",
				"Biome format exited with code 1 while applying formatting.",
			},
			wantBackendArgv: []string{"format", "--write", "src"},
		},
		{
			name:            "lint check",
			args:            []string{"lint", "src"},
			backendExitCode: 1,
			wantCode:        "TSPACK_LINT_CHECK_FAILED",
			wantText: []string{
				"lint check failed",
				"Biome reported lint violations.",
				"Run `tspack lint --fix` to apply safe fixes where possible.",
			},
			wantBackendArgv: []string{"lint", "src"},
			wantOmittedArgv: []string{"--write"},
		},
		{
			name:            "lint fix incomplete",
			args:            []string{"lint", "src", "--fix"},
			backendExitCode: 1,
			wantCode:        "TSPACK_LINT_FIX_INCOMPLETE",
			wantText: []string{
				"lint fix incomplete",
				"Biome may have applied safe fixes, but violations remain.",
				"Review the remaining diagnostics.",
				"Unsafe fixes are not applied by default.",
			},
			wantBackendArgv: []string{"lint", "--write", "src"},
		},
		{
			name:            "lint unsafe fix incomplete",
			args:            []string{"lint", "src", "--fix", "--unsafe"},
			backendExitCode: 1,
			wantCode:        "TSPACK_LINT_FIX_INCOMPLETE",
			wantText: []string{
				"lint fix incomplete",
				"Biome may have applied safe and unsafe fixes, but violations remain.",
				"Unsafe fixes were enabled for this run.",
				"Review the remaining diagnostics.",
			},
			wantOmittedCode: "TSPACK_BIOME_COMMAND_FAILED",
			wantBackendArgv: []string{"lint", "--write", "--unsafe", "src"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			capture := filepath.Join(root, "capture.json")
			localBiome := filepath.Join(root, "node_modules", ".bin", "biome")
			writeBiomeExitBackend(t, localBiome, capture, tc.backendExitCode, "BIOME_STDOUT", "BIOME_STDERR")

			output, err := runTSPackForBiome(t, repo, root, append(tc.args, "--root", root), "")
			if err == nil {
				t.Fatalf("expected command to fail, output:\n%s", output)
			}
			if !strings.Contains(output, tc.wantCode) {
				t.Fatalf("expected diagnostic %s in output:\n%s", tc.wantCode, output)
			}
			if tc.wantOmittedCode != "" && strings.Contains(output, tc.wantOmittedCode) {
				t.Fatalf("did not expect diagnostic %s in output:\n%s", tc.wantOmittedCode, output)
			}
			for _, want := range tc.wantText {
				if !strings.Contains(output, want) {
					t.Fatalf("expected %q in output:\n%s", want, output)
				}
			}
			if !strings.Contains(output, "BIOME_STDOUT") {
				t.Fatalf("expected Biome stdout to be preserved:\n%s", output)
			}
			if !strings.Contains(output, "BIOME_STDERR") {
				t.Fatalf("expected Biome stderr to be preserved:\n%s", output)
			}

			got := readCapturedBiomeArgv(t, capture)
			assertBiomeArgsInclude(t, got, tc.wantBackendArgv...)
			assertBiomeArgsOmit(t, got, tc.wantOmittedArgv...)
		})
	}
}

func TestCLIBiomeSuccessPathsDoNotEmitFailureDiagnostics(t *testing.T) {
	repo := filepath.Join("..", "..")

	cases := []struct {
		name string
		args []string
	}{
		{name: "format check", args: []string{"format", "src", "--check"}},
		{name: "lint check", args: []string{"lint", "src"}},
		{name: "lint fix", args: []string{"lint", "src", "--fix"}},
		{name: "lint unsafe fix", args: []string{"lint", "src", "--fix", "--unsafe"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			capture := filepath.Join(root, "capture.json")
			localBiome := filepath.Join(root, "node_modules", ".bin", "biome")
			writeBiomeExitBackend(t, localBiome, capture, 0, "BIOME_OK", "")

			output, err := runTSPackForBiome(t, repo, root, append(tc.args, "--root", root), "")
			if err != nil {
				t.Fatalf("expected command to succeed: %v\n%s", err, output)
			}
			failureCodes := []string{
				"TSPACK_FORMAT_CHECK_FAILED",
				"TSPACK_FORMAT_WRITE_FAILED",
				"TSPACK_LINT_CHECK_FAILED",
				"TSPACK_LINT_FIX_INCOMPLETE",
				"TSPACK_BIOME_COMMAND_FAILED",
			}
			for _, code := range failureCodes {
				if strings.Contains(output, code) {
					t.Fatalf("did not expect failure diagnostic %s in output:\n%s", code, output)
				}
			}
			if !strings.Contains(output, "BIOME_OK") {
				t.Fatalf("expected Biome stdout to be preserved:\n%s", output)
			}
		})
	}
}

func TestCLIBiomeBackendSignalStaysGeneric(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("signal termination test uses a POSIX shell")
	}

	repo := filepath.Join("..", "..")
	root := t.TempDir()
	localBiome := filepath.Join(root, "node_modules", ".bin", "biome")
	if err := os.MkdirAll(filepath.Dir(localBiome), 0o755); err != nil {
		t.Fatal(err)
	}
	backend := "#!/bin/sh\nkill -TERM $$\n"
	if err := os.WriteFile(localBiome, []byte(backend), 0o755); err != nil {
		t.Fatal(err)
	}

	output, err := runTSPackForBiome(t, repo, root, []string{"lint", "--root", root}, "")
	if err == nil {
		t.Fatalf("expected command to fail, output:\n%s", output)
	}
	if !strings.Contains(output, "TSPACK_BIOME_COMMAND_FAILED") {
		t.Fatalf("expected generic backend failure for signal termination:\n%s", output)
	}
	if strings.Contains(output, "TSPACK_LINT_CHECK_FAILED") {
		t.Fatalf("signal termination should not be reported as lint findings:\n%s", output)
	}
}

func TestCLIBiomeBackendStartFailureStaysGeneric(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	localBiome := filepath.Join(root, "node_modules", ".bin", "biome")
	if err := os.MkdirAll(filepath.Dir(localBiome), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localBiome, []byte("not executable\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := runTSPackForBiome(t, repo, root, []string{"format", "--check", "--root", root}, "")
	if err == nil {
		t.Fatalf("expected command to fail, output:\n%s", output)
	}
	if !strings.Contains(output, "TSPACK_BIOME_COMMAND_FAILED") {
		t.Fatalf("expected generic backend failure for start failure:\n%s", output)
	}
	if strings.Contains(output, "TSPACK_FORMAT_CHECK_FAILED") {
		t.Fatalf("start failure should not be reported as format check failure:\n%s", output)
	}
}

type capturedBiomeInvocation struct {
	Argv       []string `json:"argv"`
	Cwd        string   `json:"cwd"`
	ConfigPath string   `json:"configPath"`
	ConfigJSON string   `json:"configJSON"`
}

func writeValidCheckFrontendStub(t *testing.T, repo string) {
	t.Helper()
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	if err := os.MkdirAll(frontend, 0o755); err != nil {
		t.Fatal(err)
	}
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const out = {
  ok: true,
  ir: {
    format: 1,
    workspace: { name: "ws" },
    packages: [
      {
        name: "app",
        version: "1.0.0",
        kind: "library",
        dependencies: [],
        targets: [
          {
            name: "core",
            export: ".",
            entry: "src/index.ts",
            runtime: "dist/index.js",
            types: "dist/index.d.ts",
            deps: [],
            peers: []
          }
        ],
        tools: [],
        boundaries: [],
        publish: { include: ["dist/**"], exclude: [] },
        policies: { types: {}, boundaries: {} }
      }
    ]
  },
  diagnostics: []
};
process.stdout.write(JSON.stringify(out));`
	if err := os.WriteFile(cliPath, []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(cliPath) })
}

func writeValidCheckProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"src", "dist"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"manifest.tsx":    "export default {}\n",
		"src/index.ts":    "export const value = 1;\n",
		"dist/index.js":   "export const value = 1;\n",
		"dist/index.d.ts": "export declare const value: number;\n",
		"ts-lock.toml":    "[lock]\nformat = 1\ntool = \"tspack\"\n\n[[target]]\npackage = \"app\"\nname = \"core\"\nexport = \".\"\nentry = \"src/index.ts\"\nruntime = \"dist/index.js\"\ntypes = \"dist/index.d.ts\"\n",
	}
	for path, contents := range files {
		if err := os.WriteFile(filepath.Join(root, path), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func writeBiomeConfigCaptureBackend(t *testing.T, path string, capture string, stdoutText string, stderrText string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	backend := fmt.Sprintf(`#!/usr/bin/env node
const fs = require('fs');
const argv = process.argv.slice(2);
const configFlagIndex = argv.indexOf('--config-path');
let configPath = '';
let configJSON = '';
if (configFlagIndex >= 0) {
  configPath = argv[configFlagIndex + 1] || '';
  configJSON = fs.readFileSync(configPath, 'utf8');
  JSON.parse(configJSON);
}
fs.writeFileSync(%q, JSON.stringify({ argv, cwd: process.cwd(), configPath, configJSON }));
if (%q) {
  process.stdout.write(%q + '\n');
}
if (%q) {
  process.stderr.write(%q + '\n');
}
`, capture, stdoutText, stdoutText, stderrText, stderrText)
	if err := os.WriteFile(path, []byte(backend), 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeBiomeCaptureBackend(t *testing.T, path string, marker string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	backend := fmt.Sprintf(`#!/usr/bin/env node
const fs = require('fs');
const capture = process.env.TSPACK_CAPTURE;
fs.writeFileSync(capture, JSON.stringify({ argv: process.argv.slice(2), cwd: process.cwd() }));
process.stdout.write(%q + '\n');
`, marker)
	if err := os.WriteFile(path, []byte(backend), 0o755); err != nil {
		t.Fatal(err)
	}
}

func runTSPackWithBiomeCapture(t *testing.T, repo string, root string, args []string, capture string, pathDir string) string {
	t.Helper()
	_ = os.Remove(capture)
	cmd := exec.Command("go", append([]string{"run", "./cmd/tspack"}, args...)...)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+pathDir+":"+os.Getenv("PATH"), "TSPACK_CAPTURE="+capture)
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tspack failed: %v\n%s", err, string(b))
	}
	return string(b)
}

func writeBiomeExitBackend(t *testing.T, path string, capture string, exitCode int, stdoutText string, stderrText string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	backend := fmt.Sprintf(`#!/usr/bin/env node
const fs = require('fs');
fs.writeFileSync(%q, JSON.stringify({ argv: process.argv.slice(2), cwd: process.cwd() }));
if (%q) {
  process.stdout.write(%q + '\n');
}
if (%q) {
  process.stderr.write(%q + '\n');
}
process.exit(%d);
`, capture, stdoutText, stdoutText, stderrText, stderrText, exitCode)
	if err := os.WriteFile(path, []byte(backend), 0o755); err != nil {
		t.Fatal(err)
	}
}

func runTSPackForBiome(t *testing.T, repo string, root string, args []string, pathDir string) (string, error) {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", "./cmd/tspack"}, args...)...)
	cmd.Dir = repo
	pathValue := os.Getenv("PATH")
	if pathDir != "" {
		pathValue = pathDir + string(os.PathListSeparator) + pathValue
	}
	cmd.Env = append(os.Environ(), "PATH="+pathValue)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func runTSPackForBiomeSplit(t *testing.T, repo string, root string, args []string, pathDir string) (string, string, error) {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", "./cmd/tspack"}, args...)...)
	cmd.Dir = repo
	pathValue := os.Getenv("PATH")
	if pathDir != "" {
		pathValue = pathDir + string(os.PathListSeparator) + pathValue
	}
	cmd.Env = append(os.Environ(), "PATH="+pathValue)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func runTSPackBinarySplit(t *testing.T, repo string, binPath string, args []string, pathDir string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Dir = repo
	pathValue := os.Getenv("PATH")
	if pathDir != "" {
		pathValue = pathDir + string(os.PathListSeparator) + pathValue
	}
	cmd.Env = append(os.Environ(), "PATH="+pathValue)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func runTSPackBinarySplitWithExactPath(t *testing.T, repo string, binPath string, args []string, pathValue string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+pathValue)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func pathWithNodeOnly(t *testing.T) string {
	t.Helper()
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("node is required for check command tests: %v", err)
	}
	pathDir := t.TempDir()
	linkPath := filepath.Join(pathDir, "node")
	if runtime.GOOS == "windows" {
		linkPath = filepath.Join(pathDir, "node.exe")
	}
	if err := os.Symlink(nodePath, linkPath); err != nil {
		t.Fatal(err)
	}
	return pathDir
}

func readCapturedBiomeArgv(t *testing.T, capture string) []string {
	t.Helper()
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
	return got.Argv
}

func readCapturedBiomeInvocation(t *testing.T, capture string) capturedBiomeInvocation {
	t.Helper()
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var got capturedBiomeInvocation
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func assertBiomeArgsInclude(t *testing.T, got []string, want ...string) {
	t.Helper()
	joined := strings.Join(got, " ")
	for _, arg := range want {
		if !containsExactArg(got, arg) {
			t.Fatalf("expected argv to include %q in %s", arg, joined)
		}
	}
}

func assertBiomeArgsIncludeInOrder(t *testing.T, got []string, want ...string) {
	t.Helper()
	joined := strings.Join(got, " ")
	nextIndex := 0
	for _, arg := range got {
		if nextIndex >= len(want) {
			break
		}
		if arg == want[nextIndex] {
			nextIndex++
		}
	}
	if nextIndex != len(want) {
		t.Fatalf("expected argv to include %q in order in %s", strings.Join(want, " "), joined)
	}
}

func assertBiomeArgsOmit(t *testing.T, got []string, unwanted ...string) {
	t.Helper()
	joined := strings.Join(got, " ")
	for _, arg := range unwanted {
		if containsExactArg(got, arg) {
			t.Fatalf("expected argv to omit %q in %s", arg, joined)
		}
	}
}

func assertNestedValue(t *testing.T, root map[string]any, want any, path ...string) {
	t.Helper()
	var current any = root
	for _, key := range path {
		currentMap, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("expected object at %s, got %#v", strings.Join(path, "."), current)
		}
		value, ok := currentMap[key]
		if !ok {
			t.Fatalf("missing key %s", strings.Join(path, "."))
		}
		current = value
	}
	if !reflect.DeepEqual(current, want) {
		t.Fatalf("expected %s to be %#v, got %#v", strings.Join(path, "."), want, current)
	}
}

func containsExactArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func TestCLICheckFormatFlagParsingAndRoot(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeValidCheckFrontendStub(t, repo)

	plainRoot := writeValidCheckProject(t)
	plainCmd := exec.Command("go", "run", "./cmd/tspack", "check", "--root", plainRoot)
	plainCmd.Dir = repo
	plainOutput, plainErr := plainCmd.CombinedOutput()
	if plainErr != nil {
		t.Fatalf("plain check should not require or invoke Biome: %v\n%s", plainErr, string(plainOutput))
	}

	root := writeValidCheckProject(t)
	capture := filepath.Join(root, "capture.json")
	localBiome := filepath.Join(root, "node_modules", ".bin", "biome")
	writeBiomeExitBackend(t, localBiome, capture, 0, "", "")

	output, err := runTSPackForBiome(t, repo, root, []string{"check", "--format", "--root", root}, "")
	if err != nil {
		t.Fatalf("check --format should succeed: %v\n%s", err, output)
	}
	got := readCapturedBiomeArgv(t, capture)
	assertBiomeArgsInclude(t, got, "format", ".")
	assertBiomeArgsOmit(t, got, "--write", "--check")

	captured := readCapturedBiomeInvocation(t, capture)
	if captured.Cwd != root {
		t.Fatalf("check --format should run Biome in --root directory, got %q want %q", captured.Cwd, root)
	}
}

func TestCLICheckFormatPreservesManifestAndReportsTextFailure(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeValidCheckFrontendStub(t, repo)
	root := writeValidCheckProject(t)
	explicitManifest := filepath.Join(root, "custom-manifest.tsx")
	if err := os.Rename(filepath.Join(root, "manifest.tsx"), explicitManifest); err != nil {
		t.Fatal(err)
	}

	capture := filepath.Join(root, "capture.json")
	localBiome := filepath.Join(root, "node_modules", ".bin", "biome")
	writeBiomeExitBackend(t, localBiome, capture, 1, "BIOME_STDOUT", "BIOME_STDERR")

	stdout, stderr, err := runTSPackForBiomeSplit(t, repo, root, []string{"check", "--format", "--manifest", explicitManifest, "--root", root}, "")
	if err == nil {
		t.Fatalf("check --format should fail when Biome exits nonzero")
	}
	combined := stdout + stderr
	for _, want := range []string{"BIOME_STDOUT", "BIOME_STDERR", "TSPACK_FORMAT_CHECK_FAILED", "Run `tspack format`"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("check --format text output missing %q:\nstdout=%s\nstderr=%s", want, stdout, stderr)
		}
	}

	got := readCapturedBiomeArgv(t, capture)
	assertBiomeArgsInclude(t, got, "format", ".")
	assertBiomeArgsOmit(t, got, "--write", "--check")
}

func TestCLICheckFormatSuccessNoFailureDiagnostic(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeValidCheckFrontendStub(t, repo)
	root := writeValidCheckProject(t)
	capture := filepath.Join(root, "capture.json")
	localBiome := filepath.Join(root, "node_modules", ".bin", "biome")
	writeBiomeExitBackend(t, localBiome, capture, 0, "BIOME_OK", "")

	output, err := runTSPackForBiome(t, repo, root, []string{"check", "--format", "--root", root}, "")
	if err != nil {
		t.Fatalf("check --format should succeed when check and format succeed: %v\n%s", err, output)
	}
	if strings.Contains(output, "TSPACK_FORMAT_CHECK_FAILED") {
		t.Fatalf("successful check --format emitted failure diagnostic:\n%s", output)
	}
	if !strings.Contains(output, "BIOME_OK") {
		t.Fatalf("text check --format should preserve Biome output:\n%s", output)
	}
}

func TestCLICheckFormatBackendAndConfigBehavior(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeValidCheckFrontendStub(t, repo)

	missingRoot := writeValidCheckProject(t)
	missingBinPath := buildTspackBinary(t, repo)
	missingStdout, missingStderr, missingErr := runTSPackBinarySplitWithExactPath(
		t,
		repo,
		missingBinPath,
		[]string{"check", "--format", "--root", missingRoot},
		pathWithNodeOnly(t),
	)
	missingOutput := missingStdout + missingStderr
	if missingErr == nil || !strings.Contains(missingOutput, "TSPACK_BIOME_BACKEND_NOT_FOUND") {
		t.Fatalf("missing backend should fail with diagnostic: %v\nstdout=%s\nstderr=%s", missingErr, missingStdout, missingStderr)
	}

	defaultRoot := writeValidCheckProject(t)
	defaultCapture := filepath.Join(defaultRoot, "capture.json")
	writeBiomeConfigCaptureBackend(t, filepath.Join(defaultRoot, "node_modules", ".bin", "biome"), defaultCapture, "", "")
	_, defaultStderr, defaultErr := runTSPackForBiomeSplit(t, repo, defaultRoot, []string{"check", "--format", "--root", defaultRoot}, "")
	if defaultErr != nil {
		t.Fatalf("check --format with default config should succeed: %v\n%s", defaultErr, defaultStderr)
	}
	if !strings.Contains(defaultStderr, defaultBiomeConfigStatusLine) {
		t.Fatalf("expected default config signal on stderr:\n%s", defaultStderr)
	}
	defaultInvocation := readCapturedBiomeInvocation(t, defaultCapture)
	if defaultInvocation.ConfigPath == "" {
		t.Fatalf("expected temp default config path: %#v", defaultInvocation)
	}

	projectRoot := writeValidCheckProject(t)
	projectCapture := filepath.Join(projectRoot, "capture.json")
	if err := os.WriteFile(filepath.Join(projectRoot, "biome.jsonc"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeBiomeConfigCaptureBackend(t, filepath.Join(projectRoot, "node_modules", ".bin", "biome"), projectCapture, "", "")
	projectStdout, projectStderr, projectErr := runTSPackForBiomeSplit(t, repo, projectRoot, []string{"check", "--format", "--root", projectRoot}, "")
	if projectErr != nil {
		t.Fatalf("check --format with project config should succeed: %v\nstdout=%s\nstderr=%s", projectErr, projectStdout, projectStderr)
	}
	if strings.Contains(projectStderr, defaultBiomeConfigStatusLine) || strings.Contains(projectStdout, defaultBiomeConfigStatusLine) {
		t.Fatalf("project config should suppress default message:\nstdout=%s\nstderr=%s", projectStdout, projectStderr)
	}
	projectInvocation := readCapturedBiomeInvocation(t, projectCapture)
	if projectInvocation.ConfigPath != "" {
		t.Fatalf("project config should not pass temp --config-path: %#v", projectInvocation)
	}
}

func TestCLICheckFormatJSONSuccessAndFailureAreClean(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeValidCheckFrontendStub(t, repo)

	binPath := buildTspackBinary(t, repo)

	successRoot := writeValidCheckProject(t)
	successCapture := filepath.Join(successRoot, "capture.json")
	writeBiomeExitBackend(t, filepath.Join(successRoot, "node_modules", ".bin", "biome"), successCapture, 0, "BIOME_STDOUT", "BIOME_STDERR")
	successStdout, successStderr, successErr := runTSPackBinarySplit(t, repo, binPath, []string{"check", "--format", "--json", "--root", successRoot}, "")
	if successErr != nil {
		t.Fatalf("json check --format success should exit zero: %v\nstdout=%s\nstderr=%s", successErr, successStdout, successStderr)
	}
	if successStderr != "" {
		t.Fatalf("json check --format success should keep stderr clean, got %q", successStderr)
	}
	if strings.Contains(successStdout, "BIOME_STDOUT") || strings.Contains(successStdout, "BIOME_STDERR") {
		t.Fatalf("json success should not mix captured Biome human output into stdout:\n%s", successStdout)
	}
	var successReport checkJSONReport
	if err := json.Unmarshal([]byte(successStdout), &successReport); err != nil {
		t.Fatalf("success stdout should be JSON: %v\n%s", err, successStdout)
	}
	if !successReport.OK {
		t.Fatalf("expected ok=true: %+v", successReport)
	}

	failureRoot := writeValidCheckProject(t)
	failureCapture := filepath.Join(failureRoot, "capture.json")
	writeBiomeExitBackend(t, filepath.Join(failureRoot, "node_modules", ".bin", "biome"), failureCapture, 1, "BIOME_STDOUT", "BIOME_STDERR")
	failureStdout, failureStderr, failureErr := runTSPackBinarySplit(t, repo, binPath, []string{"check", "--format", "--json", "--root", failureRoot}, "")
	if failureErr == nil {
		t.Fatalf("json check --format failure should exit nonzero")
	}
	if failureStderr != "" {
		t.Fatalf("json check --format failure should keep stderr clean, got %q", failureStderr)
	}
	var failureReport checkJSONReport
	if err := json.Unmarshal([]byte(failureStdout), &failureReport); err != nil {
		t.Fatalf("failure stdout should be JSON: %v\n%s", err, failureStdout)
	}
	if failureReport.OK {
		t.Fatalf("expected ok=false: %+v", failureReport)
	}
	formatDiagnosticFound := false
	for _, diagnostic := range failureReport.Diagnostics {
		if diagnostic.Code == "TSPACK_FORMAT_CHECK_FAILED" {
			formatDiagnosticFound = true
			details := strings.Join(diagnostic.Details, "\n")
			if !strings.Contains(details, "Run `tspack format`") || !strings.Contains(details, "BIOME_STDOUT") || !strings.Contains(details, "BIOME_STDERR") {
				t.Fatalf("format diagnostic details should include guidance and captured output: %+v", diagnostic)
			}
		}
	}
	if !formatDiagnosticFound {
		t.Fatalf("expected TSPACK_FORMAT_CHECK_FAILED diagnostic: %+v", failureReport.Diagnostics)
	}
}

func TestCLICheckFormatJSONMissingBackendIsStructured(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeValidCheckFrontendStub(t, repo)
	root := writeValidCheckProject(t)
	binPath := buildTspackBinary(t, repo)
	stdout, stderr, err := runTSPackBinarySplitWithExactPath(
		t,
		repo,
		binPath,
		[]string{"check", "--format", "--json", "--root", root},
		pathWithNodeOnly(t),
	)
	if err == nil {
		t.Fatalf("missing backend should fail")
	}
	if stderr != "" {
		t.Fatalf("json missing backend should keep stderr clean, got %q", stderr)
	}
	var report checkJSONReport
	if jsonErr := json.Unmarshal([]byte(stdout), &report); jsonErr != nil {
		t.Fatalf("missing backend stdout should be JSON: %v\n%s", jsonErr, stdout)
	}
	if report.OK {
		t.Fatalf("missing backend should report ok=false: %+v", report)
	}
	found := false
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Code == "TSPACK_BIOME_BACKEND_NOT_FOUND" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected structured missing backend diagnostic: %+v", report.Diagnostics)
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

	cmd = exec.Command("go", "run", "./cmd/tspack", "lint", "--unsafe", "--root", root)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_LINT_INVALID_FLAGS") || !strings.Contains(string(b), "--unsafe requires --fix") {
		t.Fatalf("lint unsafe invalid flags missing: %v\n%s", err, string(b))
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "format", "--unsafe", "--root", root)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_FORMAT_INVALID_FLAGS") {
		t.Fatalf("format unsafe invalid flags missing: %v\n%s", err, string(b))
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "format", "--check", "--unsafe", "--root", root)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_FORMAT_INVALID_FLAGS") {
		t.Fatalf("format check unsafe invalid flags missing: %v\n%s", err, string(b))
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

	cmd = exec.Command("go", "run", "./cmd/tspack", "how", "TSPACK_PACK_CHANGELOG_NOT_INCLUDED")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("how changelog code failed: %v\n%s", err, string(b))
	}
	text = string(b)
	if !strings.Contains(text, "CHANGELOG.md") || !strings.Contains(text, "Publish include") {
		t.Fatalf("expected changelog publish guidance: %s", text)
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

func TestCheckJSONBoundaryDiagnosticsAndTextOutput(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	if err := os.MkdirAll(frontend, 0o755); err != nil {
		t.Fatal(err)
	}
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const manifestIR = {
  format: 1,
  workspace: { name: "ws" },
  packages: [
    {
      name: "app",
      version: "1.0.0",
      kind: "library",
      dependencies: [
        {
          key: "react-dom",
          kind: "runtime",
          source: { kind: "npm", package: "react-dom", range: "^19.0.0" }
        },
        {
          key: "vite",
          kind: "tool",
          source: { kind: "npm", package: "vite", range: "^6.0.0" }
        }
      ],
      targets: [
        {
          name: "core",
          export: ".",
          entry: "src/index.ts",
          runtime: "dist/index.js",
          types: "dist/index.d.ts",
          deps: ["react-dom"],
          peers: []
        }
      ],
      tools: ["vite"],
      boundaries: [
        {
          transitiveFrom: "src/index.ts",
          denyDeps: ["react-dom"]
        }
      ],
      publish: { include: ["dist/**"], exclude: [] },
      policies: { types: {}, boundaries: {} }
    }
  ]
};

const out = { ok: true, ir: manifestIR, diagnostics: [] };
process.stdout.write(JSON.stringify(out));`
	if err := os.WriteFile(cliPath, []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(cliPath) })

	binPath := buildTspackBinary(t, repo)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	indexSource := `import "react-dom";
import "vite";
`
	if err := os.WriteFile(filepath.Join(root, "src", "index.ts"), []byte(indexSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dist", "index.js"), []byte("export const x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dist", "index.d.ts"), []byte("export declare const x: number\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	jsonCmd := exec.Command(binPath, "check", "--root", root, "--json")
	jsonCmd.Dir = repo
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	jsonCmd.Stdout = stdout
	jsonCmd.Stderr = stderr
	err := jsonCmd.Run()
	if err == nil {
		t.Fatalf("expected boundary violations to exit nonzero")
	}
	if strings.Contains(stdout.String(), "TSPACK_BOUNDARY_EXPLICIT_DENY:") || strings.Contains(stdout.String(), "TSPACK_BOUNDARY_TOOL_RUNTIME_IMPORT:") {
		t.Fatalf("expected stdout to contain JSON only, got human diagnostics: %s", stdout.String())
	}
	if strings.Contains(stderr.String(), "TSPACK_BOUNDARY_") {
		t.Fatalf("expected no duplicated human boundary diagnostics on stderr, got: %s", stderr.String())
	}

	var report checkJSONReport
	if unmarshalErr := json.Unmarshal(stdout.Bytes(), &report); unmarshalErr != nil {
		t.Fatalf("stdout is not valid json: %v\n%s", unmarshalErr, stdout.String())
	}
	if report.OK {
		t.Fatalf("expected ok=false for boundary violations: %+v", report)
	}
	if report.Summary.Errors == 0 {
		t.Fatalf("expected summary.errors > 0: %+v", report.Summary)
	}

	foundExplicitDeny := false
	foundToolRuntime := false
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Code != "TSPACK_BOUNDARY_EXPLICIT_DENY" && diagnostic.Code != "TSPACK_BOUNDARY_TOOL_RUNTIME_IMPORT" {
			continue
		}
		if diagnostic.Severity != "error" {
			t.Fatalf("expected boundary diagnostic severity to be error, got %+v", diagnostic)
		}
		detailsText := strings.Join(diagnostic.Details, "\n")
		if !strings.Contains(detailsText, "path=") || !strings.Contains(detailsText, "src/index.ts") {
			t.Fatalf("expected structured import-chain details, got %+v", diagnostic)
		}
		if diagnostic.Code == "TSPACK_BOUNDARY_EXPLICIT_DENY" {
			foundExplicitDeny = true
			if !strings.Contains(detailsText, "react-dom") {
				t.Fatalf("explicit deny details missing react-dom: %+v", diagnostic)
			}
			if !strings.Contains(detailsText, "transitiveFrom=src/index.ts") || !strings.Contains(detailsText, "seed=src/index.ts") {
				t.Fatalf("transitive explicit deny details missing scope: %+v", diagnostic)
			}
		}
		if diagnostic.Code == "TSPACK_BOUNDARY_TOOL_RUNTIME_IMPORT" {
			foundToolRuntime = true
			if !strings.Contains(detailsText, "vite") {
				t.Fatalf("tool runtime details missing vite: %+v", diagnostic)
			}
		}
	}
	if !foundExplicitDeny || !foundToolRuntime {
		t.Fatalf("expected both boundary diagnostics in one JSON report: %+v", report.Diagnostics)
	}

	textCmd := exec.Command(binPath, "check", "--root", root)
	textCmd.Dir = repo
	textOut, textErr := textCmd.CombinedOutput()
	if textErr == nil {
		t.Fatalf("expected text check to fail")
	}
	text := string(textOut)
	if !strings.Contains(text, "TSPACK_BOUNDARY_EXPLICIT_DENY: import denied by explicit transitive boundary") {
		t.Fatalf("missing explicit deny human diagnostic: %s", text)
	}
	if !strings.Contains(text, "TSPACK_BOUNDARY_TOOL_RUNTIME_IMPORT: tool dependency imported at runtime") {
		t.Fatalf("missing tool runtime human diagnostic: %s", text)
	}
	if !strings.Contains(text, "  path=") || !strings.Contains(text, "src/index.ts") {
		t.Fatalf("missing human-readable boundary detail lines: %s", text)
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

func TestCLIUpdateProgressStderrAndQuiet(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const out={ok:true,ir:{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"library",dependencies:[{key:"local-dep",kind:"dep",source:{kind:"path",path:"local-dep"}}],targets:[{name:"core",export:".",entry:"src/index.ts",runtime:"src/index.ts",types:"dist/index.d.ts",deps:["local-dep"],peers:[]}],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{types:{},boundaries:{}}}]},diagnostics:[]};
process.stdout.write(JSON.stringify(out));`
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })
	binPath := buildTspackBinary(t, repo)
	root := writeCLIPathUpdateFixture(t)

	cmd := exec.Command(binPath, "update", "--root", root)
	cmd.Dir = repo
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("update failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if stdout.String() != "lockfile diff: +1 -0\n" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	for _, want := range []string{"resolving packages...", "populating store...", "writing lockfile...", "update complete"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr progress missing %q:\n%s", want, stderr.String())
		}
	}

	quietRoot := writeCLIPathUpdateFixture(t)
	cmd = exec.Command(binPath, "update", "--root", quietRoot, "--quiet")
	cmd.Dir = repo
	stdout.Reset()
	stderr.Reset()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("quiet update failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "resolving packages") || strings.Contains(stderr.String(), "update complete") {
		t.Fatalf("--quiet should suppress progress, got stderr: %s", stderr.String())
	}
}

func TestCLIUpdateDryRunProgressStderr(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const out={ok:true,ir:{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"library",dependencies:[{key:"local-dep",kind:"dep",source:{kind:"path",path:"local-dep"}}],targets:[{name:"core",export:".",entry:"src/index.ts",runtime:"src/index.ts",types:"dist/index.d.ts",deps:["local-dep"],peers:[]}],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{types:{},boundaries:{}}}]},diagnostics:[]};
process.stdout.write(JSON.stringify(out));`
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })
	binPath := buildTspackBinary(t, repo)
	root := writeCLIPathUpdateFixture(t)

	cmd := exec.Command(binPath, "update", "--root", root, "--dry-run")
	cmd.Dir = repo
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("dry-run failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	for _, want := range []string{"resolving packages...", "computing lockfile diff...", "dry run complete"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr progress missing %q:\n%s", want, stderr.String())
		}
	}
	if strings.Contains(stdout.String(), "resolving packages") {
		t.Fatalf("progress should not be written to stdout: %s", stdout.String())
	}
}

func TestCLIUpdateTargetedProgressStderr(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const out={ok:true,ir:{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"library",dependencies:[{key:"local-dep",kind:"dep",source:{kind:"path",path:"local-dep"}}],targets:[{name:"core",export:".",entry:"src/index.ts",runtime:"src/index.ts",types:"dist/index.d.ts",deps:["local-dep"],peers:[]}],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{types:{},boundaries:{}}}]},diagnostics:[]};
process.stdout.write(JSON.stringify(out));`
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })
	binPath := buildTspackBinary(t, repo)
	root := writeCLIPathUpdateFixture(t)

	cmd := exec.Command(binPath, "update", "local-dep", "--root", root)
	cmd.Dir = repo
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatalf("path targeted update should fail before npm-only support expands")
	}
	if !strings.Contains(stderr.String(), "updating target dependency: local-dep") {
		t.Fatalf("targeted progress missing query:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "TSPACK_UPDATE_TARGET_UNSUPPORTED_SOURCE") {
		t.Fatalf("targeted diagnostic missing after progress:\n%s", stderr.String())
	}

	dryRoot := writeCLIPathUpdateFixture(t)
	cmd = exec.Command(binPath, "update", "local-dep", "--root", dryRoot, "--dry-run")
	cmd.Dir = repo
	stdout.Reset()
	stderr.Reset()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatalf("path targeted dry-run should fail before npm-only support expands")
	}
	if !strings.Contains(stderr.String(), "planning targeted update: local-dep") {
		t.Fatalf("targeted dry-run progress missing query:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "TSPACK_UPDATE_TARGET_UNSUPPORTED_SOURCE") {
		t.Fatalf("targeted dry-run diagnostic missing after progress:\n%s", stderr.String())
	}
}

func writeCLIPathUpdateFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "src"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, "dist"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, "local-dep"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "src", "index.ts"), []byte("export const x=1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "dist", "index.d.ts"), []byte("export declare const x:number\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "local-dep", "package.json"), []byte(`{"name":"local-dep","version":"1.0.0"}`), 0o644)
	return root
}

func TestCheckExplainTextJSONAndValidation(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	if err := os.MkdirAll(frontend, 0o755); err != nil {
		t.Fatal(err)
	}
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const manifestIR = {
  format: 1,
  workspace: { name: "ws" },
  packages: [
    {
      name: "app",
      version: "1.0.0",
      kind: "library",
      dependencies: [
        { key: "react", kind: "peer", source: { kind: "npm", package: "react", range: "^19.0.0" } },
        { key: "react-dom", kind: "peer", source: { kind: "npm", package: "react-dom", range: "^19.0.0" } },
        { key: "typescript", kind: "tool", source: { kind: "npm", package: "typescript", range: "^5.0.0" } }
      ],
      targets: [
        { name: "core", export: ".", entry: "src/index.ts", runtime: "dist/index.js", types: "dist/index.d.ts", deps: [], peers: ["react"] }
      ],
      tools: ["typescript"],
      boundaries: [{ from: "src/**", denyDeps: ["react-dom"] }],
      publish: { include: ["dist/**"], exclude: [] },
      policies: { types: {}, boundaries: {} }
    }
  ]
};
process.stdout.write(JSON.stringify({ ok: true, ir: manifestIR, diagnostics: [] }));`
	if err := os.WriteFile(cliPath, []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(cliPath) })

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"manifest.tsx":        "export default {};\n",
		"src/index.ts":        "import \"./button.js\";\n",
		"src/button.tsx":      "import React from \"react\";\nimport \"react-dom\";\nimport ts from \"typescript\";\nimport \"./style-helper.js\";\n",
		"src/style-helper.ts": "export const style = true;\n",
		"README.md":           "# demo\n",
	}
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	bin := buildTspackBinary(t, repo)
	cmd := exec.Command(bin, "check", "--root", root, "--explain", "src/button.tsx")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("explain failed: %v\n%s", err, string(out))
	}
	text := string(out)
	for _, want := range []string{"Reachable from targets:", "core", "src/index.ts -> src/button.tsx", "from: src/**", "denyDeps: react-dom", "react-dom", "TSPACK_BOUNDARY_EXPLICIT_DENY", "typescript", "TSPACK_BOUNDARY_TOOL_RUNTIME_IMPORT", "./style-helper.js", "resolved: src/style-helper.ts", "matches the file"} {
		if !strings.Contains(text, want) {
			t.Fatalf("explain text missing %q:\n%s", want, text)
		}
	}

	jsonCmd := exec.Command(bin, "check", "--root", root, "--explain", "src/button.tsx", "--json")
	jsonCmd.Dir = repo
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	jsonCmd.Stdout = stdout
	jsonCmd.Stderr = stderr
	if err := jsonCmd.Run(); err != nil {
		t.Fatalf("json explain failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("json explain wrote stderr: %s", stderr.String())
	}
	var report map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("json explain was not parseable: %v\n%s", err, stdout.String())
	}
	if report["mode"] != "explain" || report["command"] != "check" {
		t.Fatalf("unexpected json report: %#v", report)
	}
	if _, ok := report["reachableFrom"].([]any); !ok {
		t.Fatalf("missing reachableFrom: %#v", report)
	}
	if _, ok := report["matchedRules"].([]any); !ok {
		t.Fatalf("missing matchedRules: %#v", report)
	}
	if _, ok := report["imports"].([]any); !ok {
		t.Fatalf("missing imports: %#v", report)
	}

	missing := exec.Command(bin, "check", "--root", root, "--explain", "src/missing.ts")
	missing.Dir = repo
	missingOut, missingErr := missing.CombinedOutput()
	if missingErr == nil || !strings.Contains(string(missingOut), "TSPACK_CHECK_EXPLAIN_FILE_NOT_FOUND") {
		t.Fatalf("missing file diagnostic not surfaced: err=%v output=%s", missingErr, string(missingOut))
	}

	unsupported := exec.Command(bin, "check", "--root", root, "--explain", "README.md")
	unsupported.Dir = repo
	unsupportedOut, unsupportedErr := unsupported.CombinedOutput()
	if unsupportedErr == nil || !strings.Contains(string(unsupportedOut), "TSPACK_CHECK_EXPLAIN_UNSUPPORTED_FILE") {
		t.Fatalf("unsupported diagnostic not surfaced: err=%v output=%s", unsupportedErr, string(unsupportedOut))
	}
}

func TestCheckTypeBoundaryJSONTextAndExplain(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	if err := os.MkdirAll(frontend, 0o755); err != nil {
		t.Fatal(err)
	}
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const manifestIR = {
  format: 1,
  workspace: { name: "ws" },
  packages: [{
    name: "app",
    version: "1.0.0",
    kind: "library",
    dependencies: [
      { key: "react-dom", kind: "peer", source: { kind: "npm", package: "react-dom", range: "^19.0.0" } }
    ],
    targets: [{ name: "core", export: ".", entry: "src/index.ts", runtime: "dist/index.js", types: "dist/index.d.ts", deps: [], peers: ["react-dom"] }],
    tools: [],
    boundaries: [{ from: "src/index.ts", denyTypeDeps: ["react-dom"] }],
    publish: { include: ["dist/**"], exclude: [] },
    policies: { types: {}, boundaries: {} }
  }]
};
process.stdout.write(JSON.stringify({ ok: true, ir: manifestIR, diagnostics: [] }));`
	if err := os.WriteFile(cliPath, []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(cliPath) })

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {};\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "index.ts"), []byte("import type { Foo } from \"react-dom/client\";\nexport const ok = true;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	bin := buildTspackBinary(t, repo)
	jsonCmd := exec.Command(bin, "check", "--root", root, "--json")
	jsonCmd.Dir = repo
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	jsonCmd.Stdout = stdout
	jsonCmd.Stderr = stderr
	if err := jsonCmd.Run(); err == nil {
		t.Fatalf("expected json check to fail")
	}
	if stderr.Len() != 0 {
		t.Fatalf("json check wrote stderr: %s", stderr.String())
	}
	var report struct {
		Diagnostics []struct {
			Code    string   `json:"code"`
			Details []string `json:"details"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("json check was not parseable: %v\n%s", err, stdout.String())
	}
	found := false
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Code != "TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY" {
			continue
		}
		found = true
		if !containsString(diagnostic.Details, "package=react-dom") || !containsString(diagnostic.Details, "import=react-dom/client") {
			t.Fatalf("type diagnostic missing package/import details: %+v", diagnostic)
		}
	}
	if !found {
		t.Fatalf("expected type boundary diagnostic in json report: %+v", report.Diagnostics)
	}

	textCmd := exec.Command(bin, "check", "--root", root)
	textCmd.Dir = repo
	textOut, textErr := textCmd.CombinedOutput()
	if textErr == nil {
		t.Fatalf("expected text check to fail")
	}
	text := string(textOut)
	if !strings.Contains(text, "TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY: type import denied by explicit boundary") || !strings.Contains(text, "denyTypeDeps=react-dom") {
		t.Fatalf("missing type boundary text details: %s", text)
	}

	explainCmd := exec.Command(bin, "check", "--root", root, "--explain", "src/index.ts")
	explainCmd.Dir = repo
	explainOut, explainErr := explainCmd.CombinedOutput()
	if explainErr != nil {
		t.Fatalf("explain failed: %v\n%s", explainErr, string(explainOut))
	}
	explainText := string(explainOut)
	for _, want := range []string{"Type imports:", "react-dom/client", "TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY", "denyTypeDeps: react-dom"} {
		if !strings.Contains(explainText, want) {
			t.Fatalf("explain output missing %q:\n%s", want, explainText)
		}
	}

	explainJSONCmd := exec.Command(bin, "check", "--root", root, "--explain", "src/index.ts", "--json")
	explainJSONCmd.Dir = repo
	explainJSONOut, explainJSONErr := explainJSONCmd.CombinedOutput()
	if explainJSONErr != nil {
		t.Fatalf("json explain failed: %v\n%s", explainJSONErr, string(explainJSONOut))
	}
	if !strings.Contains(string(explainJSONOut), `"typeOnly": true`) || !strings.Contains(string(explainJSONOut), `"diagnostic": "TSPACK_BOUNDARY_TYPE_EXPLICIT_DENY"`) {
		t.Fatalf("json explain missing type import decision: %s", string(explainJSONOut))
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestCLITestXTestBridgeOverrideAndCopiedListFilter(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	recordPath := filepath.Join(root, "bridge-args.txt")
	bridge := filepath.Join(root, "native-test-cli.js")
	stub := fmt.Sprintf(`#!/usr/bin/env node
import fs from 'node:fs';
const args = process.argv.slice(2);
fs.appendFileSync(%q, args.join('\t') + '\n');
const filterIndex = args.indexOf('--filter');
const filter = filterIndex >= 0 ? args[filterIndex + 1] : '';
const id = 'src/cx.xtest.tsx::suite/fact/joins';
const caseId = 'src/cx.xtest.tsx::suite/theory/name[2]';
if (args.includes('--list')) {
  console.log('Native xTest results');
  console.log('');
  console.log('PASS ' + id);
  console.log('PASS ' + caseId);
  process.exit(0);
}
if (filter === id) {
  console.log('Native xTest results');
  console.log('');
  console.log('PASS ' + id);
  process.exit(0);
}
if (filter === '[2]') {
  console.log('Native xTest results');
  console.log('');
  console.log('PASS ' + caseId);
  process.exit(0);
}
console.error('unexpected filter: ' + filter);
process.exit(1);
`, recordPath)
	if err := os.WriteFile(bridge, []byte(stub), 0o755); err != nil {
		t.Fatalf("write bridge: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "cx.xtest.tsx"), []byte("export default null\n"), 0o644); err != nil {
		t.Fatalf("write xtest: %v", err)
	}

	cmd := exec.Command("go", "run", "./cmd/tspack", "test", "--root", root, "--list", "--xtest-bridge", bridge)
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list failed: %v\n%s", err, string(b))
	}
	text := string(b)
	if !strings.Contains(text, "PASS src/cx.xtest.tsx::suite/fact/joins") {
		t.Fatalf("missing root-relative listed ID: %s", text)
	}
	if strings.Contains(text, root) {
		t.Fatalf("list output leaked absolute root %q: %s", root, text)
	}

	listedID := "src/cx.xtest.tsx::suite/fact/joins"
	cmd = exec.Command("go", "run", "./cmd/tspack", "test", "--root", root, "--filter", listedID, "--xtest-bridge", bridge)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "PASS "+listedID) {
		t.Fatalf("copied ID filter failed: %v\n%s", err, string(b))
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "test", "--root", root, "--filter", "[2]", "--xtest-bridge", bridge)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "PASS src/cx.xtest.tsx::suite/theory/name[2]") {
		t.Fatalf("case suffix filter failed: %v\n%s", err, string(b))
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "test", "--root", root, "--filter", listedID, "--compact", "--xtest-bridge", bridge)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "PASS "+listedID) {
		t.Fatalf("compact bridge smoke failed: %v\n%s", err, string(b))
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "test", "--root", root, "--list", "--compact", "--xtest-bridge", bridge)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "PASS "+listedID) {
		t.Fatalf("compact list bridge smoke failed: %v\n%s", err, string(b))
	}

	recorded, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read recorded args: %v", err)
	}
	recordedText := string(recorded)
	if !strings.Contains(recordedText, "--root\t"+root) || !strings.Contains(recordedText, "--filter\t"+listedID) {
		t.Fatalf("bridge did not receive expected args:\n%s", recordedText)
	}
	if !strings.Contains(recordedText, "--filter\t"+listedID+"\t--compact") {
		t.Fatalf("bridge did not receive compact for run output:\n%s", recordedText)
	}
	for _, line := range strings.Split(recordedText, "\n") {
		if strings.Contains(line, "--list") && strings.Contains(line, "--compact") {
			t.Fatalf("compact should not be forwarded to list mode:\n%s", recordedText)
		}
	}
}

func TestCLITestXTestTheoryStructureSmoke(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	bridge := filepath.Join(root, "native-test-cli.js")
	stub := `#!/usr/bin/env node
const args = process.argv.slice(2);
const filterIndex = args.indexOf('--filter');
const filter = filterIndex >= 0 ? args[filterIndex + 1] : '';
if (filter === 'zero') {
  console.error('TSPACK_TEST_THEORY_NO_CASES: Theory requires at least one Case child');
  process.exit(1);
}
console.log('Native xTest results');
console.log('');
console.log('PASS src/theory.xtest.tsx::suite/callback before[0]');
console.log('PASS src/theory.xtest.tsx::suite/callback before[1]');
process.exit(0);
`
	if err := os.WriteFile(bridge, []byte(stub), 0o755); err != nil {
		t.Fatalf("write bridge: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "theory.xtest.tsx"), []byte("export default null\n"), 0o644); err != nil {
		t.Fatalf("write xtest: %v", err)
	}

	cmd := exec.Command("go", "run", "./cmd/tspack", "test", "--root", root, "--xtest-bridge", bridge)
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "PASS src/theory.xtest.tsx::suite/callback before[1]") {
		t.Fatalf("callback-before-cases bridge smoke failed: %v\n%s", err, string(b))
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "test", "--root", root, "--filter", "zero", "--xtest-bridge", bridge)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_TEST_THEORY_NO_CASES") {
		t.Fatalf("zero-case theory diagnostic smoke failed: %v\n%s", err, string(b))
	}
}

func TestCLITestXTestBridgeMissingDiagnostic(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample.xtest.tsx"), []byte("export default null\n"), 0o644); err != nil {
		t.Fatalf("write xtest: %v", err)
	}
	missing := filepath.Join(root, "missing-bridge.js")
	cmd := exec.Command("go", "run", "./cmd/tspack", "test", "--root", root, "--xtest-bridge", missing)
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected missing bridge failure")
	}
	text := string(b)
	if !strings.Contains(text, "TSPACK_TEST_XTEST_BRIDGE_MISSING") || !strings.Contains(text, missing) || !strings.Contains(text, "searched paths:") {
		t.Fatalf("missing bridge diagnostic details: %s", text)
	}
}

func TestCLITestDefaultBridgeResolutionFromUnrelatedCWD(t *testing.T) {
	repo := filepath.Join("..", "..")
	bridge := filepath.Join(repo, "manifest-frontend", "dist", "src", "native-test-cli.js")
	backup := bridge + ".m34a-bak"
	if err := os.MkdirAll(filepath.Dir(bridge), 0o755); err != nil {
		t.Fatalf("mkdir bridge dir: %v", err)
	}
	if _, err := os.Stat(bridge); err == nil {
		if renameErr := os.Rename(bridge, backup); renameErr != nil {
			t.Fatalf("backup bridge: %v", renameErr)
		}
		defer func() { _ = os.Rename(backup, bridge) }()
	} else {
		defer func() { _ = os.Remove(bridge) }()
	}
	stub := `#!/usr/bin/env node
const args = process.argv.slice(2);
if (args[0] !== 'test') process.exit(2);
console.log('Native xTest results');
console.log('');
console.log('PASS src/cwd.xtest.tsx::cwd/pass');
`
	if err := os.WriteFile(bridge, []byte(stub), 0o755); err != nil {
		t.Fatalf("write default bridge stub: %v", err)
	}

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "cwd.xtest.tsx"), []byte("export default null\n"), 0o644); err != nil {
		t.Fatalf("write xtest: %v", err)
	}

	bin := buildTspackBinary(t, repo)
	cmd := exec.Command(bin, "test", "--root", root, "--list")
	cmd.Dir = t.TempDir()
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "PASS src/cwd.xtest.tsx::cwd/pass") {
		t.Fatalf("default bridge from unrelated cwd failed: %v\n%s", err, string(b))
	}
}

func TestCLIRunStatusUsesStderrAndChildStreamsPassThrough(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	port := reservePort(t)
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	server := fmt.Sprintf(`const http=require('http');
console.log('child stdout');
console.error('child stderr');
http.createServer((_,res)=>{res.statusCode=200;res.end('ok')}).listen(%d,'127.0.0.1');
setInterval(()=>{},1000);
`, port)
	_ = os.WriteFile(filepath.Join(root, "server.js"), []byte(server), 0o644)
	writeRunFrontendStub(t, fmt.Sprintf(`{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","server.js"],url:"http://127.0.0.1:%d",ready:{kind:"http",path:"/"}}]}]}`, port))

	cmd := exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--once")
	cmd.Dir = repo
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	stdoutText := stdout.String()
	stderrText := stderr.String()
	if strings.Contains(stdoutText, "Starting run target") || strings.Contains(stdoutText, "Runtime:") || strings.Contains(stdoutText, "Ready:") {
		t.Fatalf("status leaked to stdout:\nstdout=%q\nstderr=%q", stdoutText, stderrText)
	}
	if !strings.Contains(stdoutText, "child stdout") {
		t.Fatalf("child stdout did not pass through stdout:\nstdout=%q\nstderr=%q", stdoutText, stderrText)
	}
	for _, expected := range []string{"Starting run target", "Runtime:", "Waiting for:", "Ready:", "child stderr"} {
		if !strings.Contains(stderrText, expected) {
			t.Fatalf("stderr missing %q:\nstdout=%q\nstderr=%q", expected, stdoutText, stderrText)
		}
	}
}

func TestCLIRunManifestFlag(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	defaultPort := reservePort(t)
	explicitPort := reservePort(t)
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	explicitManifest := filepath.Join(root, "package.manifest.tsx")
	_ = os.WriteFile(explicitManifest, []byte("export default {}\n"), 0o644)
	server := `const http=require('http'); const p=Number(process.argv[2]); http.createServer((_,res)=>{res.statusCode=200;res.end('ok')}).listen(p,'127.0.0.1'); setInterval(()=>{},1000);`
	_ = os.WriteFile(filepath.Join(root, "server.js"), []byte(server), 0o644)
	writeRunManifestSwitchingStub(t, repo, filepath.Base(explicitManifest), defaultPort, explicitPort)

	cmd := exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--once")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), fmt.Sprintf("Ready: http://127.0.0.1:%d", defaultPort)) {
		t.Fatalf("default manifest run failed: %v\n%s", err, string(b))
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--manifest", explicitManifest, "--once")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), fmt.Sprintf("Ready: http://127.0.0.1:%d", explicitPort)) {
		t.Fatalf("explicit manifest run failed: %v\n%s", err, string(b))
	}

	missingManifest := filepath.Join(root, "missing.manifest.tsx")
	cmd = exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--manifest", missingManifest, "--once")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED") {
		t.Fatalf("expected missing explicit manifest diagnostic: %v\n%s", err, string(b))
	}
}

func writeRunManifestSwitchingStub(t *testing.T, repo string, explicitBase string, defaultPort int, explicitPort int) {
	t.Helper()
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := fmt.Sprintf(`#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
const manifestPath = process.argv[2];
if (!fs.existsSync(manifestPath)) {
  console.error('manifest not found: ' + manifestPath);
  process.exit(1);
}
const explicit = path.basename(manifestPath) === %q;
const port = explicit ? %d : %d;
const name = explicit ? 'explicit' : 'dev';
const out = {ok:true,ir:{format:1,workspace:{name:'ws'},packages:[{name:'app',version:'1.0.0',kind:'app',dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:['dist/**'],exclude:[]},policies:{},runTargets:[{name,runtime:'system',command:['node','server.js',String(port)],url:'http://127.0.0.1:' + port,ready:{kind:'http',path:'/'}}]}]},diagnostics:[]};
process.stdout.write(JSON.stringify(out));
`, explicitBase, explicitPort, defaultPort)
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })
}

func TestCLIRunListAndPackageScoping(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	markerPath := filepath.Join(root, "started.txt")
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "marker.js"), []byte("require('fs').writeFileSync('started.txt', 'started')\n"), 0o644)
	stubIR := `{format:1,workspace:{name:"ws"},packages:[{name:"@prisma-ui/demo",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","marker.js"],url:"http://127.0.0.1:5991",ready:{kind:"http",path:"/"}},{name:"preview",runtime:"node",command:["vite","preview"],url:"http://127.0.0.1:5992",ready:{kind:"http",path:"/"}}]},{name:"@prisma-ui/docs",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","docs-server.js"],url:"http://127.0.0.1:5993",ready:{kind:"http",path:"/"}}]}]}`
	writeRunFrontendStub(t, stubIR)

	cmd := exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--list")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run --list failed: %v\n%s", err, string(b))
	}
	out := string(b)
	for _, expected := range []string{"Run targets", "@prisma-ui/demo", "dev", "preview", "@prisma-ui/docs", "runtime: system", "ready: http /"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("run --list missing %q:\n%s", expected, out)
		}
	}
	if _, statErr := os.Stat(markerPath); !os.IsNotExist(statErr) {
		t.Fatalf("run --list started a process; marker stat err=%v", statErr)
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--package", "@prisma-ui/demo", "--list", "--json")
	cmd.Dir = repo
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run --list --json failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("json list wrote stderr: %q", stderr.String())
	}
	var payload struct {
		Command string `json:"command"`
		Mode    string `json:"mode"`
		Package string `json:"package"`
		Targets []struct {
			ID      string   `json:"id"`
			Package string   `json:"package"`
			Name    string   `json:"name"`
			Command []string `json:"command"`
		} `json:"targets"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &payload); err != nil {
		t.Fatalf("json list stdout was not parseable: %v\n%s", err, stdout.String())
	}
	if payload.Command != "run" || payload.Mode != "list" || payload.Package != "@prisma-ui/demo" || len(payload.Targets) != 2 {
		t.Fatalf("unexpected json list payload: %+v", payload)
	}
	if payload.Targets[0].ID != "@prisma-ui/demo:dev" || payload.Targets[1].ID != "@prisma-ui/demo:preview" {
		t.Fatalf("unexpected target ids: %+v", payload.Targets)
	}
}

func TestCLIRunPackageSelectionAndAmbiguityDiagnostics(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	server := `const http=require('http'); const p=Number(process.argv[2]); http.createServer((_,res)=>{res.statusCode=200;res.end('ok')}).listen(p,'127.0.0.1'); setInterval(()=>{},1000);`
	_ = os.WriteFile(filepath.Join(root, "server.js"), []byte(server), 0o644)
	portDemo := reservePort(t)
	portDocs := reservePort(t)
	portTools := reservePort(t)
	stubIR := fmt.Sprintf(`{format:1,workspace:{name:"ws"},packages:[{name:"@prisma-ui/demo",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","server.js","%d"],url:"http://127.0.0.1:%d",ready:{kind:"http",path:"/"}}]},{name:"@prisma-ui/docs",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","server.js","%d"],url:"http://127.0.0.1:%d",ready:{kind:"http",path:"/"}}]},{name:"@prisma-ui/tools",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"lint",runtime:"system",command:["node","server.js","%d"],url:"http://127.0.0.1:%d",ready:{kind:"http",path:"/"}}]}]}`, portDemo, portDemo, portDocs, portDocs, portTools, portTools)
	writeRunFrontendStub(t, stubIR)

	cmd := exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "dev", "--once")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_RUN_TARGET_AMBIGUOUS") || !strings.Contains(string(b), "@prisma-ui/demo:dev") || !strings.Contains(string(b), "@prisma-ui/docs:dev") || !strings.Contains(string(b), "--package <name>") {
		t.Fatalf("expected package-qualified ambiguity: %v\n%s", err, string(b))
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--package", "@prisma-ui/demo", "dev", "--once")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), `Starting run target "@prisma-ui/demo:dev"`) || !strings.Contains(string(b), "Package: @prisma-ui/demo") || !strings.Contains(string(b), fmt.Sprintf("Ready: http://127.0.0.1:%d", portDemo)) {
		t.Fatalf("package-scoped run failed: %v\n%s", err, string(b))
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--package", "@prisma-ui/tools", "--once")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), `Starting run target "@prisma-ui/tools:lint"`) {
		t.Fatalf("package single-target fallback failed: %v\n%s", err, string(b))
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--package", "@prisma-ui/missing", "dev", "--once")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_RUN_PACKAGE_NOT_FOUND") || !strings.Contains(string(b), "@prisma-ui/demo") || !strings.Contains(string(b), "@prisma-ui/docs") {
		t.Fatalf("expected missing package diagnostic: %v\n%s", err, string(b))
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--package", "@prisma-ui/demo", "missing", "--once")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_RUN_TARGET_NOT_FOUND") || !strings.Contains(string(b), "@prisma-ui/demo") || !strings.Contains(string(b), "dev") {
		t.Fatalf("expected package target-not-found diagnostic: %v\n%s", err, string(b))
	}
}

func TestCLIRunCwdPolicyWorkspaceAndPackage(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	packageDir := filepath.Join(root, "packages", "demo")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	server := `const fs=require('fs'); const http=require('http'); const path=require('path');
const marker=process.argv[2]; const port=Number(process.argv[3]); fs.writeFileSync(marker, process.cwd());
http.createServer((_,res)=>{res.statusCode=200;res.end('ok')}).listen(port,'127.0.0.1'); setInterval(()=>{},1000);`
	_ = os.WriteFile(filepath.Join(packageDir, "server.js"), []byte(server), 0o644)
	workspaceMarker := filepath.Join(root, "workspace-cwd.txt")
	packageMarker := filepath.Join(root, "package-cwd.txt")
	workspacePort := reservePort(t)
	packagePort := reservePort(t)
	stubIR := fmt.Sprintf(`{format:1,workspace:{name:"ws"},packages:[{name:"@acme/demo",version:"1.0.0",root:"packages/demo",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"workspace-dev",runtime:"system",cwd:"workspace",command:["node","packages/demo/server.js",%q,"%d"],url:"http://127.0.0.1:%d",ready:{kind:"http",path:"/"}},{name:"package-dev",runtime:"system",cwd:"package",command:["node","server.js",%q,"%d"],url:"http://127.0.0.1:%d",ready:{kind:"http",path:"/"}}]}]}`, workspaceMarker, workspacePort, workspacePort, packageMarker, packagePort, packagePort)
	writeRunFrontendStub(t, stubIR)

	cmd := exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--package", "@acme/demo", "workspace-dev", "--once")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "Cwd: workspace (") {
		t.Fatalf("workspace cwd run failed: %v\n%s", err, string(b))
	}
	workspaceCwd, err := os.ReadFile(workspaceMarker)
	if err != nil || string(workspaceCwd) != root {
		t.Fatalf("workspace cwd marker = %q, err=%v, want %q", string(workspaceCwd), err, root)
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--package", "@acme/demo", "package-dev", "--once")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "Cwd: package (") || !strings.Contains(string(b), packageDir) {
		t.Fatalf("package cwd run failed: %v\n%s", err, string(b))
	}
	packageCwd, err := os.ReadFile(packageMarker)
	if err != nil || string(packageCwd) != packageDir {
		t.Fatalf("package cwd marker = %q, err=%v, want %q", string(packageCwd), err, packageDir)
	}
}

func TestCLIRunOmittedCwdListsAsWorkspace(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	stubIR := `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","server.js"],url:"http://127.0.0.1:5999"}]}]}`
	writeRunFrontendStub(t, stubIR)

	cmd := exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--list")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "cwd: workspace (") {
		t.Fatalf("list text missing workspace cwd: %v\n%s", err, string(b))
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--list", "--json")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list json failed: %v\n%s", err, string(b))
	}
	var payload struct {
		Targets []struct {
			Cwd     string `json:"cwd"`
			CwdPath string `json:"cwdPath"`
		} `json:"targets"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, string(b))
	}
	if len(payload.Targets) != 1 || payload.Targets[0].Cwd != "workspace" || payload.Targets[0].CwdPath != root {
		t.Fatalf("unexpected cwd json: %+v", payload.Targets)
	}
}

func TestCLIRunListInvalidArgs(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","server.js"],url:"http://127.0.0.1:5999"}]}]}`)

	for _, args := range [][]string{
		{"run", "--root", root, "--list", "dev"},
		{"run", "--root", root, "--list", "--once"},
		{"run", "--root", root, "--package"},
	} {
		cmd := exec.Command("go", append([]string{"run", "./cmd/tspack"}, args...)...)
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(b), "TSPACK_RUN_INVALID_ARGS") {
			t.Fatalf("expected invalid args for %v: %v\n%s", args, err, string(b))
		}
	}
}

func TestRunEnvOverlayParsing(t *testing.T) {
	tests := []struct {
		name        string
		assignments []string
		wantKeys    []string
		wantValues  map[string]string
		wantErr     string
	}{
		{name: "single", assignments: []string{"KEY=VALUE"}, wantKeys: []string{"KEY"}, wantValues: map[string]string{"KEY": "VALUE"}},
		{name: "duplicate last wins", assignments: []string{"KEY=first", "KEY=second"}, wantKeys: []string{"KEY"}, wantValues: map[string]string{"KEY": "second"}},
		{name: "empty value", assignments: []string{"FOO="}, wantKeys: []string{"FOO"}, wantValues: map[string]string{"FOO": ""}},
		{name: "value with equals", assignments: []string{"FOO=bar=baz"}, wantKeys: []string{"FOO"}, wantValues: map[string]string{"FOO": "bar=baz"}},
		{name: "missing equals", assignments: []string{"FOO"}, wantErr: "TSPACK_RUN_INVALID_ENV"},
		{name: "empty key", assignments: []string{"=bar"}, wantErr: "TSPACK_RUN_INVALID_ENV"},
		{name: "digit key", assignments: []string{"1FOO=bar"}, wantErr: "TSPACK_RUN_INVALID_ENV"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			overlay := runEnvOverlay{}
			var gotErr *runErr
			for _, assignment := range tc.assignments {
				overlay, gotErr = overlay.WithAssignment(assignment)
				if gotErr != nil {
					break
				}
			}
			if tc.wantErr != "" {
				if gotErr == nil || gotErr.code != tc.wantErr {
					t.Fatalf("expected %s, got %#v", tc.wantErr, gotErr)
				}
				return
			}
			if gotErr != nil {
				t.Fatalf("unexpected error: %#v", gotErr)
			}
			if !reflect.DeepEqual(overlay.Keys, tc.wantKeys) {
				t.Fatalf("keys = %#v, want %#v", overlay.Keys, tc.wantKeys)
			}
			if !reflect.DeepEqual(overlay.Values, tc.wantValues) {
				t.Fatalf("values = %#v, want %#v", overlay.Values, tc.wantValues)
			}
		})
	}
}

func TestRunEnvOverlayExecutionStreamsAndParentEnv(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	script := `process.stdout.write('PORT=' + process.env.PORT + '\n');
process.stdout.write('EMPTY=' + JSON.stringify(process.env.EMPTY_VALUE) + '\n');
process.stdout.write('EQUALS=' + process.env.EQUALS_VALUE + '\n');
process.stdout.write('INHERITED=' + process.env.TSPACK_PARENT_ENV + '\n');
process.stderr.write('child stderr passthrough\n');
process.stdout.write('READY\n');
setInterval(() => {}, 1000);
`
	_ = os.WriteFile(filepath.Join(root, "env-ready.js"), []byte(script), 0o644)
	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","env-ready.js"],ready:{kind:"stdout-match",pattern:"READY",stream:"stdout"}}]}]}`)

	t.Setenv("TSPACK_PARENT_ENV", "from-parent")
	if before := os.Getenv("PORT"); before != "" {
		t.Setenv("PORT", before)
	} else {
		_ = os.Unsetenv("PORT")
	}

	cmd := exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--ready-timeout", "3", "--once", "--env", "PORT=1111", "--env", "PORT=2222", "--env", "EMPTY_VALUE=", "--env", "EQUALS_VALUE=bar=baz", "--env", "SECRET_VALUE=top-secret")
	cmd.Dir = repo
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run --env failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	stdoutText := stdout.String()
	stderrText := stderr.String()
	for _, expected := range []string{"PORT=2222", "EMPTY=\"\"", "EQUALS=bar=baz", "INHERITED=from-parent", "READY"} {
		if !strings.Contains(stdoutText, expected) {
			t.Fatalf("stdout missing %q:\n%s", expected, stdoutText)
		}
	}
	if !strings.Contains(stderrText, "child stderr passthrough") {
		t.Fatalf("child stderr did not pass through:\n%s", stderrText)
	}
	if !strings.Contains(stderrText, "Env: PORT, EMPTY_VALUE, EQUALS_VALUE, SECRET_VALUE") {
		t.Fatalf("stderr missing env keys:\n%s", stderrText)
	}
	for _, leaked := range []string{"Env: PORT=", "1111", "2222", "bar=baz", "top-secret"} {
		if strings.Contains(stderrText, leaked) {
			t.Fatalf("stderr leaked env value %q:\n%s", leaked, stderrText)
		}
	}
	if os.Getenv("PORT") == "2222" || os.Getenv("SECRET_VALUE") == "top-secret" {
		t.Fatalf("parent environment was mutated")
	}
	if strings.Contains(stdoutText, "Starting run target") || strings.Contains(stdoutText, "Env:") {
		t.Fatalf("stdout contains TSPack status:\n%s", stdoutText)
	}
}

func TestRunEnvOverlayInvalidCLIAndList(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","server.js"],url:"http://127.0.0.1:5999"}]}]}`)

	tests := []struct {
		name string
		args []string
		code string
	}{
		{name: "no value", args: []string{"run", "--root", root, "--env"}, code: "TSPACK_RUN_INVALID_ENV"},
		{name: "missing equals", args: []string{"run", "--root", root, "--env", "FOO"}, code: "TSPACK_RUN_INVALID_ENV"},
		{name: "empty key", args: []string{"run", "--root", root, "--env", "=bar"}, code: "TSPACK_RUN_INVALID_ENV"},
		{name: "digit key", args: []string{"run", "--root", root, "--env", "1FOO=bar"}, code: "TSPACK_RUN_INVALID_ENV"},
		{name: "list env", args: []string{"run", "--root", root, "--list", "--env", "PORT=3001"}, code: "TSPACK_RUN_INVALID_ARGS"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("go", append([]string{"run", "./cmd/tspack"}, tc.args...)...)
			cmd.Dir = repo
			b, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(b), tc.code) {
				t.Fatalf("expected %s for %v: %v\n%s", tc.code, tc.args, err, string(b))
			}
		})
	}
}

func TestRunEnvOverlayHTTPReadinessPackageCwdAndManifest(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	packageDir := filepath.Join(root, "packages", "app")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "custom.manifest.tsx")
	_ = os.WriteFile(manifestPath, []byte("export default {}\n"), 0o644)
	markerPath := filepath.Join(root, "env-marker.txt")
	port := reservePort(t)
	script := `const fs = require('fs');
const http = require('http');
const port = Number(process.env.PORT);
fs.writeFileSync(process.env.MARKER_PATH, process.env.PACKAGE_ENV + '|' + process.cwd());
http.createServer((_, res) => { res.statusCode = 200; res.end('ok'); }).listen(port, '127.0.0.1');
setInterval(() => {}, 1000);
`
	_ = os.WriteFile(filepath.Join(packageDir, "server.js"), []byte(script), 0o644)
	stubIR := fmt.Sprintf(`{format:1,workspace:{name:"ws"},packages:[{name:"@acme/app",version:"1.0.0",root:"packages/app",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",cwd:"package",command:["node","server.js"],url:"http://127.0.0.1:%d",ready:{kind:"http",path:"/"}}]}]}`, port)
	writeRunFrontendStub(t, stubIR)

	cmd := exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--manifest", manifestPath, "--package", "@acme/app", "dev", "--ready-timeout", "3", "--once", "--env", fmt.Sprintf("PORT=%d", port), "--env", "PACKAGE_ENV=ok", "--env", "MARKER_PATH="+markerPath)
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("package cwd manifest env run failed: %v\n%s", err, string(b))
	}
	marker, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("missing marker: %v", err)
	}
	if string(marker) != "ok|"+packageDir {
		t.Fatalf("marker = %q, want %q", string(marker), "ok|"+packageDir)
	}
	if !strings.Contains(string(b), "Env: PORT, PACKAGE_ENV, MARKER_PATH") || strings.Contains(string(b), "PACKAGE_ENV=ok") {
		t.Fatalf("unexpected status output:\n%s", string(b))
	}
}

func TestInspectRunEnvOverlay(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	port := reservePort(t)
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	script := `const http = require('http');
const port = Number(process.env.PORT);
http.createServer((_, res) => { res.statusCode = 200; res.end(process.env.INSPECT_ENV); }).listen(port, '127.0.0.1');
setInterval(() => {}, 1000);
`
	_ = os.WriteFile(filepath.Join(root, "server.js"), []byte(script), 0o644)
	writeRunFrontendStub(t, fmt.Sprintf(`{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","server.js"],url:"http://127.0.0.1:%d",ready:{kind:"http",path:"/"}}]}]}`, port))

	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	bridge := filepath.Join(frontend, "inspect-cli.js")
	stub := `#!/usr/bin/env node
import http from 'node:http';
const args = process.argv.slice(2);
http.get(args[1], (res) => {
  let body = '';
  res.on('data', (chunk) => body += chunk);
  res.on('end', () => {
    if (body !== 'inspect-value') {
      console.error('missing inspect env: ' + body);
      process.exit(2);
    }
    console.log('{"ok":true}');
  });
}).on('error', (error) => {
  console.error(error.message);
  process.exit(3);
});
`
	_ = os.WriteFile(bridge, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(bridge) })

	cmd := exec.Command("go", "run", "./cmd/tspack", "inspect", "--run", "dev", "--root", root, "--json", "--env", fmt.Sprintf("PORT=%d", port), "--env", "INSPECT_ENV=inspect-value")
	cmd.Dir = repo
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("inspect --run --env failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != `{"ok":true}` {
		t.Fatalf("inspect stdout not clean JSON: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Env: PORT, INSPECT_ENV") || strings.Contains(stderr.String(), "inspect-value") {
		t.Fatalf("unexpected inspect stderr:\n%s", stderr.String())
	}
}

func TestCLIRunTCPReadyKind(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	port := reservePort(t)
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	script := `const net = require('net');
const port = Number(process.argv[2]);
setTimeout(() => {
  const server = net.createServer((socket) => socket.end('ok'));
  server.listen(port, '127.0.0.1');
}, 200);
setInterval(() => {}, 1000);
`
	_ = os.WriteFile(filepath.Join(root, "tcp-server.js"), []byte(script), 0o644)
	writeRunFrontendStub(t, fmt.Sprintf(`{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","tcp-server.js","%d"],ready:{kind:"tcp",port:%d}}]}]}`, port, port))

	cmd := exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--ready-timeout", "3", "--once")
	cmd.Dir = repo
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("tcp run failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "Waiting for:") || !strings.Contains(stderr.String(), fmt.Sprintf("Waiting for: tcp 127.0.0.1:%d", port)) || !strings.Contains(stderr.String(), fmt.Sprintf("Ready: tcp 127.0.0.1:%d", port)) {
		t.Fatalf("unexpected tcp run output:\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
}

func TestCLIRunTCPReadyTimeout(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	port := reservePort(t)
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "hang.js"), []byte("setInterval(() => {}, 1000);\n"), 0o644)
	writeRunFrontendStub(t, fmt.Sprintf(`{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","hang.js"],ready:{kind:"tcp",port:%d}}]}]}`, port))

	cmd := exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--ready-timeout", "1", "--once")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_RUN_READY_TIMEOUT") {
		t.Fatalf("expected tcp timeout: %v\n%s", err, string(b))
	}
}

func TestCLIRunStdoutMatchReadyKindPreservesStreams(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	script := `process.stdout.write('child stdout before READY');
setTimeout(() => process.stdout.write('-TOKEN after\n'), 100);
process.stderr.write('child stderr passthrough\n');
setInterval(() => {}, 1000);
`
	_ = os.WriteFile(filepath.Join(root, "stdout-ready.js"), []byte(script), 0o644)
	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","stdout-ready.js"],url:"http://127.0.0.1:5173",ready:{kind:"stdout-match",pattern:"READY-TOKEN",stream:"stdout"}}]}]}`)

	cmd := exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--ready-timeout", "3", "--once")
	cmd.Dir = repo
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("stdout-match run failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "READY-TOKEN") || !strings.Contains(stderr.String(), "child stderr passthrough") {
		t.Fatalf("child streams were not preserved:\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "Waiting for:") || !strings.Contains(stderr.String(), `Waiting for: stdout-match "READY-TOKEN" on stdout`) || !strings.Contains(stderr.String(), `Ready: matched "READY-TOKEN"`) || !strings.Contains(stderr.String(), "URL: http://127.0.0.1:5173") {
		t.Fatalf("unexpected stdout-match status output:\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
}

func TestCLIRunStdoutMatchStreamSelectionAndEarlyExit(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "stderr-only.js"), []byte("process.stderr.write('READY on stderr\\n'); setInterval(() => {}, 1000);\n"), 0o644)
	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","stderr-only.js"],ready:{kind:"stdout-match",pattern:"READY",stream:"stdout"}}]}]}`)
	cmd := exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--ready-timeout", "1", "--once")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_RUN_READY_TIMEOUT") || !strings.Contains(string(b), "READY on stderr") {
		t.Fatalf("expected stream-specific timeout with stderr passthrough: %v\n%s", err, string(b))
	}

	_ = os.WriteFile(filepath.Join(root, "exit-before-ready.js"), []byte("process.stdout.write('not yet\\n');\n"), 0o644)
	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","exit-before-ready.js"],ready:{kind:"stdout-match",pattern:"READY"}}]}]}`)
	cmd = exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--ready-timeout", "2", "--once")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_RUN_PROCESS_EXITED_EARLY") {
		t.Fatalf("expected stdout-match early exit: %v\n%s", err, string(b))
	}
}

func TestCLIRunListShowsNewReadyKinds(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"tcp",runtime:"system",command:["node","server.js"],ready:{kind:"tcp",host:"127.0.0.1",port:5432}},{name:"stdout",runtime:"system",command:["node","server.js"],ready:{kind:"stdout-match",pattern:"Local:",stream:"both"}}]}]}`)

	cmd := exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--list")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "ready: tcp 127.0.0.1:5432") || !strings.Contains(string(b), `ready: stdout-match "Local:" on both`) {
		t.Fatalf("run --list missing new ready details: %v\n%s", err, string(b))
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "run", "--root", root, "--list", "--json")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), `"kind": "tcp"`) || !strings.Contains(string(b), `"port": 5432`) || !strings.Contains(string(b), `"kind": "stdout-match"`) || !strings.Contains(string(b), `"pattern": "Local:"`) {
		t.Fatalf("run --list --json missing new ready details: %v\n%s", err, string(b))
	}
}
