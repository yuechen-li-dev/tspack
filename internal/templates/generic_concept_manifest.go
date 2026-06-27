package templates

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/concepts"
)

func renderGenericConceptManifest(values map[string]string, templateKind string, conceptIR *concepts.MergedConceptIR) (string, error) {
	if conceptIR == nil {
		return "", fmt.Errorf("TSPACK_TEMPLATE_INVALID: concept composition produced no manifest IR")
	}
	projectName := values["projectName"]
	packageName := values["packageName"]
	runtime := values["runtime"]
	if projectName == "" || packageName == "" || runtime == "" {
		return "", fmt.Errorf("TSPACK_TEMPLATE_INVALID: concept manifest generation requires projectName, packageName, and runtime")
	}
	packageKind := templateKind
	if conceptIR.Manifest.Package != nil && conceptIR.Manifest.Package.Kind != "" {
		packageKind = conceptIR.Manifest.Package.Kind
	}
	if !isSupportedGenericPackageKind(packageKind) {
		return "", fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_UNSUPPORTED_CONTRIBUTION: unsupported package kind %q", packageKind)
	}
	if err := validateGenericConceptManifestSupport(conceptIR); err != nil {
		return "", err
	}

	imports := []string{"define", "npm", "Package", "RunTargets", "Targets", "Workspace"}
	if hasAnyDependencyRows(conceptIR) {
		imports = append(imports, "defineDeps")
	}
	if len(conceptIR.Manifest.Dependencies) > 0 {
		imports = append(imports, "dep")
	}
	if len(conceptIR.Manifest.Tools) > 0 {
		imports = append(imports, "tool", "Tools")
	}
	if len(conceptIR.Manifest.Peers) > 0 {
		imports = append(imports, "peer")
	}
	if len(conceptIR.Manifest.UpdatePolicy) > 0 {
		imports = append(imports, "UpdatePolicy")
	}
	if len(conceptIR.Manifest.SecurityPolicy) > 0 {
		imports = append(imports, "Security")
	}
	sort.Strings(imports)

	var b strings.Builder
	b.WriteString("import {\n")
	for _, name := range imports {
		b.WriteString("  " + name + ",\n")
	}
	b.WriteString("} from \"tspack/manifest\";\n\n")
	b.WriteString(renderConceptCommentLines(conceptIR.Concepts))
	renderGenericDependencyConstants(&b, "dependencies", conceptIR.Manifest.Dependencies, "npm")
	renderGenericDependencyConstants(&b, "tools", conceptIR.Manifest.Tools, "tool")
	renderGenericDependencyConstants(&b, "peers", conceptIR.Manifest.Peers, "peer")
	b.WriteString("export default define(\n")
	b.WriteString(fmt.Sprintf("  <Workspace name=%q runtime=%q>\n", projectName, runtime))
	b.WriteString(fmt.Sprintf("    <Package name=%q version=\"0.1.0\" kind=%q", packageName, packageKind))
	if len(conceptIR.Manifest.Dependencies) > 0 {
		b.WriteString(fmt.Sprintf(" dependencies={{ values: [%s] }}", renderPackageDependencyRefs(conceptIR)))
	}
	if len(conceptIR.Manifest.Peers) > 0 {
		b.WriteString(fmt.Sprintf(" peers={[%s]}", renderDependencyRefs("peers", conceptIR.Manifest.Peers)))
	}
	b.WriteString(">\n")
	if len(conceptIR.Manifest.Tools) > 0 {
		b.WriteString(fmt.Sprintf("      <Tools values={[%s]} />\n", renderDependencyRefs("tools", conceptIR.Manifest.Tools)))
	}
	if len(conceptIR.Manifest.Targets) > 0 {
		b.WriteString("      <Targets\n        rows={[\n")
		for _, target := range conceptIR.Manifest.Targets {
			b.WriteString("          {\n")
			writeTSStringField(&b, "name", target.Name, 12)
			writeTSStringField(&b, "export", target.Export, 12)
			writeTSStringField(&b, "entry", target.Entry, 12)
			writeTSStringField(&b, "runtime", target.Runtime, 12)
			writeTSStringField(&b, "types", target.Types, 12)
			b.WriteString("          },\n")
		}
		b.WriteString("        ]}\n      />\n")
	}
	if len(conceptIR.Manifest.RunTargets) > 0 {
		b.WriteString("      <RunTargets\n        rows={[\n")
		for _, target := range conceptIR.Manifest.RunTargets {
			b.WriteString("          {\n")
			writeTSStringField(&b, "name", target.Name, 12)
			b.WriteString("            runtime: \"node\",\n")
			b.WriteString(fmt.Sprintf("            command: %s,\n", renderCommandArray(target.Command)))
			writeTSStringField(&b, "cwd", target.Cwd, 12)
			b.WriteString("          },\n")
		}
		b.WriteString("        ]}\n      />\n")
	}
	b.WriteString("    </Package>\n")
	renderGenericPolicySections(&b, conceptIR)
	b.WriteString("  </Workspace>,\n);\n")
	return b.String(), nil
}

