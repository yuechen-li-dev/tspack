package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCLIWhySmoke(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := testManifestFrontendBridgeDir(t)
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

	cmd := exec.Command(testTspackBinary, "why", "vue", "--root", root, "--lockfile", filepath.Join(root, "ts-lock.toml"))
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("why vue failed: %v\n%s", err, string(b))
	}
	o := string(b)
	if !strings.Contains(o, "vue") || !strings.Contains(o, "reachable from") || !strings.Contains(o, "not reachable from") {
		t.Fatalf("unexpected why vue output: %s", o)
	}

	cmd = exec.Command(testTspackBinary, "why", "npm:left-pad@1.2.0", "--root", root, "--lockfile", filepath.Join(root, "ts-lock.toml"))
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

	cmd = exec.Command(testTspackBinary, "why", "left-pad", "--root", root, "--lockfile", filepath.Join(root, "ts-lock.toml"))
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

	cmd = exec.Command(testTspackBinary, "why")
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
	frontend := testManifestFrontendBridgeDir(t)
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
