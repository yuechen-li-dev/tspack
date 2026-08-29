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
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

type fakeClient struct {
	mu        sync.Mutex
	meta      map[string]*PackageMetadata
	tar       map[string][]byte
	metaErr   map[string]error
	metaCalls map[string]int
	tarCalls  map[string]int
	metaDelay map[string]time.Duration
	tarDelay  map[string]time.Duration
	metaStart func(name string)
	metaDone  func(name string)
}

func (f *fakeClient) PackageMetadata(ctx context.Context, name string) (*PackageMetadata, error) {
	if f.metaStart != nil {
		f.metaStart(name)
	}
	defer func() {
		if f.metaDone != nil {
			f.metaDone(name)
		}
	}()

	f.mu.Lock()
	if f.metaCalls != nil {
		f.metaCalls[name]++
	}
	delay := f.metaDelay[name]
	err := f.metaErr[name]
	meta := f.meta[name]
	f.mu.Unlock()

	if delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, errors.New("not found")
	}
	return meta, nil
}
func (f *fakeClient) Tarball(ctx context.Context, url string) ([]byte, error) {
	f.mu.Lock()
	if f.tarCalls != nil {
		f.tarCalls[url]++
	}
	delay := f.tarDelay[url]
	body, ok := f.tar[url]
	f.mu.Unlock()

	if delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	if !ok {
		return nil, errors.New("not found")
	}
	return body, nil
}

func (f *fakeClient) metadataCallCount(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.metaCalls[name]
}

func TestResolveNPMM7Coverage(t *testing.T) {
	fc := buildFakeRegistry()
	res := ResolveNPM(context.Background(), ResolverOptions{Client: fc, Mode: ResolveModeUpdate}, ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{{Key: "dep-a", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "dep-a", Range: "1.0.0"}}}, nil, []string{"dep-a"})})
	assertHasPackage(t, res.Lock, "npm:dep-a@1.0.0")
	assertHasPackage(t, res.Lock, "npm:left-pad@1.2.0")
	assertHasEdge(t, res.Lock, "npm:dep-a@1.0.0", "npm:left-pad@1.2.0", "runtime", false)
}

