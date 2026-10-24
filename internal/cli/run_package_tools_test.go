package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

// This process test proves package tool lookup and inherited PATH even when the
// target deliberately runs from the workspace rather than its declaring package.
func TestRunTargetUsesDeclaringPackageToolsFromWorkspaceCwd(t *testing.T) {
	root := t.TempDir()
	packageRoot := filepath.Join(root, "client with spaces")
	bin := filepath.Join(packageRoot, "node_modules", ".bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "package-only-tool"
	content := "#!/bin/sh\nprintf 'package-tool:%s' \"$PWD\"\n"
	if runtime.GOOS == "windows" {
		name += ".cmd"
		content = "@echo off\r\necho package-tool:%CD%\r\n"
	}
	if err := os.WriteFile(filepath.Join(bin, name), []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	target := manifest.RunTarget{Name: "scoped", Runtime: "node", Command: []string{"package-only-tool"}}
	var stdout, stderr bytes.Buffer
	session, runErr := launchRunTargetInDir(root, root, target, &stdout, &stderr, runEnvOverlay{}, packageRoot)
	if runErr != nil {
		t.Fatalf("launch failed: %#v", runErr)
	}
	t.Cleanup(func() { _ = session.Stop() })
	if err := session.Wait(); err != nil {
		t.Fatalf("tool failed: %v; stderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "package-tool:"+root) {
		t.Fatalf("wrong tool or working directory: %q", stdout.String())
	}
	_, env, prepareErr := prepareRunTargetForInvocation(root, target, runEnvOverlay{}, packageRoot)
	if prepareErr != nil {
		t.Fatalf("prepare failed: %#v", prepareErr)
	}
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, "PATH") && strings.HasPrefix(value, bin+string(os.PathListSeparator)) {
			return
		}
	}
	t.Fatalf("package tool directory missing from child PATH: %#v", env)
}
