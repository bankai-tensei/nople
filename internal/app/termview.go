package app

import (
	"fmt"
	"github.com/bankai-tensei/nople/internal/pty"
	"github.com/bankai-tensei/nople/internal/vt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/input"
	"github.com/bankai-tensei/gowaku/paint"
	"github.com/bankai-tensei/gowaku/text"
	"github.com/bankai-tensei/gowaku/theme"
	"github.com/bankai-tensei/gowaku/widgets/basic"
	"github.com/bankai-tensei/gowaku/widgets/nav"
)

// terminal is one shell in a panel of the dock.
type terminal struct {
	id    string
	n     int
	vt    *vt.Screen
	pty   *pty.PTY
	input chan []byte
	// exited means the shell ended, and err why it did not start.
	exited bool
	err    error
	// back is how many lines the view is scrolled up into the history.
	back int
	// sel is the selection, in lines of the history counted with those
	// trimmed off its top, and cells.
	sel      termSel
	ro       *renderTerm
	focusing bool
}

// termPos is a place in the history of a terminal.
type termPos struct{ line, col int }

// termSel is a selection of a terminal, from anchor to end.
type termSel struct {
	anchor, end termPos
	active      bool
}

// ordered returns the selection from its start to its end.
func (s termSel) ordered() (termPos, termPos) {
	a, b := s.anchor, s.end
	if b.line < a.line || b.line == a.line && b.col < a.col {
		a, b = b, a
	}
	return a, b
}

// title is the label of the panel of t.
func (t *terminal) title() string {
	name := T("term.title", t.n)
	if t.exited {
		name += T("term.exited")
	}
	return name
}

// newTerminal starts a shell in a new terminal panel below the editors, in
// the folder that is open.
func (a *app) newTerminal() {
	a.nextTerm++
	t := &terminal{id: fmt.Sprintf("term:%d", a.nextTerm), n: a.nextTerm, input: make(chan []byte, 64)}
	t.vt = vt.New(80, 24, func(b []byte) { t.write(b) })
	dir := ""
	if a.ws != nil {
		dir = a.ws.Root
	}
	p, err := pty.Start(a.shellArgs(), dir, 80, 24)
	if err != nil {
		t.err, t.exited = err, true
		t.vt.Write([]byte(T("term.cannotStart", err.Error()) + "\r\n"))
	} else {
		t.pty = p
		go func() {
			for b := range t.input {
				if _, err := p.Write(b); err != nil {
					return
				}
			}
		}()
		go func() {
			buf := make([]byte, 32<<10)
			for {
				n, err := p.Read(buf)
				if n > 0 {
					data := append([]byte(nil), buf[:n]...)
					a.shell.post(func() { t.output(data) })
				}
				if err != nil {
					a.shell.post(func() { a.termExited(t) })
					return
				}
			}
		}()
	}
	others := a.termOrder
	a.terms[t.id] = t
	a.termOrder = append(a.termOrder, t.id)
	a.refresh()
	if len(others) > 0 {
		a.dock.Move(t.id, others[len(others)-1], nav.DockCenter)
	} else {
		a.dock.Move(t.id, a.anchor(), nav.DockBottom)
	}
	a.dock.Activate(t.id)
	t.focusing = true
}

// shellArgs is the command line of a new shell.
func (a *app) shellArgs() []string {
	if len(a.shellCmd) > 0 {
		return a.shellCmd
	}
	return pty.DefaultShell()
}

// write sends input to the shell.
func (t *terminal) write(b []byte) {
	if t.pty == nil || t.exited {
		return
	}
	select {
	case t.input <- b:
	default:
		// The shell reads nothing; what it does not take is lost rather
		// than holding up the window.
	}
}