func TestResolveTargetScopedPackageRequirementThroughRequirementTape(t *testing.T) {
	fc := buildFakeRegistry()
	ir := &manifest.ManifestIR{Format: 1, Workspace: manifest.Workspace{Name: "w"}, Packages: []manifest.Package{{
		Name:    "app",
		Root:    ".",
		Version: "1.0.0",
		Kind:    "app",
		Dependencies: []manifest.DependencyIntent{{
			Key: "dep-a", Kind: "tool", Source: manifest.Source{Kind: "npm", Package: "dep-a", Range: "1.0.0"},
		}},
		TestTargets: []manifest.TestTarget{{Name: "unit", Harness: "vitest", Sources: []string{"test/unit.test.ts"}, Requirements: []string{"dep-a"}}},
	}}}
	g, diagnostics := graph.Build(ir)
	if len(diagnostics) > 0 {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	res := ResolveNPM(context.Background(), ResolverOptions{Client: fc, Mode: ResolveModeUpdate}, ResolveRequest{Graph: g})
	assertHasEdge(t, res.Lock, "app:test:unit", "npm:dep-a@1.0.0", "test", false)
	if len(res.Lock.Requirements) != 1 || res.Lock.Requirements[0].Kind != "target-explicit" || res.Lock.Requirements[0].Reference != "dep-a" {
		t.Fatalf("requirements=%#v", res.Lock.Requirements)
	}
}

func TestResolveTargetScopedNPMAliasesIndependently(t *testing.T) {
	fc := buildFakeRegistry()
	ir := &manifest.ManifestIR{Format: 1, Workspace: manifest.Workspace{Name: "w"}, Packages: []manifest.Package{{
		Name: "app", Root: ".", Version: "1.0.0", Kind: "app",
		Dependencies: []manifest.DependencyIntent{
			{Key: "left-pad", Kind: "tool", Source: manifest.Source{Kind: "npm", Package: "left-pad", Range: "1.2.0"}},
			{Key: "left-pad-old", Kind: "tool", Source: manifest.Source{Kind: "npm", Package: "left-pad", Range: "1.1.0"}},
		},
		TestTargets: []manifest.TestTarget{{Name: "unit", Harness: "vitest", Sources: []string{"test/unit.test.ts"}, Requirements: []string{"left-pad", "left-pad-old"}}},
	}}}
	g, diagnostics := graph.Build(ir)
	if len(diagnostics) > 0 {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	res := ResolveNPM(context.Background(), ResolverOptions{Client: fc, Mode: ResolveModeUpdate}, ResolveRequest{Graph: g})
	if len(res.Diagnostics) > 0 {
		t.Fatalf("resolve diagnostics=%v", res.Diagnostics)
	}
	assertHasEdgeReference(t, res.Lock, "app:test:unit", "npm:left-pad@1.2.0", "left-pad")
	assertHasEdgeReference(t, res.Lock, "app:test:unit", "npm:left-pad@1.1.0", "left-pad-old")
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
	fc := buildFakeRegistry()
	fc.metaErr = map[string]error{"broken-meta": errors.New("timeout")}
	mustCode(t, resolveOne(fc, "missing", "1.0.0"), "TSPACK_RESOLVE_NPM_PACKAGE_NOT_FOUND")
	mustCode(t, resolveOne(fc, "left-pad", "bad range"), "TSPACK_RESOLVE_NPM_INVALID_RANGE")
	mustCode(t, resolveOne(fc, "left-pad", ">=9.0.0"), "TSPACK_RESOLVE_NPM_VERSION_NOT_FOUND")
	mustCode(t, resolveOne(fc, "broken-meta", "1.0.0"), "TSPACK_RESOLVE_NPM_METADATA_ERROR")
	fc2 := buildFakeRegistry()
	pv2 := fc2.meta["left-pad"].Versions["1.0.0"]
	fc2.tar[pv2.Dist.Tarball] = []byte("bad")
	pv2.Dist.Integrity = ""
	fc2.meta["left-pad"].Versions["1.0.0"] = pv2
	mustCode(t, resolveOne(fc2, "left-pad", "1.0.0"), "TSPACK_RESOLVE_NPM_TARBALL_PACKAGE_JSON_MISSING")
	fc3 := buildFakeRegistry()
	bad := fc3.meta["left-pad"].Versions["1.0.0"]
	badURL := "https://example.invalid/bad.tgz"
	bad.Dist.Tarball = badURL
	fc3.tar[badURL] = tarFor("wrong", "9.9.9", nil, nil, nil)
	bad.Dist.Integrity = "sha512-" + base64.StdEncoding.EncodeToString(sum512(fc3.tar[badURL]))
	fc3.meta["left-pad"].Versions["1.0.0"] = bad
	mustCode(t, resolveOne(fc3, "left-pad", "1.0.0"), "TSPACK_RESOLVE_NPM_TARBALL_METADATA_MISMATCH")
}

func TestParseTarballPackageJSONRootDetection(t *testing.T) {
	cases := []struct {
		name     string
		entries  map[string]string
		wantName string
		wantOK   bool
	}{
		{
			name:     "package root",
			entries:  map[string]string{"package/package.json": packageJSONFixture("pkg", "1.0.0")},
			wantName: "pkg",
			wantOK:   true,
		},
		{
			name:     "babel core root",
			entries:  map[string]string{"babel__core/package.json": packageJSONFixture("@types/babel__core", "7.20.0")},
			wantName: "@types/babel__core",
			wantOK:   true,
		},
		{
			name:     "estree root",
			entries:  map[string]string{"estree/package.json": packageJSONFixture("@types/estree", "1.0.8")},
			wantName: "@types/estree",
			wantOK:   true,
		},
		{
			name:     "types react root",
			entries:  map[string]string{"./types-react/package.json": packageJSONFixture("@types/react", "19.0.0")},
			wantName: "@types/react",
			wantOK:   true,
		},
		{
			name:     "root level fallback",
			entries:  map[string]string{"package.json": packageJSONFixture("fixture", "1.0.0")},
			wantName: "fixture",
			wantOK:   true,
		},
		{
			name:    "deep package subdir ignored",
			entries: map[string]string{"package/subdir/package.json": packageJSONFixture("pkg", "1.0.0")},
			wantOK:  false,
		},
		{
			name:    "nested too deep ignored",
			entries: map[string]string{"nested/too/deep/package.json": packageJSONFixture("pkg", "1.0.0")},
			wantOK:  false,
		},
		{
			name: "multiple single roots fail",
			entries: map[string]string{
				"anotherRoot/package.json": packageJSONFixture("other", "1.0.0"),
				"package/package.json":     packageJSONFixture("pkg", "1.0.0"),
			},
			wantOK: false,
		},
		{
			name:    "malformed package json fails",
			entries: map[string]string{"package/package.json": `{"name":`},
			wantOK:  false,
		},
		{
			name:    "traversal ignored",
			entries: map[string]string{"../package/package.json": packageJSONFixture("pkg", "1.0.0")},
			wantOK:  false,
		},
		{
			name:    "absolute ignored",
			entries: map[string]string{"/package/package.json": packageJSONFixture("pkg", "1.0.0")},
			wantOK:  false,
		},
		{
			name:    "embedded traversal ignored",
			entries: map[string]string{"package/../package.json": packageJSONFixture("pkg", "1.0.0")},
			wantOK:  false,
		},
		{
			name:    "no package json fails",
			entries: map[string]string{"package/index.js": "module.exports = {};"},
			wantOK:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest, ok := parseTarballPackageJSON(tarballWithEntries(t, tc.entries))
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v manifest=%#v", ok, tc.wantOK, manifest)
			}
			if ok && manifest.Name != tc.wantName {
				t.Fatalf("name=%q want %q", manifest.Name, tc.wantName)
			}
		})
	}
}

