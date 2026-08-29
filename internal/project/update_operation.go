package project

import (
	"context"
	"fmt"
	"github.com/yuechen-li-dev/tspack/internal/check"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/perf"
	"github.com/yuechen-li-dev/tspack/internal/resolver"
	"github.com/yuechen-li-dev/tspack/internal/store"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

func Update(opts Options) Result {
	return UpdateWithOptions(opts, UpdateOptions{})
}

func UpdateDryRun(opts Options) Result {
	return UpdateDryRunWithOptions(opts, UpdateOptions{})
}

func UpdateWithOptions(opts Options, updateOpts UpdateOptions) Result {
	return updateWithMode(opts, false, updateOpts)
}

func UpdateDryRunWithOptions(opts Options, updateOpts UpdateOptions) Result {
	return updateWithMode(opts, true, updateOpts)
}

func updateWithMode(opts Options, dryRun bool, updateOpts UpdateOptions) Result {
	perfSession, perfErr := ensurePerfSession(&opts, "update", dryRun)
	if perfErr != nil {
		return Result{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_PERF_PROFILE_INIT_FAILED", "failed to initialize performance profiling", perfErr.Error())}}
	}
	if perfSession != nil {
		defer func() {
			_ = perfSession.Close()
		}()
	}
	progress := opts.Progress
	if updateOpts.Query != "" {
		if dryRun {
			progress.Step("planning targeted update: %s", updateOpts.Query)
		} else {
			progress.Step("updating target dependency: %s", updateOpts.Query)
		}
	}
	ir, g, out := loadManifestAndGraph(opts)
	out = append(out, check.CheckPackage(check.CheckOptions{RootDir: opts.RootDir, Graph: g, AllowMissingOutput: true}).Diagnostics...)
	if hasErrors(out) {
		return Result{Diagnostics: out}
	}
	targetResult := &UpdateTargetResult{Targeted: updateOpts.Query != "", Query: updateOpts.Query, DirectPackages: directNPMPackageNames(g)}
	var old *lockfile.Lockfile
	if _, err := os.Stat(opts.LockfilePath); err == nil {
		lf, d, e := lockfile.LoadFile(opts.LockfilePath)
		if e != nil {
			out = append(out, errDiag("TSPACK_UPDATE_RESOLVE_FAILED", "failed to read existing lockfile", e.Error()))
			return Result{Diagnostics: out}
		}
		out = append(out, d...)
		old = lf
	}
	if updateOpts.Query != "" {
		selection, selectionDiags := selectUpdateTargets(g, updateOpts.Query)
		out = append(out, selectionDiags...)
		if hasErrors(out) {
			return Result{Diagnostics: out, UpdateTarget: targetResult}
		}
		targetResult.Selected = selection
		if old != nil {
			preserveDiags := preserveNonSelectedNPMLocks(g, old, selection)
			out = append(out, preserveDiags...)
			if hasErrors(out) {
				return Result{Diagnostics: out, UpdateTarget: targetResult}
			}
		}
	}
	client := opts.ResolverClient
	client = instrumentRegistryClient(client, perfSession, resolver.SourceNPM)
	backends := opts.ResolverBackends
	if backends == nil {
		if opts.ResolverClient != nil && !hasDeclaredSourcePolicy(ir) {
			backends = resolver.BackendRegistry{
				resolver.SourceNPM: resolver.NewNPMBackend(client),
				resolver.SourceJSR: resolver.NewJSRBackend(instrumentRegistryClient(resolver.NewHTTPRegistryClient(resolver.DefaultJSREndpoint), perfSession, resolver.SourceJSR)),
			}
		} else {
			var backendErr error
			backends, backendErr = sourcePolicyBackends(resolverSourcePolicy(ir), perfSession)
			if backendErr != nil {
				out = append(out, errDiag("TSPACK_SOURCE_POLICY_INVALID", "invalid registry source policy", backendErr.Error()))
				return Result{Diagnostics: out, UpdateTarget: targetResult}
			}
		}
	}
	var (
		st                    *store.Store
		storeJobs             int
		resolveJobs           int
		resolveControllerMode resolver.ResolveControllerMode
	)
	resolveJobs, jobsErr := resolveJobsFromEnv()
	if jobsErr != nil {
		out = append(out, errDiag("TSPACK_UPDATE_RESOLVE_JOBS_INVALID", "invalid TSPACK_RESOLVE_JOBS", jobsErr.Error()))
		return Result{Diagnostics: out, UpdateTarget: targetResult}
	}
	resolveControllerMode, jobsErr = resolveControllerModeFromEnv()
	if jobsErr != nil {
		out = append(out, errDiag("TSPACK_UPDATE_RESOLVE_CONTROLLER_INVALID", "invalid TSPACK_RESOLVE_CONTROLLER", jobsErr.Error()))
		return Result{Diagnostics: out, UpdateTarget: targetResult}
	}
	if !dryRun {
		storeJobs, jobsErr = storeJobsFromEnv()
		if jobsErr != nil {
			out = append(out, errDiag("TSPACK_UPDATE_STORE_JOBS_INVALID", "invalid TSPACK_STORE_JOBS", jobsErr.Error()))
			return Result{Diagnostics: out, UpdateTarget: targetResult}
		}
		var err error
		st, err = store.Open(opts.StoreRoot)
		if err != nil {
			out = append(out, errDiag("TSPACK_UPDATE_STORE_OPEN_FAILED", "failed to open store", err.Error()))
			return Result{Diagnostics: out, UpdateTarget: targetResult}
		}
	}
	progress.Step("resolving packages...")
	progress.Step("fetching metadata...")
	resolveOpts := resolver.ResolverOptions{
		Mode:                  resolver.ResolveModeUpdate,
		Client:                client,
		Backends:              backends,
		RootDir:               opts.RootDir,
		ResolveJobs:           resolveJobs,
		ResolveControllerMode: resolveControllerMode,
		ResolveHostBudget:     defaultResolveControllerHostBudget(resolveJobs),
		OnArtifactResolved:    storeArtifactCapture(st, perfSession),
		OnMetadataCacheHit: func(source string, name string) {
			perfSession.RecordMetadataCacheHit()
		},
		OnResolveJobs:     func(jobs int) { perfSession.SetResolveJobs(jobs) },
		OnResolveFrontier: func(width int) { perfSession.RecordResolveFrontier(width) },
		OnResolveController: func(decision resolver.FrontierDecision) {
			perfSession.RecordResolveControllerDecision(
				decision.FrontierIndex,
				decision.FrontierWidth,
				decision.WorkItems,
				decision.MetadataItems,
				decision.TarballItems,
				decision.TargetJobs,
				decision.MaxJobs,
				decision.Hosts,
				decision.ClampReasons,
			)
		},
		OnPreparedPackage:     func(key string) { perfSession.RecordPreparedPackage() },
		OnCommittedPackage:    func(id string) { perfSession.RecordCommittedPackage() },
		OnResolverWorkerError: func(key string) { perfSession.RecordResolveWorkerError() },
		SourcePolicy:          resolverSourcePolicy(ir),
	}
	perfSession.SetResolveController(string(resolveControllerMode), resolveJobs, defaultResolveControllerHostBudget(resolveJobs))
	if registryClient, ok := client.(*resolver.HTTPRegistryClient); ok {
		resolveOpts.RegistryURL = registryClient.BaseURL
	}
	stopResolve := perfSession.StartPhase("update.resolve")
	res := resolver.Resolve(context.Background(), resolveOpts, resolver.ResolveRequest{Graph: g, ExistingLock: old})
	stopResolve()
	out = append(out, res.Diagnostics...)
	if hasErrors(out) {
		return Result{Diagnostics: out}
	}
	if updateOpts.Query != "" && old != nil {
		res.Lock = preserveNonSelectedTargetedLockEntries(old, res.Lock, targetResult.Selected)
	}
	if dryRun {
		progress.Step("computing lockfile diff...")
	}
	d := lockfile.DiffLockfiles(old, res.Lock)
	packageChanges := lockDiffHasPackageChanges(d)
	requirementChanges := lockDiffHasRequirementChanges(d)
	if updateOpts.Query != "" && !packageChanges {
		progress.Step("%s is already at wanted version; no package changes.", updateOpts.Query)
	}
	if dryRun {
		summary := UpdateDiffSummary{
			Added:     len(d.PackagesAdded),
			Removed:   len(d.PackagesRemoved),
			Changed:   len(d.PackagesChanged),
			Unchanged: len(res.Lock.Packages) - len(d.PackagesAdded) - len(d.PackagesChanged),
		}
		if summary.Unchanged < 0 {
			summary.Unchanged = 0
		}
		progress.Step("dry run complete")
		return Result{Diagnostics: out, LockDiff: &d, DryRun: &UpdateDryRunResult{Changed: packageChanges || requirementChanges, Summary: summary}, UpdateTarget: targetResult}
	}
	if updateOpts.Query != "" && !packageChanges && !requirementChanges {
		return Result{Diagnostics: out, LockDiff: &d, UpdateTarget: targetResult}
	}
	if packageChanges || updateOpts.Query == "" {
		progress.Step("populating store...")
		// Resolution above remains deterministic and serial. Only artifact fetch/copy/extract
		// store population runs with bounded parallelism, and results are applied back to
		// the lockfile in package order before deterministic lockfile output is written.
		stopStorePopulation := perfSession.StartPhase("update.store_population")
		populateResult := populateStoreParallel(context.Background(), st, client, opts.RootDir, res.Lock.Packages, storeJobs, progress, perfSession)
		stopStorePopulation()
		out = append(out, populateResult.Diagnostics...)
		for _, populated := range populateResult.Packages {
			res.Lock.Packages[populated.Index].Hash = populated.Hash
		}
		if hasErrors(out) {
			return Result{Diagnostics: out, UpdateTarget: targetResult}
		}
	}
	progress.Step("writing lockfile...")
	stopLockfileWrite := perfSession.StartPhase("update.lockfile_write")
	b, e := lockfile.Marshal(res.Lock)
	if e != nil {
		stopLockfileWrite()
		out = append(out, errDiag("TSPACK_UPDATE_WRITE_FAILED", "failed to encode lockfile", e.Error()))
		return Result{Diagnostics: out, UpdateTarget: targetResult}
	}
	if e = os.MkdirAll(filepath.Dir(opts.LockfilePath), 0o755); e != nil {
		stopLockfileWrite()
		out = append(out, errDiag("TSPACK_UPDATE_WRITE_FAILED", "failed to create lockfile dir", e.Error()))
		return Result{Diagnostics: out, UpdateTarget: targetResult}
	}
	if e = os.WriteFile(opts.LockfilePath, b, 0o644); e != nil {
		out = append(out, errDiag("TSPACK_UPDATE_WRITE_FAILED", "failed to write lockfile", e.Error()))
	}
	stopLockfileWrite()
	if !hasErrors(out) {
		progress.Step("update complete")
	}
	return Result{Diagnostics: out, LockDiff: &d, UpdateTarget: targetResult}
}

func lockDiffHasPackageChanges(diff lockfile.Diff) bool {
	return len(diff.PackagesAdded) > 0 ||
		len(diff.PackagesRemoved) > 0 ||
		len(diff.PackagesChanged) > 0
}

func lockDiffHasRequirementChanges(diff lockfile.Diff) bool {
	return len(diff.RequirementsAdded) > 0 ||
		len(diff.RequirementsRemoved) > 0 ||
		len(diff.RequirementsChanged) > 0
}

func directNPMPackageNames(g *graph.WorkspaceGraph) []string {
	seen := map[string]bool{}
	names := []string{}
	for _, pkg := range g.AllPackages() {
		for _, dependency := range pkg.AllDependencies() {
			if dependency.Source.Kind != "npm" || dependency.Source.Package == "" || seen[dependency.Source.Package] {
				continue
			}
			seen[dependency.Source.Package] = true
			names = append(names, dependency.Source.Package)
		}
	}
	sort.Strings(names)
	return names
}

type populatedPackage struct {
	Index int
	Hash  string
}

type storePopulateResult struct {
	Packages    []populatedPackage
	Diagnostics []diag.Diagnostic
}

type storePopulateJob struct {
	Index int
	Pkg   lockfile.Package
}

type storePopulateWorkerResult struct {
	Index       int
	PackageKey  string
	Hash        string
	Diagnostics []diag.Diagnostic
}

func defaultStoreJobs() int {
	return 24
}

func defaultResolveJobs() int {
	return 24
}

func storeJobsFromEnv() (int, error) {
	value := strings.TrimSpace(os.Getenv("TSPACK_STORE_JOBS"))
	if value == "" {
		return defaultStoreJobs(), nil
	}
	jobs, err := strconv.Atoi(value)
	if err != nil || jobs <= 0 {
		return 0, fmt.Errorf("TSPACK_STORE_JOBS must be a positive integer, got %q", value)
	}
	return jobs, nil
}

func resolveJobsFromEnv() (int, error) {
	value := strings.TrimSpace(os.Getenv("TSPACK_RESOLVE_JOBS"))
	if value == "" {
		return defaultResolveJobs(), nil
	}
	jobs, err := strconv.Atoi(value)
	if err != nil || jobs <= 0 {
		return 0, fmt.Errorf("TSPACK_RESOLVE_JOBS must be a positive integer, got %q", value)
	}
	return jobs, nil
}

func defaultResolveControllerHostBudget(resolveJobs int) int {
	if resolveJobs <= 0 {
		return defaultResolveJobs()
	}
	return resolveJobs
}

func resolveControllerModeFromEnv() (resolver.ResolveControllerMode, error) {
	value := strings.TrimSpace(os.Getenv("TSPACK_RESOLVE_CONTROLLER"))
	mode, ok := resolver.ParseResolveControllerMode(value)
	if !ok {
		return "", fmt.Errorf("TSPACK_RESOLVE_CONTROLLER must be fixed or feedforward, got %q", value)
	}
	return mode, nil
}

func populateStoreParallel(ctx context.Context, st *store.Store, client resolver.NPMRegistryClient, rootDir string, packages []lockfile.Package, jobs int, progress Progress, perfSession *perf.Session) storePopulateResult {
	packagesToPopulate := packagesNeedingStorePopulation(st, packages)
	perfSession.SetStorePopulationCounts(len(packagesToPopulate), len(packages)-len(packagesToPopulate))
	if len(packagesToPopulate) == 0 {
		return storePopulateResult{}
	}
	workerCount := jobs
	if workerCount > len(packagesToPopulate) {
		workerCount = len(packagesToPopulate)
	}
	progress.Step("populating store: %d packages with %d workers", len(packagesToPopulate), workerCount)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobCh := make(chan storePopulateJob)
	resultCh := make(chan storePopulateWorkerResult, len(packagesToPopulate))
	var wg sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				select {
				case <-ctx.Done():
					resultCh <- storePopulateWorkerResult{Index: job.Index, PackageKey: job.Pkg.ID}
					continue
				default:
				}
				result := populateOneStorePackage(ctx, st, client, rootDir, job, perfSession)
				if hasErrors(result.Diagnostics) {
					cancel()
				}
				resultCh <- result
			}
		}()
	}
	go func() {
		defer close(jobCh)
		for index, pkg := range packagesToPopulate {
			select {
			case <-ctx.Done():
				return
			case jobCh <- storePopulateJob{Index: packageIndex(packages, pkg), Pkg: pkg}:
				progress.Step("%s [%d/%d] %s", storeFetchProgressLabel(pkg), index+1, len(packagesToPopulate), packageProgressLabel(pkg))
			}
		}
	}()
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	results := make([]storePopulateWorkerResult, 0, len(packagesToPopulate))
	for result := range resultCh {
		results = append(results, result)
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].PackageKey == results[j].PackageKey {
			return results[i].Index < results[j].Index
		}
		return results[i].PackageKey < results[j].PackageKey
	})
	out := storePopulateResult{Packages: make([]populatedPackage, 0, len(results))}
	for _, result := range results {
		out.Diagnostics = append(out.Diagnostics, result.Diagnostics...)
		if len(result.Diagnostics) == 0 && result.Hash != "" {
			out.Packages = append(out.Packages, populatedPackage{Index: result.Index, Hash: result.Hash})
		}
	}
	return out
}

