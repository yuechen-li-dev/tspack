package templates

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/concepts"
)

type staticManifestVariables struct {
	ProjectName string
	PackageName string
	Runtime     string
}

func renderStaticConceptManifest(values map[string]string, conceptIR *concepts.MergedConceptIR) (string, error) {
	variables := staticManifestVariables{
		ProjectName: values["projectName"],
		PackageName: values["packageName"],
		Runtime:     values["runtime"],
	}
	if variables.ProjectName == "" || variables.PackageName == "" || variables.Runtime == "" {
		return "", fmt.Errorf("TSPACK_TEMPLATE_INVALID: static manifest requires projectName, packageName, and runtime")
	}

	if err := validateStaticConceptManifestIR(conceptIR); err != nil {
		return "", err
	}

	conceptLines := renderConceptCommentLines(conceptIR.Concepts)

	return fmt.Sprintf(`import {
  define,
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

const vite = tool(npm("vite", "^5.0.0"));
const typescript = tool(npm("typescript", "^5.0.0"));
const biome = tool(npm("@biomejs/biome", "^1.9.4"), { key: "@biomejs/biome" });

export default define(
  <Workspace name="%s" runtime="%s">
    <Package
      name="%s"
      version="0.1.0"
      kind="app"
      dependencies={{ values: [vite, typescript, biome] }}
    >
      <Policies types={types} boundaries={boundaries} />
      <Tools values={[vite, typescript, biome]} />
      <Targets
        rows={[
          {
            name: "app",
            export: ".",
            entry: "src/main.ts",
            runtime: "dist/main.js",
            types: "dist/main.d.ts",
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
          reason: "Keep TypeScript current within compatible minor updates.",
        },
        {
          name: "vite",
          kind: "tool",
          strategy: "rolling",
          level: "minor",
          reason:
            "Keep static browser tooling current within compatible minor updates.",
        },
        {
          name: "@biomejs/biome",
          kind: "tool",
          strategy: "rolling",
          level: "minor",
          reason:
            "Keep formatter tooling current within compatible minor updates.",
        },
      ]}
    />
    <Security
      acknowledgedLifecycleCategories={[
        {
          category: "consumer-install",
          reason:
            "Generated template tooling may use install-time binary selectors; TSPack blocks lifecycle execution by default and records the capability for review.",
        },
        {
          category: "maintainer-publish",
          reason:
            "Template starts with explicit tool dependencies only; maintainer-publish lifecycle scripts remain blocked by default.",
        },
      ]}
    />
  </Workspace>,
);
`, conceptLines, variables.ProjectName, variables.Runtime, variables.PackageName), nil
}

func validateStaticConceptManifestIR(conceptIR *concepts.MergedConceptIR) error {
	if conceptIR == nil {
		return fmt.Errorf("TSPACK_TEMPLATE_INVALID: static concept composition produced no manifest IR")
	}
	for _, name := range []string{"typescript", "vite", "@biomejs/biome"} {
		if !hasDependencyContribution(conceptIR.Manifest.Tools, name) {
			return fmt.Errorf("TSPACK_TEMPLATE_INVALID: static concept composition missing tool %q", name)
		}
	}
	if !hasStaticTarget(conceptIR.Manifest.Targets) {
		return fmt.Errorf("TSPACK_TEMPLATE_INVALID: static concept composition missing app target")
	}
	for _, name := range []string{"dev", "build"} {
		if !hasRunTargetContribution(conceptIR.Manifest.RunTargets, name) {
			return fmt.Errorf("TSPACK_TEMPLATE_INVALID: static concept composition missing run target %q", name)
		}
	}
	if len(conceptIR.Manifest.UpdatePolicy) == 0 {
		return fmt.Errorf("TSPACK_TEMPLATE_INVALID: static concept composition missing update policy")
	}
	if len(conceptIR.Manifest.SecurityPolicy) == 0 {
		return fmt.Errorf("TSPACK_TEMPLATE_INVALID: static concept composition missing security policy")
	}
	return nil
}

func renderConceptCommentLines(names []string) string {
	ordered := append([]string{}, names...)
	sort.Strings(ordered)
	lines := []string{"// Generated from concept fragments:"}
	for _, name := range ordered {
		lines = append(lines, "// - "+name)
	}
	return strings.Join(lines, "\n") + "\n\n"
}

func hasDependencyContribution(deps []concepts.DependencyContribution, name string) bool {
	for _, dep := range deps {
		if dep.Name == name {
			return true
		}
	}
	return false
}

func hasRunTargetContribution(targets []concepts.RunTargetContribution, name string) bool {
	for _, target := range targets {
		if target.Name == name {
			return true
		}
	}
	return false
}

func hasStaticTarget(targets []concepts.TargetContribution) bool {
	for _, target := range targets {
		if target.Name == "app" && target.Export == "." && target.Entry == "src/main.ts" && target.Runtime == "dist/main.js" && target.Types == "dist/main.d.ts" {
			return true
		}
	}
	return false
}
