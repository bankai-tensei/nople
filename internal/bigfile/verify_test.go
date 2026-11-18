package bigfile

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bankai-tensei/gowaku/widgets/editor"
)

// The tests here check, on files as large as one asks, that what Nople saves
// is exactly what it should: they run only when asked, as they write
// gigabytes.
//
//	NOPLE_VERIFY=100,1024         the sizes, in MB
//	NOPLE_VERIFY_KINDS=lf,crlf,mixed,bom  the kinds of line breaks (default all)
//	NOPLE_VERIFY_DIR=dir          where the files go (default: a temp dir)
//	NOPLE_VERIFY_KILL=1024        the size of the file TestVerifyKill saves
//
// TestVerifySave writes a log of fixed lines, edits it at its start, in its
// middle and at its end, saves it, and reads the file saved line by line
// against the text it should hold, computed apart: every line, and the break
// of every line the edits did not touch. It then edits and saves again, undoes
// that and saves again, and checks both. TestVerifyKill kills a process in the
// middle of a save, and checks that the file is as it was.

// vline is the width of a line of the log, without its break.
const vline = 72

// vkind is a kind of line breaks of a log.
type vkind struct {
	name string
	bom  bool
	// crlf reports whether line i breaks with \r\n.
	crlf func(i int) bool
	// before returns how many of the lines before line i break with \r\n.
	before func(i int) int64
}

var vkinds = map[string]vkind{
	"lf":    {name: "lf", crlf: func(int) bool { return false }, before: func(int) int64 { return 0 }},
	"crlf":  {name: "crlf", crlf: func(int) bool { return true }, before: func(i int) int64 { return int64(i) }},
	"mixed": {name: "mixed", crlf: func(i int) bool { return i%3 == 0 }, before: func(i int) int64 { return int64((i + 2) / 3) }},
	"bom":   {name: "bom", bom: true, crlf: func(int) bool { return false }, before: func(int) int64 { return 0 }},
}

// vcontent is line i of the log, without its break.
func vcontent(i int) string {
	s := fmt.Sprintf("%012d request handled by worker %d in %d ms status=ok", i, i%16, i%997)
	return s + strings.Repeat(".", vline-len(s))
}

