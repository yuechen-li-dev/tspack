package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/yuechen-li-dev/tspack/internal/authoring"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/materialize"
)

const builtFixtureMarkerName = ".tspack-built-fixture.json"

// BuildCoordinator shares qualified BuildTarget results between concurrent
// TestTargets in one application execution. It does not cache across runs.
type BuildCoordinator struct {
	mu           sync.Mutex
	calls        map[string]*coordinatedBuildCall
	fixtureLocks map[string]*sync.Mutex
}

type coordinatedBuildCall struct {
	done   chan struct{}
	result BuildTargetResult
}

func NewBuildCoordinator() *BuildCoordinator {
	return &BuildCoordinator{
		calls:        map[string]*coordinatedBuildCall{},
		fixtureLocks: map[string]*sync.Mutex{},
	}
}

type coordinatedBuildExecutor struct {
	base        BuildTargetExecutor
	coordinator *BuildCoordinator
}

func (executor coordinatedBuildExecutor) BuildTarget(ctx context.Context, request BuildTargetRequest) BuildTargetResult {
	identity := request.Package.Name + ":" + request.Target.Name
	coordinator := executor.coordinator
	coordinator.mu.Lock()
	if existing, ok := coordinator.calls[identity]; ok {
		coordinator.mu.Unlock()
		select {
		case <-existing.done:
			return existing.result
		case <-ctx.Done():
			return BuildTargetResult{
				Package: request.Package.Name,
				Target:  request.Target.Name,
				Diagnostics: []diag.Diagnostic{{
					Code:     "TSPACK_BUILD_CANCELLED",
					Severity: diag.SeverityError,
					Message:  "build prerequisite wait was cancelled",
					Details:  []string{identity, ctx.Err().Error()},
				}},
			}
		}
	}
	call := &coordinatedBuildCall{done: make(chan struct{})}
	coordinator.calls[identity] = call
	coordinator.mu.Unlock()

	call.result = executor.base.BuildTarget(ctx, request)
	close(call.done)
	return call.result
}

func (coordinator *BuildCoordinator) fixtureLock(destination string) *sync.Mutex {
	key := strings.ToLower(filepath.Clean(destination))
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	lock := coordinator.fixtureLocks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		coordinator.fixtureLocks[key] = lock
	}
	return lock
}

type BuiltFixtureResult struct {
	Name               string   `json:"name"`
	ConsumerTarget     string   `json:"consumerTarget"`
	ProducerTarget     string   `json:"producerTarget"`
	Artifact           string   `json:"artifact,omitempty"`
	ArtifactIdentity   string   `json:"artifactIdentity,omitempty"`
	Artifacts          []string `json:"artifacts,omitempty"`
	ArtifactIdentities []string `json:"artifactIdentities,omitempty"`
	Binding            string   `json:"binding"`
	RealizedPath       string   `json:"realizedPath"`
	ContentHashes      []string `json:"contentHashes"`
	Reused             bool     `json:"reused,omitempty"`
}

type builtFixtureMarker struct {
	Version             int                `json:"version"`
	ConsumerTarget      string             `json:"consumerTarget"`
	ProducerTarget      string             `json:"producerTarget"`
	ArtifactIdentities  []string           `json:"artifactIdentities"`
	Binding             string             `json:"binding"`
	PackageJSONHash     string             `json:"packageJsonHash"`
	Files               []builtFixtureFile `json:"files"`
	RuntimeDependencies []string           `json:"runtimeDependencies,omitempty"`
}

type builtFixtureFile struct {
	Path        string `json:"path"`
	ContentHash string `json:"contentHash"`
	Executable  bool   `json:"executable,omitempty"`
}

func realizeBuiltTestFixtures(
	workspaceRoot string,
	ir *manifest.ManifestIR,
	consumer *manifest.Package,
	target *manifest.TestTarget,
	buildResults []BuildTargetResult,
	coordinator *BuildCoordinator,
	authoritativeLocks ...*lockfile.Lockfile,
) ([]BuiltFixtureResult, []diag.Diagnostic) {
	var authoritativeLock *lockfile.Lockfile
	if len(authoritativeLocks) > 0 {
		authoritativeLock = authoritativeLocks[0]
	}
	results := []BuiltFixtureResult{}
	diagnostics := []diag.Diagnostic{}
	for _, fixture := range target.BuiltFixtures {
		result, fixtureDiagnostics := realizeBuiltTestFixture(workspaceRoot, ir, consumer, target, fixture, buildResults, coordinator, authoritativeLock)
		diagnostics = append(diagnostics, fixtureDiagnostics...)
		if len(fixtureDiagnostics) == 0 {
			results = append(results, result)
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Name < results[j].Name })
	return results, diagnostics
}

