//go:build windows

package materialize

import (
	"fmt"
	"path/filepath"
	"sort"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type materializeLockOwner struct {
	PID  uint32
	Name string
}

type rmUniqueProcess struct {
	ProcessID        uint32
	ProcessStartTime windows.Filetime
}

type rmProcessInfo struct {
	Process          rmUniqueProcess
	AppName          [256]uint16
	ServiceShortName [64]uint16
	ApplicationType  uint32
	AppStatus        uint32
	SessionID        uint32
	Restartable      int32
}

var (
	restartManagerDLL     = windows.NewLazySystemDLL("rstrtmgr.dll")
	rmStartSession        = restartManagerDLL.NewProc("RmStartSession")
	rmRegister            = restartManagerDLL.NewProc("RmRegisterResources")
	rmGetList             = restartManagerDLL.NewProc("RmGetList")
	rmEndSession          = restartManagerDLL.NewProc("RmEndSession")
	materializeLockOwners = findMaterializeLockOwners
)

func findMaterializeLockOwners(path string) []materializeLockOwner {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	pathPointer, err := windows.UTF16PtrFromString(absolutePath)
	if err != nil {
		return nil
	}
	var session uint32
	var sessionKey [33]uint16
	if result, _, _ := rmStartSession.Call(uintptr(unsafe.Pointer(&session)), 0, uintptr(unsafe.Pointer(&sessionKey[0]))); result != 0 {
		return nil
	}
	defer rmEndSession.Call(uintptr(session))
	files := []*uint16{pathPointer}
	if result, _, _ := rmRegister.Call(uintptr(session), 1, uintptr(unsafe.Pointer(&files[0])), 0, 0, 0, 0); result != 0 {
		return nil
	}
	var needed, count, rebootReasons uint32
	result, _, _ := rmGetList.Call(uintptr(session), uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&count)), 0, uintptr(unsafe.Pointer(&rebootReasons)))
	if result == 0 && needed == 0 {
		return nil
	}
	if result != uintptr(syscall.ERROR_MORE_DATA) || needed == 0 {
		return nil
	}
	processes := make([]rmProcessInfo, needed)
	count = needed
	result, _, _ = rmGetList.Call(uintptr(session), uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&processes[0])), uintptr(unsafe.Pointer(&rebootReasons)))
	if result != 0 {
		return nil
	}
	owners := make([]materializeLockOwner, 0, count)
	for _, process := range processes[:count] {
		name := windows.UTF16ToString(process.AppName[:])
		if name == "" {
			name = "unknown"
		}
		owners = append(owners, materializeLockOwner{PID: process.Process.ProcessID, Name: name})
	}
	sort.SliceStable(owners, func(i, j int) bool { return owners[i].PID < owners[j].PID })
	return owners
}

func formatMaterializeLockOwner(owner materializeLockOwner) string {
	return fmt.Sprintf("lockOwner=%s (pid=%d)", owner.Name, owner.PID)
}
