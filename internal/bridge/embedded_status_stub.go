//go:build !tspack_embedded_bridges

package bridge

func embeddedSupportEnabled() bool {
	return false
}