func realizeBuiltTestFixture(
	workspaceRoot string,
	ir *manifest.ManifestIR,
	consumer *manifest.Package,
	testTarget *manifest.TestTarget,
	fixture manifest.BuiltArtifactFixture,
	buildResults []BuildTargetResult,
	coordinator *BuildCoordinator,
	authoritativeLock *lockfile.Lockfile,
) (BuiltFixtureResult, []diag.Diagnostic) {
	consumerIdentity := consumer.Name + ":test:" + testTarget.Name
	producerPackageName, producerTargetName := manifest.ResolveBuildTargetReference(consumer.Name, fixture.Target)
	producerIdentity := producerPackageName + ":" + producerTargetName
	artifactNames := fixture.ArtifactNames()
	artifactIdentities := make([]string, 0, len(artifactNames))
	for _, artifactName := range artifactNames {
		artifactIdentities = append(artifactIdentities, producerIdentity+":"+artifactName)
	}
	result := BuiltFixtureResult{
		Name:               fixture.Name,
		ConsumerTarget:     consumerIdentity,
		ProducerTarget:     producerIdentity,
		Artifacts:          append([]string(nil), artifactNames...),
		ArtifactIdentities: append([]string(nil), artifactIdentities...),
		Binding:            fixture.Binding,
	}
	if len(artifactNames) == 1 {
		result.Artifact = artifactNames[0]
		result.ArtifactIdentity = artifactIdentities[0]
		result.Artifacts = nil
		result.ArtifactIdentities = nil
	}

	producerPackage, producerTarget := findBuildTarget(ir, producerPackageName, producerTargetName)
	if producerPackage == nil || producerTarget == nil {
		return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_TARGET_MISSING", "built fixture producer target is unavailable", result)}
	}
	artifacts := []BuildArtifact{}
	for index, artifactName := range artifactNames {
		if _, ok := findDeclaredArtifact(*producerTarget, artifactName); !ok {
			return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_ARTIFACT_UNKNOWN", "built fixture artifact is not declared by the producer target", result)}
		}
		selected := selectQualifiedArtifacts(buildResults, producerPackageName, producerTargetName, artifactIdentities[index])
		if len(selected) == 0 {
			return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_ARTIFACT_MISSING", "producer succeeded without the requested declared artifact", result)}
		}
		artifacts = append(artifacts, selected...)
	}

	producerRoot := filepath.Join(workspaceRoot, filepath.FromSlash(producerPackage.Root))
	consumerRoot := filepath.Join(workspaceRoot, filepath.FromSlash(consumer.Root))
	destination := filepath.Join(consumerRoot, "node_modules", filepath.FromSlash(fixture.Binding))
	if !pathContainedBy(workspaceRoot, producerRoot) || !pathContainedBy(workspaceRoot, destination) {
		result.RealizedPath = destination
		return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_PATH_ESCAPES_WORKSPACE", "built fixture source or destination escapes the workspace", result)}
	}
	result.RealizedPath = destination

	files := []builtFixtureFile{}
	sources := map[string]string{}
	for _, artifact := range artifacts {
		relative, err := filepath.Rel(producerRoot, artifact.Path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_ARTIFACT_ESCAPE", "qualified artifact is outside the producer package", result)}
		}
		info, err := os.Lstat(artifact.Path)
		if err != nil || !info.Mode().IsRegular() {
			return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_ARTIFACT_MISSING", "qualified artifact is missing or is not a regular file", result)}
		}
		if !existingPathContainedBy(producerRoot, artifact.Path) {
			return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_ARTIFACT_ESCAPE", "qualified artifact resolves outside the producer package", result)}
		}
		hash, err := hashFile(artifact.Path)
		if err != nil || artifact.ContentHash == "" || hash != artifact.ContentHash {
			return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_VERIFICATION_FAILED", "qualified artifact content does not match its BuildResult", result)}
		}
		relative = filepath.ToSlash(relative)
		if _, exists := sources[relative]; exists {
			return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_ARTIFACT_COLLISION", "composed artifact members select the same fixture path", result)}
		}
		files = append(files, builtFixtureFile{
			Path:        relative,
			ContentHash: hash,
			Executable:  info.Mode().Perm()&0o111 != 0,
		})
		sources[relative] = artifact.Path
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	packageJSONPath := filepath.Join(producerRoot, "package.json")
	packageJSONHash, err := hashFile(packageJSONPath)
	if err != nil {
		return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_PACKAGE_INVALID", "producer package metadata is unavailable for the fixture binding", result)}
	}
	marker := builtFixtureMarker{
		Version:             2,
		ConsumerTarget:      consumerIdentity,
		ProducerTarget:      producerIdentity,
		ArtifactIdentities:  artifactIdentities,
		Binding:             fixture.Binding,
		PackageJSONHash:     packageJSONHash,
		Files:               files,
		RuntimeDependencies: builtFixtureRuntimeDependencies(producerPackage, producerTarget, producerRoot, consumerRoot),
	}
	for _, file := range files {
		result.ContentHashes = append(result.ContentHashes, file.ContentHash)
	}
	lock := coordinator.fixtureLock(destination)
	lock.Lock()
	defer lock.Unlock()
	if builtFixtureMatches(destination, marker) {
		result.Reused = true
		return result, nil
	}
	if _, err := os.Lstat(destination); err == nil {
		ownedProjection := ownedModuleInstanceProjection(
			workspaceRoot,
			consumerIdentity,
			fixture.Binding,
			destination,
			authoritativeLock,
		)
		if !ownedBuiltFixture(destination) && !ownedProjection {
			return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_DESTINATION_UNMANAGED", "fixture destination already exists and is not TSPack-owned", result)}
		}
		if err := os.RemoveAll(destination); err != nil {
			return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_MATERIALIZATION_FAILED", "failed to replace the previous built fixture: "+err.Error(), result)}
		}
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_MATERIALIZATION_FAILED", err.Error(), result)}
	}
	temporary, err := os.MkdirTemp(filepath.Dir(destination), ".tspack-built-fixture-")
	if err != nil {
		return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_MATERIALIZATION_FAILED", err.Error(), result)}
	}
	if !pathContainedBy(workspaceRoot, temporary) {
		_ = os.RemoveAll(temporary)
		return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_PATH_ESCAPES_WORKSPACE", "built fixture staging path escapes the workspace", result)}
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(temporary)
		}
	}()
	if err := copyRegularFile(packageJSONPath, filepath.Join(temporary, "package.json")); err != nil {
		return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_MATERIALIZATION_FAILED", err.Error(), result)}
	}
	for _, file := range files {
		if err := copyRegularFile(sources[file.Path], filepath.Join(temporary, filepath.FromSlash(file.Path))); err != nil {
			return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_MATERIALIZATION_FAILED", err.Error(), result)}
		}
	}
	for _, reference := range marker.RuntimeDependencies {
		source, sourceErr := safeBuiltFixtureDependencyPath(producerRoot, reference)
		if sourceErr == nil {
			if _, statErr := os.Stat(source); statErr != nil {
				source, sourceErr = safeBuiltFixtureDependencyPath(consumerRoot, reference)
			}
		}
		destinationPath, destinationErr := safeBuiltFixtureDependencyPath(temporary, reference)
		if sourceErr != nil || destinationErr != nil || !existingPathContainedBy(workspaceRoot, source) {
			return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_DEPENDENCY_INVALID", "built fixture runtime dependency is unavailable or escapes the workspace: "+reference, result)}
		}
		resolvedSource, resolveErr := filepath.EvalSymlinks(source)
		if resolveErr != nil || !existingPathContainedBy(workspaceRoot, resolvedSource) {
			return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_DEPENDENCY_INVALID", "built fixture runtime dependency cannot be resolved to an instance inside the workspace: "+reference, result)}
		}
		if err := materialize.LinkPackageDirectory(resolvedSource, destinationPath); err != nil {
			return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_MATERIALIZATION_FAILED", err.Error(), result)}
		}
	}
	markerBytes, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_MATERIALIZATION_FAILED", err.Error(), result)}
	}
	markerBytes = append(markerBytes, '\n')
	if err := os.WriteFile(filepath.Join(temporary, builtFixtureMarkerName), markerBytes, 0o644); err != nil {
		return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_MATERIALIZATION_FAILED", err.Error(), result)}
	}
	if err := os.Rename(temporary, destination); err != nil {
		return result, []diag.Diagnostic{builtFixtureDiagnostic("TSPACK_TEST_BUILT_FIXTURE_MATERIALIZATION_FAILED", err.Error(), result)}
	}
	cleanup = false
	return result, nil
}

