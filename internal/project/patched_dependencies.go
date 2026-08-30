package project

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/patchapply"
	"github.com/yuechen-li-dev/tspack/internal/store"
)

type declaredPackagePatch struct {
	source  string
	name    string
	version string
	path    string
	digest  string
}

func applyDeclaredPatches(root string, workspaceGraph *graph.WorkspaceGraph, locked *lockfile.Lockfile, existing *lockfile.Lockfile) []diag.Diagnostic {
	declarations, diagnostics := collectDeclaredPatches(root, workspaceGraph)
	if len(diagnostics) > 0 {
		return diagnostics
	}
	remapped := map[string]string{}
	existingRealizations := map[string]lockfile.Package{}
	if existing != nil {
		for _, pkg := range existing.Packages {
			if pkg.RealizationID != "" {
				existingRealizations[pkg.RealizationID] = pkg
			}
		}
	}
	for _, declaration := range declarations {
		matched := false
		var differentVersion *lockfile.Package
		for index := range locked.Packages {
			pkg := &locked.Packages[index]
			if pkg.Source != declaration.source || pkg.Name != declaration.name {
				continue
			}
			if pkg.Version != declaration.version {
				copy := *pkg
				differentVersion = &copy
				continue
			}
			matched = true
			oldID := pkg.ID
			pkg.SourceID = oldID
			pkg.SourceHash = pkg.Hash
			pkg.RealizationID = oldID + "#patch=" + patchapply.Algorithm + "." + strings.TrimPrefix(declaration.digest, "sha256:")
			pkg.ID = pkg.RealizationID
			pkg.Hash = ""
			pkg.Patch = &lockfile.Patch{Path: declaration.path, SHA256: declaration.digest, Algorithm: patchapply.Algorithm}
			if previous, reusable := existingRealizations[pkg.RealizationID]; reusable && previous.SourceHash == pkg.SourceHash && previous.Hash != "" {
				pkg.Hash = previous.Hash
			}
			remapped[oldID] = pkg.ID
		}
		if !matched {
			if differentVersion != nil {
				return []diag.Diagnostic{patchDiagnostic("TSPACK_PATCH_TARGET_VERSION_MISMATCH", "resolved package does not match the patch's exact target version", *differentVersion, declaration.path, "expected: "+declaration.version, "resolved: "+differentVersion.Version)}
			}
			return []diag.Diagnostic{patchDiagnostic("TSPACK_PATCH_TARGET_UNKNOWN", "patch target was not selected into the authoritative lock graph", lockfile.Package{Name: declaration.name, Version: declaration.version, Source: declaration.source}, declaration.path)}
		}
	}
	for index := range locked.Edges {
		if replacement := remapped[locked.Edges[index].From]; replacement != "" {
			locked.Edges[index].From = replacement
		}
		if replacement := remapped[locked.Edges[index].To]; replacement != "" {
			locked.Edges[index].To = replacement
		}
	}
	for index := range locked.Requirements {
		if replacement := remapped[locked.Requirements[index].PackageID]; replacement != "" {
			locked.Requirements[index].PackageID = replacement
		}
		if replacement := remapped[locked.Requirements[index].RequiringPackage]; replacement != "" {
			locked.Requirements[index].RequiringPackage = replacement
		}
	}
	return nil
}

func validateLockedPatches(root string, workspaceGraph *graph.WorkspaceGraph, locked *lockfile.Lockfile) []diag.Diagnostic {
	declarations, diagnostics := collectDeclaredPatches(root, workspaceGraph)
	if len(diagnostics) > 0 {
		return diagnostics
	}
	wanted := map[string]declaredPackagePatch{}
	for _, declaration := range declarations {
		wanted[declaration.source+"\x00"+declaration.name+"\x00"+declaration.version] = declaration
	}
	seen := map[string]bool{}
	for _, pkg := range locked.Packages {
		key := pkg.Source + "\x00" + pkg.Name + "\x00" + pkg.Version
		declaration, declared := wanted[key]
		if pkg.Patch == nil {
			if declared {
				return []diag.Diagnostic{patchDiagnostic("TSPACK_PATCH_LOCK_STALE", "lock contains the raw package but the manifest declares a patched realization; run `tspack update`", pkg, declaration.path)}
			}
			continue
		}
		if !declared {
			return []diag.Diagnostic{patchDiagnostic("TSPACK_PATCH_LOCK_STALE", "lock contains a patched realization not declared by the manifest; run `tspack update`", pkg, pkg.Patch.Path)}
		}
		seen[key] = true
		if pkg.Patch.SHA256 != declaration.digest || pkg.Patch.Algorithm != patchapply.Algorithm || pkg.RealizationID == "" || pkg.SourceID == "" || pkg.SourceHash == "" {
			return []diag.Diagnostic{patchDiagnostic("TSPACK_PATCH_LOCK_STALE", "locked patch identity differs from the manifest or lacks realization provenance; run `tspack update`", pkg, declaration.path)}
		}
	}
	for key, declaration := range wanted {
		if !seen[key] {
			return []diag.Diagnostic{patchDiagnostic("TSPACK_PATCH_LOCK_STALE", "declared patch target is absent from the lock; run `tspack update`", lockfile.Package{Source: declaration.source, Name: declaration.name, Version: declaration.version}, declaration.path)}
		}
	}
	return nil
}

