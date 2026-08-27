package manifest

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/authoring"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/pathutil"
)

type ManifestIR struct {
	Format         int            `json:"format"`
	Workspace      Workspace      `json:"workspace"`
	Security       Security       `json:"security,omitempty"`
	UpdatePolicy   UpdatePolicy   `json:"updatePolicy,omitempty"`
	RegistryPolicy RegistryPolicy `json:"registryPolicy,omitempty"`
	Packages       []Package      `json:"packages"`
	CompatFiles    []CompatFile   `json:"compatFiles,omitempty"`
}

type RegistryPolicy struct {
	AllowedSources       []string               `json:"allowedSources,omitempty"`
	Offline              bool                   `json:"offline,omitempty"`
	RequireIntegrity     bool                   `json:"requireIntegrity,omitempty"`
	RequireAuditCoverage bool                   `json:"requireAuditCoverage,omitempty"`
	Sources              []RegistrySourcePolicy `json:"sources,omitempty"`
}

type RegistrySourcePolicy struct {
	Kind      string                   `json:"kind"`
	Endpoints []RegistryEndpointPolicy `json:"endpoints"`
}

type RegistryEndpointPolicy struct {
	URL                  string   `json:"url"`
	TokenEnv             string   `json:"tokenEnv,omitempty"`
	FallbackOnNotFound   bool     `json:"fallbackOnNotFound,omitempty"`
	AllowedArtifactHosts []string `json:"allowedArtifactHosts,omitempty"`
}

type CompatFile struct {
	Path   string          `json:"path"`
	Format string          `json:"format"`
	Value  json.RawMessage `json:"value"`
}

type Workspace struct {
	Name             string `json:"name"`
	Runtime          string `json:"runtime"`
	RuntimeSpecified bool   `json:"runtimeSpecified,omitempty"`
}

func (w *Workspace) UnmarshalJSON(data []byte) error {
	type workspaceAlias Workspace
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var alias workspaceAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*w = Workspace(alias)
	if specifiedRaw, ok := raw["runtimeSpecified"]; ok {
		_ = json.Unmarshal(specifiedRaw, &w.RuntimeSpecified)
	} else {
		_, w.RuntimeSpecified = raw["runtime"]
	}
	return nil
}

type Security struct {
	AcknowledgedCapabilities        []AcknowledgedCapability        `json:"acknowledgedCapabilities,omitempty"`
	AcknowledgedLifecycleCategories []AcknowledgedLifecycleCategory `json:"acknowledgedLifecycleCategories,omitempty"`
}

type UpdatePolicy struct {
	Rows []UpdatePolicyRow `json:"rows,omitempty"`
}

type UpdatePolicyRow struct {
	Name              string   `json:"name"`
	Kind              string   `json:"kind"`
	Strategy          string   `json:"strategy"`
	Level             string   `json:"level,omitempty"`
	Reason            string   `json:"reason,omitempty"`
	IncludePrerelease bool     `json:"includePrerelease,omitempty"`
	Packages          []string `json:"packages,omitempty"`
}

type AcknowledgedCapability struct {
	Package         string `json:"package"`
	Kind            string `json:"kind"`
	Script          string `json:"script"`
	Command         string `json:"command"`
	Reason          string `json:"reason"`
	BehaviorFixture string `json:"behaviorFixture,omitempty"`
	BehaviorReport  string `json:"behaviorReport,omitempty"`
}

func (a AcknowledgedCapability) Key() string {
	return a.Package + "|" + a.Kind + "|" + a.Script + "|" + a.Command
}

type AcknowledgedLifecycleCategory struct {
	Category string   `json:"category"`
	Scripts  []string `json:"scripts,omitempty"`
	Reason   string   `json:"reason"`
}

func (a AcknowledgedLifecycleCategory) Key() string {
	return a.Category + "|" + strings.Join(a.Scripts, ",")
}

type Package struct {
	Name                string                    `json:"name"`
	ManifestPath        string                    `json:"manifestPath,omitempty"`
	Version             string                    `json:"version"`
	Compiler            string                    `json:"compiler,omitempty"`
	CompilerPath        string                    `json:"compilerPath,omitempty"`
	Root                string                    `json:"root,omitempty"`
	License             string                    `json:"license,omitempty"`
	Kind                string                    `json:"kind"`
	Policies            Policies                  `json:"policies"`
	Dependencies        []DependencyIntent        `json:"dependencies"`
	DependencyAuthoring *authoring.PackageIR      `json:"dependencyAuthoring,omitempty"`
	DependencyTape      *authoring.TapeResolution `json:"-"`
	Targets             []Target                  `json:"targets"`
	Tools               []string                  `json:"tools"`
	Boundaries          []BoundaryRule            `json:"boundaries"`
	Publish             PublishPolicy             `json:"publish"`
	RunTargets          []RunTarget               `json:"runTargets,omitempty"`
	DevBackend          *DevBackend               `json:"devBackend,omitempty"`
	Skyrim              *SkyrimTarget             `json:"skyrim,omitempty"`
}

type SkyrimTarget struct {
	Name                   string                  `json:"name"`
	Host                   string                  `json:"host"`
	RuntimeVersion         string                  `json:"runtimeVersion"`
	Bridge                 string                  `json:"bridge"`
	NativeConfigure        SkyrimCommand           `json:"nativeConfigure"`
	NativeBuild            SkyrimCommand           `json:"nativeBuild"`
	NativeTests            SkyrimCommand           `json:"nativeTests"`
	NativeDLL              string                  `json:"nativeDll"`
	AssetCompilerProject   string                  `json:"assetCompilerProject"`
	AssetTestsProject      string                  `json:"assetTestsProject"`
	AssetPacks             []SkyrimAssetPack       `json:"assetPacks"`
	AssetOutput            string                  `json:"assetOutput"`
	RuntimeConfig          string                  `json:"runtimeConfig"`
	RuntimeOverrideTarget  string                  `json:"runtimeOverrideTarget,omitempty"`
	RuntimeOverrideFields  []SkyrimRuntimeOverride `json:"runtimeOverrideFields,omitempty"`
	INIOverrideFields      []SkyrimINIOverride     `json:"iniOverrideFields,omitempty"`
	DLLDestination         string                  `json:"dllDestination"`
	ConfigDestination      string                  `json:"configDestination"`
	ExpectedRecords        []SkyrimExpectedRecord  `json:"expectedRecords"`
	StalePlugins           []string                `json:"stalePlugins,omitempty"`
	RuntimeEvidencePattern string                  `json:"runtimeEvidencePattern"`
	ReadyMarker            string                  `json:"readyMarker"`
}

// SkyrimRuntimeOverride describes one authored TOML leaf that a selected,
// machine-local Skyrim host may vary for a run. It is intentionally a typed
// allowlist rather than a general TOML patch language.
type SkyrimRuntimeOverride struct {
	Path   string `json:"path"`
	Type   string `json:"type"`
	Secret bool   `json:"secret,omitempty"`
}

// SkyrimINIOverride describes one Skyrim engine setting that a selected host
// may vary for a bounded run. This is intentionally separate from runtime TOML
// overrides because the target is an engine-owned file.
type SkyrimINIOverride struct {
	Section string `json:"section"`
	Key     string `json:"key"`
	Type    string `json:"type"`
}

type SkyrimCommand struct {
	Command []string `json:"command"`
	Cwd     string   `json:"cwd,omitempty"`
}

type SkyrimAssetPack struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

type SkyrimExpectedRecord struct {
	EditorID    string `json:"editorId"`
	LocalFormID string `json:"localFormId"`
}