func findBuildTarget(ir *manifest.ManifestIR, packageName string, targetName string) (*manifest.Package, *manifest.Target) {
	for packageIndex := range ir.Packages {
		pkg := &ir.Packages[packageIndex]
		if pkg.Name != packageName {
			continue
		}
		for targetIndex := range pkg.Targets {
			if pkg.Targets[targetIndex].Name == targetName {
				return pkg, &pkg.Targets[targetIndex]
			}
		}
	}
	return nil, nil
}

func findDeclaredArtifact(target manifest.Target, name string) (manifest.TargetArtifact, bool) {
	for _, artifact := range manifest.DeclaredTargetArtifacts(target) {
		if artifact.Name == name {
			return artifact, true
		}
	}
	return manifest.TargetArtifact{}, false
}

func selectQualifiedArtifacts(results []BuildTargetResult, packageName string, targetName string, identity string) []BuildArtifact {
	artifacts := []BuildArtifact{}
	seen := map[string]struct{}{}
	for _, result := range results {
		if !result.Succeeded || result.Package != packageName || result.Target != targetName {
			continue
		}
		for _, artifact := range result.Artifacts {
			if artifact.Identity == identity || strings.HasPrefix(artifact.Identity, identity+":") {
				key := artifact.Identity + "\x00" + artifact.Path + "\x00" + artifact.ContentHash
				if _, duplicate := seen[key]; duplicate {
					continue
				}
				seen[key] = struct{}{}
				artifacts = append(artifacts, artifact)
			}
		}
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Identity < artifacts[j].Identity })
	return artifacts
}

