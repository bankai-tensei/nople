package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bankai-tensei/gowaku/input"
	"github.com/bankai-tensei/gowaku/widgets/editor"

	"github.com/bankai-tensei/nople/internal/config"
	"github.com/bankai-tensei/nople/internal/git"
	"github.com/bankai-tensei/nople/internal/i18n"
	"github.com/bankai-tensei/nople/internal/syntax"
	"github.com/bankai-tensei/nople/internal/textfile"
)

// largeLog writes a log above textfile.LargeSize into dir.
func largeLog(t *testing.T, dir string) (string, string) {
	t.Helper()
	var b strings.Builder
	for i := 0; b.Len() <= textfile.LargeSize; i++ {
		fmt.Fprintf(&b, "line %07d of a log that is large enough to open through bigfile\n", i)
	}
	path := filepath.Join(dir, "big.log")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, b.String()
}

func TestBigFile_OpensEditsSavesAndFollows(t *testing.T) {
	h, dir := openSample(t, false)
	path, content := largeLog(t, dir)
	h.app.openFile(path, false)
	h.settle()
	tb := h.app.currentTab()
	d := tb.doc
	if d.big == nil {
		t.Fatalf("a file of %d bytes should open through bigfile", len(content))
	}
	h.waitFor("the index", func() bool { return !d.big.Loading() })
	lines := strings.Count(content, "\n") + 1
	if n := d.ctrl.LineCount(); n != lines {
		t.Fatalf("%d lines, want %d", n, lines)
	}
	if h.has("サイズが大きいため、一部の表示を省いています（" + sizeText(d.content.Size) + "）") {
		t.Errorf("a log outside git leaves nothing out, and needs no notice:\n%s", h.tt.Dump())
	}

	// An edit, saved: the file holds it and the rest as it was.
	d.ctrl.Replace(editor.TextRange{}, "mine ")
	h.app.edited(d)
	if err := h.app.save(d); err != nil {
		t.Fatal(err)
	}
	h.waitFor("the save", func() bool { return !d.saving() })
	saved, _ := os.ReadFile(path)
	if string(saved) != "mine "+content || d.dirty {
		t.Fatalf("saved %d bytes, dirty %v", len(saved), d.dirty)
	}

	// A program appends to the log: the text follows it.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("appended outside\n")
	f.Close()
	h.waitFor("the append", func() bool {
		n := d.ctrl.LineCount()
		return n == lines+1 && d.ctrl.Line(n-2) == "appended outside"
	})

	// Another program rewrites the file while it has an edit: the edit
	// stays, placed in the new contents.
	d.ctrl.Replace(editor.TextRange{Start: editor.TextPosition{Line: 10}, End: editor.TextPosition{Line: 10}}, "edited ")
	h.app.edited(d)
	now, _ := os.ReadFile(path)
	tmp := path + ".new"
	if err := os.WriteFile(tmp, append([]byte("theirs\n"), now...), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatalf("another program must be able to replace the open file: %v", err)
	}
	h.waitFor("the reload keeping the edit", func() bool { return d.ctrl.Line(0) == "theirs" })
	if got := d.ctrl.Line(11); !strings.HasPrefix(got, "edited line 0000010") || !d.dirty || d.conflict {
		t.Errorf("line 11 %q, dirty %v, conflict %v", got, d.dirty, d.conflict)
	}
	if err := h.app.save(d); err != nil {
		t.Fatal(err)
	}
	h.waitFor("the save", func() bool { return !d.saving() })
	saved, _ = os.ReadFile(path)
	if !strings.HasPrefix(string(saved), "theirs\nmine line 0000000") || !strings.Contains(string(saved), "\nedited line 0000010") {
		t.Errorf("saved %q…", saved[:80])
	}
	h.app.closeTab(tb.id)
}