func populateOneStorePackage(ctx context.Context, st *store.Store, client resolver.NPMRegistryClient, rootDir string, job storePopulateJob, perfSession *perf.Session) storePopulateWorkerResult {
	pkg := job.Pkg
	result := storePopulateWorkerResult{Index: job.Index, PackageKey: pkg.ID}
	if pkg.Hash != "" && st.Has(pkg.Hash) {
		result.Hash = pkg.Hash
		return result
	}
	perfSession.RecordStorePopulationFetch()
	switch pkg.Source {
	case "npm":
		body, fetchErr := client.Tarball(ctx, findTarballURL(&pkg, client))
		if fetchErr != nil {
			result.Diagnostics = append(result.Diagnostics, errDiag("TSPACK_RESOLVE_NPM_TARBALL_FETCH_FAILED", "failed to fetch npm tarball", pkg.ID, fetchErr.Error()))
			return result
		}
		ref, diags := st.PutArtifact(store.Artifact{ID: pkg.ID, Name: pkg.Name, Version: pkg.Version, Source: pkg.Source, Hash: pkg.Hash, Integrity: pkg.Integrity, Kind: store.ArtifactNPMTarball, Bytes: body, Metadata: store.PackageMetadata{Name: pkg.Name, Version: pkg.Version, Source: pkg.Source, PackageID: pkg.ID, Integrity: pkg.Integrity, Capabilities: pkg.Capabilities}})
		result.Diagnostics = append(result.Diagnostics, diags...)
		result.Hash = ref.Hash
	case "path", "workspace":
		abs := filepath.Join(rootDir, filepath.FromSlash(pkg.Path))
		kind := store.ArtifactPathTree
		if pkg.Source == "workspace" {
			kind = store.ArtifactWorkspace
		}
		ref, diags := st.PutArtifact(store.Artifact{ID: pkg.ID, Name: pkg.Name, Version: pkg.Version, Source: pkg.Source, Hash: pkg.Hash, Kind: kind, RootDir: abs, Metadata: store.PackageMetadata{Name: pkg.Name, Version: pkg.Version, Source: pkg.Source, PackageID: pkg.ID, Capabilities: pkg.Capabilities}})
		result.Diagnostics = append(result.Diagnostics, diags...)
		result.Hash = ref.Hash
	}
	return result
}

