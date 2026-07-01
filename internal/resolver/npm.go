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
	"github.com/yuechen-li-dev/tspack/internal/capability"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
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
	Mode                  ResolveMode
	RootDir               string
	ResolveJobs           int
	ResolveControllerMode ResolveControllerMode
	ResolveHostBudget     int
	OnArtifactResolved    func(pkg lockfile.Package, artifact []byte) error
	OnMetadataCacheHit    func(name string)
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
	Dist                 PackageDist       `json:"dist"`
	Scripts              map[string]string `json:"scripts"`
}

type PackageDist struct {
	Tarball   string `json:"tarball"`
	Integrity string `json:"integrity"`
}

type resolverState struct {
	opts        ResolverOptions
	result      *ResolveResult
	seenPkg     map[string]bool
	graph       *graph.WorkspaceGraph
	controller  ResolveOccupancyController
	hostMap     map[string]int
	frontierSeq int
	metaMu      sync.Mutex
	meta        map[string]*metadataMemoEntry
	prepMu      sync.Mutex
	prep        map[string]*preparedMemoEntry
}

type metadataMemoEntry struct {
	ready chan struct{}
	meta  *PackageMetadata
	err   error
}

type preparedMemoEntry struct {
	ready    chan struct{}
	prepared preparedPackage
}

type resolveWorkItem struct {
	name     string
	rng      string
	from     string
	kind     string
	optional bool
}

type resolveWorkGroup struct {
	key         string
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
	if req.Graph == nil || opts.Client == nil {
		result.Diagnostics = append(result.Diagnostics, dErr("TSPACK_RESOLVE_INTERNAL_ERROR", "resolver requires graph and client"))
		return result
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
		opts:       opts,
		result:     &result,
		seenPkg:    map[string]bool{},
		graph:      req.Graph,
		controller: NewResolveOccupancyController(opts.ResolveControllerMode, resolveJobs, opts.ResolveHostBudget),
		hostMap:    ResolveControllerHostMap(opts.RegistryURL),
	}

	frontier := make([]resolveWorkItem, 0)
	for _, pkg := range req.Graph.AllPackages() {
		for _, target := range pkg.AllTargets() {
			from := fmt.Sprintf("%s:target:%s", pkg.Name, target.Name)
			for _, dep := range target.AllowedRuntimeDependencies() {
				frontier = state.enqueueDirectRequest(ctx, frontier, dep, from, dependencyEdgeKind(dep))
			}
			for _, dep := range target.AllowedPeerDependencies() {
				frontier = state.enqueueDirectRequest(ctx, frontier, dep, from, "peer")
			}
		}
		for _, dep := range pkg.ToolDependencies() {
			frontier = state.enqueueDirectRequest(ctx, frontier, dep, fmt.Sprintf("%s:tool", pkg.Name), "tool")
		}
	}

	for len(frontier) > 0 {
		nextFrontier, fatal := state.resolveFrontier(ctx, frontier, resolveJobs)
		if fatal {
			break
		}
		frontier = nextFrontier
	}

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
	if dep.Source.Kind != "npm" {
		r.resolveNonNPMDependency(ctx, dep, from, kind)
		return frontier
	}
	frontier = append(frontier, resolveWorkItem{
		name:     dep.Source.Package,
		rng:      dep.Source.Range,
		from:     from,
		kind:     kind,
		optional: dep.Optional,
	})
	return frontier
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
		groupKey := packageWorkGroupKey(request.name, request.rng)
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

		r.result.Lock.Edges = append(r.result.Lock.Edges, lockfile.Edge{
			From:     request.from,
			To:       prepared.packageID,
			Kind:     request.kind,
			Optional: request.optional,
		})

		if r.seenPkg[prepared.packageID] {
			continue
		}

		r.result.Lock.Packages = append(r.result.Lock.Packages, prepared.packageDef)
		r.seenPkg[prepared.packageID] = true
		if r.opts.OnCommittedPackage != nil {
			r.opts.OnCommittedPackage(prepared.packageID)
		}
		nextFrontier = append(nextFrontier, prepared.transitive...)
	}

	return nextFrontier, fatal
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
		groupKey := packageWorkGroupKey(request.name, request.rng)
		group, ok := groupsByKey[groupKey]
		if !ok {
			group = &resolveWorkGroup{
				key:  groupKey,
				name: request.name,
				rng:  request.rng,
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
	return request.name + "|" + request.rng + "|" + request.from + "|" + request.kind + "|" + boolKey(request.optional)
}

func packageWorkGroupKey(name, rng string) string {
	return name + "|" + rng
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

	meta, err := r.packageMetadata(ctx, group.name)
	if err != nil {
		code := "TSPACK_RESOLVE_NPM_METADATA_ERROR"
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			code = "TSPACK_RESOLVE_NPM_PACKAGE_NOT_FOUND"
		}
		if errorsFromCancellation(err) {
			prepared.cancelled = true
			return prepared
		}
		prepared.failure = prepareFailure(code, "failed to fetch npm metadata", group.name, err.Error())
		r.recordResolverWorkerError(group.key)
		return prepared
	}

	pv, version, selCode, ok := selectVersion(meta, group.rng)
	if !ok {
		code := "TSPACK_RESOLVE_NPM_VERSION_NOT_FOUND"
		if selCode != "" {
			code = selCode
		}
		prepared.failure = prepareFailure(code, "failed to select npm version", group.name, group.rng)
		r.recordResolverWorkerError(group.key)
		return prepared
	}

	id := fmt.Sprintf("npm:%s@%s", group.name, version)
	if r.seenPkg[id] {
		prepared.packageID = id
		return prepared
	}

	prepared = r.prepareSelectedPackage(ctx, group.key, id, pv)
	prepared.groupKey = group.key
	return prepared
}

