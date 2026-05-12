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
