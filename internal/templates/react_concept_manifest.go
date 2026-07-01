package templates

import (
	"fmt"

	"github.com/yuechen-li-dev/tspack/internal/concepts"
)

func renderReactConceptManifest(values map[string]string, conceptIR *concepts.MergedConceptIR) (string, error) {
	variables := staticManifestVariables{
		ProjectName: values["projectName"],
		PackageName: values["packageName"],
		Runtime:     values["runtime"],
	}
	if variables.ProjectName == "" || variables.PackageName == "" || variables.Runtime == "" {
		return "", fmt.Errorf("TSPACK_TEMPLATE_INVALID: react manifest requires projectName, packageName, and runtime")
	}
	if err := validateReactConceptManifestIR(conceptIR); err != nil {
		return "", err
	}
	conceptLines := renderConceptCommentLines(conceptIR.Concepts)
	return fmt.Sprintf(`import {
  define,
  defineDeps,
  dep,
  npm,
  Package,
  Policies,
  RunTargets,
  Security,
  Targets,
  tool,
  Tools,
  UpdatePolicy,
  Workspace,
  type BoundaryPolicy,
  type TypePolicy,
} from "tspack/manifest";

%sconst types = {
  declarations: "optional",
  missingTypes: "ignore",
  publicTypeLeakage: "warn",
  typeOnlyRuntimeLeakage: "error",
} satisfies TypePolicy;

const boundaries = {
  undeclaredImports: "error",
  phantomDependencies: "error",
  crossTargetImports: "error",
} satisfies BoundaryPolicy;

const deps = defineDeps({
  react: dep(npm("react", "^19.0.0")),
  reactDom: dep(npm("react-dom", "^19.0.0"), { key: "react-dom" }),
  typescript: tool(npm("typescript", "^5.9.0")),
  vite: tool(npm("vite", "^5.0.0")),
  viteReact: tool(npm("@vitejs/plugin-react", "^4.0.0"), {
    key: "@vitejs/plugin-react",
  }),
  reactTypes: tool(npm("@types/react", "^19.0.0"), { key: "@types/react" }),
  reactDomTypes: tool(npm("@types/react-dom", "^19.0.0"), {
    key: "@types/react-dom",
  }),
  biome: tool(npm("@biomejs/biome", "^1.9.4"), { key: "@biomejs/biome" }),
});

export default define(
  <Workspace name="%s" runtime="%s">
    <Package
      name="%s"
      version="0.1.0"
      kind="app"
      license="MIT"
      dependencies={{
        values: [
          deps.react,
          deps.reactDom,
          deps.typescript,
          deps.vite,
          deps.viteReact,
          deps.reactTypes,
          deps.reactDomTypes,
          deps.biome,
        ],
      }}
    >
      <Policies types={types} boundaries={boundaries} />
      <Tools
        values={[
          deps.typescript,
          deps.vite,
          deps.viteReact,
          deps.reactTypes,
          deps.reactDomTypes,
          deps.biome,
        ]}
      />
      <Targets
        rows={[
          {
            name: "app",
            export: ".",
            entry: "src/main.tsx",
            runtime: "dist/main.js",
            types: "dist/main.d.ts",
            deps: [deps.react, deps.reactDom],
          },
        ]}
      />
      <RunTargets
        rows={[
          {
            name: "dev",
            runtime: "node",
            command: ["vite", "--host", "127.0.0.1"],
            url: "http://127.0.0.1:5173",
            ready: { kind: "http", path: "/" },
          },
          {
            name: "build",
            runtime: "node",
            command: ["vite", "build"],
            url: "http://127.0.0.1:4173",
          },
          {
            name: "preview",
            runtime: "node",
            command: ["vite", "preview", "--host", "127.0.0.1"],
            url: "http://127.0.0.1:4173",
            ready: { kind: "http", path: "/" },
          },
        ]}
      />
    </Package>
    <UpdatePolicy
      rows={[
        {
          name: "typescript",
          kind: "tool",
          strategy: "rolling",
          level: "minor",
          reason:
            "Keep TypeScript current within compatible compiler minor updates.",
        },
        {
          name: "vite",
          kind: "tool",
          strategy: "rolling",
          level: "minor",
          reason: "Keep Vite current within compatible minor updates.",
        },
        {
          name: "@vitejs/plugin-react",
          kind: "tool",
          strategy: "rolling",
          level: "minor",
          reason: "Keep the Vite React plugin aligned with Vite minor updates.",
        },
        {
          name: "@types/react",
          kind: "tool",
          strategy: "rolling",
          level: "minor",
          reason:
            "Keep React type declarations current within compatible minor updates.",
        },
        {
          name: "@types/react-dom",
          kind: "tool",
          strategy: "rolling",
          level: "minor",
          reason:
            "Keep React DOM type declarations current within compatible minor updates.",
        },
        {
          name: "@biomejs/biome",
          kind: "tool",
          strategy: "rolling",
          level: "minor",
          reason:
            "Keep formatter tooling current within compatible minor updates.",
        },
        {
          name: "react",
          kind: "dep",
          strategy: "manual",
          reason: "React runtime upgrades are coordinated manually.",
        },
        {
          name: "react-dom",
          kind: "dep",
          strategy: "manual",
          reason: "React DOM runtime upgrades are coordinated manually.",
        },
      ]}
    />
    <Security
      acknowledgedLifecycleCategories={[
        {
          category: "consumer-install",
          reason:
            "Generated app tooling may use install-time binary selectors; TSPack blocks lifecycle execution by default and records the capability for review.",
        },
        {
          category: "maintainer-publish",
          reason:
            "Template dependencies are inert until explicitly updated; maintainer-publish lifecycle scripts remain blocked by default.",
        },
      ]}
    />
  </Workspace>,
);
`, conceptLines, variables.ProjectName, variables.Runtime, variables.PackageName), nil
}

