package main

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

	t.Run("default static template without kind", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command("go", "run", "./cmd/tspack", "init", "--root", root, "--name", "acme-demo")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(b), `Created project from template "static"`) {
			t.Fatalf("expected default static template: %v\n%s", err, string(b))
		}
		if _, err := os.Stat(filepath.Join(root, "index.html")); err != nil {
			t.Fatalf("missing static index.html: %v", err)
		}
	})

	t.Run("list includes react template concepts", func(t *testing.T) {
		cmd := exec.Command("go", "run", "./cmd/tspack", "init", "--list-templates")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("list templates failed: %v\n%s", err, string(b))
		}
		for _, want := range []string{"static", "react", "react.app", "vite.app", "browser.spa"} {
			if !strings.Contains(string(b), want) {
				t.Fatalf("template list missing %q:\n%s", want, string(b))
			}
		}
	})

	t.Run("react template generation", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command("go", "run", "./cmd/tspack", "init", "--root", root, "--template", "react", "--name", "hello-react", "--package", "@acme/hello-react", "--runtime", "bun")
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
		for _, rel := range []string{"manifest.tsx", "tsconfig.tspack.json", "tsconfig.json", "biome.json", "vite.config.ts", "package.json", "index.html", "src/main.tsx", "src/App.tsx", "src/style.css", "README.md"} {
			if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
				t.Fatalf("missing generated file %s: %v", rel, err)
			}
		}
		assertGeneratedTSPackTSConfig(t, root)
		assertGeneratedReactAppTSConfig(t, root)

		manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.tsx"))
		if err != nil {
			t.Fatalf("read manifest: %v", err)
		}
		manifestText := string(manifestBytes)
		for _, want := range []string{`name="hello-react" runtime="bun"`, `name="@acme/hello-react"`, `runtime: "node"`, `command: ["vite", "build"]`, `strategy: "manual"`} {
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

	t.Run("react template rejects invalid runtime", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command("go", "run", "./cmd/tspack", "init", "--root", root, "--template", "react", "--name", "hello-react", "--runtime", "npm")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(b), "TSPACK_TEMPLATE_VARIABLE_INVALID") {
			t.Fatalf("expected invalid runtime diagnostic: %v\n%s", err, string(b))
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

	t.Run("unsupported flags", func(t *testing.T) {
		root := t.TempDir()
		cmd := exec.Command("go", "run", "./cmd/tspack", "init", "--root", root, "--kind", "library", "--name", "demo", "--target", "core")
		cmd.Dir = repo
		b, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(b), "TSPACK_INIT_UNSUPPORTED_FLAG") {
			t.Fatalf("expected unsupported flag diagnostic: %v\n%s", err, string(b))
		}

		cmd = exec.Command("go", "run", "./cmd/tspack", "init", "--root", root, "--kind", "app", "--name", "demo", "--runtime-target")
		cmd.Dir = repo
		b, err = cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(b), "TSPACK_INIT_UNSUPPORTED_FLAG") {
			t.Fatalf("expected unsupported runtime-target diagnostic: %v\n%s", err, string(b))
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
		if _, err := os.Stat(filepath.Join(root, ".tspack", "types", "tspack-manifest.d.ts")); err != nil {
			t.Fatalf("missing manifest type support: %v", err)
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
		ignoreValues, ok := biomeConfig["files"].(map[string]any)["ignore"].([]any)
		if !ok {
			t.Fatalf("generated biome config missing files.ignore: %#v", biomeConfig["files"])
		}
		ignoreSet := map[string]bool{}
		for _, value := range ignoreValues {
			ignoreSet[value.(string)] = true
		}
		for _, want := range []string{".tspack/**", "node_modules/**", "dist/**", "tspack-artifacts/**"} {
			if !ignoreSet[want] {
				t.Fatalf("generated biome config missing ignore %q in %#v", want, ignoreValues)
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
		if !strings.Contains(string(b), "Would write") || !strings.Contains(string(b), "src/main.ts") || !strings.Contains(string(b), ".tspack/types/tspack-manifest.d.ts") || !strings.Contains(string(b), "tsconfig.tspack.json") || !strings.Contains(string(b), "tspack-env.d.ts") {
			t.Fatalf("dry-run output missing file list: %s", string(b))
		}

		cmd = exec.Command("go", "run", "./cmd/tspack", "init", "--root", root, "--kind", "app", "--name", "acme-demo")
		cmd.Dir = repo
		b, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("init app failed: %v\n%s", err, string(b))
		}
		manifestPath := filepath.Join(root, "manifest.tsx")
		if _, err := os.Stat(filepath.Join(root, ".tspack", "types", "tspack-manifest.d.ts")); err != nil {
			t.Fatalf("missing manifest type support: %v", err)
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
		ignoreValues, ok := biomeConfig["files"].(map[string]any)["ignore"].([]any)
		if !ok {
			t.Fatalf("generated biome config missing files.ignore: %#v", biomeConfig["files"])
		}
		ignoreSet := map[string]bool{}
		for _, value := range ignoreValues {
			ignoreSet[value.(string)] = true
		}
		for _, want := range []string{".tspack/**", "node_modules/**", "dist/**", "tspack-artifacts/**"} {
			if !ignoreSet[want] {
				t.Fatalf("generated biome config missing ignore %q in %#v", want, ignoreValues)
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

		cmd := exec.Command("go", "run", "./cmd/tspack", "init", "--root", root, "--kind", "app", "--name", "acme-demo")
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
				cmd := exec.Command("go", "run", "./cmd/tspack", "init", "--root", root, "--kind", tc.kind, "--name", tc.name, "--version", tc.version)
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
	if string(canonical) != initManifestTypesDTS {
		t.Fatalf("init type support drifted from canonical declaration")
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
			cmd := exec.Command("go", "run", "./cmd/tspack", "init", "--root", root, "--kind", tc.kind, "--name", tc.name)
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
			if !strings.Contains(string(manifestBytes), `from "tspack/manifest"`) {
				t.Fatalf("generated manifest should import tspack/manifest:\n%s", string(manifestBytes))
			}
			if !strings.Contains(string(typeBytes), "declare module 'tspack/manifest'") && !strings.Contains(string(typeBytes), `declare module "tspack/manifest"`) {
				t.Fatalf("generated type surface should declare tspack/manifest module")
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
	for _, want := range []string{"src/**", "dist/**", "node_modules/**", ".tspack/store/**", "tspack-artifacts/**"} {
		if !excludeSet[want] {
			t.Fatalf("generated TSPack tsconfig missing exclude %q in %#v", want, excludeSet)
		}
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
