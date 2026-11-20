// Package bigfile shows a file of any size in an editor.Controller without
// reading it all: File is an editor.LineSource that indexes the file on a
// goroutine of its own and reads the lines that are asked for, and Doc binds
// one to a controller, with what an editor of files does besides showing it:
// it notices the file changing on disk, follows the lines a program appends
// to it, saves the edits through a file beside it that then takes its place,
// and reads the file again keeping the edits, as a patch is applied.
//
// The lines break at \n and \r\n, which the text holds as \n, while a lone \r
// stays in its line; a UTF-8 BOM is kept out of the text and written back.
// The bytes are taken as UTF-8.
//
// gowaku's editor did this itself until it took its lines through
// editor.LineSource (see its docs/ADR/0153, 0155, 0156 and 0157); the
// behavior here follows those records.
package bigfile

import (
	"bytes"
	"errors"
	"io"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bankai-tensei/gowaku/widgets/editor"
)

// groupLines is how many lines share an entry of the index. A line is found
// by reading its group and cutting it at the breaks, so the index of a file of
// a hundred million lines takes 25 MB.
const groupLines = 64

// syncIndex is how much of a file Open indexes before it returns; the rest is
// indexed on a goroutine of its own. indexChunk is how much that goroutine
// reads at a time, and indexReport how much it reads between reports. They
// are variables for the tests.
var (
	syncIndex   int64 = 1 << 20
	indexChunk  int64 = 1 << 20
	indexReport int64 = 32 << 20
)

// cachedGroups is how many groups of lines a file keeps read.
const cachedGroups = 512

// bom is the byte order mark of UTF-8.
var bom = []byte{0xEF, 0xBB, 0xBF}

// File is a file whose lines an editor shows without holding them. It keeps
// an index of where every groupLines-th line starts, in the file and in the
// text, where a \r\n counts as the one byte of \n, and reads the groups of
// lines that are asked for.
//
// Its methods run on the UI goroutine, but for Reader's reader. The goroutine
// that indexes hands what it finds over through pending, which apply takes.
type File struct {
	path string
	// held is the file kept open where holdOpen says so; reading opens the
	// file by its path elsewhere, and checks it is the same file.
	held atomic.Pointer[os.File]
	size int64
	// raw[g] and norm[g] are the offsets of line g*groupLines in the file and
	// in the text.
	raw, norm []int64
	// n is the number of lines indexed: the complete lines, and the last one
	// once done. rawEnd is where the lines indexed end in the file, and
	// normEnd where they end in the text, with a break after each.
	n       int
	rawEnd  int64
	normEnd int64
	done    bool
	// crlf says that the first line ends in \r\n, as new lines then do when
	// the text is saved; hasBOM that the file starts with a BOM.
	crlf, hasBOM bool
	// info, mod and tail are what the file was when it was read: the file
	// itself, its time and its last bytes, against which probe tells how it
	// changed since.
	info os.FileInfo
	mod  time.Time
	tail []byte

	cache groupCache
	// recent are the groups read last, first the latest (plus one, so that
	// zero is none), and spans their bytes and those of the end of the file
	// with their sums, for probe and watch to read again: a file changed
	// without a change of its size or time is caught where the editor shows
	// it.
	recent [recentGroups]int
	spans  atomic.Pointer[[]span]

	// stop ends the goroutines of the file.
	stop atomic.Bool
	// mu guards pending, the findings the indexing goroutine has not handed
	// over.
	mu      sync.Mutex
	pending []indexBatch
}

// group is a group of lines read from the file. cr marks the lines whose
// break is \r\n, and span the bytes it was read from.
type group struct {
	lines []string
	cr    uint64
	span  span
}

// span is a range of bytes of the file and the FNV-1a sum of what they held.
type span struct {
	off, n int64
	sum    uint64
}

// recentGroups is how many groups read last probe and watch read again.
const recentGroups = 4

