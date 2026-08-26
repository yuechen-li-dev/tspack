package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

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

	cmd := exec.Command(testTspackBinary, "test", "--root", root, "--list", "--xtest-bridge", bridge)
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
	cmd = exec.Command(testTspackBinary, "test", "--root", root, "--filter", listedID, "--xtest-bridge", bridge)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "PASS "+listedID) {
		t.Fatalf("copied ID filter failed: %v\n%s", err, string(b))
	}

	cmd = exec.Command(testTspackBinary, "test", "--root", root, "--filter", "[2]", "--xtest-bridge", bridge)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "PASS src/cx.xtest.tsx::suite/theory/name[2]") {
		t.Fatalf("case suffix filter failed: %v\n%s", err, string(b))
	}

	cmd = exec.Command(testTspackBinary, "test", "--root", root, "--filter", listedID, "--compact", "--xtest-bridge", bridge)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "PASS "+listedID) {
		t.Fatalf("compact bridge smoke failed: %v\n%s", err, string(b))
	}

	cmd = exec.Command(testTspackBinary, "test", "--root", root, "--list", "--compact", "--xtest-bridge", bridge)
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

	cmd := exec.Command(testTspackBinary, "test", "--root", root, "--xtest-bridge", bridge)
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "PASS src/theory.xtest.tsx::suite/callback before[1]") {
		t.Fatalf("callback-before-cases bridge smoke failed: %v\n%s", err, string(b))
	}

	cmd = exec.Command(testTspackBinary, "test", "--root", root, "--filter", "zero", "--xtest-bridge", bridge)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_TEST_THEORY_NO_CASES") {
		t.Fatalf("zero-case theory diagnostic smoke failed: %v\n%s", err, string(b))
	}
}

