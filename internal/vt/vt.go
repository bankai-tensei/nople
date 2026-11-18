package vt

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/width"
)

// Color is a color of a cell: the default, one of the 256 of the palette,
// or 24-bit, by the kind in the top byte.
type Color uint32

const (
	ColorDefault Color = 0
	ColorIndexed Color = 1 << 24
	ColorRGB     Color = 2 << 24
)

func indexed(i int) Color { return ColorIndexed | Color(i&0xFF) }
func rgb(r, g, b int) Color {
	return ColorRGB | Color(r&0xFF)<<16 | Color(g&0xFF)<<8 | Color(b&0xFF)
}
func (c Color) Kind() Color     { return c & 0xFF000000 }
func (c Color) Value() uint32   { return uint32(c & 0xFFFFFF) }
func (c Color) IsDefault() bool { return c == ColorDefault }

// Attributes of a cell.
const (
	AttrBold uint8 = 1 << iota
	AttrDim
	AttrItalic
	AttrUnderline
	AttrInverse
	AttrStrike
	// AttrWide is the first cell of a wide character, AttrWideTail the
	// second, which holds nothing.
	AttrWide
	AttrWideTail
)

// Cell is one place of the grid.
type Cell struct {
	R      rune
	FG, BG Color
	Attr   uint8
}

// Line is one line of the grid. ver changes whenever its cells do, so that
// the view knows which lines to draw again; wrapped means the line goes on
// in the next one, as a long line does.
type Line struct {
	Cells   []Cell
	Ver     uint64
	Wrapped bool
}

// lineVersion numbers the changes of lines of all terminals, so that a
// version is never seen twice. It is used on the UI goroutine only.
var lineVersion uint64

// MaxScrollback is how many lines that scrolled off the top a terminal keeps.
const MaxScrollback = 5000

// Screen is the screen of a terminal: it reads the output of a program, with its
// VT escapes, into a grid of cells, and keeps the lines that scrolled off.
type Screen struct {
	Cols, Rows int
	// Lines are those of the screen; Scrollback those that scrolled off the
	// top, oldest first.
	Lines      []*Line
	Scrollback []*Line
	// trimmed counts the lines dropped off the top of the scrollback, so
	// that a position in the history stays put.
	Trimmed int

	// alt is the main screen while the alternate one shows, as for vim.
	alt      []*Line
	altSaved Cursor

	// Cur is the cursor.
	Cur   Cursor
	saved Cursor
	// wrapNext means the cursor is past the last column: the next character
	// goes on the next line.
	wrapNext bool
	// top and bottom are the scroll region, rows from 0, inclusive.
	top, bottom int

	CursorVisible  bool
	AppCursor      bool
	BracketedPaste bool
	autowrap       bool
	Title          string
	last           rune

	// reply sends answers to the program, such as the place of the cursor.
	reply func(b []byte)
	// changed counts every change, for the view.
	changed uint64

	// The parser.
	state   int
	params  []int
	private byte
	inter   []byte
	osc     []byte
	pending []byte
}

// Parser states.
const (
	stGround = iota
	stEscape
	stEscInter
	stCSI
	stOSC
	stOSCEsc
	stString
	stStringEsc
)

// Cursor is where the cursor is and how it writes.
type Cursor struct {
	X, Y  int
	pen   Cell
	valid bool
}

// New makes a screen of cols by rows.
func New(cols, rows int, reply func([]byte)) *Screen {
	t := &Screen{reply: reply, CursorVisible: true, autowrap: true}
	t.Cols, t.Rows = max(cols, 1), max(rows, 1)
	t.Lines = t.blankLines(t.Rows)
	t.top, t.bottom = 0, t.Rows-1
	return t
}

func (t *Screen) blankLines(n int) []*Line {
	ls := make([]*Line, n)
	for i := range ls {
		ls[i] = t.blankLine()
	}
	return ls
}

func (t *Screen) blankLine() *Line {
	lineVersion++
	l := &Line{Cells: make([]Cell, t.Cols), Ver: lineVersion}
	bg := t.Cur.pen.BG
	for i := range l.Cells {
		l.Cells[i] = Cell{R: ' ', BG: bg}
	}
	return l
}

// touch marks line l changed.
func (t *Screen) touch(l *Line) {
	lineVersion++
	l.Ver = lineVersion
	t.changed++
}

