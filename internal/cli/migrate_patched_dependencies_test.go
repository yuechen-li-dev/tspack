package cli

import (
	"strings"
	"testing"
)

func TestPnpmPatchedDependencyRecoveryIsExactAndDeterministic(t *testing.T) {
	dependency := migratedDependency{Key: "cac", PackageName: "cac", SourceName: "cac", SourceKind: "npm", Range: "^6.7.14", Kind: "tool"}
	attachPnpmPatch(&dependency, map[string]string{
		"cac@6.7.14": "patches/cac@6.7.14.patch",
	})
	got := renderDependencyCall(dependency)
	want := `tool(npm("cac", "^6.7.14"), { patch: { path: "patches/cac@6.7.14.patch", version: "6.7.14" } })`
	if got != want {
		t.Fatalf("rendered dependency = %s", got)
	}
	name, version, ok := parseExactPatchedDependencySelector("@scope/demo@1.2.3")
	if !ok || name != "@scope/demo" || version != "1.2.3" {
		t.Fatalf("scoped selector = %q %q %t", name, version, ok)
	}
	if _, _, ok := parseExactPatchedDependencySelector("demo@^1.2.3"); ok {
		t.Fatal("non-exact selector was accepted")
	}
	if strings.Contains(got, "pnpm") {
		t.Fatal("native declaration delegated patch authority")
	}
}
