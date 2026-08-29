package resolver

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/graph"
	"github.com/yuechen-li-dev/tspack/internal/lockfile"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func TestResolveGitRevBranchAndLifecycle(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := t.TempDir()
	repo, commit := createGitRepo(t, tmp, `{"name":"dep-git","version":"1.0.0","scripts":{"postinstall":"node write-marker.js"}}`)

	for name, src := range map[string]manifest.Source{
		"rev":    {Kind: "git", Repo: repo, Rev: commit},
		"branch": {Kind: "git", Repo: repo, Branch: currentBranch(t, repo)},
	} {
		t.Run(name, func(t *testing.T) {
			res := ResolveNPM(context.Background(), ResolverOptions{Mode: ResolveModeUpdate, Client: buildFakeRegistry()}, ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{{Key: "dep", Kind: "dep", Source: src}}, nil, []string{"dep"})})
			if len(res.Diagnostics) != 0 {
				t.Fatalf("unexpected diagnostics: %#v", res.Diagnostics)
			}
			if len(res.Lock.Packages) != 1 {
				t.Fatalf("unexpected package count: %d", len(res.Lock.Packages))
			}
			p := res.Lock.Packages[0]
			if p.Source != "git" || p.Rev != commit || p.TreeHash == "" {
				t.Fatalf("unexpected git package: %#v", p)
			}
			assertLifecycleCap(t, p, "postinstall")
			if _, err := os.Stat(filepath.Join(repo, "marker.txt")); !os.IsNotExist(err) {
				t.Fatalf("marker should not exist, scripts must not execute: %v", err)
			}
		})
	}
}

func TestGitNegativeDiagnostics(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := t.TempDir()
	repo, _ := createGitRepo(t, tmp, `{"name":"dep-git","version":"1.0.0"}`)
	badRepo := ResolveNPM(context.Background(), ResolverOptions{Mode: ResolveModeUpdate, Client: buildFakeRegistry()}, ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{{Key: "dep", Kind: "dep", Source: manifest.Source{Kind: "git", Repo: filepath.Join(tmp, "missing")}}}, nil, []string{"dep"})})
	mustCode(t, badRepo, "TSPACK_RESOLVE_GIT_REPO_INVALID")

	badRef := ResolveNPM(context.Background(), ResolverOptions{Mode: ResolveModeUpdate, Client: buildFakeRegistry()}, ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{{Key: "dep", Kind: "dep", Source: manifest.Source{Kind: "git", Repo: repo, Rev: "does-not-exist"}}}, nil, []string{"dep"})})
	mustCode(t, badRef, "TSPACK_RESOLVE_GIT_REF_NOT_FOUND")

	invalidPkgRepo, _ := createGitRepo(t, tmp, `{"name":`)
	badPkg := ResolveNPM(context.Background(), ResolverOptions{Mode: ResolveModeUpdate, Client: buildFakeRegistry()}, ResolveRequest{Graph: graphForDeps([]manifest.DependencyIntent{{Key: "dep", Kind: "dep", Source: manifest.Source{Kind: "git", Repo: invalidPkgRepo}}}, nil, []string{"dep"})})
	mustCode(t, badPkg, "TSPACK_RESOLVE_GIT_PACKAGE_JSON_INVALID")
}

