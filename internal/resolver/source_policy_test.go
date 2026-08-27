package resolver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func TestSourcePolicyDeniesSemanticSourceBeforeNetwork(t *testing.T) {
	jsrClient := newRegistryFixture()
	addRegistryFixture(jsrClient, "@jsr/std__path", "1.0.0", nil)
	dependency := manifest.DependencyIntent{
		Key:  "path",
		Kind: "dep",
		Source: manifest.Source{
			Kind:    SourceJSR,
			Package: "@std/path",
			Range:   "1.0.0",
		},
	}
	result := Resolve(context.Background(), ResolverOptions{
		Mode: ResolveModeUpdate,
		Backends: BackendRegistry{
			SourceJSR: NewJSRBackend(jsrClient),
		},
		SourcePolicy: SourcePolicy{
			Origin:         "manifest RegistryPolicy",
			AllowedSources: map[string]bool{SourceNPM: true},
		},
	}, ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{dependency}, nil, []string{"path"})})

	mustCode(t, result, "TSPACK_SOURCE_POLICY_DENIED")
	if len(jsrClient.metaCalls) != 0 || len(jsrClient.tarCalls) != 0 {
		t.Fatalf("denied source performed network-like calls: metadata=%v tarball=%v", jsrClient.metaCalls, jsrClient.tarCalls)
	}
}

func TestExplicitEmptySourceAllowlistDeniesAllRegistrySources(t *testing.T) {
	policy := SourcePolicy{AllowedSources: map[string]bool{}}
	if policy.Allows(SourceNPM) || policy.Allows(SourceJSR) {
		t.Fatal("explicit empty allowlist must deny npm and jsr")
	}
}

func TestFallbackRegistryBackendUsesDeclaredOrderForServerFailure(t *testing.T) {
	var primaryRequests atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryRequests.Add(1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer primary.Close()

	var fallbackRequests atomic.Int32
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackRequests.Add(1)
		_ = json.NewEncoder(w).Encode(PackageMetadata{Name: "pkg", Versions: map[string]PackageVersion{}})
	}))
	defer fallback.Close()

	policy := SourcePolicy{
		AllowedSources:   map[string]bool{SourceNPM: true},
		RecordProvenance: true,
		Endpoints: map[string][]RegistryEndpoint{
			SourceNPM: {{URL: primary.URL}, {URL: fallback.URL}},
		},
	}
	backends, err := policy.Backends(nil)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := backends[SourceNPM].Metadata(context.Background(), "pkg")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Endpoint != fallback.URL {
		t.Fatalf("endpoint=%q want %q", metadata.Endpoint, fallback.URL)
	}
	if primaryRequests.Load() != 1 || fallbackRequests.Load() != 1 {
		t.Fatalf("request order/count primary=%d fallback=%d", primaryRequests.Load(), fallbackRequests.Load())
	}
}

func TestFallbackRegistryBackendFailsClosedForAuthAndNotFoundByDefault(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "failure", status)
			}))
			defer primary.Close()
			var fallbackRequests atomic.Int32
			fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fallbackRequests.Add(1)
				_ = json.NewEncoder(w).Encode(PackageMetadata{})
			}))
			defer fallback.Close()
			policy := SourcePolicy{
				AllowedSources: map[string]bool{SourceNPM: true},
				Endpoints: map[string][]RegistryEndpoint{
					SourceNPM: {{URL: primary.URL}, {URL: fallback.URL}},
				},
			}
			backends, err := policy.Backends(nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = backends[SourceNPM].Metadata(context.Background(), "pkg")
			if err == nil {
				t.Fatal("expected endpoint failure")
			}
			if fallbackRequests.Load() != 0 {
				t.Fatalf("status %d silently fell back", status)
			}
		})
	}
}

