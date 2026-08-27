package project

import (
	"path/filepath"
	"testing"

	"github.com/yuechen-li-dev/tspack/internal/manifest"
)

func TestSelectDependencyEditTargetByNamePathAndWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	packages := []manifest.Package{
		{Name: "@repo/app", Root: "packages/app"},
		{Name: "@repo/lib", Root: "packages/lib"},
	}
	byName, diagnostics := selectDependencyEditTarget(packages, "@repo/lib", root, "", "TSPACK_ADD")
	if hasErrors(diagnostics) || byName == nil || byName.Name != "@repo/lib" {
		t.Fatalf("name target = %#v %#v", byName, diagnostics)
	}
	byPath, diagnostics := selectDependencyEditTarget(packages, "packages/lib", root, "", "TSPACK_ADD")
	if hasErrors(diagnostics) || byPath == nil || byPath.Name != "@repo/lib" {
		t.Fatalf("path target = %#v %#v", byPath, diagnostics)
	}
	fromDirectory, diagnostics := selectDependencyEditTarget(packages, "", root, filepath.Join(root, "packages", "app", "src"), "TSPACK_REMOVE")
	if hasErrors(diagnostics) || fromDirectory == nil || fromDirectory.Name != "@repo/app" {
		t.Fatalf("directory target = %#v %#v", fromDirectory, diagnostics)
	}
}
