package manifestedit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/authoring"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/manifestfrontend"
	"github.com/yuechen-li-dev/tspack/internal/templates"
)

func TestFrontendSemanticEditProjectionRoundTrip(t *testing.T) {
	frontendPath, err := filepath.Abs(filepath.Join("..", "..", "manifest-frontend", "dist", "cli.js"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(frontendPath); err != nil {
		t.Skip("manifest frontend build is unavailable; run npm --prefix manifest-frontend run build")
	}
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.tsx")
	original := `import { Package, Workspace, define, defineDeps, dep, npm } from "tspack/manifest";

const WeirdButValidThing = { preserve: "exactly" };
const deps = defineDeps({ react: dep(npm("react", "^19")) });

export default define(
  <Workspace name="demo">
    <Package name="app" version="1.0.0" kind="app" dependencies={{ values: [deps.react] }} />
  </Workspace>,
);
`
	if err := os.WriteFile(manifestPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded := loadRoundTripManifest(t, frontendPath, manifestPath)
	packageIR := loaded.Packages[0].DependencyAuthoring
	if packageIR == nil {
		t.Fatal("frontend did not emit dependency authoring IR")
	}
	added := authoring.DependencyDeclaration{
		ID:          "lodash-explicit",
		Identity:    authoring.PackageIdentity{Source: "npm", Name: "lodash"},
		Source:      authoring.PackageSource{Kind: "npm", Package: "lodash", Range: "^4"},
		Constraint:  "^4",
		Kind:        authoring.DependencyRuntime,
		Authority:   authoring.AuthorityOwned,
		Editability: authoring.EditabilityEditable,
	}
	addEdit := authoring.Add(*packageIR, added)
	addProjection, err := PlanFile(frontendPath, manifestPath, "app", addEdit)
	if err != nil {
		t.Fatal(err)
	}
	assertNoDiagnostics(t, addProjection)
	if err := os.WriteFile(manifestPath, []byte(addProjection.UpdatedSource), 0o644); err != nil {
		t.Fatal(err)
	}

	afterAdd := loadRoundTripManifest(t, frontendPath, manifestPath)
	if !containsIdentity(afterAdd.Packages[0].DependencyAuthoring, "npm", "lodash") {
		t.Fatalf("reloaded tape does not contain added lodash declaration: %#v", afterAdd.Packages[0].DependencyAuthoring)
	}
	removeEdit, err := authoring.Remove(
		*afterAdd.Packages[0].DependencyAuthoring,
		authoring.DeclarationSelector{
			Identity:     &authoring.PackageIdentity{Source: "npm", Name: "lodash"},
			EditableOnly: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	removeProjection, err := PlanFile(frontendPath, manifestPath, "app", removeEdit)
	if err != nil {
		t.Fatal(err)
	}
	assertNoDiagnostics(t, removeProjection)
	if removeProjection.UpdatedSource != original {
		t.Fatalf("add/remove did not restore original source\nwant:\n%s\ngot:\n%s", original, removeProjection.UpdatedSource)
	}

	noOp := Plan(ProjectionRequest{
		SourceText:   original,
		ManifestPath: manifestPath,
		PackageName:  "app",
		Analysis:     mustAnalyze(t, frontendPath, manifestPath, "app"),
		Edit: authoring.EditResult{
			Before: authoring.Build(packageIR.Declarations),
			After:  authoring.Build(packageIR.Declarations),
		},
	})
	if noOp.Changed || noOp.UpdatedSource != original {
		t.Fatalf("roundtrip no-op changed source: %#v", noOp)
	}
}

func TestBuiltinTemplateProjectionDogfood(t *testing.T) {
	frontendPath, err := filepath.Abs(filepath.Join("..", "..", "manifest-frontend", "dist", "cli.js"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(frontendPath); err != nil {
		t.Skip("manifest frontend build is unavailable; run npm --prefix manifest-frontend run build")
	}
	for _, templateName := range []string{"static", "react", "react-library"} {
		t.Run(templateName, func(t *testing.T) {
			template, err := templates.LoadBuiltin(templateName)
			if err != nil {
				t.Fatal(err)
			}
			values, err := template.ResolveValues(nil)
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			if _, err := template.Apply(templates.ApplyOptions{Destination: root, Values: values}); err != nil {
				t.Fatal(err)
			}
			manifestPath := filepath.Join(root, "manifest.tsx")
			original, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			loaded := loadRoundTripManifest(t, frontendPath, manifestPath)
			if len(loaded.Packages) != 1 || loaded.Packages[0].DependencyAuthoring == nil {
				t.Fatalf("generated template has no package authoring IR: %#v", loaded.Packages)
			}
			packageName := loaded.Packages[0].Name
			analysis := mustAnalyze(t, frontendPath, manifestPath, packageName)
			if analysis.Status != manifestfrontend.DependencyIslandOwnedCanonical {
				t.Fatalf("template island status = %q", analysis.Status)
			}

			added := authoring.DependencyDeclaration{
				ID:          "dogfood-lodash",
				Identity:    authoring.PackageIdentity{Source: "npm", Name: "lodash"},
				Source:      authoring.PackageSource{Kind: "npm", Package: "lodash", Range: "^4"},
				Constraint:  "^4",
				Kind:        authoring.DependencyTool,
				Authority:   authoring.AuthorityOwned,
				Editability: authoring.EditabilityEditable,
			}
			addEdit := authoring.Add(*loaded.Packages[0].DependencyAuthoring, added)
			addProjection, err := PlanFile(frontendPath, manifestPath, packageName, addEdit)
			if err != nil {
				t.Fatal(err)
			}
			assertNoDiagnostics(t, addProjection)
			if err := os.WriteFile(manifestPath, []byte(addProjection.UpdatedSource), 0o644); err != nil {
				t.Fatal(err)
			}
			afterAdd := loadRoundTripManifest(t, frontendPath, manifestPath)
			if !containsIdentity(afterAdd.Packages[0].DependencyAuthoring, "npm", "lodash") {
				t.Fatal("template reload lost added dependency")
			}

			removeEdit, err := authoring.Remove(
				*afterAdd.Packages[0].DependencyAuthoring,
				authoring.DeclarationSelector{
					Identity:     &authoring.PackageIdentity{Source: "npm", Name: "lodash"},
					EditableOnly: true,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			removeProjection, err := PlanFile(frontendPath, manifestPath, packageName, removeEdit)
			if err != nil {
				t.Fatal(err)
			}
			assertNoDiagnostics(t, removeProjection)
			if removeProjection.UpdatedSource != string(original) {
				t.Fatalf("template add/remove did not restore original source\nwant:\n%s\ngot:\n%s", original, removeProjection.UpdatedSource)
			}
		})
	}
}

func TestRootManifestCopyProjectionDogfood(t *testing.T) {
	frontendPath, err := filepath.Abs(filepath.Join("..", "..", "manifest-frontend", "dist", "cli.js"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(frontendPath); err != nil {
		t.Skip("manifest frontend build is unavailable; run npm --prefix manifest-frontend run build")
	}
	repositoryManifest, err := filepath.Abs(filepath.Join("..", "..", "manifest.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(repositoryManifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(t.TempDir(), "manifest.tsx")
	if err := os.WriteFile(manifestPath, original, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded := loadRoundTripManifest(t, frontendPath, manifestPath)
	var packageIR *authoring.PackageIR
	for index := range loaded.Packages {
		if loaded.Packages[index].Name == "@tspack/manifest-frontend" {
			packageIR = loaded.Packages[index].DependencyAuthoring
			break
		}
	}
	if packageIR == nil {
		t.Fatal("root manifest frontend package has no dependency authoring IR")
	}
	added := authoring.DependencyDeclaration{
		ID:          "root-dogfood-lodash",
		Identity:    authoring.PackageIdentity{Source: "npm", Name: "lodash"},
		Source:      authoring.PackageSource{Kind: "npm", Package: "lodash", Range: "^4"},
		Constraint:  "^4",
		Kind:        authoring.DependencyTool,
		Authority:   authoring.AuthorityOwned,
		Editability: authoring.EditabilityEditable,
	}
	addEdit := authoring.Add(*packageIR, added)
	addProjection, err := PlanFile(frontendPath, manifestPath, "@tspack/manifest-frontend", addEdit)
	if err != nil {
		t.Fatal(err)
	}
	assertNoDiagnostics(t, addProjection)
	if len(addProjection.Edits) != 1 {
		t.Fatalf("root dry-run add edits = %#v, want one dependency-island edit", addProjection.Edits)
	}
	if err := os.WriteFile(manifestPath, []byte(addProjection.UpdatedSource), 0o644); err != nil {
		t.Fatal(err)
	}
	afterAdd := loadRoundTripManifest(t, frontendPath, manifestPath)
	var afterAddIR *authoring.PackageIR
	for index := range afterAdd.Packages {
		if afterAdd.Packages[index].Name == "@tspack/manifest-frontend" {
			afterAddIR = afterAdd.Packages[index].DependencyAuthoring
			break
		}
	}
	if !containsIdentity(afterAddIR, "npm", "lodash") {
		t.Fatal("root dry-run add did not reload")
	}
	removeEdit, err := authoring.Remove(
		*afterAddIR,
		authoring.DeclarationSelector{
			Identity:     &authoring.PackageIdentity{Source: "npm", Name: "lodash"},
			EditableOnly: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	removeProjection, err := PlanFile(frontendPath, manifestPath, "@tspack/manifest-frontend", removeEdit)
	if err != nil {
		t.Fatal(err)
	}
	assertNoDiagnostics(t, removeProjection)
	if removeProjection.UpdatedSource != string(original) {
		t.Fatal("root copy add/remove did not restore the original source")
	}
}

func loadRoundTripManifest(t *testing.T, frontendPath string, manifestPath string) *manifest.ManifestIR {
	t.Helper()
	parsed, err := manifestfrontend.Execute(frontendPath, manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.OK {
		t.Fatalf("frontend diagnostics: %#v", parsed.Diagnostics)
	}
	ir, diagnostics := manifest.LoadBytes(manifestPath, parsed.IR)
	if len(diagnostics) > 0 {
		t.Fatalf("manifest diagnostics: %#v", diagnostics)
	}
	return ir
}

func mustAnalyze(t *testing.T, frontendPath string, manifestPath string, packageName string) manifestfrontend.DependencySourceAnalysis {
	t.Helper()
	analysis, err := manifestfrontend.AnalyzeDependencies(frontendPath, manifestPath, packageName)
	if err != nil {
		t.Fatal(err)
	}
	return analysis
}

func containsIdentity(ir *authoring.PackageIR, source string, name string) bool {
	if ir == nil {
		return false
	}
	for _, declaration := range ir.Declarations {
		if declaration.Identity == (authoring.PackageIdentity{Source: source, Name: name}) {
			return true
		}
	}
	return false
}
