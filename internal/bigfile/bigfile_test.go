package bigfile

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bankai-tensei/gowaku/widgets/editor"
)

// ui stands for the UI goroutine: what is posted to it runs when the test
// pumps it.
type ui chan func()

func newUI() ui { return make(ui, 1<<16) }

func (u ui) post(fn func()) { u <- fn }

// pump runs what was posted until cond holds, or fails after a while.
func (u ui) pump(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for !cond() {
		select {
		case fn := <-u:
			fn()
		case <-deadline:
			t.Fatalf("%s never happened", what)
		}
	}
}

// drain runs what was posted within d.
func (u ui) drain(d time.Duration) {
	end := time.After(d)
	for {
		select {
		case fn := <-u:
			fn()
		case <-end:
			return
		}
	}
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "doc.txt")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// normalized is content as the text holds it.
// bomText is the byte order mark of UTF-8.
const bomText = "\xef\xbb\xbf"

func normalized(content string) string {
	return strings.ReplaceAll(strings.TrimPrefix(content, bomText), "\r\n", "\n")
}

// smallSteps makes the index go in small steps, as for a file of gigabytes.
func smallSteps(t *testing.T) {
	s, c, r := syncIndex, indexChunk, indexReport
	syncIndex, indexChunk, indexReport = 64, 97, 300
	t.Cleanup(func() { syncIndex, indexChunk, indexReport = s, c, r })
}

func numbered(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "line %d of the file\n", i)
	}
	return b.String()
}

func clip(s string) string {
	if len(s) > 200 {
		return s[:100] + "…" + s[len(s)-100:]
	}
	return s
}

// checkSource checks the lines and offsets of s against the text want.
func checkSource(t *testing.T, s editor.LineSource, want string) {
	t.Helper()
	lines := strings.Split(want, "\n")
	if s.Len() != len(lines) {
		t.Fatalf("Len = %d, want %d", s.Len(), len(lines))
	}
	off := int64(0)
	for i, l := range lines {
		if got := s.Line(i); got != l {
			t.Fatalf("Line(%d) = %q, want %q", i, got, l)
		}
		if got := s.Offset(i); got != off {
			t.Fatalf("Offset(%d) = %d, want %d", i, got, off)
		}
		for _, k := range []int64{0, int64(len(l))} {
			if got := s.LineAt(off + k); got != i {
				t.Fatalf("LineAt(%d) = %d, want %d", off+k, got, i)
			}
		}
		off += int64(len(l)) + 1
	}
	if got := s.Offset(s.Len()); got != off {
		t.Fatalf("Offset(Len) = %d, want %d", got, off)
	}
}