func TestFallbackRegistryBackendCanExplicitlyFallbackOnNotFound(t *testing.T) {
	primary := httptest.NewServer(http.NotFoundHandler())
	defer primary.Close()
	var fallbackRequests atomic.Int32
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackRequests.Add(1)
		_ = json.NewEncoder(w).Encode(PackageMetadata{Name: "pkg", Versions: map[string]PackageVersion{}})
	}))
	defer fallback.Close()
	policy := SourcePolicy{
		AllowedSources: map[string]bool{SourceNPM: true},
		Endpoints: map[string][]RegistryEndpoint{
			SourceNPM: {{URL: primary.URL, FallbackOnNotFound: true}, {URL: fallback.URL}},
		},
	}
	backends, err := policy.Backends(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backends[SourceNPM].Metadata(context.Background(), "pkg"); err != nil {
		t.Fatal(err)
	}
	if fallbackRequests.Load() != 1 {
		t.Fatalf("explicit not-found fallback requests=%d", fallbackRequests.Load())
	}
}

func TestEndpointCredentialsAreScopedAndDiagnosticURLsAreRedacted(t *testing.T) {
	t.Setenv("COMPANY_REGISTRY_TOKEN", "super-secret-token")
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(PackageMetadata{Name: "pkg", Versions: map[string]PackageVersion{}})
	}))
	defer server.Close()
	policy := SourcePolicy{
		AllowedSources: map[string]bool{SourceNPM: true},
		Endpoints: map[string][]RegistryEndpoint{
			SourceNPM: {{URL: server.URL, TokenEnv: "COMPANY_REGISTRY_TOKEN"}},
		},
	}
	backends, err := policy.Backends(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backends[SourceNPM].Metadata(context.Background(), "pkg"); err != nil {
		t.Fatal(err)
	}
	if authorization != "Bearer super-secret-token" {
		t.Fatalf("authorization header=%q", authorization)
	}
	redacted := RedactURL("https://user:password@example.test/npm?token=url-secret&ok=value")
	if strings.Contains(redacted, "password") || strings.Contains(redacted, "url-secret") || !strings.Contains(redacted, "REDACTED") {
		t.Fatalf("URL was not safely redacted: %s", redacted)
	}
}

func TestArtifactHostAllowlistRejectsUnexpectedHostBeforeRequest(t *testing.T) {
	client := NewHTTPRegistryClient("https://registry.example.test")
	client.AllowedArtifactHosts = []string{"cdn.example.test"}
	_, err := client.Tarball(context.Background(), "https://evil.example.test/pkg.tgz?token=secret")
	if err == nil || !strings.Contains(err.Error(), "TSPACK_REGISTRY_ENDPOINT_DENIED") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unexpected artifact-host diagnostic: %v", err)
	}
}

func TestIntegrityMismatchNeverFallsThroughToAnotherEndpoint(t *testing.T) {
	artifact := tarFor("pkg", "1.0.0", nil, nil, nil)
	var primary *httptest.Server
	primary = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".tgz") {
			_, _ = w.Write(artifact)
			return
		}
		_ = json.NewEncoder(w).Encode(PackageMetadata{
			Name: "pkg",
			Versions: map[string]PackageVersion{
				"1.0.0": {
					Name:    "pkg",
					Version: "1.0.0",
					Dist: PackageDist{
						Tarball:   primary.URL + "/pkg-1.0.0.tgz",
						Integrity: "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
					},
				},
			},
		})
	}))
	defer primary.Close()
	var fallbackRequests atomic.Int32
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackRequests.Add(1)
		http.Error(w, "must not be used", http.StatusInternalServerError)
	}))
	defer fallback.Close()
	policy := SourcePolicy{
		AllowedSources: map[string]bool{SourceNPM: true},
		Endpoints: map[string][]RegistryEndpoint{
			SourceNPM: {{URL: primary.URL}, {URL: fallback.URL}},
		},
	}
	backends, err := policy.Backends(nil)
	if err != nil {
		t.Fatal(err)
	}
	dependency := manifest.DependencyIntent{Key: "pkg", Kind: "dep", Source: manifest.Source{Kind: SourceNPM, Package: "pkg", Range: "1.0.0"}}
	result := Resolve(context.Background(), ResolverOptions{Mode: ResolveModeUpdate, Backends: backends, SourcePolicy: policy}, ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{dependency}, nil, []string{"pkg"})})
	mustCode(t, result, "TSPACK_RESOLVE_NPM_INTEGRITY_MISMATCH")
	if fallbackRequests.Load() != 0 {
		t.Fatalf("integrity failure silently fell back: requests=%d", fallbackRequests.Load())
	}
}

