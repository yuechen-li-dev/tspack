package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLICheckFormatFlagParsingAndRoot(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeValidCheckFrontendStub(t, repo)

	plainRoot := writeValidCheckProject(t)
	plainCmd := exec.Command(testTspackBinary, "check", "--root", plainRoot)
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
	assertBiomeArgsInclude(t, got, "format", "manifest.tsx", "src")
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
	assertBiomeArgsInclude(t, got, "format", "src")
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
	if missingErr == nil || !strings.Contains(missingOutput, "TSPACK_FORMAT_BACKEND_MISSING") {
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
		if diagnostic.Code == "TSPACK_FORMAT_BACKEND_MISSING" {
			found = true
			details := strings.Join(diagnostic.Details, "\n")
			if !strings.Contains(details, "underlying: TSPACK_BIOME_BACKEND_NOT_FOUND") {
				t.Fatalf("missing underlying detail: %+v", diagnostic)
			}
		}
	}
	if !found {
		t.Fatalf("expected structured format backend missing diagnostic: %+v", report.Diagnostics)
	}
}

func TestCLIBiomeMissingBackendAndInvalidFlags(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	cmd := exec.Command(testTspackBinary, "format", "--root", root)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+t.TempDir())
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_BIOME_BACKEND_NOT_FOUND") {
		t.Fatalf("missing backend diagnostic not shown: %v\n%s", err, string(b))
	}

	cmd = exec.Command(testTspackBinary, "format", "--fix", "--root", root)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_FORMAT_INVALID_FLAGS") {
		t.Fatalf("format invalid flags missing: %v\n%s", err, string(b))
	}

	cmd = exec.Command(testTspackBinary, "lint", "--check", "--root", root)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_LINT_INVALID_FLAGS") {
		t.Fatalf("lint invalid flags missing: %v\n%s", err, string(b))
	}

	cmd = exec.Command(testTspackBinary, "lint", "--unsafe", "--root", root)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_LINT_INVALID_FLAGS") || !strings.Contains(string(b), "--unsafe requires --fix") {
		t.Fatalf("lint unsafe invalid flags missing: %v\n%s", err, string(b))
	}

	cmd = exec.Command(testTspackBinary, "format", "--unsafe", "--root", root)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_FORMAT_INVALID_FLAGS") {
		t.Fatalf("format unsafe invalid flags missing: %v\n%s", err, string(b))
	}

	cmd = exec.Command(testTspackBinary, "format", "--check", "--unsafe", "--root", root)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_FORMAT_INVALID_FLAGS") {
		t.Fatalf("format check unsafe invalid flags missing: %v\n%s", err, string(b))
	}
}

func TestCLIHowCommand(t *testing.T) {
	repo := filepath.Join("..", "..")
	cmd := exec.Command(testTspackBinary, "how")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_HOW_CODE_REQUIRED") {
		t.Fatalf("expected required code diagnostic: %v\n%s", err, string(b))
	}

	cmd = exec.Command(testTspackBinary, "how", "NOPE")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_HOW_CODE_NOT_FOUND") || !strings.Contains(string(b), "tspack how --list") {
		t.Fatalf("expected not found diagnostic: %v\n%s", err, string(b))
	}

	cmd = exec.Command(testTspackBinary, "how", "TSPACK_LOCK_VERSION_CONFLICT")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("how new code failed: %v\n%s", err, string(b))
	}
	text := string(b)
	if !strings.Contains(text, "multiple versions") || !strings.Contains(text, "tspack why") || !strings.Contains(text, "valid") {
		t.Fatalf("expected guidance for conflict diagnostic: %s", text)
	}

	cmd = exec.Command(testTspackBinary, "how", "TSPACK_IR_INVALID_RELATIVE_PATH")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("how known code failed: %v\n%s", err, string(b))
	}
	if !strings.Contains(string(b), "types: \"\"") {
		t.Fatalf("expected app types empty string note: %s", string(b))
	}

	cmd = exec.Command(testTspackBinary, "how", "TSPACK_PACK_CHANGELOG_NOT_INCLUDED")
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
	frontend := testManifestFrontendBridgeDir(t)
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
	frontend := testManifestFrontendBridgeDir(t)
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
		detailsText := filepath.ToSlash(strings.Join(diagnostic.Details, "\n"))
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
	frontend := testManifestFrontendBridgeDir(t)
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
	frontend := testManifestFrontendBridgeDir(t)
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

