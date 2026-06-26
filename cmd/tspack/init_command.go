package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/templates"
)

var (
	initPackageNameRe = regexp.MustCompile(`^(?:@[a-z0-9._-]+/[a-z0-9._-]+|[a-z0-9._-]+)$`)
	initVersionRe     = regexp.MustCompile(`^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$`)
)

type initConfig struct {
	root          string
	kind          string
	name          string
	version       string
	license       string
	force         bool
	dryRun        bool
	template      string
	listTemplates bool
	packageName   string
	runtime       string
}

type plannedFile struct {
	path    string
	content string
}

func runInitCommand(args []string) {
	cfg, diags := parseInitArgs(args)
	if len(diags) > 0 {
		for _, d := range diags {
			fmt.Fprintln(os.Stderr, d)
		}
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  tspack init --template static --name acme-demo")
		fmt.Fprintln(os.Stderr, "  tspack init --template ./templates/app --force")
		os.Exit(1)
	}

	if cfg.listTemplates {
		renderTemplateList()
		return
	}

	if cfg.root == "/" {
		fmt.Fprintln(os.Stderr, "TSPACK_INIT_UNSAFE_ROOT: refusing to initialize at filesystem root")
		os.Exit(1)
	}
	if wd, err := filepath.Abs(cfg.root); err != nil || wd == "/" {
		fmt.Fprintln(os.Stderr, "TSPACK_INIT_UNSAFE_ROOT: refusing to initialize at filesystem root")
		os.Exit(1)
	}

	if cfg.template == "" && cfg.kind != "" {
		runLegacyInit(cfg)
		return
	}

	if err := runTemplateInit(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runLegacyInit(cfg initConfig) {
	files := buildInitFiles(cfg)
	if !cfg.force {
		for _, f := range files {
			fp := filepath.Join(cfg.root, f.path)
			if _, err := os.Stat(fp); err == nil {
				fmt.Fprintf(os.Stderr, "TSPACK_INIT_FILE_EXISTS: %s\n", f.path)
				os.Exit(1)
			}
		}
	}
	if cfg.dryRun {
		fmt.Println("TSPack init dry run")
		fmt.Println("Would write:")
		for _, f := range files {
			fmt.Printf("  %s\n", f.path)
		}
		return
	}
	for _, f := range files {
		fp := filepath.Join(cfg.root, f.path)
		if err := writeGeneratedFile(fp, f.content, cfg.force); err != nil {
			fmt.Fprintf(os.Stderr, "TSPACK_INIT_WRITE_FAILED: %s (%v)\n", f.path, err)
			os.Exit(1)
		}
	}
	fmt.Printf("Initialized TSPack %s package %q.\n", cfg.kind, cfg.name)
	fmt.Println("Wrote:")
	for _, f := range files {
		fmt.Printf("  %s\n", f.path)
	}
	fmt.Println("Generated tsconfig.tspack.json for TSPack manifest editor support.")
	fmt.Println("VS Code may need ‘TypeScript: Restart TS Server’ if it already had the project open.")
	if _, err := os.Stat(filepath.Join(cfg.root, "tsconfig.json")); err == nil {
		fmt.Println("Existing tsconfig.json was left unchanged. If your app TypeScript config includes manifest.tsx or *.xtest.tsx, exclude TSPack-owned files or use tsconfig.tspack.json for manifest editing.")
	}
}

func renderTemplateList() {
	builtins, err := templates.ListBuiltins()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Available templates:")
	for _, tmpl := range builtins {
		fmt.Printf("%s  %s  %s\n", tmpl.Name, tmpl.Kind, tmpl.Description)
		fmt.Printf("concepts: %s\n", strings.Join(tmpl.Concepts, ", "))
	}
}

func runTemplateInit(cfg initConfig) error {
	templateName := cfg.template
	if templateName == "" {
		templateName = "static"
	}
	tmpl, err := templates.Load(templateName)
	if err != nil {
		return err
	}
	projectName := cfg.name
	if projectName == "" {
		if absRoot, err := filepath.Abs(cfg.root); err == nil {
			projectName = filepath.Base(absRoot)
		}
	}
	overrides := map[string]string{
		"projectName": projectName,
		"packageName": cfg.packageName,
		"runtime":     cfg.runtime,
	}
	if overrides["packageName"] == "" {
		overrides["packageName"] = projectName
	}
	values, err := tmpl.ResolveValues(overrides)
	if err != nil {
		return err
	}
	planned, err := tmpl.Apply(templates.ApplyOptions{Destination: cfg.root, Values: values, Force: cfg.force, DryRun: cfg.dryRun})
	if err != nil {
		return err
	}
	if cfg.dryRun {
		fmt.Printf("TSPack init dry run for template %q\n", tmpl.Name)
		fmt.Println("Would write:")
		for _, f := range planned {
			fmt.Printf("  %s\n", f.Path)
		}
		return nil
	}
	fmt.Printf("Created project from template %q.\n", tmpl.Name)
	fmt.Println("Concepts:")
	for _, concept := range tmpl.Concepts {
		fmt.Println(concept)
	}
	fmt.Println("Next steps:")
	fmt.Println("tspack update")
	fmt.Println("tspack sync")
	fmt.Println("tspack run dev")
	fmt.Println("tspack check")
	fmt.Println("Generated tsconfig.tspack.json for manifest editor support.")
	fmt.Println("If VS Code was already open, run \"TypeScript: Restart TS Server\".")
	if _, err := os.Stat(filepath.Join(cfg.root, "tsconfig.json")); err == nil {
		fmt.Println("Existing tsconfig.json was left unchanged. If your app TypeScript config includes manifest.tsx or *.xtest.tsx, exclude TSPack-owned files or use tsconfig.tspack.json for manifest editing.")
	}
	return nil
}

func parseInitArgs(args []string) (initConfig, []string) {
	cfg := initConfig{root: ".", version: "0.1.0", license: "MIT"}
	var diags []string
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--root":
			i++
			if i >= len(args) {
				diags = append(diags, "TSPACK_INIT_WRITE_FAILED: --root requires a value")
				continue
			}
			cfg.root = args[i]
		case "--template":
			i++
			if i >= len(args) {
				diags = append(diags, "TSPACK_TEMPLATE_NOT_FOUND: --template requires a value")
				continue
			}
			cfg.template = args[i]
		case "--list-templates":
			cfg.listTemplates = true
		case "--package":
			i++
			if i >= len(args) {
				diags = append(diags, "TSPACK_TEMPLATE_VARIABLE_MISSING: --package requires a value")
				continue
			}
			cfg.packageName = args[i]
		case "--runtime":
			i++
			if i >= len(args) {
				diags = append(diags, "TSPACK_TEMPLATE_VARIABLE_MISSING: --runtime requires a value")
				continue
			}
			cfg.runtime = args[i]
		case "--kind":
			i++
			if i >= len(args) {
				diags = append(diags, "TSPACK_INIT_KIND_REQUIRED: --kind requires a value")
				continue
			}
			cfg.kind = args[i]
		case "--name":
			i++
			if i >= len(args) {
				diags = append(diags, "TSPACK_INIT_NAME_REQUIRED: --name requires a value")
				continue
			}
			cfg.name = args[i]
		case "--version":
			i++
			if i >= len(args) {
				diags = append(diags, "TSPACK_INIT_INVALID_VERSION: --version requires a value")
				continue
			}
			cfg.version = args[i]
		case "--license":
			i++
			if i >= len(args) {
				diags = append(diags, "TSPACK_INIT_WRITE_FAILED: --license requires a value")
				continue
			}
			cfg.license = args[i]
		case "--force":
			cfg.force = true
		case "--dry-run":
			cfg.dryRun = true
		case "--target", "--runtime-target":
			diags = append(diags, "TSPACK_INIT_UNSUPPORTED_FLAG: "+a+" is not supported in M30")
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
			}
		default:
			diags = append(diags, "TSPACK_INIT_WRITE_FAILED: unknown init flag: "+a)
		}
	}
	if cfg.kind != "" && cfg.name == "" {
		diags = append(diags, "TSPACK_INIT_NAME_REQUIRED: --name is required")
	} else if cfg.kind != "" && cfg.kind != "library" && cfg.kind != "app" {
		diags = append(diags, "TSPACK_INIT_INVALID_KIND: --kind must be library or app")
	}
	if cfg.name != "" && !initPackageNameRe.MatchString(cfg.name) {
		diags = append(diags, "TSPACK_INIT_INVALID_NAME: invalid package name")
	}
	if cfg.packageName != "" && !initPackageNameRe.MatchString(cfg.packageName) {
		diags = append(diags, "TSPACK_INIT_INVALID_NAME: invalid package name")
	}
	if !initVersionRe.MatchString(cfg.version) {
		diags = append(diags, "TSPACK_INIT_INVALID_VERSION: invalid semantic version")
	}
	return cfg, diags
}

