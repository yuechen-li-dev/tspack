package project

import (
	"sort"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
)

type LockedPackageInspectResult struct {
	Packages    []LockedPackageSummary `json:"packages"`
	Diagnostics []diag.Diagnostic      `json:"diagnostics,omitempty"`
}

type LockedPackageSummary struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Version         string          `json:"version"`
	Source          string          `json:"source"`
	SourceID        string          `json:"sourceIdentity"`
	SourceHash      string          `json:"sourceHash,omitempty"`
	RealizationID   string          `json:"realizationIdentity"`
	RealizationHash string          `json:"realizationHash"`
	Patch           *lockfile.Patch `json:"patch,omitempty"`
}

func InspectLockedPackages(options Options) LockedPackageInspectResult {
	locked, diagnostics, err := lockfile.LoadFile(options.LockfilePath)
	if err != nil {
		return LockedPackageInspectResult{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_INSPECT_LOCK_READ_FAILED", "could not read authoritative lockfile", err.Error())}}
	}
	result := LockedPackageInspectResult{Diagnostics: diagnostics}
	for _, pkg := range locked.Packages {
		sourceID := pkg.SourceID
		if sourceID == "" {
			sourceID = pkg.ID
		}
		realizationID := pkg.RealizationID
		if realizationID == "" {
			realizationID = pkg.ID
		}
		result.Packages = append(result.Packages, LockedPackageSummary{ID: pkg.ID, Name: pkg.Name, Version: pkg.Version, Source: pkg.Source, SourceID: sourceID, SourceHash: pkg.SourceHash, RealizationID: realizationID, RealizationHash: pkg.Hash, Patch: pkg.Patch})
	}
	sort.SliceStable(result.Packages, func(i, j int) bool {
		return result.Packages[i].ID < result.Packages[j].ID
	})
	return result
}
