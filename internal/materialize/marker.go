package materialize

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/lockfile"
)

const materializationMarkerSchemaVersion = 2

type markerReadStatus string

const (
	markerReadHit      markerReadStatus = "hit"
	markerReadMiss     markerReadStatus = "miss"
	markerReadMismatch markerReadStatus = "mismatch"
	markerReadCorrupt  markerReadStatus = "corrupt"
)

type materializationMarker struct {
	SchemaVersion  int    `json:"schemaVersion"`
	Materializer   string `json:"materializer"`
	Platform       string `json:"platform"`
	LinkMode       string `json:"linkMode"`
	PlanDigest     string `json:"planDigest"`
	PackageCount   int    `json:"packageCount"`
	FileCount      int    `json:"fileCount"`
	DirectoryCount int    `json:"directoryCount"`
	GeneratedAt    string `json:"generatedAt,omitempty"`
}

type materializationPlan struct {
	mode        LinkMode
	platform    string
	digest      string
	rootVisible []lockfile.Package
	entries     []materializationPlanEntry
}

type materializationPlanEntry struct {
	PackageID   string
	PackageName string
	PackageHash string
	Destination string
}

func buildMaterializationPlan(lf *lockfile.Lockfile, nmRoot string, mode LinkMode) materializationPlan {
	pkgs := map[string]lockfile.Package{}
	for _, pkg := range lf.Packages {
		pkgs[pkg.ID] = pkg
	}

	edgesByFrom := map[string][]lockfile.Edge{}
	for _, edge := range lf.Edges {
		edgesByFrom[edge.From] = append(edgesByFrom[edge.From], edge)
	}
	for key := range edgesByFrom {
		sort.SliceStable(edgesByFrom[key], func(i, j int) bool {
			if edgesByFrom[key][i].To != edgesByFrom[key][j].To {
				return edgesByFrom[key][i].To < edgesByFrom[key][j].To
			}
			return edgesByFrom[key][i].Kind < edgesByFrom[key][j].Kind
		})
	}

	rootEdges := collectRootEdges(lf.Edges)
	rootVisible := make([]lockfile.Package, 0, len(rootEdges))
	for _, edge := range rootEdges {
		pkg, ok := pkgs[edge.To]
		if !ok {
			continue
		}
		rootVisible = append(rootVisible, pkg)
	}
	sort.SliceStable(rootVisible, func(i, j int) bool {
		if rootVisible[i].Name != rootVisible[j].Name {
			return rootVisible[i].Name < rootVisible[j].Name
		}
		return rootVisible[i].ID < rootVisible[j].ID
	})

	state := newMaterializeState()
	entries := make([]materializationPlanEntry, 0)
	for _, edge := range rootEdges {
		pkg, ok := pkgs[edge.To]
		if !ok {
			continue
		}
		appendMaterializationPlanEntries(&entries, pkgs, edgesByFrom, pkg, nmRoot, state)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Destination != entries[j].Destination {
			return entries[i].Destination < entries[j].Destination
		}
		if entries[i].PackageID != entries[j].PackageID {
			return entries[i].PackageID < entries[j].PackageID
		}
		return entries[i].PackageHash < entries[j].PackageHash
	})

	return materializationPlan{
		mode:        mode,
		platform:    runtime.GOOS,
		digest:      computeMaterializationPlanDigest(entries, mode, runtime.GOOS),
		rootVisible: rootVisible,
		entries:     entries,
	}
}

func appendMaterializationPlanEntries(entries *[]materializationPlanEntry, pkgs map[string]lockfile.Package, edgesByFrom map[string][]lockfile.Edge, pkg lockfile.Package, parentNodeModules string, state *materializeState) {
	dest, err := safePackagePath(parentNodeModules, pkg.Name)
	if err != nil {
		return
	}
	key := pkg.ID + "@" + dest
	if _, ok := state.seenLocations[key]; ok {
		return
	}
	state.seenLocations[key] = struct{}{}

	hash, _ := PackageStoreHash(pkg)
	*entries = append(*entries, materializationPlanEntry{
		PackageID:   pkg.ID,
		PackageName: pkg.Name,
		PackageHash: hash,
		Destination: filepath.ToSlash(dest),
	})

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
		appendMaterializationPlanEntries(entries, pkgs, edgesByFrom, dep, childNM, state)
	}
}

func computeMaterializationPlanDigest(entries []materializationPlanEntry, mode LinkMode, platform string) string {
	sum := sha256.New()
	_, _ = sum.Write([]byte("schema=1\n"))
	_, _ = sum.Write([]byte("platform=" + normalizeDigestText(platform) + "\n"))
	_, _ = sum.Write([]byte("mode=" + normalizeDigestText(string(mode)) + "\n"))
	for _, entry := range entries {
		line := strings.Join([]string{
			"pkg",
			normalizeDigestText(entry.PackageID),
			normalizeDigestText(entry.PackageName),
			normalizeDigestText(entry.PackageHash),
			normalizeDigestText(entry.Destination),
		}, "|")
		_, _ = sum.Write([]byte(line + "\n"))
	}
	return hex.EncodeToString(sum.Sum(nil))
}

