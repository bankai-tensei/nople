package app

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/bankai-tensei/gowaku/input"
	"github.com/bankai-tensei/nople/internal/config"
)

// The tests of what gowaku took over from Nople (PLAN §5 #14, #19 to #21).

// closeButton is the label of the × of the tab title.
func (h *harness) closeButton(t *testing.T, title string) string {
	for _, l := range []string{title + " を閉じる", title + "を閉じる", "Close " + title, title + " を閉じます"} {
		if _, ok := h.tt.Find(l); ok {
			return l
		}
	}
	t.Fatalf("no close button of %s:\n%s", title, h.tt.Dump())
	return ""
}

func TestFollow_DockCloseAsksFirst(t *testing.T) {
	h, _, tb := editSample(t, config.AutosaveOff, "main.go")
	h.tt.Type("zz")
	h.settle()
	h.tt.Tap(h.closeButton(t, "● main.go"))
	h.settle()
	if !h.has("main.go の変更を保存しますか？") {
		t.Fatalf("the × of a changed tab should ask first:\n%s", h.tt.Dump())
	}
	if h.app.tabs[tb.id] == nil {
		t.Fatal("the tab should stay while the user is asked")
	}
	h.tt.Tap("キャンセル")
	h.settle()
	if h.app.tabs[tb.id] == nil || !tb.doc.dirty || !strings.Contains(tb.doc.ctrl.Text(), "zz") {
		t.Error("キャンセル should keep the tab and its changes")
	}
}

func TestFollow_Keys(t *testing.T) {
	h, dir := openSample(t, false)
	h.app.openFile(filepath.Join(dir, "main.go"), false)
	h.settle()
	h.tt.Key(input.KeyBackslash, input.ModControl)
	h.settle()
	if n := len(h.app.tabs); n != 2 {
		t.Errorf("Ctrl+\\ should split the tab: %d tabs", n)
	}
	size := h.app.fontSize
	h.tt.Key(input.KeySemicolon, input.ModControl)
	h.settle()
	if h.app.fontSize <= size {
		t.Errorf("Ctrl+; should make the text bigger: %v -> %v", size, h.app.fontSize)
	}
}

func TestFollow_ExplorerMenuKeepsFlyout(t *testing.T) {
	h, dir := openSample(t, false)
	h.app.showPanel(explorerID)
	h.settle()
	h.tt.Tap("go.mod")
	h.settle()
	if h.app.dock.Flyout() != explorerID {
		t.Fatalf("the explorer should be out, flyout = %q", h.app.dock.Flyout())
	}
	// Shift+F10 on the row asks the tree for its menu (#20).
	h.tt.Key(input.KeyF10, input.ModShift)
	h.settle()
	if !h.has("名前の変更(M)") {
		t.Fatalf("Shift+F10 should open the menu of the row:\n%s", h.tt.Dump())
	}
	if h.app.dock.Flyout() != explorerID {
		t.Errorf("the menu should keep the explorer out (#21), flyout = %q", h.app.dock.Flyout())
	}
	h.tt.Tap("名前の変更(M)")
	h.settle()
	if h.app.dock.Flyout() != explorerID {
		t.Errorf("the dialog should keep the explorer out (#21), flyout = %q", h.app.dock.Flyout())
	}
	if sel := h.app.selectedPaths(); len(sel) != 1 || sel[0] != filepath.Join(dir, "go.mod") {
		t.Errorf("the menu should act on the row: %v", sel)
	}
}
