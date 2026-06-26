package lockfile

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/diag"
	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func hasLockDiagnosticCode(diagnostics []diag.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

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
	lf := &Lockfile{Lock: LockHeader{Format: 1, Tool: "tspack"}, Packages: []Package{{ID: "npm:a@1", Name: "a", Source: "npm", Version: "1", Integrity: "x", Capabilities: []Capability{{Kind: "lifecycleScript", Script: "postinstall", Command: "node postinstall.js"}, {Kind: "lifecycleScript", Script: "install", Command: "node install.js"}}}}}
	b, err := Marshal(lf)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	parsed, diags := Parse("roundtrip", b)
	if len(diags) != 0 {
		t.Fatalf("parse diagnostics: %#v", diags)
	}
	if !reflect.DeepEqual(parsed.Packages[0].Capabilities, []Capability{{Kind: "lifecycleScript", Script: "install", Command: "node install.js"}, {Kind: "lifecycleScript", Script: "postinstall", Command: "node postinstall.js"}}) {
		t.Fatalf("expected sorted capabilities, got %#v", parsed.Packages[0].Capabilities)
	}

	old := &Lockfile{Lock: LockHeader{Format: 1}, Packages: []Package{{ID: "npm:a@1", Name: "a", Source: "npm", Version: "1", Integrity: "x"}}}
	next := &Lockfile{Lock: LockHeader{Format: 1}, Packages: []Package{{ID: "npm:a@1", Name: "a", Source: "npm", Version: "1", Integrity: "x", Capabilities: []Capability{{Kind: "lifecycleScript", Script: "postinstall", Command: "node postinstall.js"}}}}}
	d := DiffLockfiles(old, next)
	if len(d.PackagesChanged) != 1 {
		t.Fatalf("expected package changed for added capability, got %#v", d)
	}
}

func TestCapabilityFixtureSemanticRoundTrip(t *testing.T) {
	b, err := os.ReadFile("../../fixtures/lockfiles/lifecycle-capability.ts-lock.toml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	parsed, diags := Parse("lifecycle-capability.ts-lock.toml", b)
	if len(diags) != 0 {
		t.Fatalf("parse diagnostics: %#v", diags)
	}
	encoded, err := Marshal(parsed)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	parsedAgain, diags := Parse("lifecycle-capability.roundtrip.ts-lock.toml", encoded)
	if len(diags) != 0 {
		t.Fatalf("roundtrip diagnostics: %#v", diags)
	}
	if !reflect.DeepEqual(parsed.Packages, parsedAgain.Packages) {
		t.Fatalf("package semantic roundtrip mismatch: got %#v want %#v", parsedAgain.Packages, parsed.Packages)
	}
}

func TestOldFixtureWithoutCapabilitiesParses(t *testing.T) {
	b, err := os.ReadFile("../../fixtures/lockfiles/minimal.ts-lock.toml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	parsed, diags := Parse("minimal.ts-lock.toml", b)
	if len(diags) != 0 {
		t.Fatalf("parse diagnostics: %#v", diags)
	}
	for _, pkg := range parsed.Packages {
		if len(pkg.Capabilities) != 0 {
			t.Fatalf("old fixture should not have capabilities: %#v", pkg)
		}
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

func TestCheckVersionConflicts(t *testing.T) {
	t.Run("no warning when one version", func(t *testing.T) {
		lf := &Lockfile{Packages: []Package{{ID: "npm:react@18.3.1", Source: "npm", Name: "react", Version: "18.3.1"}}}
		res := CheckVersionConflicts(lf)
		if len(res.Diagnostics) != 0 {
			t.Fatalf("expected no diagnostics, got %#v", res.Diagnostics)
		}
	})

	t.Run("warns for duplicate npm versions with both package IDs", func(t *testing.T) {
		lf := &Lockfile{Packages: []Package{
			{ID: "npm:react@18.3.1", Source: "npm", Name: "react", Version: "18.3.1"},
			{ID: "npm:react@19.2.6", Source: "npm", Name: "react", Version: "19.2.6"},
		}}
		res := CheckVersionConflicts(lf)
		if len(res.Diagnostics) != 1 {
			t.Fatalf("expected 1 diagnostic, got %#v", res.Diagnostics)
		}
		d := res.Diagnostics[0]
		if d.Code != "TSPACK_LOCK_VERSION_CONFLICT" {
			t.Fatalf("unexpected code: %#v", d)
		}
		if d.Severity != "warning" {
			t.Fatalf("expected warning severity, got %#v", d)
		}
		joined := strings.Join(d.Details, "\n")
		if !strings.Contains(joined, "npm:react@18.3.1") || !strings.Contains(joined, "npm:react@19.2.6") {
			t.Fatalf("expected both package IDs in details, got %#v", d.Details)
		}
	})

	t.Run("scoped package name groups correctly", func(t *testing.T) {
		lf := &Lockfile{Packages: []Package{
			{ID: "npm:@types/react@18.3.1", Source: "npm", Name: "@types/react", Version: "18.3.1"},
			{ID: "npm:@types/react@19.2.6", Source: "npm", Name: "@types/react", Version: "19.2.6"},
		}}
		res := CheckVersionConflicts(lf)
		if len(res.Diagnostics) != 1 {
			t.Fatalf("expected 1 diagnostic, got %#v", res.Diagnostics)
		}
		if !strings.Contains(res.Diagnostics[0].Message, "@types/react") {
			t.Fatalf("expected scoped package name in message, got %q", res.Diagnostics[0].Message)
		}
	})

	t.Run("different source kinds do not conflict", func(t *testing.T) {
		lf := &Lockfile{Packages: []Package{
			{ID: "npm:react@18.3.1", Source: "npm", Name: "react", Version: "18.3.1"},
			{ID: "workspace:react#abc", Source: "workspace", Name: "react", Version: ""},
		}}
		res := CheckVersionConflicts(lf)
		if len(res.Diagnostics) != 0 {
			t.Fatalf("expected no diagnostics, got %#v", res.Diagnostics)
		}
	})

	t.Run("missing version ignored", func(t *testing.T) {
		lf := &Lockfile{Packages: []Package{
			{ID: "git:react#abc", Source: "git", Name: "react", Version: ""},
			{ID: "git:react#def", Source: "git", Name: "react", Version: ""},
		}}
		res := CheckVersionConflicts(lf)
		if len(res.Diagnostics) != 0 {
			t.Fatalf("expected no diagnostics, got %#v", res.Diagnostics)
		}
	})
}

func TestLockfileRejectsReservedPyPISource(t *testing.T) {
	lf := []byte("[lock]\nformat = 1\n\n[[package]]\nid = 'pypi:requests@2.0.0'\nname = 'requests'\nsource = 'pypi'\nversion = '2.0.0'\n")
	_, diags := Parse("ts-lock.toml", lf)
	if !hasLockDiagnosticCode(diags, "TSPACK_LOCK_INVALID_SOURCE") {
		t.Fatalf("expected reserved pypi source to be rejected: %#v", diags)
	}
}