// Write reads output of the program.
func (t *Screen) Write(p []byte) {
	if len(t.pending) > 0 {
		p = append(t.pending, p...)
		t.pending = nil
	}
	for len(p) > 0 {
		b := p[0]
		if b >= 0x80 && t.state == stGround {
			if !utf8.FullRune(p) {
				t.pending = append([]byte(nil), p...)
				return
			}
			r, n := utf8.DecodeRune(p)
			t.print(r)
			p = p[n:]
			continue
		}
		t.byte(b)
		p = p[1:]
	}
	t.changed++
}

// byte runs the parser on one byte below 0x80, or any byte of an escape.
func (t *Screen) byte(b byte) {
	switch t.state {
	case stGround:
		switch {
		case b == 0x1B:
			t.state = stEscape
		case b < 0x20 || b == 0x7F:
			t.control(b)
		default:
			t.print(rune(b))
		}
	case stEscape:
		switch {
		case b == '[':
			t.state, t.params, t.private, t.inter = stCSI, t.params[:0], 0, t.inter[:0]
			t.params = append(t.params, -1)
		case b == ']':
			t.state, t.osc = stOSC, t.osc[:0]
		case b == 'P' || b == 'X' || b == '^' || b == '_':
			t.state = stString
		case b >= 0x20 && b <= 0x2F:
			t.state = stEscInter
		default:
			t.state = stGround
			t.escape(b)
		}
	case stEscInter:
		// Character sets and the like, which a UTF-8 terminal ignores.
		if b >= 0x30 {
			t.state = stGround
		}
	case stCSI:
		switch {
		case b >= '0' && b <= '9':
			i := len(t.params) - 1
			if t.params[i] < 0 {
				t.params[i] = 0
			}
			t.params[i] = min(t.params[i]*10+int(b-'0'), 1<<16)
		case b == ';' || b == ':':
			t.params = append(t.params, -1)
		case b >= '<' && b <= '?':
			t.private = b
		case b >= 0x20 && b <= 0x2F:
			t.inter = append(t.inter, b)
		case b >= 0x40 && b <= 0x7E:
			t.state = stGround
			t.csi(b)
		case b == 0x1B:
			t.state = stEscape
		case b < 0x20:
			t.control(b)
		}
	case stOSC:
		switch b {
		case 0x07:
			t.state = stGround
			t.oscDone()
		case 0x1B:
			t.state = stOSCEsc
		default:
			if len(t.osc) < 4096 {
				t.osc = append(t.osc, b)
			}
		}
	case stOSCEsc:
		t.state = stGround
		t.oscDone()
		if b != '\\' {
			t.byte(b)
		}
	case stString:
		if b == 0x1B {
			t.state = stStringEsc
		} else if b == 0x07 {
			t.state = stGround
		}
	case stStringEsc:
		t.state = stGround
		if b != '\\' {
			t.byte(b)
		}
	}
}

// control runs a C0 control.
func (t *Screen) control(b byte) {
	switch b {
	case '\b':
		if t.Cur.X > 0 {
			t.Cur.X--
		}
		t.wrapNext = false
	case '\t':
		t.Cur.X = min((t.Cur.X/8+1)*8, t.Cols-1)
		t.wrapNext = false
	case '\n', '\v', '\f':
		t.lineFeed()
	case '\r':
		t.Cur.X = 0
		t.wrapNext = false
	}
}

// escape runs ESC and a final byte.
func (t *Screen) escape(b byte) {
	switch b {
	case '7':
		t.saved = t.Cur
		t.saved.valid = true
	case '8':
		t.restoreCursor()
	case 'D':
		t.lineFeed()
	case 'E':
		t.Cur.X = 0
		t.lineFeed()
	case 'M':
		if t.Cur.Y == t.top {
			t.scrollDown(1)
		} else if t.Cur.Y > 0 {
			t.Cur.Y--
		}
	case 'c':
		reply := t.reply
		sb, trimmed := t.Scrollback, t.Trimmed
		*t = *New(t.Cols, t.Rows, reply)
		t.Scrollback, t.Trimmed = sb, trimmed
	}
}

func (t *Screen) restoreCursor() {
	if t.saved.valid {
		t.Cur = t.saved
		t.Cur.X, t.Cur.Y = min(t.Cur.X, t.Cols-1), min(t.Cur.Y, t.Rows-1)
	}
	t.wrapNext = false
}

