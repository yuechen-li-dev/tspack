package main

import (
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
	stripDogfoodEditorSupport(t, dst)
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

func TestAdoptCheckAnnotationsReportsClassificationMismatch(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := copyExampleProject(t, repo, "incremental-existing-monorepo")

	cmd := exec.Command(bin, "adopt", "--check-annotations", "--root", root)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("adopt check annotations should fail on warning drift:\n%s", string(out))
	}
	text := string(out)
	for _, expected := range []string{"Package annotation consistency check", "classification-mismatch", "react", "Annotation drift found"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("check output missing %q:\n%s", expected, text)
		}
	}
	assertNoGeneratedAdoptionFiles(t, root)
	if _, err := os.Stat(filepath.Join(root, "packages", "ui", "package.manifest.tsx")); err != nil {
		t.Fatalf("checked annotation manifest missing after read-only check: %v", err)
	}
}

func TestAdoptCheckAnnotationsJSONReportsRangeMismatch(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	root := copyExampleProject(t, repo, "incremental-existing-monorepo")
	packageJSONPath := filepath.Join(root, "packages", "ui", "package.json")
	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(data), `"clsx": "^2.1.1"`, `"clsx": "^2.2.0"`, 1)
	if err := os.WriteFile(packageJSONPath, []byte(mutated), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "adopt", "--check-annotations", "--json", "--root", root)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("adopt check annotations json should fail on warning drift:\n%s", string(out))
	}
	var report struct {
		Summary struct {
			Warnings int `json:"warnings"`
		} `json:"summary"`
		Findings []struct {
			Code           string `json:"code"`
			DependencyName string `json:"dependencyName"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("decode check json: %v\n%s", err, string(out))
	}
	if report.Summary.Warnings == 0 || !jsonFindingExists(report.Findings, "range-mismatch", "clsx") {
		t.Fatalf("range mismatch was not reported: %#v", report)
	}
}

func TestAdoptCheckAnnotationsNoAnnotationsAndNoticeOnlyExitZero(t *testing.T) {
	repo := repoRoot(t)
	bin := buildTspackBinary(t, repo)
	noAnnotationRoot := copyDogfoodProject(t, repo)
	cmd := exec.Command(bin, "adopt", "--check-annotations", "--root", noAnnotationRoot)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("no annotations should exit zero: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "No package annotation manifests found") {
		t.Fatalf("no annotation message missing:\n%s", string(out))
	}

	noticeRoot := t.TempDir()
	pkgRoot := filepath.Join(noticeRoot, "packages", "ui")
	if err := os.MkdirAll(pkgRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(noticeRoot, "package.json"), []byte(`{"workspaces":["packages/*"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	pkgJSON := `{"name":"@acme/ui","dependencies":{"clsx":"^2.1.1","lodash":"^4.17.21"}}`
	if err := os.WriteFile(filepath.Join(pkgRoot, "package.json"), []byte(pkgJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `import { PackageAnnotations, annotatePackage, defineDeps, dep, npm } from "tspack/manifest";
const deps = defineDeps({ clsx: dep(npm("clsx", "^2.1.1")) });
export default annotatePackage(<PackageAnnotations name="@acme/ui" dependencies={{ values: [deps.clsx] }} />);
`
	if err := os.WriteFile(filepath.Join(pkgRoot, "package.manifest.tsx"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(bin, "adopt", "--check-annotations", "--root", noticeRoot)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("notice-only check should exit zero: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "unannotated-package-json-dependency") || !strings.Contains(string(out), "Package annotations are consistent") {
		t.Fatalf("notice-only output missing expected content:\n%s", string(out))
	}
}

func copyExampleProject(t *testing.T, repo string, name string) string {
	t.Helper()
	src := filepath.Join(repo, "examples", name)
	dst := filepath.Join(t.TempDir(), name)
	copyPath(t, src, dst)
	stripDogfoodEditorSupport(t, dst)
	return dst
}

func stripDogfoodEditorSupport(t *testing.T, root string) {
	t.Helper()

	for _, relativePath := range []string{
		"manifest.tsx",
		"tsconfig.tspack.json",
		"tspack-env.d.ts",
		".tspack",
		".vscode",
	} {
		path := filepath.Join(root, relativePath)
		if err := os.RemoveAll(path); err != nil {
			t.Fatalf("remove %s: %v", path, err)
		}
	}
}

func jsonFindingExists(findings []struct {
	Code           string `json:"code"`
	DependencyName string `json:"dependencyName"`
}, code string, dependencyName string) bool {
	for _, finding := range findings {
		if finding.Code == code && finding.DependencyName == dependencyName {
			return true
		}
	}
	return false
}
