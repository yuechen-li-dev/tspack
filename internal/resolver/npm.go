package resolver

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"sync"

	semver "github.com/Masterminds/semver/v3"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/packageidentity"
	"github.com/yuechen-li-dev/tspack/internal/requirements"
)

type ResolveMode string

const (
	ResolveModeUpdate ResolveMode = "update"
	ResolveModeSync   ResolveMode = "sync"
)

const defaultResolveJobs = 24

type NPMRegistryClient interface {
	PackageMetadata(ctx context.Context, name string) (*PackageMetadata, error)
	Tarball(ctx context.Context, url string) ([]byte, error)
}

type ResolverOptions struct {
	RegistryURL           string
	Client                NPMRegistryClient
	Backends              BackendRegistry
	Mode                  ResolveMode
	RootDir               string
	ResolveJobs           int
	ResolveControllerMode ResolveControllerMode
	ResolveHostBudget     int
	OnArtifactResolved    func(pkg lockfile.Package, artifact []byte) error
	OnMetadataCacheHit    func(source string, name string)
	OnResolveJobs         func(jobs int)
	OnResolveFrontier     func(width int)
	OnResolveController   func(decision FrontierDecision)
	OnPreparedPackage     func(key string)
	OnCommittedPackage    func(id string)
	OnResolverWorkerError func(key string)
}

type ResolveRequest struct {
	Graph        *graph.WorkspaceGraph
	ExistingLock *lockfile.Lockfile
}

type ResolveResult struct {
	Lock        *lockfile.Lockfile
	Diagnostics []diag.Diagnostic
}

type PackageMetadata struct {
	Name     string                    `json:"name"`
	Versions map[string]PackageVersion `json:"versions"`
	DistTags map[string]string         `json:"dist-tags"`
}

