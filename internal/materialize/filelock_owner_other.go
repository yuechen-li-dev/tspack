//go:build !windows

package materialize

type materializeLockOwner struct {
	PID  uint32
	Name string
}

var materializeLockOwners = func(string) []materializeLockOwner { return nil }

func formatMaterializeLockOwner(owner materializeLockOwner) string { return "" }
