package project

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/yuechen-li-dev/tspack/internal/bridge"
	capmodel "github.com/yuechen-li-dev/tspack/internal/capability"
	"github.com/yuechen-li-dev/tspack/internal/check"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/materialize"
	"github.com/yuechen-li-dev/tspack/internal/pack"
	"github.com/yuechen-li-dev/tspack/internal/resolver"
	"github.com/yuechen-li-dev/tspack/internal/securityevidence"
	"github.com/yuechen-li-dev/tspack/internal/store"
	"github.com/yuechen-li-dev/tspack/internal/why"
)

type Options struct {
	RootDir, ManifestPath, LockfilePath, StoreRoot string
	ManifestIRPath                                 string
	FrontendCLIPath                                string
	ResolverClient                                 resolver.NPMRegistryClient
	Progress                                       Progress
}
type Result struct {
	Diagnostics  []diag.Diagnostic
	LockDiff     *lockfile.Diff
	DryRun       *UpdateDryRunResult
	UpdateTarget *UpdateTargetResult
	PackResult   *PackResult
	WhyResult    *why.Result
	Outdated     *OutdatedResult
	Explain      *check.ExplainResult
}
type UpdateDryRunResult struct {
	Changed bool
	Summary UpdateDiffSummary
}
type UpdateDiffSummary struct {
	Added, Removed, Changed, Unchanged int
}

type UpdateOptions struct {
	Query string
}

type UpdateTargetResult struct {
	Targeted bool
	Query    string
	Selected []UpdateSelectedTarget
}

type UpdateSelectedTarget struct {
	Package string
	Key     string
	Name    string
	Source  string
}

type WhyOptions struct {
	Query       string
	PackageName string
	Reverse     bool
}

type PackOptions struct {
	OutputDir   string
	PackageName string
	DryRun      bool
	Verify      bool
}
type PackResult struct {
	Artifacts []PackArtifact
	Preview   []PackFile
}
type PackArtifact struct {
	PackageName string
	Version     string
	Path        string
	Hash        string
	Size        int64
	Verified    bool
}
type PackFile struct {
	PackageName string
	SourcePath  string
	ArchivePath string
	Size        int64
	Reason      string
}

func DefaultOptions(root string) Options {
	if root == "" {
		root = "."
	}
	return Options{RootDir: root, ManifestPath: filepath.Join(root, "manifest.tsx"), LockfilePath: filepath.Join(root, "ts-lock.toml"), StoreRoot: filepath.Join(root, ".tspack", "store")}
}

func Check(opts Options) Result {
	ir, g, out := loadManifestAndGraph(opts)
	if ir != nil {
		out = append(out, securityevidence.Diagnostics(opts.RootDir, ir.Security.AcknowledgedCapabilities)...)
	}
	out = append(out, check.CheckPackage(check.CheckOptions{RootDir: opts.RootDir, Graph: g}).Diagnostics...)
	if _, err := os.Stat(opts.LockfilePath); err == nil {
		lf, d, e := lockfile.LoadFile(opts.LockfilePath)
		if e != nil {
			out = append(out, errDiag("TSPACK_CHECK_FAILED", "failed to read lockfile", e.Error()))
		} else {
			out = append(out, d...)
			out = append(out, lockfile.CheckGraphConsistency(g, lf).Diagnostics...)
			out = append(out, lockfile.CheckVersionConflicts(lf).Diagnostics...)
			out = append(out, lifecycleCapabilityDiagnostics(lf, lifecycleAcknowledgementSet(ir), lifecycleCategoryAcknowledgements(ir))...)
		}
	} else if os.IsNotExist(err) {
		out = append(out, diag.Diagnostic{Code: "TSPACK_CHECK_LOCKFILE_MISSING", Severity: diag.SeverityWarning, Message: "lockfile is missing"})
	}
	diag.SortDiagnostics(out)
	return Result{Diagnostics: out}
}

