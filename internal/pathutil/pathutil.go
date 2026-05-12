package pathutil

import (
	"path"
	"strings"
)

// IsSafePackageFilePath reports whether p is a portable, slash-separated relative file path.
// Bare "." is rejected.
func IsSafePackageFilePath(p string) bool {
	if strings.TrimSpace(p) == "" {
		return false
	}
	if strings.Contains(p, "\\") || strings.Contains(p, "..") {
		return false
	}
	if strings.HasPrefix(p, "/") {
		return false
	}
	clean := path.Clean(p)
	if clean == "." {
		return false
	}
	if strings.HasPrefix(clean, "../") || clean == ".." {
		return false
	}
	if strings.Contains(clean, "/../") {
		return false
	}
	return true
}

// IsSafePackageRoot reports whether p is a safe package root path.
// Bare "." is allowed for package root declarations.
func IsSafePackageRoot(p string) bool {
	if p == "." {
		return true
	}
	return IsSafePackageFilePath(p)
}

// IsSafeRelativeGlob reports whether p is a glob-like relative path that does not traverse.
func IsSafeRelativeGlob(p string) bool {
	if !strings.ContainsAny(p, "*?[]") {
		return false
	}
	replaced := strings.NewReplacer("*", "x", "?", "x", "[", "x", "]", "x").Replace(p)
	return IsSafePackageFilePath(replaced)
}