func TestArtifactAvailabilityFallbackUsesEquivalentDeclaredEndpoint(t *testing.T) {
	artifact := tarFor("pkg", "1.0.0", nil, nil, nil)
	integrity := "sha512-" + base64.StdEncoding.EncodeToString(sum512(artifact))
	var primary *httptest.Server
	primary = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".tgz") {
			http.Error(w, "artifact unavailable", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(PackageMetadata{Name: "pkg", Versions: map[string]PackageVersion{"1.0.0": {Name: "pkg", Version: "1.0.0", Dist: PackageDist{Tarball: primary.URL + "/pkg.tgz", Integrity: integrity}}}})
	}))
	defer primary.Close()
	var fallback *httptest.Server
	fallback = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".tgz") {
			_, _ = w.Write(artifact)
			return
		}
		_ = json.NewEncoder(w).Encode(PackageMetadata{Name: "pkg", Versions: map[string]PackageVersion{"1.0.0": {Name: "pkg", Version: "1.0.0", Dist: PackageDist{Tarball: fallback.URL + "/pkg.tgz", Integrity: integrity}}}})
	}))
	defer fallback.Close()
	policy := SourcePolicy{
		AllowedSources:   map[string]bool{SourceNPM: true},
		RecordProvenance: true,
		Endpoints: map[string][]RegistryEndpoint{
			SourceNPM: {{URL: primary.URL}, {URL: fallback.URL}},
		},
	}
	backends, err := policy.Backends(nil)
	if err != nil {
		t.Fatal(err)
	}
	dependency := manifest.DependencyIntent{Key: "pkg", Kind: "dep", Source: manifest.Source{Kind: SourceNPM, Package: "pkg", Range: "1.0.0"}}
	result := Resolve(context.Background(), ResolverOptions{Mode: ResolveModeUpdate, Backends: backends, SourcePolicy: policy}, ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{dependency}, nil, []string{"pkg"})})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("artifact fallback diagnostics: %#v", result.Diagnostics)
	}
	if len(result.Lock.Packages) != 1 || result.Lock.Packages[0].MetadataEndpoint != primary.URL || result.Lock.Packages[0].RegistryEndpoint != fallback.URL {
		t.Fatalf("artifact fallback provenance: %#v", result.Lock.Packages)
	}
}

func TestOfflineUpdateFailsBeforeRegistryRequests(t *testing.T) {
	client := newRegistryFixture()
	addRegistryFixture(client, "pkg", "1.0.0", nil)
	dependency := manifest.DependencyIntent{Key: "pkg", Kind: "dep", Source: manifest.Source{Kind: SourceNPM, Package: "pkg", Range: "1.0.0"}}
	result := Resolve(context.Background(), ResolverOptions{
		Mode:         ResolveModeUpdate,
		Backends:     BackendRegistry{SourceNPM: NewNPMBackend(client)},
		SourcePolicy: SourcePolicy{AllowedSources: map[string]bool{SourceNPM: true}, Offline: true},
	}, ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{dependency}, nil, []string{"pkg"})})
	mustCode(t, result, "TSPACK_REGISTRY_OFFLINE_MISS")
	if len(client.metaCalls) != 0 || len(client.tarCalls) != 0 {
		t.Fatalf("offline update performed calls: metadata=%v tarball=%v", client.metaCalls, client.tarCalls)
	}
}

func TestHTTPRegistryClientHonorsBoundedRetryAfter(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode(PackageMetadata{Name: "pkg", Versions: map[string]PackageVersion{}})
	}))
	defer server.Close()
	client := NewHTTPRegistryClient(server.URL)
	if _, err := client.PackageMetadata(context.Background(), "pkg"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests=%d want one bounded retry", requests.Load())
	}
}

func TestEndpointCredentialIsNotSentToCrossHostArtifact(t *testing.T) {
	var artifactAuthorization string
	artifactServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		artifactAuthorization = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("artifact"))
	}))
	defer artifactServer.Close()
	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(PackageMetadata{})
	}))
	defer registryServer.Close()
	client := NewHTTPRegistryClient(registryServer.URL)
	client.Authorization = "Bearer secret"
	if _, err := client.Tarball(context.Background(), artifactServer.URL+"/pkg.tgz"); err != nil {
		t.Fatal(err)
	}
	if artifactAuthorization != "" {
		t.Fatalf("credential leaked to artifact host: %q", artifactAuthorization)
	}
}

