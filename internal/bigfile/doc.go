package bigfile

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/bankai-tensei/nople/internal/safewrite"

	"github.com/bankai-tensei/gowaku/widgets/editor"
)

// Errors of Doc.
var (
	// ErrLoading is returned while the file is still being indexed.
	ErrLoading = errors.New("bigfile: the file is still loading")
	// ErrChangedOnDisk is returned by Save and SaveAs when the file changed
	// on disk since it was read. See Change.
	ErrChangedOnDisk = errors.New("bigfile: the file changed on disk")
	// ErrSaving is returned while a save that StartSaveAs began runs.
	ErrSaving = errors.New("bigfile: the file is being saved")
	// ErrChangedWhileSaving ends a save of StartSaveAs whose text changed
	// before it was written whole: the file stays as it was.
	ErrChangedWhileSaving = errors.New("bigfile: the text changed while it was saved")
)

// Options are the options of Open.
type Options struct {
	// Post runs a function on the UI goroutine, as gowaku.Post or the Poster
	// of a State does. Without it the file is indexed before Open returns,
	// and is not watched.
	Post func(func())
	// OnLoad, when not nil, runs on the UI goroutine after each step of
	// indexing the file, with how far it is indexed and its size, the last
	// time with both equal; again so when Follow indexes lines appended.
	OnLoad func(loaded, size int64)
	// OnChange, when not nil, runs on the UI goroutine when the file changes
	// on disk, with how it changed, once per change until it changes again.
	// An append that Follow takes in is told too.
	OnChange func(Change)
	// Watch looks at the file once a WatchPeriod, on a goroutine of its own,
	// and checks it on the UI goroutine when it changed. Without it the file
	// is checked by Check, which the application calls when it learns of a
	// change some other way, and by Save.
	Watch bool
	// Follow takes in the lines a program appends to the file as they come,
	// as tail -f shows a log: the text grows at its end, also while it shows
	// edits, and a search goes on into the new lines.
	Follow bool
}

// WatchPeriod is how often a file opened with Options.Watch is looked at. Each
// look is one os.Stat; the UI wakes only when the file changed.
var WatchPeriod = time.Second

// Doc is a file shown in an editor.Controller through a File. Its methods run
// on the UI goroutine.
type Doc struct {
	ctrl *editor.Controller
	f    *File
	opts Options
	// told is the change OnChange told last, loadErr the error that ended
	// the indexing.
	told    Change
	loadErr error
	// saving is the save StartSaveAs began, while it runs, and closed
	// tells that Close ran.
	saving *saveJob
	closed bool
}

// Open shows the file at path in ctrl, without reading it all: the start of
// the file is indexed before Open returns, and the rest on a goroutine of its
// own while the editor shows the start, the text growing at its end as lines
// are found. The undo history of ctrl is forgotten and the caret goes to the
// start.
func Open(ctrl *editor.Controller, path string, opts Options) (*Doc, error) {
	d := &Doc{ctrl: ctrl, opts: opts}
	s, err := d.load(path, opts.Post == nil)
	if err != nil {
		return nil, err
	}
	d.show(s)
	return d, nil
}

// load opens the file at path and indexes it, all of it when whole, or the
// rest on a goroutine.
func (d *Doc) load(path string, whole bool) (*File, error) {
	s, x, at, err := openFile(path)
	if err != nil {
		return nil, err
	}
	if x == nil {
		return s, nil
	}
	if whole {
		s.index(s.opener(), x, -1, at, s.size, indexChunk, 1<<62, 0, func() {})
		for _, b := range s.take() {
			s.apply(b)
			if b.err != nil {
				_ = s.Close() //nolint:errcheck // the error of the read is the one to report
				return nil, b.err
			}
		}
		return s, nil
	}
	go s.index(s.opener(), x, -1, at, s.size, indexChunk, indexReport, 0, d.waker(s))
	return s, nil
}

// show makes s the file of the doc, in place of the one before, and watches
// it.
func (d *Doc) show(s *File) {
	if d.f != nil && d.f != s {
		_ = d.f.Close() //nolint:errcheck // a file only read, which s replaces
	}
	d.f, d.told, d.loadErr = s, Unchanged, nil
	d.ctrl.SetSource(s)
	d.watch()
	if s.done && d.opts.OnLoad != nil {
		d.opts.OnLoad(s.size, s.size)
	}
}

