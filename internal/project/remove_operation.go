package project

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/authoring"
	"github.com/yuechen-li-dev/tspack/internal/bridge"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifestedit"
	"github.com/yuechen-li-dev/tspack/internal/resolver"
)

// RemoveDependencyRequest describes removal of one editable authoring
// declaration. It deliberately does not request removal from the resolved graph.
type RemoveDependencyRequest struct {
	Project          Options
	PackageSpec      string
	TargetPackage    string
	WorkingDirectory string
	Kind             authoring.DependencyKind
	Source           string
	Optional         *bool
	DryRun           bool
}

// RemoveDependencyResult keeps authoring truth separate from resolved truth.
type RemoveDependencyResult struct {
	Package                   string
	Source                    string
	Kind                      authoring.DependencyKind
	Optional                  bool
	RemovedConstraint         string
	TargetPackage             string
	ManifestPath              string
	DeclarationRemoved        bool
	ManifestChanged           bool
	LockChanged               bool
	StillDeclared             bool
	StillRequired             bool
	StillResolved             bool
	ResolvedStatusKnown       bool
	LockPackageRemoved        bool
	NoEditableDeclaration     bool
	RemovedDeclaration        *authoring.DependencyDeclaration
	NewlyEffectiveDeclaration *authoring.DependencyDeclaration
	RemainingDeclarations     []authoring.DependencyDeclaration
	SemanticChanges           []authoring.AuthoringChange
	LockDiff                  *lockfile.Diff
	DryRun                    bool
	Performance               RemoveDependencyPerformance
	Diagnostics               []diag.Diagnostic
}

type RemoveDependencyPerformance struct {
	ManifestLoad             time.Duration
	SemanticRemoval          time.Duration
	Projection               time.Duration
	Update                   time.Duration
	Total                    time.Duration
	RegistryMetadataRequests int
	RegistryTarballRequests  int
}

func ParseRemovePackageSelector(value string) (authoring.PackageIdentity, error) {
	parsed, err := ParsePackageSpec(value)
	if err != nil {
		return authoring.PackageIdentity{}, err
	}
	if parsed.ExplicitConstraint != "" {
		return authoring.PackageIdentity{}, fmt.Errorf("remove selects an authored declaration by package name; use `tspack remove %s`", parsed.Identity.Name)
	}
	return parsed.Identity, nil
}

