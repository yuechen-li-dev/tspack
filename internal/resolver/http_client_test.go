package resolver

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func TestPackageVersionIgnoresNonStringHistoricalScriptValues(t *testing.T) {
	var version PackageVersion
	err := json.Unmarshal([]byte(`{
		"name":"legacy",
		"version":"0.1.0",
		"scripts":{"install":"node install.js","blanket":{"pattern":"test/**"}}
	}`), &version)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(version.Scripts, map[string]string{"install": "node install.js"}) {
		t.Fatalf("scripts=%#v", version.Scripts)
	}
}

func TestHTTPRegistryClientPackageURL(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		packageName string
		wantURL     string
	}{
		{
			name:        "unscoped react",
			baseURL:     "https://registry.npmjs.org",
			packageName: "react",
			wantURL:     "https://registry.npmjs.org/react",
		},
		{
			name:        "unscoped left pad",
			baseURL:     "https://registry.npmjs.org",
			packageName: "left-pad",
			wantURL:     "https://registry.npmjs.org/left-pad",
		},
		{
			name:        "types scoped",
			baseURL:     "https://registry.npmjs.org",
			packageName: "@types/react",
			wantURL:     "https://registry.npmjs.org/@types%2Freact",
		},
		{
			name:        "biome scoped",
			baseURL:     "https://registry.npmjs.org",
			packageName: "@biomejs/biome",
			wantURL:     "https://registry.npmjs.org/@biomejs%2Fbiome",
		},
		{
			name:        "babel scoped",
			baseURL:     "https://registry.npmjs.org",
			packageName: "@babel/core",
			wantURL:     "https://registry.npmjs.org/@babel%2Fcore",
		},
		{
			name:        "custom trailing slash",
			baseURL:     "https://registry.example.test/npm/",
			packageName: "@types/react",
			wantURL:     "https://registry.example.test/npm/@types%2Freact",
		},
		{
			name:        "custom no trailing slash",
			baseURL:     "https://registry.example.test/npm",
			packageName: "react",
			wantURL:     "https://registry.example.test/npm/react",
		},
		{
			name:        "custom path prefix",
			baseURL:     "https://registry.example.test/custom/npm",
			packageName: "@babel/core",
			wantURL:     "https://registry.example.test/custom/npm/@babel%2Fcore",
		},
		{
			name:        "custom path prefix with query",
			baseURL:     "https://registry.example.test/custom/npm/?cache=metadata",
			packageName: "@types/react",
			wantURL:     "https://registry.example.test/custom/npm/@types%2Freact?cache=metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewHTTPRegistryClient(tt.baseURL)
			got, err := c.packageURL(tt.packageName)
			if err != nil {
				t.Fatalf("packageURL returned error: %v", err)
			}
			if got != tt.wantURL {
				t.Fatalf("packageURL() = %q, want %q", got, tt.wantURL)
			}
			if strings.HasPrefix(tt.packageName, "@") && strings.Contains(got, "%25") {
				t.Fatalf("scoped package URL is double encoded: %s", got)
			}
		})
	}
}

func TestHTTPRegistryClientFetchesMetadataAndTarball(t *testing.T) {
	tarball := makeClientTarball(t, "pkg-a", "1.0.0")
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	mux.HandleFunc("/pkg-a", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(PackageMetadata{Name: "pkg-a", Versions: map[string]PackageVersion{"1.0.0": {Name: "pkg-a", Version: "1.0.0", Dist: PackageDist{Tarball: server.URL + "/tarballs/pkg-a-1.0.0.tgz"}}}})
	})
	mux.HandleFunc("/tarballs/pkg-a-1.0.0.tgz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	})

	c := NewHTTPRegistryClient(server.URL)
	meta, err := c.PackageMetadata(context.Background(), "pkg-a")
	if err != nil || meta.Name != "pkg-a" {
		t.Fatalf("metadata err=%v meta=%+v", err, meta)
	}
	b, err := c.Tarball(context.Background(), server.URL+"/tarballs/pkg-a-1.0.0.tgz")
	if err != nil || len(b) == 0 {
		t.Fatalf("tarball err=%v bytes=%d", err, len(b))
	}
}

