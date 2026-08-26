//go:build !windows

package cli

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureRunTargetProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func signalRunTargetProcessGroup(pid int, signal syscall.Signal) error {
	err := syscall.Kill(-pid, signal)
	if err == nil || errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func attachRunTargetCleanup(cmd *exec.Cmd) (runTargetCleanupHandle, error) {
	return nil, nil
}

func cleanupExitedRunTargetProcessTree(rootPID int, cleanupHandle runTargetCleanupHandle) error {
	_ = cleanupHandle
	if rootPID == 0 {
		return nil
	}
	return signalRunTargetProcessGroup(rootPID, syscall.SIGKILL)
}
