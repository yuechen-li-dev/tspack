package materialize

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/packageidentity"
)

const testFixtureMarkerVersion = 1

type testFixtureProjection struct {
	TargetIdentity string `json:"targetIdentity"`
	Fixture        string `json:"fixture"`
	ProducerID     string `json:"producerId"`
	InstanceID     string `json:"instanceId,omitempty"`
	Binding        string `json:"binding"`
	Mode           string `json:"mode"`
	Source         string `json:"source"`
	Destination    string `json:"destination"`
}

type testFixtureMarker struct {
	Version     int                     `json:"version"`
	PlanDigest  string                  `json:"planDigest"`
	Projections []testFixtureProjection `json:"projections"`
}

func materializeTestFixtures(req Request, nodeModulesRoot string, mode LinkMode) Result {
	result := Result{}
	if req.Graph == nil {
		return result
	}
	projections, diagnostics := buildTestFixtureProjections(req, nodeModulesRoot)
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	if len(result.Diagnostics) > 0 || len(projections) == 0 {
		return result
	}
	markerPath := filepath.Join(req.WorkspaceRoot, ".tspack", "test-fixtures", "materialization.json")
	digest := testFixturePlanDigest(projections)
	previous, previousOK := readTestFixtureMarker(markerPath)
	if previousOK && previous.PlanDigest == digest && testFixtureProjectionsExist(req.WorkspaceRoot, projections) {
		return result
	}
	if previousOK {
		for _, projection := range previous.Projections {
			if projection.Source == projection.Destination {
				continue
			}
			destination, ok := containedWorkspacePath(req.WorkspaceRoot, projection.Destination)
			if !ok {
				result.Diagnostics = append(result.Diagnostics, testFixtureDiagnostic(
					"TSPACK_TEST_FIXTURE_PATH_ESCAPES_WORKSPACE",
					"recorded fixture destination escapes the workspace",
					projection,
				))
				continue
			}
			if err := removeMaterializedPath(destination); err != nil {
				result.Diagnostics = append(result.Diagnostics, testFixtureDiagnostic(
					"TSPACK_TEST_FIXTURE_MATERIALIZATION_FAILED",
					"failed to remove a stale fixture projection: "+err.Error(),
					projection,
				))
			}
		}
	}
	if len(result.Diagnostics) > 0 {
		return result
	}
	for _, projection := range projections {
		source, sourceOK := containedWorkspacePath(req.WorkspaceRoot, projection.Source)
		destination, destinationOK := containedWorkspacePath(req.WorkspaceRoot, projection.Destination)
		if !sourceOK || !destinationOK {
			result.Diagnostics = append(result.Diagnostics, testFixtureDiagnostic(
				"TSPACK_TEST_FIXTURE_PATH_ESCAPES_WORKSPACE",
				"fixture source or destination escapes the workspace",
				projection,
			))
			continue
		}
		if filepath.Clean(source) == filepath.Clean(destination) {
			continue
		}
		if _, err := os.Stat(source); err != nil {
			result.Diagnostics = append(result.Diagnostics, testFixtureDiagnostic(
				"TSPACK_TEST_FIXTURE_CONTENT_INVALID",
				"fixture producer was not materialized: "+err.Error(),
				projection,
			))
			continue
		}
		if projection.Mode == "source" && !existingPathContainedByWorkspace(req.WorkspaceRoot, source) {
			result.Diagnostics = append(result.Diagnostics, testFixtureDiagnostic(
				"TSPACK_TEST_FIXTURE_PATH_ESCAPES_WORKSPACE",
				"source fixture resolves outside the workspace",
				projection,
			))
			continue
		}
		if _, err := os.Lstat(destination); err == nil {
			if !req.Options.Clean && !req.Options.Force {
				result.Diagnostics = append(result.Diagnostics, testFixtureDiagnostic(
					"TSPACK_TEST_FIXTURE_DESTINATION_UNMANAGED",
					"fixture destination already exists and is not owned by TSPack; use sync --clean or remove it explicitly",
					projection,
				))
				continue
			}
			if err := removeMaterializedPath(destination); err != nil {
				result.Diagnostics = append(result.Diagnostics, testFixtureDiagnostic(
					"TSPACK_TEST_FIXTURE_MATERIALIZATION_FAILED",
					"failed to replace the fixture binding during clean materialization: "+err.Error(),
					projection,
				))
				continue
			}
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			result.Diagnostics = append(result.Diagnostics, testFixtureDiagnostic(
				"TSPACK_TEST_FIXTURE_MATERIALIZATION_FAILED",
				"failed to create the fixture binding directory: "+err.Error(),
				projection,
			))
			continue
		}
		if !existingPathContainedByWorkspace(req.WorkspaceRoot, filepath.Dir(destination)) {
			result.Diagnostics = append(result.Diagnostics, testFixtureDiagnostic(
				"TSPACK_TEST_FIXTURE_PATH_ESCAPES_WORKSPACE",
				"fixture destination resolves outside the workspace",
				projection,
			))
			continue
		}
		var materializeErr error
		if projection.Mode == "source" {
			materializeErr = createWorkspaceDirectoryLink(source, destination)
		} else {
			materializeErr = copyPackageTree(source, destination, mode, req.Options.Stats)
			if materializeErr == nil {
				materializeErr = linkPackageFixtureDependencies(nodeModulesRoot, destination, projection.InstanceID, req.Lock)
			}
		}
		if materializeErr != nil {
			result.Diagnostics = append(result.Diagnostics, testFixtureDiagnostic(
				"TSPACK_TEST_FIXTURE_MATERIALIZATION_FAILED",
				"failed to materialize the fixture: "+materializeErr.Error(),
				projection,
			))
			continue
		}
		result.Written = append(result.Written, WrittenPath{Path: destination, Kind: "testFixture", PackageID: projection.ProducerID})
	}
	if len(result.Diagnostics) > 0 {
		return result
	}
	marker := testFixtureMarker{Version: testFixtureMarkerVersion, PlanDigest: digest, Projections: projections}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_TEST_FIXTURE_MARKER_FAILED", Severity: diag.SeverityError, Message: err.Error()})
		return result
	}
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
		result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_TEST_FIXTURE_MARKER_FAILED", Severity: diag.SeverityError, Message: err.Error(), File: markerPath})
		return result
	}
	data = append(data, '\n')
	if err := os.WriteFile(markerPath, data, 0o644); err != nil {
		result.Diagnostics = append(result.Diagnostics, diag.Diagnostic{Code: "TSPACK_TEST_FIXTURE_MARKER_FAILED", Severity: diag.SeverityError, Message: err.Error(), File: markerPath})
	}
	return result
}