// output takes what the shell wrote.
func (t *terminal) output(b []byte) {
	before := t.vt.HistoryLen() + t.vt.Trimmed
	t.vt.Write(b)
	if t.back > 0 {
		// The view stays on the lines it showed.
		t.back += t.vt.HistoryLen() + t.vt.Trimmed - before
		t.back = min(t.back, len(t.vt.Scrollback))
	}
	if t.ro != nil {
		t.ro.MarkNeedsPaint()
	}
}

// termExited marks t ended.
func (a *app) termExited(t *terminal) {
	if t.exited {
		return
	}
	t.exited = true
	t.vt.Write([]byte("\r\n\x1b[2m" + T("term.processExited") + "\x1b[0m\r\n"))
	if t.ro != nil {
		t.ro.MarkNeedsPaint()
	}
	a.refresh()
}

// termClosed ends the terminal id, whose panel closed.
func (a *app) termClosed(id string) {
	t := a.terms[id]
	if t == nil {
		return
	}
	delete(a.terms, id)
	a.termOrder = slices.DeleteFunc(a.termOrder, func(s string) bool { return s == id })
	t.exited = true
	close(t.input)
	if t.pty != nil {
		go t.pty.Close()
	}
	a.refresh()
}

// closeTerminals ends every terminal and its shell, as the window closes.
func (a *app) closeTerminals() {
	for _, id := range slices.Clone(a.termOrder) {
		t := a.terms[id]
		a.termClosed(id)
		if t.pty != nil {
			t.pty.Close()
		}
	}
}

// toggleTerminal shows the terminals, starting one if there is none, or
// slides them down to the strip at the bottom if they show.
func (a *app) toggleTerminal() {
	if len(a.termOrder) == 0 {
		a.newTerminal()
		return
	}
	id := a.termOrder[len(a.termOrder)-1]
	arr := a.dock.Arrangement()
	for _, h := range arr.Hidden {
		if slices.Contains(h.Origin, id) || h.ID == id {
			a.dock.Pin(h.ID)
			a.focusTerminal(id)
			return
		}
	}
	if g := groupOf(arr, id); g != nil {
		if act := activeIn(arr, id); act != "" {
			id = act
		}
	}
	a.dock.UnpinGroup(id, nav.DockBottom)
}

// activeIn is the active panel of the group of id.
func activeIn(arr nav.DockArrangement, id string) string {
	if g := findGroup(&arr.Root, id); g != nil {
		return g.Active
	}
	return ""
}

// focusTerminal gives the keyboard to terminal id once it shows.
func (a *app) focusTerminal(id string) {
	if t := a.terms[id]; t != nil {
		t.focusing = true
		a.dock.Activate(id)
		a.refresh()
	}
}

// isTerminal reports whether id is the panel of a terminal.
func isTerminal(id string) bool { return strings.HasPrefix(id, "term:") }

// terminalView is the content of the panel of a terminal.
type terminalView struct {
	core.StatelessBase
	app  *app
	term *terminal
}

// Build implements core.StatelessWidget.
func (v terminalView) Build(ctx core.BuildContext) core.Widget {
	th := basic.ThemeOf(ctx)
	shaper, _ := core.DependOn[*text.Shaper](ctx)
	return basic.Container{Color: th.Color.Canvas, Padding: geom.Symmetric(theme.Space8, theme.Space4), Child: termBox{
		app: v.app, term: v.term, shaper: shaper,
		style:  text.Style{Family: v.app.settings.FontFamily, Size: v.app.fontSize},
		colors: termColorsOf(th),
	}}
}

// termColors are the colors a terminal draws with.
type termColors struct {
	fg, bg, cursor, selection geom.Color
	palette                   [256]geom.Color
}