func TestPathPositiveNegativeAndDeterminism(t *testing.T) {
	tmp := t.TempDir()
	pkgDir := filepath.Join(tmp, "dep")
	_ = os.MkdirAll(pkgDir, 0o755)
	_ = os.WriteFile(filepath.Join(pkgDir, "b.txt"), []byte("b"), 0o644)
	_ = os.WriteFile(filepath.Join(pkgDir, "a.txt"), []byte("a"), 0o644)
	_ = os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(`{"name":"dep-path","version":"1.2.3","scripts":{"postinstall":"node write-marker.js"}}`), 0o644)

	resA := ResolveNPM(context.Background(), ResolverOptions{Mode: ResolveModeUpdate, Client: buildFakeRegistry()}, ResolveRequest{Graph: localGraph(tmp, []manifest.DependencyIntent{{Key: "dep", Kind: "dep", Source: manifest.Source{Kind: "path", Path: "dep"}}}, []string{"dep"})})
	resB := ResolveNPM(context.Background(), ResolverOptions{Mode: ResolveModeUpdate, Client: buildFakeRegistry()}, ResolveRequest{Graph: localGraph(tmp, []manifest.DependencyIntent{{Key: "dep", Kind: "dep", Source: manifest.Source{Kind: "path", Path: "dep"}}}, []string{"dep"})})
	if !reflect.DeepEqual(resA.Lock, resB.Lock) {
		t.Fatalf("path lock should be deterministic")
	}
	ab, _ := lockfile.Marshal(resA.Lock)
	bb, _ := lockfile.Marshal(resB.Lock)
	if !bytes.Equal(ab, bb) {
		t.Fatalf("marshal should be byte-identical")
	}
	p := resA.Lock.Packages[0]
	assertLifecycleCap(t, p, "postinstall")
	if _, err := os.Stat(filepath.Join(pkgDir, "marker.txt")); !os.IsNotExist(err) {
		t.Fatalf("marker should not exist: %v", err)
	}

	absRes := ResolveNPM(context.Background(), ResolverOptions{Mode: ResolveModeUpdate, Client: buildFakeRegistry()}, ResolveRequest{Graph: localGraph(tmp, []manifest.DependencyIntent{{Key: "dep", Kind: "dep", Source: manifest.Source{Kind: "path", Path: pkgDir}}}, []string{"dep"})})
	mustCode(t, absRes, "TSPACK_RESOLVE_PATH_INVALID")

	outRes := ResolveNPM(context.Background(), ResolverOptions{Mode: ResolveModeUpdate, Client: buildFakeRegistry()}, ResolveRequest{Graph: localGraph(tmp, []manifest.DependencyIntent{{Key: "dep", Kind: "dep", Source: manifest.Source{Kind: "path", Path: "../outside"}}}, []string{"dep"})})
	mustCode(t, outRes, "TSPACK_RESOLVE_PATH_OUTSIDE_WORKSPACE")

	missingRes := ResolveNPM(context.Background(), ResolverOptions{Mode: ResolveModeUpdate, Client: buildFakeRegistry()}, ResolveRequest{Graph: localGraph(tmp, []manifest.DependencyIntent{{Key: "dep", Kind: "dep", Source: manifest.Source{Kind: "path", Path: "missing"}}}, []string{"dep"})})
	mustCode(t, missingRes, "TSPACK_RESOLVE_PATH_NOT_FOUND")

	invalidDir := filepath.Join(tmp, "badjson")
	_ = os.MkdirAll(invalidDir, 0o755)
	_ = os.WriteFile(filepath.Join(invalidDir, "package.json"), []byte(`{"name":`), 0o644)
	invalidRes := ResolveNPM(context.Background(), ResolverOptions{Mode: ResolveModeUpdate, Client: buildFakeRegistry()}, ResolveRequest{Graph: localGraph(tmp, []manifest.DependencyIntent{{Key: "dep", Kind: "dep", Source: manifest.Source{Kind: "path", Path: "badjson"}}}, []string{"dep"})})
	mustCode(t, invalidRes, "TSPACK_RESOLVE_PATH_PACKAGE_JSON_INVALID")
}

func TestPathDependencyCanReferenceWorkspaceSibling(t *testing.T) {
	root := t.TempDir()
	dependencyRoot := filepath.Join(root, "packages", "dependency")
	if err := os.MkdirAll(dependencyRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dependencyRoot, "package.json"), []byte(`{"name":"sibling","version":"1.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ir := &manifest.ManifestIR{
		Workspace: manifest.Workspace{Name: "workspace"},
		Packages: []manifest.Package{{
			Name:    "app",
			Version: "1.0.0",
			Root:    "packages/app",
			Kind:    "app",
			Dependencies: []manifest.DependencyIntent{{
				Key:  "sibling",
				Kind: "dep",
				Source: manifest.Source{
					Kind: "path",
					Path: "../dependency",
				},
			}},
			Targets: []manifest.Target{{Name: "app", Deps: []string{"sibling"}}},
		}},
	}
	workspaceGraph, diagnostics := graph.Build(ir)
	if len(diagnostics) > 0 {
		t.Fatalf("graph diagnostics: %#v", diagnostics)
	}

	result := ResolveNPM(
		context.Background(),
		ResolverOptions{Mode: ResolveModeUpdate, Client: buildFakeRegistry(), RootDir: root},
		ResolveRequest{Graph: workspaceGraph},
	)

	if len(result.Diagnostics) > 0 {
		t.Fatalf("resolve diagnostics: %#v", result.Diagnostics)
	}
	if len(result.Lock.Packages) != 1 || result.Lock.Packages[0].Path != "packages/dependency" {
		t.Fatalf("unexpected sibling path package: %#v", result.Lock.Packages)
	}
}

func TestPathHashIgnoresGeneratedLockfile(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"dep-path","version":"1.0.0"}`), 0o644)
	_ = os.WriteFile(filepath.Join(root, "ts-lock.toml"), []byte("# first generated lockfile\n"), 0o644)

	first, ok := hashDirectory(root)
	if !ok {
		t.Fatalf("expected first hash")
	}

	_ = os.WriteFile(filepath.Join(root, "ts-lock.toml"), []byte("# second generated lockfile\n"), 0o644)
	second, ok := hashDirectory(root)
	if !ok {
		t.Fatalf("expected second hash")
	}

	if first != second {
		t.Fatalf("expected lockfile changes to be ignored, got %q then %q", first, second)
	}
}