func TestCLITestNativeBridgeIgnoresWorkspaceRuntimeProfile(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	bridge := filepath.Join(root, "native-test-cli.js")
	recordPath := filepath.Join(root, "bridge-args.txt")
	stub := `#!/usr/bin/env node
const fs = require('fs');
fs.writeFileSync(process.env.TSPACK_BRIDGE_ARGS, process.argv.slice(2).join('\t'));
console.log('Native xTest results');
console.log('');
console.log('PASS src/runtime.xtest.tsx::runtime/pass');
`
	if err := os.WriteFile(bridge, []byte(stub), 0o755); err != nil {
		t.Fatalf("write bridge: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	manifestText := `import { define } from "tspack/manifest";
export default define(
  <Workspace name="runtime-test" runtime="deno">
    <Package name="runtime-test" version="1.0.0" kind="library" />
  </Workspace>,
);
`
	if err := os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte(manifestText), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "runtime.xtest.tsx"), []byte("export default null\n"), 0o644); err != nil {
		t.Fatalf("write xtest: %v", err)
	}

	cmd := exec.Command(testTspackBinary, "test", "--root", root, "--xtest-bridge", bridge)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "TSPACK_BRIDGE_ARGS="+recordPath)
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "PASS src/runtime.xtest.tsx::runtime/pass") {
		t.Fatalf("native xTest bridge failed: %v\n%s", err, string(b))
	}
	recorded, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read bridge args: %v", err)
	}
	recordedText := string(recorded)
	for _, forbidden := range []string{"deno", "bun", "nodejs", "--runtime"} {
		if strings.Contains(recordedText, forbidden) {
			t.Fatalf("workspace runtime leaked into native xTest bridge args: %q", recordedText)
		}
	}
}

func TestCLITestXTestBridgeMissingDiagnostic(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample.xtest.tsx"), []byte("export default null\n"), 0o644); err != nil {
		t.Fatalf("write xtest: %v", err)
	}
	missing := filepath.Join(root, "missing-bridge.js")
	cmd := exec.Command(testTspackBinary, "test", "--root", root, "--xtest-bridge", missing)
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
	bridge := filepath.Join(repo, "manifest-frontend", "dist", "native-test-cli.js")
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

	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--once")
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

	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--once")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), fmt.Sprintf("Ready: http://127.0.0.1:%d", defaultPort)) {
		t.Fatalf("default manifest run failed: %v\n%s", err, string(b))
	}

	cmd = exec.Command(testTspackBinary, "run", "--root", root, "--manifest", explicitManifest, "--once")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), fmt.Sprintf("Ready: http://127.0.0.1:%d", explicitPort)) {
		t.Fatalf("explicit manifest run failed: %v\n%s", err, string(b))
	}

	missingManifest := filepath.Join(root, "missing.manifest.tsx")
	cmd = exec.Command(testTspackBinary, "run", "--root", root, "--manifest", missingManifest, "--once")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED") {
		t.Fatalf("expected missing explicit manifest diagnostic: %v\n%s", err, string(b))
	}
}

func writeRunManifestSwitchingStub(t *testing.T, repo string, explicitBase string, defaultPort int, explicitPort int) {
	t.Helper()
	frontend := testManifestFrontendBridgeDir(t)
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
	stubIR := `{format:1,workspace:{name:"ws"},packages:[{name:"@prisma-ui/demo",version:"1.0.0",kind:"service",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","marker.js"],url:"http://127.0.0.1:5991",ready:{kind:"http",path:"/"}},{name:"preview",runtime:"node",command:["vite","preview"],url:"http://127.0.0.1:5992",ready:{kind:"http",path:"/"}}]},{name:"@prisma-ui/docs",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","docs-server.js"],url:"http://127.0.0.1:5993",ready:{kind:"http",path:"/"}}]}]}`
	writeRunFrontendStub(t, stubIR)

	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--list")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run --list failed: %v\n%s", err, string(b))
	}
	out := string(b)
	for _, expected := range []string{
		"Run targets",
		"@prisma-ui/demo (service)",
		"dev",
		"preview",
		"@prisma-ui/docs",
		"runtime: system (explicit)",
		"ready: http /",
		"Runtime notes:",
		"node: resolves bare commands from project tool bins first; does not prepend node to script paths.",
		"system: runs commands directly without node-local tool resolution.",
		"explicit RunTarget runtime overrides the workspace runtime profile.",
		"unspecified RunTarget runtime inherits the workspace runtime profile.",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("run --list missing %q:\n%s", expected, out)
		}
	}
	if _, statErr := os.Stat(markerPath); !os.IsNotExist(statErr) {
		t.Fatalf("run --list started a process; marker stat err=%v", statErr)
	}

	cmd = exec.Command(testTspackBinary, "run", "--root", root, "--package", "@prisma-ui/demo", "--list", "--json")
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
			ID          string   `json:"id"`
			Package     string   `json:"package"`
			PackageKind string   `json:"packageKind"`
			Name        string   `json:"name"`
			Command     []string `json:"command"`
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
	if payload.Targets[0].PackageKind != "service" || payload.Targets[1].PackageKind != "service" {
		t.Fatalf("unexpected package kinds: %+v", payload.Targets)
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

	cmd := exec.Command(testTspackBinary, "run", "--root", root, "dev", "--once")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_RUN_TARGET_AMBIGUOUS") || !strings.Contains(string(b), "@prisma-ui/demo:dev") || !strings.Contains(string(b), "@prisma-ui/docs:dev") || !strings.Contains(string(b), "--package <name>") {
		t.Fatalf("expected package-qualified ambiguity: %v\n%s", err, string(b))
	}

	cmd = exec.Command(testTspackBinary, "run", "--root", root, "--package", "@prisma-ui/demo", "dev", "--once")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), `Starting run target "@prisma-ui/demo:dev"`) || !strings.Contains(string(b), "Package: @prisma-ui/demo") || !strings.Contains(string(b), fmt.Sprintf("Ready: http://127.0.0.1:%d", portDemo)) {
		t.Fatalf("package-scoped run failed: %v\n%s", err, string(b))
	}

	cmd = exec.Command(testTspackBinary, "run", "--root", root, "--package", "@prisma-ui/tools", "--once")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), `Starting run target "@prisma-ui/tools:lint"`) {
		t.Fatalf("package single-target fallback failed: %v\n%s", err, string(b))
	}

	cmd = exec.Command(testTspackBinary, "run", "--root", root, "--package", "@prisma-ui/missing", "dev", "--once")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_RUN_PACKAGE_NOT_FOUND") || !strings.Contains(string(b), "@prisma-ui/demo") || !strings.Contains(string(b), "@prisma-ui/docs") {
		t.Fatalf("expected missing package diagnostic: %v\n%s", err, string(b))
	}

	cmd = exec.Command(testTspackBinary, "run", "--root", root, "--package", "@prisma-ui/demo", "missing", "--once")
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

	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--package", "@acme/demo", "workspace-dev", "--once")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "Cwd: workspace (") {
		t.Fatalf("workspace cwd run failed: %v\n%s", err, string(b))
	}
	workspaceCwd, err := os.ReadFile(workspaceMarker)
	if err != nil || string(workspaceCwd) != root {
		t.Fatalf("workspace cwd marker = %q, err=%v, want %q", string(workspaceCwd), err, root)
	}

	cmd = exec.Command(testTspackBinary, "run", "--root", root, "--package", "@acme/demo", "package-dev", "--once")
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

