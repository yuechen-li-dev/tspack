package cli

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestExecutableContainsBootstrapOnly(t *testing.T) {
	commandDirectory := filepath.Join("..", "..", "cmd", "tspack")
	entries, err := os.ReadDir(commandDirectory)
	if err != nil {
		t.Fatal(err)
	}
	goFiles := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".go" {
			goFiles = append(goFiles, entry.Name())
		}
	}
	if len(goFiles) != 1 || goFiles[0] != "main.go" {
		t.Fatalf("cmd/tspack must contain only bootstrap main.go; found %v", goFiles)
	}

	imports := parseImports(t, filepath.Join(commandDirectory, "main.go"))
	if !imports["github.com/yuechen-li-dev/tspack/internal/cli"] {
		t.Fatal("cmd/tspack/main.go must delegate to internal/cli")
	}
}

func TestCorePackagesDoNotDependOnPresentationOrIntegrations(t *testing.T) {
	internalRoot := filepath.Join("..", "..", "internal")
	err := filepath.WalkDir(internalRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			relative, err := filepath.Rel(internalRoot, path)
			if err != nil {
				return err
			}
			first := strings.Split(filepath.ToSlash(relative), "/")[0]
			if first == "cli" || first == "integrations" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		for imported := range parseImports(t, path) {
			if strings.Contains(imported, "/internal/cli") || strings.Contains(imported, "/internal/integrations/") {
				t.Errorf("core package %s imports outward-facing package %s", path, imported)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestIntegrationsDoNotDependOnCLI(t *testing.T) {
	integrationRoot := filepath.Join("..", "..", "internal", "integrations")
	err := filepath.WalkDir(integrationRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		for imported := range parseImports(t, path) {
			if strings.Contains(imported, "/internal/cli") {
				t.Errorf("integration package %s imports CLI package %s", path, imported)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCommandRegistryIsExplicit(t *testing.T) {
	want := []string{
		"adopt", "artifact", "audit", "bench", "build", "check", "compat",
		"doctor", "doom", "format", "how", "init", "inspect", "lint",
		"materialize-tree", "migrate", "npm", "outdated", "pack", "run",
		"scenario", "skyrim", "sync", "test", "update", "why",
	}
	got := make([]string, 0, len(commandHandlers))
	for name := range commandHandlers {
		got = append(got, name)
	}
	sort.Strings(got)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("command registry changed without updating its architecture contract:\nwant %v\ngot  %v", want, got)
	}
}

func parseImports(t *testing.T, path string) map[string]bool {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse imports for %s: %v", path, err)
	}
	imports := map[string]bool{}
	for _, imported := range parsed.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Fatalf("unquote import in %s: %v", path, err)
		}
		imports[path] = true
	}
	return imports
}
