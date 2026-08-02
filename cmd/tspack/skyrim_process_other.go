//go:build !windows

package main

func findSkyrimRuntimeProcess() int {
	return 0
}

func skyrimRuntimeProcessExists(pid int) bool {
	return false
}