func TestResolveScopedPackageWithNonStandardTarballRootOverHTTP(t *testing.T) {
	server := newTarballRootRegistry(t, map[string]string{
		"@types/babel__core": "babel__core",
		"@types/estree":      "estree",
	})

	client := NewHTTPRegistryClient(server.URL)
	deps := []manifest.DependencyIntent{
		{Key: "babelCore", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "@types/babel__core", Range: "1.0.0"}},
		{Key: "estree", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "@types/estree", Range: "1.0.0"}},
	}
	res := ResolveNPM(context.Background(), ResolverOptions{Client: client, Mode: ResolveModeUpdate}, ResolveRequest{Graph: graphForDeps(deps, nil, []string{"babelCore", "estree"})})
	if len(res.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", res.Diagnostics)
	}
	assertHasPackage(t, res.Lock, "npm:@types/babel__core@1.0.0")
	assertHasPackage(t, res.Lock, "npm:@types/estree@1.0.0")
}

func TestDeterministicOutput(t *testing.T) {
	fc := buildFakeRegistry()
	g := graphForDeps([]manifest.DependencyIntent{{Key: "dep-a", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "dep-a", Range: "1.0.0"}}, {Key: "typescript", Kind: "tool", Source: manifest.Source{Kind: "npm", Package: "typescript", Range: "5.6.0"}}}, nil, []string{"dep-a"}, "typescript")
	a := ResolveNPM(context.Background(), ResolverOptions{Client: fc, Mode: ResolveModeUpdate}, ResolveRequest{Graph: g})
	b := ResolveNPM(context.Background(), ResolverOptions{Client: fc, Mode: ResolveModeUpdate}, ResolveRequest{Graph: g})
	if !reflect.DeepEqual(a.Lock, b.Lock) {
		t.Fatalf("lockfile models differ")
	}
	ab, _ := lockfile.Marshal(a.Lock)
	bb, _ := lockfile.Marshal(b.Lock)
	if !bytes.Equal(ab, bb) {
		t.Fatalf("marshal not byte-identical")
	}
	if !sortCheck(a.Lock) {
		t.Fatalf("lock entries not deterministically ordered")
	}
}

