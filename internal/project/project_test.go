package project

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
	"github.com/yuechen-li-dev/tspack/internal/resolver"
	"github.com/yuechen-li-dev/tspack/internal/store"
)

type fakeClient struct {
	mu               sync.Mutex
	meta             map[string]*resolver.PackageMetadata
	metaErr          map[string]error
	tar              map[string][]byte
	metaCalls        []string
	tarCalls         []string
	packageMetaCalls int
}

func (f *fakeClient) PackageMetadata(_ context.Context, name string) (*resolver.PackageMetadata, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.metaCalls = append(f.metaCalls, name)
	f.packageMetaCalls++
	if f.metaErr != nil {
		if err, ok := f.metaErr[name]; ok {
			return nil, err
		}
	}
	m, ok := f.meta[name]
	if !ok {
		return nil, errors.New("not found")
	}
	return m, nil
}
func (f *fakeClient) Tarball(_ context.Context, url string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tarCalls = append(f.tarCalls, url)
	b, ok := f.tar[url]
	if !ok {
		return nil, errors.New("not found")
	}
	return b, nil
}

func TestCheckDoesNotMutateLockfile(t *testing.T) {
	dir := t.TempDir()
	irPath := writeIR(t, dir, simpleIR())
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"}, Targets: []lockfile.Target{{Package: "app", Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts"}}}
	b, _ := lockfile.Marshal(lf)
	lockPath := filepath.Join(dir, "ts-lock.toml")
	_ = os.WriteFile(lockPath, b, 0o644)
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = irPath
	res := Check(opts)
	if hasErrCode(res.Diagnostics, "TSPACK_CHECK_FAILED") {
		t.Fatalf("unexpected failure: %#v", res.Diagnostics)
	}
	after, _ := os.ReadFile(lockPath)
	if !bytes.Equal(b, after) {
		t.Fatalf("lockfile mutated")
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("node_modules should not exist")
	}
}

func TestCheckMissingLockfileWarning(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, simpleIR())
	res := Check(opts)
	if !hasErrCode(res.Diagnostics, "TSPACK_CHECK_LOCKFILE_MISSING") {
		t.Fatalf("missing warning")
	}
	if _, err := os.Stat(filepath.Join(dir, "ts-lock.toml")); !os.IsNotExist(err) {
		t.Fatalf("check created lockfile")
	}
}

func TestUpdateDeterministicAndNoNodeModules(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	opts.ResolverClient = buildRegistry()
	r1 := Update(opts)
	if hasErrors(r1.Diagnostics) {
		t.Fatalf("update failed: %#v", r1.Diagnostics)
	}
	b1, _ := os.ReadFile(opts.LockfilePath)
	r2 := Update(opts)
	if hasErrors(r2.Diagnostics) {
		t.Fatalf("update2 failed: %#v", r2.Diagnostics)
	}
	b2, _ := os.ReadFile(opts.LockfilePath)
	if !bytes.Equal(b1, b2) {
		t.Fatalf("nondeterministic lockfile")
	}
	if r2.LockDiff == nil || len(r2.LockDiff.PackagesAdded)+len(r2.LockDiff.PackagesRemoved)+len(r2.LockDiff.PackagesChanged) != 0 {
		t.Fatalf("expected empty package diff")
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("update created node_modules")
	}
}

func TestUpdateDryRunExistingLockNoChangesLeavesBytesUntouched(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, simpleIR())
	opts.ResolverClient = buildRegistry()
	first := Update(opts)
	if hasErrors(first.Diagnostics) {
		t.Fatalf("initial update failed: %#v", first.Diagnostics)
	}
	before, _ := os.ReadFile(opts.LockfilePath)
	dry := UpdateDryRun(opts)
	if hasErrors(dry.Diagnostics) {
		t.Fatalf("dry-run failed: %#v", dry.Diagnostics)
	}
	after, _ := os.ReadFile(opts.LockfilePath)
	if !bytes.Equal(before, after) {
		t.Fatalf("dry-run mutated lockfile")
	}
	if dry.LockDiff == nil || len(dry.LockDiff.PackagesAdded)+len(dry.LockDiff.PackagesRemoved)+len(dry.LockDiff.PackagesChanged) != 0 {
		t.Fatalf("expected no diff for dry-run")
	}
}

func TestSyncMutationGuardAndMaterialization(t *testing.T) {
	dir := t.TempDir()
	irPath := writeIR(t, dir, simpleIR())
	st, _ := store.Open(filepath.Join(dir, ".tspack", "store"))
	aHash := putPkg(t, st, "dep-a", "1.0.0")
	lHash := putPkg(t, st, "left-pad", "1.2.0")
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Packages: []lockfile.Package{{ID: "npm:dep-a@1.0.0", Name: "dep-a", Version: "1.0.0", Source: "npm", Hash: aHash}, {ID: "npm:left-pad@1.2.0", Name: "left-pad", Version: "1.2.0", Source: "npm", Hash: lHash}},
		Edges:    []lockfile.Edge{{From: "app:target:core", To: "npm:dep-a@1.0.0", Kind: "runtime"}, {From: "npm:dep-a@1.0.0", To: "npm:left-pad@1.2.0", Kind: "runtime"}},
		Targets:  []lockfile.Target{{Package: "app", Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts"}},
	}
	lb, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(filepath.Join(dir, "ts-lock.toml"), lb, 0o644)
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = irPath
	res := Sync(opts, false)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("sync failed: %#v", res.Diagnostics)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "ts-lock.toml"))
	if !bytes.Equal(lb, after) {
		t.Fatalf("lock mutated")
	}
	mustExist(t, filepath.Join(dir, "node_modules", "dep-a", "package.json"))
	mustExist(t, filepath.Join(dir, "node_modules", "dep-a", "node_modules", "left-pad", "package.json"))
	if _, err := os.Stat(filepath.Join(dir, "node_modules", "left-pad")); !os.IsNotExist(err) {
		t.Fatalf("phantom root transitive present")
	}
}

