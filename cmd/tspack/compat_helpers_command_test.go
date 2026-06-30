package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompatHelpersFixtureCommands(t *testing.T) {
	repo := filepath.Join("..", "..")
	ensureManifestFrontendCLI(t, repo)
	bin := buildTspackBinary(t, repo)
	root := t.TempDir()
	copyFile(t, filepath.Join(repo, "examples", "compat-json-basic", "manifest.tsx"), filepath.Join(root, "manifest.tsx"))

	listOut, err := runCompatHelperCommand(bin, root, "list")
	if err != nil {
		t.Fatalf("compat list failed: %v\n%s", err, listOut)
	}
	for _, expected := range []string{"tsconfig.tspack.json", ".vscode/settings.json", ".vscode/extensions.json", "compat.raw.json", ".tspack/types/tspack-manifest.d.ts", ".tspack/types/tspack-xtest.d.ts"} {
		if !strings.Contains(listOut, expected) {
			t.Fatalf("compat list missing %s:\n%s", expected, listOut)
		}
	}

	diffOut, err := runCompatHelperCommand(bin, root, "diff")
	if err == nil {
		t.Fatalf("compat diff before write unexpectedly succeeded:\n%s", diffOut)
	}
	if !strings.Contains(diffOut, "compilerOptions") || !strings.Contains(diffOut, "typescript.tsdk") || !strings.Contains(diffOut, "recommendations") {
		t.Fatalf("compat diff did not show helper-authored JSON keys:\n%s", diffOut)
	}

	writeOut, err := runCompatHelperCommand(bin, root, "write")
	if err != nil {
		t.Fatalf("compat write failed: %v\n%s", err, writeOut)
	}

	diffOut, err = runCompatHelperCommand(bin, root, "diff")
	if err != nil {
		t.Fatalf("compat diff after write failed: %v\n%s", err, diffOut)
	}

	assertJSONFileContainsKey(t, filepath.Join(root, "tsconfig.tspack.json"), "compilerOptions")
	assertJSONFileContainsKey(t, filepath.Join(root, ".vscode", "settings.json"), "typescript.tsdk")
	assertJSONFileContainsKey(t, filepath.Join(root, ".vscode", "extensions.json"), "recommendations")
	assertJSONFileContainsKey(t, filepath.Join(root, "compat.raw.json"), "raw")
	gotTSConfig, err := os.ReadFile(filepath.Join(root, "tsconfig.tspack.json"))
	if err != nil {
		t.Fatalf("read generated tsconfig.tspack.json: %v", err)
	}
	if !jsonBytesEqual(t, gotTSConfig, []byte(initTSPackTSConfigJSON)) {
		t.Fatalf("compat helper tsconfig.tspack.json drifted from initTSPackTSConfigJSON:\n%s", string(gotTSConfig))
	}
	if _, err := os.Stat(filepath.Join(root, ".tspack", "types", "tspack-manifest.d.ts")); err != nil {
		t.Fatalf("compat write did not create manifest type support: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".tspack", "types", "tspack-xtest.d.ts")); err != nil {
		t.Fatalf("compat write did not create xtest type support: %v", err)
	}
}

func ensureManifestFrontendCLI(t *testing.T, repo string) {
	t.Helper()
	cmd := exec.Command("npm", "--prefix", "manifest-frontend", "run", "build")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build manifest frontend failed: %v\n%s", err, string(out))
	}

}

func runCompatHelperCommand(bin string, root string, subcommand string) (string, error) {
	cmd := exec.Command(bin, "compat", subcommand, "--root", root)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func assertJSONFileContainsKey(t *testing.T, path string, key string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(contents, &value); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if _, ok := value[key]; !ok {
		t.Fatalf("%s did not contain key %s: %s", path, key, string(contents))
	}
}

func jsonBytesEqual(t *testing.T, left []byte, right []byte) bool {
	t.Helper()

	var leftValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		t.Fatalf("parse left JSON: %v", err)
	}
	var rightValue any
	if err := json.Unmarshal(right, &rightValue); err != nil {
		t.Fatalf("parse right JSON: %v", err)
	}

	leftCanon, err := json.Marshal(leftValue)
	if err != nil {
		t.Fatalf("marshal left JSON: %v", err)
	}
	rightCanon, err := json.Marshal(rightValue)
	if err != nil {
		t.Fatalf("marshal right JSON: %v", err)
	}

	return string(leftCanon) == string(rightCanon)
}
