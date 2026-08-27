package resolver

import (
	"bytes"
	"context"
	"fmt"
	"strings"
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

func TestRequirementTapeProjectOverrideControlsTransitivePeers(t *testing.T) {
	client := newRegistryFixture()
	addRegistryFixture(client, "old-widget", "1.0.0", nil)
	addRegistryFixture(client, "new-widget", "1.0.0", nil)
	addRegistryFixture(client, "react", "18.3.0", nil)
	addRegistryFixture(client, "react", "19.1.0", nil)
	setFixturePeers(client, "old-widget", "1.0.0", map[string]string{"react": "^18"}, nil)
	setFixturePeers(client, "new-widget", "1.0.0", map[string]string{"react": "^19"}, nil)

	dependencies := []manifest.DependencyIntent{
		{Key: "old-widget", Kind: "dep", Source: manifest.Source{Kind: SourceNPM, Package: "old-widget", Range: "1.0.0"}},
		{Key: "new-widget", Kind: "dep", Source: manifest.Source{Kind: SourceNPM, Package: "new-widget", Range: "1.0.0"}},
		{Key: "react", Kind: "dep", Source: manifest.Source{Kind: SourceNPM, Package: "react", Range: "^19.1"}},
	}
	result := Resolve(context.Background(), ResolverOptions{
		Mode: ResolveModeUpdate,
		Backends: BackendRegistry{
			SourceNPM: NewNPMBackend(client),
		},
	}, ResolveRequest{Graph: graphForDeps(dependencies, nil, []string{"old-widget", "new-widget", "react"})})

	assertHasPackage(t, result.Lock, "npm:react@19.1.0")
	mustCode(t, result, "TSPACK_PEER_REQUIREMENT_OVERRIDDEN")
	assertRequirementStatus(t, result.Lock, "npm", "react", "^18", "overridden-incompatible", false)
	assertRequirementStatus(t, result.Lock, "npm", "react", "^19", "shadowed-compatible", false)
	assertRequirementStatus(t, result.Lock, "npm", "react", "^19.1", "controlling", true)
	for _, edge := range result.Lock.Edges {
		if edge.From == "npm:old-widget@1.0.0" && edge.To == "npm:react@19.1.0" {
			t.Fatalf("transitive peer became an ownership edge: %#v", edge)
		}
	}
	firstLock, err := lockfile.Marshal(result.Lock)
	if err != nil {
		t.Fatal(err)
	}
	client.metaDelay["old-widget"] = 20 * time.Millisecond
	client.metaDelay["new-widget"] = time.Millisecond
	client.metaDelay["react"] = 10 * time.Millisecond
	second := Resolve(context.Background(), ResolverOptions{
		Mode:     ResolveModeUpdate,
		Backends: BackendRegistry{SourceNPM: NewNPMBackend(client)},
	}, ResolveRequest{Graph: graphForDeps(dependencies, nil, []string{"old-widget", "new-widget", "react"})})
	secondLock, err := lockfile.Marshal(second.Lock)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstLock, secondLock) {
		t.Fatalf("requirement lock changed with metadata completion order:\nfirst:\n%s\nsecond:\n%s", firstLock, secondLock)
	}

	withoutOverride := Resolve(context.Background(), ResolverOptions{
		Mode:     ResolveModeUpdate,
		Backends: BackendRegistry{SourceNPM: NewNPMBackend(client)},
	}, ResolveRequest{Graph: graphForDeps(dependencies[:2], nil, []string{"old-widget", "new-widget"})})
	assertHasPackage(t, withoutOverride.Lock, "npm:react@18.3.0")
	assertRequirementStatus(t, withoutOverride.Lock, "npm", "react", "^18", "controlling", true)
	assertRequirementStatus(t, withoutOverride.Lock, "npm", "react", "^19", "overridden-incompatible", false)
}