func TestBigFile_ColorsTheLinesInView(t *testing.T) {
	h, dir := openSample(t, false)
	var b strings.Builder
	paths := []string{"/api/items", "/api/items/42", "/login", "/api/search?q=nople", "/static/app.js"}
	for i := 0; b.Len() <= textfile.LargeSize; i++ {
		ts := fmt.Sprintf("2026-11-18T09:%02d:%02d.%03dZ", i/60%60, i%60, i*37%1000)
		switch i % 10 {
		case 3:
			fmt.Fprintf(&b, "%s ERROR worker-%d: request %07d failed: \"connection reset by peer\"\n", ts, i%8, i)
		case 5:
			fmt.Fprintf(&b, "%s DEBUG cache hit key=\"session:%d\" age=%dms\n", ts, i%997, i%300)
		case 7:
			fmt.Fprintf(&b, "%s WARN  slow query took %d ms: \"SELECT * FROM items\"\n", ts, 800+i%400)
		default:
			fmt.Fprintf(&b, "%s INFO  request %07d \"GET %s\" 200 in %d ms\n", ts, i, paths[i%len(paths)], 3+i%40)
		}
	}
	path := filepath.Join(dir, "server.log")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	h.app.openFile(path, false)
	h.settle()
	d := h.app.currentTab().doc
	if d.big == nil || d.bighl == nil {
		t.Fatalf("a large log should open through bigfile with colors: big %v", d.big != nil)
	}
	h.waitFor("the colors of the first block", func() bool {
		blk := d.bighl.blocks[0]
		return blk != nil && blk.h != nil && len(blk.h.Lines) > 3
	})
	spans := d.bighl.blocks[0].h.Lines[3]
	kinds := map[syntax.Kind]bool{}
	for _, sp := range spans {
		kinds[sp.Kind] = true
	}
	if !kinds[syntax.KindComment] || !kinds[syntax.KindDeleted] || !kinds[syntax.KindString] {
		t.Errorf("line 3 should show its time, its ERROR and its string: %v", spans)
	}
	h.settle()
	h.shot("big-log")
	if os.Getenv("NOPLE_SHOTS") != "" {
		i18n.Set("en")
		defer i18n.Set("ja")
		h.app.refresh()
		h.settle()
		h.shot("big-log-en")
	}
	// An edit throws the colors away, and the lines in view are lexed again.
	d.ctrl.Replace(editor.TextRange{}, "x")
	h.app.edited(d)
	h.app.refill(d)
	h.waitFor("the colors again", func() bool {
		blk := d.bighl.blocks[0]
		return blk != nil && blk.h != nil
	})
}

// TestBigFile_TypeSaveUndoByKeys drives the UI as a user does: it types in a
// large file with CRLF breaks, saves with Ctrl+S, undoes with Ctrl+Z after the
// save, and saves again, checking the file each time to the byte.
func TestBigFile_TypeSaveUndoByKeys(t *testing.T) {
	h, dir := openSample(t, false)
	h.app.settings.Autosave = config.AutosaveOff
	var b strings.Builder
	for i := 0; b.Len() <= textfile.LargeSize; i++ {
		fmt.Fprintf(&b, "line %07d of a log with CRLF breaks\r\n", i)
	}
	content := b.String()
	path := filepath.Join(dir, "crlf.log")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	h.app.openFile(path, false)
	h.settle()
	tb := h.app.currentTab()
	d := tb.doc
	if d.big == nil {
		t.Fatal("the file should open through bigfile")
	}
	h.waitFor("the index", func() bool { return !d.big.Loading() })
	h.tt.Tap(d.title())
	h.settle()
	disk := func() string {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	save := func() {
		h.tt.Key(input.KeyS, input.ModControl)
		h.settle()
		h.waitFor("the save", func() bool { return !d.saving() })
		if d.dirty || d.saveErr != nil {
			t.Fatalf("Ctrl+S should save: dirty %v, error %v", d.dirty, d.saveErr)
		}
	}

	// The tap that focused the editor put the caret where it landed.
	h.tt.Key(input.KeyHome, input.ModControl)
	h.tt.Type("head ")
	h.tt.Key(input.KeyEnd, input.ModControl)
	h.tt.Type("tail")
	h.settle()
	save()
	if got := disk(); got != "head "+content+"tail" {
		i := strings.Index(got, "head ")
		t.Fatalf("saved %d bytes, want %d; start %q, end %q; head at %d (%q), caret %+v", len(got), len(content)+9, got[:20], got[len(got)-20:], i, got[max(0, i-40):min(len(got), i+20)], tb.view.Selection())
	}
	// Undo goes on working after the save, and what it gives back saves.
	h.tt.Key(input.KeyZ, input.ModControl)
	h.settle()
	save()
	if got := disk(); got != "head "+content {
		t.Fatalf("after one undo, saved %d bytes, end %q", len(got), got[len(got)-20:])
	}
	h.tt.Key(input.KeyZ, input.ModControl)
	h.settle()
	save()
	if got := disk(); got != content {
		t.Fatalf("after undoing all, the file should be as it was: %d bytes, start %q", len(got), got[:20])
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("a temp file was left: %s", e.Name())
		}
	}
}

