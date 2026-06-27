package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/lockfile"
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
	manifestText := string(manifest)
	if !strings.Contains(manifestText, `name="Hello" runtime="bun"`) {
		t.Fatalf("manifest not rendered:\n%s", manifestText)
	}

	for _, want := range []string{
		`name: "app"`,
		`export: "."`,
		`entry: "src/main.ts"`,
		`runtime: "dist/main.js"`,
		`types: "dist/main.d.ts"`,
		`@biomejs/biome`,
		`category: "consumer-install"`,
	} {
		if !strings.Contains(manifestText, want) {
			t.Fatalf("static manifest target missing %q:\n%s", want, manifestText)
		}
	}

	generatedTarget := lockfile.Target{
		Package: "hello",
		Name:    "app",
		Export:  ".",
		Entry:   "src/main.ts",
		Runtime: "dist/main.js",
		Types:   "dist/main.d.ts",
	}
	lock := &lockfile.Lockfile{
		Lock:    lockfile.LockHeader{Format: lockfile.FormatVersion, Tool: lockfile.ToolName},
		Targets: []lockfile.Target{generatedTarget},
	}
	lockBytes, err := lockfile.Marshal(lock)
	if err != nil {
		t.Fatalf("marshal lockfile: %v", err)
	}
	packageJSON, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Fatalf("read package.json: %v", err)
	}
	assertNoLifecycleScripts(t, string(packageJSON))

	_, diagnostics := lockfile.Parse("ts-lock.toml", lockBytes)
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "TSPACK_LOCK_INVALID_TARGET" {
			t.Fatalf("static target should satisfy lock target contract: %#v", diagnostic)
		}
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
	for _, want := range []string{`name="Hello React" runtime="bun"`, `name="hello-react"`, `react-dom`, `@vitejs/plugin-react`, `@biomejs/biome`, `category: "consumer-install"`, `strategy: "manual"`} {
		if !strings.Contains(manifestText, want) {
			t.Fatalf("manifest missing %q:\n%s", want, manifestText)
		}
	}

	packageJSON, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Fatalf("read package.json: %v", err)
	}
	assertNoLifecycleScripts(t, string(packageJSON))

	if _, err := tmpl.ResolveValues(map[string]string{"projectName": "bad", "packageName": "bad", "runtime": "npm"}); err == nil || !strings.Contains(err.Error(), "TSPACK_TEMPLATE_VARIABLE_INVALID") {
		t.Fatalf("expected invalid runtime error, got %v", err)
	}
}

func TestBuiltinReactLibraryLoadsAndRenders(t *testing.T) {
	tmpl, err := LoadBuiltin("react-library")
	if err != nil {
		t.Fatalf("load react-library: %v", err)
	}
	for _, concept := range []string{"react.library", "vite.library", "package.exports", "tspack.pack", "typescript.library"} {
		if !contains(tmpl.Concepts, concept) {
			t.Fatalf("react-library template missing concept %q: %#v", concept, tmpl.Concepts)
		}
	}

	root := t.TempDir()
	values, err := tmpl.ResolveValues(map[string]string{"projectName": "UI Kit", "packageName": "@local/ui-kit", "runtime": "deno"})
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
		"tsconfig.build.json",
		"biome.json",
		"vite.config.ts",
		"package.json",
		"src/index.ts",
		"src/Button.tsx",
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
	for _, want := range []string{`name="UI Kit" runtime="deno"`, `name="@local/ui-kit"`, `peer(npm("react"`, `<Publish include={["dist/**", "README.md", "package.json"]}`, `command: ["tsc", "-p", "tsconfig.build.json", "--listEmittedFiles"]`} {
		if !strings.Contains(manifestText, want) {
			t.Fatalf("manifest missing %q:\n%s", want, manifestText)
		}
	}

	packageJSON, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Fatalf("read package.json: %v", err)
	}
	packageText := string(packageJSON)
	for _, want := range []string{`"peerDependencies"`, `"react"`, `"exports"`, `"private": true`} {
		if !strings.Contains(packageText, want) {
			t.Fatalf("package.json missing %q:\n%s", want, packageText)
		}
	}
	assertNoLifecycleScripts(t, packageText)

	if _, err := tmpl.ResolveValues(map[string]string{"projectName": "bad", "packageName": "bad", "runtime": "npm"}); err == nil || !strings.Contains(err.Error(), "TSPACK_TEMPLATE_VARIABLE_INVALID") {
		t.Fatalf("expected invalid runtime error, got %v", err)
	}
}

