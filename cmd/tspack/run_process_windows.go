//go:build windows

package main

import (
	"os/exec"
	"strconv"
	"syscall"
)

func configureRunTargetProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func signalRunTargetProcessGroup(pid int, signal syscall.Signal) error {
	_ = signal

	cmd := exec.Command("taskkill", "/T", "/PID", strconv.Itoa(pid))
	if err := cmd.Run(); err == nil {
		return nil
	}

	force := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	return force.Run()
}
