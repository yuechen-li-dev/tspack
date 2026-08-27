package project

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	semver "github.com/Masterminds/semver/v3"
	"github.com/yuechen-li-dev/tspack/internal/authoring"
	"github.com/yuechen-li-dev/tspack/internal/bridge"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/manifestedit"
	"github.com/yuechen-li-dev/tspack/internal/resolver"
)

// AddDependencyRequest describes dependency authoring intent independently of
// command-line spelling.
type AddDependencyRequest struct {
	Project       Options
	PackageSpec   string
	Kind          authoring.DependencyKind
	Source        *authoring.PackageSource
	TargetPackage string
	Optional      bool
	DryRun        bool
}

type ParsedPackageSpec struct {
	Identity           authoring.PackageIdentity
	ExplicitConstraint string
}

type AddDependencyResult struct {
	Package              string
	Source               string
	Kind                 authoring.DependencyKind
	Optional             bool
	RequestedConstraint  string
	SelectedVersion      string
	WrittenConstraint    string
	TargetPackage        string
	ManifestPath         string
	ManifestChanged      bool
	LockChanged          bool
	AlreadyPresent       bool
	DryRun               bool
	SemanticChanges      []authoring.AuthoringChange
	ShadowedDeclarations []authoring.DependencyDeclaration
	LockDiff             *lockfile.Diff
	Diagnostics          []diag.Diagnostic
}

func ParsePackageSpec(value string) (ParsedPackageSpec, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return ParsedPackageSpec{}, fmt.Errorf("package specification is empty")
	}
	if strings.ContainsAny(value, " \\#") || strings.Contains(value, ":") || strings.Contains(value, "/../") {
		return ParsedPackageSpec{}, fmt.Errorf("unsupported npm package specification %q", value)
	}

	name := value
	constraint := ""
	if strings.HasPrefix(value, "@") {
		slash := strings.Index(value, "/")
		if slash <= 1 || slash == len(value)-1 {
			return ParsedPackageSpec{}, fmt.Errorf("invalid scoped npm package specification %q", value)
		}
		if separator := strings.LastIndex(value, "@"); separator > slash {
			name = value[:separator]
			constraint = value[separator+1:]
		}
	} else if separator := strings.LastIndex(value, "@"); separator >= 0 {
		name = value[:separator]
		constraint = value[separator+1:]
	}
	if name == "" || constraint == "" && strings.HasSuffix(value, "@") {
		return ParsedPackageSpec{}, fmt.Errorf("invalid npm package specification %q", value)
	}
	if strings.Count(name, "/") > 1 || (!strings.HasPrefix(name, "@") && strings.Contains(name, "/")) {
		return ParsedPackageSpec{}, fmt.Errorf("unsupported npm package name %q", name)
	}
	if constraint != "" {
		if _, err := semver.NewConstraint(constraint); err != nil {
			return ParsedPackageSpec{}, fmt.Errorf("invalid npm version constraint %q: %w", constraint, err)
		}
	}
	return ParsedPackageSpec{
		Identity:           authoring.PackageIdentity{Source: string(authoring.SourceNPM), Name: name},
		ExplicitConstraint: constraint,
	}, nil
}

