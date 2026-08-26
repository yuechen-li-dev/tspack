//go:build !windows

package skyrim

import "os"

func replaceFileAtomic(source string, destination string) error {
	return os.Rename(source, destination)
}