func validateReactConceptManifestIR(conceptIR *concepts.MergedConceptIR) error {
	if conceptIR == nil {
		return fmt.Errorf("TSPACK_TEMPLATE_INVALID: react concept composition produced no manifest IR")
	}
	for _, name := range []string{"react", "react-dom"} {
		if !hasDependencyContribution(conceptIR.Manifest.Dependencies, name) {
			return fmt.Errorf("TSPACK_TEMPLATE_INVALID: react concept composition missing dependency %q", name)
		}
	}
	for _, name := range []string{"typescript", "vite", "@vitejs/plugin-react", "@types/react", "@types/react-dom", "@biomejs/biome"} {
		if !hasDependencyContribution(conceptIR.Manifest.Tools, name) {
			return fmt.Errorf("TSPACK_TEMPLATE_INVALID: react concept composition missing tool %q", name)
		}
	}
	if !hasReactTarget(conceptIR.Manifest.Targets) {
		return fmt.Errorf("TSPACK_TEMPLATE_INVALID: react concept composition missing app target")
	}
	for _, name := range []string{"dev", "build", "preview"} {
		if !hasRunTargetContribution(conceptIR.Manifest.RunTargets, name) {
			return fmt.Errorf("TSPACK_TEMPLATE_INVALID: react concept composition missing run target %q", name)
		}
	}
	if len(conceptIR.Manifest.UpdatePolicy) == 0 {
		return fmt.Errorf("TSPACK_TEMPLATE_INVALID: react concept composition missing update policy")
	}
	if len(conceptIR.Manifest.SecurityPolicy) == 0 {
		return fmt.Errorf("TSPACK_TEMPLATE_INVALID: react concept composition missing security policy")
	}
	return nil
}

func hasReactTarget(targets []concepts.TargetContribution) bool {
	for _, target := range targets {
		if target.Name == "app" && target.Export == "." && target.Entry == "src/main.tsx" && target.Runtime == "dist/main.js" && target.Types == "dist/main.d.ts" {
			return true
		}
	}
	return false
}