func TestTransitivePeerCreatesEnvironmentSelectionWithoutChildOwnership(t *testing.T) {
	client := newRegistryFixture()
	addRegistryFixture(client, "widget", "1.0.0", nil)
	addRegistryFixture(client, "react", "19.1.0", nil)
	setFixturePeers(client, "widget", "1.0.0", map[string]string{"react": "^19"}, nil)
	dependency := manifest.DependencyIntent{Key: "widget", Kind: "dep", Source: manifest.Source{Kind: SourceNPM, Package: "widget", Range: "1.0.0"}}
	result := Resolve(context.Background(), ResolverOptions{
		Mode:     ResolveModeUpdate,
		Backends: BackendRegistry{SourceNPM: NewNPMBackend(client)},
	}, ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{dependency}, nil, []string{"widget"})})

	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics: %#v", result.Diagnostics)
	}
	assertHasPackage(t, result.Lock, "npm:react@19.1.0")
	assertRequirementStatus(t, result.Lock, "npm", "react", "^19", "controlling", true)
	assertHasEdge(t, result.Lock, "workspace:peer:npm:react", "npm:react@19.1.0", "peer", false)
	for _, edge := range result.Lock.Edges {
		if edge.From == "npm:widget@1.0.0" && edge.To == "npm:react@19.1.0" {
			t.Fatalf("peer became nested ownership: %#v", edge)
		}
	}
}

func TestJSRPeersRemainSourceQualifiedAcrossNPMAndJSR(t *testing.T) {
	npmClient := newRegistryFixture()
	jsrClient := newRegistryFixture()
	addRegistryFixture(npmClient, "react", "19.1.0", nil)
	addRegistryFixture(jsrClient, "@jsr/scope__parent", "1.0.0", nil)
	addRegistryFixture(jsrClient, "@jsr/scope__peer", "2.0.0", nil)
	addRegistryFixture(jsrClient, "@jsr/scope__react", "19.1.0", nil)
	setFixturePeers(jsrClient, "@jsr/scope__parent", "1.0.0", map[string]string{
		"react":            "^19",
		"@jsr/scope__peer": "^2",
	}, nil)
	dependencies := []manifest.DependencyIntent{
		{Key: "parent", Kind: "dep", Source: manifest.Source{Kind: SourceJSR, Package: "@scope/parent", Range: "1.0.0"}},
		{Key: "jsr-react", Kind: "dep", Source: manifest.Source{Kind: SourceJSR, Package: "@scope/react", Range: "19.1.0"}},
	}
	result := Resolve(context.Background(), ResolverOptions{
		Mode: ResolveModeUpdate,
		Backends: BackendRegistry{
			SourceNPM: NewNPMBackend(npmClient),
			SourceJSR: NewJSRBackend(jsrClient),
		},
	}, ResolveRequest{Graph: graphForDeps(dependencies, nil, []string{"parent", "jsr-react"})})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics: %#v", result.Diagnostics)
	}
	assertHasPackage(t, result.Lock, "npm:react@19.1.0")
	assertHasPackage(t, result.Lock, "jsr:@scope/react@19.1.0")
	assertHasPackage(t, result.Lock, "jsr:@scope/peer@2.0.0")
	assertRequirementStatus(t, result.Lock, SourceNPM, "react", "^19", "controlling", true)
	assertRequirementStatus(t, result.Lock, SourceJSR, "@scope/peer", "^2", "controlling", true)
}

