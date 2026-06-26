//go:build tspack_embedded_bridges

package bridge

import (
	"os"
	"path/filepath"

	"github.com/yuechen-li-dev/tspack/internal/embeddedbridges"
)

func resolveEmbedded(name string) (string, bool) {
	assets := embeddedbridges.Assets
	content, ok := assets[name]
	if !ok {
		return "", false
	}
	dir, err := os.MkdirTemp("", "tspack-bridges-*")
	if err != nil {
		return "", false
	}
	for assetName, assetContent := range assets {
		path := filepath.Join(dir, filepath.FromSlash(assetName))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", false
		}
		if err := os.WriteFile(path, assetContent, 0o644); err != nil {
			return "", false
		}
	}
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", false
	}
	return path, true
}