func TestMissingCredentialEnvironmentFailsBeforeRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	client := NewHTTPRegistryClient(server.URL)
	client.AuthorizationEnv = "TSPACK_TEST_MISSING_REGISTRY_TOKEN"
	_, err := client.PackageMetadata(context.Background(), "pkg")
	if err == nil || !strings.Contains(err.Error(), "TSPACK_REGISTRY_AUTH_FAILED") {
		t.Fatalf("unexpected credential error: %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("missing credential performed %d requests", requests.Load())
	}
}

func TestRegistryRedirectDoesNotLeakCredentialAcrossOrigins(t *testing.T) {
	var redirectedAuthorization string
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedAuthorization = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(PackageMetadata{Name: "pkg", Versions: map[string]PackageVersion{}})
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	client := NewHTTPRegistryClient(source.URL)
	client.Authorization = "Bearer secret"
	if _, err := client.PackageMetadata(context.Background(), "pkg"); err != nil {
		t.Fatal(err)
	}
	if redirectedAuthorization != "" {
		t.Fatalf("redirect leaked credential across origins: %q", redirectedAuthorization)
	}
}

func TestEndpointTimeoutFallsBackInDeclaredOrder(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(PackageMetadata{Name: "pkg", Versions: map[string]PackageVersion{}})
	}))
	defer fallback.Close()
	primaryClient := NewHTTPRegistryClient(primary.URL)
	primaryClient.Client = &http.Client{Timeout: 10 * time.Millisecond}
	backend := NewFallbackRegistryBackend(SourceNPM, []endpointClient{
		{endpoint: RegistryEndpoint{URL: primary.URL}, client: primaryClient},
		{endpoint: RegistryEndpoint{URL: fallback.URL}, client: NewHTTPRegistryClient(fallback.URL)},
	}, true)
	metadata, err := backend.Metadata(context.Background(), "pkg")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Endpoint != fallback.URL {
		t.Fatalf("timeout fallback endpoint=%q want %q", metadata.Endpoint, fallback.URL)
	}
}

func TestFallbackExhaustionHasStructuredDiagnostic(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusBadGateway)
	}))
	defer second.Close()
	policy := SourcePolicy{AllowedSources: map[string]bool{SourceNPM: true}, Endpoints: map[string][]RegistryEndpoint{SourceNPM: {{URL: first.URL}, {URL: second.URL}}}}
	backends, err := policy.Backends(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = backends[SourceNPM].Metadata(context.Background(), "pkg")
	if err == nil || !strings.Contains(err.Error(), "TSPACK_REGISTRY_FALLBACK_EXHAUSTED") {
		t.Fatalf("unexpected exhaustion error: %v", err)
	}
}

func TestSameVersionDivergentBytesFailAgainstLockedContent(t *testing.T) {
	client := newRegistryFixture()
	addRegistryFixture(client, "pkg", "1.0.0", nil)
	dependency := manifest.DependencyIntent{Key: "pkg", Kind: "dep", Source: manifest.Source{Kind: SourceNPM, Package: "pkg", Range: "1.0.0"}}
	options := ResolverOptions{Mode: ResolveModeUpdate, Backends: BackendRegistry{SourceNPM: NewNPMBackend(client)}}
	graph := graphForDeps([]manifest.DependencyIntent{dependency}, nil, []string{"pkg"})
	first := Resolve(context.Background(), options, ResolveRequest{Graph: graph})
	if len(first.Diagnostics) != 0 {
		t.Fatalf("first resolution: %#v", first.Diagnostics)
	}
	artifactURL := client.meta["pkg"].Versions["1.0.0"].Dist.Tarball
	client.tar[artifactURL] = tarFor("pkg", "1.0.0", map[string]string{"unexpected": "1.0.0"}, nil, nil)
	second := Resolve(context.Background(), options, ResolveRequest{Graph: graph, ExistingLock: first.Lock})
	mustCode(t, second, "TSPACK_REGISTRY_TRUST_FAILED")
}
