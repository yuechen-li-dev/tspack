//go:build !windows

package skyrim

func findSkyrimRuntimeProcess(expectedPath string) int {
	return 0
}

func skyrimRuntimeProcessExists(pid int) bool {
	return false
}
