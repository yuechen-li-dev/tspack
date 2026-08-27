package materialize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/store"
)

type Materializer interface {
	Materialize(ctx context.Context, req Request) Result
}

type Request struct {
	WorkspaceRoot string
	Graph         *graph.WorkspaceGraph
	Lock          *lockfile.Lockfile
	Store         *store.Store
	Options       Options
}

type Options struct {
	Clean     bool
	Force     bool
	LinkMode  LinkMode
	OnPackage func(index int, total int, pkg lockfile.Package)
	Stats     StatsObserver
}

type StatsObserver interface {
	RecordMaterializedPackage(pkg lockfile.Package)
	RecordMaterializedDirectory(path string)
	RecordMaterializedFile(path string, size int64)
	RecordHardlink(path string, size int64)
	RecordCopy(path string, size int64)
	RecordMaterializationMarkerHit()
	RecordMaterializationMarkerMiss()
	RecordMaterializationMarkerMismatch()
	RecordMaterializationMarkerCorrupt()
	RecordMaterializationNoop(packages int, files int, directories int)
	RecordForcedMaterialization()
	RecordMaterializationMarkerWrite()
}

type LinkMode string

const (
	LinkModeCopy     LinkMode = "copy"
	LinkModeSymlink  LinkMode = "symlink"
	LinkModeHardlink LinkMode = "hardlink"
	LinkModeAuto     LinkMode = "auto"
)

type Result struct {
	Diagnostics []diag.Diagnostic
	Written     []WrittenPath
}

type WrittenPath struct {
	Path      string
	Kind      string
	PackageID string
}

type NodeModulesMaterializer struct{}

const markerFile = ".tspack-materialized"

const maxMaterializeDependencyDepth = 64

var materializeLink = os.Link

type materializeFileLockError struct {
	Op       string
	Path     string
	Attempts int
	Err      error
}

func (e *materializeFileLockError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s %s: %v", e.Op, e.Path, e.Err)
}

