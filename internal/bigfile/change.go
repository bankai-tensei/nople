package bigfile

import (
	"bytes"
	"errors"
	"os"
	"time"
)

// Change says how a file changed on disk since it was read.
type Change int

const (
	// Unchanged is a file as it was read.
	Unchanged Change = iota
	// Appended is a file that grew at its end, the rest as it was, as a log
	// does that a program writes to.
	Appended
	// Rewritten is a file whose bytes changed in place, or that got shorter:
	// lines the text has not edited may no longer be read as they were.
	Rewritten
	// Replaced is a path that now names another file, or none, as after
	// another program saved by writing a new file and renaming it.
	Replaced
)

// String implements fmt.Stringer.
func (c Change) String() string {
	switch c {
	case Appended:
		return "appended"
	case Rewritten:
		return "rewritten"
	case Replaced:
		return "replaced"
	}
	return "unchanged"
}

// probe tells how the file changed since it was read.
func (s *File) probe() Change {
	st, err := os.Stat(s.path)
	if err != nil || !os.SameFile(st, s.info) {
		return Replaced
	}
	size := st.Size()
	switch {
	case size == s.size && st.ModTime().Equal(s.mod):
		// The same size and time, but the bytes may have changed all the
		// same: look at those of the lines read last and at the end.
		if !s.unchanged() {
			return Rewritten
		}
		return Unchanged
	case size < s.size:
		return Rewritten
	}
	// A file that grew is appended to if its old end is still there. Its time
	// alone cannot tell: writing in place moves it too.
	tail := make([]byte, len(s.tail))
	if len(tail) > 0 {
		if err := s.readAt(tail, s.size-int64(len(tail))); err != nil {
			if errors.Is(err, errReplaced) {
				return Replaced
			}
			return Rewritten
		}
	}
	if size > s.size && bytes.Equal(tail, s.tail) {
		return Appended
	}
	return Rewritten
}

// unchanged reports whether the groups read last and the end of the file
// hold what they held.
func (s *File) unchanged() bool { return sameBytes(s.opener(), s.spans.Load()) }

// sameBytes reports whether the file of o still holds what spans held. It runs
// on any goroutine.
func sameBytes(o opener, spans *[]span) bool {
	if spans == nil {
		return true
	}
	f, err := o.open()
	if err != nil {
		return false
	}
	defer f.Close() //nolint:errcheck // only read
	for _, sp := range *spans {
		if !same(f, sp) {
			return false
		}
	}
	return true
}

// reopen readies the file to index what was appended to it: the last line,
// which had no break, is indexed again with what follows it. It returns the
// indexer to go on with from the offset at, and the line indexed again; the
// lines known stay until the index hands its batch over.
func (s *File) reopen(st os.FileInfo) (x *indexer, last int, at int64, err error) {
	last = max(s.n-1, 0)
	pos := s.rawStart(last)
	x = &indexer{lines: last, pos: pos, drop: pos - s.Offset(last), crlf: s.crlf, firstSeen: true}
	f, done, err := s.handle()
	if err != nil {
		return nil, 0, 0, err
	}
	defer done()
	s.size, s.done = st.Size(), false
	return x, last, pos, s.remember(st, f)
}

// watch looks at the file every period on a goroutine of its own, until the
// file closes, and calls changed when what os.Stat tells of it differs from
// the last time, or when the bytes of the lines read last changed without
// it. It reads nothing else, so that the UI wakes only when the file changed.
func (s *File) watch(period time.Duration, changed func()) {
	o := s.opener()
	last, err := os.Stat(o.path)
	t := time.NewTicker(period)
	defer t.Stop()
	// told keeps a change of the bytes from being told on every look.
	told := false
	for range t.C {
		if s.stop.Load() {
			return
		}
		st, err2 := os.Stat(o.path)
		unmoved := err == nil && err2 == nil && os.SameFile(st, last) &&
			st.Size() == last.Size() && st.ModTime().Equal(last.ModTime())
		switch {
		case (err == nil) != (err2 == nil) || !unmoved && err2 == nil:
			changed()
			told = false
		case unmoved && !told && !sameBytes(o, s.spans.Load()):
			// The size and the time stayed, the bytes did not.
			changed()
			told = true
		}
		last, err = st, err2
	}
}