func TestNPMSourcePackageCanExposeJSRPeerThroughNormalizedBackendMetadata(t *testing.T) {
	parentArtifact := tarFor("npm-parent", "1.0.0", nil, nil, nil)
	parentBackend := &staticRegistryBackend{
		source: SourceNPM,
		metadata: map[string]*RegistryPackageMetadata{
			"npm-parent": {
				Identity: PackageIdentity{Source: SourceNPM, Name: "npm-parent"},
				Versions: map[string]RegistryPackageVersion{
					"1.0.0": {
						Identity:            PackageIdentity{Source: SourceNPM, Name: "npm-parent"},
						Version:             "1.0.0",
						Artifact:            ArtifactDescriptor{Kind: "tarball", URL: "parent.tgz"},
						ArtifactPackageName: "npm-parent",
						PeerRequirements: []DependencyRequirement{{
							Identity:   PackageIdentity{Source: SourceJSR, Name: "@scope/peer"},
							Reference:  "@jsr/scope__peer",
							Constraint: "^2",
							Kind:       "peer",
						}},
					},
				},
			},
		},
		artifacts: map[string][]byte{"parent.tgz": parentArtifact},
	}
	jsrClient := newRegistryFixture()
	addRegistryFixture(jsrClient, "@jsr/scope__peer", "2.0.0", nil)
	dependency := manifest.DependencyIntent{Key: "parent", Kind: "dep", Source: manifest.Source{Kind: SourceNPM, Package: "npm-parent", Range: "1.0.0"}}
	result := Resolve(context.Background(), ResolverOptions{
		Mode: ResolveModeUpdate,
		Backends: BackendRegistry{
			SourceNPM: parentBackend,
			SourceJSR: NewJSRBackend(jsrClient),
		},
	}, ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{dependency}, nil, []string{"parent"})})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics: %#v", result.Diagnostics)
	}
	assertHasPackage(t, result.Lock, "jsr:@scope/peer@2.0.0")
	assertRequirementStatus(t, result.Lock, SourceJSR, "@scope/peer", "^2", "controlling", true)
}

func TestOptionalPeerFailureIsVisibleButNonFatal(t *testing.T) {
	client := newRegistryFixture()
	addRegistryFixture(client, "widget", "1.0.0", nil)
	setFixturePeers(client, "widget", "1.0.0", map[string]string{"missing-peer": "^1"}, map[string]bool{"missing-peer": true})
	dependency := manifest.DependencyIntent{Key: "widget", Kind: "dep", Source: manifest.Source{Kind: SourceNPM, Package: "widget", Range: "1.0.0"}}
	result := Resolve(context.Background(), ResolverOptions{
		Mode:     ResolveModeUpdate,
		Backends: BackendRegistry{SourceNPM: NewNPMBackend(client)},
	}, ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{dependency}, nil, []string{"widget"})})
	if hasErrorDiagnostics(result.Diagnostics) {
		t.Fatalf("optional peer was fatal: %#v", result.Diagnostics)
	}
	assertRequirementStatus(t, result.Lock, SourceNPM, "missing-peer", "^1", "optional-unsatisfied", true)
}

func TestRegistryAliasSeparatesReferenceFromSemanticTarget(t *testing.T) {
	client := newRegistryFixture()
	addRegistryFixture(client, "parent", "1.0.0", map[string]string{"foo": "npm:bar@^2"})
	addRegistryFixture(client, "bar", "2.1.0", nil)
	dependency := manifest.DependencyIntent{Key: "parent", Kind: "dep", Source: manifest.Source{Kind: SourceNPM, Package: "parent", Range: "1.0.0"}}
	result := Resolve(context.Background(), ResolverOptions{
		Mode:     ResolveModeUpdate,
		Backends: BackendRegistry{SourceNPM: NewNPMBackend(client)},
	}, ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{dependency}, nil, []string{"parent"})})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics: %#v", result.Diagnostics)
	}
	assertHasPackage(t, result.Lock, "npm:bar@2.1.0")
	for _, edge := range result.Lock.Edges {
		if edge.From == "npm:parent@1.0.0" && edge.To == "npm:bar@2.1.0" {
			if edge.Reference != "foo" {
				t.Fatalf("alias edge reference = %q, want foo", edge.Reference)
			}
			return
		}
	}
	t.Fatal("missing alias edge to semantic npm:bar target")
}

func TestParseScopedNPMAlias(t *testing.T) {
	identity, constraint, err := parseNPMAlias("npm:@scope/bar@^2")
	if err != nil {
		t.Fatal(err)
	}
	if identity.Source != SourceNPM || identity.Name != "@scope/bar" || constraint != "^2" {
		t.Fatalf("parsed alias = %#v %q", identity, constraint)
	}
	if _, _, err := parseNPMAlias("npm:bar@"); err == nil {
		t.Fatal("expected empty alias constraint to fail")
	}
}

func TestJSRBackendRejectsUnscopedPackageName(t *testing.T) {
	backend := NewJSRBackend(newRegistryFixture())
	_, err := backend.Metadata(context.Background(), "unscoped")
	if err == nil {
		t.Fatal("expected invalid JSR package name error")
	}
}

