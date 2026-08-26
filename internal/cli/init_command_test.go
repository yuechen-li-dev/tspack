package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/project"
)

func TestInitLegacyHelpIncludesCommand(t *testing.T) {
	repo := filepath.Join("..", "..")
	cmd := exec.Command(testTspackBinary, "help", "all")
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

	t.Run("default static template without kind", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command(testTspackBinary, "init", "--root", root, "--name", "acme-demo")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(b), `Created TSPack project: acme-demo`) {
			t.Fatalf("expected default static template: %v\n%s", err, string(b))
		}
		if _, err := os.Stat(filepath.Join(root, "index.html")); err != nil {
			t.Fatalf("missing static index.html: %v", err)
		}
		assertGeneratedVSCodeSettings(t, root, "node_modules/typescript/lib")
		manifestText := readFileString(t, filepath.Join(root, "manifest.tsx"))
		if !strings.Contains(manifestText, `tool(npm("typescript", "^5.9.0"))`) {
			t.Fatalf("static template should declare TypeScript as a managed tool dependency:\n%s", manifestText)
		}
	})

	t.Run("list includes react template concepts", func(t *testing.T) {
		cmd := exec.Command(testTspackBinary, "init", "--list-templates")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("list templates failed: %v\n%s", err, string(b))
		}
		for _, want := range []string{"static", "react", "react-library", "react.app", "vite.app", "browser.spa", "react.library", "vite.library", "package.exports", "tspack.pack"} {
			if !strings.Contains(string(b), want) {
				t.Fatalf("template list missing %q:\n%s", want, string(b))
			}
		}
	})

	t.Run("react template generation", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command(testTspackBinary, "init", "--root", root, "--template", "react", "--name", "hello-react", "--package", "@acme/hello-react", "--runtime", "bun")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("react init failed: %v\n%s", err, string(b))
		}
		for _, want := range []string{"react.app", "vite.app", "browser.spa"} {
			if !strings.Contains(string(b), want) {
				t.Fatalf("init output missing concept %q:\n%s", want, string(b))
			}
		}
		for _, rel := range []string{"manifest.tsx", ".tspack-version", "tsconfig.tspack.json", ".vscode/settings.json", "tsconfig.json", "biome.json", "vite.config.ts", "package.json", "index.html", "src/main.tsx", "src/App.tsx", "src/style.css", "README.md"} {
			if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
				t.Fatalf("missing generated file %s: %v", rel, err)
			}
		}
		assertGeneratedTSPackTSConfig(t, root)
		assertGeneratedVSCodeSettings(t, root, "node_modules/typescript/lib")
		assertGeneratedReactAppTSConfig(t, root)

		manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.tsx"))
		if err != nil {
			t.Fatalf("read manifest: %v", err)
		}
		manifestText := string(manifestBytes)
		for _, want := range []string{`name="hello-react" runtime="bun"`, `name="@acme/hello-react"`, `tool(npm("typescript", "^5.9.0"))`, `runtime: "node"`, `command: ["vite", "build"]`, `strategy: "manual"`} {
			if !strings.Contains(manifestText, want) {
				t.Fatalf("manifest missing %q:\n%s", want, manifestText)
			}
		}

		packageBytes, err := os.ReadFile(filepath.Join(root, "package.json"))
		if err != nil {
			t.Fatalf("read package.json: %v", err)
		}
		if strings.Contains(string(packageBytes), "postinstall") || strings.Contains(string(packageBytes), "prepare") {
			t.Fatalf("package.json should not include lifecycle scripts:\n%s", string(packageBytes))
		}
	})

	t.Run("react-library template generation", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command(testTspackBinary, "init", "--root", root, "--template", "react-library", "--name", "ui-kit", "--package", "@local/ui-kit", "--runtime", "bun")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("react-library init failed: %v\n%s", err, string(b))
		}
		for _, want := range []string{"react.library", "vite.library", "package.exports", "tspack.pack"} {
			if !strings.Contains(string(b), want) {
				t.Fatalf("init output missing concept %q:\n%s", want, string(b))
			}
		}
		for _, rel := range []string{"manifest.tsx", ".tspack-version", "tsconfig.tspack.json", ".vscode/settings.json", "tsconfig.json", "tsconfig.build.json", "biome.json", "vite.config.ts", "package.json", "src/index.ts", "src/Button.tsx", "src/style.css", "README.md"} {
			if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
				t.Fatalf("missing generated file %s: %v", rel, err)
			}
		}
		assertGeneratedTSPackTSConfig(t, root)
		assertGeneratedVSCodeSettings(t, root, "node_modules/typescript/lib")
		assertGeneratedReactAppTSConfig(t, root)

		manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.tsx"))
		if err != nil {
			t.Fatalf("read manifest: %v", err)
		}
		manifestText := string(manifestBytes)
		for _, want := range []string{`name="ui-kit" runtime="bun"`, `name="@local/ui-kit"`, `kind="library"`, `tool(npm("typescript", "^5.9.0"))`, `peer(npm("react"`, `peers: [deps.react, deps.reactDom]`, `<Publish include={["dist/**", "README.md", "package.json"]}`, `command: ["vite", "build"]`, `command: ["tsc", "-p", "tsconfig.build.json", "--listEmittedFiles"]`} {
			if !strings.Contains(manifestText, want) {
				t.Fatalf("manifest missing %q:\n%s", want, manifestText)
			}
		}

		packageBytes, err := os.ReadFile(filepath.Join(root, "package.json"))
		if err != nil {
			t.Fatalf("read package.json: %v", err)
		}
		packageText := string(packageBytes)
		for _, want := range []string{`"name": "@local/ui-kit"`, `"private": true`, `"exports"`, `"peerDependencies"`, `"react-dom"`} {
			if !strings.Contains(packageText, want) {
				t.Fatalf("package.json missing %q:\n%s", want, packageText)
			}
		}
		for _, forbidden := range []string{"postinstall", "prepare", "prepublish"} {
			if strings.Contains(packageText, forbidden) {
				t.Fatalf("package.json should not include lifecycle script %q:\n%s", forbidden, packageText)
			}
		}
	})

	t.Run("react template rejects invalid runtime", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command(testTspackBinary, "init", "--root", root, "--template", "react", "--name", "hello-react", "--runtime", "npm")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(b), "TSPACK_TEMPLATE_VARIABLE_INVALID") {
			t.Fatalf("expected invalid runtime diagnostic: %v\n%s", err, string(b))
		}
	})

	t.Run("invalid kind", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command(testTspackBinary, "init", "--root", root, "--kind", "service", "--name", "acme-demo")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(b), "TSPACK_INIT_INVALID_KIND") {
			t.Fatalf("expected invalid kind diagnostic: %v\n%s", err, string(b))
		}
	})

	t.Run("missing name", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command(testTspackBinary, "init", "--root", root, "--kind", "library")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(b), "TSPACK_INIT_NAME_REQUIRED") {
			t.Fatalf("expected missing name diagnostic: %v\n%s", err, string(b))
		}
	})

	t.Run("invalid package name", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command(testTspackBinary, "init", "--root", root, "--kind", "library", "--name", "Bad Name")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(b), "TSPACK_INIT_INVALID_NAME") {
			t.Fatalf("expected invalid name diagnostic: %v\n%s", err, string(b))
		}
	})

	t.Run("invalid version", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command(testTspackBinary, "init", "--root", root, "--kind", "library", "--name", "acme-demo", "--version", "1")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(b), "TSPACK_INIT_INVALID_VERSION") {
			t.Fatalf("expected invalid version diagnostic: %v\n%s", err, string(b))
		}
	})

	t.Run("unsupported flags", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command(testTspackBinary, "init", "--root", root, "--kind", "library", "--name", "demo", "--target", "core")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(b), "TSPACK_INIT_UNSUPPORTED_FLAG") {
			t.Fatalf("expected unsupported flag diagnostic: %v\n%s", err, string(b))
		}

		cmd = exec.Command(testTspackBinary, "init", "--root", root, "--kind", "app", "--name", "demo", "--runtime-target")
		cmd.Dir = repo
		b, err = cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(b), "TSPACK_INIT_UNSUPPORTED_FLAG") {
			t.Fatalf("expected unsupported runtime-target diagnostic: %v\n%s", err, string(b))
		}
	})

	t.Run("library write and force", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command(testTspackBinary, "init", "--root", root, "--kind", "library", "--name", "@acme/widgets")
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
		if _, err := os.Stat(filepath.Join(root, ".tspack", "types", "tspack-manifest.d.ts")); err != nil {
			t.Fatalf("missing manifest type support: %v", err)
		}
		if _, err := os.Stat(filepath.Join(root, ".tspack", "types", "tspack-xtest.d.ts")); err != nil {
			t.Fatalf("missing xtest type support: %v", err)
		}
		assertGeneratedTSPackTSConfig(t, root)
		if !strings.Contains(string(b), "Generated tsconfig.tspack.json for TSPack manifest editor support") {
			t.Fatalf("init output missing editor support note: %s", string(b))
		}
		if _, err := os.Stat(filepath.Join(root, "tspack-env.d.ts")); err != nil {
			t.Fatalf("missing tspack env type support: %v", err)
		}
		biomeBytes, err := os.ReadFile(filepath.Join(root, "biome.json"))
		if err != nil {
			t.Fatalf("missing generated biome config: %v", err)
		}
		var biomeConfig map[string]any
		if err := json.Unmarshal(biomeBytes, &biomeConfig); err != nil {
			t.Fatalf("generated biome config should be valid JSON: %v", err)
		}
		includeValues, ok := biomeConfig["files"].(map[string]any)["includes"].([]any)
		if !ok {
			t.Fatalf("generated biome config missing files.includes: %#v", biomeConfig["files"])
		}
		includeSet := map[string]bool{}
		for _, value := range includeValues {
			includeSet[value.(string)] = true
		}
		for _, want := range []string{".tspack/**", "node_modules/**", "dist/**", "tspack-artifacts/**"} {
			exclusion := "!" + want
			if !includeSet[exclusion] {
				t.Fatalf("generated biome config missing exclusion %q in %#v", exclusion, includeValues)
			}
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

		cmd = exec.Command(testTspackBinary, "init", "--root", root, "--kind", "library", "--name", "@acme/widgets")
		cmd.Dir = repo
		b, err = cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(b), "TSPACK_INIT_FILE_EXISTS") {
			t.Fatalf("expected file exists diagnostic: %v\n%s", err, string(b))
		}

		_ = os.WriteFile(srcPath, []byte("custom\n"), 0o644)
		cmd = exec.Command(testTspackBinary, "init", "--root", root, "--kind", "library", "--name", "@acme/widgets", "--force")
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
		cmd := exec.Command(testTspackBinary, "init", "--root", root, "--kind", "app", "--name", "acme-demo", "--dry-run")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("dry run failed: %v\n%s", err, string(b))
		}
		if _, err := os.Stat(filepath.Join(root, "manifest.tsx")); !os.IsNotExist(err) {
			t.Fatalf("dry-run should not write files")
		}
		if !strings.Contains(string(b), "Would write") || !strings.Contains(string(b), "src/main.ts") || !strings.Contains(string(b), ".tspack/types/tspack-manifest.d.ts") || !strings.Contains(string(b), ".tspack/types/tspack-xtest.d.ts") || !strings.Contains(string(b), "tsconfig.tspack.json") || !strings.Contains(string(b), "tspack-env.d.ts") {
			t.Fatalf("dry-run output missing file list: %s", string(b))
		}

		cmd = exec.Command(testTspackBinary, "init", "--root", root, "--kind", "app", "--name", "acme-demo")
		cmd.Dir = repo
		b, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("init app failed: %v\n%s", err, string(b))
		}
		manifestPath := filepath.Join(root, "manifest.tsx")
		if _, err := os.Stat(filepath.Join(root, ".tspack", "types", "tspack-manifest.d.ts")); err != nil {
			t.Fatalf("missing manifest type support: %v", err)
		}
		if _, err := os.Stat(filepath.Join(root, ".tspack", "types", "tspack-xtest.d.ts")); err != nil {
			t.Fatalf("missing xtest type support: %v", err)
		}
		assertGeneratedTSPackTSConfig(t, root)
		if _, err := os.Stat(filepath.Join(root, "tspack-env.d.ts")); err != nil {
			t.Fatalf("missing tspack env type support: %v", err)
		}
		biomeBytes, err := os.ReadFile(filepath.Join(root, "biome.json"))
		if err != nil {
			t.Fatalf("missing generated biome config: %v", err)
		}
		var biomeConfig map[string]any
		if err := json.Unmarshal(biomeBytes, &biomeConfig); err != nil {
			t.Fatalf("generated biome config should be valid JSON: %v", err)
		}
		includeValues, ok := biomeConfig["files"].(map[string]any)["includes"].([]any)
		if !ok {
			t.Fatalf("generated biome config missing files.includes: %#v", biomeConfig["files"])
		}
		includeSet := map[string]bool{}
		for _, value := range includeValues {
			includeSet[value.(string)] = true
		}
		for _, want := range []string{".tspack/**", "node_modules/**", "dist/**", "tspack-artifacts/**"} {
			exclusion := "!" + want
			if !includeSet[exclusion] {
				t.Fatalf("generated biome config missing exclusion %q in %#v", exclusion, includeValues)
			}
		}
		text, _ := os.ReadFile(manifestPath)
		m := string(text)
		for _, want := range []string{"kind=\"app\"", "name: \"app\"", "missingTypes: \"ignore\"", "declarations: \"optional\""} {
			if !strings.Contains(m, want) {
				t.Fatalf("app manifest missing %q\n%s", want, m)
			}
		}
	})

	t.Run("existing tsconfig is left unchanged with note", func(t *testing.T) {
		root := t.TempDir()
		existingTSConfig := `{
  "compilerOptions": {
    "target": "ES2020"
  },
  "include": ["**/*.tsx"]
}
`
		if err := os.WriteFile(filepath.Join(root, "tsconfig.json"), []byte(existingTSConfig), 0o644); err != nil {
			t.Fatalf("write existing tsconfig: %v", err)
		}

		cmd := exec.Command(testTspackBinary, "init", "--root", root, "--kind", "app", "--name", "acme-demo")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("init with existing tsconfig failed: %v\n%s", err, string(b))
		}

		afterBytes, err := os.ReadFile(filepath.Join(root, "tsconfig.json"))
		if err != nil {
			t.Fatalf("read existing tsconfig: %v", err)
		}
		if string(afterBytes) != existingTSConfig {
			t.Fatalf("existing tsconfig should be left unchanged; got:\n%s", string(afterBytes))
		}
		if !strings.Contains(string(b), "Existing tsconfig.json was left unchanged") {
			t.Fatalf("init output missing existing tsconfig note: %s", string(b))
		}
		assertGeneratedTSPackTSConfig(t, root)
	})

	t.Run("generated manifests parse through frontend and validate", func(t *testing.T) {
		frontendCLIPath := filepath.Join(repo, "manifest-frontend", "dist", "cli.js")
		if _, err := os.Stat(frontendCLIPath); err != nil {
			t.Skipf("frontend CLI not built: %v", err)
		}

		cases := []struct {
			kind             string
			name             string
			version          string
			targetName       string
			wantDeclarations string
		}{
			{
				kind:             "library",
				name:             "@acme/widgets",
				version:          "0.1.0",
				targetName:       "core",
				wantDeclarations: "required",
			},
			{
				kind:             "app",
				name:             "acme-demo",
				version:          "0.1.0",
				targetName:       "app",
				wantDeclarations: "optional",
			},
		}

		for _, tc := range cases {
			tc := tc
			t.Run(tc.kind, func(t *testing.T) {
				root := t.TempDir()
				cmd := exec.Command(testTspackBinary, "init", "--root", root, "--kind", tc.kind, "--name", tc.name, "--version", tc.version)
				cmd.Dir = repo
				b, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("init failed: %v\n%s", err, string(b))
				}

				if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
					t.Fatalf("mkdir dist: %v", err)
				}
				if err := os.WriteFile(filepath.Join(root, "dist", "index.d.ts"), []byte("export declare const value: string;\n"), 0o644); err != nil {
					t.Fatalf("write declaration output: %v", err)
				}

				opts := project.DefaultOptions(root)
				opts.FrontendCLIPath = frontendCLIPath
				result := project.Check(opts)
				for _, d := range result.Diagnostics {
					if d.Severity == "error" {
						t.Fatalf("manifest frontend/IR validation failed: %#v", result.Diagnostics)
					}
				}

				rawIR, err := loadManifestIR(opts)
				if err != nil {
					t.Fatalf("frontend IR diagnostics: %v", err)
				}
				packages, ok := rawIR["packages"].([]any)
				if !ok || len(packages) != 1 {
					t.Fatalf("expected one package, got %#v", rawIR["packages"])
				}
				pkg, ok := packages[0].(map[string]any)
				if !ok {
					t.Fatalf("package should be object: %#v", packages[0])
				}
				if pkg["name"] != tc.name {
					t.Fatalf("unexpected package name: %#v", pkg["name"])
				}
				if pkg["version"] != tc.version {
					t.Fatalf("unexpected package version: %#v", pkg["version"])
				}
				if pkg["kind"] != tc.kind {
					t.Fatalf("unexpected package kind: %#v", pkg["kind"])
				}
				targets, ok := pkg["targets"].([]any)
				if !ok || len(targets) != 1 {
					t.Fatalf("unexpected target set: %#v", pkg["targets"])
				}
				target, ok := targets[0].(map[string]any)
				if !ok || target["name"] != tc.targetName {
					t.Fatalf("unexpected target set: %#v", targets)
				}
				policies, ok := pkg["policies"].(map[string]any)
				if !ok {
					t.Fatalf("missing policies: %#v", pkg["policies"])
				}
				typePolicy, ok := policies["types"].(map[string]any)
				if !ok || typePolicy["declarations"] != tc.wantDeclarations {
					t.Fatalf("unexpected declarations policy: %#v", typePolicy)
				}
				boundariesPolicy, ok := policies["boundaries"].(map[string]any)
				if !ok || boundariesPolicy["crossTargetImports"] == "" {
					t.Fatalf("boundaries policy should be present: %#v", boundariesPolicy)
				}
			})
		}
	})
}

