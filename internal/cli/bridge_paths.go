package cli

import (
	"fmt"
	"os"

	"github.com/yuechen-li-dev/tspack/internal/bridge"
)

type manifestFrontendBridgeResolution = bridge.Resolution

func findManifestFrontendBridge(bridgeName string) manifestFrontendBridgeResolution {
	return bridge.Resolve(bridgeName)
}

func manifestFrontendCLIPath() string {
	resolution := findManifestFrontendBridge("cli.js")
	if resolution.Path != "" {
		return resolution.Path
	}
	return resolution.SearchedPaths[0]
}

func requireManifestFrontendBridge(bridgeName string, code string, label string) string {
	resolution := findManifestFrontendBridge(bridgeName)
	if resolution.Path != "" {
		return resolution.Path
	}
	fmt.Fprint(os.Stderr, bridge.MissingMessage(code, label, resolution))
	exit(1)
	return ""
}
