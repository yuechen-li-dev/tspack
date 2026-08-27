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

func TestParsePackageSpec(t *testing.T) {
	tests := []struct {
		input      string
		name       string
		constraint string
	}{
		{input: "foo", name: "foo"},
		{input: "foo@^4", name: "foo", constraint: "^4"},
		{input: "foo@4.17.21", name: "foo", constraint: "4.17.21"},
		{input: "@scope/foo", name: "@scope/foo"},
		{input: "@scope/foo@^3", name: "@scope/foo", constraint: "^3"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			parsed, err := ParsePackageSpec(test.input)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Identity.Name != test.name || parsed.ExplicitConstraint != test.constraint {
				t.Fatalf("parsed = %#v", parsed)
			}
		})
	}
	for _, invalid := range []string{"", "foo@", "@scope", "github:user/repo", "file:../foo", "foo/bar"} {
		if _, err := ParsePackageSpec(invalid); err == nil {
			t.Fatalf("ParsePackageSpec(%q) unexpectedly succeeded", invalid)
		}
	}
}

func TestSelectAddConstraintPrefersStableAndPreservesExplicitIntent(t *testing.T) {
	metadata := &resolver.PackageMetadata{Versions: map[string]resolver.PackageVersion{
		"4.17.21":      {Version: "4.17.21"},
		"5.0.0-beta.1": {Version: "5.0.0-beta.1"},
	}}
	written, selected, err := selectAddConstraint(metadata, "")
	if err != nil {
		t.Fatal(err)
	}
	if written != "^4.17.21" || selected != "4.17.21" {
		t.Fatalf("constraint selection = %q, %q", written, selected)
	}
	written, selected, err = selectAddConstraint(metadata, "^4")
	if err != nil || written != "^4" || selected != "4.17.21" {
		t.Fatalf("explicit range selection = %q, %q, %v", written, selected, err)
	}
	written, selected, err = selectAddConstraint(metadata, "4.17.21")
	if err != nil || written != "4.17.21" || selected != "4.17.21" {
		t.Fatalf("explicit exact selection = %q, %q, %v", written, selected, err)
	}
	prereleaseOnly := &resolver.PackageMetadata{Versions: map[string]resolver.PackageVersion{
		"1.0.0-beta.1": {Version: "1.0.0-beta.1"},
	}}
	if _, _, err := selectAddConstraint(prereleaseOnly, ""); err == nil {
		t.Fatal("unqualified selection accepted a prerelease-only package")
	}
}