func TestCheckWarnsOnLifecycleCapability(t *testing.T) {
	dir := t.TempDir()
	irPath := writeIR(t, dir, simpleIR())
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Packages: []lockfile.Package{{ID: "npm:dep-a@1.0.0", Name: "dep-a", Version: "1.0.0", Source: "npm", Integrity: "x", Capabilities: []lockfile.Capability{{Kind: "lifecycleScript", Script: "postinstall", Command: "node install.js"}}}},
		Targets:  []lockfile.Target{{Package: "app", Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts"}},
	}
	b, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(filepath.Join(dir, "ts-lock.toml"), b, 0o644)
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = irPath
	res := Check(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("check failed: %#v", res.Diagnostics)
	}
	diagnostic := findDiagnostic(res.Diagnostics, "TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT")
	if diagnostic == nil {
		t.Fatalf("expected lifecycle capability warning")
	}
	details := strings.Join(diagnostic.Details, "\n")
	for _, expected := range []string{"package: npm:dep-a@1.0.0", "script: postinstall", "command: node install.js", "execution: blocked by default"} {
		if !strings.Contains(details, expected) {
			t.Fatalf("missing detail %q in %#v", expected, diagnostic.Details)
		}
	}
	after, err := os.ReadFile(filepath.Join(dir, "ts-lock.toml"))
	if err != nil {
		t.Fatalf("read lockfile after check: %v", err)
	}
	if !bytes.Equal(b, after) {
		t.Fatalf("check mutated lockfile")
	}
}

func TestCheckAcknowledgedLifecycleCapabilitySuppressesDefaultWarning(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	ir["security"] = map[string]any{
		"acknowledgedCapabilities": []map[string]any{{
			"package": "npm:dep-a@1.0.0",
			"kind":    "lifecycleScript",
			"script":  "postinstall",
			"command": "node install.js",
			"reason":  "Known package lifecycle script; execution remains blocked by TSPack.",
		}},
	}
	irPath := writeIR(t, dir, ir)
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Packages: []lockfile.Package{{ID: "npm:dep-a@1.0.0", Name: "dep-a", Version: "1.0.0", Source: "npm", Integrity: "x", Capabilities: []lockfile.Capability{{Kind: "lifecycleScript", Script: "postinstall", Command: "node install.js"}}}},
		Targets:  []lockfile.Target{{Package: "app", Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts"}},
	}
	b, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(filepath.Join(dir, "ts-lock.toml"), b, 0o644)

	res := Check(DefaultOptionsWithIR(dir, irPath))
	if hasErrors(res.Diagnostics) {
		t.Fatalf("check failed: %#v", res.Diagnostics)
	}
	if hasErrCode(res.Diagnostics, "TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT") {
		t.Fatalf("acknowledged lifecycle capability should not emit default warning: %#v", res.Diagnostics)
	}
	after, err := os.ReadFile(filepath.Join(dir, "ts-lock.toml"))
	if err != nil {
		t.Fatalf("read lockfile after check: %v", err)
	}
	if !bytes.Equal(b, after) {
		t.Fatalf("check mutated lockfile")
	}
}

func TestCheckLifecycleAcknowledgementDriftAndUnused(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	ir["security"] = map[string]any{
		"acknowledgedCapabilities": []map[string]any{{
			"package": "npm:dep-a@1.0.0",
			"kind":    "lifecycleScript",
			"script":  "postinstall",
			"command": "node install.js",
			"reason":  "Known package lifecycle script; execution remains blocked by TSPack.",
		}},
	}
	irPath := writeIR(t, dir, ir)
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Packages: []lockfile.Package{{ID: "npm:dep-a@1.0.0", Name: "dep-a", Version: "1.0.0", Source: "npm", Integrity: "x", Capabilities: []lockfile.Capability{{Kind: "lifecycleScript", Script: "postinstall", Command: "node malicious.js"}}}},
		Targets:  []lockfile.Target{{Package: "app", Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts"}},
	}
	b, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(filepath.Join(dir, "ts-lock.toml"), b, 0o644)

	res := Check(DefaultOptionsWithIR(dir, irPath))
	if !hasErrCode(res.Diagnostics, "TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT") {
		t.Fatalf("expected lifecycle warning for changed command: %#v", res.Diagnostics)
	}
	if !hasErrCode(res.Diagnostics, "TSPACK_SECURITY_ACKNOWLEDGED_CAPABILITY_STALE") {
		t.Fatalf("expected stale acknowledgement warning: %#v", res.Diagnostics)
	}
	if hasErrCode(res.Diagnostics, "TSPACK_SECURITY_ACKNOWLEDGED_CAPABILITY_UNUSED") {
		t.Fatalf("stale acknowledgement should not also be reported unused: %#v", res.Diagnostics)
	}
}

func TestCheckUnusedLifecycleAcknowledgement(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	ir["security"] = map[string]any{
		"acknowledgedCapabilities": []map[string]any{{
			"package": "npm:dep-a@1.0.0",
			"kind":    "lifecycleScript",
			"script":  "postinstall",
			"command": "node install.js",
			"reason":  "Known package lifecycle script; execution remains blocked by TSPack.",
		}},
	}
	irPath := writeIR(t, dir, ir)
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Packages: []lockfile.Package{{ID: "npm:other@1.0.0", Name: "other", Version: "1.0.0", Source: "npm", Integrity: "x"}},
		Targets:  []lockfile.Target{{Package: "app", Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts"}},
	}
	b, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(filepath.Join(dir, "ts-lock.toml"), b, 0o644)

	res := Check(DefaultOptionsWithIR(dir, irPath))
	if !hasErrCode(res.Diagnostics, "TSPACK_SECURITY_ACKNOWLEDGED_CAPABILITY_UNUSED") {
		t.Fatalf("expected unused acknowledgement warning: %#v", res.Diagnostics)
	}

	_ = os.Remove(filepath.Join(dir, "ts-lock.toml"))
	missingLock := Check(DefaultOptionsWithIR(dir, irPath))
	if hasErrCode(missingLock.Diagnostics, "TSPACK_SECURITY_ACKNOWLEDGED_CAPABILITY_UNUSED") {
		t.Fatalf("missing lockfile should not emit unused acknowledgement warning: %#v", missingLock.Diagnostics)
	}
}

func TestCheckLifecycleAcknowledgementBehaviorEvidence(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "security"), 0o755); err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(dir, "security", "dep-a-postinstall.valid.xtest.tsx")
	markerPath := filepath.Join(dir, "marker.txt")
	fixtureSource := "import fs from 'node:fs';\n" +
		"import { lifecycle } from 'tspack/x/test';\n" +
		"fs.writeFileSync('" + filepath.ToSlash(markerPath) + "', 'fixture executed');\n" +
		"lifecycle.runScript({ package: 'npm:dep-a@1.0.0', script: 'postinstall', command: 'node install.js' });\n"
	if err := os.WriteFile(fixturePath, []byte(fixtureSource), 0o644); err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(dir, "security", "dep-a-postinstall.report.json")
	if err := os.WriteFile(reportPath, []byte(`{"package":"npm:dep-a@1.0.0","script":"postinstall","command":"node install.js","ok":true,"violations":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ir := simpleIR()
	ir["security"] = map[string]any{
		"acknowledgedCapabilities": []map[string]any{{
			"package":         "npm:dep-a@1.0.0",
			"kind":            "lifecycleScript",
			"script":          "postinstall",
			"command":         "node install.js",
			"reason":          "Known package lifecycle script; execution remains blocked by TSPack.",
			"behaviorFixture": "security/dep-a-postinstall.valid.xtest.tsx",
			"behaviorReport":  "security/dep-a-postinstall.report.json",
		}},
	}
	irPath := writeIR(t, dir, ir)
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Packages: []lockfile.Package{{ID: "npm:dep-a@1.0.0", Name: "dep-a", Version: "1.0.0", Source: "npm", Integrity: "x", Capabilities: []lockfile.Capability{{Kind: "lifecycleScript", Script: "postinstall", Command: "node install.js"}}}},
		Targets:  []lockfile.Target{{Package: "app", Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts"}},
	}
	b, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(filepath.Join(dir, "ts-lock.toml"), b, 0o644)

	res := Check(DefaultOptionsWithIR(dir, irPath))
	if hasErrCode(res.Diagnostics, "TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT") {
		t.Fatalf("exact acknowledgement should suppress lifecycle warning: %#v", res.Diagnostics)
	}
	if hasErrCode(res.Diagnostics, "TSPACK_SECURITY_BEHAVIOR_FIXTURE_MISSING") || hasErrCode(res.Diagnostics, "TSPACK_SECURITY_BEHAVIOR_REPORT_MISSING") || hasErrCode(res.Diagnostics, "TSPACK_SECURITY_BEHAVIOR_REPORT_INVALID") {
		t.Fatalf("present evidence should not warn: %#v", res.Diagnostics)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("check executed behavior fixture or created marker: %v", err)
	}
}

func TestCheckLifecycleAcknowledgementMissingBehaviorEvidence(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	ir["security"] = map[string]any{
		"acknowledgedCapabilities": []map[string]any{{
			"package":         "npm:dep-a@1.0.0",
			"kind":            "lifecycleScript",
			"script":          "postinstall",
			"command":         "node install.js",
			"reason":          "Known package lifecycle script; execution remains blocked by TSPack.",
			"behaviorFixture": "security/missing.valid.xtest.tsx",
			"behaviorReport":  "security/missing.report.json",
		}},
	}
	irPath := writeIR(t, dir, ir)
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Packages: []lockfile.Package{{ID: "npm:dep-a@1.0.0", Name: "dep-a", Version: "1.0.0", Source: "npm", Integrity: "x", Capabilities: []lockfile.Capability{{Kind: "lifecycleScript", Script: "postinstall", Command: "node install.js"}}}},
		Targets:  []lockfile.Target{{Package: "app", Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts"}},
	}
	b, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(filepath.Join(dir, "ts-lock.toml"), b, 0o644)

	res := Check(DefaultOptionsWithIR(dir, irPath))
	if hasErrCode(res.Diagnostics, "TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT") {
		t.Fatalf("exact acknowledgement should still suppress lifecycle warning: %#v", res.Diagnostics)
	}
	if !hasErrCode(res.Diagnostics, "TSPACK_SECURITY_BEHAVIOR_FIXTURE_MISSING") {
		t.Fatalf("missing fixture should warn: %#v", res.Diagnostics)
	}
	if !hasErrCode(res.Diagnostics, "TSPACK_SECURITY_BEHAVIOR_REPORT_MISSING") {
		t.Fatalf("missing report should warn: %#v", res.Diagnostics)
	}

	_ = os.Remove(filepath.Join(dir, "ts-lock.toml"))
	missingLock := Check(DefaultOptionsWithIR(dir, irPath))
	if !hasErrCode(missingLock.Diagnostics, "TSPACK_SECURITY_BEHAVIOR_FIXTURE_MISSING") {
		t.Fatalf("missing fixture should warn without lockfile: %#v", missingLock.Diagnostics)
	}
	if hasErrCode(missingLock.Diagnostics, "TSPACK_SECURITY_ACKNOWLEDGED_CAPABILITY_UNUSED") {
		t.Fatalf("missing lockfile should not emit unused acknowledgement warning: %#v", missingLock.Diagnostics)
	}
}

func TestCheckLifecycleAcknowledgementInvalidBehaviorReport(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "security"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "security", "invalid.report.json"), []byte(`not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	ir := simpleIR()
	ir["security"] = map[string]any{
		"acknowledgedCapabilities": []map[string]any{{
			"package":        "npm:dep-a@1.0.0",
			"kind":           "lifecycleScript",
			"script":         "postinstall",
			"command":        "node install.js",
			"reason":         "Known package lifecycle script; execution remains blocked by TSPack.",
			"behaviorReport": "security/invalid.report.json",
		}},
	}
	irPath := writeIR(t, dir, ir)
	res := Check(DefaultOptionsWithIR(dir, irPath))
	if !hasErrCode(res.Diagnostics, "TSPACK_SECURITY_BEHAVIOR_REPORT_INVALID") {
		t.Fatalf("invalid report should warn: %#v", res.Diagnostics)
	}
}

func TestWhyShowsLifecycleAcknowledgement(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "security"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "security", "dep-a-postinstall.valid.xtest.tsx"), []byte("// behavior evidence fixture; not run by why\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ir := simpleIR()
	ir["security"] = map[string]any{
		"acknowledgedCapabilities": []map[string]any{{
			"package":         "npm:dep-a@1.0.0",
			"kind":            "lifecycleScript",
			"script":          "postinstall",
			"command":         "node install.js",
			"reason":          "Known package lifecycle script; execution remains blocked by TSPack.",
			"behaviorFixture": "security/dep-a-postinstall.valid.xtest.tsx",
		}},
	}
	irPath := writeIR(t, dir, ir)
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Packages: []lockfile.Package{{ID: "npm:dep-a@1.0.0", Name: "dep-a", Version: "1.0.0", Source: "npm", Integrity: "x", Capabilities: []lockfile.Capability{{Kind: "lifecycleScript", Script: "postinstall", Command: "node install.js"}}}},
		Edges:    []lockfile.Edge{{From: "app:target:core", To: "npm:dep-a@1.0.0", Kind: "runtime"}},
		Targets:  []lockfile.Target{{Package: "app", Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts"}},
	}
	b, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(filepath.Join(dir, "ts-lock.toml"), b, 0o644)

	res := Why(DefaultOptionsWithIR(dir, irPath), WhyOptions{Query: "npm:dep-a@1.0.0"})
	if hasErrors(res.Diagnostics) {
		t.Fatalf("why failed: %#v", res.Diagnostics)
	}
	if len(res.WhyResult.Explanations) != 1 || len(res.WhyResult.Explanations[0].LockPackages) != 1 {
		t.Fatalf("expected lock package explanation: %#v", res.WhyResult)
	}
	capability := res.WhyResult.Explanations[0].LockPackages[0].Capabilities[0]
	if !capability.Acknowledged {
		t.Fatalf("expected acknowledged capability: %#v", capability)
	}
	if !strings.Contains(capability.AcknowledgementReason, "execution remains blocked") {
		t.Fatalf("missing acknowledgement reason: %#v", capability)
	}
	if capability.BehaviorFixture != "security/dep-a-postinstall.valid.xtest.tsx" || capability.BehaviorFixtureStatus != "present" {
		t.Fatalf("missing why behavior fixture evidence: %#v", capability)
	}

	reverse := Why(DefaultOptionsWithIR(dir, irPath), WhyOptions{Query: "npm:dep-a@1.0.0", Reverse: true})
	if hasErrors(reverse.Diagnostics) {
		t.Fatalf("reverse why failed: %#v", reverse.Diagnostics)
	}
	if len(reverse.WhyResult.LockPackages) != 1 || !reverse.WhyResult.LockPackages[0].Capabilities[0].Acknowledged {
		t.Fatalf("expected reverse why acknowledged metadata: %#v", reverse.WhyResult.LockPackages)
	}
	if reverse.WhyResult.LockPackages[0].Capabilities[0].BehaviorFixtureStatus != "present" {
		t.Fatalf("expected reverse why behavior evidence metadata: %#v", reverse.WhyResult.LockPackages)
	}
}

func TestSyncDoesNotExecuteLifecycleScripts(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	ir["security"] = map[string]any{
		"acknowledgedCapabilities": []map[string]any{{
			"package": "npm:dep-a@1.0.0",
			"kind":    "lifecycleScript",
			"script":  "postinstall",
			"command": "node install.js",
			"reason":  "Known package lifecycle script; execution remains blocked by TSPack.",
		}},
	}
	irPath := writeIR(t, dir, ir)
	st, _ := store.Open(filepath.Join(dir, ".tspack", "store"))
	marker := filepath.Join(dir, "marker.txt")
	depRoot := t.TempDir()
	packageJSON := "{\"name\":\"dep-a\",\"version\":\"1.0.0\",\"scripts\":{\"postinstall\":\"sh -c 'echo bad > " + filepath.ToSlash(marker) + "'\"}}"
	_ = os.WriteFile(filepath.Join(depRoot, "package.json"), []byte(packageJSON), 0o644)
	ref, diags := st.PutArtifact(store.Artifact{ID: "npm:dep-a@1.0.0", Name: "dep-a", Version: "1.0.0", Source: "npm", Kind: store.ArtifactPathTree, RootDir: depRoot})
	if len(diags) > 0 {
		t.Fatalf("unexpected put artifact diagnostics: %#v", diags)
	}
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Packages: []lockfile.Package{{ID: "npm:dep-a@1.0.0", Name: "dep-a", Version: "1.0.0", Source: "npm", Hash: ref.Hash, Capabilities: []lockfile.Capability{{Kind: "lifecycleScript", Script: "postinstall", Command: "node install.js"}}}},
		Edges:    []lockfile.Edge{{From: "app:target:core", To: "npm:dep-a@1.0.0", Kind: "runtime"}},
		Targets:  []lockfile.Target{{Package: "app", Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts"}},
	}
	b, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(filepath.Join(dir, "ts-lock.toml"), b, 0o644)
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = irPath
	res := Sync(opts, false)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("sync failed: %#v", res.Diagnostics)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("marker file exists; lifecycle script was executed")
	}
}

func TestSyncMissingAndStaleLockfile(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, simpleIR())
	if !hasErrCode(Sync(opts, false).Diagnostics, "TSPACK_SYNC_LOCKFILE_MISSING") {
		t.Fatalf("missing expected code")
	}
	_ = os.WriteFile(opts.LockfilePath, []byte("bad"), 0o644)
	before, _ := os.ReadFile(opts.LockfilePath)
	res := Sync(opts, false)
	if !hasErrCode(res.Diagnostics, "TSPACK_SYNC_LOCKFILE_STALE") && !hasErrCode(res.Diagnostics, "TSPACK_LOCK_INVALID_TOML") {
		t.Fatalf("expected stale lock diagnostics")
	}
	after, _ := os.ReadFile(opts.LockfilePath)
	if !bytes.Equal(before, after) {
		t.Fatalf("stale lock mutated")
	}
}

func TestFrontendBridgeMissingCLI(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.ManifestPath = filepath.Join(dir, "manifest.tsx")
	_ = os.WriteFile(opts.ManifestPath, []byte("export default {}"), 0o644)
	res := Check(opts)
	if !hasErrCode(res.Diagnostics, "TSPACK_PROJECT_MANIFEST_FRONTEND_FAILED") {
		t.Fatalf("expected frontend failure")
	}
}

func TestOutdatedWantedAndLatestAvailable(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	pkgs := ir["packages"].([]map[string]any)
	pkgs[0]["dependencies"] = []map[string]any{
		{"kind": "peer", "name": "react", "source": map[string]any{"kind": "npm", "package": "react", "range": ">=18 <20"}},
	}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"}, Packages: []lockfile.Package{{ID: "npm:react@18.2.0", Name: "react", Source: "npm", Version: "18.2.0", Hash: "sha256:dummy"}}}
	b, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(opts.LockfilePath, b, 0o644)
	opts.ResolverClient = &fakeClient{meta: map[string]*resolver.PackageMetadata{"react": {Name: "react", DistTags: map[string]string{"latest": "20.0.0"}, Versions: map[string]resolver.PackageVersion{"18.2.0": {Version: "18.2.0"}, "19.2.0": {Version: "19.2.0"}, "20.0.0": {Version: "20.0.0"}}}}}
	res := Outdated(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("outdated failed: %#v", res.Diagnostics)
	}
	if got := res.Outdated.Dependencies[0].Status; got != "wanted_available" {
		t.Fatalf("expected wanted_available, got %s", got)
	}
}

func TestOutdatedMissingLockWarning(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	pkgs := ir["packages"].([]map[string]any)
	pkgs[0]["dependencies"] = []map[string]any{
		{"kind": "dep", "name": "left-pad", "source": map[string]any{"kind": "npm", "package": "left-pad", "range": "^1.0.0"}},
	}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	opts.ResolverClient = &fakeClient{meta: map[string]*resolver.PackageMetadata{"left-pad": {Name: "left-pad", DistTags: map[string]string{"latest": "1.3.0"}, Versions: map[string]resolver.PackageVersion{"1.0.0": {Version: "1.0.0"}, "1.3.0": {Version: "1.3.0"}}}}}
	res := Outdated(opts)
	if !hasErrCode(res.Diagnostics, "TSPACK_OUTDATED_LOCKFILE_MISSING") {
		t.Fatalf("expected lockfile warning")
	}
	if status := res.Outdated.Dependencies[0].Status; status != "missing_lock" {
		t.Fatalf("expected missing_lock, got %s", status)
	}
	if opts.ResolverClient.(*fakeClient).packageMetaCalls != 1 {
		t.Fatalf("expected metadata fetch for missing lock")
	}
}

func TestOutdatedCurrentStatusAndNoDiagnostics(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	pkgs := ir["packages"].([]map[string]any)
	pkgs[0]["dependencies"] = []map[string]any{
		{"kind": "dep", "name": "left-pad", "source": map[string]any{"kind": "npm", "package": "left-pad", "range": "^1.0.0"}},
	}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"}, Packages: []lockfile.Package{{ID: "npm:left-pad@1.0.0", Name: "left-pad", Source: "npm", Version: "1.0.0", Hash: "sha256:dummy"}}}
	lockBytes, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(opts.LockfilePath, lockBytes, 0o644)
	client := &fakeClient{meta: map[string]*resolver.PackageMetadata{"left-pad": {Name: "left-pad", DistTags: map[string]string{"latest": "1.0.0"}, Versions: map[string]resolver.PackageVersion{"1.0.0": {Version: "1.0.0"}}}}}
	opts.ResolverClient = client
	res := Outdated(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("outdated failed: %#v", res.Diagnostics)
	}
	if len(res.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got %#v", res.Diagnostics)
	}
	dep := res.Outdated.Dependencies[0]
	if dep.Status != "current" {
		t.Fatalf("expected current, got %s", dep.Status)
	}
	if res.Outdated.Summary.Current != 1 || res.Outdated.Summary.Outdated != 0 {
		t.Fatalf("unexpected summary: %#v", res.Outdated.Summary)
	}
	if len(client.tarCalls) != 0 {
		t.Fatalf("outdated should not fetch tarballs")
	}
	lockAfter, _ := os.ReadFile(opts.LockfilePath)
	if !bytes.Equal(lockBytes, lockAfter) {
		t.Fatalf("outdated mutated lockfile")
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("outdated created node_modules")
	}
	if _, err := os.Stat(filepath.Join(dir, ".tspack", "store")); !os.IsNotExist(err) {
		t.Fatalf("outdated created store")
	}
}

func TestOutdatedLatestOutsideRange(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	pkgs := ir["packages"].([]map[string]any)
	pkgs[0]["dependencies"] = []map[string]any{
		{"kind": "dep", "name": "dep-a", "source": map[string]any{"kind": "npm", "package": "dep-a", "range": "^1.0.0"}},
	}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"}, Packages: []lockfile.Package{{ID: "npm:dep-a@1.5.0", Name: "dep-a", Source: "npm", Version: "1.5.0", Hash: "sha256:dummy"}}}
	lockBytes, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(opts.LockfilePath, lockBytes, 0o644)
	opts.ResolverClient = &fakeClient{meta: map[string]*resolver.PackageMetadata{"dep-a": {Name: "dep-a", DistTags: map[string]string{"latest": "2.0.0"}, Versions: map[string]resolver.PackageVersion{"1.4.0": {Version: "1.4.0"}, "1.5.0": {Version: "1.5.0"}, "2.0.0": {Version: "2.0.0"}}}}}
	res := Outdated(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("outdated failed: %#v", res.Diagnostics)
	}
	dep := res.Outdated.Dependencies[0]
	if dep.Status != "latest_available" || dep.Wanted != "1.5.0" || dep.Latest != "2.0.0" {
		t.Fatalf("unexpected dependency: %#v", dep)
	}
}

func TestOutdatedSkipsNonNPMSourcesWithoutMetadataFetch(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	pkgs := ir["packages"].([]map[string]any)
	pkgs[0]["dependencies"] = []map[string]any{
		{"kind": "workspace", "name": "core", "source": map[string]any{"kind": "workspace", "package": "core"}},
	}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	client := &fakeClient{meta: map[string]*resolver.PackageMetadata{}}
	opts.ResolverClient = client
	res := Outdated(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("outdated failed: %#v", res.Diagnostics)
	}
	if client.packageMetaCalls != 0 {
		t.Fatalf("expected no registry fetch for non-npm dependency")
	}
	dep := res.Outdated.Dependencies[0]
	if dep.Status != "not_applicable" {
		t.Fatalf("expected not_applicable, got %s", dep.Status)
	}
	if !hasErrCode(res.Diagnostics, "TSPACK_OUTDATED_NON_REGISTRY_DEP") {
		t.Fatalf("expected unsupported source warning")
	}
}

func TestOutdatedMetadataFailureAndMultipleLockedVersions(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	pkgs := ir["packages"].([]map[string]any)
	pkgs[0]["dependencies"] = []map[string]any{
		{"kind": "dep", "name": "dep-a", "source": map[string]any{"kind": "npm", "package": "dep-a", "range": "^1.0.0"}},
	}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	lf := &lockfile.Lockfile{
		Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Packages: []lockfile.Package{
			{ID: "npm:dep-a@1.0.0", Name: "dep-a", Source: "npm", Version: "1.0.0", Hash: "sha256:a"},
			{ID: "npm:dep-a@1.1.0", Name: "dep-a", Source: "npm", Version: "1.1.0", Hash: "sha256:b"},
		},
	}
	lockBytes, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(opts.LockfilePath, lockBytes, 0o644)
	client := &fakeClient{
		meta:    map[string]*resolver.PackageMetadata{},
		metaErr: map[string]error{"dep-a": errors.New("metadata request failed: status=500 package=dep-a")},
	}
	opts.ResolverClient = client
	res := Outdated(opts)
	if !hasErrors(res.Diagnostics) {
		t.Fatalf("expected metadata error")
	}
	dep := res.Outdated.Dependencies[0]
	if dep.Status != "error" {
		t.Fatalf("expected error status, got %s", dep.Status)
	}
	if !reflect.DeepEqual(dep.Current, []string{"1.0.0", "1.1.0"}) {
		t.Fatalf("unexpected current versions: %#v", dep.Current)
	}
	if !hasErrCode(res.Diagnostics, "TSPACK_OUTDATED_METADATA_FETCH_FAILED") {
		t.Fatalf("expected metadata fetch diagnostic")
	}
	if len(res.Diagnostics) == 0 || len(res.Diagnostics[0].Details) == 0 {
		t.Fatalf("expected diagnostic details with package identity")
	}
}

func TestOutdatedRootDirFromDifferentCWD(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	ir := simpleIR()
	pkgs := ir["packages"].([]map[string]any)
	pkgs[0]["dependencies"] = []map[string]any{
		{"kind": "dep", "name": "dep-a", "source": map[string]any{"kind": "npm", "package": "dep-a", "range": "^1.0.0"}},
	}
	opts := DefaultOptions(root)
	opts.ManifestIRPath = writeIR(t, root, ir)
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"}, Packages: []lockfile.Package{{ID: "npm:dep-a@1.0.0", Name: "dep-a", Source: "npm", Version: "1.0.0", Hash: "sha256:dummy"}}}
	lockBytes, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(opts.LockfilePath, lockBytes, 0o644)
	client := &fakeClient{meta: map[string]*resolver.PackageMetadata{"dep-a": {Name: "dep-a", DistTags: map[string]string{"latest": "1.0.0"}, Versions: map[string]resolver.PackageVersion{"1.0.0": {Version: "1.0.0"}}}}}
	opts.ResolverClient = client
	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	_ = os.Chdir(other)
	res := Outdated(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("outdated failed: %#v", res.Diagnostics)
	}
	if client.packageMetaCalls != 1 || len(client.metaCalls) != 1 || client.metaCalls[0] != "dep-a" {
		t.Fatalf("unexpected registry lookup calls: %#v", client.metaCalls)
	}
}

func writeIR(t *testing.T, dir string, ir map[string]any) string {
	t.Helper()
	_ = os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	_ = os.MkdirAll(filepath.Join(dir, "dist"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "src", "index.ts"), []byte("export const x = 1;\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "dist", "index.d.ts"), []byte("export declare const x: number;\n"), 0o644)
	b, _ := json.Marshal(ir)
	p := filepath.Join(dir, "manifest.ir.json")
	_ = os.WriteFile(p, b, 0o644)
	return p
}
func simpleIR() map[string]any {
	return map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{{"name": "app", "version": "1.0.0", "kind": "library", "dependencies": []map[string]any{}, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}}}}
}
func hasErrCode(diags []diag.Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}
func mustExist(t *testing.T, p string) {
	t.Helper()
	if _, e := os.Stat(p); e != nil {
		t.Fatalf("missing %s", p)
	}
}

func buildRegistry() *fakeClient {
	tarballs := map[string][]byte{}
	mk := func(name, version string, deps map[string]string) resolver.PackageVersion {
		tgz := tarball(name, version, deps)
		u := "https://example.invalid/" + name + "-" + version + ".tgz"
		tarballs[u] = tgz
		sum := sha512sum(tgz)
		return resolver.PackageVersion{Name: name, Version: version, Dependencies: deps, Dist: resolver.PackageDist{Tarball: u, Integrity: "sha512-" + base64.StdEncoding.EncodeToString(sum)}}
	}
	meta := map[string]*resolver.PackageMetadata{"dep-a": {Name: "dep-a", Versions: map[string]resolver.PackageVersion{"1.0.0": mk("dep-a", "1.0.0", map[string]string{"left-pad": "1.2.0"})}}, "left-pad": {Name: "left-pad", Versions: map[string]resolver.PackageVersion{"1.2.0": mk("left-pad", "1.2.0", nil)}}}
	return &fakeClient{meta: meta, tar: tarballs}
}
func sha512sum(b []byte) []byte { h := sha512.Sum512(b); return h[:] }
func tarball(name, version string, deps map[string]string) []byte {
	pj := map[string]any{"name": name, "version": version}
	if len(deps) > 0 {
		pj["dependencies"] = deps
	}
	jb, _ := json.Marshal(pj)
	buf := bytes.NewBuffer(nil)
	gz := gzip.NewWriter(buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "package/package.json", Mode: 0o644, Size: int64(len(jb))})
	_, _ = tw.Write(jb)
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}
func putPkg(t *testing.T, st *store.Store, name, version string) string {
	d := t.TempDir()
	_ = os.WriteFile(filepath.Join(d, "package.json"), []byte("{\"name\":\""+name+"\",\"version\":\""+version+"\"}"), 0o644)
	ref, diags := st.PutArtifact(store.Artifact{ID: "npm:" + name + "@" + version, Name: name, Version: version, Source: "npm", Kind: store.ArtifactPathTree, RootDir: d})
	if len(diags) > 0 {
		t.Fatalf("put artifact diags: %#v", diags)
	}
	return ref.Hash
}

func TestPackDryRunAndDeterministic(t *testing.T) {
	dir := t.TempDir()
	irPath := writeIR(t, dir, simpleIR())
	_ = os.WriteFile(filepath.Join(dir, "src", "index.ts"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("readme"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "dist", "index.js"), []byte("export const x=1\n"), 0o644)
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = irPath
	r := Pack(opts, PackOptions{DryRun: true})
	if hasErrors(r.Diagnostics) {
		t.Fatalf("dry run failed: %#v", r.Diagnostics)
	}
	if len(r.PackResult.Preview) == 0 {
		t.Fatalf("expected preview")
	}
	if _, err := os.Stat(filepath.Join(dir, "tspack-artifacts")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote artifact")
	}

	r1 := Pack(opts, PackOptions{})
	r2 := Pack(opts, PackOptions{})
	if hasErrors(r1.Diagnostics) || hasErrors(r2.Diagnostics) {
		t.Fatalf("pack failed")
	}
	b1, _ := os.ReadFile(r1.PackResult.Artifacts[0].Path)
	b2, _ := os.ReadFile(r2.PackResult.Artifacts[0].Path)
	if !bytes.Equal(b1, b2) {
		t.Fatalf("nondeterministic")
	}
}

func TestPackEdgeCasesAndIntegration(t *testing.T) {
	t.Run("basic archive contents", func(t *testing.T) {
		dir := t.TempDir()
		ir := simpleIR()
		pkg := ir["packages"].([]map[string]any)[0]
		pkg["targets"] = []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "dist/index.js", "types": "dist/index.d.ts", "deps": []string{}, "peers": []string{}}}
		pkg["publish"] = map[string]any{"include": []string{"dist/**", "README.md", "LICENSE"}, "exclude": []string{"src/**"}}
		irPath := writeIR(t, dir, ir)
		_ = os.WriteFile(filepath.Join(dir, "dist", "index.js"), []byte("export {}\n"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("r\n"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "LICENSE"), []byte("l\n"), 0o644)
		r := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{})
		if hasErrors(r.Diagnostics) {
			t.Fatalf("pack failed: %#v", r.Diagnostics)
		}
		entries := readEntries(t, r.PackResult.Artifacts[0].Path)
		mustContain(t, entries, "package/package.json", "package/dist/index.js", "package/dist/index.d.ts", "package/README.md", "package/LICENSE")
		mustNotContain(t, entries, "package/src/index.ts")
	})

	t.Run("missing runtime and missing types", func(t *testing.T) {
		dir := t.TempDir()
		ir := simpleIR()
		ir["packages"].([]map[string]any)[0]["targets"] = []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "dist/missing.js", "types": "dist/missing.d.ts", "deps": []string{}, "peers": []string{}}}
		irPath := writeIR(t, dir, ir)
		r := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{})
		if !hasErrCode(r.Diagnostics, "TSPACK_TYPE_MISSING_OUTPUT") {
			t.Fatalf("missing diagnostics: %#v", r.Diagnostics)
		}
		if r.PackResult != nil && len(r.PackResult.Artifacts) > 0 {
			t.Fatalf("unexpected artifact")
		}
	})

	t.Run("workspace all and selector", func(t *testing.T) {
		dir := t.TempDir()
		ir := map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{
			{"name": "pkg-a", "version": "1.0.0", "root": "packages/a", "kind": "library", "dependencies": []map[string]any{}, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "dist/index.js", "types": "dist/index.d.ts", "deps": []string{}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}},
			{"name": "pkg-b", "version": "2.0.0", "root": "packages/b", "kind": "library", "dependencies": []map[string]any{}, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "dist/index.js", "types": "dist/index.d.ts", "deps": []string{}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}},
		}}
		for _, p := range []string{"packages/a/src", "packages/a/dist", "packages/b/src", "packages/b/dist"} {
			_ = os.MkdirAll(filepath.Join(dir, p), 0o755)
		}
		_ = os.WriteFile(filepath.Join(dir, "packages/a/src/index.ts"), []byte("a"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "packages/a/dist/index.js"), []byte("a"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "packages/a/dist/index.d.ts"), []byte("a"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "packages/b/src/index.ts"), []byte("b"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "packages/b/dist/index.js"), []byte("b"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "packages/b/dist/index.d.ts"), []byte("b"), 0o644)
		irPath := writeIR(t, dir, ir)
		r := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{})
		if hasErrors(r.Diagnostics) || len(r.PackResult.Artifacts) != 2 {
			t.Fatalf("expected two artifacts %#v %#v", r.Diagnostics, r.PackResult)
		}
		rs := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{PackageName: "pkg-a"})
		if hasErrors(rs.Diagnostics) || len(rs.PackResult.Artifacts) != 1 {
			t.Fatalf("selector failed")
		}
		rm := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{PackageName: "missing"})
		if !hasErrCode(rm.Diagnostics, "TSPACK_PACK_PACKAGE_NOT_FOUND") {
			t.Fatalf("expected not found")
		}
	})
}

