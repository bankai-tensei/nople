package app

import (
	"github.com/bankai-tensei/nople/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/japanese"

	"github.com/bankai-tensei/gowaku/input"
	"github.com/bankai-tensei/gowaku/theme"
)

// editSample opens the sample with the given autosave and the file name
// open in a tab, focused.
func editSample(t *testing.T, autosave string, name string) (*harness, string, *tab) {
	t.Helper()
	h, dir := openSample(t, false)
	h.app.settings.Autosave = autosave
	h.app.settings.AutosaveDelay = 50
	h.app.openFile(filepath.Join(dir, name), false)
	h.settle()
	tb := h.app.currentTab()
	h.tt.Tap(tb.doc.title())
	h.settle()
	return h, dir, tb
}

func TestEdit_TypeAndSave(t *testing.T) {
	h, dir, tb := editSample(t, config.AutosaveOff, "main.go")
	h.tt.Type("xyz")
	h.settle()
	d := tb.doc
	if !d.dirty || !strings.Contains(d.ctrl.Text(), "xyz") {
		t.Fatalf("typing should change the text: dirty = %v", d.dirty)
	}
	if !h.has("● main.go") || !h.has("未保存") || !strings.HasPrefix(h.sh.title, "● main.go") {
		t.Errorf("the tab, the status bar and the title should show the change:\n%s", h.tt.Dump())
	}
	h.shot("phase9-dirty")
	h.tt.Key(input.KeyS, input.ModControl)
	h.settle()
	data, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	if d.dirty || !strings.Contains(string(data), "xyz") {
		t.Errorf("Ctrl+S should save: dirty = %v, file = %q", d.dirty, data)
	}
	if h.has("● main.go") {
		t.Error("the dot should go after saving")
	}
	// The save comes back from the watcher as a change that is no change.
	time.Sleep(300 * time.Millisecond)
	h.settle()
	if d.conflict {
		t.Error("our own save should not look like a conflict")
	}
}

func TestEdit_KeepsEncodingAndLineBreaks(t *testing.T) {
	h, dir := openSample(t, false)
	h.app.settings.Autosave = config.AutosaveOff
	path := filepath.Join(dir, "sjis.txt")
	sjis, _ := japanese.ShiftJIS.NewEncoder().Bytes([]byte("一行目\r\n二行目\r\n"))
	os.WriteFile(path, sjis, 0o644)
	h.app.openFile(path, false)
	h.settle()
	tb := h.app.currentTab()
	h.tt.Tap(tb.doc.title())
	h.tt.Key(input.KeyEnd, input.ModControl)
	h.tt.Type("三行目")
	h.settle()
	if err := h.app.save(tb.doc); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	text, err := japanese.ShiftJIS.NewDecoder().Bytes(data)
	if err != nil || string(text) != "一行目\r\n二行目\r\n三行目" {
		t.Errorf("the file should stay Shift_JIS with CRLF: %q (%v)", text, err)
	}

	// A character Shift_JIS has not is an error, and nothing is written.
	h.tt.Type("😀")
	h.settle()
	if err := h.app.save(tb.doc); err == nil {
		t.Error("an emoji cannot be Shift_JIS")
	}
	h.settle()
	if !h.has("保存できませんでした") {
		t.Errorf("the error should show:\n%s", h.tt.Dump())
	}
	if again, _ := os.ReadFile(path); string(again) != string(data) {
		t.Error("a failed save should leave the file")
	}
}

func TestEdit_Autosave(t *testing.T) {
	h, dir, tb := editSample(t, config.AutosaveAfterDelay, "go.mod")
	h.tt.Type("// auto")
	h.waitFor("the autosave", func() bool {
		data, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
		return strings.Contains(string(data), "// auto")
	})
	h.settle()
	if tb.doc.dirty {
		t.Error("after the autosave the document is clean")
	}

	// On a change of focus.
	h.app.settings.Autosave = config.AutosaveFocusChange
	h.tt.Type(" more")
	h.settle()
	h.app.openFile(filepath.Join(dir, "main.go"), false)
	h.settle()
	data, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	if !strings.Contains(string(data), "// auto more") {
		t.Errorf("leaving the tab should save it: %q", data)
	}
}

func TestEdit_CloseAsks(t *testing.T) {
	h, dir, tb := editSample(t, config.AutosaveOff, "main.go")
	before, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	h.tt.Type("zz")
	h.settle()
	h.tt.Key(input.KeyW, input.ModControl)
	h.settle()
	if !h.has("main.go の変更を保存しますか？") {
		t.Fatalf("closing a changed tab should ask:\n%s", h.tt.Dump())
	}
	h.shot("phase9-ask")
	h.tt.Tap("保存しない")
	h.settle()
	if h.app.tabs[tb.id] != nil {
		t.Error("the tab should close")
	}
	if after, _ := os.ReadFile(filepath.Join(dir, "main.go")); string(after) != string(before) {
		t.Error("保存しない should leave the file")
	}

	// The window asks too, and closes after saving.
	h.app.openFile(filepath.Join(dir, "go.mod"), false)
	h.settle()
	h.tt.Tap("go.mod")
	h.tt.Type("qq")
	h.settle()
	if h.app.confirmClose() {
		t.Fatal("the window should not close at once with changes")
	}
	h.settle()
	h.tt.Tap("保存する")
	h.settle()
	data, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	if !h.sh.closeRequested || !strings.Contains(string(data), "qq") {
		t.Errorf("保存する should save and close: closed = %v file = %q", h.sh.closeRequested, data)
	}
}