func TestResolveNPMDeterministicAcrossResolveJobs(t *testing.T) {
	g := graphForDeps(
		[]manifest.DependencyIntent{
			{Key: "dep-a", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "dep-a", Range: "1.0.0"}},
			{Key: "react", Kind: "peer", Source: manifest.Source{Kind: "npm", Package: "react", Range: ">=18 <20"}},
			{Key: "typescript", Kind: "tool", Source: manifest.Source{Kind: "npm", Package: "typescript", Range: "5.6.0"}},
		},
		[]string{"react"},
		[]string{"dep-a"},
		"typescript",
	)

	serial := ResolveNPM(context.Background(), ResolverOptions{Client: buildFakeRegistry(), Mode: ResolveModeUpdate, ResolveJobs: 1}, ResolveRequest{Graph: g})
	parallel := ResolveNPM(context.Background(), ResolverOptions{Client: buildFakeRegistry(), Mode: ResolveModeUpdate, ResolveJobs: 24}, ResolveRequest{Graph: g})
	serialBytes, _ := lockfile.Marshal(serial.Lock)
	parallelBytes, _ := lockfile.Marshal(parallel.Lock)
	if !bytes.Equal(serialBytes, parallelBytes) {
		t.Fatalf("lock bytes changed between resolve jobs=1 and jobs=24\n--- jobs=1 ---\n%s\n--- jobs=24 ---\n%s", serialBytes, parallelBytes)
	}
}

func TestResolveNPMRepeatedParallelRunsStayDeterministic(t *testing.T) {
	g := graphForDeps(
		[]manifest.DependencyIntent{
			{Key: "dep-a", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "dep-a", Range: "1.0.0"}},
			{Key: "react-dom", Kind: "peer", Source: manifest.Source{Kind: "npm", Package: "react-dom", Range: ">=18 <20"}},
			{Key: "optional-parent", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "optional-parent", Range: "1.0.0"}},
		},
		[]string{"react-dom"},
		[]string{"dep-a", "optional-parent"},
	)

	var baseline []byte
	for run := 0; run < 5; run++ {
		res := ResolveNPM(context.Background(), ResolverOptions{Client: buildFakeRegistry(), Mode: ResolveModeUpdate, ResolveJobs: 24}, ResolveRequest{Graph: g})
		body, _ := lockfile.Marshal(res.Lock)
		if run == 0 {
			baseline = body
			continue
		}
		if !bytes.Equal(baseline, body) {
			t.Fatalf("run %d changed lock output\n--- baseline ---\n%s\n--- run ---\n%s", run, baseline, body)
		}
	}
}

func TestResolveNPMMemoizesMetadataWithinResolvePass(t *testing.T) {
	fc := buildFakeRegistry()
	fc.metaCalls = map[string]int{}

	deps := []manifest.DependencyIntent{
		{Key: "dep-a-a", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "dep-a", Range: "1.0.0"}},
		{Key: "dep-a-b", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "dep-a", Range: "1.0.0"}},
	}
	res := ResolveNPM(
		context.Background(),
		ResolverOptions{Client: fc, Mode: ResolveModeUpdate},
		ResolveRequest{Graph: graphForDeps(deps, nil, []string{"dep-a-a", "dep-a-b"})},
	)
	if len(res.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", res.Diagnostics)
	}
	if fc.metadataCallCount("dep-a") != 1 {
		t.Fatalf("dep-a metadata calls=%d want 1", fc.metadataCallCount("dep-a"))
	}
	if fc.metadataCallCount("left-pad") != 1 {
		t.Fatalf("left-pad metadata calls=%d want 1", fc.metadataCallCount("left-pad"))
	}
}

func TestResolveNPMMemoizesMetadataErrorsWithinResolvePass(t *testing.T) {
	fc := buildFakeRegistry()
	fc.metaCalls = map[string]int{}
	fc.metaErr = map[string]error{"broken-meta": errors.New("timeout")}

	deps := []manifest.DependencyIntent{
		{Key: "broken-a", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "broken-meta", Range: "1.0.0"}},
		{Key: "broken-b", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "broken-meta", Range: "1.0.0"}},
	}
	res := ResolveNPM(
		context.Background(),
		ResolverOptions{Client: fc, Mode: ResolveModeUpdate},
		ResolveRequest{Graph: graphForDeps(deps, nil, []string{"broken-a", "broken-b"})},
	)
	mustCode(t, res, "TSPACK_RESOLVE_NPM_METADATA_ERROR")
	if fc.metadataCallCount("broken-meta") != 1 {
		t.Fatalf("broken-meta metadata calls=%d want 1", fc.metadataCallCount("broken-meta"))
	}
}

