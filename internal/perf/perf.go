package perf

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"time"
)

type EnvConfig struct {
	Enabled        bool
	EmitText       bool
	JSONPath       string
	CPUProfilePath string
	MemProfilePath string
}

type Session struct {
	mu         sync.Mutex
	command    string
	rootDir    string
	dryRun     bool
	startedAt  time.Time
	closed     bool
	textWriter io.Writer
	config     EnvConfig
	cpuFile    *os.File
	phases     map[string]time.Duration
	counters   Counters
	httpKinds  map[string]int64
	httpHosts  map[string]int64
	httpStatus map[string]int64
}

type Counters struct {
	ResolveJobs                     int64 `json:"resolveJobs"`
	ResolveFrontiers                int64 `json:"resolveFrontiers"`
	ResolveMaxFrontierWidth         int64 `json:"resolveMaxFrontierWidth"`
	ResolvePreparedPackages         int64 `json:"resolvePreparedPackages"`
	ResolveCommittedPackages        int64 `json:"resolveCommittedPackages"`
	ResolveWorkerErrors             int64 `json:"resolveWorkerErrors"`
	MetadataRequests                int64 `json:"metadataRequests"`
	MetadataCacheHits               int64 `json:"metadataCacheHits"`
	TarballRequests                 int64 `json:"tarballRequests"`
	ArtifactsCaptured               int64 `json:"artifactsCaptured"`
	ArtifactsAlreadyInStore         int64 `json:"artifactsAlreadyInStore"`
	ArtifactsNeedingStorePopulation int64 `json:"artifactsNeedingStorePopulation"`
	StorePopulationSkipped          int64 `json:"storePopulationSkipped"`
	StorePopulationFetched          int64 `json:"storePopulationFetched"`
	SyncHydrationSkipped            int64 `json:"syncHydrationSkipped"`
	SyncHydrationFetched            int64 `json:"syncHydrationFetched"`
	MaterializedPackages            int64 `json:"materializedPackages"`
	MaterializedFiles               int64 `json:"materializedFiles"`
	MaterializedDirectories         int64 `json:"materializedDirectories"`
	HardlinkCount                   int64 `json:"hardlinkCount"`
	CopyFallbackCount               int64 `json:"copyFallbackCount"`
	LogicalBytesMaterialized        int64 `json:"logicalBytesMaterialized"`
	BytesCopied                     int64 `json:"bytesCopied"`
}

type Report struct {
	Command    string        `json:"command"`
	RootDir    string        `json:"rootDir"`
	DryRun     bool          `json:"dryRun,omitempty"`
	StartedAt  string        `json:"startedAt"`
	FinishedAt string        `json:"finishedAt"`
	TotalMs    int64         `json:"totalMs"`
	Phases     []PhaseReport `json:"phases,omitempty"`
	Counters   Counters      `json:"counters"`
	HTTP       HTTPReport    `json:"http,omitempty"`
	Profiles   ProfileReport `json:"profiles,omitempty"`
}

type PhaseReport struct {
	Name       string `json:"name"`
	DurationMs int64  `json:"durationMs"`
}

type HTTPReport struct {
	RequestKinds map[string]int64 `json:"requestKinds,omitempty"`
	Hosts        map[string]int64 `json:"hosts,omitempty"`
	StatusCodes  map[string]int64 `json:"statusCodes,omitempty"`
}

type ProfileReport struct {
	CPUProfile string `json:"cpuProfile,omitempty"`
	MemProfile string `json:"memProfile,omitempty"`
}

func ConfigFromEnv() EnvConfig {
	cfg := EnvConfig{
		JSONPath:       strings.TrimSpace(os.Getenv("TSPACK_TRACE_PERF_JSON")),
		CPUProfilePath: strings.TrimSpace(os.Getenv("TSPACK_CPU_PROFILE")),
		MemProfilePath: strings.TrimSpace(os.Getenv("TSPACK_MEM_PROFILE")),
	}
	traceValue := strings.TrimSpace(os.Getenv("TSPACK_TRACE_PERF"))
	cfg.EmitText = traceValue == "1" || strings.EqualFold(traceValue, "true")
	cfg.Enabled = cfg.EmitText || cfg.JSONPath != "" || cfg.CPUProfilePath != "" || cfg.MemProfilePath != ""
	return cfg
}

func NewSession(command string, rootDir string, dryRun bool, cfg EnvConfig, textWriter io.Writer) (*Session, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	s := &Session{
		command:    command,
		rootDir:    rootDir,
		dryRun:     dryRun,
		startedAt:  time.Now().UTC(),
		textWriter: textWriter,
		config:     cfg,
		phases:     map[string]time.Duration{},
		httpKinds:  map[string]int64{},
		httpHosts:  map[string]int64{},
		httpStatus: map[string]int64{},
	}
	if cfg.CPUProfilePath != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.CPUProfilePath), 0o755); err != nil {
			return nil, fmt.Errorf("create cpu profile dir: %w", err)
		}
		file, err := os.Create(cfg.CPUProfilePath)
		if err != nil {
			return nil, fmt.Errorf("create cpu profile: %w", err)
		}
		if err := pprof.StartCPUProfile(file); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("start cpu profile: %w", err)
		}
		s.cpuFile = file
	}
	return s, nil
}

func (s *Session) Enabled() bool {
	return s != nil
}

func (s *Session) StartPhase(name string) func() {
	if s == nil {
		return func() {}
	}
	started := time.Now()
	return func() {
		s.AddPhaseDuration(name, time.Since(started))
	}
}

func (s *Session) AddPhaseDuration(name string, duration time.Duration) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phases[name] += duration
}

func (s *Session) RecordMetadataRequest() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters.MetadataRequests++
}

func (s *Session) SetResolveJobs(jobs int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters.ResolveJobs = int64(jobs)
}