// termColorsOf makes the colors of a terminal from th: the ink and the
// canvas of the theme, and 16 colors that read on it.
func termColorsOf(th *theme.Theme) *termColors {
	c := &termColors{fg: th.Color.Ink, bg: th.Color.Canvas, cursor: th.Color.AccentInk, selection: th.Color.AccentTint}
	ansi := []uint32{
		0x1E1E1E, 0xCD3131, 0x107C10, 0x949800, 0x0451A5, 0xBC05BC, 0x0598BC, 0x555555,
		0x666666, 0xCD3131, 0x14CE14, 0xB5BA00, 0x0451A5, 0xBC05BC, 0x0598BC, 0xA5A5A5,
	}
	if th.Brightness == theme.BrightnessDark {
		ansi = []uint32{
			0x000000, 0xCD3131, 0x0DBC79, 0xE5E510, 0x2472C8, 0xBC3FBC, 0x11A8CD, 0xE5E5E5,
			0x666666, 0xF14C4C, 0x23D18B, 0xF5F543, 0x3B8EEA, 0xD670D6, 0x29B8DB, 0xFFFFFF,
		}
	}
	for i, v := range ansi {
		c.palette[i] = geom.Hex(v)
	}
	steps := []uint32{0, 95, 135, 175, 215, 255}
	for i := range 216 {
		r, g, b := steps[i/36], steps[i/6%6], steps[i%6]
		c.palette[16+i] = geom.Hex(r<<16 | g<<8 | b)
	}
	for i := range 24 {
		v := uint32(8 + 10*i)
		c.palette[232+i] = geom.Hex(v<<16 | v<<8 | v)
	}
	return c
}

// color resolves a color of a cell; fg says which default to use.
func (c *termColors) color(tc vt.Color, fg bool) geom.Color {
	switch tc.Kind() {
	case vt.ColorIndexed:
		return c.palette[tc.Value()&0xFF]
	case vt.ColorRGB:
		return geom.Hex(tc.Value())
	}
	if fg {
		return c.fg
	}
	return c.bg
}

// termBox is the grid of a terminal.
type termBox struct {
	app    *app
	term   *terminal
	shaper *text.Shaper
	style  text.Style
	colors *termColors
}

// Key implements core.Widget.
func (termBox) Key() core.Key { return nil }

// CreateRenderObject implements core.RenderWidget.
func (b termBox) CreateRenderObject() core.RenderObject {
	r := &renderTerm{}
	b.UpdateRenderObject(r)
	return r
}

// UpdateRenderObject implements core.RenderWidget.
func (b termBox) UpdateRenderObject(ro core.RenderObject) {
	r := ro.(*renderTerm)
	if r.term != b.term && r.term != nil && r.term.ro == r {
		r.term.ro = nil
	}
	if r.shaper != b.shaper || r.style != b.style {
		r.shaper, r.style = b.shaper, b.style
		r.glyphs = map[glyphKey]glyphSet{}
		r.rows = map[*vt.Line]rowRuns{}
		r.measure()
		r.MarkNeedsLayout()
	}
	if r.colors != b.colors {
		r.colors = b.colors
		r.rows = map[*vt.Line]rowRuns{}
	}
	r.app, r.term = b.app, b.term
	r.term.ro = r
	if r.term.focusing && r.app != nil {
		// The focus waits for the object to be in the tree.
		r.term.focusing = false
		r.app.shell.post(func() {
			if r.Attached() {
				r.RequestFocus()
			}
		})
	}
	r.MarkNeedsPaint()
}

// glyphKey is a character in a style, whose glyphs the view keeps.
type glyphKey struct {
	r            rune
	bold, italic bool
}

// glyphSet is the glyphs of one character, and how far it advances.
type glyphSet struct {
	runs []paint.GlyphRun
	adv  float32
}

// rowRuns are the glyphs of a line, at the version they were made for.
type rowRuns struct {
	ver  uint64
	runs []paint.GlyphRun
}

// renderTerm draws the grid of a terminal and takes its keys, its text and
// the IME.
type renderTerm struct {
	core.RenderBase
	app    *app
	term   *terminal
	shaper *text.Shaper
	style  text.Style
	colors *termColors

	cellW, cellH, ascent float32
	glyphs               map[glyphKey]glyphSet
	rows                 map[*vt.Line]rowRuns
	focused              bool
	composition          *input.Composition
	dragging             bool
}

