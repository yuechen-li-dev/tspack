package cli

import (
	"encoding/json"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/project"
	"io"
	"os"
	"strings"
	"testing"
)

func countDiagnostics(diagnostics []checkJSONDiagnostic, code string) int {
	count := 0
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			count++
		}
	}
	return count
}

func TestOutdatedJSONEntriesIncludePolicyFields(t *testing.T) {
	result := &project.OutdatedResult{Dependencies: []project.OutdatedDependency{{
		Name:           "typescript",
		Kind:           "tool",
		Source:         "npm",
		Requested:      "^5.8.0",
		Current:        []string{"5.8.0"},
		Wanted:         "5.9.3",
		Latest:         "5.9.3",
		Status:         "wanted_available",
		PolicyStrategy: "rolling",
		PolicyLevel:    "minor",
		PolicyStatus:   "allowed",
		PolicyReason:   "tooling can roll",
		PolicyMatched:  true,
		PolicyMessage:  "rolling minor policy allows this candidate",
	}}}
	entries := outdatedJSONEntries(result, true)
	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %#v", entries)
	}
	entry := entries[0]
	if entry.PolicyStatus != "allowed" || entry.PolicyStrategy != "rolling" || entry.PolicyLevel != "minor" || !entry.PolicyMatched {
		t.Fatalf("missing policy fields in JSON entry: %#v", entry)
	}
}

func TestPolicyUpdatePlanBucketsAndDryRunSemantics(t *testing.T) {
	outdated := &project.OutdatedResult{
		HasPolicy: true,
		Groups: []project.OutdatedDependency{
			{Name: "typescript", Kind: "tool", Source: "npm", Requested: "^5.0.0", Current: []string{"5.8.0"}, Latest: "5.9.3", PolicyStatus: "allowed", CandidateMetadataAvailable: true, PolicyStrategy: "rolling", PolicyLevel: "minor", PolicyMessage: "rolling minor policy allows this candidate", PackageCount: 3},
			{Name: "vite", Kind: "tool", Source: "npm", Requested: "^5.0.0", Current: []string{"5.4.21"}, Latest: "8.0.16", PolicyStatus: "outside-policy-level", CandidateMetadataAvailable: true, PolicyStrategy: "rolling", PolicyLevel: "minor", PolicyMessage: "candidate is outside rolling minor policy", PackageCount: 1},
			{Name: "react", Kind: "dep", Source: "npm", Requested: "^18.0.0", Current: []string{"18.3.1"}, Latest: "19.2.7", PolicyStatus: "blocked-manual", CandidateMetadataAvailable: true, PolicyStrategy: "manual", PolicyMessage: "updates require an explicit manual decision", PackageCount: 1},
			{Name: "react-dom", Kind: "peer", Source: "npm", Requested: "^18.0.0", Current: []string{"18.3.1"}, Latest: "19.2.7", PolicyStatus: "pinned", CandidateMetadataAvailable: true, PolicyStrategy: "pinned", PolicyMessage: "dependency is intended to stay pinned until manifest intent changes", PackageCount: 1},
			{Name: "clsx", Kind: "dep", Source: "npm", Requested: "^2.0.0", Current: []string{"2.0.0"}, Latest: "2.1.1", PolicyStatus: "unclassified", CandidateMetadataAvailable: true, PolicyMessage: "no update policy row matches this dependency", PackageCount: 1},
			{Name: "@repo/components", Kind: "dep", Source: "workspace", PolicyStatus: "not-applicable", PolicyMessage: "non-registry dependencies are not evaluated by update policy", PackageCount: 1},
		},
	}

	plan := buildPolicyUpdatePlan(outdated)
	if !plan.PolicyPresent || !plan.WouldUpdate {
		t.Fatalf("expected present policy with wouldUpdate=true: %#v", plan)
	}
	if plan.Summary.Allowed != 1 || plan.Summary.Blocked != 3 || plan.Summary.Unclassified != 1 || plan.Summary.NotApplicable != 1 {
		t.Fatalf("unexpected summary: %#v", plan.Summary)
	}
	if !plan.SecurityGatesEvaluated || plan.SecurityGateStatus != "mixed" || !plan.WouldApply {
		t.Fatalf("security gates should be evaluated: %#v", plan)
	}
	if plan.Summary.Ready != 1 || plan.Summary.SecurityBlocked != 0 || plan.Summary.ReviewRequired != 0 {
		t.Fatalf("unexpected security summary: %#v", plan.Summary)
	}
	if plan.Allowed[0].Action != "update" || plan.Allowed[0].EffectiveAction != "update" || plan.Blocked[0].Action != "outside-policy" {
		t.Fatalf("unexpected actions: allowed=%#v blocked=%#v", plan.Allowed[0], plan.Blocked[0])
	}
}