func buildTestFixtureProjections(req Request, nodeModulesRoot string) ([]testFixtureProjection, []diag.Diagnostic) {
	packagesByID := map[string]lockfile.Package{}
	instancesByID := map[string]lockfile.ModuleInstance{}
	rootInstances := map[string]lockfile.RootModuleInstance{}
	for _, pkg := range req.Lock.Packages {
		packagesByID[pkg.ID] = pkg
	}
	for _, instance := range req.Lock.Instances {
		instancesByID[instance.ID] = instance
	}
	for _, root := range req.Lock.RootInstances {
		rootInstances[root.From+"\x00"+root.Reference] = root
	}
	edgesByTarget := map[string][]lockfile.Edge{}
	for _, edge := range req.Lock.Edges {
		edgesByTarget[edge.From] = append(edgesByTarget[edge.From], edge)
	}
	projections := []testFixtureProjection{}
	diagnostics := []diag.Diagnostic{}
	destinationOwners := map[string]testFixtureProjection{}
	for _, pkg := range req.Graph.AllPackages() {
		packageRoot := filepath.Join(req.WorkspaceRoot, filepath.FromSlash(pkg.Root))
		for _, target := range pkg.AllTestTargets() {
			targetIdentity := pkg.Name + ":test:" + target.Name
			for _, fixture := range target.Fixtures {
				edge, ok := testFixtureEdge(edgesByTarget[targetIdentity], fixture.Dependency.Key)
				if !ok {
					projection := testFixtureProjection{TargetIdentity: targetIdentity, Fixture: fixture.Name, Binding: fixture.Binding}
					diagnostics = append(diagnostics, testFixtureDiagnostic("TSPACK_TEST_FIXTURE_PRODUCER_NOT_LOCKED", "fixture producer has no target-scoped lock edge", projection))
					continue
				}
				producer, ok := packagesByID[edge.To]
				if !ok {
					projection := testFixtureProjection{TargetIdentity: targetIdentity, Fixture: fixture.Name, ProducerID: edge.To, Binding: fixture.Binding}
					diagnostics = append(diagnostics, testFixtureDiagnostic("TSPACK_TEST_FIXTURE_PRODUCER_NOT_FOUND", "fixture lock edge points to an unknown producer", projection))
					continue
				}
				var source string
				projectionInstanceID := ""
				if fixture.Mode == "source" {
					var sourceDiagnostic *diag.Diagnostic
					source, sourceDiagnostic = testFixtureSource(req, pkg, fixture)
					if sourceDiagnostic != nil {
						diagnostics = append(diagnostics, *sourceDiagnostic)
						continue
					}
				} else {
					installName := edgeMaterializationName(edge, producer)
					source = filepath.Join(nodeModulesRoot, filepath.FromSlash(installName))
					if root, exists := rootInstances[targetIdentity+"\x00"+fixture.Dependency.Key]; exists {
						projectionInstanceID = root.InstanceID
						instance, instanceExists := instancesByID[root.InstanceID]
						if !instanceExists {
							continue
						}
						instancePath, pathErr := moduleInstancePackagePath(nodeModulesRoot, instance, producer)
						if pathErr != nil {
							projection := testFixtureProjection{TargetIdentity: targetIdentity, Fixture: fixture.Name, ProducerID: producer.ID, Binding: fixture.Binding}
							diagnostics = append(diagnostics, testFixtureDiagnostic("TSPACK_TEST_FIXTURE_PRODUCER_INVALID", "fixture module-instance path is invalid: "+pathErr.Error(), projection))
							continue
						}
						source = instancePath
					}
				}
				destination := filepath.Join(packageRoot, "node_modules", filepath.FromSlash(fixture.Binding))
				relSource, sourceOK := relativeWorkspacePath(req.WorkspaceRoot, source)
				relDestination, destinationOK := relativeWorkspacePath(req.WorkspaceRoot, destination)
				projection := testFixtureProjection{
					TargetIdentity: targetIdentity,
					Fixture:        fixture.Name,
					ProducerID:     producer.ID,
					InstanceID:     projectionInstanceID,
					Binding:        fixture.Binding,
					Mode:           fixture.Mode,
					Source:         relSource,
					Destination:    relDestination,
				}
				if !sourceOK || !destinationOK {
					diagnostics = append(diagnostics, testFixtureDiagnostic("TSPACK_TEST_FIXTURE_PATH_ESCAPES_WORKSPACE", "fixture source or destination escapes the workspace", projection))
					continue
				}
				destinationKey := strings.ToLower(filepath.Clean(destination))
				if owner, exists := destinationOwners[destinationKey]; exists {
					if owner.ProducerID != projection.ProducerID {
						diagnostics = append(diagnostics, testFixtureDiagnostic("TSPACK_TEST_FIXTURE_BINDING_CONFLICT", "fixture binding maps to different producers", projection))
					}
					continue
				}
				destinationOwners[destinationKey] = projection
				projections = append(projections, projection)
			}
		}
	}
	sort.Slice(projections, func(i, j int) bool {
		if projections[i].TargetIdentity != projections[j].TargetIdentity {
			return projections[i].TargetIdentity < projections[j].TargetIdentity
		}
		return projections[i].Fixture < projections[j].Fixture
	})
	return projections, diagnostics
}

