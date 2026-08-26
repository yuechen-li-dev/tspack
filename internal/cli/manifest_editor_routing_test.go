package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRepoRootManifestEditorSolutionTSConfigReferencesTSPackProject(t *testing.T) {
	repo := repoRoot(t)
	assertSolutionReferences(t, filepath.Join(repo, "tsconfig.json"), "./tsconfig.tspack.json")
}

func TestDogfoodExamplesWithNoRootTSConfigNowReferenceManifestEditorProject(t *testing.T) {
	repo := repoRoot(t)
	for _, rel := range []string{
		filepath.Join("examples", "compat-json-basic", "tsconfig.json"),
		filepath.Join("examples", "incremental-existing-monorepo", "tsconfig.json"),
		filepath.Join("examples", "runtime-switch-notes", "tsconfig.json"),
		filepath.Join("examples", "update-policy-notes", "tsconfig.json"),
	} {
		assertSolutionReferences(t, filepath.Join(repo, rel), "./tsconfig.tspack.json")
	}
}

func assertSolutionReferences(t *testing.T, configPath string, expectedPath string) {
	t.Helper()

	bytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read %s: %v", configPath, err)
	}

	var config struct {
		Files      []string `json:"files"`
		References []struct {
			Path string `json:"path"`
		} `json:"references"`
	}
	if err := json.Unmarshal(bytes, &config); err != nil {
		t.Fatalf("parse %s: %v", configPath, err)
	}

	if len(config.Files) != 0 {
		t.Fatalf("%s should be solution-style with empty files, got %#v", configPath, config.Files)
	}
	if len(config.References) != 1 || config.References[0].Path != expectedPath {
		t.Fatalf("%s should reference %q, got %#v", configPath, expectedPath, config.References)
	}
}