func DefaultOptionsWithIR(dir, irPath string) Options {
	o := DefaultOptions(dir)
	o.ManifestIRPath = irPath
	return o
}
func readEntries(t *testing.T, path string) []string {
	t.Helper()
	b, _ := os.ReadFile(path)
	gr, _ := gzip.NewReader(bytes.NewReader(b))
	tr := tar.NewReader(gr)
	out := []string{}
	for {
		h, e := tr.Next()
		if e != nil {
			break
		}
		if !h.ModTime.Equal((h.ModTime).UTC()) {
		}
		out = append(out, h.Name)
	}
	_ = gr.Close()
	return out
}
func mustContain(t *testing.T, entries []string, vals ...string) {
	t.Helper()
	m := map[string]bool{}
	for _, e := range entries {
		m[e] = true
	}
	for _, v := range vals {
		if !m[v] {
			t.Fatalf("missing %s in %v", v, entries)
		}
	}
}
func mustNotContain(t *testing.T, entries []string, val string) {
	t.Helper()
	for _, e := range entries {
		if e == val {
			t.Fatalf("unexpected %s", val)
		}
	}
}

func TestPackMutationGuaranteesAndGeneratedPackageJSON(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	pkg := ir["packages"].([]map[string]any)[0]
	pkg["license"] = "MIT"
	pkg["dependencies"] = []map[string]any{
		{"key": "react-dom", "kind": "peer", "source": map[string]any{"kind": "npm", "package": "react-dom", "range": ">=18 <20"}},
		{"key": "react", "kind": "peer", "source": map[string]any{"kind": "npm", "package": "react", "range": ">=18 <20"}},
	}
	pkg["targets"] = []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "dist/index.js", "types": "dist/index.d.ts", "deps": []string{}, "peers": []string{"react-dom", "react"}}}
	pkg["publish"] = map[string]any{"include": []string{"dist/**", "README.md"}, "exclude": []string{}}
	irPath := writeIR(t, dir, ir)
	_ = os.WriteFile(filepath.Join(dir, "dist", "index.js"), []byte("export const x = 1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "dist", "index.d.ts"), []byte("export declare const x: number;\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("readme\n"), 0o644)
	lockPath := filepath.Join(dir, "ts-lock.toml")
	lf := &lockfile.Lockfile{
		Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Targets: []lockfile.Target{{
			Package: "app",
			Name:    "core",
			Export:  ".",
			Entry:   "src/index.ts",
			Runtime: "dist/index.js",
			Types:   "dist/index.d.ts",
		}},
	}
	before, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(lockPath, before, 0o644)

	r := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{})
	if hasErrors(r.Diagnostics) {
		t.Fatalf("pack failed: %#v", r.Diagnostics)
	}
	after, _ := os.ReadFile(lockPath)
	if !bytes.Equal(before, after) {
		t.Fatalf("lockfile mutated")
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("node_modules created")
	}
	if _, err := os.Stat(filepath.Join(dir, ".tspack", "store")); !os.IsNotExist(err) {
		t.Fatalf("store created")
	}

	artifactBytes, _ := os.ReadFile(r.PackResult.Artifacts[0].Path)
	gz, _ := gzip.NewReader(bytes.NewReader(artifactBytes))
	tr := tar.NewReader(gz)
	var packageJSON []byte
	for {
		h, e := tr.Next()
		if e != nil {
			break
		}
		if h.Name == "package/package.json" {
			packageJSON, _ = io.ReadAll(tr)
		}
	}
	_ = gz.Close()
	if len(packageJSON) == 0 {
		t.Fatalf("missing generated package.json")
	}
	var parsed map[string]any
	_ = json.Unmarshal(packageJSON, &parsed)
	if parsed["name"] != "app" || parsed["version"] != "1.0.0" {
		t.Fatalf("missing name/version: %s", string(packageJSON))
	}
	if parsed["license"] != "MIT" || parsed["main"] != "./dist/index.js" || parsed["types"] != "./dist/index.d.ts" {
		t.Fatalf("missing generated package metadata: %s", string(packageJSON))
	}
	peers := parsed["peerDependencies"].(map[string]any)
	if peers["react"] != ">=18 <20" || peers["react-dom"] != ">=18 <20" {
		t.Fatalf("missing generated package peers: %s", string(packageJSON))
	}
	exports, ok := parsed["exports"].(map[string]any)
	if !ok || exports["."] == nil {
		t.Fatalf("missing exports: %s", string(packageJSON))
	}
}

