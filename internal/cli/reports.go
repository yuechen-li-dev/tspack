package cli

import (
	"github.com/yuechen-li-dev/tspack/internal/packageidentity"
	"github.com/yuechen-li-dev/tspack/internal/project"
)

type CheckJSONReport struct {
	Command      string                `json:"command"`
	OK           bool                  `json:"ok"`
	Root         string                `json:"root"`
	ManifestPath string                `json:"manifestPath,omitempty"`
	LockfilePath string                `json:"lockfilePath,omitempty"`
	Summary      CheckJSONSummary      `json:"summary"`
	Diagnostics  []CheckJSONDiagnostic `json:"diagnostics"`
}

type CheckJSONSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`
	Total    int `json:"total"`
}

type CheckJSONDiagnostic struct {
	Code                   string      `json:"code"`
	Severity               string      `json:"severity"`
	Message                string      `json:"message"`
	File                   string      `json:"file,omitempty"`
	LifecycleScriptName    string      `json:"lifecycleScriptName,omitempty"`
	LifecycleCategory      string      `json:"lifecycleCategory,omitempty"`
	ConsumerInstallTime    *bool       `json:"consumerInstallTime,omitempty"`
	Acknowledged           *bool       `json:"acknowledged,omitempty"`
	AcknowledgmentKind     *string     `json:"acknowledgmentKind,omitempty"`
	AcknowledgedByCategory string      `json:"acknowledgedByCategory,omitempty"`
	Details                interface{} `json:"details,omitempty"`
	Fixes                  interface{} `json:"fixes,omitempty"`
}

type WhyJSONReport struct {
	Command      string               `json:"command"`
	Mode         string               `json:"mode,omitempty"`
	Query        string               `json:"query"`
	Package      *string              `json:"package"`
	OK           bool                 `json:"ok"`
	Root         string               `json:"root"`
	ManifestPath string               `json:"manifestPath,omitempty"`
	LockfilePath string               `json:"lockfilePath,omitempty"`
	Summary      WhyJSONSummary       `json:"summary"`
	Explanations []WhyJSONExplanation `json:"explanations"`
	LockPackages []WhyJSONLockPackage `json:"lockPackages,omitempty"`
	Reverse      []WhyJSONReversePath `json:"reverse,omitempty"`
	Notes        []string             `json:"notes,omitempty"`
	Diagnostics  []WhyJSONDiagnostic  `json:"diagnostics"`
}

type WhyJSONSummary struct {
	Explanations int `json:"explanations"`
	LockPackages int `json:"lockPackages,omitempty"`
	ReversePaths int `json:"reversePaths,omitempty"`
	Diagnostics  int `json:"diagnostics"`
	Warnings     int `json:"warnings"`
	Errors       int `json:"errors"`
}

type WhyJSONExplanation struct {
	Kind                string                `json:"kind"`
	PackageName         string                `json:"package,omitempty"`
	DependencyKey       string                `json:"dependencyKey,omitempty"`
	DependencyKind      string                `json:"dependencyKind,omitempty"`
	ExternalPackageName string                `json:"externalPackageName,omitempty"`
	TargetName          string                `json:"targetName,omitempty"`
	Optional            bool                  `json:"optional,omitempty"`
	Source              *WhyJSONSource        `json:"source,omitempty"`
	DeclaredBy          []WhyJSONDeclaration  `json:"declaredBy,omitempty"`
	ReachableFrom       []WhyJSONReachability `json:"reachableFrom,omitempty"`
	NotReachableFrom    []WhyJSONReachability `json:"notReachableFrom,omitempty"`
	LockPackages        []WhyJSONLockPackage  `json:"lockPackages,omitempty"`
	LockEdges           []WhyJSONLockEdge     `json:"lockEdges,omitempty"`
	Requirements        []WhyJSONRequirement  `json:"requirements,omitempty"`
	DirectProject       *bool                 `json:"directProject,omitempty"`
}

type WhyJSONRequirement struct {
	Origin          string `json:"origin,omitempty"`
	Kind            string `json:"kind"`
	Constraint      string `json:"constraint"`
	Status          string `json:"status"`
	Target          string `json:"target"`
	Reference       string `json:"reference,omitempty"`
	SelectedVersion string `json:"selectedVersion,omitempty"`
	Controlling     bool   `json:"controlling"`
	Optional        bool   `json:"optional,omitempty"`
}

type WhyJSONSource struct {
	Kind    string `json:"kind,omitempty"`
	Package string `json:"package,omitempty"`
	Range   string `json:"range,omitempty"`
}

type WhyJSONDeclaration struct {
	PackageName   string         `json:"package,omitempty"`
	Scope         string         `json:"scope,omitempty"`
	TargetName    string         `json:"targetName,omitempty"`
	DependencyKey string         `json:"dependencyKey,omitempty"`
	Kind          string         `json:"kind,omitempty"`
	Optional      bool           `json:"optional,omitempty"`
	Source        *WhyJSONSource `json:"source,omitempty"`
}

type WhyJSONReachability struct {
	PackageName string `json:"package"`
	TargetName  string `json:"target"`
	Reason      string `json:"reason"`
	Ref         string `json:"ref"`
}

type WhyJSONLockPackage struct {
	ID           string                        `json:"id"`
	Name         string                        `json:"name,omitempty"`
	Version      string                        `json:"version,omitempty"`
	Source       string                        `json:"source,omitempty"`
	Hash         string                        `json:"hash,omitempty"`
	Capabilities []WhyJSONCapability           `json:"capabilities,omitempty"`
	Usage        *packageidentity.PackageUsage `json:"usage,omitempty"`
}

type WhyJSONCapability struct {
	Kind                  string `json:"kind"`
	Script                string `json:"script,omitempty"`
	Command               string `json:"command,omitempty"`
	Execution             string `json:"execution,omitempty"`
	LifecycleCategory     string `json:"lifecycleCategory,omitempty"`
	ConsumerInstallTime   bool   `json:"consumerInstallTime"`
	Acknowledged          bool   `json:"acknowledged"`
	AcknowledgementReason string `json:"acknowledgementReason,omitempty"`
	BehaviorFixture       string `json:"behaviorFixture,omitempty"`
	BehaviorFixtureStatus string `json:"behaviorFixtureStatus,omitempty"`
	BehaviorReport        string `json:"behaviorReport,omitempty"`
	BehaviorReportStatus  string `json:"behaviorReportStatus,omitempty"`
}

type WhyJSONLockEdge struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Kind      string `json:"kind"`
	Optional  bool   `json:"optional"`
	Reference string `json:"reference,omitempty"`
}

type WhyJSONReversePath struct {
	LockPackage string            `json:"lockPackage"`
	Root        string            `json:"root"`
	Path        []string          `json:"path"`
	Edges       []WhyJSONLockEdge `json:"edges"`
}

type WhyJSONDiagnostic struct {
	Code     string   `json:"code"`
	Severity string   `json:"severity"`
	Message  string   `json:"message"`
	File     string   `json:"file,omitempty"`
	Details  []string `json:"details"`
	Fixes    []string `json:"fixes,omitempty"`
}

type UpdateDryRunJSONReport struct {
	Command     string                         `json:"command"`
	DryRun      UpdateDryRunJSONState          `json:"dryRun"`
	OK          bool                           `json:"ok"`
	Root        string                         `json:"root"`
	Changed     bool                           `json:"changed"`
	Targeted    bool                           `json:"targeted,omitempty"`
	Query       string                         `json:"query,omitempty"`
	Selected    []project.UpdateSelectedTarget `json:"selected,omitempty"`
	Summary     UpdateDryRunSummary            `json:"summary"`
	Changes     UpdateDryRunChanges            `json:"changes"`
	Diagnostics []CheckJSONDiagnostic          `json:"diagnostics"`
}

type PolicyUpdateDryRunJSONReport struct {
	Command     string                `json:"command"`
	DryRun      UpdateDryRunJSONState `json:"dryRun"`
	OK          bool                  `json:"ok"`
	Root        string                `json:"root"`
	PolicyPlan  PolicyUpdatePlanJSON  `json:"policyPlan"`
	Diagnostics []CheckJSONDiagnostic `json:"diagnostics"`
}

type PolicyUpdatePlanJSON struct {
	PolicyPresent          bool                    `json:"policyPresent"`
	WouldUpdate            bool                    `json:"wouldUpdate"`
	WouldApply             bool                    `json:"wouldApply"`
	SecurityGatesEvaluated bool                    `json:"securityGatesEvaluated"`
	SecurityGateStatus     string                  `json:"securityGateStatus"`
	Summary                PolicyUpdatePlanSummary `json:"summary"`
	Allowed                []PolicyUpdateCandidate `json:"allowed"`
	Blocked                []PolicyUpdateCandidate `json:"blocked"`
	Unclassified           []PolicyUpdateCandidate `json:"unclassified"`
	NotApplicable          []PolicyUpdateCandidate `json:"notApplicable"`
	Noop                   []PolicyUpdateCandidate `json:"noop"`
}

type PolicyUpdatePlanSummary struct {
	Allowed         int `json:"allowed"`
	Blocked         int `json:"blocked"`
	Unclassified    int `json:"unclassified"`
	NotApplicable   int `json:"notApplicable"`
	Noop            int `json:"noop"`
	Ready           int `json:"ready"`
	SecurityBlocked int `json:"securityBlocked"`
	ReviewRequired  int `json:"reviewRequired"`
}

type PolicyUpdateCandidate struct {
	Name                    string                                 `json:"name"`
	Kind                    string                                 `json:"kind"`
	Requested               string                                 `json:"requested"`
	Current                 []string                               `json:"current"`
	Wanted                  string                                 `json:"wanted"`
	Latest                  string                                 `json:"latest"`
	Packages                []project.OutdatedPackage              `json:"packages"`
	PackageCount            int                                    `json:"packageCount"`
	PolicyStrategy          string                                 `json:"policyStrategy,omitempty"`
	PolicyLevel             string                                 `json:"policyLevel,omitempty"`
	PolicyStatus            string                                 `json:"policyStatus"`
	PolicyReason            string                                 `json:"policyReason,omitempty"`
	Action                  string                                 `json:"action"`
	EffectiveAction         string                                 `json:"effectiveAction"`
	Message                 string                                 `json:"message"`
	SecurityGateStatus      string                                 `json:"securityGateStatus"`
	SecurityGateReasons     []string                               `json:"securityGateReasons"`
	SecurityGateDiagnostics []project.PolicySecurityGateDiagnostic `json:"securityGateDiagnostics,omitempty"`
}

type UpdateDryRunJSONState struct {
	Enabled bool                `json:"enabled"`
	Changed bool                `json:"changed"`
	Summary UpdateDryRunSummary `json:"summary"`
}

type UpdateDryRunSummary struct {
	Added     int `json:"added"`
	Removed   int `json:"removed"`
	Changed   int `json:"changed"`
	Unchanged int `json:"unchanged"`
}
type UpdateDryRunChanges struct {
	Added   []lockDiffPackage       `json:"added"`
	Removed []lockDiffPackage       `json:"removed"`
	Changed []lockDiffPackageChange `json:"changed"`
}
type lockDiffPackage struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"`
	Direct  bool   `json:"direct"`
}
type lockDiffPackageChange struct {
	From lockDiffPackage `json:"from"`
	To   lockDiffPackage `json:"to"`
}
