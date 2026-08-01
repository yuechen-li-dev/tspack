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
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/version"
)

const skyrimProfileRelativePath = ".tspack/skyrim-hosts.toml"

type skyrimRunOptions struct {
	root     string
	dryRun   bool
	json     bool
	noLaunch bool
}

type skyrimProfilesFile struct {
	Hosts map[string]skyrimHostProfile `toml:"hosts"`
}

type skyrimHostProfile struct {
	GameRoot            string            `toml:"gameRoot"`
	DataDirectory       string            `toml:"dataDirectory"`
	SKSELauncher        string            `toml:"skseLauncher"`
	PluginState         string            `toml:"pluginState"`
	RuntimeLogDirectory string            `toml:"runtimeLogDirectory"`
	RuntimeVersion      string            `toml:"runtimeVersion"`
	Tools               map[string]string `toml:"tools"`
}

type skyrimTargetRef struct {
	PackageName string
	PackageRoot string
	Target      manifest.SkyrimTarget
}

type skyrimFilePlan struct {
	Kind          string `json:"kind"`
	Source        string `json:"source"`
	Destination   string `json:"destination"`
	SourceSHA256  string `json:"sourceSha256,omitempty"`
	CurrentSHA256 string `json:"currentSha256,omitempty"`
	Action        string `json:"action"`
}

type skyrimPluginStatePlan struct {
	Path           string   `json:"path"`
	CurrentEntries []string `json:"currentEntries"`
	DesiredEntries []string `json:"desiredEntries"`
	RemovedStale   []string `json:"removedStale,omitempty"`
	Changed        bool     `json:"changed"`
}

type skyrimPlanReport struct {
	Command              string                     `json:"command"`
	TSPackVersion        string                     `json:"tspackVersion"`
	TSPackCommit         string                     `json:"tspackCommit"`
	ManifestPath         string                     `json:"manifestPath"`
	ManifestSHA256       string                     `json:"manifestSha256"`
	Package              string                     `json:"package"`
	Target               string                     `json:"target"`
	Host                 string                     `json:"host"`
	RuntimeVersion       string                     `json:"runtimeVersion"`
	GameRoot             string                     `json:"gameRoot"`
	DataDirectory        string                     `json:"dataDirectory"`
	SKSELauncher         string                     `json:"skseLauncher"`
	AssetPacks           []manifest.SkyrimAssetPack `json:"assetPacks"`
	NativeConfigure      []string                   `json:"nativeConfigure"`
	NativeBuild          []string                   `json:"nativeBuild"`
	AssetCompilerCommand []string                   `json:"assetCompilerCommand"`
	Files                []skyrimFilePlan           `json:"files"`
	PluginState          skyrimPluginStatePlan      `json:"pluginState"`
	LaunchCommand        []string                   `json:"launchCommand"`
	RuntimeEvidence      string                     `json:"runtimeEvidence"`
	ReadyMarker          string                     `json:"readyMarker"`
	Blockers             []string                   `json:"blockers"`
}

