package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/version"
)

const skyrimProfileRelativePath = ".tspack/skyrim-hosts.toml"

type skyrimRunOptions struct {
	root             string
	host             string
	dryRun           bool
	json             bool
	noLaunch         bool
	sessionBootstrap bool
	dominatusSkyrim  bool
}

type skyrimProfilesFile struct {
	Hosts map[string]skyrimHostProfile `toml:"hosts"`
}

type skyrimHostProfile struct {
	GameRoot            string                    `toml:"gameRoot"`
	DataDirectory       string                    `toml:"dataDirectory"`
	SKSELauncher        string                    `toml:"skseLauncher"`
	PluginState         string                    `toml:"pluginState"`
	RuntimeLogDirectory string                    `toml:"runtimeLogDirectory"`
	RuntimeVersion      string                    `toml:"runtimeVersion"`
	SaveDirectory       string                    `toml:"saveDirectory"`
	INIPath             string                    `toml:"iniPath"`
	INIOverrides        map[string]any            `toml:"iniOverrides"`
	TestSaves           map[string]skyrimTestSave `toml:"testSaves"`
	Tools               map[string]string         `toml:"tools"`
	RuntimeOverrides    map[string]map[string]any `toml:"runtimeOverrides"`
}

// skyrimTestSave is machine-local fixture provenance. It contains no source
// path: the save directory is owned by the selected host profile.
type skyrimTestSave struct {
	Filename          string `toml:"filename"`
	Disposable        bool   `toml:"disposable"`
	ReadOnly          bool   `toml:"readOnly"`
	SidecarPresent    bool   `toml:"sidecarPresent"`
	EssSHA256         string `toml:"essSha256"`
	SkseSHA256        string `toml:"skseSha256"`
	SourceCandidateID string `toml:"sourceCandidateId"`
	SourceEssSHA256   string `toml:"sourceEssSha256"`
	SourceSkseSHA256  string `toml:"sourceSkseSha256"`
}

type skyrimTargetRef struct {
	PackageName string
	PackageRoot string
	Target      manifest.SkyrimTarget
}

type skyrimFilePlan struct {
	Kind               string `json:"kind"`
	Source             string `json:"source"`
	Destination        string `json:"destination"`
	SourceSHA256       string `json:"sourceSha256,omitempty"`
	MaterializedSource string `json:"materializedSource,omitempty"`
	CurrentSHA256      string `json:"currentSha256,omitempty"`
	Action             string `json:"action"`
}

type skyrimRuntimeOverrideValue struct {
	Path           string `json:"path"`
	Type           string `json:"type"`
	SourceValue    any    `json:"sourceValue"`
	EffectiveValue any    `json:"effectiveValue"`
	Secret         bool   `json:"secret,omitempty"`
}

type skyrimRuntimeConfigPlan struct {
	SourcePath         string                           `json:"sourcePath"`
	SourceSHA256       string                           `json:"sourceSha256"`
	EffectiveSHA256    string                           `json:"effectiveSha256"`
	StagedPath         string                           `json:"stagedPath,omitempty"`
	OverrideSource     string                           `json:"overrideSource,omitempty"`
	OverrideTarget     string                           `json:"overrideTarget,omitempty"`
	AllowedOverrides   []manifest.SkyrimRuntimeOverride `json:"allowedOverrides,omitempty"`
	AppliedOverrides   []skyrimRuntimeOverrideValue     `json:"appliedOverrides,omitempty"`
	RestorationPlanned bool                             `json:"restorationPlanned"`
}

type skyrimINIPlan struct {
	Enabled            bool                     `json:"enabled"`
	ResolvedPath       string                   `json:"resolvedIniPath,omitempty"`
	PathSource         string                   `json:"pathSource,omitempty"`
	SourceSHA256       string                   `json:"sourceSha256,omitempty"`
	EffectiveSHA256    string                   `json:"effectiveSha256,omitempty"`
	StagedPath         string                   `json:"stagedPath,omitempty"`
	AppliedOverrides   []skyrimINIOverrideValue `json:"appliedOverrides,omitempty"`
	RestorationPlanned bool                     `json:"restorationPlanned"`

	sourcePath string
}

type skyrimINIOverrideValue struct {
	Section        string `json:"section"`
	Key            string `json:"key"`
	SourceValue    any    `json:"sourceValue"`
	EffectiveValue bool   `json:"effectiveValue"`
}

type skyrimPluginStatePlan struct {
	Path           string   `json:"path"`
	CurrentEntries []string `json:"currentEntries"`
	DesiredEntries []string `json:"desiredEntries"`
	RemovedStale   []string `json:"removedStale,omitempty"`
	Changed        bool     `json:"changed"`
}

type skyrimPlanReport struct {
	Command                  string                     `json:"command"`
	TSPackVersion            string                     `json:"tspackVersion"`
	TSPackCommit             string                     `json:"tspackCommit"`
	ManifestPath             string                     `json:"manifestPath"`
	ManifestSHA256           string                     `json:"manifestSha256"`
	Package                  string                     `json:"package"`
	Target                   string                     `json:"target"`
	Host                     string                     `json:"host"`
	RuntimeVersion           string                     `json:"runtimeVersion"`
	GameRoot                 string                     `json:"gameRoot"`
	DataDirectory            string                     `json:"dataDirectory"`
	SKSELauncher             string                     `json:"skseLauncher"`
	AssetPacks               []manifest.SkyrimAssetPack `json:"assetPacks"`
	NativeConfigure          []string                   `json:"nativeConfigure"`
	NativeBuild              []string                   `json:"nativeBuild"`
	AssetCompilerCommand     []string                   `json:"assetCompilerCommand"`
	Files                    []skyrimFilePlan           `json:"files"`
	PluginState              skyrimPluginStatePlan      `json:"pluginState"`
	LaunchCommand            []string                   `json:"launchCommand"`
	ManagedControllerCommand []string                   `json:"managedControllerCommand,omitempty"`
	RuntimeEvidence          string                     `json:"runtimeEvidence"`
	ReadyMarker              string                     `json:"readyMarker"`
	RuntimeConfig            skyrimRuntimeConfigPlan    `json:"runtimeConfig"`
	INI                      skyrimINIPlan              `json:"ini"`
	Blockers                 []string                   `json:"blockers"`
}

type skyrimMaterializationReport struct {
	Plan                    skyrimPlanReport `json:"plan"`
	ChangedFiles            []string         `json:"changedFiles"`
	UnchangedFiles          []string         `json:"unchangedFiles"`
	RollbackPath            string           `json:"rollbackPath"`
	PluginStateChanged      bool             `json:"pluginStateChanged"`
	LaunchedExecutable      string           `json:"launchedExecutable,omitempty"`
	LauncherPID             int              `json:"launcherPid,omitempty"`
	RuntimePID              int              `json:"runtimePid,omitempty"`
	RuntimeLog              string           `json:"runtimeLog,omitempty"`
	RuntimeVerified         bool             `json:"runtimeVerified"`
	RestorationAttempted    bool             `json:"restorationAttempted"`
	RestorationVerified     bool             `json:"restorationVerified"`
	RestoredConfigSHA256    string           `json:"restoredConfigSha256,omitempty"`
	INIRestorationAttempted bool             `json:"iniRestorationAttempted"`
	INIRestorationVerified  bool             `json:"iniRestorationVerified"`
	RestoredINISHA256       string           `json:"restoredIniSha256,omitempty"`
	RestorationError        string           `json:"restorationError,omitempty"`
	SessionBootstrapConfig  string           `json:"sessionBootstrapConfig,omitempty"`
	ManagedControllerPID    int              `json:"managedControllerPid,omitempty"`
	ManagedControllerLog    string           `json:"managedControllerLog,omitempty"`
	ManagedControllerExited bool             `json:"managedControllerExited"`
}

