// Package skyrim owns the Skyrim-specific runtime, host-profile, INI, process,
// and save-fixture integration. Generic CLI dispatch reaches it through a thin
// adapter and supplies manifest loading explicitly.
package skyrim

import "github.com/yuechen-li-dev/tspack/internal/manifest"

type ManifestLoader func(root string, manifestPath string) *manifest.ManifestIR

type TargetRef = skyrimTargetRef
type HostProfile = skyrimHostProfile

const ProfileRelativePath = skyrimProfileRelativePath

func IsRunInvocation(args []string) bool {
	return isSkyrimRunInvocation(args)
}

func Run(args []string, loadManifest ManifestLoader) {
	runSkyrimCommand(args, loadManifest)
}

func RunFixture(args []string, loadManifest ManifestLoader) {
	runSkyrimFixtureCommand(args, loadManifest)
}

func SelectTarget(ir *manifest.ManifestIR, root string, name string) TargetRef {
	return selectSkyrimTarget(ir, root, name)
}

func LoadProfile(root string, name string) (HostProfile, error) {
	return loadSkyrimProfile(root, name)
}
