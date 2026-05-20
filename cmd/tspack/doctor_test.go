package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifestStub(t *testing.T, repo string) {
	t.Helper()
	frontend := filepath.Join(repo, "manifest-frontend", "dist", "src")
	_ = os.MkdirAll(frontend, 0o755)
	cliPath := filepath.Join(frontend, "cli.js")
	stub := `#!/usr/bin/env node
const out={ok:true,ir:{format:1,workspace:{name:"ws"},packages:[{name:"app",version:"1.0.0",kind:"library",dependencies:[],targets:[{name:"core",export:".",entry:"src/index.ts",runtime:"dist/index.js",types:"dist/index.d.ts",deps:[],peers:[]}],tools:[],boundaries:[],publish:{include:["dist/**"],exclude:[]},policies:{types:{},boundaries:{}},runTargets:[{name:"dev",runtime:"node",url:"http://127.0.0.1:5173",command:["node","server.js"]}]}]},diagnostics:[]};
process.stdout.write(JSON.stringify(out));`
	_ = os.WriteFile(cliPath, []byte(stub), 0o755)
	t.Cleanup(func() { _ = os.Remove(cliPath) })
}

func TestDoctorHelpAndJson(t *testing.T) {
	repo := filepath.Join("..", "..")
	writeManifestStub(t, repo)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "ts-lock.toml"), []byte("[lock]\nformat=1\n"), 0o644)

	cmd := exec.Command("go", "run", "./cmd/tspack", "help")
	cmd.Dir = repo
	b, _ := cmd.CombinedOutput()
	if !strings.Contains(string(b), "tspack doctor") {
		t.Fatalf("help missing doctor")
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "doctor", "--root", root)
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor failed: %v\n%s", err, string(b))
	}
	for _, s := range []string{"Project", "Format/Lint", "Run", "Inspect", "Summary"} {
		if !strings.Contains(string(b), s) {
			t.Fatalf("missing section %s", s)
		}
	}

	cmd = exec.Command("go", "run", "./cmd/tspack", "doctor", "--root", root, "--json")
	cmd.Dir = repo
	b, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor json failed: %v\n%s", err, string(b))
	}
	var parsed map[string]any
	if e := json.Unmarshal(b, &parsed); e != nil {
		t.Fatalf("invalid json: %v", e)
	}
	if !strings.HasSuffix(string(b), "\n") {
		t.Fatalf("doctor json output must end with newline")
	}
	if !strings.Contains(string(b), "\n  \"root\"") {
		t.Fatalf("doctor json must use two-space indentation")
	}
}

func TestDoctorInvalidScope(t *testing.T) {
	repo := filepath.Join("..", "..")
	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "badscope")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "TSPACK_DOCTOR_INVALID_SCOPE") {
		t.Fatalf("expected invalid scope: %v\n%s", err, string(b))
	}
}

func TestDoctorFormatMissingBiomeExitsNonzero(t *testing.T) {
	repo := filepath.Join("..", "..")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("export default {}\n"), 0o644)
	cmd := exec.Command("go", "run", "./cmd/tspack", "doctor", "format", "--root", root)
	cmd.Dir = repo
	cmd.Env = []string{"PATH=/definitely-missing"}
	b, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected nonzero exit for missing biome: %s", string(b))
	}
}
