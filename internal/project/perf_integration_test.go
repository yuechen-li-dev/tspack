package project

import (
	"testing"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/perf"
	"github.com/yuechen-li-dev/tspack/internal/resolver"
)

func TestUpdatePerfCapturesResolveAndStoreCounters(t *testing.T) {
	root := t.TempDir()
	irPath := writeIR(t, root, map[string]any{
		"format":    1,
		"workspace": map[string]any{"name": "ws"},
		"packages": []map[string]any{{
			"name":    "app",
			"version": "1.0.0",
			"kind":    "library",
			"dependencies": []map[string]any{
				{"key": "dep-a", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "dep-a", "range": "1.0.0"}},
				{"key": "left-pad", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "left-pad", "range": "1.0.0"}},
			},
			"targets": []map[string]any{{
				"name":    "core",
				"export":  ".",
				"entry":   "src/index.ts",
				"runtime": "src/index.ts",
				"types":   "dist/index.d.ts",
				"deps":    []string{"dep-a", "left-pad"},
				"peers":   []string{},
			}},
			"tools":      []string{},
			"boundaries": []any{},
			"publish":    map[string]any{"include": []string{"dist/**"}, "exclude": []string{}},
			"policies":   map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}},
		}},
	})

	registry := newFakeRegistryServer(t)
	defer registry.Close()

	session, err := perf.NewSession("update", root, false, perf.EnvConfig{Enabled: true}, nil)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	opts.ResolverClient = resolver.NewHTTPRegistryClient(registry.URL)
	opts.Perf = session

	result := Update(opts)
	if hasErrors(result.Diagnostics) {
		t.Fatalf("update failed: %#v", result.Diagnostics)
	}
	report := session.Snapshot(time.Now().UTC())
	if report.Counters.ResolveJobs != 24 {
		t.Fatalf("resolve jobs=%d want 24", report.Counters.ResolveJobs)
	}
	if report.Counters.ResolveFrontiers == 0 {
		t.Fatalf("expected resolve frontiers, got %#v", report.Counters)
	}
	if report.Counters.ResolveMaxFrontierWidth == 0 {
		t.Fatalf("expected resolve max frontier width, got %#v", report.Counters)
	}
	if report.Counters.ResolvePreparedPackages != 2 {
		t.Fatalf("prepared packages=%d want 2", report.Counters.ResolvePreparedPackages)
	}
	if report.Counters.ResolveCommittedPackages != 2 {
		t.Fatalf("committed packages=%d want 2", report.Counters.ResolveCommittedPackages)
	}
	if report.Counters.ResolveWorkerErrors != 0 {
		t.Fatalf("resolve worker errors=%d want 0", report.Counters.ResolveWorkerErrors)
	}
	if report.Counters.MetadataRequests != 2 {
		t.Fatalf("metadata requests=%d want 2", report.Counters.MetadataRequests)
	}
	if report.Counters.MetadataCacheHits != 1 {
		t.Fatalf("metadata cache hits=%d want 1", report.Counters.MetadataCacheHits)
	}
	if report.Counters.TarballRequests != 2 {
		t.Fatalf("tarball requests=%d want 2", report.Counters.TarballRequests)
	}
	if report.Counters.ArtifactsCaptured != 2 {
		t.Fatalf("artifacts captured=%d want 2", report.Counters.ArtifactsCaptured)
	}
	if report.Counters.ArtifactsNeedingStorePopulation != 0 {
		t.Fatalf("artifacts needing store population=%d want 0", report.Counters.ArtifactsNeedingStorePopulation)
	}
	if report.Counters.StorePopulationSkipped != 2 {
		t.Fatalf("store population skipped=%d want 2", report.Counters.StorePopulationSkipped)
	}
	for _, phase := range []string{"update.resolve", "update.store_population", "update.lockfile_write"} {
		if !phasePresent(report, phase) {
			t.Fatalf("missing phase %q in %#v", phase, report.Phases)
		}
	}
}

func TestSyncPerfCapturesHydrationAndMaterializationCounters(t *testing.T) {
	root := t.TempDir()
	irPath := writeIR(t, root, map[string]any{
		"format":    1,
		"workspace": map[string]any{"name": "ws"},
		"packages": []map[string]any{{
			"name":    "app",
			"version": "1.0.0",
			"kind":    "library",
			"dependencies": []map[string]any{
				{"key": "dep-a", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "dep-a", "range": "1.0.0"}},
			},
			"targets": []map[string]any{{
				"name":    "core",
				"export":  ".",
				"entry":   "src/index.ts",
				"runtime": "src/index.ts",
				"types":   "dist/index.d.ts",
				"deps":    []string{"dep-a"},
				"peers":   []string{},
			}},
			"tools":      []string{},
			"boundaries": []any{},
			"publish":    map[string]any{"include": []string{"dist/**"}, "exclude": []string{}},
			"policies":   map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}},
		}},
	})
	registry := newFakeRegistryServer(t)
	defer registry.Close()

	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	opts.ResolverClient = resolver.NewHTTPRegistryClient(registry.URL)
	up := Update(opts)
	if hasErrors(up.Diagnostics) {
		t.Fatalf("update failed: %#v", up.Diagnostics)
	}

	session, err := perf.NewSession("sync", root, false, perf.EnvConfig{Enabled: true}, nil)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	opts.Perf = session
	syncResult := Sync(opts, false)
	if hasErrors(syncResult.Diagnostics) {
		t.Fatalf("sync failed: %#v", syncResult.Diagnostics)
	}
	report := session.Snapshot(time.Now().UTC())
	if report.Counters.SyncHydrationSkipped == 0 {
		t.Fatalf("expected sync hydration skips, got %#v", report.Counters)
	}
	if report.Counters.MaterializedPackages == 0 || report.Counters.MaterializedFiles == 0 {
		t.Fatalf("expected materialization counters, got %#v", report.Counters)
	}
	if report.Counters.HardlinkCount == 0 && report.Counters.CopyFallbackCount == 0 {
		t.Fatalf("expected hardlink or copy accounting, got %#v", report.Counters)
	}
	for _, phase := range []string{"sync.hydrate_store", "sync.materialize"} {
		if !phasePresent(report, phase) {
			t.Fatalf("missing phase %q in %#v", phase, report.Phases)
		}
	}
}

func phasePresent(report perf.Report, name string) bool {
	for _, phase := range report.Phases {
		if phase.Name == name {
			return true
		}
	}
	return false
}
