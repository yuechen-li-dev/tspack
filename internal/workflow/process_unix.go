//go:build !windows

package workflow

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

type workflowCleanupHandle interface {
	Close() error
}

func configureWorkflowProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func attachWorkflowCleanup(command *exec.Cmd) (workflowCleanupHandle, error) {
	_ = command
	return nil, nil
}

func cleanupWorkflowProcessTree(processID int, cleanup workflowCleanupHandle) error {
	_ = cleanup
	err := syscall.Kill(-processID, syscall.SIGKILL)
	if err == nil || errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