// DevBackend is an optional development-only process and proxy contract. It
// describes generic process facts; aspnet is its first dogfood kind, not a
// requirement for browser-only projects.
type DevBackend struct {
	Kind        string          `json:"kind"`
	Command     []string        `json:"command,omitempty"`
	URL         string          `json:"url"`
	Cwd         string          `json:"cwd,omitempty"`
	Ready       *RunReadyCheck  `json:"ready,omitempty"`
	Env         []RunTargetEnv  `json:"env,omitempty"`
	OwnsProcess bool            `json:"ownsProcess"`
	ProxyRoutes []DevProxyRoute `json:"proxyRoutes,omitempty"`
}

type DevProxyRoute struct {
	Path      string `json:"path"`
	Target    string `json:"target,omitempty"`
	WebSocket bool   `json:"webSocket,omitempty"`
	Secure    bool   `json:"secure,omitempty"`
}

type RunTarget struct {
	Name            string                 `json:"name"`
	Runtime         string                 `json:"runtime"`
	ExplicitRuntime string                 `json:"explicitRuntime,omitempty"`
	Command         []string               `json:"command"`
	URL             string                 `json:"url"`
	Cwd             string                 `json:"cwd,omitempty"`
	Ready           *RunReadyCheck         `json:"ready,omitempty"`
	Env             []RunTargetEnv         `json:"env,omitempty"`
	Requires        []RunTargetRequirement `json:"requires,omitempty"`
}

type RunTargetEnv struct {
	Name        string  `json:"name"`
	Required    bool    `json:"required,omitempty"`
	Default     *string `json:"default,omitempty"`
	Secret      bool    `json:"secret,omitempty"`
	Description string  `json:"description,omitempty"`
}

type RunTargetRequirement struct {
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	TCP          string `json:"tcp,omitempty"`
	HTTP         string `json:"http,omitempty"`
	ExpectStatus int    `json:"expectStatus,omitempty"`
	TimeoutMs    int    `json:"timeoutMs,omitempty"`
	Optional     bool   `json:"optional,omitempty"`
	Description  string `json:"description,omitempty"`
}

type RunReadyCheck struct {
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	Host    string `json:"host,omitempty"`
	Port    int    `json:"port,omitempty"`
	Pattern string `json:"pattern,omitempty"`
	Stream  string `json:"stream,omitempty"`
}

type Policies struct {
	Types      map[string]string `json:"types,omitempty"`
	Boundaries map[string]string `json:"boundaries,omitempty"`
}

type DependencyIntent = authoring.EffectiveDependency

type Source = authoring.PackageSource

type Target struct {
	Name              string                `json:"name"`
	Export            string                `json:"export"`
	Entry             string                `json:"entry"`
	Runtime           string                `json:"runtime"`
	Types             string                `json:"types"`
	JavaScriptRuntime string                `json:"javascriptRuntime,omitempty"`
	TsXmlProfile      string                `json:"tsXmlProfile,omitempty"`
	NpmContracts      []CopelandNpmContract `json:"npmContracts,omitempty"`
	Peers             []string              `json:"peers"`
	Deps              []string              `json:"deps"`
	Optional          bool                  `json:"optional,omitempty"`
}

// CopelandNpmContract is static package metadata selected by TSPack after the
// lock graph has resolved a package. It is intentionally not a declaration
// parser and does not teach TSPack Copeland source semantics.
type CopelandNpmContract struct {
	Package    string                         `json:"package"`
	Exports    []CopelandNpmExportContract    `json:"exports"`
	Components []CopelandNpmComponentContract `json:"components,omitempty"`
}

type CopelandNpmExportContract struct {
	Name        string   `json:"name"`
	Parameters  []string `json:"parameters"`
	Result      string   `json:"result"`
	RemoteError string   `json:"remoteError,omitempty"`
	Promise     bool     `json:"promise,omitempty"`
}

type CopelandNpmComponentContract struct {
	Name       string                         `json:"name"`
	Properties []CopelandNpmComponentProperty `json:"properties,omitempty"`
	Members    []CopelandNpmComponentMember   `json:"members,omitempty"`
}

type CopelandNpmComponentMember struct {
	Name       string                         `json:"name"`
	Properties []CopelandNpmComponentProperty `json:"properties,omitempty"`
}

type CopelandNpmComponentProperty struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
}

type BoundaryRule struct {
	From                    string   `json:"from,omitempty"`
	TransitiveFrom          string   `json:"transitiveFrom,omitempty"`
	FromSpecified           bool     `json:"-"`
	TransitiveFromSpecified bool     `json:"-"`
	Allow                   []string `json:"allow,omitempty"`
	Deny                    []string `json:"deny,omitempty"`
	AllowDeps               []string `json:"allowDeps,omitempty"`
	DenyDeps                []string `json:"denyDeps,omitempty"`
	DenyTypeDeps            []string `json:"denyTypeDeps,omitempty"`
	AllowOnly               []string `json:"allowOnly,omitempty"`
	AllowOnlySpecified      bool     `json:"-"`
	AllowTargets            []string `json:"allowTargets,omitempty"`
	DenyTargets             []string `json:"denyTargets,omitempty"`
}

func (b *BoundaryRule) UnmarshalJSON(data []byte) error {
	type boundaryRuleAlias BoundaryRule
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var alias boundaryRuleAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*b = BoundaryRule(alias)
	_, b.FromSpecified = raw["from"]
	_, b.TransitiveFromSpecified = raw["transitiveFrom"]
	_, b.AllowOnlySpecified = raw["allowOnly"]
	return nil
}

type PublishPolicy struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

func LoadBytes(filename string, data []byte) (*ManifestIR, []diag.Diagnostic) {
	var ir ManifestIR
	if err := json.Unmarshal(data, &ir); err != nil {
		return nil, []diag.Diagnostic{{Code: "TSPACK_IR_INVALID_JSON", Severity: diag.SeverityError, Message: "invalid manifest IR JSON", File: filename, Details: []string{err.Error()}}}
	}
	diags := Validate(filename, &ir)
	if len(diags) > 0 {
		return nil, diags
	}
	return &ir, nil
}

func LoadFile(filePath string) (*ManifestIR, []diag.Diagnostic, error) {
	b, err := osReadFile(filePath)
	if err != nil {
		return nil, nil, err
	}
	ir, diags := LoadBytes(filePath, b)
	return ir, diags, nil
}

var osReadFile = func(filePath string) ([]byte, error) { return os.ReadFile(filePath) }

