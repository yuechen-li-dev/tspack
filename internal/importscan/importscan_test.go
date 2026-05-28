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
		if !ok || got != want {
			t.Fatalf("%s -> %s,%v", in, got, ok)
		}
	}
	for _, in := range []string{"", "./local", "../local"} {
		if _, ok := ExternalPackageName(in); ok {
			t.Fatalf("expected false for %s", in)
		}
	}
}

func TestScanForms(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "a.ts")
	src := `import x from "pkg"; import type {X} from "pkg-type"; export {y} from "reexp"; export type {Z} from "retyp"; import "side"; const a=require("req"); const b=import("dyn"); const c=import(name);`
	os.WriteFile(p, []byte(src), 0o644)
	imps, diags := ScanFile(p)
	if len(imps) < 7 {
		t.Fatalf("imports=%v", imps)
	}
	if len(diags) == 0 {
		t.Fatal("expected dynamic warning")
	}
}

func TestResolveRelative(t *testing.T) {
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, "foo"), 0o755)
	os.WriteFile(filepath.Join(d, "index.ts"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(d, "foo.ts"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(d, "foo", "index.ts"), []byte(""), 0o644)
	if _, ok := ResolveRelative(filepath.Join(d, "index.ts"), "./foo"); !ok {
		t.Fatal("resolve foo")
	}
	if _, ok := ResolveRelative(filepath.Join(d, "index.ts"), "./missing"); ok {
		t.Fatal("expected unresolved")
	}
}

func TestResolveRelativeTypeScriptEsmAliases(t *testing.T) {
	cases := []struct {
		name     string
		spec     string
		files    []string
		expected string
	}{
		{
			name:     "js specifier resolves to ts",
			spec:     "./button.js",
			files:    []string{"button.ts"},
			expected: "button.ts",
		},
		{
			name:     "js specifier resolves to tsx",
			spec:     "./button.js",
			files:    []string{"button.tsx"},
			expected: "button.tsx",
		},
		{
			name:     "exact js wins over ts",
			spec:     "./button.js",
			files:    []string{"button.js", "button.ts"},
			expected: "button.js",
		},
		{
			name:     "jsx specifier resolves to tsx",
			spec:     "./button.jsx",
			files:    []string{"button.tsx"},
			expected: "button.tsx",
		},
		{
			name:     "index js specifier resolves to index ts",
			spec:     "./dir/index.js",
			files:    []string{filepath.Join("dir", "index.ts")},
			expected: filepath.Join("dir", "index.ts"),
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			d := t.TempDir()
			baseFile := filepath.Join(d, "index.ts")
			writeTestFile(t, baseFile)
			for _, file := range tt.files {
				writeTestFile(t, filepath.Join(d, file))
			}

			got, ok := ResolveRelative(baseFile, tt.spec)
			if !ok {
				t.Fatalf("expected %s to resolve", tt.spec)
			}

			expected := filepath.Join(d, tt.expected)
			if got != expected {
				t.Fatalf("got %s, want %s", got, expected)
			}
		})
	}
}

func TestResolveRelativeTypeScriptEsmAliasUnresolved(t *testing.T) {
	d := t.TempDir()
	baseFile := filepath.Join(d, "index.ts")
	writeTestFile(t, baseFile)

	if got, ok := ResolveRelative(baseFile, "./missing.js"); ok {
		t.Fatalf("expected unresolved import, got %s", got)
	}
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
}