// param returns parameter i, or def if it is missing or 0 where 0 means
// the default.
func (t *Screen) param(i, def int) int {
	if i >= len(t.params) || t.params[i] <= 0 {
		return def
	}
	return t.params[i]
}

// csi runs a control sequence.
func (t *Screen) csi(final byte) {
	if t.private == '?' {
		switch final {
		case 'h', 'l':
			for i := range t.params {
				t.mode(t.params[i], final == 'h')
			}
		}
		return
	}
	if t.private != 0 || len(t.inter) > 0 {
		// Queries of secondary attributes and the like: answered by nothing.
		if t.private == '>' && final == 'c' {
			t.send("\x1b[>0;0;0c")
		}
		return
	}
	switch final {
	case 'A':
		t.Cur.Y = max(t.Cur.Y-t.param(0, 1), t.topFor())
		t.wrapNext = false
	case 'B', 'e':
		t.Cur.Y = min(t.Cur.Y+t.param(0, 1), t.bottomFor())
		t.wrapNext = false
	case 'C', 'a':
		t.Cur.X = min(t.Cur.X+t.param(0, 1), t.Cols-1)
		t.wrapNext = false
	case 'D':
		t.Cur.X = max(t.Cur.X-t.param(0, 1), 0)
		t.wrapNext = false
	case 'E':
		t.Cur.Y = min(t.Cur.Y+t.param(0, 1), t.bottomFor())
		t.Cur.X = 0
		t.wrapNext = false
	case 'F':
		t.Cur.Y = max(t.Cur.Y-t.param(0, 1), t.topFor())
		t.Cur.X = 0
		t.wrapNext = false
	case 'G', '`':
		t.Cur.X = min(t.param(0, 1), t.Cols) - 1
		t.wrapNext = false
	case 'H', 'f':
		t.Cur.Y = min(t.param(0, 1), t.Rows) - 1
		t.Cur.X = min(t.param(1, 1), t.Cols) - 1
		t.wrapNext = false
	case 'd':
		t.Cur.Y = min(t.param(0, 1), t.Rows) - 1
		t.wrapNext = false
	case 'J':
		t.eraseDisplay(max(t.params[0], 0))
	case 'K':
		t.eraseLine(max(t.params[0], 0))
	case 'L':
		if t.Cur.Y >= t.top && t.Cur.Y <= t.bottom {
			t.scrollRegion(t.Cur.Y, t.bottom, -t.param(0, 1))
		}
	case 'M':
		if t.Cur.Y >= t.top && t.Cur.Y <= t.bottom {
			t.scrollRegion(t.Cur.Y, t.bottom, t.param(0, 1))
		}
	case 'P':
		t.deleteChars(t.param(0, 1))
	case '@':
		t.insertChars(t.param(0, 1))
	case 'X':
		l := t.Lines[t.Cur.Y]
		for i := t.Cur.X; i < min(t.Cur.X+t.param(0, 1), t.Cols); i++ {
			l.Cells[i] = Cell{R: ' ', BG: t.Cur.pen.BG}
		}
		t.touch(l)
	case 'S':
		t.scrollUp(t.param(0, 1))
	case 'T':
		t.scrollDown(t.param(0, 1))
	case 'b':
		for range min(t.param(0, 1), t.Cols*t.Rows) {
			t.print(t.last)
		}
	case 'm':
		t.sgr()
	case 'r':
		top, bottom := t.param(0, 1)-1, t.param(1, t.Rows)-1
		if top < bottom && bottom < t.Rows {
			t.top, t.bottom = top, bottom
			t.Cur.X, t.Cur.Y = 0, 0
			t.wrapNext = false
		}
	case 's':
		t.saved = t.Cur
		t.saved.valid = true
	case 'u':
		t.restoreCursor()
	case 'n':
		switch t.param(0, 0) {
		case 5:
			t.send("\x1b[0n")
		case 6:
			t.send(fmt.Sprintf("\x1b[%d;%dR", t.Cur.Y+1, t.Cur.X+1))
		}
	case 'c':
		t.send("\x1b[?1;2c")
	}
}

func (t *Screen) topFor() int {
	if t.Cur.Y >= t.top {
		return t.top
	}
	return 0
}

func (t *Screen) bottomFor() int {
	if t.Cur.Y <= t.bottom {
		return t.bottom
	}
	return t.Rows - 1
}