type PackageVersion struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Dependencies         map[string]string `json:"dependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	PeerDependenciesMeta map[string]struct {
		Optional bool `json:"optional"`
	} `json:"peerDependenciesMeta"`
	Dist    PackageDist       `json:"dist"`
	Scripts map[string]string `json:"scripts"`
}

type PackageDist struct {
	Tarball   string `json:"tarball"`
	Integrity string `json:"integrity"`
}

type resolverState struct {
	opts                      ResolverOptions
	result                    *ResolveResult
	seenPkg                   map[string]bool
	graph                     *graph.WorkspaceGraph
	controller                ResolveOccupancyController
	hostMap                   map[string]int
	backends                  BackendRegistry
	frontierSeq               int
	metaMu                    sync.Mutex
	meta                      map[string]*metadataMemoEntry
	prepMu                    sync.Mutex
	prep                      map[string]*preparedMemoEntry
	requirementFacts          []requirements.PackageRequirement
	requirementOrder          int
	selectedSlots             map[string]string
	peerRequirementsByPackage map[string][]requirements.PackageRequirement
}

type metadataMemoEntry struct {
	ready chan struct{}
	meta  *RegistryPackageMetadata
	err   error
}

type preparedMemoEntry struct {
	ready    chan struct{}
	prepared preparedPackage
}

type resolveWorkItem struct {
	source    string
	name      string
	rng       string
	from      string
	kind      string
	optional  bool
	reference string
	slot      string
}

type resolveWorkGroup struct {
	key         string
	source      string
	name        string
	rng         string
	requests    []resolveWorkItem
	hasRequired bool
}

type preparedPackage struct {
	groupKey   string
	packageID  string
	packageDef lockfile.Package
	transitive []resolveWorkItem
	peers      []DependencyRequirement
	failure    *preparedFailure
	cancelled  bool
}

type preparedFailure struct {
	code    string
	message string
	details []string
}

func ResolveNPM(ctx context.Context, opts ResolverOptions, req ResolveRequest) ResolveResult {
	result := ResolveResult{Lock: &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: lockfile.FormatVersion, Tool: lockfile.ToolName}}}
	if opts.Mode == ResolveModeSync {
		result.Diagnostics = append(result.Diagnostics, dErr("TSPACK_RESOLVE_MODE_UNSUPPORTED", "sync mode not yet supported in M7"))
		return result
	}
	if req.Graph == nil {
		result.Diagnostics = append(result.Diagnostics, dErr("TSPACK_RESOLVE_INTERNAL_ERROR", "resolver requires graph"))
		return result
	}
	backends := opts.Backends
	if backends == nil {
		backends = BackendRegistry{}
	}
	if _, ok := backends.Backend(SourceNPM); !ok {
		backends[SourceNPM] = NewNPMBackend(opts.Client)
	}
	if _, ok := backends.Backend(SourceJSR); !ok {
		backends[SourceJSR] = NewJSRBackend(nil)
	}

	resolveJobs := opts.ResolveJobs
	if resolveJobs <= 0 {
		resolveJobs = defaultResolveJobs
	}
	if opts.OnResolveJobs != nil {
		opts.OnResolveJobs(resolveJobs)
	}

	for _, pkg := range req.Graph.AllPackages() {
		for _, target := range pkg.AllTargets() {
			result.Lock.Targets = append(result.Lock.Targets, lockfile.Target{
				Package: pkg.Name,
				Name:    target.Name,
				Export:  target.Export,
				Entry:   target.Entry,
				Runtime: target.Runtime,
				Types:   target.Types,
			})
		}
	}

	state := &resolverState{
		opts:                      opts,
		result:                    &result,
		seenPkg:                   map[string]bool{},
		graph:                     req.Graph,
		controller:                NewResolveOccupancyController(opts.ResolveControllerMode, resolveJobs, opts.ResolveHostBudget),
		hostMap:                   backendHostMap(backends),
		backends:                  backends,
		selectedSlots:             map[string]string{},
		peerRequirementsByPackage: map[string][]requirements.PackageRequirement{},
	}

	frontier := make([]resolveWorkItem, 0)
	for _, pkg := range req.Graph.AllPackages() {
		selectedRuntimeDependencies := map[string]bool{}
		for _, target := range pkg.AllTargets() {
			from := fmt.Sprintf("%s:target:%s", pkg.Name, target.Name)
			for _, dep := range target.AllowedRuntimeDependencies() {
				selectedRuntimeDependencies[dep.Key] = true
				frontier = state.enqueueSharedDirectRequest(ctx, frontier, dep, from, dependencyEdgeKind(dep), requirements.KindProjectExplicit)
			}
			for _, dep := range target.AllowedPeerDependencies() {
				frontier = state.enqueueSharedDirectRequest(ctx, frontier, dep, from, "peer", requirements.KindPeer)
			}
		}
		for _, dep := range pkg.AllDependencies() {
			if selectedRuntimeDependencies[dep.Key] {
				continue
			}
			if dep.Kind != graph.DependencyKindDep && dep.Kind != graph.DependencyKindRuntime {
				continue
			}
			frontier = state.enqueueSharedDirectRequest(ctx, frontier, dep, fmt.Sprintf("%s:dependency", pkg.Name), dependencyEdgeKind(dep), requirements.KindProjectExplicit)
		}
		for _, dep := range pkg.ToolDependencies() {
			frontier = state.enqueueDirectRequest(ctx, frontier, dep, fmt.Sprintf("%s:tool", pkg.Name), "tool")
		}
	}
	frontier = state.applyRequirementControllers(frontier)

	for len(frontier) > 0 {
		nextFrontier, fatal := state.resolveFrontier(ctx, frontier, resolveJobs)
		if fatal {
			break
		}
		frontier = nextFrontier
	}
	if !hasErrorDiagnostics(result.Diagnostics) {
		state.stabilizePeerRequirements(ctx, resolveJobs)
	}
	state.finalizeRequirementTape()

	result.Lock = mustNormalizeLock(result.Lock)
	diag.SortDiagnostics(result.Diagnostics)
	return result
}

func dependencyEdgeKind(dep *graph.DependencyNode) string {
	if dep.Kind == graph.DependencyKindType {
		return "type"
	}
	return "runtime"
}

func (r *resolverState) enqueueDirectRequest(ctx context.Context, frontier []resolveWorkItem, dep *graph.DependencyNode, from, kind string) []resolveWorkItem {
	if _, ok := r.backends.Backend(dep.Source.Kind); !ok {
		r.resolveNonNPMDependency(ctx, dep, from, kind)
		return frontier
	}
	frontier = append(frontier, resolveWorkItem{
		source:   dep.Source.Kind,
		name:     dep.Source.Package,
		rng:      dep.Source.Range,
		from:     from,
		kind:     kind,
		optional: dep.Optional,
	})
	return frontier
}

func (r *resolverState) enqueueSharedDirectRequest(ctx context.Context, frontier []resolveWorkItem, dep *graph.DependencyNode, from, edgeKind string, requirementKind requirements.Kind) []resolveWorkItem {
	if _, ok := r.backends.Backend(dep.Source.Kind); !ok {
		r.resolveNonNPMDependency(ctx, dep, from, edgeKind)
		return frontier
	}
	requirement := requirements.PackageRequirement{
		ID:         fmt.Sprintf("direct:%s:%s:%06d", from, dep.Key, r.requirementOrder),
		Target:     packageidentity.PackageIdentity{Source: dep.Source.Kind, Name: dep.Source.Package},
		Reference:  dep.Key,
		Constraint: dep.Source.Range,
		Kind:       requirementKind,
		Optional:   dep.Optional,
		Origin: requirements.Origin{
			RequiringPackage: dep.Package.Name,
			DependencyKey:    dep.Key,
			Source:           "manifest",
		},
		Scope: requirements.Scope{Environment: "workspace"},
		Order: r.requirementOrder,
	}
	r.requirementOrder++
	r.requirementFacts = append(r.requirementFacts, requirement)
	return append(frontier, resolveWorkItem{
		source:   dep.Source.Kind,
		name:     dep.Source.Package,
		rng:      dep.Source.Range,
		from:     from,
		kind:     edgeKind,
		optional: dep.Optional,
		slot:     requirements.SlotKey(requirement),
	})
}

func (r *resolverState) resolveFrontier(ctx context.Context, frontier []resolveWorkItem, resolveJobs int) ([]resolveWorkItem, bool) {
	normalized := normalizeWorkItems(frontier)
	if len(normalized) == 0 {
		return nil, false
	}
	if r.opts.OnResolveFrontier != nil {
		r.opts.OnResolveFrontier(len(normalized))
	}

	groups := groupWorkItems(normalized)
	preparedByGroup := r.prepareGroups(ctx, normalized, groups, resolveJobs)

	nextFrontier := make([]resolveWorkItem, 0)
	fatal := false
	for _, request := range normalized {
		groupKey := packageWorkGroupKey(request.source, request.name, request.rng)
		prepared, ok := preparedByGroup[groupKey]
		if !ok || prepared.cancelled {
			continue
		}

		if prepared.failure != nil {
			r.result.Diagnostics = append(r.result.Diagnostics, buildResolveDiagnostic(request.optional, prepared.failure))
			if !request.optional {
				fatal = true
			}
			continue
		}

		if strings.HasPrefix(request.from, "workspace:peer:") {
			r.removeEdgesFrom(request.from)
		}
		r.result.Lock.Edges = append(r.result.Lock.Edges, lockfile.Edge{
			From:      request.from,
			To:        prepared.packageID,
			Kind:      request.kind,
			Optional:  request.optional,
			Reference: request.reference,
		})
		if request.slot != "" {
			r.selectedSlots[request.slot] = prepared.packageDef.Version
			if prepared.packageDef.Version == "" {
				r.selectedSlots[request.slot] = packageVersionFromID(prepared.packageID)
			}
		}

		if r.seenPkg[prepared.packageID] {
			continue
		}

		r.result.Lock.Packages = append(r.result.Lock.Packages, prepared.packageDef)
		r.seenPkg[prepared.packageID] = true
		if r.opts.OnCommittedPackage != nil {
			r.opts.OnCommittedPackage(prepared.packageID)
		}
		nextFrontier = append(nextFrontier, prepared.transitive...)
		r.appendPeerRequirements(prepared.packageID, prepared.peers)
	}

	return nextFrontier, fatal
}

func (r *resolverState) removeEdgesFrom(from string) {
	kept := r.result.Lock.Edges[:0]
	for _, edge := range r.result.Lock.Edges {
		if edge.From != from {
			kept = append(kept, edge)
		}
	}
	r.result.Lock.Edges = kept
}

func normalizeWorkItems(frontier []resolveWorkItem) []resolveWorkItem {
	out := append([]resolveWorkItem(nil), frontier...)
	sort.SliceStable(out, func(i, j int) bool {
		return resolveWorkItemKey(out[i]) < resolveWorkItemKey(out[j])
	})
	return out
}

func groupWorkItems(frontier []resolveWorkItem) []resolveWorkGroup {
	groupsByKey := make(map[string]*resolveWorkGroup, len(frontier))
	order := make([]string, 0, len(frontier))
	for _, request := range frontier {
		groupKey := packageWorkGroupKey(request.source, request.name, request.rng)
		group, ok := groupsByKey[groupKey]
		if !ok {
			group = &resolveWorkGroup{
				key:    groupKey,
				source: request.source,
				name:   request.name,
				rng:    request.rng,
			}
			groupsByKey[groupKey] = group
			order = append(order, groupKey)
		}
		group.requests = append(group.requests, request)
		if !request.optional {
			group.hasRequired = true
		}
	}

	out := make([]resolveWorkGroup, 0, len(order))
	for _, groupKey := range order {
		out = append(out, *groupsByKey[groupKey])
	}
	return out
}

func resolveWorkItemKey(request resolveWorkItem) string {
	return request.source + "|" + request.name + "|" + request.rng + "|" + request.from + "|" + request.kind + "|" + boolKey(request.optional)
}

func packageWorkGroupKey(source, name, rng string) string {
	return source + "|" + name + "|" + rng
}

func boolKey(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func (r *resolverState) prepareGroups(ctx context.Context, normalized []resolveWorkItem, groups []resolveWorkGroup, resolveJobs int) map[string]preparedPackage {
	preparedByGroup := make(map[string]preparedPackage, len(groups))
	if len(groups) == 0 {
		return preparedByGroup
	}

	decision := r.controller.Decide(FrontierInput{
		FrontierIndex: r.frontierSeq,
		FrontierWidth: len(normalized),
		WorkItems:     len(groups),
		MetadataItems: len(groups),
		TarballItems:  0,
		Hosts:         r.hostMap,
	})
	r.frontierSeq++
	if r.opts.OnResolveController != nil {
		r.opts.OnResolveController(decision)
	}

	workerCount := decision.TargetJobs
	if workerCount == 1 {
		for _, group := range groups {
			preparedByGroup[group.key] = r.prepareGroup(ctx, group)
		}
		return preparedByGroup
	}

	jobCh := make(chan resolveWorkGroup)
	resultCh := make(chan preparedPackage, len(groups))
	var wg sync.WaitGroup
	var stopMu sync.Mutex
	stopSubmitting := false

	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for group := range jobCh {
				if ctx.Err() != nil {
					resultCh <- preparedPackage{groupKey: group.key, cancelled: true}
					continue
				}

				prepared := r.prepareGroup(ctx, group)
				if prepared.failure != nil && group.hasRequired {
					stopMu.Lock()
					stopSubmitting = true
					stopMu.Unlock()
				}
				resultCh <- prepared
			}
		}()
	}

	go func() {
		defer close(jobCh)
		for _, group := range groups {
			stopMu.Lock()
			shouldStop := stopSubmitting
			stopMu.Unlock()
			if shouldStop || ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case jobCh <- group:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	for prepared := range resultCh {
		preparedByGroup[prepared.groupKey] = prepared
	}
	return preparedByGroup
}

func (r *resolverState) prepareGroup(ctx context.Context, group resolveWorkGroup) preparedPackage {
	prepared := preparedPackage{groupKey: group.key}

	backend, ok := r.backends.Backend(group.source)
	if !ok {
		prepared.failure = prepareFailure("TSPACK_REGISTRY_SOURCE_UNSUPPORTED", "registry package source is not supported", group.source, group.name, "Use a configured package source such as npm or jsr.")
		return prepared
	}
	meta, err := r.packageMetadata(ctx, backend, group.name)
	if err != nil {
		code := sourceDiagnosticCode(group.source, "METADATA_ERROR")
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			code = sourceDiagnosticCode(group.source, "PACKAGE_NOT_FOUND")
		}
		if errorsFromCancellation(err) {
			prepared.cancelled = true
			return prepared
		}
		if group.source == SourceNPM {
			prepared.failure = prepareFailure(code, "failed to fetch npm metadata", group.name, err.Error())
		} else {
			prepared.failure = prepareFailure(code, "failed to fetch registry package metadata", group.source+":"+group.name, err.Error(), "Check the source-qualified package name and registry availability.")
		}
		r.recordResolverWorkerError(group.key)
		return prepared
	}

	pv, version, selCode, ok := selectRegistryVersion(meta, group.rng)
	if !ok {
		code := sourceDiagnosticCode(group.source, "VERSION_NOT_FOUND")
		if selCode != "" {
			code = sourceDiagnosticCode(group.source, "INVALID_RANGE")
		}
		if group.source == SourceNPM {
			prepared.failure = prepareFailure(code, "failed to select npm version", group.name, group.rng)
		} else {
			prepared.failure = prepareFailure(code, "failed to select registry package version", group.source+":"+group.name, group.rng, "Use a valid SemVer constraint matching a published, non-yanked version.")
		}
		r.recordResolverWorkerError(group.key)
		return prepared
	}

	id := pv.Identity.ID(version)
	if r.seenPkg[id] {
		prepared.packageID = id
		return prepared
	}

	prepared = r.prepareSelectedPackage(ctx, group.key, id, backend, pv)
	prepared.groupKey = group.key
	return prepared
}

func (r *resolverState) prepareSelectedPackage(ctx context.Context, groupKey string, id string, backend RegistryBackend, pv RegistryPackageVersion) preparedPackage {
	r.prepMu.Lock()
	if r.prep == nil {
		r.prep = map[string]*preparedMemoEntry{}
	}
	if entry, ok := r.prep[id]; ok {
		r.prepMu.Unlock()
		select {
		case <-entry.ready:
			prepared := entry.prepared
			prepared.groupKey = groupKey
			return prepared
		case <-ctx.Done():
			return preparedPackage{groupKey: groupKey, cancelled: true}
		}
	}
	entry := &preparedMemoEntry{ready: make(chan struct{})}
	r.prep[id] = entry
	r.prepMu.Unlock()

	prepared := r.prepareSelectedPackageUncached(ctx, groupKey, id, backend, pv)
	entry.prepared = prepared
	close(entry.ready)
	return prepared
}

func (r *resolverState) prepareSelectedPackageUncached(ctx context.Context, groupKey string, id string, backend RegistryBackend, pv RegistryPackageVersion) preparedPackage {
	prepared := preparedPackage{groupKey: groupKey}

	body, err := backend.FetchArtifact(ctx, pv.Artifact)
	if err != nil {
		if errorsFromCancellation(err) {
			prepared.cancelled = true
			return prepared
		}
		prepared.failure = prepareFailure(sourceDiagnosticCode(pv.Identity.Source, "ARTIFACT_ERROR"), "failed to fetch registry package artifact", id, err.Error())
		r.recordResolverWorkerError(groupKey)
		return prepared
	}

	if pv.Artifact.Integrity != "" {
		ok, code := verifyIntegrity(body, pv.Artifact.Integrity)
		if code != "" {
			prepared.failure = prepareFailure(sourceDiagnosticCode(pv.Identity.Source, "UNSUPPORTED_INTEGRITY"), "unsupported registry artifact integrity algorithm", id, pv.Artifact.Integrity)
			r.recordResolverWorkerError(groupKey)
			return prepared
		}
		if !ok {
			prepared.failure = prepareFailure(sourceDiagnosticCode(pv.Identity.Source, "ARTIFACT_INTEGRITY_FAILED"), "registry artifact integrity mismatch", id)
			r.recordResolverWorkerError(groupKey)
			return prepared
		}
	}

	manifest, ok := parseTarballPackageJSON(body)
	if !ok {
		prepared.failure = prepareFailure(sourceDiagnosticCode(pv.Identity.Source, "ARTIFACT_PACKAGE_JSON_MISSING"), "registry tarball package.json missing", id)
		r.recordResolverWorkerError(groupKey)
		return prepared
	}
	if manifest.Name != pv.ArtifactPackageName || manifest.Version != pv.Version {
		prepared.failure = prepareFailure(sourceDiagnosticCode(pv.Identity.Source, "ARTIFACT_METADATA_MISMATCH"), "registry artifact package metadata mismatch", id, manifest.Name, pv.ArtifactPackageName)
		r.recordResolverWorkerError(groupKey)
		return prepared
	}

	hash := sha256.Sum256(body)
	pkg := lockfile.Package{
		ID:           id,
		Name:         pv.Identity.Name,
		Version:      pv.Version,
		Source:       pv.Identity.Source,
		Integrity:    pv.Artifact.Integrity,
		Hash:         "sha256:" + hex.EncodeToString(hash[:]),
		Capabilities: append([]lockfile.Capability(nil), pv.Capabilities...),
	}

	if r.opts.OnArtifactResolved != nil {
		if err := r.opts.OnArtifactResolved(pkg, body); err != nil {
			prepared.failure = prepareFailure("TSPACK_RESOLVE_ARTIFACT_CAPTURE_FAILED", "failed to capture resolved artifact", id, err.Error())
			r.recordResolverWorkerError(groupKey)
			return prepared
		}
	}

	transitive := make([]resolveWorkItem, 0, len(pv.Dependencies))
	for _, dep := range pv.Dependencies {
		transitive = append(transitive, resolveWorkItem{
			source:    dep.Identity.Source,
			name:      dep.Identity.Name,
			rng:       dep.Constraint,
			from:      id,
			kind:      dep.Kind,
			optional:  dep.Optional,
			reference: dep.Reference,
		})
	}

	prepared.packageID = id
	prepared.packageDef = pkg
	prepared.transitive = transitive
	prepared.peers = append([]DependencyRequirement(nil), pv.PeerRequirements...)
	if r.opts.OnPreparedPackage != nil {
		r.opts.OnPreparedPackage(groupKey)
	}
	return prepared
}

func (r *resolverState) applyRequirementControllers(frontier []resolveWorkItem) []resolveWorkItem {
	tape := requirements.Build(r.requirementFacts)
	controllers := map[string]requirements.PackageRequirement{}
	for _, controller := range tape.Controllers() {
		controllers[requirements.SlotKey(controller)] = controller
	}
	out := make([]resolveWorkItem, 0, len(frontier))
	for _, request := range frontier {
		if request.slot == "" {
			out = append(out, request)
			continue
		}
		controller, ok := controllers[request.slot]
		if !ok {
			continue
		}
		request.source = controller.Target.Source
		request.name = controller.Target.Name
		request.rng = controller.Constraint
		out = append(out, request)
	}
	return out
}

func (r *resolverState) appendPeerRequirements(packageID string, peers []DependencyRequirement) {
	if _, exists := r.peerRequirementsByPackage[packageID]; exists {
		return
	}
	packageRequirements := []requirements.PackageRequirement{}
	for _, peer := range peers {
		requirement := requirements.PackageRequirement{
			ID:         fmt.Sprintf("peer:%s:%s:%06d", packageID, peer.Reference, r.requirementOrder),
			Target:     packageidentity.PackageIdentity{Source: peer.Identity.Source, Name: peer.Identity.Name},
			Reference:  peer.Reference,
			Constraint: peer.Constraint,
			Kind:       requirements.KindPeer,
			Optional:   peer.Optional,
			Origin: requirements.Origin{
				PackageID:     packageID,
				DependencyKey: peer.Reference,
				Source:        "registry-metadata",
			},
			Scope: requirements.Scope{Environment: "workspace"},
			Order: r.requirementOrder,
		}
		r.requirementOrder++
		r.requirementFacts = append(r.requirementFacts, requirement)
		packageRequirements = append(packageRequirements, requirement)
	}
	r.peerRequirementsByPackage[packageID] = packageRequirements
}

func (r *resolverState) stabilizePeerRequirements(ctx context.Context, resolveJobs int) {
	r.resolvePeerRequirements(ctx, resolveJobs)
	r.reconcilePeerEnvironmentEdges()
	r.pruneUnreachablePackages()
	r.recomputeSelectedSlots()
}

func (r *resolverState) resolvePeerRequirements(ctx context.Context, resolveJobs int) {
	for iteration := 0; iteration <= len(r.requirementFacts)+1; iteration++ {
		tape := requirements.Build(r.requirementFacts)
		frontier := []resolveWorkItem{}
		for _, controller := range tape.Controllers() {
			if controller.Kind != requirements.KindPeer {
				continue
			}
			slot := requirements.SlotKey(controller)
			if selectedVersionSatisfies(r.selectedSlots[slot], controller.Constraint) {
				continue
			}
			frontier = append(frontier, resolveWorkItem{
				source:    controller.Target.Source,
				name:      controller.Target.Name,
				rng:       controller.Constraint,
				from:      peerEnvironmentRoot(controller),
				kind:      "peer",
				optional:  controller.Optional,
				reference: controller.Reference,
				slot:      slot,
			})
		}
		if len(frontier) == 0 {
			return
		}

		factsBefore := len(r.requirementFacts)
		for len(frontier) > 0 {
			next, fatal := r.resolveFrontier(ctx, frontier, resolveJobs)
			if fatal {
				return
			}
			frontier = next
		}
		if factsBefore == len(r.requirementFacts) {
			return
		}
	}
}

func (r *resolverState) reconcilePeerEnvironmentEdges() {
	controllers := map[string]requirements.PackageRequirement{}
	for _, controller := range requirements.Build(r.requirementFacts).Controllers() {
		controllers[requirements.SlotKey(controller)] = controller
	}
	packages := map[string]lockfile.Package{}
	for _, pkg := range r.result.Lock.Packages {
		packages[pkg.ID] = pkg
	}
	kept := r.result.Lock.Edges[:0]
	for _, edge := range r.result.Lock.Edges {
		if !strings.HasPrefix(edge.From, "workspace:peer:") {
			kept = append(kept, edge)
			continue
		}
		pkg, ok := packages[edge.To]
		if !ok {
			continue
		}
		requirement := requirements.PackageRequirement{
			Target: packageidentity.PackageIdentity{Source: pkg.Source, Name: pkg.Name},
			Scope:  requirements.Scope{Environment: "workspace"},
		}
		controller, ok := controllers[requirements.SlotKey(requirement)]
		if !ok || controller.Kind != requirements.KindPeer || !selectedVersionSatisfies(pkg.Version, controller.Constraint) {
			continue
		}
		kept = append(kept, edge)
	}
	r.result.Lock.Edges = kept
}

func (r *resolverState) pruneUnreachablePackages() map[string]bool {
	packageIDs := map[string]bool{}
	for _, pkg := range r.result.Lock.Packages {
		packageIDs[pkg.ID] = true
	}
	reachable := map[string]bool{}
	frontier := []string{}
	for _, edge := range r.result.Lock.Edges {
		if !packageIDs[edge.From] && packageIDs[edge.To] && !reachable[edge.To] {
			reachable[edge.To] = true
			frontier = append(frontier, edge.To)
		}
	}
	for len(frontier) > 0 {
		current := frontier[0]
		frontier = frontier[1:]
		for _, edge := range r.result.Lock.Edges {
			if edge.From != current || !packageIDs[edge.To] || reachable[edge.To] {
				continue
			}
			reachable[edge.To] = true
			frontier = append(frontier, edge.To)
		}
	}
	packages := r.result.Lock.Packages[:0]
	for _, pkg := range r.result.Lock.Packages {
		if reachable[pkg.ID] {
			packages = append(packages, pkg)
		}
	}
	r.result.Lock.Packages = packages
	edges := r.result.Lock.Edges[:0]
	for _, edge := range r.result.Lock.Edges {
		if !reachable[edge.To] {
			continue
		}
		if packageIDs[edge.From] && !reachable[edge.From] {
			continue
		}
		edges = append(edges, edge)
	}
	r.result.Lock.Edges = edges
	return reachable
}

func (r *resolverState) recomputeSelectedSlots() {
	r.selectedSlots = map[string]string{}
	packages := map[string]lockfile.Package{}
	for _, pkg := range r.result.Lock.Packages {
		packages[pkg.ID] = pkg
	}
	knownSlots := map[string]bool{}
	for _, requirement := range r.requirementFacts {
		knownSlots[requirements.SlotKey(requirement)] = true
	}
	for _, edge := range r.result.Lock.Edges {
		if _, fromPackage := packages[edge.From]; fromPackage {
			continue
		}
		pkg, ok := packages[edge.To]
		if !ok {
			continue
		}
		requirement := requirements.PackageRequirement{
			Target: packageidentity.PackageIdentity{Source: pkg.Source, Name: pkg.Name},
			Scope:  requirements.Scope{Environment: "workspace"},
		}
		slot := requirements.SlotKey(requirement)
		if knownSlots[slot] {
			r.selectedSlots[slot] = pkg.Version
		}
	}
}

func (r *resolverState) finalizeRequirementTape() {
	classified := requirements.Build(r.requirementFacts).Classify(r.selectedSlots)
	r.result.Lock.Requirements = make([]lockfile.Requirement, 0, len(classified.Entries))
	for _, entry := range classified.Entries {
		shadowedBy := ""
		if entry.ShadowedBy != nil {
			shadowedBy = entry.ShadowedBy.ID
		}
		requirement := entry.Requirement
		r.result.Lock.Requirements = append(r.result.Lock.Requirements, lockfile.Requirement{
			ID:               requirement.ID,
			Scope:            requirement.Scope.Key(),
			TargetSource:     requirement.Target.Source,
			TargetName:       requirement.Target.Name,
			Reference:        requirement.Reference,
			Constraint:       requirement.Constraint,
			Kind:             string(requirement.Kind),
			RequiringPackage: requirement.Origin.RequiringPackage,
			PackageID:        requirement.Origin.PackageID,
			DependencyKey:    requirement.Origin.DependencyKey,
			OriginSource:     requirement.Origin.Source,
			Order:            requirement.Order,
			Optional:         requirement.Optional,
			Controlling:      entry.Controlling,
			ShadowedBy:       shadowedBy,
			Status:           string(entry.Status),
			SelectedVersion:  entry.SelectedVersion,
		})
		if entry.Status == requirements.StatusOverriddenIncompatible && requirement.Kind == requirements.KindPeer {
			r.result.Diagnostics = append(r.result.Diagnostics, diag.Diagnostic{
				Code:     "TSPACK_PEER_REQUIREMENT_OVERRIDDEN",
				Severity: diag.SeverityWarning,
				Message:  fmt.Sprintf("%s requests peer %s %s, but the selected environment version is %s", requirementOriginLabel(requirement), requirement.Target.Key(), requirement.Constraint, entry.SelectedVersion),
				Details: []string{
					"requirement: " + requirement.ID,
					"target: " + requirement.Target.Key(),
					"constraint: " + requirement.Constraint,
					"selectedVersion: " + entry.SelectedVersion,
					"controllingRequirement: " + shadowedBy,
				},
			})
		}
	}
}

func peerEnvironmentRoot(requirement requirements.PackageRequirement) string {
	return "workspace:peer:" + requirement.Target.Source + ":" + requirement.Target.Name
}

func selectedVersionSatisfies(version string, constraint string) bool {
	if version == "" {
		return false
	}
	wanted, err := semver.NewConstraint(constraint)
	if err != nil {
		return false
	}
	selected, err := semver.NewVersion(version)
	return err == nil && wanted.Check(selected)
}

func packageVersionFromID(id string) string {
	separator := strings.LastIndex(id, "@")
	if separator < 0 {
		return ""
	}
	return id[separator+1:]
}

func requirementOriginLabel(requirement requirements.PackageRequirement) string {
	if requirement.Origin.PackageID != "" {
		return requirement.Origin.PackageID
	}
	if requirement.Origin.RequiringPackage != "" {
		return requirement.Origin.RequiringPackage
	}
	return requirement.ID
}

func hasErrorDiagnostics(diagnostics []diag.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == diag.SeverityError {
			return true
		}
	}
	return false
}

func (r *resolverState) packageMetadata(ctx context.Context, backend RegistryBackend, name string) (*RegistryPackageMetadata, error) {
	key := backend.Source() + "|" + name

	r.metaMu.Lock()
	if r.meta == nil {
		r.meta = map[string]*metadataMemoEntry{}
	}
	if entry, ok := r.meta[key]; ok {
		r.metaMu.Unlock()
		if r.opts.OnMetadataCacheHit != nil {
			r.opts.OnMetadataCacheHit(backend.Source(), name)
		}
		select {
		case <-entry.ready:
			return entry.meta, entry.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	entry := &metadataMemoEntry{ready: make(chan struct{})}
	r.meta[key] = entry
	r.metaMu.Unlock()

	entry.meta, entry.err = backend.Metadata(ctx, name)
	close(entry.ready)
	return entry.meta, entry.err
}

func (r *resolverState) recordResolverWorkerError(groupKey string) {
	if r.opts.OnResolverWorkerError != nil {
		r.opts.OnResolverWorkerError(groupKey)
	}
}

func errorsFromCancellation(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}

func prepareFailure(code, message string, details ...string) *preparedFailure {
	return &preparedFailure{
		code:    code,
		message: message,
		details: append([]string(nil), details...),
	}
}

func buildResolveDiagnostic(optional bool, failure *preparedFailure) diag.Diagnostic {
	if optional {
		return dWarn(failure.code, failure.message, failure.details...)
	}
	return dErr(failure.code, failure.message, failure.details...)
}

func selectRegistryVersion(meta *RegistryPackageMetadata, rng string) (RegistryPackageVersion, string, string, bool) {
	c, err := semver.NewConstraint(rng)
	if err != nil {
		return RegistryPackageVersion{}, "", "invalid-range", false
	}
	versions := make([]*semver.Version, 0, len(meta.Versions))
	lookup := map[string]RegistryPackageVersion{}
	for version, pkgVersion := range meta.Versions {
		semVersion, err := semver.NewVersion(version)
		if err != nil {
			continue
		}
		versions = append(versions, semVersion)
		lookup[semVersion.Original()] = pkgVersion
	}
	sort.SliceStable(versions, func(i, j int) bool {
		return versions[i].GreaterThan(versions[j])
	})
	for _, version := range versions {
		if c.Check(version) {
			return lookup[version.Original()], version.Original(), "", true
		}
	}
	return RegistryPackageVersion{}, "", "", false
}

func backendHostMap(backends BackendRegistry) map[string]int {
	hosts := map[string]int{}
	for _, backend := range backends {
		host := backend.Host()
		if host != "" {
			hosts[host]++
		}
	}
	return hosts
}

func sourceDiagnosticCode(source string, condition string) string {
	if source == SourceNPM {
		npmConditions := map[string]string{
			"METADATA_ERROR":                "TSPACK_RESOLVE_NPM_METADATA_ERROR",
			"PACKAGE_NOT_FOUND":             "TSPACK_RESOLVE_NPM_PACKAGE_NOT_FOUND",
			"VERSION_NOT_FOUND":             "TSPACK_RESOLVE_NPM_VERSION_NOT_FOUND",
			"INVALID_RANGE":                 "TSPACK_RESOLVE_NPM_INVALID_RANGE",
			"ARTIFACT_ERROR":                "TSPACK_RESOLVE_NPM_TARBALL_ERROR",
			"UNSUPPORTED_INTEGRITY":         "TSPACK_RESOLVE_NPM_UNSUPPORTED_INTEGRITY",
			"ARTIFACT_INTEGRITY_FAILED":     "TSPACK_RESOLVE_NPM_INTEGRITY_MISMATCH",
			"ARTIFACT_PACKAGE_JSON_MISSING": "TSPACK_RESOLVE_NPM_TARBALL_PACKAGE_JSON_MISSING",
			"ARTIFACT_METADATA_MISMATCH":    "TSPACK_RESOLVE_NPM_TARBALL_METADATA_MISMATCH",
		}
		if code := npmConditions[condition]; code != "" {
			return code
		}
	}
	return "TSPACK_" + strings.ToUpper(source) + "_" + condition
}

type depEntry struct {
	name string
	rng  string
}

func sortedDeps(dependencies map[string]string) []depEntry {
	out := make([]depEntry, 0, len(dependencies))
	for name, rng := range dependencies {
		out = append(out, depEntry{name: name, rng: rng})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].name < out[j].name
	})
	return out
}

type packageJSON struct {
	Name    string            `json:"name"`
	Version string            `json:"version"`
	Scripts map[string]string `json:"-"`
}

type rawPackageJSON struct {
	Name    string         `json:"name"`
	Version string         `json:"version"`
	Scripts map[string]any `json:"scripts"`
}

func parseTarballPackageJSON(body []byte) (packageJSON, bool) {
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return packageJSON{}, false
	}
	defer gz.Close()

	var rootPackageJSON []byte
	var topLevelPackageJSON []byte
	foundTopLevelPackageJSON := false
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return packageJSON{}, false
		}

		cleanName, ok := cleanTarballPackageJSONPath(hdr.Name)
		if !ok {
			continue
		}

		content, err := io.ReadAll(tr)
		if err != nil {
			return packageJSON{}, false
		}

		if cleanName == "package.json" {
			rootPackageJSON = content
			continue
		}

		if foundTopLevelPackageJSON {
			return packageJSON{}, false
		}
		topLevelPackageJSON = content
		foundTopLevelPackageJSON = true
	}

	selected := topLevelPackageJSON
	if !foundTopLevelPackageJSON {
		selected = rootPackageJSON
	}
	if selected == nil {
		return packageJSON{}, false
	}

	var raw rawPackageJSON
	if err := json.Unmarshal(selected, &raw); err != nil {
		return packageJSON{}, false
	}
	return packageJSON{
		Name:    raw.Name,
		Version: raw.Version,
		Scripts: stringScripts(raw.Scripts),
	}, true
}

func cleanTarballPackageJSONPath(name string) (string, bool) {
	if name == "" || strings.HasPrefix(name, "/") {
		return "", false
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", false
		}
	}

	cleanName := path.Clean(name)
	if cleanName == "." || cleanName == "" {
		return "", false
	}
	if cleanName == ".." || strings.HasPrefix(cleanName, "../") || strings.Contains(cleanName, "/../") {
		return "", false
	}

	if cleanName == "package.json" {
		return cleanName, true
	}
	if !strings.HasSuffix(cleanName, "/package.json") {
		return "", false
	}

	parts := strings.Split(cleanName, "/")
	if len(parts) != 2 || parts[1] != "package.json" || parts[0] == "" {
		return "", false
	}
	return cleanName, true
}

func verifyIntegrity(body []byte, integrity string) (bool, string) {
	parts := strings.SplitN(integrity, "-", 2)
	if len(parts) != 2 {
		return false, "TSPACK_RESOLVE_NPM_UNSUPPORTED_INTEGRITY"
	}
	want, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return false, "TSPACK_RESOLVE_NPM_UNSUPPORTED_INTEGRITY"
	}

	var got []byte
	switch parts[0] {
	case "sha512":
		hash := sha512.Sum512(body)
		got = hash[:]
	case "sha256":
		hash := sha256.Sum256(body)
		got = hash[:]
	default:
		return false, "TSPACK_RESOLVE_NPM_UNSUPPORTED_INTEGRITY"
	}
	return subtle.ConstantTimeCompare(got, want) == 1, ""
}

func mustNormalizeLock(lf *lockfile.Lockfile) *lockfile.Lockfile {
	body, _ := lockfile.Marshal(lf)
	normalized, _ := lockfile.Parse("generated.ts-lock.toml", body)
	return normalized
}

func dErr(code, msg string, details ...string) diag.Diagnostic {
	return diag.Diagnostic{Code: code, Severity: diag.SeverityError, Message: msg, Details: details}
}

func dWarn(code, msg string, details ...string) diag.Diagnostic {
	return diag.Diagnostic{Code: code, Severity: diag.SeverityWarning, Message: msg, Details: details}
}

func stringScripts(raw map[string]any) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	scripts := map[string]string{}
	for name, value := range raw {
		command, ok := value.(string)
		if !ok {
			continue
		}
		scripts[name] = command
	}
	if len(scripts) == 0 {
		return nil
	}
	return scripts
}