func TestEdit_Conflict(t *testing.T) {
	h, dir, tb := editSample(t, config.AutosaveOff, "main.go")
	h.tt.Type("mine")
	h.settle()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main // theirs\n"), 0o644)
	h.waitFor("the conflict", func() bool { return tb.doc.conflict })
	h.settle()
	if !h.has("ディスクのファイルが変わりました") || !strings.Contains(tb.doc.ctrl.Text(), "mine") {
		t.Fatalf("a change on disk under changes should ask, keeping them:\n%s", h.tt.Dump())
	}
	h.tt.Tap("ディスクから読み直す")
	h.settle()
	if tb.doc.dirty || tb.doc.ctrl.Text() != "package main // theirs\n" {
		t.Errorf("reading again should take the disk: %q", tb.doc.ctrl.Text())
	}
}

func TestSettings(t *testing.T) {
	h, _ := openSample(t, false)
	path := filepath.Join(t.TempDir(), "settings.json")
	h.app.settingsPath = path
	h.app.openSettings()
	h.settle()
	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), `"autosave": "afterDelay"`) {
		t.Fatalf("the settings should be written with the defaults: %q %v", data, err)
	}
	if tb := h.app.currentTab(); tb == nil || tb.doc.path != path || !tb.doc.canEdit() {
		t.Fatal("the settings should open in a tab, editable")
	}
	os.WriteFile(path, []byte(`{"fontSize": 18, "theme": "dark", "autosave": "off", "shell": "cmd.exe /q"}`), 0o644)
	h.app.applySettings()
	h.settle()
	if h.app.fontSize != 18 || h.app.settings.Theme != "dark" || h.app.settings.Autosave != config.AutosaveOff || strings.Join(h.app.shellCmd, " ") != "cmd.exe /q" {
		t.Errorf("settings = %+v font = %v shell = %v", h.app.settings, h.app.fontSize, h.app.shellCmd)
	}
	if h.app.settings.FontFamily != theme.MonoFamilies {
		t.Error("a field left out keeps its default")
	}
	h.shot("phase9-settings-dark")
	os.WriteFile(path, []byte(`{"fontSize": `), 0o644)
	h.app.applySettings()
	h.settle()
	if h.app.settingsErr == nil || !h.has("設定を使えません") {
		t.Errorf("a broken file should say so:\n%s", h.tt.Dump())
	}
}

func TestEdit_LossyIsReadOnly(t *testing.T) {
	h, dir := openSample(t, false)
	path := filepath.Join(dir, "bad.txt")
	os.WriteFile(path, []byte("ok \xff\xfe\xfd end\n"), 0o644)
	h.app.openFile(path, false)
	h.settle()
	if h.app.currentTab().doc.canEdit() || !h.has("読み取り専用") {
		t.Error("text that did not read cleanly should be read only")
	}
}

// TestEdit_QuitAsksAboutChanges keeps File → Quit from dropping changes: it
// asks, as the close button of the window does, and quits only on an answer.
func TestEdit_QuitAsksAboutChanges(t *testing.T) {
	h, dir, _ := editSample(t, config.AutosaveOff, "main.go")
	h.tt.Type("zz")
	h.settle()
	h.app.quit()
	h.settle()
	if h.sh.quitted || h.sh.closeRequested {
		t.Fatal("quitting with changes should ask first")
	}
	if !h.has("main.go の変更を保存しますか？") {
		t.Fatalf("quitting should ask about the changes:\n%s", h.tt.Dump())
	}
	h.tt.Tap("保存する")
	h.settle()
	if data, _ := os.ReadFile(filepath.Join(dir, "main.go")); !strings.Contains(string(data), "zz") {
		t.Error("保存する should save before quitting")
	}
	if !h.sh.closeRequested {
		t.Error("the window should close once saved")
	}
}

// TestEdit_QuitSavesWithAutosave saves the changes and quits, with autosave on.
func TestEdit_QuitSavesWithAutosave(t *testing.T) {
	h, dir, _ := editSample(t, config.AutosaveFocusChange, "main.go")
	h.tt.Type("yy")
	h.settle()
	h.app.quit()
	h.settle()
	if data, _ := os.ReadFile(filepath.Join(dir, "main.go")); !strings.Contains(string(data), "yy") || !h.sh.quitted {
		t.Errorf("quitting with autosave should save and quit: quitted %v", h.sh.quitted)
	}
}
