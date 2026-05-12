package pack

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/tspack/tspack/internal/diag"
	"github.com/tspack/tspack/internal/graph"
	"github.com/tspack/tspack/internal/manifest"
)

func TestPackUnsafeEmptyAndSymlink(t *testing.T) {
	pkg := &graph.PackageNode{Name: "app", Version: "1.0.0", Publish: manifest.PublishPolicy{Include: []string{"../evil"}}}
	r := Pack(t.TempDir(), pkg, Options{DryRun: true})
	if !hasCode(r.Diagnostics, "TSPACK_PACK_INVALID_PUBLISH_PATH") {
		t.Fatalf("expected invalid path")
	}

	d := t.TempDir()
	pkg = &graph.PackageNode{Name: "app", Version: "1.0.0", Publish: manifest.PublishPolicy{Include: []string{"nope/**"}}}
	r = Pack(d, pkg, Options{DryRun: true})
	if !hasCode(r.Diagnostics, "TSPACK_PACK_EMPTY_PACKAGE") {
		t.Fatalf("expected empty package")
	}

	d2 := t.TempDir()
	_ = os.MkdirAll(filepath.Join(d2, "dist"), 0o755)
	_ = os.WriteFile(filepath.Join(d2, "dist", "real.js"), []byte("x"), 0o644)
	if err := os.Symlink(filepath.Join(d2, "dist", "real.js"), filepath.Join(d2, "dist", "link.js")); err == nil {
		pkg = &graph.PackageNode{Name: "app", Version: "1.0.0", Publish: manifest.PublishPolicy{Include: []string{"dist/**"}}}
		r = Pack(d2, pkg, Options{DryRun: true})
		if !hasCode(r.Diagnostics, "TSPACK_PACK_SYMLINK_UNSUPPORTED") {
			t.Fatalf("expected symlink unsupported: %#v", r.Diagnostics)
		}
	}
}

func TestPackArchiveMetadataAndGeneratedPackageJSON(t *testing.T) {
	d := t.TempDir()
	_ = os.MkdirAll(filepath.Join(d, "dist"), 0o755)
	_ = os.WriteFile(filepath.Join(d, "dist", "index.js"), []byte("export{}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(d, "dist", "index.d.ts"), []byte("export declare const x: number;\n"), 0o644)
	pkg := &graph.PackageNode{Name: "pkg", Version: "1.2.3", Publish: manifest.PublishPolicy{Include: []string{"dist/**"}}, Targets: []*graph.TargetNode{{Export: ".", Runtime: "dist/index.js", Types: "dist/index.d.ts"}}}
	r := Pack(d, pkg, Options{})
	if len(r.Artifacts) != 1 {
		t.Fatalf("missing artifact")
	}
	b, _ := os.ReadFile(r.Artifacts[0].Path)
	gz, _ := gzip.NewReader(bytes.NewReader(b))
	tr := tar.NewReader(gz)
	entries := []string{}
	for {
		h, e := tr.Next()
		if e != nil {
			break
		}
		entries = append(entries, h.Name)
		if h.ModTime != time.Unix(0, 0) {
			t.Fatalf("non-normalized mtime")
		}
		if h.Name[:8] != "package/" {
			t.Fatalf("missing prefix")
		}
	}
	sorted := append([]string{}, entries...)
	sort.Strings(sorted)
	if !equal(entries, sorted) {
		t.Fatalf("entries not sorted: %v", entries)
	}
	_ = gz.Close()
	if !contains(entries, "package/package.json") {
		t.Fatalf("missing generated package.json")
	}
}

func hasCode(diags []diag.Diagnostic, c string) bool {
	for _, d := range diags {
		if d.Code == c {
			return true
		}
	}
	return false
}
func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
