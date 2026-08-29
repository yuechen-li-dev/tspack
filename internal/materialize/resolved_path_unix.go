//go:build !windows

package materialize

import "path/filepath"

func resolvedExistingPath(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
