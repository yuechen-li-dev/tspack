//go:build windows

package materialize

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

func createWorkspaceDirectoryLink(target string, link string) error {
	if err := os.Mkdir(link, 0o755); err != nil {
		return err
	}
	target = filepath.Clean(target)
	substituteName := utf16.Encode([]rune(`\??\` + target))
	printName := utf16.Encode([]rune(target))
	substituteLength := len(substituteName) * 2
	printLength := len(printName) * 2
	pathBufferLength := substituteLength + 2 + printLength + 2
	reparseDataLength := 8 + pathBufferLength
	buffer := make([]byte, 8+reparseDataLength)
	binary.LittleEndian.PutUint32(buffer[0:4], windows.IO_REPARSE_TAG_MOUNT_POINT)
	binary.LittleEndian.PutUint16(buffer[4:6], uint16(reparseDataLength))
	binary.LittleEndian.PutUint16(buffer[8:10], 0)
	binary.LittleEndian.PutUint16(buffer[10:12], uint16(substituteLength))
	binary.LittleEndian.PutUint16(buffer[12:14], uint16(substituteLength+2))
	binary.LittleEndian.PutUint16(buffer[14:16], uint16(printLength))
	writeUTF16Bytes(buffer[16:], substituteName)
	writeUTF16Bytes(buffer[16+substituteLength+2:], printName)

	linkPointer, err := windows.UTF16PtrFromString(link)
	if err != nil {
		_ = os.Remove(link)
		return err
	}
	handle, err := windows.CreateFile(
		linkPointer,
		windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		_ = os.Remove(link)
		return err
	}
	defer windows.CloseHandle(handle)
	var bytesReturned uint32
	err = windows.DeviceIoControl(
		handle,
		windows.FSCTL_SET_REPARSE_POINT,
		&buffer[0],
		uint32(len(buffer)),
		nil,
		0,
		&bytesReturned,
		nil,
	)
	if err != nil {
		_ = os.Remove(link)
		return err
	}
	return nil
}

func writeUTF16Bytes(destination []byte, value []uint16) {
	for index, character := range value {
		binary.LittleEndian.PutUint16(destination[index*2:index*2+2], character)
	}
}
