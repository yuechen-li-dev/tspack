package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func TestTsclProjectRequestKeepsNodeRuntimeSeparateFromCompilerIdentity(t *testing.T) {
	root := t.TempDir()
	sourceDirectory := filepath.Join(root, "src")
	if err := os.MkdirAll(sourceDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDirectory, "Main.ts"), []byte("export function Main(): string { return \"ok\"; }"), 0o644); err != nil {
		t.Fatal(err)
	}

	request, err := newTsclProjectRequest(root, manifest.Target{
		Name:    "app",
		Entry:   "src/Main.ts",
		Runtime: "dist/main.js",
	}, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	if request.JavaScriptRuntime != "node" || request.JavaScriptProfile != "production" {
		t.Fatalf("request runtime/profile = %q/%q", request.JavaScriptRuntime, request.JavaScriptProfile)
	}
	if request.Entry.Module != "src/Main.ts" || request.EntryOutputPath != "main.js" {
		t.Fatalf("unexpected entry contract: %#v", request)
	}
	if request.BuildFingerprint == "" {
		t.Fatal("missing build fingerprint")
	}
}

func TestTsclProjectRequestSelectsBrowserRuntimeAndDistinctFingerprint(t *testing.T) {
	root := t.TempDir()
	sourceDirectory := filepath.Join(root, "src")
	if err := os.MkdirAll(sourceDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDirectory, "Main.ts"), []byte("export function Main(): void {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	browserTarget := manifest.Target{
		Name:              "browser",
		Entry:             "src/Main.ts",
		Runtime:           "dist/browser/main.js",
		JavaScriptRuntime: "browser",
	}
	browserRequest, err := newTsclProjectRequest(root, browserTarget, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}
	nodeRequest, err := newTsclProjectRequest(root, manifest.Target{
		Name:    "node",
		Entry:   "src/Main.ts",
		Runtime: "dist/node/main.js",
	}, "1.2.3", nil)
	if err != nil {
		t.Fatal(err)
	}

	if browserRequest.JavaScriptRuntime != "browser" {
		t.Fatalf("browser runtime = %q", browserRequest.JavaScriptRuntime)
	}
	if browserRequest.JavaScriptProfile != "production" {
		t.Fatalf("browser profile = %q", browserRequest.JavaScriptProfile)
	}
	if browserRequest.BuildFingerprint == nodeRequest.BuildFingerprint {
		t.Fatal("browser and node target requests must not share a build fingerprint")
	}
}

func TestCollectTsclNpmContractsDoesNotTreatViteToolAsCopelandDependency(t *testing.T) {
	pkg := &manifest.Package{
		Tools: []string{"vite"},
		Dependencies: []manifest.DependencyIntent{
			{Key: "nanoid", Source: manifest.Source{Kind: "npm", Package: "nanoid"}},
			{Key: "vite", Source: manifest.Source{Kind: "npm", Package: "vite"}},
		},
	}
	if isPackageTool(pkg, pkg.Dependencies[0]) {
		t.Fatal("runtime package was incorrectly classified as a tool")
	}
	if !isPackageTool(pkg, pkg.Dependencies[1]) {
		t.Fatal("Vite tool was incorrectly passed to the Copeland package contract")
	}
}

func TestMaterializeBrowserGraphSelectsBrowserExportAndWritesDeterministicImport(t *testing.T) {
	root := t.TempDir()
	packageDirectory := filepath.Join(root, "node_modules", "fixture-browser-package")
	if err := os.MkdirAll(packageDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	packageJSON := `{
  "name": "fixture-browser-package",
  "exports": {
    ".": {
      "browser": "./browser.js",
      "import": "./import.js",
      "default": "./default.js"
    }
  }
}`
	if err := os.WriteFile(filepath.Join(packageDirectory, "package.json"), []byte(packageJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDirectory, "browser.js"), []byte("export const value = 'browser';\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	outputDirectory := filepath.Join(root, "dist", "browser")
	materialization, err := materializeBrowserGraph(outputDirectory, []tsclNpmContract{
		{
			PackageName:         "fixture-browser-package",
			MaterializationPath: packageDirectory,
			Materialized:        true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(materialization.Imports) != 2 {
		t.Fatalf("materialized imports = %#v", materialization.Imports)
	}
	if materialization.Imports[1] != (browserImport{
		Specifier: "fixture-browser-package",
		URL:       "./packages/fixture-browser-package/browser.js",
	}) {
		t.Fatalf("package import = %#v", materialization.Imports[1])
	}
	if _, err := os.Stat(filepath.Join(outputDirectory, "packages", "fixture-browser-package", "browser.js")); err != nil {
		t.Fatalf("browser export was not copied: %v", err)
	}
	hostBytes, err := os.ReadFile(filepath.Join(outputDirectory, "packages", "copeland-browser-v1", "index.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(hostBytes), "export function copyText") {
		t.Fatalf("generated browser host does not expose the clipboard contract: %s", hostBytes)
	}
	if !strings.Contains(string(hostBytes), "export function subscribeViewport") {
		t.Fatalf("generated browser host does not expose the viewport subscription contract: %s", hostBytes)
	}
	for _, api := range []string{
		"export function scheduleRendererAttachment",
		"export function attachRenderer",
		"export function updateRenderer",
		"export function detachRenderer",
		"const rendererAdapters",
		"\"CustomElement\"",
	} {
		if !strings.Contains(string(hostBytes), api) {
			t.Fatalf("generated browser host does not expose renderer attachment API %q: %s", api, hostBytes)
		}
	}
	secondOutputDirectory := filepath.Join(root, "dist", "browser-second")
	if _, err := materializeBrowserGraph(secondOutputDirectory, []tsclNpmContract{
		{
			PackageName:         "fixture-browser-package",
			MaterializationPath: packageDirectory,
			Materialized:        true,
		},
	}); err != nil {
		t.Fatal(err)
	}
	secondHostBytes, err := os.ReadFile(filepath.Join(secondOutputDirectory, "packages", "copeland-browser-v1", "index.js"))
	if err != nil {
		t.Fatal(err)
	}
	if string(hostBytes) != string(secondHostBytes) {
		t.Fatal("generated browser host changed across identical materializations")
	}
}

func TestBrowserHostCopiesLocallyLinkedStaticAssets(t *testing.T) {
	root := t.TempDir()
	outputDirectory := filepath.Join(root, "dist", "browser")
	if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte(`<!doctype html><link rel="stylesheet" href="./machina-hero.generated.css">`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "machina-hero.generated.css"), []byte(".m-frame-root {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeBrowserHost(root, outputDirectory, "main.js", browserMaterialization{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outputDirectory, "machina-hero.generated.css")); err != nil {
		t.Fatalf("linked generated asset was not copied: %v", err)
	}
}

func TestMaterializeBrowserGraphTransformsCommonJSEntry(t *testing.T) {
	root := t.TempDir()
	packageDirectory := filepath.Join(root, "node_modules", "fixture-commonjs-package")
	if err := os.MkdirAll(packageDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDirectory, "package.json"), []byte(`{"main":"./index.js"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDirectory, "index.js"), []byte("module.exports = { value: 1 };\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	materialization, err := materializeBrowserGraph(filepath.Join(root, "dist", "browser"), []tsclNpmContract{
		{
			PackageName:         "fixture-commonjs-package",
			MaterializationPath: packageDirectory,
			Materialized:        true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if materialization.Packages[1].Mode != "transformed-esm" {
		t.Fatalf("CommonJS materialization mode = %#v", materialization.Packages[1])
	}
}