func TestInitManifestTypeSupportDriftAndTypecheck(t *testing.T) {
	repo := filepath.Join("..", "..")
	canonicalPath := filepath.Join(repo, "manifest-frontend", "src", "tspack-manifest.d.ts")
	canonical, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical declaration: %v", err)
	}
	if normalizeManifestTypeDecl(string(canonical)) != normalizeManifestTypeDecl(initManifestTypesDTS) {
		t.Fatalf("init type support drifted from canonical declaration")
	}
	xtestCanonicalPath := filepath.Join(repo, "manifest-frontend", "src", "tspack-xtest.d.ts")
	xtestCanonical, err := os.ReadFile(xtestCanonicalPath)
	if err != nil {
		t.Fatalf("read canonical xtest declaration: %v", err)
	}
	if normalizeManifestTypeDecl(string(xtestCanonical)) != normalizeManifestTypeDecl(initXTestTypesDTS) {
		t.Fatalf("init xtest type support drifted from canonical declaration")
	}

	cases := []struct {
		kind string
		name string
	}{
		{kind: "library", name: "@acme/widgets"},
		{kind: "app", name: "acme-demo"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.kind, func(t *testing.T) {
			root := t.TempDir()
			cmd := exec.Command(testTspackBinary, "init", "--root", root, "--kind", tc.kind, "--name", tc.name)
			cmd.Dir = repo
			b, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("init failed: %v\n%s", err, string(b))
			}
			manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.tsx"))
			if err != nil {
				t.Fatalf("read manifest: %v", err)
			}
			typeBytes, err := os.ReadFile(filepath.Join(root, ".tspack", "types", "tspack-manifest.d.ts"))
			if err != nil {
				t.Fatalf("read manifest declarations: %v", err)
			}
			xtestTypeBytes, err := os.ReadFile(filepath.Join(root, ".tspack", "types", "tspack-xtest.d.ts"))
			if err != nil {
				t.Fatalf("read xtest declarations: %v", err)
			}
			if !strings.Contains(string(manifestBytes), `from "tspack/manifest"`) {
				t.Fatalf("generated manifest should import tspack/manifest:\n%s", string(manifestBytes))
			}
			if !strings.Contains(string(typeBytes), "declare module 'tspack/manifest'") && !strings.Contains(string(typeBytes), `declare module "tspack/manifest"`) {
				t.Fatalf("generated type surface should declare tspack/manifest module")
			}
			if strings.Contains(string(xtestTypeBytes), "interface IntrinsicElements") {
				t.Fatalf("materialized xtest declarations must not declare JSX.IntrinsicElements")
			}
			xtestPath := filepath.Join(root, "src", "Button.xtest.tsx")
			if err := os.MkdirAll(filepath.Dir(xtestPath), 0o755); err != nil {
				t.Fatalf("mkdir xtest dir: %v", err)
			}
			if err := os.WriteFile(xtestPath, []byte(`export default (
  <Suite name="button">
    <Fact name="renders">{() => {
      expect(true).toBe(true).because("boolean stays true");
      assert.ok(true, "truthy value is allowed");
    }}</Fact>
    <Fact name="can skip">{() => {
      skip("example");
    }}</Fact>
  </Suite>
);
`), 0o644); err != nil {
				t.Fatalf("write xtest fixture: %v", err)
			}
			tscPath, err := filepath.Abs(filepath.Join(repo, "manifest-frontend", "node_modules", "typescript", "bin", "tsc"))
			if err != nil {
				t.Fatalf("resolve tsc path: %v", err)
			}
			tsc := exec.Command("node", tscPath, "--project", filepath.Join(root, "tsconfig.tspack.json"), "--noEmit")
			tsc.Dir = repo
			out, err := tsc.CombinedOutput()
			if err != nil {
				t.Fatalf("tsc typecheck failed: %v\n%s", err, string(out))
			}
		})
	}
}

