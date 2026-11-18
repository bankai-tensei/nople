package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bankai-tensei/gowaku/gowakutest"
	"github.com/bankai-tensei/gowaku/widgets/editor"
)

func TestWatch_ReloadsChangedFiles(t *testing.T) {
	h, dir := openSample(t, false)
	path := filepath.Join(dir, "internal", "util", "strings.go")
	h.app.openFile(path, false)
	h.settle()
	tb := h.app.currentTab()
	tb.view.SetSelection(editor.TextRange{Start: at(3, 5), End: at(3, 12)})
	h.settle()

	// Another program writes the file.
	body := "package util\n\n// Reverse, now changed outside.\nfunc Reverse(s string) string {\n\treturn s\n}\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	h.waitFor("the reload", func() bool { return tb.doc.ctrl.Text() == body })
	if sel := tb.view.Selection(); sel.Start != at(3, 5) {
		t.Errorf("the selection should stay, got %+v", sel)
	}
	if !h.has("Reverse, now changed outside.") && !strings.Contains(tb.doc.ctrl.Text(), "outside") {
		t.Error("the new text should show")
	}

	// A new file shows in the explorer.
	if err := os.WriteFile(filepath.Join(dir, "added.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.app.showPanel(explorerID)
	h.waitFor("the new file in the explorer", func() bool { return h.has("added.txt") })

	// A deleted file says so and keeps its text.
	h.app.dock.HideFlyout()
	h.app.activate(tb.id)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	h.waitFor("the deletion", func() bool { return tb.doc.gone })
	h.settle()
	if !h.has("ディスクから削除されました") || tb.doc.ctrl.Text() != body {
		t.Errorf("a deleted file should say so and keep its text:\n%s", h.tt.Dump())
	}
	h.shot("phase8-deleted")
}

func TestSession_SaveAndRestore(t *testing.T) {
	dir := sampleFolder(t)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(t.TempDir(), "session.json")

	h := newHarness(t, false)
	h.app.sessionPath = store
	if err := h.app.openFolder(dir); err != nil {
		t.Fatal(err)
	}
	h.settle()
	h.app.dock.HideFlyout()
	h.app.openFile(filepath.Join(dir, "main.go"), false)
	h.app.openFile(filepath.Join(dir, "README.md"), false)
	h.settle()
	h.app.toggleSource() // README.md shows its source
	h.app.openFile(filepath.Join(dir, "internal", "util", "strings.go"), false)
	h.settle()
	h.app.splitRight()
	h.settle()
	split := h.app.currentTab()
	split.view.SetSelection(editor.TextRange{Start: at(4, 1), End: at(4, 6)})
	h.app.setWrap(true)
	h.app.zoom(1)
	h.app.shellCmd = []string{"no-such-shell-for-nople"}
	h.app.newTerminal()
	h.settle()
	wantGroups := len(h.app.dock.Arrangement().Root.Children)
	if err := h.app.saveSession(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(store)
	if strings.Contains(string(data), "term:") {
		t.Error("a terminal should not be saved")
	}

	// A new window restores it.
	sh := &localShell{}
	a := newApp(sh)
	a.sessionPath = store
	tt := gowakutest.New(t, windowView{app: a}, gowakutest.Options{Width: windowWidth, Height: windowHeight, Settle: 300 * time.Millisecond})
	h2 := &harness{t: t, sh: sh, app: a, tt: tt}
	h2.settle()
	if a.ws == nil || a.ws.Root != dir {
		t.Fatalf("the folder should open again, ws = %v", a.ws)
	}
	if len(a.order) != 4 {
		t.Fatalf("four tabs should come back, got %v", a.order)
	}
	if !a.wrap || a.fontSize != h.app.fontSize {
		t.Errorf("wrap = %v font = %v", a.wrap, a.fontSize)
	}
	cur := a.currentTab()
	if cur == nil || cur.id != split.id || cur.view.Selection().Start != at(4, 1) {
		t.Errorf("the current tab and its selection should come back: %+v", cur)
	}
	var md *tab
	for _, id := range a.order {
		if a.tabs[id].doc.markdown {
			md = a.tabs[id]
		}
	}
	if md == nil || !md.source {
		t.Error("README.md should show its source again")
	}
	if got := len(a.dock.Arrangement().Root.Children); got != wantGroups {
		t.Errorf("the split should come back: %d groups, want %d", got, wantGroups)
	}
	if a.dock.Flyout() != "" {
		t.Errorf("the explorer should not slide out on restore, flyout = %q", a.dock.Flyout())
	}
	// Shared documents are shared again.
	n := 0
	for _, id := range a.order {
		if filepath.Base(a.tabs[id].doc.path) == "strings.go" {
			n++
		}
	}
	if n != 2 || a.tabs[a.order[2]].doc != a.tabs[a.order[3]].doc {
		t.Error("the two tabs of strings.go should share one document")
	}
	h2.shot("phase8-restored")
	h.app.closeTerminals()
}
