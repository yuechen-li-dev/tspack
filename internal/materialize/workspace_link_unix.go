//go:build !windows

package materialize

import "os"

func createWorkspaceDirectoryLink(target string, link string) error {
	return os.Symlink(target, link)
}
