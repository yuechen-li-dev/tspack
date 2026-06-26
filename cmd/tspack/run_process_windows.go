//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureRunTargetProcess(cmd *exec.Cmd) {
	_ = cmd
}

func signalRunTargetProcessGroup(pid int, signal syscall.Signal) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	err = process.Signal(signal)
	if err == nil || errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}
