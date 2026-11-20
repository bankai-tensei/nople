package app

import (
	"errors"
	"os"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/bankai-tensei/nople/internal/bigfile"
	"github.com/bankai-tensei/nople/internal/textfile"
)

// A text file above textfile.LargeSize opens through bigfile, without being
// read all: the editor reads the lines it shows, the rest of the file is
// indexed on a goroutine of its own, and the file is watched for changes and
// followed as it grows, as a log is. Its colors come by the blocks of lines in
// view (see bighl.go); it shows no git marks or outline, as those read the
// whole text.

// headSize is how much of a large file decides how it opens.
const headSize = 64 << 10

// bigContent tells whether the file at path opens through bigfile, and what
// is known of it before: its size, and its encoding from its start. A binary
// file opens as one, without its text; a file whose start is not UTF-8 opens
// as before while it is small enough to read, as it may be Shift_JIS, and read
// only beyond.
func bigContent(path string) (textfile.Content, bool) {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() || st.Size() <= textfile.LargeSize {
		return textfile.Content{}, false
	}
	c := textfile.Content{Size: st.Size(), Large: true}
	f, err := os.Open(path)
	if err != nil {
		return textfile.Content{}, false
	}
	defer f.Close() //nolint:errcheck // only read
	head := make([]byte, headSize)
	n, _ := f.Read(head)
	head = head[:n]
	switch {
	case textfile.IsBinary(head):
		c.Binary = true
		return c, true
	case len(head) >= 3 && head[0] == 0xEF && head[1] == 0xBB && head[2] == 0xBF:
		c.Encoding = textfile.UTF8BOM
	case validStart(head):
		c.Encoding = textfile.UTF8
	case st.Size() <= textfile.MaxOpenSize:
		return textfile.Content{}, false
	default:
		c.Encoding, c.Lossy = textfile.UTF8, true
	}
	return c, true
}

// validStart reports whether b is UTF-8 but for a character cut at its end.
func validStart(b []byte) bool {
	for k := 0; k < utf8.UTFMax && len(b) > 0; k++ {
		if utf8.Valid(b) {
			return true
		}
		b = b[:len(b)-1]
	}
	return utf8.Valid(b)
}

// openBig shows the file of d through bigfile.
func (a *app) openBig(d *document) {
	doc, err := bigfile.Open(&d.ctrl, d.path, bigfile.Options{
		Post: func(fn func()) {
			a.shell.post(func() {
				fn()
				a.refresh()
			})
		},
		OnChange: func(c bigfile.Change) { a.bigChanged(d, c) },
		Watch:    true,
		Follow:   true,
	})
	if err != nil {
		d.content.Err = err
		return
	}
	d.big = doc
	d.content.CRLF = doc.File().CRLF()
	a.startBigHighlight(d)
}

// bigChanged takes in a change on disk of the file of d, which bigfile
// noticed. What was appended is already in the text. A file changed otherwise
// is read again, with the edits placed in it; when they cannot all be placed,
// the text stays and the user decides.
func (a *app) bigChanged(d *document, c bigfile.Change) {
	switch c {
	case bigfile.Unchanged, bigfile.Appended:
		d.gone = false
		return
	case bigfile.Replaced:
		if _, err := os.Stat(d.path); err != nil {
			d.gone = true
			for _, id := range a.tabsOf(d) {
				a.tabs[id].noticeClosed = false
			}
			return
		}
	}
	d.gone = false
	defer a.resetBigHighlight(d)
	if !d.dirty {
		a.keepPlaces(d, func() {
			if err := d.big.Reload(); err != nil {
				d.saveErr = err
			}
		})
		return
	}
	// The selections and the scroll move with the edits placed.
	conflicts, err := d.big.ReloadKeepingEdits(false)
	if err != nil || len(conflicts) > 0 {
		d.conflict = true
	}
}

// saveShowAfter is how long a save of a large file runs before the status bar
// and the tab show how far it is.
var saveShowAfter = 300 * time.Millisecond

// saving reports whether a save of the large file of d runs.
func (d *document) saving() bool { return d.big != nil && d.big.Saving() }

// saveProgress is how far the save of d is, in percent, and whether to show
// it: while it runs, once it ran for saveShowAfter.
func (d *document) saveProgress() (int, bool) {
	if !d.saving() || time.Since(d.saveStart) < saveShowAfter {
		return 0, false
	}
	if d.saveTotal <= 0 {
		return 0, true
	}
	return int(min(d.saveWritten*100/d.saveTotal, 100)), true
}

