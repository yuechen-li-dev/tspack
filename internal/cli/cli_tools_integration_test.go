package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCLITestNoBackendsAndVitestUnavailable(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()

	cmd := exec.Command(testTspackBinary, "test", "--root", root)
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero when no backends are present")
	}
	if !strings.Contains(string(b), "TSPACK_TEST_NO_BACKENDS") {
		t.Fatalf("missing no backends diagnostic: %s", string(b))
	}

	cmd = exec.Command(testTspackBinary, "test", "-vitest", "--root", root)
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
	frontend := testManifestFrontendBridgeDir(t)
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
	cmd := exec.Command(testTspackBinary, "artifact", "--root", root, "--list")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "TSPack artifacts") {
		t.Fatalf("artifact list failed: %v\n%s", err, string(b))
	}

	out := filepath.Join(root, "out")
	cmd = exec.Command(testTspackBinary, "artifact", "--root", root, "--out", out)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("artifact run failed: %v\n%s", err, string(b))
	}
	if _, err = os.Stat(filepath.Join(out, "artifact.txt")); err != nil {
		t.Fatalf("expected written artifact: %v", err)
	}

	cmd = exec.Command(testTspackBinary, "artifact", "--root", root, "--filter", "no-match")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_ARTIFACT_FILTER_NO_MATCH") {
		t.Fatalf("expected no-match failure: %v\n%s", err, string(b))
	}

	cmd = exec.Command(testTspackBinary, "artifact", "--root", root, "--json")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "\"summary\"") {
		t.Fatalf("expected json output: %v\n%s", err, string(b))
	}
}

func TestCLIDoomCommand(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := testManifestFrontendBridgeDir(t)
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
	cmd := exec.Command(testTspackBinary, "doom", "--root", root, "--list")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "TSPack doom") {
		t.Fatalf("doom list failed: %v\n%s", err, string(b))
	}
	cmd = exec.Command(testTspackBinary, "doom", "--root", root)
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "PASS") {
		t.Fatalf("doom run failed: %v\n%s", err, string(b))
	}
	cmd = exec.Command(testTspackBinary, "doom", "--root", root, "--filter", "none")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_DOOM_FILTER_NO_MATCH") {
		t.Fatalf("expected doom no-match failure: %v\n%s", err, string(b))
	}
	cmd = exec.Command(testTspackBinary, "doom", "--root", root, "--json")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "\"prophecies\"") {
		t.Fatalf("expected doom json output: %v\n%s", err, string(b))
	}
}

func TestCLIDoomBridgeMissing(t *testing.T) {
	repo := filepath.Join("..", "..")
	bridges := []string{
		filepath.Join(repo, "manifest-frontend", "dist", "native-test-cli.js"),
		filepath.Join(repo, "manifest-frontend", "dist", "src", "native-test-cli.js"),
	}
	for _, bridge := range bridges {
		backup := bridge + ".bak"
		if _, err := os.Stat(bridge); err == nil {
			if renameErr := os.Rename(bridge, backup); renameErr != nil {
				t.Fatalf("backup bridge: %v", renameErr)
			}
			t.Cleanup(func() { _ = os.Rename(backup, bridge) })
		}
	}
	cmd := exec.Command(testTspackBinary, "doom", "--root", t.TempDir())
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_DOOM_BRIDGE_MISSING") {
		t.Fatalf("expected bridge missing diagnostic: %v\n%s", err, string(b))
	}
}

func TestCLIInspectCommandRouting(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := testManifestFrontendBridgeDir(t)
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

	cmd := exec.Command(testTspackBinary, "inspect", "http://example.test", "--json", "--cdp", "http://127.0.0.1:9222", "--host-path", "/tmp/host", "--browser-path", "/tmp/browser", "--list-targets", "--target", "0", "--target-url", "localhost:5173")
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

	frontend := testManifestFrontendBridgeDir(t)
	bridge := filepath.Join(frontend, "inspect-cli.js")
	stub := `#!/usr/bin/env node
const args=process.argv.slice(2);
if(args[1] !== 'http://127.0.0.1:5221'){console.error('missing-run-url');process.exit(1)}
console.log('{"ok":true}');`
	_ = os.WriteFile(bridge, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(bridge) })
	cmd := exec.Command(testTspackBinary, "inspect", "dev", "--root", root, "--json")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "{\"ok\":true}") {
		t.Fatalf("inspect run target failed: %v\n%s", err, string(b))
	}
}