func TestJSRCompatibilityLifecycleScriptsDoNotBecomeCapabilities(t *testing.T) {
	jsrClient := newRegistryFixture()
	name := "@jsr/scope__pkg"
	version := "1.0.0"
	artifactURL := "https://registry.example/jsr-scripted.tgz"
	scripts := map[string]string{
		"preinstall":  "node preinstall.js",
		"postinstall": "node postinstall.js",
	}
	body := tarFor(name, version, nil, nil, scripts)
	jsrClient.meta[name] = &PackageMetadata{
		Name: name,
		Versions: map[string]PackageVersion{
			version: {
				Name:    name,
				Version: version,
				Dist:    PackageDist{Tarball: artifactURL},
				Scripts: scripts,
			},
		},
	}
	jsrClient.tar[artifactURL] = body

	dependency := manifest.DependencyIntent{
		Key:    "pkg",
		Kind:   "dep",
		Source: manifest.Source{Kind: "jsr", Package: "@scope/pkg", Range: version},
	}
	result := Resolve(context.Background(), ResolverOptions{
		Mode: ResolveModeUpdate,
		Backends: BackendRegistry{
			SourceJSR: NewJSRBackend(jsrClient),
		},
	}, ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{dependency}, nil, []string{"pkg"})})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("resolve diagnostics: %#v", result.Diagnostics)
	}
	if len(result.Lock.Packages) != 1 {
		t.Fatalf("unexpected packages: %#v", result.Lock.Packages)
	}
	if len(result.Lock.Packages[0].Capabilities) != 0 {
		t.Fatalf("JSR compatibility scripts became executable capabilities: %#v", result.Lock.Packages[0].Capabilities)
	}
}

func TestJSRCompatibilityNamesAreScopedAndBijective(t *testing.T) {
	valid := []struct {
		logical       string
		compatibility string
	}{
		{logical: "@scope/package", compatibility: "@jsr/scope__package"},
		{logical: "@scope_name/package_name", compatibility: "@jsr/scope_name__package_name"},
		{logical: "@scope-name/package-name", compatibility: "@jsr/scope-name__package-name"},
	}
	for _, test := range valid {
		compatibility, err := jsrCompatibilityName(test.logical)
		if err != nil || compatibility != test.compatibility {
			t.Fatalf("compatibility name for %q = %q, %v", test.logical, compatibility, err)
		}
		logical, err := logicalJSRName(compatibility)
		if err != nil || logical != test.logical {
			t.Fatalf("logical name for %q = %q, %v", compatibility, logical, err)
		}
	}

	invalidLogical := []string{
		"package",
		"@scope",
		"@scope/",
		"@/package",
		"@scope/package/subpath",
		"@scope__nested/package",
		"@scope/package__variant",
	}
	for _, name := range invalidLogical {
		if compatibility, err := jsrCompatibilityName(name); err == nil {
			t.Errorf("ambiguous or malformed logical name %q produced %q", name, compatibility)
		}
	}

	invalidCompatibility := []string{
		"scope__package",
		"@jsr/scope",
		"@jsr/__package",
		"@jsr/scope__",
		"@jsr/scope__package__variant",
		"@jsr/scope/name__package",
	}
	for _, name := range invalidCompatibility {
		if logical, err := logicalJSRName(name); err == nil {
			t.Errorf("ambiguous or malformed compatibility name %q produced %q", name, logical)
		}
	}
}

