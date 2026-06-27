//go:build windows

package materialize

import (
	"errors"
	"os"
	"syscall"
)

const materializeFileLockRetriesEnabled = true

func isTransientMaterializeLockErr(err error) bool {
	if err == nil {
		return false
	}
	if os.IsPermission(err) {
		return true
	}
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	switch errno {
	case syscall.Errno(5), syscall.Errno(32), syscall.Errno(33), syscall.Errno(145):
		return true
	default:
		return false
	}
}
