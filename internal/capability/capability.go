package capability

import (
	"sort"

	"github.com/tspack/tspack/internal/lockfile"
)

const LifecycleScriptKind = "lifecycleScript"

var lifecycleScriptNames = []string{
	"preinstall",
	"install",
	"postinstall",
	"prepack",
	"prepare",
	"postpack",
	"prepublish",
	"prepublishOnly",
	"postpublish",
}

func FromPackageJSONScripts(scripts map[string]string) []lockfile.Capability {
	if len(scripts) == 0 {
		return nil
	}

	capabilities := make([]lockfile.Capability, 0)
	for _, scriptName := range lifecycleScriptNames {
		command, ok := scripts[scriptName]
		if !ok {
			continue
		}
		capabilities = append(capabilities, lockfile.Capability{
			Kind:    LifecycleScriptKind,
			Script:  scriptName,
			Command: command,
		})
	}

	if len(capabilities) == 0 {
		return nil
	}

	sort.SliceStable(capabilities, func(i, j int) bool {
		if capabilities[i].Kind != capabilities[j].Kind {
			return capabilities[i].Kind < capabilities[j].Kind
		}
		if capabilities[i].Script != capabilities[j].Script {
			return capabilities[i].Script < capabilities[j].Script
		}
		return capabilities[i].Command < capabilities[j].Command
	})

	return capabilities
}