func (s *Session) RecordResolveFrontier(width int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters.ResolveFrontiers++
	if int64(width) > s.counters.ResolveMaxFrontierWidth {
		s.counters.ResolveMaxFrontierWidth = int64(width)
	}
}

func (s *Session) RecordPreparedPackage() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters.ResolvePreparedPackages++
}

func (s *Session) RecordCommittedPackage() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters.ResolveCommittedPackages++
}

func (s *Session) RecordResolveWorkerError() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters.ResolveWorkerErrors++
}

func (s *Session) RecordMetadataCacheHit() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters.MetadataCacheHits++
}

func (s *Session) RecordTarballRequest() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters.TarballRequests++
}

func (s *Session) RecordArtifactCaptured() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters.ArtifactsCaptured++
}

func (s *Session) RecordArtifactAlreadyInStore() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters.ArtifactsAlreadyInStore++
}

func (s *Session) SetStorePopulationCounts(needed int, skipped int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters.ArtifactsNeedingStorePopulation = int64(needed)
	s.counters.StorePopulationSkipped = int64(skipped)
}

func (s *Session) RecordStorePopulationFetch() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters.StorePopulationFetched++
}

func (s *Session) RecordSyncHydrationSkip() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters.SyncHydrationSkipped++
}

func (s *Session) RecordSyncHydrationFetch() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters.SyncHydrationFetched++
}

func (s *Session) RecordMaterializedPackage() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters.MaterializedPackages++
}

func (s *Session) RecordMaterializedDirectory() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters.MaterializedDirectories++
}

func (s *Session) RecordMaterializedFile(size int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters.MaterializedFiles++
	s.counters.LogicalBytesMaterialized += size
}

func (s *Session) RecordHardlink(size int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters.HardlinkCount++
}

func (s *Session) RecordCopy(size int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters.CopyFallbackCount++
	s.counters.BytesCopied += size
}

func (s *Session) RecordHTTPRequest(kind string, host string, status int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if kind != "" {
		s.httpKinds[kind]++
	}
	if host != "" {
		s.httpHosts[host]++
	}
	if status > 0 {
		s.httpStatus[fmt.Sprintf("%d", status)]++
	}
}

func (s *Session) Snapshot(finishedAt time.Time) Report {
	if s == nil {
		return Report{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked(finishedAt)
}

func (s *Session) snapshotLocked(finishedAt time.Time) Report {
	phases := make([]PhaseReport, 0, len(s.phases))
	for name, duration := range s.phases {
		phases = append(phases, PhaseReport{Name: name, DurationMs: duration.Milliseconds()})
	}
	sort.SliceStable(phases, func(i, j int) bool {
		return phases[i].Name < phases[j].Name
	})
	report := Report{
		Command:    s.command,
		RootDir:    s.rootDir,
		DryRun:     s.dryRun,
		StartedAt:  s.startedAt.Format(time.RFC3339Nano),
		FinishedAt: finishedAt.UTC().Format(time.RFC3339Nano),
		TotalMs:    finishedAt.Sub(s.startedAt).Milliseconds(),
		Phases:     phases,
		Counters:   s.counters,
	}
	if len(s.httpKinds) > 0 || len(s.httpHosts) > 0 || len(s.httpStatus) > 0 {
		report.HTTP = HTTPReport{
			RequestKinds: cloneMap(s.httpKinds),
			Hosts:        cloneMap(s.httpHosts),
			StatusCodes:  cloneMap(s.httpStatus),
		}
	}
	if s.config.CPUProfilePath != "" || s.config.MemProfilePath != "" {
		report.Profiles = ProfileReport{
			CPUProfile: s.config.CPUProfilePath,
			MemProfile: s.config.MemProfilePath,
		}
	}
	return report
}

func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	if s.cpuFile != nil {
		pprof.StopCPUProfile()
		if err := s.cpuFile.Close(); err != nil {
			return err
		}
	}
	if s.config.MemProfilePath != "" {
		if err := os.MkdirAll(filepath.Dir(s.config.MemProfilePath), 0o755); err != nil {
			return err
		}
		file, err := os.Create(s.config.MemProfilePath)
		if err != nil {
			return err
		}
		runtime.GC()
		if err := pprof.WriteHeapProfile(file); err != nil {
			_ = file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}

	finishedAt := time.Now().UTC()
	report := s.Snapshot(finishedAt)
	if s.config.JSONPath != "" {
		if err := writeReport(s.config.JSONPath, report); err != nil {
			return err
		}
	}
	if s.config.EmitText && s.textWriter != nil {
		if err := writeTextSummary(s.textWriter, report); err != nil {
			return err
		}
	}
	return nil
}

func writeReport(path string, report Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

func writeTextSummary(w io.Writer, report Report) error {
	if _, err := fmt.Fprintf(w, "tspack perf: command=%s total=%dms metadata=%d tarballs=%d hardlinks=%d copies=%d\n",
		report.Command,
		report.TotalMs,
		report.Counters.MetadataRequests,
		report.Counters.TarballRequests,
		report.Counters.HardlinkCount,
		report.Counters.CopyFallbackCount,
	); err != nil {
		return err
	}
	for _, phase := range report.Phases {
		if _, err := fmt.Fprintf(w, "  %s=%dms\n", phase.Name, phase.DurationMs); err != nil {
			return err
		}
	}
	if report.Profiles.CPUProfile != "" || report.Profiles.MemProfile != "" {
		if _, err := fmt.Fprintf(w, "  profiles: cpu=%s mem=%s\n", report.Profiles.CPUProfile, report.Profiles.MemProfile); err != nil {
			return err
		}
	}
	return nil
}

func cloneMap(input map[string]int64) map[string]int64 {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]int64, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