func RunAddDependency(request AddDependencyRequest) AddDependencyResult {
	result := AddDependencyResult{DryRun: request.DryRun, Optional: request.Optional}
	parsed, err := ParsePackageSpec(request.PackageSpec)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, addDiagnostic("TSPACK_ADD_PACKAGE_SPEC_INVALID", "invalid package specification", err.Error()))
		return result
	}

	result.Package = parsed.Identity.Name
	result.Source = parsed.Identity.Source
	result.RequestedConstraint = parsed.ExplicitConstraint
	kind := request.Kind
	if kind == "" {
		kind = authoring.DependencyRuntime
	}
	result.Kind = kind
	if kind != authoring.DependencyRuntime && kind != authoring.DependencyPeer && kind != authoring.DependencyTool {
		result.Diagnostics = append(result.Diagnostics, addDiagnostic("TSPACK_ADD_KIND_UNSUPPORTED", "dependency kind cannot be projected into a native manifest", string(kind)))
		return result
	}

	ir, diagnostics := loadManifestIR(request.Project)
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	if hasErrors(result.Diagnostics) || ir == nil {
		return result
	}
	target, targetDiagnostics := selectAddTarget(ir, request.TargetPackage)
	result.Diagnostics = append(result.Diagnostics, targetDiagnostics...)
	if hasErrors(result.Diagnostics) {
		return result
	}
	result.TargetPackage = target.Name
	manifestPath, pathErr := addManifestPath(request.Project, target)
	if pathErr != nil {
		result.Diagnostics = append(result.Diagnostics, addDiagnostic("TSPACK_ADD_MANIFEST_TARGET_INVALID", "cannot identify an editable native manifest for the selected package", pathErr.Error()))
		return result
	}
	result.ManifestPath = manifestPath

	identity := parsed.Identity
	if request.Source != nil {
		identity = request.Source.Identity()
		if !identity.Valid() {
			result.Diagnostics = append(result.Diagnostics, addDiagnostic("TSPACK_ADD_SOURCE_INVALID", "dependency source has no package identity"))
			return result
		}
		if request.Source.Kind != string(authoring.SourceNPM) {
			result.Diagnostics = append(result.Diagnostics, addDiagnostic("TSPACK_ADD_SOURCE_UNSUPPORTED", "M69c supports npm as the add source", request.Source.Kind))
			return result
		}
		result.Package = identity.Name
		result.Source = identity.Source
	}

	editable := editableDeclarations(*target.DependencyAuthoring, identity)
	if len(editable) > 1 {
		result.Diagnostics = append(result.Diagnostics, addDiagnostic("TSPACK_ADD_DECLARATION_AMBIGUOUS", "several editable declarations match the package", declarationDetails(editable)...))
		return result
	}
	if len(editable) == 1 && parsed.ExplicitConstraint == "" && editable[0].Kind == kind && editable[0].Optional == request.Optional {
		result.WrittenConstraint = editable[0].Constraint
		result.AlreadyPresent = true
		return result
	}

	client := request.Project.ResolverClient
	if client == nil {
		client = resolver.NewHTTPRegistryClient("")
	}
	client = newDependencyEditMemoClient(client)
	metadata, metadataErr := client.PackageMetadata(context.Background(), identity.Name)
	if metadataErr != nil {
		result.Diagnostics = append(result.Diagnostics, addDiagnostic("TSPACK_ADD_METADATA_FETCH_FAILED", "failed to fetch npm package metadata", identity.Name, metadataErr.Error()))
		return result
	}
	writtenConstraint, selectedVersion, selectionErr := selectAddConstraint(metadata, parsed.ExplicitConstraint)
	if selectionErr != nil {
		result.Diagnostics = append(result.Diagnostics, addDiagnostic("TSPACK_ADD_VERSION_SELECTION_FAILED", "failed to select an npm package version", identity.Name, selectionErr.Error()))
		return result
	}
	result.WrittenConstraint = writtenConstraint
	result.SelectedVersion = selectedVersion

	source := authoring.PackageSource{Kind: string(authoring.SourceNPM), Package: identity.Name, Range: writtenConstraint}
	if request.Source != nil {
		source = *request.Source
		source.Range = writtenConstraint
	}
	declaration := authoring.DependencyDeclaration{
		Identity:    identity,
		Source:      source,
		Constraint:  writtenConstraint,
		Kind:        kind,
		Optional:    request.Optional,
		Origin:      authoring.DeclarationOrigin{Kind: authoring.OriginExplicitUserOperation, Name: "tspack add", SourcePath: filepath.ToSlash(target.ManifestPath)},
		Layer:       authoring.LayerExplicit,
		Authority:   authoring.AuthorityOwned,
		Editability: authoring.EditabilityEditable,
	}

	var edit authoring.EditResult
	if len(editable) == 1 {
		selector := authoring.DeclarationSelector{ID: editable[0].ID, EditableOnly: true}
		declaration.ID = editable[0].ID
		declaration.Order = editable[0].Order
		edit, err = authoring.Replace(*target.DependencyAuthoring, selector, declaration)
	} else {
		edit = authoring.Add(*target.DependencyAuthoring, declaration)
	}
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, addDiagnostic("TSPACK_ADD_SEMANTIC_EDIT_FAILED", "dependency authoring edit failed", err.Error()))
		return result
	}
	if authoring.HasFatalConflicts(edit.After) {
		result.Diagnostics = append(result.Diagnostics, addDiagnostic("TSPACK_ADD_SEMANTIC_CONFLICT", "dependency authoring edit produced a fatal conflict"))
		return result
	}
	result.SemanticChanges = append([]authoring.AuthoringChange(nil), edit.Changes...)
	for _, change := range edit.Changes {
		if change.Kind == authoring.ChangeShadowed {
			result.ShadowedDeclarations = append(result.ShadowedDeclarations, change.Declaration)
		}
	}

	frontendPath := request.Project.FrontendCLIPath
	if frontendPath == "" {
		frontendPath = bridge.ResolveWithOptions("cli.js", bridge.ResolveOptions{ProjectRoot: request.Project.RootDir}).Path
	}
	if frontendPath == "" {
		result.Diagnostics = append(result.Diagnostics, addDiagnostic("TSPACK_ADD_FRONTEND_MISSING", "manifest frontend CLI not found"))
		return result
	}
	projection, projectionErr := manifestedit.PlanFile(frontendPath, manifestPath, target.Name, edit)
	if projectionErr != nil {
		result.Diagnostics = append(result.Diagnostics, addDiagnostic("TSPACK_ADD_PROJECTION_FAILED", "failed to plan manifest projection", projectionErr.Error()))
		return result
	}
	result.Diagnostics = append(result.Diagnostics, projection.Diagnostics...)
	if hasErrors(result.Diagnostics) || !projection.Changed {
		return result
	}
	result.ManifestChanged = true
	if request.DryRun {
		return result
	}

	written, writeErr := manifestedit.WritePlannedFile(manifestPath, projection)
	if writeErr != nil || !written {
		details := []string{}
		if writeErr != nil {
			details = append(details, writeErr.Error())
		}
		result.Diagnostics = append(result.Diagnostics, addDiagnostic("TSPACK_ADD_MANIFEST_WRITE_FAILED", "failed to write manifest projection", details...))
		result.ManifestChanged = false
		return result
	}

	updateOptions := request.Project
	updateOptions.ResolverClient = client
	preflight := RunUpdate(UpdateRequest{Project: updateOptions, DryRun: true})
	if hasErrors(preflight.Diagnostics) {
		result.Diagnostics = append(result.Diagnostics, preflight.Diagnostics...)
		rollbackAddManifest(&result, manifestPath, projection)
		return result
	}
	updated := RunUpdate(UpdateRequest{Project: updateOptions})
	result.Diagnostics = append(result.Diagnostics, updated.Diagnostics...)
	result.LockDiff = updated.LockDiff
	result.LockChanged = lockDiffChanged(updated.LockDiff)
	if hasErrors(updated.Diagnostics) {
		if !addHasDiagnosticCode(updated.Diagnostics, "TSPACK_UPDATE_WRITE_FAILED") {
			rollbackAddManifest(&result, manifestPath, projection)
		}
		return result
	}
	return result
}

