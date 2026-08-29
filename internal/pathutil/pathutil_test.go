package pathutil

import "testing"

func TestIsSafePackageFilePath(t *testing.T) {
	valid := []string{"src/index.ts", "dist/index.d.ts", "packages/core/src/index.ts"}
	invalid := []string{"", " ", ".", "/abs", "../evil", "src/../evil", `C:\\evil`, "src\\index.ts"}
	for _, p := range valid {
		if !IsSafePackageFilePath(p) {
			t.Fatalf("expected valid file path: %q", p)
		}
	}
	for _, p := range invalid {
		if IsSafePackageFilePath(p) {
			t.Fatalf("expected invalid file path: %q", p)
		}
	}
}

func TestIsSafePackageRoot(t *testing.T) {
	valid := []string{".", "packages/core"}
	invalid := []string{"", "..", "../outside", "/abs", "packages/../outside"}
	for _, p := range valid {
		if !IsSafePackageRoot(p) {
			t.Fatalf("expected valid package root: %q", p)
		}
	}
	for _, p := range invalid {
		if IsSafePackageRoot(p) {
			t.Fatalf("expected invalid package root: %q", p)
		}
	}
}

func TestIsSafeRelativeGlob(t *testing.T) {
	valid := []string{"dist/**", "src/**/*.ts"}
	invalid := []string{"dist/../**", "../*.ts"}
	for _, p := range valid {
		if !IsSafeRelativeGlob(p) {
			t.Fatalf("expected valid glob path: %q", p)
		}
	}
	for _, p := range invalid {
		if IsSafeRelativeGlob(p) {
			t.Fatalf("expected invalid glob path: %q", p)
		}
	}
}

func TestIsSafePackageDependencyPathAllowsSiblingTraversal(t *testing.T) {
	for _, value := range []string{"./local", "../sibling", "../../fixtures/pkg"} {
		if !IsSafePackageDependencyPath(value) {
			t.Fatalf("expected valid package dependency path: %q", value)
		}
	}
	for _, value := range []string{"", ".", "/absolute", `..\sibling`} {
		if IsSafePackageDependencyPath(value) {
			t.Fatalf("expected invalid package dependency path: %q", value)
		}
	}
}