// TestBigFile_NoAutosaveWhileTyping keeps a large file from being saved each
// time typing pauses, which would freeze the window for its whole save.
func TestBigFile_NoAutosaveWhileTyping(t *testing.T) {
	h, dir := openSample(t, false)
	h.app.settings.Autosave = config.AutosaveAfterDelay
	h.app.settings.AutosaveDelay = 10
	path, content := largeLog(t, dir)
	h.app.openFile(path, false)
	h.settle()
	d := h.app.currentTab().doc
	h.waitFor("the index", func() bool { return !d.big.Loading() })
	d.ctrl.Replace(editor.TextRange{}, "typed ")
	h.app.edited(d)
	time.Sleep(100 * time.Millisecond)
	h.settle()
	if data, _ := os.ReadFile(path); string(data) != content || !d.dirty {
		t.Errorf("a pause in typing should not save a large file: dirty %v", d.dirty)
	}
}

// TestBigFile_NoticeSaysWhatIsLeftOut shows the notice of a large file only
// with what it goes without: Git changes for a file git tracks, the outline
// for Markdown, and no notice at all for a log git does not track.
func TestBigFile_NoticeSaysWhatIsLeftOut(t *testing.T) {
	dir := gitSample(t)
	var b strings.Builder
	for i := 0; b.Len() <= textfile.LargeSize; i++ {
		fmt.Fprintf(&b, "## Section %d\n\nSome text of the section.\n\n", i)
	}
	big := filepath.Join(dir, "big.md")
	if err := os.WriteFile(big, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "big.md"}, {"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "big"}} {
		if out, err := git.Run(dir, args...); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	untracked, _ := largeLog(t, dir)
	h := newHarness(t, false)
	if err := h.app.openFolder(dir); err != nil {
		t.Fatal(err)
	}
	h.settle()
	h.app.openFile(big, false)
	h.settle()
	tb := h.app.currentTab()
	h.waitFor("git to say it tracks the file", func() bool { return tb.doc.tracked })
	bar := notice(h.app, tb)
	if !bar.Open || bar.Message != "Git の変更は表示されません。\nアウトラインは非表示になりました。" {
		t.Errorf("a large Markdown file git tracks: open %v, %q", bar.Open, bar.Message)
	}
	h.app.dock.HideFlyout()
	h.settle()
	h.shot("big-notice")

	h.app.openFile(untracked, false)
	h.settle()
	tb = h.app.currentTab()
	h.settle()
	if bar := notice(h.app, tb); bar.Open || tb.doc.tracked {
		t.Errorf("a log git does not track leaves nothing out, and needs no notice: %q", bar.Title)
	}
}
