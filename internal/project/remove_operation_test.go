package project

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/authoring"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/resolver"
)

func TestParseRemovePackageSelector(t *testing.T) {
	for _, value := range []string{"foo", "@scope/foo"} {
		identity, err := ParseRemovePackageSelector(value)
		if err != nil {
			t.Fatalf("ParseRemovePackageSelector(%q): %v", value, err)
		}
		if identity.Name != value || identity.Source != "npm" {
			t.Fatalf("identity = %#v", identity)
		}
	}
	for _, value := range []string{"", "foo@^4", "@scope/foo@^3", "file:../foo"} {
		if _, err := ParseRemovePackageSelector(value); err == nil {
			t.Fatalf("ParseRemovePackageSelector(%q) unexpectedly succeeded", value)
		}
	}
}

func TestRunRemoveDependencyWritesManifestAndLockThenBecomesNoop(t *testing.T) {
	frontendPath := addFrontendPath(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.tsx")
	original := `import { Package, Workspace, define } from "tspack/manifest";
export default define(<Workspace name="demo"><Package name="app" version="1.0.0" kind="app" /></Workspace>);
`
	if err := os.WriteFile(manifestPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	client := addRegistryClient("lodash", "4.17.21")
	options := DefaultOptions(root)
	options.FrontendCLIPath = frontendPath
	options.ResolverClient = client
	added := RunAddDependency(AddDependencyRequest{Project: options, PackageSpec: "lodash"})
	if hasErrors(added.Diagnostics) {
		t.Fatalf("add failed: %#v", added.Diagnostics)
	}

	result := RunRemoveDependency(RemoveDependencyRequest{Project: options, PackageSpec: "lodash"})
	if hasErrors(result.Diagnostics) {
		t.Fatalf("remove failed: %#v", result.Diagnostics)
	}
	if !result.DeclarationRemoved || !result.ManifestChanged || !result.LockChanged || result.StillRequired || result.StillResolved || !result.LockPackageRemoved {
		t.Fatalf("unexpected remove result: %#v", result)
	}
	if result.Performance.RegistryMetadataRequests != 0 || result.Performance.RegistryTarballRequests != 0 {
		t.Fatalf("last-dependency removal made registry requests: %#v", result.Performance)
	}
	after, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(after, []byte("lodash")) {
		t.Fatalf("remove left the dependency declaration in source:\n%s", after)
	}
	locked, diagnostics, err := lockfile.LoadFile(options.LockfilePath)
	if err != nil || hasErrors(diagnostics) || len(locked.Packages) != 0 {
		t.Fatalf("lock after remove: %v %#v %#v", err, diagnostics, locked)
	}

	manifestBeforeNoop := append([]byte(nil), after...)
	lockBeforeNoop, _ := os.ReadFile(options.LockfilePath)
	second := RunRemoveDependency(RemoveDependencyRequest{Project: options, PackageSpec: "lodash"})
	if hasErrors(second.Diagnostics) || !second.NoEditableDeclaration || second.ManifestChanged || second.LockChanged {
		t.Fatalf("repeated remove = %#v", second)
	}
	manifestAfterNoop, _ := os.ReadFile(manifestPath)
	lockAfterNoop, _ := os.ReadFile(options.LockfilePath)
	if !bytes.Equal(manifestBeforeNoop, manifestAfterNoop) || !bytes.Equal(lockBeforeNoop, lockAfterNoop) {
		t.Fatal("repeated remove rewrote project files")
	}

	readded := RunAddDependency(AddDependencyRequest{Project: options, PackageSpec: "lodash"})
	if hasErrors(readded.Diagnostics) {
		t.Fatalf("re-add failed: %#v", readded.Diagnostics)
	}
	rereremoved := RunRemoveDependency(RemoveDependencyRequest{Project: options, PackageSpec: "lodash"})
	if hasErrors(rereremoved.Diagnostics) {
		t.Fatalf("second remove after re-add failed: %#v", rereremoved.Diagnostics)
	}
	converged, _ := os.ReadFile(manifestPath)
	if !bytes.Equal(manifestBeforeNoop, converged) {
		t.Fatalf("add/remove/add/remove did not converge:\n%s", converged)
	}
}

func TestRunRemoveDependencyDryRunUnshadowsConceptDeclaration(t *testing.T) {
	frontendPath := addFrontendPath(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.tsx")
	original := `import { Package, Workspace, define, dep, npm } from "tspack/manifest";
export default define(
  <Workspace name="demo">
    <Package
      name="app"
      version="1.0.0"
      kind="app"
      dependencyDeclaration={{
        origin: { kind: "concept", name: "ReactLibrary" },
        layer: "concept",
        authority: "generated",
        editability: "concept-owned",
      }}
      dependencies={{
        values: [
          dep(npm("typescript", "^5.9")),
          dep(npm("typescript", "^6"), {
            declaration: {
              origin: { kind: "explicit-user-operation", name: "tspack add" },
              layer: "explicit",
              authority: "owned",
              editability: "editable",
            },
          }),
        ],
      }}
    />
  </Workspace>,
);
`
	if err := os.WriteFile(manifestPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	options := DefaultOptions(root)
	options.FrontendCLIPath = frontendPath
	result := RunRemoveDependency(RemoveDependencyRequest{Project: options, PackageSpec: "typescript", DryRun: true})
	if hasErrors(result.Diagnostics) {
		t.Fatalf("remove failed: %#v", result.Diagnostics)
	}
	if !result.DeclarationRemoved || !result.ManifestChanged || !result.StillRequired || result.NewlyEffectiveDeclaration == nil {
		t.Fatalf("unexpected unshadow result: %#v", result)
	}
	if result.NewlyEffectiveDeclaration.Constraint != "^5.9" || result.NewlyEffectiveDeclaration.Origin.Name != "ReactLibrary" {
		t.Fatalf("newly effective = %#v", result.NewlyEffectiveDeclaration)
	}
	after, _ := os.ReadFile(manifestPath)
	if !bytes.Equal(after, []byte(original)) {
		t.Fatal("dry-run wrote the manifest")
	}
}

func TestRunRemoveDependencyAmbiguityOptionalAndWorkspaceTargeting(t *testing.T) {
	declarations := []authoring.DependencyDeclaration{
		removeTestDeclaration("normal", false),
		removeTestDeclaration("optional", true),
	}
	candidates := filterRemoveCandidates(declarations, "", nil)
	if len(candidates) != 2 {
		t.Fatalf("unqualified candidates = %#v", candidates)
	}
	optional := true
	candidates = filterRemoveCandidates(declarations, "", &optional)
	if len(candidates) != 1 || !candidates[0].Optional {
		t.Fatalf("optional candidates = %#v", candidates)
	}

	frontendPath := addFrontendPath(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.tsx")
	source := `import { Package, Workspace, define, dep, npm } from "tspack/manifest";
export default define(<Workspace name="demo"><Package name="one" version="1.0.0" kind="app" dependencies={{ values: [dep(npm("lodash", "^4"))] }} /><Package name="two" version="1.0.0" kind="app" dependencies={{ values: [dep(npm("lodash", "^4"))] }} /></Workspace>);
`
	if err := os.WriteFile(manifestPath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	options := DefaultOptions(root)
	options.FrontendCLIPath = frontendPath
	ambiguous := RunRemoveDependency(RemoveDependencyRequest{Project: options, PackageSpec: "lodash", DryRun: true})
	if !addHasDiagnosticCode(ambiguous.Diagnostics, "TSPACK_REMOVE_PACKAGE_TARGET_AMBIGUOUS") {
		t.Fatalf("workspace ambiguity = %#v", ambiguous.Diagnostics)
	}
	targeted := RunRemoveDependency(RemoveDependencyRequest{Project: options, PackageSpec: "lodash", TargetPackage: "two", DryRun: true})
	if hasErrors(targeted.Diagnostics) || targeted.TargetPackage != "two" || !targeted.ManifestChanged {
		t.Fatalf("targeted remove = %#v", targeted)
	}
}

func TestRunRemoveDependencyDoesNotRemoveDerivedDeclaration(t *testing.T) {
	frontendPath := addFrontendPath(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.tsx")
	source := `import { Package, Workspace, define, dep, npm } from "tspack/manifest";
export default define(<Workspace name="demo"><Package name="app" version="1.0.0" kind="app" dependencyDeclaration={{ origin: { kind: "concept", name: "ReactLibrary" }, layer: "concept", authority: "generated", editability: "concept-owned" }} dependencies={{ values: [dep(npm("typescript", "^5.9"))] }} /></Workspace>);
`
	if err := os.WriteFile(manifestPath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	options := DefaultOptions(root)
	options.FrontendCLIPath = frontendPath
	result := RunRemoveDependency(RemoveDependencyRequest{Project: options, PackageSpec: "typescript"})
	if hasErrors(result.Diagnostics) || !result.NoEditableDeclaration || !result.StillRequired || result.ManifestChanged {
		t.Fatalf("derived-only remove = %#v", result)
	}
	after, _ := os.ReadFile(manifestPath)
	if !bytes.Equal(after, []byte(source)) {
		t.Fatal("derived-only remove changed source")
	}
}

func TestRunRemoveDependencyRollsBackManifestWhenResolutionFails(t *testing.T) {
	frontendPath := addFrontendPath(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.tsx")
	original := `import { Package, Workspace, define, dep, npm } from "tspack/manifest";
export default define(<Workspace name="demo"><Package name="app" version="1.0.0" kind="app" dependencies={{ values: [dep(npm("remove-me", "1.0.0")), dep(npm("broken", "1.0.0"))] }} /></Workspace>);
`
	if err := os.WriteFile(manifestPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	client := removeRegistryClient("broken", "1.0.0")
	client.tar = map[string][]byte{}
	options := DefaultOptions(root)
	options.FrontendCLIPath = frontendPath
	options.ResolverClient = client
	result := RunRemoveDependency(RemoveDependencyRequest{Project: options, PackageSpec: "remove-me"})
	if !hasErrors(result.Diagnostics) || result.ManifestChanged {
		t.Fatalf("failed remove = %#v", result)
	}
	after, _ := os.ReadFile(manifestPath)
	if !bytes.Equal(after, []byte(original)) {
		t.Fatalf("failed update did not roll back manifest:\n%s", after)
	}
}

func TestRunRemoveDependencyDistinguishesTransitiveLockPersistence(t *testing.T) {
	frontendPath := addFrontendPath(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.tsx")
	source := `import { Package, Workspace, define, dep, npm } from "tspack/manifest";
export default define(
  <Workspace name="demo">
    <Package name="app" version="1.0.0" kind="app" dependencies={{ values: [dep(npm("dep-a", "1.0.0")), dep(npm("dep-b", "1.0.0"))] }} />
  </Workspace>,
);
`
	if err := os.WriteFile(manifestPath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	options := DefaultOptions(root)
	options.FrontendCLIPath = frontendPath
	options.ResolverClient = removeTransitiveRegistry()
	updated := RunUpdate(UpdateRequest{Project: options})
	if hasErrors(updated.Diagnostics) {
		t.Fatalf("initial update failed: %#v", updated.Diagnostics)
	}

	result := RunRemoveDependency(RemoveDependencyRequest{Project: options, PackageSpec: "dep-a"})
	if hasErrors(result.Diagnostics) {
		t.Fatalf("remove failed: %#v", result.Diagnostics)
	}
	if result.StillRequired || !result.ResolvedStatusKnown || !result.StillResolved || result.LockPackageRemoved {
		t.Fatalf("transitive persistence classification = %#v", result)
	}
	if !result.LockChanged {
		t.Fatalf("direct-edge removal should change the lock: %#v", result.LockDiff)
	}
	if result.Performance.RegistryMetadataRequests > 2 {
		t.Fatalf("preflight and commit duplicated registry metadata requests: %#v", result.Performance)
	}
}

func removeTestDeclaration(id string, optional bool) authoring.DependencyDeclaration {
	return authoring.DependencyDeclaration{
		ID:          id,
		Identity:    authoring.PackageIdentity{Source: "npm", Name: "lodash"},
		Source:      authoring.PackageSource{Kind: "npm", Package: "lodash", Range: "^4"},
		Constraint:  "^4",
		Kind:        authoring.DependencyRuntime,
		Optional:    optional,
		Authority:   authoring.AuthorityOwned,
		Editability: authoring.EditabilityEditable,
	}
}

func removeRegistryClient(name string, versions ...string) *fakeClient {
	client := &fakeClient{
		meta: map[string]*resolver.PackageMetadata{
			name: {Name: name, Versions: map[string]resolver.PackageVersion{}},
		},
		tar: map[string][]byte{},
	}
	for _, version := range versions {
		body := tarball(name, version, nil)
		url := "https://example.invalid/" + name + "-" + version + ".tgz"
		integrity := "sha512-" + base64.StdEncoding.EncodeToString(sha512sum(body))
		client.meta[name].Versions[version] = resolver.PackageVersion{
			Name:    name,
			Version: version,
			Dist:    resolver.PackageDist{Tarball: url, Integrity: integrity},
		}
		client.tar[url] = body
	}
	return client
}

func removeTransitiveRegistry() *fakeClient {
	client := &fakeClient{
		meta: map[string]*resolver.PackageMetadata{},
		tar:  map[string][]byte{},
	}
	addVersion := func(name string, dependencies map[string]string) {
		version := "1.0.0"
		body := tarball(name, version, dependencies)
		url := "https://example.invalid/" + name + "-" + version + ".tgz"
		integrity := "sha512-" + base64.StdEncoding.EncodeToString(sha512sum(body))
		client.meta[name] = &resolver.PackageMetadata{
			Name: name,
			Versions: map[string]resolver.PackageVersion{
				version: {
					Name:         name,
					Version:      version,
					Dependencies: dependencies,
					Dist:         resolver.PackageDist{Tarball: url, Integrity: integrity},
				},
			},
		}
		client.tar[url] = body
	}
	addVersion("dep-a", nil)
	addVersion("dep-b", map[string]string{"dep-a": "1.0.0"})
	return client
}
