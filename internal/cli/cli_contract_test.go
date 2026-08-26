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

func TestCLIHelpAndUnsupportedCommands(t *testing.T) {
	repo := filepath.Join("..", "..")
	help := newInProcessCommand("help")
	help.Dir = repo
	b, err := help.CombinedOutput()
	if err != nil {
		t.Fatalf("help failed: %v\n%s", err, string(b))
	}
	text := string(b)
	for _, cmd := range []string{"check", "update", "sync", "audit", "pack", "why", "outdated", "how", "format", "lint", "run", "test", "artifact", "bench", "doom", "inspect", "--version", "help"} {
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
	for _, unsupported := range []string{"dev", "publish", "add", "remove", "install"} {
		if strings.Contains(text, "tspack "+unsupported) {
			t.Fatalf("help unexpectedly advertises unsupported command %s", unsupported)
		}
	}

	// build has a real, deliberately bounded tscl implementation. Keep this
	// assertion focused on commands that are actually unsupported.
	for _, c := range []string{"publish"} {
		cmd := newInProcessCommand(c)
		cmd.Dir = repo
		ob, e := cmd.CombinedOutput()
		if e == nil || !strings.Contains(string(ob), "unknown command") {
			t.Fatalf("expected deterministic unknown command for %s: %v\n%s", c, e, string(ob))
		}
	}
}

func TestCLIVersionPrintsMetadata(t *testing.T) {
	repo := filepath.Join("..", "..")
	cmd := newInProcessCommand("--version")
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version failed: %v\n%s", err, string(output))
	}

	text := string(output)
	for _, expected := range []string{"tspack ", "commit ", "built "} {
		if !strings.Contains(text, expected) {
			t.Fatalf("version output missing %q: %s", expected, text)
		}
	}
}

func TestCLIUpdateDryRunUnknownFlagFailsDeterministically(t *testing.T) {
	repo := filepath.Join("..", "..")
	cmd := newInProcessCommand("update", "--dry-run", "--unknown")
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
	frontend := testManifestFrontendBridgeDir(t)
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

	cmd := newInProcessCommand("update", "--root", root, "--dry-run", "--json")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("update dry-run --json failed: %v\n%s", err, string(b))
	}
	var report updateDryRunJSONReport
	if e := json.Unmarshal(b, &report); e != nil {
		t.Fatalf("invalid json output: %v\n%s", e, string(b))
	}
	if report.Command != "update" || !report.DryRun.Enabled || !report.OK {
		t.Fatalf("unexpected json report: %#v", report)
	}
	if report.DryRun.Summary.Added != 0 || report.DryRun.Summary.Changed != 0 || report.DryRun.Summary.Removed != 0 {
		t.Fatalf("expected empty change summary: %#v", report.DryRun.Summary)
	}
}

func TestCLIRootRecomputesDefaultManifestPathAndRespectsExplicitManifest(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("abs repo: %v", err)
	}
	frontend := testManifestFrontendBridgeDir(t)
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const expected = process.env.TSPACK_EXPECT_MANIFEST;
const got = process.argv[2];
if (expected && got !== expected) {
  process.stdout.write(JSON.stringify({ok:false,ir:null,diagnostics:[{code:"TSPACK_TEST_UNEXPECTED_MANIFEST_PATH",severity:"error",message:"expected="+expected+" got="+got}]}));
  process.exit(0);
}
const out={ok:true,ir:{format:1,workspace:{name:"ws"},security:{acknowledgedCapabilities:[{package:"npm:unused@1.0.0",kind:"lifecycleScript",script:"postinstall",command:"node install.js",reason:"known fixture"}]},packages:[{name:"app",version:"1.0.0",kind:"library",dependencies:[],targets:[{name:"core",export:".",entry:"src/index.ts",runtime:"dist/index.js",types:"dist/index.d.ts",deps:[],peers:[]}],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{types:{},boundaries:{}}}]},diagnostics:[]};
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

	binPath := buildTspackBinary(t, repo)

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

func TestDocsReleaseGateIncludesMigrationCloseoutSmoke(t *testing.T) {
	closeoutDoc := filepath.Join("..", "..", "docs", "claude-fooding-migration.md")
	if _, err := os.Stat(closeoutDoc); err != nil {
		t.Fatalf("migration closeout doc missing: %v", err)
	}

	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "release-gate.md"))
	if err != nil {
		t.Fatalf("read release gate doc: %v", err)
	}

	text := string(doc)
	for _, phrase := range []string{
		"tspack migrate --check",
		"MIGRATION_TODO_TARGETS",
		"package-lock evidence",
		"source scan evidence",
		"RunTarget suggestions",
		"no script execution",
	} {
		if !strings.Contains(text, phrase) {
			t.Fatalf("release gate doc missing migration closeout smoke phrase %q", phrase)
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

func TestDocsReleaseGateIncludesPhase8FormatLintSmoke(t *testing.T) {
	phase8Doc := filepath.Join("..", "..", "docs", "claude-fooding-phase8.md")
	if _, err := os.Stat(phase8Doc); err != nil {
		t.Fatalf("phase 8 closeout doc missing: %v", err)
	}

	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "release-gate.md"))
	if err != nil {
		t.Fatalf("read release gate doc: %v", err)
	}

	text := string(doc)
	for _, phrase := range []string{
		"`tspack format --check`",
		"`tspack lint --fix --unsafe`",
		"`tspack check --format`",
		"node_modules/@biomejs/biome/bin/biome",
		"TSPACK_LINT_FIX_INCOMPLETE",
	} {
		if !strings.Contains(text, phrase) {
			t.Fatalf("release gate doc missing phase 8 format/lint smoke phrase %q", phrase)
		}
	}
}

func TestDocsReleaseGateIncludesRuntimeGroundedInspectCloseoutSmoke(t *testing.T) {
	closeoutDoc := filepath.Join("..", "..", "docs", "claude-fooding-runtime-grounded-ide.md")
	if _, err := os.Stat(closeoutDoc); err != nil {
		t.Fatalf("runtime-grounded IDE closeout doc missing: %v", err)
	}

	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "release-gate.md"))
	if err != nil {
		t.Fatalf("read release gate doc: %v", err)
	}

	text := string(doc)
	for _, phrase := range []string{
		"runtime-grounded",
		"inspect.url",
		"assert.inspect",
		"Copy Selected Inspect Node LLM Context",
		"Reveal Source",
		"Target.getTargets",
		"No screenshots",
	} {
		if !strings.Contains(text, phrase) {
			t.Fatalf("release gate doc missing runtime-grounded inspect smoke phrase %q", phrase)
		}
	}
}

func TestDocsReleaseGateIncludesRuntimeProfileCloseoutSmoke(t *testing.T) {
	closeoutDoc := filepath.Join("..", "..", "docs", "claude-fooding-runtime-profiles.md")
	if _, err := os.Stat(closeoutDoc); err != nil {
		t.Fatalf("runtime profile closeout doc missing: %v", err)
	}

	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "release-gate.md"))
	if err != nil {
		t.Fatalf("read release gate doc: %v", err)
	}

	text := string(doc)
	for _, phrase := range []string{
		`runtime="bun"`,
		`runtime="deno"`,
		"packageManagerDelegated",
		"bun <declared argv>",
		"deno <declared argv>",
		"no package-manager delegation",
		"Workspace.Runtime",
	} {
		if !strings.Contains(text, phrase) {
			t.Fatalf("release gate doc missing runtime profile smoke phrase %q", phrase)
		}
	}
}

func TestCheckJSONWarningOnlyLockfileMissing(t *testing.T) {
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