func TestTemplateManifestTypeSupportDrift(t *testing.T) {
	repo := filepath.Join("..", "..")
	canonicalPath := filepath.Join(repo, "manifest-frontend", "src", "tspack-manifest.d.ts")
	canonical, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical declaration: %v", err)
	}
	xtestCanonicalPath := filepath.Join(repo, "manifest-frontend", "src", "tspack-xtest.d.ts")
	xtestCanonical, err := os.ReadFile(xtestCanonicalPath)
	if err != nil {
		t.Fatalf("read canonical xtest declaration: %v", err)
	}

	templatePaths := []string{
		filepath.Join(repo, "internal", "templates", "builtin", "static", "files", "tspack-types", "tspack-manifest.d.ts"),
		filepath.Join(repo, "internal", "templates", "builtin", "react", "files", "tspack-types", "tspack-manifest.d.ts"),
		filepath.Join(repo, "internal", "templates", "builtin", "react-library", "files", "tspack-types", "tspack-manifest.d.ts"),
		filepath.Join(repo, "internal", "templates", "testdata", "local-concepts", "concept-manifest-app", "files", "tspack-types", "tspack-manifest.d.ts"),
		filepath.Join(repo, "internal", "templates", "testdata", "local-concepts", "machina-react-app", "files", "tspack-types", "tspack-manifest.d.ts"),
		filepath.Join(repo, "internal", "templates", "testdata", "local-concepts", "tailwind-machina-react-app", "files", "tspack-types", "tspack-manifest.d.ts"),
		filepath.Join(repo, "internal", "templates", "testdata", "local-concepts", "tailwind-react-app", "files", "tspack-types", "tspack-manifest.d.ts"),
	}
	xtestTemplatePaths := []string{
		filepath.Join(repo, "internal", "templates", "builtin", "static", "files", "tspack-types", "tspack-xtest.d.ts"),
		filepath.Join(repo, "internal", "templates", "builtin", "react", "files", "tspack-types", "tspack-xtest.d.ts"),
		filepath.Join(repo, "internal", "templates", "builtin", "react-library", "files", "tspack-types", "tspack-xtest.d.ts"),
		filepath.Join(repo, "internal", "templates", "testdata", "local-concepts", "concept-manifest-app", "files", "tspack-types", "tspack-xtest.d.ts"),
		filepath.Join(repo, "internal", "templates", "testdata", "local-concepts", "machina-react-app", "files", "tspack-types", "tspack-xtest.d.ts"),
		filepath.Join(repo, "internal", "templates", "testdata", "local-concepts", "tailwind-machina-react-app", "files", "tspack-types", "tspack-xtest.d.ts"),
		filepath.Join(repo, "internal", "templates", "testdata", "local-concepts", "tailwind-react-app", "files", "tspack-types", "tspack-xtest.d.ts"),
	}

	for _, templatePath := range templatePaths {
		templateBytes, err := os.ReadFile(templatePath)
		if err != nil {
			t.Fatalf("read template declaration %s: %v", templatePath, err)
		}
		if normalizeManifestTypeDecl(string(templateBytes)) != normalizeManifestTypeDecl(string(canonical)) {
			t.Fatalf("template declaration drifted from canonical: %s", templatePath)
		}
	}
	for _, templatePath := range xtestTemplatePaths {
		templateBytes, err := os.ReadFile(templatePath)
		if err != nil {
			t.Fatalf("read template xtest declaration %s: %v", templatePath, err)
		}
		if normalizeManifestTypeDecl(string(templateBytes)) != normalizeManifestTypeDecl(string(xtestCanonical)) {
			t.Fatalf("template xtest declaration drifted from canonical: %s", templatePath)
		}
	}
}