func assertNoLifecycleScripts(t *testing.T, packageText string) {
	t.Helper()

	for _, forbidden := range []string{"postinstall", "prepare", "prepublish", "preinstall", "install"} {
		if strings.Contains(packageText, forbidden) {
			t.Fatalf("package.json should not include lifecycle script %q:\n%s", forbidden, packageText)
		}
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

func TestRawTemplateLoadsBuiltinsAndLocal(t *testing.T) {
	for _, name := range []string{"static", "react", "react-library"} {
		raw, err := LoadRawBuiltin(name)
		if err != nil {
			t.Fatalf("load raw builtin %s: %v", name, err)
		}
		if raw.Name != name || raw.Source.Kind != SourceKindBuiltin || raw.Format != 1 {
			t.Fatalf("unexpected raw builtin %s: %#v", name, raw)
		}
	}

	templateRoot := makeLocalTemplate(t, `hello {{projectName}}\n`, "hello.txt")
	raw, err := LoadRawLocal(templateRoot)
	if err != nil {
		t.Fatalf("load raw local: %v", err)
	}
	if raw.Name != "local" || raw.Source.Kind != SourceKindLocal || raw.Source.Path == "" {
		t.Fatalf("unexpected raw local: %#v", raw)
	}
}

func TestTemplateIRNormalizationValidation(t *testing.T) {
	raw, err := LoadRawBuiltin("react-library")
	if err != nil {
		t.Fatalf("load raw react-library: %v", err)
	}
	ir, err := Normalize(raw)
	if err != nil {
		t.Fatalf("normalize react-library: %v", err)
	}
	if !contains(ir.Concepts, "react.library") || len(ir.Files) == 0 || ir.Source.Kind != SourceKindBuiltin {
		t.Fatalf("unexpected ir: %#v", ir)
	}

	raw.Variables["runtime"] = Variable{Default: "npm", Allowed: []string{"nodejs", "bun", "deno"}}
	if _, err := Normalize(raw); err == nil || !strings.Contains(err.Error(), "TSPACK_TEMPLATE_VARIABLE_INVALID") {
		t.Fatalf("expected invalid default, got %v", err)
	}
}

func TestTemplateIRRejectsDuplicateDestination(t *testing.T) {
	templateRoot := makeLocalTemplate(t, "hello", "same.txt")
	metadataPath := filepath.Join(templateRoot, MetadataFile)
	metadata, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	metadata = append(metadata, []byte(`
[[files]]
from = "files/hello.txt.tmpl"
to = "same.txt"
`)...)
	if err := os.WriteFile(metadataPath, metadata, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = LoadLocal(templateRoot)
	if err == nil || !strings.Contains(err.Error(), "TSPACK_TEMPLATE_PATH_INVALID") {
		t.Fatalf("expected duplicate destination error, got %v", err)
	}
}

func TestTemplatePlanBindsAndChecksBeforeApply(t *testing.T) {
	tmpl, err := LoadBuiltin("react")
	if err != nil {
		t.Fatalf("load react: %v", err)
	}
	values, err := tmpl.ResolveValues(map[string]string{"projectName": "Hello", "packageName": "@acme/hello", "runtime": "bun"})
	if err != nil {
		t.Fatalf("resolve values: %v", err)
	}
	plan, err := tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
	if err != nil {
		t.Fatalf("plan react: %v", err)
	}
	if plan.TemplateName != "react" || plan.Values["runtime"] != "bun" || len(plan.Files) != len(tmpl.Files) {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	if plan.Files[0].DestinationPath == "" || plan.Files[0].SourcePath == "" {
		t.Fatalf("plan did not resolve paths: %#v", plan.Files[0])
	}
}

func TestTemplatePlanUnknownPlaceholderDoesNotWrite(t *testing.T) {
	templateRoot := makeLocalTemplate(t, "hello {{missing}}\n", "hello.txt")
	tmpl, err := LoadLocal(templateRoot)
	if err != nil {
		t.Fatalf("load local: %v", err)
	}
	dest := t.TempDir()
	values, err := tmpl.ResolveValues(map[string]string{"projectName": "world"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tmpl.Apply(ApplyOptions{Destination: dest, Values: values})
	if err == nil || !strings.Contains(err.Error(), "TSPACK_TEMPLATE_UNKNOWN_VARIABLE") {
		t.Fatalf("expected unknown variable error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dest, "hello.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("expected planning failure to avoid writes, stat err: %v", statErr)
	}
}

func TestApplyPlanWritesExpectedFiles(t *testing.T) {
	templateRoot := makeLocalTemplate(t, "hello {{projectName}}\n", "hello.txt")
	tmpl, err := LoadLocal(templateRoot)
	if err != nil {
		t.Fatalf("load local: %v", err)
	}
	dest := t.TempDir()
	values, err := tmpl.ResolveValues(map[string]string{"projectName": "world"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := tmpl.Plan(PlanOptions{Destination: dest, Values: values})
	if err != nil {
		t.Fatalf("plan local: %v", err)
	}
	if err := ApplyPlan(plan); err != nil {
		t.Fatalf("apply plan: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dest, "hello.txt"))
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	if string(content) != "hello world\n" {
		t.Fatalf("unexpected content: %q", string(content))
	}
}

func makeLocalTemplate(t *testing.T, renderedContent string, destination string) string {
	t.Helper()
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
to = "` + destination + `"
`
	if err := os.WriteFile(filepath.Join(templateRoot, MetadataFile), []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(templateRoot, "files", "hello.txt.tmpl"), []byte(renderedContent), 0o644); err != nil {
		t.Fatal(err)
	}
	return templateRoot
}

func TestStaticConceptManifestRendererUsesMergedConceptIR(t *testing.T) {
	tmpl, err := LoadBuiltin("static")
	if err != nil {
		t.Fatalf("load static: %v", err)
	}
	values, err := tmpl.ResolveValues(map[string]string{"projectName": "Hello Static", "packageName": "hello-static", "runtime": "nodejs"})
	if err != nil {
		t.Fatalf("resolve values: %v", err)
	}
	plan, err := tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	manifest := ""
	for _, file := range plan.Files {
		if file.Path == "manifest.tsx" {
			manifest = string(file.content)
		}
	}
	if manifest == "" {
		t.Fatal("manifest.tsx was not planned")
	}
	for _, want := range []string{
		"// Generated from concept fragments:",
		"// - tspack.workspace",
		"// - tspack.manifestBoundary",
		"// - tspack.securityPolicy",
		"// - tspack.updatePolicy",
		"// - typescript.app",
		"// - vite.app",
		"// - browser.static",
		`const vite = tool(npm("vite", "^5.0.0"));`,
		`const typescript = tool(npm("typescript", "^5.0.0"));`,
		`const biome = tool(npm("@biomejs/biome", "^1.9.4"), { key: "@biomejs/biome" });`,
		`name: "dev"`,
		`name: "build"`,
		`name: "app"`,
		`entry: "src/main.ts"`,
		`types: "dist/main.d.ts"`,
		`<UpdatePolicy`,
		`<Security`,
	} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("concept-rendered manifest missing %q:\n%s", want, manifest)
		}
	}
}

func TestStaticConceptManifestCompositionFailureDiagnostic(t *testing.T) {
	tmpl, err := LoadBuiltin("static")
	if err != nil {
		t.Fatalf("load static: %v", err)
	}
	tmpl.Concepts = append([]string{}, tmpl.Concepts...)
	tmpl.Concepts = append(tmpl.Concepts, "react.library")
	values, err := tmpl.ResolveValues(map[string]string{"projectName": "bad", "packageName": "bad", "runtime": "nodejs"})
	if err != nil {
		t.Fatalf("resolve values: %v", err)
	}
	_, err = tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
	if err == nil {
		t.Fatal("expected composition failure")
	}
	message := err.Error()
	for _, want := range []string{"TSPACK_TEMPLATE_CONCEPT_COMPOSITION_FAILED", `template "static"`, "react.library", "incompatible"} {
		if !strings.Contains(message, want) {
			t.Fatalf("diagnostic missing %q: %v", want, err)
		}
	}
}

func TestReactLibraryConceptManifestRendererUsesMergedConceptIR(t *testing.T) {
	tmpl, err := LoadBuiltin("react-library")
	if err != nil {
		t.Fatalf("load react-library: %v", err)
	}
	values, err := tmpl.ResolveValues(map[string]string{"projectName": "ui-kit", "packageName": "@local/ui-kit", "runtime": "nodejs"})
	if err != nil {
		t.Fatalf("resolve values: %v", err)
	}
	plan, err := tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	manifest := ""
	for _, file := range plan.Files {
		if file.Path == "manifest.tsx" {
			manifest = string(file.content)
		}
	}
	if manifest == "" {
		t.Fatal("manifest.tsx was not planned")
	}
	for _, want := range []string{
		"// Generated from concept fragments:",
		"// - react.library",
		"// - package.peerDependencies",
		"// - package.exports",
		"// - tspack.pack",
		"// - vite.library",
		"// - typescript.library",
		`name="@local/ui-kit"`,
		`kind="library"`,
		`react: peer(npm("react", "^19.0.0"))`,
		`reactDom: peer(npm("react-dom", "^19.0.0"), { key: "react-dom" })`,
		`name: "library"`,
		`entry: "src/index.ts"`,
		`runtime: "dist/index.js"`,
		`types: "dist/index.d.ts"`,
		`peers: [deps.react, deps.reactDom]`,
		`name: "typecheck"`,
		`name: "build"`,
		`name: "build-types"`,
		`<Publish include={["dist/**", "README.md", "package.json"]} />`,
		`category: "consumer-install"`,
		`category: "maintainer-publish"`,
	} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("concept-rendered react-library manifest missing %q:\n%s", want, manifest)
		}
	}
}

func TestReactLibraryConceptManifestCompositionFailureDiagnostic(t *testing.T) {
	tmpl, err := LoadBuiltin("react-library")
	if err != nil {
		t.Fatalf("load react-library: %v", err)
	}
	tmpl.Concepts = removeTemplateConcept(tmpl.Concepts, "package.exports")
	values, err := tmpl.ResolveValues(map[string]string{"projectName": "ui-kit", "packageName": "@local/ui-kit", "runtime": "nodejs"})
	if err != nil {
		t.Fatalf("resolve values: %v", err)
	}
	_, err = tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
	if err == nil {
		t.Fatal("expected composition failure")
	}
	message := err.Error()
	for _, want := range []string{"TSPACK_TEMPLATE_CONCEPT_COMPOSITION_FAILED", `template "react-library"`, "package.exports", "expects"} {
		if !strings.Contains(message, want) {
			t.Fatalf("diagnostic missing %q: %v", want, err)
		}
	}
}

func removeTemplateConcept(concepts []string, removed string) []string {
	result := []string{}
	for _, concept := range concepts {
		if concept != removed {
			result = append(result, concept)
		}
	}
	return result
}
