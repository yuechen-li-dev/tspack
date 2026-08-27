package project

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	jsr, err := ParsePackageSpec("@std/path@^1", authoring.SourceJSR)
	if err != nil || jsr.Identity.Source != "jsr" || jsr.Identity.Name != "@std/path" || jsr.ExplicitConstraint != "^1" {
		t.Fatalf("JSR scoped package parse = %#v, %v", jsr, err)
	}
}

func TestSelectAddConstraintPrefersStableAndPreservesExplicitIntent(t *testing.T) {
	metadata := &resolver.RegistryPackageMetadata{Versions: map[string]resolver.RegistryPackageVersion{
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
	prereleaseOnly := &resolver.RegistryPackageMetadata{Versions: map[string]resolver.RegistryPackageVersion{
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

func TestRunAddDependencyJSRUsesBackendProjectionAndSharedRequestMemo(t *testing.T) {
	frontendPath := addFrontendPath(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.tsx")
	original := `import { Package, Workspace, define } from "tspack/manifest";
export default define(<Workspace name="demo"><Package name="app" version="1.0.0" kind="app" /></Workspace>);
`
	if err := os.WriteFile(manifestPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	jsrClient := addJSRRegistryClient("@std/path", "1.1.6", nil)
	options := DefaultOptions(root)
	options.FrontendCLIPath = frontendPath
	options.ResolverBackends = resolver.BackendRegistry{
		resolver.SourceJSR: resolver.NewJSRBackend(jsrClient),
	}

	result := RunAddDependency(AddDependencyRequest{
		Project:     options,
		PackageSpec: "@std/path",
		Source:      authoring.SourceJSR,
	})
	if hasErrors(result.Diagnostics) {
		t.Fatalf("JSR add failed: %#v", result.Diagnostics)
	}
	if result.Source != "jsr" || result.Package != "@std/path" || result.WrittenConstraint != "^1.1.6" || result.SelectedVersion != "1.1.6" {
		t.Fatalf("unexpected JSR add result: %#v", result)
	}
	if result.Usage == nil || result.Usage.Semantic.Key() != "jsr:@std/path" || result.Usage.Import.Specifier != "@jsr/std__path" {
		t.Fatalf("unexpected JSR usage guidance: %#v", result.Usage)
	}
	if result.Performance.RegistryMetadataRequests != 1 || result.Performance.RegistryTarballRequests != 1 {
		t.Fatalf("request counts = %#v", result.Performance)
	}
	if result.Performance.RegistryRequests["jsr.metadata"] != 1 || result.Performance.RegistryRequests["jsr.tarball"] != 1 {
		t.Fatalf("source request attribution = %#v", result.Performance.RegistryRequests)
	}
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(manifestBytes, []byte(`dep(jsr("@std/path", "^1.1.6")`)) {
		t.Fatalf("unexpected projected manifest:\n%s", manifestBytes)
	}
	locked, diagnostics, err := lockfile.LoadFile(options.LockfilePath)
	if err != nil || hasErrors(diagnostics) {
		t.Fatalf("load lockfile: %v %#v", err, diagnostics)
	}
	if len(locked.Packages) != 1 || locked.Packages[0].ID != "jsr:@std/path@1.1.6" {
		t.Fatalf("lock packages = %#v", locked.Packages)
	}
	if jsrClient.packageMetaCalls != 1 || len(jsrClient.tarCalls) != 1 {
		t.Fatalf("JSR client calls: metadata=%d tarballs=%d", jsrClient.packageMetaCalls, len(jsrClient.tarCalls))
	}

	manifestBeforeNoop := append([]byte(nil), manifestBytes...)
	lockBeforeNoop, _ := os.ReadFile(options.LockfilePath)
	second := RunAddDependency(AddDependencyRequest{
		Project:     options,
		PackageSpec: "@std/path",
		Source:      authoring.SourceJSR,
	})
	if hasErrors(second.Diagnostics) || !second.AlreadyPresent || second.ManifestChanged || second.LockChanged {
		t.Fatalf("second JSR add = %#v", second)
	}
	manifestAfterNoop, _ := os.ReadFile(manifestPath)
	lockAfterNoop, _ := os.ReadFile(options.LockfilePath)
	if !bytes.Equal(manifestBeforeNoop, manifestAfterNoop) || !bytes.Equal(lockBeforeNoop, lockAfterNoop) {
		t.Fatal("no-op JSR add rewrote project files")
	}
	if jsrClient.packageMetaCalls != 1 || len(jsrClient.tarCalls) != 1 {
		t.Fatalf("no-op JSR add made registry requests: metadata=%d tarballs=%d", jsrClient.packageMetaCalls, len(jsrClient.tarCalls))
	}
}

func TestRunAddDependencyKeepsSameNameRegistrySourcesDistinct(t *testing.T) {
	frontendPath := addFrontendPath(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.tsx")
	original := `import { Package, Workspace, define } from "tspack/manifest";
export default define(<Workspace name="demo"><Package name="app" version="1.0.0" kind="app" /></Workspace>);
`
	if err := os.WriteFile(manifestPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	npmClient := addRegistryClient("@scope/foo", "1.0.0")
	jsrClient := addJSRRegistryClient("@scope/foo", "2.0.0", nil)
	options := DefaultOptions(root)
	options.FrontendCLIPath = frontendPath
	options.ResolverClient = npmClient
	options.ResolverBackends = resolver.BackendRegistry{
		resolver.SourceNPM: resolver.NewNPMBackend(npmClient),
		resolver.SourceJSR: resolver.NewJSRBackend(jsrClient),
	}

	npmResult := RunAddDependency(AddDependencyRequest{Project: options, PackageSpec: "@scope/foo"})
	if hasErrors(npmResult.Diagnostics) {
		t.Fatalf("npm add failed: %#v", npmResult.Diagnostics)
	}
	jsrResult := RunAddDependency(AddDependencyRequest{Project: options, PackageSpec: "@scope/foo", Source: authoring.SourceJSR})
	if hasErrors(jsrResult.Diagnostics) {
		t.Fatalf("JSR collision add failed: %#v", jsrResult.Diagnostics)
	}
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(manifestBytes, []byte(`npm("@scope/foo", "^1.0.0")`)) ||
		!bytes.Contains(manifestBytes, []byte(`jsr("@scope/foo", "^2.0.0")`)) ||
		!bytes.Contains(manifestBytes, []byte(`key: "jsr:@scope/foo"`)) {
		t.Fatalf("source collision was not projected distinctly:\n%s", manifestBytes)
	}
	locked, diagnostics, err := lockfile.LoadFile(options.LockfilePath)
	if err != nil || hasErrors(diagnostics) {
		t.Fatalf("load lockfile: %v %#v", err, diagnostics)
	}
	packageIDs := map[string]bool{}
	for _, pkg := range locked.Packages {
		packageIDs[pkg.ID] = true
	}
	if !packageIDs["npm:@scope/foo@1.0.0"] || !packageIDs["jsr:@scope/foo@2.0.0"] {
		t.Fatalf("source-qualified lock packages = %#v", locked.Packages)
	}

	replaced := RunAddDependency(AddDependencyRequest{
		Project:     options,
		PackageSpec: "@scope/foo@^2",
		Source:      authoring.SourceJSR,
	})
	if hasErrors(replaced.Diagnostics) || !replaced.DeclarationChanged || replaced.PreviousConstraint != "^2.0.0" || replaced.WrittenConstraint != "^2" {
		t.Fatalf("source-qualified replacement = %#v", replaced)
	}
	manifestBytes, err = os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(manifestBytes, []byte(`npm("@scope/foo", "^1.0.0")`)) ||
		!bytes.Contains(manifestBytes, []byte(`jsr("@scope/foo", "^2")`)) {
		t.Fatalf("JSR replacement touched the wrong source:\n%s", manifestBytes)
	}
}

func TestRunAddDependencyJSRUsesNormalMixedTransitiveUpdate(t *testing.T) {
	frontendPath := addFrontendPath(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.tsx")
	original := `import { Package, Workspace, define } from "tspack/manifest";
export default define(<Workspace name="demo"><Package name="app" version="1.0.0" kind="app" /></Workspace>);
`
	if err := os.WriteFile(manifestPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	jsrClient := addJSRRegistryClient("@demo/root", "1.2.0", map[string]string{
		"@jsr/demo__child": "^2",
		"left-pad":         "^1",
	})
	mergeFakeRegistry(jsrClient, addJSRRegistryClient("@demo/child", "2.1.0", nil))
	npmClient := addRegistryClient("left-pad", "1.3.0")
	options := DefaultOptions(root)
	options.FrontendCLIPath = frontendPath
	options.ResolverClient = npmClient
	options.ResolverBackends = resolver.BackendRegistry{
		resolver.SourceNPM: resolver.NewNPMBackend(npmClient),
		resolver.SourceJSR: resolver.NewJSRBackend(jsrClient),
	}

	result := RunAddDependency(AddDependencyRequest{
		Project:     options,
		PackageSpec: "@demo/root",
		Source:      authoring.SourceJSR,
	})
	if hasErrors(result.Diagnostics) {
		t.Fatalf("mixed-transitive JSR add failed: %#v", result.Diagnostics)
	}
	locked, diagnostics, err := lockfile.LoadFile(options.LockfilePath)
	if err != nil || hasErrors(diagnostics) {
		t.Fatalf("load lockfile: %v %#v", err, diagnostics)
	}
	wanted := map[string]bool{
		"jsr:@demo/root@1.2.0":  false,
		"jsr:@demo/child@2.1.0": false,
		"npm:left-pad@1.3.0":    false,
	}
	for _, pkg := range locked.Packages {
		if _, ok := wanted[pkg.ID]; ok {
			wanted[pkg.ID] = true
		}
	}
	for packageID, found := range wanted {
		if !found {
			t.Fatalf("mixed source lock is missing %s: %#v", packageID, locked.Packages)
		}
	}
	if result.Performance.RegistryMetadataRequests != 3 || result.Performance.RegistryTarballRequests != 3 {
		t.Fatalf("mixed request counts = %#v", result.Performance)
	}
}

func TestRunAddDependencyDoesNotSearchOtherRegistries(t *testing.T) {
	frontendPath := addFrontendPath(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.tsx")
	original := `import { Package, Workspace, define } from "tspack/manifest";
export default define(<Workspace name="demo"><Package name="app" version="1.0.0" kind="app" /></Workspace>);
`
	if err := os.WriteFile(manifestPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	npmClient := &fakeClient{
		meta:    map[string]*resolver.PackageMetadata{},
		metaErr: map[string]error{"@only/jsr": errors.New("npm package not found")},
		tar:     map[string][]byte{},
	}
	jsrClient := addJSRRegistryClient("@only/jsr", "1.0.0", nil)
	options := DefaultOptions(root)
	options.FrontendCLIPath = frontendPath
	options.ResolverClient = npmClient
	options.ResolverBackends = resolver.BackendRegistry{
		resolver.SourceNPM: resolver.NewNPMBackend(npmClient),
		resolver.SourceJSR: resolver.NewJSRBackend(jsrClient),
	}

	result := RunAddDependency(AddDependencyRequest{Project: options, PackageSpec: "@only/jsr"})
	if !addHasDiagnosticCode(result.Diagnostics, "TSPACK_ADD_METADATA_FETCH_FAILED") {
		t.Fatalf("default npm failure diagnostics = %#v", result.Diagnostics)
	}
	if npmClient.packageMetaCalls != 1 || jsrClient.packageMetaCalls != 0 {
		t.Fatalf("registry auto-search occurred: npm=%d jsr=%d", npmClient.packageMetaCalls, jsrClient.packageMetaCalls)
	}
}

func TestDependencyEditMemoBackendsKeepSourceQualifiedMetadata(t *testing.T) {
	npmClient := addRegistryClient("@scope/foo", "1.0.0")
	jsrClient := addJSRRegistryClient("@scope/foo", "2.0.0", nil)
	memo := newDependencyEditMemoBackends(resolver.BackendRegistry{
		resolver.SourceNPM: resolver.NewNPMBackend(npmClient),
		resolver.SourceJSR: resolver.NewJSRBackend(jsrClient),
	})
	npmBackend, _ := memo.Registry().Backend(resolver.SourceNPM)
	jsrBackend, _ := memo.Registry().Backend(resolver.SourceJSR)
	for iteration := 0; iteration < 2; iteration++ {
		if _, err := npmBackend.Metadata(context.Background(), "@scope/foo"); err != nil {
			t.Fatal(err)
		}
		if _, err := jsrBackend.Metadata(context.Background(), "@scope/foo"); err != nil {
			t.Fatal(err)
		}
	}
	metadataRequests, artifactRequests := memo.RequestCounts()
	if metadataRequests != 2 || artifactRequests != 0 {
		t.Fatalf("memo request counts = %d, %d", metadataRequests, artifactRequests)
	}
	requestsByKind := memo.RequestsByKind()
	if requestsByKind["npm.metadata"] != 1 || requestsByKind["jsr.metadata"] != 1 {
		t.Fatalf("source-qualified memo attribution = %#v", requestsByKind)
	}
	if npmClient.packageMetaCalls != 1 || jsrClient.packageMetaCalls != 1 {
		t.Fatalf("source caches crossed: npm=%d jsr=%d", npmClient.packageMetaCalls, jsrClient.packageMetaCalls)
	}
}

func TestRunAddDependencyJSRDryRunSelectsWithoutWritingArtifacts(t *testing.T) {
	frontendPath := addFrontendPath(t)
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.tsx")
	original := `import { Package, Workspace, define } from "tspack/manifest";
export default define(<Workspace name="demo"><Package name="app" version="1.0.0" kind="app" /></Workspace>);
`
	if err := os.WriteFile(manifestPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	jsrClient := addJSRRegistryClient("@std/path", "1.1.6", nil)
	options := DefaultOptions(root)
	options.FrontendCLIPath = frontendPath
	options.ResolverBackends = resolver.BackendRegistry{
		resolver.SourceJSR: resolver.NewJSRBackend(jsrClient),
	}

	result := RunAddDependency(AddDependencyRequest{
		Project:     options,
		PackageSpec: "@std/path",
		Source:      authoring.SourceJSR,
		DryRun:      true,
	})
	if hasErrors(result.Diagnostics) || !result.ManifestChanged || result.LockChanged || result.SelectedVersion != "1.1.6" {
		t.Fatalf("JSR dry run = %#v", result)
	}
	after, _ := os.ReadFile(manifestPath)
	if !bytes.Equal([]byte(original), after) {
		t.Fatal("JSR dry run wrote manifest")
	}
	if _, err := os.Stat(options.LockfilePath); !os.IsNotExist(err) {
		t.Fatal("JSR dry run wrote lockfile")
	}
	if jsrClient.packageMetaCalls != 1 || len(jsrClient.tarCalls) != 0 {
		t.Fatalf("JSR dry-run requests: metadata=%d tarballs=%d", jsrClient.packageMetaCalls, len(jsrClient.tarCalls))
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
	jsrClient := addJSRRegistryClient("@std/path", "1.1.6", nil)
	multiOptions.ResolverBackends = resolver.BackendRegistry{
		resolver.SourceJSR: resolver.NewJSRBackend(jsrClient),
	}
	targeted := RunAddDependency(AddDependencyRequest{
		Project:       multiOptions,
		PackageSpec:   "@std/path",
		Source:        authoring.SourceJSR,
		TargetPackage: "two",
		Optional:      true,
		DryRun:        true,
	})
	if hasErrors(targeted.Diagnostics) || targeted.TargetPackage != "two" || !targeted.Optional || !targeted.ManifestChanged {
		t.Fatalf("targeted optional JSR dry run = %#v", targeted)
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
		Source:      authoring.SourceKind("banana"),
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

func addJSRRegistryClient(name string, version string, dependencies map[string]string) *fakeClient {
	parts := strings.Split(strings.TrimPrefix(name, "@"), "/")
	compatibilityName := "@jsr/" + parts[0] + "__" + parts[1]
	body := tarball(compatibilityName, version, dependencies)
	url := "https://example.invalid/" + strings.ReplaceAll(compatibilityName, "/", "-") + "-" + version + ".tgz"
	integrity := "sha512-" + base64.StdEncoding.EncodeToString(sha512sum(body))
	return &fakeClient{
		meta: map[string]*resolver.PackageMetadata{
			compatibilityName: {
				Name: compatibilityName,
				Versions: map[string]resolver.PackageVersion{
					version: {
						Name:         compatibilityName,
						Version:      version,
						Dependencies: dependencies,
						Dist:         resolver.PackageDist{Tarball: url, Integrity: integrity},
					},
				},
			},
		},
		tar: map[string][]byte{url: body},
	}
}

func mergeFakeRegistry(destination *fakeClient, source *fakeClient) {
	for name, metadata := range source.meta {
		destination.meta[name] = metadata
	}
	for url, body := range source.tar {
		destination.tar[url] = body
	}
}
