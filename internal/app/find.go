package app

import (
	"github.com/bankai-tensei/gowaku/widgets/field"
	"github.com/bankai-tensei/nople/internal/workspace"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Limits of quick open.
const (
	// maxListedFiles is the most files quick open knows of a workspace.
	maxListedFiles = 200_000
	// pickerShown is the most rows a picker lists.
	pickerShown = 50
)

// fileList is the files of the workspace for quick open, read once off the
// UI goroutine when the folder opens.
type fileList struct {
	ws *workspace.Workspace
	// rel are the paths from the root, with /.
	rel   []string
	ready bool
	// cut means there were more than maxListedFiles.
	cut bool
}

// listFiles reads the files of the open folder for quick open.
func (a *app) listFiles() {
	ws := a.ws
	if ws == nil {
		a.files = fileList{}
		return
	}
	a.files = fileList{ws: ws}
	a.shell.async(func() func() {
		var rel []string
		cut := false
		workspace.Walk(ws.Root, ws.Gitignore, func() bool { return a.files.ws != ws }, func(p string) bool {
			if len(rel) == maxListedFiles {
				cut = true
				return true
			}
			rel = append(rel, ws.Rel(p))
			return false
		})
		return func() {
			if a.files.ws == ws {
				a.files = fileList{ws: ws, rel: rel, ready: true, cut: cut}
			}
		}
	})
}

// splitLineSuffix takes a line number off the end of a query, as in
// "main.go:12".
func splitLineSuffix(q string) (string, int) {
	if i := strings.LastIndexByte(q, ':'); i > 0 {
		if n, err := strconv.Atoi(q[i+1:]); err == nil && n > 0 {
			return q[:i], n
		}
	}
	return q, 0
}

// nameBonus puts a file whose name alone matches the query ahead of one
// that matches only with its folders: more than any score of the folders
// can add.
const nameBonus = 1 << 20

// quickOpenItems are the rows of quick open for query: the open files while
// it is empty, else the files that match, best first.
func (a *app) quickOpenItems(query string) ([]pickerItem, string) {
	ws := a.ws
	if ws == nil {
		return nil, T("quick.noFolder")
	}
	q, line := splitLineSuffix(strings.TrimSpace(query))
	open := func(rel string) func() {
		p := filepath.Join(ws.Root, filepath.FromSlash(rel))
		return func() {
			a.openFile(p, false)
			if t := a.currentTab(); t != nil && line > 0 {
				a.gotoLine(t, line-1, 0, 0)
			} else {
				a.focusEditor(t)
			}
		}
	}
	item := func(rel string, marks []int) pickerItem {
		dir := path.Dir(rel)
		if dir == "." {
			dir = ""
		}
		base := path.Base(rel)
		// The marks are in rel; the row shows the name, and the folder after.
		var bm []int
		off := len(rel) - len(base)
		for k := 0; k+1 < len(marks); k += 2 {
			if marks[k+1] > off {
				bm = append(bm, max(marks[k], off)-off, marks[k+1]-off)
			}
		}
		return pickerItem{label: base, marks: bm, detail: dir, run: open(rel)}
	}
	if q == "" {
		var rows []pickerItem
		for i := len(a.order) - 1; i >= 0 && len(rows) < pickerShown; i-- {
			rel := ws.Rel(a.tabs[a.order[i]].doc.path)
			if rel == "" || strings.HasPrefix(rel, "../") || slices.ContainsFunc(rows, func(r pickerItem) bool { return path.Join(r.detail, r.label) == rel }) {
				continue
			}
			rows = append(rows, item(rel, nil))
		}
		note := T("quick.hint")
		if !a.files.ready {
			note = T("quick.counting")
		}
		return rows, note
	}
	if !a.files.ready {
		return nil, T("quick.counting")
	}
	type hit struct {
		rel   string
		score int
		marks []int
	}
	// field.Fuzzy is the matcher of the command palette, so that both
	// pickers find and mark alike (PLAN §5 #17). A file whose name alone
	// matches comes before one that needs its folders to.
	var f field.Fuzzy
	f.SetQuery(q)
	var hits []hit
	for _, rel := range a.files.rel {
		off := strings.LastIndexByte(rel, '/') + 1
		if s, ok := f.Match(rel[off:]); ok {
			m := slices.Clone(f.Ranges())
			for k := range m {
				m[k] += off
			}
			hits = append(hits, hit{rel, s + nameBonus, m})
		} else if s, ok := f.Match(rel); ok {
			hits = append(hits, hit{rel, s, slices.Clone(f.Ranges())})
		}
	}
	slices.SortStableFunc(hits, func(x, y hit) int { return y.score - x.score })
	note := T("quick.count", len(hits))
	if len(hits) > pickerShown {
		note = T("quick.countOf", len(hits), pickerShown)
		hits = hits[:pickerShown]
	}
	if len(hits) == 0 {
		note = T("common.notFound")
	}
	if a.files.cut {
		note += T("quick.cut", maxListedFiles)
	}
	rows := make([]pickerItem, len(hits))
	for i, h := range hits {
		rows[i] = item(h.rel, h.marks)
	}
	return rows, note
}

// quickOpen opens the picker of the files of the folder.
func (a *app) quickOpen() {
	if !a.ctxOK {
		return
	}
	showPicker(a.ctx, picker{placeholder: T("quick.placeholder"), items: a.quickOpenItems})
}

// gotoLineItems are the rows of the picker of Ctrl+G for query.
func (a *app) gotoLineItems(query string) ([]pickerItem, string) {
	t := a.currentTab()
	if t == nil || !t.hasText() {
		return nil, T("goto.noTab")
	}
	n := t.doc.ctrl.LineCount()
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, T("goto.hint", n)
	}
	lineText, colText, _ := strings.Cut(q, ":")
	line, err := strconv.Atoi(strings.TrimSpace(lineText))
	if err != nil || line < 1 {
		return nil, T("goto.number")
	}
	line = min(line, n)
	col := 0
	if c, err := strconv.Atoi(strings.TrimSpace(colText)); err == nil && c > 0 {
		col = c - 1
	}
	label := T("goto.line", line)
	if col > 0 {
		label = T("goto.lineCol", line, col+1)
	}
	id := t.id
	return []pickerItem{{label: label, run: func() {
		if t := a.tabs[id]; t != nil {
			a.gotoLine(t, line-1, col, 0)
		}
	}}}, ""
}

// gotoLinePicker opens the picker of Ctrl+G.
func (a *app) gotoLinePicker() {
	if !a.ctxOK {
		return
	}
	showPicker(a.ctx, picker{placeholder: T("goto.placeholder"), items: a.gotoLineItems})
}

// hasText reports whether the tab has text to move in: not a picture or a
// binary file.
func (t *tab) hasText() bool {
	c := t.doc.content
	return t.doc.diff == nil && c.Err == nil && t.doc.image == nil && (!c.Binary || c.Forced)
}

// gotoLine shows line of tab t, from the col'th character for n characters
// selected, in its text, and gives it the keyboard: a tab that shows a
// drawing or rendered Markdown shows the source.
func (a *app) gotoLine(t *tab, line, col, n int) {
	if !t.hasText() {
		return
	}
	c := &t.doc.ctrl
	line = min(max(line, 0), c.LineCount()-1)
	s := c.Line(line)
	a.gotoRange(t, line, runeOffset(s, col), runeOffset(s, col+n))
	a.focusEditor(t)
}

// runeOffset is the byte offset of the n'th character of s, or its length.
func runeOffset(s string, n int) int {
	for i := range s {
		if n == 0 {
			return i
		}
		n--
	}
	return len(s)
}