func TestPackFailsWhenCheckFailsAndWritesNoArtifact(t *testing.T) {
	dir := t.TempDir()
	irPath := writeIR(t, dir, simpleIR())
	_ = os.Remove(filepath.Join(dir, "src", "index.ts"))
	_ = os.WriteFile(filepath.Join(dir, "dist", "index.js"), []byte("export const x = 1\n"), 0o644)
	outDir := filepath.Join(dir, "out")
	r := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: outDir})
	if !hasErrCode(r.Diagnostics, "TSPACK_IMPORT_PARSE_ERROR") {
		t.Fatalf("expected propagated check error: %#v", r.Diagnostics)
	}
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Fatalf("unexpected output dir created")
	}
}

func TestWhyDoesNotMutateLockfile(t *testing.T) {
	dir := t.TempDir()
	irPath := writeIR(t, dir, simpleIR())
	lockPath := filepath.Join(dir, "ts-lock.toml")
	_ = os.WriteFile(lockPath, []byte("[lock]\nformat=1\ntool=\"tspack\"\n"), 0o644)
	before, _ := os.ReadFile(lockPath)
	r := Why(DefaultOptionsWithIR(dir, irPath), WhyOptions{Query: "core"})
	if r.WhyResult == nil {
		t.Fatalf("missing why result")
	}
	after, _ := os.ReadFile(lockPath)
	if !bytes.Equal(before, after) {
		t.Fatalf("lockfile mutated")
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("node_modules should not exist")
	}
}

func TestWhyMissingAndInvalidLockfileAndDeterminism(t *testing.T) {
	dir := t.TempDir()
	irPath := writeIR(t, dir, simpleIR())
	opts := DefaultOptionsWithIR(dir, irPath)

	missing := Why(opts, WhyOptions{Query: "core"})
	if missing.WhyResult == nil {
		t.Fatalf("missing why result")
	}
	if !hasErrCode(missing.Diagnostics, "TSPACK_WHY_LOCKFILE_MISSING") {
		t.Fatalf("expected missing lock warning")
	}
	if hasErrors(missing.Diagnostics) {
		t.Fatalf("missing lock should not fail: %#v", missing.Diagnostics)
	}

	_ = os.WriteFile(opts.LockfilePath, []byte("bad"), 0o644)
	invalid := Why(opts, WhyOptions{Query: "core"})
	if invalid.WhyResult == nil {
		t.Fatalf("invalid lock should still provide why result")
	}
	if !hasErrCode(invalid.Diagnostics, "TSPACK_LOCK_INVALID_TOML") {
		t.Fatalf("expected invalid lock diagnostics")
	}

	validLock := []byte("[lock]\nformat=1\ntool=\"tspack\"\n")
	_ = os.WriteFile(opts.LockfilePath, validLock, 0o644)
	r1 := Why(opts, WhyOptions{Query: "core"})
	r2 := Why(opts, WhyOptions{Query: "core"})
	if !reflect.DeepEqual(r1.WhyResult, r2.WhyResult) {
		t.Fatalf("non-deterministic why result")
	}
	if _, err := os.Stat(filepath.Join(dir, ".tspack", "store")); !os.IsNotExist(err) {
		t.Fatalf("store should not be created")
	}
}

func TestV1GoldenPathCommandLoop(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	opts.ResolverClient = buildRegistry()

	check1 := Check(opts)
	if hasErrors(check1.Diagnostics) && !hasErrCode(check1.Diagnostics, "TSPACK_CHECK_LOCKFILE_MISSING") {
		t.Fatalf("initial check unexpected errors: %#v", check1.Diagnostics)
	}
	if _, err := os.Stat(opts.LockfilePath); !os.IsNotExist(err) {
		t.Fatalf("check should not create lockfile")
	}

	up := Update(opts)
	if hasErrors(up.Diagnostics) {
		t.Fatalf("update failed: %#v", up.Diagnostics)
	}
	lockAfterUpdate, _ := os.ReadFile(opts.LockfilePath)

	syncRes := Sync(opts, false)
	if hasErrors(syncRes.Diagnostics) {
		t.Fatalf("sync failed: %#v", syncRes.Diagnostics)
	}
	lockAfterSync, _ := os.ReadFile(opts.LockfilePath)
	if !bytes.Equal(lockAfterUpdate, lockAfterSync) {
		t.Fatalf("sync mutated lockfile")
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules", ".tspack-materialized")); err != nil {
		t.Fatalf("sync should materialize node_modules marker: %v", err)
	}

	whyRes := Why(opts, WhyOptions{Query: "core"})
	if hasErrors(whyRes.Diagnostics) || whyRes.WhyResult == nil || len(whyRes.WhyResult.Explanations) == 0 {
		t.Fatalf("why failed: %#v", whyRes.Diagnostics)
	}
	lockAfterWhy, _ := os.ReadFile(opts.LockfilePath)
	if !bytes.Equal(lockAfterSync, lockAfterWhy) {
		t.Fatalf("why mutated lockfile")
	}

	if _, err := os.Stat(filepath.Join(dir, "tspack-artifacts")); !os.IsNotExist(err) {
		t.Fatalf("pack output dir should not pre-exist")
	}
	pack1 := Pack(opts, PackOptions{OutputDir: filepath.Join(dir, "out")})
	if hasErrors(pack1.Diagnostics) || pack1.PackResult == nil || len(pack1.PackResult.Artifacts) != 1 {
		t.Fatalf("pack1 failed: %#v", pack1.Diagnostics)
	}
	pack2 := Pack(opts, PackOptions{OutputDir: filepath.Join(dir, "out2")})
	if hasErrors(pack2.Diagnostics) || pack2.PackResult == nil || len(pack2.PackResult.Artifacts) != 1 {
		t.Fatalf("pack2 failed: %#v", pack2.Diagnostics)
	}
	if pack1.PackResult.Artifacts[0].Hash != pack2.PackResult.Artifacts[0].Hash {
		t.Fatalf("pack hash not stable: %s vs %s", pack1.PackResult.Artifacts[0].Hash, pack2.PackResult.Artifacts[0].Hash)
	}
	lockAfterPack, _ := os.ReadFile(opts.LockfilePath)
	if !bytes.Equal(lockAfterWhy, lockAfterPack) {
		t.Fatalf("pack mutated lockfile")
	}
}

func targetedIRWithDeps(deps []map[string]any, depRefs []string) map[string]any {
	return map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{{"name": "app", "version": "1.0.0", "kind": "library", "dependencies": deps, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": depRefs, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}}}}
}

func TestTargetedSelectionKeyNameAndQualifiedName(t *testing.T) {
	deps := []map[string]any{{"key": "react", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "react", "range": "^18"}}, {"key": "lodash", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "lodash", "range": "^4"}}}
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, targetedIRWithDeps(deps, []string{"react", "lodash"}))
	opts.ResolverClient = buildRegistryForTargetedSelection(t)
	for _, q := range []string{"react", "npm:react"} {
		res := UpdateDryRunWithOptions(opts, UpdateOptions{Query: q})
		if hasErrors(res.Diagnostics) {
			t.Fatalf("query %s failed: %#v", q, res.Diagnostics)
		}
		if res.UpdateTarget == nil || len(res.UpdateTarget.Selected) != 1 || res.UpdateTarget.Selected[0].Name != "react" {
			t.Fatalf("query %s selected unexpected target: %#v", q, res.UpdateTarget)
		}
	}
	resByName := UpdateDryRunWithOptions(opts, UpdateOptions{Query: "lodash"})
	if hasErrors(resByName.Diagnostics) {
		t.Fatalf("query by package name failed: %#v", resByName.Diagnostics)
	}
	if resByName.UpdateTarget == nil || len(resByName.UpdateTarget.Selected) != 1 || resByName.UpdateTarget.Selected[0].Name != "lodash" {
		t.Fatalf("query by package name selected unexpected target: %#v", resByName.UpdateTarget)
	}
}

func TestTargetedSelectionNotFoundAndTransitiveOnly(t *testing.T) {
	dir := t.TempDir()
	deps := []map[string]any{{"key": "react", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "react", "range": "^18"}}}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, targetedIRWithDeps(deps, []string{"react"}))
	opts.ResolverClient = buildRegistryForTargetedSelection(t)
	res := UpdateDryRunWithOptions(opts, UpdateOptions{Query: "loose-envify"})
	if !hasErrCode(res.Diagnostics, "TSPACK_UPDATE_TARGET_NOT_FOUND") {
		t.Fatalf("expected not found diagnostic: %#v", res.Diagnostics)
	}
	if !diagnosticHasDetail(res.Diagnostics, "TSPACK_UPDATE_TARGET_NOT_FOUND", "targeted update only updates declared dependencies") {
		t.Fatalf("expected declared-only detail: %#v", res.Diagnostics)
	}
}

func TestTargetedSelectionAmbiguousAndUnsupportedSource(t *testing.T) {
	dir := t.TempDir()
	deps := []map[string]any{{"key": "react", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "react", "range": "^18"}}, {"key": "local-shared", "kind": "dep", "source": map[string]any{"kind": "path", "path": "vendor/shared"}}}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{{"name": "app", "version": "1.0.0", "kind": "library", "dependencies": deps, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{"react", "local-shared"}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}}}})
	opts.ResolverClient = buildRegistryForTargetedSelection(t)
	res := UpdateDryRunWithOptions(opts, UpdateOptions{Query: "local-shared"})
	if !hasErrCode(res.Diagnostics, "TSPACK_UPDATE_TARGET_UNSUPPORTED_SOURCE") {
		t.Fatalf("expected unsupported source diagnostic: %#v", res.Diagnostics)
	}

	dir2 := t.TempDir()
	opts2 := DefaultOptions(dir2)
	app1 := map[string]any{
		"name":         "app",
		"version":      "1.0.0",
		"kind":         "library",
		"dependencies": []map[string]any{{"key": "shared", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "react", "range": "^18"}}},
		"targets":      []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{"shared"}, "peers": []string{}}},
		"tools":        []string{},
		"boundaries":   []any{},
		"publish":      map[string]any{"include": []string{"dist/**"}, "exclude": []string{}},
		"policies":     map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}},
	}
	app2 := map[string]any{
		"name":         "app2",
		"version":      "1.0.0",
		"kind":         "library",
		"dependencies": []map[string]any{{"key": "shared", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "react-dom", "range": "^18"}}},
		"targets":      []map[string]any{{"name": "core", "export": "./2", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{"shared"}, "peers": []string{}}},
		"tools":        []string{},
		"boundaries":   []any{},
		"publish":      map[string]any{"include": []string{"dist/**"}, "exclude": []string{}},
		"policies":     map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}},
	}
	opts2.ManifestIRPath = writeIR(t, dir2, map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{app1, app2}})
	opts2.ResolverClient = buildRegistryForTargetedSelection(t)
	res2 := UpdateDryRunWithOptions(opts2, UpdateOptions{Query: "shared"})
	if !hasErrCode(res2.Diagnostics, "TSPACK_UPDATE_TARGET_AMBIGUOUS") {
		t.Fatalf("expected ambiguous diagnostic: %#v", res2.Diagnostics)
	}
	if !diagnosticHasDetail(res2.Diagnostics, "TSPACK_UPDATE_TARGET_AMBIGUOUS", "query matched multiple declared dependencies") {
		t.Fatalf("expected ambiguous matches detail: %#v", res2.Diagnostics)
	}
}

func diagnosticHasDetail(diags []diag.Diagnostic, code, needle string) bool {
	for _, d := range diags {
		if d.Code != code {
			continue
		}
		for _, detail := range d.Details {
			if strings.Contains(detail, needle) {
				return true
			}
		}
	}
	return false
}

func buildRegistryForTargetedSelection(t *testing.T) *fakeClient {
	t.Helper()
	tarballs := map[string][]byte{}
	makeVer := func(name, version string, deps map[string]string) resolver.PackageVersion {
		body := tarball(name, version, deps)
		url := "https://example.invalid/" + name + "-" + version + ".tgz"
		tarballs[url] = body
		sum := sha512sum(body)
		return resolver.PackageVersion{Name: name, Version: version, Dependencies: deps, Dist: resolver.PackageDist{Tarball: url, Integrity: "sha512-" + base64.StdEncoding.EncodeToString(sum)}}
	}
	meta := map[string]*resolver.PackageMetadata{
		"react":        {Name: "react", Versions: map[string]resolver.PackageVersion{"18.2.0": makeVer("react", "18.2.0", map[string]string{"loose-envify": "1.4.0"})}},
		"lodash":       {Name: "lodash", Versions: map[string]resolver.PackageVersion{"4.17.20": makeVer("lodash", "4.17.20", nil)}},
		"react-dom":    {Name: "react-dom", Versions: map[string]resolver.PackageVersion{"18.2.0": makeVer("react-dom", "18.2.0", nil)}},
		"loose-envify": {Name: "loose-envify", Versions: map[string]resolver.PackageVersion{"1.4.0": makeVer("loose-envify", "1.4.0", nil)}},
	}
	return &fakeClient{meta: meta, tar: tarballs}
}

