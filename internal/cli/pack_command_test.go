package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/cli/clitest"
)

func TestCLIPackSmokeAndDryRun(t *testing.T) {
	repo := filepath.Join("..", "..")
	frontend := testManifestFrontendBridgeDir(t)
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
	packed := clitest.RunInDir(t, repo, testTspackBinary, "pack", "--root", root, "--out", outDir)
	clitest.AssertExit(t, packed, 0)
	if !strings.Contains(packed.Stdout, "packed app@1.0.0") {
		t.Fatalf("expected packed output, got: %s", packed)
	}
	if _, err := os.Stat(filepath.Join(outDir, "app-1.0.0.tgz")); err != nil {
		t.Fatalf("expected artifact: %v", err)
	}

	dryDir := filepath.Join(root, "dry")
	dryRun := clitest.RunInDir(t, repo, testTspackBinary, "pack", "--root", root, "--out", dryDir, "--dry-run")
	clitest.AssertExit(t, dryRun, 0)
	if _, err := os.Stat(filepath.Join(dryDir, "app-1.0.0.tgz")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote artifact")
	}
	if !strings.Contains(dryRun.Stdout, "package/dist/index.js") {
		t.Fatalf("expected preview output, got: %s", dryRun)
	}

	help := clitest.RunInDir(t, repo, testTspackBinary, "help")
	clitest.AssertExit(t, help, 0)
	if !strings.Contains(help.Stdout, "tspack pack") {
		t.Fatalf("help missing pack: %s", help)
	}
}