func normalizeManifestTypeDecl(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.TrimSuffix(value, "\n")
}

func runManifestEditorTypecheck(t *testing.T, repo string, root string, configName string) {
	t.Helper()

	tscPath, err := filepath.Abs(filepath.Join(repo, "manifest-frontend", "node_modules", "typescript", "bin", "tsc"))
	if err != nil {
		t.Fatalf("resolve tsc path: %v", err)
	}

	tsc := exec.Command("node", tscPath, "--project", filepath.Join(root, configName), "--noEmit")
	tsc.Dir = repo
	out, err := tsc.CombinedOutput()
	if err != nil {
		t.Fatalf("tsc manifest editor typecheck failed: %v\n%s", err, string(out))
	}
}

func runManifestEditorListFiles(t *testing.T, repo string, root string, configName string) []string {
	t.Helper()

	tscPath, err := filepath.Abs(filepath.Join(repo, "manifest-frontend", "node_modules", "typescript", "bin", "tsc"))
	if err != nil {
		t.Fatalf("resolve tsc path: %v", err)
	}

	tsc := exec.Command("node", tscPath, "--project", filepath.Join(root, configName), "--noEmit", "--listFiles")
	tsc.Dir = repo
	out, err := tsc.CombinedOutput()
	if err != nil {
		t.Fatalf("tsc manifest editor listFiles failed: %v\n%s", err, string(out))
	}

	lines := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
	files := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		files = append(files, filepath.Clean(trimmed))
	}
	return files
}

