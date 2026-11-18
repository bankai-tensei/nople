package i18n

import "golang.org/x/sys/windows"

// osUILanguages are the display languages of Windows, best first.
func osUILanguages() []string {
	langs, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil {
		return nil
	}
	return langs
}