func storeArtifactCapture(st *store.Store, perfSession *perf.Session) func(pkg lockfile.Package, artifact []byte) error {
	if st == nil {
		return nil
	}
	return func(pkg lockfile.Package, artifact []byte) error {
		alreadyPresent := pkg.Hash != "" && st.Has(pkg.Hash)
		if alreadyPresent {
			perfSession.RecordArtifactAlreadyInStore()
		}
		artifactKind := store.ArtifactRegistryTarball
		if pkg.Source == resolver.SourceNPM {
			artifactKind = store.ArtifactNPMTarball
		}
		_, diags := st.PutArtifact(store.Artifact{
			ID:        pkg.ID,
			Name:      pkg.Name,
			Version:   pkg.Version,
			Source:    pkg.Source,
			Hash:      pkg.Hash,
			Integrity: pkg.Integrity,
			Kind:      artifactKind,
			Bytes:     artifact,
			Metadata: store.PackageMetadata{
				Name:             pkg.Name,
				Version:          pkg.Version,
				Source:           pkg.Source,
				PackageID:        pkg.ID,
				Integrity:        pkg.Integrity,
				RegistryEndpoint: pkg.RegistryEndpoint,
				MetadataEndpoint: pkg.MetadataEndpoint,
				ArtifactHost:     pkg.ArtifactHost,
				Capabilities:     append([]lockfile.Capability(nil), pkg.Capabilities...),
			},
		})
		if len(diags) > 0 {
			return fmt.Errorf("%s", diags[0].Message)
		}
		if !alreadyPresent {
			perfSession.RecordArtifactCaptured()
		}
		return nil
	}
}

