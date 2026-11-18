package app

import (
	"errors"
	"github.com/bankai-tensei/nople/internal/diff"
	"github.com/bankai-tensei/nople/internal/git"
	"github.com/bankai-tensei/nople/internal/syntax"
	"github.com/bankai-tensei/nople/internal/textfile"
	"path/filepath"

	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/layout"
	"github.com/bankai-tensei/gowaku/paint"
	"github.com/bankai-tensei/gowaku/text"
	"github.com/bankai-tensei/gowaku/theme"
	"github.com/bankai-tensei/gowaku/widgets/basic"
	"github.com/bankai-tensei/gowaku/widgets/collection"
	"github.com/bankai-tensei/gowaku/widgets/control"
	"github.com/bankai-tensei/gowaku/widgets/editor"
)

// readRepo reads the Git state of the open folder off the UI goroutine, and
// then the change marks of the open files.
func (a *app) readRepo() {
	ws := a.ws
	if ws == nil {
		a.repo, a.repoErr = nil, nil
		return
	}
	a.shell.async(func() func() {
		r, err := git.Read(ws.Root)
		return func() {
			if a.ws != ws {
				return
			}
			a.repo, a.repoErr = r, err
			for _, id := range a.order {
				a.startMarks(a.tabs[id].doc)
			}
			a.refresh()
		}
	})
}

// Kinds of a line in the change marks of an editor.
const (
	markNone byte = iota
	markAdded
	markModified
)

// marks are the lines of a file that differ from the index, for the bars in
// the gutter of its editor.
type marks struct {
	line []byte
	// deletedBefore marks a line before which lines were deleted, and
	// deletedAtEnd lines deleted after the last.
	deletedBefore []bool
	deletedAtEnd  bool
}

// startMarks finds the lines of d that differ from the index off the UI
// goroutine. A file that git does not track has none.
func (a *app) startMarks(d *document) {
	r := a.repo
	c := d.content
	if d.big != nil && r != nil {
		// A large file shows no marks, but says so only when git tracks it.
		path := d.path
		a.shell.async(func() func() {
			tracked := r.Tracked(path)
			return func() {
				if a.repo == r && d.tracked != tracked {
					d.tracked = tracked
					a.refresh()
				}
			}
		})
	}
	if r == nil || d.diff != nil || d.big != nil || d.wantBig || c.Err != nil || c.Binary || d.image != nil {
		d.marks = nil
		return
	}
	if _, untracked := r.ByPath[d.path]; untracked && r.ByPath[d.path] == 'U' {
		d.marks = nil
		return
	}
	text := d.ctrl.Text()
	a.shell.async(func() func() {
		old, ok := r.Show("", d.path)
		var m *marks
		if ok {
			m = changeMarks(diff.Split(old), diff.Split(text))
		}
		return func() {
			if a.repo == r {
				d.marks = m
				d.ctrl.Redecorate()
			}
		}
	})
}

// changeMarks marks the lines of b by how they differ from a.
func changeMarks(a, b []string) *marks {
	m := &marks{line: make([]byte, len(b)), deletedBefore: make([]bool, len(b))}
	for _, h := range diff.Lines(a, b) {
		switch {
		case h.NewLen == 0 && h.NewStart < len(b):
			m.deletedBefore[h.NewStart] = true
		case h.NewLen == 0:
			m.deletedAtEnd = true
		default:
			k := markAdded
			if h.OldLen > 0 {
				k = markModified
			}
			for i := h.NewStart; i < h.NewStart+h.NewLen; i++ {
				m.line[i] = k
			}
		}
	}
	return m
}

// changeColors are the colors of git's marks in a theme: added, modified,
// deleted.
func changeColors(th *theme.Theme) (added, modified, deleted geom.Color) {
	p := paletteOf(th)
	return p[syntax.KindInserted].Color, p[syntax.KindNumber].Color, p[syntax.KindDeleted].Color
}

