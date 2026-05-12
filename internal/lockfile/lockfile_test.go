package lockfile

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tspack/tspack/internal/graph"
	"github.com/tspack/tspack/internal/manifest"
)

func TestParseAndMarshal(t *testing.T) {
	b, _ := os.ReadFile("../../fixtures/lockfiles/minimal.ts-lock.toml")
	lf, ds := Parse("minimal", b)
	if len(ds) != 0 || len(lf.Packages) != 1 {
		t.Fatalf("parse failed: %v", ds)
	}
	m1, _ := Marshal(lf)
	m2, _ := Marshal(lf)
	if string(m1) != string(m2) {
		t.Fatal("marshal not deterministic")
	}
}

func TestCapabilityRoundTripAndDiff(t *testing.T) {
	lf := &Lockfile{Lock: LockHeader{Format: 1, Tool: "tspack"}, Packages: []Package{{ID: "npm:a@1", Name: "a", Source: "npm", Version: "1", Integrity: "x", Capabilities: []Capability{{Kind: "lifecycle-script", Detail: "postinstall"}, {Kind: "lifecycle-script", Detail: "install"}}}}}
	b, err := Marshal(lf)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	parsed, diags := Parse("roundtrip", b)
	if len(diags) != 0 {
		t.Fatalf("parse diagnostics: %#v", diags)
	}
	if !reflect.DeepEqual(parsed.Packages[0].Capabilities, []Capability{{Kind: "lifecycle-script", Detail: "install"}, {Kind: "lifecycle-script", Detail: "postinstall"}}) {
		t.Fatalf("expected sorted capabilities, got %#v", parsed.Packages[0].Capabilities)
	}

	old := &Lockfile{Lock: LockHeader{Format: 1}, Packages: []Package{{ID: "npm:a@1", Name: "a", Source: "npm", Version: "1", Integrity: "x"}}}
	next := &Lockfile{Lock: LockHeader{Format: 1}, Packages: []Package{{ID: "npm:a@1", Name: "a", Source: "npm", Version: "1", Integrity: "x", Capabilities: []Capability{{Kind: "lifecycle-script", Detail: "postinstall"}}}}}
	d := DiffLockfiles(old, next)
	if len(d.PackagesChanged) != 1 {
		t.Fatalf("expected package changed for added capability, got %#v", d)
	}
}
func TestInvalids(t *testing.T) {
	cases := []struct{ f, code string }{{"invalid-bad-format.ts-lock.toml", "TSPACK_LOCK_UNSUPPORTED_FORMAT"}, {"invalid-duplicate-package.ts-lock.toml", "TSPACK_LOCK_DUPLICATE_PACKAGE"}, {"invalid-unknown-edge-ref.ts-lock.toml", "TSPACK_LOCK_UNKNOWN_PACKAGE_REF"}, {"invalid-target-path.ts-lock.toml", "TSPACK_LOCK_INVALID_PATH"}}
	for _, tc := range cases {
		b, _ := os.ReadFile(filepath.Join("../../fixtures/lockfiles", tc.f))
		_, ds := Parse(tc.f, b)
		found := false
		for _, d := range ds {
			if d.Code == tc.code {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s missing %s", tc.f, tc.code)
		}
	}
}
func TestDiff(t *testing.T) {
	a := &Lockfile{Lock: LockHeader{Format: 1}, Packages: []Package{{ID: "npm:a@1", Name: "a", Source: "npm", Version: "1", Integrity: "x"}}, Targets: []Target{{Package: "p", Name: "a", Export: "./a", Entry: "src/a.ts", Runtime: "dist/a.js", Types: "dist/a.d.ts"}}}
	b := &Lockfile{Lock: LockHeader{Format: 1}, Packages: []Package{{ID: "npm:a@2", Name: "a", Source: "npm", Version: "2", Integrity: "x"}}, Targets: []Target{{Package: "p", Name: "a", Export: "./a", Entry: "src/b.ts", Runtime: "dist/a.js", Types: "dist/a.d.ts"}}, Edges: []Edge{{From: "x", To: "npm:a@2", Kind: "dep"}}}
	d := DiffLockfiles(a, b)
	if len(d.PackagesAdded) != 1 || len(d.PackagesRemoved) != 1 || len(d.TargetsChanged) != 1 || len(d.EdgesAdded) != 1 {
		t.Fatal("unexpected diff")
	}
}
func TestConsistency(t *testing.T) {
	jb, _ := os.ReadFile("../../fixtures/valid/machinalayout-like/manifest.ir.golden.json")
	ir, di := manifest.LoadBytes("m", jb)
	if len(di) != 0 {
		t.Fatal(di)
	}
	g, gd := graph.Build(ir)
	if len(gd) != 0 {
		t.Fatal(gd)
	}
	lf := &Lockfile{Lock: LockHeader{Format: 1, Tool: "tspack"}}
	for _, p := range g.AllPackages() {
		for _, tg := range p.AllTargets() {
			lf.Targets = append(lf.Targets, Target{Package: p.Name, Name: tg.Name, Export: tg.Export, Entry: tg.Entry, Runtime: tg.Runtime, Types: tg.Types})
		}
	}
	if len(CheckGraphConsistency(g, lf).Diagnostics) != 0 {
		t.Fatal("expected consistent")
	}
	lf.Targets = lf.Targets[:len(lf.Targets)-1]
	if len(CheckGraphConsistency(g, lf).Diagnostics) == 0 {
		t.Fatal("expected stale missing")
	}
}

func TestLockfileTargetPathsRejectBareDot(t *testing.T) {
	lf := []byte("[lock]\nformat = 1\n\n[[target]]\npackage = 'p'\nname = 'core'\nexport = '.'\nentry = '.'\nruntime = 'dist/index.js'\ntypes = 'dist/index.d.ts'\n")
	_, diags := Parse("x.ts-lock.toml", lf)
	found := false
	for _, d := range diags {
		if d.Code == "TSPACK_LOCK_INVALID_PATH" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TSPACK_LOCK_INVALID_PATH, got %#v", diags)
	}
}
