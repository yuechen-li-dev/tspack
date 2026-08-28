package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

var templateRoots = []string{
	"internal/templates/builtin/static/files/tspack-types",
	"internal/templates/builtin/react/files/tspack-types",
	"internal/templates/builtin/react-library/files/tspack-types",
	"internal/templates/testdata/local-concepts/concept-manifest-app/files/tspack-types",
	"internal/templates/testdata/local-concepts/machina-react-app/files/tspack-types",
	"internal/templates/testdata/local-concepts/tailwind-machina-react-app/files/tspack-types",
	"internal/templates/testdata/local-concepts/tailwind-react-app/files/tspack-types",
}

func main() {
	for _, name := range []string{"tspack-manifest.d.ts", "tspack-xtest.d.ts"} {
		sourcePath := filepath.Join("manifest-frontend", "src", name)
		contents, err := os.ReadFile(sourcePath)
		if err != nil {
			fatalf("read canonical %s: %v", sourcePath, err)
		}
		targets := []string{
			filepath.Join("internal", "manifesttypes", name),
			filepath.Join(".tspack", "types", name),
		}
		for _, root := range templateRoots {
			targets = append(targets, filepath.Join(root, name))
		}
		for _, target := range targets {
			if err := writeIfChanged(target, contents); err != nil {
				fatalf("write %s: %v", target, err)
			}
		}
	}
}

func writeIfChanged(path string, contents []byte) error {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, contents) {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, contents, 0o644)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "sync-manifest-types: "+format+"\n", args...)
	os.Exit(1)
}