// statusColor is the color of a file of the code of git.
func statusColor(th *theme.Theme, code byte) geom.Color {
	added, modified, deleted := changeColors(th)
	switch code {
	case 'A', 'U', 'C':
		return added
	case 'D', '!':
		return deleted
	}
	return modified
}

// gutterWidth is the room of the change bars beside the line numbers.
const gutterWidth = 4

// markGutter draws the change bars of d in the gutter of its editor.
func markGutter(th *theme.Theme, d *document) func(cv *paint.Canvas, l editor.GutterLine) {
	added, modified, deleted := changeColors(th)
	return func(cv *paint.Canvas, l editor.GutterLine) {
		m := d.marks
		if m == nil || l.Line >= len(m.line) {
			return
		}
		r := geom.Rect{X: l.Rect.X + 1, Y: l.Rect.Y, W: 3, H: l.Rect.H}
		switch m.line[l.Line] {
		case markAdded:
			cv.DrawRRect(geom.RRectAll(r, 0), paint.Paint{Color: added})
		case markModified:
			cv.DrawRRect(geom.RRectAll(r, 0), paint.Paint{Color: modified})
		}
		if m.deletedBefore[l.Line] || m.deletedAtEnd && l.Line == len(m.line)-1 {
			y := l.Rect.Y
			if !m.deletedBefore[l.Line] {
				y += l.Rect.H
			}
			cv.DrawRRect(geom.RRectAll(geom.Rect{X: l.Rect.X, Y: y - 1.5, W: gutterWidth, H: 3}, 0), paint.Paint{Color: deleted})
		}
	}
}

// diffDoc is the two sides of a change of a file, which a diff tab shows.
type diffDoc struct {
	change git.Change
	// leftName and rightName say what each side is, such as HEAD.
	leftName, rightName string
	left, right         editor.Controller
	lv, rv              editor.View
	hunks               []diff.Hunk
	lm                  diff.LineMap
	// leftKind and rightKind mark the deleted and inserted lines; their
	// intra are the parts of changed lines that differ.
	leftKind, rightKind   []bool
	leftIntra, rightIntra map[int][2]int
	// rightGap marks the lines of the right before which lines were deleted.
	rightGap    map[int]bool
	lhl, rhl    *syntax.Highlight
	ready       bool
	err         error
	added, gone int
}

// diffKey names the diff of change c, so that it opens in one tab.
func diffKey(c git.Change) string {
	if c.Staged {
		return "staged\t" + c.Path
	}
	return "unstaged\t" + c.Path
}

// openDiff shows the change c in a diff tab.
func (a *app) openDiff(c git.Change, preview bool) {
	key := diffKey(c)
	for _, id := range a.order {
		if dd := a.tabs[id].doc.diff; dd != nil && diffKey(dd.change) == key {
			if !preview {
				a.keep(id)
			}
			a.activate(id)
			return
		}
	}
	dd := &diffDoc{change: c}
	d := &document{path: c.Path, diff: dd}
	switch {
	case c.Staged:
		dd.leftName, dd.rightName = "HEAD", T("scm.index")
	default:
		dd.leftName, dd.rightName = T("scm.index"), T("scm.workTree")
	}
	a.loadDiff(dd)
	a.place(d, preview)
}

