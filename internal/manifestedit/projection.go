// Package manifestedit plans source-preserving edits to dependency declaration
// surfaces that the manifest frontend has proved structurally safe to edit.
package manifestedit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/authoring"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/manifestfrontend"
)

type SourceEdit struct {
	Start       int    `json:"start"`
	End         int    `json:"end"`
	Replacement string `json:"replacement"`
}

type ProjectionRequest struct {
	SourceText   string
	ManifestPath string
	PackageName  string
	Analysis     manifestfrontend.DependencySourceAnalysis
	Edit         authoring.EditResult
}

type ProjectionResult struct {
	UpdatedSource  string                      `json:"updatedSource"`
	Changed        bool                        `json:"changed"`
	Edits          []SourceEdit                `json:"edits"`
	Changes        []authoring.AuthoringChange `json:"changes"`
	Diagnostics    []diag.Diagnostic           `json:"diagnostics"`
	originalSource string
}

func PlanFile(frontendPath string, manifestPath string, packageName string, edit authoring.EditResult) (ProjectionResult, error) {
	source, err := os.ReadFile(manifestPath)
	if err != nil {
		return ProjectionResult{}, err
	}
	analysis, err := manifestfrontend.AnalyzeDependencies(frontendPath, manifestPath, packageName)
	if err != nil {
		return ProjectionResult{}, err
	}
	return Plan(ProjectionRequest{
		SourceText:   string(source),
		ManifestPath: manifestPath,
		PackageName:  packageName,
		Analysis:     analysis,
		Edit:         edit,
	}), nil
}

func Plan(request ProjectionRequest) ProjectionResult {
	result := ProjectionResult{
		UpdatedSource:  request.SourceText,
		Changes:        primaryChanges(request.Edit.Changes),
		originalSource: request.SourceText,
	}
	if len(result.Changes) == 0 {
		return result
	}
	if len(request.Analysis.Diagnostics) > 0 {
		result.Diagnostics = append(result.Diagnostics, request.Analysis.Diagnostics...)
		return result
	}
	if request.Analysis.Authority == "annotation" {
		result.Diagnostics = append(result.Diagnostics, projectionDiagnostic(
			request.ManifestPath,
			"TSPACK_MANIFEST_EDIT_AUTHORITY_DENIED",
			"package.manifest.tsx annotations classify package.json dependencies; package.json remains authoritative and this surface is not a native dependency write target",
		))
		return result
	}
	if request.Analysis.PackageName != "" && request.PackageName != "" && request.Analysis.PackageName != request.PackageName {
		result.Diagnostics = append(result.Diagnostics, projectionDiagnostic(
			request.ManifestPath,
			"TSPACK_MANIFEST_DEPENDENCY_PROJECTION_UNSAFE",
			"dependency source analysis belongs to a different package",
		))
		return result
	}

	declarations := sourceOrderedDeclarations(request.Edit.Before)
	edits := make([]SourceEdit, 0, len(result.Changes)+1)
	requiredHelpers := map[string]bool{}
	for _, change := range result.Changes {
		if diagnostic := validateChangeAuthority(request.ManifestPath, change); diagnostic != nil {
			result.Diagnostics = append(result.Diagnostics, *diagnostic)
			continue
		}
		switch change.Kind {
		case authoring.ChangeAdded:
			replacement, helpers, err := renderDeclaration(change.Declaration)
			if err != nil {
				result.Diagnostics = append(result.Diagnostics, projectionDiagnostic(request.ManifestPath, "TSPACK_MANIFEST_DEPENDENCY_PROJECTION_UNSAFE", err.Error()))
				continue
			}
			mergeHelpers(requiredHelpers, helpers)
			if request.Analysis.Status == manifestfrontend.DependencyIslandAbsent {
				edit, ok := absentIslandEdit(request, replacement)
				if !ok {
					result.Diagnostics = append(result.Diagnostics, projectionDiagnostic(request.ManifestPath, "TSPACK_MANIFEST_DEPENDENCY_PROJECTION_UNSAFE", "the absent dependency island has no safe AST insertion point"))
					continue
				}
				edits = append(edits, edit)
				continue
			}
			edit, ok := appendElementEdit(request.SourceText, request.Analysis.Island, replacement)
			if !ok {
				result.Diagnostics = append(result.Diagnostics, projectionDiagnostic(request.ManifestPath, "TSPACK_MANIFEST_DEPENDENCY_PROJECTION_UNSAFE", "the owned dependency values array is unavailable"))
				continue
			}
			edits = append(edits, edit)
		case authoring.ChangeRemoved:
			index := declarationIndex(declarations, change.Declaration)
			edit, ok := removeElementEdit(request.SourceText, request.Analysis.Island, index)
			if !ok {
				result.Diagnostics = append(result.Diagnostics, projectionDiagnostic(request.ManifestPath, "TSPACK_MANIFEST_DEPENDENCY_PROJECTION_UNSAFE", "the removed declaration does not map to one owned dependency element"))
				continue
			}
			edits = append(edits, edit)
		case authoring.ChangeChanged:
			previous := change.Declaration
			if change.Previous != nil {
				previous = *change.Previous
			}
			index := declarationIndex(declarations, previous)
			replacement, helpers, err := renderDeclaration(change.Declaration)
			if err != nil {
				result.Diagnostics = append(result.Diagnostics, projectionDiagnostic(request.ManifestPath, "TSPACK_MANIFEST_DEPENDENCY_PROJECTION_UNSAFE", err.Error()))
				continue
			}
			mergeHelpers(requiredHelpers, helpers)
			edit, ok := replaceElementEdit(request.Analysis.Island, index, replacement)
			if !ok {
				result.Diagnostics = append(result.Diagnostics, projectionDiagnostic(request.ManifestPath, "TSPACK_MANIFEST_DEPENDENCY_PROJECTION_UNSAFE", "the changed declaration does not map to one owned dependency element"))
				continue
			}
			edits = append(edits, edit)
		}
	}
	if len(result.Diagnostics) > 0 {
		return result
	}
	if importEdit, ok := missingImportEdit(request.SourceText, request.Analysis.ManifestImport, requiredHelpers); ok {
		edits = append(edits, importEdit)
	} else if hasMissingHelpers(request.Analysis.ManifestImport, requiredHelpers) {
		result.Diagnostics = append(result.Diagnostics, projectionDiagnostic(
			request.ManifestPath,
			"TSPACK_MANIFEST_DEPENDENCY_PROJECTION_UNSAFE",
			"the projection requires dependency helpers that are not available through one editable tspack/manifest named import",
		))
		return result
	}

	updated, err := applyEdits(request.SourceText, edits)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, projectionDiagnostic(request.ManifestPath, "TSPACK_MANIFEST_DEPENDENCY_PROJECTION_UNSAFE", err.Error()))
		return result
	}
	result.Edits = edits
	result.UpdatedSource = updated
	result.Changed = updated != request.SourceText
	return result
}