func writeManifestEditorFixtureFiles(t *testing.T, root string) {
	t.Helper()

	xtestPath := filepath.Join(root, "src", "Button.xtest.tsx")
	if err := os.MkdirAll(filepath.Dir(xtestPath), 0o755); err != nil {
		t.Fatalf("mkdir xtest dir: %v", err)
	}
	if err := os.WriteFile(xtestPath, []byte(`export default (
  <Suite name="button">
    <Fact name="renders">{() => {
      expect(true).toBe(true).because("boolean stays true");
      assert.ok(true, "truthy value is allowed");
    }}</Fact>
    <Fact name="can skip">{() => {
      skip("example");
    }}</Fact>
  </Suite>
);
`), 0o644); err != nil {
		t.Fatalf("write xtest fixture: %v", err)
	}

	appPath := filepath.Join(root, "src", "App.tsx")
	if err := os.WriteFile(appPath, []byte(`export default function App() {
  return <div>app</div>;
}
`), 0o644); err != nil {
		t.Fatalf("write app fixture: %v", err)
	}
}

func assertListContainsPath(t *testing.T, files []string, path string) {
	t.Helper()

	target := filepath.Clean(path)
	for _, file := range files {
		if filepath.Clean(file) == target {
			return
		}
	}
	t.Fatalf("expected listFiles to include %s\nfiles:\n%s", target, strings.Join(files, "\n"))
}

func assertListDoesNotContainPath(t *testing.T, files []string, path string) {
	t.Helper()

	target := filepath.Clean(path)
	for _, file := range files {
		if filepath.Clean(file) == target {
			t.Fatalf("expected listFiles to exclude %s\nfiles:\n%s", target, strings.Join(files, "\n"))
		}
	}
}

