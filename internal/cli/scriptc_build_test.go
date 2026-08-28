package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	compilerir "github.com/yuechen-li-dev/tspack/internal/compiler"
	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func TestCompilerGlobMatchesDoubleStar(t *testing.T) {
	for _, path := range []string{"src/hot/main.ts", "src/hot/math/kernel.ts"} {
		if !compilerGlobMatches("src/hot/**", path) {
			t.Fatalf("pattern did not match %s", path)
		}
	}
	if compilerGlobMatches("src/hot/**", "src/app/main.ts") {
		t.Fatal("hot-path pattern matched normal app source")
	}
}

func TestCompilerSourceOwnershipRejectsOverlap(t *testing.T) {
	root := t.TempDir()
	writeScriptCTestFile(t, root, "src/shared.ts", "export const value = 1;\n")
	pkg := &manifest.Package{Targets: []manifest.Target{
		{Name: "one", Compiler: "tsc", Inputs: []string{"src/**"}},
		{Name: "two", Compiler: "scriptc", Inputs: []string{"src/shared.ts"}},
	}}
	err := validateCompilerSourceOwnership(root, pkg)
	if err == nil || !strings.Contains(err.Error(), "TSPACK_COMPILER_SOURCE_OVERLAP") {
		t.Fatalf("overlap error=%v", err)
	}
}

func TestCompilerSourceOwnershipRejectsCrossCompilerImport(t *testing.T) {
	root := t.TempDir()
	writeScriptCTestFile(t, root, "src/app/main.ts", `import { compute } from "../hot/compute.js";`)
	writeScriptCTestFile(t, root, "src/hot/compute.ts", "export const compute = () => 1;\n")
	pkg := &manifest.Package{Targets: []manifest.Target{
		{Name: "app", Compiler: "tsc", Inputs: []string{"src/app/**"}},
		{Name: "hot", Compiler: "scriptc", Inputs: []string{"src/hot/**"}},
	}}
	err := validateCompilerSourceOwnership(root, pkg)
	if err == nil || !strings.Contains(err.Error(), "TSPACK_COMPILER_CROSS_TARGET_SOURCE_IMPORT") {
		t.Fatalf("cross-target import error=%v", err)
	}
}

func TestBuildTargetOrderIncludesArtifactDependencies(t *testing.T) {
	pkg := &manifest.Package{Targets: []manifest.Target{
		{Name: "app", DependsOn: []string{"hot"}},
		{Name: "hot"},
	}}
	ordered, err := orderBuildTargets(pkg, "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(ordered) != 2 || ordered[0].Name != "hot" || ordered[1].Name != "app" {
		t.Fatalf("unexpected order: %#v", ordered)
	}
}

func TestScriptCConfigRejectsImplicitPackageDiscovery(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "scriptc.json")
	writeScriptCTestFile(t, root, "scriptc.json", `{"schemaVersion":1,"npmStatic":["auto"]}`)
	_, err := loadScriptCConfig(path)
	if err == nil || !strings.Contains(err.Error(), "not deterministic") {
		t.Fatalf("config error=%v", err)
	}
}

func TestPerryConfigRejectsUnsupportedOutputType(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "perry.json")
	writeScriptCTestFile(t, root, "perry.json", `{"schemaVersion":1,"outputType":"dylib"}`)
	_, err := loadPerryConfig(path)
	if err == nil || !strings.Contains(err.Error(), "outputType executable") {
		t.Fatalf("config error=%v", err)
	}
}

func TestPerryConfigAcceptsOwnedExecutableOptions(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "perry.json")
	writeScriptCTestFile(t, root, "perry.json", `{"schemaVersion":1,"target":"windows","fastMath":true,"fpContract":"fast","features":["simd"]}`)
	config, err := loadPerryConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.OutputType != "executable" || !config.FastMath || config.FPContract != "fast" {
		t.Fatalf("unexpected config: %#v", config)
	}
}

func TestScriptCStaticPolicyReadsCompilerCoverageEvidence(t *testing.T) {
	if !scriptCCoverageRequiresDynamic([]byte("runs with --dynamic   2 sites")) {
		t.Fatal("dynamic-only coverage was accepted as static")
	}
	if scriptCCoverageRequiresDynamic([]byte("fully static — this program has no dynamic remainder.")) {
		t.Fatal("fully static coverage was rejected")
	}
}

func TestTSCProjectionRebasesScriptCExcludes(t *testing.T) {
	root := t.TempDir()
	pkg := &manifest.Package{Name: "app", Targets: []manifest.Target{
		{Name: "app", Compiler: "tsc"},
		{Name: "hot", Compiler: "scriptc", Inputs: []string{"src/hot/**"}},
	}}
	config, err := projectTSCConfig(root, pkg, pkg.Targets[0], compilerir.ConfigRef{
		Kind: "file", Path: "tsconfig.json", Fingerprint: "base",
	})
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(config.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.ToSlash(string(contents)), "../../src/hot/**") {
		t.Fatalf("projection did not rebase exclude: %s", contents)
	}
}

func writeScriptCTestFile(t *testing.T, root string, relativePath string, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