// fnv returns the 64-bit FNV-1a sum of b.
func fnv(b []byte) uint64 {
	h := uint64(14695981039346656037)
	for _, c := range b {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return h
}

// same reports whether f still holds at sp what it held.
func same(f *os.File, sp span) bool {
	b := make([]byte, sp.n)
	if _, err := f.ReadAt(b, sp.off); err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	return fnv(b) == sp.sum
}

// groupCache keeps the groups read lately, in two generations, so that the
// lines in view stay read while a scroll reads new ones.
type groupCache struct {
	cur, old map[int]*group
}

func (c *groupCache) get(g int) *group {
	if p, ok := c.cur[g]; ok {
		return p
	}
	if p, ok := c.old[g]; ok {
		c.put(g, p)
		return p
	}
	return nil
}

func (c *groupCache) put(g int, p *group) {
	if c.cur == nil || len(c.cur) >= cachedGroups/2 {
		c.old, c.cur = c.cur, make(map[int]*group, cachedGroups/2)
	}
	c.cur[g] = p
}

func (c *groupCache) drop(g int) {
	delete(c.cur, g)
	delete(c.old, g)
}

// indexer finds the line breaks of a file fed to it in order.
type indexer struct {
	// raw and norm are the group starts found since the last batch.
	raw, norm []int64
	// lines is the number of complete lines, pos where the next line starts
	// in the file, and drop the bytes before it that are not in the text: the
	// \r of each \r\n, and the BOM.
	lines  int
	pos    int64
	drop   int64
	prevCR bool
	// crlf says how the first line ended, once firstSeen.
	crlf, firstSeen bool
}

// feed takes the bytes of the file from offset at on.
func (x *indexer) feed(b []byte, at int64) {
	for i := 0; i < len(b); {
		k := bytes.IndexByte(b[i:], '\n')
		if k < 0 {
			break
		}
		i += k
		crlf := i > 0 && b[i-1] == '\r' || i == 0 && x.prevCR
		if crlf {
			x.drop++
		}
		if !x.firstSeen {
			x.crlf, x.firstSeen = crlf, true
		}
		x.lines++
		x.pos = at + int64(i) + 1
		if x.lines%groupLines == 0 {
			x.raw = append(x.raw, x.pos)
			x.norm = append(x.norm, x.pos-x.drop)
		}
		i++
	}
	if len(b) > 0 {
		x.prevCR = b[len(b)-1] == '\r'
	}
}

// indexBatch is what the indexing goroutine found since its last report.
// from, when not negative, is the line the index was found again from, as
// after an append: the index past its group is replaced.
type indexBatch struct {
	from      int
	raw, norm []int64
	lines     int
	pos, drop int64
	done      bool
	err       error
}

// batch returns the findings since the last call.
func (x *indexer) batch(done bool) indexBatch {
	b := indexBatch{from: -1, raw: x.raw, norm: x.norm, lines: x.lines, pos: x.pos, drop: x.drop, done: done}
	x.raw, x.norm = nil, nil
	return b
}

// apply takes a batch of the indexing goroutine. It returns the number of
// lines known before, from which the lines of the batch are new or changed.
func (s *File) apply(b indexBatch) int {
	before := s.n
	if b.from >= 0 {
		// The index past the group of the line found again goes. The slices
		// are cut to their length, so that the readers of the old index keep
		// what they read.
		g := b.from/groupLines + 1
		s.raw, s.norm = slices.Clip(s.raw[:min(g, len(s.raw))]), slices.Clip(s.norm[:min(g, len(s.norm))])
		before = b.from
	}
	s.raw = append(s.raw, b.raw...)
	s.norm = append(s.norm, b.norm...)
	s.n = b.lines
	s.rawEnd = b.pos
	s.normEnd = b.pos - b.drop
	if b.done {
		// The last line runs to the end of the file, without a break.
		s.n++
		s.normEnd += s.size - b.pos + 1
		s.rawEnd = s.size
		s.done = true
	}
	// The last group read may have had only some of its lines.
	s.cache.drop(before / groupLines)
	return before
}

// openFile opens path and indexes its start. It returns the file, and the
// indexer to go on with from the offset at, or nil when the whole file is
// indexed.
func openFile(path string) (s *File, x *indexer, at int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, 0, err
	}
	fail := func(err error) (*File, *indexer, int64, error) {
		_ = f.Close() //nolint:errcheck // the error before is the one to report
		return nil, nil, 0, err
	}
	st, err := f.Stat()
	if err != nil {
		return fail(err)
	}
	if st.IsDir() {
		return fail(errors.New(path + ": is a folder"))
	}
	s = &File{path: path, size: st.Size()}
	head := make([]byte, 3)
	if n, _ := f.ReadAt(head, 0); n == 3 && bytes.Equal(head, bom) {
		s.hasBOM, at = true, 3
	}
	s.raw, s.norm = []int64{at}, []int64{0}
	if err := s.remember(st, f); err != nil {
		return fail(err)
	}
	x = &indexer{pos: at, drop: at}
	buf := make([]byte, min(syncIndex, max(s.size, 1)))
	// The start, and on until a line is complete or the file ends.
	for at < s.size && (at < syncIndex || x.lines == 0) {
		n, err := f.ReadAt(buf, at)
		x.feed(buf[:n], at)
		at += int64(n)
		if err != nil && !errors.Is(err, io.EOF) {
			return fail(err)
		}
		if n == 0 {
			break
		}
	}
	done := at >= s.size
	s.apply(x.batch(done))
	s.crlf = x.crlf
	s.keep(f)
	if done {
		return s, nil, at, nil
	}
	return s, x, at, nil
}

