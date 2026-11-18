package app

import (
	"github.com/bankai-tensei/nople/internal/config"
	"github.com/bankai-tensei/nople/internal/workspace"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/bankai-tensei/gowaku/input"
	"github.com/bankai-tensei/gowaku/semantics"
)

func TestWalkFiles_Ignores(t *testing.T) {
	dir := sampleFolder(t)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	walk := func(gitignore bool) []string {
		var got []string
		workspace.Walk(dir, gitignore, func() bool { return false }, func(p string) bool {
			rel, _ := filepath.Rel(dir, p)
			got = append(got, filepath.ToSlash(rel))
			return false
		})
		return got
	}
	got := walk(true)
	for _, want := range []string{"main.go", "internal/util/strings.go", ".gitignore"} {
		if !slices.Contains(got, want) {
			t.Errorf("walk should find %s, got %v", want, got)
		}
	}
	for _, not := range []string{"debug.log", "build/out.txt", ".git/HEAD"} {
		if slices.Contains(got, not) {
			t.Errorf("walk should leave out %s", not)
		}
	}

	// Without .gitignore, it finds the ignored files too, but not .git.
	got = walk(false)
	for _, want := range []string{"main.go", "debug.log", "build/out.txt"} {
		if !slices.Contains(got, want) {
			t.Errorf("walk without .gitignore should find %s, got %v", want, got)
		}
	}
	if slices.Contains(got, ".git/HEAD") {
		t.Error("walk should leave out .git either way")
	}
}

func TestQuickOpen(t *testing.T) {
	h, dir := openSample(t, false)
	if !h.app.files.ready {
		t.Fatal("the files should be counted once the folder opens")
	}
	h.tt.Key(input.KeyP, input.ModControl)
	h.settle()
	h.tt.Type("strgo")
	h.settle()
	if !h.has("strings.go") {
		t.Fatalf("quick open should list strings.go:\n%s", h.tt.Dump())
	}
	h.shot("phase5-quickopen")
	h.tt.Key(input.KeyEnter, 0)
	h.settle()
	if tb := h.app.currentTab(); tb == nil || tb.doc.path != filepath.Join(dir, "internal", "util", "strings.go") {
		t.Fatalf("Enter should open strings.go, current = %v", tb)
	}
	if !h.app.currentTab().view.Focused() {
		t.Error("the opened file should take the keyboard")
	}

	// A line after a colon goes to it.
	rows, _ := h.app.quickOpenItems("main:6")
	if len(rows) == 0 || rows[0].label != "main.go" {
		t.Fatalf("rows = %+v", rows)
	}
	rows[0].run()
	h.settle()
	if sel := h.app.currentTab().view.Selection(); sel.Start.Line != 5 {
		t.Errorf("main:6 should go to line 6, selection = %+v", sel)
	}
}

func TestGotoLine(t *testing.T) {
	h, dir := openSample(t, false)
	h.app.openFile(filepath.Join(dir, "internal", "util", "strings.go"), false)
	h.settle()
	h.tt.Key(input.KeyG, input.ModControl)
	h.settle()
	h.tt.Type("6:3")
	h.settle()
	if !h.has("6 行目の 3 文字目へ移る") {
		t.Fatalf("the picker should offer the line:\n%s", h.tt.Dump())
	}
	h.shot("phase5-gotoline")
	h.tt.Key(input.KeyEnter, 0)
	h.settle()
	sel := h.app.currentTab().view.Selection()
	if sel.End.Line != 5 || sel.End.Column != 2 {
		t.Errorf("selection = %+v, want line 5 column 2", sel)
	}
	if !h.has("行 6、列 3") {
		t.Errorf("the status bar should follow:\n%s", h.tt.Dump())
	}
	// The keyboard is in the editor, so typing goes to the line.
	h.tt.Type("X")
	h.settle()
	if got := h.app.currentTab().doc.ctrl.Line(5); !strings.Contains(got, "X") {
		t.Errorf("typing after Ctrl+G should edit line 6: %q", got)
	}
}