func TestRegistryOptionalDependenciesPreserveSourceIdentity(t *testing.T) {
	npmClient := newRegistryFixture()
	addRegistryFixture(npmClient, "optional-parent", "1.0.0", nil)
	npmVersion := npmClient.meta["optional-parent"].Versions["1.0.0"]
	npmVersion.OptionalDependencies = map[string]string{"@scope/foo": "^1.0.0"}
	npmClient.meta["optional-parent"].Versions["1.0.0"] = npmVersion

	npmMetadata, err := NewNPMBackend(npmClient).Metadata(context.Background(), "optional-parent")
	if err != nil {
		t.Fatal(err)
	}
	assertOptionalRequirement(t, npmMetadata.Versions["1.0.0"].Dependencies, SourceNPM, "@scope/foo", "^1.0.0")

	jsrClient := newRegistryFixture()
	addRegistryFixture(jsrClient, "@jsr/scope__parent", "1.0.0", nil)
	jsrVersion := jsrClient.meta["@jsr/scope__parent"].Versions["1.0.0"]
	jsrVersion.OptionalDependencies = map[string]string{
		"@jsr/scope__child": "^2.0.0",
		"optional-npm":      "^3.0.0",
	}
	jsrClient.meta["@jsr/scope__parent"].Versions["1.0.0"] = jsrVersion

	jsrMetadata, err := NewJSRBackend(jsrClient).Metadata(context.Background(), "@scope/parent")
	if err != nil {
		t.Fatal(err)
	}
	dependencies := jsrMetadata.Versions["1.0.0"].Dependencies
	assertOptionalRequirement(t, dependencies, SourceJSR, "@scope/child", "^2.0.0")
	assertOptionalRequirement(t, dependencies, SourceNPM, "optional-npm", "^3.0.0")
}

func TestResolveMixedOptionalDependenciesPreservesSourcesInLock(t *testing.T) {
	npmClient := newRegistryFixture()
	addRegistryFixture(npmClient, "optional-parent", "1.0.0", nil)
	addRegistryFixture(npmClient, "@scope/foo", "1.0.0", nil)
	addRegistryFixture(npmClient, "optional-npm", "3.0.0", nil)
	npmParent := npmClient.meta["optional-parent"].Versions["1.0.0"]
	npmParent.OptionalDependencies = map[string]string{"@scope/foo": "^1.0.0"}
	npmClient.meta["optional-parent"].Versions["1.0.0"] = npmParent

	jsrClient := newRegistryFixture()
	addRegistryFixture(jsrClient, "@jsr/scope__parent", "1.0.0", nil)
	addRegistryFixture(jsrClient, "@jsr/scope__child", "2.0.0", nil)
	addRegistryFixture(jsrClient, "@jsr/scope__foo", "1.0.0", nil)
	jsrParent := jsrClient.meta["@jsr/scope__parent"].Versions["1.0.0"]
	jsrParent.OptionalDependencies = map[string]string{
		"@jsr/scope__child": "^2.0.0",
		"optional-npm":      "^3.0.0",
	}
	jsrClient.meta["@jsr/scope__parent"].Versions["1.0.0"] = jsrParent

	dependencies := []manifest.DependencyIntent{
		{Key: "npmParent", Kind: "dep", Source: manifest.Source{Kind: SourceNPM, Package: "optional-parent", Range: "1.0.0"}},
		{Key: "jsrParent", Kind: "dep", Source: manifest.Source{Kind: SourceJSR, Package: "@scope/parent", Range: "1.0.0"}},
		{Key: "jsrFoo", Kind: "dep", Source: manifest.Source{Kind: SourceJSR, Package: "@scope/foo", Range: "1.0.0"}},
	}
	result := Resolve(context.Background(), ResolverOptions{
		Mode: ResolveModeUpdate,
		Backends: BackendRegistry{
			SourceNPM: NewNPMBackend(npmClient),
			SourceJSR: NewJSRBackend(jsrClient),
		},
	}, ResolveRequest{Graph: graphForDeps(dependencies, nil, []string{"npmParent", "jsrParent", "jsrFoo"})})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("mixed optional resolution diagnostics: %#v", result.Diagnostics)
	}

	assertHasEdge(t, result.Lock, "npm:optional-parent@1.0.0", "npm:@scope/foo@1.0.0", "runtime", true)
	assertHasEdge(t, result.Lock, "jsr:@scope/parent@1.0.0", "jsr:@scope/child@2.0.0", "runtime", true)
	assertHasEdge(t, result.Lock, "jsr:@scope/parent@1.0.0", "npm:optional-npm@3.0.0", "runtime", true)
	assertHasPackage(t, result.Lock, "npm:@scope/foo@1.0.0")
	assertHasPackage(t, result.Lock, "jsr:@scope/foo@1.0.0")
}

