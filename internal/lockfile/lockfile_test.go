package lockfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tspack/tspack/internal/graph"
	"github.com/tspack/tspack/internal/manifest"
)

func TestParseAndMarshal(t *testing.T) {
	b, _ := os.ReadFile("../../fixtures/lockfiles/minimal.ts-lock.toml")
	lf, ds := Parse("minimal", b)
	if len(ds) != 0 || len(lf.Packages) != 1 { t.Fatalf("parse failed: %v", ds) }
	m1, _ := Marshal(lf); m2, _ := Marshal(lf)
	if string(m1) != string(m2) { t.Fatal("marshal not deterministic") }
}
func TestInvalids(t *testing.T) {
	cases := []struct{ f, code string }{{"invalid-bad-format.ts-lock.toml", "TSPACK_LOCK_UNSUPPORTED_FORMAT"}, {"invalid-duplicate-package.ts-lock.toml", "TSPACK_LOCK_DUPLICATE_PACKAGE"}, {"invalid-unknown-edge-ref.ts-lock.toml", "TSPACK_LOCK_UNKNOWN_PACKAGE_REF"}, {"invalid-target-path.ts-lock.toml", "TSPACK_LOCK_INVALID_PATH"}}
	for _, tc := range cases { b, _ := os.ReadFile(filepath.Join("../../fixtures/lockfiles", tc.f)); _, ds := Parse(tc.f, b); found := false; for _, d := range ds { if d.Code == tc.code { found = true } }; if !found { t.Fatalf("%s missing %s", tc.f, tc.code) } }
}
func TestDiff(t *testing.T) {
	a := &Lockfile{Lock: LockHeader{Format: 1}, Packages: []Package{{ID: "npm:a@1", Name: "a", Source: "npm", Version: "1", Integrity: "x"}}, Targets: []Target{{Package: "p", Name: "a", Export: "./a", Entry: "src/a.ts", Runtime: "dist/a.js", Types: "dist/a.d.ts"}}}
	b := &Lockfile{Lock: LockHeader{Format: 1}, Packages: []Package{{ID: "npm:a@2", Name: "a", Source: "npm", Version: "2", Integrity: "x"}}, Targets: []Target{{Package: "p", Name: "a", Export: "./a", Entry: "src/b.ts", Runtime: "dist/a.js", Types: "dist/a.d.ts"}}, Edges: []Edge{{From: "x", To: "npm:a@2", Kind: "dep"}}}
	d := DiffLockfiles(a, b)
	if len(d.PackagesAdded) != 1 || len(d.PackagesRemoved) != 1 || len(d.TargetsChanged) != 1 || len(d.EdgesAdded) != 1 { t.Fatal("unexpected diff") }
}
func TestConsistency(t *testing.T) {
	jb, _ := os.ReadFile("../../fixtures/valid/machinalayout-like/manifest.ir.golden.json")
	ir, di := manifest.LoadBytes("m", jb); if len(di) != 0 { t.Fatal(di) }
	g, gd := graph.Build(ir); if len(gd) != 0 { t.Fatal(gd) }
	lf := &Lockfile{Lock: LockHeader{Format: 1, Tool: "tspack"}}
	for _, p := range g.AllPackages() { for _, tg := range p.AllTargets() { lf.Targets = append(lf.Targets, Target{Package: p.Name, Name: tg.Name, Export: tg.Export, Entry: tg.Entry, Runtime: tg.Runtime, Types: tg.Types}) } }
	if len(CheckGraphConsistency(g, lf).Diagnostics) != 0 { t.Fatal("expected consistent") }
	lf.Targets = lf.Targets[:len(lf.Targets)-1]
	if len(CheckGraphConsistency(g, lf).Diagnostics) == 0 { t.Fatal("expected stale missing") }
}
