package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitHelpIncludesCommand(t *testing.T) {
	repo := filepath.Join("..", "..")
	cmd := exec.Command("go", "run", "./cmd/tspack", "help")
	cmd.Dir = repo
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("help failed: %v\n%s", err, string(b))
	}
	if !strings.Contains(string(b), "tspack init") {
		t.Fatalf("help missing init: %s", string(b))
	}
}

func TestInitValidationAndWriteFlow(t *testing.T) {
	repo := filepath.Join("..", "..")

	t.Run("missing kind", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command("go", "run", "./cmd/tspack", "init", "--root", root, "--name", "acme-demo")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(b), "TSPACK_INIT_KIND_REQUIRED") {
			t.Fatalf("expected missing kind diagnostic: %v\n%s", err, string(b))
		}
	})

	t.Run("invalid kind", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command("go", "run", "./cmd/tspack", "init", "--root", root, "--kind", "service", "--name", "acme-demo")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(b), "TSPACK_INIT_INVALID_KIND") {
			t.Fatalf("expected invalid kind diagnostic: %v\n%s", err, string(b))
		}
	})

	t.Run("missing name", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command("go", "run", "./cmd/tspack", "init", "--root", root, "--kind", "library")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(b), "TSPACK_INIT_NAME_REQUIRED") {
			t.Fatalf("expected missing name diagnostic: %v\n%s", err, string(b))
		}
	})

	t.Run("invalid package name", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command("go", "run", "./cmd/tspack", "init", "--root", root, "--kind", "library", "--name", "Bad Name")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(b), "TSPACK_INIT_INVALID_NAME") {
			t.Fatalf("expected invalid name diagnostic: %v\n%s", err, string(b))
		}
	})

	t.Run("invalid version", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command("go", "run", "./cmd/tspack", "init", "--root", root, "--kind", "library", "--name", "acme-demo", "--version", "1")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(b), "TSPACK_INIT_INVALID_VERSION") {
			t.Fatalf("expected invalid version diagnostic: %v\n%s", err, string(b))
		}
	})

	t.Run("library write and force", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command("go", "run", "./cmd/tspack", "init", "--root", root, "--kind", "library", "--name", "@acme/widgets")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("init library failed: %v\n%s", err, string(b))
		}
		manifestPath := filepath.Join(root, "manifest.tsx")
		srcPath := filepath.Join(root, "src", "index.ts")
		if _, err := os.Stat(manifestPath); err != nil {
			t.Fatalf("missing manifest: %v", err)
		}
		if _, err := os.Stat(srcPath); err != nil {
			t.Fatalf("missing src index: %v", err)
		}
		text, _ := os.ReadFile(manifestPath)
		m := string(text)
		for _, want := range []string{"from \"tspack/manifest\"", "<Workspace", "kind=\"library\"", "name: \"core\"", "<Publish"} {
			if !strings.Contains(m, want) {
				t.Fatalf("manifest missing %q\n%s", want, m)
			}
		}
		if _, err := os.Stat(filepath.Join(root, "ts-lock.toml")); !os.IsNotExist(err) {
			t.Fatalf("ts-lock should not be created")
		}
		if _, err := os.Stat(filepath.Join(root, "node_modules")); !os.IsNotExist(err) {
			t.Fatalf("node_modules should not be created")
		}

		cmd = exec.Command("go", "run", "./cmd/tspack", "init", "--root", root, "--kind", "library", "--name", "@acme/widgets")
		cmd.Dir = repo
		b, err = cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(b), "TSPACK_INIT_FILE_EXISTS") {
			t.Fatalf("expected file exists diagnostic: %v\n%s", err, string(b))
		}

		_ = os.WriteFile(srcPath, []byte("custom\n"), 0o644)
		cmd = exec.Command("go", "run", "./cmd/tspack", "init", "--root", root, "--kind", "library", "--name", "@acme/widgets", "--force")
		cmd.Dir = repo
		b, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("init force failed: %v\n%s", err, string(b))
		}
		text, _ = os.ReadFile(srcPath)
		if strings.Contains(string(text), "custom") {
			t.Fatalf("force should overwrite generated files")
		}
	})

	t.Run("app write and dry run", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command("go", "run", "./cmd/tspack", "init", "--root", root, "--kind", "app", "--name", "acme-demo", "--dry-run")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("dry run failed: %v\n%s", err, string(b))
		}
		if _, err := os.Stat(filepath.Join(root, "manifest.tsx")); !os.IsNotExist(err) {
			t.Fatalf("dry-run should not write files")
		}
		if !strings.Contains(string(b), "Would write") || !strings.Contains(string(b), "src/main.ts") {
			t.Fatalf("dry-run output missing file list: %s", string(b))
		}

		cmd = exec.Command("go", "run", "./cmd/tspack", "init", "--root", root, "--kind", "app", "--name", "acme-demo")
		cmd.Dir = repo
		b, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("init app failed: %v\n%s", err, string(b))
		}
		manifestPath := filepath.Join(root, "manifest.tsx")
		text, _ := os.ReadFile(manifestPath)
		m := string(text)
		for _, want := range []string{"kind=\"app\"", "name: \"app\"", "missingTypes: \"ignore\"", "declarations: \"optional\""} {
			if !strings.Contains(m, want) {
				t.Fatalf("app manifest missing %q\n%s", want, m)
			}
		}
	})
}