// keep keeps f open as the file of s where holdOpen says so, and closes it
// elsewhere.
func (s *File) keep(f *os.File) {
	if holdOpen {
		s.held.Store(f)
		return
	}
	_ = f.Close() //nolint:errcheck // only read
}

// errReplaced is the error of reading a file whose path names another file
// now, where the file is opened for each read.
var errReplaced = errors.New("bigfile: the file was replaced")

// opener opens the file of a File on any goroutine, as it was when the
// opener was made.
type opener struct {
	path string
	info os.FileInfo
	held *os.File
}

// opener returns an opener of the file, for another goroutine.
func (s *File) opener() opener { return opener{path: s.path, info: s.info, held: s.held.Load()} }

// open returns the file to read from, which the caller closes: a copy of the
// file held open, or the file opened by its path, checked to be the same file
// as before.
func (o opener) open() (*os.File, error) {
	if o.held != nil {
		return dup(o.held)
	}
	f, err := os.Open(o.path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err == nil && !os.SameFile(st, o.info) {
		err = errReplaced
	}
	if err != nil {
		_ = f.Close() //nolint:errcheck // only read
		return nil, err
	}
	return f, nil
}

// handle returns the file to read from on the UI goroutine and the function
// to call when done.
func (s *File) handle() (*os.File, func(), error) {
	if f := s.held.Load(); f != nil {
		return f, func() {}, nil
	}
	f, err := s.opener().open()
	if err != nil {
		return nil, nil, err
	}
	return f, func() { _ = f.Close() }, nil //nolint:errcheck // only read
}

// index reads the rest of the file from at on, on a goroutine of its own, and
// hands what it finds to pending, calling wake after each report. It reports
// before the end only once x has at least least lines.
func (s *File) index(o opener, x *indexer, from int, at, size, chunk, every int64, least int, wake func()) {
	report := func(b indexBatch) {
		b.from, from = from, -1
		s.mu.Lock()
		s.pending = append(s.pending, b)
		s.mu.Unlock()
		wake()
	}
	fail := func(err error) {
		b := x.batch(false)
		b.err = err
		report(b)
	}
	f, err := o.open()
	if err != nil {
		fail(err)
		return
	}
	defer f.Close() //nolint:errcheck // only read
	buf := make([]byte, chunk)
	since := int64(0)
	for at < size {
		if s.stop.Load() {
			return
		}
		n, err := f.ReadAt(buf[:min(chunk, size-at)], at)
		x.feed(buf[:n], at)
		at += int64(n)
		since += int64(n)
		if err != nil && !errors.Is(err, io.EOF) {
			fail(err)
			return
		}
		if n == 0 {
			break
		}
		if since >= every && at < size && x.lines >= least {
			since = 0
			report(x.batch(false))
		}
	}
	report(x.batch(true))
}

// take returns the batches the goroutine handed over.
func (s *File) take() []indexBatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.pending
	s.pending = nil
	return b
}

// Close stops the goroutines of the file and lets it go.
func (s *File) Close() error {
	s.stop.Store(true)
	if f := s.held.Swap(nil); f != nil {
		return f.Close()
	}
	return nil
}

// Path returns the path of the file.
func (s *File) Path() string { return s.path }

// Size returns the size of the file as it was read.
func (s *File) Size() int64 { return s.size }

// Indexed returns how far the file is indexed, in bytes.
func (s *File) Indexed() int64 { return s.rawEnd }

// Done reports whether the whole file is indexed.
func (s *File) Done() bool { return s.done }

// CRLF reports whether the first line of the file ends in \r\n.
func (s *File) CRLF() bool { return s.crlf }

// BOM reports whether the file starts with the byte order mark of UTF-8.
func (s *File) BOM() bool { return s.hasBOM }

func groupEnd(raw []int64, n int, rawEnd int64, g int) int64 {
	if g+1 < len(raw) && (g+1)*groupLines <= n {
		return raw[g+1]
	}
	return rawEnd
}