func (e *materializeFileLockError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (m NodeModulesMaterializer) Materialize(ctx context.Context, req Request) Result {
	_ = ctx
	out := Result{}
	if req.Lock == nil || req.Store == nil || req.WorkspaceRoot == "" {
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_WRITE_FAILED", Severity: diag.SeverityError, Message: "materialize request missing required fields"})
		return out
	}
	mode := req.Options.LinkMode
	if mode == "" || mode == LinkModeAuto {
		mode = LinkModeHardlink
	}
	if mode != LinkModeCopy && mode != LinkModeHardlink {
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_UNSUPPORTED_LINK_MODE", Severity: diag.SeverityError, Message: fmt.Sprintf("link mode %q is not supported by the node_modules materializer", mode)})
		return finalize(out)
	}
	nmRoot := filepath.Join(req.WorkspaceRoot, "node_modules")
	observer := newMaterializeObserver(req.Options.Stats)
	plan := buildMaterializationPlan(req.Lock, nmRoot, mode)
	if req.Options.Force {
		observer.RecordForcedMaterialization()
	}
	if !req.Options.Clean && !req.Options.Force {
		marker, status := loadMaterializationMarker(nmRoot, plan)
		switch status {
		case markerReadHit:
			observer.RecordMaterializationMarkerHit()
			if markerSanityCheck(nmRoot, plan) {
				observer.RecordMaterializationNoop(marker.PackageCount, marker.FileCount, marker.DirectoryCount)
				return finalize(out)
			}
			observer.RecordMaterializationMarkerMismatch()
		case markerReadMiss:
			observer.RecordMaterializationMarkerMiss()
		case markerReadMismatch:
			_ = marker
			observer.RecordMaterializationMarkerMismatch()
		case markerReadCorrupt:
			observer.RecordMaterializationMarkerCorrupt()
		}
	}
	if req.Options.Clean {
		if err := cleanNodeModules(nmRoot); err != nil {
			out.Diagnostics = append(out.Diagnostics, materializeDiagnosticFromError(err, "", nmRoot, ""))
			return finalize(out)
		}
	}
	if err := retryMaterializeFileOp("mkdir", nmRoot, func() error {
		return os.MkdirAll(nmRoot, 0o755)
	}); err != nil {
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_WRITE_FAILED", Severity: diag.SeverityError, File: nmRoot, Message: err.Error()})
		return finalize(out)
	}

	pkgs := map[string]lockfile.Package{}
	for _, p := range req.Lock.Packages {
		pkgs[p.ID] = p
	}
	edgesByFrom := map[string][]lockfile.Edge{}
	for _, e := range req.Lock.Edges {
		edgesByFrom[e.From] = append(edgesByFrom[e.From], e)
	}
	for k := range edgesByFrom {
		sort.SliceStable(edgesByFrom[k], func(i, j int) bool {
			if edgesByFrom[k][i].To != edgesByFrom[k][j].To {
				return edgesByFrom[k][i].To < edgesByFrom[k][j].To
			}
			return edgesByFrom[k][i].Kind < edgesByFrom[k][j].Kind
		})
	}

	rootEdges := collectRootEdges(req.Lock.Edges)
	rootVisible := map[string]lockfile.Package{}
	state := newMaterializeState()
	if req.Options.OnPackage != nil {
		state.progress = req.Options.OnPackage
		state.progressTotal = len(plan.entries)
	}
	for _, e := range rootEdges {
		pkg, ok := pkgs[e.To]
		if !ok {
			out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_EDGE_UNKNOWN_PACKAGE", Severity: diag.SeverityError, Message: "edge points to unknown package", Details: []string{e.From, e.To}})
			continue
		}
		if _, err := safePackagePath(nmRoot, materializedPackageName(pkg)); err != nil {
			out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_INVALID_PACKAGE_NAME", Severity: diag.SeverityError, Message: err.Error(), Details: []string{pkg.ID, pkg.Name}})
			continue
		}
		rootVisible[pkg.ID] = pkg
		materializePkg(req, &out, pkgs, edgesByFrom, pkg, nmRoot, state, observer)
	}
	if len(out.Diagnostics) == 0 {
		if err := pruneNodeModulesRoot(nmRoot, plan.rootVisible); err != nil {
			out.Diagnostics = append(out.Diagnostics, materializeDiagnosticFromError(err, "", nmRoot, ""))
		}
	}
	materializeRootBins(req, &out, nmRoot, rootVisible)
	if len(out.Diagnostics) == 0 {
		marker := expectedMaterializationMarker(plan, observer)
		if err := writeMaterializationMarker(nmRoot, marker); err != nil {
			out.Diagnostics = append(out.Diagnostics, materializeDiagnosticFromError(markerWriteError(materializationMarkerPath(nmRoot), err), "", materializationMarkerPath(nmRoot), ""))
			return finalize(out)
		}
		observer.RecordMaterializationMarkerWrite()
	}
	return finalize(out)
}

type packageJSON struct {
	Name string      `json:"name"`
	Bin  interface{} `json:"bin"`
}

func collectRootEdges(edges []lockfile.Edge) []lockfile.Edge {
	var out []lockfile.Edge
	for _, e := range edges {
		if strings.Contains(e.From, ":target:") || strings.HasSuffix(e.From, ":tool") {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		if out[i].To != out[j].To {
			return out[i].To < out[j].To
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

type materializeState struct {
	seenLocations map[string]struct{}
	stack         []string
	inStack       map[string]int
	progress      func(index int, total int, pkg lockfile.Package)
	progressTotal int
	progressIndex int
}

type materializeObserver struct {
	inner          StatsObserver
	packageCount   int
	fileCount      int
	directoryCount int
}

func newMaterializeObserver(inner StatsObserver) *materializeObserver {
	return &materializeObserver{inner: inner}
}

func (o *materializeObserver) RecordMaterializedPackage(pkg lockfile.Package) {
	o.packageCount++
	if o.inner != nil {
		o.inner.RecordMaterializedPackage(pkg)
	}
}

func (o *materializeObserver) RecordMaterializedDirectory(path string) {
	o.directoryCount++
	if o.inner != nil {
		o.inner.RecordMaterializedDirectory(path)
	}
}

func (o *materializeObserver) RecordMaterializedFile(path string, size int64) {
	o.fileCount++
	if o.inner != nil {
		o.inner.RecordMaterializedFile(path, size)
	}
}

func (o *materializeObserver) RecordHardlink(path string, size int64) {
	if o.inner != nil {
		o.inner.RecordHardlink(path, size)
	}
}

func (o *materializeObserver) RecordCopy(path string, size int64) {
	if o.inner != nil {
		o.inner.RecordCopy(path, size)
	}
}

func (o *materializeObserver) RecordMaterializationMarkerHit() {
	if o.inner != nil {
		o.inner.RecordMaterializationMarkerHit()
	}
}

func (o *materializeObserver) RecordMaterializationMarkerMiss() {
	if o.inner != nil {
		o.inner.RecordMaterializationMarkerMiss()
	}
}

func (o *materializeObserver) RecordMaterializationMarkerMismatch() {
	if o.inner != nil {
		o.inner.RecordMaterializationMarkerMismatch()
	}
}

func (o *materializeObserver) RecordMaterializationMarkerCorrupt() {
	if o.inner != nil {
		o.inner.RecordMaterializationMarkerCorrupt()
	}
}

func (o *materializeObserver) RecordMaterializationNoop(packages int, files int, directories int) {
	if o.inner != nil {
		o.inner.RecordMaterializationNoop(packages, files, directories)
	}
}

func (o *materializeObserver) RecordForcedMaterialization() {
	if o.inner != nil {
		o.inner.RecordForcedMaterialization()
	}
}

func (o *materializeObserver) RecordMaterializationMarkerWrite() {
	if o.inner != nil {
		o.inner.RecordMaterializationMarkerWrite()
	}
}

func newMaterializeState() *materializeState {
	return &materializeState{
		seenLocations: map[string]struct{}{},
		inStack:       map[string]int{},
	}
}

func (s *materializeState) containsPackage(pkgID string) bool {
	_, ok := s.inStack[pkgID]
	return ok
}

func (s *materializeState) push(pkgID string) {
	s.stack = append(s.stack, pkgID)
	s.inStack[pkgID]++
}

func (s *materializeState) pop(pkgID string) {
	if len(s.stack) > 0 {
		s.stack = s.stack[:len(s.stack)-1]
	}
	remaining := s.inStack[pkgID] - 1
	if remaining <= 0 {
		delete(s.inStack, pkgID)
		return
	}
	s.inStack[pkgID] = remaining
}

func (s *materializeState) pathWith(pkgID string) string {
	path := append([]string{}, s.stack...)
	path = append(path, pkgID)
	return strings.Join(path, " -> ")
}

func materializePkg(req Request, out *Result, pkgs map[string]lockfile.Package, edgesByFrom map[string][]lockfile.Edge, pkg lockfile.Package, parentNodeModules string, state *materializeState, observer *materializeObserver) {
	dest, err := safePackagePath(parentNodeModules, materializedPackageName(pkg))
	if err != nil {
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_INVALID_DESTINATION", Severity: diag.SeverityError, Message: err.Error(), Details: []string{pkg.ID}})
		return
	}
	key := pkg.ID + "@" + dest
	if _, ok := state.seenLocations[key]; ok {
		return
	}
	state.seenLocations[key] = struct{}{}
	if state.progress != nil && state.progressTotal > 0 {
		state.progressIndex++
		state.progress(state.progressIndex, state.progressTotal, pkg)
	}
	cycleLeaf := state.containsPackage(pkg.ID)
	if !cycleLeaf && len(state.stack) >= maxMaterializeDependencyDepth {
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{
			Code:     "TSPACK_MATERIALIZE_PATH_DEPTH_EXCEEDED",
			Severity: diag.SeverityError,
			File:     dest,
			Message:  "materialization dependency path exceeded safety depth",
			Details: []string{
				pkg.ID,
				fmt.Sprintf("depth=%d", len(state.stack)+1),
				fmt.Sprintf("max=%d", maxMaterializeDependencyDepth),
				state.pathWith(pkg.ID),
				"hint: likely dependency cycle or materializer traversal bug",
			},
		})
		return
	}

	hash, ok := PackageStoreHash(pkg)
	if !ok {
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_PACKAGE_HASH_MISSING", Severity: diag.SeverityError, Message: "package missing store hash", Details: []string{pkg.ID}})
		return
	}
	if d := req.Store.Verify(hash); len(d) > 0 {
		code := "TSPACK_MATERIALIZE_STORE_VERIFY_FAILED"
		if d[0].Code == "TSPACK_STORE_ARTIFACT_NOT_FOUND" {
			code = "TSPACK_MATERIALIZE_MISSING_STORE_ARTIFACT"
		}
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: code, Severity: diag.SeverityError, Message: "store artifact verification failed", Details: []string{pkg.ID, hash, d[0].Code}})
		return
	}
	ref, d := req.Store.Get(hash)
	if len(d) > 0 {
		out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_MISSING_STORE_ARTIFACT", Severity: diag.SeverityError, Message: "missing store artifact", Details: []string{pkg.ID, hash}})
		return
	}
	if err := copyTree(ref.ExtractedPath, dest, req.Options.LinkMode, observer); err != nil {
		out.Diagnostics = append(out.Diagnostics, materializeDiagnosticFromError(err, pkg.ID, dest, pkg.Name))
		return
	}
	out.Written = append(out.Written, WrittenPath{Path: dest, Kind: "package", PackageID: pkg.ID})
	observer.RecordMaterializedPackage(pkg)

	if cycleLeaf {
		return
	}

	state.push(pkg.ID)
	defer state.pop(pkg.ID)

	childNM := filepath.Join(dest, "node_modules")
	for _, edge := range edgesByFrom[pkg.ID] {
		dep, ok := pkgs[edge.To]
		if !ok {
			out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_EDGE_UNKNOWN_PACKAGE", Severity: diag.SeverityError, Message: "edge points to unknown package", Details: []string{edge.From, edge.To}})
			continue
		}
		materializePkg(req, out, pkgs, edgesByFrom, dep, childNM, state, observer)
	}
}

func PackageStoreHash(pkg lockfile.Package) (string, bool) {
	if pkg.Hash != "" {
		return pkg.Hash, true
	}
	if pkg.TreeHash != "" {
		return pkg.TreeHash, true
	}
	return "", false
}

func buildMaterializePlan(rootEdges []lockfile.Edge, pkgs map[string]lockfile.Package, edgesByFrom map[string][]lockfile.Edge, nmRoot string) []lockfile.Package {
	state := newMaterializeState()
	plan := []lockfile.Package{}
	for _, edge := range rootEdges {
		pkg, ok := pkgs[edge.To]
		if !ok {
			continue
		}
		appendMaterializePlan(&plan, pkgs, edgesByFrom, pkg, nmRoot, state)
	}
	return plan
}

func appendMaterializePlan(plan *[]lockfile.Package, pkgs map[string]lockfile.Package, edgesByFrom map[string][]lockfile.Edge, pkg lockfile.Package, parentNodeModules string, state *materializeState) {
	dest, err := safePackagePath(parentNodeModules, materializedPackageName(pkg))
	if err != nil {
		return
	}
	key := pkg.ID + "@" + dest
	if _, ok := state.seenLocations[key]; ok {
		return
	}
	state.seenLocations[key] = struct{}{}
	*plan = append(*plan, pkg)
	cycleLeaf := state.containsPackage(pkg.ID)
	if cycleLeaf || len(state.stack) >= maxMaterializeDependencyDepth {
		return
	}

	state.push(pkg.ID)
	defer state.pop(pkg.ID)

	childNM := filepath.Join(dest, "node_modules")
	for _, edge := range edgesByFrom[pkg.ID] {
		dep, ok := pkgs[edge.To]
		if !ok {
			continue
		}
		appendMaterializePlan(plan, pkgs, edgesByFrom, dep, childNM, state)
	}
}

func safePackagePath(base, name string) (string, error) {
	if name == "" || strings.HasPrefix(name, ".") || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid package name %q", name)
	}
	parts := strings.Split(name, "/")
	if len(parts) > 2 || (strings.HasPrefix(name, "@") && len(parts) != 2) {
		return "", fmt.Errorf("invalid package name %q", name)
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." || strings.ContainsRune(p, filepath.Separator) {
			return "", fmt.Errorf("invalid package name %q", name)
		}
	}
	dest := filepath.Join(append([]string{base}, parts...)...)
	cleanBase := filepath.Clean(base)
	cleanDest := filepath.Clean(dest)
	if !strings.HasPrefix(cleanDest, cleanBase+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid destination for %q", name)
	}
	return dest, nil
}

