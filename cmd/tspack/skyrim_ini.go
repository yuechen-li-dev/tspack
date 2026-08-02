package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

const skyrimAlwaysActiveOverridePath = "General.bAlwaysActive"

func buildSkyrimINIPlan(root string, target skyrimTargetRef, profile skyrimHostProfile, stage bool) (skyrimINIPlan, error) {
	plan := skyrimINIPlan{}
	if len(profile.INIOverrides) == 0 {
		return plan, nil
	}
	if err := validateSkyrimINIOverrides(target.Target.INIOverrideFields, profile.INIOverrides); err != nil {
		return plan, err
	}

	iniPath, pathSource, err := resolveSkyrimINIPath(profile)
	if err != nil {
		return plan, err
	}
	sourceBytes, err := os.ReadFile(iniPath)
	if err != nil {
		return plan, fmt.Errorf("read active Skyrim INI: %w", err)
	}

	enabled := profile.INIOverrides[skyrimAlwaysActiveOverridePath].(bool)
	effectiveBytes, sourceValue, err := applySkyrimAlwaysActiveOverride(sourceBytes, enabled)
	if err != nil {
		return plan, err
	}

	plan = skyrimINIPlan{
		Enabled:         true,
		ResolvedPath:    "<redacted>",
		PathSource:      pathSource,
		SourceSHA256:    hashBytes(sourceBytes),
		EffectiveSHA256: hashBytes(effectiveBytes),
		AppliedOverrides: []skyrimINIOverrideValue{
			{
				Section:        "General",
				Key:            "bAlwaysActive",
				SourceValue:    sourceValue,
				EffectiveValue: enabled,
			},
		},
		sourcePath: iniPath,
	}
	plan.RestorationPlanned = plan.EffectiveSHA256 != plan.SourceSHA256
	if stage {
		stageDirectory := filepath.Join(root, "build", "skyrim", "ini")
		if err := os.MkdirAll(stageDirectory, 0o755); err != nil {
			return skyrimINIPlan{}, err
		}
		plan.StagedPath = filepath.Join(stageDirectory, "Skyrim-"+strings.ToLower(plan.EffectiveSHA256[:16])+".ini")
		if err := os.WriteFile(plan.StagedPath, effectiveBytes, 0o644); err != nil {
			return skyrimINIPlan{}, err
		}
	}
	return plan, nil
}

func validateSkyrimINIOverrides(fields []manifest.SkyrimINIOverride, values map[string]any) error {
	if len(fields) != 1 || fields[0].Section != "General" || fields[0].Key != "bAlwaysActive" || fields[0].Type != "boolean" {
		return errors.New("Skyrim target must explicitly allow only General.bAlwaysActive as a boolean INI override")
	}
	if len(values) != 1 {
		return errors.New("selected Skyrim host may supply only one declared INI override")
	}
	value, found := values[skyrimAlwaysActiveOverridePath]
	if !found {
		return errors.New("unknown Skyrim INI override")
	}
	if _, ok := value.(bool); !ok {
		return errors.New("Skyrim INI override General.bAlwaysActive must be boolean")
	}
	return nil
}

func resolveSkyrimINIPath(profile skyrimHostProfile) (string, string, error) {
	if profile.INIPath != "" {
		if !filepath.IsAbs(profile.INIPath) {
			return "", "", errors.New("host iniPath must be an absolute machine-local path")
		}
		info, err := os.Stat(profile.INIPath)
		if err != nil || info.IsDir() {
			return "", "", errors.New("configured Skyrim iniPath is unavailable")
		}
		return filepath.Clean(profile.INIPath), "profile", nil
	}
	if profile.SaveDirectory == "" || !filepath.IsAbs(profile.SaveDirectory) || !strings.EqualFold(filepath.Base(profile.SaveDirectory), "Saves") {
		return "", "", errors.New("Skyrim INI path is not declared and cannot be derived from the selected saveDirectory")
	}
	iniPath := filepath.Join(filepath.Dir(profile.SaveDirectory), "Skyrim.ini")
	info, err := os.Stat(iniPath)
	if err != nil || info.IsDir() {
		return "", "", errors.New("derived Skyrim.ini beside selected saveDirectory is unavailable")
	}
	return filepath.Clean(iniPath), "save_directory_parent", nil
}

func applySkyrimAlwaysActiveOverride(source []byte, enabled bool) ([]byte, any, error) {
	lineEnding := "\n"
	if bytes.Contains(source, []byte("\r\n")) {
		lineEnding = "\r\n"
	}
	text := strings.ReplaceAll(string(source), "\r\n", "\n")
	hasFinalNewline := strings.HasSuffix(text, "\n")
	if hasFinalNewline {
		text = strings.TrimSuffix(text, "\n")
	}
	lines := strings.Split(text, "\n")

	generalStart := -1
	generalEnd := len(lines)
	activeSection := ""
	keyIndex := -1
	var sourceValue any
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			if !strings.HasSuffix(trimmed, "]") || len(trimmed) <= 2 {
				return nil, nil, fmt.Errorf("malformed Skyrim INI section at line %d", index+1)
			}
			if strings.EqualFold(activeSection, "General") && generalEnd == len(lines) {
				generalEnd = index
			}
			activeSection = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			if strings.EqualFold(activeSection, "General") {
				if generalStart != -1 {
					return nil, nil, errors.New("duplicate [General] section in Skyrim INI")
				}
				generalStart = index
			}
			continue
		}
		equals := strings.IndexByte(trimmed, '=')
		if equals <= 0 {
			return nil, nil, fmt.Errorf("malformed Skyrim INI assignment at line %d", index+1)
		}
		if !strings.EqualFold(activeSection, "General") {
			continue
		}
		key := strings.TrimSpace(trimmed[:equals])
		if !strings.EqualFold(key, "bAlwaysActive") {
			continue
		}
		if keyIndex != -1 {
			return nil, nil, errors.New("duplicate General.bAlwaysActive key in Skyrim INI")
		}
		value, err := parseSkyrimINIBoolean(strings.TrimSpace(trimmed[equals+1:]))
		if err != nil {
			return nil, nil, err
		}
		keyIndex = index
		sourceValue = value
	}
	if generalStart == -1 {
		return nil, nil, errors.New("Skyrim INI does not contain a [General] section")
	}

	newValue := "0"
	if enabled {
		newValue = "1"
	}
	if keyIndex == -1 {
		lines = append(lines[:generalEnd], append([]string{"bAlwaysActive=" + newValue}, lines[generalEnd:]...)...)
	} else {
		indentation := lines[keyIndex][:len(lines[keyIndex])-len(strings.TrimLeft(lines[keyIndex], " \t"))]
		lines[keyIndex] = indentation + "bAlwaysActive=" + newValue
	}
	effective := strings.Join(lines, lineEnding)
	if hasFinalNewline {
		effective += lineEnding
	}
	return []byte(effective), sourceValue, nil
}

func parseSkyrimINIBoolean(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "1", "true":
		return true, nil
	case "0", "false":
		return false, nil
	default:
		return false, fmt.Errorf("General.bAlwaysActive must be boolean, got %q", value)
	}
}
