package manifestedit

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/authoring"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/manifestfrontend"
)

func TestWritePlannedFileWritesOnlyMatchingChangedSource(t *testing.T) {
	source := canonicalSource("\r\n")
	beforeDeclaration := testDeclaration("react", "^19", 0)
	added := testDeclaration("lodash", "^4", 0)
	added.ID = "lodash"
	added.Identity.Name = "lodash"
	added.Source.Package = "lodash"
	added.Layer = authoring.LayerExplicit
	added.Origin = authoring.DeclarationOrigin{Kind: authoring.OriginExplicitUserOperation}
	projection := Plan(ProjectionRequest{
		SourceText:   source,
		ManifestPath: "manifest.tsx",
		PackageName:  "app",
		Analysis:     canonicalAnalysis(t, source),
		Edit: authoring.Add(
			authoring.PackageIR{Declarations: []authoring.DependencyDeclaration{beforeDeclaration}},
			added,
		),
	})
	assertNoDiagnostics(t, projection)
	path := filepath.Join(t.TempDir(), "manifest.tsx")
	if err := os.WriteFile(path, []byte(source), 0o640); err != nil {
		t.Fatal(err)
	}
	written, err := WritePlannedFile(path, projection)
	if err != nil || !written {
		t.Fatalf("write result = %t, %v", written, err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != projection.UpdatedSource {
		t.Fatal("atomic write did not persist planned bytes")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o, want 640", info.Mode().Perm())
	}

	conflictPath := filepath.Join(t.TempDir(), "manifest.tsx")
	if err := os.WriteFile(conflictPath, []byte(source+"// concurrent change\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if written, err := WritePlannedFile(conflictPath, projection); err == nil || written || !strings.Contains(err.Error(), "TSPACK_MANIFEST_SOURCE_CHANGED") {
		t.Fatalf("concurrent write result = %t, %v", written, err)
	}
}

func TestPlanNoOpIsByteIdentical(t *testing.T) {
	source := canonicalSource("\n")
	result := Plan(ProjectionRequest{
		SourceText:   source,
		ManifestPath: "manifest.tsx",
		PackageName:  "app",
		Analysis:     canonicalAnalysis(t, source),
		Edit: authoring.EditResult{
			Before: authoring.Build([]authoring.DependencyDeclaration{testDeclaration("react", "^19", 0)}),
			After:  authoring.Build([]authoring.DependencyDeclaration{testDeclaration("react", "^19", 0)}),
		},
	})

	if result.Changed || result.UpdatedSource != source || len(result.Edits) != 0 {
		t.Fatalf("no-op projection changed source: %#v", result)
	}
}

func TestPlanAddChangesOnlyOwnedIslandAndRequiredImport(t *testing.T) {
	source := strings.Replace(canonicalSource("\n"), "dep,", "", 1)
	beforeDeclaration := testDeclaration("react", "^19", 0)
	added := testDeclaration("lodash", "^4", 0)
	added.ID = "lodash"
	added.Identity.Name = "lodash"
	added.Source.Package = "lodash"
	added.Layer = authoring.LayerExplicit
	added.Origin = authoring.DeclarationOrigin{Kind: authoring.OriginExplicitUserOperation}
	edit := authoring.Add(authoring.PackageIR{Declarations: []authoring.DependencyDeclaration{beforeDeclaration}}, added)

	result := Plan(ProjectionRequest{
		SourceText:   source,
		ManifestPath: "manifest.tsx",
		PackageName:  "app",
		Analysis:     canonicalAnalysis(t, source),
		Edit:         edit,
	})

	assertNoDiagnostics(t, result)
	if !result.Changed {
		t.Fatal("add projection did not change source")
	}
	for _, want := range []string{
		"tool, dep }",
		"deps.react,",
		`dep(npm("lodash", "^4")`,
		"const untouched = { strange: true };",
		"// island-level comment",
	} {
		if !strings.Contains(result.UpdatedSource, want) {
			t.Fatalf("updated source does not contain %q:\n%s", want, result.UpdatedSource)
		}
	}
}

func TestPlanRemovePreservesNextEntryCommentAndReportsUnshadow(t *testing.T) {
	source := canonicalSource("\n")
	concept := testDeclaration("react-concept", "^18", 0)
	concept.Layer = authoring.LayerConcept
	concept.Origin = authoring.DeclarationOrigin{Kind: authoring.OriginConcept, Name: "React"}
	concept.Authority = authoring.AuthorityGenerated
	concept.Editability = authoring.EditabilityConceptOwned
	concept.Identity.Name = "typescript"
	concept.Source.Package = "typescript"
	concept.Kind = authoring.DependencyTool
	explicit := testDeclaration("typescript", "^6", 1)
	explicit.ID = "typescript-explicit"
	explicit.Identity.Name = "typescript"
	explicit.Source.Package = "typescript"
	explicit.Kind = authoring.DependencyTool
	explicit.Layer = authoring.LayerExplicit
	explicit.Origin = authoring.DeclarationOrigin{Kind: authoring.OriginExplicitUserOperation}
	edit, err := authoring.Remove(
		authoring.PackageIR{Declarations: []authoring.DependencyDeclaration{concept, explicit}},
		authoring.DeclarationSelector{ID: explicit.ID, EditableOnly: true},
	)
	if err != nil {
		t.Fatal(err)
	}

	result := Plan(ProjectionRequest{
		SourceText:   source,
		ManifestPath: "manifest.tsx",
		PackageName:  "app",
		Analysis:     canonicalAnalysis(t, source),
		Edit:         edit,
	})

	assertNoDiagnostics(t, result)
	if strings.Contains(result.UpdatedSource, "deps.typescript") {
		t.Fatalf("removed dependency reference remains:\n%s", result.UpdatedSource)
	}
	if !strings.Contains(result.UpdatedSource, "// island-level comment") {
		t.Fatalf("island-level comment was lost:\n%s", result.UpdatedSource)
	}
	if strings.Contains(result.UpdatedSource, "// comment attached to typescript") {
		t.Fatalf("comment attached to removed declaration survived:\n%s", result.UpdatedSource)
	}
	if !hasChange(edit.Changes, authoring.ChangeUnshadowed) {
		t.Fatalf("semantic edit did not retain unshadow evidence: %#v", edit.Changes)
	}
}

func TestPlanChangeConstraintReplacesOnlySelectedElement(t *testing.T) {
	source := canonicalSource("\n")
	react := testDeclaration("react", "^19", 0)
	typescript := testDeclaration("typescript", "^5.9", 1)
	typescript.Identity.Name = "typescript"
	typescript.Source.Package = "typescript"
	typescript.Kind = authoring.DependencyTool
	edit, err := authoring.ChangeConstraint(
		authoring.PackageIR{Declarations: []authoring.DependencyDeclaration{react, typescript}},
		authoring.DeclarationSelector{ID: react.ID, EditableOnly: true},
		"^20",
	)
	if err != nil {
		t.Fatal(err)
	}

	result := Plan(ProjectionRequest{
		SourceText:   source,
		ManifestPath: "manifest.tsx",
		PackageName:  "app",
		Analysis:     canonicalAnalysis(t, source),
		Edit:         edit,
	})

	assertNoDiagnostics(t, result)
	if !strings.Contains(result.UpdatedSource, `dep(npm("react", "^20"))`) {
		t.Fatalf("changed constraint was not projected:\n%s", result.UpdatedSource)
	}
	for _, unchanged := range []string{"deps.typescript", "// comment attached to typescript", "const untouched = { strange: true };"} {
		if !strings.Contains(result.UpdatedSource, unchanged) {
			t.Fatalf("unrelated source %q changed:\n%s", unchanged, result.UpdatedSource)
		}
	}
}

func TestPlanPreservesCRLFAndInsertsAbsentIsland(t *testing.T) {
	source := "\ufeff" + strings.Join([]string{
		`import { Package, Workspace, define, dep, npm } from "tspack/manifest";`,
		`export default define(`,
		`  <Workspace name="demo">`,
		`    <Package`,
		`      name="app"`,
		`      version="1.0.0"`,
		`    />`,
		`  </Workspace>,`,
		`);`,
		``,
	}, "\r\n")
	added := testDeclaration("react", "^19", 0)
	edit := authoring.Add(authoring.PackageIR{}, added)
	lineStart := strings.Index(source, "    />")
	analysis := manifestfrontend.DependencySourceAnalysis{
		Status:      manifestfrontend.DependencyIslandAbsent,
		PackageName: "app",
		Insertion: &manifestfrontend.DependencyInsertion{
			Offset:          lineStart,
			Multiline:       true,
			AttributeIndent: "      ",
			ClosingIndent:   "    ",
		},
		ManifestImport: importAnalysis(t, source),
	}

	result := Plan(ProjectionRequest{
		SourceText:   source,
		ManifestPath: "manifest.tsx",
		PackageName:  "app",
		Analysis:     analysis,
		Edit:         edit,
	})

	assertNoDiagnostics(t, result)
	if !strings.HasPrefix(result.UpdatedSource, "\ufeff") {
		t.Fatal("projection removed UTF-8 BOM")
	}
	if strings.Contains(strings.ReplaceAll(result.UpdatedSource, "\r\n", ""), "\n") {
		t.Fatalf("projection introduced LF-only line endings:\n%q", result.UpdatedSource)
	}
	if !strings.Contains(result.UpdatedSource, `      dependencies={{ values: [dep(npm("react", "^19"))] }}`) {
		t.Fatalf("absent island was not inserted:\n%s", result.UpdatedSource)
	}
}

func TestPlanFailsSafelyForDynamicAmbiguousAndObservedDeclarations(t *testing.T) {
	declaration := testDeclaration("react", "^19", 0)
	edit := authoring.Add(authoring.PackageIR{}, declaration)
	for _, code := range []string{
		"TSPACK_MANIFEST_DEPENDENCIES_DYNAMIC",
		"TSPACK_MANIFEST_DEPENDENCY_ISLAND_AMBIGUOUS",
	} {
		result := Plan(ProjectionRequest{
			SourceText:   canonicalSource("\n"),
			ManifestPath: "manifest.tsx",
			PackageName:  "app",
			Analysis: manifestfrontend.DependencySourceAnalysis{
				Status:      manifestfrontend.DependencyIslandUserDynamic,
				Diagnostics: []diag.Diagnostic{{Code: code, Severity: diag.SeverityError}},
			},
			Edit: edit,
		})
		if result.Changed || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != code {
			t.Fatalf("safe failure for %s = %#v", code, result)
		}
	}

	observed := testDeclaration("react", "^19", 0)
	observed.Authority = authoring.AuthorityObserved
	observed.Editability = authoring.EditabilityObserved
	remove, err := authoring.Remove(authoring.PackageIR{Declarations: []authoring.DependencyDeclaration{observed}}, authoring.DeclarationSelector{ID: observed.ID})
	if err != nil {
		t.Fatal(err)
	}
	result := Plan(ProjectionRequest{
		SourceText:   canonicalSource("\n"),
		ManifestPath: "manifest.tsx",
		PackageName:  "app",
		Analysis:     canonicalAnalysis(t, canonicalSource("\n")),
		Edit:         remove,
	})
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "TSPACK_MANIFEST_EDIT_AUTHORITY_DENIED" {
		t.Fatalf("observed authority result = %#v", result)
	}

	annotationAnalysis := canonicalAnalysis(t, canonicalSource("\n"))
	annotationAnalysis.Authority = "annotation"
	annotationResult := Plan(ProjectionRequest{
		SourceText:   canonicalSource("\n"),
		ManifestPath: "package.manifest.tsx",
		PackageName:  "app",
		Analysis:     annotationAnalysis,
		Edit:         edit,
	})
	if len(annotationResult.Diagnostics) != 1 || annotationResult.Diagnostics[0].Code != "TSPACK_MANIFEST_EDIT_AUTHORITY_DENIED" {
		t.Fatalf("annotation authority result = %#v", annotationResult)
	}
}

func canonicalSource(eol string) string {
	lines := []string{
		`import { Package, Workspace, define, dep, npm, tool } from "tspack/manifest";`,
		``,
		`const untouched = { strange: true };`,
		`const deps = { react: dep(npm("react", "^19")), typescript: tool(npm("typescript", "^5.9")) };`,
		``,
		`export default define(`,
		`  <Workspace name="demo">`,
		`    <Package name="app" version="1.0.0" dependencies={{ values: [`,
		`      // island-level comment`,
		`      deps.react,`,
		`      // comment attached to typescript`,
		`      deps.typescript,`,
		`    ] }}>`,
		`      <Package />`,
		`    </Package>`,
		`  </Workspace>,`,
		`);`,
		``,
	}
	return strings.Join(lines, eol)
}

func canonicalAnalysis(t *testing.T, source string) manifestfrontend.DependencySourceAnalysis {
	t.Helper()
	contentMarker := "values: ["
	contentStart := strings.Index(source, contentMarker) + len(contentMarker)
	contentEnd := strings.Index(source[contentStart:], "] }}") + contentStart
	if contentStart < len(contentMarker) || contentEnd < contentStart {
		t.Fatalf("invalid test source:\n%s", source)
	}
	elementTexts := []string{"deps.react", "deps.typescript"}
	elements := make([]manifestfrontend.DependencyIslandElement, 0, len(elementTexts))
	searchStart := contentStart
	for _, text := range elementTexts {
		start := strings.Index(source[searchStart:contentEnd], text) + searchStart
		if start < searchStart {
			t.Fatalf("element %q not found", text)
		}
		elements = append(elements, manifestfrontend.DependencyIslandElement{
			SourceRange: manifestfrontend.SourceRange{Start: start, End: start + len(text)},
			FullStart:   start,
		})
		searchStart = start + len(text)
	}
	return manifestfrontend.DependencySourceAnalysis{
		Status:      manifestfrontend.DependencyIslandOwnedCanonical,
		PackageName: "app",
		Island: &manifestfrontend.DependencyIsland{
			SourceRange:  manifestfrontend.SourceRange{Start: strings.Index(source, "dependencies="), End: contentEnd + len("] }}")},
			ContentStart: contentStart,
			ContentEnd:   contentEnd,
			Elements:     elements,
		},
		ManifestImport: importAnalysis(t, source),
	}
}

func importAnalysis(t *testing.T, source string) *manifestfrontend.ManifestImport {
	t.Helper()
	start := strings.Index(source, "{") + 1
	end := strings.Index(source, "}")
	if start <= 0 || end < start {
		t.Fatalf("manifest import not found")
	}
	content := source[start:end]
	parts := strings.Split(content, ",")
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name != "" {
			names = append(names, name)
		}
	}
	return &manifestfrontend.ManifestImport{ContentStart: start, ContentEnd: end, Names: names}
}

func testDeclaration(id string, constraint string, order int) authoring.DependencyDeclaration {
	return authoring.DependencyDeclaration{
		ID:          id,
		Identity:    authoring.PackageIdentity{Source: "npm", Name: "react"},
		Source:      authoring.PackageSource{Kind: "npm", Package: "react", Range: constraint},
		Constraint:  constraint,
		Kind:        authoring.DependencyRuntime,
		Origin:      authoring.DeclarationOrigin{Kind: authoring.OriginProjectManifest, SourcePath: "manifest.tsx"},
		Layer:       authoring.LayerProject,
		Order:       order,
		Authority:   authoring.AuthorityOwned,
		Editability: authoring.EditabilityEditable,
	}
}

func hasChange(changes []authoring.AuthoringChange, kind authoring.ChangeKind) bool {
	for _, change := range changes {
		if change.Kind == kind {
			return true
		}
	}
	return false
}

func assertNoDiagnostics(t *testing.T, result ProjectionResult) {
	t.Helper()
	if len(result.Diagnostics) > 0 {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
}

func BenchmarkPlanDependencyAdd(b *testing.B) {
	source := canonicalSource("\n")
	analysis := canonicalAnalysisForBenchmark(source)
	beforeDeclaration := testDeclaration("react", "^19", 0)
	added := testDeclaration("lodash", "^4", 0)
	added.ID = "lodash"
	added.Identity.Name = "lodash"
	added.Source.Package = "lodash"
	added.Layer = authoring.LayerExplicit
	added.Origin = authoring.DeclarationOrigin{Kind: authoring.OriginExplicitUserOperation}
	edit := authoring.Add(authoring.PackageIR{Declarations: []authoring.DependencyDeclaration{beforeDeclaration}}, added)
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		result := Plan(ProjectionRequest{
			SourceText:   source,
			ManifestPath: "manifest.tsx",
			PackageName:  "app",
			Analysis:     analysis,
			Edit:         edit,
		})
		if !result.Changed {
			b.Fatal("benchmark projection did not change source")
		}
	}
}

func canonicalAnalysisForBenchmark(source string) manifestfrontend.DependencySourceAnalysis {
	contentMarker := "values: ["
	contentStart := strings.Index(source, contentMarker) + len(contentMarker)
	contentEnd := strings.Index(source[contentStart:], "] }}") + contentStart
	elementTexts := []string{"deps.react", "deps.typescript"}
	elements := make([]manifestfrontend.DependencyIslandElement, 0, len(elementTexts))
	searchStart := contentStart
	for _, text := range elementTexts {
		start := strings.Index(source[searchStart:contentEnd], text) + searchStart
		elements = append(elements, manifestfrontend.DependencyIslandElement{
			SourceRange: manifestfrontend.SourceRange{Start: start, End: start + len(text)},
			FullStart:   start,
		})
		searchStart = start + len(text)
	}
	importStart := strings.Index(source, "{") + 1
	importEnd := strings.Index(source, "}")
	return manifestfrontend.DependencySourceAnalysis{
		Status:      manifestfrontend.DependencyIslandOwnedCanonical,
		PackageName: "app",
		Island: &manifestfrontend.DependencyIsland{
			ContentStart: contentStart,
			ContentEnd:   contentEnd,
			Elements:     elements,
		},
		ManifestImport: &manifestfrontend.ManifestImport{
			ContentStart: importStart,
			ContentEnd:   importEnd,
			Names:        []string{"Package", "Workspace", "define", "dep", "npm", "tool"},
		},
	}
}