// watch starts looking at the file, when the options ask for it.
func (d *Doc) watch() {
	if !d.opts.Watch || d.opts.Post == nil {
		return
	}
	s := d.f
	go s.watch(WatchPeriod, func() {
		d.opts.Post(func() {
			if d.f == s {
				d.Check()
			}
		})
	})
}

// waker returns the function the indexing goroutine of s calls after each
// report, which takes the report on the UI goroutine.
func (d *Doc) waker(s *File) func() {
	return func() {
		d.opts.Post(func() {
			if d.f == s {
				d.applyLoad()
			}
		})
	}
}

// applyLoad grows the text by the lines the indexing found.
func (d *Doc) applyLoad() {
	s := d.f
	batches := s.take()
	if len(batches) == 0 {
		return
	}
	from := -1
	for _, b := range batches {
		before := s.apply(b)
		if from < 0 || before < from {
			from = before
		}
		if b.err != nil && d.loadErr == nil {
			d.loadErr = b.err
			s.done = true // nothing more comes
		}
	}
	d.ctrl.SourceGrew(from)
	if d.opts.OnLoad != nil {
		d.opts.OnLoad(s.rawEnd, s.size)
	}
}

// File returns the file the doc shows.
func (d *Doc) File() *File { return d.f }

// Path returns the path of the file the doc shows.
func (d *Doc) Path() string { return d.f.path }

// Loading reports whether the file is still being indexed.
func (d *Doc) Loading() bool { return !d.f.done }

// LoadErr returns the error that ended indexing the file, or nil.
func (d *Doc) LoadErr() error { return d.loadErr }

// Close stops looking at the file and lets it go. The controller keeps the
// file as its source until it is given another text.
func (d *Doc) Close() error {
	d.closed = true
	return d.f.Close()
}

// Check looks at the file on disk and tells how it changed since it was read:
// through Options.OnChange, once per change, and its result. With
// Options.Follow, lines appended are then indexed and join the text. A file
// still loading reports Unchanged.
func (d *Doc) Check() Change {
	s := d.f
	if !s.done || d.saving != nil {
		return Unchanged
	}
	change := s.probe()
	if change == Appended && d.opts.Follow {
		if d.follow() {
			return change
		}
		change = Rewritten
	}
	if change != Unchanged && change != d.told {
		d.told = change
		if d.opts.OnChange != nil {
			d.opts.OnChange(change)
		}
	}
	return change
}

// follow indexes the lines appended to the file. It reports false when the
// file cannot be read.
func (d *Doc) follow() bool {
	s := d.f
	st, err := os.Stat(s.path)
	if err != nil {
		return false
	}
	x, last, at, err := s.reopen(st)
	if err != nil {
		return false
	}
	// The lines stay as they are until the index has found at least as many
	// again, so that the text never shrinks.
	least := s.n
	if d.opts.Post == nil {
		s.index(s.opener(), x, last, at, s.size, indexChunk, 1<<62, least, func() {})
		d.applyLoad()
	} else {
		go s.index(s.opener(), x, last, at, s.size, indexChunk, indexReport, least, d.waker(s))
	}
	d.told = Unchanged
	if d.opts.OnChange != nil {
		d.opts.OnChange(Appended)
	}
	return true
}

// Reload shows the file again as it is now on disk: edits and the undo
// history go, and the caret goes to the start.
func (d *Doc) Reload() error {
	if d.saving != nil {
		return ErrSaving
	}
	s, err := d.load(d.f.path, d.opts.Post == nil)
	if err != nil {
		return err
	}
	d.show(s)
	return nil
}

// Save writes the text to its file, as SaveAs does to its path.
func (d *Doc) Save() error { return d.SaveAs(d.f.path) }

// Saving reports whether a save that StartSaveAs began is still running.
func (d *Doc) Saving() bool { return d.saving != nil }

// progressEvery is how often a save running on its own goroutine tells how far
// it is.
var progressEvery = 100 * time.Millisecond

// HoldSave runs, when not nil, on the goroutine of a save that StartSaveAs
// began, before it writes. Tests set it to keep a save running while they
// look at the window.
var HoldSave func()