func open(t *testing.T, ctl *editor.Controller, path string, opts Options) *Doc {
	t.Helper()
	d, err := Open(ctl, path, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestOpen_ShowsTheLinesOfAFile(t *testing.T) {
	for _, content := range []string{
		"", "one", "one\n", "a\nb\r\nc", "a\r\nb\r\n", "lone\rcr\n\r\n", bomText + "bom\r\nfile",
		numbered(1000),
	} {
		var ctl editor.Controller
		d := open(t, &ctl, writeFile(t, content), Options{})
		if d.Loading() {
			t.Fatalf("%q: a file opened without Post is indexed before Open returns", content)
		}
		want := normalized(content)
		checkSource(t, d.File(), want)
		if got := ctl.Text(); got != want {
			t.Fatalf("%q: Text = %q", content, got)
		}
	}
}

func TestOpen_IndexesInStepsAndGrows(t *testing.T) {
	smallSteps(t)
	content := numbered(2000)
	u := newUI()
	var ctl editor.Controller
	steps := 0
	var last [2]int64
	d := open(t, &ctl, writeFile(t, content), Options{Post: u.post, OnLoad: func(loaded, size int64) {
		steps++
		last = [2]int64{loaded, size}
	}})
	if !d.Loading() || ctl.LineCount() >= 2001 {
		t.Fatalf("the file must still be loading, with %d lines so far", ctl.LineCount())
	}
	u.pump(t, "the end of the index", func() bool { return !d.Loading() })
	if steps < 3 || last[0] != last[1] || last[1] != int64(len(content)) {
		t.Errorf("OnLoad ran %d times, last with %v", steps, last)
	}
	checkSource(t, d.File(), content)
	if ctl.Text() != content {
		t.Errorf("the text is %q", clip(ctl.Text()))
	}
}

// edit makes a random edit of ctl, and the same of want.
func edit(r *rand.Rand, ctl *editor.Controller, want *string) {
	lines := ctl.LineCount()
	at := func() editor.TextPosition {
		l := r.IntN(lines)
		return editor.TextPosition{Line: l, Column: r.IntN(len(ctl.Line(l)) + 1)}
	}
	rg := editor.TextRange{Start: at(), End: at()}.Ordered()
	if r.IntN(3) == 0 {
		rg.End = rg.Start
	}
	ins := []string{"", "x", "new\nlines", "\n", "é"}[r.IntN(5)]
	a, b := ctl.Offset(rg.Start), ctl.Offset(rg.End)
	*want = (*want)[:a] + ins + (*want)[b:]
	ctl.Replace(rg, ins)
}

func TestOpen_EditsWhileLoadingMatchAString(t *testing.T) {
	smallSteps(t)
	content := numbered(3000)
	u := newUI()
	var ctl editor.Controller
	d := open(t, &ctl, writeFile(t, content), Options{Post: u.post})
	r := rand.New(rand.NewPCG(1, 2))
	// The text known while loading, with the edits, and the file past it.
	want := ctl.Text()
	lines := strings.Split(content, "\n")
	for step := 0; d.Loading() || step < 200; step++ {
		select {
		case fn := <-u:
			known := ctl.Source().Len()
			fn()
			if n := ctl.Source().Len(); n > known {
				// The lines found join the text at its end, after the edits.
				want += "\n" + strings.Join(lines[known:n], "\n")
			}
		default:
		}
		edit(r, &ctl, &want)
		if got := ctl.Text(); got != want {
			t.Fatalf("step %d: %q, want %q", step, clip(got), clip(want))
		}
	}
	// What is saved is what the text shows.
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, d.Path()); got != want {
		t.Fatalf("saved %q, want %q", clip(got), clip(want))
	}
}

func TestSave_KeepsTheBreaksAndTheBOM(t *testing.T) {
	content := bomText + "first\r\nsecond\nthird\r\nlone\rcr\r\nlast"
	path := writeFile(t, content)
	var ctl editor.Controller
	d := open(t, &ctl, path, Options{})
	// A new line after the second, in the break of the first line.
	ctl.Replace(editor.TextRange{Start: editor.TextPosition{Line: 1, Column: 6}, End: editor.TextPosition{Line: 1, Column: 6}}, "\nadded")
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	want := bomText + "first\r\nsecond\r\nadded\r\nthird\r\nlone\rcr\r\nlast"
	if got := readFile(t, path); got != want {
		t.Fatalf("saved %q, want %q", got, want)
	}
	// The doc now shows the file written, indexed as it was written, and the
	// controller took it as the same text, BOM and all.
	checkSource(t, d.File(), normalized(want))
	if !ctl.CanUndo() || ctl.Source() != d.File() {
		t.Errorf("Save should keep the undo history through ReplaceSource")
	}
	if d.Check() != Unchanged {
		t.Errorf("the file saved looks changed: %v", d.Check())
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("the folder holds %d files, want only the document", len(entries))
	}
}

func TestSave_EditsAtTheEnds(t *testing.T) {
	for _, tc := range []struct {
		content string
		edit    func(*editor.Controller)
		want    string
	}{
		{"a\nb", func(c *editor.Controller) { c.Replace(editor.TextRange{}, "start ") }, "start a\nb"},
		{"a\nb", func(c *editor.Controller) {
			p := editor.TextPosition{Line: 1, Column: 1}
			c.Replace(editor.TextRange{Start: p, End: p}, "\nend")
		}, "a\nb\nend"},
		{"a\r\nb\r\n", func(c *editor.Controller) {
			p := editor.TextPosition{Line: 2}
			c.Replace(editor.TextRange{Start: p, End: p}, "c")
		}, "a\r\nb\r\nc"},
		{"a\nb\nc", func(c *editor.Controller) {
			c.Replace(editor.TextRange{Start: editor.TextPosition{Line: 0, Column: 1}, End: editor.TextPosition{Line: 2}}, "")
		}, "ac"},
		{"", func(c *editor.Controller) { c.Replace(editor.TextRange{}, "x\ny") }, "x\ny"},
	} {
		path := writeFile(t, tc.content)
		var ctl editor.Controller
		d := open(t, &ctl, path, Options{})
		tc.edit(&ctl)
		if err := d.Save(); err != nil {
			t.Fatal(err)
		}
		if got := readFile(t, path); got != tc.want {
			t.Errorf("%q: saved %q, want %q", tc.content, got, tc.want)
		}
	}
}

func TestSave_RefusesWhileLoading(t *testing.T) {
	smallSteps(t)
	u := newUI()
	var ctl editor.Controller
	d := open(t, &ctl, writeFile(t, numbered(2000)), Options{Post: u.post})
	if err := d.Save(); err != ErrLoading {
		t.Errorf("Save while loading: %v", err)
	}
	u.pump(t, "the end of the index", func() bool { return !d.Loading() })
	if err := d.Save(); err != nil {
		t.Errorf("Save once loaded: %v", err)
	}
}

func appendTo(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// rewrite writes s over the start of the file, in place.
func rewrite(t *testing.T, path string, off int64, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte(s), off); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// keepTime writes s at off without moving the time of the file.
func keepTime(t *testing.T, path string, off int64, s string) {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	rewrite(t, path, off, s)
	if err := os.Chtimes(path, st.ModTime(), st.ModTime()); err != nil {
		t.Fatal(err)
	}
}

// replace writes content as another program saves: a new file renamed over
// the old one.
func replace(t *testing.T, path, content string) {
	t.Helper()
	tmp := path + ".new"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatalf("another program must be able to replace the open file: %v", err)
	}
}

func TestCheck_TellsHowTheFileChanged(t *testing.T) {
	content := numbered(300)
	cases := []struct {
		name   string
		change func(path string)
		want   Change
	}{
		{"nothing", func(string) {}, Unchanged},
		{"an append", func(p string) { appendTo(t, p, "more\n") }, Appended},
		{"a rewrite", func(p string) { rewrite(t, p, 5, "X") }, Rewritten},
		{"a shorter file", func(p string) { os.Truncate(p, 10) }, Rewritten},
		{"a replacement", func(p string) { replace(t, p, "other\n") }, Replaced},
		{"a deletion", func(p string) { os.Remove(p) }, Replaced},
		{"bytes in view without the time", func(p string) { keepTime(t, p, 3, "Z") }, Rewritten},
	}
	for _, tc := range cases {
		path := writeFile(t, content)
		var ctl editor.Controller
		d := open(t, &ctl, path, Options{})
		_ = ctl.Line(0) // the first lines are in view
		d.File().Line(0)
		tc.change(path)
		if got := d.Check(); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSave_RefusesAFileChangedOnDisk(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(path string)
		other  bool // SaveAs to another path still works
	}{
		{"appended", func(p string) { appendTo(t, p, "theirs\n") }, true},
		{"replaced", func(p string) { replace(t, p, "theirs\n") }, true},
		{"rewritten", func(p string) { rewrite(t, p, 0, "THEIRS") }, false},
	} {
		path := writeFile(t, "mine\nand more\n")
		var ctl editor.Controller
		d := open(t, &ctl, path, Options{})
		ctl.Replace(editor.TextRange{}, "edited ")
		tc.change(path)
		disk := readFile(t, path)
		if err := d.Save(); err != ErrChangedOnDisk {
			t.Fatalf("%s: Save over a changed file: %v", tc.name, err)
		}
		if readFile(t, path) != disk {
			t.Errorf("%s: the refused Save wrote", tc.name)
		}
		other := filepath.Join(filepath.Dir(path), "other.txt")
		err := d.SaveAs(other)
		switch {
		case tc.name == "replaced" && !holdOpen:
			// Windows: the file that was replaced cannot be read again, and
			// so neither can the lines the text did not edit.
			if err == nil {
				t.Errorf("%s: SaveAs saved lines it cannot read", tc.name)
			}
		case tc.other && err != nil:
			t.Errorf("%s: SaveAs to another path: %v", tc.name, err)
		case tc.other && readFile(t, other) != "edited mine\nand more\n" && tc.name == "appended":
			t.Errorf("%s: saved %q", tc.name, readFile(t, other))
		case !tc.other && err != ErrChangedOnDisk:
			t.Errorf("%s: SaveAs of a rewritten file: %v", tc.name, err)
		}
	}
}

func TestOnChange_IsToldOncePerChange(t *testing.T) {
	path := writeFile(t, "a\nb\n")
	var ctl editor.Controller
	var told []Change
	d := open(t, &ctl, path, Options{OnChange: func(c Change) { told = append(told, c) }})
	appendTo(t, path, "c\n")
	d.Check()
	d.Check()
	rewrite(t, path, 0, "X")
	d.Check()
	d.Check()
	if fmt.Sprint(told) != "[appended rewritten]" {
		t.Errorf("told %v", told)
	}
	if err := d.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := d.Check(); got != Unchanged || ctl.Text() != "X\nb\nc\n" {
		t.Errorf("after Reload: %v, %q", got, ctl.Text())
	}
}

func TestFollow_TakesInAppendedLines(t *testing.T) {
	for _, tc := range []struct{ content, more string }{
		{"a\nb\n", "c\nd\n"},
		{"a\nb", "c\nd"}, // the last line goes on
		{"", "first\n"},
		{"x\r\n", "y\r\nz"},
		{numbered(500), numbered(300)},
	} {
		path := writeFile(t, tc.content)
		var ctl editor.Controller
		d := open(t, &ctl, path, Options{Follow: true})
		appendTo(t, path, tc.more)
		if got := d.Check(); got != Appended {
			t.Fatalf("Check = %v", got)
		}
		want := normalized(tc.content + tc.more)
		if ctl.Text() != want {
			t.Errorf("%q + %q: %q", tc.content, tc.more, clip(ctl.Text()))
		}
		checkSource(t, d.File(), want)
	}
}

func TestFollow_KeepsEditsAndGrowsWhileLoading(t *testing.T) {
	smallSteps(t)
	path := writeFile(t, numbered(100))
	u := newUI()
	var ctl editor.Controller
	d := open(t, &ctl, path, Options{Post: u.post, Follow: true})
	u.pump(t, "the index", func() bool { return !d.Loading() })
	ctl.Replace(editor.TextRange{}, "mine ")
	want := "mine " + numbered(100)
	for step := range 5 {
		more := numbered(200)
		appendTo(t, path, more)
		want += more
		d.Check()
		u.pump(t, "the lines appended", func() bool { return !d.Loading() })
		if got := ctl.Text(); got != want {
			t.Fatalf("step %d: %q, want %q", step, clip(got), clip(want))
		}
	}
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != want {
		t.Errorf("saved %q", clip(got))
	}
}

func TestWatch_WakesOnlyWhenTheFileChanged(t *testing.T) {
	period := WatchPeriod
	WatchPeriod = 20 * time.Millisecond
	t.Cleanup(func() { WatchPeriod = period })
	path := writeFile(t, "log\n")
	u := newUI()
	var ctl editor.Controller
	var told []Change
	d := open(t, &ctl, path, Options{Post: u.post, Watch: true, Follow: true, OnChange: func(c Change) { told = append(told, c) }})
	ctl.Line(0)
	// Nothing changes: nothing is posted.
	time.Sleep(150 * time.Millisecond)
	if len(u) != 0 {
		t.Fatalf("%d wakes of a file that did not change", len(u))
	}
	appendTo(t, path, "more\n")
	u.pump(t, "the append", func() bool { return ctl.Text() == "log\nmore\n" && !d.Loading() })
	// The bytes in view change, the size and the time do not.
	keepTime(t, path, 0, "LOG")
	u.pump(t, "the change in view", func() bool { return len(told) > 0 && told[len(told)-1] == Rewritten })
	u.drain(100 * time.Millisecond)
	if n := strings.Count(fmt.Sprint(told), "rewritten"); n != 1 {
		t.Errorf("told %v", told)
	}
}

func TestOpenFile_LetsOtherProgramsReplaceAndDeleteIt(t *testing.T) {
	path := writeFile(t, numbered(100))
	var ctl editor.Controller
	d := open(t, &ctl, path, Options{})
	ctl.Line(50)
	replace(t, path, "theirs\n")
	if got := d.Check(); got != Replaced {
		t.Errorf("a replaced file: %v", got)
	}
	if err := d.Reload(); err != nil || ctl.Text() != "theirs\n" {
		t.Fatalf("reload: %v, %q", err, ctl.Text())
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("an open file must be deletable: %v", err)
	}
	if got := d.Check(); got != Replaced {
		t.Errorf("a deleted file: %v", got)
	}
}

func TestReader_ReadsOnAnotherGoroutine(t *testing.T) {
	content := numbered(1000)
	var ctl editor.Controller
	d := open(t, &ctl, writeFile(t, content), Options{})
	r := d.File().Reader()
	lines := strings.Split(content, "\n")
	errc := make(chan error, 1)
	go func() {
		defer r.Close()
		var got []string
		err := r.ReadLines(130, 900, func(i int, l string) bool {
			if i != 130+len(got) {
				errc <- fmt.Errorf("line %d after %d lines", i, len(got))
				return false
			}
			got = append(got, l)
			return true
		})
		if err == nil && strings.Join(got, "\n") != strings.Join(lines[130:900], "\n") {
			err = fmt.Errorf("read %d lines that differ", len(got))
		}
		errc <- err
	}()
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

func TestReloadKeepingEdits_PlacesEditsInTheChangedFile(t *testing.T) {
	path := writeFile(t, numbered(1000))
	var ctl editor.Controller
	d := open(t, &ctl, path, Options{})
	at := func(l int) editor.TextRange {
		p := editor.TextPosition{Line: l}
		return editor.TextRange{Start: p, End: p}
	}
	ctl.Replace(at(100), "mine 1 ")
	ctl.Replace(at(800), "mine 2\n")
	// Another program puts 50 lines at the top and changes line 500.
	lines := strings.Split(numbered(1000), "\n")
	lines[500] = "theirs"
	replace(t, path, numbered(50)+strings.Join(lines, "\n"))
	conflicts, err := d.ReloadKeepingEdits(false)
	if err != nil || len(conflicts) != 0 {
		t.Fatalf("ReloadKeepingEdits = %+v, %v", conflicts, err)
	}
	got := strings.Split(ctl.Text(), "\n")
	if got[150] != "mine 1 line 100 of the file" || got[850] != "mine 2" || got[851] != "line 800 of the file" || got[550] != "theirs" {
		t.Errorf("after the reload: %q %q %q %q", got[150], got[850], got[851], got[550])
	}
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	if readFile(t, path) != ctl.Text() {
		t.Errorf("the saved file differs from the text")
	}
}

func TestReloadKeepingEdits_ReportsEditsItCannotPlace(t *testing.T) {
	path := writeFile(t, "a\nb\nc\nd\ne\nf\ng\n")
	var ctl editor.Controller
	d := open(t, &ctl, path, Options{})
	ctl.Replace(editor.TextRange{Start: editor.TextPosition{Line: 1}, End: editor.TextPosition{Line: 1, Column: 1}}, "B")
	ctl.Replace(editor.TextRange{Start: editor.TextPosition{Line: 5}, End: editor.TextPosition{Line: 5, Column: 1}}, "F")
	before := ctl.Text()
	rewrite(t, path, 2, "X") // b, which the first edit replaced, is gone
	conflicts, err := d.ReloadKeepingEdits(false)
	if err != nil || len(conflicts) != 1 || conflicts[0].Line != 1 || fmt.Sprint(conflicts[0].Removed) != "[b]" {
		t.Fatalf("conflicts %+v, %v", conflicts, err)
	}
	if ctl.Text() != before {
		t.Fatalf("without partial the text must stay: %q", ctl.Text())
	}
	conflicts, err = d.ReloadKeepingEdits(true)
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("partial: %+v, %v", conflicts, err)
	}
	if ctl.Text() != "a\nX\nc\nd\ne\nF\ng\n" {
		t.Errorf("partial: %q", ctl.Text())
	}
}

func TestSave_KeepsTheUndoHistoryAndTheSelection(t *testing.T) {
	path := writeFile(t, numbered(500))
	var ctl editor.Controller
	d := open(t, &ctl, path, Options{})
	p := editor.TextPosition{Line: 300, Column: 4}
	ctl.Replace(editor.TextRange{Start: p, End: p}, "mine")
	sel := editor.TextRange{Start: editor.TextPosition{Line: 300}, End: editor.TextPosition{Line: 300, Column: 8}}
	ctl.SetSelection(sel)
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	if !ctl.CanUndo() || ctl.Selection() != sel || len(ctl.Edits()) != 0 {
		t.Fatalf("after Save: undo %v, selection %+v, %d edits", ctl.CanUndo(), ctl.Selection(), len(ctl.Edits()))
	}
	ctl.Undo()
	if ctl.Text() != numbered(500) {
		t.Fatal("undo after Save should give the text back")
	}
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	if readFile(t, path) != numbered(500) {
		t.Error("the text undone should save")
	}
}

func TestReloadKeepingEdits_MovesTheCaretWithTheEdits(t *testing.T) {
	path := writeFile(t, numbered(200))
	var ctl editor.Controller
	d := open(t, &ctl, path, Options{})
	p := editor.TextPosition{Line: 100, Column: 3}
	ctl.Replace(editor.TextRange{Start: p, End: p}, "X")
	at := editor.TextPosition{Line: 100, Column: 4}
	ctl.SetSelection(editor.TextRange{Start: at, End: at})
	replace(t, path, numbered(10)+numbered(200))
	if c, err := d.ReloadKeepingEdits(false); err != nil || len(c) != 0 {
		t.Fatalf("%v, %v", c, err)
	}
	want := editor.TextPosition{Line: 110, Column: 4}
	if got := ctl.Selection(); got.Start != want || got.End != want {
		t.Errorf("the caret is at %+v, want %+v", got, want)
	}
}

func TestReader_WritesLinesAsTheFileDoes(t *testing.T) {
	// Mixed breaks, a BOM and a lone \r, over several groups of lines, and a
	// last line without a break.
	var b strings.Builder
	b.WriteString(bomText)
	for i := range 300 {
		nl := "\n"
		if i%3 == 0 {
			nl = "\r\n"
		}
		fmt.Fprintf(&b, "line %d\rof the file%s", i, nl)
	}
	b.WriteString("the last line")
	var ctl editor.Controller
	d := open(t, &ctl, writeFile(t, b.String()), Options{})
	f := d.File()
	lw, ok := f.Reader().(editor.LineWriter)
	if !ok {
		t.Fatal("the reader of a File should be an editor.LineWriter")
	}
	defer lw.(editor.LineReader).Close()
	n := f.Len()
	for _, r := range [][2]int{{0, 1}, {0, n}, {1, 2}, {63, 65}, {64, 128}, {100, n}, {n - 1, n}, {5, 5}, {200, 299}} {
		var want, got strings.Builder
		if err := f.WriteLines(&want, r[0], r[1]); err != nil {
			t.Fatal(err)
		}
		// On another goroutine, as a snapshot writes.
		errc := make(chan error, 1)
		go func() { errc <- lw.WriteLines(&got, r[0], r[1]) }()
		if err := <-errc; err != nil {
			t.Fatal(err)
		}
		if got.String() != want.String() {
			t.Errorf("lines %d to %d: the reader wrote %q, the file %q", r[0], r[1], clip(got.String()), clip(want.String()))
		}
	}
}

// holdSaves keeps each save of StartSaveAs from writing until the test
// closes the channel it returns.
func holdSaves(t *testing.T) chan struct{} {
	gate := make(chan struct{})
	HoldSave = func() { <-gate }
	t.Cleanup(func() {
		HoldSave = nil
		select {
		case <-gate:
		default:
			close(gate)
		}
	})
	return gate
}

// startSave begins a save of d to its path and returns a function that
// reports whether it ended and with which error.
func startSave(t *testing.T, d *Doc, progress func(written, total int64)) func() (bool, error) {
	t.Helper()
	ended, saveErr := false, error(nil)
	if err := d.StartSaveAs(d.Path(), progress, func(err error) { ended, saveErr = true, err }); err != nil {
		t.Fatal(err)
	}
	return func() (bool, error) { return ended, saveErr }
}

func TestStartSave_WritesOnAnotherGoroutine(t *testing.T) {
	content := bomText + "first\r\nsecond\r\n" + numbered(3000)
	path := writeFile(t, content)
	u := newUI()
	var ctl editor.Controller
	d := open(t, &ctl, path, Options{Post: u.post})
	u.pump(t, "the end of the index", func() bool { return !d.Loading() })
	ctl.Replace(editor.TextRange{}, "start ")
	gate := holdSaves(t)
	result := startSave(t, d, nil)

	// While it runs, the doc refuses what would change the text it writes,
	// and the file is as it was.
	if !d.Saving() {
		t.Fatal("Saving should report the save")
	}
	if err := d.Save(); err != ErrSaving {
		t.Errorf("Save while saving: %v", err)
	}
	if err := d.StartSaveAs(path, nil, nil); err != ErrSaving {
		t.Errorf("a second StartSaveAs: %v", err)
	}
	if err := d.Reload(); err != ErrSaving {
		t.Errorf("Reload while saving: %v", err)
	}
	if got := readFile(t, path); got != content {
		t.Errorf("the file changed before the save wrote it")
	}
	// The UI goroutine reads the text meanwhile.
	if got := ctl.Line(0); got != bomText+"start first" && got != "start first" {
		t.Errorf("line 0 while saving = %q", got)
	}

	close(gate)
	u.pump(t, "the end of the save", func() bool { ended, _ := result(); return ended })
	if _, err := result(); err != nil {
		t.Fatal(err)
	}
	want := bomText + "start first\r\nsecond\r\n" + numbered(3000)
	if got := readFile(t, path); got != want {
		t.Fatalf("saved %q, want %q", clip(got), clip(want))
	}
	checkSource(t, d.File(), normalized(want))
	if d.Saving() || !ctl.CanUndo() || ctl.Source() != d.File() {
		t.Errorf("the save should end with the controller reading the new file, its history kept")
	}
	if d.Check() != Unchanged {
		t.Errorf("the file saved looks changed")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("the folder holds %d files, want only the document", len(entries))
	}
}

func TestStartSave_TellsHowFarItIs(t *testing.T) {
	every := progressEvery
	progressEvery = 0
	t.Cleanup(func() { progressEvery = every })
	content := numbered(200000) // about 4 MB, many writes
	path := writeFile(t, content)
	u := newUI()
	var ctl editor.Controller
	d := open(t, &ctl, path, Options{Post: u.post})
	u.pump(t, "the end of the index", func() bool { return !d.Loading() })
	ctl.Replace(editor.TextRange{}, "x")
	var steps []int64
	var total int64
	result := startSave(t, d, func(written, all int64) {
		steps = append(steps, written)
		total = all
		// The window reads the lines meanwhile, through the cache of the File.
		_ = ctl.Line(int(written/7) % ctl.LineCount())
		_ = d.File().Line(int(written/11) % d.File().Len())
	})
	u.pump(t, "the end of the save", func() bool { ended, _ := result(); return ended })
	if _, err := result(); err != nil {
		t.Fatal(err)
	}
	if len(steps) < 2 {
		t.Fatalf("progress told %d times", len(steps))
	}
	for i := 1; i < len(steps); i++ {
		if steps[i] < steps[i-1] {
			t.Fatalf("progress went back: %v", steps)
		}
	}
	if size := int64(len(content) + 1); total != size || steps[len(steps)-1] > total {
		t.Errorf("total %d, last %d, want a total of %d", total, steps[len(steps)-1], size)
	}
}

func TestStartSave_LeavesAFileChangedMeanwhile(t *testing.T) {
	content := numbered(500)
	path := writeFile(t, content)
	u := newUI()
	var ctl editor.Controller
	var told []Change
	d := open(t, &ctl, path, Options{Post: u.post, OnChange: func(c Change) { told = append(told, c) }})
	ctl.Replace(editor.TextRange{}, "mine ")
	gate := holdSaves(t)
	result := startSave(t, d, nil)
	appendTo(t, path, "another program\n")
	if c := d.Check(); c != Unchanged || len(told) > 0 {
		t.Errorf("a doc that saves should not look at its file: %v %v", c, told)
	}
	close(gate)
	u.pump(t, "the end of the save", func() bool { ended, _ := result(); return ended })
	if _, err := result(); err != ErrChangedOnDisk {
		t.Errorf("the save ended with %v, want ErrChangedOnDisk", err)
	}
	if got := readFile(t, path); got != content+"another program\n" {
		t.Errorf("the save wrote over the other program")
	}
	if len(told) == 0 {
		t.Errorf("after the save, the change should be told")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("the folder holds %d files, want only the document", len(entries))
	}
}

func TestStartSave_RefusesATextChangedMeanwhile(t *testing.T) {
	content := numbered(500)
	path := writeFile(t, content)
	u := newUI()
	var ctl editor.Controller
	d := open(t, &ctl, path, Options{Post: u.post})
	ctl.Replace(editor.TextRange{}, "first ")
	gate := holdSaves(t)
	result := startSave(t, d, nil)
	ctl.Replace(editor.TextRange{}, "second ")
	close(gate)
	u.pump(t, "the end of the save", func() bool { ended, _ := result(); return ended })
	if _, err := result(); err != ErrChangedWhileSaving {
		t.Errorf("the save ended with %v, want ErrChangedWhileSaving", err)
	}
	if got := readFile(t, path); got != content {
		t.Errorf("a save whose text changed should leave the file")
	}
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); !strings.HasPrefix(got, "second first line 0") {
		t.Errorf("saved again: %q", clip(got))
	}
}

func TestStartSave_LandsAfterClose(t *testing.T) {
	path := writeFile(t, numbered(500))
	u := newUI()
	var ctl editor.Controller
	d, err := Open(&ctl, path, Options{Post: u.post})
	if err != nil {
		t.Fatal(err)
	}
	ctl.Replace(editor.TextRange{}, "kept ")
	gate := holdSaves(t)
	result := startSave(t, d, nil)
	d.Close()
	close(gate)
	u.pump(t, "the end of the save", func() bool { ended, _ := result(); return ended })
	if _, err := result(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != "kept "+numbered(500) {
		t.Errorf("a save that began should land after Close: %q", clip(got))
	}
}

func TestStartSave_WithoutPostSavesAtOnce(t *testing.T) {
	path := writeFile(t, "a\r\nb")
	var ctl editor.Controller
	d := open(t, &ctl, path, Options{})
	ctl.Replace(editor.TextRange{}, "z")
	result := startSave(t, d, nil)
	if ended, err := result(); !ended || err != nil {
		t.Fatalf("ended %v, %v", ended, err)
	}
	if got := readFile(t, path); got != "za\r\nb" {
		t.Errorf("saved %q", got)
	}
}
