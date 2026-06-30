package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckReportsNodeMissingWithMiseGuidance(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "manifest.tsx"), "export default {}\n")

	frontendDir := t.TempDir()
	frontendCLI := filepath.Join(frontendDir, "cli.js")
	writeFileMode(t, frontendCLI, "#!/usr/bin/env node\nprocess.stdout.write('{}')\n", 0o755)

	env := append(os.Environ(),
		"PATH="+t.TempDir(),
		"TSPACK_MANIFEST_FRONTEND="+frontendCLI,
	)
	out, err := runTspackWithEnv(bin, env, "check", "--root", root)
	if err == nil {
		t.Fatalf("expected check to fail without node:\n%s", out)
	}
	for _, want := range []string{
		"TSPACK_NODE_NOT_FOUND",
		"TSPack does not manage JavaScript runtime versions",
		"https://mise.jdx.dev/",
		"mise use node@lts",
		"mise install",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in node diagnostic:\n%s", want, out)
		}
	}
}
