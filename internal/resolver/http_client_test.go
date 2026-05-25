package resolver

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tspack/tspack/internal/manifest"
)

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

func TestHTTPRegistryClientScopedPackageEncoding(t *testing.T) {
	hit := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = r.URL.Path
		_ = json.NewEncoder(w).Encode(PackageMetadata{Name: "@scope/pkg", Versions: map[string]PackageVersion{}})
	}))
	defer server.Close()

	c := NewHTTPRegistryClient(server.URL)
	_, _ = c.PackageMetadata(context.Background(), "@scope/pkg")
	if hit != "/@scope%2Fpkg" {
		t.Fatalf("expected scoped path encoding, got %s", hit)
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
