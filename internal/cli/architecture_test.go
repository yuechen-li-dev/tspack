package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
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

func TestProcessTerminationBelongsToExecutableBootstrap(t *testing.T) {
	cliDirectory := filepath.Join(".")
	err := filepath.WalkDir(cliDirectory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if countSelectorCalls(t, path, "os", "Exit") != 0 {
			t.Errorf("CLI production code must return status instead of calling os.Exit: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	bootstrap := filepath.Join("..", "..", "cmd", "tspack", "main.go")
	if calls := countSelectorCalls(t, bootstrap, "os", "Exit"); calls != 1 {
		t.Fatalf("cmd/tspack bootstrap must own exactly one os.Exit call; got %d", calls)
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
		"add", "adopt", "artifact", "audit", "bench", "build", "check", "compat",
		"doctor", "doom", "format", "how", "init", "inspect", "lint",
		"materialize-tree", "migrate", "npm", "outdated", "pack", "remove",
		"run", "scenario", "skyrim", "sync", "test", "update", "why",
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

func TestLifecycleCommandsHaveDedicatedHandlers(t *testing.T) {
	want := map[string]string{
		"add":      "runAddCommand",
		"check":    "runCheckCommand",
		"remove":   "runRemoveCommand",
		"update":   "runUpdateCommand",
		"sync":     "runSyncCommand",
		"pack":     "runPackCommand",
		"why":      "runWhyCommand",
		"outdated": "runOutdatedCommand",
	}
	for command, handlerName := range want {
		handler := commandHandlers[command]
		function := runtime.FuncForPC(reflect.ValueOf(handler).Pointer())
		if function == nil || !strings.HasSuffix(function.Name(), "."+handlerName) {
			t.Errorf("%s must use dedicated handler %s; got %v", command, handlerName, function)
		}
	}
}

func TestLifecycleApplicationFacadeDoesNotImportPresentation(t *testing.T) {
	path := filepath.Join("..", "project", "lifecycle_operations.go")
	imports := parseImports(t, path)
	for _, forbidden := range []string{"fmt", "os", "encoding/json", "github.com/yuechen-li-dev/tspack/internal/cli"} {
		if imports[forbidden] {
			t.Errorf("application lifecycle facade imports presentation dependency %s", forbidden)
		}
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

func countSelectorCalls(t *testing.T, path string, packageName string, functionName string) int {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse calls for %s: %v", path, err)
	}
	count := 0
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != functionName {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if ok && identifier.Name == packageName {
			count++
		}
		return true
	})
	return count
}
