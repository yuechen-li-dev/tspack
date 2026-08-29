//go:build windows

package materialize

import (
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

func resolvedExistingPath(path string) (string, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	pathPointer, err := windows.UTF16PtrFromString(absolutePath)
	if err != nil {
		return "", err
	}
	handle, err := windows.CreateFile(
		pathPointer,
		windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)

	buffer := make([]uint16, windows.MAX_PATH)
	for {
		length, err := windows.GetFinalPathNameByHandle(handle, &buffer[0], uint32(len(buffer)), 0)
		if err != nil {
			return "", err
		}
		if length < uint32(len(buffer)) {
			return filepath.Clean(syscall.UTF16ToString(buffer[:length])), nil
		}
		buffer = make([]uint16, length+1)
	}
}
