package why

import (
	"os"
	"reflect"
	"strings"
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

	t.Run("missing query with transitive lock suggestion", func(t *testing.T) {
		r := Analyze(g, lf, Options{Query: "left-pad"})
		d := mustFindDiag(t, r.Diagnostics, "TSPACK_WHY_NOT_FOUND")
		if !strings.Contains(d.Message, "why query not found: left-pad") {
			t.Fatalf("unexpected message: %s", d.Message)
		}
		assertDetailContains(t, d.Details, "matching lock packages exist:")
		assertDetailContains(t, d.Details, "  npm:left-pad@1.2.0")
		assertDetailContains(t, d.Details, "  tspack why npm:left-pad@1.2.0")
	})

	t.Run("dedupe duplicate lock edges", func(t *testing.T) {
		r := Analyze(g, buildLockWithDuplicateEdge(), Options{Query: "left-pad"})
		d := mustFindDiag(t, r.Diagnostics, "TSPACK_WHY_NOT_FOUND")
		assertDetailContains(t, d.Details, "  npm:left-pad@1.2.0")

		r2 := Analyze(g, buildLockWithDuplicateEdge(), Options{Query: "npm:left-pad@1.2.0"})
		e := mustFindExplanation(t, r2, "lock-package", "", "", "")
		if len(e.LockEdges) != 1 {
			t.Fatalf("expected single deduped lock edge, got %#v", e.LockEdges)
		}
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

	t.Run("bare transitive loose-envify suggests full lock id", func(t *testing.T) {
		r := Analyze(g, lf, Options{Query: "loose-envify"})
		d := mustFindDiag(t, r.Diagnostics, "TSPACK_WHY_NOT_FOUND")
		assertDetailContains(t, d.Details, "  npm:loose-envify@1.4.0")
		assertDetailContains(t, d.Details, "  tspack why npm:loose-envify@1.4.0")
	})

	t.Run("multiple transitive suggestions are sorted", func(t *testing.T) {
		r := Analyze(g, buildLockWithMultipleFooVersions(), Options{Query: "foo"})
		d := mustFindDiag(t, r.Diagnostics, "TSPACK_WHY_NOT_FOUND")
		expected := []string{
			"  npm:foo@1.0.0",
			"  npm:foo@2.0.0",
			"  tspack why npm:foo@1.0.0",
			"  tspack why npm:foo@2.0.0",
		}
		assertDetailsInOrder(t, d.Details, expected)
	})

	t.Run("scoped transitive name suggests full lock id", func(t *testing.T) {
		r := Analyze(g, buildLockWithScopedPackage(), Options{Query: "@scope/pkg"})
		d := mustFindDiag(t, r.Diagnostics, "TSPACK_WHY_NOT_FOUND")
		assertDetailContains(t, d.Details, "  npm:@scope/pkg@1.2.3")
		assertDetailContains(t, d.Details, "  tspack why npm:@scope/pkg@1.2.3")
	})

	t.Run("npm package name without version suggests full lock id", func(t *testing.T) {
		r := Analyze(g, lf, Options{Query: "npm:loose-envify"})
		d := mustFindDiag(t, r.Diagnostics, "TSPACK_WHY_NOT_FOUND")
		assertDetailContains(t, d.Details, "  npm:loose-envify@1.4.0")
		assertDetailContains(t, d.Details, "  tspack why npm:loose-envify@1.4.0")
	})

	t.Run("unknown query remains concise", func(t *testing.T) {
		r := Analyze(g, lf, Options{Query: "missing-everywhere"})
		d := mustFindDiag(t, r.Diagnostics, "TSPACK_WHY_NOT_FOUND")
		if len(d.Details) != 1 {
			t.Fatalf("expected concise details, got %#v", d.Details)
		}
		if strings.Contains(strings.Join(d.Details, "\n"), "npm:react") {
			t.Fatalf("unexpected lockfile dump: %#v", d.Details)
		}
	})

	t.Run("missing lockfile has no transitive suggestions", func(t *testing.T) {
		r := Analyze(g, nil, Options{Query: "loose-envify"})
		d := mustFindDiag(t, r.Diagnostics, "TSPACK_WHY_NOT_FOUND")
		if len(d.Details) != 1 {
			t.Fatalf("expected no lock suggestions without lockfile, got %#v", d.Details)
		}
	})

	t.Run("lock edges are scoped per declaration", func(t *testing.T) {
		scopedGraph := buildScopedGraph(t)
		scopedLock := buildScopedLock()
		r := Analyze(scopedGraph, scopedLock, Options{Query: "react"})

		components := mustFindExplanation(t, r, "dependency", "@prisma-ui/components", "react", "")
		if hasLockEdge(components.LockEdges, "npm:react@18.3.1", "npm:loose-envify@1.4.0", "runtime") {
			t.Fatalf("components should not include demo transitive loose-envify edge: %#v", components.LockEdges)
		}
		if !hasLockEdge(components.LockEdges, "@prisma-ui/components:target:core", "npm:react@19.2.6", "peer") {
			t.Fatalf("components missing own root edge: %#v", components.LockEdges)
		}

		demo := mustFindExplanation(t, r, "dependency", "@prisma-ui/demo", "react", "")
		if !hasLockEdge(demo.LockEdges, "npm:react@18.3.1", "npm:loose-envify@1.4.0", "runtime") {
			t.Fatalf("demo missing reachable loose-envify edge: %#v", demo.LockEdges)
		}
		if !hasLockEdge(demo.LockEdges, "@prisma-ui/demo:target:app", "npm:react@18.3.1", "runtime") {
			t.Fatalf("demo missing own root edge: %#v", demo.LockEdges)
		}
	})

	t.Run("duplicate scoped lock edges dedupe within result", func(t *testing.T) {
		scopedGraph := buildScopedGraph(t)
		scopedLock := buildScopedLock()
		scopedLock.Edges = append(scopedLock.Edges, lockfile.Edge{From: "npm:react@18.3.1", To: "npm:loose-envify@1.4.0", Kind: "runtime"})
		r := Analyze(scopedGraph, scopedLock, Options{Query: "react"})
		demo := mustFindExplanation(t, r, "dependency", "@prisma-ui/demo", "react", "")
		if countLockEdge(demo.LockEdges, "npm:react@18.3.1", "npm:loose-envify@1.4.0", "runtime") != 1 {
			t.Fatalf("expected one deduped loose-envify edge, got %#v", demo.LockEdges)
		}
	})

	t.Run("lock id transitive query shows inbound dependents", func(t *testing.T) {
		scopedGraph := buildScopedGraph(t)
		r := Analyze(scopedGraph, buildScopedLock(), Options{Query: "npm:loose-envify@1.4.0"})
		e := mustFindExplanation(t, r, "lock-package", "", "", "")
		if !hasLockEdge(e.LockEdges, "npm:react@18.3.1", "npm:loose-envify@1.4.0", "runtime") {
			t.Fatalf("lock id query missing inbound dependent: %#v", e.LockEdges)
		}
	})

	t.Run("platform package explanation keeps binary edges", func(t *testing.T) {
		platformGraph := buildPlatformGraph(t)
		platformLock := buildPlatformLock()
		r := Analyze(platformGraph, platformLock, Options{Query: "@biomejs/biome"})
		e := mustFindExplanation(t, r, "dependency", "app", "@biomejs/biome", "")
		if !hasLockEdge(e.LockEdges, "app:tool", "npm:@biomejs/biome@1.9.4", "tool") {
			t.Fatalf("platform explanation missing tool root edge: %#v", e.LockEdges)
		}
		if !hasLockEdge(e.LockEdges, "npm:@biomejs/biome@1.9.4", "npm:@biomejs/cli-linux-x64@1.9.4", "runtime") {
			t.Fatalf("platform explanation missing binary edge: %#v", e.LockEdges)
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

func TestWhyDocsMentionLockIDForm(t *testing.T) {
	content, err := os.ReadFile("../../docs/why.md")
	if err != nil {
		t.Fatalf("read docs/why.md: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "npm:<name>@<version>") {
		t.Fatalf("docs/why.md should mention lock ID query form")
	}
}

func buildGraph(t *testing.T) *graph.WorkspaceGraph {
	t.Helper()
	ir := &manifest.ManifestIR{
		Format:    1,
		Workspace: manifest.Workspace{Name: "ws"},
		Packages: []manifest.Package{
			{
				Name:    "app",
				Version: "1.0.0",
				Kind:    "library",
				Dependencies: []manifest.DependencyIntent{
					{Key: "react", Kind: "peer", Source: manifest.Source{Kind: "npm", Package: "react"}},
					{Key: "vue", Kind: "peer", Optional: true, Source: manifest.Source{Kind: "npm", Package: "vue"}},
					{Key: "typescript", Kind: "tool", Source: manifest.Source{Kind: "npm", Package: "typescript"}},
				},
				Targets: []manifest.Target{
					{Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts"},
					{Name: "react", Export: "./react", Entry: "src/react.ts", Runtime: "src/react.ts", Types: "dist/react.d.ts", Peers: []string{"react"}},
					{Name: "vue", Export: "./vue", Entry: "src/vue.ts", Runtime: "src/vue.ts", Types: "dist/vue.d.ts", Peers: []string{"vue"}},
				},
				Tools: []string{"typescript"},
			},
			{
				Name:    "app2",
				Version: "1.0.0",
				Kind:    "library",
				Dependencies: []manifest.DependencyIntent{
					{Key: "react", Kind: "peer", Source: manifest.Source{Kind: "npm", Package: "react"}},
				},
				Targets: []manifest.Target{
					{Name: "react", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts", Peers: []string{"react"}},
				},
			},
		},
	}
	g, d := graph.Build(ir)
	if len(d) > 0 {
		t.Fatalf("graph build diags: %#v", d)
	}
	return g
}

func buildLock() *lockfile.Lockfile {
	return &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:vue@3.4.0", Name: "vue", Version: "3.4.0", Source: "npm"},
			{ID: "npm:react@19.1.0", Name: "react", Version: "19.1.0", Source: "npm"},
			{ID: "npm:loose-envify@1.4.0", Name: "loose-envify", Version: "1.4.0", Source: "npm"},
			{ID: "npm:left-pad@1.2.0", Name: "left-pad", Version: "1.2.0", Source: "npm"},
			{ID: "npm:dep-a@1.0.0", Name: "dep-a", Version: "1.0.0", Source: "npm"},
		},
		Edges: []lockfile.Edge{
			{From: "app:target:vue", To: "npm:vue@3.4.0", Kind: "peer", Optional: true},
			{From: "app:target:react", To: "npm:react@19.1.0", Kind: "peer"},
			{From: "npm:react@19.1.0", To: "npm:loose-envify@1.4.0", Kind: "runtime"},
			{From: "npm:dep-a@1.0.0", To: "npm:left-pad@1.2.0", Kind: "runtime"},
		},
	}
}

func buildLockWithDuplicateEdge() *lockfile.Lockfile {
	lf := buildLock()
	lf.Edges = append(lf.Edges, lockfile.Edge{From: "npm:dep-a@1.0.0", To: "npm:left-pad@1.2.0", Kind: "runtime"})
	return lf
}

func buildLockWithMultipleFooVersions() *lockfile.Lockfile {
	lf := buildLock()
	lf.Packages = append(lf.Packages,
		lockfile.Package{ID: "npm:foo@2.0.0", Name: "foo", Version: "2.0.0", Source: "npm"},
		lockfile.Package{ID: "npm:foo@1.0.0", Name: "foo", Version: "1.0.0", Source: "npm"},
	)
	return lf
}

func buildLockWithScopedPackage() *lockfile.Lockfile {
	lf := buildLock()
	lf.Packages = append(lf.Packages, lockfile.Package{ID: "npm:@scope/pkg@1.2.3", Name: "@scope/pkg", Version: "1.2.3", Source: "npm"})
	return lf
}

func buildScopedGraph(t *testing.T) *graph.WorkspaceGraph {
	t.Helper()
	ir := &manifest.ManifestIR{
		Format:    1,
		Workspace: manifest.Workspace{Name: "ws"},
		Packages: []manifest.Package{
			{
				Name:    "@prisma-ui/components",
				Version: "1.0.0",
				Kind:    "library",
				Dependencies: []manifest.DependencyIntent{
					{Key: "react", Kind: "peer", Source: manifest.Source{Kind: "npm", Package: "react"}},
				},
				Targets: []manifest.Target{
					{Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts", Peers: []string{"react"}},
				},
			},
			{
				Name:    "@prisma-ui/demo",
				Version: "1.0.0",
				Kind:    "app",
				Dependencies: []manifest.DependencyIntent{
					{Key: "react", Kind: "runtime", Source: manifest.Source{Kind: "npm", Package: "react"}},
				},
				Targets: []manifest.Target{
					{Name: "app", Export: ".", Entry: "src/app.ts", Runtime: "src/app.ts", Types: "", Deps: []string{"react"}},
				},
			},
		},
	}
	g, d := graph.Build(ir)
	if len(d) > 0 {
		t.Fatalf("graph build diags: %#v", d)
	}
	return g
}

func buildScopedLock() *lockfile.Lockfile {
	return &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:react@19.2.6", Name: "react", Version: "19.2.6", Source: "npm"},
			{ID: "npm:react@18.3.1", Name: "react", Version: "18.3.1", Source: "npm"},
			{ID: "npm:loose-envify@1.4.0", Name: "loose-envify", Version: "1.4.0", Source: "npm"},
		},
		Edges: []lockfile.Edge{
			{From: "@prisma-ui/components:target:core", To: "npm:react@19.2.6", Kind: "peer"},
			{From: "@prisma-ui/demo:target:app", To: "npm:react@18.3.1", Kind: "runtime"},
			{From: "npm:react@18.3.1", To: "npm:loose-envify@1.4.0", Kind: "runtime"},
		},
	}
}

func buildPlatformGraph(t *testing.T) *graph.WorkspaceGraph {
	t.Helper()
	ir := &manifest.ManifestIR{
		Format:    1,
		Workspace: manifest.Workspace{Name: "ws"},
		Packages: []manifest.Package{
			{
				Name:    "app",
				Version: "1.0.0",
				Kind:    "library",
				Dependencies: []manifest.DependencyIntent{
					{Key: "@biomejs/biome", Kind: "tool", Source: manifest.Source{Kind: "npm", Package: "@biomejs/biome"}},
				},
				Targets: []manifest.Target{
					{Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "src/index.ts", Types: "dist/index.d.ts"},
				},
				Tools: []string{"@biomejs/biome"},
			},
		},
	}
	g, d := graph.Build(ir)
	if len(d) > 0 {
		t.Fatalf("graph build diags: %#v", d)
	}
	return g
}

func buildPlatformLock() *lockfile.Lockfile {
	return &lockfile.Lockfile{
		Packages: []lockfile.Package{
			{ID: "npm:@biomejs/biome@1.9.4", Name: "@biomejs/biome", Version: "1.9.4", Source: "npm"},
			{ID: "npm:@biomejs/cli-linux-x64@1.9.4", Name: "@biomejs/cli-linux-x64", Version: "1.9.4", Source: "npm"},
		},
		Edges: []lockfile.Edge{
			{From: "app:tool", To: "npm:@biomejs/biome@1.9.4", Kind: "tool"},
			{From: "npm:@biomejs/biome@1.9.4", To: "npm:@biomejs/cli-linux-x64@1.9.4", Kind: "runtime", Optional: true},
		},
	}
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

func mustFindDiag(t *testing.T, diags []diag.Diagnostic, code string) diag.Diagnostic {
	t.Helper()
	for _, d := range diags {
		if d.Code == code {
			return d
		}
	}
	t.Fatalf("missing diagnostic %s in %#v", code, diags)
	return diag.Diagnostic{}
}

func assertDetailContains(t *testing.T, details []string, expected string) {
	t.Helper()
	for _, detail := range details {
		if detail == expected {
			return
		}
	}
	t.Fatalf("expected detail %q in %#v", expected, details)
}

func assertDetailsInOrder(t *testing.T, details []string, expected []string) {
	t.Helper()
	next := 0
	for _, detail := range details {
		if next < len(expected) && detail == expected[next] {
			next++
		}
	}
	if next != len(expected) {
		t.Fatalf("expected details in order %q in %#v", expected, details)
	}
}

func hasLockEdge(edges []LockEdgeRef, from string, to string, kind string) bool {
	return countLockEdge(edges, from, to, kind) > 0
}

func countLockEdge(edges []LockEdgeRef, from string, to string, kind string) int {
	count := 0
	for _, edge := range edges {
		if edge.From == from && edge.To == to && edge.Kind == kind {
			count++
		}
	}
	return count
}