func cleanNodeModules(nmRoot string) error {
	if _, err := os.Stat(nmRoot); os.IsNotExist(err) {
		return nil
	}
	marker := filepath.Join(nmRoot, markerFile)
	if _, err := os.Stat(marker); err != nil {
		return fmt.Errorf("refusing to clean unmanaged node_modules (missing %s)", markerFile)
	}
	return removeMaterializedPath(nmRoot)
}

func copyTree(src, dest string, linkMode LinkMode, stats StatsObserver) error {
	if linkMode == "" || linkMode == LinkModeAuto {
		linkMode = LinkModeHardlink
	}
	return replaceMaterializedDirectory(dest, func(stage string) error {
		return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			out := filepath.Join(stage, rel)
			if info.IsDir() {
				mode := info.Mode().Perm()
				if mode == 0 {
					mode = 0o755
				}
				if stats != nil {
					stats.RecordMaterializedDirectory(out)
				}
				return retryMaterializeFileOp("mkdir", out, func() error {
					return os.MkdirAll(out, mode)
				})
			}
			if err := retryMaterializeFileOp("mkdir", filepath.Dir(out), func() error {
				return os.MkdirAll(filepath.Dir(out), 0o755)
			}); err != nil {
				return err
			}
			if stats != nil {
				stats.RecordMaterializedFile(out, info.Size())
			}
			return materializeFile(path, out, info, linkMode, stats)
		})
	})
}