func TestHTTPRegistryClientUsesSharedTunedTransport(t *testing.T) {
	first := NewHTTPRegistryClient("")
	second := NewHTTPRegistryClient("")

	if first.Client != second.Client {
		t.Fatalf("expected shared registry http client")
	}

	transport, ok := first.Client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", first.Client.Transport)
	}
	if transport.MaxIdleConns != 128 {
		t.Fatalf("MaxIdleConns=%d want 128", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 64 {
		t.Fatalf("MaxIdleConnsPerHost=%d want 64", transport.MaxIdleConnsPerHost)
	}
	if !transport.ForceAttemptHTTP2 {
		t.Fatalf("ForceAttemptHTTP2=false want true")
	}
	if transport.Proxy == nil {
		t.Fatalf("expected proxy function to be configured")
	}
}

func TestHTTPRegistryClientNilClientFallsBackToSharedTransport(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(PackageMetadata{Name: "pkg-a", Versions: map[string]PackageVersion{}})
	}))
	server.Listener = listener
	server.Start()
	defer server.Close()

	client := &HTTPRegistryClient{BaseURL: server.URL}
	meta, err := client.PackageMetadata(context.Background(), "pkg-a")
	if err != nil {
		t.Fatalf("PackageMetadata returned error: %v", err)
	}
	if meta.Name != "pkg-a" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
}

func TestHTTPRegistryClientScopedPackageEncoding(t *testing.T) {
	hit := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = r.RequestURI
		if strings.Contains(hit, "%25") {
			http.Error(w, "double encoded path", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(PackageMetadata{Name: "@scope/pkg", Versions: map[string]PackageVersion{}})
	}))
	defer server.Close()

	c := NewHTTPRegistryClient(server.URL)
	_, err := c.PackageMetadata(context.Background(), "@scope/pkg")
	if err != nil {
		t.Fatalf("PackageMetadata returned error: %v", err)
	}
	if hit != "/@scope%2Fpkg" {
		t.Fatalf("expected scoped path encoding, got %s", hit)
	}
}