func selectAddTarget(ir *manifest.ManifestIR, requested string) (*manifest.Package, []diag.Diagnostic) {
	if len(ir.Packages) == 0 {
		return nil, []diag.Diagnostic{addDiagnostic("TSPACK_ADD_AUTHORITY_DENIED", "no editable native package is available; package.json remains authoritative", "Use `tspack npm install <package>` for an incremental npm project.")}
	}
	if requested != "" {
		for index := range ir.Packages {
			if ir.Packages[index].Name == requested {
				return &ir.Packages[index], nil
			}
		}
		return nil, []diag.Diagnostic{addDiagnostic("TSPACK_ADD_PACKAGE_TARGET_NOT_FOUND", "selected package target was not found", requested, strings.Join(packageNames(ir.Packages), ", "))}
	}
	if len(ir.Packages) != 1 {
		return nil, []diag.Diagnostic{addDiagnostic("TSPACK_ADD_PACKAGE_TARGET_AMBIGUOUS", "several editable packages are available; select one with --package", packageNames(ir.Packages)...)}
	}
	return &ir.Packages[0], nil
}

func addManifestPath(options Options, pkg *manifest.Package) (string, error) {
	if filepath.Base(options.ManifestPath) == "package.manifest.tsx" {
		return filepath.Abs(options.ManifestPath)
	}
	relative := pkg.ManifestPath
	if relative == "" && pkg.DependencyAuthoring != nil {
		paths := map[string]bool{}
		for _, declaration := range pkg.DependencyAuthoring.Declarations {
			if declaration.Origin.SourcePath != "" {
				paths[declaration.Origin.SourcePath] = true
			}
		}
		if len(paths) == 1 {
			for path := range paths {
				relative = path
			}
		}
	}
	if relative == "" {
		return options.ManifestPath, nil
	}
	if filepath.IsAbs(relative) {
		return filepath.Clean(relative), nil
	}
	absolute := filepath.Join(options.RootDir, filepath.FromSlash(relative))
	root, rootErr := filepath.Abs(options.RootDir)
	target, targetErr := filepath.Abs(absolute)
	if rootErr != nil || targetErr != nil {
		return "", fmt.Errorf("resolve manifest path")
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("manifest path escapes the project root: %s", relative)
	}
	return target, nil
}