func TestCLIInspectRunTargetConflict(t *testing.T) {
	repo := filepath.Join("..", "..")
	cmd := exec.Command(testTspackBinary, "inspect", "http://localhost:1234", "--run", "dev")
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
	frontend := testManifestFrontendBridgeDir(t)
	bridge := filepath.Join(frontend, "inspect-cli.js")
	stub := fmt.Sprintf(`#!/usr/bin/env node
const args=process.argv.slice(2);
if (args[1] !== 'http://127.0.0.1:%d') { process.exit(2); }
console.log('{"ok":true}');`, port)
	_ = os.WriteFile(bridge, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(bridge) })
	cmd := exec.Command(testTspackBinary, "inspect", "--run", "dev", "--root", root, "--json")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), `{"ok":true}`) || !strings.Contains(string(b), `Stopped run target "dev".`) {
		t.Fatalf("inspect --run failed: %v\n%s", err, string(b))
	}
}

func TestCLIInspectTargetRequiredStillEnforced(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := testManifestFrontendBridgeDir(t)
	_ = os.MkdirAll(frontend, 0o755)
	bridge := filepath.Join(frontend, "inspect-cli.js")
	_ = os.WriteFile(bridge, []byte("#!/usr/bin/env node\nprocess.stderr.write('TSPACK_INSPECT_TARGET_REQUIRED\\n'); process.exit(1)\n"), 0o755)
	t.Cleanup(func() { _ = os.Remove(bridge) })
	cmd := exec.Command(testTspackBinary, "inspect")
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
	frontend := testManifestFrontendBridgeDir(t)
	bridge := filepath.Join(frontend, "inspect-cli.js")
	_ = os.WriteFile(bridge, []byte("#!/usr/bin/env node\nprocess.exit(0)\n"), 0o755)
	t.Cleanup(func() { _ = os.Remove(bridge) })
	cmd := exec.Command(testTspackBinary, "inspect", "nope", "--root", root)
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
	frontend := testManifestFrontendBridgeDir(t)
	bridge := filepath.Join(frontend, "inspect-cli.js")
	_ = os.WriteFile(bridge, []byte("#!/usr/bin/env node\nconsole.log('{\"ok\":true}')\n"), 0o755)
	t.Cleanup(func() { _ = os.Remove(bridge) })

	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","hang.js"],url:"http://127.0.0.1:5233",ready:{kind:"http",path:"/"}}]}]}`)
	cmd := exec.Command(testTspackBinary, "inspect", "--run", "dev", "--root", root, "--run-ready-timeout", "1")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_INSPECT_RUN_READY_TIMEOUT") {
		t.Fatalf("expected inspect run timeout: %v\n%s", err, string(b))
	}

	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","exit.js"],url:"http://127.0.0.1:5234",ready:{kind:"http",path:"/"}}]}]}`)
	cmd = exec.Command(testTspackBinary, "inspect", "--run", "dev", "--root", root, "--run-ready-timeout", "2")
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
	frontend := testManifestFrontendBridgeDir(t)
	bridge := filepath.Join(frontend, "inspect-cli.js")
	stub := `#!/usr/bin/env node
console.error("bridge-failed");
process.exit(7);`
	_ = os.WriteFile(bridge, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(bridge) })
	cmd := exec.Command(testTspackBinary, "inspect", "--run", "dev", "--root", root, "--json")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "bridge-failed") || !strings.Contains(string(out), `Stopped run target "dev".`) {
		t.Fatalf("expected bridge failure with shutdown: %v\n%s", err, string(out))
	}
	if runtime.GOOS == "windows" {
		return
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
	_ = writeNodeBackedExecutable(t, filepath.Join(root, "node_modules", ".bin", "dev-server"), fmt.Sprintf(`#!/usr/bin/env node
require(%q);
`, filepath.Join(root, "server.js")))
	argsPath := filepath.Join(root, "bridge-args.json")
	frontend := testManifestFrontendBridgeDir(t)
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
	cmd := exec.Command(testTspackBinary, "inspect", "--run", "dev", "--root", root, "--json", "--out", outPath, "--text", textPath, "--selector", "#root", "--point", "320,148", "--viewport", "1024x768", "--cdp", "http://127.0.0.1:9222", "--host-path", "/tmp/host")
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
	var argsList []string
	if err := json.Unmarshal(argsRaw, &argsList); err != nil {
		t.Fatalf("invalid bridge args json: %v\n%s", err, string(argsRaw))
	}
	for _, expected := range []string{"--out", outPath, "--text", textPath, "--selector", "#root", "--point", "320,148", "--viewport", "1024x768", "--cdp", "http://127.0.0.1:9222", "--host-path", "/tmp/host"} {
		if !containsString(argsList, expected) {
			t.Fatalf("missing bridge passthrough %q in %#v", expected, argsList)
		}
	}
	if !containsString(argsList, fmt.Sprintf("http://127.0.0.1:%d", port)) {
		t.Fatalf("missing resolved run url in bridge args: %#v", argsList)
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
	cmd = exec.Command(testTspackBinary, "inspect", "dev", "--root", root)
	cmd.Dir = repo
	bad, runErr := cmd.CombinedOutput()
	if runErr == nil || !strings.Contains(string(bad), "TSPACK_INSPECT_RUN_TARGET_NOT_FOUND") {
		t.Fatalf("expected no npm script inference failure: %v\n%s", runErr, string(bad))
	}
}

func TestCLIInspectBridgeMissing(t *testing.T) {
	repo := filepath.Join("..", "..")
	bridges := []string{
		filepath.Join(repo, "manifest-frontend", "dist", "src", "inspect-cli.js"),
		filepath.Join(repo, "manifest-frontend", "dist", "inspect-cli.js"),
	}
	backups := map[string]string{}
	for _, bridge := range bridges {
		backup := bridge + ".bak-test"
		if _, err := os.Stat(bridge); err == nil {
			if renameErr := os.Rename(bridge, backup); renameErr != nil {
				t.Fatalf("backup inspect bridge: %v", renameErr)
			}
			backups[bridge] = backup
		}
	}
	t.Cleanup(func() {
		for bridge, backup := range backups {
			_ = os.Rename(backup, bridge)
		}
	})

	cmd := exec.Command(testTspackBinary, "inspect", "http://example.test")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_INSPECT_BRIDGE_MISSING") {
		t.Fatalf("expected bridge missing: %v\n%s", err, string(b))
	}
}

