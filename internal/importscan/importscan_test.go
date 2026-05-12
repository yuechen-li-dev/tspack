package importscan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExternalPackageName(t *testing.T) {
	cases := map[string]string{"react": "react", "react/jsx-runtime": "react", "@scope/pkg": "@scope/pkg", "@scope/pkg/sub/path": "@scope/pkg"}
	for in, want := range cases {
		got, ok := ExternalPackageName(in)
		if !ok || got != want { t.Fatalf("%s -> %s,%v", in, got, ok) }
	}
	for _, in := range []string{"", "./local", "../local"} {
		if _, ok := ExternalPackageName(in); ok { t.Fatalf("expected false for %s", in) }
	}
}

func TestScanForms(t *testing.T) {
	d := t.TempDir(); p := filepath.Join(d, "a.ts")
	src := `import x from "pkg"; import type {X} from "pkg-type"; export {y} from "reexp"; export type {Z} from "retyp"; import "side"; const a=require("req"); const b=import("dyn"); const c=import(name);`
	os.WriteFile(p, []byte(src), 0o644)
	imps, diags := ScanFile(p)
	if len(imps) < 7 { t.Fatalf("imports=%v", imps) }
	if len(diags) == 0 { t.Fatal("expected dynamic warning") }
}

func TestResolveRelative(t *testing.T) {
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, "foo"), 0o755)
	os.WriteFile(filepath.Join(d, "index.ts"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(d, "foo.ts"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(d, "foo", "index.ts"), []byte(""), 0o644)
	if _, ok := ResolveRelative(filepath.Join(d, "index.ts"), "./foo"); !ok { t.Fatal("resolve foo") }
	if _, ok := ResolveRelative(filepath.Join(d, "index.ts"), "./missing"); ok { t.Fatal("expected unresolved") }
}