func buildInitFiles(cfg initConfig) []plannedFile {
	manifest := renderManifest(cfg)
	entryPath := "src/index.ts"
	if cfg.kind == "app" {
		entryPath = "src/main.ts"
	}
	entry := fmt.Sprintf("export const version = %q;\n", cfg.version)
	return []plannedFile{
		{path: "manifest.tsx", content: manifest},
		{path: entryPath, content: entry},
		{path: ".tspack/types/tspack-manifest.d.ts", content: initManifestTypesDTS},
		{path: "tsconfig.tspack.json", content: initTSPackTSConfigJSON},
		{path: "tspack-env.d.ts", content: initTSPackEnvDTS},
		{path: "biome.json", content: string(defaultBiomeConfigBytes())},
	}
}

const initTSPackTSConfigJSON = `{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "jsx": "preserve",
    "strict": true,
    "noEmit": true,
    "baseUrl": ".",
    "paths": {
      "tspack/manifest": [".tspack/types/tspack-manifest.d.ts"]
    }
  },
  "include": [
    "manifest.tsx",
    "package.manifest.tsx",
    "**/*.manifest.tsx",
    "**/*.xtest.tsx",
    ".tspack/types/**/*.d.ts"
  ],
  "exclude": [
    "src/**",
    "dist/**",
    "node_modules/**",
    ".tspack/store/**",
    "tspack-artifacts/**"
  ]
}
`