func packageIndex(packages []lockfile.Package, target lockfile.Package) int {
	for i, pkg := range packages {
		if pkg.ID == target.ID && pkg.Source == target.Source && pkg.Version == target.Version && pkg.Path == target.Path {
			return i
		}
	}
	return 0
}

func packagesNeedingStorePopulation(st *store.Store, packages []lockfile.Package) []lockfile.Package {
	out := make([]lockfile.Package, 0)
	for _, pkg := range packages {
		if pkg.Hash != "" && st.Has(pkg.Hash) {
			continue
		}
		out = append(out, pkg)
	}
	return out
}

func storeFetchProgressLabel(pkg lockfile.Package) string {
	switch pkg.Source {
	case "npm":
		return "fetching npm artifacts"
	case "path", "workspace":
		return "capturing local packages"
	default:
		return "populating store artifacts"
	}
}

func packageProgressLabel(pkg lockfile.Package) string {
	if pkg.Name != "" && pkg.Version != "" {
		return pkg.Name + "@" + pkg.Version
	}
	if pkg.Name != "" {
		return pkg.Name
	}
	if pkg.ID != "" {
		return pkg.ID
	}
	return "<unknown package>"
}

func selectUpdateTargets(g *graph.WorkspaceGraph, query string) ([]UpdateSelectedTarget, []diag.Diagnostic) {
	type match struct {
		pkgName string
		dep     *graph.DependencyNode
	}
	matches := make([]match, 0)
	for _, pkg := range g.AllPackages() {
		for _, dep := range pkg.AllDependencies() {
			if dep.Key == query {
				matches = append(matches, match{pkgName: pkg.Name, dep: dep})
			}
		}
	}
	if len(matches) == 0 {
		for _, pkg := range g.AllPackages() {
			for _, dep := range pkg.AllDependencies() {
				if dep.Source.Kind == "npm" && dep.Source.Package == query {
					matches = append(matches, match{pkgName: pkg.Name, dep: dep})
				}
			}
		}
	}
	if len(matches) == 0 && strings.HasPrefix(query, "npm:") {
		npmName := strings.TrimPrefix(query, "npm:")
		for _, pkg := range g.AllPackages() {
			for _, dep := range pkg.AllDependencies() {
				if dep.Source.Kind == "npm" && dep.Source.Package == npmName {
					matches = append(matches, match{pkgName: pkg.Name, dep: dep})
				}
			}
		}
	}
	if len(matches) == 0 {
		d := errDiag("TSPACK_UPDATE_TARGET_NOT_FOUND", "targeted update query did not match declared dependency", fmt.Sprintf("no declared dependency key or npm package matched %q", query), "targeted update only updates declared dependencies", "use `tspack outdated` to see declared dependencies", "use full update for transitive refresh")
		return nil, []diag.Diagnostic{d}
	}
	selected := make([]UpdateSelectedTarget, 0, len(matches))
	packageName := ""
	for _, m := range matches {
		if m.dep.Source.Kind != "npm" {
			return nil, []diag.Diagnostic{errDiag("TSPACK_UPDATE_TARGET_UNSUPPORTED_SOURCE", "targeted update currently supports npm dependencies only", m.dep.Key, m.dep.Source.Kind)}
		}
		if packageName == "" {
			packageName = m.dep.Source.Package
		} else if packageName != m.dep.Source.Package {
			details := []string{"query matched multiple declared dependencies:"}
			for _, mm := range matches {
				details = append(details, fmt.Sprintf("%s.%s -> %s:%s", mm.pkgName, mm.dep.Key, mm.dep.Source.Kind, mm.dep.Source.Package))
			}
			return nil, []diag.Diagnostic{errDiag("TSPACK_UPDATE_TARGET_AMBIGUOUS", "targeted update query is ambiguous", details...)}
		}
		selected = append(selected, UpdateSelectedTarget{Package: m.pkgName, Key: m.dep.Key, Name: m.dep.Source.Package, Source: m.dep.Source.Kind})
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].Package != selected[j].Package {
			return selected[i].Package < selected[j].Package
		}
		return selected[i].Key < selected[j].Key
	})
	return selected, nil
}