func builtFixtureMatches(destination string, expected builtFixtureMarker) bool {
	markerBytes, err := os.ReadFile(filepath.Join(destination, builtFixtureMarkerName))
	if err != nil {
		return false
	}
	actual := builtFixtureMarker{}
	if json.Unmarshal(markerBytes, &actual) != nil {
		return false
	}
	if actual.Version != expected.Version ||
		actual.ProducerTarget != expected.ProducerTarget ||
		actual.Binding != expected.Binding ||
		actual.PackageJSONHash != expected.PackageJSONHash {
		return false
	}
	expectedIdentities, _ := json.Marshal(expected.ArtifactIdentities)
	actualIdentities, _ := json.Marshal(actual.ArtifactIdentities)
	if string(expectedIdentities) != string(actualIdentities) {
		return false
	}
	expectedFiles, _ := json.Marshal(expected.Files)
	actualFiles, _ := json.Marshal(actual.Files)
	if string(expectedFiles) != string(actualFiles) {
		return false
	}
	expectedDependencies, _ := json.Marshal(expected.RuntimeDependencies)
	actualDependencies, _ := json.Marshal(actual.RuntimeDependencies)
	if string(expectedDependencies) != string(actualDependencies) {
		return false
	}
	for _, reference := range expected.RuntimeDependencies {
		dependencyPath, err := safeBuiltFixtureDependencyPath(destination, reference)
		if err != nil {
			return false
		}
		if _, err := os.Stat(dependencyPath); err != nil {
			return false
		}
	}
	packageHash, err := hashFile(filepath.Join(destination, "package.json"))
	if err != nil || packageHash != expected.PackageJSONHash {
		return false
	}
	for _, file := range expected.Files {
		path := filepath.Join(destination, filepath.FromSlash(file.Path))
		hash, err := hashFile(path)
		if err != nil || hash != file.ContentHash {
			return false
		}
		info, err := os.Stat(path)
		if err != nil || (info.Mode().Perm()&0o111 != 0) != file.Executable {
			return false
		}
	}
	return true
}

func builtFixtureRuntimeDependencies(pkg *manifest.Package, target *manifest.Target, producerRoot string, consumerRoot string) []string {
	selected := map[string]struct{}{}
	for _, reference := range target.Deps {
		selected[reference] = struct{}{}
	}
	for _, reference := range target.Peers {
		selected[reference] = struct{}{}
	}
	var dependencies []string
	for _, dependency := range pkg.Dependencies {
		reference := authoring.EffectiveIdentity(dependency)
		if _, ok := selected[reference]; !ok {
			continue
		}
		if dependency.Kind != "dep" && dependency.Kind != "runtime" && dependency.Kind != "peer" {
			continue
		}
		if dependency.Source.Kind == "workspace" && dependency.Kind != "peer" {
			continue
		}
		producerPath, producerErr := safeBuiltFixtureDependencyPath(producerRoot, reference)
		consumerPath, consumerErr := safeBuiltFixtureDependencyPath(consumerRoot, reference)
		_, producerStatErr := os.Stat(producerPath)
		_, consumerStatErr := os.Stat(consumerPath)
		if dependency.Optional && (producerErr != nil || producerStatErr != nil) && (consumerErr != nil || consumerStatErr != nil) {
			continue
		}
		dependencies = append(dependencies, reference)
	}
	sort.Strings(dependencies)
	return dependencies
}