func TestHelpMarksInspectExperimental(t *testing.T) {
	repo := filepath.Join("..", "..")
	cmd := exec.Command(testTspackBinary, "help")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("help failed: %v\n%s", err, string(b))
	}
	if !strings.Contains(string(b), "tspack inspect <url> [experimental]") {
		t.Fatalf("inspect help not marked experimental:\n%s", string(b))
	}
}

func TestInspectHelpDoesNotRequireBridge(t *testing.T) {
	repo := filepath.Join("..", "..")
	cmd := exec.Command(testTspackBinary, "inspect", "--help")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect --help failed: %v\n%s", err, string(b))
	}
	text := string(b)
	if strings.Contains(text, "TSPACK_INSPECT_BRIDGE_MISSING") {
		t.Fatalf("inspect --help should not require bridge:\n%s", text)
	}
	if !strings.Contains(text, "tspack inspect <url> [experimental]") {
		t.Fatalf("inspect subcommand help missing usage:\n%s", text)
	}
}

func writeRunFrontendStub(t *testing.T, irJSON string) {
	t.Helper()
	frontend := testManifestFrontendBridgeDir(t)
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := "#!/usr/bin/env node\nconst out={ok:true,ir:" + irJSON + ",diagnostics:[]};process.stdout.write(JSON.stringify(out));"
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })
}

