package templates

import (
	"bytes"
	"io/fs"
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
		`const typescript = tool(npm("typescript", "^5.9.0"));`,
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

func TestLocalConceptAddsRenderedFile(t *testing.T) {
	templateRoot := makeLocalConceptTemplate(t, "react.app", "")
	tmpl, err := LoadLocal(templateRoot)
	if err != nil {
		t.Fatalf("load local concept template: %v", err)
	}
	values, err := tmpl.ResolveValues(map[string]string{"projectName": "Acme App"})
	if err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if _, err := tmpl.Apply(ApplyOptions{Destination: dest, Values: values}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dest, "src", "design-system.ts"))
	if err != nil {
		t.Fatalf("read local concept file: %v", err)
	}
	if !strings.Contains(string(content), "Acme App") {
		t.Fatalf("local concept file was not rendered: %s", string(content))
	}
}

func TestLocalConceptValidationFailures(t *testing.T) {
	cases := []struct {
		name          string
		metadataExtra string
		conceptName   string
		conceptBody   string
		want          string
	}{
		{name: "missing expected", metadataExtra: `concepts = ["my-company.design-system"]`, conceptName: "my-company.design-system", conceptBody: `format = 1
name = "my-company.design-system"
expects = ["react.app"]
`, want: "expects \"react.app\""},
		{name: "name mismatch", metadataExtra: `concepts = ["react.app", "my-company.design-system"]`, conceptName: "my-company.design-system", conceptBody: `format = 1
name = "wrong.name"
`, want: "declares name"},
		{name: "shadow builtin", metadataExtra: `concepts = ["react.app"]`, conceptName: "react.app", conceptBody: `format = 1
name = "react.app"
`, want: "shadows a built-in"},
		{name: "unknown field", conceptName: "my-company.design-system", conceptBody: `format = 1
name = "my-company.design-system"
script = "echo nope"
`, want: "TSPACK_TEMPLATE_CONCEPT_INVALID"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := makeLocalConceptTemplateWithBody(t, tc.metadataExtra, tc.conceptName, tc.conceptBody)
			tmpl, err := LoadLocal(root)
			if err != nil {
				t.Fatalf("load should defer concept diagnostics to planning where possible: %v", err)
			}
			values, _ := tmpl.ResolveValues(map[string]string{"projectName": "demo"})
			_, err = tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
}

func TestLocalConceptPathConflictAndUnsupportedManifestContribution(t *testing.T) {
	t.Run("destination traversal rejected", func(t *testing.T) {
		body := `format = 1
name = "my-company.design-system"
[[files]]
destination = "../escape.ts"
source = "files/design-system.ts.tmpl"
render = true
`
		root := makeLocalConceptTemplateWithBody(t, "", "my-company.design-system", body)
		tmpl, _ := LoadLocal(root)
		values, _ := tmpl.ResolveValues(map[string]string{"projectName": "demo"})
		_, err := tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
		if err == nil || !strings.Contains(err.Error(), "TSPACK_TEMPLATE_PATH_INVALID") {
			t.Fatalf("expected path diagnostic, got %v", err)
		}
	})

	t.Run("conflict with template file rejected", func(t *testing.T) {
		root := makeLocalConceptTemplate(t, "react.app", "")
		metadataPath := filepath.Join(root, MetadataFile)
		metadata, _ := os.ReadFile(metadataPath)
		metadata = append(metadata, []byte("\n[[files]]\nfrom = \"files/hello.txt.tmpl\"\nto = \"src/design-system.ts\"\n")...)
		_ = os.WriteFile(metadataPath, metadata, 0o644)
		tmpl, _ := LoadLocal(root)
		values, _ := tmpl.ResolveValues(map[string]string{"projectName": "demo"})
		_, err := tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
		if err == nil || !strings.Contains(err.Error(), "TSPACK_TEMPLATE_CONCEPT_CONFLICT") {
			t.Fatalf("expected conflict, got %v", err)
		}
	})

	t.Run("conflict between local concepts rejected", func(t *testing.T) {
		root := t.TempDir()
		for _, dir := range []string{"files", "concepts/files-a", "concepts/files-b"} {
			if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		metadata := `format = 1
name = "company-react"
description = "Company React starter"
kind = "app"
concepts = ["react.app", "browser.spa", "typescript.app", "my-company.first", "my-company.second"]

[variables.projectName]
default = "demo"

[[files]]
from = "files/hello.txt.tmpl"
to = "README.md"

[[localConcepts]]
name = "my-company.first"
path = "concepts/first.toml"

[[localConcepts]]
name = "my-company.second"
path = "concepts/second.toml"
`
		if err := os.WriteFile(filepath.Join(root, MetadataFile), []byte(metadata), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "files", "hello.txt.tmpl"), []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}
		firstConcept := `format = 1
name = "my-company.first"
provides = ["my-company.first"]
expects = ["react.app"]
compatibleKinds = ["app"]

[[files]]
destination = "src/shared.ts"
source = "files-a/shared.ts"
render = false
`
		if err := os.WriteFile(filepath.Join(root, "concepts", "first.toml"), []byte(firstConcept), 0o644); err != nil {
			t.Fatal(err)
		}
		secondConcept := `format = 1
name = "my-company.second"
provides = ["my-company.second"]
expects = ["react.app"]
compatibleKinds = ["app"]

[[files]]
destination = "src/shared.ts"
source = "files-b/shared.ts"
render = false
`
		if err := os.WriteFile(filepath.Join(root, "concepts", "second.toml"), []byte(secondConcept), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "concepts", "files-a", "shared.ts"), []byte("export const owner = 'first';\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "concepts", "files-b", "shared.ts"), []byte("export const owner = 'second';\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		tmpl, err := LoadLocal(root)
		if err != nil {
			t.Fatal(err)
		}
		values, _ := tmpl.ResolveValues(map[string]string{"projectName": "demo"})
		_, err = tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
		if err == nil || !strings.Contains(err.Error(), `conflict at files.src/shared.ts`) {
			t.Fatalf("expected deterministic concept file conflict, got %v", err)
		}
	})

	t.Run("manifest contribution unsupported", func(t *testing.T) {
		body := `format = 1
name = "my-company.design-system"
[[dependencies]]
key = "@my-company/design-system"
source = "npm"
range = "^1.2.0"
`
		root := makeLocalConceptTemplateWithBody(t, "", "my-company.design-system", body)
		tmpl, _ := LoadLocal(root)
		values, _ := tmpl.ResolveValues(map[string]string{"projectName": "demo"})
		_, err := tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
		if err == nil || !strings.Contains(err.Error(), "TSPACK_TEMPLATE_CONCEPT_UNSUPPORTED_CONTRIBUTION") {
			t.Fatalf("expected unsupported contribution, got %v", err)
		}
	})
}

func makeLocalConceptTemplate(t *testing.T, expected string, extra string) string {
	body := `format = 1
name = "my-company.design-system"
description = "Company design system additions"
provides = ["my-company.design-system"]
expects = ["` + expected + `"]

[[files]]
destination = "src/design-system.ts"
source = "files/design-system.ts.tmpl"
render = true
` + extra
	return makeLocalConceptTemplateWithBody(t, "", "my-company.design-system", body)
}

func makeLocalConceptTemplateWithBody(t *testing.T, metadataExtra string, conceptName string, conceptBody string) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"files", "concepts/files"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	conceptsLine := `concepts = ["react.app", "browser.spa", "typescript.app", "my-company.design-system"]`
	if metadataExtra != "" {
		conceptsLine = metadataExtra
	}
	metadata := `format = 1
name = "company-react"
description = "Company React starter"
kind = "app"
` + conceptsLine + `
[variables.projectName]
default = "demo"
[[files]]
from = "files/hello.txt.tmpl"
to = "hello.txt"
[[localConcepts]]
name = "` + conceptName + `"
path = "concepts/design-system.toml"
`
	if err := os.WriteFile(filepath.Join(root, MetadataFile), []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "files", "hello.txt.tmpl"), []byte("hello {{projectName}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "concepts", "design-system.toml"), []byte(conceptBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "concepts", "files", "design-system.ts.tmpl"), []byte("export const project = \"{{projectName}}\";\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLocalConceptFixtureCompanyReact(t *testing.T) {
	tmpl, err := LoadLocal(filepath.Join("testdata", "local-concepts", "company-react"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	values, err := tmpl.ResolveValues(map[string]string{"projectName": "Fixture App"})
	if err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if _, err := tmpl.Apply(ApplyOptions{Destination: dest, Values: values}); err != nil {
		t.Fatalf("apply fixture: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dest, "src", "design-system.ts"))
	if err != nil {
		t.Fatalf("read fixture contribution: %v", err)
	}
	if !strings.Contains(string(content), "Fixture App") {
		t.Fatalf("fixture local concept did not render variables: %s", string(content))
	}
}

func TestLocalConceptManifestOptInFixture(t *testing.T) {
	tmpl, err := LoadLocal(filepath.Join("testdata", "local-concepts", "concept-manifest-app"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	values, err := tmpl.ResolveValues(map[string]string{"projectName": "Fixture Concept App", "packageName": "fixture-concept-app", "runtime": "nodejs"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
	if err != nil {
		t.Fatalf("plan fixture: %v", err)
	}
	manifest := ""
	foundFile := false
	for _, file := range plan.Files {
		if file.Path == "manifest.tsx" {
			manifest = string(file.content)
		}
		if file.Path == "src/design-system.ts" {
			foundFile = true
		}
	}
	if manifest == "" {
		t.Fatal("manifest.tsx was not planned")
	}
	for _, want := range []string{
		"// - my-company.design-system",
		`clsx: dep(npm("clsx", "^2.1.1"))`,
		`name: "generate-icons"`,
		`command: ["node", "-e", "console.log('icons-generated')"]`,
	} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("manifest missing %q:\n%s", want, manifest)
		}
	}
	for _, requiredPath := range []string{
		"src/design-system.ts",
		"src/main.tsx",
		"src/App.tsx",
		"src/ui.ts",
		"vite.config.ts",
		"tsconfig.json",
		"package.json",
	} {
		foundRequiredPath := false
		for _, file := range plan.Files {
			if file.Path == requiredPath {
				foundRequiredPath = true
			}
		}
		if !foundRequiredPath {
			t.Fatalf("required generated file %q was not planned", requiredPath)
		}
	}
	if !foundFile {
		t.Fatal("local concept rendered file was not planned")
	}
}

func TestTailwindLocalConceptManifestFixture(t *testing.T) {
	tmpl, err := LoadLocal(filepath.Join("testdata", "local-concepts", "tailwind-react-app"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	values, err := tmpl.ResolveValues(map[string]string{"projectName": "Tailwind Fixture", "packageName": "tailwind-fixture", "runtime": "nodejs"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
	if err != nil {
		t.Fatalf("plan fixture: %v", err)
	}
	planned := map[string]string{}
	for _, file := range plan.Files {
		planned[file.Path] = string(file.content)
	}
	manifest := planned["manifest.tsx"]
	if manifest == "" {
		t.Fatal("manifest.tsx was not planned")
	}
	for _, want := range []string{
		"// - my-company.tailwind",
		`tailwindcss: dep(npm("tailwindcss", "^4.0.0"))`,
		`tailwindcssVite: tool(npm("@tailwindcss/vite", "^4.0.0"), {`,
	} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("manifest missing %q:\n%s", want, manifest)
		}
	}
	for _, requiredPath := range []string{
		"src/style.css",
		"src/main.tsx",
		"src/App.tsx",
		"vite.config.ts",
		"tsconfig.json",
		"package.json",
		"README.md",
	} {
		if _, ok := planned[requiredPath]; !ok {
			t.Fatalf("required generated file %q was not planned", requiredPath)
		}
	}
	if !strings.Contains(planned["src/style.css"], `@import "tailwindcss";`) {
		t.Fatalf("Tailwind CSS import was not contributed:\n%s", planned["src/style.css"])
	}
	if !strings.Contains(planned["src/App.tsx"], "min-h-screen bg-slate-950") {
		t.Fatalf("fixture app does not use Tailwind utility classes:\n%s", planned["src/App.tsx"])
	}
}

func TestTailwindLocalConceptCompanionDiagnostics(t *testing.T) {
	root := filepath.Join("testdata", "local-concepts", "tailwind-react-app")
	t.Run("expects vite app", func(t *testing.T) {
		copyRoot := copyTemplateFixture(t, root)
		metadataPath := filepath.Join(copyRoot, MetadataFile)
		metadata, err := os.ReadFile(metadataPath)
		if err != nil {
			t.Fatal(err)
		}
		metadata = replaceMetadataOnce(t, metadata, "  \"vite.app\",\n", "")
		if err := os.WriteFile(metadataPath, metadata, 0o644); err != nil {
			t.Fatal(err)
		}
		tmpl, err := LoadLocal(copyRoot)
		if err != nil {
			t.Fatal(err)
		}
		values, err := tmpl.ResolveValues(map[string]string{"projectName": "demo", "packageName": "demo", "runtime": "nodejs"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
		if err == nil || !strings.Contains(err.Error(), `concept "my-company.tailwind" expects "vite.app"`) {
			t.Fatalf("expected vite companion diagnostic, got %v", err)
		}
	})

	t.Run("excludes library kind", func(t *testing.T) {
		copyRoot := copyTemplateFixture(t, root)
		metadataPath := filepath.Join(copyRoot, MetadataFile)
		metadata, err := os.ReadFile(metadataPath)
		if err != nil {
			t.Fatal(err)
		}
		metadata = replaceMetadataOnce(t, metadata, `kind = "app"`, `kind = "library"`)
		if err := os.WriteFile(metadataPath, metadata, 0o644); err != nil {
			t.Fatal(err)
		}
		tmpl, err := LoadLocal(copyRoot)
		if err != nil {
			t.Fatal(err)
		}
		values, err := tmpl.ResolveValues(map[string]string{"projectName": "demo", "packageName": "demo", "runtime": "nodejs"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
		if err == nil || !strings.Contains(err.Error(), `concept "react.app" is incompatible with kind "library"`) {
			t.Fatalf("expected library incompatibility diagnostic, got %v", err)
		}
	})
}

func TestTailwindMachinaLocalConceptManifestFixture(t *testing.T) {
	tmpl, err := LoadLocal(filepath.Join("testdata", "local-concepts", "tailwind-machina-react-app"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	values, err := tmpl.ResolveValues(map[string]string{"projectName": "Tailwind Machina Fixture", "packageName": "tailwind-machina-fixture", "runtime": "nodejs"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
	if err != nil {
		t.Fatalf("plan fixture: %v", err)
	}
	planned := map[string]string{}
	for _, file := range plan.Files {
		planned[file.Path] = string(file.content)
	}
	manifest := planned["manifest.tsx"]
	if manifest == "" {
		t.Fatal("manifest.tsx was not planned")
	}
	for _, want := range []string{
		"// - my-company.tailwind",
		"// - my-company.machina-layout",
		`tailwindcss: dep(npm("tailwindcss", "^4.0.0"))`,
		`tailwindcssVite: tool(npm("@tailwindcss/vite", "^4.0.0"), {`,
		`machinalayout: dep(npm("machinalayout", "^0.3.1"))`,
	} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("manifest missing %q:\n%s", want, manifest)
		}
	}
	for _, requiredPath := range []string{
		"src/style.css",
		"src/machina-layout.tsx",
		"src/main.tsx",
		"src/App.tsx",
		"vite.config.ts",
		"tsconfig.json",
		"package.json",
		"README.md",
	} {
		if _, ok := planned[requiredPath]; !ok {
			t.Fatalf("required generated file %q was not planned", requiredPath)
		}
	}
	if !strings.Contains(planned["src/style.css"], `@import "tailwindcss";`) {
		t.Fatalf("Tailwind CSS import was not contributed:\n%s", planned["src/style.css"])
	}
	if !strings.Contains(planned["src/machina-layout.tsx"], `import { MachinaReactView } from "machinalayout/react";`) {
		t.Fatalf("MachinaLayout React adapter import was not contributed:\n%s", planned["src/machina-layout.tsx"])
	}
	if !strings.Contains(planned["src/App.tsx"], "min-h-screen bg-slate-950") {
		t.Fatalf("fixture app does not use Tailwind utility classes:\n%s", planned["src/App.tsx"])
	}
	if !strings.Contains(planned["src/App.tsx"], "MachinaLayoutExample") {
		t.Fatalf("fixture app does not use contributed MachinaLayout component:\n%s", planned["src/App.tsx"])
	}
}

func TestTailwindMachinaLocalConceptCompanionDiagnostics(t *testing.T) {
	root := filepath.Join("testdata", "local-concepts", "tailwind-machina-react-app")
	t.Run("tailwind expects vite app", func(t *testing.T) {
		copyRoot := copyTemplateFixture(t, root)
		metadataPath := filepath.Join(copyRoot, MetadataFile)
		metadata, err := os.ReadFile(metadataPath)
		if err != nil {
			t.Fatal(err)
		}
		metadata = replaceMetadataOnce(t, metadata, "  \"vite.app\",\n", "")
		if err := os.WriteFile(metadataPath, metadata, 0o644); err != nil {
			t.Fatal(err)
		}
		tmpl, err := LoadLocal(copyRoot)
		if err != nil {
			t.Fatal(err)
		}
		values, err := tmpl.ResolveValues(map[string]string{"projectName": "demo", "packageName": "demo", "runtime": "nodejs"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
		if err == nil || !strings.Contains(err.Error(), `concept "my-company.tailwind" expects "vite.app"`) {
			t.Fatalf("expected Tailwind vite companion diagnostic, got %v", err)
		}
	})

	t.Run("machina expects react app", func(t *testing.T) {
		copyRoot := copyTemplateFixture(t, root)
		metadataPath := filepath.Join(copyRoot, MetadataFile)
		metadata, err := os.ReadFile(metadataPath)
		if err != nil {
			t.Fatal(err)
		}
		stackStart := bytes.Index(metadata, []byte("concepts = ["))
		stackEnd := bytes.Index(metadata[stackStart:], []byte("]\n\n[generation]"))
		if stackStart < 0 || stackEnd < 0 {
			t.Fatalf("fixture concept stack shape changed:\n%s", string(metadata))
		}
		stackEnd += stackStart + len("]")
		metadata = append(
			append([]byte{}, metadata[:stackStart]...),
			append([]byte("concepts = [\n  \"browser.spa\",\n  \"vite.app\",\n  \"typescript.app\",\n  \"my-company.machina-layout\",\n  \"tspack.workspace\",\n]"), metadata[stackEnd:]...)...,
		)
		localConceptStart := bytes.Index(metadata, []byte("[[localConcepts]]\nname = \"my-company.tailwind\""))
		if localConceptStart >= 0 {
			localConceptEnd := bytes.Index(metadata[localConceptStart+1:], []byte("[[localConcepts]]"))
			if localConceptEnd >= 0 {
				localConceptEnd += localConceptStart + 1
				metadata = append(metadata[:localConceptStart], metadata[localConceptEnd:]...)
			}
		}
		if err := os.WriteFile(metadataPath, metadata, 0o644); err != nil {
			t.Fatal(err)
		}
		tmpl, err := LoadLocal(copyRoot)
		if err != nil {
			t.Fatal(err)
		}
		values, err := tmpl.ResolveValues(map[string]string{"projectName": "demo", "packageName": "demo", "runtime": "nodejs"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
		if err == nil || !strings.Contains(err.Error(), `concept "my-company.machina-layout" expects "react.app"`) {
			t.Fatalf("expected MachinaLayout react companion diagnostic, got %v", err)
		}
	})

	t.Run("excludes library kind", func(t *testing.T) {
		copyRoot := copyTemplateFixture(t, root)
		metadataPath := filepath.Join(copyRoot, MetadataFile)
		metadata, err := os.ReadFile(metadataPath)
		if err != nil {
			t.Fatal(err)
		}
		metadata = replaceMetadataOnce(t, metadata, `kind = "app"`, `kind = "library"`)
		stackStart := bytes.Index(metadata, []byte("concepts = ["))
		stackEnd := bytes.Index(metadata[stackStart:], []byte("]\n\n[generation]"))
		if stackStart < 0 || stackEnd < 0 {
			t.Fatalf("fixture concept stack shape changed:\n%s", string(metadata))
		}
		stackEnd += stackStart + len("]")
		metadata = append(
			append([]byte{}, metadata[:stackStart]...),
			append([]byte("concepts = [\n  \"my-company.tailwind\",\n  \"my-company.machina-layout\",\n  \"tspack.workspace\",\n]"), metadata[stackEnd:]...)...,
		)
		if err := os.WriteFile(metadataPath, metadata, 0o644); err != nil {
			t.Fatal(err)
		}
		tmpl, err := LoadLocal(copyRoot)
		if err != nil {
			t.Fatal(err)
		}
		values, err := tmpl.ResolveValues(map[string]string{"projectName": "demo", "packageName": "demo", "runtime": "nodejs"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
		if err == nil || !strings.Contains(err.Error(), `is incompatible with kind "library"`) {
			t.Fatalf("expected library incompatibility diagnostic, got %v", err)
		}
	})
}

func TestMachinaLayoutLocalConceptManifestFixture(t *testing.T) {
	tmpl, err := LoadLocal(filepath.Join("testdata", "local-concepts", "machina-react-app"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	values, err := tmpl.ResolveValues(map[string]string{"projectName": "Machina Fixture", "packageName": "machina-fixture", "runtime": "nodejs"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
	if err != nil {
		t.Fatalf("plan fixture: %v", err)
	}
	planned := map[string]string{}
	for _, file := range plan.Files {
		planned[file.Path] = string(file.content)
	}
	manifest := planned["manifest.tsx"]
	if manifest == "" {
		t.Fatal("manifest.tsx was not planned")
	}
	for _, want := range []string{
		"// - my-company.machina-layout",
		`machinalayout: dep(npm("machinalayout", "^0.3.1"))`,
	} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("manifest missing %q:\n%s", want, manifest)
		}
	}
	for _, requiredPath := range []string{
		"src/style.css",
		"src/machina-layout.tsx",
		"src/main.tsx",
		"src/App.tsx",
		"vite.config.ts",
		"tsconfig.json",
		"package.json",
		"README.md",
	} {
		if _, ok := planned[requiredPath]; !ok {
			t.Fatalf("required generated file %q was not planned", requiredPath)
		}
	}
	if !strings.Contains(planned["src/machina-layout.tsx"], `import { MachinaReactView } from "machinalayout/react";`) {
		t.Fatalf("MachinaLayout React adapter import was not contributed:\n%s", planned["src/machina-layout.tsx"])
	}
	if !strings.Contains(planned["src/App.tsx"], "MachinaLayoutExample") {
		t.Fatalf("fixture app does not use contributed MachinaLayout component:\n%s", planned["src/App.tsx"])
	}
}

func TestMachinaLayoutLocalConceptCompanionDiagnostics(t *testing.T) {
	root := filepath.Join("testdata", "local-concepts", "machina-react-app")
	t.Run("expects vite app", func(t *testing.T) {
		copyRoot := copyTemplateFixture(t, root)
		metadataPath := filepath.Join(copyRoot, MetadataFile)
		metadata, err := os.ReadFile(metadataPath)
		if err != nil {
			t.Fatal(err)
		}
		metadata = replaceMetadataOnce(t, metadata, "  \"vite.app\",\n", "")
		if err := os.WriteFile(metadataPath, metadata, 0o644); err != nil {
			t.Fatal(err)
		}
		tmpl, err := LoadLocal(copyRoot)
		if err != nil {
			t.Fatal(err)
		}
		values, err := tmpl.ResolveValues(map[string]string{"projectName": "demo", "packageName": "demo", "runtime": "nodejs"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
		if err == nil || !strings.Contains(err.Error(), `concept "my-company.machina-layout" expects "vite.app"`) {
			t.Fatalf("expected vite companion diagnostic, got %v", err)
		}
	})

	t.Run("excludes library kind", func(t *testing.T) {
		copyRoot := copyTemplateFixture(t, root)
		metadataPath := filepath.Join(copyRoot, MetadataFile)
		metadata, err := os.ReadFile(metadataPath)
		if err != nil {
			t.Fatal(err)
		}
		metadata = replaceMetadataOnce(t, metadata, `kind = "app"`, `kind = "library"`)
		stackStart := bytes.Index(metadata, []byte("concepts = ["))
		stackEnd := bytes.Index(metadata[stackStart:], []byte("]\n\n[generation]"))
		if stackStart < 0 || stackEnd < 0 {
			t.Fatalf("fixture concept stack shape changed:\n%s", string(metadata))
		}
		stackEnd += stackStart + len("]")
		metadata = append(
			append([]byte{}, metadata[:stackStart]...),
			append([]byte("concepts = [\n  \"my-company.machina-layout\",\n  \"tspack.workspace\",\n]"), metadata[stackEnd:]...)...,
		)
		if err := os.WriteFile(metadataPath, metadata, 0o644); err != nil {
			t.Fatal(err)
		}
		tmpl, err := LoadLocal(copyRoot)
		if err != nil {
			t.Fatal(err)
		}
		values, err := tmpl.ResolveValues(map[string]string{"projectName": "demo", "packageName": "demo", "runtime": "nodejs"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values})
		if err == nil || !strings.Contains(err.Error(), `concept "my-company.machina-layout" is incompatible with kind "library"`) {
			t.Fatalf("expected library incompatibility diagnostic, got %v", err)
		}
	})
}

func replaceMetadataOnce(t *testing.T, metadata []byte, old string, new string) []byte {
	t.Helper()
	oldBytes := []byte(old)
	count := bytes.Count(metadata, oldBytes)
	if count != 1 {
		t.Fatalf("expected metadata replacement %q exactly once, found %d:\n%s", old, count, string(metadata))
	}
	return bytes.Replace(metadata, oldBytes, []byte(new), 1)
}

func copyTemplateFixture(t *testing.T, source string) string {
	t.Helper()
	destination := t.TempDir()
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		from := filepath.Join(source, entry.Name())
		to := filepath.Join(destination, entry.Name())
		if entry.IsDir() {
			if err := copyDirectory(t, from, to); err != nil {
				t.Fatal(err)
			}
			continue
		}
		data, err := os.ReadFile(from)
		if err != nil {
			t.Fatal(err)
		}
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		if err := os.WriteFile(to, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return destination
}

func copyDirectory(t *testing.T, source string, destination string) error {
	t.Helper()
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func TestLocalConceptManifestOptInRejectsManifestProjection(t *testing.T) {
	root := makeLocalConceptTemplate(t, "react.app", "")
	metadataPath := filepath.Join(root, MetadataFile)
	metadata, _ := os.ReadFile(metadataPath)
	metadata = append(metadata, []byte("\n[generation]\nmanifest = \"concept\"\n[[files]]\nfrom = \"files/hello.txt.tmpl\"\nto = \"manifest.tsx\"\n")...)
	_ = os.WriteFile(metadataPath, metadata, 0o644)
	tmpl, err := LoadLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	values, _ := tmpl.ResolveValues(map[string]string{"projectName": "demo", "packageName": "demo", "runtime": "nodejs"})
	_, err = tmpl.Plan(PlanOptions{Destination: t.TempDir(), Values: values, Force: true})
	if err == nil || !strings.Contains(err.Error(), "TSPACK_TEMPLATE_CONCEPT_CONFLICT") {
		t.Fatalf("expected concept manifest projection conflict, got %v", err)
	}
}

func TestLocalConceptManifestUnknownGenerationMode(t *testing.T) {
	root := makeLocalConceptTemplate(t, "react.app", "")
	metadataPath := filepath.Join(root, MetadataFile)
	metadata, _ := os.ReadFile(metadataPath)
	metadata = append(metadata, []byte("\n[generation]\nmanifest = \"magic\"\n")...)
	_ = os.WriteFile(metadataPath, metadata, 0o644)
	_, err := LoadLocal(root)
	if err == nil || !strings.Contains(err.Error(), "TSPACK_TEMPLATE_INVALID") {
		t.Fatalf("expected invalid generation mode, got %v", err)
	}
}
