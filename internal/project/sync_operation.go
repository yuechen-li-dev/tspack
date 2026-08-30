package project

import (
	"context"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/materialize"
	"github.com/yuechen-li-dev/tspack/internal/resolver"
	"github.com/yuechen-li-dev/tspack/internal/store"
	"os"
	"path/filepath"
)

func Sync(opts Options, clean bool, force bool) Result {
	perfSession, perfErr := ensurePerfSession(&opts, "sync", false)
	if perfErr != nil {
		return Result{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_PERF_PROFILE_INIT_FAILED", "failed to initialize performance profiling", perfErr.Error())}}
	}
	if perfSession != nil {
		defer func() {
			_ = perfSession.Close()
		}()
	}
	ir, g, out := loadManifestAndGraph(opts)
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
	out = append(out, sourcePolicyLockDiagnostics(ir, lf)...)
	out = append(out, validateLockedPatches(opts.RootDir, g, lf)...)
	if hasErrors(out) {
		return Result{Diagnostics: out}
	}
	opts.ResolverClient = instrumentRegistryClient(opts.ResolverClient, perfSession, resolver.SourceNPM)
	if opts.ResolverBackends == nil {
		if opts.ResolverClient != nil && !hasDeclaredSourcePolicy(ir) {
			opts.ResolverBackends = resolver.BackendRegistry{
				resolver.SourceNPM: resolver.NewNPMBackend(opts.ResolverClient),
				resolver.SourceJSR: resolver.NewJSRBackend(instrumentRegistryClient(resolver.NewHTTPRegistryClient(resolver.DefaultJSREndpoint), perfSession, resolver.SourceJSR)),
			}
		} else {
			var backendErr error
			opts.ResolverBackends, backendErr = sourcePolicyBackends(resolverSourcePolicy(ir), perfSession)
			if backendErr != nil {
				return Result{Diagnostics: append(out, errDiag("TSPACK_SOURCE_POLICY_INVALID", "invalid registry source policy", backendErr.Error()))}
			}
		}
	}
	st, err := store.Open(opts.StoreRoot)
	if err != nil {
		return Result{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_SYNC_STORE_ARTIFACT_MISSING", "failed to open store", err.Error())}}
	}
	stopHydrate := perfSession.StartPhase("sync.hydrate_store")
	out = append(out, ensureStoreArtifactsForLock(context.Background(), opts, st, lf, resolverSourcePolicy(ir))...)
	stopHydrate()
	if hasErrors(out) {
		diag.SortDiagnostics(out)
		return Result{Diagnostics: out}
	}
	mat := materialize.NodeModulesMaterializer{}
	materializeOptions := materialize.Options{Clean: clean, Force: force, Stats: materializeStatsObserver(perfSession)}
	if shouldReportMaterializeProgress(opts.RootDir, clean) {
		materializeOptions.OnPackage = func(index int, total int, pkg lockfile.Package) {
			opts.Progress.Step("materializing packages [%d/%d] %s", index, total, packageProgressLabel(pkg))
		}
	}
	stopMaterialize := perfSession.StartPhase("sync.materialize")
	mr := mat.Materialize(context.Background(), materialize.Request{WorkspaceRoot: opts.RootDir, Graph: g, Lock: lf, Store: st, Options: materializeOptions})
	stopMaterialize()
	out = append(out, mr.Diagnostics...)
	diag.SortDiagnostics(out)
	return Result{Diagnostics: out}
}

func shouldReportMaterializeProgress(rootDir string, clean bool) bool {
	if clean {
		return true
	}
	markerPath := filepath.Join(rootDir, "node_modules", ".tspack-materialized")
	_, err := os.Stat(markerPath)
	return os.IsNotExist(err)
}