func linkPackageFixtureDependencies(nodeModulesRoot string, destination string, instanceID string, locked *lockfile.Lockfile) error {
	if locked == nil || instanceID == "" {
		return nil
	}
	instances := moduleInstancesByID(locked.Instances)
	packages := map[string]lockfile.Package{}
	for _, pkg := range locked.Packages {
		packages[pkg.ID] = pkg
	}
	instance, ok := instances[instanceID]
	if !ok {
		return fmt.Errorf("fixture module instance is missing from the lock: %s", instanceID)
	}
	links := map[string]string{}
	for _, dependency := range instance.Dependencies {
		links[dependency.Reference] = dependency.InstanceID
	}
	for _, peer := range instance.Peers {
		if peer.State == packageidentity.PeerBindingPresent && peer.InstanceID != "" {
			links[peer.Reference] = peer.InstanceID
		}
	}
	references := make([]string, 0, len(links))
	for reference := range links {
		references = append(references, reference)
	}
	sort.Strings(references)
	for _, reference := range references {
		dependencyInstance, exists := instances[links[reference]]
		if !exists {
			return fmt.Errorf("fixture dependency instance is missing from the lock: %s", links[reference])
		}
		dependencyPackage, exists := packages[dependencyInstance.PackageID]
		if !exists {
			return fmt.Errorf("fixture dependency package is missing from the lock: %s", dependencyInstance.PackageID)
		}
		target, err := moduleInstancePackagePath(nodeModulesRoot, dependencyInstance, dependencyPackage)
		if err != nil {
			return err
		}
		link, err := safePackagePath(filepath.Join(destination, "node_modules"), reference)
		if err != nil {
			return err
		}
		if err := replacePackageDirectoryLink(target, link); err != nil {
			return err
		}
	}
	return nil
}

