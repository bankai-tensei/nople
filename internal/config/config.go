// Package config is the settings of the user, kept in one JSON file beside
// the session, which the settings tab writes and the user may edit by hand.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bankai-tensei/nople/internal/safewrite"

	"github.com/bankai-tensei/gowaku/theme"
)

// DefaultFontSize is the size of the text of a new editor.
const DefaultFontSize = 14

// Autosave modes.
const (
	AutosaveOff         = "off"
	AutosaveAfterDelay  = "afterDelay"
	AutosaveFocusChange = "onFocusChange"
)

// Settings are the choices of the user, kept in one JSON file.
type Settings struct {
	// FontFamily and FontSize are those of the editor and the terminal.
	FontFamily string  `json:"fontFamily"`
	FontSize   float32 `json:"fontSize"`
	// Theme is "system", "light" or "dark".
	Theme string `json:"theme"`
	Wrap  bool   `json:"wrap"`
	// Shell is the command line of a new terminal; empty selects the default.
	Shell string `json:"shell"`
	// Language is the language of the UI: "system", or the name of a file in
	// locales, such as "ja" or "en".
	Language string `json:"language"`
	// Autosave is "afterDelay", "onFocusChange" or "off"; AutosaveDelay is
	// the wait of afterDelay, in milliseconds.
	Autosave      string `json:"autosave"`
	AutosaveDelay int    `json:"autosaveDelay"`
	// Gitignore leaves what .gitignore ignores out of the explorer, quick
	// open and search. The .git folder is left out either way.
	Gitignore bool `json:"gitignore"`
}

// Default are the settings without a file.
func Default() Settings {
	return Settings{
		FontFamily: theme.MonoFamilies, FontSize: DefaultFontSize, Theme: "system", Language: "system",
		Autosave: AutosaveAfterDelay, AutosaveDelay: 1000,
	}
}

// DefaultPath is the file of the settings beside the session.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "Nople", "settings.json")
}

// Load reads the settings at path over the defaults. A field the file
// leaves out keeps its default; a file that does not parse is an error and
// changes nothing.
func Load(path string) (Settings, error) {
	s := Default()
	if path == "" {
		return s, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return Default(), fmt.Errorf("config: %s: %w", filepath.Base(path), err)
	}
	if s.FontSize <= 0 {
		s.FontSize = DefaultFontSize
	}
	if s.FontFamily == "" {
		s.FontFamily = theme.MonoFamilies
	}
	if s.AutosaveDelay <= 0 {
		s.AutosaveDelay = 1000
	}
	switch s.Autosave {
	case AutosaveOff, AutosaveAfterDelay, AutosaveFocusChange:
	default:
		s.Autosave = AutosaveAfterDelay
	}
	return s, nil
}

// Save writes s to the file at path, making its folder.
func Save(path string, s Settings) error {
	data, err := json.MarshalIndent(s, "", "\t")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return safewrite.WriteFile(path, append(data, '\n'), 0o644)
}