func WritePlannedFile(path string, result ProjectionResult) (bool, error) {
	if len(result.Diagnostics) > 0 {
		return false, fmt.Errorf("TSPACK_MANIFEST_WRITE_FAILED: projection has diagnostics")
	}
	if !result.Changed || result.UpdatedSource == result.originalSource {
		return false, nil
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("TSPACK_MANIFEST_WRITE_FAILED: read current source: %w", err)
	}
	if !bytes.Equal(current, []byte(result.originalSource)) {
		return false, fmt.Errorf("TSPACK_MANIFEST_SOURCE_CHANGED: manifest changed after projection was planned")
	}
	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("TSPACK_MANIFEST_WRITE_FAILED: stat current source: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".tspack-manifest-*.tmp")
	if err != nil {
		return false, fmt.Errorf("TSPACK_MANIFEST_WRITE_FAILED: create temporary source: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		_ = temporary.Close()
		return false, fmt.Errorf("TSPACK_MANIFEST_WRITE_FAILED: preserve source mode: %w", err)
	}
	if _, err := temporary.Write([]byte(result.UpdatedSource)); err != nil {
		_ = temporary.Close()
		return false, fmt.Errorf("TSPACK_MANIFEST_WRITE_FAILED: write temporary source: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return false, fmt.Errorf("TSPACK_MANIFEST_WRITE_FAILED: sync temporary source: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return false, fmt.Errorf("TSPACK_MANIFEST_WRITE_FAILED: close temporary source: %w", err)
	}
	if err := replaceFileAtomic(temporaryPath, path); err != nil {
		return false, fmt.Errorf("TSPACK_MANIFEST_WRITE_FAILED: atomically replace source: %w", err)
	}
	return true, nil
}

func primaryChanges(changes []authoring.AuthoringChange) []authoring.AuthoringChange {
	out := make([]authoring.AuthoringChange, 0, len(changes))
	for _, change := range changes {
		if change.Kind == authoring.ChangeAdded || change.Kind == authoring.ChangeRemoved || change.Kind == authoring.ChangeChanged {
			out = append(out, change)
		}
	}
	return out
}

func validateChangeAuthority(path string, change authoring.AuthoringChange) *diag.Diagnostic {
	declaration := change.Declaration
	if change.Kind == authoring.ChangeChanged && change.Previous != nil {
		declaration = *change.Previous
	}
	if change.Kind == authoring.ChangeAdded {
		declaration = change.Declaration
	}
	if declaration.Authority == authoring.AuthorityOwned && declaration.Editability == authoring.EditabilityEditable {
		return nil
	}
	diagnostic := projectionDiagnostic(path, "TSPACK_MANIFEST_EDIT_AUTHORITY_DENIED", "dependency projection is allowed only for owned, editable declarations")
	diagnostic.Details = []string{
		"authority: " + string(declaration.Authority),
		"editability: " + string(declaration.Editability),
		"dependency: " + declaration.Identity.Key(),
	}
	return &diagnostic
}

func sourceOrderedDeclarations(resolution authoring.TapeResolution) []authoring.DependencyDeclaration {
	declarations := make([]authoring.DependencyDeclaration, 0, len(resolution.Entries))
	for _, entry := range resolution.Entries {
		declarations = append(declarations, entry.Declaration)
	}
	sort.SliceStable(declarations, func(i int, j int) bool {
		return declarations[i].Order < declarations[j].Order
	})
	return declarations
}

func declarationIndex(declarations []authoring.DependencyDeclaration, target authoring.DependencyDeclaration) int {
	for index, declaration := range declarations {
		if target.ID != "" && declaration.ID == target.ID {
			return index
		}
		if declaration.Order == target.Order && declaration.Kind == target.Kind && declaration.Identity == target.Identity && declaration.Key == target.Key {
			return index
		}
	}
	return -1
}

func replaceElementEdit(island *manifestfrontend.DependencyIsland, index int, replacement string) (SourceEdit, bool) {
	if island == nil || index < 0 || index >= len(island.Elements) {
		return SourceEdit{}, false
	}
	element := island.Elements[index]
	return SourceEdit{Start: element.Start, End: element.End, Replacement: replacement}, true
}

func removeElementEdit(source string, island *manifestfrontend.DependencyIsland, index int) (SourceEdit, bool) {
	if island == nil || index < 0 || index >= len(island.Elements) {
		return SourceEdit{}, false
	}
	elements := island.Elements
	current := elements[index]
	if len(elements) == 1 {
		return SourceEdit{Start: current.Start, End: trailingEntryEnd(source, current.End, island.ContentEnd)}, true
	}
	if index < len(elements)-1 {
		end := boundaryBeforeNextComment(source, current.End, elements[index+1].Start)
		return SourceEdit{Start: current.Start, End: end}, true
	}
	previous := elements[index-1]
	start := previous.End
	if start > len(source) || current.End > len(source) {
		return SourceEdit{}, false
	}
	return SourceEdit{Start: start, End: trailingEntryEnd(source, current.End, island.ContentEnd)}, true
}

func boundaryBeforeNextComment(source string, currentEnd int, nextStart int) int {
	if currentEnd < 0 || nextStart > len(source) || currentEnd > nextStart {
		return nextStart
	}
	between := source[currentEnd:nextStart]
	commentOffset := firstCommentOffset(between)
	if commentOffset < 0 {
		return nextStart
	}
	beforeComment := between[:commentOffset]
	if !strings.ContainsAny(beforeComment, "\r\n") {
		return nextStart
	}
	return currentEnd + commentOffset
}

func trailingEntryEnd(source string, currentEnd int, contentEnd int) int {
	if currentEnd < 0 || contentEnd > len(source) || currentEnd > contentEnd {
		return currentEnd
	}
	trailing := source[currentEnd:contentEnd]
	commentOffset := firstCommentOffset(trailing)
	if commentOffset < 0 || strings.ContainsAny(trailing[:commentOffset], "\r\n") {
		return currentEnd
	}
	lineEnd := strings.IndexAny(trailing[commentOffset:], "\r\n")
	if lineEnd < 0 {
		return contentEnd
	}
	end := currentEnd + commentOffset + lineEnd
	if source[end] == '\r' && end+1 < len(source) && source[end+1] == '\n' {
		return end + 2
	}
	return end + 1
}

func firstCommentOffset(text string) int {
	line := strings.Index(text, "//")
	block := strings.Index(text, "/*")
	if line < 0 {
		return block
	}
	if block < 0 || line < block {
		return line
	}
	return block
}

func appendElementEdit(source string, island *manifestfrontend.DependencyIsland, replacement string) (SourceEdit, bool) {
	if island == nil || island.ContentStart < 0 || island.ContentEnd < island.ContentStart || island.ContentEnd > len(source) {
		return SourceEdit{}, false
	}
	eol := lineEnding(source)
	content := source[island.ContentStart:island.ContentEnd]
	if strings.Contains(content, "\n") || strings.Contains(content, "\r") {
		lineStart := strings.LastIndexAny(source[:island.ContentEnd], "\r\n") + 1
		closingIndent := source[lineStart:island.ContentEnd]
		entryIndent := closingIndent + indentUnit(source)
		if len(island.Elements) > 0 {
			entryLineStart := strings.LastIndexAny(source[:island.Elements[len(island.Elements)-1].Start], "\r\n") + 1
			entryIndent = source[entryLineStart:island.Elements[len(island.Elements)-1].Start]
		}
		return SourceEdit{
			Start:       lineStart,
			End:         lineStart,
			Replacement: entryIndent + replacement + "," + eol,
		}, true
	}
	trimmedEnd := island.ContentEnd
	for trimmedEnd > island.ContentStart && (source[trimmedEnd-1] == ' ' || source[trimmedEnd-1] == '\t') {
		trimmedEnd--
	}
	if len(island.Elements) == 0 {
		return SourceEdit{Start: island.ContentStart, End: island.ContentEnd, Replacement: replacement}, true
	}
	separator := ", "
	if trimmedEnd > 0 && source[trimmedEnd-1] == ',' {
		separator = " "
	}
	return SourceEdit{Start: trimmedEnd, End: trimmedEnd, Replacement: separator + replacement}, true
}

func absentIslandEdit(request ProjectionRequest, replacement string) (SourceEdit, bool) {
	insertion := request.Analysis.Insertion
	if insertion == nil || insertion.Offset < 0 || insertion.Offset > len(request.SourceText) {
		return SourceEdit{}, false
	}
	if !insertion.Multiline {
		return SourceEdit{Start: insertion.Offset, End: insertion.Offset, Replacement: " dependencies={{ values: [" + replacement + "] }}"}, true
	}
	eol := lineEnding(request.SourceText)
	replacementText := insertion.AttributeIndent + "dependencies={{ values: [" + replacement + "] }}" + eol
	return SourceEdit{Start: insertion.Offset, End: insertion.Offset, Replacement: replacementText}, true
}

func missingImportEdit(source string, manifestImport *manifestfrontend.ManifestImport, required map[string]bool) (SourceEdit, bool) {
	missing := missingHelpers(manifestImport, required)
	if len(missing) == 0 || manifestImport == nil {
		return SourceEdit{}, false
	}
	if manifestImport.ContentStart < 0 || manifestImport.ContentEnd < manifestImport.ContentStart || manifestImport.ContentEnd > len(source) {
		return SourceEdit{}, false
	}
	sort.Strings(missing)
	content := source[manifestImport.ContentStart:manifestImport.ContentEnd]
	if strings.Contains(content, "\n") || strings.Contains(content, "\r") {
		eol := lineEnding(source)
		lineStart := strings.LastIndexAny(source[:manifestImport.ContentEnd], "\r\n") + 1
		indent := indentUnit(source)
		var builder strings.Builder
		for _, name := range missing {
			builder.WriteString(indent)
			builder.WriteString(name)
			builder.WriteString(",")
			builder.WriteString(eol)
		}
		return SourceEdit{Start: lineStart, End: lineStart, Replacement: builder.String()}, true
	}
	separator := ""
	trimmedEnd := manifestImport.ContentEnd
	for trimmedEnd > manifestImport.ContentStart && (source[trimmedEnd-1] == ' ' || source[trimmedEnd-1] == '\t') {
		trimmedEnd--
	}
	if strings.TrimSpace(content) != "" && (trimmedEnd == manifestImport.ContentStart || source[trimmedEnd-1] != ',') {
		separator = ", "
	} else if strings.TrimSpace(content) != "" {
		separator = " "
	}
	return SourceEdit{Start: trimmedEnd, End: trimmedEnd, Replacement: separator + strings.Join(missing, ", ")}, true
}

func hasMissingHelpers(manifestImport *manifestfrontend.ManifestImport, required map[string]bool) bool {
	return len(missingHelpers(manifestImport, required)) > 0
}

func missingHelpers(manifestImport *manifestfrontend.ManifestImport, required map[string]bool) []string {
	available := map[string]bool{}
	if manifestImport != nil {
		for _, name := range manifestImport.Names {
			available[name] = true
		}
	}
	var missing []string
	for name := range required {
		if !available[name] {
			missing = append(missing, name)
		}
	}
	return missing
}

func renderDeclaration(declaration authoring.DependencyDeclaration) (string, []string, error) {
	sourceExpression, sourceHelper, err := renderSource(declaration.Source)
	if err != nil {
		return "", nil, err
	}
	kind := string(declaration.Kind)
	if kind != "dep" && kind != "peer" && kind != "tool" {
		return "", nil, fmt.Errorf("dependency kind %q has no manifest helper", declaration.Kind)
	}
	options := make([]string, 0, 2)
	if declaration.Key != "" {
		options = append(options, "key: "+quote(declaration.Key))
	}
	if declaration.Layer == authoring.LayerExplicit || declaration.Origin.Kind == authoring.OriginExplicitUserOperation {
		origin := "origin: { kind: " + quote(string(authoring.OriginExplicitUserOperation))
		if declaration.Origin.Name != "" {
			origin += ", name: " + quote(declaration.Origin.Name)
		}
		origin += " }"
		metadata := "declaration: { " + origin +
			", layer: " + quote(string(authoring.LayerExplicit)) +
			", authority: " + quote(string(authoring.AuthorityOwned)) +
			", editability: " + quote(string(authoring.EditabilityEditable)) + " }"
		options = append(options, metadata)
	}
	if declaration.Optional {
		options = append(options, "optional: true")
	}
	expression := kind + "(" + sourceExpression
	if len(options) > 0 {
		expression += ", { " + strings.Join(options, ", ") + " }"
	}
	expression += ")"
	return expression, []string{kind, sourceHelper}, nil
}

func renderSource(source authoring.PackageSource) (string, string, error) {
	switch source.Kind {
	case "npm":
		return "npm(" + quote(source.Package) + ", " + quote(source.Range) + ")", "npm", nil
	case "workspace":
		return "workspace(" + quote(source.Name) + ")", "workspace", nil
	case "path":
		return "path(" + quote(source.Path) + ")", "path", nil
	case "git":
		options := make([]string, 0, 3)
		if source.Tag != "" {
			options = append(options, "tag: "+quote(source.Tag))
		}
		if source.Rev != "" {
			options = append(options, "rev: "+quote(source.Rev))
		}
		if source.Branch != "" {
			options = append(options, "branch: "+quote(source.Branch))
		}
		expression := "git(" + quote(source.Ref)
		if len(options) > 0 {
			expression += ", { " + strings.Join(options, ", ") + " }"
		}
		expression += ")"
		return expression, "git", nil
	default:
		return "", "", fmt.Errorf("dependency source %q is not supported by the manifest projector", source.Kind)
	}
}

func quote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func mergeHelpers(destination map[string]bool, helpers []string) {
	for _, helper := range helpers {
		destination[helper] = true
	}
}

func applyEdits(source string, edits []SourceEdit) (string, error) {
	sorted := append([]SourceEdit(nil), edits...)
	sort.SliceStable(sorted, func(i int, j int) bool {
		return sorted[i].Start > sorted[j].Start
	})
	updated := source
	previousStart := len(source) + 1
	for _, edit := range sorted {
		if edit.Start < 0 || edit.End < edit.Start || edit.End > len(source) {
			return "", fmt.Errorf("source edit range [%d:%d] is outside the manifest", edit.Start, edit.End)
		}
		if edit.End > previousStart {
			return "", fmt.Errorf("source edit ranges overlap")
		}
		updated = updated[:edit.Start] + edit.Replacement + updated[edit.End:]
		previousStart = edit.Start
	}
	return updated, nil
}

func projectionDiagnostic(path string, code string, message string) diag.Diagnostic {
	return diag.Diagnostic{
		Code:     code,
		Severity: diag.SeverityError,
		Message:  message,
		File:     path,
	}
}

func lineEnding(source string) string {
	if strings.Contains(source, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func indentUnit(source string) string {
	for _, line := range strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "\t") {
			return "\t"
		}
		spaces := len(line) - len(strings.TrimLeft(line, " "))
		if spaces > 0 {
			return strings.Repeat(" ", spaces)
		}
	}
	return "  "
}
