package materialize

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/store"
)

func TestSharedProvidersUseExplicitRuntimeReferencesOnly(t *testing.T) {
	providers := sharedRuntimeProviders([]lockfile.RootModuleInstance{
		{Kind: "runtime", Reference: "alias", InstanceID: "provider"},
		{Kind: "runtime", Reference: "alias", InstanceID: "provider"},
		{Kind: "tool", Reference: "tool", InstanceID: "tool"},
		{Kind: "runtime", Reference: "conflict", InstanceID: "first"},
		{Kind: "runtime", Reference: "conflict", InstanceID: "second"},
	})
	if len(providers) != 1 || providers["alias"] != "provider" {
		t.Fatalf("unexpected shared runtime providers: %#v", providers)
	}
}

func TestRuntimeProviderProjectionPreservesConsumerVersions(t *testing.T) {
	for _, test := range []struct {
		name        string
		conflicting bool
	}{
		{name: "shared"},
		{name: "conflicting", conflicting: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			contentStore, err := store.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			provider := lockfile.Package{ID: "npm:provider@1.0.0", Name: "provider", Version: "1.0.0", Source: "npm"}
			provider.Hash = putPkgWithPackageJSON(t, contentStore, provider.ID, provider.Name, `{"name":"provider","version":"1.0.0"}`, nil)
			library := lockfile.Package{ID: "npm:library@1.0.0", Name: "library", Version: "1.0.0", Source: "npm"}
			library.Hash = putPkg(t, contentStore, library.ID, library.Name)
			private := lockfile.Package{ID: "npm:private@1.0.0", Name: "private", Version: "1.0.0", Source: "npm"}
			private.Hash = putPkg(t, contentStore, private.ID, private.Name)
			locked := &lockfile.Lockfile{
				Packages: []lockfile.Package{provider, library, private},
				Edges: []lockfile.Edge{
					{From: "app:target:client", To: provider.ID, Kind: "runtime", Reference: "provider"},
					{From: "app:target:client", To: library.ID, Kind: "runtime", Reference: "library"},
					{From: library.ID, To: private.ID, Kind: "runtime", Reference: "private"},
				},
			}
			workspaceGraph := &graph.WorkspaceGraph{PackagesByName: map[string]*graph.PackageNode{
				"app":   {Name: "app", Root: "packages/app"},
				"other": {Name: "other", Root: "packages/other"},
			}}
			if test.conflicting {
				other := provider
				other.ID = "npm:provider@2.0.0"
				other.Version = "2.0.0"
				other.Hash = putPkgWithPackageJSON(t, contentStore, other.ID, other.Name, `{"name":"provider","version":"2.0.0"}`, nil)
				locked.Packages = append(locked.Packages, other)
				locked.Edges = append(locked.Edges, lockfile.Edge{From: "other:target:client", To: other.ID, Kind: "runtime", Reference: "provider"})
			}
			lockfile.RebuildModuleInstances(locked)
			result := NodeModulesMaterializer{}.Materialize(context.Background(), Request{
				WorkspaceRoot: workspace, Graph: workspaceGraph, Lock: locked, Store: contentStore,
				Options: Options{LinkMode: LinkModeCopy},
			})
			if len(result.Diagnostics) != 0 {
				t.Fatalf("materialization failed: %#v", result.Diagnostics)
			}
			assertFileContent(t, filepath.Join(workspace, "packages", "app", "node_modules", "provider", "package.json"), `{"name":"provider","version":"1.0.0"}`)
			_, err = os.Stat(filepath.Join(workspace, "node_modules", "provider", "package.json"))
			if test.conflicting {
				assertFileContent(t, filepath.Join(workspace, "packages", "other", "node_modules", "provider", "package.json"), `{"name":"provider","version":"2.0.0"}`)
				if !os.IsNotExist(err) {
					t.Fatalf("ambiguous provider acquired a shared winner: %v", err)
				}
			} else if err != nil {
				t.Fatalf("explicit runtime provider missing from shared ancestor: %v", err)
			}
			if _, err := os.Stat(filepath.Join(workspace, "node_modules", "private")); !os.IsNotExist(err) {
				t.Fatalf("transitive-only runtime package promoted: %v", err)
			}
		})
	}
}