// measure finds the size of a cell from the font.
func (r *renderTerm) measure() {
	r.cellW, r.cellH, r.ascent = r.style.Size*0.6, r.style.Size*1.4, r.style.Size
	if r.shaper == nil {
		return
	}
	l := r.shaper.Shape("MMMMMMMMMM", r.style).Layout(geom.Inf)
	sz := l.Size()
	r.cellW = sz.W / 10
	r.cellH = float32(int(sz.H*1.15 + 0.5))
	if runs := l.Runs(); len(runs) > 0 {
		r.ascent = runs[0].Origin.Y + (r.cellH-sz.H)/2
	}
}

// Layout implements core.RenderObject. The grid takes the room it is given,
// and the terminal takes the size that fits.
func (r *renderTerm) Layout(c geom.Constraints) geom.Size {
	sz := c.Constrain(geom.Size{W: c.MaxW, H: c.MaxH})
	if sz.W >= geom.Inf || sz.H >= geom.Inf {
		sz = geom.Size{W: 640, H: 320}
	}
	if r.cellW > 0 && r.cellH > 0 && r.term != nil {
		cols, rows := max(int(sz.W/r.cellW), 2), max(int(sz.H/r.cellH), 1)
		if cols != r.term.vt.Cols || rows != r.term.vt.Rows {
			r.term.vt.Resize(cols, rows)
			r.term.back = min(r.term.back, len(r.term.vt.Scrollback))
			if p := r.term.pty; p != nil && !r.term.exited {
				go p.Resize(cols, rows)
			}
		}
	}
	return sz
}

// first is the line of the history at the top of the view.
func (r *renderTerm) first() int {
	t := r.term.vt
	return len(t.Scrollback) - r.term.back
}