func TestResolveNPMScopedPackageUsesEncodedMetadataPath(t *testing.T) {
	metadataRequests := []string{}
	var metadataMu sync.Mutex
	typesTarball := makeClientTarball(t, "@types/react", "1.0.0")
	biomeTarball := makeClientTarball(t, "@biomejs/biome", "2.0.0")
	babelTarball := makeClientTarball(t, "@babel/core", "7.0.0")

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.RequestURI, "%25") {
			http.Error(w, "double encoded path", http.StatusBadRequest)
			return
		}

		switch r.RequestURI {
		case "/@types%2Freact":
			metadataMu.Lock()
			metadataRequests = append(metadataRequests, r.RequestURI)
			metadataMu.Unlock()
			metadata := scopedPackageMetadata(
				"@types/react",
				"1.0.0",
				server.URL+"/tarballs/types-react-1.0.0.tgz",
			)
			_ = json.NewEncoder(w).Encode(metadata)
		case "/@biomejs%2Fbiome":
			metadataMu.Lock()
			metadataRequests = append(metadataRequests, r.RequestURI)
			metadataMu.Unlock()
			metadata := scopedPackageMetadata(
				"@biomejs/biome",
				"2.0.0",
				server.URL+"/tarballs/biome-2.0.0.tgz",
			)
			_ = json.NewEncoder(w).Encode(metadata)
		case "/@babel%2Fcore":
			metadataMu.Lock()
			metadataRequests = append(metadataRequests, r.RequestURI)
			metadataMu.Unlock()
			metadata := scopedPackageMetadata(
				"@babel/core",
				"7.0.0",
				server.URL+"/tarballs/babel-core-7.0.0.tgz",
			)
			_ = json.NewEncoder(w).Encode(metadata)
		case "/tarballs/types-react-1.0.0.tgz":
			_, _ = w.Write(typesTarball)
		case "/tarballs/biome-2.0.0.tgz":
			_, _ = w.Write(biomeTarball)
		case "/tarballs/babel-core-7.0.0.tgz":
			_, _ = w.Write(babelTarball)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	deps := []manifest.DependencyIntent{
		{
			Key:  "@types/react",
			Kind: "dep",
			Source: manifest.Source{
				Kind:    "npm",
				Package: "@types/react",
				Range:   "1.0.0",
			},
		},
		{
			Key:  "@biomejs/biome",
			Kind: "dep",
			Source: manifest.Source{
				Kind:    "npm",
				Package: "@biomejs/biome",
				Range:   "2.0.0",
			},
		},
		{
			Key:  "@babel/core",
			Kind: "dep",
			Source: manifest.Source{
				Kind:    "npm",
				Package: "@babel/core",
				Range:   "7.0.0",
			},
		},
	}
	res := ResolveNPM(
		context.Background(),
		ResolverOptions{Client: NewHTTPRegistryClient(server.URL), Mode: ResolveModeUpdate},
		ResolveRequest{Graph: graphForDeps(deps, nil, []string{"@types/react", "@biomejs/biome", "@babel/core"})},
	)
	if len(res.Diagnostics) > 0 {
		t.Fatalf("resolver diagnostics: %#v", res.Diagnostics)
	}
	assertHasPackage(t, res.Lock, "npm:@types/react@1.0.0")
	assertHasPackage(t, res.Lock, "npm:@biomejs/biome@2.0.0")
	assertHasPackage(t, res.Lock, "npm:@babel/core@7.0.0")
	metadataMu.Lock()
	defer metadataMu.Unlock()
	if len(metadataRequests) != 3 {
		t.Fatalf("expected three metadata requests, got %#v", metadataRequests)
	}
	for _, requestPath := range metadataRequests {
		if requestPath != "/@types%2Freact" && requestPath != "/@biomejs%2Fbiome" && requestPath != "/@babel%2Fcore" {
			t.Fatalf("unexpected metadata request path: %s", requestPath)
		}
	}
}

func scopedPackageMetadata(name, version, tarballURL string) PackageMetadata {
	return PackageMetadata{
		Name: name,
		Versions: map[string]PackageVersion{
			version: {
				Name:    name,
				Version: version,
				Dist: PackageDist{
					Tarball: tarballURL,
				},
			},
		},
	}
}

func TestResolveNPMTarballFetchFailureIncludesPackageVersion(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	mux.HandleFunc("/pkg-a", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(PackageMetadata{Name: "pkg-a", Versions: map[string]PackageVersion{"1.0.0": {Name: "pkg-a", Version: "1.0.0", Dist: PackageDist{Tarball: server.URL + "/missing.tgz"}}}})
	})
	res := ResolveNPM(context.Background(), ResolverOptions{Client: NewHTTPRegistryClient(server.URL), Mode: ResolveModeUpdate}, ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{{Key: "pkg-a", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "pkg-a", Range: "1.0.0"}}}, nil, []string{"pkg-a"})})
	mustCode(t, res, "TSPACK_RESOLVE_NPM_TARBALL_ERROR")
	mustContainDetail(t, res, "npm:pkg-a@1.0.0")
}

func mustContainDetail(t *testing.T, res ResolveResult, want string) {
	t.Helper()
	for _, d := range res.Diagnostics {
		for _, detail := range d.Details {
			if detail == want {
				return
			}
		}
	}
	t.Fatalf("missing detail %q in %#v", want, res.Diagnostics)
}

func makeClientTarball(t *testing.T, name, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	pj, _ := json.Marshal(map[string]any{"name": name, "version": version})
	_ = tw.WriteHeader(&tar.Header{Name: "package/package.json", Mode: 0o644, Size: int64(len(pj))})
	_, _ = tw.Write(pj)
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}