func preserveNonSelectedNPMLocks(g *graph.WorkspaceGraph, old *lockfile.Lockfile, selected []UpdateSelectedTarget) []diag.Diagnostic {
	selectedKeys := map[string]bool{}
	for _, s := range selected {
		selectedKeys[s.Package+":"+s.Key] = true
	}
	lockedVersionByName := map[string]string{}
	for _, e := range old.Edges {
		if !isLifecycleRootEdge(e.From) {
			continue
		}
		if !strings.HasPrefix(e.To, "npm:") {
			continue
		}
		for _, pkg := range old.Packages {
			if pkg.ID == e.To {
				lockedVersionByName[pkg.Name] = pkg.Version
			}
		}
	}
	var out []diag.Diagnostic
	for _, pkg := range g.AllPackages() {
		for _, dep := range pkg.AllDependencies() {
			if dep.Source.Kind != "npm" {
				continue
			}
			depID := pkg.Name + ":" + dep.Key
			if selectedKeys[depID] {
				continue
			}
			lockedVersion, ok := lockedVersionByName[dep.Source.Package]
			if !ok {
				out = append(out, diag.Diagnostic{Code: "TSPACK_UPDATE_TARGET_LOCK_MISSING", Severity: diag.SeverityWarning, Message: "non-selected dependency not locked; resolver may refresh it", Details: []string{pkg.Name, dep.Key, dep.Source.Package}})
				continue
			}
			dep.Source.Range = lockedVersion
		}
	}
	return out
}