func materializeFile(src string, dest string, info fs.FileInfo, linkMode LinkMode, stats StatsObserver) error {
	if linkMode == "" || linkMode == LinkModeAuto {
		linkMode = LinkModeHardlink
	}
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	if info.Mode().IsRegular() && linkMode == LinkModeHardlink {
		if err := retryMaterializeFileOp("link", dest, func() error {
			return materializeLink(src, dest)
		}); err == nil {
			if stats != nil {
				stats.RecordHardlink(dest, info.Size())
			}
			return nil
		}
	}
	if stats != nil {
		stats.RecordCopy(dest, info.Size())
	}
	return copyMaterializedFile(src, dest, mode)
}

func copyMaterializedFile(src string, dest string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	f, err := openMaterializedFileForWrite(dest, mode)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, in); err != nil {
		return err
	}
	return retryMaterializeFileOp("chmod", dest, func() error {
		return os.Chmod(dest, mode)
	})
}

func replaceMaterializedDirectory(dest string, populate func(stage string) error) error {
	parent := filepath.Dir(dest)
	if err := retryMaterializeFileOp("mkdir", parent, func() error {
		return os.MkdirAll(parent, 0o755)
	}); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, filepath.Base(dest)+".*.tmp")
	if err != nil {
		return err
	}
	keepStage := true
	defer func() {
		if keepStage {
			_ = os.RemoveAll(stage)
		}
	}()
	if err := populate(stage); err != nil {
		return err
	}
	if err := removeMaterializedPath(dest); err != nil {
		return err
	}
	if err := retryMaterializeFileOp("rename", dest, func() error {
		return os.Rename(stage, dest)
	}); err != nil {
		return err
	}
	keepStage = false
	return nil
}