func CheckExplain(opts Options, requestedFile string) Result {
	rootAbs, err := filepath.Abs(opts.RootDir)
	if err != nil {
		return Result{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_CHECK_EXPLAIN_FAILED", "failed to resolve project root", err.Error())}}
	}
	requestPath := requestedFile
	if !filepath.IsAbs(requestPath) {
		requestPath = filepath.Join(rootAbs, requestPath)
	}
	fileAbs, err := filepath.Abs(requestPath)
	if err != nil {
		return Result{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_CHECK_EXPLAIN_FAILED", "failed to resolve explain file", err.Error())}}
	}
	rel, err := filepath.Rel(rootAbs, fileAbs)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return Result{Diagnostics: []diag.Diagnostic{{Code: "TSPACK_CHECK_EXPLAIN_FILE_OUTSIDE_ROOT", Severity: diag.SeverityError, Message: "explain file is outside the project root", File: requestedFile}}}
	}
	st, err := os.Stat(fileAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{Diagnostics: []diag.Diagnostic{{Code: "TSPACK_CHECK_EXPLAIN_FILE_NOT_FOUND", Severity: diag.SeverityError, Message: "explain file does not exist", File: filepath.ToSlash(rel)}}}
		}
		return Result{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_CHECK_EXPLAIN_FAILED", "failed to stat explain file", err.Error())}}
	}
	if st.IsDir() {
		return Result{Diagnostics: []diag.Diagnostic{{Code: "TSPACK_CHECK_EXPLAIN_UNSUPPORTED_FILE", Severity: diag.SeverityError, Message: "explain path must be a supported source file", File: filepath.ToSlash(rel)}}}
	}
	if !isExplainSourceFile(fileAbs) {
		return Result{Diagnostics: []diag.Diagnostic{{Code: "TSPACK_CHECK_EXPLAIN_UNSUPPORTED_FILE", Severity: diag.SeverityError, Message: "explain file must be .ts, .tsx, .js, or .jsx", File: filepath.ToSlash(rel)}}}
	}
	_, g, out := loadManifestAndGraph(opts)
	if hasErrors(out) {
		diag.SortDiagnostics(out)
		return Result{Diagnostics: out}
	}
	explain := check.Explain(check.ExplainOptions{RootDir: rootAbs, Graph: g, File: filepath.ToSlash(rel)})
	return Result{Diagnostics: out, Explain: &explain}
}

func isExplainSourceFile(path string) bool {
	switch filepath.Ext(path) {
	case ".ts", ".tsx", ".js", ".jsx":
		return true
	default:
		return false
	}
}

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
	progress := opts.Progress
	if updateOpts.Query != "" {
		if dryRun {
			progress.Step("planning targeted update: %s", updateOpts.Query)
		} else {
			progress.Step("updating target dependency: %s", updateOpts.Query)
		}
	}
	_, g, out := loadManifestAndGraph(opts)
	out = append(out, check.CheckPackage(check.CheckOptions{RootDir: opts.RootDir, Graph: g}).Diagnostics...)
	if hasErrors(out) {
		return Result{Diagnostics: out}
	}
	targetResult := &UpdateTargetResult{Targeted: updateOpts.Query != "", Query: updateOpts.Query}
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
	if client == nil {
		client = resolver.NewHTTPRegistryClient("")
	}
	progress.Step("resolving packages...")
	progress.Step("fetching metadata...")
	res := resolver.Resolve(context.Background(), resolver.ResolverOptions{Mode: resolver.ResolveModeUpdate, Client: client, RootDir: opts.RootDir}, resolver.ResolveRequest{Graph: g, ExistingLock: old})
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
	if updateOpts.Query != "" && !lockDiffHasPackageChanges(d) {
		progress.Step("%s is already at wanted version; no lockfile changes.", updateOpts.Query)
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
		return Result{Diagnostics: out, LockDiff: &d, DryRun: &UpdateDryRunResult{Changed: lockDiffHasPackageChanges(d), Summary: summary}, UpdateTarget: targetResult}
	}
	if updateOpts.Query != "" && !lockDiffHasPackageChanges(d) {
		return Result{Diagnostics: out, LockDiff: &d, UpdateTarget: targetResult}
	}
	progress.Step("populating store...")
	st, err := store.Open(opts.StoreRoot)
	if err != nil {
		out = append(out, errDiag("TSPACK_UPDATE_STORE_OPEN_FAILED", "failed to open store", err.Error()))
		return Result{Diagnostics: out, UpdateTarget: targetResult}
	}
	jobs, jobsErr := storeJobsFromEnv()
	if jobsErr != nil {
		out = append(out, errDiag("TSPACK_UPDATE_STORE_JOBS_INVALID", "invalid TSPACK_STORE_JOBS", jobsErr.Error()))
		return Result{Diagnostics: out, UpdateTarget: targetResult}
	}
	// Resolution above remains deterministic and serial. Only artifact fetch/copy/extract
	// store population runs with bounded parallelism, and results are applied back to
	// the lockfile in package order before deterministic lockfile output is written.
	populateResult := populateStoreParallel(context.Background(), st, client, opts.RootDir, res.Lock.Packages, jobs, progress)
	out = append(out, populateResult.Diagnostics...)
	for _, populated := range populateResult.Packages {
		res.Lock.Packages[populated.Index].Hash = populated.Hash
	}
	if hasErrors(out) {
		return Result{Diagnostics: out, UpdateTarget: targetResult}
	}
	progress.Step("writing lockfile...")
	b, e := lockfile.Marshal(res.Lock)
	if e != nil {
		out = append(out, errDiag("TSPACK_UPDATE_WRITE_FAILED", "failed to encode lockfile", e.Error()))
		return Result{Diagnostics: out, UpdateTarget: targetResult}
	}
	if e = os.MkdirAll(filepath.Dir(opts.LockfilePath), 0o755); e != nil {
		out = append(out, errDiag("TSPACK_UPDATE_WRITE_FAILED", "failed to create lockfile dir", e.Error()))
		return Result{Diagnostics: out, UpdateTarget: targetResult}
	}
	if e = os.WriteFile(opts.LockfilePath, b, 0o644); e != nil {
		out = append(out, errDiag("TSPACK_UPDATE_WRITE_FAILED", "failed to write lockfile", e.Error()))
	}
	if !hasErrors(out) {
		progress.Step("update complete")
	}
	return Result{Diagnostics: out, LockDiff: &d, UpdateTarget: targetResult}
}