func TestPolicyPlanDogfoodHumanAndJSONShape(t *testing.T) {
	outdated := dogfoodPolicyOutdatedResult()
	plan := buildPolicyUpdatePlan(outdated)
	if !plan.SecurityGatesEvaluated || !plan.WouldUpdate || !plan.WouldApply {
		t.Fatalf("expected dogfood policy plan to evaluate and apply ready candidates: %#v", plan)
	}
	if plan.Summary.Ready != 3 || plan.Summary.SecurityBlocked != 1 || plan.Summary.ReviewRequired != 0 || plan.Summary.Blocked != 3 || plan.Summary.NotApplicable != 1 {
		t.Fatalf("unexpected dogfood policy summary: %#v", plan.Summary)
	}
	if len(plan.Allowed) == 0 || len(plan.Blocked) == 0 || len(plan.Unclassified) != 0 || len(plan.NotApplicable) == 0 || plan.Noop == nil {
		t.Fatalf("expected stable policy arrays: %#v", plan)
	}
	seenEffectiveAction := map[string]bool{}
	seenSecurityReason := false
	for _, candidate := range plan.Allowed {
		seenEffectiveAction[candidate.EffectiveAction] = true
		if candidate.SecurityGateStatus != "passed" && len(candidate.SecurityGateReasons) > 0 {
			seenSecurityReason = true
		}
	}
	if !seenEffectiveAction["update"] || !seenEffectiveAction["blocked"] || !seenSecurityReason {
		t.Fatalf("allowed candidates should include update and security-blocked actions: %#v", plan.Allowed)
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal dogfood policy plan: %v", err)
	}
	var decoded PolicyUpdatePlanJSON
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal dogfood policy plan: %v", err)
	}
	if decoded.Summary.Ready != plan.Summary.Ready || decoded.Allowed[0].EffectiveAction == "" || decoded.Allowed[0].SecurityGateStatus == "" {
		t.Fatalf("dogfood JSON shape lost stable fields: %#v", decoded)
	}

	text := captureStdoutForTest(t, func() {
		printPolicyUpdateDryRunPlan(project.Result{Outdated: outdated})
	})
	for _, expected := range []string{"Policy update plan (dry run)", "No lockfile changes will be written.", "Ready:", "Blocked by security:", "Blocked by policy:", "Not applicable:", "security gates: evaluated", "lifecycle execution remains blocked", "lockfile written: no"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("dogfood human plan missing %q:\n%s", expected, text)
		}
	}
}

