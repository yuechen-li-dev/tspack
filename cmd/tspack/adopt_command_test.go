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