type skyrimBridgeInspection struct {
	InputSHA256 string                         `json:"InputSha256"`
	Plugin      string                         `json:"Plugin"`
	Records     []skyrimBridgeInspectionRecord `json:"Records"`
}

type skyrimBridgeInspectionRecord struct {
	EditorID             string `json:"EditorId"`
	LocalFormID          string `json:"LocalFormId"`
	Owner                string `json:"Owner"`
	SuspiciousWrongOwner bool   `json:"SuspiciousWrongOwner"`
}

func isSkyrimRunInvocation(args []string) bool {
	return len(args) > 1 && args[0] == "run" && args[1] == "skyrim"
}

func runSkyrimCommand(args []string) {
	opts := parseSkyrimRunOptions(args)
	root, err := filepath.Abs(opts.root)
	if err != nil {
		failSkyrim("TSPACK_SKYRIM_ROOT_INVALID", err)
	}
	manifestPath := filepath.Join(root, "manifest.tsx")
	ir := loadManifestPathForRun(root, manifestPath)
	target := selectSkyrimTarget(ir, root, "skyrim")
	if opts.host != "" && opts.host != target.Target.Host {
		failSkyrim("TSPACK_SKYRIM_HOST_MISMATCH", fmt.Errorf("--host %q does not match manifest host %q", opts.host, target.Target.Host))
	}
	profile, err := loadSkyrimProfile(root, target.Target.Host)
	if err != nil {
		failSkyrim("TSPACK_SKYRIM_PROFILE_INVALID", err)
	}
	if opts.sessionBootstrap {
		profile, err = withSkyrimSessionBootstrap(profile, target.Target.Host)
		if err != nil {
			failSkyrim("TSPACK_SKYRIM_SESSION_BOOTSTRAP_INVALID", err)
		}
	}
	if opts.dominatusSkyrim {
		profile, err = withDominatusSkyrimExperiment(profile, target.Target.Host)
		if err != nil {
			failSkyrim("TSPACK_SKYRIM_DOMINATUS_EXPERIMENT_INVALID", err)
		}
	}
	plan := buildSkyrimPlan(root, manifestPath, target, profile, false)
	if opts.dominatusSkyrim {
		plan.ManagedControllerCommand = managedControllerCommand(root, "<generated-transport-config>")
	}
	if opts.dryRun {
		renderSkyrimPlan(plan, opts.json)
		if len(plan.Blockers) > 0 {
			os.Exit(1)
		}
		return
	}
	if len(plan.Blockers) > 0 {
		failSkyrim("TSPACK_SKYRIM_PREFLIGHT_FAILED", errors.New(strings.Join(plan.Blockers, "; ")))
	}
	executeSkyrimBuild(root, target, profile)
	plan = buildSkyrimPlan(root, manifestPath, target, profile, true)
	if opts.dominatusSkyrim {
		plan.ManagedControllerCommand = managedControllerCommand(root, filepath.Join(root, "build", "msse-presenter-m1", "aurelian-transport.json"))
	}
	if len(plan.Blockers) > 0 {
		failSkyrim("TSPACK_SKYRIM_STAGING_FAILED", errors.New(strings.Join(plan.Blockers, "; ")))
	}
	report, err := materializeSkyrimPlan(root, plan)
	if err != nil {
		failSkyrim("TSPACK_SKYRIM_MATERIALIZATION_FAILED", err)
	}
	if opts.sessionBootstrap || opts.dominatusSkyrim {
		configPath, configErr := writeSkyrimSessionBootstrapTransportConfig(root, profile)
		if configErr != nil {
			failSkyrim("TSPACK_SKYRIM_SESSION_BOOTSTRAP_CONFIG_FAILED", configErr)
		}
		report.SessionBootstrapConfig = configPath
	}
	var lifecycleError error
	if !opts.noLaunch {
		lifecycleError = launchSkyrim(&report, target, profile, opts.dominatusSkyrim)
	}
	if plan.RuntimeConfig.RestorationPlanned {
		report.RestorationAttempted = true
		if err := restoreSkyrimRuntimeConfig(root, plan); err != nil {
			report.RestorationError = err.Error()
		} else {
			report.RestorationVerified = true
			report.RestoredConfigSHA256, _ = hashFile(plan.RuntimeConfig.SourcePath)
		}
	}
	if plan.INI.RestorationPlanned {
		report.INIRestorationAttempted = true
		if err := restoreSkyrimINI(plan, report); err != nil {
			if report.RestorationError == "" {
				report.RestorationError = err.Error()
			} else {
				report.RestorationError += "; " + err.Error()
			}
		} else {
			report.INIRestorationVerified = true
			report.RestoredINISHA256, _ = hashFile(plan.INI.sourcePath)
		}
	}
	if err := writeSkyrimReports(root, plan, report); err != nil {
		failSkyrim("TSPACK_SKYRIM_REPORT_FAILED", err)
	}
	renderSkyrimMaterialization(report, opts.json)
	if report.RestorationError != "" {
		failSkyrim("TSPACK_SKYRIM_RESTORATION_FAILED", errors.New(report.RestorationError))
	}
	if lifecycleError != nil {
		failSkyrim("TSPACK_SKYRIM_LAUNCH_FAILED", lifecycleError)
	}
}

func parseSkyrimRunOptions(args []string) skyrimRunOptions {
	opts := skyrimRunOptions{root: "."}
	for index := 2; index < len(args); index++ {
		switch args[index] {
		case "--dry-run", "--plan-only":
			opts.dryRun = true
		case "--json":
			opts.json = true
		case "--no-launch":
			opts.noLaunch = true
		case "--session-bootstrap":
			opts.sessionBootstrap = true
		case "--dominatus-skyrim":
			opts.dominatusSkyrim = true
		case "--root":
			if index+1 >= len(args) {
				failSkyrim("TSPACK_SKYRIM_ARGUMENT_INVALID", errors.New("--root requires a value"))
			}
			index++
			opts.root = args[index]
		case "--host":
			if index+1 >= len(args) {
				failSkyrim("TSPACK_SKYRIM_ARGUMENT_INVALID", errors.New("--host requires a value"))
			}
			index++
			opts.host = args[index]
		default:
			failSkyrim("TSPACK_SKYRIM_ARGUMENT_INVALID", fmt.Errorf("unknown argument %s", args[index]))
		}
	}
	return opts
}

