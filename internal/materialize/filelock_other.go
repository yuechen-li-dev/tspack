//go:build !windows

package materialize

const materializeFileLockRetriesEnabled = false

func isTransientMaterializeLockErr(err error) bool {
	return false
}