func TestCLIRunExplicitBunRuntimeUsesBunExecutable(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	packageDir := filepath.Join(root, "packages", "demo")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	capture := filepath.Join(root, "bun-capture.json")
	bunBin := filepath.Join(root, "fake-bin")
	writeFakeBunRuntime(t, filepath.Join(bunBin, "bun"), capture)
	stubIR := `{format:1,workspace:{name:"ws",runtime:"bun"},packages:[{name:"@acme/demo",version:"1.0.0",root:"packages/demo",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"hello",runtime:"bun",cwd:"package",command:["hello.js"],ready:{kind:"stdout-match",pattern:"ready",stream:"stdout"}}]}]}`
	writeRunFrontendStub(t, stubIR)

	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--package", "@acme/demo", "hello", "--once", "--env", "FOO=bar")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+bunBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	b, err := cmd.CombinedOutput()
	output := string(b)
	if err != nil {
		t.Fatalf("bun run target failed: %v\n%s", err, output)
	}
	for _, expected := range []string{"Runtime: bun", "Command: bun hello.js", "Cwd: package (" + packageDir + ")", "ready from fake bun", "stderr from fake bun", "Ready: matched \"ready\""} {
		if !strings.Contains(output, expected) {
			t.Fatalf("bun run output missing %q:\n%s", expected, output)
		}
	}
	captured := readCapturedBunInvocation(t, capture)
	if !reflect.DeepEqual(captured.Argv, []string{"hello.js"}) {
		t.Fatalf("fake bun argv = %#v", captured.Argv)
	}
	if captured.Cwd != packageDir {
		t.Fatalf("fake bun cwd = %q, want %q", captured.Cwd, packageDir)
	}
	if captured.EnvFOO != "bar" {
		t.Fatalf("fake bun did not receive env overlay: %#v", captured)
	}
}

func TestCLIRunExplicitDenoRuntimeUsesDenoExecutable(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	packageDir := filepath.Join(root, "packages", "demo")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	capture := filepath.Join(root, "deno-capture.json")
	denoBin := filepath.Join(root, "fake-bin")
	writeFakeDenoRuntime(t, filepath.Join(denoBin, "deno"), capture)
	stubIR := `{format:1,workspace:{name:"ws",runtime:"deno"},packages:[{name:"@acme/demo",version:"1.0.0",root:"packages/demo",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"hello",runtime:"deno",cwd:"package",command:["run","--allow-net=127.0.0.1:8080","server.ts"],ready:{kind:"stdout-match",pattern:"ready",stream:"stdout"}}]}]}`
	writeRunFrontendStub(t, stubIR)

	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--package", "@acme/demo", "hello", "--once", "--env", "FOO=bar")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+denoBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	b, err := cmd.CombinedOutput()
	output := string(b)
	if err != nil {
		t.Fatalf("deno run target failed: %v\n%s", err, output)
	}
	for _, expected := range []string{"Runtime: deno", "Command: deno run --allow-net=127.0.0.1:8080 server.ts", "Cwd: package (" + packageDir + ")", "ready from fake deno", "stderr from fake deno", "Ready: matched \"ready\""} {
		if !strings.Contains(output, expected) {
			t.Fatalf("deno run output missing %q:\n%s", expected, output)
		}
	}
	captured := readCapturedDenoInvocation(t, capture)
	wantArgv := []string{"run", "--allow-net=127.0.0.1:8080", "server.ts"}
	if !reflect.DeepEqual(captured.Argv, wantArgv) {
		t.Fatalf("fake deno argv = %#v", captured.Argv)
	}
	if captured.Cwd != packageDir {
		t.Fatalf("fake deno cwd = %q, want %q", captured.Cwd, packageDir)
	}
	if captured.EnvFOO != "bar" {
		t.Fatalf("fake deno did not receive env overlay: %#v", captured)
	}
}