func withSkyrimSessionBootstrap(profile skyrimHostProfile, host string) (skyrimHostProfile, error) {
	fixture, found := profile.TestSaves["ed-m2b2d"]
	if !found || !fixture.Disposable || !fixture.ReadOnly || !isSafeSkyrimFixtureFilename(fixture.Filename) {
		return skyrimHostProfile{}, errors.New("--session-bootstrap requires a provisioned disposable, read-only ed-m2b2d fixture")
	}
	copy := profile
	copy.RuntimeOverrides = map[string]map[string]any{}
	for target, values := range profile.RuntimeOverrides {
		copy.RuntimeOverrides[target] = copySkyrimRuntimeOverrideValues(values)
	}
	values := copy.RuntimeOverrides["MarionetteSSE"]
	if values == nil {
		values = map[string]any{}
		copy.RuntimeOverrides["MarionetteSSE"] = values
	}
	values["host"] = host
	values["eternal_dragonborn.development.presenter.enabled"] = true
	values["eternal_dragonborn.development.presenter.allow_semantic_actuation"] = false
	values["eternal_dragonborn.development.presenter.allow_host_request_evaluation"] = false
	values["eternal_dragonborn.development.presenter.allow_session_bootstrap"] = true
	return copy, nil
}

func withDominatusSkyrimExperiment(profile skyrimHostProfile, host string) (skyrimHostProfile, error) {
	copy, err := withSkyrimSessionBootstrap(profile, host)
	if err != nil {
		return skyrimHostProfile{}, err
	}
	values := copy.RuntimeOverrides["MarionetteSSE"]
	values["eternal_dragonborn.development.presenter.allow_semantic_actuation"] = true
	values["eternal_dragonborn.development.presenter.allow_host_request_evaluation"] = true
	values["eternal_dragonborn.development.presenter.allow_host_fixture_query"] = true
	copy.INIOverrides = map[string]any{skyrimAlwaysActiveOverridePath: true}
	return copy, nil
}

func writeSkyrimSessionBootstrapTransportConfig(root string, profile skyrimHostProfile) (string, error) {
	values := profile.RuntimeOverrides["MarionetteSSE"]
	profileName, profileOK := values["eternal_dragonborn.development.presenter.profile"].(string)
	token, tokenOK := values["eternal_dragonborn.development.presenter.token"].(string)
	if !profileOK || profileName == "" || !tokenOK || token == "" {
		return "", errors.New("session bootstrap requires a local presenter profile and token")
	}
	directory := filepath.Join(root, "build", "msse-presenter-m1")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(directory, "aurelian-transport.json")
	data, err := json.MarshalIndent(map[string]any{
		"profile":             profileName,
		"token":               token,
		"clientName":          "tspack-session-bootstrap",
		"checkpointDirectory": filepath.Join(root, "build", "skyrim", "checkpoints"),
	}, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := copyBytesAndReplaceFile(data, path); err != nil {
		return "", err
	}
	return path, nil
}

func selectSkyrimTarget(ir *manifest.ManifestIR, root string, name string) skyrimTargetRef {
	var matches []skyrimTargetRef
	for _, pkg := range ir.Packages {
		if pkg.Skyrim == nil || pkg.Skyrim.Name != name {
			continue
		}
		packageRoot := root
		if pkg.Root != "" && pkg.Root != "." {
			packageRoot = filepath.Join(root, filepath.FromSlash(pkg.Root))
		}
		matches = append(matches, skyrimTargetRef{PackageName: pkg.Name, PackageRoot: packageRoot, Target: *pkg.Skyrim})
	}
	if len(matches) != 1 {
		failSkyrim("TSPACK_SKYRIM_TARGET_NOT_FOUND", fmt.Errorf("expected exactly one Skyrim target named %q; found %d", name, len(matches)))
	}
	return matches[0]
}

func loadSkyrimProfile(root string, name string) (skyrimHostProfile, error) {
	profilePath := filepath.Join(root, filepath.FromSlash(skyrimProfileRelativePath))
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return skyrimHostProfile{}, fmt.Errorf("read %s: %w", profilePath, err)
	}
	var profiles skyrimProfilesFile
	if err := toml.Unmarshal(data, &profiles); err != nil {
		return skyrimHostProfile{}, fmt.Errorf("parse %s: %w", profilePath, err)
	}
	profile, ok := profiles.Hosts[name]
	if !ok {
		return skyrimHostProfile{}, fmt.Errorf("host %q is not declared in %s", name, profilePath)
	}
	for field, value := range map[string]string{
		"gameRoot":            profile.GameRoot,
		"dataDirectory":       profile.DataDirectory,
		"skseLauncher":        profile.SKSELauncher,
		"pluginState":         profile.PluginState,
		"runtimeLogDirectory": profile.RuntimeLogDirectory,
	} {
		if !filepath.IsAbs(value) {
			return skyrimHostProfile{}, fmt.Errorf("host %q %s must be an absolute machine-local path", name, field)
		}
	}
	return profile, nil
}

func buildSkyrimPlan(root string, manifestPath string, target skyrimTargetRef, profile skyrimHostProfile, stageRuntimeConfig bool) skyrimPlanReport {
	manifestHash, _ := hashFile(manifestPath)
	assetCommand := []string{"dotnet", "run", "--project", filepath.Join(target.PackageRoot, filepath.FromSlash(target.Target.AssetCompilerProject)), "--", "compile"}
	for _, pack := range target.Target.AssetPacks {
		assetCommand = append(assetCommand, "--source", filepath.Join(target.PackageRoot, filepath.FromSlash(pack.Source)))
	}
	assetCommand = append(assetCommand, "--output", filepath.Join(target.PackageRoot, filepath.FromSlash(target.Target.AssetOutput)), "--game-data", profile.DataDirectory)
	plan := skyrimPlanReport{
		Command:              "tspack run skyrim",
		TSPackVersion:        version.Version,
		TSPackCommit:         version.Commit,
		ManifestPath:         manifestPath,
		ManifestSHA256:       manifestHash,
		Package:              target.PackageName,
		Target:               target.Target.Name,
		Host:                 target.Target.Host,
		RuntimeVersion:       profile.RuntimeVersion,
		GameRoot:             profile.GameRoot,
		DataDirectory:        profile.DataDirectory,
		SKSELauncher:         profile.SKSELauncher,
		AssetPacks:           append([]manifest.SkyrimAssetPack(nil), target.Target.AssetPacks...),
		NativeConfigure:      resolveSkyrimCommand(target.Target.NativeConfigure.Command, profile.Tools),
		NativeBuild:          resolveSkyrimCommand(target.Target.NativeBuild.Command, profile.Tools),
		AssetCompilerCommand: resolveSkyrimCommand(assetCommand, profile.Tools),
		LaunchCommand:        []string{profile.SKSELauncher},
		RuntimeEvidence:      filepath.Join(profile.RuntimeLogDirectory, filepath.FromSlash(target.Target.RuntimeEvidencePattern)),
		ReadyMarker:          target.Target.ReadyMarker,
	}
	runtimeConfig, runtimeConfigErr := buildSkyrimRuntimeConfig(root, target, profile, stageRuntimeConfig)
	if runtimeConfigErr != nil {
		plan.Blockers = append(plan.Blockers, runtimeConfigErr.Error())
	} else {
		plan.RuntimeConfig = runtimeConfig
	}
	iniPlan, iniPlanErr := buildSkyrimINIPlan(root, target, profile, stageRuntimeConfig)
	if iniPlanErr != nil {
		plan.Blockers = append(plan.Blockers, iniPlanErr.Error())
	} else {
		plan.INI = iniPlan
	}
	if profile.RuntimeVersion != target.Target.RuntimeVersion {
		plan.Blockers = append(plan.Blockers, fmt.Sprintf("runtime version mismatch: manifest=%s host=%s", target.Target.RuntimeVersion, profile.RuntimeVersion))
	}
	for label, path := range map[string]string{
		"game root":      profile.GameRoot,
		"data directory": profile.DataDirectory,
		"SKSE launcher":  profile.SKSELauncher,
		"plugin state":   profile.PluginState,
	} {
		if _, err := os.Stat(path); err != nil {
			plan.Blockers = append(plan.Blockers, label+" missing: "+path)
		}
	}
	configSource := filepath.Join(target.PackageRoot, filepath.FromSlash(target.Target.RuntimeConfig))
	configMaterializedSource := ""
	configHash := ""
	if runtimeConfigErr == nil {
		configSource = runtimeConfig.SourcePath
		configMaterializedSource = runtimeConfig.StagedPath
		configHash = runtimeConfig.EffectiveSHA256
	}
	files := []skyrimFilePlan{
		{Kind: "esp", Source: filepath.Join(target.PackageRoot, filepath.FromSlash(target.Target.AssetOutput)), Destination: filepath.Join(profile.DataDirectory, target.Target.Bridge)},
		{Kind: "dll", Source: filepath.Join(target.PackageRoot, filepath.FromSlash(target.Target.NativeDLL)), Destination: filepath.Join(profile.DataDirectory, filepath.FromSlash(target.Target.DLLDestination))},
		{Kind: "config", Source: configSource, MaterializedSource: configMaterializedSource, Destination: filepath.Join(profile.DataDirectory, filepath.FromSlash(target.Target.ConfigDestination))},
	}
	if iniPlanErr == nil && iniPlan.Enabled {
		files = append(files, skyrimFilePlan{
			Kind:               "ini",
			Source:             iniPlan.sourcePath,
			MaterializedSource: iniPlan.StagedPath,
			Destination:        iniPlan.sourcePath,
			SourceSHA256:       iniPlan.EffectiveSHA256,
		})
	}
	for _, file := range files {
		if file.Kind == "config" && configHash != "" {
			file.SourceSHA256 = configHash
		} else if file.Kind != "ini" || file.SourceSHA256 == "" {
			file.SourceSHA256, _ = hashFile(skyrimMaterializedSource(file))
		}
		file.CurrentSHA256, _ = hashFile(file.Destination)
		if file.SourceSHA256 == "" {
			file.Action = "build"
		} else if file.SourceSHA256 == file.CurrentSHA256 {
			file.Action = "unchanged"
		} else if file.CurrentSHA256 == "" {
			file.Action = "create"
		} else {
			file.Action = "replace"
		}
		plan.Files = append(plan.Files, file)
	}
	statePlan, err := planSkyrimPluginState(profile.PluginState, target.Target.Bridge, target.Target.StalePlugins)
	if err != nil {
		plan.Blockers = append(plan.Blockers, err.Error())
	} else {
		plan.PluginState = statePlan
	}
	sort.Strings(plan.Blockers)
	return plan
}