func preserveNonSelectedTargetedLockEntries(old, next *lockfile.Lockfile, selected []UpdateSelectedTarget) *lockfile.Lockfile {
	if old == nil || next == nil || len(selected) == 0 {
		return next
	}

	selectedNames := map[string]bool{}
	for _, target := range selected {
		selectedNames[target.Name] = true
	}

	selectedClosure := packageClosureForNames(old, selectedNames)
	for id := range packageClosureForNames(next, selectedNames) {
		selectedClosure[id] = true
	}

	merged := cloneLockfile(next)
	nextPackageIndex := map[string]int{}
	for index, pkg := range merged.Packages {
		nextPackageIndex[pkg.ID] = index
	}

	for _, oldPackage := range old.Packages {
		if selectedClosure[oldPackage.ID] {
			continue
		}
		if index, ok := nextPackageIndex[oldPackage.ID]; ok {
			merged.Packages[index] = clonePackage(oldPackage)
			continue
		}
		merged.Packages = append(merged.Packages, clonePackage(oldPackage))
	}

	merged.Edges = preserveNonSelectedEdges(old, merged.Edges, selectedClosure)
	merged.Requirements = preserveNonSelectedRequirements(old, merged.Requirements, selectedClosure, selectedNames)
	return normalizeLockfile(merged)
}