func TestPackAllOrNothingValidationFailureWritesNoArtifacts(t *testing.T) {
	dir := t.TempDir()
	ir := packWorkspaceIRForM36b(true)
	writeM36bWorkspaceFiles(t, dir)
	irPath := writeIR(t, dir, ir)
	outDir := filepath.Join(dir, "out")

	result := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: outDir})
	if !hasErrCode(result.Diagnostics, "TSPACK_PACK_MISSING_PUBLISH_POLICY") {
		t.Fatalf("expected missing publish policy diagnostic: %#v", result.Diagnostics)
	}
	if result.PackResult != nil && len(result.PackResult.Artifacts) > 0 {
		t.Fatalf("expected no artifacts on validation failure: %#v", result.PackResult.Artifacts)
	}
	if _, err := os.Stat(filepath.Join(outDir, "lib-1.0.0.tgz")); !os.IsNotExist(err) {
		t.Fatalf("valid package artifact should not be written when another selected package fails")
	}
}

func TestPackPackageScopedValidPackageWritesArtifact(t *testing.T) {
	dir := t.TempDir()
	ir := packWorkspaceIRForM36b(true)
	writeM36bWorkspaceFiles(t, dir)
	irPath := writeIR(t, dir, ir)
	outDir := filepath.Join(dir, "out")

	result := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: outDir, PackageName: "lib"})
	if hasErrors(result.Diagnostics) {
		t.Fatalf("package-scoped pack failed: %#v", result.Diagnostics)
	}
	if len(result.PackResult.Artifacts) != 1 {
		t.Fatalf("expected one artifact: %#v", result.PackResult)
	}
	mustExist(t, filepath.Join(outDir, "lib-1.0.0.tgz"))
	if _, err := os.Stat(filepath.Join(outDir, "app-1.0.0.tgz")); !os.IsNotExist(err) {
		t.Fatalf("package-scoped pack wrote unselected artifact")
	}
}

func TestPackIncludeMissPolicies(t *testing.T) {
	t.Run("missing include is an error and writes no artifact", func(t *testing.T) {
		dir := t.TempDir()
		ir := simpleIR()
		ir["packages"].([]map[string]any)[0]["targets"] = []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "src/index.ts", "deps": []string{}, "peers": []string{}}}
		irPath := writeIR(t, dir, ir)
		outDir := filepath.Join(dir, "out")
		_ = os.RemoveAll(filepath.Join(dir, "dist"))
		_ = os.MkdirAll(filepath.Join(dir, "dist"), 0o755)

		result := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: outDir})
		if !hasErrCode(result.Diagnostics, "TSPACK_PACK_INCLUDE_MATCHED_NOTHING") {
			t.Fatalf("expected include miss diagnostic: %#v", result.Diagnostics)
		}
		if _, err := os.Stat(filepath.Join(outDir, "app-1.0.0.tgz")); !os.IsNotExist(err) {
			t.Fatalf("artifact should not be written when include misses")
		}
	})

	t.Run("partial include miss is an error and writes no artifact", func(t *testing.T) {
		dir := t.TempDir()
		ir := simpleIR()
		pkg := ir["packages"].([]map[string]any)[0]
		pkg["targets"] = []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "src/index.ts", "deps": []string{}, "peers": []string{}}}
		pkg["publish"] = map[string]any{"include": []string{"dist/**", "README.md"}, "exclude": []string{}}
		irPath := writeIR(t, dir, ir)
		outDir := filepath.Join(dir, "out")
		_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("readme\n"), 0o644)
		_ = os.RemoveAll(filepath.Join(dir, "dist"))
		_ = os.MkdirAll(filepath.Join(dir, "dist"), 0o755)

		result := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: outDir})
		if !hasErrCode(result.Diagnostics, "TSPACK_PACK_INCLUDE_MATCHED_NOTHING") {
			t.Fatalf("expected include miss diagnostic: %#v", result.Diagnostics)
		}
		if _, err := os.Stat(filepath.Join(outDir, "app-1.0.0.tgz")); !os.IsNotExist(err) {
			t.Fatalf("artifact should not be written when one include pattern misses")
		}
	})

	t.Run("exclude miss does not fail", func(t *testing.T) {
		dir := t.TempDir()
		ir := simpleIR()
		pkg := ir["packages"].([]map[string]any)[0]
		pkg["publish"] = map[string]any{"include": []string{"dist/**"}, "exclude": []string{"missing/**"}}
		irPath := writeIR(t, dir, ir)
		outDir := filepath.Join(dir, "out")

		result := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: outDir})
		if hasErrors(result.Diagnostics) {
			t.Fatalf("exclude miss should not fail: %#v", result.Diagnostics)
		}
		mustExist(t, filepath.Join(outDir, "app-1.0.0.tgz"))
	})
}

func TestPackDryRunValidationBehavior(t *testing.T) {
	t.Run("missing include exits validation path and writes nothing", func(t *testing.T) {
		dir := t.TempDir()
		ir := simpleIR()
		ir["packages"].([]map[string]any)[0]["targets"] = []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "src/index.ts", "deps": []string{}, "peers": []string{}}}
		irPath := writeIR(t, dir, ir)
		_ = os.RemoveAll(filepath.Join(dir, "dist"))
		_ = os.MkdirAll(filepath.Join(dir, "dist"), 0o755)

		result := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{DryRun: true})
		if !hasErrCode(result.Diagnostics, "TSPACK_PACK_INCLUDE_MATCHED_NOTHING") {
			t.Fatalf("expected dry-run include miss diagnostic: %#v", result.Diagnostics)
		}
		if _, err := os.Stat(filepath.Join(dir, "tspack-artifacts")); !os.IsNotExist(err) {
			t.Fatalf("dry run should not write artifacts")
		}
	})

	t.Run("valid dry-run prints a plan and writes nothing", func(t *testing.T) {
		dir := t.TempDir()
		irPath := writeIR(t, dir, simpleIR())

		result := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{DryRun: true})
		if hasErrors(result.Diagnostics) {
			t.Fatalf("dry run failed: %#v", result.Diagnostics)
		}
		if len(result.PackResult.Preview) == 0 {
			t.Fatalf("expected dry-run preview")
		}
		if _, err := os.Stat(filepath.Join(dir, "tspack-artifacts")); !os.IsNotExist(err) {
			t.Fatalf("dry run should not write artifacts")
		}
	})
}

func TestPackChangelogInclusionWarning(t *testing.T) {
	t.Run("omitted changelog warns and archive omits changelog", func(t *testing.T) {
		dir := t.TempDir()
		irPath := writeIR(t, dir, changelogPackIR([]string{"dist/**", "README.md", "LICENSE"}, []string{}))
		writeChangelogPackFiles(t, dir, true)

		result := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: filepath.Join(dir, "out")})
		if hasErrors(result.Diagnostics) {
			t.Fatalf("pack failed: %#v", result.Diagnostics)
		}
		if !hasDiagnosticSeverity(result.Diagnostics, "TSPACK_PACK_CHANGELOG_NOT_INCLUDED", diag.SeverityWarning) {
			t.Fatalf("expected changelog warning: %#v", result.Diagnostics)
		}
		entries := readEntries(t, result.PackResult.Artifacts[0].Path)
		mustNotContain(t, entries, "package/CHANGELOG.md")
	})

	t.Run("included changelog has no warning and archive includes changelog", func(t *testing.T) {
		dir := t.TempDir()
		irPath := writeIR(t, dir, changelogPackIR([]string{"dist/**", "README.md", "LICENSE", "CHANGELOG.md"}, []string{}))
		writeChangelogPackFiles(t, dir, true)

		result := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: filepath.Join(dir, "out")})
		if hasErrors(result.Diagnostics) {
			t.Fatalf("pack failed: %#v", result.Diagnostics)
		}
		if hasErrCode(result.Diagnostics, "TSPACK_PACK_CHANGELOG_NOT_INCLUDED") {
			t.Fatalf("unexpected changelog warning: %#v", result.Diagnostics)
		}
		entries := readEntries(t, result.PackResult.Artifacts[0].Path)
		mustContain(t, entries, "package/CHANGELOG.md")
	})

	t.Run("included then excluded warns and archive omits changelog", func(t *testing.T) {
		dir := t.TempDir()
		irPath := writeIR(t, dir, changelogPackIR([]string{"dist/**", "README.md", "LICENSE", "CHANGELOG.md"}, []string{"CHANGELOG.md"}))
		writeChangelogPackFiles(t, dir, true)

		result := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: filepath.Join(dir, "out")})
		if hasErrors(result.Diagnostics) {
			t.Fatalf("pack failed: %#v", result.Diagnostics)
		}
		if !hasDiagnosticSeverity(result.Diagnostics, "TSPACK_PACK_CHANGELOG_NOT_INCLUDED", diag.SeverityWarning) {
			t.Fatalf("expected changelog warning: %#v", result.Diagnostics)
		}
		entries := readEntries(t, result.PackResult.Artifacts[0].Path)
		mustNotContain(t, entries, "package/CHANGELOG.md")
	})

	t.Run("no changelog has no warning", func(t *testing.T) {
		dir := t.TempDir()
		irPath := writeIR(t, dir, changelogPackIR([]string{"dist/**", "README.md", "LICENSE"}, []string{}))
		writeChangelogPackFiles(t, dir, false)

		result := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: filepath.Join(dir, "out")})
		if hasErrors(result.Diagnostics) {
			t.Fatalf("pack failed: %#v", result.Diagnostics)
		}
		if hasErrCode(result.Diagnostics, "TSPACK_PACK_CHANGELOG_NOT_INCLUDED") {
			t.Fatalf("unexpected changelog warning: %#v", result.Diagnostics)
		}
	})
}

func TestPackChangelogWarningDryRunVerifyAndAllOrNothing(t *testing.T) {
	t.Run("dry-run surfaces warning and writes nothing", func(t *testing.T) {
		dir := t.TempDir()
		irPath := writeIR(t, dir, changelogPackIR([]string{"dist/**", "README.md", "LICENSE"}, []string{}))
		writeChangelogPackFiles(t, dir, true)

		result := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{DryRun: true})
		if hasErrors(result.Diagnostics) {
			t.Fatalf("dry-run failed: %#v", result.Diagnostics)
		}
		if !hasDiagnosticSeverity(result.Diagnostics, "TSPACK_PACK_CHANGELOG_NOT_INCLUDED", diag.SeverityWarning) {
			t.Fatalf("expected dry-run changelog warning: %#v", result.Diagnostics)
		}
		if len(result.PackResult.Preview) == 0 {
			t.Fatalf("expected dry-run preview")
		}
		if _, err := os.Stat(filepath.Join(dir, "tspack-artifacts")); !os.IsNotExist(err) {
			t.Fatalf("dry run should not write artifacts")
		}
	})

	t.Run("verify surfaces warning but succeeds", func(t *testing.T) {
		dir := t.TempDir()
		irPath := writeIR(t, dir, changelogPackIR([]string{"dist/**", "README.md", "LICENSE"}, []string{}))
		writeChangelogPackFiles(t, dir, true)

		result := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: filepath.Join(dir, "out"), Verify: true})
		if hasErrors(result.Diagnostics) {
			t.Fatalf("verify failed: %#v", result.Diagnostics)
		}
		if !hasDiagnosticSeverity(result.Diagnostics, "TSPACK_PACK_CHANGELOG_NOT_INCLUDED", diag.SeverityWarning) {
			t.Fatalf("expected verify changelog warning: %#v", result.Diagnostics)
		}
		if result.PackResult == nil || len(result.PackResult.Artifacts) != 1 || !result.PackResult.Artifacts[0].Verified {
			t.Fatalf("expected verified artifact: %#v", result.PackResult)
		}
	})

	t.Run("verify included changelog has no warning and archives changelog", func(t *testing.T) {
		dir := t.TempDir()
		irPath := writeIR(t, dir, changelogPackIR([]string{"dist/**", "README.md", "LICENSE", "CHANGELOG.md"}, []string{}))
		writeChangelogPackFiles(t, dir, true)

		result := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: filepath.Join(dir, "out"), Verify: true})
		if hasErrors(result.Diagnostics) {
			t.Fatalf("verify failed: %#v", result.Diagnostics)
		}
		if hasErrCode(result.Diagnostics, "TSPACK_PACK_CHANGELOG_NOT_INCLUDED") {
			t.Fatalf("unexpected changelog warning: %#v", result.Diagnostics)
		}
		entries := readEntries(t, result.PackResult.Artifacts[0].Path)
		mustContain(t, entries, "package/CHANGELOG.md")
	})

	t.Run("changelog warning alone does not block writing", func(t *testing.T) {
		dir := t.TempDir()
		irPath := writeIR(t, dir, changelogPackIR([]string{"dist/**", "README.md", "LICENSE"}, []string{}))
		writeChangelogPackFiles(t, dir, true)
		outDir := filepath.Join(dir, "out")

		result := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: outDir})
		if hasErrors(result.Diagnostics) {
			t.Fatalf("pack failed: %#v", result.Diagnostics)
		}
		if !hasErrCode(result.Diagnostics, "TSPACK_PACK_CHANGELOG_NOT_INCLUDED") {
			t.Fatalf("expected changelog warning: %#v", result.Diagnostics)
		}
		mustExist(t, filepath.Join(outDir, "app-1.0.0.tgz"))
	})

	t.Run("warning plus include miss writes no artifact", func(t *testing.T) {
		dir := t.TempDir()
		irPath := writeIR(t, dir, changelogPackIR([]string{"dist/**", "README.md", "LICENSE", "missing/**"}, []string{}))
		writeChangelogPackFiles(t, dir, true)
		outDir := filepath.Join(dir, "out")

		result := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: outDir})
		if !hasErrCode(result.Diagnostics, "TSPACK_PACK_CHANGELOG_NOT_INCLUDED") {
			t.Fatalf("expected changelog warning: %#v", result.Diagnostics)
		}
		if !hasErrCode(result.Diagnostics, "TSPACK_PACK_INCLUDE_MATCHED_NOTHING") {
			t.Fatalf("expected include miss error: %#v", result.Diagnostics)
		}
		if _, err := os.Stat(filepath.Join(outDir, "app-1.0.0.tgz")); !os.IsNotExist(err) {
			t.Fatalf("artifact should not be written when include pattern misses")
		}
	})
}

