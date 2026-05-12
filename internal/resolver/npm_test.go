package resolver

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/graph"
	"github.com/tspack/tspack/internal/lockfile"
	"github.com/tspack/tspack/internal/manifest"
)

type fakeClient struct{ meta map[string]*PackageMetadata; tar map[string][]byte; metaErr map[string]error }

func (f *fakeClient) PackageMetadata(_ context.Context, name string) (*PackageMetadata, error) {
	if e, ok := f.metaErr[name]; ok { return nil, e }
	m, ok := f.meta[name]; if !ok { return nil, errors.New("not found") }
	return m, nil
}
func (f *fakeClient) Tarball(_ context.Context, url string) ([]byte, error) { b, ok := f.tar[url]; if !ok { return nil, errors.New("not found") }; return b, nil }

func TestResolveNPMM7Coverage(t *testing.T) {
	fc := buildFakeRegistry()
	res := ResolveNPM(context.Background(), ResolverOptions{Client: fc, Mode: ResolveModeUpdate}, ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{{Key: "dep-a", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "dep-a", Range: "1.0.0"}}}, nil, []string{"dep-a"})})
	assertHasPackage(t, res.Lock, "npm:dep-a@1.0.0")
	assertHasPackage(t, res.Lock, "npm:left-pad@1.2.0")
	assertHasEdge(t, res.Lock, "npm:dep-a@1.0.0", "npm:left-pad@1.2.0", "runtime", false)
}

func TestIntegrityVariants(t *testing.T) {
	fc := buildFakeRegistry()
	mustCodeAbsent(t, resolveOne(fc, "left-pad", "1.2.0"), "TSPACK_RESOLVE_NPM_INTEGRITY_MISMATCH")
	fc.meta["left-pad"].Versions["1.2.0"] = withIntegrity(fc, "left-pad", "1.2.0", "sha999-deadbeef")
	mustCode(t, resolveOne(fc, "left-pad", "1.2.0"), "TSPACK_RESOLVE_NPM_UNSUPPORTED_INTEGRITY")
	fc = buildFakeRegistry()
	pv := fc.meta["left-pad"].Versions["1.1.0"]
	pv.Dist.Integrity = "sha256-" + base64.StdEncoding.EncodeToString(sum256(fc.tar[pv.Dist.Tarball]))
	fc.meta["left-pad"].Versions["1.1.0"] = pv
	mustCodeAbsent(t, resolveOne(fc, "left-pad", "1.1.0"), "TSPACK_RESOLVE_NPM_INTEGRITY_MISMATCH")
}

func TestPeerToolOptionalEdges(t *testing.T) {
	fc := buildFakeRegistry()
	deps := []manifest.DependencyIntent{{Key: "react", Kind: "peer", Source: manifest.Source{Kind: "npm", Package: "react", Range: ">=18 <20"}}, {Key: "react-dom", Kind: "peer", Source: manifest.Source{Kind: "npm", Package: "react-dom", Range: ">=18 <20"}}, {Key: "typescript", Kind: "tool", Source: manifest.Source{Kind: "npm", Package: "typescript", Range: "5.6.0"}}, {Key: "optional-parent", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "optional-parent", Range: "1.0.0"}}}
	res := ResolveNPM(context.Background(), ResolverOptions{Client: fc, Mode: ResolveModeUpdate}, ResolveRequest{Graph: graphForDeps(deps, []string{"react", "react-dom"}, []string{"optional-parent"}, "typescript")})
	assertHasEdge(t, res.Lock, "app:target:react", "npm:react@19.1.0", "peer", false)
	assertHasEdge(t, res.Lock, "app:target:react", "npm:react-dom@19.1.0", "peer", false)
	assertHasEdge(t, res.Lock, "app:tool", "npm:typescript@5.6.0", "tool", false)
	assertHasEdge(t, res.Lock, "npm:optional-parent@1.0.0", "npm:optional-child@1.0.0", "runtime", true)
}