func TestRunAddDependencyWritesManifestAndLockThenBecomesNoop(t *testing.T) {
	frontendPath := addFrontendPath(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.tsx")
	original := `import { Package, Workspace, define } from "tspack/manifest";

const preserved = { comment: "source outside the dependency island stays byte-identical" };

export default define(
  <Workspace name="demo">
    <Package name="app" version="1.0.0" kind="app" />
  </Workspace>,
);
`
	if err := os.WriteFile(manifestPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	client := addRegistryClient("lodash", "4.17.21")
	options := DefaultOptions(root)
	options.FrontendCLIPath = frontendPath
	options.ResolverClient = client

	result := RunAddDependency(AddDependencyRequest{Project: options, PackageSpec: "lodash"})
	if hasErrors(result.Diagnostics) {
		t.Fatalf("add failed: %#v", result.Diagnostics)
	}
	if !result.ManifestChanged || !result.LockChanged || result.WrittenConstraint != "^4.17.21" || result.SelectedVersion != "4.17.21" {
		t.Fatalf("unexpected add result: %#v", result)
	}
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(manifestBytes, []byte(`dep(npm("lodash", "^4.17.21")`)) || !bytes.Contains(manifestBytes, []byte("const preserved")) {
		t.Fatalf("unexpected projected manifest:\n%s", manifestBytes)
	}
	locked, diagnostics, err := lockfile.LoadFile(options.LockfilePath)
	if err != nil || hasErrors(diagnostics) {
		t.Fatalf("load lockfile: %v %#v", err, diagnostics)
	}
	if len(locked.Packages) != 1 || locked.Packages[0].ID != "npm:lodash@4.17.21" {
		t.Fatalf("lock packages = %#v", locked.Packages)
	}
	beforeNoop := append([]byte(nil), manifestBytes...)
	lockBeforeNoop, _ := os.ReadFile(options.LockfilePath)
	second := RunAddDependency(AddDependencyRequest{Project: options, PackageSpec: "lodash"})
	if hasErrors(second.Diagnostics) || !second.AlreadyPresent || second.ManifestChanged || second.LockChanged {
		t.Fatalf("second add = %#v", second)
	}
	afterNoop, _ := os.ReadFile(manifestPath)
	lockAfterNoop, _ := os.ReadFile(options.LockfilePath)
	if !bytes.Equal(beforeNoop, afterNoop) || !bytes.Equal(lockBeforeNoop, lockAfterNoop) {
		t.Fatal("no-op add rewrote project files")
	}
	if client.packageMetaCalls != 1 || len(client.tarCalls) != 1 {
		t.Fatalf("registry calls: metadata=%d tarballs=%d", client.packageMetaCalls, len(client.tarCalls))
	}
}

func TestRunAddDependencyDryRunWritesNothingAndWorkspaceAmbiguityFails(t *testing.T) {
	frontendPath := addFrontendPath(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.tsx")
	original := `import { Package, Workspace, define } from "tspack/manifest";
export default define(<Workspace name="demo"><Package name="app" version="1.0.0" kind="app" /></Workspace>);
`
	if err := os.WriteFile(manifestPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	options := DefaultOptions(root)
	options.FrontendCLIPath = frontendPath
	options.ResolverClient = addRegistryClient("zod", "4.1.0")
	result := RunAddDependency(AddDependencyRequest{Project: options, PackageSpec: "zod@^4", DryRun: true})
	if hasErrors(result.Diagnostics) || !result.ManifestChanged || result.LockChanged {
		t.Fatalf("dry run = %#v", result)
	}
	after, _ := os.ReadFile(manifestPath)
	if !bytes.Equal([]byte(original), after) {
		t.Fatal("dry run wrote manifest")
	}
	if _, err := os.Stat(options.LockfilePath); !os.IsNotExist(err) {
		t.Fatal("dry run wrote lockfile")
	}

	multiRoot := t.TempDir()
	multiManifest := filepath.Join(multiRoot, "manifest.tsx")
	multi := `import { Package, Workspace, define } from "tspack/manifest";
export default define(<Workspace name="demo"><Package name="one" version="1.0.0" kind="app" /><Package name="two" version="1.0.0" kind="app" /></Workspace>);
`
	if err := os.WriteFile(multiManifest, []byte(multi), 0o644); err != nil {
		t.Fatal(err)
	}
	multiOptions := DefaultOptions(multiRoot)
	multiOptions.FrontendCLIPath = frontendPath
	ambiguous := RunAddDependency(AddDependencyRequest{Project: multiOptions, PackageSpec: "zod", DryRun: true})
	if !addHasDiagnosticCode(ambiguous.Diagnostics, "TSPACK_ADD_PACKAGE_TARGET_AMBIGUOUS") {
		t.Fatalf("ambiguity diagnostics = %#v", ambiguous.Diagnostics)
	}
}

func TestRunAddDependencyEquivalentExplicitConstraintIsZeroRegistryNoop(t *testing.T) {
	frontendPath := addFrontendPath(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.tsx")
	source := `import { Package, Workspace, define, dep, npm } from "tspack/manifest";
export default define(<Workspace name="demo"><Package name="app" version="1.0.0" kind="app" dependencies={{ values: [dep(npm("lodash", "^4"))] }} /></Workspace>);
`
	if err := os.WriteFile(manifestPath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	client := addRegistryClient("lodash", "4.17.21")
	options := DefaultOptions(root)
	options.FrontendCLIPath = frontendPath
	options.ResolverClient = client
	result := RunAddDependency(AddDependencyRequest{Project: options, PackageSpec: "lodash@^4"})
	if hasErrors(result.Diagnostics) || !result.AlreadyPresent || result.ManifestChanged || result.LockChanged {
		t.Fatalf("equivalent add = %#v", result)
	}
	if client.packageMetaCalls != 0 || len(client.tarCalls) != 0 {
		t.Fatalf("equivalent add made registry requests: metadata=%d tarballs=%d", client.packageMetaCalls, len(client.tarCalls))
	}
}

func TestRunAddDependencyBoundsDevToolAndSourceSemantics(t *testing.T) {
	for _, test := range []struct {
		kind authoring.DependencyKind
		code string
	}{
		{kind: authoring.DependencyTest, code: "TSPACK_ADD_DEV_UNSUPPORTED"},
		{kind: authoring.DependencyTool, code: "TSPACK_ADD_TOOL_UNSUPPORTED"},
	} {
		result := RunAddDependency(AddDependencyRequest{PackageSpec: "vitest", Kind: test.kind})
		if !addHasDiagnosticCode(result.Diagnostics, test.code) {
			t.Fatalf("kind %s diagnostics = %#v", test.kind, result.Diagnostics)
		}
	}
	unsupportedSource := RunAddDependency(AddDependencyRequest{
		PackageSpec: "lodash",
		Source:      &authoring.PackageSource{Kind: "jsr"},
	})
	if !addHasDiagnosticCode(unsupportedSource.Diagnostics, "TSPACK_ADD_SOURCE_UNSUPPORTED") {
		t.Fatalf("source diagnostics = %#v", unsupportedSource.Diagnostics)
	}
}

func TestRunAddDependencyRollsBackManifestWhenResolutionFails(t *testing.T) {
	frontendPath := addFrontendPath(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.tsx")
	original := `import { Package, Workspace, define } from "tspack/manifest";
export default define(<Workspace name="demo"><Package name="app" version="1.0.0" kind="app" /></Workspace>);
`
	if err := os.WriteFile(manifestPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	client := addRegistryClient("broken", "1.0.0")
	client.tar = map[string][]byte{}
	options := DefaultOptions(root)
	options.FrontendCLIPath = frontendPath
	options.ResolverClient = client
	result := RunAddDependency(AddDependencyRequest{Project: options, PackageSpec: "broken"})
	if !hasErrors(result.Diagnostics) || result.ManifestChanged {
		t.Fatalf("failed add = %#v", result)
	}
	after, _ := os.ReadFile(manifestPath)
	if !bytes.Equal([]byte(original), after) {
		t.Fatalf("manifest was not rolled back:\n%s", after)
	}
}

func TestRunAddDependencyCreatesExplicitShadowOfConceptDeclaration(t *testing.T) {
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
      dependencies={{ values: [dep(npm("typescript", "^5.9"))] }}
    />
  </Workspace>,
);
`
	if err := os.WriteFile(manifestPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	options := DefaultOptions(root)
	options.FrontendCLIPath = frontendPath
	options.ResolverClient = addRegistryClient("typescript", "6.0.0")
	result := RunAddDependency(AddDependencyRequest{Project: options, PackageSpec: "typescript@^6", DryRun: true})
	if hasErrors(result.Diagnostics) {
		t.Fatalf("shadow add failed: %#v", result.Diagnostics)
	}
	if len(result.ShadowedDeclarations) != 1 || result.ShadowedDeclarations[0].Origin.Name != "ReactLibrary" {
		t.Fatalf("shadow explanation = %#v", result.ShadowedDeclarations)
	}
	after, _ := os.ReadFile(manifestPath)
	if !bytes.Equal([]byte(original), after) {
		t.Fatal("shadow dry run wrote manifest")
	}
}

func addFrontendPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "manifest-frontend", "dist", "cli.js"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Skip("manifest frontend build is unavailable")
	}
	return path
}

func addRegistryClient(name string, version string) *fakeClient {
	body := tarball(name, version, nil)
	url := "https://example.invalid/" + name + "-" + version + ".tgz"
	integrity := "sha512-" + base64.StdEncoding.EncodeToString(sha512sum(body))
	return &fakeClient{
		meta: map[string]*resolver.PackageMetadata{
			name: {
				Name: name,
				Versions: map[string]resolver.PackageVersion{
					version: {Name: name, Version: version, Dist: resolver.PackageDist{Tarball: url, Integrity: integrity}},
				},
			},
		},
		tar: map[string][]byte{url: body},
	}
}
