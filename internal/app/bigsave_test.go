package app

import (
	"os"
	"strings"
	"testing"

	"github.com/bankai-tensei/gowaku/input"
	"github.com/bankai-tensei/gowaku/widgets/editor"

	"github.com/bankai-tensei/nople/internal/bigfile"
)

// holdBigSaves keeps each save of a large file from writing until release
// runs, and shows how far a save is at once.
func holdBigSaves(t *testing.T) (release func()) {
	gate := make(chan struct{})
	bigfile.HoldSave = func() { <-gate }
	show := saveShowAfter
	saveShowAfter = 0
	released := false
	release = func() {
		if !released {
			released = true
			close(gate)
		}
	}
	t.Cleanup(func() {
		release()
		bigfile.HoldSave = nil
		saveShowAfter = show
	})
	return release
}

// openBigEdited opens a large log, waits for its index and edits its start.
func openBigEdited(t *testing.T) (*harness, *tab, string, string) {
	t.Helper()
	h, dir := openSample(t, false)
	path, content := largeLog(t, dir)
	h.app.openFile(path, false)
	h.settle()
	tb := h.app.currentTab()
	h.waitFor("the index", func() bool { return !tb.doc.big.Loading() })
	tb.doc.ctrl.Replace(editor.TextRange{}, "mine ")
	h.app.edited(tb.doc)
	h.settle()
	return h, tb, path, content
}

func TestBigFile_SavesInTheBackground(t *testing.T) {
	h, tb, path, content := openBigEdited(t)
	d := tb.doc
	release := holdBigSaves(t)
	h.app.focusEditor(tb)
	h.tt.Key(input.KeyS, input.ModControl)
	h.settle()
	if !d.saving() {
		t.Fatal("Ctrl+S should begin the save of a large file")
	}

	// The window goes on while the save runs: it draws, tells how far the
	// save is, moves the caret and finds, but does not edit.
	if !h.has("保存中 0%") || !strings.Contains(tabTitle(tb), "保存中 0%") {
		t.Errorf("the status bar and the tab should tell how far the save is: %q\n%s", tabTitle(tb), h.tt.Dump())
	}
	h.shot("bigfile-saving")
	h.tt.Type("typed")
	h.tt.Key(input.KeyEnd, input.ModControl)
	h.settle()
	if got := d.ctrl.Line(0); !strings.HasPrefix(got, "mine line 0000000") {
		t.Errorf("typing should not edit a file that saves: %q", got)
	}
	if sel := tb.view.Selection(); sel.End.Line != d.ctrl.LineCount()-1 {
		t.Errorf("Ctrl+End should go to the end while saving, selection %+v", sel)
	}
	if n, err := d.ctrl.Find("line 0000100 ", editor.FindOptions{}); err != nil || n == 0 && !d.ctrl.Searching() {
		t.Errorf("finding while saving: %d, %v", n, err)
	}
	// Saving again waits for the save that runs.
	h.app.saveCurrent()
	h.settle()
	if data, _ := os.ReadFile(path); string(data) != content {
		t.Fatal("the file changed before the save wrote it")
	}

	release()
	h.waitFor("the end of the save", func() bool { return !d.saving() })
	if data, _ := os.ReadFile(path); string(data) != "mine "+content {
		t.Fatalf("the save should write the edit and the rest as it was")
	}
	if d.dirty || d.saveErr != nil || strings.Contains(tabTitle(tb), "保存中") || h.has("保存中 100%") {
		t.Errorf("a save that ended should leave nothing to show: dirty %v, %v, %q", d.dirty, d.saveErr, tabTitle(tb))
	}
	// Editing works again, and the undo history stayed.
	h.tt.Key(input.KeyHome, input.ModControl)
	h.tt.Type("again ")
	h.settle()
	if got := d.ctrl.Line(0); !strings.HasPrefix(got, "again mine") || !d.dirty {
		t.Errorf("after the save, typing should edit: %q", got)
	}
}

func TestBigFile_ClosingTheTabWaitsForTheSave(t *testing.T) {
	h, tb, path, content := openBigEdited(t)
	release := holdBigSaves(t)
	h.app.save(tb.doc)
	h.settle()
	h.app.closeTabAsking(tb.id)
	h.settle()
	if !h.has("big.log を保存しています") {
		t.Fatalf("closing a tab that saves should ask:\n%s", h.tt.Dump())
	}
	h.shot("bigfile-saving-close")
	h.tt.Tap("保存が終わったら閉じる")
	h.settle()
	if h.app.tabs[tb.id] == nil {
		t.Fatal("the tab should stay until the save ended")
	}
	release()
	h.waitFor("the tab to close", func() bool { return h.app.tabs[tb.id] == nil })
	if data, _ := os.ReadFile(path); string(data) != "mine "+content {
		t.Errorf("the save should land before the tab closes")
	}
}

func TestBigFile_CancelingKeepsTheTab(t *testing.T) {
	h, tb, _, _ := openBigEdited(t)
	release := holdBigSaves(t)
	h.app.save(tb.doc)
	h.settle()
	if h.app.tabCloseRequest(tb.id) {
		t.Fatal("the dock should not close the tab of a file that saves")
	}
	h.settle()
	h.tt.Tap("キャンセル")
	h.settle()
	release()
	h.waitFor("the end of the save", func() bool { return !tb.doc.saving() })
	if h.app.tabs[tb.id] == nil {
		t.Error("Cancel should keep the tab")
	}
}

func TestBigFile_TheWindowClosesOnceSaved(t *testing.T) {
	h, tb, path, content := openBigEdited(t)
	release := holdBigSaves(t)
	h.app.save(tb.doc)
	h.settle()
	if h.app.confirmClose() {
		t.Fatal("the window should not close while a save runs")
	}
	h.settle()
	h.tt.Tap("保存が終わったら閉じる")
	h.settle()
	if h.sh.closeRequested {
		t.Fatal("the window should wait for the save")
	}
	release()
	h.waitFor("the window to close", func() bool { return h.sh.closeRequested })
	if data, _ := os.ReadFile(path); string(data) != "mine "+content {
		t.Errorf("the save should land before the window closes")
	}
	if !h.app.confirmClose() {
		t.Error("once saved, the window should close")
	}
}

func TestBigFile_QuitWaitsForTheSave(t *testing.T) {
	h, tb, path, content := openBigEdited(t)
	release := holdBigSaves(t)
	h.app.save(tb.doc)
	h.settle()
	h.app.quit()
	h.settle()
	if h.sh.quitted || h.sh.closeRequested {
		t.Fatal("quitting should wait for the save")
	}
	// The dialog asks whether to wait; waiting, the window closes once the
	// save ended.
	if h.has("保存が終わったら閉じる") {
		h.tt.Tap("保存が終わったら閉じる")
		h.settle()
	}
	release()
	h.waitFor("the quit", func() bool { return h.sh.quitted || h.sh.closeRequested })
	if data, _ := os.ReadFile(path); string(data) != "mine "+content {
		t.Errorf("the save should land before the app quits")
	}
}