func TestCLIRunDenoRuntimeMissingFailsBeforeFallback(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws",runtime:"deno"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"hello",runtime:"deno",command:["run","server.ts"],ready:{kind:"stdout-match",pattern:"ready",stream:"stdout"}}]}]}`)

	cmd := exec.Command(testTspackBinary, "run", "--root", root, "hello", "--once")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+pathWithNodeOnly(t))
	b, err := cmd.CombinedOutput()
	output := string(b)
	if err == nil {
		t.Fatalf("expected missing deno failure:\n%s", output)
	}
	for _, expected := range []string{"TSPACK_RUN_RUNTIME_NOT_FOUND", "runtime: deno", "executable: deno", "target: hello", "install Deno or change the RunTarget runtime"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing deno output missing %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "Cannot find module") || strings.Contains(output, "sh -c") || strings.Contains(output, "npm") || strings.Contains(output, "bun ") {
		t.Fatalf("missing deno path appears to have fallen back unexpectedly:\n%s", output)
	}
}

func TestCLIRunBunRuntimeMissingFailsBeforeFallback(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws",runtime:"bun"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"hello",runtime:"bun",command:["hello.js"],ready:{kind:"stdout-match",pattern:"ready",stream:"stdout"}}]}]}`)

	cmd := exec.Command(testTspackBinary, "run", "--root", root, "hello", "--once")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+pathWithNodeOnly(t))
	b, err := cmd.CombinedOutput()
	output := string(b)
	if err == nil {
		t.Fatalf("expected missing bun failure:\n%s", output)
	}
	for _, expected := range []string{"TSPACK_RUN_RUNTIME_NOT_FOUND", "runtime: bun", "executable: bun", "target: hello", "install Bun or change the RunTarget runtime"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing bun output missing %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "Cannot find module") || strings.Contains(output, "sh -c") || strings.Contains(output, "npm") {
		t.Fatalf("missing bun path appears to have fallen back unexpectedly:\n%s", output)
	}
}

type capturedBunInvocation struct {
	Argv   []string `json:"argv"`
	Cwd    string   `json:"cwd"`
	EnvFOO string   `json:"envFOO"`
}

func writeFakeBunRuntime(t *testing.T, path string, capture string) {
	t.Helper()
	backend := fmt.Sprintf(`#!/usr/bin/env node
const fs = require('fs');
fs.writeFileSync(%q, JSON.stringify({ argv: process.argv.slice(2), cwd: process.cwd(), envFOO: process.env.FOO || '' }));
process.stdout.write('ready from fake bun\n');
process.stderr.write('stderr from fake bun\n');
setInterval(() => {}, 1000);
`, capture)
	_ = writeNodeBackedExecutable(t, path, backend)
}