// Paint implements core.RenderObject.
func (r *renderTerm) Paint(cv *paint.Canvas, off geom.Offset) {
	t := r.term
	v := t.vt
	cols := r.colors
	cv.DrawRRect(geom.RRectAll(geom.Rect{X: off.X, Y: off.Y, W: r.Size().W, H: r.Size().H}, 0), paint.Paint{Color: cols.bg})
	first := r.first()
	sa, sb := t.sel.ordered()
	for row := range v.Rows {
		l := v.History(first + row)
		if l == nil {
			continue
		}
		y := off.Y + float32(row)*r.cellH
		abs := first + row + v.Trimmed
		// Grounds: colored cells, inverse ones and the selection.
		for x := 0; x < len(l.Cells); x++ {
			c := l.Cells[x]
			bg := c.BG
			inverse := c.Attr&vt.AttrInverse != 0
			selected := t.sel.active && inSel(sa, sb, abs, x)
			if bg.IsDefault() && !inverse && !selected {
				continue
			}
			var color geom.Color
			switch {
			case selected:
				color = cols.selection
			case inverse:
				color = cols.color(c.FG, true)
			default:
				color = cols.color(bg, false)
			}
			cv.DrawRRect(geom.RRectAll(geom.Rect{X: off.X + float32(x)*r.cellW, Y: y, W: r.cellW + 0.5, H: r.cellH}, 0), paint.Paint{Color: color})
		}
		runs := r.rowRuns(l)
		for i := range runs {
			cv.DrawGlyphs(&runs[i], geom.Offset{X: off.X, Y: y + r.ascent}, cols.fg)
		}
		// Underlines and strikes.
		for x, c := range l.Cells {
			if c.Attr&(vt.AttrUnderline|vt.AttrStrike) == 0 || c.R == ' ' {
				continue
			}
			fg := cols.color(c.FG, true)
			if c.Attr&vt.AttrUnderline != 0 {
				cv.DrawRRect(geom.RRectAll(geom.Rect{X: off.X + float32(x)*r.cellW, Y: y + r.ascent + 2, W: r.cellW, H: 1}, 0), paint.Paint{Color: fg})
			}
			if c.Attr&vt.AttrStrike != 0 {
				cv.DrawRRect(geom.RRectAll(geom.Rect{X: off.X + float32(x)*r.cellW, Y: y + r.cellH/2, W: r.cellW, H: 1}, 0), paint.Paint{Color: fg})
			}
		}
	}
	// The cursor, and what the IME converts at it.
	if t.back == 0 && !t.exited {
		cx := off.X + float32(v.Cur.X)*r.cellW
		cy := off.Y + float32(v.Cur.Y)*r.cellH
		if r.composition != nil && r.composition.Text != "" && r.shaper != nil {
			l := r.shaper.Shape(r.composition.Text, r.style).Layout(geom.Inf)
			w := l.Size().W
			cv.DrawRRect(geom.RRectAll(geom.Rect{X: cx, Y: cy, W: w, H: r.cellH}, 0), paint.Paint{Color: cols.bg})
			for _, run := range l.Runs() {
				run := run
				cv.DrawGlyphs(&run, geom.Offset{X: cx, Y: cy + r.ascent - run.Origin.Y + run.Origin.Y}, cols.fg)
			}
			cv.DrawRRect(geom.RRectAll(geom.Rect{X: cx, Y: cy + r.cellH - 2, W: w, H: 1}, 0), paint.Paint{Color: cols.cursor})
			cx += w
		}
		if v.CursorVisible {
			c := cols.cursor
			if r.focused {
				c.A = 0.55
				cv.DrawRRect(geom.RRectAll(geom.Rect{X: cx, Y: cy, W: r.cellW, H: r.cellH}, 0), paint.Paint{Color: c})
			} else {
				cv.DrawRRect(geom.RRectAll(geom.Rect{X: cx, Y: cy, W: r.cellW, H: r.cellH}, 0), paint.Paint{Color: c, Stroke: &paint.Stroke{Width: 1}})
			}
		}
		if r.focused {
			g := core.GlobalOffset(r)
			r.Host().SetIMECursorRect(geom.Rect{X: g.X + float32(v.Cur.X)*r.cellW, Y: g.Y + float32(v.Cur.Y)*r.cellH, W: r.cellW, H: r.cellH})
		}
	}
}

// inSel reports whether cell x of line abs is in the selection a to b.
func inSel(a, b termPos, abs, x int) bool {
	switch {
	case abs < a.line || abs > b.line:
		return false
	case a.line == b.line:
		return x >= a.col && x < b.col
	case abs == a.line:
		return x >= a.col
	case abs == b.line:
		return x < b.col
	}
	return true
}

// rowRuns returns the glyphs of line l, made again only when it changed:
// each character comes from the glyphs of its own, set at its cell, so that
// the grid stays a grid whatever font a character falls back to.
func (r *renderTerm) rowRuns(l *vt.Line) []paint.GlyphRun {
	if rr, ok := r.rows[l]; ok && rr.ver == l.Ver {
		return rr.runs
	}
	if len(r.rows) > 4*vt.MaxScrollback {
		r.rows = map[*vt.Line]rowRuns{}
	}
	var runs []paint.GlyphRun
	for x, c := range l.Cells {
		if c.R == ' ' || c.R == 0 || c.Attr&vt.AttrWideTail != 0 {
			continue
		}
		fg := r.colors.color(c.FG, true)
		if c.Attr&vt.AttrInverse != 0 {
			fg = r.colors.color(c.BG, false)
		}
		if c.Attr&vt.AttrBold != 0 && c.FG.Kind() == vt.ColorIndexed && c.FG.Value() < 8 {
			fg = r.colors.palette[c.FG.Value()+8]
		}
		if c.Attr&vt.AttrDim != 0 {
			fg.A *= 0.6
		}
		gs := r.glyphsOf(c.R, c.Attr&vt.AttrBold != 0, c.Attr&vt.AttrItalic != 0)
		dx := float32(x) * r.cellW
		if w := 2 * r.cellW; c.Attr&vt.AttrWide != 0 && gs.adv < w {
			// A wide character sits in the middle of its two cells.
			dx += (w - gs.adv) / 2
		}
		for _, g := range gs.runs {
			n := len(runs)
			if n == 0 || runs[n-1].Face != g.Face || runs[n-1].Size != g.Size || runs[n-1].Color != fg {
				runs = append(runs, paint.GlyphRun{Face: g.Face, Size: g.Size, Color: fg,
					Bounds: geom.Rect{X: 0, Y: -r.cellH, W: float32(len(l.Cells)+2) * r.cellW, H: 2 * r.cellH}})
				n++
			}
			for _, gl := range g.Glyphs {
				runs[n-1].Glyphs = append(runs[n-1].Glyphs, paint.Glyph{
					ID: gl.ID, X: dx + g.Origin.X + gl.X, Y: gl.Y,
				})
			}
		}
	}
	r.rows[l] = rowRuns{ver: l.Ver, runs: runs}
	return runs
}