func buildSkyrimRuntimeConfig(root string, target skyrimTargetRef, profile skyrimHostProfile, stage bool) (skyrimRuntimeConfigPlan, error) {
	sourcePath := filepath.Join(target.PackageRoot, filepath.FromSlash(target.Target.RuntimeConfig))
	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		return skyrimRuntimeConfigPlan{}, fmt.Errorf("read committed runtime config %s: %w", sourcePath, err)
	}
	sourceHash := hashBytes(sourceBytes)
	plan := skyrimRuntimeConfigPlan{
		SourcePath:       sourcePath,
		SourceSHA256:     sourceHash,
		EffectiveSHA256:  sourceHash,
		AllowedOverrides: append([]manifest.SkyrimRuntimeOverride(nil), target.Target.RuntimeOverrideFields...),
	}
	if len(profile.RuntimeOverrides) == 0 {
		return plan, nil
	}
	for overrideTarget := range profile.RuntimeOverrides {
		if overrideTarget != target.Target.RuntimeOverrideTarget {
			return skyrimRuntimeConfigPlan{}, fmt.Errorf("unknown runtime override target %q in host %q", overrideTarget, target.Target.Host)
		}
	}
	values, ok := profile.RuntimeOverrides[target.Target.RuntimeOverrideTarget]
	if !ok {
		return plan, nil
	}
	values = copySkyrimRuntimeOverrideValues(values)
	if err := applySkyrimFixtureRuntimeOverrides(profile, values); err != nil {
		return skyrimRuntimeConfigPlan{}, err
	}
	profileIdentity, ok := values["host"].(string)
	if !ok || profileIdentity == "" {
		return skyrimRuntimeConfigPlan{}, fmt.Errorf("runtime override target %q must declare its host identity", target.Target.RuntimeOverrideTarget)
	}
	if profileIdentity != target.Target.Host {
		return skyrimRuntimeConfigPlan{}, fmt.Errorf("runtime override target %q is for host %q, not selected host %q", target.Target.RuntimeOverrideTarget, profileIdentity, target.Target.Host)
	}
	var document map[string]any
	if err := toml.Unmarshal(sourceBytes, &document); err != nil {
		return skyrimRuntimeConfigPlan{}, fmt.Errorf("parse committed runtime config %s: %w", sourcePath, err)
	}
	allowed := map[string]manifest.SkyrimRuntimeOverride{}
	for _, field := range target.Target.RuntimeOverrideFields {
		allowed[field.Path] = field
		if sourceValue, found := getSkyrimTOMLValue(document, field.Path); (!found && !field.Secret) || (found && !skyrimRuntimeOverrideValueMatches(field.Type, sourceValue)) {
			return skyrimRuntimeConfigPlan{}, fmt.Errorf("committed runtime config path %q is missing or does not match declared type %s", field.Path, field.Type)
		}
	}
	for path, value := range values {
		if path == "host" {
			continue
		}
		field, declared := allowed[path]
		if !declared {
			return skyrimRuntimeConfigPlan{}, fmt.Errorf("runtime override path %q is not declared by the Skyrim target", path)
		}
		if !skyrimRuntimeOverrideValueMatches(field.Type, value) {
			return skyrimRuntimeConfigPlan{}, fmt.Errorf("runtime override path %q must be %s", path, field.Type)
		}
		sourceValue, _ := getSkyrimTOMLValue(document, path)
		plan.AppliedOverrides = append(plan.AppliedOverrides, skyrimRuntimeOverrideValue{Path: path, Type: field.Type, SourceValue: redactSkyrimOverrideValue(sourceValue, field.Secret), EffectiveValue: redactSkyrimOverrideValue(value, field.Secret), Secret: field.Secret})
	}
	sort.Slice(plan.AppliedOverrides, func(left int, right int) bool {
		return plan.AppliedOverrides[left].Path < plan.AppliedOverrides[right].Path
	})
	if len(plan.AppliedOverrides) == 0 {
		return plan, nil
	}
	effectiveBytes, err := applySkyrimRuntimeOverrides(sourceBytes, plan.AppliedOverrides, values)
	if err != nil {
		return skyrimRuntimeConfigPlan{}, err
	}
	plan.EffectiveSHA256 = hashBytes(effectiveBytes)
	plan.OverrideSource = filepath.Join(root, filepath.FromSlash(skyrimProfileRelativePath))
	plan.OverrideTarget = target.Target.RuntimeOverrideTarget
	plan.RestorationPlanned = true
	plan.StagedPath = filepath.Join(root, "build", "skyrim", "runtime", target.Target.RuntimeOverrideTarget+"-"+strings.ToLower(plan.EffectiveSHA256[:16])+".toml")
	if stage {
		if err := os.MkdirAll(filepath.Dir(plan.StagedPath), 0o755); err != nil {
			return skyrimRuntimeConfigPlan{}, err
		}
		if err := os.WriteFile(plan.StagedPath, effectiveBytes, 0o644); err != nil {
			return skyrimRuntimeConfigPlan{}, err
		}
	}
	return plan, nil
}

