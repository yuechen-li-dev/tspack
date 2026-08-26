//go:build windows

package cli

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func configureRunTargetProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func signalRunTargetProcessGroup(pid int, signal syscall.Signal) error {
	_ = signal

	if err := runTaskkill("/T", "/PID", strconv.Itoa(pid)); err == nil {
		return nil
	}
	return runTaskkill("/T", "/F", "/PID", strconv.Itoa(pid))
}

func attachRunTargetCleanup(cmd *exec.Cmd) (runTargetCleanupHandle, error) {
	if cmd == nil || cmd.Process == nil {
		return nil, nil
	}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create job object: %w", err)
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("configure job object: %w", err)
	}

	processHandle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("open process %d for cleanup: %w", cmd.Process.Pid, err)
	}
	defer windows.CloseHandle(processHandle)

	if err := windows.AssignProcessToJobObject(job, processHandle); err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("assign process %d to job object: %w", cmd.Process.Pid, err)
	}

	return &windowsRunTargetCleanupHandle{job: job}, nil
}

type windowsRunTargetCleanupHandle struct {
	job windows.Handle
}

func (h *windowsRunTargetCleanupHandle) Close() error {
	if h == nil || h.job == 0 {
		return nil
	}
	job := h.job
	h.job = 0
	return windows.CloseHandle(job)
}

func cleanupExitedRunTargetProcessTree(rootPID int, cleanupHandle runTargetCleanupHandle) error {
	_ = rootPID
	if cleanupHandle == nil {
		return nil
	}
	return cleanupHandle.Close()
}

func runTaskkill(args ...string) error {
	cmd := exec.Command("taskkill", args...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	lower := strings.ToLower(string(output))
	if strings.Contains(lower, "not found") || strings.Contains(lower, "no running instance") {
		return nil
	}
	return err
}