// readGroup reads group g of the n lines indexed in raw, which end at rawEnd,
// from f, and cuts it into lines.
func readGroup(f *os.File, raw []int64, n int, rawEnd int64, g int) (*group, error) {
	count := min(groupLines, n-g*groupLines)
	p := &group{lines: make([]string, count)}
	start, end := raw[g], groupEnd(raw, n, rawEnd, g)
	data := make([]byte, end-start)
	p.span = span{off: start, n: end - start}
	if _, err := f.ReadAt(data, start); err != nil && !errors.Is(err, io.EOF) {
		return p, err
	}
	p.span.sum = fnv(data)
	text := string(data)
	for i := range count {
		k := strings.IndexByte(text, '\n')
		if k < 0 {
			p.lines[i] = text
			break
		}
		l := text[:k]
		if strings.HasSuffix(l, "\r") {
			l = l[:len(l)-1]
			p.cr |= 1 << i
		}
		p.lines[i] = l
		text = text[k+1:]
	}
	return p, nil
}

// group returns group g of the lines, reading it if it is not at hand. A
// group that cannot be read is empty lines, and is read again next time.
func (s *File) group(g int) *group {
	if p := s.cache.get(g); p != nil {
		s.used(g)
		return p
	}
	f, done, err := s.handle()
	if err != nil {
		return &group{lines: make([]string, min(groupLines, s.n-g*groupLines))}
	}
	p, err := readGroup(f, s.raw, s.n, s.rawEnd, g)
	done()
	if err != nil {
		return p
	}
	s.cache.put(g, p)
	s.used(g)
	return p
}

// used notes that group g was read, and hands the spans of the groups read
// last to watch when they changed.
func (s *File) used(g int) {
	if s.recent[0] == g+1 {
		return
	}
	k := 0
	for k < recentGroups-1 && s.recent[k] != g+1 {
		k++
	}
	copy(s.recent[1:k+1], s.recent[:k])
	s.recent[0] = g + 1
	s.storeSpans()
}

// storeSpans hands the spans of the groups read last, and of the end of the
// file, to the goroutine of watch.
func (s *File) storeSpans() {
	spans := make([]span, 0, recentGroups+1)
	for _, r := range s.recent {
		if r == 0 {
			continue
		}
		if q := s.cache.get(r - 1); q != nil && q.span.n > 0 {
			spans = append(spans, q.span)
		}
	}
	spans = append(spans, s.tailSpan())
	s.spans.Store(&spans)
}

// Len implements editor.LineSource.
func (s *File) Len() int { return max(s.n, 1) }

// Line implements editor.LineSource.
func (s *File) Line(i int) string {
	if s.n == 0 {
		return ""
	}
	return s.group(i / groupLines).lines[i%groupLines]
}

// Offset implements editor.LineSource.
func (s *File) Offset(i int) int64 {
	if i >= s.n {
		return s.normEnd
	}
	g := i / groupLines
	off := s.norm[g]
	if k := i % groupLines; k > 0 {
		for _, l := range s.group(g).lines[:k] {
			off += int64(len(l)) + 1
		}
	}
	return off
}

// LineAt implements editor.LineSource.
func (s *File) LineAt(off int64) int {
	if off >= s.normEnd {
		return max(s.n-1, 0)
	}
	groups := min(len(s.norm), (s.n+groupLines-1)/groupLines)
	g := max(sort.Search(groups, func(g int) bool { return s.norm[g] > off })-1, 0)
	at := s.norm[g]
	p := s.group(g)
	for k, l := range p.lines {
		if at += int64(len(l)) + 1; at > off {
			return g*groupLines + k
		}
	}
	return min(g*groupLines+len(p.lines)-1, max(s.n-1, 0))
}

// rawStart returns where line i starts in the file, for i from 0 to n.
func (s *File) rawStart(i int) int64 {
	if i >= s.n {
		return s.rawEnd
	}
	g := i / groupLines
	off := s.raw[g]
	if k := i % groupLines; k > 0 {
		p := s.group(g)
		for j, l := range p.lines[:k] {
			off += int64(len(l)) + 1 + int64(p.cr>>j&1)
		}
	}
	return off
}

// WriteLines implements editor.LineWriter: it copies the bytes of the lines
// from the file, with their breaks as they are.
func (s *File) WriteLines(w io.Writer, from, to int) error {
	if from >= to {
		return nil
	}
	start, end := s.rawStart(from), s.rawStart(to)
	if to >= s.n {
		end = s.size
	}
	f, done, err := s.handle()
	if err != nil {
		return err
	}
	defer done()
	_, err = io.Copy(w, io.NewSectionReader(f, start, end-start))
	return err
}

