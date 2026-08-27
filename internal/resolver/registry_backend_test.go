package resolver

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func TestResolveMixedNPMAndJSRGraphKeepsSourceIdentity(t *testing.T) {
	npmClient := newRegistryFixture()
	jsrClient := newRegistryFixture()
	addRegistryFixture(npmClient, "@scope/foo", "1.0.0", nil)
	addRegistryFixture(npmClient, "transitive-npm", "2.0.0", nil)
	addRegistryFixture(jsrClient, "@jsr/scope__child", "1.2.0", nil)
	addRegistryFixture(jsrClient, "@jsr/scope__foo", "1.0.0", map[string]string{
		"@jsr/scope__child": "^1.0.0",
		"transitive-npm":    "^2.0.0",
	})

	dependencies := []manifest.DependencyIntent{
		{Key: "npmFoo", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "@scope/foo", Range: "^1.0.0"}},
		{Key: "jsrFoo", Kind: "dep", Source: manifest.Source{Kind: "jsr", Package: "@scope/foo", Range: "^1.0.0"}},
	}
	graph := graphForDeps(dependencies, nil, []string{"npmFoo", "jsrFoo"})
	options := ResolverOptions{
		Mode: ResolveModeUpdate,
		Backends: BackendRegistry{
			SourceNPM: NewNPMBackend(npmClient),
			SourceJSR: NewJSRBackend(jsrClient),
		},
	}

	first := Resolve(context.Background(), options, ResolveRequest{Graph: graph})
	if len(first.Diagnostics) != 0 {
		t.Fatalf("mixed resolution diagnostics: %#v", first.Diagnostics)
	}
	assertHasPackage(t, first.Lock, "npm:@scope/foo@1.0.0")
	assertHasPackage(t, first.Lock, "jsr:@scope/foo@1.0.0")
	assertHasPackage(t, first.Lock, "jsr:@scope/child@1.2.0")
	assertHasPackage(t, first.Lock, "npm:transitive-npm@2.0.0")
	assertHasEdge(t, first.Lock, "jsr:@scope/foo@1.0.0", "jsr:@scope/child@1.2.0", "runtime", false)
	assertHasEdge(t, first.Lock, "jsr:@scope/foo@1.0.0", "npm:transitive-npm@2.0.0", "runtime", false)
	for _, pkg := range first.Lock.Packages {
		if pkg.Source == SourceJSR && len(pkg.Capabilities) != 0 {
			t.Fatalf("JSR package inherited install capabilities: %#v", pkg)
		}
	}

	second := Resolve(context.Background(), options, ResolveRequest{Graph: graph})
	firstBytes, err := lockfile.Marshal(first.Lock)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := lockfile.Marshal(second.Lock)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("mixed lock output is not deterministic:\nfirst:\n%s\nsecond:\n%s", firstBytes, secondBytes)
	}
}

func TestJSRBackendRejectsUnscopedPackageName(t *testing.T) {
	backend := NewJSRBackend(newRegistryFixture())
	_, err := backend.Metadata(context.Background(), "unscoped")
	if err == nil {
		t.Fatal("expected invalid JSR package name error")
	}
}

func TestJSRResolverReportsNotFoundAndIntegrityFailures(t *testing.T) {
	jsrClient := newRegistryFixture()
	dependency := manifest.DependencyIntent{
		Key:    "pkg",
		Kind:   "dep",
		Source: manifest.Source{Kind: "jsr", Package: "@scope/pkg", Range: "1.0.0"},
	}
	request := ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{dependency}, nil, []string{"pkg"})}
	options := ResolverOptions{
		Mode: ResolveModeUpdate,
		Backends: BackendRegistry{
			SourceJSR: NewJSRBackend(jsrClient),
		},
	}

	notFound := Resolve(context.Background(), options, request)
	mustCode(t, notFound, "TSPACK_JSR_PACKAGE_NOT_FOUND")

	addRegistryFixture(jsrClient, "@jsr/scope__pkg", "1.0.0", nil)
	version := jsrClient.meta["@jsr/scope__pkg"].Versions["1.0.0"]
	version.Dist.Integrity = "sha512-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="
	jsrClient.meta["@jsr/scope__pkg"].Versions["1.0.0"] = version
	integrityFailure := Resolve(context.Background(), options, request)
	mustCode(t, integrityFailure, "TSPACK_JSR_ARTIFACT_INTEGRITY_FAILED")
}

func TestRegistryVersionSelectionReusesSemVerForJSR(t *testing.T) {
	identity := PackageIdentity{Source: SourceJSR, Name: "@scope/pkg"}
	metadata := &RegistryPackageMetadata{Versions: map[string]RegistryPackageVersion{}}
	for _, version := range []string{"1.2.0", "1.2.3", "1.5.0", "2.0.0", "3.0.0-beta.1"} {
		metadata.Versions[version] = RegistryPackageVersion{Identity: identity, Version: version}
	}

	for _, test := range []struct {
		constraint string
		want       string
	}{
		{constraint: "1.2.3", want: "1.2.3"},
		{constraint: "^1.2.0", want: "1.5.0"},
		{constraint: "~1.2.0", want: "1.2.3"},
		{constraint: ">=1.2.0 <2.0.0", want: "1.5.0"},
		{constraint: "*", want: "2.0.0"},
		{constraint: "3.0.0-beta.1", want: "3.0.0-beta.1"},
	} {
		_, version, _, ok := selectRegistryVersion(metadata, test.constraint)
		if !ok || version != test.want {
			t.Errorf("constraint %q selected %q, ok=%t, want %q", test.constraint, version, ok, test.want)
		}
	}
}

func newRegistryFixture() *fakeClient {
	return &fakeClient{
		meta:      map[string]*PackageMetadata{},
		tar:       map[string][]byte{},
		metaErr:   map[string]error{},
		metaCalls: map[string]int{},
		tarCalls:  map[string]int{},
		metaDelay: map[string]time.Duration{},
		tarDelay:  map[string]time.Duration{},
	}
}

func addRegistryFixture(client *fakeClient, name string, version string, dependencies map[string]string) {
	artifactURL := "https://registry.example/" + name + "-" + version + ".tgz"
	artifact := tarFor(name, version, dependencies, nil, nil)
	client.meta[name] = &PackageMetadata{
		Name: name,
		Versions: map[string]PackageVersion{
			version: {
				Name:         name,
				Version:      version,
				Dependencies: dependencies,
				Dist:         PackageDist{Tarball: artifactURL},
			},
		},
	}
	client.tar[artifactURL] = artifact
}
