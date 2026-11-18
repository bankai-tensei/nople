//go:build !windows

package app

// pathEqual compares two clean absolute paths as the file system does.
func pathEqual(a, b string) bool { return a == b }