func (r *resolverState) prepareSelectedPackage(ctx context.Context, groupKey string, id string, pv PackageVersion) preparedPackage {
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

	prepared := r.prepareSelectedPackageUncached(ctx, groupKey, id, pv)
	entry.prepared = prepared
	close(entry.ready)
	return prepared
}

func (r *resolverState) prepareSelectedPackageUncached(ctx context.Context, groupKey string, id string, pv PackageVersion) preparedPackage {
	prepared := preparedPackage{groupKey: groupKey}

	body, err := r.opts.Client.Tarball(ctx, pv.Dist.Tarball)
	if err != nil {
		if errorsFromCancellation(err) {
			prepared.cancelled = true
			return prepared
		}
		prepared.failure = prepareFailure("TSPACK_RESOLVE_NPM_TARBALL_ERROR", "failed to fetch npm tarball", id, err.Error())
		r.recordResolverWorkerError(groupKey)
		return prepared
	}

	if pv.Dist.Integrity != "" {
		ok, code := verifyIntegrity(body, pv.Dist.Integrity)
		if code != "" {
			prepared.failure = prepareFailure(code, "unsupported npm integrity algorithm", id, pv.Dist.Integrity)
			r.recordResolverWorkerError(groupKey)
			return prepared
		}
		if !ok {
			prepared.failure = prepareFailure("TSPACK_RESOLVE_NPM_INTEGRITY_MISMATCH", "tarball integrity mismatch", id)
			r.recordResolverWorkerError(groupKey)
			return prepared
		}
	}

	manifest, ok := parseTarballPackageJSON(body)
	if !ok {
		prepared.failure = prepareFailure("TSPACK_RESOLVE_NPM_TARBALL_PACKAGE_JSON_MISSING", "tarball package.json missing", id)
		r.recordResolverWorkerError(groupKey)
		return prepared
	}
	if manifest.Name != pv.Name || manifest.Version != pv.Version {
		prepared.failure = prepareFailure("TSPACK_RESOLVE_NPM_TARBALL_METADATA_MISMATCH", "tarball package metadata mismatch", id)
		r.recordResolverWorkerError(groupKey)
		return prepared
	}

	hash := sha256.Sum256(body)
	pkg := lockfile.Package{
		ID:           id,
		Name:         pv.Name,
		Version:      pv.Version,
		Source:       "npm",
		Integrity:    pv.Dist.Integrity,
		Hash:         "sha256:" + hex.EncodeToString(hash[:]),
		Capabilities: capability.FromPackageJSONScripts(manifest.Scripts),
	}

	if r.opts.OnArtifactResolved != nil {
		if err := r.opts.OnArtifactResolved(pkg, body); err != nil {
			prepared.failure = prepareFailure("TSPACK_RESOLVE_ARTIFACT_CAPTURE_FAILED", "failed to capture resolved artifact", id, err.Error())
			r.recordResolverWorkerError(groupKey)
			return prepared
		}
	}

	transitive := make([]resolveWorkItem, 0, len(pv.Dependencies)+len(pv.OptionalDependencies))
	for _, dep := range sortedDeps(pv.Dependencies) {
		transitive = append(transitive, resolveWorkItem{
			name: dep.name,
			rng:  dep.rng,
			from: id,
			kind: "runtime",
		})
	}
	for _, dep := range sortedDeps(pv.OptionalDependencies) {
		transitive = append(transitive, resolveWorkItem{
			name:     dep.name,
			rng:      dep.rng,
			from:     id,
			kind:     "runtime",
			optional: true,
		})
	}

	prepared.packageID = id
	prepared.packageDef = pkg
	prepared.transitive = transitive
	if r.opts.OnPreparedPackage != nil {
		r.opts.OnPreparedPackage(groupKey)
	}
	return prepared
}

func (r *resolverState) packageMetadata(ctx context.Context, name string) (*PackageMetadata, error) {
	key := r.registryMetadataCacheKey(name)

	r.metaMu.Lock()
	if r.meta == nil {
		r.meta = map[string]*metadataMemoEntry{}
	}
	if entry, ok := r.meta[key]; ok {
		r.metaMu.Unlock()
		if r.opts.OnMetadataCacheHit != nil {
			r.opts.OnMetadataCacheHit(name)
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

	entry.meta, entry.err = r.opts.Client.PackageMetadata(ctx, name)
	close(entry.ready)
	return entry.meta, entry.err
}

func (r *resolverState) registryMetadataCacheKey(name string) string {
	if client, ok := r.opts.Client.(*HTTPRegistryClient); ok && client.BaseURL != "" {
		return client.BaseURL + "|" + name
	}
	if r.opts.RegistryURL != "" {
		return r.opts.RegistryURL + "|" + name
	}
	return "default|" + name
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

func selectVersion(meta *PackageMetadata, rng string) (PackageVersion, string, string, bool) {
	c, err := semver.NewConstraint(rng)
	if err != nil {
		return PackageVersion{}, "", "TSPACK_RESOLVE_NPM_INVALID_RANGE", false
	}
	versions := make([]*semver.Version, 0, len(meta.Versions))
	lookup := map[string]PackageVersion{}
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
	return PackageVersion{}, "", "", false
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
