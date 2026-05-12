package why

import (
	"reflect"
	"testing"

	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/graph"
	"github.com/tspack/tspack/internal/lockfile"
	"github.com/tspack/tspack/internal/manifest"
)

func TestWhyMatrix(t *testing.T) {
	g := buildGraph(t)
	lf := buildLock()

	t.Run("vue optional peer", func(t *testing.T) {
		r := Analyze(g, lf, Options{Query: "vue"})
		e := mustFindExplanation(t, r, "dependency", "app", "vue", "")
		if e.Kind != "peer" || !e.Optional {
			t.Fatalf("expected peer optional, got kind=%s optional=%v", e.Kind, e.Optional)
		}
		assertHasTarget(t, e.ReachableFrom, "vue")
		assertHasTarget(t, e.NotReachableFrom, "core", "react")
	})

	t.Run("react peer and target ambiguity", func(t *testing.T) {
		r := Analyze(g, lf, Options{Query: "react"})
		dep := mustFindExplanation(t, r, "dependency", "app", "react", "")
		assertHasTarget(t, dep.ReachableFrom, "react")
		assertHasTarget(t, dep.NotReachableFrom, "core", "vue")
		target := mustFindExplanation(t, r, "target", "app", "", "react")
		if len(target.DeclaredBy) < 2 {
			t.Fatalf("expected target dependencies/peers represented")
		}
	})

	t.Run("tool query", func(t *testing.T) {
		r := Analyze(g, lf, Options{Query: "typescript"})
		e := mustFindExplanation(t, r, "dependency", "app", "typescript", "")
		if e.Kind != "tool" {
			t.Fatalf("expected tool kind")
		}
		if len(e.ReachableFrom) != 0 {
			t.Fatalf("expected no runtime reachability for tool dep")
		}
	})

	t.Run("missing query", func(t *testing.T) {
		r := Analyze(g, lf, Options{Query: "left-pad-missing"})
		assertDiag(t, r.Diagnostics, "TSPACK_WHY_NOT_FOUND")
	})

	t.Run("lock direct id", func(t *testing.T) {
		r := Analyze(g, lf, Options{Query: "npm:react@19.1.0"})
		e := mustFindExplanation(t, r, "lock-package", "", "", "")
		if len(e.LockPackages) != 1 || e.LockPackages[0].ID != "npm:react@19.1.0" {
			t.Fatalf("expected lock package react")
		}
		if e.DirectProject == nil || !*e.DirectProject {
			t.Fatalf("expected direct project=true")
		}
	})

	t.Run("lock transitive id", func(t *testing.T) {
		r := Analyze(g, lf, Options{Query: "npm:left-pad@1.2.0"})
		e := mustFindExplanation(t, r, "lock-package", "", "", "")
		if len(e.LockEdges) == 0 {
			t.Fatalf("expected lock edges")
		}
		if e.DirectProject == nil || *e.DirectProject {
			t.Fatalf("expected direct project false")
		}
	})

	t.Run("package filter", func(t *testing.T) {
		r := Analyze(g, lf, Options{Query: "react", PackageName: "app2"})
		for _, e := range r.Explanations {
			if e.PackageName != "app2" {
				t.Fatalf("expected only app2 explanations")
			}
		}
		r2 := Analyze(g, lf, Options{Query: "react", PackageName: "missing"})
		assertDiag(t, r2.Diagnostics, "TSPACK_WHY_PACKAGE_NOT_FOUND")
	})

	t.Run("determinism", func(t *testing.T) {
		r1 := Analyze(g, lf, Options{Query: "react"})
		r2 := Analyze(g, lf, Options{Query: "react"})
		if !reflect.DeepEqual(r1, r2) {
			t.Fatalf("non-deterministic why results")
		}
	})
}

func buildGraph(t *testing.T) *graph.WorkspaceGraph {
	t.Helper()
	ir := &manifest.ManifestIR{Format: 1, Workspace: manifest.Workspace{Name: "ws"}, Packages: []manifest.Package{
		{Name: "app", Version: "1.0.0", Kind: "library", Dependencies: []manifest.DependencyIntent{{Key: "react", Kind: "peer", Source: manifest.Source{Kind: "npm", Package: "react"}}, {Key: "vue", Kind: "peer", Optional: true, Source: manifest.Source{Kind: "npm", Package: "vue"}}, {Key: "typescript", Kind: "tool", Source: manifest.Source{Kind: "npm", Package: "typescript"}}}, Targets: []manifest.Target{{Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts"}, {Name: "react", Export: "./react", Entry: "src/react.ts", Runtime: "src/react.ts", Types: "dist/react.d.ts", Peers: []string{"react"}}, {Name: "vue", Export: "./vue", Entry: "src/vue.ts", Runtime: "src/vue.ts", Types: "dist/vue.d.ts", Peers: []string{"vue"}}}, Tools: []string{"typescript"}},
		{Name: "app2", Version: "1.0.0", Kind: "library", Dependencies: []manifest.DependencyIntent{{Key: "react", Kind: "peer", Source: manifest.Source{Kind: "npm", Package: "react"}}}, Targets: []manifest.Target{{Name: "react", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts", Peers: []string{"react"}}}},
	}}
	g, d := graph.Build(ir)
	if len(d) > 0 {
		t.Fatalf("graph build diags: %#v", d)
	}
	return g
}

func buildLock() *lockfile.Lockfile {
	return &lockfile.Lockfile{Packages: []lockfile.Package{{ID: "npm:vue@3.4.0", Name: "vue", Version: "3.4.0", Source: "npm"}, {ID: "npm:react@19.1.0", Name: "react", Version: "19.1.0", Source: "npm"}, {ID: "npm:left-pad@1.2.0", Name: "left-pad", Version: "1.2.0", Source: "npm"}, {ID: "npm:dep-a@1.0.0", Name: "dep-a", Version: "1.0.0", Source: "npm"}}, Edges: []lockfile.Edge{{From: "app:target:vue", To: "npm:vue@3.4.0", Kind: "peer", Optional: true}, {From: "app:target:react", To: "npm:react@19.1.0", Kind: "peer"}, {From: "npm:dep-a@1.0.0", To: "npm:left-pad@1.2.0", Kind: "runtime"}}}
}

func mustFindExplanation(t *testing.T, r Result, matchType, pkg, dep, target string) Explanation {
	t.Helper()
	for _, e := range r.Explanations {
		if e.MatchType == matchType && (pkg == "" || e.PackageName == pkg) && (dep == "" || e.DependencyKey == dep) && (target == "" || e.TargetName == target) {
			return e
		}
	}
	t.Fatalf("explanation not found: type=%s pkg=%s dep=%s target=%s got=%#v", matchType, pkg, dep, target, r.Explanations)
	return Explanation{}
}
func assertHasTarget(t *testing.T, refs []ReachabilityRef, names ...string) {
	t.Helper()
	m := map[string]bool{}
	for _, r := range refs {
		m[r.TargetName] = true
	}
	for _, n := range names {
		if !m[n] {
			t.Fatalf("missing target %s in %#v", n, refs)
		}
	}
}
func assertDiag(t *testing.T, diags []diag.Diagnostic, code string) {
	t.Helper()
	for _, d := range diags {
		if d.Code == code {
			return
		}
	}
	t.Fatalf("missing diagnostic %s in %#v", code, diags)
}