func TestJSROptionalAliasErrorIdentifiesSourceQualifiedParent(t *testing.T) {
	jsrClient := newRegistryFixture()
	addRegistryFixture(jsrClient, "@jsr/scope__parent", "1.0.0", nil)
	version := jsrClient.meta["@jsr/scope__parent"].Versions["1.0.0"]
	version.OptionalDependencies = map[string]string{"@jsr/ambiguous__package__name": "^1.0.0"}
	jsrClient.meta["@jsr/scope__parent"].Versions["1.0.0"] = version

	_, err := NewJSRBackend(jsrClient).Metadata(context.Background(), "@scope/parent")
	if err == nil {
		t.Fatal("expected malformed optional compatibility name to fail")
	}
	if !strings.Contains(err.Error(), "normalize optional dependencies for jsr:@scope/parent@1.0.0") {
		t.Fatalf("error does not identify source-qualified parent: %v", err)
	}
}

func assertOptionalRequirement(t *testing.T, requirements []DependencyRequirement, source string, name string, constraint string) {
	t.Helper()
	for _, requirement := range requirements {
		if requirement.Identity.Source == source && requirement.Identity.Name == name && requirement.Constraint == constraint && requirement.Optional {
			return
		}
	}
	t.Fatalf("missing optional requirement %s:%s %s in %#v", source, name, constraint, requirements)
}

func setFixturePeers(client *fakeClient, name string, version string, peers map[string]string, optional map[string]bool) {
	packageVersion := client.meta[name].Versions[version]
	packageVersion.PeerDependencies = peers
	packageVersion.PeerDependenciesMeta = map[string]struct {
		Optional bool `json:"optional"`
	}{}
	for peerName, isOptional := range optional {
		packageVersion.PeerDependenciesMeta[peerName] = struct {
			Optional bool `json:"optional"`
		}{Optional: isOptional}
	}
	client.meta[name].Versions[version] = packageVersion
}

func assertRequirementStatus(t *testing.T, lock *lockfile.Lockfile, source string, name string, constraint string, status string, controlling bool) {
	t.Helper()
	for _, requirement := range lock.Requirements {
		if requirement.TargetSource == source && requirement.TargetName == name && requirement.Constraint == constraint && requirement.Status == status && requirement.Controlling == controlling {
			return
		}
	}
	t.Fatalf("missing requirement %s:%s %s status=%s controlling=%t in %#v", source, name, constraint, status, controlling, lock.Requirements)
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

type staticRegistryBackend struct {
	source    string
	metadata  map[string]*RegistryPackageMetadata
	artifacts map[string][]byte
}

func (backend *staticRegistryBackend) Source() string {
	return backend.source
}

func (backend *staticRegistryBackend) Metadata(_ context.Context, name string) (*RegistryPackageMetadata, error) {
	metadata, ok := backend.metadata[name]
	if !ok {
		return nil, fmt.Errorf("not found: %s", name)
	}
	return metadata, nil
}

func (backend *staticRegistryBackend) FetchArtifact(_ context.Context, artifact ArtifactDescriptor) ([]byte, error) {
	body, ok := backend.artifacts[artifact.URL]
	if !ok {
		return nil, fmt.Errorf("not found: %s", artifact.URL)
	}
	return body, nil
}

func (backend *staticRegistryBackend) Host() string {
	return "registry.example"
}

func addRegistryFixture(client *fakeClient, name string, version string, dependencies map[string]string) {
	artifactURL := "https://registry.example/" + name + "-" + version + ".tgz"
	artifact := tarFor(name, version, dependencies, nil, nil)
	if client.meta[name] == nil {
		client.meta[name] = &PackageMetadata{Name: name, Versions: map[string]PackageVersion{}}
	}
	client.meta[name].Versions[version] = PackageVersion{
		Name:         name,
		Version:      version,
		Dependencies: dependencies,
		Dist:         PackageDist{Tarball: artifactURL},
	}
	client.tar[artifactURL] = artifact
}