func (t *Screen) send(s string) {
	if t.reply != nil {
		t.reply([]byte(s))
	}
}

// mode sets a private mode of DEC.
func (t *Screen) mode(m int, on bool) {
	switch m {
	case 1:
		t.AppCursor = on
	case 7:
		t.autowrap = on
	case 25:
		t.CursorVisible = on
	case 47, 1047, 1049:
		t.setAlt(on, m == 1049)
	case 2004:
		t.BracketedPaste = on
	}
}

// setAlt shows the alternate screen, or the main one again.
func (t *Screen) setAlt(on, saveCursor bool) {
	switch {
	case on && t.alt == nil:
		if saveCursor {
			t.altSaved = t.Cur
		}
		t.alt = t.Lines
		t.Lines = t.blankLines(t.Rows)
	case !on && t.alt != nil:
		t.Lines = t.alt
		t.alt = nil
		for _, l := range t.Lines {
			t.touch(l)
		}
		if saveCursor {
			t.Cur = t.altSaved
		}
	}
	t.top, t.bottom = 0, t.Rows-1
	t.wrapNext = false
}

// sgr sets the pen by Select Graphic Rendition.
func (t *Screen) sgr() {
	p := &t.Cur.pen
	ps := t.params
	for i := 0; i < len(ps); i++ {
		v := max(ps[i], 0)
		switch {
		case v == 0:
			p.FG, p.BG, p.Attr = ColorDefault, ColorDefault, 0
		case v == 1:
			p.Attr |= AttrBold
		case v == 2:
			p.Attr |= AttrDim
		case v == 3:
			p.Attr |= AttrItalic
		case v == 4:
			p.Attr |= AttrUnderline
		case v == 7:
			p.Attr |= AttrInverse
		case v == 9:
			p.Attr |= AttrStrike
		case v == 21 || v == 22:
			p.Attr &^= AttrBold | AttrDim
		case v == 23:
			p.Attr &^= AttrItalic
		case v == 24:
			p.Attr &^= AttrUnderline
		case v == 27:
			p.Attr &^= AttrInverse
		case v == 29:
			p.Attr &^= AttrStrike
		case v >= 30 && v <= 37:
			p.FG = indexed(v - 30)
		case v == 39:
			p.FG = ColorDefault
		case v >= 40 && v <= 47:
			p.BG = indexed(v - 40)
		case v == 49:
			p.BG = ColorDefault
		case v >= 90 && v <= 97:
			p.FG = indexed(v - 90 + 8)
		case v >= 100 && v <= 107:
			p.BG = indexed(v - 100 + 8)
		case v == 38 || v == 48:
			var c Color
			n := 0
			if i+1 < len(ps) {
				switch ps[i+1] {
				case 5:
					if i+2 < len(ps) {
						c, n = indexed(max(ps[i+2], 0)), 2
					}
				case 2:
					// 38;2;r;g;b, or 38:2::r:g:b with a color space.
					rest := ps[i+2:]
					if len(rest) >= 4 && rest[0] < 0 {
						rest = rest[1:]
						n++
					}
					if len(rest) >= 3 {
						c, n = rgb(max(rest[0], 0), max(rest[1], 0), max(rest[2], 0)), n+4
					}
				}
			}
			if n > 0 {
				if v == 38 {
					p.FG = c
				} else {
					p.BG = c
				}
			}
			i += n
		}
	}
}

// oscDone runs an operating system command: only the title is kept.
func (t *Screen) oscDone() {
	num, text, ok := strings.Cut(string(t.osc), ";")
	if !ok {
		return
	}
	if n, err := strconv.Atoi(num); err == nil && (n == 0 || n == 2) {
		t.Title = text
	}
}

// runeWidth is how many cells r takes: 2 for wide characters of East Asia,
// 0 for marks that combine with the one before.
func runeWidth(r rune) int {
	switch {
	case r < 0x300:
		return 1
	case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || r == 0x200B || r == 0x200D || r >= 0xFE00 && r <= 0xFE0F:
		return 0
	}
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	}
	// Emoji are wide in terminals, though Unicode leaves some neutral.
	if r >= 0x1F300 && r <= 0x1FAFF {
		return 2
	}
	return 1
}

