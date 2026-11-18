package app

import "strings"

// pathEqual compares two clean absolute paths as the file system does:
// without regard to case on Windows.
func pathEqual(a, b string) bool { return strings.EqualFold(a, b) }
