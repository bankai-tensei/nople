package app

import (
	"encoding/json"
	"github.com/bankai-tensei/nople/internal/config"
	"os"
	"path/filepath"
	"testing"

	"github.com/bankai-tensei/gowaku/input"
)

func TestSettingsView(t *testing.T) {
	h, _ := openSample(t, false)
	path := filepath.Join(t.TempDir(), "settings.json")
	h.app.settingsPath = path
	h.tt.Key(input.KeyComma, input.ModControl)
	h.settle()
	if !h.app.settingsOpen {
		t.Fatal("Ctrl+, should open the settings")
	}
	for _, s := range []string{"外観", "エディター", "ファイル", ".gitignore に従う", "ターミナル", "言語", "テーマ", "自動保存", "settings.json を開く"} {
		if !h.has(s) {
			t.Errorf("%q should show:\n%s", s, h.tt.Dump())
		}
	}
	if h.has("ようこそ") {
		t.Error("the welcome page should give way to the settings")
	}
	h.shot("settings")

	// A switch changes the setting, takes effect and is written.
	h.tt.Tap("折り返し")
	h.settle()
	if !h.app.settings.Wrap || !h.app.wrap {
		t.Errorf("the switch should turn wrapping on: %+v", h.app.settings)
	}
	h.tt.Tap("ダーク")
	h.settle()
	if h.app.settings.Theme != "dark" {
		t.Errorf("the segment should pick the dark theme: %q", h.app.settings.Theme)
	}
	h.shot("settings-dark")
	var saved config.Settings
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &saved); err != nil || !saved.Wrap || saved.Theme != "dark" {
		t.Errorf("the file should hold the changes: %s", data)
	}

	// An edit of the file by hand shows in the tab.
	saved.FontSize = 20
	saved.Autosave = config.AutosaveOff
	data, _ = json.Marshal(saved)
	os.WriteFile(path, data, 0o644)
	h.app.applySettings()
	h.settle()
	if h.app.fontSize != 20 || h.has("自動保存までの時間") {
		t.Errorf("the tab should follow the file: font %v", h.app.fontSize)
	}

	// The JSON opens from the tab.
	h.tt.Tap("settings.json を開く")
	h.settle()
	if tb := h.app.currentTab(); tb == nil || tb.doc.path != path {
		t.Error("the button should open settings.json")
	}

	// Closing the settings with no other tab brings the welcome page back.
	h.app.closeAll()
	h.app.dock.Close(settingsID)
	h.app.settingsClosed()
	h.settle()
	if h.app.settingsOpen || !h.has("ようこそ") {
		t.Errorf("the welcome page should come back:\n%s", h.tt.Dump())
	}
}
