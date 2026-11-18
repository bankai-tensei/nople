package app

import "github.com/bankai-tensei/nople/internal/i18n"

// T is the text of key in the language of the UI; see i18n.T.
func T(key string, args ...any) string { return i18n.T(key, args...) }