// loadDiff reads the two sides of dd off the UI goroutine and diffs them.
func (a *app) loadDiff(dd *diffDoc) {
	r := a.repo
	if r == nil {
		dd.err = errors.New(T("scm.notRepo"))
		return
	}
	c := dd.change
	a.shell.async(func() func() {
		var old, cur string
		switch {
		case c.Staged:
			from := c.Path
			if c.Orig != "" {
				from = c.Orig
			}
			if c.Code != 'A' {
				old, _ = r.Show("HEAD", from)
			}
			if c.Code != 'D' {
				cur, _ = r.Show("", c.Path)
			}
		default:
			if c.Code != 'U' {
				old, _ = r.Show("", c.Path)
			}
			if c.Code != 'D' {
				content := textfile.Read(c.Path, false)
				if content.Binary {
					return func() { dd.err = errors.New(T("common.binary")) }
				}
				cur = content.Text
			}
		}
		var oc, cc textfile.Content
		textfile.DecodeText([]byte(old), &oc)
		if c.Staged || c.Code == 'D' {
			textfile.DecodeText([]byte(cur), &cc)
		} else {
			cc.Text = cur
		}
		ol, nl := diff.Split(oc.Text), diff.Split(cc.Text)
		hs := diff.Lines(ol, nl)
		var lhl, rhl *syntax.Highlight
		if l := syntax.LexerOf(c.Path, cc.Text); l != nil && len(oc.Text)+len(cc.Text) <= syntax.MaxSize {
			lhl, rhl = syntax.Lex(l, oc.Text), syntax.Lex(l, cc.Text)
		}
		return func() {
			dd.left.SetText(oc.Text)
			dd.right.SetText(cc.Text)
			dd.setHunks(ol, nl, hs)
			dd.lhl, dd.rhl = lhl, rhl
			dd.ready = true
			a.refresh()
		}
	})
}

// setHunks marks the lines of the two sides by the hunks between them.
func (dd *diffDoc) setHunks(ol, nl []string, hs []diff.Hunk) {
	dd.hunks, dd.lm = hs, diff.LineMap{Hunks: hs}
	dd.leftKind, dd.rightKind = make([]bool, len(ol)), make([]bool, len(nl))
	dd.leftIntra, dd.rightIntra, dd.rightGap = map[int][2]int{}, map[int][2]int{}, map[int]bool{}
	dd.added, dd.gone = 0, 0
	for _, h := range hs {
		for i := range h.OldLen {
			dd.leftKind[h.OldStart+i] = true
		}
		for i := range h.NewLen {
			dd.rightKind[h.NewStart+i] = true
		}
		if h.NewLen == 0 {
			dd.rightGap[h.NewStart] = true
		}
		dd.added += h.NewLen
		dd.gone += h.OldLen
		// Lines changed one for one show what in them changed.
		for i := range min(h.OldLen, h.NewLen) {
			a, b := ol[h.OldStart+i], nl[h.NewStart+i]
			if as, ae, bs, be, ok := diff.Intraline(a, b); ok {
				dd.leftIntra[h.OldStart+i] = [2]int{as, ae}
				dd.rightIntra[h.NewStart+i] = [2]int{bs, be}
			}
		}
	}
}

// diffTitle is the label of the tab of a diff.
func diffTitle(d *document) string {
	name := filepath.Base(d.path)
	if d.diff.change.Staged {
		return T("scm.tabIndex", name)
	}
	return T("scm.tabWorkTree", name)
}

