package templates

import (
	"fmt"

	"github.com/yuechen-li-dev/tspack/internal/concepts"
)

func renderReactLibraryConceptManifest(values map[string]string, conceptIR *concepts.MergedConceptIR) (string, error) {
	variables := staticManifestVariables{
		ProjectName: values["projectName"],
		PackageName: values["packageName"],
		Runtime:     values["runtime"],
	}
	if variables.ProjectName == "" || variables.PackageName == "" || variables.Runtime == "" {
		return "", fmt.Errorf("TSPACK_TEMPLATE_INVALID: react-library manifest requires projectName, packageName, and runtime")
	}
	if err := validateReactLibraryConceptManifestIR(conceptIR); err != nil {
		return "", err
	}
	conceptLines := renderConceptCommentLines(conceptIR.Concepts)
	return fmt.Sprintf(`import {
  define,
  defineDeps,
  npm,
  Package,
  peer,
  Policies,
  Publish,
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
  missingTypes: "warn",
  publicTypeLeakage: "error",
  typeOnlyRuntimeLeakage: "error",
} satisfies TypePolicy;

const boundaries = {
  undeclaredImports: "error",
  phantomDependencies: "error",
  crossTargetImports: "error",
} satisfies BoundaryPolicy;

const deps = defineDeps({
  react: peer(npm("react", "^19.0.0")),
  reactDom: peer(npm("react-dom", "^19.0.0"), { key: "react-dom" }),
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
      kind="library"
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
            name: "library",
            export: ".",
            entry: "src/index.ts",
            runtime: "dist/index.js",
            types: "dist/index.d.ts",
            peers: [deps.react, deps.reactDom],
          },
        ]}
      />
      <RunTargets
        rows={[
          {
            name: "build",
            runtime: "node",
            command: ["vite", "build"],
          },
          {
            name: "build-types",
            runtime: "node",
            command: ["tsc", "-p", "tsconfig.build.json", "--listEmittedFiles"],
          },
          {
            name: "typecheck",
            runtime: "node",
            command: [
              "tsc",
              "-p",
              "tsconfig.json",
              "--noEmit",
              "--pretty",
              "false",
              "--diagnostics",
            ],
          },
        ]}
      />
      <Publish include={["dist/**", "README.md", "package.json"]} />
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
          strategy: "manual",
          reason:
            "Formatter upgrades are kept explicit for reproducible library diffs.",
        },
        {
          name: "react",
          kind: "peer",
          strategy: "manual",
          reason:
            "React peer compatibility ranges are coordinated manually with consumers.",
        },
        {
          name: "react-dom",
          kind: "peer",
          strategy: "manual",
          reason:
            "React DOM peer compatibility ranges are coordinated manually with consumers.",
        },
      ]}
    />
    <Security
      acknowledgedLifecycleCategories={[
        {
          category: "consumer-install",
          reason:
            "Generated library tooling may use install-time binary selectors; TSPack blocks lifecycle execution by default and records the capability for review.",
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

func validateReactLibraryConceptManifestIR(conceptIR *concepts.MergedConceptIR) error {
	if conceptIR == nil {
		return fmt.Errorf("TSPACK_TEMPLATE_INVALID: react-library concept composition produced no manifest IR")
	}
	for _, name := range []string{"react", "react-dom"} {
		if !hasDependencyContribution(conceptIR.Manifest.Peers, name) {
			return fmt.Errorf("TSPACK_TEMPLATE_INVALID: react-library concept composition missing peer %q", name)
		}
	}
	for _, name := range []string{"typescript", "vite", "@vitejs/plugin-react", "@types/react", "@types/react-dom", "@biomejs/biome"} {
		if !hasDependencyContribution(conceptIR.Manifest.Tools, name) {
			return fmt.Errorf("TSPACK_TEMPLATE_INVALID: react-library concept composition missing tool %q", name)
		}
	}
	if !hasReactLibraryTarget(conceptIR.Manifest.Targets) {
		return fmt.Errorf("TSPACK_TEMPLATE_INVALID: react-library concept composition missing library target")
	}
	for _, name := range []string{"build", "build-types", "typecheck"} {
		if !hasRunTargetContribution(conceptIR.Manifest.RunTargets, name) {
			return fmt.Errorf("TSPACK_TEMPLATE_INVALID: react-library concept composition missing run target %q", name)
		}
	}
	if conceptIR.Manifest.Pack == nil {
		return fmt.Errorf("TSPACK_TEMPLATE_INVALID: react-library concept composition missing pack metadata")
	}
	if _, ok := conceptIR.Projections.Objects["package.exports"]; !ok {
		return fmt.Errorf("TSPACK_TEMPLATE_INVALID: react-library concept composition missing package exports metadata")
	}
	if len(conceptIR.Manifest.UpdatePolicy) == 0 {
		return fmt.Errorf("TSPACK_TEMPLATE_INVALID: react-library concept composition missing update policy")
	}
	if len(conceptIR.Manifest.SecurityPolicy) == 0 {
		return fmt.Errorf("TSPACK_TEMPLATE_INVALID: react-library concept composition missing security policy")
	}
	if len(conceptIR.Concepts) == 0 {
		return fmt.Errorf("TSPACK_TEMPLATE_INVALID: react-library concept composition missing concept metadata")
	}
	return nil
}

func hasReactLibraryTarget(targets []concepts.TargetContribution) bool {
	for _, target := range targets {
		if target.Name == "library" && target.Export == "." && target.Entry == "src/index.ts" && target.Runtime == "dist/index.js" && target.Types == "dist/index.d.ts" {
			return true
		}
	}
	return false
}
