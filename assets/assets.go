// Package assets holds the pictures of Nople that ship inside the program:
// the wordmark for the welcome page and the icon of the window. The icon of
// nople.exe is icon.ico, which nople.rc puts into the executable.
package assets

import _ "embed"

var (
	// LogoLight is the wordmark in ink, for the light theme.
	//go:embed logo.svg
	LogoLight []byte
	// LogoDark is the wordmark in paper, for the dark theme.
	//go:embed logo-dark.svg
	LogoDark []byte
	// Icon is the icon of the window, 256 pixels square.
	//go:embed icon.png
	Icon []byte
)