// StartSaveAs writes the text to the file at path as SaveAs does, but on a
// goroutine of its own, so that the window goes on while a file of gigabytes
// is written. The UI goroutine takes a snapshot of the text, which is quick;
// the goroutine writes it to the temporary file and syncs it; back on the UI
// goroutine the file takes the place of the old one and the controller reads
// it through ReplaceSource, as with SaveAs.
//
// The save cannot start for the reasons SaveAs gives, and returns
// ErrSaving while another save runs; it then returns the error and calls
// nothing. Once started, progress runs on the UI goroutine now and then with
// the bytes written and about how many there are, and done runs once there at
// the end, with the error of the save or nil. Without Options.Post the save
// runs before StartSaveAs returns, and done before it too.
//
// While the save runs, the doc does not look at the file (Check reports
// Unchanged) nor take in what is appended, and Reload refuses, so that the
// text stays the one written. The application keeps the user from editing
// meanwhile: a text that changed by the end is not saved, and done gets
// ErrChangedWhileSaving. A file changed on disk meanwhile is not written
// over: done gets ErrChangedOnDisk. After a save that failed, the file is
// checked at once for what happened meanwhile.
func (d *Doc) StartSaveAs(path string, progress func(written, total int64), done func(error)) error {
	j, err := d.prepare(path)
	if err != nil {
		return err
	}
	snap := d.ctrl.Snapshot()
	total := snap.Size(j.nl()) + j.start
	post := d.opts.Post
	if post == nil {
		err := j.write(snap.WriteTo, nil)
		_ = snap.Close() //nolint:errcheck // a reader only read
		if err == nil {
			err = j.tmp.Sync()
		}
		d.finishSave(j, snap, err, done)
		return nil
	}
	d.saving = j
	hold := HoldSave
	go func() {
		if hold != nil {
			hold()
		}
		var last time.Time
		err := j.write(snap.WriteTo, func(n int64) {
			if progress == nil || time.Since(last) < progressEvery {
				return
			}
			last = time.Now()
			post(func() {
				if d.saving == j {
					progress(n, total)
				}
			})
		})
		_ = snap.Close() //nolint:errcheck // a reader only read
		if err == nil {
			// The slow part of the commit, here rather than on the UI
			// goroutine; Commit syncs again, which then has nothing to do.
			err = j.tmp.Sync()
		}
		post(func() { d.finishSave(j, snap, err, done) })
	}()
	return nil
}

// finishSave ends the save j on the UI goroutine, after its goroutine wrote
// the text of snap with the error err.
func (d *Doc) finishSave(j *saveJob, snap *editor.Snapshot, err error, done func(error)) {
	d.saving = nil
	if err == nil && !d.closed && d.ctrl.Changed(snap) {
		err = ErrChangedWhileSaving
	}
	if err == nil {
		// The file was not watched while the save ran.
		err = j.check()
	}
	switch {
	case err != nil:
		safewrite.Abort(j.tmp)
		if !d.closed {
			d.Check()
		}
	case d.closed:
		// Closed while it saved: the save still lands, and nothing shows it.
		err = safewrite.Commit(j.tmp, j.path)
	default:
		err = d.commit(j)
	}
	if done != nil {
		done(err)
	}
}

// SaveAs writes the text to the file at path, which it creates or replaces,
// and the doc then shows that file. The lines the text did not edit are copied
// from the file as they are, breaks included; the lines in memory are written
// with the break of the first line of the file, \r\n or \n, and a BOM the file
// started with stays. The new file is written beside the old one, synced, and
// takes its place at the end, so that a failure leaves the old file whole. It
// is indexed as it is written, so that it is not read again.
//
// The controller then reads the new file, which holds the text, through
// ReplaceSource: its undo history, selections, scroll and search stay, and
// the text holds no edits against its file.
//
// SaveAs returns ErrLoading while the file is still being indexed. It returns
// ErrChangedOnDisk for the doc's own path when the file changed on disk since
// it was read, in any way, as writing would lose what another program wrote,
// and for any path when the file was rewritten in place, as the lines the
// text has not edited can no longer be read as they were.
func (d *Doc) SaveAs(path string) error {
	j, err := d.prepare(path)
	if err != nil {
		return err
	}
	if err := j.write(d.ctrl.WriteTo, nil); err != nil {
		safewrite.Abort(j.tmp)
		return err
	}
	return d.commit(j)
}

