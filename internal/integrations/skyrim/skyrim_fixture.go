package skyrim

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const skyrimFixturePrefix = "MarionetteFixture-"

type skyrimSaveCandidate struct {
	CandidateID        string    `json:"candidateId"`
	DisplayName        string    `json:"displayName"`
	EssPresent         bool      `json:"essPresent"`
	SkseSidecarPresent bool      `json:"skseSidecarPresent"`
	SizeBytes          int64     `json:"sizeBytes"`
	ModifiedTime       time.Time `json:"modifiedTime"`
	SourceHashEss      string    `json:"sourceHashEss"`
	SourceHashSkse     string    `json:"sourceHashSkse,omitempty"`
	LikelyAutosave     bool      `json:"likelyAutosave"`
	LikelyQuicksave    bool      `json:"likelyQuicksave"`
	LikelyManualSave   bool      `json:"likelyManualSave"`

	essPath  string
	sksePath string
}

type skyrimSaveListOptions struct {
	root              string
	host              string
	json              bool
	manualOnly        bool
	excludeAutosaves  bool
	excludeQuicksaves bool
	requireSidecar    bool
	modifiedAfter     *time.Time
	modifiedBefore    *time.Time
}

type skyrimFixtureCreateOptions struct {
	root        string
	host        string
	fixtureID   string
	candidateID string
	replace     bool
	dryRun      bool
	json        bool
}

type skyrimFixtureFileSnapshot struct {
	SHA256       string    `json:"sha256,omitempty"`
	SizeBytes    int64     `json:"sizeBytes,omitempty"`
	ModifiedTime time.Time `json:"modifiedTime,omitempty"`
	Present      bool      `json:"present"`
}

type skyrimFixturePlan struct {
	Command             string                    `json:"command"`
	HostProfile         string                    `json:"hostProfile"`
	SaveDirectory       string                    `json:"saveDirectory"`
	SaveDirectorySource string                    `json:"saveDirectorySource"`
	FixtureSymbolicID   string                    `json:"fixtureSymbolicId"`
	SourceCandidateID   string                    `json:"sourceCandidateId"`
	SourceEss           skyrimFixtureFileSnapshot `json:"sourceEss"`
	SourceSkse          skyrimFixtureFileSnapshot `json:"sourceSkse"`
	FixtureEss          skyrimFixtureFileSnapshot `json:"fixtureEss"`
	FixtureSkse         skyrimFixtureFileSnapshot `json:"fixtureSkse"`
	FixtureFilename     string                    `json:"fixtureFilename"`
	SidecarStatus       string                    `json:"sidecarStatus"`
	Action              string                    `json:"action"`
	ProfileMapping      string                    `json:"profileMapping"`
	ProfileUpdateNeeded bool                      `json:"profileUpdateNeeded"`
}

type skyrimFixtureReport struct {
	skyrimFixturePlan
	SourceEssAfter               skyrimFixtureFileSnapshot `json:"sourceEssAfter"`
	SourceSkseAfter              skyrimFixtureFileSnapshot `json:"sourceSkseAfter"`
	FixtureProfileMappingWritten bool                      `json:"fixtureProfileMappingWritten"`
	RollbackStatus               string                    `json:"rollbackStatus"`
}

type skyrimFixtureInspection struct {
	HostProfile           string                    `json:"hostProfile"`
	FixtureSymbolicID     string                    `json:"fixtureSymbolicId"`
	MappingExists         bool                      `json:"mappingExists"`
	Disposable            bool                      `json:"disposable"`
	ReadOnly              bool                      `json:"readOnly"`
	Ess                   skyrimFixtureFileSnapshot `json:"ess"`
	Skse                  skyrimFixtureFileSnapshot `json:"skse"`
	HashesMatch           bool                      `json:"hashesMatch"`
	SourceProvenanceKnown bool                      `json:"sourceProvenanceKnown"`
}

