package project

import (
	"fmt"
	"sort"

	capmodel "github.com/yuechen-li-dev/tspack/internal/capability"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

type PolicySecurityGateResult struct {
	Status      string
	Reasons     []string
	Diagnostics []PolicySecurityGateDiagnostic
}

type PolicySecurityGateDiagnostic struct {
	PackageID           string `json:"package"`
	PackageName         string `json:"packageName"`
	Version             string `json:"version"`
	LifecycleScriptName string `json:"lifecycleScriptName"`
	LifecycleCategory   string `json:"lifecycleCategory"`
	ConsumerInstallTime bool   `json:"consumerInstallTime"`
	Acknowledged        bool   `json:"acknowledged"`
	AcknowledgmentKind  string `json:"acknowledgmentKind,omitempty"`
	Reason              string `json:"reason"`
}

func EvaluatePolicySecurityGate(dep OutdatedDependency, security manifest.Security) PolicySecurityGateResult {
	if dep.Source != "npm" || dep.Latest == "" || dep.Status == "current" {
		return PolicySecurityGateResult{Status: "not_applicable", Reasons: []string{"no registry package change to evaluate"}}
	}
	if !dep.CandidateMetadataAvailable {
		return PolicySecurityGateResult{Status: "review_required", Reasons: []string{"candidate security metadata unavailable"}}
	}
	if len(dep.CandidateCapabilities) == 0 {
		return PolicySecurityGateResult{Status: "passed", Reasons: []string{}}
	}

	packageID := "npm:" + dep.Name + "@" + dep.Latest
	result := PolicySecurityGateResult{Status: "passed"}
	capabilities := append([]lockfile.Capability(nil), dep.CandidateCapabilities...)
	sort.SliceStable(capabilities, func(i, j int) bool {
		if capabilities[i].Script != capabilities[j].Script {
			return capabilities[i].Script < capabilities[j].Script
		}
		return capabilities[i].Command < capabilities[j].Command
	})

	for _, candidateCapability := range capabilities {
		if candidateCapability.Kind != capmodel.LifecycleScriptKind {
			continue
		}
		diagnostic := evaluateCandidateLifecycleCapability(dep, packageID, candidateCapability, security)
		result.Diagnostics = append(result.Diagnostics, diagnostic)
		if diagnostic.Acknowledged {
			continue
		}
		result.Reasons = append(result.Reasons, diagnostic.Reason)
		if diagnostic.ConsumerInstallTime {
			result.Status = stricterSecurityGateStatus(result.Status, "blocked")
			continue
		}
		result.Status = stricterSecurityGateStatus(result.Status, "review_required")
	}
	return result
}

func evaluateCandidateLifecycleCapability(dep OutdatedDependency, packageID string, capability lockfile.Capability, security manifest.Security) PolicySecurityGateDiagnostic {
	classification := capmodel.ClassifyLifecycleScript(capability.Script)
	diagnostic := PolicySecurityGateDiagnostic{
		PackageID:           packageID,
		PackageName:         dep.Name,
		Version:             dep.Latest,
		LifecycleScriptName: capability.Script,
		LifecycleCategory:   classification.LifecycleCategory,
		ConsumerInstallTime: classification.ConsumerInstallTime,
	}
	for _, acknowledgement := range security.AcknowledgedCapabilities {
		if acknowledgement.Package == packageID && acknowledgement.Kind == capmodel.LifecycleScriptKind && acknowledgement.Script == capability.Script && acknowledgement.Command == capability.Command {
			diagnostic.Acknowledged = true
			diagnostic.AcknowledgmentKind = "capability"
			diagnostic.Reason = "exact lifecycle capability acknowledged"
			return diagnostic
		}
	}
	for _, acknowledgement := range security.AcknowledgedCapabilities {
		if acknowledgement.Package == packageID && acknowledgement.Kind == capmodel.LifecycleScriptKind && acknowledgement.Script == capability.Script && acknowledgement.Command != capability.Command {
			diagnostic.Reason = fmt.Sprintf("acknowledged lifecycle script %s command changed", capability.Script)
			return diagnostic
		}
	}
	for _, acknowledgement := range security.AcknowledgedLifecycleCategories {
		if acknowledgement.Category != classification.LifecycleCategory {
			continue
		}
		if lifecycleCategoryAcknowledgesScript(acknowledgement, capability.Script) {
			diagnostic.Acknowledged = true
			diagnostic.AcknowledgmentKind = "lifecycle-category"
			diagnostic.Reason = "lifecycle category acknowledged"
			return diagnostic
		}
	}
	return unacknowledgedLifecycleDiagnostic(diagnostic, capability.Script)
}

func lifecycleCategoryAcknowledgesScript(acknowledgement manifest.AcknowledgedLifecycleCategory, script string) bool {
	if len(acknowledgement.Scripts) == 0 {
		return true
	}
	for _, acknowledgedScript := range acknowledgement.Scripts {
		if acknowledgedScript == script {
			return true
		}
	}
	return false
}

func unacknowledgedLifecycleDiagnostic(diagnostic PolicySecurityGateDiagnostic, script string) PolicySecurityGateDiagnostic {
	if diagnostic.ConsumerInstallTime {
		diagnostic.Reason = fmt.Sprintf("unacknowledged consumer-install lifecycle script %s", script)
		return diagnostic
	}
	diagnostic.Reason = fmt.Sprintf("unacknowledged %s lifecycle script %s", diagnostic.LifecycleCategory, script)
	return diagnostic
}

func stricterSecurityGateStatus(current string, next string) string {
	if current == "blocked" || next == "blocked" {
		return "blocked"
	}
	if current == "review_required" || next == "review_required" {
		return "review_required"
	}
	return next
}
