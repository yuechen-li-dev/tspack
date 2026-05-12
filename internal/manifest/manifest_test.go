package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidFixtures(t *testing.T) {
	cases := []string{
		"../../fixtures/valid/minimal-library/manifest.ir.golden.json",
		"../../fixtures/valid/machinalayout-like/manifest.ir.golden.json",
		"../../fixtures/valid/git-dep/manifest.ir.golden.json",
	}
	for _, c := range cases {
		t.Run(filepath.Base(filepath.Dir(c)), func(t *testing.T) {
			b, err := os.ReadFile(c)
			if err != nil { t.Fatal(err) }
			ir, diags := LoadBytes(c, b)
			if len(diags) > 0 { t.Fatalf("unexpected diagnostics: %#v", diags) }
			if ir == nil || ir.Workspace.Name == "" || len(ir.Packages) == 0 { t.Fatalf("bad ir") }
		})
	}
}

func TestInvalidCases(t *testing.T) {
	cases := []struct{ name,json,code string }{
		{"invalid json",`{`,"TSPACK_IR_INVALID_JSON"},
		{"format",`{"format":2,"workspace":{"name":"mono"},"packages":[]}`,"TSPACK_IR_UNSUPPORTED_FORMAT"},
		{"missing workspace",`{"format":1,"workspace":{"name":""},"packages":[{"name":"p","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`,"TSPACK_IR_MISSING_WORKSPACE"},
		{"no packages",`{"format":1,"workspace":{"name":"mono"},"packages":[]}`,"TSPACK_IR_NO_PACKAGES"},
		{"bad package name",`{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"bad name","version":"1.0.0","kind":"library","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`,"TSPACK_IR_INVALID_PACKAGE_NAME"},
		{"bad version",`{"format":1,"workspace":{"name":"mono"},"packages":[{"name":"ok","version":"banana","kind":"library","dependencies":[],"targets":[],"policies":{},"boundaries":[],"tools":[],"publish":{"include":["dist/**"],"exclude":[]}}]}`,"TSPACK_IR_INVALID_PACKAGE_VERSION"},
	}
	for _,tc := range cases { t.Run(tc.name, func(t *testing.T){ _,diags:=LoadBytes("x.json",[]byte(tc.json)); found:=false; for _,d:= range diags { if d.Code==tc.code {found=true; break} }; if !found { t.Fatalf("expected code %s got %#v", tc.code, diags)}})}
}

func TestDeterministicDiagnostics(t *testing.T) {
	j := `{"format":2,"workspace":{"name":""},"packages":[]}`
	_,d1 := LoadBytes("x.json", []byte(j))
	_,d2 := LoadBytes("x.json", []byte(j))
	if string(StableDiagnosticsJSON(d1)) != string(StableDiagnosticsJSON(d2)) { t.Fatalf("nondeterministic diagnostics") }
}

func TestDependencyIdentityDerivationRule(t *testing.T) {
	cases := []struct {
		name string
		dep  DependencyIntent
		want string
	}{
		{name: "explicit key", dep: DependencyIntent{Key: "ts", Source: Source{Package: "typescript"}}, want: "ts"},
		{name: "fallback package", dep: DependencyIntent{Source: Source{Package: "typescript"}}, want: "typescript"},
		{name: "fallback workspace name", dep: DependencyIntent{Source: Source{Name: "core"}}, want: "core"},
		{name: "fallback git ref basename", dep: DependencyIntent{Source: Source{Ref: "github:acme/helper"}}, want: "helper"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := depIdentity(tc.dep)
			if got != tc.want {
				t.Fatalf("depIdentity() = %q, want %q", got, tc.want)
			}
		})
	}
}
