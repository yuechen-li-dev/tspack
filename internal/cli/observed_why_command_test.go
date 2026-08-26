package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIObservedNPMWhy(t *testing.T) {
	repo := repoRoot(t)
	root := copyTestDir(t, filepath.Join(repo, "examples", "incremental-existing-react"))

	direct := runTSPack(t, repo, "why", "react", "--root", root)
	if direct.err != nil {
		t.Fatalf("why react failed: %v\nstdout:\n%s\nstderr:\n%s", direct.err, direct.stdout, direct.stderr)
	}
	assertContains(t, direct.stdout, "source: observed npm package.json/package-lock")
	assertContains(t, direct.stdout, "package.json dependencies")
	assertContains(t, direct.stdout, "react ")
	assertContains(t, direct.stdout, "not a TSPack manifest dependency classification yet")

	dev := runTSPack(t, repo, "why", "@biomejs/biome", "--root", root)
	if dev.err != nil {
		t.Fatalf("why @biomejs/biome failed: %v\nstdout:\n%s\nstderr:\n%s", dev.err, dev.stdout, dev.stderr)
	}
	assertContains(t, dev.stdout, "package.json devDependencies")

	transitive := runTSPack(t, repo, "why", "@biomejs/cli-linux-x64", "--root", root)
	if transitive.err != nil {
		t.Fatalf("why esbuild failed: %v\nstdout:\n%s\nstderr:\n%s", transitive.err, transitive.stdout, transitive.stderr)
	}
	assertContains(t, transitive.stdout, "@biomejs/cli-linux-x64 is present in the observed npm lockfile")
	assertContains(t, transitive.stdout, "Chain:")
	assertContains(t, transitive.stdout, "@biomejs/biome")
	assertContains(t, transitive.stdout, "@biomejs/cli-linux-x64")

	missing := runTSPack(t, repo, "why", "definitely-not-a-real-m62c-package", "--root", root)
	if missing.err != nil {
		t.Fatalf("why missing package should be a helpful success: %v\nstdout:\n%s\nstderr:\n%s", missing.err, missing.stdout, missing.stderr)
	}
	assertContains(t, missing.stdout, "definitely-not-a-real-m62c-package was not found in package.json or package-lock.json")
}

func TestCLIObservedNPMWhyWithoutLockfile(t *testing.T) {
	repo := repoRoot(t)
	root := copyTestDir(t, filepath.Join(repo, "examples", "incremental-existing-react"))
	if err := os.Remove(filepath.Join(root, "package-lock.json")); err != nil {
		t.Fatal(err)
	}

	direct := runTSPack(t, repo, "why", "react", "--root", root)
	if direct.err != nil {
		t.Fatalf("why direct without lockfile failed: %v\nstdout:\n%s\nstderr:\n%s", direct.err, direct.stdout, direct.stderr)
	}
	assertContains(t, direct.stdout, "react is present in the observed npm project")
	assertContains(t, direct.stdout, "package.json dependencies")

	transitive := runTSPack(t, repo, "why", "@biomejs/cli-linux-x64", "--root", root)
	if transitive.err != nil {
		t.Fatalf("why transitive without lockfile should be a helpful success: %v\nstdout:\n%s\nstderr:\n%s", transitive.err, transitive.stdout, transitive.stderr)
	}
	assertContains(t, transitive.stdout, "No package-lock.json is available")
	assertContains(t, transitive.stdout, "tspack npm install")
}

func TestCLIObservedNPMWhyAfterInitAlongsideAndNoWrites(t *testing.T) {
	repo := repoRoot(t)
	root := copyTestDir(t, filepath.Join(repo, "examples", "incremental-existing-react"))
	for _, relativePath := range []string{"manifest.tsx", "tsconfig.tspack.json", "tspack-env.d.ts", ".tspack", ".vscode"} {
		if err := os.RemoveAll(filepath.Join(root, relativePath)); err != nil {
			t.Fatalf("remove %s: %v", relativePath, err)
		}
	}

	initResult := runTSPack(t, repo, "init", "--alongside", "--root", root)
	if initResult.err != nil {
		t.Fatalf("init alongside failed: %v\nstdout:\n%s\nstderr:\n%s", initResult.err, initResult.stdout, initResult.stderr)
	}

	whyResult := runTSPack(t, repo, "why", "react", "--root", root)
	if whyResult.err != nil {
		t.Fatalf("why after init alongside failed: %v\nstdout:\n%s\nstderr:\n%s", whyResult.err, whyResult.stdout, whyResult.stderr)
	}
	assertContains(t, whyResult.stdout, "source: observed npm package.json/package-lock")
	assertContains(t, whyResult.stdout, "react is present in the observed npm project")

	for _, forbidden := range []string{"ts-lock.toml", "node_modules"} {
		if _, err := os.Stat(filepath.Join(root, forbidden)); err == nil {
			t.Fatalf("why/init alongside should not create %s", forbidden)
		}
	}
}

type tspackRunResult struct {
	stdout string
	stderr string
	err    error
}

func runTSPack(t *testing.T, repo string, args ...string) tspackRunResult {
	t.Helper()
	cmd := exec.Command(testTspackBinary, args...)
	cmd.Dir = repo
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return tspackRunResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func copyTestDir(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		copyTestPath(t, filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name()))
	}
	return dst
}

func copyTestPath(t *testing.T, src string, dst string) {
	t.Helper()
	info, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	if info.IsDir() {
		if filepath.Base(src) == "node_modules" {
			return
		}
		if err := os.MkdirAll(dst, info.Mode()); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			copyTestPath(t, filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name()))
		}
		return
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, info.Mode()); err != nil {
		t.Fatal(err)
	}
}

func assertContains(t *testing.T, text string, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("expected output to contain %q\noutput:\n%s", want, text)
	}
}