func readCapturedBunInvocation(t *testing.T, capture string) capturedBunInvocation {
	t.Helper()
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var got capturedBunInvocation
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

type capturedDenoInvocation struct {
	Argv   []string `json:"argv"`
	Cwd    string   `json:"cwd"`
	EnvFOO string   `json:"envFOO"`
}

func writeFakeDenoRuntime(t *testing.T, path string, capture string) {
	t.Helper()
	backend := fmt.Sprintf(`#!/usr/bin/env node
const fs = require('fs');
fs.writeFileSync(%q, JSON.stringify({ argv: process.argv.slice(2), cwd: process.cwd(), envFOO: process.env.FOO || '' }));
process.stdout.write('ready from fake deno\n');
process.stderr.write('stderr from fake deno\n');
setInterval(() => {}, 1000);
`, capture)
	_ = writeNodeBackedExecutable(t, path, backend)
}

func readCapturedDenoInvocation(t *testing.T, capture string) capturedDenoInvocation {
	t.Helper()
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var got capturedDenoInvocation
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestCLIRunTargetRuntimeInheritanceResolution(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)

	cases := []struct {
		name          string
		ir            string
		wantRuntime   string
		wantSource    string
		wantExplicit  *string
		wantWorkspace *string
	}{
		{name: "workspace bun omitted", ir: `{format:1,workspace:{name:"ws",runtime:"bun"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",command:["server.js"],url:"http://127.0.0.1:5999"}]}]}`, wantRuntime: "bun", wantSource: "workspace", wantWorkspace: stringPtr("bun")},
		{name: "workspace deno omitted", ir: `{format:1,workspace:{name:"ws",runtime:"deno"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",command:["run","server.ts"],url:"http://127.0.0.1:5999"}]}]}`, wantRuntime: "deno", wantSource: "workspace", wantWorkspace: stringPtr("deno")},
		{name: "workspace nodejs omitted", ir: `{format:1,workspace:{name:"ws",runtime:"nodejs"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",command:["node","server.js"],url:"http://127.0.0.1:5999"}]}]}`, wantRuntime: "nodejs", wantSource: "workspace", wantWorkspace: stringPtr("nodejs")},
		{name: "no workspace omitted", ir: `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",command:["node","server.js"],url:"http://127.0.0.1:5999"}]}]}`, wantRuntime: "nodejs", wantSource: "default"},
		{name: "workspace bun explicit node", ir: `{format:1,workspace:{name:"ws",runtime:"bun"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"node",command:["node","server.js"],url:"http://127.0.0.1:5999"}]}]}`, wantRuntime: "node", wantSource: "explicit", wantExplicit: stringPtr("node"), wantWorkspace: stringPtr("bun")},
		{name: "workspace deno explicit system", ir: `{format:1,workspace:{name:"ws",runtime:"deno"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","server.js"],url:"http://127.0.0.1:5999"}]}]}`, wantRuntime: "system", wantSource: "explicit", wantExplicit: stringPtr("system"), wantWorkspace: stringPtr("deno")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := runListJSONForIR(t, repo, root, tc.ir)
			if len(payload.Targets) != 1 {
				t.Fatalf("expected one target: %#v", payload.Targets)
			}
			target := payload.Targets[0]
			if target.Runtime != tc.wantRuntime || target.RuntimeSource != tc.wantSource || !stringPtrEqual(target.ExplicitRuntime, tc.wantExplicit) || !stringPtrEqual(target.WorkspaceRuntime, tc.wantWorkspace) {
				t.Fatalf("unexpected runtime resolution: %#v", target)
			}
		})
	}
}

func stringPtr(value string) *string {
	return &value
}

func stringPtrEqual(left *string, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func TestCLIRunWorkspaceBunDoesNotOverrideSystemRunTarget(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	stubIR := `{format:1,workspace:{name:"ws",runtime:"bun"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",cwd:"workspace",command:["node","server.js"],url:"http://127.0.0.1:5999"}]}]}`
	payload := runListJSONForIR(t, repo, root, stubIR)
	if len(payload.Targets) != 1 || payload.Targets[0].Runtime != "system" {
		t.Fatalf("workspace bun runtime must not override explicit system RunTarget: %#v", payload.Targets)
	}
}

func TestCLIRunWorkspaceDenoDoesNotOverrideRunTargetRuntime(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	stubIR := `{format:1,workspace:{name:"ws",runtime:"deno"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"system-dev",runtime:"system",cwd:"workspace",command:["node","server.js"],url:"http://127.0.0.1:5999"},{name:"bun-dev",runtime:"bun",cwd:"workspace",command:["server.js"],url:"http://127.0.0.1:5998"}]}]}`
	payload := runListJSONForIR(t, repo, root, stubIR)
	if len(payload.Targets) != 2 {
		t.Fatalf("expected two run targets: %#v", payload.Targets)
	}
	if payload.Targets[0].Runtime != "system" || payload.Targets[1].Runtime != "bun" {
		t.Fatalf("workspace deno runtime must not override explicit RunTarget runtimes: %#v", payload.Targets)
	}
}

func TestCLIRunWorkspaceNodejsDoesNotOverrideRunTargetRuntime(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	omittedIR := `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",cwd:"workspace",command:["node","server.js"],url:"http://127.0.0.1:5999"}]}]}`
	explicitIR := `{format:1,workspace:{name:"ws",runtime:"nodejs"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",cwd:"workspace",command:["node","server.js"],url:"http://127.0.0.1:5999"}]}]}`
	omitted := runListJSONForIR(t, repo, root, omittedIR)
	explicit := runListJSONForIR(t, repo, root, explicitIR)

	if len(omitted.Targets) != 1 || omitted.Targets[0].Runtime != "system" {
		t.Fatalf("omitted workspace runtime must preserve explicit RunTarget runtime: %#v", omitted.Targets)
	}
	if len(explicit.Targets) != 1 || explicit.Targets[0].Runtime != "system" {
		t.Fatalf("workspace runtime must not override explicit RunTarget runtime: %#v", explicit.Targets)
	}
}

type runListJSONPayload struct {
	Targets []struct {
		Runtime          string  `json:"runtime"`
		RuntimeSource    string  `json:"runtimeSource"`
		ExplicitRuntime  *string `json:"explicitRuntime"`
		WorkspaceRuntime *string `json:"workspaceRuntime"`
		Cwd              string  `json:"cwd"`
		CwdPath          string  `json:"cwdPath"`
	} `json:"targets"`
}

func runListJSONForIR(t *testing.T, repo string, root string, irJSON string) runListJSONPayload {
	t.Helper()
	writeRunFrontendStub(t, irJSON)
	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--list", "--json")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run --list --json failed: %v\n%s", err, string(b))
	}
	var payload runListJSONPayload
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatalf("invalid run list json: %v\n%s", err, string(b))
	}
	return payload
}