// saveBig saves the text of d through bigfile, on a goroutine of its own, so
// that the window goes on meanwhile: the text can be read, scrolled and
// searched, but not edited, and the file is not watched. then, when not nil,
// runs at the end with its error. A save that runs is not begun again: then
// waits for it. An error that keeps the save from starting is returned, and
// then is not called. The editor keeps its undo history, selections and
// scroll.
func (a *app) saveBig(d *document, then func(error)) error {
	if d.saving() {
		if then != nil {
			d.afterSave = append(d.afterSave, then)
		}
		return nil
	}
	err := d.big.StartSaveAs(d.path, func(written, total int64) {
		d.saveWritten, d.saveTotal = written, total
	}, func(err error) {
		a.bigSaved(d, err)
	})
	if err != nil {
		return bigSaveError(d, err)
	}
	d.saveStart, d.saveWritten, d.saveTotal = time.Now(), 0, 0
	if then != nil {
		d.afterSave = append(d.afterSave, then)
	}
	// How far it is shows once it ran long enough; the progress reports
	// redraw it after.
	a.later(saveShowAfter, a.refresh)
	a.refresh()
	return nil
}

// bigSaved ends the save of the large file of d, which bigfile ended with err,
// and runs what waits for it.
func (a *app) bigSaved(d *document, err error) {
	a.closing = slices.DeleteFunc(a.closing, func(c *document) bool { return c == d })
	if err = bigSaveError(d, err); err != nil {
		d.saveErr = err
	} else {
		d.saveErr, d.conflict, d.gone, d.dirty = nil, false, false, false
		d.content.Size = d.big.File().Size()
	}
	waiting := d.afterSave
	d.afterSave = nil
	for _, fn := range waiting {
		fn(err)
	}
	a.refresh()
}

// bigSaveError says err of a save through bigfile for the user.
func bigSaveError(d *document, err error) error {
	switch {
	case errors.Is(err, bigfile.ErrLoading):
		return errors.New(T("save.loading"))
	case errors.Is(err, bigfile.ErrChangedOnDisk):
		d.conflict = true
		return errors.New(T("save.changedOnDisk"))
	case errors.Is(err, bigfile.ErrChangedWhileSaving):
		return errors.New(T("save.changedWhileSaving"))
	}
	return err
}

// afterSaved runs fn once d is not saving: now, or when its save ends.
func (a *app) afterSaved(d *document, fn func()) {
	if !d.saving() {
		fn()
		return
	}
	d.afterSave = append(d.afterSave, func(error) { fn() })
}

// whenSaved runs fn once no document is saving.
func (a *app) whenSaved(fn func()) {
	for _, d := range a.savingDocs() {
		a.afterSaved(d, func() { a.whenSaved(fn) })
		return
	}
	fn()
}

// savingDocs are the documents whose save runs, those of tabs first.
func (a *app) savingDocs() []*document {
	var ds []*document
	seen := map[*document]bool{}
	for _, id := range a.order {
		if d := a.tabs[id].doc; d.saving() && !seen[d] {
			seen[d] = true
			ds = append(ds, d)
		}
	}
	for _, d := range a.closing {
		if d.saving() && !seen[d] {
			seen[d] = true
			ds = append(ds, d)
		}
	}
	return ds
}

// revertBig reads the file of d again, dropping its changes.
func (a *app) revertBig(d *document) {
	if d.saving() {
		return
	}
	a.keepPlaces(d, func() {
		if err := d.big.Reload(); err != nil {
			d.saveErr = err
		}
	})
	a.resetBigHighlight(d)
}

// keepPlaces runs fn, which gives the editor of d another text with
// SetSource, and puts the selection and the scroll of each of its tabs back.
func (a *app) keepPlaces(d *document, fn func()) {
	type place struct {
		t      *tab
		scroll editorScroll
	}
	ids := a.tabsOf(d)
	places := make([]place, 0, len(ids))
	for _, id := range ids {
		t := a.tabs[id]
		places = append(places, place{t, editorScroll{t.view.Selection(), t.view.Scroll()}})
	}
	fn()
	for _, p := range places {
		p.t.view.SetSelection(p.scroll.sel)
		p.t.view.SetScroll(p.scroll.pos)
	}
}

// loadProgress is how much of the file of d is indexed, from 0 to 1, and
// whether it is still being indexed.
func (d *document) loadProgress() (float64, bool) {
	if d.big == nil || !d.big.Loading() {
		return 1, false
	}
	f := d.big.File()
	if f.Size() == 0 {
		return 1, true
	}
	return float64(f.Indexed()) / float64(f.Size()), true
}