func validateGenericConceptManifestSupport(conceptIR *concepts.MergedConceptIR) error {
	if len(conceptIR.Manifest.Env) > 0 {
		return fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_UNSUPPORTED_CONTRIBUTION: generic concept manifest rendering does not yet support manifest.env")
	}
	if len(conceptIR.Manifest.Services) > 0 {
		return fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_UNSUPPORTED_CONTRIBUTION: generic concept manifest rendering does not yet support manifest.services")
	}
	if conceptIR.Manifest.Pack != nil {
		return fmt.Errorf("TSPACK_TEMPLATE_CONCEPT_UNSUPPORTED_CONTRIBUTION: generic concept manifest rendering does not yet support manifest.pack")
	}
	return nil
}

func isSupportedGenericPackageKind(kind string) bool {
	return kind == "app" || kind == "library" || kind == "service" || kind == "tool"
}

func hasAnyDependencyRows(conceptIR *concepts.MergedConceptIR) bool {
	return len(conceptIR.Manifest.Dependencies) > 0 || len(conceptIR.Manifest.Tools) > 0 || len(conceptIR.Manifest.Peers) > 0
}

func renderGenericDependencyConstants(b *strings.Builder, name string, deps []concepts.DependencyContribution, helper string) {
	if len(deps) == 0 {
		return
	}
	b.WriteString("const " + name + " = defineDeps({\n")
	for _, dep := range deps {
		identifier := dependencyIdentifier(dep.Name)
		keySuffix := ""
		if identifier != dep.Name {
			keySuffix = fmt.Sprintf(", { key: %q }", dep.Name)
		}
		if helper == "tool" || helper == "peer" {
			b.WriteString(fmt.Sprintf("  %s: %s(npm(%q, %q)%s),\n", identifier, helper, dep.Name, dep.Range, keySuffix))
		} else {
			b.WriteString(fmt.Sprintf("  %s: dep(npm(%q, %q)%s),\n", identifier, dep.Name, dep.Range, keySuffix))
		}
	}
	b.WriteString("});\n\n")
}

func dependencyIdentifier(name string) string {
	cleaned := strings.NewReplacer("@", "", "/", "-", ".", "-", "_", "-").Replace(name)
	parts := strings.Split(cleaned, "-")
	out := ""
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i == 0 && out == "" {
			out = part
			continue
		}
		out += strings.ToUpper(part[:1]) + part[1:]
	}
	if out == "" {
		return "dep"
	}
	return out
}

func renderPackageDependencyRefs(conceptIR *concepts.MergedConceptIR) string {
	refs := []string{}
	refs = append(refs, renderDependencyRefs("dependencies", conceptIR.Manifest.Dependencies))
	refs = append(refs, renderDependencyRefs("tools", conceptIR.Manifest.Tools))
	refs = append(refs, renderDependencyRefs("peers", conceptIR.Manifest.Peers))
	joined := []string{}
	for _, ref := range refs {
		if ref != "" {
			joined = append(joined, ref)
		}
	}
	return strings.Join(joined, ", ")
}

func renderDependencyRefs(objectName string, deps []concepts.DependencyContribution) string {
	refs := []string{}
	for _, dep := range deps {
		refs = append(refs, objectName+"."+dependencyIdentifier(dep.Name))
	}
	return strings.Join(refs, ", ")
}

func renderCommandArray(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "[]"
	}
	quoted := []string{}
	for _, field := range fields {
		quoted = append(quoted, fmt.Sprintf("%q", field))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func writeTSStringField(b *strings.Builder, name string, value string, spaces int) {
	if value == "" {
		return
	}
	b.WriteString(strings.Repeat(" ", spaces))
	b.WriteString(fmt.Sprintf("%s: %q,\n", name, value))
}

func genericUpdateStrategy(action string) string {
	if action == "pin" {
		return "pinned"
	}
	if action == "rolling" || action == "manual" || action == "pinned" {
		return action
	}
	return "manual"
}

func genericSecurityCategory(value string) string {
	if value == "consumer-install" || value == "maintainer-publish" || value == "other" {
		return value
	}
	return "other"
}

func renderGenericPolicySections(b *strings.Builder, conceptIR *concepts.MergedConceptIR) {
	if len(conceptIR.Manifest.UpdatePolicy) > 0 {
		b.WriteString("    <UpdatePolicy rows={[\n")
		for _, row := range conceptIR.Manifest.UpdatePolicy {
			strategy := genericUpdateStrategy(row.Action)
			if strategy == "rolling" && row.Range != "" {
				b.WriteString(fmt.Sprintf("      { name: %q, kind: \"any\", strategy: %q, level: %q, reason: \"Generated from concept policy.\" },\n", row.Subject, strategy, row.Range))
			} else {
				b.WriteString(fmt.Sprintf("      { name: %q, kind: \"any\", strategy: %q, reason: \"Generated from concept policy.\" },\n", row.Subject, strategy))
			}
		}
		b.WriteString("    ]} />\n")
	}
	if len(conceptIR.Manifest.SecurityPolicy) > 0 {
		b.WriteString("    <Security acknowledgedLifecycleCategories={[\n")
		for _, row := range conceptIR.Manifest.SecurityPolicy {
			b.WriteString(fmt.Sprintf("      { category: %q, reason: \"Generated from concept security policy.\" },\n", genericSecurityCategory(row.Range)))
		}
		b.WriteString("    ]} />\n")
	}
}