func TestResolveNPMMemoizationIsPerResolvePass(t *testing.T) {
	fc := buildFakeRegistry()
	fc.metaCalls = map[string]int{}

	resolveOne(fc, "dep-a", "1.0.0")
	resolveOne(fc, "dep-a", "1.0.0")

	if fc.metadataCallCount("dep-a") != 2 {
		t.Fatalf("dep-a metadata calls=%d want 2 across two resolve passes", fc.metadataCallCount("dep-a"))
	}
}

func TestResolveNPMConcurrentMetadataRequestsShareOneFetch(t *testing.T) {
	fc := buildFakeRegistry()
	fc.metaDelay["dep-a"] = 25 * time.Millisecond

	deps := []manifest.DependencyIntent{
		{Key: "dep-a-a", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "dep-a", Range: "1.0.0"}},
		{Key: "dep-a-b", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "dep-a", Range: "1.0.0"}},
	}
	res := ResolveNPM(
		context.Background(),
		ResolverOptions{Client: fc, Mode: ResolveModeUpdate, ResolveJobs: 24},
		ResolveRequest{Graph: graphForDeps(deps, nil, []string{"dep-a-a", "dep-a-b"})},
	)
	if len(res.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", res.Diagnostics)
	}
	if fc.metadataCallCount("dep-a") != 1 {
		t.Fatalf("dep-a metadata calls=%d want 1", fc.metadataCallCount("dep-a"))
	}
}

func TestResolveNPMConcurrentDifferentMetadataRequestsRunInParallel(t *testing.T) {
	fc := buildFakeRegistry()
	fc.metaDelay["dep-a"] = 25 * time.Millisecond
	fc.metaDelay["typescript"] = 25 * time.Millisecond

	var mu sync.Mutex
	inFlight := 0
	maxInFlight := 0
	fc.metaStart = func(name string) {
		mu.Lock()
		defer mu.Unlock()
		inFlight++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
	}
	fc.metaDone = func(name string) {
		mu.Lock()
		defer mu.Unlock()
		inFlight--
	}

	deps := []manifest.DependencyIntent{
		{Key: "dep-a", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "dep-a", Range: "1.0.0"}},
		{Key: "typescript", Kind: "tool", Source: manifest.Source{Kind: "npm", Package: "typescript", Range: "5.6.0"}},
	}
	res := ResolveNPM(
		context.Background(),
		ResolverOptions{Client: fc, Mode: ResolveModeUpdate, ResolveJobs: 24},
		ResolveRequest{Graph: graphForDeps(deps, nil, []string{"dep-a"}, "typescript")},
	)
	if len(res.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", res.Diagnostics)
	}
	if maxInFlight < 2 {
		t.Fatalf("expected concurrent metadata fetches, maxInFlight=%d", maxInFlight)
	}
}

func TestResolveNPMControllerDecisionReportedPerFrontier(t *testing.T) {
	fc := buildFakeRegistry()
	g := graphForDeps(
		[]manifest.DependencyIntent{
			{Key: "dep-a", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "dep-a", Range: "1.0.0"}},
			{Key: "left-pad", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "left-pad", Range: "1.0.0"}},
		},
		nil,
		[]string{"dep-a", "left-pad"},
	)

	var decisions []FrontierDecision
	res := ResolveNPM(
		context.Background(),
		ResolverOptions{
			Client:                fc,
			Mode:                  ResolveModeUpdate,
			ResolveJobs:           24,
			ResolveControllerMode: ResolveControllerModeFeedforward,
			ResolveHostBudget:     16,
			RegistryURL:           "https://registry.example.test",
			OnResolveController: func(decision FrontierDecision) {
				decisions = append(decisions, decision)
			},
		},
		ResolveRequest{Graph: g},
	)
	if len(res.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", res.Diagnostics)
	}
	if len(decisions) == 0 {
		t.Fatalf("expected controller decisions")
	}
	if decisions[0].TargetJobs != 2 {
		t.Fatalf("first target jobs=%d want 2", decisions[0].TargetJobs)
	}
	if decisions[0].FrontierWidth != 2 || decisions[0].WorkItems != 2 {
		t.Fatalf("unexpected first decision: %#v", decisions[0])
	}
}