func TestCommandPalette(t *testing.T) {
	h, _ := openSample(t, false)
	h.tt.Key(input.KeyP, input.ModControl|input.ModShift)
	h.settle()
	h.tt.Type("折り返し")
	h.settle()
	h.shot("phase5-palette")
	h.tt.Key(input.KeyEnter, 0)
	h.settle()
	if !h.app.wrap {
		t.Error("the palette should run 折り返し")
	}
}

func TestSearch(t *testing.T) {
	h, dir := openSample(t, false)
	h.app.changeSettings(func(s *config.Settings) { s.Gitignore = true })
	h.settle()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("reverse it\nREVERSE\n\tthe Reverse of the reverse\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "debug.log"), []byte("Reverse in an ignored file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.app.search(searchOptions{query: "reverse"})
	h.settle()
	f := &h.app.find
	if f.running || f.count != 6 || len(f.files) != 2 {
		t.Fatalf("running = %v, %d matches in %d files: %+v", f.running, f.count, len(f.files), f.files)
	}
	for _, fh := range f.files {
		if strings.HasSuffix(fh.path, "debug.log") {
			t.Error("an ignored file should not be searched")
		}
	}
	h.app.showPanel(searchID)
	h.settle()
	if !h.has("2 ファイルに 6 件") || !h.has("3:6: the Reverse of the reverse") || !h.has("3:21: the Reverse of the reverse") {
		t.Errorf("the panel should show the results:\n%s", h.tt.Dump())
	}
	h.shot("phase5-search")
	if _, ok := h.tt.FindFunc(isProgress); ok {
		t.Error("no spinner should turn once the search is done")
	}
	// While it runs, a spinner turns beside the summary.
	f.running = true
	h.app.refresh()
	h.settle()
	if _, ok := h.tt.FindFunc(isProgress); !ok {
		t.Errorf("a running search should show a spinner:\n%s", h.tt.Dump())
	}
	h.shot("phase5-search-running")
	f.running = false
	h.app.refresh()
	h.settle()

	// A match opens with its text selected.
	notes := filepath.Join(dir, "notes.txt")
	h.app.openHit(hitKey(notes, 2), false)
	h.settle()
	tb := h.app.currentTab()
	if tb == nil || tb.doc.path != notes {
		t.Fatalf("the match should open notes.txt, current = %v", tb)
	}
	if got := tb.doc.ctrl.TextIn(tb.view.Selection().Ordered()); got != "Reverse" {
		t.Errorf("the match should be selected, got %q", got)
	}

	h.app.search(searchOptions{query: "Reverse", matchCase: true})
	h.settle()
	if f.count != 3 {
		t.Errorf("matching case should find 3, got %d", f.count)
	}
	h.app.search(searchOptions{query: "rev(erse", regex: true})
	h.settle()
	if f.err == nil || !h.has("正規表現を読めません") && !strings.Contains(searchSummary(f), "正規表現") {
		t.Errorf("a bad pattern should say so, err = %v", f.err)
	}
	h.app.search(searchOptions{query: "re", word: true})
	h.settle()
	if f.count != 0 {
		t.Errorf("a whole word re should find nothing, got %d", f.count)
	}
}

func TestSearch_MarkdownShowsSource(t *testing.T) {
	h, dir := openSample(t, false)
	h.app.search(searchOptions{query: "project"})
	h.settle()
	readme := filepath.Join(dir, "README.md")
	h.app.openHit(hitKey(readme, 0), false)
	h.settle()
	tb := h.app.currentTab()
	if tb.showsMarkdown() {
		t.Error("a match in Markdown should show in the source")
	}
}

// isProgress matches a progress indicator, whatever its label.
func isProgress(n *semantics.Node) bool { return n.Role == semantics.RoleProgress }