func assertGeneratedReactAppTSConfig(t *testing.T, root string) {
	t.Helper()

	configBytes, err := os.ReadFile(filepath.Join(root, "tsconfig.json"))
	if err != nil {
		t.Fatalf("missing generated app tsconfig: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(configBytes, &config); err != nil {
		t.Fatalf("generated app tsconfig should parse as JSON: %v", err)
	}

	compilerOptions, ok := config["compilerOptions"].(map[string]any)
	if !ok {
		t.Fatalf("generated app tsconfig missing compilerOptions: %#v", config)
	}
	if compilerOptions["jsx"] != "react-jsx" {
		t.Fatalf("generated app tsconfig should use React JSX, got %#v", compilerOptions["jsx"])
	}
	if compilerOptions["moduleResolution"] != "Bundler" {
		t.Fatalf("generated app tsconfig should use bundler resolution, got %#v", compilerOptions["moduleResolution"])
	}
	if compilerOptions["noEmit"] != true || compilerOptions["strict"] != true {
		t.Fatalf("generated app tsconfig should be strict/noEmit: %#v", compilerOptions)
	}

	excludeSet := jsonStringArraySet(t, config["exclude"])
	for _, want := range []string{"manifest.tsx", "package.manifest.tsx", "**/*.manifest.tsx", "**/*.xtest.tsx", ".tspack/**", "tspack-artifacts/**", "dist/**"} {
		if !excludeSet[want] {
			t.Fatalf("generated app tsconfig missing exclude %q in %#v", want, excludeSet)
		}
	}
}

func assertGeneratedTSPackTSConfig(t *testing.T, root string) {
	t.Helper()

	configBytes, err := os.ReadFile(filepath.Join(root, "tsconfig.tspack.json"))
	if err != nil {
		t.Fatalf("missing generated TSPack tsconfig: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(configBytes, &config); err != nil {
		t.Fatalf("generated TSPack tsconfig should parse as JSON: %v", err)
	}

	compilerOptions, ok := config["compilerOptions"].(map[string]any)
	if !ok {
		t.Fatalf("generated TSPack tsconfig missing compilerOptions: %#v", config)
	}
	if compilerOptions["jsx"] != "preserve" {
		t.Fatalf("generated TSPack tsconfig should preserve JSX, got %#v", compilerOptions["jsx"])
	}
	typesValues, ok := compilerOptions["types"].([]any)
	if !ok {
		t.Fatalf("generated TSPack tsconfig should set compilerOptions.types to an empty array: %#v", compilerOptions["types"])
	}
	if len(typesValues) != 0 {
		t.Fatalf("generated TSPack tsconfig should isolate ambient @types packages with an empty types array, got %#v", typesValues)
	}
	if compilerOptions["ignoreDeprecations"] != "5.0" {
		t.Fatalf("generated TSPack tsconfig should deliberately guard baseUrl deprecation warnings, got %#v", compilerOptions["ignoreDeprecations"])
	}
	if compilerOptions["baseUrl"] != "." {
		t.Fatalf("generated TSPack tsconfig should keep baseUrl for non-relative paths mapping, got %#v", compilerOptions["baseUrl"])
	}
	paths, ok := compilerOptions["paths"].(map[string]any)
	if !ok {
		t.Fatalf("generated TSPack tsconfig missing paths: %#v", compilerOptions)
	}
	manifestPathValues, ok := paths["tspack/manifest"].([]any)
	if !ok || len(manifestPathValues) != 1 || manifestPathValues[0] != ".tspack/types/tspack-manifest.d.ts" {
		t.Fatalf("generated TSPack tsconfig should map tspack/manifest to local declarations: %#v", paths["tspack/manifest"])
	}

	includeSet := jsonStringArraySet(t, config["include"])
	for _, want := range []string{"manifest.tsx", "package.manifest.tsx", "**/*.manifest.tsx", "**/*.xtest.tsx", ".tspack/types/**/*.d.ts"} {
		if !includeSet[want] {
			t.Fatalf("generated TSPack tsconfig missing include %q in %#v", want, includeSet)
		}
	}

	excludeSet := jsonStringArraySet(t, config["exclude"])
	for _, want := range []string{"dist/**", "node_modules/**", ".tspack/store/**", "tspack-artifacts/**"} {
		if !excludeSet[want] {
			t.Fatalf("generated TSPack tsconfig missing exclude %q in %#v", want, excludeSet)
		}
	}
	if excludeSet["src/**"] {
		t.Fatalf("generated TSPack tsconfig must not exclude src/** because that drops src/*.xtest.tsx")
	}
}

func assertGeneratedVSCodeSettings(t *testing.T, root string, wantTSDK string) {
	t.Helper()

	settingsBytes, err := os.ReadFile(filepath.Join(root, ".vscode", "settings.json"))
	if err != nil {
		t.Fatalf("missing generated VS Code settings: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(settingsBytes, &settings); err != nil {
		t.Fatalf("generated VS Code settings should parse as JSON: %v", err)
	}

	if settings["typescript.tsdk"] != wantTSDK {
		t.Fatalf("generated VS Code settings should point to %q, got %#v", wantTSDK, settings["typescript.tsdk"])
	}
	if settings["typescript.enablePromptUseWorkspaceTsdk"] != true {
		t.Fatalf("generated VS Code settings should enable the workspace TypeScript prompt, got %#v", settings["typescript.enablePromptUseWorkspaceTsdk"])
	}
}

func jsonStringArraySet(t *testing.T, value any) map[string]bool {
	t.Helper()

	values, ok := value.([]any)
	if !ok {
		t.Fatalf("expected JSON string array, got %#v", value)
	}
	set := map[string]bool{}
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			t.Fatalf("expected JSON string array member, got %#v", value)
		}
		set[text] = true
	}
	return set
}

func loadManifestIR(opts project.Options) (map[string]any, error) {
	cliPath := opts.FrontendCLIPath
	cmd := exec.Command("node", cliPath, opts.ManifestPath)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	type cliOutput struct {
		OK          bool            `json:"ok"`
		IR          json.RawMessage `json:"ir"`
		Diagnostics []any           `json:"diagnostics"`
	}
	var out cliOutput
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, fmt.Errorf("frontend returned diagnostics: %#v", out.Diagnostics)
	}
	var ir map[string]any
	if err := json.Unmarshal(out.IR, &ir); err != nil {
		return nil, err
	}
	return ir, nil
}

func TestInitAlongsideDryRunWritesNothing(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := copyDogfoodProject(t, repo)
	packageBefore := readFileString(t, filepath.Join(root, "package.json"))

	cmd := tspackTestCommand(t, repo, bin, "init", "--alongside", "--dry-run", "--root", root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("init alongside dry-run failed: %v\n%s", err, string(out))
	}
	text := string(out)
	for _, expected := range []string{"TSPack init --alongside dry run", "Would write:", "manifest.tsx", "TsConfig.manifestEditor()", "tspack compat write"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("dry-run output missing %q:\n%s", expected, text)
		}
	}
	assertPathMissing(t, filepath.Join(root, "manifest.tsx"))
	assertPathMissing(t, filepath.Join(root, "ts-lock.toml"))
	if got := readFileString(t, filepath.Join(root, "package.json")); got != packageBefore {
		t.Fatalf("package.json changed during dry-run")
	}
}