func TestPackChangelogWarningPackageScoping(t *testing.T) {
	dir := t.TempDir()
	ir := map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{
		{"name": "pkg-a", "version": "1.0.0", "root": "packages/a", "kind": "library", "dependencies": []map[string]any{}, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "dist/index.js", "types": "dist/index.d.ts", "deps": []string{}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}},
		{"name": "pkg-b", "version": "1.0.0", "root": "packages/b", "kind": "library", "dependencies": []map[string]any{}, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "dist/index.js", "types": "dist/index.d.ts", "deps": []string{}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}},
	}}
	irPath := writeIR(t, dir, ir)
	for _, pkgRoot := range []string{"packages/a", "packages/b"} {
		writePackageFiles(t, filepath.Join(dir, pkgRoot))
	}
	_ = os.WriteFile(filepath.Join(dir, "packages/a", "CHANGELOG.md"), []byte("# Changes\n"), 0o644)

	selected := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: filepath.Join(dir, "selected"), PackageName: "pkg-b"})
	if hasErrors(selected.Diagnostics) {
		t.Fatalf("selected pack failed: %#v", selected.Diagnostics)
	}
	if hasErrCode(selected.Diagnostics, "TSPACK_PACK_CHANGELOG_NOT_INCLUDED") {
		t.Fatalf("package B pack should not show package A warning: %#v", selected.Diagnostics)
	}

	all := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: filepath.Join(dir, "all")})
	if hasErrors(all.Diagnostics) {
		t.Fatalf("workspace pack failed: %#v", all.Diagnostics)
	}
	if !hasDiagnosticSeverity(all.Diagnostics, "TSPACK_PACK_CHANGELOG_NOT_INCLUDED", diag.SeverityWarning) {
		t.Fatalf("workspace pack should show package A warning: %#v", all.Diagnostics)
	}
}

func changelogPackIR(include []string, exclude []string) map[string]any {
	ir := simpleIR()
	pkg := ir["packages"].([]map[string]any)[0]
	pkg["targets"] = []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "dist/index.js", "types": "dist/index.d.ts", "deps": []string{}, "peers": []string{}}}
	pkg["publish"] = map[string]any{"include": include, "exclude": exclude}
	return ir
}

func writeChangelogPackFiles(t *testing.T, dir string, includeChangelog bool) {
	t.Helper()
	writePackageFiles(t, dir)
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("readme\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "LICENSE"), []byte("license\n"), 0o644)
	if includeChangelog {
		_ = os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte("# Changes\n"), 0o644)
	}
}

func writePackageFiles(t *testing.T, dir string) {
	t.Helper()
	_ = os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	_ = os.MkdirAll(filepath.Join(dir, "dist"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "src", "index.ts"), []byte("export const x = 1;\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "dist", "index.js"), []byte("export const x = 1;\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "dist", "index.d.ts"), []byte("export declare const x: number;\n"), 0o644)
}

func hasDiagnosticSeverity(diags []diag.Diagnostic, code string, severity diag.Severity) bool {
	for _, diagnostic := range diags {
		if diagnostic.Code == code && diagnostic.Severity == severity {
			return true
		}
	}
	return false
}

func TestPackWriteFailureLeavesNoFinalArtifact(t *testing.T) {
	dir := t.TempDir()
	irPath := writeIR(t, dir, simpleIR())
	outPath := filepath.Join(dir, "not-a-directory")
	_ = os.WriteFile(outPath, []byte("file blocks output dir\n"), 0o644)

	result := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: outPath})
	if !hasErrCode(result.Diagnostics, "TSPACK_PACK_WRITE_FAILED") {
		t.Fatalf("expected write failure diagnostic: %#v", result.Diagnostics)
	}
	if result.PackResult != nil && len(result.PackResult.Artifacts) > 0 {
		t.Fatalf("write failure should not report artifacts: %#v", result.PackResult.Artifacts)
	}
	if _, err := os.Stat(filepath.Join(outPath, "app-1.0.0.tgz")); err == nil {
		t.Fatalf("final artifact should not remain after write failure")
	}
}

func packWorkspaceIRForM36b(appMissingPublish bool) map[string]any {
	appPublish := map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}
	if appMissingPublish {
		appPublish = map[string]any{"include": []string{}, "exclude": []string{}}
	}
	return map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{
		{"name": "lib", "version": "1.0.0", "root": "packages/lib", "kind": "library", "dependencies": []map[string]any{}, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "dist/index.js", "types": "dist/index.d.ts", "deps": []string{}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}},
		{"name": "app", "version": "1.0.0", "root": "packages/app", "kind": "app", "dependencies": []map[string]any{}, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "dist/index.js", "types": "dist/index.d.ts", "deps": []string{}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": appPublish, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}},
	}}
}

func writeM36bWorkspaceFiles(t *testing.T, dir string) {
	t.Helper()
	for _, pkgRoot := range []string{"packages/lib", "packages/app"} {
		_ = os.MkdirAll(filepath.Join(dir, pkgRoot, "src"), 0o755)
		_ = os.MkdirAll(filepath.Join(dir, pkgRoot, "dist"), 0o755)
		_ = os.WriteFile(filepath.Join(dir, pkgRoot, "src", "index.ts"), []byte("export const x = 1;\n"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, pkgRoot, "dist", "index.js"), []byte("export const x = 1;\n"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, pkgRoot, "dist", "index.d.ts"), []byte("export declare const x: number;\n"), 0o644)
	}
}

func TestPackVerifyPackageScopedWritesVerifiedArtifact(t *testing.T) {
	dir := t.TempDir()
	ir := packWorkspaceIRForM36b(false)
	packages := ir["packages"].([]map[string]any)
	packages[0]["license"] = "MIT"
	packages[0]["dependencies"] = []map[string]any{
		{"key": "react", "kind": "peer", "source": map[string]any{"kind": "npm", "package": "react", "range": ">=18 <20"}},
	}
	packages[0]["targets"] = []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "dist/index.js", "types": "dist/index.d.ts", "deps": []string{}, "peers": []string{"react"}}}
	irPath := writeIR(t, dir, ir)
	writeM36bWorkspaceFiles(t, dir)
	outDir := filepath.Join(dir, "out")

	first := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: outDir, PackageName: "lib", Verify: true})
	if hasErrors(first.Diagnostics) {
		t.Fatalf("pack --verify failed: %#v", first.Diagnostics)
	}
	if first.PackResult == nil || len(first.PackResult.Artifacts) != 1 || !first.PackResult.Artifacts[0].Verified {
		t.Fatalf("expected one verified artifact: %#v", first.PackResult)
	}
	mustExist(t, filepath.Join(outDir, "lib-1.0.0.tgz"))
	if _, err := os.Stat(filepath.Join(outDir, "app-1.0.0.tgz")); !os.IsNotExist(err) {
		t.Fatalf("package-scoped verify should not write unselected package")
	}

	secondOutDir := filepath.Join(dir, "out2")
	second := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{OutputDir: secondOutDir, PackageName: "lib", Verify: true})
	if hasErrors(second.Diagnostics) || second.PackResult == nil || len(second.PackResult.Artifacts) != 1 {
		t.Fatalf("second pack --verify failed: %#v", second.Diagnostics)
	}
	if first.PackResult.Artifacts[0].Hash != second.PackResult.Artifacts[0].Hash {
		t.Fatalf("verified package hash is not deterministic: %s != %s", first.PackResult.Artifacts[0].Hash, second.PackResult.Artifacts[0].Hash)
	}
}

func TestPackDryRunVerifyRejected(t *testing.T) {
	dir := t.TempDir()
	irPath := writeIR(t, dir, simpleIR())

	result := Pack(DefaultOptionsWithIR(dir, irPath), PackOptions{DryRun: true, Verify: true})
	if !hasErrCode(result.Diagnostics, "TSPACK_PACK_INVALID_ARGS") {
		t.Fatalf("expected dry-run verify invalid args diagnostic: %#v", result.Diagnostics)
	}
	if result.PackResult != nil && len(result.PackResult.Artifacts) > 0 {
		t.Fatalf("dry-run verify should not report artifacts: %#v", result.PackResult.Artifacts)
	}
	if _, err := os.Stat(filepath.Join(dir, "tspack-artifacts")); !os.IsNotExist(err) {
		t.Fatalf("dry-run verify should not write artifacts")
	}
}

func findDiagnostic(diagnostics []diag.Diagnostic, code string) *diag.Diagnostic {
	for index := range diagnostics {
		if diagnostics[index].Code == code {
			return &diagnostics[index]
		}
	}
	return nil
}

func TestCheckLifecycleCategoryAcknowledgementPolicy(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	ir["security"] = map[string]any{
		"acknowledgedLifecycleCategories": []map[string]any{{
			"category": "maintainer-publish",
			"reason":   "Maintainer-side lifecycle scripts are blocked by TSPack.",
		}},
	}
	irPath := writeIR(t, dir, ir)
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Packages: []lockfile.Package{{ID: "npm:dep-a@1.0.0", Name: "dep-a", Version: "1.0.0", Source: "npm", Integrity: "x", Capabilities: []lockfile.Capability{
			{Kind: "lifecycleScript", Script: "prepare", Command: "node prepare.js"},
			{Kind: "lifecycleScript", Script: "postinstall", Command: "node install.js"},
		}}},
		Targets: []lockfile.Target{{Package: "app", Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts"}},
	}
	b, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(filepath.Join(dir, "ts-lock.toml"), b, 0o644)

	res := Check(DefaultOptionsWithIR(dir, irPath))
	if hasErrors(res.Diagnostics) {
		t.Fatalf("check failed: %#v", res.Diagnostics)
	}
	var prepareFound bool
	var postinstallFound bool
	for _, diagnostic := range res.Diagnostics {
		if diagnostic.Code != "TSPACK_SECURITY_LIFECYCLE_SCRIPT_PRESENT" {
			continue
		}
		details := strings.Join(diagnostic.Details, "\n")
		if strings.Contains(details, "script: prepare") {
			prepareFound = true
			if !strings.Contains(details, "acknowledgmentKind: lifecycle-category") || !strings.Contains(details, "acknowledgedByCategory: maintainer-publish") {
				t.Fatalf("prepare lifecycle diagnostic missing category acknowledgment details: %#v", diagnostic.Details)
			}
		}
		if strings.Contains(details, "script: postinstall") {
			postinstallFound = true
			if !strings.Contains(details, "acknowledgmentKind: null") || !strings.Contains(details, "acknowledged: false") {
				t.Fatalf("postinstall should remain unacknowledged by maintainer-publish policy: %#v", diagnostic.Details)
			}
		}
	}
	if !prepareFound || !postinstallFound {
		t.Fatalf("expected prepare and postinstall lifecycle diagnostics: %#v", res.Diagnostics)
	}
}

func TestCheckLifecycleCategoryAcknowledgementStaleAndUnused(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	ir["security"] = map[string]any{
		"acknowledgedLifecycleCategories": []map[string]any{{
			"category": "maintainer-publish",
			"scripts":  []string{"postinstall"},
			"reason":   "This intentionally stale fixture should be reported.",
		}},
	}
	irPath := writeIR(t, dir, ir)
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"},
		Packages: []lockfile.Package{{ID: "npm:dep-a@1.0.0", Name: "dep-a", Version: "1.0.0", Source: "npm", Integrity: "x", Capabilities: []lockfile.Capability{{Kind: "lifecycleScript", Script: "prepare", Command: "node prepare.js"}}}},
	}
	b, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(filepath.Join(dir, "ts-lock.toml"), b, 0o644)

	res := Check(DefaultOptionsWithIR(dir, irPath))
	if !hasErrCode(res.Diagnostics, "TSPACK_SECURITY_ACKNOWLEDGED_LIFECYCLE_CATEGORY_STALE") {
		t.Fatalf("expected stale lifecycle category acknowledgment diagnostic: %#v", res.Diagnostics)
	}
	if !hasErrCode(res.Diagnostics, "TSPACK_SECURITY_ACKNOWLEDGED_LIFECYCLE_CATEGORY_UNUSED") {
		t.Fatalf("expected unused lifecycle category acknowledgment diagnostic: %#v", res.Diagnostics)
	}
}

func TestTargetedUpdatePreservesUnrelatedPeerResolvedMultiVersionEntries(t *testing.T) {
	dir := t.TempDir()
	ir := map[string]any{"format": 1, "workspace": map[string]any{"name": "ws"}, "packages": []map[string]any{{"name": "app", "version": "1.0.0", "kind": "library", "dependencies": []map[string]any{{"key": "react", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "react", "range": "^18.0.0"}}, {"key": "clsx", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "clsx", "range": "^2.1.1"}}}, "targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{"react", "clsx"}, "peers": []string{}}}, "tools": []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}}}}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	opts.ResolverClient = targetedPeerRegressionRegistry(t)

	first := Update(opts)
	if hasErrors(first.Diagnostics) {
		t.Fatalf("initial update failed: %#v", first.Diagnostics)
	}

	lockBefore, _, err := lockfile.LoadFile(opts.LockfilePath)
	if err != nil {
		t.Fatalf("load initial lockfile: %v", err)
	}
	lockBefore.Packages = append(lockBefore.Packages,
		lockfile.Package{ID: "npm:react@19.2.7", Name: "react", Version: "19.2.7", Source: "npm", Integrity: "sha512-react19", Hash: "sha256:react19"},
		lockfile.Package{ID: "npm:react-dom@19.2.7", Name: "react-dom", Version: "19.2.7", Source: "npm", Integrity: "sha512-reactdom19", Hash: "sha256:reactdom19"},
		lockfile.Package{ID: "npm:scheduler@0.27.0", Name: "scheduler", Version: "0.27.0", Source: "npm", Integrity: "sha512-scheduler027", Hash: "sha256:scheduler027"},
	)
	lockBefore.Edges = append(lockBefore.Edges,
		lockfile.Edge{From: "npm:react-dom@19.2.7", To: "npm:react@19.2.7", Kind: "peer"},
		lockfile.Edge{From: "npm:react-dom@19.2.7", To: "npm:scheduler@0.27.0", Kind: "runtime"},
	)
	beforeBytes, marshalErr := lockfile.Marshal(lockBefore)
	if marshalErr != nil {
		t.Fatalf("marshal augmented lockfile: %v", marshalErr)
	}
	if err := os.WriteFile(opts.LockfilePath, beforeBytes, 0o644); err != nil {
		t.Fatalf("write augmented lockfile: %v", err)
	}

	beforeKeySet := lockPackageKeySet(lockBefore)
	result := UpdateWithOptions(opts, UpdateOptions{Query: "clsx"})
	if hasErrors(result.Diagnostics) {
		t.Fatalf("targeted update failed: %#v", result.Diagnostics)
	}
	if result.LockDiff == nil {
		t.Fatalf("expected targeted update diff")
	}
	if len(result.LockDiff.PackagesRemoved) != 0 || len(result.LockDiff.PackagesAdded) != 0 || len(result.LockDiff.PackagesChanged) != 0 {
		t.Fatalf("already-current targeted update should be a package no-op: %#v", result.LockDiff)
	}

	lockAfter, _, err := lockfile.LoadFile(opts.LockfilePath)
	if err != nil {
		t.Fatalf("load final lockfile: %v", err)
	}
	for _, id := range []string{"npm:react@19.2.7", "npm:react-dom@19.2.7", "npm:scheduler@0.27.0", "npm:clsx@2.1.1"} {
		if !lockContainsPackage(lockAfter, id) {
			t.Fatalf("targeted update did not preserve %s; lock=%#v", id, lockAfter.Packages)
		}
	}
	for id := range beforeKeySet {
		if !lockContainsPackage(lockAfter, id) {
			t.Fatalf("targeted update removed non-selected package %s", id)
		}
	}
}

func TestTargetedUpdateDryRunChangedBoolReflectsPackageDiff(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, targetedIRWithDeps([]map[string]any{{"key": "react", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "react", "range": "^18.0.0"}}}, []string{"react"}))
	client := &fakeClient{meta: map[string]*resolver.PackageMetadata{}, tar: map[string][]byte{}}
	react182 := tarball("react", "18.2.0", nil)
	react183 := tarball("react", "18.3.1", nil)
	client.meta["react"] = &resolver.PackageMetadata{Name: "react", Versions: map[string]resolver.PackageVersion{"18.2.0": {Name: "react", Version: "18.2.0", Dist: resolver.PackageDist{Tarball: "react-182", Integrity: "sha512-" + base64.StdEncoding.EncodeToString(sha512sum(react182))}}}}
	client.tar["react-182"] = react182
	client.tar["react-183"] = react183
	opts.ResolverClient = client

	initial := Update(opts)
	if hasErrors(initial.Diagnostics) {
		t.Fatalf("initial update failed: %#v", initial.Diagnostics)
	}
	noOp := UpdateDryRunWithOptions(opts, UpdateOptions{Query: "react"})
	if hasErrors(noOp.Diagnostics) {
		t.Fatalf("no-op dry run failed: %#v", noOp.Diagnostics)
	}
	if noOp.DryRun == nil || noOp.DryRun.Changed {
		t.Fatalf("expected no-op dry run changed=false, got %#v", noOp.DryRun)
	}

	client.meta["react"].Versions["18.3.1"] = resolver.PackageVersion{Name: "react", Version: "18.3.1", Dist: resolver.PackageDist{Tarball: "react-183", Integrity: "sha512-" + base64.StdEncoding.EncodeToString(sha512sum(react183))}}
	changed := UpdateDryRunWithOptions(opts, UpdateOptions{Query: "react"})
	if hasErrors(changed.Diagnostics) {
		t.Fatalf("changed dry run failed: %#v", changed.Diagnostics)
	}
	if changed.DryRun == nil || !changed.DryRun.Changed {
		t.Fatalf("expected dry run changed=true, got %#v", changed.DryRun)
	}
}