// glyphsOf returns the glyphs of the character ch, shaped once, with their
// origins relative to the start of the cell on the baseline.
func (r *renderTerm) glyphsOf(ch rune, bold, italic bool) glyphSet {
	k := glyphKey{ch, bold, italic}
	if g, ok := r.glyphs[k]; ok {
		return g
	}
	var out glyphSet
	if r.shaper != nil {
		st := r.style
		if bold {
			st.Weight = 700
		}
		st.Italic = italic
		l := r.shaper.Shape(string(ch), st).Layout(geom.Inf)
		out.adv = l.Size().W
		for _, run := range l.Runs() {
			run.Origin.Y = 0
			out.runs = append(out.runs, run)
		}
	}
	r.glyphs[k] = out
	return out
}

// cellAt is the place of the history under pos.
func (r *renderTerm) cellAt(pos geom.Offset) termPos {
	row := min(max(int(pos.Y/r.cellH), 0), r.term.vt.Rows-1)
	col := min(max(int(pos.X/r.cellW+0.5), 0), r.term.vt.Cols)
	return termPos{line: r.first() + row + r.term.vt.Trimmed, col: col}
}

// HandlePointer implements core.PointerTarget: a press takes the keyboard
// and starts a selection, a drag extends it, the wheel scrolls the history,
// and Ctrl and a click opens a path with a line in the output.
func (r *renderTerm) HandlePointer(e *core.PointerEvent) {
	t := r.term
	switch e.Kind {
	case core.PointerDown:
		r.RequestFocus()
		if e.Button != input.ButtonPrimary {
			return
		}
		p := r.cellAt(e.Pos)
		if e.Modifiers&input.ModControl != 0 && r.openLinkAt(p) {
			e.Handled = true
			return
		}
		t.sel = termSel{anchor: p, end: p}
		r.dragging = true
		e.Handled = true
		r.MarkNeedsPaint()
	case core.PointerMove:
		if r.dragging {
			t.sel.end = r.cellAt(e.Pos)
			t.sel.active = t.sel.end != t.sel.anchor
			r.MarkNeedsPaint()
		}
	case core.PointerUp:
		r.dragging = false
	case core.PointerScroll:
		lines := int(e.Scroll.Y * 3)
		if lines == 0 {
			return
		}
		t.back = min(max(t.back+lines, 0), len(t.vt.Scrollback))
		e.Handled = true
		r.MarkNeedsPaint()
	}
}

// linkPattern finds a path with a line, as compilers and tests write them:
// file.go:12 or file.go:12:5.
var linkPattern = regexp.MustCompile(`(?:[A-Za-z]:)?[\w.\-/\\]+\.\w+:(\d+)(?::(\d+))?`)