func runSkyrimFixtureCommand(args []string, loadManifest ManifestLoader) {
	if len(args) < 3 {
		failSkyrim("TSPACK_SKYRIM_FIXTURE_ARGUMENT_INVALID", errors.New("usage: tspack skyrim saves list | fixture create <id> --from <candidate-id> | fixture inspect <id>"))
	}
	switch args[1] {
	case "saves":
		if args[2] != "list" {
			failSkyrim("TSPACK_SKYRIM_FIXTURE_ARGUMENT_INVALID", errors.New("usage: tspack skyrim saves list"))
		}
		runSkyrimSaveList(parseSkyrimSaveListOptions(args[3:]), loadManifest)
	case "fixture":
		switch args[2] {
		case "create":
			runSkyrimFixtureCreate(parseSkyrimFixtureCreateOptions(args[3:]), loadManifest)
		case "inspect":
			fixtureID, root, host, jsonOutput := parseSkyrimFixtureInspectOptions(args[3:])
			runSkyrimFixtureInspect(fixtureID, root, host, jsonOutput, loadManifest)
		default:
			failSkyrim("TSPACK_SKYRIM_FIXTURE_ARGUMENT_INVALID", errors.New("usage: tspack skyrim fixture create|inspect"))
		}
	default:
		failSkyrim("TSPACK_SKYRIM_FIXTURE_ARGUMENT_INVALID", errors.New("usage: tspack skyrim saves|fixture"))
	}
}

func parseSkyrimSaveListOptions(args []string) skyrimSaveListOptions {
	opts := skyrimSaveListOptions{root: "."}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--root":
			opts.root = requiredSkyrimFixtureArgument(args, &index, "--root")
		case "--host":
			opts.host = requiredSkyrimFixtureArgument(args, &index, "--host")
		case "--json":
			opts.json = true
		case "--manual-only":
			opts.manualOnly = true
		case "--exclude-autosaves":
			opts.excludeAutosaves = true
		case "--exclude-quicksaves":
			opts.excludeQuicksaves = true
		case "--require-sidecar":
			opts.requireSidecar = true
		case "--modified-after":
			value := requiredSkyrimFixtureArgument(args, &index, "--modified-after")
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				failSkyrim("TSPACK_SKYRIM_FIXTURE_ARGUMENT_INVALID", fmt.Errorf("--modified-after must be RFC3339: %w", err))
			}
			opts.modifiedAfter = &parsed
		case "--modified-before":
			value := requiredSkyrimFixtureArgument(args, &index, "--modified-before")
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				failSkyrim("TSPACK_SKYRIM_FIXTURE_ARGUMENT_INVALID", fmt.Errorf("--modified-before must be RFC3339: %w", err))
			}
			opts.modifiedBefore = &parsed
		default:
			failSkyrim("TSPACK_SKYRIM_FIXTURE_ARGUMENT_INVALID", fmt.Errorf("unknown save-list argument %s", args[index]))
		}
	}
	return opts
}

func parseSkyrimFixtureCreateOptions(args []string) skyrimFixtureCreateOptions {
	if len(args) == 0 {
		failSkyrim("TSPACK_SKYRIM_FIXTURE_ARGUMENT_INVALID", errors.New("fixture symbolic ID is required"))
	}
	opts := skyrimFixtureCreateOptions{root: ".", fixtureID: args[0]}
	for index := 1; index < len(args); index++ {
		switch args[index] {
		case "--from":
			opts.candidateID = requiredSkyrimFixtureArgument(args, &index, "--from")
		case "--root":
			opts.root = requiredSkyrimFixtureArgument(args, &index, "--root")
		case "--host":
			opts.host = requiredSkyrimFixtureArgument(args, &index, "--host")
		case "--replace":
			opts.replace = true
		case "--dry-run", "--plan-only":
			opts.dryRun = true
		case "--json":
			opts.json = true
		default:
			failSkyrim("TSPACK_SKYRIM_FIXTURE_ARGUMENT_INVALID", fmt.Errorf("unknown fixture-create argument %s", args[index]))
		}
	}
	if !validSkyrimFixtureID(opts.fixtureID) {
		failSkyrim("TSPACK_SKYRIM_FIXTURE_ID_INVALID", errors.New("fixture symbolic ID must contain only lowercase letters, digits, and hyphens"))
	}
	if opts.candidateID == "" {
		failSkyrim("TSPACK_SKYRIM_FIXTURE_SOURCE_REQUIRED", errors.New("--from <candidate-id> is required"))
	}
	return opts
}

