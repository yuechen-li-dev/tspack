//go:build !tspack_embedded_bridges

package bridge

func resolveEmbedded(name string) (string, bool) {
	return "", false
}