func TestCLIRunOmittedCwdListsAsWorkspace(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	stubIR := `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","server.js"],url:"http://127.0.0.1:5999"}]}]}`
	writeRunFrontendStub(t, stubIR)

	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--list")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "cwd: workspace (") {
		t.Fatalf("list text missing workspace cwd: %v\n%s", err, string(b))
	}

	cmd = exec.Command(testTspackBinary, "run", "--root", root, "--list", "--json")
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
		cmd := exec.Command(testTspackBinary, args...)
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

	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--ready-timeout", "3", "--once", "--env", "PORT=1111", "--env", "PORT=2222", "--env", "EMPTY_VALUE=", "--env", "EQUALS_VALUE=bar=baz", "--env", "SECRET_VALUE=top-secret")
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
			cmd := exec.Command(testTspackBinary, tc.args...)
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

	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--manifest", manifestPath, "--package", "@acme/app", "dev", "--ready-timeout", "3", "--once", "--env", fmt.Sprintf("PORT=%d", port), "--env", "PACKAGE_ENV=ok", "--env", "MARKER_PATH="+markerPath)
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

	frontend := testManifestFrontendBridgeDir(t)
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

	cmd := exec.Command(testTspackBinary, "inspect", "--run", "dev", "--root", root, "--json", "--env", fmt.Sprintf("PORT=%d", port), "--env", "INSPECT_ENV=inspect-value")
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

	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--ready-timeout", "3", "--once")
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

	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--ready-timeout", "1", "--once")
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

	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--ready-timeout", "3", "--once")
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
	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--ready-timeout", "1", "--once")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_RUN_READY_TIMEOUT") || !strings.Contains(string(b), "READY on stderr") {
		t.Fatalf("expected stream-specific timeout with stderr passthrough: %v\n%s", err, string(b))
	}

	_ = os.WriteFile(filepath.Join(root, "exit-before-ready.js"), []byte("process.stdout.write('not yet\\n');\n"), 0o644)
	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","exit-before-ready.js"],ready:{kind:"stdout-match",pattern:"READY"}}]}]}`)
	cmd = exec.Command(testTspackBinary, "run", "--root", root, "--ready-timeout", "2", "--once")
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

	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--list")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "ready: tcp 127.0.0.1:5432") || !strings.Contains(string(b), `ready: stdout-match "Local:" on both`) {
		t.Fatalf("run --list missing new ready details: %v\n%s", err, string(b))
	}

	cmd = exec.Command(testTspackBinary, "run", "--root", root, "--list", "--json")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), `"kind": "tcp"`) || !strings.Contains(string(b), `"port": 5432`) || !strings.Contains(string(b), `"kind": "stdout-match"`) || !strings.Contains(string(b), `"pattern": "Local:"`) {
		t.Fatalf("run --list --json missing new ready details: %v\n%s", err, string(b))
	}
}