// openLinkAt opens the path with a line under p in a tab, if there is one.
func (r *renderTerm) openLinkAt(p termPos) bool {
	v := r.term.vt
	l := v.History(p.line - v.Trimmed)
	if l == nil || r.app == nil {
		return false
	}
	s := vt.LineText(l, 0, len(l.Cells))
	// The column in cells is a column in characters here, as long as the
	// line has no wide characters before it; good enough for paths.
	runes := []rune(s)
	at := len(string(runes[:min(p.col, len(runes))]))
	for _, m := range linkPattern.FindAllStringSubmatchIndex(s, -1) {
		if at < m[0] || at > m[1] {
			continue
		}
		path := s[m[0] : m[2]-1]
		line, _ := strconv.Atoi(s[m[2]:m[3]])
		col := 0
		if m[4] >= 0 {
			col, _ = strconv.Atoi(s[m[4]:m[5]])
		}
		return r.app.openPathAt(path, line, col)
	}
	return false
}

// openPathAt opens path, relative to the folder, at line and col from 1.
func (a *app) openPathAt(path string, line, col int) bool {
	if !filepath.IsAbs(path) && a.ws != nil {
		path = filepath.Join(a.ws.Root, path)
	}
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		return false
	}
	a.openFile(path, false)
	if t := a.currentTab(); t != nil {
		a.gotoLine(t, line-1, max(col-1, 0), 0)
	}
	return true
}

// selectedText is the text of the selection, lines joined by line breaks
// except where a long line wrapped.
func (r *renderTerm) selectedText() string {
	t := r.term
	if !t.sel.active {
		return ""
	}
	a, b := t.sel.ordered()
	var sb strings.Builder
	for line := a.line; line <= b.line; line++ {
		l := t.vt.History(line - t.vt.Trimmed)
		if l == nil {
			continue
		}
		from, to := 0, len(l.Cells)
		if line == a.line {
			from = a.col
		}
		if line == b.line {
			to = b.col
		}
		sb.WriteString(vt.LineText(l, from, to))
		if line < b.line && !l.Wrapped {
			sb.WriteString("\r\n")
		}
	}
	return sb.String()
}

// CanFocus implements core.FocusTarget.
func (r *renderTerm) CanFocus() bool { return true }

// FocusChanged implements core.FocusTarget.
func (r *renderTerm) FocusChanged(focused bool) {
	r.focused = focused
	r.Host().SetIMEEnabled(focused)
	r.MarkNeedsPaint()
}

// HandleText implements core.FocusTarget: typed text goes to the shell.
func (r *renderTerm) HandleText(s string) {
	s = strings.Map(func(c rune) rune {
		if c < 0x20 || c == 0x7F {
			return -1
		}
		return c
	}, s)
	if s == "" {
		return
	}
	r.toShell([]byte(s))
}

// HandleComposition implements core.FocusTarget: the text the IME converts
// shows at the cursor until it is committed.
func (r *renderTerm) HandleComposition(c *input.Composition) {
	r.composition = c
	r.MarkNeedsPaint()
}

// toShell sends b to the shell and brings the view back to the bottom.
func (r *renderTerm) toShell(b []byte) {
	t := r.term
	t.back = 0
	t.sel.active = false
	t.write(b)
	r.MarkNeedsPaint()
}

// paste sends the text of the clipboard, bracketed if the program asked.
func (r *renderTerm) paste() {
	s := r.Host().ClipboardText()
	if s == "" {
		return
	}
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\r"), "\n", "\r")
	if r.term.vt.BracketedPaste {
		s = "\x1b[200~" + s + "\x1b[201~"
	}
	r.toShell([]byte(s))
}