func TestDiagnostics(t *testing.T) {
	fc := buildFakeRegistry(); fc.metaErr = map[string]error{"broken-meta": errors.New("timeout")}
	mustCode(t, resolveOne(fc, "missing", "1.0.0"), "TSPACK_RESOLVE_NPM_PACKAGE_NOT_FOUND")
	mustCode(t, resolveOne(fc, "left-pad", "bad range"), "TSPACK_RESOLVE_NPM_INVALID_RANGE")
	mustCode(t, resolveOne(fc, "left-pad", ">=9.0.0"), "TSPACK_RESOLVE_NPM_VERSION_NOT_FOUND")
	mustCode(t, resolveOne(fc, "broken-meta", "1.0.0"), "TSPACK_RESOLVE_NPM_METADATA_ERROR")
	fc2 := buildFakeRegistry(); pv2 := fc2.meta["left-pad"].Versions["1.0.0"]; fc2.tar[pv2.Dist.Tarball] = []byte("bad"); pv2.Dist.Integrity = ""; fc2.meta["left-pad"].Versions["1.0.0"] = pv2; mustCode(t, resolveOne(fc2, "left-pad", "1.0.0"), "TSPACK_RESOLVE_NPM_TARBALL_PACKAGE_JSON_MISSING")
	fc3 := buildFakeRegistry(); bad := fc3.meta["left-pad"].Versions["1.0.0"]; badURL := "https://example.invalid/bad.tgz"; bad.Dist.Tarball = badURL; fc3.tar[badURL] = tarFor("wrong", "9.9.9", nil, nil, nil); bad.Dist.Integrity = "sha512-" + base64.StdEncoding.EncodeToString(sum512(fc3.tar[badURL])); fc3.meta["left-pad"].Versions["1.0.0"] = bad; mustCode(t, resolveOne(fc3, "left-pad", "1.0.0"), "TSPACK_RESOLVE_NPM_TARBALL_METADATA_MISMATCH")
}

func TestDeterministicOutput(t *testing.T) {
	fc := buildFakeRegistry(); g := graphForDeps([]manifest.DependencyIntent{{Key: "dep-a", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "dep-a", Range: "1.0.0"}}, {Key: "typescript", Kind: "tool", Source: manifest.Source{Kind: "npm", Package: "typescript", Range: "5.6.0"}}}, nil, []string{"dep-a"}, "typescript")
	a := ResolveNPM(context.Background(), ResolverOptions{Client: fc, Mode: ResolveModeUpdate}, ResolveRequest{Graph: g})
	b := ResolveNPM(context.Background(), ResolverOptions{Client: fc, Mode: ResolveModeUpdate}, ResolveRequest{Graph: g})
	if !reflect.DeepEqual(a.Lock, b.Lock) { t.Fatalf("lockfile models differ") }
	ab, _ := lockfile.Marshal(a.Lock); bb, _ := lockfile.Marshal(b.Lock)
	if !bytes.Equal(ab, bb) { t.Fatalf("marshal not byte-identical") }
	if !sortCheck(a.Lock) { t.Fatalf("lock entries not deterministically ordered") }
}

func TestLifecycleNotExecuted(t *testing.T) {
	fc := buildFakeRegistry(); marker := filepath.Join(t.TempDir(), "marker.txt")
	_ = marker // script is never executed by resolver, marker should remain absent.
	res := resolveOne(fc, "package-with-postinstall", "1.0.0")
	assertHasPackage(t, res.Lock, "npm:package-with-postinstall@1.0.0")
	if _, err := os.Stat(marker); !os.IsNotExist(err) { t.Fatalf("unexpected marker file: %v", err) }
	found := false
	for _, p := range res.Lock.Packages { if p.ID == "npm:package-with-postinstall@1.0.0" { for _, c := range p.Capabilities { if c.Kind == "lifecycle-script" && c.Detail == "postinstall" { found = true } } } }
	if !found { t.Fatalf("expected lifecycle capability") }
}

func resolveOne(fc *fakeClient, pkg, rng string) ResolveResult { return ResolveNPM(context.Background(), ResolverOptions{Client: fc, Mode: ResolveModeUpdate}, ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{{Key: pkg, Kind: "dep", Source: manifest.Source{Kind: "npm", Package: pkg, Range: rng}}}, nil, []string{pkg})}) }
func graphForDeps(deps []manifest.DependencyIntent, peers []string, runtime []string, tools ...string) *graph.WorkspaceGraph { ir := &manifest.ManifestIR{Workspace: manifest.Workspace{Name: "ws"}, Packages: []manifest.Package{{Name: "app", Version: "1.0.0", Kind: "library", Dependencies: deps, Tools: tools, Targets: []manifest.Target{{Name: "react", Export: ".", Entry: "src/index.ts", Runtime: "dist/index.js", Types: "dist/index.d.ts", Deps: runtime, Peers: peers}}}}}; g, _ := graph.Build(ir); return g }

