package graph

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func loadIR(t *testing.T, p string) *manifest.ManifestIR {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	ir, diags := manifest.LoadBytes(p, b)
	if len(diags) > 0 {
		t.Fatalf("diags: %#v", diags)
	}
	return ir
}

func TestBuildMinimalLibrary(t *testing.T) {
	ir := loadIR(t, "../../fixtures/valid/minimal-library/manifest.ir.golden.json")
	g, diags := Build(ir)
	if len(diags) > 0 {
		t.Fatalf("diags: %#v", diags)
	}
	if g.WorkspaceName != "mono" {
		t.Fatal(g.WorkspaceName)
	}
	p, ok := g.Package("minimal")
	if !ok {
		t.Fatal("missing package")
	}
	if _, ok := p.Target("core"); !ok {
		t.Fatal("missing core")
	}
	if _, ok := p.Dependency("typescript"); !ok {
		t.Fatal("missing ts dep")
	}
	core, _ := p.Target("core")
	if core.AllowsDependencyKey("typescript") {
		t.Fatal("tool leaked into runtime")
	}
	if len(p.Publish.Include) == 0 {
		t.Fatal("publish not carried")
	}
}

func TestBuildMachinalayout(t *testing.T) {
	ir := loadIR(t, "../../fixtures/valid/machinalayout-like/manifest.ir.golden.json")
	g, diags := Build(ir)
	if len(diags) > 0 {
		t.Fatalf("diags: %#v", diags)
	}
	p, ok := g.Package("machinalayout")
	if !ok {
		t.Fatal("missing")
	}
	if _, ok := p.Target("core"); !ok {
		t.Fatal("core")
	}
	if _, ok := p.Target("react"); !ok {
		t.Fatal("react")
	}
	if _, ok := p.Target("vue"); !ok {
		t.Fatal("vue")
	}
	if x, _ := p.TargetByExport("."); x.Name != "core" {
		t.Fatal("export .")
	}
	if x, _ := p.TargetByExport("./react"); x.Name != "react" {
		t.Fatal("export react")
	}
	if x, _ := p.TargetByExport("./vue"); x.Name != "vue" {
		t.Fatal("export vue")
	}
	r, _ := p.Target("react")
	if !r.AllowsExternalPackageName("react") || !r.AllowsExternalPackageName("react-dom") {
		t.Fatal("react target peers")
	}
	v, _ := p.Target("vue")
	if !v.AllowsExternalPackageName("vue") {
		t.Fatal("vue target peer")
	}
	c, _ := p.Target("core")
	if c.AllowsExternalPackageName("react") || c.AllowsExternalPackageName("vue") {
		t.Fatal("core leak")
	}
	tools := []string{}
	for _, d := range p.ToolDependencies() {
		tools = append(tools, d.Key)
	}
	if !reflect.DeepEqual(tools, []string{"typescript", "vitest"}) {
		t.Fatalf("tools=%v", tools)
	}
	vueR := p.DependencyReachability("vue")
	if !vueR.OptionalOnly || len(vueR.PeerTargets) != 1 || vueR.PeerTargets[0].Name != "vue" {
		t.Fatalf("bad vue reachability: %#v", vueR)
	}
	reactR := p.DependencyReachability("react")
	if len(reactR.PeerTargets) != 1 || reactR.PeerTargets[0].Name != "react" {
		t.Fatalf("bad react reachability: %#v", reactR)
	}
	tsR := p.DependencyReachability("typescript")
	if !tsR.ToolOnly {
		t.Fatalf("ts should be tool only: %#v", tsR)
	}
}

func TestBuildGitDepAndIdentity(t *testing.T) {
	pth := "../../fixtures/valid/git-dep/manifest.ir.golden.json"
	ir := loadIR(t, pth)
	g, diags := Build(ir)
	if len(diags) > 0 {
		t.Fatalf("diags: %#v", diags)
	}
	p, _ := g.Package("gitpkg")
	d, ok := p.Dependency("helper")
	if !ok {
		t.Fatal("missing helper key")
	}
	if d.Source.Kind != "git" || d.Source.Tag != "v1.2.0" || d.Source.Ref != "github:acme/helper" {
		t.Fatalf("bad source %#v", d.Source)
	}
}

