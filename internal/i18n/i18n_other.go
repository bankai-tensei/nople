//go:build !windows && !darwin

package i18n

// osUILanguages are the languages of the OS besides those of the
// environment, which osLanguages reads: none outside Windows and macOS.
func osUILanguages() []string { return nil }