func TestInitAlongsideWritesOnlyManifest(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := copyDogfoodProject(t, repo)
	packageBefore := readFileString(t, filepath.Join(root, "package.json"))
	lockBefore := readFileString(t, filepath.Join(root, "package-lock.json"))

	cmd := tspackTestCommand(t, repo, bin, "init", "--alongside", "--root", root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("init alongside failed: %v\n%s", err, string(out))
	}
	manifestText := readFileString(t, filepath.Join(root, "manifest.tsx"))
	for _, expected := range []string{"Workspace name=\"incremental-existing-react\"", "TsConfig.manifestEditor()", "VSCode.settings()", "VSCode.extensions()"} {
		if !strings.Contains(manifestText, expected) {
			t.Fatalf("manifest missing %q:\n%s", expected, manifestText)
		}
	}
	if got := readFileString(t, filepath.Join(root, "package.json")); got != packageBefore {
		t.Fatalf("package.json changed during init alongside")
	}
	if got := readFileString(t, filepath.Join(root, "package-lock.json")); got != lockBefore {
		t.Fatalf("package-lock.json changed during init alongside")
	}
	assertPathMissing(t, filepath.Join(root, "ts-lock.toml"))
	assertPathMissing(t, filepath.Join(root, ".vscode"))
	assertPathMissing(t, filepath.Join(root, "tsconfig.tspack.json"))
}

func TestInitGeneratedManifestEditorTSConfigListFilesIsolation(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := t.TempDir()

	cmd := tspackTestCommand(t, repo, bin, "init", "--template", "react", "--name", "m64b-react", "--root", root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init failed: %v\n%s", err, string(out))
	}

	writeManifestEditorFixtureFiles(t, root)

	reactTypesDir := filepath.Join(root, "node_modules", "@types", "react")
	if err := os.MkdirAll(reactTypesDir, 0o755); err != nil {
		t.Fatalf("mkdir @types/react fixture: %v", err)
	}
	reactTypesPath := filepath.Join(reactTypesDir, "index.d.ts")
	if err := os.WriteFile(reactTypesPath, []byte(`declare namespace JSX {
  interface IntrinsicElements {
    div: { id?: string };
  }
}
`), 0o644); err != nil {
		t.Fatalf("write @types/react fixture: %v", err)
	}

	runManifestEditorTypecheck(t, repo, root, "tsconfig.tspack.json")
	files := runManifestEditorListFiles(t, repo, root, "tsconfig.tspack.json")

	assertListContainsPath(t, files, filepath.Join(root, "manifest.tsx"))
	assertListContainsPath(t, files, filepath.Join(root, ".tspack", "types", "tspack-manifest.d.ts"))
	assertListContainsPath(t, files, filepath.Join(root, ".tspack", "types", "tspack-xtest.d.ts"))
	assertListContainsPath(t, files, filepath.Join(root, "src", "Button.xtest.tsx"))
	assertListDoesNotContainPath(t, files, filepath.Join(root, "src", "App.tsx"))
	assertListDoesNotContainPath(t, files, reactTypesPath)
}

func TestInitAlongsideManifestEditorTSConfigListFilesIsolation(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := copyDogfoodProject(t, repo)

	initCmd := tspackTestCommand(t, repo, bin, "init", "--alongside", "--root", root)
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("init alongside failed: %v\n%s", err, string(out))
	}

	compatCmd := tspackTestCommand(t, repo, bin, "compat", "write", "--root", root)
	if out, err := compatCmd.CombinedOutput(); err != nil {
		t.Fatalf("compat write failed: %v\n%s", err, string(out))
	}

	writeManifestEditorFixtureFiles(t, root)
	files := runManifestEditorListFiles(t, repo, root, "tsconfig.tspack.json")

	assertListContainsPath(t, files, filepath.Join(root, "manifest.tsx"))
	assertListContainsPath(t, files, filepath.Join(root, "src", "Button.xtest.tsx"))
	assertListDoesNotContainPath(t, files, filepath.Join(root, "src", "App.tsx"))
}

func TestInitTSPackTSConfigConstantMatchesTemplateCopies(t *testing.T) {
	repo := repoRoot(t)
	expected := normalizeManifestTypeDecl(initTSPackTSConfigJSON)

	templatePaths := []string{
		filepath.Join(repo, "internal", "templates", "builtin", "static", "files", "tsconfig.tspack.json"),
		filepath.Join(repo, "internal", "templates", "builtin", "react", "files", "tsconfig.tspack.json"),
		filepath.Join(repo, "internal", "templates", "builtin", "react-library", "files", "tsconfig.tspack.json"),
		filepath.Join(repo, "internal", "templates", "testdata", "local-concepts", "concept-manifest-app", "files", "tsconfig.tspack.json"),
		filepath.Join(repo, "internal", "templates", "testdata", "local-concepts", "machina-react-app", "files", "tsconfig.tspack.json"),
		filepath.Join(repo, "internal", "templates", "testdata", "local-concepts", "tailwind-machina-react-app", "files", "tsconfig.tspack.json"),
		filepath.Join(repo, "internal", "templates", "testdata", "local-concepts", "tailwind-react-app", "files", "tsconfig.tspack.json"),
	}

	for _, templatePath := range templatePaths {
		templateBytes, err := os.ReadFile(templatePath)
		if err != nil {
			t.Fatalf("read template tsconfig %s: %v", templatePath, err)
		}
		if normalizeManifestTypeDecl(string(templateBytes)) != expected {
			t.Fatalf("template tsconfig drifted from initTSPackTSConfigJSON: %s", templatePath)
		}
	}
}

func TestInitAlongsideCompatAndAdoptTransition(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := copyDogfoodProject(t, repo)

	before := runCommandOutput(t, bin, "adopt", "--report", "--root", root)
	if !strings.Contains(before, "Suggested adoption mode: package-json-only") {
		t.Fatalf("adopt before init did not report package-json-only:\n%s", before)
	}
	runCommandOutput(t, bin, "init", "--alongside", "--root", root)
	after := runCommandOutput(t, bin, "adopt", "--report", "--root", root)
	for _, expected := range []string{"Suggested adoption mode: observe", "not TSPack RunTargets", "no ts-lock.toml yet"} {
		if !strings.Contains(after, expected) {
			t.Fatalf("adopt after init missing %q:\n%s", expected, after)
		}
	}
	diffCmd := tspackTestCommand(t, repo, bin, "compat", "diff", "--root", root)
	diffOut, err := diffCmd.CombinedOutput()
	if err == nil {
		t.Fatalf("compat diff before write unexpectedly succeeded:\n%s", string(diffOut))
	}
	for _, expected := range []string{"missing tsconfig.tspack.json", "missing .vscode/settings.json", "missing .vscode/extensions.json", "missing .tspack/types/tspack-manifest.d.ts", "missing .tspack/types/tspack-xtest.d.ts"} {
		if !strings.Contains(string(diffOut), expected) {
			t.Fatalf("compat diff missing %q:\n%s", expected, string(diffOut))
		}
	}
	runCommandOutput(t, bin, "compat", "write", "--root", root)
	runCommandOutput(t, bin, "compat", "diff", "--root", root)
	for _, path := range []string{"tsconfig.tspack.json", ".vscode/settings.json", ".vscode/extensions.json", ".tspack/types/tspack-manifest.d.ts", ".tspack/types/tspack-xtest.d.ts"} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("compat write did not create %s: %v", path, err)
		}
	}
	assertPathMissing(t, filepath.Join(root, "ts-lock.toml"))
}