func removeMaterializedPath(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return retryMaterializeFileOp("remove", path, func() error {
		return os.RemoveAll(path)
	})
}

func openMaterializedFileForWrite(path string, mode os.FileMode) (*os.File, error) {
	var out *os.File
	err := retryMaterializeFileOp("write", path, func() error {
		f, openErr := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if openErr != nil {
			return openErr
		}
		out = f
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func retryMaterializeFileOp(op string, path string, fn func() error) error {
	err := fn()
	if err == nil || !materializeFileLockRetriesEnabled || !isTransientMaterializeLockErr(err) {
		return err
	}
	backoff := []time.Duration{
		50 * time.Millisecond,
		100 * time.Millisecond,
		150 * time.Millisecond,
		200 * time.Millisecond,
		250 * time.Millisecond,
		300 * time.Millisecond,
		400 * time.Millisecond,
		500 * time.Millisecond,
	}
	lastErr := err
	for _, delay := range backoff {
		time.Sleep(delay)
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if !isTransientMaterializeLockErr(lastErr) {
			return lastErr
		}
	}
	return &materializeFileLockError{
		Op:       op,
		Path:     path,
		Attempts: len(backoff) + 1,
		Err:      lastErr,
	}
}

func materializeDiagnosticFromError(err error, packageID string, file string, packageName string) diag.Diagnostic {
	var locked *materializeFileLockError
	if errors.As(err, &locked) {
		details := []string{
			"operation=" + locked.Op,
			"path=" + locked.Path,
			fmt.Sprintf("attempts=%d", locked.Attempts),
			locked.Err.Error(),
		}
		if packageID != "" {
			details = append(details, "package="+packageID)
		}
		if packageName != "" {
			details = append(details, "packageName="+packageName)
		}
		for _, owner := range materializeLockOwners(locked.Path) {
			details = append(details, formatMaterializeLockOwner(owner))
		}
		return diag.Diagnostic{
			Code:     "TSPACK_MATERIALIZE_FILE_LOCKED",
			Severity: diag.SeverityError,
			File:     locked.Path,
			Message:  "materialization could not replace a locked file after bounded retries",
			Details:  details,
			Fixes: []string{
				"Stop `tspack run dev` or other dev servers using this workspace.",
				"Close processes using node, esbuild, or vite for this project, then rerun `tspack sync`.",
				"Use `tspack sync --clean` after stopping file holders when the materialized tree is stale.",
				"Editors, extension hosts, or antivirus may hold executables briefly; wait a moment and retry if the lock should be transient.",
			},
		}
	}
	if strings.HasPrefix(err.Error(), "refusing to clean unmanaged node_modules") {
		return diag.Diagnostic{Code: "TSPACK_MATERIALIZE_CLEAN_REFUSED", Severity: diag.SeverityError, File: file, Message: err.Error()}
	}
	details := []string{}
	if packageID != "" {
		details = append(details, packageID)
	}
	if file != "" {
		details = append(details, file)
	}
	return diag.Diagnostic{Code: "TSPACK_MATERIALIZE_WRITE_FAILED", Severity: diag.SeverityError, File: file, Message: err.Error(), Details: details}
}

func materializeRootBins(req Request, out *Result, nmRoot string, pkgs map[string]lockfile.Package) {
	binsRoot := filepath.Join(nmRoot, ".bin")
	type candidate struct{ pkgName, absPath, relPath string }
	candidates := map[string]candidate{}
	ids := make([]string, 0, len(pkgs))
	for id := range pkgs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		pkg := pkgs[id]
		pkgRoot, err := safePackagePath(nmRoot, materializedPackageName(pkg))
		if err != nil {
			continue
		}
		defs, diags := parsePackageBins(pkgRoot)
		if len(diags) > 0 {
			out.Diagnostics = append(out.Diagnostics, diags...)
			continue
		}
		sort.SliceStable(defs, func(i, j int) bool { return defs[i].name < defs[j].name })
		for _, def := range defs {
			target := filepath.Join(pkgRoot, filepath.FromSlash(def.relPath))
			if _, err := os.Stat(target); err != nil {
				out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_BIN_TARGET_MISSING", Severity: diag.SeverityError, File: filepath.Join(pkgRoot, "package.json"), Message: "bin target does not exist", Details: []string{def.name, def.relPath}})
				continue
			}
			if prev, ok := candidates[def.name]; ok && prev.absPath != target {
				out.Diagnostics = append(out.Diagnostics, diag.Diagnostic{Code: "TSPACK_MATERIALIZE_BIN_CONFLICT", Severity: diag.SeverityError, Message: "multiple packages expose the same bin", Details: []string{def.name, prev.pkgName, pkg.Name}})
				continue
			}
			candidates[def.name] = candidate{pkgName: materializedPackageName(pkg), absPath: target, relPath: def.relPath}
		}
	}
	names := make([]string, 0, len(candidates))
	for name := range candidates {
		names = append(names, name)
	}
	sort.Strings(names)
	if err := replaceMaterializedDirectory(binsRoot, func(stage string) error {
		for _, name := range names {
			cand := candidates[name]
			targetRel := filepath.ToSlash(filepath.Join("..", cand.pkgName, filepath.FromSlash(cand.relPath)))
			binPath := filepath.Join(stage, name)
			if runtime.GOOS == "windows" {
				content := "@ECHO off\r\nnode \"%~dp0\\" + strings.ReplaceAll(targetRel, "/", "\\") + "\" %*\r\n"
				if err := retryMaterializeFileOp("write", binPath+".cmd", func() error {
					return os.WriteFile(binPath+".cmd", []byte(content), 0o644)
				}); err != nil {
					return err
				}
				continue
			}
			if err := os.Symlink(targetRel, binPath); err != nil {
				content := "#!/usr/bin/env sh\nDIR=\"$(CDPATH= cd -- \"$(dirname -- \"$0\")\" && pwd)\"\nexec \"$DIR/" + targetRel + "\" \"$@\"\n"
				if writeErr := retryMaterializeFileOp("write", binPath, func() error {
					return os.WriteFile(binPath, []byte(content), 0o755)
				}); writeErr != nil {
					return writeErr
				}
			}
			_ = retryMaterializeFileOp("chmod", cand.absPath, func() error {
				return os.Chmod(cand.absPath, 0o755)
			})
		}
		return nil
	}); err != nil {
		out.Diagnostics = append(out.Diagnostics, materializeDiagnosticFromError(err, "", binsRoot, ""))
	}
}

func materializedPackageName(pkg lockfile.Package) string {
	if pkg.Source != "jsr" || !strings.HasPrefix(pkg.Name, "@") {
		return pkg.Name
	}
	parts := strings.Split(strings.TrimPrefix(pkg.Name, "@"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return pkg.Name
	}
	return "@jsr/" + parts[0] + "__" + parts[1]
}

type binDef struct{ name, relPath string }

func parsePackageBins(pkgRoot string) ([]binDef, []diag.Diagnostic) {
	p := filepath.Join(pkgRoot, "package.json")
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, nil
	}
	var pkg packageJSON
	if err := json.Unmarshal(b, &pkg); err != nil {
		return nil, nil
	}
	switch v := pkg.Bin.(type) {
	case string:
		if pkg.Name == "" {
			return nil, []diag.Diagnostic{{Code: "TSPACK_MATERIALIZE_BIN_INVALID", Severity: diag.SeverityError, File: p, Message: "package bin string requires package name"}}
		}
		if err := validateBinPath(v); err != nil {
			return nil, []diag.Diagnostic{{Code: "TSPACK_MATERIALIZE_BIN_INVALID", Severity: diag.SeverityError, File: p, Message: err.Error(), Details: []string{pkg.Name, v}}}
		}
		return []binDef{{name: pkg.Name, relPath: v}}, nil
	case map[string]interface{}:
		out := []binDef{}
		for name, raw := range v {
			pathStr, ok := raw.(string)
			if !ok {
				return nil, []diag.Diagnostic{{Code: "TSPACK_MATERIALIZE_BIN_INVALID", Severity: diag.SeverityError, File: p, Message: "bin map entry must be a string", Details: []string{name}}}
			}
			if err := validateBinPath(pathStr); err != nil {
				return nil, []diag.Diagnostic{{Code: "TSPACK_MATERIALIZE_BIN_INVALID", Severity: diag.SeverityError, File: p, Message: err.Error(), Details: []string{name, pathStr}}}
			}
			out = append(out, binDef{name: name, relPath: pathStr})
		}
		return out, nil
	default:
		return nil, nil
	}
}

func validateBinPath(relPath string) error {
	if relPath == "" || filepath.IsAbs(relPath) {
		return fmt.Errorf("bin path must be relative")
	}
	clean := filepath.Clean(relPath)
	if clean == "." || strings.HasPrefix(clean, "..") || strings.Contains(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("bin path must stay inside package root")
	}
	return nil
}

func finalize(out Result) Result {
	sort.SliceStable(out.Written, func(i, j int) bool {
		if out.Written[i].Path != out.Written[j].Path {
			return out.Written[i].Path < out.Written[j].Path
		}
		return out.Written[i].PackageID < out.Written[j].PackageID
	})
	diag.SortDiagnostics(out.Diagnostics)
	return out
}