func targetedPeerRegressionRegistry(t *testing.T) *fakeClient {
	t.Helper()
	client := &fakeClient{meta: map[string]*resolver.PackageMetadata{}, tar: map[string][]byte{}}
	react := tarball("react", "18.3.1", nil)
	clsx := tarball("clsx", "2.1.1", nil)
	client.meta["react"] = &resolver.PackageMetadata{Name: "react", Versions: map[string]resolver.PackageVersion{"18.3.1": {Name: "react", Version: "18.3.1", Dist: resolver.PackageDist{Tarball: "react-183", Integrity: "sha512-" + base64.StdEncoding.EncodeToString(sha512sum(react))}}}}
	client.meta["clsx"] = &resolver.PackageMetadata{Name: "clsx", Versions: map[string]resolver.PackageVersion{"2.1.1": {Name: "clsx", Version: "2.1.1", Dist: resolver.PackageDist{Tarball: "clsx-211", Integrity: "sha512-" + base64.StdEncoding.EncodeToString(sha512sum(clsx))}}}}
	client.tar["react-183"] = react
	client.tar["clsx-211"] = clsx
	return client
}

func lockPackageKeySet(lf *lockfile.Lockfile) map[string]bool {
	out := map[string]bool{}
	for _, pkg := range lf.Packages {
		out[pkg.ID] = true
	}
	return out
}

func lockContainsPackage(lf *lockfile.Lockfile, id string) bool {
	for _, pkg := range lf.Packages {
		if pkg.ID == id {
			return true
		}
	}
	return false
}

func TestOutdatedGroupsIdenticalDeclarationsAndSeparatesKeyFields(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	packages := []map[string]any{}
	for _, spec := range []struct {
		name string
		root string
		kind string
		rng  string
	}{
		{name: "@repo/a", root: "packages/a", kind: "tool", rng: "^5.0.0"},
		{name: "@repo/b", root: "packages/b", kind: "tool", rng: "^5.0.0"},
		{name: "@repo/c", root: "packages/c", kind: "tool", rng: "^5.0.0"},
		{name: "@repo/d", root: "packages/d", kind: "dep", rng: "^5.0.0"},
		{name: "@repo/e", root: "packages/e", kind: "tool", rng: "^4.0.0"},
	} {
		packages = append(packages, map[string]any{
			"name":    spec.name,
			"root":    spec.root,
			"version": "1.0.0",
			"kind":    "library",
			"dependencies": []map[string]any{{
				"kind":   spec.kind,
				"name":   "typescript",
				"source": map[string]any{"kind": "npm", "package": "typescript", "range": spec.rng},
			}},
			"targets": []map[string]any{{"name": "core", "export": ".", "entry": "src/index.ts", "runtime": "src/index.ts", "types": "dist/index.d.ts", "deps": []string{}, "peers": []string{}}},
			"tools":   []string{}, "boundaries": []any{}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}},
		})
	}
	ir["packages"] = packages
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	for _, spec := range []string{"packages/a", "packages/b", "packages/c", "packages/d", "packages/e"} {
		_ = os.MkdirAll(filepath.Join(dir, spec, "src"), 0o755)
		_ = os.MkdirAll(filepath.Join(dir, spec, "dist"), 0o755)
		_ = os.WriteFile(filepath.Join(dir, spec, "src", "index.ts"), []byte("export const x = 1\n"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, spec, "dist", "index.d.ts"), []byte("export declare const x: number\n"), 0o644)
	}
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"}, Packages: []lockfile.Package{{ID: "npm:typescript@5.9.3", Name: "typescript", Source: "npm", Version: "5.9.3", Hash: "sha256:dummy"}}}
	lockBytes, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(opts.LockfilePath, lockBytes, 0o644)
	opts.ResolverClient = &fakeClient{meta: map[string]*resolver.PackageMetadata{"typescript": {Name: "typescript", DistTags: map[string]string{"latest": "5.9.3"}, Versions: map[string]resolver.PackageVersion{"4.9.5": {Version: "4.9.5"}, "5.9.3": {Version: "5.9.3"}}}}}

	res := Outdated(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("outdated failed: %#v", res.Diagnostics)
	}
	if len(res.Outdated.Dependencies) != 5 {
		t.Fatalf("expected five per-package dependencies, got %d", len(res.Outdated.Dependencies))
	}
	if len(res.Outdated.Groups) != 3 {
		t.Fatalf("expected grouped dependencies to separate kind/range, got %#v", res.Outdated.Groups)
	}
	var shared *OutdatedDependency
	for i := range res.Outdated.Groups {
		group := &res.Outdated.Groups[i]
		if group.Kind == "tool" && group.Requested == "^5.0.0" {
			shared = group
		}
	}
	if shared == nil || shared.PackageCount != 3 {
		t.Fatalf("expected shared tool group with package count 3, got %#v", shared)
	}
	gotNames := []string{shared.Packages[0].Name, shared.Packages[1].Name, shared.Packages[2].Name}
	wantNames := []string{"@repo/a", "@repo/b", "@repo/c"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("expected deterministic package order %v, got %v", wantNames, gotNames)
	}
}

func TestOutdatedNonRegistryDiagnosticUsesNotApplicableWording(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	pkgs := ir["packages"].([]map[string]any)
	pkgs[0]["dependencies"] = []map[string]any{
		{"kind": "dep", "name": "core", "source": map[string]any{"kind": "workspace", "package": "core"}},
	}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	client := &fakeClient{meta: map[string]*resolver.PackageMetadata{}}
	opts.ResolverClient = client
	res := Outdated(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("outdated failed: %#v", res.Diagnostics)
	}
	if client.packageMetaCalls != 0 {
		t.Fatalf("expected no registry fetch for non-registry dependency")
	}
	if !hasErrCode(res.Diagnostics, "TSPACK_OUTDATED_NON_REGISTRY_DEP") {
		t.Fatalf("expected non-registry diagnostic, got %#v", res.Diagnostics)
	}
	if strings.Contains(res.Diagnostics[0].Message, "unsupported") {
		t.Fatalf("diagnostic wording should not say unsupported: %#v", res.Diagnostics[0])
	}
	if res.Outdated.Groups[0].Status != "not_applicable" {
		t.Fatalf("expected not_applicable, got %#v", res.Outdated.Groups[0])
	}
}

func TestOutdatedUpdatePolicyEvaluation(t *testing.T) {
	dir := t.TempDir()
	ir := simpleIR()
	ir["updatePolicy"] = map[string]any{"rows": []map[string]any{
		{"name": "typescript", "kind": "tool", "strategy": "rolling", "level": "minor", "reason": "tooling can roll"},
		{"name": "vite", "kind": "tool", "strategy": "rolling", "level": "minor"},
		{"name": "react", "kind": "dep", "strategy": "manual"},
		{"name": "react-dom", "kind": "peer", "strategy": "pinned"},
	}}
	pkgs := ir["packages"].([]map[string]any)
	pkgs[0]["dependencies"] = []map[string]any{
		{"kind": "tool", "source": map[string]any{"kind": "npm", "package": "typescript", "range": "^5.8.0"}},
		{"kind": "tool", "source": map[string]any{"kind": "npm", "package": "vite", "range": "^5.4.0"}},
		{"kind": "dep", "source": map[string]any{"kind": "npm", "package": "react", "range": "^18.0.0"}},
		{"kind": "peer", "source": map[string]any{"kind": "npm", "package": "react-dom", "range": "^18.0.0"}},
		{"kind": "dep", "source": map[string]any{"kind": "npm", "package": "clsx", "range": "^1.0.0"}},
		{"kind": "dep", "source": map[string]any{"kind": "workspace", "name": "components"}},
	}
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	lf := &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"}, Packages: []lockfile.Package{
		{ID: "npm:typescript@5.8.0", Name: "typescript", Source: "npm", Version: "5.8.0", Hash: "sha256:dummy"},
		{ID: "npm:vite@5.4.21", Name: "vite", Source: "npm", Version: "5.4.21", Hash: "sha256:dummy"},
		{ID: "npm:react@18.3.1", Name: "react", Source: "npm", Version: "18.3.1", Hash: "sha256:dummy"},
		{ID: "npm:react-dom@18.3.1", Name: "react-dom", Source: "npm", Version: "18.3.1", Hash: "sha256:dummy"},
		{ID: "npm:clsx@1.0.0", Name: "clsx", Source: "npm", Version: "1.0.0", Hash: "sha256:dummy"},
	}}
	lockBytes, _ := lockfile.Marshal(lf)
	_ = os.WriteFile(opts.LockfilePath, lockBytes, 0o644)
	opts.ResolverClient = &fakeClient{meta: map[string]*resolver.PackageMetadata{
		"typescript": {Name: "typescript", DistTags: map[string]string{"latest": "5.9.3"}, Versions: map[string]resolver.PackageVersion{"5.8.0": {Version: "5.8.0"}, "5.9.3": {Version: "5.9.3"}}},
		"vite":       {Name: "vite", DistTags: map[string]string{"latest": "8.0.16"}, Versions: map[string]resolver.PackageVersion{"5.4.21": {Version: "5.4.21"}, "8.0.16": {Version: "8.0.16"}}},
		"react":      {Name: "react", DistTags: map[string]string{"latest": "19.2.7"}, Versions: map[string]resolver.PackageVersion{"18.3.1": {Version: "18.3.1"}, "19.2.7": {Version: "19.2.7"}}},
		"react-dom":  {Name: "react-dom", DistTags: map[string]string{"latest": "19.2.7"}, Versions: map[string]resolver.PackageVersion{"18.3.1": {Version: "18.3.1"}, "19.2.7": {Version: "19.2.7"}}},
		"clsx":       {Name: "clsx", DistTags: map[string]string{"latest": "2.0.0"}, Versions: map[string]resolver.PackageVersion{"1.0.0": {Version: "1.0.0"}, "2.0.0": {Version: "2.0.0"}}},
	}}
	res := Outdated(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("outdated failed: %#v", res.Diagnostics)
	}
	statuses := map[string]string{}
	for _, dep := range res.Outdated.Dependencies {
		statuses[dep.Name] = dep.PolicyStatus
	}
	want := map[string]string{"typescript": "allowed", "vite": "outside-policy-level", "react": "blocked-manual", "react-dom": "pinned", "clsx": "unclassified", "components": "not-applicable"}
	if !reflect.DeepEqual(statuses, want) {
		t.Fatalf("unexpected policy statuses: %#v", statuses)
	}
	lockAfter, _ := os.ReadFile(opts.LockfilePath)
	if !bytes.Equal(lockBytes, lockAfter) {
		t.Fatalf("outdated policy report mutated lockfile")
	}
}