func packageClosureForNames(lf *lockfile.Lockfile, selectedNames map[string]bool) map[string]bool {
	closure := map[string]bool{}
	if lf == nil || len(selectedNames) == 0 {
		return closure
	}

	packageByID := map[string]lockfile.Package{}
	childrenByParent := map[string][]string{}
	for _, pkg := range lf.Packages {
		packageByID[pkg.ID] = pkg
	}
	for _, edge := range lf.Edges {
		childrenByParent[edge.From] = append(childrenByParent[edge.From], edge.To)
	}

	queue := make([]string, 0)
	for _, edge := range lf.Edges {
		pkg, ok := packageByID[edge.To]
		if !ok || !selectedNames[pkg.Name] {
			continue
		}
		if !isLifecycleRootEdge(edge.From) {
			continue
		}
		if !closure[pkg.ID] {
			closure[pkg.ID] = true
			queue = append(queue, pkg.ID)
		}
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, child := range childrenByParent[current] {
			if closure[child] {
				continue
			}
			closure[child] = true
			queue = append(queue, child)
		}
	}
	return closure
}

func preserveNonSelectedEdges(old *lockfile.Lockfile, nextEdges []lockfile.Edge, selectedClosure map[string]bool) []lockfile.Edge {
	kept := make([]lockfile.Edge, 0, len(nextEdges))
	seen := map[string]bool{}
	for _, edge := range nextEdges {
		if edgeTouchesSelectedClosure(edge, selectedClosure) {
			kept = append(kept, edge)
			seen[lockEdgeKey(edge)] = true
			continue
		}
	}
	for _, edge := range old.Edges {
		if edgeTouchesSelectedClosure(edge, selectedClosure) {
			continue
		}
		key := lockEdgeKey(edge)
		if seen[key] {
			continue
		}
		kept = append(kept, edge)
		seen[key] = true
	}
	return kept
}

