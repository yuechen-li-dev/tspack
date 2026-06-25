package manifest

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/pathutil"
)

type ManifestIR struct {
	Format    int       `json:"format"`
	Workspace Workspace `json:"workspace"`
	Security  Security  `json:"security,omitempty"`
	Packages  []Package `json:"packages"`
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
	AcknowledgedCapabilities []AcknowledgedCapability `json:"acknowledgedCapabilities,omitempty"`
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

type Package struct {
	Name         string             `json:"name"`
	Version      string             `json:"version"`
	Root         string             `json:"root,omitempty"`
	License      string             `json:"license,omitempty"`
	Kind         string             `json:"kind"`
	Policies     Policies           `json:"policies"`
	Dependencies []DependencyIntent `json:"dependencies"`
	Targets      []Target           `json:"targets"`
	Tools        []string           `json:"tools"`
	Boundaries   []BoundaryRule     `json:"boundaries"`
	Publish      PublishPolicy      `json:"publish"`
	RunTargets   []RunTarget        `json:"runTargets,omitempty"`
}

type RunTarget struct {
	Name            string         `json:"name"`
	Runtime         string         `json:"runtime"`
	ExplicitRuntime string         `json:"explicitRuntime,omitempty"`
	Command         []string       `json:"command"`
	URL             string         `json:"url"`
	Cwd             string         `json:"cwd,omitempty"`
	Ready           *RunReadyCheck `json:"ready,omitempty"`
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

type DependencyIntent struct {
	Key      string `json:"key,omitempty"`
	Kind     string `json:"kind"`
	Source   Source `json:"source"`
	Optional bool   `json:"optional,omitempty"`
}

type Source struct {
	Kind    string `json:"kind"`
	Package string `json:"package,omitempty"`
	Range   string `json:"range,omitempty"`
	Ref     string `json:"ref,omitempty"`
	Repo    string `json:"repo,omitempty"`
	Tag     string `json:"tag,omitempty"`
	Rev     string `json:"rev,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Path    string `json:"path,omitempty"`
	Name    string `json:"name,omitempty"`
}

type Target struct {
	Name     string   `json:"name"`
	Export   string   `json:"export"`
	Entry    string   `json:"entry"`
	Runtime  string   `json:"runtime"`
	Types    string   `json:"types"`
	Peers    []string `json:"peers"`
	Deps     []string `json:"deps"`
	Optional bool     `json:"optional,omitempty"`
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
)

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
	case "preinstall", "install", "postinstall", "prepack", "prepare", "postpack", "prepublish", "prepublishOnly", "postpublish":
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
	if len(ir.Packages) == 0 {
		add("TSPACK_IR_NO_PACKAGES", "at least one package is required")
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
	seenPkg := map[string]struct{}{}
	for pi, p := range ir.Packages {
		pp := fmt.Sprintf("packages[%d]", pi)
		if !pkgNameRe.MatchString(p.Name) {
			add("TSPACK_IR_INVALID_PACKAGE_NAME", pp+".name is invalid")
		}
		if !versionRe.MatchString(p.Version) {
			add("TSPACK_IR_INVALID_PACKAGE_VERSION", pp+".version is invalid")
		}
		if p.Kind != "library" && p.Kind != "app" && p.Kind != "tool" {
			add("TSPACK_IR_INVALID_PACKAGE_KIND", pp+".kind is invalid")
		}
		if p.Root != "" && !pathutil.IsSafePackageRoot(p.Root) {
			add("TSPACK_IR_INVALID_PACKAGE_ROOT", pp+".root must be a safe relative path")
		}
		if _, ok := seenPkg[p.Name]; ok {
			add("TSPACK_IR_DUPLICATE_PACKAGE", "duplicate package name: "+p.Name)
		}
		seenPkg[p.Name] = struct{}{}
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
				u, err := url.Parse(rt.URL)
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
					add("TSPACK_RUN_INVALID_URL", rp+".url must be valid http/https URL")
				}
			} else if strings.TrimSpace(rt.URL) != "" {
				u, err := url.Parse(rt.URL)
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
					add("TSPACK_RUN_INVALID_URL", rp+".url must be valid http/https URL")
				}
			}
			if rt.Ready != nil && !validRunReadyCheck(rt.Ready) {
				add("TSPACK_RUN_INVALID_READY", rp+".ready is invalid")
			}
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

func runTargetURLIsOptional(rt RunTarget) bool {
	if rt.Ready == nil {
		return false
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
func validateDep(add func(string, string, ...string), pp string, i int, d DependencyIntent) {
	allowed := map[string]bool{"runtime": true, "dep": true, "peer": true, "tool": true, "type": true, "test": true, "workspace": true}
	if !allowed[d.Kind] {
		add("TSPACK_IR_INVALID_DEPENDENCY_KIND", fmt.Sprintf("%s.dependencies[%d].kind is invalid", pp, i))
	}
	sk := d.Source.Kind
	if sk != "npm" && sk != "git" && sk != "path" && sk != "workspace" {
		add("TSPACK_IR_INVALID_SOURCE_KIND", fmt.Sprintf("%s.dependencies[%d].source.kind is invalid", pp, i))
		return
	}
	switch sk {
	case "npm":
		if d.Source.Package == "" || d.Source.Range == "" {
			add("TSPACK_IR_INVALID_SOURCE", fmt.Sprintf("%s.dependencies[%d].source npm requires package and range", pp, i))
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