func TestCLIRunOnceSelectionAndErrors(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	devPort := reservePort(t)
	apiPort := reservePort(t)
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	server := `const http=require('http'); const p=Number(process.env.PORT||process.argv[2]||5173); http.createServer((_,res)=>{res.statusCode=200;res.end('ok')}).listen(p,'127.0.0.1'); setInterval(()=>{},1000);`
	_ = os.WriteFile(filepath.Join(root, "server.js"), []byte(server), 0o644)

	writeRunFrontendStub(t, fmt.Sprintf(`{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"system",command:["node","server.js","%d"],url:"http://127.0.0.1:%d",ready:{kind:"http",path:"/"}},{name:"api",runtime:"system",command:["node","server.js","%d"],url:"http://127.0.0.1:%d",ready:{kind:"http",path:"/"}}]}]}`, devPort, devPort, apiPort, apiPort))

	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--once")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), fmt.Sprintf("Ready: http://127.0.0.1:%d", devPort)) {
		t.Fatalf("run dev failed: %v\n%s", err, string(b))
	}
	cmd = exec.Command(testTspackBinary, "run", "api", "--root", root, "--once")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(b), fmt.Sprintf("Ready: http://127.0.0.1:%d", apiPort)) {
		t.Fatalf("run api failed: %v\n%s", err, string(b))
	}
	cmd = exec.Command(testTspackBinary, "run", "nope", "--root", root, "--once")
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
	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--ready-timeout", "1", "--once")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_RUN_READY_TIMEOUT") {
		t.Fatalf("expected timeout: %v\n%s", err, string(b))
	}
	cmd = exec.Command(testTspackBinary, "run", "--root", root, "--ready-timeout", "0", "--once")
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
	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--once")
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
	if runtime.GOOS == "windows" {
		t.Skip("Windows coverage uses direct command-resolution and PATH tests; .cmd child teardown is process-model-specific")
	}
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	pathBin := t.TempDir()
	port := reservePort(t)
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, "node_modules", ".bin"), 0o755)
	markerPath := filepath.Join(root, "which-bin.txt")
	localServer := fmt.Sprintf(`const fs=require('fs'); const http=require('http'); fs.writeFileSync(%q,'local-bin'); http.createServer((_,res)=>{res.statusCode=200;res.end('ok')}).listen(%d,'127.0.0.1'); setInterval(()=>{},1000);`, markerPath, port)
	_ = os.WriteFile(filepath.Join(root, "local-server.js"), []byte(localServer), 0o644)
	localPath := filepath.Join(root, "node_modules", ".bin", "fake-dev-server")
	pathToolPath := filepath.Join(pathBin, "fake-dev-server")
	local := fmt.Sprintf("#!/bin/sh\nexec node %q\n", filepath.Join(root, "local-server.js"))
	pathScript := fmt.Sprintf("#!/bin/sh\necho path-bin > %q\nexit 1\n", markerPath)
	_ = os.WriteFile(localPath, []byte(local), 0o755)
	_ = os.WriteFile(pathToolPath, []byte(pathScript), 0o755)
	writeRunFrontendStub(t, fmt.Sprintf(`{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"dev",runtime:"node",command:["fake-dev-server"],url:"http://127.0.0.1:%d",ready:{kind:"http",path:"/"}}]}]}`, port))
	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--once")
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

