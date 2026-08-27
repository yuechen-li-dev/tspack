package project

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/resolver"
	"github.com/yuechen-li-dev/tspack/internal/store"
)

func ensureStoreArtifactsForLock(ctx context.Context, opts Options, st *store.Store, lf *lockfile.Lockfile, policy resolver.SourcePolicy) []diag.Diagnostic {
	if st == nil || lf == nil {
		return nil
	}

	missing := make([]lockfile.Package, 0)
	var out []diag.Diagnostic
	for _, pkg := range lf.Packages {
		hash, ok := lockPackageStoreHash(pkg)
		if !ok {
			out = append(out, syncLockArtifactIncompleteDiagnostic(pkg, "lockfile package is missing a store hash", "run `tspack update` with the current TSPack version to refresh ts-lock.toml"))
			continue
		}
		if len(st.Verify(hash)) == 0 {
			opts.Perf.RecordSyncHydrationSkip()
			continue
		}
		missing = append(missing, pkg)
	}
	for index, pkg := range missing {
		if policy.Offline {
			out = append(out, diag.Diagnostic{
				Code:     "TSPACK_REGISTRY_OFFLINE_MISS",
				Severity: diag.SeverityError,
				Message:  "offline source policy cannot hydrate a missing store artifact",
				Details:  []string{"package: " + pkg.ID, "Populate the store before enabling offline mode."},
			})
			continue
		}
		opts.Perf.RecordSyncHydrationFetch()
		opts.Progress.Step("%s [%d/%d] %s", storeFetchProgressLabel(pkg), index+1, len(missing), packageProgressLabel(pkg))
		out = append(out, hydrateMissingStoreArtifact(ctx, opts, st, pkg)...)
	}
	return out
}

func hydrateMissingStoreArtifact(ctx context.Context, opts Options, st *store.Store, pkg lockfile.Package) []diag.Diagnostic {
	switch pkg.Source {
	case "npm":
		backend, ok := opts.ResolverBackends.Backend(resolver.SourceNPM)
		if !ok {
			return []diag.Diagnostic{syncHydrateFailedDiagnostic(pkg, "npm source is denied or not configured by registry policy")}
		}
		return hydrateMissingRegistryStoreArtifact(ctx, st, backend, pkg)
	case "jsr":
		backend := resolver.NewJSRBackend(nil)
		if configured, ok := opts.ResolverBackends.Backend(resolver.SourceJSR); ok {
			backend = configured
		}
		return hydrateMissingRegistryStoreArtifact(ctx, st, backend, pkg)
	case "path", "workspace":
		return hydrateMissingLocalStoreArtifact(opts.RootDir, st, pkg)
	case "git":
		return []diag.Diagnostic{syncHydrateFailedDiagnostic(pkg, "sync cannot hydrate missing git store artifacts", "source: git", "recreate the store artifact with `tspack update`")}
	default:
		return []diag.Diagnostic{syncHydrateFailedDiagnostic(pkg, "sync cannot hydrate missing store artifact for unsupported source", "source: "+pkg.Source, "recreate the store artifact with `tspack update`")}
	}
}

func hydrateMissingRegistryStoreArtifact(ctx context.Context, st *store.Store, backend resolver.RegistryBackend, pkg lockfile.Package) []diag.Diagnostic {
	metadata, err := backend.Metadata(ctx, pkg.Name)
	if err != nil {
		return []diag.Diagnostic{syncHydrateFailedDiagnostic(pkg, "failed to fetch locked registry metadata for sync hydration", "package: "+pkg.Name, err.Error())}
	}
	version, ok := metadata.Versions[pkg.Version]
	if !ok {
		return []diag.Diagnostic{syncHydrateFailedDiagnostic(pkg, "locked registry version is unavailable from registry metadata", "package: "+pkg.Name, "version: "+pkg.Version)}
	}
	if strings.TrimSpace(version.Artifact.URL) == "" {
		return []diag.Diagnostic{syncLockArtifactIncompleteDiagnostic(pkg, "registry metadata did not provide a tarball URL for the locked version", "package: "+pkg.Name, "version: "+pkg.Version, "run `tspack update` with the current TSPack version to refresh ts-lock.toml")}
	}
	body, err := backend.FetchArtifact(ctx, version.Artifact)
	if err != nil {
		return []diag.Diagnostic{syncHydrateFailedDiagnostic(pkg, "failed to download locked registry artifact for sync hydration", "package: "+pkg.Name, "version: "+pkg.Version, err.Error())}
	}
	if pkg.Integrity != "" {
		verified, verifyErr := verifyLockedNPMIntegrity(body, pkg.Integrity)
		if verifyErr != nil {
			return []diag.Diagnostic{syncArtifactIntegrityDiagnostic(pkg, "lockfile registry integrity is unsupported or invalid", verifyErr.Error(), "integrity: "+pkg.Integrity)}
		}
		if !verified {
			return []diag.Diagnostic{syncArtifactIntegrityDiagnostic(pkg, "downloaded registry artifact did not match lockfile integrity", "integrity: "+pkg.Integrity)}
		}
	}
	ref, diagnostics := st.PutArtifact(store.Artifact{
		ID:        pkg.ID,
		Name:      pkg.Name,
		Version:   pkg.Version,
		Source:    pkg.Source,
		Hash:      pkg.Hash,
		Integrity: pkg.Integrity,
		Kind:      registryStoreArtifactKind(pkg.Source),
		Bytes:     body,
		Metadata: store.PackageMetadata{
			Name:             pkg.Name,
			Version:          pkg.Version,
			Source:           pkg.Source,
			PackageID:        pkg.ID,
			Integrity:        pkg.Integrity,
			RegistryEndpoint: pkg.RegistryEndpoint,
			MetadataEndpoint: pkg.MetadataEndpoint,
			ArtifactHost:     pkg.ArtifactHost,
		},
	})
	if len(diagnostics) > 0 {
		return remapSyncHydrateStoreDiagnostics(pkg, diagnostics)
	}
	if len(st.Verify(ref.Hash)) > 0 {
		return []diag.Diagnostic{syncHydrateFailedDiagnostic(pkg, "hydrated registry store artifact failed post-write verification", "hash: "+ref.Hash)}
	}
	return nil
}

