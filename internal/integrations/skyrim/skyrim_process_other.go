//go:build !windows

package skyrim

func findSkyrimRuntimeProcess() int {
	return 0
}

func skyrimRuntimeProcessExists(pid int) bool {
	return false
}