func TestPolicySecurityGateLifecycleStatuses(t *testing.T) {
	baseDependency := OutdatedDependency{
		Name:                       "esbuild",
		Kind:                       "tool",
		Source:                     "npm",
		Current:                    []string{"0.21.0"},
		Latest:                     "0.25.0",
		Status:                     "wanted_available",
		PolicyStatus:               "allowed",
		CandidateMetadataAvailable: true,
	}

	noLifecycle := baseDependency
	noLifecycle.Name = "typescript"
	if gate := EvaluatePolicySecurityGate(noLifecycle, manifest.Security{}); gate.Status != "passed" {
		t.Fatalf("no lifecycle capability should pass: %#v", gate)
	}

	exactAcknowledged := baseDependency
	exactAcknowledged.Name = "biome"
	exactAcknowledged.CandidateCapabilities = []lockfile.Capability{{
		Kind:    "lifecycleScript",
		Script:  "postinstall",
		Command: "node postinstall.js",
	}}
	exactSecurity := manifest.Security{AcknowledgedCapabilities: []manifest.AcknowledgedCapability{{
		Package: "npm:biome@0.25.0",
		Kind:    "lifecycleScript",
		Script:  "postinstall",
		Command: "node postinstall.js",
		Reason:  "reviewed install hook",
	}}}
	if gate := EvaluatePolicySecurityGate(exactAcknowledged, exactSecurity); gate.Status != "passed" || gate.Diagnostics[0].AcknowledgmentKind != "capability" {
		t.Fatalf("exact acknowledged lifecycle capability should pass: %#v", gate)
	}

	categoryAcknowledged := baseDependency
	categoryAcknowledged.Name = "rollup"
	categoryAcknowledged.CandidateCapabilities = []lockfile.Capability{{
		Kind:    "lifecycleScript",
		Script:  "prepublishOnly",
		Command: "node publish-check.js",
	}}
	categorySecurity := manifest.Security{AcknowledgedLifecycleCategories: []manifest.AcknowledgedLifecycleCategory{{
		Category: "maintainer-publish",
		Reason:   "maintainer scripts are reviewed and remain blocked by default",
	}}}
	if gate := EvaluatePolicySecurityGate(categoryAcknowledged, categorySecurity); gate.Status != "passed" || gate.Diagnostics[0].AcknowledgmentKind != "lifecycle-category" {
		t.Fatalf("category acknowledged maintainer lifecycle capability should pass: %#v", gate)
	}

	needsReview := baseDependency
	needsReview.Name = "vite"
	needsReview.CandidateCapabilities = []lockfile.Capability{{
		Kind:    "lifecycleScript",
		Script:  "prepare",
		Command: "node prepare.js",
	}}
	if gate := EvaluatePolicySecurityGate(needsReview, manifest.Security{}); gate.Status != "review_required" {
		t.Fatalf("unacknowledged maintainer lifecycle capability should require review: %#v", gate)
	}

	blocked := baseDependency
	blocked.CandidateCapabilities = []lockfile.Capability{{
		Kind:    "lifecycleScript",
		Script:  "postinstall",
		Command: "node install.js",
	}}
	if gate := EvaluatePolicySecurityGate(blocked, manifest.Security{}); gate.Status != "blocked" {
		t.Fatalf("unacknowledged consumer install lifecycle capability should block: %#v", gate)
	}

	stale := blocked
	staleSecurity := manifest.Security{AcknowledgedCapabilities: []manifest.AcknowledgedCapability{{
		Package: "npm:esbuild@0.25.0",
		Kind:    "lifecycleScript",
		Script:  "postinstall",
		Command: "node old-install.js",
		Reason:  "old reviewed command",
	}}}
	if gate := EvaluatePolicySecurityGate(stale, staleSecurity); gate.Status == "passed" {
		t.Fatalf("stale exact lifecycle acknowledgment must not pass: %#v", gate)
	}
}

func TestUpdatePolicyDogfoodFixturePlanAndNoMutation(t *testing.T) {
	dir := t.TempDir()
	ir := updatePolicyDogfoodIR()
	opts := DefaultOptions(dir)
	opts.ManifestIRPath = writeIR(t, dir, ir)
	lock := updatePolicyDogfoodLockfile()
	lockBytes, err := lockfile.Marshal(lock)
	if err != nil {
		t.Fatalf("marshal dogfood lockfile: %v", err)
	}
	if err := os.WriteFile(opts.LockfilePath, lockBytes, 0o644); err != nil {
		t.Fatalf("write dogfood lockfile: %v", err)
	}
	opts.ResolverClient = updatePolicyDogfoodRegistry()
	writeDogfoodSourceFiles(t, dir)

	res := Outdated(opts)
	if hasErrors(res.Diagnostics) {
		t.Fatalf("dogfood outdated failed: %#v", res.Diagnostics)
	}
	if !res.Outdated.HasPolicy {
		t.Fatalf("dogfood fixture should expose update policy")
	}
	statuses := map[string]string{}
	for _, dep := range res.Outdated.Groups {
		statuses[dep.Name] = dep.PolicyStatus
	}
	wantStatuses := map[string]string{
		"typescript":                           "allowed",
		"vite":                                 "outside-policy-level",
		"esbuild":                              "allowed",
		"@biomejs/biome":                       "allowed",
		"rollup":                               "allowed",
		"react":                                "blocked-manual",
		"react-dom":                            "pinned",
		"@tspack-examples/update-policy-utils": "not-applicable",
	}
	for name, want := range wantStatuses {
		if statuses[name] != want {
			t.Fatalf("unexpected policy status for %s: got %q want %q; all=%#v", name, statuses[name], want, statuses)
		}
	}
	for _, group := range res.Outdated.Groups {
		if group.Name == "typescript" && group.PackageCount != 3 {
			t.Fatalf("typescript should be grouped across three packages: %#v", group)
		}
	}
	seenBlockedConsumer := false
	seenExactAcknowledged := false
	seenCategoryAcknowledged := false
	for _, group := range res.Outdated.Groups {
		gate := EvaluatePolicySecurityGate(group, res.Outdated.Security)
		switch group.Name {
		case "esbuild":
			seenBlockedConsumer = gate.Status == "blocked"
		case "@biomejs/biome":
			seenExactAcknowledged = gate.Status == "passed"
		case "rollup":
			seenCategoryAcknowledged = gate.Status == "passed"
		}
	}
	if !seenBlockedConsumer || !seenExactAcknowledged || !seenCategoryAcknowledged {
		t.Fatalf("security gates did not cover blocked/exact/category paths")
	}
	after, err := os.ReadFile(opts.LockfilePath)
	if err != nil {
		t.Fatalf("read dogfood lockfile after outdated: %v", err)
	}
	if !bytes.Equal(lockBytes, after) {
		t.Fatalf("dogfood outdated mutated lockfile")
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("dogfood outdated created materialization")
	}
	if _, err := os.Stat(opts.StoreRoot); !os.IsNotExist(err) {
		t.Fatalf("dogfood outdated populated store")
	}
}

func updatePolicyDogfoodIR() map[string]any {
	return map[string]any{
		"format":    1,
		"workspace": map[string]any{"name": "update-policy-notes", "runtime": "nodejs"},
		"security": map[string]any{
			"acknowledgedCapabilities": []map[string]any{{
				"package":         "npm:@biomejs/biome@1.10.0",
				"kind":            "lifecycleScript",
				"script":          "postinstall",
				"command":         "node ./scripts/postinstall.js",
				"reason":          "Reviewed Biome postinstall native-binary selection for this fixture.",
				"behaviorFixture": "security/biome-postinstall.xtest.ts",
			}},
			"acknowledgedLifecycleCategories": []map[string]any{{
				"category": "maintainer-publish",
				"scripts":  []string{"prepare", "prepublishOnly"},
				"reason":   "Maintainer-publish scripts are reviewed as publish-time metadata and are not run by TSPack installs.",
			}},
		},
		"updatePolicy": map[string]any{"rows": []map[string]any{
			{"name": "typescript", "kind": "tool", "strategy": "rolling", "level": "minor"},
			{"name": "vite", "kind": "tool", "strategy": "rolling", "level": "minor"},
			{"name": "esbuild", "kind": "tool", "strategy": "rolling", "level": "major"},
			{"name": "@biomejs/biome", "kind": "tool", "strategy": "rolling", "level": "minor"},
			{"name": "rollup", "kind": "tool", "strategy": "rolling", "level": "minor"},
			{"name": "react", "kind": "dep", "strategy": "manual"},
			{"name": "react-dom", "kind": "peer", "strategy": "pinned"},
		}},
		"packages": []map[string]any{
			{"name": "@tspack-examples/update-policy-app", "version": "0.1.0", "license": "MIT", "kind": "app", "dependencies": updatePolicyDogfoodDeps(true), "tools": []string{}, "boundaries": []any{}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}, "publish": map[string]any{"include": []string{}, "exclude": []string{}}, "targets": []map[string]any{{"name": "app", "export": ".", "entry": "src/app.tsx", "runtime": "public/app.js", "types": "public/app.d.ts", "deps": []string{"react", "react-dom", "@tspack-examples/update-policy-utils"}}}},
			{"name": "@tspack-examples/update-policy-lib", "version": "0.1.0", "license": "MIT", "kind": "library", "dependencies": updatePolicyDogfoodDeps(false), "tools": []string{}, "boundaries": []any{}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "targets": []map[string]any{{"name": "lib", "export": ".", "entry": "src/lib.tsx", "runtime": "dist/lib.js", "types": "dist/lib.d.ts", "deps": []string{"react", "@tspack-examples/update-policy-utils"}, "peers": []string{"react-dom"}}}},
			{"name": "@tspack-examples/update-policy-utils", "version": "0.1.0", "license": "MIT", "kind": "library", "tools": []string{}, "boundaries": []any{}, "policies": map[string]any{"types": map[string]any{}, "boundaries": map[string]any{}}, "publish": map[string]any{"include": []string{"dist/**"}, "exclude": []string{}}, "dependencies": []map[string]any{{"key": "typescript", "kind": "tool", "source": map[string]any{"kind": "npm", "package": "typescript", "range": "^5.8.0"}}, {"key": "@biomejs/biome", "kind": "tool", "source": map[string]any{"kind": "npm", "package": "@biomejs/biome", "range": "^1.9.0"}}, {"key": "rollup", "kind": "tool", "source": map[string]any{"kind": "npm", "package": "rollup", "range": "^4.20.0"}}}, "targets": []map[string]any{{"name": "utils", "export": ".", "entry": "src/utils.ts", "runtime": "dist/utils.js", "types": "dist/utils.d.ts"}}},
		},
	}
}

func updatePolicyDogfoodDeps(includeEsbuild bool) []map[string]any {
	deps := []map[string]any{
		{"key": "typescript", "kind": "tool", "source": map[string]any{"kind": "npm", "package": "typescript", "range": "^5.8.0"}},
		{"key": "vite", "kind": "tool", "source": map[string]any{"kind": "npm", "package": "vite", "range": "^5.4.0"}},
		{"key": "react", "kind": "dep", "source": map[string]any{"kind": "npm", "package": "react", "range": "^18.3.0"}},
		{"key": "react-dom", "kind": "peer", "source": map[string]any{"kind": "npm", "package": "react-dom", "range": "^18.3.0"}},
		{"key": "@tspack-examples/update-policy-utils", "kind": "dep", "source": map[string]any{"kind": "workspace", "name": "@tspack-examples/update-policy-utils"}},
	}
	if includeEsbuild {
		deps = append(deps, map[string]any{"key": "esbuild", "kind": "tool", "source": map[string]any{"kind": "npm", "package": "esbuild", "range": "^0.21.0"}})
	}
	return deps
}

func updatePolicyDogfoodLockfile() *lockfile.Lockfile {
	return &lockfile.Lockfile{Lock: lockfile.LockHeader{Format: 1, Tool: "tspack"}, Packages: []lockfile.Package{
		{ID: "npm:typescript@5.8.0", Name: "typescript", Source: "npm", Version: "5.8.0", Hash: "sha256:fixture"},
		{ID: "npm:vite@5.4.21", Name: "vite", Source: "npm", Version: "5.4.21", Hash: "sha256:fixture"},
		{ID: "npm:esbuild@0.21.0", Name: "esbuild", Source: "npm", Version: "0.21.0", Hash: "sha256:fixture"},
		{ID: "npm:@biomejs/biome@1.9.4", Name: "@biomejs/biome", Source: "npm", Version: "1.9.4", Hash: "sha256:fixture"},
		{ID: "npm:rollup@4.20.0", Name: "rollup", Source: "npm", Version: "4.20.0", Hash: "sha256:fixture", Capabilities: []lockfile.Capability{{Kind: "lifecycleScript", Script: "prepare", Command: "node ./scripts/prepare-release.js"}}},
		{ID: "npm:react@18.3.1", Name: "react", Source: "npm", Version: "18.3.1", Hash: "sha256:fixture"},
		{ID: "npm:react-dom@18.3.1", Name: "react-dom", Source: "npm", Version: "18.3.1", Hash: "sha256:fixture"},
		{ID: "workspace:@tspack-examples/update-policy-utils#fixture", Name: "@tspack-examples/update-policy-utils", Source: "workspace", Version: "0.1.0", Workspace: "@tspack-examples/update-policy-utils", Hash: "sha256:fixture"},
	}, Targets: []lockfile.Target{
		{Package: "@tspack-examples/update-policy-app", Name: "app", Export: ".", Entry: "src/app.tsx", Runtime: "public/app.js", Types: "public/app.d.ts"},
		{Package: "@tspack-examples/update-policy-lib", Name: "lib", Export: ".", Entry: "src/lib.tsx", Runtime: "dist/lib.js", Types: "dist/lib.d.ts"},
		{Package: "@tspack-examples/update-policy-utils", Name: "utils", Export: ".", Entry: "src/utils.ts", Runtime: "dist/utils.js", Types: "dist/utils.d.ts"},
	}}
}

func updatePolicyDogfoodRegistry() *fakeClient {
	return &fakeClient{meta: map[string]*resolver.PackageMetadata{
		"typescript":     {Name: "typescript", DistTags: map[string]string{"latest": "5.9.3"}, Versions: map[string]resolver.PackageVersion{"5.8.0": {Version: "5.8.0"}, "5.9.3": {Version: "5.9.3"}}},
		"vite":           {Name: "vite", DistTags: map[string]string{"latest": "8.0.16"}, Versions: map[string]resolver.PackageVersion{"5.4.21": {Version: "5.4.21"}, "8.0.16": {Version: "8.0.16"}}},
		"esbuild":        {Name: "esbuild", DistTags: map[string]string{"latest": "0.25.0"}, Versions: map[string]resolver.PackageVersion{"0.21.0": {Version: "0.21.0"}, "0.25.0": {Version: "0.25.0", Scripts: map[string]string{"postinstall": "node install.js"}}}},
		"@biomejs/biome": {Name: "@biomejs/biome", DistTags: map[string]string{"latest": "1.10.0"}, Versions: map[string]resolver.PackageVersion{"1.9.4": {Version: "1.9.4"}, "1.10.0": {Version: "1.10.0", Scripts: map[string]string{"postinstall": "node ./scripts/postinstall.js"}}}},
		"rollup":         {Name: "rollup", DistTags: map[string]string{"latest": "4.21.0"}, Versions: map[string]resolver.PackageVersion{"4.20.0": {Version: "4.20.0"}, "4.21.0": {Version: "4.21.0", Scripts: map[string]string{"prepare": "node ./scripts/prepare-release.js"}}}},
		"react":          {Name: "react", DistTags: map[string]string{"latest": "19.2.7"}, Versions: map[string]resolver.PackageVersion{"18.3.1": {Version: "18.3.1"}, "19.2.7": {Version: "19.2.7"}}},
		"react-dom":      {Name: "react-dom", DistTags: map[string]string{"latest": "19.2.7"}, Versions: map[string]resolver.PackageVersion{"18.3.1": {Version: "18.3.1"}, "19.2.7": {Version: "19.2.7"}}},
	}}
}

func writeDogfoodSourceFiles(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{
		"src/app.tsx":                         "import React from \"react\";\nexport const app = <div />;\n",
		"src/lib.tsx":                         "import React from \"react\";\nexport const lib = <span />;\n",
		"src/utils.ts":                        "export const value = 1;\n",
		"public/app.d.ts":                     "export declare const app: unknown;\n",
		"dist/lib.d.ts":                       "export declare const lib: unknown;\n",
		"dist/utils.d.ts":                     "export declare const value = 1;\n",
		"security/biome-postinstall.xtest.ts": "export {};\n",
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create dogfood dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write dogfood file: %v", err)
		}
	}
}