// saveJob is a save of the text to a temporary file that takes the place of
// the file at path at the end.
type saveJob struct {
	// path is the file written, past a symbolic link, and old the file the
	// doc showed when the save began.
	path string
	old  *File
	tmp  *os.File
	// x indexes the file as it is written, from start, past the BOM.
	x     *indexer
	start int64
}

// prepare checks that the text can be saved to path and makes the temporary
// file of the save.
func (d *Doc) prepare(path string) (*saveJob, error) {
	if d.saving != nil {
		return nil, ErrSaving
	}
	j := &saveJob{path: path, old: d.f}
	if !j.old.done {
		return nil, ErrLoading
	}
	if err := j.check(); err != nil {
		return nil, err
	}
	// A symbolic link stays, and the file it points to is replaced.
	j.path = safewrite.Target(path)
	tmp, err := safewrite.Temp(j.path, 0o644)
	if err != nil {
		return nil, err
	}
	j.tmp = tmp
	if j.old.hasBOM {
		j.start = int64(len(bom))
	}
	j.x = &indexer{pos: j.start, drop: j.start}
	return j, nil
}

// check returns ErrChangedOnDisk when the file the save began from changed
// on disk in a way that writing would lose: in any way for its own path, and
// when it was rewritten in place for any path.
func (j *saveJob) check() error {
	switch change := j.old.probe(); {
	case change == Rewritten:
		return ErrChangedOnDisk
	case change != Unchanged && samePath(j.path, j.old.path):
		return ErrChangedOnDisk
	}
	return nil
}

// nl is the break of the lines written from memory.
func (j *saveJob) nl() string {
	if j.old.crlf {
		return "\r\n"
	}
	return "\n"
}

// write writes the text to the temporary file through writeText, the BOM
// first when the file had one, indexing it as it goes. wrote, when not nil,
// is told the bytes written so far after each write.
func (j *saveJob) write(writeText func(w io.Writer, nl string) error, wrote func(int64)) error {
	bw := bufio.NewWriterSize(j.tmp, 1<<20)
	out := &indexingWriter{w: bw, x: j.x, wrote: wrote}
	if j.old.hasBOM {
		if _, err := bw.Write(bom); err != nil {
			return err
		}
		out.off = j.start
	}
	if err := writeText(out, j.nl()); err != nil {
		return err
	}
	return bw.Flush()
}

// commit makes the temporary file of j take the place of its file, and the
// doc show the new file, which holds the text.
func (d *Doc) commit(j *saveJob) error {
	old, path, start := j.old, j.path, j.start
	// Synced, then in place of the file in one rename: a save cut short
	// leaves the file as it was. Where the file is held open, it can be
	// renamed over; on Windows nothing holds it.
	if err := safewrite.Commit(j.tmp, path); err != nil {
		return err
	}
	x := j.x
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close() //nolint:errcheck // the error of Stat is the one to report
		return err
	}
	s := &File{path: path, size: st.Size(), raw: []int64{start}, norm: []int64{0}, crlf: old.crlf, hasBOM: old.hasBOM}
	s.apply(x.batch(true))
	if err := s.remember(st, f); err != nil {
		_ = f.Close() //nolint:errcheck // the error of the read is the one to report
		return err
	}
	s.keep(f)
	// The new file holds the text: the controller reads it in place of the
	// old one, keeping its undo history, selections and scroll.
	_ = old.Close() //nolint:errcheck // a file only read, which s replaces
	d.f, d.told, d.loadErr = s, Unchanged, nil
	if err := d.ctrl.ReplaceSource(s); err != nil {
		// Not the same text, which would be a fault of the writing: show
		// the file as it was written all the same.
		d.ctrl.SetSource(s)
	}
	d.watch()
	return nil
}

// indexingWriter feeds what it writes to an indexer, and tells wrote how far
// it is.
type indexingWriter struct {
	w     io.Writer
	x     *indexer
	off   int64
	wrote func(int64)
}

func (w *indexingWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.x.feed(p[:n], w.off)
	w.off += int64(n)
	if w.wrote != nil {
		w.wrote(w.off)
	}
	return n, err
}

// samePath reports whether a and b name the same file.
func samePath(a, b string) bool {
	if sa, err := os.Stat(a); err == nil {
		if sb, err := os.Stat(b); err == nil {
			return os.SameFile(sa, sb)
		}
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
