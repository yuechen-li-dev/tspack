//go:build !windows

package manifestedit

import "os"

func replaceFileAtomic(source string, destination string) error {
	return os.Rename(source, destination)
}
