package perf

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPerfSessionSnapshotDeterministicJSON(t *testing.T) {
	var stderr bytes.Buffer
	dir := t.TempDir()
	cfg := EnvConfig{
		Enabled:        true,
		EmitText:       true,
		JSONPath:       filepath.Join(dir, "perf.json"),
		CPUProfilePath: filepath.Join(dir, "cpu.pprof"),
		MemProfilePath: filepath.Join(dir, "mem.pprof"),
	}
	session, err := NewSession("update", "C:/repo", true, cfg, &stderr)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	session.RecordMetadataRequest()
	session.RecordMetadataCacheHit()
	session.RecordTarballRequest()
	session.RecordHTTPRequest("metadata", "registry.example.test", 200)
	session.RecordHTTPRequest("tarball", "registry.example.test", 200)
	stop := session.StartPhase("update.resolve")
	time.Sleep(5 * time.Millisecond)
	stop()
	if err := session.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	body, err := os.ReadFile(cfg.JSONPath)
	if err != nil {
		t.Fatalf("read json: %v", err)
	}
	var report Report
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if report.Command != "update" {
		t.Fatalf("command=%q want update", report.Command)
	}
	if report.Counters.MetadataRequests != 1 || report.Counters.MetadataCacheHits != 1 || report.Counters.TarballRequests != 1 {
		t.Fatalf("unexpected counters: %#v", report.Counters)
	}
	if len(report.Phases) != 1 || report.Phases[0].Name != "update.resolve" {
		t.Fatalf("unexpected phases: %#v", report.Phases)
	}
	if !strings.Contains(stderr.String(), "tspack perf: command=update") {
		t.Fatalf("expected text summary, got %q", stderr.String())
	}
	if _, err := os.Stat(cfg.CPUProfilePath); err != nil {
		t.Fatalf("cpu profile missing: %v", err)
	}
	if _, err := os.Stat(cfg.MemProfilePath); err != nil {
		t.Fatalf("mem profile missing: %v", err)
	}
}

func TestPerfSessionConcurrentCountersSafe(t *testing.T) {
	session, err := NewSession("sync", "C:/repo", false, EnvConfig{Enabled: true}, nil)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				session.RecordMetadataRequest()
				session.RecordTarballRequest()
				session.RecordMaterializedFile(10)
				session.RecordHardlink(10)
			}
		}()
	}
	wg.Wait()
	report := session.Snapshot(time.Now().UTC())
	if report.Counters.MetadataRequests != 3200 {
		t.Fatalf("metadata requests=%d want 3200", report.Counters.MetadataRequests)
	}
	if report.Counters.TarballRequests != 3200 {
		t.Fatalf("tarball requests=%d want 3200", report.Counters.TarballRequests)
	}
	if report.Counters.MaterializedFiles != 3200 {
		t.Fatalf("materialized files=%d want 3200", report.Counters.MaterializedFiles)
	}
	if report.Counters.HardlinkCount != 3200 {
		t.Fatalf("hardlink count=%d want 3200", report.Counters.HardlinkCount)
	}
	if report.Counters.LogicalBytesMaterialized != 32000 {
		t.Fatalf("logical bytes=%d want 32000", report.Counters.LogicalBytesMaterialized)
	}
}
