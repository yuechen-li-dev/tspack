package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/tspack/tspack/internal/lockfile"
	"github.com/tspack/tspack/internal/store"
)

func BenchmarkStorePopulation(b *testing.B) {
	for _, jobs := range []int{1, 4} {
		b.Run(fmt.Sprintf("jobs_%d", jobs), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				root := b.TempDir()
				packages := benchmarkPathPackages(b, root, 16)
				st, err := store.Open(filepath.Join(root, ".tspack", "store"))
				if err != nil {
					b.Fatalf("open store: %v", err)
				}
				result := populateStoreParallel(context.Background(), st, nil, root, packages, jobs, Progress{})
				if hasErrors(result.Diagnostics) {
					b.Fatalf("populate store failed: %#v", result.Diagnostics)
				}
				if len(result.Packages) != len(packages) {
					b.Fatalf("populated %d packages, want %d", len(result.Packages), len(packages))
				}
			}
		})
	}
}

func benchmarkPathPackages(b *testing.B, root string, count int) []lockfile.Package {
	b.Helper()
	packages := make([]lockfile.Package, 0, count)
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("bench-%02d", i)
		rel := filepath.Join("vendor", name)
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(abs, 0o755); err != nil {
			b.Fatalf("mkdir fixture: %v", err)
		}
		packageJSON := fmt.Sprintf(`{"name":%q,"version":"1.0.0"}`, name)
		if err := os.WriteFile(filepath.Join(abs, "package.json"), []byte(packageJSON), 0o644); err != nil {
			b.Fatalf("write fixture: %v", err)
		}
		packages = append(packages, lockfile.Package{ID: "path:" + name, Name: name, Version: "1.0.0", Source: "path", Path: filepath.ToSlash(rel)})
	}
	return packages
}
