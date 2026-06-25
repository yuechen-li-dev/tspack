package project

import (
	"fmt"

	semver "github.com/Masterminds/semver/v3"
	"github.com/tspack/tspack/internal/manifest"
)

const (
	policyStatusAllowed            = "allowed"
	policyStatusBlockedManual      = "blocked-manual"
	policyStatusPinned             = "pinned"
	policyStatusUnclassified       = "unclassified"
	policyStatusNotApplicable      = "not-applicable"
	policyStatusOutsidePolicyLevel = "outside-policy-level"
)

func applyUpdatePolicy(ir *manifest.ManifestIR, entries []OutdatedDependency) {
	rows := []manifest.UpdatePolicyRow(nil)
	if ir != nil {
		rows = ir.UpdatePolicy.Rows
	}
	for index := range entries {
		entry := &entries[index]
		if entry.Source != "npm" || entry.Status == "not_applicable" {
			entry.PolicyStatus = policyStatusNotApplicable
			entry.PolicyMessage = "non-registry dependencies are not evaluated by update policy"
			continue
		}
		row, rowIndex, matched := matchUpdatePolicy(rows, *entry)
		if !matched {
			entry.PolicyStatus = policyStatusUnclassified
			entry.PolicyMessage = "no update policy row matches this dependency"
			continue
		}
		entry.PolicyMatched = true
		entry.PolicyRow = rowIndex
		entry.PolicyStrategy = row.Strategy
		entry.PolicyLevel = row.Level
		entry.PolicyReason = row.Reason
		switch row.Strategy {
		case "manual":
			entry.PolicyStatus = policyStatusBlockedManual
			entry.PolicyMessage = "updates require an explicit manual decision"
		case "pinned":
			entry.PolicyStatus = policyStatusPinned
			entry.PolicyMessage = "dependency is intended to stay pinned until manifest intent changes"
		case "rolling":
			if rollingLevelAllows(entry.Current, entry.Latest, row.Level) {
				entry.PolicyStatus = policyStatusAllowed
				entry.PolicyMessage = fmt.Sprintf("rolling %s policy allows this candidate", row.Level)
			} else {
				entry.PolicyStatus = policyStatusOutsidePolicyLevel
				entry.PolicyMessage = fmt.Sprintf("candidate is outside rolling %s policy", row.Level)
			}
		}
	}
}

func matchUpdatePolicy(rows []manifest.UpdatePolicyRow, entry OutdatedDependency) (manifest.UpdatePolicyRow, int, bool) {
	bestScore := -1
	bestIndex := -1
	var best manifest.UpdatePolicyRow
	for index, row := range rows {
		if row.Name != entry.Name {
			continue
		}
		if row.Kind != entry.Kind && row.Kind != "any" {
			continue
		}
		if len(row.Packages) > 0 && !rowMatchesPackages(row, entry.Packages) {
			continue
		}
		score := 0
		if row.Kind == entry.Kind {
			score++
		}
		if len(row.Packages) > 0 {
			score += 2
		}
		if score > bestScore {
			bestScore = score
			bestIndex = index
			best = row
		}
	}
	return best, bestIndex, bestIndex >= 0
}

func rowMatchesPackages(row manifest.UpdatePolicyRow, packages []OutdatedPackage) bool {
	allowed := map[string]struct{}{}
	for _, packageName := range row.Packages {
		allowed[packageName] = struct{}{}
	}
	for _, pkg := range packages {
		if _, ok := allowed[pkg.Name]; ok {
			return true
		}
	}
	return false
}

func rollingLevelAllows(currentVersions []string, latest string, level string) bool {
	if len(currentVersions) == 0 || latest == "" {
		return false
	}
	current, currentErr := semver.NewVersion(currentVersions[len(currentVersions)-1])
	candidate, candidateErr := semver.NewVersion(latest)
	if currentErr != nil || candidateErr != nil {
		return false
	}
	if !candidate.GreaterThan(current) {
		return true
	}
	switch level {
	case "patch":
		return current.Major() == candidate.Major() && current.Minor() == candidate.Minor()
	case "minor":
		return current.Major() == candidate.Major()
	case "major", "latest":
		return true
	default:
		return false
	}
}