func parseSkyrimFixtureInspectOptions(args []string) (string, string, string, bool) {
	if len(args) == 0 || !validSkyrimFixtureID(args[0]) {
		failSkyrim("TSPACK_SKYRIM_FIXTURE_ID_INVALID", errors.New("fixture symbolic ID is required"))
	}
	root := "."
	host := ""
	jsonOut := false
	for index := 1; index < len(args); index++ {
		switch args[index] {
		case "--root":
			root = requiredSkyrimFixtureArgument(args, &index, "--root")
		case "--host":
			host = requiredSkyrimFixtureArgument(args, &index, "--host")
		case "--json":
			jsonOut = true
		default:
			failSkyrim("TSPACK_SKYRIM_FIXTURE_ARGUMENT_INVALID", fmt.Errorf("unknown fixture-inspect argument %s", args[index]))
		}
	}
	return args[0], root, host, jsonOut
}

func requiredSkyrimFixtureArgument(args []string, index *int, flag string) string {
	if *index+1 >= len(args) {
		failSkyrim("TSPACK_SKYRIM_FIXTURE_ARGUMENT_INVALID", fmt.Errorf("%s requires a value", flag))
	}
	*index++
	return args[*index]
}

func runSkyrimSaveList(opts skyrimSaveListOptions, loadManifest ManifestLoader) {
	root, target, profile := loadSkyrimFixtureContext(opts.root, opts.host, loadManifest)
	saveDirectory, source, err := resolveSkyrimSaveDirectory(profile)
	if err != nil {
		failSkyrim("TSPACK_SKYRIM_SAVE_DIRECTORY_UNAVAILABLE", err)
	}
	candidates, err := discoverSkyrimSaveCandidates(saveDirectory)
	if err != nil {
		failSkyrim("TSPACK_SKYRIM_SAVE_DISCOVERY_FAILED", err)
	}
	candidates = filterSkyrimSaveCandidates(candidates, opts)
	report := map[string]any{
		"command":               "tspack skyrim saves list",
		"hostProfile":           target.Target.Host,
		"saveDirectoryResolved": true,
		"saveDirectory":         "<redacted>",
		"saveDirectorySource":   source,
		"candidates":            candidates,
	}
	_ = root
	renderSkyrimFixtureValue(report, opts.json)
}

func runSkyrimFixtureCreate(opts skyrimFixtureCreateOptions, loadManifest ManifestLoader) {
	root, target, profile := loadSkyrimFixtureContext(opts.root, opts.host, loadManifest)
	saveDirectory, source, err := resolveSkyrimSaveDirectory(profile)
	if err != nil {
		failSkyrim("TSPACK_SKYRIM_SAVE_DIRECTORY_UNAVAILABLE", err)
	}
	candidates, err := discoverSkyrimSaveCandidates(saveDirectory)
	if err != nil {
		failSkyrim("TSPACK_SKYRIM_SAVE_DISCOVERY_FAILED", err)
	}
	candidate, found := findSkyrimSaveCandidate(candidates, opts.candidateID)
	if !found {
		failSkyrim("TSPACK_SKYRIM_FIXTURE_SOURCE_STALE", errors.New("candidate ID is unavailable or its source hash changed; run save discovery again"))
	}
	plan, err := planSkyrimFixture(target.Target.Host, saveDirectory, source, profile, opts.fixtureID, candidate)
	if err != nil {
		failSkyrim("TSPACK_SKYRIM_FIXTURE_PLAN_FAILED", err)
	}
	if plan.Action == "replace" && !opts.replace {
		failSkyrim("TSPACK_SKYRIM_FIXTURE_REPLACE_CONFIRMATION_REQUIRED", errors.New("fixture differs from the selected source; rerun with --replace after reviewing --dry-run"))
	}
	if opts.dryRun {
		renderSkyrimFixtureValue(plan, opts.json)
		return
	}
	report, err := createSkyrimFixture(root, target.Target.Host, saveDirectory, profile, plan, candidate)
	if err != nil {
		failSkyrim("TSPACK_SKYRIM_FIXTURE_CREATE_FAILED", err)
	}
	renderSkyrimFixtureValue(report, opts.json)
}

