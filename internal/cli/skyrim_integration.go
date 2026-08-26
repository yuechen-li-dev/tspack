package cli

import (
	skyrimintegration "github.com/yuechen-li-dev/tspack/internal/integrations/skyrim"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

type skyrimTargetRef = skyrimintegration.TargetRef
type skyrimHostProfile = skyrimintegration.HostProfile

const skyrimProfileRelativePath = skyrimintegration.ProfileRelativePath

func isSkyrimRunInvocation(args []string) bool {
	return skyrimintegration.IsRunInvocation(args)
}

func runSkyrimCommand(args []string) {
	skyrimintegration.Run(args, loadWorkspaceManifest)
}

func runSkyrimFixtureCommand(args []string) {
	skyrimintegration.RunFixture(args, loadWorkspaceManifest)
}

func loadWorkspaceManifest(root string, manifestPath string) *manifest.ManifestIR {
	return openWorkspace(root).LoadManifest(manifestPath)
}

func selectSkyrimTarget(ir *manifest.ManifestIR, root string, name string) skyrimTargetRef {
	return skyrimintegration.SelectTarget(ir, root, name)
}

func loadSkyrimProfile(root string, name string) (skyrimHostProfile, error) {
	return skyrimintegration.LoadProfile(root, name)
}