func edgeTouchesSelectedClosure(edge lockfile.Edge, selectedClosure map[string]bool) bool {
	return selectedClosure[edge.From] || selectedClosure[edge.To]
}

func lockEdgeKey(edge lockfile.Edge) string {
	optional := "0"
	if edge.Optional {
		optional = "1"
	}
	return edge.From + "|" + edge.To + "|" + edge.Kind + "|" + optional + "|" + edge.Reference
}

func preserveNonSelectedRequirements(old *lockfile.Lockfile, next []lockfile.Requirement, selectedClosure map[string]bool, selectedNames map[string]bool) []lockfile.Requirement {
	oldByID := map[string]lockfile.Requirement{}
	for _, requirement := range old.Requirements {
		oldByID[requirement.ID] = requirement
	}
	kept := make([]lockfile.Requirement, 0, len(next)+len(old.Requirements))
	seen := map[string]bool{}
	for _, requirement := range next {
		if selectedNames[requirement.TargetName] || selectedClosure[requirement.PackageID] {
			kept = append(kept, requirement)
			seen[requirement.ID] = true
			continue
		}
		if oldRequirement, exists := oldByID[requirement.ID]; exists {
			kept = append(kept, oldRequirement)
		} else {
			kept = append(kept, requirement)
		}
		seen[requirement.ID] = true
	}
	for _, requirement := range old.Requirements {
		if seen[requirement.ID] || selectedNames[requirement.TargetName] || selectedClosure[requirement.PackageID] {
			continue
		}
		kept = append(kept, requirement)
		seen[requirement.ID] = true
	}
	return kept
}

func cloneLockfile(lf *lockfile.Lockfile) *lockfile.Lockfile {
	if lf == nil {
		return nil
	}
	clone := *lf
	clone.Packages = make([]lockfile.Package, 0, len(lf.Packages))
	for _, pkg := range lf.Packages {
		clone.Packages = append(clone.Packages, clonePackage(pkg))
	}
	clone.Edges = append([]lockfile.Edge(nil), lf.Edges...)
	clone.Requirements = append([]lockfile.Requirement(nil), lf.Requirements...)
	clone.Targets = append([]lockfile.Target(nil), lf.Targets...)
	return &clone
}

func clonePackage(pkg lockfile.Package) lockfile.Package {
	clone := pkg
	clone.Capabilities = append([]lockfile.Capability(nil), pkg.Capabilities...)
	return clone
}

func normalizeLockfile(lf *lockfile.Lockfile) *lockfile.Lockfile {
	encoded, err := lockfile.Marshal(lf)
	if err != nil {
		return lf
	}
	normalized, diagnostics := lockfile.Parse("targeted-update.ts-lock.toml", encoded)
	if len(diagnostics) > 0 {
		return lf
	}
	return normalized
}
