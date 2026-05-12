package capability

import (
	"sort"
	"strings"

	"github.com/tspack/tspack/internal/lockfile"
)

var lifecycleScriptNames = []string{
	"preinstall",
	"install",
	"postinstall",
	"prepublish",
	"prepare",
	"prepack",
	"postpack",
}

func FromPackageJSONScripts(scripts map[string]string) []lockfile.Capability {
	if len(scripts) == 0 {
		return nil
	}

	capabilitySet := make(map[string]struct{}, len(lifecycleScriptNames))
	for _, scriptName := range lifecycleScriptNames {
		if _, ok := scripts[scriptName]; ok {
			capabilitySet["lifecycle-script|"+scriptName] = struct{}{}
		}
	}

	if len(capabilitySet) == 0 {
		return nil
	}

	capabilities := make([]lockfile.Capability, 0, len(capabilitySet))
	for key := range capabilitySet {
		kind, detail, ok := strings.Cut(key, "|")
		if !ok || kind == "" || detail == "" {
			continue
		}
		capabilities = append(capabilities, lockfile.Capability{Kind: kind, Detail: detail})
	}

	sort.SliceStable(capabilities, func(i, j int) bool {
		if capabilities[i].Kind != capabilities[j].Kind {
			return capabilities[i].Kind < capabilities[j].Kind
		}
		return capabilities[i].Detail < capabilities[j].Detail
	})

	return capabilities
}