func TestInitAlongsideManifestEditorTypecheck(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := copyDogfoodProject(t, repo)

	runCommandOutput(t, bin, "init", "--alongside", "--root", root)
	runCommandOutput(t, bin, "compat", "write", "--root", root)

	assertGeneratedVSCodeSettings(t, root, "node_modules/typescript/lib")
	runManifestEditorTypecheck(t, repo, root, "tsconfig.tspack.json")
}

func TestTemplateManifestEditorTypecheck(t *testing.T) {
	repo := repoRoot(t)

	cases := []struct {
		name string
		args []string
	}{
		{
			name: "static",
			args: []string{"run", "./cmd/tspack", "init", "--root", "", "--template", "static", "--name", "hello-static"},
		},
		{
			name: "react",
			args: []string{"run", "./cmd/tspack", "init", "--root", "", "--template", "react", "--name", "hello-react", "--package", "@acme/hello-react"},
		},
		{
			name: "react-library",
			args: []string{"run", "./cmd/tspack", "init", "--root", "", "--template", "react-library", "--name", "ui-kit", "--package", "@acme/ui-kit"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			args := append([]string{}, tc.args...)
			args[4] = root

			cmd := exec.Command("go", args...)
			cmd.Dir = repo
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("init %s failed: %v\n%s", tc.name, err, string(out))
			}

			assertGeneratedVSCodeSettings(t, root, "node_modules/typescript/lib")
			runManifestEditorTypecheck(t, repo, root, "tsconfig.tspack.json")
		})
	}
}

func TestPackageAnnotationManifestEditorTypecheck(t *testing.T) {
	repo := repoRoot(t)
	root := t.TempDir()

	sourcePath := filepath.Join(repo, "examples", "incremental-existing-monorepo", "packages", "ui", "package.manifest.tsx")
	contents, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read package annotation manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.manifest.tsx"), contents, 0o644); err != nil {
		t.Fatalf("write package annotation manifest: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".tspack", "types"), 0o755); err != nil {
		t.Fatalf("mkdir manifest types: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".tspack", "types", "tspack-manifest.d.ts"), []byte(initManifestTypesDTS), 0o644); err != nil {
		t.Fatalf("write manifest declarations: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".tspack", "types", "tspack-xtest.d.ts"), []byte(initXTestTypesDTS), 0o644); err != nil {
		t.Fatalf("write xtest declarations: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "tsconfig.tspack.json"), []byte(initTSPackTSConfigJSON), 0o644); err != nil {
		t.Fatalf("write tsconfig.tspack.json: %v", err)
	}

	runManifestEditorTypecheck(t, repo, root, "tsconfig.tspack.json")
}

func TestInitAlongsideExistingManifestForceAndPackageJSONErrors(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := copyDogfoodProject(t, repo)
	if err := os.WriteFile(filepath.Join(root, "manifest.tsx"), []byte("old manifest\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := tspackTestCommand(t, repo, bin, "init", "--alongside", "--root", root)
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "TSPACK_INIT_ALONGSIDE_MANIFEST_EXISTS") {
		t.Fatalf("existing manifest should fail clearly: %v\n%s", err, string(out))
	}
	runCommandOutput(t, bin, "init", "--alongside", "--force", "--root", root)
	if strings.Contains(readFileString(t, filepath.Join(root, "manifest.tsx")), "old manifest") {
		t.Fatalf("--force did not replace manifest")
	}

	missingRoot := t.TempDir()
	cmd = tspackTestCommand(t, repo, bin, "init", "--alongside", "--root", missingRoot)
	out, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "TSPACK_INIT_ALONGSIDE_REQUIRES_PACKAGE_JSON") {
		t.Fatalf("missing package.json should fail clearly: %v\n%s", err, string(out))
	}

	malformedRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(malformedRoot, "package.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd = tspackTestCommand(t, repo, bin, "init", "--alongside", "--root", malformedRoot)
	out, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "TSPACK_ADOPT_PACKAGE_JSON_MALFORMED") {
		t.Fatalf("malformed package.json should fail clearly: %v\n%s", err, string(out))
	}
}

func TestInitAlongsideDoesNotMakePackageScriptsRunTargets(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := copyDogfoodProject(t, repo)
	runCommandOutput(t, bin, "init", "--alongside", "--root", root)
	cmd := tspackTestCommand(t, repo, bin, "run", "build", "--root", root)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("tspack run build unexpectedly succeeded by falling back to package.json:\n%s", string(out))
	}
	if !strings.Contains(string(out), "TSPACK_RUN_TARGET_NOT_FOUND") && !strings.Contains(string(out), "target") {
		t.Fatalf("run failure did not look like missing manifest RunTarget:\n%s", string(out))
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected %s to be absent", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", path, err)
	}
}

func runCommandOutput(t *testing.T, bin string, args ...string) string {
	t.Helper()
	repo := repoRoot(t)
	cmd := tspackTestCommand(t, repo, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %s %s\n%v\n%s", bin, strings.Join(args, " "), err, string(out))
	}
	return string(out)
}

func tspackTestCommand(t *testing.T, repo string, bin string, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cliPath := filepath.Join(repo, "manifest-frontend", "dist", "cli.js")
	bridgeDir := filepath.Join(repo, "manifest-frontend", "dist")
	cmd.Env = append(os.Environ(),
		"TSPACK_MANIFEST_FRONTEND="+cliPath,
		"TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR="+bridgeDir,
	)
	return cmd
}
