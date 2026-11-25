//go:build windows

package skyrim

import (
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func findSkyrimRuntimeProcess(expectedPath string) int {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return 0
	}
	for {
		name := windows.UTF16ToString(entry.ExeFile[:])
		if strings.EqualFold(name, "SkyrimSE.exe") {
			handle, openErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
			if openErr == nil {
				path := make([]uint16, 32768)
				size := uint32(len(path))
				queryErr := windows.QueryFullProcessImageName(handle, 0, &path[0], &size)
				_ = windows.CloseHandle(handle)
				if queryErr == nil && strings.EqualFold(filepath.Clean(windows.UTF16ToString(path[:size])), filepath.Clean(expectedPath)) {
					return int(entry.ProcessID)
				}
			}
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			return 0
		}
	}
}

func skyrimRuntimeProcessExists(pid int) bool {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	status, err := windows.WaitForSingleObject(handle, 0)
	return err == nil && status == 258 // WAIT_TIMEOUT
}