var (
	workspaceNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	pkgNameRe       = regexp.MustCompile(`^(?:@[a-z0-9._-]+/[a-z0-9._-]+|[a-z0-9._-]+)$`)
	versionRe       = regexp.MustCompile(`^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$`)
	targetNameRe    = regexp.MustCompile(`^[A-Za-z0-9_/-]+$`)
	envNameRe       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// Workspace runtime profiles are JavaScript execution runtimes today.
// Future Python-family execution should be modeled through separate runtime
// family, implementation, and environment axes rather than a flat "python"
// value bolted into this JS runtime list.
func isValidRuntimeProfile(value string) bool {
	switch value {
	case "nodejs", "bun", "deno":
		return true
	default:
		return false
	}
}

func isSupportedLifecycleScript(scriptName string) bool {
	switch scriptName {
	case "preinstall", "install", "postinstall", "prepack", "prepare", "postpack", "prepublish", "prepublishOnly", "publish", "postpublish":
		return true
	default:
		return false
	}
}

func isSupportedLifecycleCategory(category string) bool {
	switch category {
	case "consumer-install", "maintainer-publish", "other":
		return true
	default:
		return false
	}
}

func isValidBehaviorFixturePath(filePath string) bool {
	if !isNormalizedSafeProjectPath(filePath) {
		return false
	}
	return strings.HasSuffix(filePath, ".xtest.ts") || strings.HasSuffix(filePath, ".xtest.tsx")
}

func isValidBehaviorReportPath(filePath string) bool {
	if !isNormalizedSafeProjectPath(filePath) {
		return false
	}
	return strings.HasSuffix(filePath, ".json")
}

func validateCompatFile(add func(string, string, ...string), prefix string, compatFile CompatFile) {
	if strings.TrimSpace(compatFile.Path) == "" {
		add("TSPACK_COMPAT_PATH_INVALID", prefix+".path is required")
		return
	}
	if compatFile.Format != "json" {
		add("TSPACK_COMPAT_UNSUPPORTED_FILE", prefix+".format must be json")
	}
	if !isValidCompatPath(compatFile.Path) {
		add("TSPACK_COMPAT_PATH_INVALID", prefix+".path must be a safe project-relative JSON path")
	}
	if isUnsupportedCompatPath(compatFile.Path) {
		add("TSPACK_COMPAT_UNSUPPORTED_FILE", prefix+".path is not supported by M63a compat file management")
	}
	if len(compatFile.Value) == 0 || string(compatFile.Value) == "null" {
		add("TSPACK_COMPAT_VALUE_INVALID", prefix+".value is required")
		return
	}
	var decoded any
	if err := json.Unmarshal(compatFile.Value, &decoded); err != nil {
		add("TSPACK_COMPAT_VALUE_INVALID", prefix+".value must be valid JSON", err.Error())
	}
}

func isValidCompatPath(value string) bool {
	if strings.Contains(value, `\`) || strings.Contains(value, "://") || strings.Contains(value, ":") {
		return false
	}
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "./") || value == "." {
		return false
	}
	if strings.Contains(value, "//") || strings.Contains(value, "/../") || strings.HasPrefix(value, "../") || strings.HasSuffix(value, "/..") {
		return false
	}
	if strings.Contains(value, "/./") || strings.HasSuffix(value, "/") || strings.Contains(value, "//") {
		return false
	}
	if strings.HasPrefix(value, "node_modules/") || strings.HasPrefix(value, ".git/") {
		return false
	}
	return strings.HasSuffix(value, ".json")
}

func isUnsupportedCompatPath(value string) bool {
	switch value {
	case "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "bun.lock", "bun.lock" + "b", "manifest.tsx":
		return true
	default:
		return false
	}
}

func isNormalizedSafeProjectPath(filePath string) bool {
	if !pathutil.IsSafePackageFilePath(filePath) {
		return false
	}
	return path.Clean(filePath) == filePath
}

func Validate(file string, ir *ManifestIR) []diag.Diagnostic { /* shortened? */
	var out []diag.Diagnostic
	add := func(code, msg string, details ...string) {
		out = append(out, diag.Diagnostic{Code: code, Severity: diag.SeverityError, Message: msg, File: file, Details: details})
	}
	if ir.Format != 1 {
		add("TSPACK_IR_UNSUPPORTED_FORMAT", "format must be 1")
	}
	if strings.TrimSpace(ir.Workspace.Name) == "" {
		add("TSPACK_IR_MISSING_WORKSPACE", "workspace.name is required")
	} else if !workspaceNameRe.MatchString(ir.Workspace.Name) || strings.Contains(ir.Workspace.Name, "/") || strings.Contains(ir.Workspace.Name, `\\`) {
		add("TSPACK_IR_INVALID_WORKSPACE_NAME", "invalid workspace.name")
	}
	if strings.TrimSpace(ir.Workspace.Runtime) == "" {
		ir.Workspace.Runtime = "nodejs"
	}
	if !isValidRuntimeProfile(ir.Workspace.Runtime) {
		add(
			"TSPACK_MANIFEST_INVALID_RUNTIME_PROFILE",
			"runtime profile must be nodejs, bun, or deno",
			"value="+ir.Workspace.Runtime,
			"allowed=nodejs,bun,deno",
			"package manager names such as npm/pnpm/yarn are not runtime profiles",
		)
	}
	if len(ir.Packages) == 0 && len(ir.CompatFiles) == 0 {
		add("TSPACK_IR_NO_PACKAGES", "at least one package or root compatibility declaration is required")
	}
	validateRegistryPolicy(add, ir.RegistryPolicy)

	seenCompatPaths := map[string]struct{}{}
	for index, compatFile := range ir.CompatFiles {
		prefix := fmt.Sprintf("compatFiles[%d]", index)
		validateCompatFile(add, prefix, compatFile)
		if compatFile.Path != "" {
			if _, ok := seenCompatPaths[compatFile.Path]; ok {
				add("TSPACK_COMPAT_DUPLICATE_FILE", "duplicate compat file path: "+compatFile.Path)
			}
			seenCompatPaths[compatFile.Path] = struct{}{}
		}
	}

	seenAcknowledgedCapabilities := map[string]struct{}{}
	for index, acknowledged := range ir.Security.AcknowledgedCapabilities {
		prefix := fmt.Sprintf("security.acknowledgedCapabilities[%d]", index)
		if strings.TrimSpace(acknowledged.Package) == "" {
			add("TSPACK_SECURITY_INVALID_ACKNOWLEDGED_CAPABILITY", prefix+".package is required")
		}
		if acknowledged.Kind != "lifecycleScript" {
			add("TSPACK_SECURITY_INVALID_ACKNOWLEDGED_CAPABILITY", prefix+".kind must be lifecycleScript")
		}
		if !isSupportedLifecycleScript(acknowledged.Script) {
			add("TSPACK_SECURITY_INVALID_ACKNOWLEDGED_CAPABILITY", prefix+".script is not a supported lifecycle script")
		}
		if strings.TrimSpace(acknowledged.Command) == "" {
			add("TSPACK_SECURITY_INVALID_ACKNOWLEDGED_CAPABILITY", prefix+".command is required")
		}
		if strings.TrimSpace(acknowledged.Reason) == "" {
			add("TSPACK_SECURITY_INVALID_ACKNOWLEDGED_CAPABILITY", prefix+".reason is required")
		}
		if acknowledged.BehaviorFixture != "" {
			if !isValidBehaviorFixturePath(acknowledged.BehaviorFixture) {
				add("TSPACK_SECURITY_INVALID_BEHAVIOR_FIXTURE", prefix+".behaviorFixture must be a safe relative .xtest.ts or .xtest.tsx path")
			}
		}
		if acknowledged.BehaviorReport != "" {
			if !isValidBehaviorReportPath(acknowledged.BehaviorReport) {
				add("TSPACK_SECURITY_INVALID_BEHAVIOR_REPORT", prefix+".behaviorReport must be a safe relative .json path")
			}
		}
		key := acknowledged.Key()
		if _, ok := seenAcknowledgedCapabilities[key]; ok {
			add("TSPACK_SECURITY_DUPLICATE_ACKNOWLEDGED_CAPABILITY", "duplicate acknowledged capability: "+key)
		}
		seenAcknowledgedCapabilities[key] = struct{}{}
	}
	seenAcknowledgedLifecycleCategories := map[string]struct{}{}
	for index, acknowledged := range ir.Security.AcknowledgedLifecycleCategories {
		prefix := fmt.Sprintf("security.acknowledgedLifecycleCategories[%d]", index)
		if !isSupportedLifecycleCategory(acknowledged.Category) {
			add("TSPACK_SECURITY_INVALID_ACKNOWLEDGED_LIFECYCLE_CATEGORY", prefix+".category must be consumer-install, maintainer-publish, or other")
		}
		if strings.TrimSpace(acknowledged.Reason) == "" {
			add("TSPACK_SECURITY_INVALID_ACKNOWLEDGED_LIFECYCLE_CATEGORY", prefix+".reason is required")
		}
		for scriptIndex, script := range acknowledged.Scripts {
			if !isSupportedLifecycleScript(script) {
				add("TSPACK_SECURITY_INVALID_ACKNOWLEDGED_LIFECYCLE_CATEGORY", fmt.Sprintf("%s.scripts[%d] is not a supported lifecycle script", prefix, scriptIndex))
			}
		}
		key := acknowledged.Key()
		if _, ok := seenAcknowledgedLifecycleCategories[key]; ok {
			add("TSPACK_SECURITY_DUPLICATE_ACKNOWLEDGED_LIFECYCLE_CATEGORY", "duplicate acknowledged lifecycle category: "+key)
		}
		seenAcknowledgedLifecycleCategories[key] = struct{}{}
	}

	seenUpdatePolicyRows := map[string]struct{}{}
	for index, row := range ir.UpdatePolicy.Rows {
		prefix := fmt.Sprintf("updatePolicy.rows[%d]", index)
		if strings.TrimSpace(row.Name) == "" {
			add("TSPACK_UPDATE_POLICY_INVALID_ROW", prefix+".name is required")
		}
		if row.Kind != "tool" && row.Kind != "dep" && row.Kind != "peer" && row.Kind != "any" {
			add("TSPACK_UPDATE_POLICY_INVALID_KIND", prefix+".kind must be tool, dep, peer, or any")
		}
		if row.Strategy != "manual" && row.Strategy != "pinned" && row.Strategy != "rolling" {
			add("TSPACK_UPDATE_POLICY_INVALID_STRATEGY", prefix+".strategy must be manual, pinned, or rolling")
		}
		if row.Strategy == "rolling" {
			if row.Level != "patch" && row.Level != "minor" && row.Level != "major" && row.Level != "latest" {
				add("TSPACK_UPDATE_POLICY_INVALID_LEVEL", prefix+".level must be patch, minor, major, or latest for rolling policy")
			}
		} else if row.Level != "" {
			add("TSPACK_UPDATE_POLICY_LEVEL_NOT_ALLOWED", prefix+".level is only allowed for rolling policy")
		}
		for packageIndex, packageName := range row.Packages {
			if strings.TrimSpace(packageName) == "" {
				add("TSPACK_UPDATE_POLICY_INVALID_PACKAGE_SCOPE", fmt.Sprintf("%s.packages[%d] must be a non-empty string", prefix, packageIndex))
			}
		}
		scopedPackages := append([]string(nil), row.Packages...)
		sort.Strings(scopedPackages)
		key := row.Name + "|" + row.Kind + "|" + strings.Join(scopedPackages, ",")
		if _, ok := seenUpdatePolicyRows[key]; ok {
			add("TSPACK_UPDATE_POLICY_DUPLICATE_ROW", "duplicate update policy row: "+key)
		}
		seenUpdatePolicyRows[key] = struct{}{}
	}

	seenPkg := map[string]struct{}{}
	skyrimTargetCount := 0
	for pi := range ir.Packages {
		p := &ir.Packages[pi]
		pp := fmt.Sprintf("packages[%d]", pi)
		if !pkgNameRe.MatchString(p.Name) {
			add("TSPACK_IR_INVALID_PACKAGE_NAME", pp+".name is invalid")
		}
		if !versionRe.MatchString(p.Version) {
			add("TSPACK_IR_INVALID_PACKAGE_VERSION", pp+".version is invalid")
		}
		if !isValidPackageKind(p.Kind) {
			add("TSPACK_IR_INVALID_PACKAGE_KIND", pp+".kind is invalid")
		}
		if p.Compiler == "" {
			p.Compiler = "tsc"
		}
		if p.Compiler != "tsc" && p.Compiler != "tscl" {
			add("TSPACK_MANIFEST_INVALID_COMPILER", pp+".compiler must be tsc or tscl")
		}
		if p.Compiler == "tscl" && strings.TrimSpace(p.CompilerPath) == "" {
			add("TSPACK_TSCL_PATH_REQUIRED", pp+".compilerPath is required when compiler is tscl")
		}
		if p.Root != "" && !pathutil.IsSafePackageRoot(p.Root) {
			add("TSPACK_IR_INVALID_PACKAGE_ROOT", pp+".root must be a safe relative path")
		}
		if _, ok := seenPkg[p.Name]; ok {
			add("TSPACK_IR_DUPLICATE_PACKAGE", "duplicate package name: "+p.Name)
		}
		seenPkg[p.Name] = struct{}{}
		preparePackageAuthoring(add, pp, p)
		depKinds := map[string]string{}
		for i, d := range p.Dependencies {
			k := depIdentity(d)
			if k == "" {
				add("TSPACK_IR_DUPLICATE_DEPENDENCY", fmt.Sprintf("%s.dependencies[%d] missing key", pp, i))
				continue
			}
			if _, ok := depKinds[k]; ok {
				add("TSPACK_IR_DUPLICATE_DEPENDENCY", "duplicate dependency key: "+k)
			}
			depKinds[k] = d.Kind
			validateDep(add, pp, i, d)
		}
		seenTarget := map[string]struct{}{}
		seenExport := map[string]struct{}{}
		for ti, t := range p.Targets {
			tp := fmt.Sprintf("%s.targets[%d]", pp, ti)
			if t.Name == "" || !targetNameRe.MatchString(t.Name) || strings.HasPrefix(t.Name, "/") || strings.HasSuffix(t.Name, "/") || strings.Contains(t.Name, "..") {
				add("TSPACK_IR_INVALID_TARGET_NAME", tp+".name is invalid")
			}
			if _, ok := seenTarget[t.Name]; ok {
				add("TSPACK_IR_DUPLICATE_TARGET", "duplicate target name: "+t.Name)
			}
			seenTarget[t.Name] = struct{}{}
			if t.Export != "." && (!strings.HasPrefix(t.Export, "./") || strings.Contains(t.Export, "..")) {
				add("TSPACK_IR_INVALID_EXPORT_PATH", tp+".export is invalid")
			}
			if _, ok := seenExport[t.Export]; ok {
				add("TSPACK_IR_DUPLICATE_EXPORT", "duplicate export path: "+t.Export)
			}
			seenExport[t.Export] = struct{}{}
			if t.JavaScriptRuntime != "" && t.JavaScriptRuntime != "node" && t.JavaScriptRuntime != "browser" {
				add("TSPACK_IR_INVALID_JAVASCRIPT_RUNTIME", tp+".javascriptRuntime must be node or browser")
			}
			for _, f := range []struct{ name, v string }{{"entry", t.Entry}, {"runtime", t.Runtime}, {"types", t.Types}} {
				if p.Kind == "app" && f.name == "types" && f.v == "" {
					continue
				}
				if !pathutil.IsSafePackageFilePath(f.v) {
					add("TSPACK_IR_INVALID_RELATIVE_PATH", tp+"."+f.name+" must be a safe relative path")
				}
			}
			for _, ref := range t.Peers {
				if _, ok := depKinds[ref]; !ok {
					add("TSPACK_IR_UNKNOWN_DEPENDENCY_REF", tp+".peers has unknown dependency: "+ref)
				}
			}
			for _, ref := range t.Deps {
				if _, ok := depKinds[ref]; !ok {
					add("TSPACK_IR_UNKNOWN_DEPENDENCY_REF", tp+".deps has unknown dependency: "+ref)
				}
			}
		}
		for _, tool := range p.Tools {
			kind, ok := depKinds[tool]
			if !ok {
				add("TSPACK_IR_UNKNOWN_DEPENDENCY_REF", pp+".tools has unknown dependency: "+tool)
			} else if kind != "tool" {
				add("TSPACK_IR_INVALID_DEPENDENCY_KIND", pp+".tools must reference tool dependencies")
			}
		}
		validateDevBackend(add, pp, p.DevBackend)
		validateSkyrimTarget(add, pp, p.Skyrim)
		if p.Skyrim != nil {
			skyrimTargetCount++
			if skyrimTargetCount > 1 {
				add("TSPACK_SKYRIM_BRIDGE_DUPLICATE", "only one Skyrim bridge target may be declared per workspace")
			}
		}
		seenRunTargets := map[string]struct{}{}
		for ri, rt := range p.RunTargets {
			rp := fmt.Sprintf("%s.runTargets[%d]", pp, ri)
			if rt.Name == "" || !targetNameRe.MatchString(rt.Name) {
				add("TSPACK_RUN_INVALID_TARGET", rp+".name is invalid")
			}
			if _, ok := seenRunTargets[rt.Name]; ok {
				add("TSPACK_RUN_DUPLICATE_TARGET", "duplicate run target name: "+rt.Name)
			}
			seenRunTargets[rt.Name] = struct{}{}
			if rt.Runtime != "" && rt.Runtime != "system" && rt.Runtime != "node" && rt.Runtime != "bun" && rt.Runtime != "deno" {
				add("TSPACK_RUN_INVALID_RUNTIME", rp+".runtime is invalid")
			}
			if rt.ExplicitRuntime != "" && rt.ExplicitRuntime != rt.Runtime {
				add("TSPACK_RUN_INVALID_RUNTIME", rp+".explicitRuntime must match runtime")
			}
			if rt.Cwd != "" && rt.Cwd != "workspace" && rt.Cwd != "package" {
				add("TSPACK_RUN_INVALID_CWD", rp+".cwd must be workspace or package")
			}
			if len(rt.Command) == 0 {
				add("TSPACK_RUN_INVALID_COMMAND", rp+".command must be non-empty")
			}
			for _, part := range rt.Command {
				if strings.TrimSpace(part) == "" {
					add("TSPACK_RUN_INVALID_COMMAND", rp+".command entries must be non-empty")
				}
			}
			if !runTargetURLIsOptional(rt) {
				u, err := url.Parse(runTargetURLForValidation(rt.URL))
				if err != nil || !validRunReadyURLPlaceholders(rt.URL) || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
					add("TSPACK_RUN_INVALID_URL", rp+".url must be valid http/https URL")
				}
			} else if strings.TrimSpace(rt.URL) != "" {
				u, err := url.Parse(runTargetURLForValidation(rt.URL))
				if err != nil || !validRunReadyURLPlaceholders(rt.URL) || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
					add("TSPACK_RUN_INVALID_URL", rp+".url must be valid http/https URL")
				}
			}
			if rt.Ready != nil && !validRunReadyCheck(rt.Ready) {
				add("TSPACK_RUN_INVALID_READY", rp+".ready is invalid")
			}
			seenEnv := map[string]string{}
			for ei, env := range rt.Env {
				ep := fmt.Sprintf("%s.env[%d]", rp, ei)
				if env.Name == "" || !envNameRe.MatchString(env.Name) {
					add("TSPACK_MANIFEST_ENV_INVALID", ep+".name is invalid")
				}
				key := strings.ToUpper(env.Name)
				if previous, ok := seenEnv[key]; ok {
					add("TSPACK_MANIFEST_ENV_DUPLICATE", "duplicate run target env name: "+previous+" and "+env.Name)
				}
				seenEnv[key] = env.Name
			}
			validateRunTargetRequirements(add, rp, rt.Requires)
		}
		for k, v := range p.Policies.Types {
			if !isValidTypePolicy(k, v) {
				add("TSPACK_IR_INVALID_POLICY", "invalid types policy: "+k)
			}
		}
		for k, v := range p.Policies.Boundaries {
			if !isValidBoundaryPolicy(k, v) {
				add("TSPACK_IR_INVALID_POLICY", "invalid boundaries policy: "+k)
			}
		}
		for bi, b := range p.Boundaries {
			bp := fmt.Sprintf("%s.boundaries[%d]", pp, bi)
			if scopeSpecified(b.From, b.FromSpecified) && scopeSpecified(b.TransitiveFrom, b.TransitiveFromSpecified) {
				add("TSPACK_BOUNDARY_INVALID_SCOPE", bp+" cannot specify both from and transitiveFrom")
			}
			if scopeSpecified(b.From, b.FromSpecified) && !isValidBoundaryScopePattern(b.From) {
				add("TSPACK_IR_INVALID_BOUNDARY", bp+".from is invalid")
			}
			if scopeSpecified(b.TransitiveFrom, b.TransitiveFromSpecified) && !isValidBoundaryScopePattern(b.TransitiveFrom) {
				add("TSPACK_BOUNDARY_INVALID_TRANSITIVE_FROM", bp+".transitiveFrom is invalid")
			}
			for ai, allowed := range b.AllowOnly {
				if strings.TrimSpace(allowed) == "" {
					add("TSPACK_BOUNDARY_INVALID_ALLOW_ONLY", fmt.Sprintf("%s.allowOnly[%d] must be a non-empty string", bp, ai))
				}
			}
			for di, denied := range b.DenyTypeDeps {
				if strings.TrimSpace(denied) == "" {
					add("TSPACK_BOUNDARY_INVALID_DENY_TYPE_DEPS", fmt.Sprintf("%s.denyTypeDeps[%d] must be a non-empty string", bp, di))
				}
			}
		}
		if p.Kind == "library" && len(p.Publish.Include) == 0 {
			add("TSPACK_IR_INVALID_PUBLISH_POLICY", pp+".publish.include required for library")
		}
		for _, it := range append(append([]string{}, p.Publish.Include...), p.Publish.Exclude...) {
			if !pathutil.IsSafePackageFilePath(it) && !pathutil.IsSafeRelativeGlob(it) {
				add("TSPACK_IR_INVALID_PUBLISH_POLICY", "publish path is invalid: "+it)
			}
		}
	}
	diag.SortDiagnostics(out)
	return out
}

func validateRegistryPolicy(add func(string, string, ...string), policy RegistryPolicy) {
	seenAllowed := map[string]bool{}
	for index, source := range policy.AllowedSources {
		if source != "npm" && source != "jsr" {
			add("TSPACK_SOURCE_POLICY_INVALID", fmt.Sprintf("registryPolicy.allowedSources[%d] must be npm or jsr", index))
		}
		if seenAllowed[source] {
			add("TSPACK_SOURCE_POLICY_INVALID", "registryPolicy.allowedSources contains duplicate source: "+source)
		}
		seenAllowed[source] = true
	}
	seenSources := map[string]bool{}
	for sourceIndex, source := range policy.Sources {
		prefix := fmt.Sprintf("registryPolicy.sources[%d]", sourceIndex)
		if source.Kind != "npm" && source.Kind != "jsr" {
			add("TSPACK_SOURCE_POLICY_INVALID", prefix+".kind must be npm or jsr")
		}
		if seenSources[source.Kind] {
			add("TSPACK_SOURCE_POLICY_INVALID", "registryPolicy contains duplicate source: "+source.Kind)
		}
		seenSources[source.Kind] = true
		if len(source.Endpoints) == 0 {
			add("TSPACK_SOURCE_POLICY_INVALID", prefix+".endpoints must not be empty")
		}
		for endpointIndex, endpoint := range source.Endpoints {
			endpointPrefix := fmt.Sprintf("%s.endpoints[%d]", prefix, endpointIndex)
			parsed, err := url.Parse(endpoint.URL)
			if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil {
				add("TSPACK_SOURCE_POLICY_INVALID", endpointPrefix+".url must be an HTTP(S) URL without embedded credentials")
			}
			if parsed != nil && hasSensitiveRegistryQuery(parsed) {
				add("TSPACK_SOURCE_POLICY_INVALID", endpointPrefix+".url must not contain credential-bearing query parameters; use tokenEnv")
			}
			if endpoint.TokenEnv != "" && !envNameRe.MatchString(endpoint.TokenEnv) {
				add("TSPACK_SOURCE_POLICY_INVALID", endpointPrefix+".tokenEnv must be an environment variable name")
			}
			for hostIndex, host := range endpoint.AllowedArtifactHosts {
				if strings.TrimSpace(host) == "" || strings.ContainsAny(host, "/:@?") {
					add("TSPACK_SOURCE_POLICY_INVALID", fmt.Sprintf("%s.allowedArtifactHosts[%d] must be a hostname", endpointPrefix, hostIndex))
				}
			}
		}
	}
}

func hasSensitiveRegistryQuery(parsed *url.URL) bool {
	for key := range parsed.Query() {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "auth") || strings.Contains(lower, "key") {
			return true
		}
	}
	return false
}

func preparePackageAuthoring(
	add func(string, string, ...string),
	packagePath string,
	pkg *Package,
) {
	if pkg.DependencyAuthoring == nil {
		declarations := make([]authoring.DependencyDeclaration, 0, len(pkg.Dependencies))
		for order, dependency := range pkg.Dependencies {
			declarations = append(declarations, authoring.DependencyDeclaration{
				Key:         dependency.Key,
				Identity:    dependency.Source.Identity(),
				Source:      dependency.Source,
				Constraint:  dependency.Source.Range,
				Kind:        authoring.DependencyKind(dependency.Kind),
				Optional:    dependency.Optional,
				Origin:      authoring.DeclarationOrigin{Kind: authoring.OriginGenerated, Name: "legacy normalized manifest IR"},
				Layer:       authoring.LayerProject,
				Order:       order,
				Authority:   authoring.AuthorityGenerated,
				Editability: authoring.EditabilityDerived,
			})
		}
		pkg.DependencyAuthoring = &authoring.PackageIR{Declarations: declarations}
	}

	for index := range pkg.DependencyAuthoring.Declarations {
		declaration := &pkg.DependencyAuthoring.Declarations[index]
		if !declaration.Identity.Valid() {
			declaration.Identity = declaration.Source.Identity()
		}
		if !declaration.Identity.Valid() {
			add(
				"TSPACK_AUTHORING_INVALID_IDENTITY",
				fmt.Sprintf("%s.dependencyAuthoring.declarations[%d] has no package identity", packagePath, index),
			)
		}
		if declaration.Source.Kind != declaration.Identity.Source {
			add(
				"TSPACK_AUTHORING_SOURCE_IDENTITY_MISMATCH",
				fmt.Sprintf("%s.dependencyAuthoring.declarations[%d] source and identity differ", packagePath, index),
				"source="+declaration.Source.Kind,
				"identity="+declaration.Identity.Source,
			)
		}
	}

	resolution := authoring.Build(pkg.DependencyAuthoring.Declarations)
	pkg.DependencyTape = &resolution
	pkg.Dependencies = append([]authoring.EffectiveDependency(nil), resolution.Effective...)
	for _, conflict := range resolution.Conflicts {
		if !conflict.Fatal {
			continue
		}
		add(
			"TSPACK_AUTHORING_DEPENDENCY_KEY_COLLISION",
			packagePath+" has dependency declarations with colliding effective keys",
			conflict.Message,
		)
	}
}

func validateSkyrimTarget(add func(string, string, ...string), packagePath string, target *SkyrimTarget) {
	if target == nil {
		return
	}
	prefix := packagePath + ".skyrim"
	if strings.TrimSpace(target.Name) == "" || !targetNameRe.MatchString(target.Name) {
		add("TSPACK_SKYRIM_TARGET_INVALID", prefix+".name is invalid")
	}
	if strings.TrimSpace(target.Host) == "" || !targetNameRe.MatchString(target.Host) {
		add("TSPACK_SKYRIM_HOST_INVALID", prefix+".host is invalid")
	}
	if strings.TrimSpace(target.RuntimeVersion) == "" {
		add("TSPACK_SKYRIM_RUNTIME_INVALID", prefix+".runtimeVersion is required")
	}
	if target.Bridge != "MarionetteSSE.esp" {
		add("TSPACK_SKYRIM_BRIDGE_INVALID", prefix+".bridge must be MarionetteSSE.esp")
	}
	if target.AssetOutput != "build/assets/MarionetteSSE.esp" {
		add("TSPACK_SKYRIM_BRIDGE_INVALID", prefix+".assetOutput must be build/assets/MarionetteSSE.esp")
	}
	validateSkyrimCommand(add, prefix+".nativeConfigure", target.NativeConfigure)
	validateSkyrimCommand(add, prefix+".nativeBuild", target.NativeBuild)
	validateSkyrimCommand(add, prefix+".nativeTests", target.NativeTests)
	for field, value := range map[string]string{
		"nativeDll":              target.NativeDLL,
		"assetCompilerProject":   target.AssetCompilerProject,
		"assetTestsProject":      target.AssetTestsProject,
		"assetOutput":            target.AssetOutput,
		"runtimeConfig":          target.RuntimeConfig,
		"runtimeEvidencePattern": target.RuntimeEvidencePattern,
	} {
		if !pathutil.IsSafePackageFilePath(value) && !pathutil.IsSafeRelativeGlob(value) {
			add("TSPACK_SKYRIM_PATH_INVALID", prefix+"."+field+" must be a safe project-relative path")
		}
	}
	if target.DLLDestination != "SKSE/Plugins/MarionetteSSE.dll" {
		add("TSPACK_SKYRIM_DESTINATION_INVALID", prefix+".dllDestination must be SKSE/Plugins/MarionetteSSE.dll")
	}
	if target.ConfigDestination != "SKSE/Plugins/MarionetteSSE.toml" {
		add("TSPACK_SKYRIM_DESTINATION_INVALID", prefix+".configDestination must be SKSE/Plugins/MarionetteSSE.toml")
	}
	if len(target.RuntimeOverrideFields) > 0 && (strings.TrimSpace(target.RuntimeOverrideTarget) == "" || !targetNameRe.MatchString(target.RuntimeOverrideTarget)) {
		add("TSPACK_SKYRIM_OVERRIDE_TARGET_INVALID", prefix+".runtimeOverrideTarget is required and must be a valid target name when runtimeOverrideFields are declared")
	}
	seenOverridePaths := map[string]struct{}{}
	for index, field := range target.RuntimeOverrideFields {
		fieldPath := fmt.Sprintf("%s.runtimeOverrideFields[%d]", prefix, index)
		if strings.TrimSpace(field.Path) == "" || strings.HasPrefix(field.Path, ".") || strings.HasSuffix(field.Path, ".") || strings.Contains(field.Path, "..") {
			add("TSPACK_SKYRIM_OVERRIDE_PATH_INVALID", fieldPath+".path must be a non-empty dotted TOML path")
		}
		if _, ok := seenOverridePaths[field.Path]; ok {
			add("TSPACK_SKYRIM_OVERRIDE_DUPLICATE", "duplicate Skyrim runtime override path: "+field.Path)
		}
		seenOverridePaths[field.Path] = struct{}{}
		if field.Type != "boolean" && field.Type != "string" && field.Type != "integer" {
			add("TSPACK_SKYRIM_OVERRIDE_TYPE_INVALID", fieldPath+".type must be boolean, string, or integer")
		}
	}
	if len(target.INIOverrideFields) > 1 {
		add("TSPACK_SKYRIM_INI_OVERRIDE_INVALID", prefix+".iniOverrideFields supports only the declared General.bAlwaysActive setting")
	}
	for index, field := range target.INIOverrideFields {
		fieldPath := fmt.Sprintf("%s.iniOverrideFields[%d]", prefix, index)
		if field.Section != "General" || field.Key != "bAlwaysActive" || field.Type != "boolean" {
			add("TSPACK_SKYRIM_INI_OVERRIDE_INVALID", fieldPath+" must declare General.bAlwaysActive as boolean")
		}
	}
	seenPacks := map[string]struct{}{}
	for index, pack := range target.AssetPacks {
		packPath := fmt.Sprintf("%s.assetPacks[%d]", prefix, index)
		if strings.TrimSpace(pack.Name) == "" {
			add("TSPACK_SKYRIM_ASSET_PACK_INVALID", packPath+".name is required")
		}
		if _, ok := seenPacks[pack.Name]; ok {
			add("TSPACK_SKYRIM_ASSET_PACK_DUPLICATE", "duplicate Skyrim asset pack: "+pack.Name)
		}
		seenPacks[pack.Name] = struct{}{}
		if !pathutil.IsSafePackageFilePath(pack.Source) {
			add("TSPACK_SKYRIM_PATH_INVALID", packPath+".source must be a safe project-relative path")
		}
	}
	if len(target.AssetPacks) == 0 {
		add("TSPACK_SKYRIM_ASSET_PACK_INVALID", prefix+".assetPacks must be non-empty")
	}
	if len(target.ExpectedRecords) == 0 {
		add("TSPACK_SKYRIM_RECORDS_INVALID", prefix+".expectedRecords must be non-empty")
	}
	if strings.TrimSpace(target.ReadyMarker) == "" {
		add("TSPACK_SKYRIM_EVIDENCE_INVALID", prefix+".readyMarker is required")
	}
}

func validateSkyrimCommand(add func(string, string, ...string), prefix string, command SkyrimCommand) {
	if len(command.Command) == 0 {
		add("TSPACK_SKYRIM_COMMAND_INVALID", prefix+".command must be non-empty")
	}
	for _, part := range command.Command {
		if strings.TrimSpace(part) == "" {
			add("TSPACK_SKYRIM_COMMAND_INVALID", prefix+".command entries must be non-empty")
		}
	}
	if command.Cwd != "" && command.Cwd != "workspace" && command.Cwd != "package" {
		add("TSPACK_SKYRIM_COMMAND_INVALID", prefix+".cwd must be workspace or package")
	}
}

func validateDevBackend(add func(string, string, ...string), packagePath string, backend *DevBackend) {
	if backend == nil {
		return
	}
	backendPath := packagePath + ".devBackend"
	if backend.Kind != "process" && backend.Kind != "aspnet" {
		add("TSPACK_VITE_BACKEND_KIND_INVALID", backendPath+".kind must be process or aspnet")
	}
	if backend.Cwd != "" && backend.Cwd != "workspace" && backend.Cwd != "package" {
		add("TSPACK_VITE_BACKEND_CWD_INVALID", backendPath+".cwd must be workspace or package")
	}
	if backend.OwnsProcess && len(backend.Command) == 0 {
		add("TSPACK_VITE_BACKEND_COMMAND_MISSING", backendPath+".command is required when ownsProcess is true")
	}
	for _, part := range backend.Command {
		if strings.TrimSpace(part) == "" {
			add("TSPACK_VITE_BACKEND_COMMAND_INVALID", backendPath+".command entries must be non-empty")
		}
	}
	if strings.TrimSpace(backend.URL) == "" {
		add("TSPACK_VITE_BACKEND_URL_INVALID", backendPath+".url is required")
	} else {
		u, err := url.Parse(runTargetURLForValidation(backend.URL))
		if err != nil || !validRunReadyURLPlaceholders(backend.URL) || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			add("TSPACK_VITE_BACKEND_URL_INVALID", backendPath+".url must be a valid http/https URL")
		}
	}
	if backend.OwnsProcess && backend.Ready == nil {
		add("TSPACK_VITE_BACKEND_READY_MISSING", backendPath+".ready is required when ownsProcess is true")
	} else if backend.Ready != nil && !validRunReadyCheck(backend.Ready) {
		add("TSPACK_VITE_BACKEND_READY_INVALID", backendPath+".ready is invalid")
	}
	seenPaths := map[string]struct{}{}
	for routeIndex, route := range backend.ProxyRoutes {
		routePath := fmt.Sprintf("%s.proxyRoutes[%d]", backendPath, routeIndex)
		if !strings.HasPrefix(route.Path, "/") || strings.Contains(route.Path, " ") {
			add("TSPACK_VITE_PROXY_ROUTE_INVALID", routePath+".path must start with /")
		}
		if _, found := seenPaths[route.Path]; found {
			add("TSPACK_VITE_PROXY_ROUTE_DUPLICATE", "duplicate proxy route: "+route.Path)
		}
		seenPaths[route.Path] = struct{}{}
		if route.Target == "" {
			continue
		}
		u, err := url.Parse(runTargetURLForValidation(route.Target))
		if err != nil || !validRunReadyURLPlaceholders(route.Target) || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "ws" && u.Scheme != "wss") || u.Host == "" {
			add("TSPACK_VITE_PROXY_TARGET_INVALID", routePath+".target must be a valid http/https/ws/wss URL")
		}
	}
}

func isValidPackageKind(kind string) bool {
	switch kind {
	case "library", "app", "tool", "service":
		return true
	default:
		return false
	}
}

func validateRunTargetRequirements(add func(string, string, ...string), rp string, requires []RunTargetRequirement) {
	seenServices := map[string]string{}
	for i, req := range requires {
		reqPath := fmt.Sprintf("%s.requires[%d]", rp, i)
		if req.Kind != "" && req.Kind != "service" {
			add("TSPACK_MANIFEST_SERVICE_INVALID", reqPath+".kind must be service")
		}
		if strings.TrimSpace(req.Name) == "" {
			add("TSPACK_MANIFEST_SERVICE_INVALID", reqPath+".name must be non-empty")
		}
		key := strings.ToLower(req.Name)
		if previous, ok := seenServices[key]; ok {
			add("TSPACK_MANIFEST_SERVICE_DUPLICATE", "duplicate run target service requirement name: "+previous+" and "+req.Name)
		}
		seenServices[key] = req.Name

		hasTCP := strings.TrimSpace(req.TCP) != ""
		hasHTTP := strings.TrimSpace(req.HTTP) != ""
		if hasTCP == hasHTTP {
			add("TSPACK_MANIFEST_SERVICE_INVALID", reqPath+" must provide exactly one of tcp or http")
		}
		if hasTCP && !validRunTargetServiceTCP(req.TCP) {
			add("TSPACK_MANIFEST_SERVICE_INVALID", reqPath+".tcp must be host:port with port 1-65535")
		}
		if hasHTTP && !validRunTargetServiceHTTP(req.HTTP) {
			add("TSPACK_MANIFEST_SERVICE_INVALID", reqPath+".http must be an http/https URL")
		}
		if req.ExpectStatus != 0 && (req.ExpectStatus < 100 || req.ExpectStatus > 599) {
			add("TSPACK_MANIFEST_SERVICE_INVALID", reqPath+".expectStatus must be between 100 and 599")
		}
		if req.TimeoutMs != 0 && (req.TimeoutMs < 1 || req.TimeoutMs > 60000) {
			add("TSPACK_MANIFEST_SERVICE_INVALID", reqPath+".timeoutMs must be between 1 and 60000")
		}
	}
}

func runTargetURLForValidation(raw string) string {
	var builder strings.Builder
	for index := 0; index < len(raw); {
		if raw[index] == '$' && index+1 < len(raw) && raw[index+1] == '{' {
			end := strings.IndexByte(raw[index+2:], '}')
			if end < 0 {
				return raw
			}
			builder.WriteString("3000")
			index += end + 3
			continue
		}
		builder.WriteByte(raw[index])
		index++
	}
	return builder.String()
}

func validRunReadyURLPlaceholders(raw string) bool {
	for index := 0; index < len(raw); {
		if raw[index] != '$' || index+1 >= len(raw) || raw[index+1] != '{' {
			index++
			continue
		}
		end := strings.IndexByte(raw[index+2:], '}')
		if end < 0 {
			return false
		}
		name := raw[index+2 : index+2+end]
		if !envNameRe.MatchString(name) {
			return false
		}
		index += end + 3
	}
	return true
}

func validRunTargetServiceTCP(endpoint string) bool {
	host, portText, err := net.SplitHostPort(endpoint)
	if err != nil {
		host, portText, err = net.SplitHostPort("host:" + endpoint)
		if err != nil || host != "host" {
			return false
		}
	}
	if strings.TrimSpace(host) == "" {
		return false
	}
	port, err := strconv.Atoi(portText)
	return err == nil && port >= 1 && port <= 65535
}

func validRunTargetServiceHTTP(endpoint string) bool {
	u, err := url.Parse(endpoint)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func runTargetURLIsOptional(rt RunTarget) bool {
	if rt.Ready == nil {
		return true
	}
	return rt.Ready.Kind == "tcp" || rt.Ready.Kind == "stdout-match"
}

func validRunReadyCheck(ready *RunReadyCheck) bool {
	switch ready.Kind {
	case "http":
		return strings.HasPrefix(ready.Path, "/")
	case "tcp":
		if ready.Port < 1 || ready.Port > 65535 {
			return false
		}
		return strings.TrimSpace(ready.Host) == ready.Host
	case "stdout-match":
		if ready.Pattern == "" {
			return false
		}
		return ready.Stream == "" || ready.Stream == "stdout" || ready.Stream == "stderr" || ready.Stream == "both"
	default:
		return false
	}
}

// depIdentity resolves a deterministic dependency identity for cross-references
// (targets.peers/targets.deps/tools) when IR omits explicit dependency keys.
//
// Rule (in priority order):
//  1. dependency.key
//  2. dependency.source.package
//  3. dependency.source.name
//  4. path.Base(dependency.source.ref)
func DependencyIdentity(d DependencyIntent) string {
	if d.Key != "" {
		return d.Key
	}
	if d.Source.Package != "" {
		return d.Source.Package
	}
	if d.Source.Name != "" {
		return d.Source.Name
	}
	if d.Source.Ref != "" {
		return path.Base(d.Source.Ref)
	}
	return ""
}

func depIdentity(d DependencyIntent) string { return DependencyIdentity(d) }

// validateDep accepts registry sources supported by resolver backends and the
// existing local source kinds.
func validateDep(add func(string, string, ...string), pp string, i int, d DependencyIntent) {
	allowed := map[string]bool{"runtime": true, "dep": true, "peer": true, "tool": true, "type": true, "test": true, "workspace": true}
	if !allowed[d.Kind] {
		add("TSPACK_IR_INVALID_DEPENDENCY_KIND", fmt.Sprintf("%s.dependencies[%d].kind is invalid", pp, i))
	}
	sk := d.Source.Kind
	if sk != "npm" && sk != "jsr" && sk != "git" && sk != "path" && sk != "workspace" {
		add("TSPACK_IR_INVALID_SOURCE_KIND", fmt.Sprintf("%s.dependencies[%d].source.kind is invalid", pp, i))
		return
	}
	switch sk {
	case "npm", "jsr":
		if d.Source.Package == "" || d.Source.Range == "" {
			add("TSPACK_IR_INVALID_SOURCE", fmt.Sprintf("%s.dependencies[%d].source %s requires package and range", pp, i, sk))
		}
	case "git":
		if d.Source.Repo == "" && d.Source.Ref == "" {
			add("TSPACK_IR_INVALID_SOURCE", fmt.Sprintf("%s.dependencies[%d].source git requires repo/ref", pp, i))
		}
		if d.Source.Tag == "" && d.Source.Rev == "" && d.Source.Branch == "" {
			add("TSPACK_IR_INVALID_SOURCE", fmt.Sprintf("%s.dependencies[%d].source git requires tag/rev/branch", pp, i))
		}
	case "path":
		if !pathutil.IsSafePackageFilePath(d.Source.Path) {
			add("TSPACK_IR_INVALID_SOURCE", fmt.Sprintf("%s.dependencies[%d].source path must be safe relative", pp, i))
		}
	case "workspace":
		if d.Source.Name == "" && d.Source.Package == "" {
			add("TSPACK_IR_INVALID_SOURCE", fmt.Sprintf("%s.dependencies[%d].source workspace requires name/package", pp, i))
		}
	}
}
func scopeSpecified(value string, specified bool) bool {
	return specified || value != ""
}

func isValidBoundaryScopePattern(value string) bool {
	return pathutil.IsSafePackageFilePath(value) || pathutil.IsSafeRelativeGlob(value)
}

func isValidTypePolicy(k, v string) bool {
	allowed := map[string][]string{"declarations": {"required", "optional", "none"}, "missingTypes": {"error", "warn", "ignore"}, "publicTypeLeakage": {"error", "warn", "ignore"}, "typeOnlyRuntimeLeakage": {"error", "warn", "ignore"}}
	vals, ok := allowed[k]
	if !ok {
		return false
	}
	for _, x := range vals {
		if v == x {
			return true
		}
	}
	return false
}
func isValidBoundaryPolicy(k, v string) bool {
	allowed := map[string][]string{"undeclaredImports": {"error", "warn", "ignore"}, "phantomDependencies": {"error", "warn", "ignore"}, "crossTargetImports": {"error", "warn", "ignore"}, "strict": {"error", "warn", "ignore"}}
	vals, ok := allowed[k]
	if !ok {
		return false
	}
	for _, x := range vals {
		if v == x {
			return true
		}
	}
	return false
}

func StableDiagnosticsJSON(diags []diag.Diagnostic) []byte {
	cp := append([]diag.Diagnostic(nil), diags...)
	diag.SortDiagnostics(cp)
	b, _ := json.Marshal(cp)
	return b
}