// diffView shows the two sides of the diff of tab t side by side, scrolling
// together.
func diffView(th *theme.Theme, a *app, t *tab) core.Widget {
	dd := t.doc.diff
	switch {
	case dd.err != nil:
		return message(th, T("scm.diffFailed"), dd.err.Error())
	case !dd.ready:
		return message(th, T("scm.diffLoading"), "")
	}
	added, _, deleted := changeColors(th)
	tint := func(c geom.Color, alpha float32) geom.Color { c.A = alpha; return c }
	lineA, wordA := float32(0.16), float32(0.38)
	if th.Brightness == theme.BrightnessDark {
		lineA, wordA = 0.22, 0.45
	}
	p := paletteOf(th)
	side := func(ctl *editor.Controller, view *editor.View, hl *syntax.Highlight, kind []bool, intra map[int][2]int, c geom.Color, gap map[int]bool) core.Widget {
		return editor.Editor{
			Controller: ctl, View: view, ReadOnly: true, LineNumbers: true, Wrap: false,
			Style:       text.Style{Family: a.settings.FontFamily, Size: a.fontSize},
			GutterWidth: gutterWidth,
			Gutter: func(cv *paint.Canvas, l editor.GutterLine) {
				if l.Line < len(kind) && kind[l.Line] {
					cv.DrawRRect(geom.RRectAll(geom.Rect{X: l.Rect.X + 1, Y: l.Rect.Y, W: 3, H: l.Rect.H}, 0), paint.Paint{Color: c})
				}
				if gap[l.Line] {
					cv.DrawRRect(geom.RRectAll(geom.Rect{X: l.Rect.X, Y: l.Rect.Y - 1.5, W: gutterWidth, H: 3}, 0), paint.Paint{Color: deleted})
				}
			},
			// The added and deleted lines are tinted across the whole width,
			// empty ones included (PLAN §5 #18); what changed in a line
			// shows darker on top.
			LineStyle: func(i int, s string) editor.LineStyle {
				if i < len(kind) && kind[i] {
					return editor.LineStyle{Background: tint(c, lineA)}
				}
				return editor.LineStyle{}
			},
			Decorations: func(i int, s string, dst []editor.Decoration) []editor.Decoration {
				if i < len(kind) && kind[i] {
					if r, ok := intra[i]; ok && r[1] > r[0] && r[1] <= len(s) {
						dst = append(dst, editor.Decoration{Start: r[0], End: r[1], Background: tint(c, wordA)})
					}
				}
				return decorate(hl, p, i, s, dst)
			},
		}
	}
	header := func(name string) core.Widget {
		return basic.Padding{Insets: geom.Symmetric(theme.Space12, theme.Space4), Child: basic.Text{
			Content: name, Style: th.Text.Caption.WithColor(th.Color.Ink2),
		}}
	}
	pane := func(name string, w core.Widget) core.Widget {
		return basic.Expanded{Child: basic.Column{Cross: layout.CrossStretch, Children: []core.Widget{
			header(name), basic.Expanded{Child: w},
		}}}
	}
	return basic.Row{Cross: layout.CrossStretch, Children: []core.Widget{
		pane(dd.leftName, side(&dd.left, &dd.lv, dd.lhl, dd.leftKind, dd.leftIntra, deleted, nil)),
		basic.Container{Width: 1, Color: th.Color.Hair},
		pane(dd.rightName, side(&dd.right, &dd.rv, dd.rhl, dd.rightKind, dd.rightIntra, added, dd.rightGap)),
	}}
}

// linkScroll makes the two editors of dd scroll together: each, when the
// user scrolls it, moves the other to the line beside its own. SetScroll does
// not run OnScroll, so the two do not chase each other.
func (dd *diffDoc) linkScroll() {
	follow := func(to *editor.View, mapLine func(float64) float64) func(editor.ScrollPosition) {
		return func(p editor.ScrollPosition) {
			l := mapLine(float64(p.Line))
			to.SetScroll(editor.ScrollPosition{Line: int(l), Offset: p.Offset, X: p.X})
		}
	}
	dd.lv.OnScroll = follow(&dd.rv, dd.toNew)
	dd.rv.OnScroll = follow(&dd.lv, dd.toOld)
}

func (dd *diffDoc) toNew(l float64) float64 { return dd.lm.ToNew(l) }
func (dd *diffDoc) toOld(l float64) float64 { return dd.lm.ToOld(l) }

// scmView is the panel of the changes of the repository.
type scmView struct {
	core.StatelessBase
	app *app
}

// Keys of the groups of the tree of changes.
const (
	stagedGroup   = "\x00staged"
	unstagedGroup = "\x00unstaged"
)