func TestResolveNPMFeedforwardControllerClampsWorkerConcurrency(t *testing.T) {
	fc := buildFakeRegistry()
	fc.metaDelay["dep-a"] = 25 * time.Millisecond
	fc.metaDelay["left-pad"] = 25 * time.Millisecond
	fc.metaDelay["typescript"] = 25 * time.Millisecond

	var mu sync.Mutex
	inFlight := 0
	maxInFlight := 0
	fc.metaStart = func(name string) {
		mu.Lock()
		defer mu.Unlock()
		inFlight++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
	}
	fc.metaDone = func(name string) {
		mu.Lock()
		defer mu.Unlock()
		inFlight--
	}

	deps := []manifest.DependencyIntent{
		{Key: "dep-a", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "dep-a", Range: "1.0.0"}},
		{Key: "left-pad", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "left-pad", Range: "1.0.0"}},
		{Key: "typescript", Kind: "tool", Source: manifest.Source{Kind: "npm", Package: "typescript", Range: "5.6.0"}},
	}
	res := ResolveNPM(
		context.Background(),
		ResolverOptions{
			Client:                fc,
			Mode:                  ResolveModeUpdate,
			ResolveJobs:           24,
			ResolveControllerMode: ResolveControllerModeFeedforward,
			ResolveHostBudget:     1,
			RegistryURL:           "https://registry.example.test",
		},
		ResolveRequest{Graph: graphForDeps(deps, nil, []string{"dep-a", "left-pad"}, "typescript")},
	)
	if len(res.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", res.Diagnostics)
	}
	if maxInFlight != 1 {
		t.Fatalf("max in flight=%d want 1", maxInFlight)
	}
}

func TestResolveNPMErrorOrderIsDeterministicAcrossParallelRuns(t *testing.T) {
	deps := []manifest.DependencyIntent{
		{Key: "z-missing", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "z-missing", Range: "1.0.0"}},
		{Key: "a-missing", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "a-missing", Range: "1.0.0"}},
	}
	for run := 0; run < 3; run++ {
		res := ResolveNPM(
			context.Background(),
			ResolverOptions{Client: buildFakeRegistry(), Mode: ResolveModeUpdate, ResolveJobs: 24},
			ResolveRequest{Graph: graphForDeps(deps, nil, []string{"z-missing", "a-missing"})},
		)
		first := firstDiagnosticWithCode(res.Diagnostics, "TSPACK_RESOLVE_NPM_PACKAGE_NOT_FOUND")
		if first == nil {
			t.Fatalf("missing package not found diagnostic in %#v", res.Diagnostics)
		}
		if len(first.Details) == 0 || first.Details[0] != "a-missing" {
			t.Fatalf("unexpected first diagnostic ordering: %#v", res.Diagnostics)
		}
	}
}

func TestResolveNPMArtifactCallbackErrorAbortsResolution(t *testing.T) {
	fc := buildFakeRegistry()

	res := ResolveNPM(
		context.Background(),
		ResolverOptions{
			Client: fc,
			Mode:   ResolveModeUpdate,
			OnArtifactResolved: func(pkg lockfile.Package, artifact []byte) error {
				return errors.New("capture failed")
			},
		},
		ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{{Key: "dep-a", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "dep-a", Range: "1.0.0"}}}, nil, []string{"dep-a"})},
	)
	mustCode(t, res, "TSPACK_RESOLVE_ARTIFACT_CAPTURE_FAILED")
	mustContainDetail(t, res, "capture failed")
}

func TestResolveNPMArtifactCallbackPreservesDeterministicLockOutput(t *testing.T) {
	fc := buildFakeRegistry()

	withoutCallback := ResolveNPM(
		context.Background(),
		ResolverOptions{Client: fc, Mode: ResolveModeUpdate},
		ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{{Key: "dep-a", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "dep-a", Range: "1.0.0"}}}, nil, []string{"dep-a"})},
	)
	withCallback := ResolveNPM(
		context.Background(),
		ResolverOptions{
			Client: fc,
			Mode:   ResolveModeUpdate,
			OnArtifactResolved: func(pkg lockfile.Package, artifact []byte) error {
				return nil
			},
		},
		ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{{Key: "dep-a", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "dep-a", Range: "1.0.0"}}}, nil, []string{"dep-a"})},
	)

	if len(withoutCallback.Diagnostics) != 0 || len(withCallback.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics without=%#v with=%#v", withoutCallback.Diagnostics, withCallback.Diagnostics)
	}
	if !reflect.DeepEqual(withoutCallback.Lock, withCallback.Lock) {
		t.Fatalf("callback changed lock model")
	}
}