// vlog writes a log of n lines of kind k at path, each with its break, unless
// one of its size is there.
func vlog(t *testing.T, path string, k vkind, n int) {
	size := int64(n)*(vline+1) + k.before(n)
	if k.bom {
		size += 3
	}
	if st, err := os.Stat(path); err == nil && st.Size() == size {
		return
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriterSize(f, 4<<20)
	if k.bom {
		w.Write(bom)
	}
	for i := range n {
		w.WriteString(vcontent(i))
		if k.crlf(i) {
			w.WriteString("\r\n")
		} else {
			w.WriteByte('\n')
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// vplan is where the edits go in a log of n lines, each on a line of the
// file as it was written: a word replaced in line m, two lines put before
// line k, lines d to d+99 taken out (they cross a group of the index), line j
// joined with the next, and two lines after the last.
type vplan struct{ n, m, k, d, j int }

func planFor(n int) vplan { return vplan{n: n, m: n/5 + 11, k: n/3 + 5, d: n/2 + 37, j: 3*n/4 + 1} }

// edit makes the edits of p in ctl, from the end, so that each is where the
// file had it. With second, it also puts "SECOND " at the start, as an edit
// after the save.
func (p vplan) edit(ctl *editor.Controller) {
	at := func(line, col int) editor.TextPosition { return editor.TextPosition{Line: line, Column: col} }
	put := func(from, to editor.TextPosition, s string) {
		ctl.Replace(editor.TextRange{Start: from, End: to}, s)
	}
	put(at(p.n, 0), at(p.n, 0), "appended line A\nappended line B")
	put(at(p.j, vline), at(p.j+1, 0), "")
	put(at(p.d, 0), at(p.d+100, 0), "")
	put(at(p.k, 0), at(p.k, 0), "inserted 1\ninserted 2\n")
	put(at(p.m, 3), at(p.m, 9), "MID")
	put(at(0, 0), at(0, 0), "HEAD ")
}

// vout is a line the saved file should hold: its text, and the line of the
// file it comes from with its break, or -1 for a line the edits made or
// touched, which may break as the file's first line does.
type vout struct {
	text string
	orig int
}

// expect calls fn with the lines the saved file should hold, in order.
func (p vplan) expect(second bool, fn func(o vout) bool) {
	touched := func(i int) bool {
		for _, e := range []int{0, p.m, p.k, p.d - 1, p.d + 100, p.j, p.j + 1, p.n - 1, p.n} {
			if i >= e-1 && i <= e+1 {
				return true
			}
		}
		return false
	}
	for i := 0; i <= p.n; i++ {
		orig := i
		if touched(i) {
			orig = -1
		}
		var outs []vout
		switch {
		case i == 0:
			head := "HEAD " + vcontent(0)
			if second {
				head = "SECOND " + head
			}
			outs = []vout{{head, -1}}
		case i == p.m:
			c := vcontent(i)
			outs = []vout{{c[:3] + "MID" + c[9:], -1}}
		case i == p.k:
			outs = []vout{{"inserted 1", -1}, {"inserted 2", -1}, {vcontent(i), orig}}
		case i >= p.d && i < p.d+100, i == p.j+1:
			// Taken out, or joined to the line before.
		case i == p.j:
			outs = []vout{{vcontent(i) + vcontent(i+1), -1}}
		case i == p.n:
			outs = []vout{{"appended line A", -1}, {"appended line B", -1}}
		default:
			outs = []vout{{vcontent(i), orig}}
		}
		for _, o := range outs {
			if !fn(o) {
				return
			}
		}
	}
}

// check reads the file at path against what p expects, and returns the
// SHA-256 of the file.
func (p vplan) check(t *testing.T, path string, k vkind, second bool) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	r := bufio.NewReaderSize(io.TeeReader(f, h), 4<<20)
	if k.bom {
		head := make([]byte, 3)
		if _, err := io.ReadFull(r, head); err != nil || !bytes.Equal(head, bom) {
			t.Fatalf("the BOM is gone: %x", head)
		}
	}
	nl := "\n"
	if k.crlf(0) {
		nl = "\r\n"
	}
	line, fails := 0, 0
	var total int
	p.expect(second, func(vout) bool { total++; return true })
	p.expect(second, func(o vout) bool {
		raw, err := r.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		text, brk := raw, ""
		switch {
		case strings.HasSuffix(raw, "\r\n"):
			text, brk = raw[:len(raw)-2], "\r\n"
		case strings.HasSuffix(raw, "\n"):
			text, brk = raw[:len(raw)-1], "\n"
		}
		last := line == total-1
		want := ""
		switch {
		case last:
			want = ""
		case o.orig >= 0 && k.crlf(o.orig):
			want = "\r\n"
		case o.orig >= 0:
			want = "\n"
		}
		bad := text != o.text
		switch {
		case last && brk != "":
			bad = true
		case !last && o.orig >= 0 && brk != want:
			bad = true
		case !last && o.orig < 0 && brk != nl && brk != "\n" && brk != "\r\n":
			bad = true
		case !last && brk == "":
			bad = true
		}
		if bad {
			fails++
			t.Errorf("line %d: %q break %q, want %q break %q (from line %d)", line, clip(text), brk, clip(o.text), want, o.orig)
			if fails > 10 {
				return false
			}
		}
		line++
		return true
	})
	if extra, _ := r.ReadString(0); extra != "" {
		t.Errorf("%d bytes after the last line: %q", len(extra), clip(extra))
	}
	if line != total {
		t.Errorf("%d lines, want %d", line, total)
	}
	return h.Sum(nil)
}

func verifyDir(t *testing.T) string {
	if dir := os.Getenv("NOPLE_VERIFY_DIR"); dir != "" {
		return dir
	}
	return t.TempDir()
}

func TestVerifySave(t *testing.T) {
	sizes := os.Getenv("NOPLE_VERIFY")
	if sizes == "" {
		t.Skip("set NOPLE_VERIFY to verify saving large files")
	}
	kinds := strings.Split(os.Getenv("NOPLE_VERIFY_KINDS"), ",")
	if kinds[0] == "" {
		kinds = []string{"lf", "crlf", "mixed", "bom"}
	}
	dir := verifyDir(t)
	for _, s := range strings.Split(sizes, ",") {
		mb, _ := strconv.Atoi(strings.TrimSpace(s))
		for _, kn := range kinds {
			k, ok := vkinds[kn]
			if !ok {
				t.Fatalf("no kind %q", kn)
			}
			t.Run(fmt.Sprintf("%dMB-%s", mb, kn), func(t *testing.T) { verifySave(t, dir, mb, k) })
		}
	}
}

func verifySave(t *testing.T, dir string, mb int, k vkind) {
	sub := filepath.Join(dir, fmt.Sprintf("verify-%d-%s", mb, k.name))
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sub, "big.log")
	n := int((int64(mb) << 20) / (vline + 1))
	start := time.Now()
	vlog(t, path, k, n)
	wrote := time.Since(start)
	p := planFor(n)

	u := newUI()
	var ctl editor.Controller
	start = time.Now()
	d, err := Open(&ctl, path, Options{Post: u.post})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for d.Loading() {
		(<-u)()
	}
	indexed := time.Since(start)
	if got := ctl.LineCount(); got != n+1 {
		t.Fatalf("%d lines, want %d", got, n+1)
	}
	p.edit(&ctl)

	// The first and the last save on a goroutine of their own, as the
	// application saves a large file; the one between in place.
	start = time.Now()
	if err := backgroundSave(t, u, d); err != nil {
		t.Fatal(err)
	}
	saved := time.Since(start)
	first := p.check(t, path, k, false)
	if t.Failed() {
		return
	}

	// Again, after the save: the text reads the file it wrote.
	ctl.Replace(editor.TextRange{}, "SECOND ")
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	p.check(t, path, k, true)
	// Undone, the text is what the first save wrote, to the byte.
	ctl.Undo()
	if err := backgroundSave(t, u, d); err != nil {
		t.Fatal(err)
	}
	if again := p.check(t, path, k, false); !bytes.Equal(again, first) {
		t.Errorf("after undo, the file differs from the first save")
	}
	entries, _ := os.ReadDir(sub)
	if len(entries) != 1 {
		t.Errorf("the folder holds %d files, want only the log", len(entries))
	}
	t.Logf("%d MB %s: %d lines; wrote the log in %v, indexed in %v, saved in %v",
		mb, k.name, n, wrote.Round(time.Millisecond), indexed.Round(time.Millisecond), saved.Round(time.Millisecond))
}

// childPrefix is what the child process puts at the start of the file
// before it saves.
const childPrefix = "edited by the child "

// TestVerifyKill kills a process that saves a large file, at points through
// the save, and checks each time that the file is either as it was or, when
// the save ended just before the kill, as the save wrote it whole; never in
// between.
func TestVerifyKill(t *testing.T) {
	if os.Getenv("NOPLE_VERIFY_CHILD") != "" {
		verifyChild()
		return
	}
	size := os.Getenv("NOPLE_VERIFY_KILL")
	if size == "" {
		t.Skip("set NOPLE_VERIFY_KILL to kill saves")
	}
	mb, _ := strconv.Atoi(size)
	sub := filepath.Join(verifyDir(t), fmt.Sprintf("kill-%d", mb))
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sub, "big.log")
	n := int((int64(mb) << 20) / (vline + 1))
	sum := func(prefix string) []byte {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		h := sha256.New()
		h.Write([]byte(prefix))
		io.Copy(h, f)
		return h.Sum(nil)
	}
	for _, frac := range []float64{0.05, 0.5, 0.95} {
		os.Remove(path)
		vlog(t, path, vkinds["crlf"], n)
		before, whole := sum(""), sum(childPrefix)
		st, _ := os.Stat(path)
		cmd := exec.Command(os.Args[0], "-test.run", "^TestVerifyKill$")
		cmd.Env = append(os.Environ(), "NOPLE_VERIFY_CHILD="+path)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		// Kill it once the file it writes is that far.
		killed, ended := false, false
		for !killed && !ended {
			select {
			case <-done:
				ended = true
				continue
			default:
			}
			entries, _ := os.ReadDir(sub)
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".tmp") {
					if info, err := e.Info(); err == nil && float64(info.Size()) >= frac*float64(st.Size()) {
						cmd.Process.Kill()
						killed = true
					}
				}
			}
		}
		if killed {
			<-done
		}
		after := sum("")
		state := "as it was"
		switch {
		case bytes.Equal(after, before):
		case bytes.Equal(after, whole):
			state = "saved whole, just before the kill"
		default:
			t.Fatalf("killed at %.0f%% of the save: the file is neither as it was nor as saved", frac*100)
		}
		var left []string
		entries, _ := os.ReadDir(sub)
		for _, e := range entries {
			if e.Name() != "big.log" {
				left = append(left, e.Name())
				os.Remove(filepath.Join(sub, e.Name()))
			}
		}
		if !killed {
			state += " (the save ended before the kill)"
		}
		t.Logf("killed at %.0f%% of the save: the file is %s; left behind: %v", frac*100, state, left)
	}
}

// backgroundSave saves d through StartSaveAs, running what it posts to u,
// and returns the error of the save once it ended.
func backgroundSave(t *testing.T, u ui, d *Doc) error {
	t.Helper()
	ended := false
	var saveErr error
	if err := d.StartSaveAs(d.Path(), nil, func(err error) { ended, saveErr = true, err }); err != nil {
		return err
	}
	for !ended {
		(<-u)()
	}
	return saveErr
}

// verifyChild opens the file of NOPLE_VERIFY_CHILD, edits its start and
// saves it once, on a goroutine of its own as the application does.
func verifyChild() {
	path := os.Getenv("NOPLE_VERIFY_CHILD")
	u := newUI()
	var ctl editor.Controller
	d, err := Open(&ctl, path, Options{Post: u.post})
	if err != nil {
		os.Exit(2)
	}
	for d.Loading() {
		(<-u)()
	}
	ctl.Replace(editor.TextRange{}, childPrefix)
	ended := false
	var saveErr error
	if err := d.StartSaveAs(path, nil, func(err error) { ended, saveErr = true, err }); err != nil {
		os.Exit(3)
	}
	for !ended {
		(<-u)()
	}
	if saveErr != nil {
		os.Exit(3)
	}
	os.Exit(0)
}