func runSkyrimFixtureInspect(fixtureID string, rootInput string, hostInput string, jsonOut bool, loadManifest ManifestLoader) {
	_, target, profile := loadSkyrimFixtureContext(rootInput, hostInput, loadManifest)
	saveDirectory, _, err := resolveSkyrimSaveDirectory(profile)
	if err != nil {
		failSkyrim("TSPACK_SKYRIM_SAVE_DIRECTORY_UNAVAILABLE", err)
	}
	inspection, err := inspectSkyrimFixture(target.Target.Host, saveDirectory, profile, fixtureID)
	if err != nil {
		failSkyrim("TSPACK_SKYRIM_FIXTURE_INSPECTION_FAILED", err)
	}
	renderSkyrimFixtureValue(inspection, jsonOut)
}

func loadSkyrimFixtureContext(rootInput string, hostInput string, loadManifest ManifestLoader) (string, skyrimTargetRef, skyrimHostProfile) {
	root, err := filepath.Abs(rootInput)
	if err != nil {
		failSkyrim("TSPACK_SKYRIM_ROOT_INVALID", err)
	}
	manifestPath := filepath.Join(root, "manifest.tsx")
	ir := loadManifest(root, manifestPath)
	target := selectSkyrimTarget(ir, root, "skyrim")
	if hostInput != "" && hostInput != target.Target.Host {
		failSkyrim("TSPACK_SKYRIM_HOST_MISMATCH", fmt.Errorf("--host %q does not match manifest host %q", hostInput, target.Target.Host))
	}
	profile, err := loadSkyrimProfile(root, target.Target.Host)
	if err != nil {
		failSkyrim("TSPACK_SKYRIM_PROFILE_INVALID", err)
	}
	return root, target, profile
}

func resolveSkyrimSaveDirectory(profile skyrimHostProfile) (string, string, error) {
	if profile.SaveDirectory != "" {
		if !filepath.IsAbs(profile.SaveDirectory) {
			return "", "", errors.New("host saveDirectory must be an absolute machine-local path")
		}
		info, err := os.Stat(profile.SaveDirectory)
		if err != nil || !info.IsDir() {
			return "", "", fmt.Errorf("configured Skyrim saveDirectory is unavailable")
		}
		return filepath.Clean(profile.SaveDirectory), "profile", nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("derive default Skyrim save directory: %w", err)
	}
	candidates := []string{
		filepath.Join(home, "Documents", "My Games", "Skyrim Special Edition", "Saves"),
		filepath.Join(home, "OneDrive", "Documents", "My Games", "Skyrim Special Edition", "Saves"),
	}
	if oneDrive := os.Getenv("OneDrive"); oneDrive != "" {
		candidates = append(candidates, filepath.Join(oneDrive, "Documents", "My Games", "Skyrim Special Edition", "Saves"))
	}
	found := []string{}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		absolute, absErr := filepath.Abs(candidate)
		if absErr != nil || seen[strings.ToLower(absolute)] {
			continue
		}
		seen[strings.ToLower(absolute)] = true
		entries, readErr := os.ReadDir(absolute)
		if readErr != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".ess") {
				found = append(found, absolute)
				break
			}
		}
	}
	if len(found) != 1 {
		return "", "", errors.New("could not safely derive one active Skyrim save directory; declare saveDirectory in the selected ignored host profile")
	}
	return found[0], "windows-known-folder", nil
}