func TestLifecycleNotExecuted(t *testing.T) {
	fc := buildFakeRegistry()
	marker := filepath.Join(t.TempDir(), "marker.txt")
	_ = marker // script is never executed by resolver, marker should remain absent.
	res := resolveOne(fc, "package-with-postinstall", "1.0.0")
	assertHasPackage(t, res.Lock, "npm:package-with-postinstall@1.0.0")
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("unexpected marker file: %v", err)
	}
	found := false
	for _, p := range res.Lock.Packages {
		if p.ID == "npm:package-with-postinstall@1.0.0" {
			for _, c := range p.Capabilities {
				if c.Kind == "lifecycleScript" && c.Script == "postinstall" && c.Command == "node write-marker.js" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatalf("expected lifecycle capability")
	}
}

func resolveOne(fc *fakeClient, pkg, rng string) ResolveResult {
	return ResolveNPM(context.Background(), ResolverOptions{Client: fc, Mode: ResolveModeUpdate}, ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{{Key: pkg, Kind: "dep", Source: manifest.Source{Kind: "npm", Package: pkg, Range: rng}}}, nil, []string{pkg})})
}
func graphForDeps(deps []manifest.DependencyIntent, peers []string, runtime []string, tools ...string) *graph.WorkspaceGraph {
	ir := &manifest.ManifestIR{Workspace: manifest.Workspace{Name: "ws"}, Packages: []manifest.Package{{Name: "app", Version: "1.0.0", Kind: "library", Dependencies: deps, Tools: tools, Targets: []manifest.Target{{Name: "react", Export: ".", Entry: "src/index.ts", Runtime: "dist/index.js", Types: "dist/index.d.ts", Deps: runtime, Peers: peers}}}}}
	g, _ := graph.Build(ir)
	return g
}

func buildFakeRegistry() *fakeClient {
	m := map[string]*PackageMetadata{}
	tarballs := map[string][]byte{}
	add := func(name, version string, deps, opt map[string]string, scripts map[string]string) {
		url := "https://example.invalid/" + name + "-" + version + ".tgz"
		body := tarFor(name, version, deps, opt, scripts)
		pv := PackageVersion{Name: name, Version: version, Dependencies: deps, OptionalDependencies: opt, Scripts: scripts, Dist: PackageDist{Tarball: url, Integrity: "sha512-" + base64.StdEncoding.EncodeToString(sum512(body))}}
		if m[name] == nil {
			m[name] = &PackageMetadata{Name: name, Versions: map[string]PackageVersion{}, DistTags: map[string]string{}}
		}
		m[name].Versions[version] = pv
		m[name].DistTags["latest"] = version
		tarballs[url] = body
	}
	add("left-pad", "1.0.0", nil, nil, nil)
	add("left-pad", "1.1.0", nil, nil, nil)
	add("left-pad", "1.2.0", nil, nil, nil)
	add("dep-a", "1.0.0", map[string]string{"left-pad": "^1.1.0"}, nil, nil)
	add("react", "18.2.0", nil, nil, nil)
	add("react", "19.1.0", nil, nil, nil)
	add("react-dom", "18.2.0", nil, nil, nil)
	add("react-dom", "19.1.0", nil, nil, nil)
	add("typescript", "5.6.0", nil, nil, nil)
	add("optional-parent", "1.0.0", nil, map[string]string{"optional-child": "^1.0.0"}, nil)
	add("optional-child", "1.0.0", nil, nil, nil)
	add("package-with-postinstall", "1.0.0", nil, nil, map[string]string{"postinstall": "node write-marker.js"})
	return &fakeClient{
		meta:      m,
		tar:       tarballs,
		metaErr:   map[string]error{},
		metaCalls: map[string]int{},
		tarCalls:  map[string]int{},
		metaDelay: map[string]time.Duration{},
		tarDelay:  map[string]time.Duration{},
	}
}

