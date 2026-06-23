package bridge

import (
	"fmt"
	"os"
	"path/filepath"
)

type Resolution struct {
	Path          string
	SearchedPaths []string
	Embedded      bool
}

func Resolve(name string) Resolution {
	if path, ok := resolveEmbedded(name); ok {
		return Resolution{Path: path, Embedded: true}
	}
	return ResolveFilesystem(name)
}

func ResolveFilesystem(name string) Resolution {
	candidates := filesystemCandidates(name)
	resolution := Resolution{SearchedPaths: candidates}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			resolution.Path = candidate
			return resolution
		}
	}
	return resolution
}

func filesystemCandidates(name string) []string {
	candidates := []string{}
	if overrideDir := os.Getenv("TSPACK_MANIFEST_FRONTEND_BRIDGE_DIR"); overrideDir != "" {
		candidates = append(candidates, filepath.Join(overrideDir, name))
	}
	if name == "cli.js" {
		if override := os.Getenv("TSPACK_MANIFEST_FRONTEND_CLI"); override != "" {
			candidates = append(candidates, override)
		}
	}
	return append(candidates,
		filepath.Join("manifest-frontend", "dist", name),
		filepath.Join("manifest-frontend", "dist", "src", name),
	)
}

func BuildNeededDetails() []string {
	return []string{
		"Build manifest frontend bridges with:",
		"  cd manifest-frontend && npm run build",
		"or build a release binary with:",
		"  ./scripts/build-release.sh",
	}
}

func MissingMessage(code string, label string, resolution Resolution) string {
	message := fmt.Sprintf("%s: %s not found\nsearched paths:\n", code, label)
	for _, searched := range resolution.SearchedPaths {
		message += fmt.Sprintf("  %s\n", searched)
	}
	for _, line := range BuildNeededDetails() {
		message += line + "\n"
	}
	return message
}