// Build implements core.StatelessWidget.
func (v scmView) Build(ctx core.BuildContext) core.Widget {
	a := v.app
	th := basic.ThemeOf(ctx)
	switch {
	case a.ws == nil:
		return placeholder(ctx, T("scm.noFolder"))
	case a.repoErr != nil:
		return placeholder(ctx, T("scm.unreadable", repoErrText(a.repoErr)))
	case a.repo == nil:
		return placeholder(ctx, T("scm.folderNotRepo"))
	}
	r := a.repo
	byKey := map[string]git.Change{}
	keys := func(cs []git.Change, prefix string) []string {
		ks := make([]string, len(cs))
		for i, c := range cs {
			ks[i] = prefix + c.Path
			byKey[ks[i]] = c
		}
		return ks
	}
	staged, unstaged := keys(r.Staged, "s\t"), keys(r.Unstaged, "u\t")
	var roots []string
	if len(staged) > 0 {
		roots = append(roots, stagedGroup)
	}
	if len(unstaged) > 0 {
		roots = append(roots, unstagedGroup)
	}
	head := basic.Padding{Insets: geom.Symmetric(theme.Space12, theme.Space8), Child: basic.Row{
		Cross: layout.CrossCenter, Spacing: theme.Space8, Children: []core.Widget{
			basic.Expanded{Child: basic.Text{Content: T("scm.branch", r.Branch), Style: th.Text.Section.WithColor(th.Color.Ink2)}},
			control.Button{Label: T("scm.refresh"), OnPress: a.readRepo},
		},
	}}
	var body core.Widget
	if len(roots) == 0 {
		body = basic.Padding{Insets: geom.All(theme.Space16), Child: basic.Text{
			Content: T("scm.clean"), Style: th.Text.Caption.WithColor(th.Color.Ink2),
		}}
	} else {
		for _, g := range roots {
			a.scmTree.Expand(g)
		}
		body = collection.Tree[string]{
			Roots: roots,
			ChildrenOf: func(k string) []string {
				switch k {
				case stagedGroup:
					return staged
				case unstagedGroup:
					return unstaged
				}
				return nil
			},
			HasChildren: func(k string) bool { return k == stagedGroup || k == unstagedGroup },
			LabelOf: func(k string) string {
				switch k {
				case stagedGroup:
					return T("scm.staged", len(staged))
				case unstagedGroup:
					return T("scm.changes", len(unstaged))
				}
				c := byKey[k]
				label := filepath.Base(c.Path)
				if dir := filepath.ToSlash(filepath.Dir(r.Rel(c.Path))); dir != "." {
					label += " — " + dir
				}
				return label
			},
			IconOf: func(k string) core.Widget {
				c, ok := byKey[k]
				if !ok {
					return nil
				}
				return basic.SizedBox{Width: 16, Child: basic.Text{
					Content: string(c.Code), Style: th.Text.Caption.WithColor(statusColor(th, c.Code)).WithWeight(700),
				}}
			},
			Label:      T("scm.treeLabel"),
			Controller: &a.scmTree,
			OnSelectionChange: func(sel []string) {
				if len(sel) == 1 {
					if c, ok := byKey[sel[0]]; ok {
						a.openDiff(c, true)
					}
				}
			},
			OnActivate: func(k string) {
				if c, ok := byKey[k]; ok {
					a.openDiff(c, false)
					a.dock.HideFlyout()
					return
				}
				a.scmTree.SetExpanded(k, !a.scmTree.Expanded(k))
			},
		}
	}
	return basic.Container{Color: th.Color.Sidebar, Child: basic.Column{Cross: layout.CrossStretch, Children: []core.Widget{
		head, basic.Expanded{Child: body},
	}}}
}

// changeName is the name of a code of git in the language of the UI.
func changeName(code byte) string {
	switch code {
	case 'M':
		return T("git.modified")
	case 'A':
		return T("git.added")
	case 'D':
		return T("git.deleted")
	case 'R':
		return T("git.renamed")
	case 'C':
		return T("git.copied")
	case 'T':
		return T("git.typeChanged")
	case 'U':
		return T("git.untracked")
	case '!':
		return T("git.conflict")
	}
	return string(code)
}

// repoErrText says why git could not read the folder.
func repoErrText(err error) string {
	if errors.Is(err, git.ErrNoGit) {
		return T("git.notFound")
	}
	return err.Error()
}
