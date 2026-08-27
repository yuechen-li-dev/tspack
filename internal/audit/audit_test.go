package audit

import (
	"context"
	"errors"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/lockfile"
)

type fakeClient struct {
	queryResults []QueryResult
	records      map[string]Vulnerability
	err          error
}

func (f *fakeClient) QueryBatch(_ context.Context, _ []Query) ([]QueryResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.queryResults, nil
}

func (f *fakeClient) Get(_ context.Context, id string) (Vulnerability, error) {
	record, ok := f.records[id]
	if !ok {
		return Vulnerability{}, errors.New("missing record")
	}
	return record, nil
}

func TestScanUsesLockedNPMVersionsAndReportsPaths(t *testing.T) {
	lf := &lockfile.Lockfile{
		Packages: []lockfile.Package{{ID: "npm:demo@1.0.0", Name: "demo", Version: "1.0.0", Source: "npm"}},
		Edges:    []lockfile.Edge{{From: "app:target:web", To: "npm:demo@1.0.0", Kind: "runtime"}},
	}
	record := Vulnerability{ID: "GHSA-test", Summary: "test advisory", DatabaseSpecific: map[string]any{"severity": "HIGH"}}
	record.Aliases = []string{"CVE-TEST"}
	record.Affected = []Affected{{}}
	record.Affected[0].Package.Name = "demo"
	record.Affected[0].Ranges = []Range{{Events: []Event{{Fixed: "1.0.1"}}}}
	client := &fakeClient{queryResults: []QueryResult{{IDs: []string{"GHSA-test"}}}, records: map[string]Vulnerability{"GHSA-test": record}}

	report, err := Scan(context.Background(), lf, client)
	if err != nil {
		t.Fatal(err)
	}
	if report.Packages != 1 || len(report.Findings) != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
	finding := report.Findings[0]
	if finding.Severity != SeverityHigh || len(finding.Fixed) != 1 || finding.Fixed[0] != "1.0.1" {
		t.Fatalf("unexpected finding: %#v", finding)
	}
	if len(finding.Paths) != 1 || len(finding.Paths[0]) != 2 {
		t.Fatalf("missing dependency path: %#v", finding.Paths)
	}
}

func TestThresholds(t *testing.T) {
	threshold, err := ParseThreshold("high")
	if err != nil {
		t.Fatal(err)
	}
	if FailsThreshold(SeverityModerate, threshold) || !FailsThreshold(SeverityHigh, threshold) {
		t.Fatal("high threshold classification failed")
	}
	any, _ := ParseThreshold("any")
	if !FailsThreshold(SeverityUnknown, any) {
		t.Fatal("default threshold must fail unknown findings")
	}
	if FailsThreshold(SeverityUnknown, threshold) {
		t.Fatal("unknown severity must not meet an explicit high threshold")
	}
}

func TestScanFailsClosedWhenAdvisoryServiceFails(t *testing.T) {
	lf := &lockfile.Lockfile{Packages: []lockfile.Package{{ID: "npm:demo@1.0.0", Name: "demo", Version: "1.0.0", Source: "npm"}}}
	_, err := Scan(context.Background(), lf, &fakeClient{err: errors.New("offline")})
	if err == nil {
		t.Fatal("expected service error")
	}
}

func TestScanReportsJSRCoverageAsNotChecked(t *testing.T) {
	lf := &lockfile.Lockfile{Packages: []lockfile.Package{
		{ID: "npm:demo@1.0.0", Name: "demo", Version: "1.0.0", Source: "npm"},
		{ID: "jsr:@std/path@1.1.6", Name: "@std/path", Version: "1.1.6", Source: "jsr"},
	}}
	report, err := Scan(context.Background(), lf, &fakeClient{queryResults: []QueryResult{{}}, records: map[string]Vulnerability{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Coverage) != 2 || report.Coverage[1].Source != "jsr" || report.Coverage[1].Status != "not-checked" {
		t.Fatalf("JSR audit coverage was not explicit: %#v", report.Coverage)
	}
}