func collectDeclaredPatches(root string, workspaceGraph *graph.WorkspaceGraph) ([]declaredPackagePatch, []diag.Diagnostic) {
	byTarget := map[string]declaredPackagePatch{}
	for _, pkg := range workspaceGraph.AllPackages() {
		for _, dependency := range pkg.AllDependencies() {
			if dependency.Patch == nil {
				continue
			}
			patchPath := filepath.Join(root, filepath.FromSlash(dependency.Patch.Path))
			contents, err := os.ReadFile(patchPath)
			if err != nil {
				return nil, []diag.Diagnostic{patchDiagnostic("TSPACK_PATCH_FILE_MISSING", "declared dependency patch could not be read", lockfile.Package{Name: dependency.Source.Package, Version: dependency.Patch.Version, Source: dependency.Source.Kind}, dependency.Patch.Path, err.Error())}
			}
			declaration := declaredPackagePatch{source: dependency.Source.Kind, name: dependency.Source.Package, version: dependency.Patch.Version, path: filepath.ToSlash(dependency.Patch.Path), digest: patchapply.Digest(contents)}
			key := declaration.source + "\x00" + declaration.name + "\x00" + declaration.version
			if previous, exists := byTarget[key]; exists && previous.digest != declaration.digest {
				return nil, []diag.Diagnostic{patchDiagnostic("TSPACK_PATCH_DECLARATION_CONFLICT", "the same exact package has multiple different patch declarations", lockfile.Package{Name: declaration.name, Version: declaration.version, Source: declaration.source}, declaration.path, "other patch: "+previous.path)}
			}
			byTarget[key] = declaration
		}
	}
	out := make([]declaredPackagePatch, 0, len(byTarget))
	for _, declaration := range byTarget {
		out = append(out, declaration)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := out[i].source + ":" + out[i].name + "@" + out[i].version
		right := out[j].source + ":" + out[j].name + "@" + out[j].version
		return left < right
	})
	return out, nil
}

func populatePatchedPackage(root string, st *store.Store, pkg lockfile.Package) (string, []diag.Diagnostic) {
	patchPath := filepath.Join(root, filepath.FromSlash(pkg.Patch.Path))
	patchBytes, err := os.ReadFile(patchPath)
	if err != nil {
		return "", []diag.Diagnostic{patchDiagnostic("TSPACK_PATCH_FILE_MISSING", "locked dependency patch could not be read", pkg, pkg.Patch.Path, err.Error())}
	}
	if got := patchapply.Digest(patchBytes); got != pkg.Patch.SHA256 {
		return "", []diag.Diagnostic{patchDiagnostic("TSPACK_PATCH_DIGEST_MISMATCH", "patch content does not match the authoritative lock", pkg, pkg.Patch.Path, "locked: "+pkg.Patch.SHA256, "actual: "+got)}
	}
	if pkg.Patch.Algorithm != patchapply.Algorithm {
		return "", []diag.Diagnostic{patchDiagnostic("TSPACK_PATCH_ALGORITHM_UNSUPPORTED", "lock requires an unsupported patch application algorithm", pkg, pkg.Patch.Path, pkg.Patch.Algorithm)}
	}
	sourceRef, sourceDiagnostics := st.Get(pkg.SourceHash)
	if len(sourceDiagnostics) > 0 {
		return "", []diag.Diagnostic{patchDiagnostic("TSPACK_PATCH_SOURCE_MISSING", "verified raw package source is missing from the store", pkg, pkg.Patch.Path, "source hash: "+pkg.SourceHash)}
	}
	stagingRoot := filepath.Join(root, ".tspack")
	if err := os.MkdirAll(stagingRoot, 0o755); err != nil {
		return "", []diag.Diagnostic{patchDiagnostic("TSPACK_PATCH_APPLY_FAILED", "could not create patch staging root", pkg, pkg.Patch.Path, err.Error())}
	}
	stage, err := os.MkdirTemp(stagingRoot, "patched-package-*")
	if err != nil {
		return "", []diag.Diagnostic{patchDiagnostic("TSPACK_PATCH_APPLY_FAILED", "could not create patch staging directory", pkg, pkg.Patch.Path, err.Error())}
	}
	defer os.RemoveAll(stage)
	if err := patchapply.Materialize(sourceRef.ExtractedPath, stage, patchBytes); err != nil {
		return "", []diag.Diagnostic{patchDiagnostic("TSPACK_PATCH_APPLY_FAILED", "dependency patch failed exact deterministic application", pkg, pkg.Patch.Path, err.Error())}
	}
	ref, storeDiagnostics := st.PutPatchedTree(store.Artifact{ID: pkg.ID, Name: pkg.Name, Version: pkg.Version, Source: pkg.Source, Hash: pkg.Hash, RootDir: stage, Metadata: store.PackageMetadata{Name: pkg.Name, Version: pkg.Version, Source: pkg.Source, PackageID: pkg.ID, SourcePackageID: pkg.SourceID, RealizationID: pkg.RealizationID, Integrity: pkg.Integrity, Patch: pkg.Patch, Capabilities: pkg.Capabilities}})
	if len(storeDiagnostics) > 0 {
		return "", []diag.Diagnostic{patchDiagnostic("TSPACK_PATCHED_STORE_VERIFICATION_FAILED", "patched dependency realization could not be stored safely", pkg, pkg.Patch.Path, storeDiagnostics[0].Message)}
	}
	if diagnostics := st.Verify(ref.Hash); len(diagnostics) > 0 {
		return "", []diag.Diagnostic{patchDiagnostic("TSPACK_PATCHED_STORE_VERIFICATION_FAILED", "patched dependency realization failed post-write verification", pkg, pkg.Patch.Path, diagnostics[0].Message)}
	}
	return ref.Hash, nil
}

func patchDiagnostic(code string, message string, pkg lockfile.Package, patchPath string, details ...string) diag.Diagnostic {
	prefix := []string{"package: " + pkg.Source + ":" + pkg.Name + "@" + pkg.Version, "patch: " + patchPath}
	return diag.Diagnostic{Code: code, Severity: diag.SeverityError, Message: message, Details: append(prefix, details...)}
}