// print writes the character r at the cursor.
func (t *Screen) print(r rune) {
	w := runeWidth(r)
	if w == 0 {
		return
	}
	if t.wrapNext && t.autowrap {
		t.Lines[t.Cur.Y].Wrapped = true
		t.Cur.X = 0
		t.lineFeed()
	}
	t.wrapNext = false
	if w == 2 && t.Cur.X == t.Cols-1 {
		if t.autowrap {
			t.Lines[t.Cur.Y].Cells[t.Cur.X] = Cell{R: ' ', BG: t.Cur.pen.BG}
			t.Lines[t.Cur.Y].Wrapped = true
			t.Cur.X = 0
			t.lineFeed()
		} else {
			return
		}
	}
	l := t.Lines[t.Cur.Y]
	// Writing over half of a wide character clears its other half.
	t.clearWide(l, t.Cur.X)
	if w == 2 {
		t.clearWide(l, t.Cur.X+1)
	}
	c := t.Cur.pen
	c.R = r
	c.Attr &^= AttrWide | AttrWideTail
	if w == 2 {
		c.Attr |= AttrWide
		l.Cells[t.Cur.X] = c
		tail := t.Cur.pen
		tail.R = 0
		tail.Attr = tail.Attr&^AttrWide | AttrWideTail
		l.Cells[t.Cur.X+1] = tail
	} else {
		l.Cells[t.Cur.X] = c
	}
	t.touch(l)
	t.last = r
	t.Cur.X += w
	if t.Cur.X >= t.Cols {
		t.Cur.X = t.Cols - 1
		t.wrapNext = true
	}
}

// clearWide blanks the wide character that has a half at x.
func (t *Screen) clearWide(l *Line, x int) {
	if x >= t.Cols {
		return
	}
	switch c := l.Cells[x]; {
	case c.Attr&AttrWide != 0 && x+1 < t.Cols:
		l.Cells[x+1] = Cell{R: ' ', BG: c.BG}
	case c.Attr&AttrWideTail != 0 && x > 0:
		l.Cells[x-1] = Cell{R: ' ', BG: c.BG}
	}
}

// lineFeed moves the cursor down a line, scrolling at the bottom of the
// region.
func (t *Screen) lineFeed() {
	t.wrapNext = false
	switch {
	case t.Cur.Y == t.bottom:
		t.scrollUp(1)
	case t.Cur.Y < t.Rows-1:
		t.Cur.Y++
	}
}

// scrollUp scrolls the region up n lines. Lines leaving the top of the whole
// main screen go to the scrollback.
func (t *Screen) scrollUp(n int) { t.scrollRegion(t.top, t.bottom, n) }

// scrollDown scrolls the region down n lines.
func (t *Screen) scrollDown(n int) { t.scrollRegion(t.top, t.bottom, -n) }

// scrollRegion moves the lines from top to bottom up n lines, or down -n,
// bringing in blank lines.
func (t *Screen) scrollRegion(top, bottom, n int) {
	size := bottom - top + 1
	switch {
	case n > 0:
		n = min(n, size)
		if top == 0 && t.alt == nil {
			t.Scrollback = append(t.Scrollback, t.Lines[:n]...)
			if over := len(t.Scrollback) - MaxScrollback; over > 0 {
				t.Scrollback = append(t.Scrollback[:0:0], t.Scrollback[over:]...)
				t.Trimmed += over
			}
		}
		copy(t.Lines[top:], t.Lines[top+n:bottom+1])
		for i := bottom - n + 1; i <= bottom; i++ {
			t.Lines[i] = t.blankLine()
		}
	case n < 0:
		n = min(-n, size)
		copy(t.Lines[top+n:bottom+1], t.Lines[top:bottom+1-n])
		for i := top; i < top+n; i++ {
			t.Lines[i] = t.blankLine()
		}
	}
	t.changed++
}

// eraseDisplay clears below the cursor (0), above it (1), all (2), or the
// scrollback too (3).
func (t *Screen) eraseDisplay(m int) {
	switch m {
	case 0:
		t.eraseLine(0)
		for y := t.Cur.Y + 1; y < t.Rows; y++ {
			t.Lines[y] = t.blankLine()
		}
	case 1:
		t.eraseLine(1)
		for y := range t.Cur.Y {
			t.Lines[y] = t.blankLine()
		}
	case 2, 3:
		for y := range t.Rows {
			t.Lines[y] = t.blankLine()
		}
		if m == 3 {
			t.Trimmed += len(t.Scrollback)
			t.Scrollback = nil
		}
	}
	t.changed++
}

