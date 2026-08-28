//go:build windows

package workflow

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type workflowCleanupHandle interface {
	Close() error
}

func configureWorkflowProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func attachWorkflowCleanup(command *exec.Cmd) (workflowCleanupHandle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create job object: %w", err)
	}
	information := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	information.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&information)),
		uint32(unsafe.Sizeof(information)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("configure job object: %w", err)
	}
	processHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(command.Process.Pid),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("open process %d: %w", command.Process.Pid, err)
	}
	defer windows.CloseHandle(processHandle)
	if err := windows.AssignProcessToJobObject(job, processHandle); err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("assign process %d to job object: %w", command.Process.Pid, err)
	}
	return &windowsWorkflowCleanup{job: job}, nil
}

type windowsWorkflowCleanup struct {
	job windows.Handle
}

func (cleanup *windowsWorkflowCleanup) Close() error {
	if cleanup == nil || cleanup.job == 0 {
		return nil
	}
	job := cleanup.job
	cleanup.job = 0
	return windows.CloseHandle(job)
}

func cleanupWorkflowProcessTree(processID int, cleanup workflowCleanupHandle) error {
	_ = processID
	if cleanup == nil {
		return nil
	}
	return cleanup.Close()
}