type skyrimMaterializationReport struct {
	Plan               skyrimPlanReport `json:"plan"`
	ChangedFiles       []string         `json:"changedFiles"`
	UnchangedFiles     []string         `json:"unchangedFiles"`
	RollbackPath       string           `json:"rollbackPath"`
	PluginStateChanged bool             `json:"pluginStateChanged"`
	LaunchedExecutable string           `json:"launchedExecutable,omitempty"`
	LauncherPID        int              `json:"launcherPid,omitempty"`
	RuntimePID         int              `json:"runtimePid,omitempty"`
	RuntimeLog         string           `json:"runtimeLog,omitempty"`
	RuntimeVerified    bool             `json:"runtimeVerified"`
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
	profile, err := loadSkyrimProfile(root, target.Target.Host)
	if err != nil {
		failSkyrim("TSPACK_SKYRIM_PROFILE_INVALID", err)
	}
	plan := buildSkyrimPlan(root, manifestPath, target, profile)
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
	plan = buildSkyrimPlan(root, manifestPath, target, profile)
	if len(plan.Blockers) > 0 {
		failSkyrim("TSPACK_SKYRIM_STAGING_FAILED", errors.New(strings.Join(plan.Blockers, "; ")))
	}
	report, err := materializeSkyrimPlan(root, plan)
	if err != nil {
		failSkyrim("TSPACK_SKYRIM_MATERIALIZATION_FAILED", err)
	}
	if !opts.noLaunch {
		launchSkyrim(&report, target, profile)
	}
	if err := writeSkyrimReports(root, plan, report); err != nil {
		failSkyrim("TSPACK_SKYRIM_REPORT_FAILED", err)
	}
	renderSkyrimMaterialization(report, opts.json)
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
		case "--root":
			if index+1 >= len(args) {
				failSkyrim("TSPACK_SKYRIM_ARGUMENT_INVALID", errors.New("--root requires a value"))
			}
			index++
			opts.root = args[index]
		default:
			failSkyrim("TSPACK_SKYRIM_ARGUMENT_INVALID", fmt.Errorf("unknown argument %s", args[index]))
		}
	}
	return opts
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

func buildSkyrimPlan(root string, manifestPath string, target skyrimTargetRef, profile skyrimHostProfile) skyrimPlanReport {
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
	files := []skyrimFilePlan{
		{Kind: "esp", Source: filepath.Join(target.PackageRoot, filepath.FromSlash(target.Target.AssetOutput)), Destination: filepath.Join(profile.DataDirectory, target.Target.Bridge)},
		{Kind: "dll", Source: filepath.Join(target.PackageRoot, filepath.FromSlash(target.Target.NativeDLL)), Destination: filepath.Join(profile.DataDirectory, filepath.FromSlash(target.Target.DLLDestination))},
		{Kind: "config", Source: filepath.Join(target.PackageRoot, filepath.FromSlash(target.Target.RuntimeConfig)), Destination: filepath.Join(profile.DataDirectory, filepath.FromSlash(target.Target.ConfigDestination))},
	}
	for _, file := range files {
		file.SourceSHA256, _ = hashFile(file.Source)
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
		if err := apply(file.Source, file.Destination, file.Kind+filepath.Ext(file.Destination)); err != nil {
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

func launchSkyrim(report *skyrimMaterializationReport, target skyrimTargetRef, profile skyrimHostProfile) {
	before := newestMatchingFile(profile.RuntimeLogDirectory, target.Target.RuntimeEvidencePattern)
	cmd := exec.Command(profile.SKSELauncher)
	cmd.Dir = profile.GameRoot
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		failSkyrim("TSPACK_SKYRIM_LAUNCH_FAILED", err)
	}
	report.LaunchedExecutable = profile.SKSELauncher
	report.LauncherPID = cmd.Process.Pid
	_ = cmd.Process.Release()
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
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
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
	verification := fmt.Sprintf("launched=%t\nlauncherPid=%d\nruntimePid=%d\nlog=%s\nmarker=%s\nverified=%t\n", report.LauncherPID != 0, report.LauncherPID, report.RuntimePID, report.RuntimeLog, report.Plan.ReadyMarker, report.RuntimeVerified)
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
	fmt.Fprintf(&builder, "Plugin state: %s changed=%t\nLaunch: %s\nEvidence: %s\n", plan.PluginState.Path, plan.PluginState.Changed, strings.Join(plan.LaunchCommand, " "), plan.RuntimeEvidence)
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
	return fmt.Sprintf("Skyrim materialization complete\nChanged files: %d\nUnchanged files: %d\nPlugin state changed: %t\nRollback: %s\nLaunched: %s launcherPID=%d runtimePID=%d\nRuntime log: %s\nRuntime verified: %t\n", len(report.ChangedFiles), len(report.UnchangedFiles), report.PluginStateChanged, report.RollbackPath, report.LaunchedExecutable, report.LauncherPID, report.RuntimePID, report.RuntimeLog, report.RuntimeVerified)
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