func registryStoreArtifactKind(source string) store.ArtifactKind {
	if source == resolver.SourceNPM {
		return store.ArtifactNPMTarball
	}
	return store.ArtifactRegistryTarball
}

func hydrateMissingNPMStoreArtifact(ctx context.Context, st *store.Store, client resolver.NPMRegistryClient, pkg lockfile.Package) []diag.Diagnostic {
	if pkg.Name == "" || pkg.Version == "" {
		return []diag.Diagnostic{syncLockArtifactIncompleteDiagnostic(pkg, "lockfile npm package is missing name or version", "run `tspack update` with the current TSPack version to refresh ts-lock.toml")}
	}
	if _, ok := lockPackageStoreHash(pkg); !ok {
		return []diag.Diagnostic{syncLockArtifactIncompleteDiagnostic(pkg, "lockfile npm package is missing artifact hash", "run `tspack update` with the current TSPack version to refresh ts-lock.toml")}
	}

	meta, err := client.PackageMetadata(ctx, pkg.Name)
	if err != nil {
		return []diag.Diagnostic{syncHydrateFailedDiagnostic(pkg, "failed to fetch locked npm metadata for sync hydration", "package: "+pkg.Name, err.Error())}
	}
	versionMeta, ok := meta.Versions[pkg.Version]
	if !ok {
		return []diag.Diagnostic{syncHydrateFailedDiagnostic(pkg, "locked npm version is unavailable from registry metadata", "package: "+pkg.Name, "version: "+pkg.Version)}
	}
	if strings.TrimSpace(versionMeta.Dist.Tarball) == "" {
		return []diag.Diagnostic{syncLockArtifactIncompleteDiagnostic(pkg, "registry metadata did not provide a tarball URL for the locked version", "package: "+pkg.Name, "version: "+pkg.Version, "run `tspack update` with the current TSPack version to refresh ts-lock.toml")}
	}

	body, err := client.Tarball(ctx, versionMeta.Dist.Tarball)
	if err != nil {
		return []diag.Diagnostic{syncHydrateFailedDiagnostic(pkg, "failed to download locked npm tarball for sync hydration", "package: "+pkg.Name, "version: "+pkg.Version, "tarball: "+versionMeta.Dist.Tarball, err.Error())}
	}
	if pkg.Integrity != "" {
		ok, verifyErr := verifyLockedNPMIntegrity(body, pkg.Integrity)
		if verifyErr != nil {
			return []diag.Diagnostic{syncArtifactIntegrityDiagnostic(pkg, "lockfile npm integrity is unsupported or invalid", verifyErr.Error(), "integrity: "+pkg.Integrity)}
		}
		if !ok {
			return []diag.Diagnostic{syncArtifactIntegrityDiagnostic(pkg, "downloaded tarball did not match lockfile integrity", "integrity: "+pkg.Integrity)}
		}
	}

	ref, diags := st.PutArtifact(store.Artifact{
		ID:        pkg.ID,
		Name:      pkg.Name,
		Version:   pkg.Version,
		Source:    pkg.Source,
		Hash:      pkg.Hash,
		Integrity: pkg.Integrity,
		Kind:      store.ArtifactNPMTarball,
		Bytes:     body,
		Metadata: store.PackageMetadata{
			Name:         pkg.Name,
			Version:      pkg.Version,
			Source:       pkg.Source,
			PackageID:    pkg.ID,
			Integrity:    pkg.Integrity,
			Capabilities: pkg.Capabilities,
		},
	})
	if len(diags) > 0 {
		return remapSyncHydrateStoreDiagnostics(pkg, diags)
	}
	if len(st.Verify(ref.Hash)) > 0 {
		return []diag.Diagnostic{syncHydrateFailedDiagnostic(pkg, "hydrated store artifact failed post-write verification", "hash: "+ref.Hash)}
	}
	return nil
}

