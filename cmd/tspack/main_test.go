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