func TestResolveNodeLocalCommandUsesProjectToolBin(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	expected := filepath.Join(binDir, "vite")
	if runtime.GOOS == "windows" {
		expected += ".cmd"
	}
	if err := os.WriteFile(expected, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved := resolveNodeLocalCommand(root, []string{"vite", "--host", "127.0.0.1"})
	if len(resolved) != 3 {
		t.Fatalf("unexpected argv: %#v", resolved)
	}
	if resolved[0] != expected {
		t.Fatalf("expected %q, got %#v", expected, resolved)
	}
}

func TestBuildRunCommandEnvPrependsProjectToolBin(t *testing.T) {
	root := t.TempDir()
	env := prependProjectToolBinsToEnv([]string{"Path=C:\\Windows\\System32", "HOME=C:\\Users\\test"}, projectToolBinDirs(root))
	found := ""
	for _, entry := range env {
		if strings.HasPrefix(strings.ToLower(entry), "path=") {
			found = entry
			break
		}
	}
	if found == "" {
		t.Fatalf("missing path entry: %#v", env)
	}
	wantPrefix := "Path=" + filepath.Join(root, "node_modules", ".bin")
	if !strings.HasPrefix(found, wantPrefix) {
		t.Fatalf("path entry %q missing prefix %q", found, wantPrefix)
	}
	if !strings.Contains(found, "C:\\Windows\\System32") {
		t.Fatalf("path entry should preserve existing path: %q", found)
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
	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--once")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_RUN_TARGET_MISSING") {
		t.Fatalf("expected missing target: %v\n%s", err, string(b))
	}

	writeRunFrontendStub(t, `{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"api",runtime:"system",command:["node","server.js","5203"],url:"http://127.0.0.1:5203",ready:{kind:"http",path:"/"}}]}]} `)
	beforeManifest, _ := os.ReadFile(manifestPath)
	beforeLock, _ := os.ReadFile(lockPath)
	cmd = exec.Command(testTspackBinary, "run", "--root", root, "--once")
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
	cmd = exec.Command(testTspackBinary, "run", "--root", root, "--once")
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
	cmd := exec.Command(testTspackBinary, "run", "--root", root, "--once", "--ready-timeout", "2")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_RUN_PROCESS_EXITED_EARLY") {
		t.Fatalf("expected exited early: %v\n%s", err, string(b))
	}
}

func TestCLIRunFiniteTargetKillsLingeringChildProcess(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	port := reservePort(t)
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	child := `const http=require('http');
const port=Number(process.argv[2]);
http.createServer((_,res)=>{res.statusCode=200;res.end('child')}).listen(port,'127.0.0.1');
setInterval(()=>{},1000);
`
	parent := fmt.Sprintf(`const {spawn}=require('child_process');
const path=require('path');
spawn(process.execPath,[path.join(__dirname,'child.js'),String(%d)],{stdio:'ignore'});
setTimeout(()=>process.exit(0),300);
`, port)
	_ = os.WriteFile(filepath.Join(root, "child.js"), []byte(child), 0o644)
	_ = os.WriteFile(filepath.Join(root, "parent.js"), []byte(parent), 0o644)
	writeRunFrontendStub(t, fmt.Sprintf(`{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"build",runtime:"system",command:["node","parent.js"],url:"http://127.0.0.1:%d"}]}]}`, port))

	cmd := exec.Command(testTspackBinary, "run", "--root", root, "build")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("finite run failed: %v\n%s", err, string(b))
	}
	output := string(b)
	if !strings.Contains(output, "Waiting for: process exit") || !strings.Contains(output, "Completed: exit code 0") {
		t.Fatalf("finite run missing completion output:\n%s", output)
	}

	portEventuallyClosed(t, port, 3*time.Second)
}

