package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdoptReportDogfoodProject(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := copyDogfoodProject(t, repo)

	cmd := exec.Command(bin, "adopt", "--report", "--root", root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("adopt report failed: %v\n%s", err, string(out))
	}
	text := string(out)
	for _, expected := range []string{
		"TSPack adoption report",
		"incremental-existing-react@0.0.0",
		"Suggested adoption mode: package-json-only",
		"build, dev, format, preview, typecheck",
		"package-lock.json (npm)",
		"manifest.tsx exists: false",
		"ts-lock.toml exists: false",
		"not TSPack RunTargets",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("report did not contain %q:\n%s", expected, text)
		}
	}
	assertNoGeneratedAdoptionFiles(t, root)
}

func TestAdoptReportJSONDogfoodProject(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := copyDogfoodProject(t, repo)

	cmd := exec.Command(bin, "adopt", "--report", "--json", "--root", root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("adopt report json failed: %v\n%s", err, string(out))
	}
	var report struct {
		PackageName           string         `json:"packageName"`
		SuggestedAdoptionMode string         `json:"suggestedAdoptionMode"`
		DependencyCounts      map[string]int `json:"dependencyCounts"`
		ManifestExists        bool           `json:"manifestExists"`
		LockfileExists        bool           `json:"lockfileExists"`
		Scripts               []string       `json:"scripts"`
		Lockfiles             []struct {
			Name           string `json:"name"`
			PackageManager string `json:"packageManager"`
		} `json:"lockfiles"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("decode report json: %v\n%s", err, string(out))
	}
	if report.PackageName != "incremental-existing-react" || report.SuggestedAdoptionMode != "package-json-only" {
		t.Fatalf("unexpected JSON report: %#v", report)
	}
	if report.ManifestExists || report.LockfileExists {
		t.Fatalf("dogfood project should not have TSPack files: %#v", report)
	}
	if report.DependencyCounts["dependencies"] == 0 || report.DependencyCounts["devDependencies"] == 0 {
		t.Fatalf("dependency counts were not reported: %#v", report.DependencyCounts)
	}
	if len(report.Scripts) == 0 || len(report.Lockfiles) != 1 || report.Lockfiles[0].Name != "package-lock.json" {
		t.Fatalf("scripts or lockfiles were not reported: %#v", report)
	}
	assertNoGeneratedAdoptionFiles(t, root)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func copyDogfoodProject(t *testing.T, repo string) string {
	t.Helper()
	src := filepath.Join(repo, "examples", "incremental-existing-react")
	dst := filepath.Join(t.TempDir(), "incremental-existing-react")
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		copyPath(t, filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name()))
	}
	return dst
}

func copyPath(t *testing.T, src string, dst string) {
	t.Helper()
	info, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	if info.IsDir() {
		entries, err := os.ReadDir(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(dst, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			copyPath(t, filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name()))
		}
		return
	}
	copyFile(t, src, dst)
}

func assertNoGeneratedAdoptionFiles(t *testing.T, root string) {
	t.Helper()
	for _, name := range []string{"manifest.tsx", "ts-lock.toml"} {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("adopt report unexpectedly wrote %s", path)
		}
	}
}

func TestAdoptSuggestPackageCommandPrintsAndWritesNothing(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := t.TempDir()
	pkgRoot := filepath.Join(root, "packages", "ui")
	if err := os.MkdirAll(pkgRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	packageJSON := `{"dependencies":{"clsx":"^2.1.1","react":"^19.0.0"},"devDependencies":{"typescript":"^5.9.0"},"peerDependencies":{"react-dom":"^19.0.0"}}`
	packageJSONPath := filepath.Join(pkgRoot, "package.json")
	if err := os.WriteFile(packageJSONPath, []byte(packageJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "adopt", "--suggest-package", "packages/ui", "--root", root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("adopt suggest failed: %v\n%s", err, string(out))
	}
	text := string(out)
	for _, expected := range []string{"annotatePackage(", "clsx: dep(", "react: dep(", "reactDom: peer(", "typescript: tool("} {
		if !strings.Contains(text, expected) {
			t.Fatalf("suggest output missing %q:\n%s", expected, text)
		}
	}
	if _, err := os.Stat(filepath.Join(pkgRoot, "package.manifest.tsx")); !os.IsNotExist(err) {
		t.Fatalf("suggest command wrote package.manifest.tsx or stat failed: %v", err)
	}
	afterPackageJSON, err := os.ReadFile(packageJSONPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterPackageJSON) != packageJSON {
		t.Fatalf("suggest command mutated package.json:\n%s", string(afterPackageJSON))
	}
	if _, err := os.Stat(filepath.Join(root, "ts-lock.toml")); !os.IsNotExist(err) {
		t.Fatalf("suggest command wrote ts-lock.toml or stat failed: %v", err)
	}
}

func TestAdoptCheckAnnotationsPassesForSuggestedManifest(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := t.TempDir()
	pkgRoot := filepath.Join(root, "packages", "ui")
	if err := os.MkdirAll(pkgRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	packageJSON := `{"name":"@acme/ui","dependencies":{"clsx":"^2.1.1","react":"^19.0.0"},"devDependencies":{"typescript":"^5.9.0"}}`
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"workspace","workspaces":["packages/*"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgRoot, "package.json"), []byte(packageJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	suggestCmd := exec.Command(bin, "adopt", "--suggest-package", "packages/ui", "--root", root)
	suggestOut, err := suggestCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("adopt suggest failed: %v\n%s", err, string(suggestOut))
	}
	if err := os.WriteFile(filepath.Join(pkgRoot, "package.manifest.tsx"), suggestOut, 0o644); err != nil {
		t.Fatal(err)
	}

	checkCmd := exec.Command(bin, "adopt", "--check-annotations", "--root", root)
	checkOut, err := checkCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("annotation check failed: %v\n%s", err, string(checkOut))
	}
	if !strings.Contains(string(checkOut), "Package annotation check passed for 1 package.") {
		t.Fatalf("unexpected check output:\n%s", string(checkOut))
	}
}

func TestAdoptSuggestPackageSupportsRedirectToPackageManifestPath(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := t.TempDir()
	pkgRoot := filepath.Join(root, "packages", "ui")
	if err := os.MkdirAll(pkgRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	packageJSON := `{"name":"@acme/ui","dependencies":{"clsx":"^2.1.1","react":"^19.0.0"},"devDependencies":{"typescript":"^5.9.0"}}`
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"workspace","workspaces":["packages/*"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgRoot, "package.json"), []byte(packageJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(pkgRoot, "package.manifest.tsx")
	file, err := os.Create(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	cmd := exec.Command(bin, "adopt", "--suggest-package", "packages/ui", "--root", root)
	cmd.Stdout = file
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		t.Fatalf("adopt suggest with redirected stdout failed: %v\n%s", err, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr when redirecting to an empty package.manifest.tsx target, got:\n%s", stderr.String())
	}

	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, expected := range []string{"annotatePackage(", "clsx: dep(", "react: dep(", "typescript: tool("} {
		if !strings.Contains(text, expected) {
			t.Fatalf("redirected suggest output missing %q:\n%s", expected, text)
		}
	}
}

func TestAdoptCheckAnnotationsFailsForMismatchedManifest(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := copyMonorepoDogfoodProject(t, repo)

	cmd := exec.Command(bin, "adopt", "--check-annotations", "--root", root)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected annotation check to fail:\n%s", string(out))
	}
	text := string(out)
	for _, expected := range []string{
		"TSPACK_ADOPT_ANNOTATION_MISMATCH",
		"Package annotation check found mismatches:",
		"annotation peer(react) differs from package.json section dependencies",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected %q in output:\n%s", expected, text)
		}
	}
}

func copyMonorepoDogfoodProject(t *testing.T, repo string) string {
	t.Helper()
	src := filepath.Join(repo, "examples", "incremental-existing-monorepo")
	dst := filepath.Join(t.TempDir(), "incremental-existing-monorepo")
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		copyPath(t, filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name()))
	}
	return dst
}