func normalizeDigestText(value string) string {
	if value == "" {
		return "-"
	}
	return strings.ReplaceAll(value, "\\", "/")
}

func materializationMarkerPath(nmRoot string) string {
	return filepath.Join(nmRoot, markerFile)
}

func loadMaterializationMarker(nmRoot string, plan materializationPlan) (materializationMarker, markerReadStatus) {
	path := materializationMarkerPath(nmRoot)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return materializationMarker{}, markerReadMiss
	}
	if err != nil {
		return materializationMarker{}, markerReadCorrupt
	}

	var marker materializationMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return materializationMarker{}, markerReadCorrupt
	}
	if marker.SchemaVersion == 0 || marker.Materializer == "" || marker.LinkMode == "" || marker.PlanDigest == "" {
		return materializationMarker{}, markerReadCorrupt
	}
	if marker.SchemaVersion != materializationMarkerSchemaVersion {
		return marker, markerReadMismatch
	}
	if marker.Materializer != "node_modules" || marker.Platform != plan.platform || marker.LinkMode != string(plan.mode) || marker.PlanDigest != plan.digest {
		return marker, markerReadMismatch
	}
	return marker, markerReadHit
}

func markerSanityCheck(nmRoot string, plan materializationPlan) bool {
	info, err := os.Stat(nmRoot)
	if err != nil || !info.IsDir() {
		return false
	}

	binsInfo, err := os.Stat(filepath.Join(nmRoot, ".bin"))
	if err != nil || !binsInfo.IsDir() {
		return false
	}

	for _, pkg := range plan.rootVisible {
		dest, err := safePackagePath(nmRoot, pkg.Name)
		if err != nil {
			return false
		}
		pkgInfo, err := os.Stat(dest)
		if err != nil || !pkgInfo.IsDir() {
			return false
		}
	}

	return true
}

func writeMaterializationMarker(nmRoot string, marker materializationMarker) error {
	path := materializationMarkerPath(nmRoot)
	body, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')

	tempPath := path + ".tmp"
	if err := retryMaterializeFileOp("write", tempPath, func() error {
		return os.WriteFile(tempPath, body, 0o644)
	}); err != nil {
		return err
	}
	if err := retryMaterializeFileOp("rename", path, func() error {
		return os.Rename(tempPath, path)
	}); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func expectedMaterializationMarker(plan materializationPlan, observer *materializeObserver) materializationMarker {
	return materializationMarker{
		SchemaVersion:  materializationMarkerSchemaVersion,
		Materializer:   "node_modules",
		Platform:       plan.platform,
		LinkMode:       string(plan.mode),
		PlanDigest:     plan.digest,
		PackageCount:   observer.packageCount,
		FileCount:      observer.fileCount,
		DirectoryCount: observer.directoryCount,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func pruneNodeModulesRoot(nmRoot string, rootVisible []lockfile.Package) error {
	keepRootFiles := map[string]struct{}{
		".bin":     {},
		markerFile: {},
	}
	keepPackages := map[string]map[string]struct{}{}
	for _, pkg := range rootVisible {
		if strings.HasPrefix(pkg.Name, "@") {
			parts := strings.Split(pkg.Name, "/")
			if len(parts) != 2 {
				continue
			}
			scopePackages := keepPackages[parts[0]]
			if scopePackages == nil {
				scopePackages = map[string]struct{}{}
				keepPackages[parts[0]] = scopePackages
			}
			scopePackages[parts[1]] = struct{}{}
			continue
		}
		keepPackages[pkg.Name] = nil
	}

	entries, err := os.ReadDir(nmRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if _, ok := keepRootFiles[name]; ok {
			continue
		}

		expectedScopePackages, isScope := keepPackages[name]
		if !isScope {
			if err := removeMaterializedPath(filepath.Join(nmRoot, name)); err != nil {
				return err
			}
			continue
		}

		if expectedScopePackages == nil {
			continue
		}
		scopeRoot := filepath.Join(nmRoot, name)
		scopeEntries, err := os.ReadDir(scopeRoot)
		if err != nil {
			return err
		}
		for _, scopeEntry := range scopeEntries {
			if _, ok := expectedScopePackages[scopeEntry.Name()]; ok {
				continue
			}
			if err := removeMaterializedPath(filepath.Join(scopeRoot, scopeEntry.Name())); err != nil {
				return err
			}
		}
		remaining, err := os.ReadDir(scopeRoot)
		if err != nil {
			return err
		}
		if len(remaining) == 0 {
			if err := removeMaterializedPath(scopeRoot); err != nil {
				return err
			}
		}
	}
	return nil
}

func planDigestForLock(lf *lockfile.Lockfile, mode LinkMode) string {
	return buildMaterializationPlan(lf, "node_modules", mode).digest
}

func materializationMarkerForRoot(root string) string {
	return materializationMarkerPath(filepath.Join(root, "node_modules"))
}

func markerWriteError(path string, err error) error {
	return fmt.Errorf("write materialization marker %s: %w", path, err)
}
