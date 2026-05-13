package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
	lock := "[lock]\nformat=1\ntool=\"tspack\"\n[[package]]\nid=\"npm:vue@3.4.0\"\nname=\"vue\"\nversion=\"3.4.0\"\nsource=\"npm\"\nhash=\"h\"\n[[package]]\nid=\"npm:dep-a@1.0.0\"\nname=\"dep-a\"\nversion=\"1.0.0\"\nsource=\"npm\"\nhash=\"h\"\n[[package]]\nid=\"npm:left-pad@1.2.0\"\nname=\"left-pad\"\nversion=\"1.2.0\"\nsource=\"npm\"\nhash=\"h\"\n[[edge]]\nfrom=\"app:target:vue\"\nto=\"npm:vue@3.4.0\"\nkind=\"peer\"\noptional=true\n[[edge]]\nfrom=\"npm:dep-a@1.0.0\"\nto=\"npm:left-pad@1.2.0\"\nkind=\"runtime\"\n[[target]]\npackage=\"app\"\nname=\"core\"\nexport=\".\"\nentry=\"src/index.ts\"\nruntime=\"src/index.ts\"\ntypes=\"dist/index.d.ts\"\n[[target]]\npackage=\"app\"\nname=\"react\"\nexport=\"./react\"\nentry=\"src/react.ts\"\nruntime=\"src/react.ts\"\ntypes=\"dist/react.d.ts\"\n[[target]]\npackage=\"app\"\nname=\"vue\"\nexport=\"./vue\"\nentry=\"src/vue.ts\"\nruntime=\"src/vue.ts\"\ntypes=\"dist/vue.d.ts\"\n"
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
	for _, cmd := range []string{"check", "update", "sync", "pack", "why", "--version", "help"} {
		if !strings.Contains(text, cmd) {
			t.Fatalf("help missing %s: %s", cmd, text)
		}
	}
	for _, unsupported := range []string{"build", "dev", "publish", "add", "remove"} {
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