func safeBuiltFixtureDependencyPath(root string, reference string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(reference))
	if cleaned == "." || cleaned == ".." || filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", os.ErrInvalid
	}
	path := filepath.Join(root, "node_modules", cleaned)
	if !pathContainedBy(filepath.Join(root, "node_modules"), path) {
		return "", os.ErrInvalid
	}
	return path, nil
}

func ownedBuiltFixture(destination string) bool {
	markerBytes, err := os.ReadFile(filepath.Join(destination, builtFixtureMarkerName))
	if err != nil {
		return false
	}
	marker := builtFixtureMarker{}
	return json.Unmarshal(markerBytes, &marker) == nil && (marker.Version == 1 || marker.Version == 2)
}

func ownedModuleInstanceProjection(
	workspaceRoot string,
	consumerIdentity string,
	binding string,
	destination string,
	authoritativeLock *lockfile.Lockfile,
) bool {
	if authoritativeLock == nil {
		return false
	}
	instances := map[string]lockfile.ModuleInstance{}
	packages := map[string]lockfile.Package{}
	for _, instance := range authoritativeLock.Instances {
		instances[instance.ID] = instance
	}
	for _, pkg := range authoritativeLock.Packages {
		packages[pkg.ID] = pkg
	}
	for _, root := range authoritativeLock.RootInstances {
		if root.From != consumerIdentity || root.Reference != binding {
			continue
		}
		instance, ok := instances[root.InstanceID]
		if !ok {
			return false
		}
		pkg, ok := packages[instance.PackageID]
		if !ok {
			return false
		}
		digest := sha256.Sum256([]byte(instance.ID))
		expected := filepath.Join(
			workspaceRoot,
			"node_modules",
			".tspack-instances",
			hex.EncodeToString(digest[:]),
			"node_modules",
			filepath.FromSlash(pkg.Name),
		)
		destinationInfo, destinationErr := os.Stat(destination)
		expectedInfo, expectedErr := os.Stat(expected)
		if destinationErr != nil || expectedErr != nil {
			return false
		}
		return os.SameFile(destinationInfo, expectedInfo)
	}
	return false
}

func pathContainedBy(root string, path string) bool {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(absoluteRoot, absolutePath)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func existingPathContainedBy(root string, path string) bool {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	return pathContainedBy(resolvedRoot, resolvedPath)
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyRegularFile(source string, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	inputInfo, err := input.Stat()
	if err != nil {
		return err
	}
	mode := copiedRegularFileMode(inputInfo.Mode())
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chmod(destination, mode)
}

func copiedRegularFileMode(sourceMode os.FileMode) os.FileMode {
	if sourceMode.Perm()&0o111 != 0 {
		return 0o755
	}
	return 0o644
}

func builtFixtureDiagnostic(code string, message string, fixture BuiltFixtureResult) diag.Diagnostic {
	requestedArtifacts := fixture.ArtifactIdentities
	if len(requestedArtifacts) == 0 && fixture.ArtifactIdentity != "" {
		requestedArtifacts = []string{fixture.ArtifactIdentity}
	}
	details := []string{
		"consumer target: " + fixture.ConsumerTarget,
		"producer target: " + fixture.ProducerTarget,
		"requested artifacts: " + strings.Join(requestedArtifacts, ", "),
		"fixture binding: " + fixture.Name + " -> " + fixture.Binding,
	}
	if fixture.RealizedPath != "" {
		details = append(details, "expected realization: "+fixture.RealizedPath)
	}
	return diag.Diagnostic{Code: code, Severity: diag.SeverityError, Message: message, Details: details, File: fixture.RealizedPath}
}

func buildPrerequisiteFailure(consumerIdentity string, diagnostics []diag.Diagnostic) []diag.Diagnostic {
	result := make([]diag.Diagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		copy := diagnostic
		copy.Details = append([]string{"consumer target: " + consumerIdentity}, copy.Details...)
		result = append(result, copy)
	}
	if len(result) == 0 {
		result = append(result, diag.Diagnostic{
			Code:     "TSPACK_TEST_BUILD_PREREQUISITE_FAILED",
			Severity: diag.SeverityError,
			Message:  "required BuildTarget did not produce a qualified successful result",
			Details:  []string{"consumer target: " + consumerIdentity},
		})
	}
	return result
}