func lockDiffHasPackageChanges(diff lockfile.Diff) bool {
	return len(diff.PackagesAdded) > 0 || len(diff.PackagesRemoved) > 0 || len(diff.PackagesChanged) > 0
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
	jobs := runtime.NumCPU() * 2
	if jobs < 2 {
		jobs = 2
	}
	if jobs > 8 {
		jobs = 8
	}
	return jobs
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

func populateStoreParallel(ctx context.Context, st *store.Store, client resolver.NPMRegistryClient, rootDir string, packages []lockfile.Package, jobs int, progress Progress) storePopulateResult {
	packagesToPopulate := packagesNeedingStorePopulation(st, packages)
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
				result := populateOneStorePackage(ctx, st, client, rootDir, job)
				if hasErrors(result.Diagnostics) {
					cancel()
				}
				resultCh <- result
			}
		}()
	}
	go func() {
		defer close(jobCh)
		for _, pkg := range packagesToPopulate {
			select {
			case <-ctx.Done():
				return
			case jobCh <- storePopulateJob{Index: packageIndex(packages, pkg), Pkg: pkg}:
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

func populateOneStorePackage(ctx context.Context, st *store.Store, client resolver.NPMRegistryClient, rootDir string, job storePopulateJob) storePopulateWorkerResult {
	pkg := job.Pkg
	result := storePopulateWorkerResult{Index: job.Index, PackageKey: pkg.ID}
	if pkg.Hash != "" && st.Has(pkg.Hash) {
		result.Hash = pkg.Hash
		return result
	}
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
		ref, diags := st.PutArtifact(store.Artifact{ID: pkg.ID, Name: pkg.Name, Version: pkg.Version, Source: pkg.Source, Hash: pkg.Hash, Kind: store.ArtifactPathTree, RootDir: abs, Metadata: store.PackageMetadata{Name: pkg.Name, Version: pkg.Version, Source: pkg.Source, PackageID: pkg.ID, Capabilities: pkg.Capabilities}})
		result.Diagnostics = append(result.Diagnostics, diags...)
		result.Hash = ref.Hash
	}
	return result
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
		if !strings.Contains(e.From, ":target:") && !strings.HasSuffix(e.From, ":tool") {
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
		if !strings.Contains(edge.From, ":target:") && !strings.HasSuffix(edge.From, ":tool") {
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
	return edge.From + "|" + edge.To + "|" + edge.Kind + "|" + optional
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

func Pack(opts Options, packOpts PackOptions) Result {
	if packOpts.DryRun && packOpts.Verify {
		return Result{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_PACK_INVALID_ARGS", "--verify requires a produced archive and cannot be combined with --dry-run")}}
	}

	_, g, out := loadManifestAndGraph(opts)
	out = append(out, check.CheckPackage(check.CheckOptions{RootDir: opts.RootDir, Graph: g}).Diagnostics...)
	if _, err := os.Stat(opts.LockfilePath); err == nil {
		lf, d, e := lockfile.LoadFile(opts.LockfilePath)
		if e != nil {
			out = append(out, errDiag("TSPACK_PACK_LOCKFILE_STALE", "failed to read lockfile", e.Error()))
		} else {
			out = append(out, d...)
			out = append(out, lockfile.CheckGraphConsistency(g, lf).Diagnostics...)
		}
	} else if os.IsNotExist(err) {
		out = append(out, diag.Diagnostic{Code: "TSPACK_CHECK_LOCKFILE_MISSING", Severity: diag.SeverityWarning, Message: "lockfile is missing"})
	}
	if hasErrors(out) {
		diag.SortDiagnostics(out)
		return Result{Diagnostics: out}
	}
	pkgs := g.AllPackages()
	if packOpts.PackageName != "" {
		p, ok := g.Package(packOpts.PackageName)
		if !ok {
			return Result{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_PACK_PACKAGE_NOT_FOUND", "package not found", packOpts.PackageName)}}
		}
		pkgs = []*graph.PackageNode{p}
	}
	pr := &PackResult{}
	plans := []pack.Plan{}
	packOptions := pack.Options{OutputDir: packOpts.OutputDir, DryRun: packOpts.DryRun, Verify: packOpts.Verify}
	for _, p := range pkgs {
		r := pack.PlanPackage(opts.RootDir, p, packOptions)
		out = append(out, r.Diagnostics...)
		plans = append(plans, r.Plans...)
		for _, f := range r.Preview {
			pr.Preview = append(pr.Preview, PackFile(f))
		}
	}
	if hasErrors(out) {
		diag.SortDiagnostics(out)
		return Result{Diagnostics: out, PackResult: pr}
	}
	if !packOpts.DryRun {
		writeResult := pack.WritePlans(plans, packOptions)
		out = append(out, writeResult.Diagnostics...)
		for _, a := range writeResult.Artifacts {
			pr.Artifacts = append(pr.Artifacts, PackArtifact(a))
		}
	}
	diag.SortDiagnostics(out)
	return Result{Diagnostics: out, PackResult: pr}
}

func Why(opts Options, whyOpts WhyOptions) Result {
	ir, g, out := loadManifestAndGraph(opts)
	if hasErrors(out) {
		return Result{Diagnostics: out}
	}
	var lf *lockfile.Lockfile
	lockfileMissing := false
	if _, err := os.Stat(opts.LockfilePath); err == nil {
		parsed, d, e := lockfile.LoadFile(opts.LockfilePath)
		if e != nil {
			out = append(out, errDiag("TSPACK_WHY_LOCKFILE_INVALID", "failed to read lockfile", e.Error()))
		} else {
			lf = parsed
			out = append(out, d...)
			out = append(out, lockfile.CheckGraphConsistency(g, lf).Diagnostics...)
		}
	} else if os.IsNotExist(err) {
		severity := diag.SeverityWarning
		message := "lockfile is missing"
		details := []string(nil)
		if whyOpts.Reverse {
			severity = diag.SeverityError
			details = []string{"reverse why requires a lockfile; run tspack update"}
		}
		out = append(out, diag.Diagnostic{Code: "TSPACK_WHY_LOCKFILE_MISSING", Severity: severity, Message: message, Details: details})
		lockfileMissing = true
	}
	wr := why.Result{}
	if whyOpts.Reverse && lockfileMissing {
		wr = why.Result{}
	} else {
		wr = why.Analyze(g, lf, why.Options{Query: whyOpts.Query, PackageName: whyOpts.PackageName, Reverse: whyOpts.Reverse, RootDir: opts.RootDir, AcknowledgedCapabilities: ir.Security.AcknowledgedCapabilities})
		out = append(out, wr.Diagnostics...)
	}
	diag.SortDiagnostics(out)
	return Result{Diagnostics: out, WhyResult: &wr}
}
func Sync(opts Options, clean bool) Result {
	_, g, out := loadManifestAndGraph(opts)
	_ = g
	lf, d, e := lockfile.LoadFile(opts.LockfilePath)
	if os.IsNotExist(e) {
		return Result{Diagnostics: []diag.Diagnostic{{Code: "TSPACK_SYNC_LOCKFILE_MISSING", Severity: diag.SeverityError, Message: "lockfile is required; run tspack update"}}}
	}
	if e != nil {
		return Result{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_SYNC_LOCKFILE_STALE", "failed to read lockfile", e.Error())}}
	}
	out = append(out, d...)
	out = append(out, lockfile.CheckGraphConsistency(g, lf).Diagnostics...)
	if hasErrors(out) {
		return Result{Diagnostics: out}
	}
	st, err := store.Open(opts.StoreRoot)
	if err != nil {
		return Result{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_SYNC_STORE_ARTIFACT_MISSING", "failed to open store", err.Error())}}
	}
	mat := materialize.NodeModulesMaterializer{}
	mr := mat.Materialize(context.Background(), materialize.Request{WorkspaceRoot: opts.RootDir, Graph: g, Lock: lf, Store: st, Options: materialize.Options{Clean: clean}})
	out = append(out, mr.Diagnostics...)
	diag.SortDiagnostics(out)
	return Result{Diagnostics: out}
}

func loadManifestAndGraph(opts Options) (*manifest.ManifestIR, *graph.WorkspaceGraph, []diag.Diagnostic) {
	ir, d := loadManifestIR(opts)
	if len(d) > 0 {
		return nil, nil, d
	}
	g, gd := graph.Build(ir)
	return ir, g, append([]diag.Diagnostic{}, gd...)
}

func manifestFrontendCLIPath() string {
	resolution := bridge.Resolve("cli.js")
	if resolution.Path != "" {
		return resolution.Path
	}
	if len(resolution.SearchedPaths) > 0 {
		return resolution.SearchedPaths[0]
	}
	return filepath.Join("manifest-frontend", "dist", "cli.js")
}

func loadManifestIR(opts Options) (*manifest.ManifestIR, []diag.Diagnostic) {
	if opts.ManifestIRPath != "" {
		b, err := os.ReadFile(opts.ManifestIRPath)
		if err != nil {
			return nil, []diag.Diagnostic{errDiag("TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED", "failed to read manifest IR", err.Error())}
		}
		return manifest.LoadBytes(opts.ManifestIRPath, b)
	}
	cliPath := opts.FrontendCLIPath
	if cliPath == "" {
		cliPath = manifestFrontendCLIPath()
	}
	if _, err := os.Stat(cliPath); err != nil {
		return nil, []diag.Diagnostic{errDiag("TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED", "manifest frontend CLI not found", strings.Join(bridge.BuildNeededDetails(), "\n"), cliPath)}
	}
	cmd := exec.Command("node", cliPath, opts.ManifestPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, []diag.Diagnostic{{Code: "TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED", Severity: diag.SeverityError, Message: "manifest frontend failed", Details: []string{err.Error(), stderr.String()}}}
	}
	var parsed struct {
		OK          bool              `json:"ok"`
		IR          json.RawMessage   `json:"ir"`
		Diagnostics []diag.Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		return nil, []diag.Diagnostic{errDiag("TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED", "invalid frontend JSON", err.Error())}
	}
	if !parsed.OK {
		if len(parsed.Diagnostics) == 0 {
			return nil, []diag.Diagnostic{errDiag("TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED", "manifest frontend returned failure")}
		}
		return nil, parsed.Diagnostics
	}
	ir, d := manifest.LoadBytes(opts.ManifestPath, parsed.IR)
	return ir, d
}

func errDiag(code, msg string, details ...string) diag.Diagnostic {
	return diag.Diagnostic{Code: code, Severity: diag.SeverityError, Message: msg, Details: details}
}

func FormatResult(r Result) string { return fmt.Sprintf("diagnostics=%d", len(r.Diagnostics)) }

func findTarballURL(pkg *lockfile.Package, client resolver.NPMRegistryClient) string {
	meta, err := client.PackageMetadata(context.Background(), pkg.Name)
	if err != nil {
		return ""
	}
	pv, ok := meta.Versions[pkg.Version]
	if !ok {
		return ""
	}
	return pv.Dist.Tarball
}

func hasErrors(diags []diag.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return true
		}
	}
	return false
}

func lifecycleCapabilityDiagnostics(lf *lockfile.Lockfile, acknowledgements map[string]manifest.AcknowledgedCapability, categoryAcknowledgements []manifest.AcknowledgedLifecycleCategory) []diag.Diagnostic {
	if lf == nil {
		return nil
	}
	diagnostics := []diag.Diagnostic{}
	usedAcknowledgements := map[string]bool{}
	usedCategoryAcknowledgements := map[int]int{}
	staleAcknowledgements := map[string]staleLifecycleAcknowledgement{}
	pathsByPackage := lifecyclePulledByPaths(lf)
	for _, pkg := range lf.Packages {
		for _, capability := range pkg.Capabilities {
			if !isLifecycleCapability(capability) {
				continue
			}
			ackKey := lifecycleAcknowledgementKey(pkg.ID, capability.Script, capability.Command)
			if _, ok := acknowledgements[ackKey]; ok {
				usedAcknowledgements[ackKey] = true
				continue
			}
			classification := capmodel.ClassifyLifecycleScript(capability.Script)
			categoryAcknowledgement, categoryAcknowledgementIndex, categoryAcknowledged := matchingLifecycleCategoryAcknowledgement(classification.LifecycleCategory, capability.Script, categoryAcknowledgements)
			staleKey := lifecycleStaleAcknowledgementKey(pkg.ID, capability.Script)
			for _, acknowledgement := range acknowledgements {
				if acknowledgement.Package == pkg.ID && acknowledgement.Script == capability.Script && acknowledgement.Command != capability.Command {
					acknowledgementKey := lifecycleAcknowledgementKey(acknowledgement.Package, acknowledgement.Script, acknowledgement.Command)
					usedAcknowledgements[acknowledgementKey] = true
					staleAcknowledgements[staleKey] = staleLifecycleAcknowledgement{Acknowledgement: acknowledgement, ActualCommand: capability.Command}
				}
			}
			details := []string{
				"package: " + pkg.ID,
				"lifecycleScriptName: " + capability.Script,
				"script: " + capability.Script,
				"command: " + capability.Command,
				"lifecycleCategory: " + classification.LifecycleCategory,
				"consumerInstallTime: " + fmt.Sprintf("%t", classification.ConsumerInstallTime),
				"execution: blocked by default",
			}
			if categoryAcknowledged {
				usedCategoryAcknowledgements[categoryAcknowledgementIndex]++
				details = append(details,
					"acknowledged: true",
					"acknowledgmentKind: lifecycle-category",
					"acknowledgedByCategory: "+categoryAcknowledgement.Category,
					"reason: "+categoryAcknowledgement.Reason,
				)
			} else {
				details = append(details,
					"acknowledged: false",
					"acknowledgmentKind: null",
				)
			}
			paths := pathsByPackage[pkg.ID]
			if len(paths) > 0 {
				details = append(details, "pulled by:")
				for _, path := range paths {
					details = append(details, "  "+strings.Join(path, " -> "))
				}
			}
			diagnostics = append(diagnostics, diag.Diagnostic{
				Code:     "TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT",
				Severity: diag.SeverityWarning,
				Message:  "package declares install-time lifecycle script",
				Details:  details,
			})
		}
	}
	for _, stale := range staleAcknowledgements {
		diagnostics = append(diagnostics, diag.Diagnostic{
			Code:     "TSPACK_SECURITY_ACKNOWLEDGED_CAPABILITY_STALE",
			Severity: diag.SeverityWarning,
			Message:  "acknowledged lifecycle capability command no longer matches lockfile",
			Details: []string{
				"package: " + stale.Acknowledgement.Package,
				"script: " + stale.Acknowledgement.Script,
				"acknowledged command: " + stale.Acknowledgement.Command,
				"actual command: " + stale.ActualCommand,
			},
		})
	}
	for index, acknowledgement := range categoryAcknowledgements {
		for _, script := range acknowledgement.Scripts {
			if categoryAcknowledgementScriptStale(acknowledgement, script) {
				diagnostics = append(diagnostics, diag.Diagnostic{
					Code:     "TSPACK_SECURITY_ACKNOWLEDGED_LIFECYCLE_CATEGORY_STALE",
					Severity: diag.SeverityWarning,
					Message:  "acknowledged lifecycle category includes script outside that category",
					Details: []string{
						"category: " + acknowledgement.Category,
						"script: " + script,
						"actual category: " + capmodel.ClassifyLifecycleScript(script).LifecycleCategory,
						"reason: " + acknowledgement.Reason,
					},
				})
			}
		}
		if usedCategoryAcknowledgements[index] == 0 {
			diagnostics = append(diagnostics, diag.Diagnostic{
				Code:     "TSPACK_SECURITY_ACKNOWLEDGED_LIFECYCLE_CATEGORY_UNUSED",
				Severity: diag.SeverityWarning,
				Message:  "acknowledged lifecycle category did not match any lockfile capabilities",
				Details: []string{
					"category: " + acknowledgement.Category,
					"scripts: " + strings.Join(acknowledgement.Scripts, ","),
					"reason: " + acknowledgement.Reason,
				},
			})
		}
	}
	for key, acknowledgement := range acknowledgements {
		if usedAcknowledgements[key] {
			continue
		}
		diagnostics = append(diagnostics, diag.Diagnostic{
			Code:     "TSPACK_SECURITY_ACKNOWLEDGED_CAPABILITY_UNUSED",
			Severity: diag.SeverityWarning,
			Message:  "acknowledged lifecycle capability is not present in the lockfile",
			Details: []string{
				"package: " + acknowledgement.Package,
				"script: " + acknowledgement.Script,
				"command: " + acknowledgement.Command,
				"reason: " + acknowledgement.Reason,
			},
		})
	}
	diag.SortDiagnostics(diagnostics)
	return diagnostics
}

type staleLifecycleAcknowledgement struct {
	Acknowledgement manifest.AcknowledgedCapability
	ActualCommand   string
}

func lifecycleCategoryAcknowledgements(ir *manifest.ManifestIR) []manifest.AcknowledgedLifecycleCategory {
	if ir == nil {
		return nil
	}
	return append([]manifest.AcknowledgedLifecycleCategory(nil), ir.Security.AcknowledgedLifecycleCategories...)
}

func matchingLifecycleCategoryAcknowledgement(category string, script string, acknowledgements []manifest.AcknowledgedLifecycleCategory) (manifest.AcknowledgedLifecycleCategory, int, bool) {
	for index, acknowledgement := range acknowledgements {
		if acknowledgement.Category != category {
			continue
		}
		if len(acknowledgement.Scripts) == 0 {
			return acknowledgement, index, true
		}
		for _, acknowledgedScript := range acknowledgement.Scripts {
			if acknowledgedScript == script {
				return acknowledgement, index, true
			}
		}
	}
	return manifest.AcknowledgedLifecycleCategory{}, -1, false
}

func categoryAcknowledgementScriptStale(acknowledgement manifest.AcknowledgedLifecycleCategory, script string) bool {
	classification := capmodel.ClassifyLifecycleScript(script)
	return classification.LifecycleCategory != acknowledgement.Category
}
func lifecycleAcknowledgementSet(ir *manifest.ManifestIR) map[string]manifest.AcknowledgedCapability {
	acknowledgements := map[string]manifest.AcknowledgedCapability{}
	if ir == nil {
		return acknowledgements
	}
	for _, acknowledgement := range ir.Security.AcknowledgedCapabilities {
		if acknowledgement.Kind != capmodel.LifecycleScriptKind {
			continue
		}
		key := lifecycleAcknowledgementKey(acknowledgement.Package, acknowledgement.Script, acknowledgement.Command)
		acknowledgements[key] = acknowledgement
	}
	return acknowledgements
}

func lifecycleAcknowledgementKey(packageID string, script string, command string) string {
	return packageID + "|" + capmodel.LifecycleScriptKind + "|" + script + "|" + command
}

func lifecycleStaleAcknowledgementKey(packageID string, script string) string {
	return packageID + "|" + capmodel.LifecycleScriptKind + "|" + script
}

func isLifecycleCapability(capability lockfile.Capability) bool {
	return capability.Kind == "lifecycleScript" || capability.Kind == "lifecycle-script"
}

func lifecyclePulledByPaths(lf *lockfile.Lockfile) map[string][][]string {
	edgesByFrom := map[string][]lockfile.Edge{}
	for _, edge := range lf.Edges {
		edgesByFrom[edge.From] = append(edgesByFrom[edge.From], edge)
	}
	for from := range edgesByFrom {
		sort.SliceStable(edgesByFrom[from], func(i, j int) bool {
			if edgesByFrom[from][i].To != edgesByFrom[from][j].To {
				return edgesByFrom[from][i].To < edgesByFrom[from][j].To
			}
			return edgesByFrom[from][i].Kind < edgesByFrom[from][j].Kind
		})
	}

	roots := []string{}
	for from := range edgesByFrom {
		if strings.Contains(from, ":target:") || strings.HasSuffix(from, ":tool") {
			roots = append(roots, from)
		}
	}
	sort.Strings(roots)

	pathsByPackage := map[string][][]string{}
	for _, root := range roots {
		queue := [][]string{{root}}
		seen := map[string]bool{}
		for len(queue) > 0 {
			path := queue[0]
			queue = queue[1:]
			current := path[len(path)-1]
			for _, edge := range edgesByFrom[current] {
				if seen[edge.To] {
					continue
				}
				seen[edge.To] = true
				nextPath := append(append([]string(nil), path...), edge.To)
				pathsByPackage[edge.To] = append(pathsByPackage[edge.To], nextPath)
				queue = append(queue, nextPath)
			}
		}
	}
	for packageID := range pathsByPackage {
		paths := pathsByPackage[packageID]
		sort.SliceStable(paths, func(i, j int) bool {
			return strings.Join(paths[i], " -> ") < strings.Join(paths[j], " -> ")
		})
	}
	return pathsByPackage
}
