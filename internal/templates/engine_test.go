package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinStaticLoadsAndRenders(t *testing.T) {
	tmpl, err := LoadBuiltin("static")
	if err != nil {
		t.Fatalf("load static: %v", err)
	}
	if tmpl.Name != "static" || !contains(tmpl.Concepts, "browser.static") {
		t.Fatalf("unexpected static template: %#v", tmpl)
	}
	root := t.TempDir()
	values, err := tmpl.ResolveValues(map[string]string{"projectName": "Hello", "packageName": "hello", "runtime": "bun"})
	if err != nil {
		t.Fatalf("resolve values: %v", err)
	}
	if _, err := tmpl.Apply(ApplyOptions{Destination: root, Values: values}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	manifest, err := os.ReadFile(filepath.Join(root, "manifest.tsx"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.Contains(string(manifest), `name="Hello" runtime="bun"`) {
		t.Fatalf("manifest not rendered:\n%s", string(manifest))
	}
}

func TestBuiltinReactLoadsAndRenders(t *testing.T) {
	tmpl, err := LoadBuiltin("react")
	if err != nil {
		t.Fatalf("load react: %v", err)
	}
	for _, concept := range []string{"react.app", "vite.app", "browser.spa"} {
		if !contains(tmpl.Concepts, concept) {
			t.Fatalf("react template missing concept %q: %#v", concept, tmpl.Concepts)
		}
	}

	root := t.TempDir()
	values, err := tmpl.ResolveValues(map[string]string{"projectName": "Hello React", "packageName": "hello-react", "runtime": "bun"})
	if err != nil {
		t.Fatalf("resolve values: %v", err)
	}
	if _, err := tmpl.Apply(ApplyOptions{Destination: root, Values: values}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	wantFiles := []string{
		"manifest.tsx",
		"tsconfig.tspack.json",
		"tsconfig.json",
		"biome.json",
		"vite.config.ts",
		"package.json",
		"index.html",
		"src/main.tsx",
		"src/App.tsx",
		"src/style.css",
		"README.md",
	}
	for _, rel := range wantFiles {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("missing generated file %s: %v", rel, err)
		}
	}

	manifest, err := os.ReadFile(filepath.Join(root, "manifest.tsx"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifestText := string(manifest)
	for _, want := range []string{`name="Hello React" runtime="bun"`, `name="hello-react"`, `react-dom`, `@vitejs/plugin-react`, `strategy: "manual"`} {
		if !strings.Contains(manifestText, want) {
			t.Fatalf("manifest missing %q:\n%s", want, manifestText)
		}
	}

	if _, err := tmpl.ResolveValues(map[string]string{"projectName": "bad", "packageName": "bad", "runtime": "npm"}); err == nil || !strings.Contains(err.Error(), "TSPACK_TEMPLATE_VARIABLE_INVALID") {
		t.Fatalf("expected invalid runtime error, got %v", err)
	}
}

func TestLocalTemplateSafetyAndOverwrite(t *testing.T) {
	templateRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(templateRoot, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	metadata := `format = 1
name = "local"
description = "Local template"
kind = "app"
concepts = ["custom.app"]
[variables.projectName]
default = "demo"
[[files]]
from = "files/hello.txt.tmpl"
to = "hello.txt"
`
	if err := os.WriteFile(filepath.Join(templateRoot, MetadataFile), []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(templateRoot, "files", "hello.txt.tmpl"), []byte("hello {{projectName}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tmpl, err := LoadLocal(templateRoot)
	if err != nil {
		t.Fatalf("load local: %v", err)
	}
	root := t.TempDir()
	values, err := tmpl.ResolveValues(map[string]string{"projectName": "world"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmpl.Apply(ApplyOptions{Destination: root, Values: values}); err != nil {
		t.Fatalf("apply local: %v", err)
	}
	if _, err := tmpl.Apply(ApplyOptions{Destination: root, Values: values}); err == nil || !strings.Contains(err.Error(), "TSPACK_TEMPLATE_FILE_EXISTS") {
		t.Fatalf("expected exists error, got %v", err)
	}
}

func TestInvalidConceptAndPathTraversalFail(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	metadata := `format = 1
name = "bad"
description = "Bad"
kind = "app"
concepts = ["bad concept"]
[[files]]
from = "files/a.txt"
to = "../a.txt"
`
	_ = os.WriteFile(filepath.Join(root, MetadataFile), []byte(metadata), 0o644)
	_ = os.WriteFile(filepath.Join(root, "files", "a.txt"), []byte("x"), 0o644)
	_, err := LoadLocal(root)
	if err == nil || !strings.Contains(err.Error(), "TSPACK_TEMPLATE_INVALID") {
		t.Fatalf("expected invalid template, got %v", err)
	}
}
