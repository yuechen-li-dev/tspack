package materialize

import (
	"path/filepath"

	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
)

// Shared instances need Node's ordinary ancestor lookup for application-supplied
// runtime providers. Only explicit roots with one module identity are eligible;
// transitive packages and ambiguous consumer versions are never promoted.
func sharedRuntimeProviders(roots []lockfile.RootModuleInstance) map[string]string {
	providers := map[string]string{}
	ambiguous := map[string]bool{}
	for _, root := range roots {
		if root.Kind != "runtime" {
			continue
		}
		if previous, exists := providers[root.Reference]; exists && previous != root.InstanceID {
			ambiguous[root.Reference] = true
		}
		providers[root.Reference] = root.InstanceID
	}
	for reference := range ambiguous {
		delete(providers, reference)
	}
	return providers
}

func moduleRootLinkDirectories(workspaceRoot, nodeModules string, root lockfile.RootModuleInstance, packages map[string]lockfile.Package, workspaceGraph *graph.WorkspaceGraph, providers map[string]string) []string {
	primary := moduleRootLinkNodeModules(workspaceRoot, nodeModules, root.From, root.Reference, packages, workspaceGraph)
	directories := []string{primary}
	if filepath.Clean(primary) != filepath.Clean(nodeModules) && providers[root.Reference] == root.InstanceID {
		directories = append(directories, nodeModules)
	}
	return directories
}
