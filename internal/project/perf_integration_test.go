package project

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/perf"
	"github.com/yuechen-li-dev/tspack/internal/resolver"
)

func TestRegistryPerfAttributesRequestsBySource(t *testing.T) {
	registry := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/@scope/pkg", "/%40scope%2Fpkg":
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"name":"@scope/pkg"}`))
		case "/pkg.tgz":
			_, _ = response.Write([]byte("artifact"))
		default:
			http.NotFound(response, request)
		}
	}))
	defer registry.Close()

	session, err := perf.NewSession("update", t.TempDir(), false, perf.EnvConfig{Enabled: true}, nil)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	client := instrumentRegistryClient(
		resolver.NewHTTPRegistryClient(registry.URL),
		session,
		resolver.SourceJSR,
	)
	if _, err := client.PackageMetadata(context.Background(), "@scope/pkg"); err != nil {
		t.Fatalf("PackageMetadata failed: %v", err)
	}
	if _, err := client.Tarball(context.Background(), registry.URL+"/pkg.tgz"); err != nil {
		t.Fatalf("Tarball failed: %v", err)
	}

	report := session.Snapshot(time.Now().UTC())
	if report.HTTP.RequestKinds["jsr.metadata"] != 1 {
		t.Fatalf("jsr metadata request count=%d want 1", report.HTTP.RequestKinds["jsr.metadata"])
	}
	if report.HTTP.RequestKinds["jsr.tarball"] != 1 {
		t.Fatalf("jsr tarball request count=%d want 1", report.HTTP.RequestKinds["jsr.tarball"])
	}
}

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
	if !report.Controller.Enabled {
		t.Fatalf("expected controller enabled, got %#v", report.Controller)
	}
	if report.Controller.Mode != "fixed" {
		t.Fatalf("controller mode=%q want fixed", report.Controller.Mode)
	}
	if len(report.Controller.Decisions) == 0 {
		t.Fatalf("expected controller decisions, got %#v", report.Controller)
	}
	if report.Controller.ClampReasons["frontier_width"] == 0 {
		t.Fatalf("expected clamp reasons, got %#v", report.Controller.ClampReasons)
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
	syncResult := Sync(opts, false, false)
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

func TestSyncPerfCapturesNoopMarkerCounters(t *testing.T) {
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
	first := Sync(opts, false, false)
	if hasErrors(first.Diagnostics) {
		t.Fatalf("first sync failed: %#v", first.Diagnostics)
	}

	session, err := perf.NewSession("sync", root, false, perf.EnvConfig{Enabled: true}, nil)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	opts.Perf = session
	second := Sync(opts, false, false)
	if hasErrors(second.Diagnostics) {
		t.Fatalf("second sync failed: %#v", second.Diagnostics)
	}

	report := session.Snapshot(time.Now().UTC())
	if !report.Counters.MaterializationNoop {
		t.Fatalf("expected noop marker counter, got %#v", report.Counters)
	}
	if report.Counters.MaterializedFiles != 0 || report.Counters.HardlinkCount != 0 || report.Counters.CopyFallbackCount != 0 {
		t.Fatalf("expected noop sync to avoid file relinking, got %#v", report.Counters)
	}
	if report.Counters.MaterializationMarkerHits == 0 {
		t.Fatalf("expected marker hit accounting, got %#v", report.Counters)
	}
}

func TestSyncPerfCapturesForcedRematerialization(t *testing.T) {
	root := t.TempDir()
	irPath := writeIR(t, root, progressNPMIR("dep-a", "dep-a", "1.0.0"))
	opts := DefaultOptions(root)
	opts.ManifestIRPath = irPath
	opts.ResolverClient = buildRegistry()

	up := Update(opts)
	if hasErrors(up.Diagnostics) {
		t.Fatalf("update failed: %#v", up.Diagnostics)
	}
	first := Sync(opts, false, false)
	if hasErrors(first.Diagnostics) {
		t.Fatalf("first sync failed: %#v", first.Diagnostics)
	}

	session, err := perf.NewSession("sync", root, false, perf.EnvConfig{Enabled: true}, nil)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	opts.Perf = session
	forced := Sync(opts, false, true)
	if hasErrors(forced.Diagnostics) {
		t.Fatalf("forced sync failed: %#v", forced.Diagnostics)
	}

	report := session.Snapshot(time.Now().UTC())
	if !report.Counters.MaterializationForced {
		t.Fatalf("expected forced materialization counter, got %#v", report.Counters)
	}
	if report.Counters.MaterializationNoop {
		t.Fatalf("force should bypass noop marker path, got %#v", report.Counters)
	}
	if report.Counters.MaterializedFiles == 0 {
		t.Fatalf("force should rematerialize files, got %#v", report.Counters)
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
