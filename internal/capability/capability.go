package capability

import (
	"sort"

	"github.com/yuechen-li-dev/tspack/internal/lockfile"
)

const LifecycleScriptKind = "lifecycleScript"

const (
	LifecycleCategoryConsumerInstall   = "consumer-install"
	LifecycleCategoryMaintainerPublish = "maintainer-publish"
	LifecycleCategoryOther             = "other"
)

var lifecycleScriptNames = []string{
	"preinstall",
	"install",
	"postinstall",
	"prepack",
	"prepare",
	"postpack",
	"prepublish",
	"prepublishOnly",
	"publish",
	"postpublish",
}

var consumerInstallLifecycleScripts = map[string]bool{
	"preinstall":  true,
	"install":     true,
	"postinstall": true,
}

var maintainerPublishLifecycleScripts = map[string]bool{
	"prepublishOnly": true,
	"prepublish":     true,
	"prepare":        true,
	"prepack":        true,
	"postpack":       true,
	"publish":        true,
	"postpublish":    true,
}

type LifecycleClassification struct {
	ScriptName          string
	LifecycleCategory   string
	ConsumerInstallTime bool
}

func ClassifyLifecycleScript(scriptName string) LifecycleClassification {
	classification := LifecycleClassification{
		ScriptName:          scriptName,
		LifecycleCategory:   LifecycleCategoryOther,
		ConsumerInstallTime: false,
	}
	if consumerInstallLifecycleScripts[scriptName] {
		classification.LifecycleCategory = LifecycleCategoryConsumerInstall
		classification.ConsumerInstallTime = true
		return classification
	}
	if maintainerPublishLifecycleScripts[scriptName] {
		classification.LifecycleCategory = LifecycleCategoryMaintainerPublish
		return classification
	}
	return classification
}

func IsSupportedLifecycleScript(scriptName string) bool {
	for _, supported := range lifecycleScriptNames {
		if scriptName == supported {
			return true
		}
	}
	return false
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