// eraseLine clears the line from the cursor (0), to it (1), or all (2).
func (t *Screen) eraseLine(m int) {
	l := t.Lines[t.Cur.Y]
	from, to := 0, t.Cols
	switch m {
	case 0:
		from = t.Cur.X
		l.Wrapped = false
	case 1:
		to = t.Cur.X + 1
	}
	for i := from; i < to; i++ {
		l.Cells[i] = Cell{R: ' ', BG: t.Cur.pen.BG}
	}
	t.touch(l)
}

func (t *Screen) deleteChars(n int) {
	l := t.Lines[t.Cur.Y]
	n = min(n, t.Cols-t.Cur.X)
	copy(l.Cells[t.Cur.X:], l.Cells[t.Cur.X+n:])
	for i := t.Cols - n; i < t.Cols; i++ {
		l.Cells[i] = Cell{R: ' ', BG: t.Cur.pen.BG}
	}
	t.touch(l)
}

func (t *Screen) insertChars(n int) {
	l := t.Lines[t.Cur.Y]
	n = min(n, t.Cols-t.Cur.X)
	copy(l.Cells[t.Cur.X+n:], l.Cells[t.Cur.X:t.Cols-n])
	for i := t.Cur.X; i < t.Cur.X+n; i++ {
		l.Cells[i] = Cell{R: ' ', BG: t.Cur.pen.BG}
	}
	t.touch(l)
}

// Resize changes the size of the grid. Lines are cut or padded, not
// reflowed; the program draws again after the resize anyway.
func (t *Screen) Resize(cols, rows int) {
	cols, rows = max(cols, 1), max(rows, 1)
	if cols == t.Cols && rows == t.Rows {
		return
	}
	fit := func(ls []*Line) {
		for _, l := range ls {
			switch {
			case len(l.Cells) > cols:
				l.Cells = l.Cells[:cols]
			case len(l.Cells) < cols:
				for len(l.Cells) < cols {
					l.Cells = append(l.Cells, Cell{R: ' '})
				}
			}
			t.touch(l)
		}
	}
	t.Cols = cols
	fit(t.Lines)
	fit(t.alt)
	for rows < len(t.Lines) {
		// The top line goes, into the scrollback, while the cursor is below
		// it; else the bottom one.
		if t.Cur.Y > 0 {
			if t.alt == nil {
				t.Scrollback = append(t.Scrollback, t.Lines[0])
			}
			t.Lines = t.Lines[1:]
			t.Cur.Y--
		} else {
			t.Lines = t.Lines[:len(t.Lines)-1]
		}
	}
	for rows > len(t.Lines) {
		t.Lines = append(t.Lines, t.blankLine())
	}
	if t.alt != nil {
		for len(t.alt) > rows {
			t.alt = t.alt[:len(t.alt)-1]
		}
		for len(t.alt) < rows {
			t.alt = append(t.alt, t.blankLine())
		}
	}
	t.Rows = rows
	t.top, t.bottom = 0, rows-1
	t.Cur.X, t.Cur.Y = min(t.Cur.X, cols-1), min(t.Cur.Y, rows-1)
	t.wrapNext = false
	t.changed++
}

// History returns line i of the whole history, the scrollback then the
// screen, or nil.
func (t *Screen) History(i int) *Line {
	switch {
	case i < 0:
		return nil
	case i < len(t.Scrollback):
		return t.Scrollback[i]
	case i-len(t.Scrollback) < len(t.Lines):
		return t.Lines[i-len(t.Scrollback)]
	}
	return nil
}

// HistoryLen is the number of lines in the history.
func (t *Screen) HistoryLen() int { return len(t.Scrollback) + len(t.Lines) }

// LineText is the text of l from cell a to b, without the spaces at its
// end.
func LineText(l *Line, a, b int) string {
	var sb strings.Builder
	for i := a; i < min(b, len(l.Cells)); i++ {
		c := l.Cells[i]
		if c.Attr&AttrWideTail != 0 {
			continue
		}
		if c.R == 0 {
			sb.WriteByte(' ')
			continue
		}
		sb.WriteRune(c.R)
	}
	return strings.TrimRight(sb.String(), " ")
}

// ScreenText is the text of the screen, a line each, for tests.
func (t *Screen) ScreenText() string {
	var sb strings.Builder
	for _, l := range t.Lines {
		sb.WriteString(LineText(l, 0, t.Cols))
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n")
}