func buildFakeRegistry() *fakeClient {
	m := map[string]*PackageMetadata{}; tarballs := map[string][]byte{}
	add := func(name, version string, deps, opt map[string]string, scripts map[string]string) {
		url := "https://example.invalid/" + name + "-" + version + ".tgz"
		body := tarFor(name, version, deps, opt, scripts)
		pv := PackageVersion{Name: name, Version: version, Dependencies: deps, OptionalDependencies: opt, Scripts: scripts, Dist: PackageDist{Tarball: url, Integrity: "sha512-" + base64.StdEncoding.EncodeToString(sum512(body))}}
		if m[name] == nil { m[name] = &PackageMetadata{Name: name, Versions: map[string]PackageVersion{}, DistTags: map[string]string{}} }
		m[name].Versions[version] = pv; m[name].DistTags["latest"] = version; tarballs[url] = body
	}
	add("left-pad", "1.0.0", nil, nil, nil); add("left-pad", "1.1.0", nil, nil, nil); add("left-pad", "1.2.0", nil, nil, nil)
	add("dep-a", "1.0.0", map[string]string{"left-pad": "^1.1.0"}, nil, nil)
	add("react", "18.2.0", nil, nil, nil); add("react", "19.1.0", nil, nil, nil)
	add("react-dom", "18.2.0", nil, nil, nil); add("react-dom", "19.1.0", nil, nil, nil)
	add("typescript", "5.6.0", nil, nil, nil)
	add("optional-parent", "1.0.0", nil, map[string]string{"optional-child": "^1.0.0"}, nil)
	add("optional-child", "1.0.0", nil, nil, nil)
	add("package-with-postinstall", "1.0.0", nil, nil, map[string]string{"postinstall": "node write-marker.js"})
	return &fakeClient{meta: m, tar: tarballs, metaErr: map[string]error{}}
}

func tarFor(name, version string, deps, optional, scripts map[string]string) []byte { var buf bytes.Buffer; gz := gzip.NewWriter(&buf); tw := tar.NewWriter(gz); body, _ := json.Marshal(map[string]any{"name": name, "version": version, "dependencies": deps, "optionalDependencies": optional, "scripts": scripts}); _ = tw.WriteHeader(&tar.Header{Name: "package/package.json", Mode: 0o644, Size: int64(len(body))}); _, _ = tw.Write(body); _ = tw.Close(); _ = gz.Close(); return buf.Bytes() }
func sum512(b []byte) []byte { h := sha512.Sum512(b); return h[:] }
func sum256(b []byte) []byte { h := sha256.Sum256(b); return h[:] }
func withIntegrity(fc *fakeClient, name, version, integrity string) PackageVersion { pv := fc.meta[name].Versions[version]; pv.Dist.Integrity = integrity; return pv }

func mustCode(t *testing.T, res ResolveResult, code string) { t.Helper(); for _, d := range res.Diagnostics { if d.Code == code { return } }; t.Fatalf("missing code %s in %#v", code, res.Diagnostics) }
func mustCodeAbsent(t *testing.T, res ResolveResult, code string) { t.Helper(); for _, d := range res.Diagnostics { if d.Code == code { t.Fatalf("unexpected code %s", code) } } }
func assertHasPackage(t *testing.T, lf *lockfile.Lockfile, id string) { t.Helper(); for _, p := range lf.Packages { if p.ID == id { return } }; t.Fatalf("missing package %s", id) }
func assertHasEdge(t *testing.T, lf *lockfile.Lockfile, from, to, kind string, optional bool) { t.Helper(); for _, e := range lf.Edges { if e.From == from && e.To == to && e.Kind == kind && e.Optional == optional { return } }; t.Fatalf("missing edge %s->%s %s optional=%v", from, to, kind, optional) }
func sortCheck(lf *lockfile.Lockfile) bool { for i := 1; i < len(lf.Packages); i++ { if lf.Packages[i-1].ID > lf.Packages[i].ID { return false } }; for i := 1; i < len(lf.Targets); i++ { if lf.Targets[i-1].Package+lf.Targets[i-1].Name > lf.Targets[i].Package+lf.Targets[i].Name { return false } }; return true }

var _ = diag.SeverityInfo