func copySkyrimRuntimeOverrideValues(source map[string]any) map[string]any {
	copy := make(map[string]any, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func applySkyrimFixtureRuntimeOverrides(profile skyrimHostProfile, values map[string]any) error {
	const bootstrapPath = "eternal_dragonborn.development.presenter.allow_session_bootstrap"
	const filenamePath = "eternal_dragonborn.development.presenter.development_session_ed_m2b2d"
	const readOnlyPath = "eternal_dragonborn.development.presenter.development_session_ed_m2b2d_read_only"
	bootstrapEnabled, _ := values[bootstrapPath].(bool)
	if !bootstrapEnabled {
		return nil
	}
	fixture, found := profile.TestSaves["ed-m2b2d"]
	if !found || !fixture.Disposable || !fixture.ReadOnly || !isSafeSkyrimFixtureFilename(fixture.Filename) {
		return errors.New("session bootstrap requires a declared disposable, read-only ed-m2b2d fixture")
	}
	if existing, declared := values[filenamePath]; declared && existing != fixture.Filename {
		return errors.New("session bootstrap filename must be supplied by the ed-m2b2d fixture mapping")
	}
	if existing, declared := values[readOnlyPath]; declared && existing != true {
		return errors.New("session bootstrap fixture mapping requires read-only=true")
	}
	values[filenamePath] = fixture.Filename
	values[readOnlyPath] = true
	return nil
}

func isSafeSkyrimFixtureFilename(value string) bool {
	if value == "" || filepath.Base(value) != value || !strings.EqualFold(filepath.Ext(value), ".ess") {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' || character == ' ' {
			continue
		}
		return false
	}
	return true
}

func getSkyrimTOMLValue(document map[string]any, path string) (any, bool) {
	var current any = document
	for _, segment := range strings.Split(path, ".") {
		table, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = table[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func applySkyrimRuntimeOverrides(source []byte, overrides []skyrimRuntimeOverrideValue, values map[string]any) ([]byte, error) {
	effective := append([]byte(nil), source...)
	for _, override := range overrides {
		value, ok := values[override.Path]
		if !ok {
			return nil, fmt.Errorf("runtime override path %q is missing its value", override.Path)
		}
		var err error
		effective, err = replaceSkyrimTOMLScalar(effective, override.Path, formatSkyrimTOMLScalar(value, override.Type), override.Secret)
		if err != nil {
			return nil, err
		}
	}
	return effective, nil
}

func replaceSkyrimTOMLScalar(source []byte, path string, replacement string, insertIfMissing bool) ([]byte, error) {
	segments := strings.Split(path, ".")
	if len(segments) < 2 {
		return nil, fmt.Errorf("runtime override path %q is not a TOML table leaf", path)
	}
	table := strings.Join(segments[:len(segments)-1], ".")
	key := segments[len(segments)-1]
	lines := strings.Split(string(source), "\n")
	currentTable := ""
	found := false
	tableStart := -1
	tableEnd := -1
	for index, line := range lines {
		lineEnding := ""
		content := line
		if strings.HasSuffix(content, "\r") {
			content = strings.TrimSuffix(content, "\r")
			lineEnding = "\r"
		}
		trimmed := strings.TrimSpace(content)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if currentTable == table && tableEnd == -1 {
				tableEnd = index
			}
			currentTable = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]"))
			if currentTable == table {
				tableStart = index
			}
			continue
		}
		if currentTable != table {
			continue
		}
		equals := strings.Index(trimmed, "=")
		if equals < 0 || strings.TrimSpace(trimmed[:equals]) != key {
			continue
		}
		if found {
			return nil, fmt.Errorf("committed runtime config path %q is declared more than once", path)
		}
		indentLength := len(content) - len(strings.TrimLeft(content, " \t"))
		lines[index] = content[:indentLength] + key + " = " + replacement + lineEnding
		found = true
	}
	if currentTable == table && tableEnd == -1 {
		tableEnd = len(lines)
	}
	if !found {
		if insertIfMissing && tableStart >= 0 && tableEnd > tableStart {
			lineEnding := ""
			if strings.Contains(string(source), "\r\n") {
				lineEnding = "\r"
			}
			insert := key + " = " + replacement + lineEnding
			lines = append(lines, "")
			copy(lines[tableEnd+1:], lines[tableEnd:])
			lines[tableEnd] = insert
			return []byte(strings.Join(lines, "\n")), nil
		}
		return nil, fmt.Errorf("committed runtime config path %q is missing", path)
	}
	return []byte(strings.Join(lines, "\n")), nil
}

func formatSkyrimTOMLScalar(value any, valueType string) string {
	switch valueType {
	case "boolean":
		return strconv.FormatBool(value.(bool))
	case "string":
		return strconv.Quote(value.(string))
	case "integer":
		switch integer := value.(type) {
		case int64:
			return strconv.FormatInt(integer, 10)
		case int32:
			return strconv.FormatInt(int64(integer), 10)
		case int:
			return strconv.Itoa(integer)
		}
	}
	return ""
}

func skyrimRuntimeOverrideValueMatches(valueType string, value any) bool {
	switch valueType {
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "integer":
		switch value.(type) {
		case int64, int32, int:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func redactSkyrimOverrideValue(value any, secret bool) any {
	if secret {
		return "<redacted>"
	}
	return value
}

func skyrimMaterializedSource(file skyrimFilePlan) string {
	if file.MaterializedSource != "" {
		return file.MaterializedSource
	}
	return file.Source
}

func executeSkyrimBuild(root string, target skyrimTargetRef, profile skyrimHostProfile) {
	commands := []struct {
		name string
		argv []string
		cwd  string
	}{
		{"native configure", target.Target.NativeConfigure.Command, skyrimCommandCwd(root, target, target.Target.NativeConfigure)},
		{"native build", target.Target.NativeBuild.Command, skyrimCommandCwd(root, target, target.Target.NativeBuild)},
		{"native tests", target.Target.NativeTests.Command, skyrimCommandCwd(root, target, target.Target.NativeTests)},
		{"asset tests", []string{"dotnet", "test", filepath.Join(target.PackageRoot, filepath.FromSlash(target.Target.AssetTestsProject))}, target.PackageRoot},
	}
	for _, command := range commands {
		if err := runSkyrimProcess(command.name, resolveSkyrimCommand(command.argv, profile.Tools), command.cwd); err != nil {
			failSkyrim("TSPACK_SKYRIM_BUILD_FAILED", err)
		}
	}
	assetCommand := []string{"dotnet", "run", "--project", filepath.Join(target.PackageRoot, filepath.FromSlash(target.Target.AssetCompilerProject)), "--", "compile"}
	for _, pack := range target.Target.AssetPacks {
		assetCommand = append(assetCommand, "--source", filepath.Join(target.PackageRoot, filepath.FromSlash(pack.Source)))
	}
	assetCommand = append(assetCommand, "--output", filepath.Join(target.PackageRoot, filepath.FromSlash(target.Target.AssetOutput)), "--game-data", profile.DataDirectory)
	if err := runSkyrimProcess("asset bridge compile", resolveSkyrimCommand(assetCommand, profile.Tools), target.PackageRoot); err != nil {
		failSkyrim("TSPACK_SKYRIM_ASSET_BUILD_FAILED", err)
	}
	if err := inspectAndVerifySkyrimBridges(target, profile); err != nil {
		failSkyrim("TSPACK_SKYRIM_BRIDGE_PARITY_FAILED", err)
	}
}

func inspectAndVerifySkyrimBridges(target skyrimTargetRef, profile skyrimHostProfile) error {
	project := filepath.Join(target.PackageRoot, filepath.FromSlash(target.Target.AssetCompilerProject))
	reportDirectory := filepath.Join(target.PackageRoot, "build", "skyrim")
	legacyPath := filepath.Join(profile.DataDirectory, target.Target.Bridge)
	stagedPath := filepath.Join(target.PackageRoot, filepath.FromSlash(target.Target.AssetOutput))
	deployedReport := filepath.Join(reportDirectory, "deployed-bridge-report")
	migrationReport := filepath.Join(reportDirectory, "bridge-migration-report")
	stagedReport := filepath.Join(reportDirectory, "staged-bridge-report")
	for _, inspection := range []struct {
		name   string
		input  string
		report string
	}{
		{"deployed bridge inspection", legacyPath, deployedReport},
		{"staged bridge inspection", stagedPath, stagedReport},
	} {
		command := resolveSkyrimCommand([]string{"dotnet", "run", "--project", project, "--", "inspect-bridge", "--input", inspection.input, "--report", inspection.report}, profile.Tools)
		if err := runSkyrimProcess(inspection.name, command, target.PackageRoot); err != nil {
			return err
		}
	}
	// The migration audit is historical evidence about the bridge that existed
	// before TSPack first took ownership. Recurring lifecycle runs refresh the
	// deployed report without overwriting that evidence.
	if _, err := os.Stat(migrationReport + ".json"); os.IsNotExist(err) {
		for _, extension := range []string{".json", ".txt"} {
			if err := copySkyrimFile(deployedReport+extension, migrationReport+extension); err != nil {
				return err
			}
		}
	}
	legacy, err := readSkyrimBridgeInspection(deployedReport + ".json")
	if err != nil {
		return err
	}
	staged, err := readSkyrimBridgeInspection(stagedReport + ".json")
	if err != nil {
		return err
	}
	expected := map[string]string{}
	for _, record := range target.Target.ExpectedRecords {
		expected[record.EditorID] = strings.ToUpper(record.LocalFormID)
	}
	if err := verifySkyrimBridgeRecords(legacy, expected, false); err != nil {
		return fmt.Errorf("deployed bridge parity: %w", err)
	}
	if err := verifySkyrimBridgeRecords(staged, expected, true); err != nil {
		return fmt.Errorf("staged bridge parity: %w", err)
	}
	return nil
}

func readSkyrimBridgeInspection(path string) (skyrimBridgeInspection, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return skyrimBridgeInspection{}, err
	}
	var report skyrimBridgeInspection
	if err := json.Unmarshal(data, &report); err != nil {
		return skyrimBridgeInspection{}, err
	}
	return report, nil
}

func verifySkyrimBridgeRecords(report skyrimBridgeInspection, expected map[string]string, requireOwned bool) error {
	if len(report.Records) != len(expected) {
		return fmt.Errorf("record count mismatch: expected=%d actual=%d", len(expected), len(report.Records))
	}
	seen := map[string]struct{}{}
	for _, record := range report.Records {
		expectedID, ok := expected[record.EditorID]
		if !ok {
			return fmt.Errorf("unrepresented record %s", record.EditorID)
		}
		if strings.ToUpper(record.LocalFormID) != expectedID {
			return fmt.Errorf("record %s local FormID changed: expected=%s actual=%s", record.EditorID, expectedID, record.LocalFormID)
		}
		if requireOwned && (record.Owner != "MarionetteSSE.esp" || record.SuspiciousWrongOwner) {
			return fmt.Errorf("record %s has wrong staged owner %s", record.EditorID, record.Owner)
		}
		seen[record.EditorID] = struct{}{}
	}
	for editorID := range expected {
		if _, ok := seen[editorID]; !ok {
			return fmt.Errorf("required record %s is missing", editorID)
		}
	}
	return nil
}

func skyrimCommandCwd(root string, target skyrimTargetRef, command manifest.SkyrimCommand) string {
	if command.Cwd == "workspace" {
		return root
	}
	return target.PackageRoot
}

func resolveSkyrimCommand(argv []string, tools map[string]string) []string {
	resolved := append([]string(nil), argv...)
	if len(resolved) > 0 {
		if tool, ok := tools[resolved[0]]; ok && tool != "" {
			resolved[0] = tool
		}
	}
	return resolved
}

func runSkyrimProcess(name string, argv []string, cwd string) error {
	if len(argv) == 0 {
		return fmt.Errorf("%s command is empty", name)
	}
	fmt.Fprintf(os.Stderr, "Skyrim lifecycle: %s\n", name)
	executable := argv[0]
	if !filepath.IsAbs(executable) && strings.ContainsAny(executable, `/\`) {
		executable = filepath.Join(cwd, filepath.FromSlash(executable))
	}
	cmd := exec.Command(executable, argv[1:]...)
	cmd.Dir = cwd
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}

func planSkyrimPluginState(path string, bridge string, stale []string) (skyrimPluginStatePlan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return skyrimPluginStatePlan{}, fmt.Errorf("plugin state unavailable: %s: %w", path, err)
	}
	if !utf8.Valid(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})) || bytes.IndexByte(data, 0) >= 0 {
		return skyrimPluginStatePlan{}, fmt.Errorf("plugin state is malformed or unsupported: %s", path)
	}
	lines := splitStateLines(data)
	desired, removed := normalizeSkyrimPluginEntries(lines, bridge, stale)
	return skyrimPluginStatePlan{Path: path, CurrentEntries: lines, DesiredEntries: desired, RemovedStale: removed, Changed: !stringSlicesEqual(lines, desired)}, nil
}

func splitStateLines(data []byte) []string {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return []string{}
	}
	return strings.Split(text, "\n")
}

func normalizeSkyrimPluginEntries(lines []string, bridge string, stale []string) ([]string, []string) {
	staleSet := map[string]struct{}{}
	for _, entry := range stale {
		staleSet[strings.ToLower(entry)] = struct{}{}
	}
	result := make([]string, 0, len(lines)+1)
	foundBridge := false
	var removed []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		name := strings.TrimPrefix(trimmed, "*")
		if strings.EqualFold(name, bridge) {
			if !foundBridge {
				result = append(result, "*"+bridge)
				foundBridge = true
			}
			continue
		}
		if _, ok := staleSet[strings.ToLower(name)]; ok {
			removed = append(removed, name)
			continue
		}
		result = append(result, line)
	}
	if !foundBridge {
		result = append(result, "*"+bridge)
	}
	sort.Strings(removed)
	return result, removed
}

func materializeSkyrimPlan(root string, plan skyrimPlanReport) (skyrimMaterializationReport, error) {
	report := skyrimMaterializationReport{Plan: plan}
	rollbackID := plan.ManifestSHA256
	for _, file := range plan.Files {
		rollbackID += file.SourceSHA256
	}
	sum := sha256.Sum256([]byte(rollbackID))
	report.RollbackPath = filepath.Join(root, "build", "skyrim", "rollback", hex.EncodeToString(sum[:8]))
	if err := os.MkdirAll(report.RollbackPath, 0o755); err != nil {
		return report, err
	}
	type appliedFile struct {
		destination string
		backup      string
		existed     bool
	}
	var applied []appliedFile
	rollback := func() {
		for index := len(applied) - 1; index >= 0; index-- {
			item := applied[index]
			if item.existed {
				_ = copyAndReplaceFile(item.backup, item.destination)
			} else {
				_ = os.Remove(item.destination)
			}
		}
	}
	apply := func(source string, destination string, backupName string) error {
		backup := filepath.Join(report.RollbackPath, backupName)
		_, statErr := os.Stat(destination)
		existed := statErr == nil
		if existed {
			if err := copySkyrimFile(destination, backup); err != nil {
				return err
			}
		}
		if err := copyAndReplaceFile(source, destination); err != nil {
			return err
		}
		applied = append(applied, appliedFile{destination: destination, backup: backup, existed: existed})
		return nil
	}
	for _, file := range plan.Files {
		if file.Action == "unchanged" {
			report.UnchangedFiles = append(report.UnchangedFiles, file.Destination)
			continue
		}
		if err := apply(skyrimMaterializedSource(file), file.Destination, file.Kind+filepath.Ext(file.Destination)); err != nil {
			rollback()
			return report, err
		}
		deployedHash, err := hashFile(file.Destination)
		if err != nil || deployedHash != file.SourceSHA256 {
			rollback()
			return report, fmt.Errorf("deployed hash verification failed for %s", file.Destination)
		}
		report.ChangedFiles = append(report.ChangedFiles, file.Destination)
	}
	if plan.PluginState.Changed {
		stateBytes, err := os.ReadFile(plan.PluginState.Path)
		if err != nil {
			rollback()
			return report, err
		}
		lineEnding := "\n"
		if bytes.Contains(stateBytes, []byte("\r\n")) {
			lineEnding = "\r\n"
		}
		prefix := []byte{}
		if bytes.HasPrefix(stateBytes, []byte{0xEF, 0xBB, 0xBF}) {
			prefix = []byte{0xEF, 0xBB, 0xBF}
		}
		desiredBytes := append(prefix, []byte(strings.Join(plan.PluginState.DesiredEntries, lineEnding)+lineEnding)...)
		tempSource := filepath.Join(report.RollbackPath, "plugins.desired.txt")
		if err := os.WriteFile(tempSource, desiredBytes, 0o644); err != nil {
			rollback()
			return report, err
		}
		if err := apply(tempSource, plan.PluginState.Path, "plugins.previous.txt"); err != nil {
			rollback()
			return report, err
		}
		report.PluginStateChanged = true
	}
	return report, nil
}

func copySkyrimFile(source string, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(destination)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func copyAndReplaceFile(source string, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(destination), ".tspack-skyrim-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	keep := true
	defer func() {
		_ = temp.Close()
		if keep {
			_ = os.Remove(tempPath)
		}
	}()
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(temp, in)
	closeInErr := in.Close()
	syncErr := temp.Sync()
	closeErr := temp.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeInErr != nil {
		return closeInErr
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := replaceFileAtomic(tempPath, destination); err != nil {
		return err
	}
	keep = false
	return nil
}

func launchSkyrim(report *skyrimMaterializationReport, target skyrimTargetRef, profile skyrimHostProfile, launchManagedController bool) error {
	before := newestMatchingFile(profile.RuntimeLogDirectory, target.Target.RuntimeEvidencePattern)
	var managed *exec.Cmd
	var managedDone chan error
	if launchManagedController {
		command := managedControllerCommand(target.PackageRoot, report.SessionBootstrapConfig)
		if len(command) == 0 {
			return errors.New("managed Aurelian controller command is unavailable")
		}
		logPath := filepath.Join(target.PackageRoot, "build", "skyrim", "managed-controller.log")
		if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
			return err
		}
		logFile, err := os.Create(logPath)
		if err != nil {
			return err
		}
		defer logFile.Close()
		managed = exec.Command(command[0], command[1:]...)
		managed.Dir = target.PackageRoot
		managed.Env = os.Environ()
		managed.Stdout = logFile
		managed.Stderr = logFile
		if err := managed.Start(); err != nil {
			return fmt.Errorf("start managed Aurelian controller: %w", err)
		}
		report.ManagedControllerPID = managed.Process.Pid
		report.ManagedControllerLog = logPath
		managedDone = make(chan error, 1)
		go func() { managedDone <- managed.Wait() }()
		select {
		case err := <-managedDone:
			report.ManagedControllerExited = true
			if err == nil {
				return errors.New("managed Aurelian controller exited before Skyrim launch")
			}
			return fmt.Errorf("managed Aurelian controller startup failed: %w", err)
		case <-time.After(250 * time.Millisecond):
		}
	}
	if managed != nil {
		defer func() {
			if !report.ManagedControllerExited && managed.Process != nil {
				_ = managed.Process.Kill()
				<-managedDone
				report.ManagedControllerExited = true
			}
		}()
	}
	cmd := exec.Command(profile.SKSELauncher)
	cmd.Dir = profile.GameRoot
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return err
	}
	report.LaunchedExecutable = profile.SKSELauncher
	report.LauncherPID = cmd.Process.Pid
	launcherDone := make(chan error, 1)
	go func() {
		launcherDone <- cmd.Wait()
	}()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if runtimePID := findSkyrimRuntimeProcess(); runtimePID != 0 {
			report.RuntimePID = runtimePID
		}
		candidate := newestMatchingFile(profile.RuntimeLogDirectory, target.Target.RuntimeEvidencePattern)
		if candidate != "" && candidate != before {
			report.RuntimeLog = candidate
			data, _ := os.ReadFile(candidate)
			if bytes.Contains(data, []byte(target.Target.ReadyMarker)) {
				report.RuntimeVerified = true
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !report.RuntimeVerified {
		return fmt.Errorf("runtime ready marker %q was not observed within 30 seconds", target.Target.ReadyMarker)
	}
	if err := <-launcherDone; err != nil {
		return fmt.Errorf("SKSE launcher exited: %w", err)
	}
	for report.RuntimePID != 0 && skyrimRuntimeProcessExists(report.RuntimePID) {
		if managedDone != nil && !report.ManagedControllerExited {
			select {
			case managedErr := <-managedDone:
				report.ManagedControllerExited = true
				if managedErr != nil {
					for skyrimRuntimeProcessExists(report.RuntimePID) {
						time.Sleep(500 * time.Millisecond)
					}
					return fmt.Errorf("managed Aurelian controller failed; Skyrim exited before scoped restoration: %w", managedErr)
				}
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}
		time.Sleep(500 * time.Millisecond)
	}
	if managedDone != nil && !report.ManagedControllerExited {
		select {
		case managedErr := <-managedDone:
			report.ManagedControllerExited = true
			if managedErr != nil {
				return fmt.Errorf("managed Aurelian controller exited with failure: %w", managedErr)
			}
		case <-time.After(2 * time.Second):
			_ = managed.Process.Kill()
			<-managedDone
			report.ManagedControllerExited = true
		}
	}
	return nil
}

func managedControllerCommand(root string, configPath string) []string {
	project := os.Getenv("AURELIAN_MARIONETTE_PROJECT")
	if project == "" {
		userProfile, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		project = filepath.Join(userProfile, "source", "repos", "Copeland", "src", "Aurelian", "Aurelian.Marionette.Transport", "Aurelian.Marionette.Transport.csproj")
	}
	return []string{
		"dotnet", "run", "--project", project, "--",
		"live-save-correlation", "--config", configPath,
	}
}

func newestMatchingFile(directory string, pattern string) string {
	matches, _ := filepath.Glob(filepath.Join(directory, filepath.FromSlash(pattern)))
	sort.Slice(matches, func(i, j int) bool {
		left, leftErr := os.Stat(matches[i])
		right, rightErr := os.Stat(matches[j])
		if leftErr != nil || rightErr != nil {
			return matches[i] < matches[j]
		}
		return left.ModTime().After(right.ModTime())
	})
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}

func writeSkyrimReports(root string, plan skyrimPlanReport, report skyrimMaterializationReport) error {
	directory := filepath.Join(root, "build", "skyrim")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(directory, "deployment-plan.json"), plan); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(directory, "deployment-plan.txt"), []byte(formatSkyrimPlan(plan)), 0o644); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(directory, "materialization-report.json"), report); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(directory, "materialization-report.txt"), []byte(formatSkyrimMaterialization(report)), 0o644); err != nil {
		return err
	}
	verification := fmt.Sprintf("launched=%t\nlauncherPid=%d\nruntimePid=%d\nlog=%s\nmarker=%s\nverified=%t\nmanagedControllerPid=%d\nmanagedControllerLog=%s\nmanagedControllerExited=%t\n", report.LauncherPID != 0, report.LauncherPID, report.RuntimePID, report.RuntimeLog, report.Plan.ReadyMarker, report.RuntimeVerified, report.ManagedControllerPID, report.ManagedControllerLog, report.ManagedControllerExited)
	return os.WriteFile(filepath.Join(directory, "runtime-verification.txt"), []byte(verification), 0o644)
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func renderSkyrimPlan(plan skyrimPlanReport, jsonOutput bool) {
	if jsonOutput {
		data, _ := json.MarshalIndent(plan, "", "  ")
		fmt.Println(string(data))
		return
	}
	fmt.Print(formatSkyrimPlan(plan))
}

func formatSkyrimPlan(plan skyrimPlanReport) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Skyrim deployment plan\nHost: %s\nGame: %s\nData: %s\n", plan.Host, plan.GameRoot, plan.DataDirectory)
	fmt.Fprintf(&builder, "Native configure: %s\nNative build: %s\nAsset compiler: %s\n", strings.Join(plan.NativeConfigure, " "), strings.Join(plan.NativeBuild, " "), strings.Join(plan.AssetCompilerCommand, " "))
	for _, file := range plan.Files {
		fmt.Fprintf(&builder, "%s: %s -> %s (%s)\n", file.Kind, file.Source, file.Destination, file.Action)
	}
	fmt.Fprintf(&builder, "Runtime config: %s source=%s effective=%s\n", plan.RuntimeConfig.SourcePath, plan.RuntimeConfig.SourceSHA256, plan.RuntimeConfig.EffectiveSHA256)
	for _, override := range plan.RuntimeConfig.AppliedOverrides {
		fmt.Fprintf(&builder, "runtime override: %s: %v -> %v\n", override.Path, override.SourceValue, override.EffectiveValue)
	}
	if plan.RuntimeConfig.RestorationPlanned {
		fmt.Fprintf(&builder, "post-run restoration: %s -> committed safe configuration\n", plan.RuntimeConfig.SourcePath)
	}
	fmt.Fprintf(&builder, "Plugin state: %s changed=%t\nLaunch: %s\n", plan.PluginState.Path, plan.PluginState.Changed, strings.Join(plan.LaunchCommand, " "))
	if len(plan.ManagedControllerCommand) > 0 {
		fmt.Fprintf(&builder, "Managed controller: %s\n", strings.Join(plan.ManagedControllerCommand, " "))
	}
	fmt.Fprintf(&builder, "Evidence: %s\n", plan.RuntimeEvidence)
	for _, blocker := range plan.Blockers {
		fmt.Fprintf(&builder, "BLOCKER: %s\n", blocker)
	}
	return builder.String()
}

func renderSkyrimMaterialization(report skyrimMaterializationReport, jsonOutput bool) {
	if jsonOutput {
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
		return
	}
	fmt.Print(formatSkyrimMaterialization(report))
}

func formatSkyrimMaterialization(report skyrimMaterializationReport) string {
	return fmt.Sprintf("Skyrim materialization complete\nChanged files: %d\nUnchanged files: %d\nPlugin state changed: %t\nRollback: %s\nLaunched: %s launcherPID=%d runtimePID=%d\nRuntime log: %s\nRuntime verified: %t\nRestoration attempted: %t verified: %t hash: %s error: %s\n", len(report.ChangedFiles), len(report.UnchangedFiles), report.PluginStateChanged, report.RollbackPath, report.LaunchedExecutable, report.LauncherPID, report.RuntimePID, report.RuntimeLog, report.RuntimeVerified, report.RestorationAttempted, report.RestorationVerified, report.RestoredConfigSHA256, report.RestorationError)
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(hash.Sum(nil))), nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func restoreSkyrimRuntimeConfig(root string, plan skyrimPlanReport) error {
	if !plan.RuntimeConfig.RestorationPlanned {
		return nil
	}
	for _, file := range plan.Files {
		if file.Kind != "config" {
			continue
		}
		safeHash, err := hashFile(plan.RuntimeConfig.SourcePath)
		if err != nil {
			return err
		}
		restoreFile := file
		restoreFile.MaterializedSource = ""
		restoreFile.SourceSHA256 = safeHash
		restoreFile.CurrentSHA256, _ = hashFile(file.Destination)
		if restoreFile.CurrentSHA256 == safeHash {
			restoreFile.Action = "unchanged"
		} else if restoreFile.CurrentSHA256 == "" {
			restoreFile.Action = "create"
		} else {
			restoreFile.Action = "replace"
		}
		_, err = materializeSkyrimPlan(root, skyrimPlanReport{ManifestSHA256: plan.ManifestSHA256 + "-restore", Files: []skyrimFilePlan{restoreFile}})
		return err
	}
	return fmt.Errorf("runtime restoration was planned but no config file was present")
}

func restoreSkyrimINI(plan skyrimPlanReport, report skyrimMaterializationReport) error {
	if !plan.INI.RestorationPlanned {
		return nil
	}
	backupPath := filepath.Join(report.RollbackPath, "ini"+filepath.Ext(plan.INI.sourcePath))
	if err := copyAndReplaceFile(backupPath, plan.INI.sourcePath); err != nil {
		return fmt.Errorf("restore Skyrim INI: %w", err)
	}
	restoredHash, err := hashFile(plan.INI.sourcePath)
	if err != nil {
		return fmt.Errorf("hash restored Skyrim INI: %w", err)
	}
	if restoredHash != plan.INI.SourceSHA256 {
		return fmt.Errorf("restored Skyrim INI hash mismatch")
	}
	return nil
}

func stringSlicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func failSkyrim(code string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", code, err)
	os.Exit(1)
}