func TestCLIRunOnceStopsServerBeforeReturning(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	port := reservePort(t)
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	server := `const http=require('http');
const port=Number(process.argv[2]);
http.createServer((_,res)=>{res.statusCode=200;res.end('ok')}).listen(port,'127.0.0.1',()=>{process.stdout.write('READY\n');});
setInterval(()=>{},1000);
`
	_ = os.WriteFile(filepath.Join(root, "server.js"), []byte(server), 0o644)
	writeRunFrontendStub(t, fmt.Sprintf(`{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"app",dependencies:[],targets:[],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{},runTargets:[{name:"preview",runtime:"system",command:["node","server.js","%d"],url:"http://127.0.0.1:%d",ready:{kind:"http",path:"/"}}]}]}`, port, port))

	cmd := exec.Command(testTspackBinary, "run", "--root", root, "preview", "--once", "--ready-timeout", "3")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("preview --once failed: %v\n%s", err, string(b))
	}
	if !strings.Contains(string(b), fmt.Sprintf("Ready: http://127.0.0.1:%d", port)) {
		t.Fatalf("preview --once missing readiness output:\n%s", string(b))
	}

	portEventuallyClosed(t, port, 3*time.Second)
}

func TestTemplateBuildTargetsAreFinite(t *testing.T) {
	repo := filepath.Join("..", "..")
	reactTemplate := filepath.Join(repo, "internal", "templates", "builtin", "react", "files", "manifest.tsx.tmpl")
	reactLibraryTemplate := filepath.Join(repo, "internal", "templates", "builtin", "react-library", "files", "manifest.tsx.tmpl")

	reactData, err := os.ReadFile(reactTemplate)
	if err != nil {
		t.Fatal(err)
	}
	reactText := string(reactData)
	if strings.Contains(reactText, `name: "build"`) && strings.Contains(reactText, `ready: { kind: "stdout-match", pattern: "built in" }`) {
		t.Fatalf("react build target should not declare stdout readiness:\n%s", reactText)
	}

	reactLibraryData, err := os.ReadFile(reactLibraryTemplate)
	if err != nil {
		t.Fatal(err)
	}
	reactLibraryText := string(reactLibraryData)
	for _, pattern := range []string{
		`ready: { kind: "stdout-match", pattern: "built in" }`,
		`ready: { kind: "stdout-match", pattern: "TSFILE" }`,
		`ready: { kind: "stdout-match", pattern: "Files:" }`,
	} {
		if strings.Contains(reactLibraryText, pattern) {
			t.Fatalf("react-library finite target should not declare readiness %q", pattern)
		}
	}
}

func TestCheckFormatDerivedPathsAreScopedAndDeterministic(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"packages/app/src", ".tspack/store/metadata", "dist", "tspack-artifacts"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"manifest.tsx":                          "export default {}\n",
		"packages/app/src/bad.ts":               "export const bad=1\n",
		".tspack/store/metadata/generated.json": "{\"z\":1}",
		"dist/generated.js":                     "export const x=1",
		"tspack-artifacts/report.json":          "{\"z\":1}",
		"ts-lock.toml":                          "[lock]\nformat = 1\ntool = \"tspack\"\n\n[[target]]\npackage = \"app\"\nname = \"core\"\nentry = \"packages/app/src/bad.ts\"\nruntime = \"dist/generated.js\"\n",
	}
	for path, contents := range files {
		if err := os.WriteFile(filepath.Join(root, path), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := deriveCheckFormatPaths(root)
	want := []string{"manifest.tsx", "packages/app/src"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected derived paths: got %#v want %#v", got, want)
	}
}

func TestSanitizeTerminalOutputStripsANSIAndPreservesText(t *testing.T) {
	got := sanitizeTerminalOutput("\x1b[31mred\x1b[0m useful ┌text┐")
	if strings.Contains(got, "\x1b") {
		t.Fatalf("sanitized output still contains escape: %q", got)
	}
	if !strings.Contains(got, "red useful ┌text┐") {
		t.Fatalf("sanitized output lost useful text: %q", got)
	}
}

func TestCLIHelpIncludesFormatAndLint(t *testing.T) {
	repo := filepath.Join("..", "..")
	cmd := exec.Command(testTspackBinary, "help")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("help failed: %v\n%s", err, string(b))
	}
	out := string(b)
	if !strings.Contains(out, "tspack format") || !strings.Contains(out, "tspack lint") || !strings.Contains(out, "--format") {
		t.Fatalf("help missing format/lint: %s", out)
	}
}