func tarFor(name, version string, deps, optional, scripts map[string]string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body, _ := json.Marshal(map[string]any{"name": name, "version": version, "dependencies": deps, "optionalDependencies": optional, "scripts": scripts})
	_ = tw.WriteHeader(&tar.Header{Name: "package/package.json", Mode: 0o644, Size: int64(len(body))})
	_, _ = tw.Write(body)
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}
func sum512(b []byte) []byte { h := sha512.Sum512(b); return h[:] }
func sum256(b []byte) []byte { h := sha256.Sum256(b); return h[:] }
func withIntegrity(fc *fakeClient, name, version, integrity string) PackageVersion {
	pv := fc.meta[name].Versions[version]
	pv.Dist.Integrity = integrity
	return pv
}

func mustCode(t *testing.T, res ResolveResult, code string) {
	t.Helper()
	for _, d := range res.Diagnostics {
		if d.Code == code {
			return
		}
	}
	t.Fatalf("missing code %s in %#v", code, res.Diagnostics)
}
func mustCodeAbsent(t *testing.T, res ResolveResult, code string) {
	t.Helper()
	for _, d := range res.Diagnostics {
		if d.Code == code {
			t.Fatalf("unexpected code %s", code)
		}
	}
}
func assertHasPackage(t *testing.T, lf *lockfile.Lockfile, id string) {
	t.Helper()
	for _, p := range lf.Packages {
		if p.ID == id {
			return
		}
	}
	t.Fatalf("missing package %s", id)
}
func assertHasEdge(t *testing.T, lf *lockfile.Lockfile, from, to, kind string, optional bool) {
	t.Helper()
	for _, e := range lf.Edges {
		if e.From == from && e.To == to && e.Kind == kind && e.Optional == optional {
			return
		}
	}
	t.Fatalf("missing edge %s->%s %s optional=%v", from, to, kind, optional)
}

func assertHasEdgeReference(t *testing.T, lf *lockfile.Lockfile, from string, to string, reference string) {
	t.Helper()
	for _, edge := range lf.Edges {
		if edge.From == from && edge.To == to && edge.Reference == reference {
			return
		}
	}
	t.Fatalf("missing edge %s -> %s with reference %s in %#v", from, to, reference, lf.Edges)
}
func sortCheck(lf *lockfile.Lockfile) bool {
	for i := 1; i < len(lf.Packages); i++ {
		if lf.Packages[i-1].ID > lf.Packages[i].ID {
			return false
		}
	}
	for i := 1; i < len(lf.Targets); i++ {
		if lf.Targets[i-1].Package+lf.Targets[i-1].Name > lf.Targets[i].Package+lf.Targets[i].Name {
			return false
		}
	}
	return true
}

func firstDiagnosticWithCode(diags []diag.Diagnostic, code string) *diag.Diagnostic {
	for index := range diags {
		if diags[index].Code == code {
			return &diags[index]
		}
	}
	return nil
}

var _ = diag.SeverityInfo

func packageJSONFixture(name, version string) string {
	body, _ := json.Marshal(map[string]any{"name": name, "version": version})
	return string(body)
}

func tarballWithEntries(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range entries {
		b := []byte(body)
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(b))}); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write(b); err != nil {
			t.Fatalf("write body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func newTarballRootRegistry(t *testing.T, packages map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	metadataByEscapedPath := map[string]PackageMetadata{}
	tarballsByPath := map[string][]byte{}
	for packageName, tarballRoot := range packages {
		version := "1.0.0"
		tarballPath := "/tarballs/" + tarballRoot + "-" + version + ".tgz"
		tarballURL := server.URL + tarballPath
		body := tarballWithEntries(t, map[string]string{tarballRoot + "/package.json": packageJSONFixture(packageName, version)})
		metadataByEscapedPath["/"+url.PathEscape(packageName)] = PackageMetadata{
			Name: packageName,
			Versions: map[string]PackageVersion{
				version: {
					Name:    packageName,
					Version: version,
					Dist: PackageDist{
						Tarball:   tarballURL,
						Integrity: "sha512-" + base64.StdEncoding.EncodeToString(sum512(body)),
					},
				},
			},
			DistTags: map[string]string{"latest": version},
		}
		tarballsByPath[tarballPath] = body
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if body, ok := tarballsByPath[r.URL.Path]; ok {
			_, _ = w.Write(body)
			return
		}
		metadata, ok := metadataByEscapedPath[r.URL.EscapedPath()]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(metadata)
	})
	return server
}