func TestCLIRunDenoDoesNotIntroduceToolingDelegation(t *testing.T) {
	repo := filepath.Join("..", "..")
	forbidden := []string{
		"deno " + "task",
		"deno " + "install",
		"deno " + "add",
		"deno " + "cache",
		"deno " + "vendor",
		"deno." + "lock",
		"deno." + "json",
		"import" + " map",
		"J" + "SR",
	}

	goFiles, err := filepath.Glob(filepath.Join(repo, "cmd", "tspack", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	goFiles = append(goFiles, filepath.Join(repo, "internal", "manifest", "ir.go"))
	for _, goFile := range goFiles {
		data, err := os.ReadFile(goFile)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, blocked := range forbidden {
			if strings.Contains(text, blocked) {
				t.Fatalf("Deno tooling delegation marker %q found in %s", blocked, goFile)
			}
		}
	}
}

func TestCLIRunRuntimeSwitchExplicitTargetsStayExplicitAcrossWorkspaceProfiles(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeBin := filepath.Join(root, "fake-bin")
	captureDir := filepath.Join(root, "captures")
	if err := os.MkdirAll(captureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeShellCaptureRuntime(t, filepath.Join(fakeBin, "bun"), filepath.Join(captureDir, "bun.txt"), "ready from fake bun")
	writeShellCaptureRuntime(t, filepath.Join(fakeBin, "deno"), filepath.Join(captureDir, "deno.txt"), "ready from fake deno")
	writeShellCaptureRuntime(t, filepath.Join(root, "scripts", "system-hello"), filepath.Join(captureDir, "system.txt"), "ready from fake system")

	for _, profile := range []string{"nodejs", "bun", "deno"} {
		t.Run(profile, func(t *testing.T) {
			irJSON := readRuntimeSwitchFixtureIRJSONForRun(t, repo, profile)
			writeRunFrontendStub(t, irJSON)

			listPayload := runListJSONForIR(t, repo, root, irJSON)
			gotRuntimes := make([]string, 0, len(listPayload.Targets))
			for _, target := range listPayload.Targets {
				gotRuntimes = append(gotRuntimes, target.Runtime)
			}
			wantRuntimes := []string{"node", "bun", "deno", "system"}
			if !reflect.DeepEqual(gotRuntimes, wantRuntimes) {
				t.Fatalf("workspace runtime %s changed explicit target runtimes: got %#v want %#v", profile, gotRuntimes, wantRuntimes)
			}

			runRuntimeSwitchTarget(t, repo, root, fakeBin, "bun-hello", "ready from fake bun")
			assertShellCapture(t, filepath.Join(captureDir, "bun.txt"), "scripts/bun-hello.js from-bun")
			runRuntimeSwitchTarget(t, repo, root, fakeBin, "deno-hello", "ready from fake deno")
			assertShellCapture(t, filepath.Join(captureDir, "deno.txt"), "run scripts/deno-hello.ts from-deno")
			runRuntimeSwitchTarget(t, repo, root, fakeBin, "system-hello", "ready from fake system")
			assertShellCapture(t, filepath.Join(captureDir, "system.txt"), "from-system")
		})
	}
}

func TestRunTargetStopTerminatesProcessGroupAfterStdoutReady(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process group termination is Unix-specific")
	}
	root := t.TempDir()
	scriptPath := filepath.Join(root, "long-running.sh")
	script := "#!/bin/sh\n" +
		"echo ready-from-child\n" +
		"while true; do sleep 1; done\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	target := manifest.RunTarget{
		Name:    "long-running",
		Runtime: "system",
		Command: []string{scriptPath},
		Ready: &manifest.RunReadyCheck{
			Kind:    "stdout-match",
			Pattern: "ready-from-child",
			Stream:  "stdout",
		},
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session, readyErr := startRunTargetInDir(root, root, target, 5*time.Second, &stdout, &stderr, runEnvOverlay{})
	if readyErr != nil {
		t.Fatalf("start run target failed: %s: %s\nstdout:\n%s\nstderr:\n%s", readyErr.code, readyErr.msg, stdout.String(), stderr.String())
	}

	done := make(chan error, 1)
	go func() {
		done <- session.Stop()
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stop failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return after readiness; likely stdout pipe remained open in a child process")
	}
}

func readRuntimeSwitchFixtureIRJSONForRun(t *testing.T, repo string, profile string) string {
	t.Helper()
	path := filepath.Join(repo, "fixtures", "valid", "runtime-switch-"+profile, "manifest.ir.golden.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	irJSON := string(data)
	if runtime.GOOS == "windows" {
		irJSON = strings.ReplaceAll(irJSON, `"scripts/system-hello"`, `".\\scripts\\system-hello.cmd"`)
	}
	return irJSON
}

func writeShellCaptureRuntime(t *testing.T, path string, capture string, readyLine string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		cmdPath := path
		if filepath.Ext(cmdPath) == "" {
			cmdPath += ".cmd"
		}
		script := fmt.Sprintf("@echo off\r\nsetlocal EnableExtensions\r\n> %s echo %%*\r\necho %s\r\n:loop\r\ntimeout /t 1 /nobreak >nul\r\ngoto loop\r\n", windowsBatchQuote(capture), readyLine)
		if err := os.WriteFile(cmdPath, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		return
	}
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$*\" > %q\necho %q\nwhile true; do sleep 1; done\n", capture, readyLine)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func windowsBatchQuote(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func runRuntimeSwitchTarget(t *testing.T, repo string, root string, fakeBin string, target string, expectedOutput string) {
	t.Helper()
	cmd := exec.Command(testTspackBinary, "run", "--root", root, target, "--once")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run %s failed: %v\n%s", target, err, string(output))
	}
	if !strings.Contains(string(output), expectedOutput) {
		t.Fatalf("run %s missing %q:\n%s", target, expectedOutput, string(output))
	}
}

func assertShellCapture(t *testing.T, path string, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != expected {
		t.Fatalf("capture %s = %q, want %q", path, strings.TrimSpace(string(data)), expected)
	}
}

func writeBasicFrontendStub(t *testing.T, repo string) {
	t.Helper()
	frontend := testManifestFrontendBridgeDir(t)
	if err := os.MkdirAll(frontend, 0o755); err != nil {
		t.Fatalf("create frontend stub directory: %v", err)
	}
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const out={ok:true,ir:{format:1,workspace:{name:"ws"},security:{acknowledgedCapabilities:[{package:"npm:unused@1.0.0",kind:"lifecycleScript",script:"postinstall",command:"node install.js",reason:"known fixture"}]},packages:[{name:"app",version:"1.0.0",kind:"library",dependencies:[],targets:[{name:"core",export:".",entry:"src/index.ts",runtime:"dist/index.js",types:"dist/index.d.ts",deps:[],peers:[]}],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{types:{},boundaries:{}}}]},diagnostics:[]};
process.stdout.write(JSON.stringify(out));`
	if err := os.WriteFile(cliPath, []byte(stub), 0o755); err != nil {
		t.Fatalf("write frontend stub: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(cliPath) })
}

func writeNoisyCheckFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"src", "dist"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("create fixture directory %s: %v", dir, err)
		}
	}
	files := map[string]string{
		"manifest.tsx":    "export default {}\n",
		"src/index.ts":    "export const x = 1\n",
		"dist/index.js":   "export const x = 1\n",
		"dist/index.d.ts": "export declare const x: number\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write fixture file %s: %v", name, err)
		}
	}
	lockBody := `[lock]
format = 1
tool = "tspack"

[[package]]
id = "npm:@types/estree@1.0.8"
name = "@types/estree"
version = "1.0.8"
source = "npm"
integrity = "sha512-a"

[[package]]
id = "npm:@types/estree@1.0.9"
name = "@types/estree"
version = "1.0.9"
source = "npm"
integrity = "sha512-b"

[[package]]
id = "npm:js-tokens@4.0.0"
name = "js-tokens"
version = "4.0.0"
source = "npm"
integrity = "sha512-c"

[[package]]
id = "npm:js-tokens@9.0.1"
name = "js-tokens"
version = "9.0.1"
source = "npm"
integrity = "sha512-d"

[[package]]
id = "npm:culori@4.0.0"
name = "culori"
version = "4.0.0"
source = "npm"
integrity = "sha512-e"
  [[package.capability]]
  kind = "lifecycleScript"
  script = "postinstall"
  command = "node install.js"

[[package]]
id = "npm:esbuild@1.0.0"
name = "esbuild"
version = "1.0.0"
source = "npm"
integrity = "sha512-f"
  [[package.capability]]
  kind = "lifecycleScript"
  script = "prepare"
  command = "node prepare.js"

[[target]]
package = "app"
name = "core"
export = "."
entry = "src/index.ts"
runtime = "dist/index.js"
types = "dist/index.d.ts"

[[edge]]
from = "app:target:core"
to = "npm:culori@4.0.0"
kind = "runtime"

[[edge]]
from = "app:target:core"
to = "npm:esbuild@1.0.0"
kind = "runtime"
`
	if err := os.WriteFile(filepath.Join(root, "ts-lock.toml"), []byte(lockBody), 0o644); err != nil {
		t.Fatalf("write fixture lockfile: %v", err)
	}
	return root
}