func testFixtureSource(req Request, consumerPackage *graph.PackageNode, fixture graph.TestFixtureNode) (string, *diag.Diagnostic) {
	source := fixture.Dependency.Source
	var path string
	switch source.Kind {
	case "path":
		consumerRoot := filepath.Join(req.WorkspaceRoot, filepath.FromSlash(consumerPackage.Root))
		path = filepath.Join(consumerRoot, filepath.FromSlash(source.Path))
	case "workspace":
		producer := req.Graph.PackagesByName[source.Name]
		if producer == nil {
			projection := testFixtureProjection{Fixture: fixture.Name, Binding: fixture.Binding, Mode: fixture.Mode}
			diagnostic := testFixtureDiagnostic("TSPACK_TEST_FIXTURE_PRODUCER_NOT_FOUND", "workspace fixture producer is not in the authoritative graph", projection)
			return "", &diagnostic
		}
		path = filepath.Join(req.WorkspaceRoot, filepath.FromSlash(producer.Root))
	default:
		projection := testFixtureProjection{Fixture: fixture.Name, Binding: fixture.Binding, Mode: fixture.Mode}
		diagnostic := testFixtureDiagnostic("TSPACK_TEST_FIXTURE_PRODUCER_INVALID", "source fixtures require a path or workspace producer", projection)
		return "", &diagnostic
	}
	return path, nil
}

func testFixtureEdge(edges []lockfile.Edge, dependency string) (lockfile.Edge, bool) {
	for _, edge := range edges {
		if edge.Reference == dependency {
			return edge, true
		}
	}
	return lockfile.Edge{}, false
}

func relativeWorkspacePath(workspaceRoot string, path string) (string, bool) {
	absoluteRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", false
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	relative, err := filepath.Rel(absoluteRoot, absolutePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(relative), true
}

func containedWorkspacePath(workspaceRoot string, relative string) (string, bool) {
	if filepath.IsAbs(relative) {
		return "", false
	}
	path := filepath.Join(workspaceRoot, filepath.FromSlash(relative))
	_, ok := relativeWorkspacePath(workspaceRoot, path)
	if !ok {
		return "", false
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	return absolutePath, true
}

func existingPathContainedByWorkspace(workspaceRoot string, path string) bool {
	absoluteRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return false
	}
	resolvedRoot, err := resolvedExistingPath(absoluteRoot)
	if err != nil {
		return false
	}
	resolvedPath, err := resolvedExistingPath(path)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedPath)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func testFixturePlanDigest(projections []testFixtureProjection) string {
	data, _ := json.Marshal(projections)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func readTestFixtureMarker(path string) (testFixtureMarker, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return testFixtureMarker{}, false
	}
	marker := testFixtureMarker{}
	if json.Unmarshal(data, &marker) != nil || marker.Version != testFixtureMarkerVersion {
		return testFixtureMarker{}, false
	}
	return marker, true
}

func testFixtureProjectionsExist(workspaceRoot string, projections []testFixtureProjection) bool {
	for _, projection := range projections {
		destination, destinationOK := containedWorkspacePath(workspaceRoot, projection.Destination)
		source, sourceOK := containedWorkspacePath(workspaceRoot, projection.Source)
		if !destinationOK || !sourceOK {
			return false
		}
		destinationInfo, destinationErr := os.Stat(destination)
		sourceInfo, sourceErr := os.Stat(source)
		if destinationErr != nil || sourceErr != nil {
			return false
		}
		samePhysicalDirectory := os.SameFile(destinationInfo, sourceInfo)
		if projection.Mode == "source" && !samePhysicalDirectory {
			return false
		}
		if projection.Mode == "package" && samePhysicalDirectory {
			return false
		}
	}
	return true
}

func testFixtureDiagnostic(code string, message string, projection testFixtureProjection) diag.Diagnostic {
	details := []string{
		"consumer target: " + projection.TargetIdentity,
		"fixture binding: " + projection.Fixture + " -> " + projection.Binding,
	}
	if projection.ProducerID != "" {
		details = append(details, "producer: "+projection.ProducerID)
	}
	return diag.Diagnostic{Code: code, Severity: diag.SeverityError, Message: message, Details: details, File: projection.Destination}
}