func selectAddConstraint(metadata *resolver.PackageMetadata, explicit string) (string, string, error) {
	if metadata == nil {
		return "", "", fmt.Errorf("registry returned empty metadata")
	}
	if explicit != "" {
		selected, err := highestMatchingVersion(metadata, explicit, true)
		if err != nil {
			return "", "", err
		}
		return explicit, selected, nil
	}
	selected, err := highestMatchingVersion(metadata, "*", false)
	if err != nil {
		return "", "", fmt.Errorf("no stable release is available; specify an explicit prerelease constraint: %w", err)
	}
	version, err := semver.NewVersion(selected)
	if err != nil {
		return "", "", err
	}
	return "^" + version.String(), selected, nil
}

func highestMatchingVersion(metadata *resolver.PackageMetadata, constraintText string, allowPrerelease bool) (string, error) {
	constraint, err := semver.NewConstraint(constraintText)
	if err != nil {
		return "", err
	}
	versions := make([]*semver.Version, 0, len(metadata.Versions))
	originals := map[string]string{}
	for original := range metadata.Versions {
		version, parseErr := semver.NewVersion(original)
		if parseErr != nil || (!allowPrerelease && version.Prerelease() != "") {
			continue
		}
		versions = append(versions, version)
		originals[version.String()] = original
	}
	sort.SliceStable(versions, func(left int, right int) bool {
		return versions[left].GreaterThan(versions[right])
	})
	for _, version := range versions {
		if constraint.Check(version) {
			return originals[version.String()], nil
		}
	}
	return "", fmt.Errorf("no published version satisfies %q", constraintText)
}

func editableDeclarations(ir authoring.PackageIR, identity authoring.PackageIdentity) []authoring.DependencyDeclaration {
	declarations := []authoring.DependencyDeclaration{}
	for _, declaration := range ir.Declarations {
		declarationIdentity := declaration.Identity
		if !declarationIdentity.Valid() {
			declarationIdentity = declaration.Source.Identity()
		}
		if declarationIdentity == identity && declaration.Authority == authoring.AuthorityOwned && declaration.Editability == authoring.EditabilityEditable {
			declarations = append(declarations, declaration)
		}
	}
	return declarations
}

