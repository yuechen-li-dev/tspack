package adoption

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSuggestPackageAnnotationBasic(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, root, `{
		"name": "example-lib",
		"types": "dist/index.d.ts",
		"dependencies": {"clsx": "^2.1.1", "react": "^19.0.0"},
		"devDependencies": {"typescript": "^5.9.0", "vitest": "^3.0.0"},
		"peerDependencies": {"react-dom": "^19.0.0"}
	}`)

	suggestion, err := SuggestPackageAnnotation(root, ".")
	if err != nil {
		t.Fatal(err)
	}
	content := suggestion.Content
	for _, expected := range []string{
		"// Suggested by `tspack adopt --suggest-package`.",
		"clsx: dep(npm(\"clsx\", \"^2.1.1\"))",
		"react: dep(npm(\"react\", \"^19.0.0\"))",
		"reactDom: peer(npm(\"react-dom\", \"^19.0.0\"))",
		"typescript: tool(npm(\"typescript\", \"^5.9.0\"))",
		"vitest: tool(npm(\"vitest\", \"^3.0.0\"))",
		"react is in dependencies. If this package is a library, consider peer(...) instead.",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("suggestion missing %q:\n%s", expected, content)
		}
	}
}

func TestSuggestPackageAnnotationKeyGeneration(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, root, `{
		"dependencies": {
			"@types/react": "^19.0.0",
			"types-react": "^1.0.0",
			"@vitejs/plugin-react": "latest",
			"react-dom": "^19.0.0"
		}
	}`)

	suggestion, err := SuggestPackageAnnotation(root, ".")
	if err != nil {
		t.Fatal(err)
	}
	content := suggestion.Content
	for _, expected := range []string{"typesReact: dep(", "typesReact2: tool(", "vitejsPluginReact: tool(", "reactDom: dep("} {
		if !strings.Contains(content, expected) {
			t.Fatalf("suggestion missing key %q:\n%s", expected, content)
		}
	}
}

func TestSuggestPackageAnnotationExistingManifestNoWrite(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, root, `{"dependencies": {"clsx": "^2.1.1"}}`)
	manifestPath := filepath.Join(root, "package.manifest.tsx")
	original := "export default annotatePackage(<PackageAnnotations />);\n"
	if err := os.WriteFile(manifestPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	suggestion, err := SuggestPackageAnnotation(root, ".")
	if err != nil {
		t.Fatal(err)
	}
	if suggestion.ExistingManifestKind != "package annotation manifest" {
		t.Fatalf("unexpected existing manifest kind: %q", suggestion.ExistingManifestKind)
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("suggestion overwrote manifest:\n%s", string(data))
	}
	if _, err := os.Stat(filepath.Join(root, "ts-lock.toml")); !os.IsNotExist(err) {
		t.Fatalf("suggestion created ts-lock.toml or stat failed: %v", err)
	}
}

func TestSuggestPackageAnnotationMissingAndMalformedPackageJSON(t *testing.T) {
	_, missingErr := SuggestPackageAnnotation(t.TempDir(), ".")
	if missingErr == nil || !strings.Contains(missingErr.Error(), "TSPACK_ADOPT_SUGGEST_PACKAGE_JSON_MISSING") {
		t.Fatalf("expected missing diagnostic, got %v", missingErr)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, malformedErr := SuggestPackageAnnotation(root, ".")
	if malformedErr == nil || !strings.Contains(malformedErr.Error(), "TSPACK_ADOPT_SUGGEST_PACKAGE_JSON_MALFORMED") {
		t.Fatalf("expected malformed diagnostic, got %v", malformedErr)
	}
}

func writePackageJSON(t *testing.T, root string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