// HandleKey implements core.FocusTarget. Keys with Ctrl go to the shell as
// their controls, as do the arrows and the like; Ctrl+Shift and a letter,
// Ctrl+J and Ctrl+Tab are left to the window, but Ctrl+Shift+C copies and
// Ctrl+Shift+V or Ctrl+V pastes.
func (r *renderTerm) HandleKey(e *core.KeyEvent) bool {
	m := e.Modifiers
	ctrl, shift, alt := m&input.ModControl != 0, m&input.ModShift != 0, m&input.ModAlt != 0
	switch {
	case ctrl && shift && e.Key == input.KeyC:
		if s := r.selectedText(); s != "" {
			r.Host().SetClipboardText(s)
		}
		return true
	case ctrl && (e.Key == input.KeyV) && !alt:
		r.paste()
		return true
	case ctrl && !shift && e.Key == input.KeyC && r.term.sel.active:
		r.Host().SetClipboardText(r.selectedText())
		r.term.sel.active = false
		r.MarkNeedsPaint()
		return true
	case ctrl && shift, ctrl && e.Key == input.KeyJ, ctrl && e.Key == input.KeyAt, ctrl && e.Key == input.KeyTab:
		return false
	}
	if s := keySequence(e.Key, ctrl, shift, alt, r.term.vt.AppCursor); s != "" {
		r.toShell([]byte(s))
		return true
	}
	return false
}

// keySequence is what a terminal sends for a key that is not text.
func keySequence(k input.KeyCode, ctrl, shift, alt, appCursor bool) string {
	mod := 1
	if shift {
		mod++
	}
	if alt {
		mod += 2
	}
	if ctrl {
		mod += 4
	}
	cursor := func(final byte) string {
		switch {
		case mod > 1:
			return fmt.Sprintf("\x1b[1;%d%c", mod, final)
		case appCursor:
			return "\x1bO" + string(final)
		}
		return "\x1b[" + string(final)
	}
	tilde := func(n int) string {
		if mod > 1 {
			return fmt.Sprintf("\x1b[%d;%d~", n, mod)
		}
		return fmt.Sprintf("\x1b[%d~", n)
	}
	switch k {
	case input.KeyEnter:
		if alt {
			return "\x1b\r"
		}
		return "\r"
	case input.KeyBackspace:
		if ctrl {
			return "\x08"
		}
		if alt {
			return "\x1b\x7f"
		}
		return "\x7f"
	case input.KeyTab:
		if shift {
			return "\x1b[Z"
		}
		return "\t"
	case input.KeyEscape:
		return "\x1b"
	case input.KeyUp:
		return cursor('A')
	case input.KeyDown:
		return cursor('B')
	case input.KeyRight:
		return cursor('C')
	case input.KeyLeft:
		return cursor('D')
	case input.KeyHome:
		return cursor('H')
	case input.KeyEnd:
		return cursor('F')
	case input.KeyInsert:
		return tilde(2)
	case input.KeyDelete:
		return tilde(3)
	case input.KeyPageUp:
		return tilde(5)
	case input.KeyPageDown:
		return tilde(6)
	case input.KeyF1, input.KeyF2, input.KeyF3, input.KeyF4:
		final := "PQRS"[k-input.KeyF1]
		if mod > 1 {
			return fmt.Sprintf("\x1b[1;%d%c", mod, final)
		}
		return "\x1bO" + string(final)
	case input.KeyF5, input.KeyF6, input.KeyF7, input.KeyF8, input.KeyF9, input.KeyF10, input.KeyF11, input.KeyF12:
		codes := []int{15, 17, 18, 19, 20, 21, 23, 24}
		return tilde(codes[k-input.KeyF5])
	case input.KeySpace:
		if ctrl {
			return "\x00"
		}
		return ""
	}
	if k >= input.KeyA && k <= input.KeyZ {
		letter := byte(k-input.KeyA) + 'a'
		switch {
		case ctrl:
			s := string(rune(letter - 'a' + 1))
			if alt {
				s = "\x1b" + s
			}
			return s
		case alt:
			if shift {
				letter -= 'a' - 'A'
			}
			return "\x1b" + string(letter)
		}
	}
	return ""
}