const initTSPackEnvDTS = `/// <reference path="./.tspack/types/tspack-manifest.d.ts" />
`

func writeGeneratedFile(path, content string, force bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if !force {
		if _, err := os.Stat(path); err == nil {
			return errors.New("file exists")
		}
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func workspaceNameFromPackage(name string) string {
	if strings.HasPrefix(name, "@") {
		parts := strings.SplitN(strings.TrimPrefix(name, "@"), "/", 2)
		if len(parts) > 0 && parts[0] != "" {
			return parts[0]
		}
	}
	parts := strings.Split(name, "/")
	return parts[0]
}

func renderManifest(cfg initConfig) string {
	entry := "src/index.ts"
	runtime := "dist/index.js"
	types := "dist/index.d.ts"
	targetName := "core"
	typePolicy := `  const types = {
    declarations: "required",
    missingTypes: "error",
    publicTypeLeakage: "error",
    typeOnlyRuntimeLeakage: "error",
  } satisfies TypePolicy;
`
	if cfg.kind == "app" {
		entry = "src/main.ts"
		runtime = "dist/main.js"
		types = ""
		targetName = "app"
		typePolicy = `  const types = {
    declarations: "optional",
    missingTypes: "ignore",
    publicTypeLeakage: "warn",
    typeOnlyRuntimeLeakage: "error",
  } satisfies TypePolicy;
`
	}

	return fmt.Sprintf(`import {
  define,
  Package,
  Policies,
  Publish,
  Targets,
  Workspace,
  type BoundaryPolicy,
  type TypePolicy,
} from "tspack/manifest";

%s
const boundaries = {
  undeclaredImports: "error",
  phantomDependencies: "error",
  crossTargetImports: "error",
} satisfies BoundaryPolicy;

export default define(
  <Workspace name=%q>
    <Package
      name=%q
      version=%q
      license=%q
      kind=%q
    >
      <Policies types={types} boundaries={boundaries} />

      <Targets
        rows={[
          {
            name: %q,
            export: ".",
            entry: %q,
            runtime: %q,
            types: %q,
          },
        ]}
      />

      <Publish
        include={["dist/**", "README.md", "LICENSE"]}
        exclude={["src/**", "test/**", "tests/**", "fixtures/**"]}
      />
    </Package>
  </Workspace>
);
`, typePolicy, workspaceNameFromPackage(cfg.name), cfg.name, cfg.version, cfg.license, cfg.kind, targetName, entry, runtime, types)
}
