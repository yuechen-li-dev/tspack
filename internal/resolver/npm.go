package resolver

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"sync"

	semver "github.com/Masterminds/semver/v3"
	"github.com/yuechen-li-dev/tspack/internal/capability"
	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
)

type ResolveMode string

const (
	ResolveModeUpdate ResolveMode = "update"
	ResolveModeSync   ResolveMode = "sync"
)

type NPMRegistryClient interface {
	PackageMetadata(ctx context.Context, name string) (*PackageMetadata, error)
	Tarball(ctx context.Context, url string) ([]byte, error)
}

type ResolverOptions struct {
	RegistryURL        string
	Client             NPMRegistryClient
	Mode               ResolveMode
	RootDir            string
	OnArtifactResolved func(pkg lockfile.Package, artifact []byte) error
	OnMetadataCacheHit func(name string)
}

type ResolveRequest struct {
	Graph        *graph.WorkspaceGraph
	ExistingLock *lockfile.Lockfile
}

type ResolveResult struct {
	Lock        *lockfile.Lockfile
	Diagnostics []diag.Diagnostic
}

type PackageMetadata struct {
	Name     string                    `json:"name"`
	Versions map[string]PackageVersion `json:"versions"`
	DistTags map[string]string         `json:"dist-tags"`
}

type PackageVersion struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Dependencies         map[string]string `json:"dependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
	Dist                 PackageDist       `json:"dist"`
	Scripts              map[string]string `json:"scripts"`
}

type PackageDist struct {
	Tarball   string `json:"tarball"`
	Integrity string `json:"integrity"`
}

func ResolveNPM(ctx context.Context, opts ResolverOptions, req ResolveRequest) ResolveResult {
	result := ResolveResult{Lock: &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: lockfile.FormatVersion, Tool: lockfile.ToolName}}}
	if opts.Mode == ResolveModeSync {
		result.Diagnostics = append(result.Diagnostics, dErr("TSPACK_RESOLVE_MODE_UNSUPPORTED", "sync mode not yet supported in M7"))
		return result
	}
	if req.Graph == nil || opts.Client == nil {
		result.Diagnostics = append(result.Diagnostics, dErr("TSPACK_RESOLVE_INTERNAL_ERROR", "resolver requires graph and client"))
		return result
	}
	for _, p := range req.Graph.AllPackages() {
		for _, t := range p.AllTargets() {
			result.Lock.Targets = append(result.Lock.Targets, lockfile.Target{Package: p.Name, Name: t.Name, Export: t.Export, Entry: t.Entry, Runtime: t.Runtime, Types: t.Types})
		}
	}
	state := &resolverState{opts: opts, result: &result, seenPkg: map[string]bool{}, graph: req.Graph}
	for _, p := range req.Graph.AllPackages() {
		for _, t := range p.AllTargets() {
			from := fmt.Sprintf("%s:target:%s", p.Name, t.Name)
			for _, dep := range t.AllowedRuntimeDependencies() {
				state.resolveDirect(ctx, dep, from)
			}
			for _, dep := range t.AllowedPeerDependencies() {
				state.resolveDirectAsKind(ctx, dep, from, "peer")
			}
		}
		for _, dep := range p.ToolDependencies() {
			state.resolveDirectAsKind(ctx, dep, fmt.Sprintf("%s:tool", p.Name), "tool")
		}
	}
	result.Lock = mustNormalizeLock(result.Lock)
	diag.SortDiagnostics(result.Diagnostics)
	return result
}

type resolverState struct {
	opts    ResolverOptions
	result  *ResolveResult
	seenPkg map[string]bool
	graph   *graph.WorkspaceGraph
	metaMu  sync.Mutex
	meta    map[string]*metadataMemoEntry
}

type metadataMemoEntry struct {
	ready chan struct{}
	meta  *PackageMetadata
	err   error
}

func (r *resolverState) resolveDirect(ctx context.Context, dep *graph.DependencyNode, from string) {
	kind := "runtime"
	if dep.Kind == graph.DependencyKindType {
		kind = "type"
	}
	r.resolveDirectAsKind(ctx, dep, from, kind)
}

func (r *resolverState) resolveDirectAsKind(ctx context.Context, dep *graph.DependencyNode, from, kind string) {
	if dep.Source.Kind != "npm" {
		r.resolveNonNPMDependency(ctx, dep, from, kind)
		return
	}
	id, optional, ok := r.resolvePackage(ctx, dep.Source.Package, dep.Source.Range, dep.Optional, "")
	if !ok {
		return
	}
	r.result.Lock.Edges = append(r.result.Lock.Edges, lockfile.Edge{From: from, To: id, Kind: kind, Optional: optional})
}

func (r *resolverState) resolvePackage(ctx context.Context, name, rng string, optional bool, parentID string) (string, bool, bool) {
	meta, err := r.packageMetadata(ctx, name)
	if err != nil {
		code := "TSPACK_RESOLVE_NPM_METADATA_ERROR"
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			code = "TSPACK_RESOLVE_NPM_PACKAGE_NOT_FOUND"
		}
		return "", optional, r.emitLookupError(optional, code, "failed to fetch npm metadata", name, err.Error())
	}
	pv, version, selCode, ok := selectVersion(meta, rng)
	if !ok {
		code := "TSPACK_RESOLVE_NPM_VERSION_NOT_FOUND"
		if selCode != "" {
			code = selCode
		}
		return "", optional, r.emitLookupError(optional, code, "failed to select npm version", name, rng)
	}
	id := fmt.Sprintf("npm:%s@%s", name, version)
	if !r.seenPkg[id] {
		if !r.addResolvedPackage(ctx, id, pv, optional) {
			return "", optional, false
		}
		r.seenPkg[id] = true
		r.resolveTransitive(ctx, id, pv)
	}
	if parentID != "" {
		r.result.Lock.Edges = append(r.result.Lock.Edges, lockfile.Edge{From: parentID, To: id, Kind: "runtime", Optional: optional})
	}
	return id, optional, true
}

func (r *resolverState) packageMetadata(ctx context.Context, name string) (*PackageMetadata, error) {
	key := r.registryMetadataCacheKey(name)

	r.metaMu.Lock()
	if r.meta == nil {
		r.meta = map[string]*metadataMemoEntry{}
	}
	if entry, ok := r.meta[key]; ok {
		r.metaMu.Unlock()
		if r.opts.OnMetadataCacheHit != nil {
			r.opts.OnMetadataCacheHit(name)
		}
		<-entry.ready
		return entry.meta, entry.err
	}
	entry := &metadataMemoEntry{ready: make(chan struct{})}
	r.meta[key] = entry
	r.metaMu.Unlock()

	entry.meta, entry.err = r.opts.Client.PackageMetadata(ctx, name)
	close(entry.ready)
	return entry.meta, entry.err
}

func (r *resolverState) registryMetadataCacheKey(name string) string {
	if client, ok := r.opts.Client.(*HTTPRegistryClient); ok && client.BaseURL != "" {
		return client.BaseURL + "|" + name
	}
	if r.opts.RegistryURL != "" {
		return r.opts.RegistryURL + "|" + name
	}
	return "default|" + name
}

func (r *resolverState) emitLookupError(optional bool, code, msg string, details ...string) bool {
	if optional {
		r.result.Diagnostics = append(r.result.Diagnostics, dWarn(code, msg, details...))
		return false
	}
	r.result.Diagnostics = append(r.result.Diagnostics, dErr(code, msg, details...))
	return false
}

func selectVersion(meta *PackageMetadata, rng string) (PackageVersion, string, string, bool) {
	c, err := semver.NewConstraint(rng)
	if err != nil {
		return PackageVersion{}, "", "TSPACK_RESOLVE_NPM_INVALID_RANGE", false
	}
	versions := make([]*semver.Version, 0, len(meta.Versions))
	lookup := map[string]PackageVersion{}
	for v, pv := range meta.Versions {
		sv, err := semver.NewVersion(v)
		if err != nil {
			continue
		}
		versions = append(versions, sv)
		lookup[sv.Original()] = pv
	}
	sort.SliceStable(versions, func(i, j int) bool { return versions[i].GreaterThan(versions[j]) })
	for _, v := range versions {
		if c.Check(v) {
			return lookup[v.Original()], v.Original(), "", true
		}
	}
	return PackageVersion{}, "", "", false
}

func (r *resolverState) addResolvedPackage(ctx context.Context, id string, pv PackageVersion, optional bool) bool {
	body, err := r.opts.Client.Tarball(ctx, pv.Dist.Tarball)
	if err != nil {
		r.emitLookupError(optional, "TSPACK_RESOLVE_NPM_TARBALL_ERROR", "failed to fetch npm tarball", id, err.Error())
		return false
	}
	if pv.Dist.Integrity != "" {
		ok, code := verifyIntegrity(body, pv.Dist.Integrity)
		if code != "" {
			r.emitLookupError(optional, code, "unsupported npm integrity algorithm", id, pv.Dist.Integrity)
			return false
		}
		if !ok {
			r.emitLookupError(optional, "TSPACK_RESOLVE_NPM_INTEGRITY_MISMATCH", "tarball integrity mismatch", id)
			return false
		}
	}
	manifest, ok := parseTarballPackageJSON(body)
	if !ok {
		r.emitLookupError(optional, "TSPACK_RESOLVE_NPM_TARBALL_PACKAGE_JSON_MISSING", "tarball package.json missing", id)
		return false
	}
	if manifest.Name != pv.Name || manifest.Version != pv.Version {
		r.emitLookupError(optional, "TSPACK_RESOLVE_NPM_TARBALL_METADATA_MISMATCH", "tarball package metadata mismatch", id)
		return false
	}
	h := sha256.Sum256(body)
	pkg := lockfile.Package{ID: id, Name: pv.Name, Version: pv.Version, Source: "npm", Integrity: pv.Dist.Integrity, Hash: "sha256:" + hex.EncodeToString(h[:])}
	pkg.Capabilities = capability.FromPackageJSONScripts(manifest.Scripts)
	if r.opts.OnArtifactResolved != nil {
		if err := r.opts.OnArtifactResolved(pkg, body); err != nil {
			r.emitLookupError(optional, "TSPACK_RESOLVE_ARTIFACT_CAPTURE_FAILED", "failed to capture resolved artifact", id, err.Error())
			return false
		}
	}
	r.result.Lock.Packages = append(r.result.Lock.Packages, pkg)
	return true
}

func (r *resolverState) resolveTransitive(ctx context.Context, parentID string, pv PackageVersion) {
	for _, entry := range sortedDeps(pv.Dependencies) {
		r.resolvePackage(ctx, entry.name, entry.rng, false, parentID)
	}
	for _, entry := range sortedDeps(pv.OptionalDependencies) {
		r.resolvePackage(ctx, entry.name, entry.rng, true, parentID)
	}
}

type depEntry struct{ name, rng string }

func sortedDeps(m map[string]string) []depEntry {
	out := make([]depEntry, 0, len(m))
	for k, v := range m {
		out = append(out, depEntry{name: k, rng: v})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

type packageJSON struct {
	Name    string            `json:"name"`
	Version string            `json:"version"`
	Scripts map[string]string `json:"-"`
}

type rawPackageJSON struct {
	Name    string         `json:"name"`
	Version string         `json:"version"`
	Scripts map[string]any `json:"scripts"`
}

func parseTarballPackageJSON(body []byte) (packageJSON, bool) {
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return packageJSON{}, false
	}
	defer gz.Close()

	var rootPackageJSON []byte
	var topLevelPackageJSON []byte
	foundTopLevelPackageJSON := false
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return packageJSON{}, false
		}

		cleanName, ok := cleanTarballPackageJSONPath(hdr.Name)
		if !ok {
			continue
		}

		b, err := io.ReadAll(tr)
		if err != nil {
			return packageJSON{}, false
		}

		if cleanName == "package.json" {
			rootPackageJSON = b
			continue
		}

		if foundTopLevelPackageJSON {
			return packageJSON{}, false
		}
		topLevelPackageJSON = b
		foundTopLevelPackageJSON = true
	}

	selected := topLevelPackageJSON
	if !foundTopLevelPackageJSON {
		selected = rootPackageJSON
	}
	if selected == nil {
		return packageJSON{}, false
	}

	var raw rawPackageJSON
	if err := json.Unmarshal(selected, &raw); err != nil {
		return packageJSON{}, false
	}
	return packageJSON{Name: raw.Name, Version: raw.Version, Scripts: stringScripts(raw.Scripts)}, true
}

func cleanTarballPackageJSONPath(name string) (string, bool) {
	if name == "" || strings.HasPrefix(name, "/") {
		return "", false
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", false
		}
	}

	cleanName := path.Clean(name)
	if cleanName == "." || cleanName == "" {
		return "", false
	}
	if cleanName == ".." || strings.HasPrefix(cleanName, "../") || strings.Contains(cleanName, "/../") {
		return "", false
	}

	if cleanName == "package.json" {
		return cleanName, true
	}
	if !strings.HasSuffix(cleanName, "/package.json") {
		return "", false
	}

	parts := strings.Split(cleanName, "/")
	if len(parts) != 2 || parts[1] != "package.json" || parts[0] == "" {
		return "", false
	}
	return cleanName, true
}

func verifyIntegrity(body []byte, integrity string) (bool, string) {
	parts := strings.SplitN(integrity, "-", 2)
	if len(parts) != 2 {
		return false, "TSPACK_RESOLVE_NPM_UNSUPPORTED_INTEGRITY"
	}
	want, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return false, "TSPACK_RESOLVE_NPM_UNSUPPORTED_INTEGRITY"
	}
	var got []byte
	switch parts[0] {
	case "sha512":
		h := sha512.Sum512(body)
		got = h[:]
	case "sha256":
		h := sha256.Sum256(body)
		got = h[:]
	default:
		return false, "TSPACK_RESOLVE_NPM_UNSUPPORTED_INTEGRITY"
	}
	return subtle.ConstantTimeCompare(got, want) == 1, ""
}

func mustNormalizeLock(lf *lockfile.Lockfile) *lockfile.Lockfile {
	b, _ := lockfile.Marshal(lf)
	n, _ := lockfile.Parse("generated.ts-lock.toml", b)
	return n
}

func dErr(code, msg string, details ...string) diag.Diagnostic {
	return diag.Diagnostic{Code: code, Severity: diag.SeverityError, Message: msg, Details: details}
}
func dWarn(code, msg string, details ...string) diag.Diagnostic {
	return diag.Diagnostic{Code: code, Severity: diag.SeverityWarning, Message: msg, Details: details}
}

func stringScripts(raw map[string]any) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	scripts := map[string]string{}
	for name, value := range raw {
		command, ok := value.(string)
		if !ok {
			continue
		}
		scripts[name] = command
	}
	if len(scripts) == 0 {
		return nil
	}
	return scripts
}