func hydrateMissingLocalStoreArtifact(rootDir string, st *store.Store, pkg lockfile.Package) []diag.Diagnostic {
	if strings.TrimSpace(pkg.Path) == "" {
		return []diag.Diagnostic{syncLockArtifactIncompleteDiagnostic(pkg, "lockfile local package is missing path metadata", "run `tspack update` with the current TSPack version to refresh ts-lock.toml")}
	}

	abs := filepath.Join(rootDir, filepath.FromSlash(pkg.Path))
	info, err := os.Stat(abs)
	if err != nil {
		return []diag.Diagnostic{syncHydrateFailedDiagnostic(pkg, "local dependency path needed for sync hydration is unavailable", "path: "+pkg.Path, err.Error())}
	}
	if !info.IsDir() {
		return []diag.Diagnostic{syncHydrateFailedDiagnostic(pkg, "local dependency path needed for sync hydration is not a directory", "path: "+pkg.Path)}
	}

	kind := store.ArtifactPathTree
	if pkg.Source == "workspace" {
		kind = store.ArtifactWorkspace
	}
	ref, diags := st.PutArtifact(store.Artifact{
		ID:      pkg.ID,
		Name:    pkg.Name,
		Version: pkg.Version,
		Source:  pkg.Source,
		Hash:    pkg.Hash,
		Kind:    kind,
		RootDir: abs,
		Metadata: store.PackageMetadata{
			Name:         pkg.Name,
			Version:      pkg.Version,
			Source:       pkg.Source,
			PackageID:    pkg.ID,
			Capabilities: pkg.Capabilities,
		},
	})
	if len(diags) > 0 {
		return remapSyncHydrateStoreDiagnostics(pkg, diags)
	}
	if len(st.Verify(ref.Hash)) > 0 {
		return []diag.Diagnostic{syncHydrateFailedDiagnostic(pkg, "hydrated local store artifact failed post-write verification", "hash: "+ref.Hash)}
	}
	return nil
}

func remapSyncHydrateStoreDiagnostics(pkg lockfile.Package, diags []diag.Diagnostic) []diag.Diagnostic {
	out := make([]diag.Diagnostic, 0, len(diags))
	for _, d := range diags {
		switch d.Code {
		case "TSPACK_STORE_HASH_MISMATCH":
			details := append([]string{"downloaded artifact did not match the locked content hash", "lock hash: " + pkg.Hash}, d.Details...)
			out = append(out, syncArtifactIntegrityDiagnostic(pkg, details...))
		default:
			details := append([]string{pkg.ID}, d.Details...)
			out = append(out, diag.Diagnostic{
				Code:     "TSPACK_SYNC_HYDRATE_FAILED",
				Severity: diag.SeverityError,
				Message:  "failed to hydrate missing store artifact from lockfile",
				Details:  details,
			})
		}
	}
	return out
}

func verifyLockedNPMIntegrity(body []byte, integrity string) (bool, error) {
	parts := strings.SplitN(integrity, "-", 2)
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid integrity %q", integrity)
	}
	want, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return false, fmt.Errorf("invalid integrity %q", integrity)
	}

	var got []byte
	switch parts[0] {
	case "sha512":
		sum := sha512.Sum512(body)
		got = sum[:]
	case "sha256":
		sum := sha256.Sum256(body)
		got = sum[:]
	default:
		return false, fmt.Errorf("unsupported integrity algorithm %q", parts[0])
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func lockPackageStoreHash(pkg lockfile.Package) (string, bool) {
	if pkg.Hash != "" {
		return pkg.Hash, true
	}
	if pkg.TreeHash != "" {
		return pkg.TreeHash, true
	}
	return "", false
}

func syncLockArtifactIncompleteDiagnostic(pkg lockfile.Package, details ...string) diag.Diagnostic {
	return diag.Diagnostic{
		Code:     "TSPACK_SYNC_LOCK_ARTIFACT_INCOMPLETE",
		Severity: diag.SeverityError,
		Message:  "lockfile package lacks enough metadata to hydrate a missing store artifact",
		Details:  append([]string{pkg.ID}, details...),
	}
}

func syncHydrateFailedDiagnostic(pkg lockfile.Package, details ...string) diag.Diagnostic {
	return diag.Diagnostic{
		Code:     "TSPACK_SYNC_HYDRATE_FAILED",
		Severity: diag.SeverityError,
		Message:  "failed to hydrate missing store artifact from lockfile",
		Details:  append([]string{pkg.ID}, details...),
	}
}

func syncArtifactIntegrityDiagnostic(pkg lockfile.Package, details ...string) diag.Diagnostic {
	return diag.Diagnostic{
		Code:     "TSPACK_SYNC_ARTIFACT_INTEGRITY_FAILED",
		Severity: diag.SeverityError,
		Message:  "hydrated artifact did not match lockfile integrity",
		Details:  append([]string{pkg.ID}, details...),
	}
}