func TestWorkspaceResolveAndMissing(t *testing.T) {
	res := ResolveNPM(context.Background(), ResolverOptions{Mode: ResolveModeUpdate, Client: buildFakeRegistry()}, ResolveRequest{Graph: localGraph(".", []manifest.DependencyIntent{{Key: "dep", Kind: "dep", Source: manifest.Source{Kind: "workspace", Name: "app"}}}, []string{"dep"})})
	if len(res.Lock.Packages) != 1 || res.Lock.Packages[0].Source != "workspace" || res.Lock.Packages[0].Hash == "" {
		t.Fatalf("unexpected workspace package: %#v", res.Lock.Packages)
	}

	missing := ResolveNPM(context.Background(), ResolverOptions{Mode: ResolveModeUpdate, Client: buildFakeRegistry()}, ResolveRequest{Graph: localGraph(".", []manifest.DependencyIntent{{Key: "dep", Kind: "dep", Source: manifest.Source{Kind: "workspace", Name: "@none/missing"}}}, []string{"dep"})})
	mustCode(t, missing, "TSPACK_RESOLVE_WORKSPACE_PACKAGE_NOT_FOUND")
}

func TestMixedSourcesDeterminism(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := t.TempDir()
	repo, _ := createGitRepo(t, tmp, `{"name":"dep-git","version":"1.0.0"}`)
	_ = os.MkdirAll(filepath.Join(tmp, "dep"), 0o755)
	_ = os.WriteFile(filepath.Join(tmp, "dep", "package.json"), []byte(`{"name":"dep-path","version":"1.0.0"}`), 0o644)

	deps := []manifest.DependencyIntent{
		{Key: "npmdep", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "left-pad", Range: "1.0.0"}},
		{Key: "gitdep", Kind: "dep", Source: manifest.Source{Kind: "git", Repo: repo, Branch: currentBranch(t, repo)}},
		{Key: "pathdep", Kind: "dep", Source: manifest.Source{Kind: "path", Path: "dep"}},
		{Key: "wsdep", Kind: "dep", Source: manifest.Source{Kind: "workspace", Name: "app"}},
	}
	g := localGraph(tmp, deps, []string{"npmdep", "gitdep", "pathdep", "wsdep"})
	a := ResolveNPM(context.Background(), ResolverOptions{Mode: ResolveModeUpdate, Client: buildFakeRegistry()}, ResolveRequest{Graph: g})
	b := ResolveNPM(context.Background(), ResolverOptions{Mode: ResolveModeUpdate, Client: buildFakeRegistry()}, ResolveRequest{Graph: g})
	if len(a.Lock.Targets) == 0 {
		t.Fatalf("expected targets")
	}
	if !reflect.DeepEqual(a.Lock, b.Lock) {
		t.Fatalf("locks differ")
	}
	ab, _ := lockfile.Marshal(a.Lock)
	bb, _ := lockfile.Marshal(b.Lock)
	if !bytes.Equal(ab, bb) {
		t.Fatalf("marshal differs")
	}
}

func createGitRepo(t *testing.T, tmp string, pkgJSON string) (string, string) {
	t.Helper()
	repo, err := os.MkdirTemp(tmp, "repo-")
	if err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, string(out))
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte(pkgJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	run("tag", "v1.0.0")
	cmd := exec.Command("git", "-C", repo, "rev-parse", "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse: %v %s", err, string(out))
	}
	return repo, string(bytes.TrimSpace(out))
}

func currentBranch(t *testing.T, repo string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "branch", "--show-current")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("branch: %v %s", err, string(out))
	}
	b := string(bytes.TrimSpace(out))
	if b == "" {
		return "master"
	}
	return b
}

func assertLifecycleCap(t *testing.T, p lockfile.Package, detail string) {
	t.Helper()
	for _, c := range p.Capabilities {
		if c.Kind == "lifecycleScript" && c.Script == detail {
			return
		}
	}
	t.Fatalf("missing lifecycle capability %s in %#v", detail, p.Capabilities)
}

func localGraph(root string, deps []manifest.DependencyIntent, runtime []string) *graph.WorkspaceGraph {
	ir := &manifest.ManifestIR{Workspace: manifest.Workspace{Name: "ws"}, Packages: []manifest.Package{{Name: "app", Version: "1.0.0", Root: root, Kind: "library", Dependencies: deps, Targets: []manifest.Target{{Name: "main", Export: ".", Entry: "src/index.ts", Runtime: "dist/index.js", Types: "dist/index.d.ts", Deps: runtime}}}}}
	g, _ := graph.Build(ir)
	return g
}
