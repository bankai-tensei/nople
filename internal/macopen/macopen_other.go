//go:build !darwin

package macopen

// Listen does nothing outside macOS, where what to open is on the command
// line.
func Listen(_ func(paths []string)) {}