func TestDefaultBiomeConfigContent(t *testing.T) {
	var config map[string]any
	if err := json.Unmarshal(defaultBiomeConfigBytes(), &config); err != nil {
		t.Fatalf("default Biome config must be valid JSON: %v", err)
	}

	assertNestedValue(t, config, true, "formatter", "enabled")
	scannerIgnores, ok := config["files"].(map[string]any)["experimentalScannerIgnores"].([]any)
	if !ok {
		t.Fatalf("default Biome config missing files.experimentalScannerIgnores: %#v", config["files"])
	}
	scannerIgnoreSet := map[string]bool{}
	for _, value := range scannerIgnores {
		scannerIgnoreSet[value.(string)] = true
	}
	for _, want := range []string{".tspack", "node_modules", "dist", "tspack-artifacts"} {
		if !scannerIgnoreSet[want] {
			t.Fatalf("default Biome config missing scanner ignore %q in %#v", want, scannerIgnores)
		}
	}
	includeValues, ok := config["files"].(map[string]any)["includes"].([]any)
	if !ok {
		t.Fatalf("default Biome config missing files.includes: %#v", config["files"])
	}
	includeSet := map[string]bool{}
	for _, value := range includeValues {
		includeSet[value.(string)] = true
	}
	for _, want := range []string{".tspack/**", "node_modules/**", "dist/**", "tspack-artifacts/**"} {
		exclusion := "!" + want
		if !includeSet[exclusion] {
			t.Fatalf("default Biome config missing exclusion %q in %#v", exclusion, includeValues)
		}
	}
	assertNestedValue(t, config, "tab", "formatter", "indentStyle")
	assertNestedValue(t, config, float64(100), "formatter", "lineWidth")
	assertNestedValue(t, config, "on", "assist", "actions", "source", "organizeImports")
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
			assertNestedValue(t, config, "on", "assist", "actions", "source", "organizeImports")
			assertStringArrayContains(t, config, ".tspack", "files", "experimentalScannerIgnores")
			assertStringArrayContains(t, config, "!.tspack/**", "files", "includes")
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
	frontend := testManifestFrontendBridgeDir(t)
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
	_ = writeNodeBackedExecutable(t, path, backend)
}

func writeBiomeCaptureBackend(t *testing.T, path string, marker string) {
	t.Helper()
	backend := fmt.Sprintf(`#!/usr/bin/env node
const fs = require('fs');
const capture = process.env.TSPACK_CAPTURE;
fs.writeFileSync(capture, JSON.stringify({ argv: process.argv.slice(2), cwd: process.cwd() }));
process.stdout.write(%q + '\n');
`, marker)
	_ = writeNodeBackedExecutable(t, path, backend)
}

func runTSPackWithBiomeCapture(t *testing.T, repo string, root string, args []string, capture string, pathDir string) string {
	t.Helper()
	_ = os.Remove(capture)
	cmd := exec.Command(testTspackBinary, args...)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "PATH="+pathDir+string(os.PathListSeparator)+os.Getenv("PATH"), "TSPACK_CAPTURE="+capture)
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tspack failed: %v\n%s", err, string(b))
	}
	return string(b)
}

func writeBiomeExitBackend(t *testing.T, path string, capture string, exitCode int, stdoutText string, stderrText string) {
	t.Helper()
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
	_ = writeNodeBackedExecutable(t, path, backend)
}

func runTSPackForBiome(t *testing.T, repo string, root string, args []string, pathDir string) (string, error) {
	t.Helper()
	cmd := exec.Command(testTspackBinary, args...)
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
	cmd := exec.Command(testTspackBinary, args...)
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
	copyFile(t, nodePath, linkPath)
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

func assertStringArrayContains(t *testing.T, root map[string]any, want string, path ...string) {
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
	values, ok := current.([]any)
	if !ok {
		t.Fatalf("expected array at %s, got %#v", strings.Join(path, "."), current)
	}
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("expected %s to contain %q, got %#v", strings.Join(path, "."), want, values)
}

func containsExactArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