// Reader implements editor.LineSource. The reader opens the file itself, on
// the goroutine that reads, and reads the lines by the index as it is now. It
// is also an editor.LineWriter, so that a snapshot of the text written on
// another goroutine copies the lines it did not edit as they are, breaks
// included.
func (s *File) Reader() editor.LineReader {
	r := &reader{o: s.opener(), raw: s.raw, n: s.n, rawEnd: s.rawEnd, size: s.size}
	if r.o.held != nil {
		// A copy of the file held open, taken now, while it is open: the
		// reader may first read after the File closed, as a save that
		// lands after Close does.
		r.f, r.err = dup(r.o.held)
	}
	return r
}

// reader reads the lines of a File on another goroutine. raw is the index as
// it was when the reader was made: the index only grows at its end, so that
// the part of it the reader knows stays as it was.
type reader struct {
	o opener
	// f is the file read, opened at the first read, or taken when the
	// reader was made with err the error of taking it.
	f      *os.File
	err    error
	raw    []int64
	n      int
	rawEnd int64
	size   int64
}

// file returns the file the reader reads, opening it the first time.
func (r *reader) file() (*os.File, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.f == nil {
		f, err := r.o.open()
		if err != nil {
			return nil, err
		}
		r.f = f
	}
	return r.f, nil
}

// rawStart returns where line i starts in the file, for i from 0 to n, as
// File.rawStart does but reading the group of the line from the file.
func (r *reader) rawStart(f *os.File, i int) (int64, error) {
	if i >= r.n {
		return r.rawEnd, nil
	}
	g := i / groupLines
	off := r.raw[g]
	if k := i % groupLines; k > 0 {
		p, err := readGroup(f, r.raw, r.n, r.rawEnd, g)
		if err != nil {
			return 0, err
		}
		for j, l := range p.lines[:k] {
			off += int64(len(l)) + 1 + int64(p.cr>>j&1)
		}
	}
	return off, nil
}

// WriteLines implements editor.LineWriter, as File.WriteLines does, on the
// goroutine of the reader: it copies the bytes of the lines from the file,
// with their breaks as they are.
func (r *reader) WriteLines(w io.Writer, from, to int) error {
	if from >= to {
		return nil
	}
	f, err := r.file()
	if err != nil {
		return err
	}
	start, err := r.rawStart(f, from)
	if err != nil {
		return err
	}
	end := r.size
	if to < r.n {
		if end, err = r.rawStart(f, to); err != nil {
			return err
		}
	}
	_, err = io.Copy(w, io.NewSectionReader(f, start, end-start))
	return err
}

// ReadLines implements editor.LineReader.
func (r *reader) ReadLines(from, to int, fn func(i int, line string) bool) error {
	if _, err := r.file(); err != nil {
		return err
	}
	for g := from / groupLines; g*groupLines < min(to, r.n); g++ {
		p, err := readGroup(r.f, r.raw, r.n, r.rawEnd, g)
		if err != nil {
			return err
		}
		for k, l := range p.lines {
			if i := g*groupLines + k; i >= from && i < to && !fn(i, l) {
				return nil
			}
		}
	}
	return nil
}

// Close implements editor.LineReader.
func (r *reader) Close() error {
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}

// tailBytes is how much of the end of a file probe compares to tell an
// append from a rewrite.
const tailBytes = 4096

// remember notes what the file is now, for probe, reading its end from f.
func (s *File) remember(st os.FileInfo, f *os.File) error {
	s.info, s.mod = st, st.ModTime()
	n := min(int64(tailBytes), s.size)
	s.tail = make([]byte, n)
	var err error
	if n > 0 {
		_, err = f.ReadAt(s.tail, s.size-n)
		if errors.Is(err, io.EOF) {
			err = nil
		}
	}
	s.storeSpans()
	return err
}

// tailSpan returns the span of the end of the file that remember read.
func (s *File) tailSpan() span {
	n := int64(len(s.tail))
	return span{off: s.size - n, n: n, sum: fnv(s.tail)}
}

// readAt reads len(b) bytes of the file at off; the end of the file is no
// error.
func (s *File) readAt(b []byte, off int64) error {
	f, done, err := s.handle()
	if err != nil {
		return err
	}
	defer done()
	_, err = f.ReadAt(b, off)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return err
}