func discoverSkyrimSaveCandidates(saveDirectory string) ([]skyrimSaveCandidate, error) {
	entries, err := os.ReadDir(saveDirectory)
	if err != nil {
		return nil, err
	}
	candidates := []skyrimSaveCandidate{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".ess") {
			continue
		}
		essPath := filepath.Join(saveDirectory, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		essHash, err := hashFile(essPath)
		if err != nil {
			return nil, fmt.Errorf("hash save candidate: %w", err)
		}
		base := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		sksePath := filepath.Join(saveDirectory, base+".skse")
		skseHash := ""
		if _, err := os.Stat(sksePath); err == nil {
			skseHash, err = hashFile(sksePath)
			if err != nil {
				return nil, fmt.Errorf("hash SKSE sidecar: %w", err)
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		lowerName := strings.ToLower(entry.Name())
		candidate := skyrimSaveCandidate{
			CandidateID:        stableSkyrimSaveCandidateID(entry.Name(), essHash),
			DisplayName:        "save-" + stableShortHash(entry.Name()),
			EssPresent:         true,
			SkseSidecarPresent: skseHash != "",
			SizeBytes:          info.Size(),
			ModifiedTime:       info.ModTime().UTC(),
			SourceHashEss:      essHash,
			SourceHashSkse:     skseHash,
			LikelyAutosave:     strings.HasPrefix(lowerName, "autosave"),
			LikelyQuicksave:    strings.HasPrefix(lowerName, "quicksave"),
			essPath:            essPath,
			sksePath:           sksePath,
		}
		candidate.LikelyManualSave = !candidate.LikelyAutosave && !candidate.LikelyQuicksave
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(left int, right int) bool { return candidates[left].CandidateID < candidates[right].CandidateID })
	return candidates, nil
}

func stableSkyrimSaveCandidateID(filename string, essHash string) string {
	return "skyrim-save-" + stableShortHash(strings.ToLower(filename)+"\x00"+essHash)
}

func stableShortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return strings.ToLower(hex.EncodeToString(sum[:8]))
}

func filterSkyrimSaveCandidates(candidates []skyrimSaveCandidate, opts skyrimSaveListOptions) []skyrimSaveCandidate {
	filtered := []skyrimSaveCandidate{}
	for _, candidate := range candidates {
		if opts.manualOnly && !candidate.LikelyManualSave {
			continue
		}
		if opts.excludeAutosaves && candidate.LikelyAutosave {
			continue
		}
		if opts.excludeQuicksaves && candidate.LikelyQuicksave {
			continue
		}
		if opts.requireSidecar && !candidate.SkseSidecarPresent {
			continue
		}
		if opts.modifiedAfter != nil && !candidate.ModifiedTime.After(*opts.modifiedAfter) {
			continue
		}
		if opts.modifiedBefore != nil && !candidate.ModifiedTime.Before(*opts.modifiedBefore) {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

func findSkyrimSaveCandidate(candidates []skyrimSaveCandidate, candidateID string) (skyrimSaveCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.CandidateID == candidateID {
			return candidate, true
		}
	}
	return skyrimSaveCandidate{}, false
}

func validSkyrimFixtureID(value string) bool {
	if value == "" || len(value) > 48 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

func fixtureFilename(fixtureID string) string {
	return skyrimFixturePrefix + fixtureID + ".ess"
}

func planSkyrimFixture(host string, saveDirectory string, directorySource string, profile skyrimHostProfile, fixtureID string, candidate skyrimSaveCandidate) (skyrimFixturePlan, error) {
	if !validSkyrimFixtureID(fixtureID) {
		return skyrimFixturePlan{}, errors.New("invalid fixture symbolic ID")
	}
	if !candidate.EssPresent {
		return skyrimFixturePlan{}, errors.New("selected candidate has no .ess")
	}
	filename := fixtureFilename(fixtureID)
	fixtureEssPath := filepath.Join(saveDirectory, filename)
	if sameSkyrimFile(candidate.essPath, fixtureEssPath) {
		return skyrimFixturePlan{}, errors.New("source and fixture destination resolve to the same file")
	}
	fixtureSksePath := strings.TrimSuffix(fixtureEssPath, ".ess") + ".skse"
	plan := skyrimFixturePlan{
		Command:             "tspack skyrim fixture create",
		HostProfile:         host,
		SaveDirectory:       "<redacted>",
		SaveDirectorySource: directorySource,
		FixtureSymbolicID:   fixtureID,
		SourceCandidateID:   candidate.CandidateID,
		SourceEss:           snapshotSkyrimFixtureFile(candidate.essPath),
		SourceSkse:          snapshotSkyrimFixtureFileIfPresent(candidate.sksePath, candidate.SkseSidecarPresent),
		FixtureEss:          snapshotSkyrimFixtureFile(fixtureEssPath),
		FixtureSkse:         snapshotSkyrimFixtureFile(fixtureSksePath),
		FixtureFilename:     filename,
		ProfileMapping:      "hosts." + host + ".testSaves." + fixtureID,
	}
	if candidate.SkseSidecarPresent {
		plan.SidecarStatus = "paired"
	} else {
		plan.SidecarStatus = "source-sidecar-missing"
	}
	fixture, mappingExists := profile.TestSaves[fixtureID]
	plan.ProfileUpdateNeeded = !mappingExists || fixture.Filename != filename || fixture.EssSHA256 != candidate.SourceHashEss || fixture.SkseSHA256 != candidate.SourceHashSkse || !fixture.Disposable || !fixture.ReadOnly || fixture.SidecarPresent != candidate.SkseSidecarPresent
	if plan.FixtureEss.Present && plan.FixtureEss.SHA256 == candidate.SourceHashEss && (!candidate.SkseSidecarPresent || (plan.FixtureSkse.Present && plan.FixtureSkse.SHA256 == candidate.SourceHashSkse)) && (candidate.SkseSidecarPresent || !plan.FixtureSkse.Present) {
		plan.Action = "unchanged"
	} else if plan.FixtureEss.Present || plan.FixtureSkse.Present || mappingExists {
		plan.Action = "replace"
	} else {
		plan.Action = "create"
	}
	return plan, nil
}

func snapshotSkyrimFixtureFile(path string) skyrimFixtureFileSnapshot {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return skyrimFixtureFileSnapshot{}
	}
	hash, err := hashFile(path)
	if err != nil {
		return skyrimFixtureFileSnapshot{}
	}
	return skyrimFixtureFileSnapshot{Present: true, SHA256: hash, SizeBytes: info.Size(), ModifiedTime: info.ModTime().UTC()}
}

func snapshotSkyrimFixtureFileIfPresent(path string, present bool) skyrimFixtureFileSnapshot {
	if !present {
		return skyrimFixtureFileSnapshot{}
	}
	return snapshotSkyrimFixtureFile(path)
}

func sameSkyrimFile(left string, right string) bool {
	leftAbsolute, leftErr := filepath.Abs(left)
	rightAbsolute, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(leftAbsolute), filepath.Clean(rightAbsolute))
}

func createSkyrimFixture(root string, host string, saveDirectory string, profile skyrimHostProfile, plan skyrimFixturePlan, candidate skyrimSaveCandidate) (skyrimFixtureReport, error) {
	beforeEss := snapshotSkyrimFixtureFile(candidate.essPath)
	beforeSkse := snapshotSkyrimFixtureFileIfPresent(candidate.sksePath, candidate.SkseSidecarPresent)
	if beforeEss.SHA256 != candidate.SourceHashEss || (candidate.SkseSidecarPresent && beforeSkse.SHA256 != candidate.SourceHashSkse) {
		return skyrimFixtureReport{}, errors.New("source changed after discovery; run save discovery again")
	}
	stagedEss, err := stageSkyrimFixtureFile(saveDirectory, candidate.essPath, candidate.SourceHashEss)
	if err != nil {
		return skyrimFixtureReport{}, err
	}
	defer os.Remove(stagedEss)
	stagedSkse := ""
	if candidate.SkseSidecarPresent {
		stagedSkse, err = stageSkyrimFixtureFile(saveDirectory, candidate.sksePath, candidate.SourceHashSkse)
		if err != nil {
			return skyrimFixtureReport{}, err
		}
		defer os.Remove(stagedSkse)
	}
	if plan.Action != "unchanged" {
		if err := commitSkyrimFixturePair(saveDirectory, plan, stagedEss, stagedSkse, candidate.SkseSidecarPresent); err != nil {
			return skyrimFixtureReport{}, err
		}
	}
	fixtureEssPath := filepath.Join(saveDirectory, plan.FixtureFilename)
	fixtureSksePath := strings.TrimSuffix(fixtureEssPath, ".ess") + ".skse"
	fixtureEss := snapshotSkyrimFixtureFile(fixtureEssPath)
	fixtureSkse := snapshotSkyrimFixtureFile(fixtureSksePath)
	if fixtureEss.SHA256 != candidate.SourceHashEss || (candidate.SkseSidecarPresent && fixtureSkse.SHA256 != candidate.SourceHashSkse) || (!candidate.SkseSidecarPresent && fixtureSkse.Present) {
		return skyrimFixtureReport{}, errors.New("fixture verification failed")
	}
	updated := profile
	if updated.TestSaves == nil {
		updated.TestSaves = map[string]skyrimTestSave{}
	}
	updated.SaveDirectory = saveDirectory
	updated.TestSaves[plan.FixtureSymbolicID] = skyrimTestSave{
		Filename: plan.FixtureFilename, Disposable: true, ReadOnly: true, SidecarPresent: candidate.SkseSidecarPresent,
		EssSHA256: fixtureEss.SHA256, SkseSHA256: fixtureSkse.SHA256,
		SourceCandidateID: candidate.CandidateID, SourceEssSHA256: candidate.SourceHashEss, SourceSkseSHA256: candidate.SourceHashSkse,
	}
	if err := writeSkyrimHostProfile(root, host, updated); err != nil {
		return skyrimFixtureReport{}, err
	}
	afterEss := snapshotSkyrimFixtureFile(candidate.essPath)
	afterSkse := snapshotSkyrimFixtureFileIfPresent(candidate.sksePath, candidate.SkseSidecarPresent)
	if !sameSkyrimFixtureSnapshot(beforeEss, afterEss) || !sameSkyrimFixtureSnapshot(beforeSkse, afterSkse) {
		return skyrimFixtureReport{}, errors.New("source immutability verification failed")
	}
	report := skyrimFixtureReport{skyrimFixturePlan: plan, SourceEssAfter: afterEss, SourceSkseAfter: afterSkse, FixtureProfileMappingWritten: true, RollbackStatus: "not-needed"}
	return report, nil
}

func stageSkyrimFixtureFile(directory string, source string, expectedHash string) (string, error) {
	temp, err := os.CreateTemp(directory, ".tspack-skyrim-fixture-*.tmp")
	if err != nil {
		return "", err
	}
	path := temp.Name()
	if err := temp.Close(); err != nil {
		return "", err
	}
	if err := copySkyrimFile(source, path); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	hash, err := hashFile(path)
	if err != nil || hash != expectedHash {
		_ = os.Remove(path)
		return "", errors.New("staged fixture hash verification failed")
	}
	return path, nil
}

func commitSkyrimFixturePair(directory string, plan skyrimFixturePlan, stagedEss string, stagedSkse string, sourceHasSidecar bool) error {
	fixtureEss := filepath.Join(directory, plan.FixtureFilename)
	fixtureSkse := strings.TrimSuffix(fixtureEss, ".ess") + ".skse"
	backupDirectory, err := os.MkdirTemp(directory, ".tspack-skyrim-fixture-rollback-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(backupDirectory)
	type backup struct {
		destination string
		backup      string
		existed     bool
	}
	backups := []backup{}
	rollback := func() {
		for index := len(backups) - 1; index >= 0; index-- {
			item := backups[index]
			if item.existed {
				_ = copyAndReplaceFile(item.backup, item.destination)
			} else {
				_ = os.Remove(item.destination)
			}
		}
	}
	apply := func(source string, destination string, label string) error {
		backupPath := filepath.Join(backupDirectory, label)
		_, statErr := os.Stat(destination)
		existed := statErr == nil
		if existed && copySkyrimFile(destination, backupPath) != nil {
			return errors.New("fixture backup failed")
		}
		if err := copyAndReplaceFile(source, destination); err != nil {
			return err
		}
		backups = append(backups, backup{destination: destination, backup: backupPath, existed: existed})
		return nil
	}
	if err := apply(stagedEss, fixtureEss, "fixture.ess"); err != nil {
		rollback()
		return err
	}
	if sourceHasSidecar {
		if err := apply(stagedSkse, fixtureSkse, "fixture.skse"); err != nil {
			rollback()
			return err
		}
	}
	return nil
}

func sameSkyrimFixtureSnapshot(left skyrimFixtureFileSnapshot, right skyrimFixtureFileSnapshot) bool {
	return left.Present == right.Present && left.SHA256 == right.SHA256 && left.SizeBytes == right.SizeBytes && left.ModifiedTime.Equal(right.ModifiedTime)
}

func writeSkyrimHostProfile(root string, host string, profile skyrimHostProfile) error {
	profilePath := filepath.Join(root, filepath.FromSlash(skyrimProfileRelativePath))
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return err
	}
	var profiles skyrimProfilesFile
	if err := toml.Unmarshal(data, &profiles); err != nil {
		return err
	}
	if profiles.Hosts == nil {
		return errors.New("host profile has no hosts table")
	}
	if _, found := profiles.Hosts[host]; !found {
		return errors.New("selected host profile disappeared")
	}
	profiles.Hosts[host] = profile
	encoded, err := toml.Marshal(profiles)
	if err != nil {
		return err
	}
	return copyBytesAndReplaceFile(encoded, profilePath)
}

func copyBytesAndReplaceFile(data []byte, destination string) error {
	temp, err := os.CreateTemp(filepath.Dir(destination), ".tspack-skyrim-profile-*.tmp")
	if err != nil {
		return err
	}
	path := temp.Name()
	keep := true
	defer func() {
		_ = temp.Close()
		if keep {
			_ = os.Remove(path)
		}
	}()
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := replaceFileAtomic(path, destination); err != nil {
		return err
	}
	keep = false
	return nil
}

func inspectSkyrimFixture(host string, saveDirectory string, profile skyrimHostProfile, fixtureID string) (skyrimFixtureInspection, error) {
	inspection := skyrimFixtureInspection{HostProfile: host, FixtureSymbolicID: fixtureID}
	fixture, found := profile.TestSaves[fixtureID]
	if !found {
		return inspection, nil
	}
	inspection.MappingExists = true
	inspection.Disposable = fixture.Disposable
	inspection.ReadOnly = fixture.ReadOnly
	if !fixture.Disposable || !fixture.ReadOnly || filepath.Base(fixture.Filename) != fixture.Filename || !strings.EqualFold(filepath.Ext(fixture.Filename), ".ess") {
		return inspection, errors.New("fixture profile mapping is unsafe")
	}
	essPath := filepath.Join(saveDirectory, fixture.Filename)
	sksePath := strings.TrimSuffix(essPath, ".ess") + ".skse"
	inspection.Ess = snapshotSkyrimFixtureFile(essPath)
	inspection.Skse = snapshotSkyrimFixtureFile(sksePath)
	inspection.HashesMatch = inspection.Ess.Present && inspection.Ess.SHA256 == fixture.EssSHA256 && inspection.Skse.Present == fixture.SidecarPresent && (!fixture.SidecarPresent || inspection.Skse.SHA256 == fixture.SkseSHA256)
	inspection.SourceProvenanceKnown = fixture.SourceCandidateID != "" && fixture.SourceEssSHA256 != "" && (!fixture.SidecarPresent || fixture.SourceSkseSHA256 != "")
	return inspection, nil
}

func renderSkyrimFixtureValue(value any, jsonOut bool) {
	if jsonOut {
		data, _ := json.MarshalIndent(value, "", "  ")
		fmt.Println(string(data))
		return
	}
	data, _ := json.MarshalIndent(value, "", "  ")
	fmt.Println(string(data))
}
