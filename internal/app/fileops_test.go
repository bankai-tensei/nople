package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/bankai-tensei/gowaku/input"
)

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestFileOps_Create(t *testing.T) {
	h, dir := openSample(t, false)
	h.app.tree.Select(filepath.Join(dir, "internal"))
	h.app.newFile()
	h.settle()
	if !h.has("新しいファイル（internal に）") {
		t.Fatalf("the dialog should ask in the selected folder:\n%s", h.tt.Dump())
	}
	h.shot("fileops-new")
	h.tt.Type("pkg/new.go")
	h.tt.Key(input.KeyEnter, 0)
	h.settle()
	p := filepath.Join(dir, "internal", "pkg", "new.go")
	if !exists(p) {
		t.Fatal("the file should be made, with its folder")
	}
	if tb := h.app.currentTab(); tb == nil || tb.doc.path != p {
		t.Error("the new file should open")
	}
	// A name that is there already keeps the dialog, with why.
	h.app.newFile()
	h.settle()
	h.tt.Type("new.go")
	h.tt.Key(input.KeyEnter, 0)
	h.settle()
	if !h.has("「new.go」はもうあります") {
		t.Errorf("a taken name should say so:\n%s", h.tt.Dump())
	}
	h.tt.Tap("キャンセル")
	h.settle()

	h.app.tree.ClearSelection()
	h.app.newFolder()
	h.settle()
	h.tt.Type("docs")
	h.tt.Key(input.KeyEnter, 0)
	h.settle()
	if st, err := os.Stat(filepath.Join(dir, "docs")); err != nil || !st.IsDir() {
		t.Error("the folder should be made at the root")
	}
	h.waitFor("the folder in the explorer", func() bool { return slices.Contains(h.app.ws.Children[dir], filepath.Join(dir, "docs")) })
}

func TestFileOps_RenameAndMove(t *testing.T) {
	h, dir := openSample(t, false)
	util := filepath.Join(dir, "internal", "util")
	file := filepath.Join(util, "strings.go")
	h.app.openFile(file, false)
	h.settle()
	tb := h.app.currentTab()

	if err := h.app.rename(file, "text.go"); err != nil {
		t.Fatal(err)
	}
	h.settle()
	renamed := filepath.Join(util, "text.go")
	if !exists(renamed) || exists(file) || tb.doc.path != renamed || !h.has("text.go") {
		t.Fatalf("the file and its tab should take the new name: %s", tb.doc.path)
	}
	if err := h.app.rename(renamed, "a/b.go"); err == nil {
		t.Error("a name with a separator should be refused")
	}

	// A folder renamed carries its open files with it.
	if err := h.app.rename(util, "strutil"); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "internal", "strutil", "text.go")
	if tb.doc.path != want {
		t.Errorf("the tab should follow its folder: %s", tb.doc.path)
	}

	// Moving to the root.
	if err := h.app.moveInto([]string{want}, dir); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(dir, "text.go")
	if !exists(moved) || tb.doc.path != moved {
		t.Errorf("the file should move to the root: %s", tb.doc.path)
	}
	// A folder cannot go into itself, and a name that is taken stays.
	internal := filepath.Join(dir, "internal")
	if err := h.app.moveInto([]string{internal}, filepath.Join(internal, "strutil")); err == nil {
		t.Error("a folder into itself should be refused")
	}
	os.WriteFile(filepath.Join(internal, "main.go"), []byte("x"), 0o644)
	if err := h.app.moveInto([]string{filepath.Join(dir, "main.go")}, internal); err == nil {
		t.Error("a taken name should be refused")
	}
	if !exists(filepath.Join(dir, "main.go")) {
		t.Error("a refused move leaves the file")
	}
}

func TestFileOps_Delete(t *testing.T) {
	h, dir := openSample(t, false)
	var trashed []string
	h.app.trash = func(p string) error {
		trashed = append(trashed, p)
		return os.RemoveAll(p)
	}
	h.app.showPanel(explorerID)
	h.settle()
	h.tt.Tap("main.go") // selects the row, and previews it
	h.settle()
	h.tt.Key(input.KeyDelete, 0)
	h.settle()
	if !h.has("「main.go」 をごみ箱に移しますか？") {
		t.Fatalf("Delete should ask:\n%s", h.tt.Dump())
	}
	h.shot("fileops-delete")
	h.tt.Tap("ごみ箱に移す")
	h.settle()
	if exists(filepath.Join(dir, "main.go")) || len(trashed) != 1 {
		t.Fatalf("the file should go to the trash: %v", trashed)
	}
	if len(h.app.tabs) != 0 {
		t.Error("the tab of a deleted file should close")
	}

	// A folder, by the menu, and the tabs of the files in it close.
	internal := filepath.Join(dir, "internal")
	h.app.openFile(filepath.Join(internal, "util", "strings.go"), false)
	h.settle()
	h.app.tree.ClearSelection()
	h.app.tree.Select(internal)
	if err := h.app.trashPaths(h.app.selectedPaths()); err != nil {
		t.Fatal(err)
	}
	h.settle()
	if exists(internal) || len(h.app.tabs) != 0 {
		t.Errorf("the folder and the tabs in it should go: tabs = %d", len(h.app.tabs))
	}
}

func TestCheckName(t *testing.T) {
	for _, bad := range []string{"", " ", ".", "..", "a/b", `a\b`, "a:b", "a?"} {
		if checkName(bad) == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
	for _, good := range []string{"main.go", ".env", "日本語.md", "a b.txt"} {
		if err := checkName(good); err != nil {
			t.Errorf("%q: %v", good, err)
		}
	}
}

func TestFileOps_ExplorerStaysOut(t *testing.T) {
	h, _ := openSample(t, false)
	h.app.trash = func(string) error { return nil }
	h.app.showPanel(explorerID)
	h.settle()
	h.tt.Tap("go.mod")
	h.settle()
	if h.app.dock.Flyout() != explorerID {
		t.Fatalf("the explorer should be out, flyout = %q", h.app.dock.Flyout())
	}
	h.tt.Key(input.KeyDelete, 0)
	h.settle()
	h.tt.Tap("キャンセル")
	h.settle()
	if h.app.dock.Flyout() != explorerID {
		t.Errorf("the explorer should be out after the dialog, flyout = %q", h.app.dock.Flyout())
	}

}

// TestFileOps_NameDialogSelectsValue checks that the name dialog opens with
// the keyboard in its field and the old name selected, so typing replaces it.
func TestFileOps_NameDialogSelectsValue(t *testing.T) {
	h, _ := openSample(t, false)
	got := ""
	h.app.askName("rename", "", "old.go", func(name string) error { got = name; return nil })
	h.settle()
	h.tt.Type("new.go")
	h.tt.Key(input.KeyEnter, 0)
	h.settle()
	if got != "new.go" {
		t.Errorf("name = %q, want new.go", got)
	}
}
