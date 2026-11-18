//go:build !windows

package shellreg

import "errors"

var errUnsupported = errors.New("putting Nople into the shell works only on Windows")

// Register fails outside Windows.
func Register(exe string, l Labels, iconDir string) error { return errUnsupported }

// Unregister fails outside Windows.
func Unregister() error { return errUnsupported }

// IconFile is "" outside Windows.
func IconFile(dir, ext string) string { return "" }