func TestDeterministicOrder(t *testing.T) {
	ir := loadIR(t, "../../fixtures/valid/machinalayout-like/manifest.ir.golden.json")
	g1, _ := Build(ir)
	g2, _ := Build(ir)
	p1, _ := g1.Package("machinalayout")
	p2, _ := g2.Package("machinalayout")
	deps1, deps2 := []string{}, []string{}
	for _, d := range p1.AllDependencies() {
		deps1 = append(deps1, d.Key)
	}
	for _, d := range p2.AllDependencies() {
		deps2 = append(deps2, d.Key)
	}
	if !reflect.DeepEqual(deps1, deps2) {
		t.Fatal("deps nondeterministic")
	}
	t1, t2 := []string{}, []string{}
	for _, x := range p1.AllTargets() {
		t1 = append(t1, x.Name)
	}
	for _, x := range p2.AllTargets() {
		t2 = append(t2, x.Name)
	}
	if !reflect.DeepEqual(t1, t2) {
		t.Fatal("targets nondeterministic")
	}
	a1, a2 := []string{}, []string{}
	for _, x := range p1.TargetsAllowingDependencyKey("react") {
		a1 = append(a1, x.Name)
	}
	for _, x := range p2.TargetsAllowingDependencyKey("react") {
		a2 = append(a2, x.Name)
	}
	if !reflect.DeepEqual(a1, a2) {
		t.Fatal("allow nondeterministic")
	}
}

func TestBuildInfersLifecycleToolRequirementsFromAuthoredDependencies(t *testing.T) {
	ir := &manifest.ManifestIR{Format: 1, Workspace: manifest.Workspace{Name: "w"}, Packages: []manifest.Package{
		{
			Name:    "root",
			Root:    ".",
			Version: "1.0.0",
			Kind:    "app",
			Dependencies: []manifest.DependencyIntent{
				{Key: "rollup", Kind: "tool", Source: manifest.Source{Kind: "npm", Package: "rollup", Range: "^4.0.0"}},
			},
		},
		{
			Name:    "lib",
			Root:    "packages/lib",
			Version: "1.0.0",
			Kind:    "library",
			Dependencies: []manifest.DependencyIntent{
				{Key: "vitest", Kind: "tool", Source: manifest.Source{Kind: "npm", Package: "vitest", Range: "^4.0.0"}},
			},
			Targets:     []manifest.Target{{Name: "package", Compiler: "rollup", Export: "."}},
			TestTargets: []manifest.TestTarget{{Name: "unit", Harness: "vitest", Sources: []string{"test/unit.test.ts"}}},
		},
	}}
	g, diagnostics := Build(ir)
	if len(diagnostics) > 0 {
		t.Fatalf("diags: %#v", diagnostics)
	}
	lib, _ := g.Package("lib")
	requirements := lib.LifecycleToolDependencies()
	if len(requirements) != 2 {
		t.Fatalf("lifecycle tools=%#v", requirements)
	}
	if requirements[0].From != "lib:target:package" || requirements[0].Dependency.Package.Name != "root" || requirements[0].Tool != "rollup" {
		t.Fatalf("build tool requirement=%#v", requirements[0])
	}
	if requirements[1].From != "lib:test:unit" || requirements[1].Dependency.Package.Name != "lib" || requirements[1].Tool != "vitest" {
		t.Fatalf("test tool requirement=%#v", requirements[1])
	}
}

func TestBuildDoesNotInferProjectManagedCompilerWhenPathIsExplicit(t *testing.T) {
	ir := &manifest.ManifestIR{Format: 1, Workspace: manifest.Workspace{Name: "w"}, Packages: []manifest.Package{{
		Name:    "lib",
		Root:    ".",
		Version: "1.0.0",
		Kind:    "library",
		Dependencies: []manifest.DependencyIntent{
			{Key: "rollup", Kind: "tool", Source: manifest.Source{Kind: "npm", Package: "rollup", Range: "^4.0.0"}},
		},
		Targets: []manifest.Target{{Name: "package", Compiler: "rollup", CompilerPath: "tools/rollup", Export: "."}},
	}}}
	g, diagnostics := Build(ir)
	if len(diagnostics) > 0 {
		t.Fatalf("diags: %#v", diagnostics)
	}
	lib, _ := g.Package("lib")
	if requirements := lib.LifecycleToolDependencies(); len(requirements) != 0 {
		t.Fatalf("explicit compiler path should not infer a package tool: %#v", requirements)
	}
}

func TestDefensiveMalformedIR(t *testing.T) {
	ir := &manifest.ManifestIR{Format: 1, Workspace: manifest.Workspace{Name: "w"}, Packages: []manifest.Package{
		{Name: "p", Version: "1.0.0", Kind: "library", Dependencies: []manifest.DependencyIntent{{Key: "dup", Kind: "dep", Source: manifest.Source{Kind: "npm", Package: "a"}}, {Key: "dup", Kind: "peer", Source: manifest.Source{Kind: "npm", Package: "b"}}, {Key: "bad", Kind: "wat", Source: manifest.Source{Kind: "npm", Package: "c"}}, {Key: "toolx", Kind: "tool", Source: manifest.Source{Kind: "npm", Package: "t"}}}, Tools: []string{"toolx"}, Targets: []manifest.Target{{Name: "core", Export: ".", Deps: []string{"toolx", "missing"}, Peers: []string{"missingpeer"}}, {Name: "core", Export: "./core2"}}},
		{Name: "p2", Version: "1.0.0", Kind: "library", Targets: []manifest.Target{{Name: "a", Export: "./x"}, {Name: "b", Export: "./x"}}},
	}}
	_, diags := Build(ir)
	codes := map[string]bool{}
	for _, d := range diags {
		codes[d.Code] = true
	}
	for _, c := range []string{"TSPACK_GRAPH_DUPLICATE_TARGET", "TSPACK_GRAPH_DUPLICATE_EXPORT", "TSPACK_GRAPH_DUPLICATE_DEPENDENCY", "TSPACK_GRAPH_UNKNOWN_DEPENDENCY_REF", "TSPACK_GRAPH_INVALID_DEPENDENCY_KIND", "TSPACK_GRAPH_INVALID_TARGET_REF"} {
		if !codes[c] {
			t.Fatalf("missing %s in %#v", c, diags)
		}
	}
}