func RunRemoveDependency(request RemoveDependencyRequest) (result RemoveDependencyResult) {
	result.DryRun = request.DryRun
	totalStarted := time.Now()
	defer func() {
		result.Performance.Total = time.Since(totalStarted)
	}()
	identity, err := ParseRemovePackageSelector(request.PackageSpec)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, removeDiagnostic("TSPACK_REMOVE_PACKAGE_SPEC_INVALID", "invalid package selector", err.Error()))
		return result
	}
	result.Package = identity.Name
	result.Source = identity.Source
	if request.Source != "" {
		if request.Source != string(authoring.SourceNPM) && request.Source != string(authoring.SourceJSR) {
			result.Diagnostics = append(result.Diagnostics, removeDiagnostic("TSPACK_REMOVE_SOURCE_UNSUPPORTED", "dependency source is not supported", request.Source, "Use --source npm or --source jsr."))
			return result
		}
		identity.Source = request.Source
		result.Source = request.Source
	}
	if request.Kind != "" && request.Kind != authoring.DependencyRuntime && request.Kind != authoring.DependencyPeer && request.Kind != authoring.DependencyTool && request.Kind != authoring.DependencyTest {
		result.Diagnostics = append(result.Diagnostics, removeDiagnostic("TSPACK_REMOVE_KIND_UNSUPPORTED", "dependency kind selector is not supported", string(request.Kind), "Use dep, peer, tool, or test."))
		return result
	}

	manifestLoadStarted := time.Now()
	ir, diagnostics := loadManifestIR(request.Project)
	result.Performance.ManifestLoad = time.Since(manifestLoadStarted)
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	if hasErrors(result.Diagnostics) || ir == nil {
		return result
	}
	target, targetDiagnostics := selectDependencyEditTarget(ir.Packages, request.TargetPackage, request.Project.RootDir, request.WorkingDirectory, "TSPACK_REMOVE")
	result.Diagnostics = append(result.Diagnostics, targetDiagnostics...)
	if hasErrors(result.Diagnostics) {
		return result
	}
	result.TargetPackage = target.Name
	manifestPath, pathErr := addManifestPath(request.Project, target)
	if pathErr != nil {
		result.Diagnostics = append(result.Diagnostics, removeDiagnostic("TSPACK_REMOVE_MANIFEST_TARGET_INVALID", "cannot identify an editable native manifest for the selected package", pathErr.Error()))
		return result
	}
	result.ManifestPath = manifestPath

	if target.DependencyAuthoring == nil {
		result.NoEditableDeclaration = true
		populateCurrentResolvedStatus(&result, request.Project.LockfilePath)
		return result
	}
	if request.Source == "" {
		sources := declarationSourcesForName(*target.DependencyAuthoring, identity.Name)
		if len(sources) > 1 {
			result.Source = ""
			result.Diagnostics = append(result.Diagnostics, removeDiagnostic(
				"TSPACK_REMOVE_SOURCE_AMBIGUOUS",
				"the package name is declared from more than one source",
				"package: "+identity.Name,
				"sources: "+strings.Join(sources, ", "),
				"Use --source npm or --source jsr to select one declaration.",
			))
			return result
		}
		if len(sources) == 1 {
			identity.Source = sources[0]
			result.Source = sources[0]
		}
	}
	allMatches := declarationsForIdentity(*target.DependencyAuthoring, identity)
	editable := filterRemoveCandidates(allMatches, request.Kind, request.Optional)
	if len(editable) == 0 {
		result.NoEditableDeclaration = true
		result.RemainingDeclarations = append(result.RemainingDeclarations, effectiveIdentityDeclarations(authoring.Build(target.DependencyAuthoring.Declarations), identity)...)
		result.StillDeclared = len(result.RemainingDeclarations) > 0
		result.StillRequired = result.StillDeclared
		populateCurrentResolvedStatus(&result, request.Project.LockfilePath)
		return result
	}
	if len(editable) > 1 {
		result.Diagnostics = append(result.Diagnostics, removeDiagnostic("TSPACK_REMOVE_DECLARATION_AMBIGUOUS", "several editable declarations match the package", removeDeclarationDetails(editable)...))
		return result
	}

	removed := editable[0]
	result.Kind = removed.Kind
	result.Optional = removed.Optional
	result.RemovedConstraint = removed.Constraint
	result.RemovedDeclaration = &removed
	semanticRemovalStarted := time.Now()
	edit, removeErr := authoring.Remove(
		*target.DependencyAuthoring,
		authoring.DeclarationSelector{
			ID:           removed.ID,
			Identity:     &identity,
			Kind:         removed.Kind,
			SourcePath:   removed.Origin.SourcePath,
			EditableOnly: true,
		},
	)
	result.Performance.SemanticRemoval = time.Since(semanticRemovalStarted)
	if removeErr != nil {
		var ambiguous authoring.AmbiguousRemovalError
		if errors.As(removeErr, &ambiguous) {
			result.Diagnostics = append(result.Diagnostics, removeDiagnostic("TSPACK_REMOVE_DECLARATION_AMBIGUOUS", "several editable declarations match the package", removeDeclarationDetails(ambiguous.Matches)...))
		} else {
			result.Diagnostics = append(result.Diagnostics, removeDiagnostic("TSPACK_REMOVE_SEMANTIC_EDIT_FAILED", "dependency authoring edit failed", removeErr.Error()))
		}
		return result
	}
	if authoring.HasFatalConflicts(edit.After) {
		result.Diagnostics = append(result.Diagnostics, removeDiagnostic("TSPACK_REMOVE_SEMANTIC_CONFLICT", "dependency authoring edit produced a fatal conflict"))
		return result
	}
	result.DeclarationRemoved = true
	result.SemanticChanges = append([]authoring.AuthoringChange(nil), edit.Changes...)
	result.RemainingDeclarations = append(result.RemainingDeclarations, effectiveIdentityDeclarations(edit.After, identity)...)
	result.StillDeclared = len(result.RemainingDeclarations) > 0
	result.StillRequired = result.StillDeclared
	for _, change := range edit.Changes {
		if change.Kind == authoring.ChangeUnshadowed {
			declaration := change.Declaration
			result.NewlyEffectiveDeclaration = &declaration
			break
		}
	}

	frontendPath := request.Project.FrontendCLIPath
	if frontendPath == "" {
		frontendPath = bridge.ResolveWithOptions("cli.js", bridge.ResolveOptions{ProjectRoot: request.Project.RootDir}).Path
	}
	if frontendPath == "" {
		result.Diagnostics = append(result.Diagnostics, removeDiagnostic("TSPACK_REMOVE_FRONTEND_MISSING", "manifest frontend CLI not found"))
		return result
	}
	projectionStarted := time.Now()
	projection, projectionErr := manifestedit.PlanFile(frontendPath, manifestPath, target.Name, edit)
	result.Performance.Projection = time.Since(projectionStarted)
	if projectionErr != nil {
		result.Diagnostics = append(result.Diagnostics, removeDiagnostic("TSPACK_REMOVE_PROJECTION_FAILED", "failed to plan manifest projection", projectionErr.Error()))
		return result
	}
	result.Diagnostics = append(result.Diagnostics, projection.Diagnostics...)
	if hasErrors(result.Diagnostics) || !projection.Changed {
		return result
	}
	result.ManifestChanged = true
	if request.DryRun {
		// Resolution is intentionally left unknown: update cannot consume an
		// unwritten TSX projection without violating dry-run's no-write contract.
		return result
	}

	written, writeErr := manifestedit.WritePlannedFile(manifestPath, projection)
	if writeErr != nil || !written {
		details := []string{}
		if writeErr != nil {
			details = append(details, writeErr.Error())
		}
		result.Diagnostics = append(result.Diagnostics, removeDiagnostic("TSPACK_REMOVE_MANIFEST_WRITE_FAILED", "failed to write manifest projection", details...))
		result.ManifestChanged = false
		return result
	}

	updateOptions := request.Project
	registryClient := updateOptions.ResolverClient
	if registryClient == nil {
		registryClient = resolver.NewHTTPRegistryClient("")
	}
	memoClient := newDependencyEditMemoClient(registryClient)
	updateOptions.ResolverClient = memoClient
	updateStarted := time.Now()
	defer func() {
		result.Performance.Update = time.Since(updateStarted)
		result.Performance.RegistryMetadataRequests, result.Performance.RegistryTarballRequests = memoClient.RequestCounts()
	}()
	preflight := RunUpdate(UpdateRequest{Project: updateOptions, DryRun: true})
	if hasErrors(preflight.Diagnostics) {
		result.Diagnostics = append(result.Diagnostics, preflight.Diagnostics...)
		rollbackRemoveManifest(&result, manifestPath, projection)
		return result
	}
	updated := RunUpdate(UpdateRequest{Project: updateOptions})
	result.Diagnostics = append(result.Diagnostics, updated.Diagnostics...)
	result.LockDiff = updated.LockDiff
	result.LockChanged = lockDiffChanged(updated.LockDiff)
	if hasErrors(updated.Diagnostics) {
		if !addHasDiagnosticCode(updated.Diagnostics, "TSPACK_UPDATE_WRITE_FAILED") {
			rollbackRemoveManifest(&result, manifestPath, projection)
		}
		return result
	}
	populateCurrentResolvedStatus(&result, request.Project.LockfilePath)
	result.LockPackageRemoved = !result.StillResolved && lockDiffRemovesPackage(updated.LockDiff, result.Source, result.Package)
	return result
}

