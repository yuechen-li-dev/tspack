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

func TestDeclaredTypesAreVisibleFromSharedInstancesWithoutPromotingTransitives(t *testing.T) {
	workspace := t.TempDir()
	contentStore, _ := store.Open(t.TempDir())
	typesHash := putPkg(t, contentStore, "npm:@types/react@19.0.0", "@types/react")
	privateHash := putPkg(t, contentStore, "npm:@types/private@1.0.0", "@types/private")
	libraryHash := putPkg(t, contentStore, "npm:library@1.0.0", "library")
	locked := &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:@types/react@19.0.0", Name: "@types/react", Version: "19.0.0", Source: "npm", Hash: typesHash},
			{ID: "npm:@types/private@1.0.0", Name: "@types/private", Version: "1.0.0", Source: "npm", Hash: privateHash},
			{ID: "npm:library@1.0.0", Name: "library", Version: "1.0.0", Source: "npm", Hash: libraryHash},
		},
		Edges: []lockfile.Edge{
			{From: "app:tool", To: "npm:@types/react@19.0.0", Kind: "tool", Reference: "@types/react"},
			{From: "app:target:client", To: "npm:library@1.0.0", Kind: "runtime", Reference: "library"},
			{From: "npm:library@1.0.0", To: "npm:@types/private@1.0.0", Kind: "runtime", Reference: "@types/private"},
		},
	}
	lockfile.RebuildModuleInstances(locked)
	workspaceGraph := &graph.WorkspaceGraph{PackagesByName: map[string]*graph.PackageNode{
		"app": {Name: "app", Root: "packages/app"},
	}}
	result := NodeModulesMaterializer{}.Materialize(context.Background(), Request{
		WorkspaceRoot: workspace, Graph: workspaceGraph, Lock: locked, Store: contentStore,
		Options: Options{LinkMode: LinkModeCopy},
	})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("materialization failed: %#v", result.Diagnostics)
	}
	mustExist(t, filepath.Join(workspace, "node_modules", "@types", "react", "package.json"))
	mustExist(t, filepath.Join(workspace, "packages", "app", "node_modules", "library", "package.json"))
	if _, err := os.Stat(filepath.Join(workspace, "node_modules", "@types", "private")); !os.IsNotExist(err) {
		t.Fatalf("transitive-only type provider was exposed at workspace root: %v", err)
	}
}