func TestFixturePaths(t *testing.T) {
	if _, err := os.Stat(filepath.Clean("../../fixtures/valid/minimal-library/manifest.ir.golden.json")); err != nil {
		t.Fatal(err)
	}
}

func TestBuildM6BSplitWorkspace(t *testing.T) {
	ir := loadIR(t, "../../fixtures/valid/m6b-workspace-split/manifest.ir.golden.json")
	g, diags := Build(ir)
	if len(diags) > 0 {
		t.Fatalf("diags: %#v", diags)
	}
	core, ok := g.Package("@m6b/core")
	if !ok {
		t.Fatal("missing @m6b/core")
	}
	react, ok := g.Package("@m6b/react")
	if !ok {
		t.Fatal("missing @m6b/react")
	}
	if core.Root != "packages/core" {
		t.Fatalf("core root=%q", core.Root)
	}
	if react.Root != "packages/react" {
		t.Fatalf("react root=%q", react.Root)
	}
	if _, ok := core.Target("core"); !ok {
		t.Fatal("missing core target")
	}
	if _, ok := react.Target("react"); !ok {
		t.Fatal("missing react target")
	}
}

func TestDepKindAllowedAsRuntimeDependency(t *testing.T) {
	ir := &manifest.ManifestIR{Format: 1, Workspace: manifest.Workspace{Name: "w"}, Packages: []manifest.Package{{
		Name:    "p",
		Version: "1.0.0",
		Kind:    "library",
		Dependencies: []manifest.DependencyIntent{{
			Key:    "leftpad",
			Kind:   "dep",
			Source: manifest.Source{Kind: "npm", Package: "leftpad", Range: "^1.0.0"},
		}},
		Targets: []manifest.Target{{Name: "core", Export: ".", Entry: "src/index.ts", Runtime: "dist/index.js", Types: "dist/index.d.ts", Deps: []string{"leftpad"}}},
	}}}
	g, diags := Build(ir)
	if len(diags) != 0 {
		t.Fatalf("unexpected diags: %#v", diags)
	}
	p, _ := g.Package("p")
	core, _ := p.Target("core")
	if !core.AllowsDependencyKey("leftpad") {
		t.Fatal("dep should be runtime-equivalent for target deps")
	}
}

func TestAllowsExternalPackageNameMatchesDeclaredDependencyIdentifiers(t *testing.T) {
	packageNode := &PackageNode{Name: "demo"}
	target := &TargetNode{
		Name:    "core",
		Package: packageNode,
		RuntimeDeps: []*DependencyNode{
			{
				Key:  "components",
				Kind: DependencyKindWorkspace,
				Source: manifest.Source{
					Kind: "workspace",
					Name: "@prisma-ui/components",
				},
			},
			{
				Key:  "@prisma-ui/icons",
				Kind: DependencyKindWorkspace,
				Source: manifest.Source{
					Kind: "workspace",
					Name: "@prisma-ui/icons",
				},
			},
			{
				Key:  "local-utils",
				Kind: DependencyKindRuntime,
				Source: manifest.Source{
					Kind: "path",
					Name: "@prisma-ui/local-utils",
					Path: "../local-utils",
				},
			},
			{
				Key:  "aliased-react",
				Kind: DependencyKindRuntime,
				Source: manifest.Source{
					Kind:    "npm",
					Package: "react",
					Range:   "^19.0.0",
				},
			},
		},
	}

	allowedPackages := []string{
		"components",
		"@prisma-ui/components",
		"@prisma-ui/icons",
		"local-utils",
		"@prisma-ui/local-utils",
		"aliased-react",
		"react",
	}
	for _, packageName := range allowedPackages {
		if !target.AllowsExternalPackageName(packageName) {
			t.Fatalf("expected %s to be allowed", packageName)
		}
	}

	if target.AllowsExternalPackageName("@prisma-ui/undeclared") {
		t.Fatal("undeclared workspace package should not be allowed")
	}
}
