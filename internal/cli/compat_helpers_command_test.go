package cli

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var manifestFrontendBuild struct {
	sync.Once
	err    error
	output []byte
}

func TestCompatHelpersFixtureCommands(t *testing.T) {
	repo := filepath.Join("..", "..")
	ensureManifestFrontendCLI(t, repo)
	root := t.TempDir()
	copyFile(t, filepath.Join(repo, "examples", "compat-json-basic", "manifest.tsx"), filepath.Join(root, "manifest.tsx"))

	listOut, err := runCompatHelperCommand("", root, "list")
	if err != nil {
		t.Fatalf("compat list failed: %v\n%s", err, listOut)
	}
	for _, expected := range []string{"tsconfig.tspack.json", ".vscode/settings.json", ".vscode/extensions.json", "compat.raw.json", ".tspack/types/tspack-manifest.d.ts", ".tspack/types/tspack-xtest.d.ts"} {
		if !strings.Contains(listOut, expected) {
			t.Fatalf("compat list missing %s:\n%s", expected, listOut)
		}
	}

	diffOut, err := runCompatHelperCommand("", root, "diff")
	if err == nil {
		t.Fatalf("compat diff before write unexpectedly succeeded:\n%s", diffOut)
	}
	if !strings.Contains(diffOut, "compilerOptions") || !strings.Contains(diffOut, "typescript.tsdk") || !strings.Contains(diffOut, "recommendations") {
		t.Fatalf("compat diff did not show helper-authored JSON keys:\n%s", diffOut)
	}

	writeOut, err := runCompatHelperCommand("", root, "write")
	if err != nil {
		t.Fatalf("compat write failed: %v\n%s", err, writeOut)
	}

	diffOut, err = runCompatHelperCommand("", root, "diff")
	if err != nil {
		t.Fatalf("compat diff after write failed: %v\n%s", err, diffOut)
	}

	assertJSONFileContainsKey(t, filepath.Join(root, "tsconfig.tspack.json"), "compilerOptions")
	assertJSONFileContainsKey(t, filepath.Join(root, ".vscode", "settings.json"), "typescript.tsdk")
	assertJSONFileContainsKey(t, filepath.Join(root, ".vscode", "settings.json"), "typescript.enablePromptUseWorkspaceTsdk")
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

func TestRepoRootManifestNarrowsManifestEditorTSConfig(t *testing.T) {
	repo := filepath.Join("..", "..")
	ensureManifestFrontendCLI(t, repo)
	root := t.TempDir()
	copyFile(t, filepath.Join(repo, "manifest.tsx"), filepath.Join(root, "manifest.tsx"))

	writeOut, err := runCompatHelperCommand("", root, "write")
	if err != nil {
		t.Fatalf("compat write failed: %v\n%s", err, writeOut)
	}

	diffOut, err := runCompatHelperCommand("", root, "diff")
	if err != nil {
		t.Fatalf("compat diff after write failed: %v\n%s", err, diffOut)
	}

	configBytes, err := os.ReadFile(filepath.Join(root, "tsconfig.tspack.json"))
	if err != nil {
		t.Fatalf("read generated tsconfig.tspack.json: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(configBytes, &config); err != nil {
		t.Fatalf("parse generated tsconfig.tspack.json: %v", err)
	}

	includeSet := jsonStringArraySet(t, config["include"])
	for _, want := range []string{
		"manifest.tsx",
		"examples/compat-json-basic/manifest.tsx",
		"examples/incremental-existing-react/manifest.tsx",
		"examples/incremental-existing-monorepo/manifest.tsx",
		"examples/incremental-existing-monorepo/packages/ui/package.manifest.tsx",
		"examples/nestjs-service/manifest.tsx",
		"examples/runtime-switch-notes/manifest.tsx",
		"examples/runtime-switch-notes/tests/inspect.xtest.tsx",
		"examples/runtime-switch-notes/tests/runtime-switch.xtest.tsx",
		"examples/update-policy-notes/manifest.tsx",
		".tspack/types/**/*.d.ts",
	} {
		if !includeSet[want] {
			t.Fatalf("repo-root tsconfig include missing %q in %#v", want, includeSet)
		}
	}
	if includeSet["**/*.manifest.tsx"] || includeSet["**/*.xtest.tsx"] {
		t.Fatalf("repo-root tsconfig should use narrowed includes, got %#v", includeSet)
	}

	excludeSet := jsonStringArraySet(t, config["exclude"])
	for _, want := range []string{
		"node_modules/**",
		"dist/**",
		".tspack/store/**",
		"tspack-artifacts/**",
		"fixtures/**",
		"testdata/**",
		"manifest-frontend/**/fixtures/**",
		"manifest-frontend/**/testdata/**",
	} {
		if !excludeSet[want] {
			t.Fatalf("repo-root tsconfig exclude missing %q in %#v", want, excludeSet)
		}
	}

	settingsBytes, err := os.ReadFile(filepath.Join(root, ".vscode", "settings.json"))
	if err != nil {
		t.Fatalf("read generated .vscode/settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(settingsBytes, &settings); err != nil {
		t.Fatalf("parse generated .vscode/settings.json: %v", err)
	}
	if settings["typescript.tsdk"] != "manifest-frontend/node_modules/typescript/lib" {
		t.Fatalf("repo-root settings should point to nested TypeScript SDK, got %#v", settings["typescript.tsdk"])
	}
	if settings["typescript.enablePromptUseWorkspaceTsdk"] != true {
		t.Fatalf("repo-root settings should enable the workspace TypeScript prompt, got %#v", settings["typescript.enablePromptUseWorkspaceTsdk"])
	}
}

func ensureManifestFrontendCLI(t *testing.T, repo string) {
	t.Helper()
	manifestFrontendBuild.Do(func() {
		current, err := manifestFrontendCLIIsCurrent(repo)
		if err != nil {
			manifestFrontendBuild.err = err
			return
		}
		if current {
			return
		}

		cmd := exec.Command("npm", "--prefix", "manifest-frontend", "run", "build")
		cmd.Dir = repo
		manifestFrontendBuild.output, manifestFrontendBuild.err = cmd.CombinedOutput()
	})
	if manifestFrontendBuild.err != nil {
		t.Fatalf("build manifest frontend failed: %v\n%s", manifestFrontendBuild.err, string(manifestFrontendBuild.output))
	}
}

func manifestFrontendCLIIsCurrent(repo string) (bool, error) {
	frontendRoot := filepath.Join(repo, "manifest-frontend")
	outputInfo, err := os.Stat(filepath.Join(frontendRoot, "dist", "cli.js"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	inputs := []string{
		filepath.Join(frontendRoot, "src"),
		filepath.Join(frontendRoot, "package.json"),
		filepath.Join(frontendRoot, "package-lock.json"),
		filepath.Join(frontendRoot, "tsconfig.json"),
	}
	for _, input := range inputs {
		inputInfo, statErr := os.Stat(input)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return false, statErr
		}
		if !inputInfo.IsDir() {
			if inputInfo.ModTime().After(outputInfo.ModTime()) {
				return false, nil
			}
			continue
		}

		walkErr := filepath.WalkDir(input, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			info, infoErr := entry.Info()
			if infoErr != nil {
				return infoErr
			}
			if info.ModTime().After(outputInfo.ModTime()) {
				return errManifestFrontendSourceIsNewer
			}
			return nil
		})
		if errors.Is(walkErr, errManifestFrontendSourceIsNewer) {
			return false, nil
		}
		if walkErr != nil {
			return false, walkErr
		}
	}
	return true, nil
}

var errManifestFrontendSourceIsNewer = errors.New("manifest frontend source is newer than compiled CLI")

func TestManifestFrontendCurrentCheckAvoidsUnchangedRebuilds(t *testing.T) {
	repo := t.TempDir()
	frontendRoot := filepath.Join(repo, "manifest-frontend")
	sourcePath := filepath.Join(frontendRoot, "src", "index.ts")
	outputPath := filepath.Join(frontendRoot, "dist", "cli.js")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("export {};\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputPath, []byte("export {};\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	baseTime := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(sourcePath, baseTime, baseTime); err != nil {
		t.Fatal(err)
	}
	outputTime := baseTime.Add(time.Minute)
	if err := os.Chtimes(outputPath, outputTime, outputTime); err != nil {
		t.Fatal(err)
	}
	current, err := manifestFrontendCLIIsCurrent(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !current {
		t.Fatal("unchanged manifest frontend should reuse compiled CLI")
	}

	newerTime := outputTime.Add(time.Minute)
	if err := os.Chtimes(sourcePath, newerTime, newerTime); err != nil {
		t.Fatal(err)
	}
	current, err = manifestFrontendCLIIsCurrent(repo)
	if err != nil {
		t.Fatal(err)
	}
	if current {
		t.Fatal("newer manifest frontend source should require rebuild")
	}
}

func runCompatHelperCommand(_ string, root string, subcommand string) (string, error) {
	cmd := newInProcessCommand("compat", subcommand, "--root", root)
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
