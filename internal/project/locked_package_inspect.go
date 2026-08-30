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
	ID              string                        `json:"id"`
	Name            string                        `json:"name"`
	Version         string                        `json:"version"`
	Source          string                        `json:"source"`
	SourceID        string                        `json:"sourceIdentity"`
	SourceHash      string                        `json:"sourceHash,omitempty"`
	RealizationID   string                        `json:"realizationIdentity"`
	RealizationHash string                        `json:"realizationHash"`
	Patch           *lockfile.Patch               `json:"patch,omitempty"`
	Instances       []LockedModuleInstanceSummary `json:"moduleInstances"`
}

type LockedModuleInstanceSummary struct {
	ID            string                   `json:"id"`
	PeerContextID string                   `json:"peerContextId"`
	Peers         []lockfile.PeerBinding   `json:"peers"`
	Consumers     []LockedInstanceConsumer `json:"consumers,omitempty"`
}

type LockedInstanceConsumer struct {
	From      string `json:"from"`
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
}

func InspectLockedPackages(options Options) LockedPackageInspectResult {
	locked, diagnostics, err := lockfile.LoadFile(options.LockfilePath)
	if err != nil {
		return LockedPackageInspectResult{Diagnostics: []diag.Diagnostic{errDiag("TSPACK_INSPECT_LOCK_READ_FAILED", "could not read authoritative lockfile", err.Error())}}
	}
	result := LockedPackageInspectResult{Diagnostics: diagnostics}
	instancesByPackage := map[string][]LockedModuleInstanceSummary{}
	for _, instance := range locked.Instances {
		consumers := []LockedInstanceConsumer{}
		for _, edge := range locked.Edges {
			if edge.To != instance.PackageID {
				continue
			}
			consumers = append(consumers, LockedInstanceConsumer{From: edge.From, Kind: edge.Kind, Reference: edge.Reference})
		}
		instancesByPackage[instance.PackageID] = append(instancesByPackage[instance.PackageID], LockedModuleInstanceSummary{
			ID:            instance.ID,
			PeerContextID: instance.PeerContextID,
			Peers:         append([]lockfile.PeerBinding(nil), instance.Peers...),
			Consumers:     consumers,
		})
	}
	for _, pkg := range locked.Packages {
		sourceID := pkg.SourceID
		if sourceID == "" {
			sourceID = pkg.ID
		}
		realizationID := pkg.RealizationID
		if realizationID == "" {
			realizationID = pkg.ID
		}
		result.Packages = append(result.Packages, LockedPackageSummary{ID: pkg.ID, Name: pkg.Name, Version: pkg.Version, Source: pkg.Source, SourceID: sourceID, SourceHash: pkg.SourceHash, RealizationID: realizationID, RealizationHash: pkg.Hash, Patch: pkg.Patch, Instances: instancesByPackage[pkg.ID]})
	}
	sort.SliceStable(result.Packages, func(i, j int) bool {
		return result.Packages[i].ID < result.Packages[j].ID
	})
	return result
}