func declarationSourcesForName(ir authoring.PackageIR, name string) []string {
	sourceSet := map[string]bool{}
	for _, declaration := range ir.Declarations {
		identity := declaration.Identity
		if !identity.Valid() {
			identity = declaration.Source.Identity()
		}
		if identity.Name == name {
			sourceSet[identity.Source] = true
		}
	}
	sources := make([]string, 0, len(sourceSet))
	for source := range sourceSet {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	return sources
}

func declarationsForIdentity(ir authoring.PackageIR, identity authoring.PackageIdentity) []authoring.DependencyDeclaration {
	declarations := []authoring.DependencyDeclaration{}
	for _, declaration := range ir.Declarations {
		declarationIdentity := declaration.Identity
		if !declarationIdentity.Valid() {
			declarationIdentity = declaration.Source.Identity()
		}
		if declarationIdentity == identity {
			declarations = append(declarations, declaration)
		}
	}
	return declarations
}

func filterRemoveCandidates(declarations []authoring.DependencyDeclaration, kind authoring.DependencyKind, optional *bool) []authoring.DependencyDeclaration {
	candidates := []authoring.DependencyDeclaration{}
	for _, declaration := range declarations {
		if declaration.Authority != authoring.AuthorityOwned || declaration.Editability != authoring.EditabilityEditable {
			continue
		}
		if kind != "" && declaration.Kind != kind {
			continue
		}
		if optional != nil && declaration.Optional != *optional {
			continue
		}
		candidates = append(candidates, declaration)
	}
	return candidates
}

func effectiveIdentityDeclarations(resolution authoring.TapeResolution, identity authoring.PackageIdentity) []authoring.DependencyDeclaration {
	declarations := []authoring.DependencyDeclaration{}
	for _, entry := range resolution.Entries {
		if !entry.Effective {
			continue
		}
		entryIdentity := entry.Declaration.Identity
		if !entryIdentity.Valid() {
			entryIdentity = entry.Declaration.Source.Identity()
		}
		if entryIdentity == identity {
			declarations = append(declarations, entry.Declaration)
		}
	}
	return declarations
}

func removeDeclarationDetails(declarations []authoring.DependencyDeclaration) []string {
	details := make([]string, 0, len(declarations))
	for _, declaration := range declarations {
		detail := fmt.Sprintf("%s source=%s kind=%s optional=%t constraint=%s", declaration.Identity.Name, declaration.Identity.Source, declaration.Kind, declaration.Optional, declaration.Constraint)
		if declaration.Origin.SourcePath != "" {
			detail += " manifest=" + declaration.Origin.SourcePath
		}
		details = append(details, detail)
	}
	return details
}

func populateCurrentResolvedStatus(result *RemoveDependencyResult, path string) {
	if path == "" {
		return
	}
	locked, diagnostics, err := lockfile.LoadFile(path)
	if err != nil || hasErrors(diagnostics) {
		return
	}
	result.ResolvedStatusKnown = true
	for _, pkg := range locked.Packages {
		if pkg.Name == result.Package && (result.Source == "" || pkg.Source == result.Source) {
			result.StillResolved = true
			return
		}
	}
}

func lockDiffRemovesPackage(diff *lockfile.Diff, source string, name string) bool {
	if diff == nil {
		return false
	}
	for _, pkg := range diff.PackagesRemoved {
		if pkg.Source == source && pkg.Name == name {
			return true
		}
	}
	return false
}

func rollbackRemoveManifest(result *RemoveDependencyResult, path string, projection manifestedit.ProjectionResult) {
	if err := manifestedit.RestorePlannedFile(path, projection); err != nil {
		result.Diagnostics = append(result.Diagnostics, removeDiagnostic("TSPACK_REMOVE_ROLLBACK_FAILED", "failed to restore the manifest after update failure", err.Error()))
		return
	}
	result.ManifestChanged = false
}

func removeDiagnostic(code string, message string, details ...string) diag.Diagnostic {
	return diag.Diagnostic{Code: code, Severity: diag.SeverityError, Message: message, Details: details}
}