func dogfoodPolicyOutdatedResult() *project.OutdatedResult {
	security := manifest.Security{
		AcknowledgedCapabilities: []manifest.AcknowledgedCapability{{
			Package: "npm:@biomejs/biome@1.10.0",
			Kind:    "lifecycleScript",
			Script:  "postinstall",
			Command: "node ./scripts/postinstall.js",
			Reason:  "Reviewed Biome postinstall native-binary selection for this fixture.",
		}},
		AcknowledgedLifecycleCategories: []manifest.AcknowledgedLifecycleCategory{{
			Category: "maintainer-publish",
			Scripts:  []string{"prepare", "prepublishOnly"},
			Reason:   "Maintainer-publish scripts are reviewed as publish-time metadata and are not run by TSPack installs.",
		}},
	}
	return &project.OutdatedResult{HasPolicy: true, Security: security, Groups: []project.OutdatedDependency{
		{Name: "typescript", Kind: "tool", Source: "npm", Requested: "^5.8.0", Current: []string{"5.8.0"}, Wanted: "5.9.3", Latest: "5.9.3", Status: "wanted_available", PolicyStatus: "allowed", PolicyStrategy: "rolling", PolicyLevel: "minor", PolicyMessage: "rolling minor policy allows this candidate", CandidateMetadataAvailable: true, PackageCount: 3},
		{Name: "@biomejs/biome", Kind: "tool", Source: "npm", Requested: "^1.9.0", Current: []string{"1.9.4"}, Wanted: "1.10.0", Latest: "1.10.0", Status: "wanted_available", PolicyStatus: "allowed", PolicyStrategy: "rolling", PolicyLevel: "minor", PolicyMessage: "rolling minor policy allows this candidate", CandidateMetadataAvailable: true, CandidateCapabilities: []lockfile.Capability{{Kind: "lifecycleScript", Script: "postinstall", Command: "node ./scripts/postinstall.js"}}, PackageCount: 1},
		{Name: "rollup", Kind: "tool", Source: "npm", Requested: "^4.20.0", Current: []string{"4.20.0"}, Wanted: "4.21.0", Latest: "4.21.0", Status: "wanted_available", PolicyStatus: "allowed", PolicyStrategy: "rolling", PolicyLevel: "minor", PolicyMessage: "rolling minor policy allows this candidate", CandidateMetadataAvailable: true, CandidateCapabilities: []lockfile.Capability{{Kind: "lifecycleScript", Script: "prepare", Command: "node ./scripts/prepare-release.js"}}, PackageCount: 1},
		{Name: "esbuild", Kind: "tool", Source: "npm", Requested: "^0.21.0", Current: []string{"0.21.0"}, Wanted: "0.25.0", Latest: "0.25.0", Status: "wanted_available", PolicyStatus: "allowed", PolicyStrategy: "rolling", PolicyLevel: "major", PolicyMessage: "rolling major policy allows this candidate", CandidateMetadataAvailable: true, CandidateCapabilities: []lockfile.Capability{{Kind: "lifecycleScript", Script: "postinstall", Command: "node install.js"}}, PackageCount: 1},
		{Name: "vite", Kind: "tool", Source: "npm", Requested: "^5.4.0", Current: []string{"5.4.21"}, Wanted: "5.4.21", Latest: "8.0.16", Status: "latest_available", PolicyStatus: "outside-policy-level", PolicyStrategy: "rolling", PolicyLevel: "minor", PolicyMessage: "candidate is outside rolling minor policy", CandidateMetadataAvailable: true, PackageCount: 2},
		{Name: "react", Kind: "dep", Source: "npm", Requested: "^18.3.0", Current: []string{"18.3.1"}, Wanted: "18.3.1", Latest: "19.2.7", Status: "latest_available", PolicyStatus: "blocked-manual", PolicyStrategy: "manual", PolicyMessage: "updates require an explicit manual decision", CandidateMetadataAvailable: true, PackageCount: 2},
		{Name: "react-dom", Kind: "peer", Source: "npm", Requested: "^18.3.0", Current: []string{"18.3.1"}, Wanted: "18.3.1", Latest: "19.2.7", Status: "latest_available", PolicyStatus: "pinned", PolicyStrategy: "pinned", PolicyMessage: "dependency is intended to stay pinned until manifest intent changes", CandidateMetadataAvailable: true, PackageCount: 2},
		{Name: "@tspack-examples/update-policy-utils", Kind: "dep", Source: "workspace", Status: "not_applicable", PolicyStatus: "not-applicable", PolicyMessage: "non-registry dependencies are not evaluated by update policy", PackageCount: 2},
	}}
}

func captureStdoutForTest(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = w
	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close stdout pipe: %v", err)
	}
	os.Stdout = old
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout pipe: %v", err)
	}
	return string(data)
}