func TestCLICheckSummarizesNoisyWarningsWithRevealFlags(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeBasicFrontendStub(t, repo)
	binPath := buildTspackBinary(t, repo)
	root := writeNoisyCheckFixture(t)

	defaultCmd := exec.Command(binPath, "check", "--root", root)
	defaultCmd.Dir = repo
	defaultOutput, err := defaultCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("warnings-only check should exit zero: %v\n%s", err, string(defaultOutput))
	}
	defaultText := string(defaultOutput)
	for _, expected := range []string{
		"TSPACK_LOCK_VERSION_CONFLICT: Version conflicts: 2 packages have multiple resolved versions.",
		"Examples: @types/estree (1.0.8, 1.0.9), js-tokens (4.0.0, 9.0.1)",
		"Run `tspack check --show-conflicts` for full conflict diagnostics.",
		"TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT: Lifecycle scripts: 1 consumer install-time scripts and 1 maintainer-side scripts found; execution is blocked by policy.",
		"Consumer examples: culori postinstall",
		"Maintainer examples: esbuild prepare",
		"Run `tspack check --show-lifecycle` for full script and pull-chain details.",
		"Run `tspack doctor security` for policy posture.",
		"TSPACK_SECURITY_ACKNOWLEDGED_CAPABILITY_UNUSED",
	} {
		if !strings.Contains(defaultText, expected) {
			t.Fatalf("default output missing %q:\n%s", expected, defaultText)
		}
	}
	for _, hidden := range []string{
		"TSPACK_LOCK_VERSION_CONFLICT: package \"@types/estree\" appears at multiple versions",
		"TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT: package declares install-time lifecycle script",
		"pulled by:",
	} {
		if strings.Contains(defaultText, hidden) {
			t.Fatalf("default output should summarize %q:\n%s", hidden, defaultText)
		}
	}

	revealCmd := exec.Command(binPath, "check", "--root", root, "--show-conflicts", "--show-lifecycle")
	revealCmd.Dir = repo
	revealOutput, err := revealCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("revealed warnings-only check should exit zero: %v\n%s", err, string(revealOutput))
	}
	revealText := string(revealOutput)
	for _, expected := range []string{
		"TSPACK_LOCK_VERSION_CONFLICT: package \"@types/estree\" appears at multiple versions",
		"TSPACK_LOCK_VERSION_CONFLICT: package \"js-tokens\" appears at multiple versions",
		"TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT: package declares install-time lifecycle script",
		"package: npm:culori@4.0.0",
		"lifecycleCategory: consumer-install",
		"consumerInstallTime: true",
		"pulled by:",
		"app:target:core -> npm:culori@4.0.0",
	} {
		if !strings.Contains(revealText, expected) {
			t.Fatalf("revealed output missing %q:\n%s", expected, revealText)
		}
	}
	jsonCmd := exec.Command(binPath, "check", "--root", root, "--json")
	jsonCmd.Dir = repo
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	jsonCmd.Stdout = &stdout
	jsonCmd.Stderr = &stderr
	if err := jsonCmd.Run(); err != nil {
		t.Fatalf("json warnings-only check should exit zero: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("json check wrote stderr: %s", stderr.String())
	}
	var report checkJSONReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, stdout.String())
	}
	if countDiagnostics(report.Diagnostics, "TSPACK_LOCK_VERSION_CONFLICT") != 2 {
		t.Fatalf("json should include all conflict diagnostics: %#v", report.Diagnostics)
	}
	if countDiagnostics(report.Diagnostics, "TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT") != 2 {
		t.Fatalf("json should include all lifecycle diagnostics: %#v", report.Diagnostics)
	}
	foundConsumer := false
	foundMaintainer := false
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Code != "TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT" {
			continue
		}
		if diagnostic.LifecycleCategory == "consumer-install" && diagnostic.ConsumerInstallTime != nil && *diagnostic.ConsumerInstallTime {
			foundConsumer = true
		}
		if diagnostic.LifecycleCategory == "maintainer-publish" && diagnostic.ConsumerInstallTime != nil && !*diagnostic.ConsumerInstallTime {
			foundMaintainer = true
		}
	}
	if !foundConsumer || !foundMaintainer {
		t.Fatalf("json lifecycle diagnostics missing classification: %#v", report.Diagnostics)
	}
}

func TestCLICheckHelpIncludesNoisyWarningRevealFlags(t *testing.T) {
	repo := filepath.Join("..", "..")
	binPath := buildTspackBinary(t, repo)
	cmd := exec.Command(binPath, "help")
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("help failed: %v\n%s", err, string(output))
	}
	text := string(output)
	for _, expected := range []string{
		"--show-conflicts   Show individual version conflict diagnostics instead of summary",
		"--show-lifecycle   Show individual lifecycle script diagnostics instead of summary",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("help missing %q:\n%s", expected, text)
		}
	}
}

func TestCLIUpdateTargetedDryRunJSONIncludesTargetFieldsOnlyJSON(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := testManifestFrontendBridgeDir(t)
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

	cmd := exec.Command(testTspackBinary, "update", "react", "--root", root, "--dry-run", "--json")
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
	frontend := testManifestFrontendBridgeDir(t)
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
	if stdout.String() != "lockfile diff: +1 -0\nchange attribution: 0 direct, 1 transitive closure\n" {
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
	frontend := testManifestFrontendBridgeDir(t)
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
	frontend := testManifestFrontendBridgeDir(t)
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
	frontend := testManifestFrontendBridgeDir(t)
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
	frontend := testManifestFrontendBridgeDir(t)
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