func packageNames(packages []manifest.Package) []string {
	names := make([]string, 0, len(packages))
	for _, pkg := range packages {
		names = append(names, pkg.Name)
	}
	sort.Strings(names)
	return names
}

func declarationDetails(declarations []authoring.DependencyDeclaration) []string {
	details := make([]string, 0, len(declarations))
	for _, declaration := range declarations {
		details = append(details, declaration.ID+" "+string(declaration.Kind)+" "+declaration.Constraint)
	}
	return details
}

func rollbackAddManifest(result *AddDependencyResult, path string, projection manifestedit.ProjectionResult) {
	if err := manifestedit.RestorePlannedFile(path, projection); err != nil {
		result.Diagnostics = append(result.Diagnostics, addDiagnostic("TSPACK_ADD_ROLLBACK_FAILED", "failed to restore the manifest after update failure", err.Error()))
		return
	}
	result.ManifestChanged = false
}

func lockDiffChanged(diff *lockfile.Diff) bool {
	if diff == nil {
		return false
	}
	return len(diff.PackagesAdded) > 0 ||
		len(diff.PackagesRemoved) > 0 ||
		len(diff.PackagesChanged) > 0 ||
		len(diff.EdgesAdded) > 0 ||
		len(diff.EdgesRemoved) > 0 ||
		len(diff.TargetsAdded) > 0 ||
		len(diff.TargetsRemoved) > 0 ||
		len(diff.TargetsChanged) > 0
}

func addHasDiagnosticCode(diagnostics []diag.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func addDiagnostic(code string, message string, details ...string) diag.Diagnostic {
	return diag.Diagnostic{Code: code, Severity: diag.SeverityError, Message: message, Details: details}
}

type dependencyEditMemoClient struct {
	inner            resolver.NPMRegistryClient
	mu               sync.Mutex
	metadata         map[string]addMetadataMemo
	tarballs         map[string]addTarballMemo
	metadataRequests int
	tarballRequests  int
}

type addMetadataMemo struct {
	metadata *resolver.PackageMetadata
	err      error
}

type addTarballMemo struct {
	body []byte
	err  error
}

func newDependencyEditMemoClient(inner resolver.NPMRegistryClient) *dependencyEditMemoClient {
	return &dependencyEditMemoClient{inner: inner, metadata: map[string]addMetadataMemo{}, tarballs: map[string]addTarballMemo{}}
}

func (client *dependencyEditMemoClient) PackageMetadata(ctx context.Context, name string) (*resolver.PackageMetadata, error) {
	client.mu.Lock()
	if memo, ok := client.metadata[name]; ok {
		client.mu.Unlock()
		return memo.metadata, memo.err
	}
	client.mu.Unlock()
	client.mu.Lock()
	client.metadataRequests++
	client.mu.Unlock()
	metadata, err := client.inner.PackageMetadata(ctx, name)
	client.mu.Lock()
	client.metadata[name] = addMetadataMemo{metadata: metadata, err: err}
	client.mu.Unlock()
	return metadata, err
}

func (client *dependencyEditMemoClient) Tarball(ctx context.Context, url string) ([]byte, error) {
	client.mu.Lock()
	if memo, ok := client.tarballs[url]; ok {
		client.mu.Unlock()
		return append([]byte(nil), memo.body...), memo.err
	}
	client.mu.Unlock()
	client.mu.Lock()
	client.tarballRequests++
	client.mu.Unlock()
	body, err := client.inner.Tarball(ctx, url)
	client.mu.Lock()
	client.tarballs[url] = addTarballMemo{body: append([]byte(nil), body...), err: err}
	client.mu.Unlock()
	return body, err
}

func (client *dependencyEditMemoClient) RequestCounts() (int, int) {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.metadataRequests, client.tarballRequests
}
