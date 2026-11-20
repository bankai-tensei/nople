package app

import (
	"fmt"
	"unicode/utf8"

	"github.com/bankai-tensei/nople/internal/textfile"

	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/gowaku/widgets/editor"
	"github.com/bankai-tensei/gowaku/widgets/nav"
)

// statusView is the status bar at the bottom of the window. It builds by
// itself, apart from the window, as the caret moves: the view of each
// document tells the app through View.OnSelectionChange.
type statusView struct {
	core.StatefulBase
	app *app
}

type statusState struct {
	core.StateOf[statusView]
}

// CreateState implements core.StatefulWidget.
func (statusView) CreateState() core.State { return &statusState{} }

// InitState implements core.State.
func (s *statusState) InitState() { s.Widget().app.status = s }

// Dispose implements core.State.
func (s *statusState) Dispose() {
	if a := s.Widget().app; a.status == s {
		a.status = nil
	}
}

// Build implements core.State.
func (s *statusState) Build(core.BuildContext) core.Widget {
	a := s.Widget().app
	var leading, trailing []nav.StatusItem
	if a.ws != nil {
		leading = append(leading, nav.StatusItem{Text: a.ws.Name(), Tooltip: a.ws.Root, Priority: 1})
	}
	if r := a.repo; r != nil && r.Branch != "" {
		leading = append(leading, nav.StatusItem{
			Text: T("scm.branch", r.Branch), Tooltip: T("status.branchTip", len(r.Unstaged), len(r.Staged)),
			OnPress: func() { a.readRepo(); a.showPanel(scmID) }, Priority: 2,
		})
	}
	t := a.currentTab()
	if t != nil {
		d := t.doc
		c := d.content
		switch {
		case d.diff != nil:
			if dd := d.diff; dd.ready {
				trailing = append(trailing,
					nav.StatusItem{Text: fmt.Sprintf("+%d −%d", dd.added, dd.gone), Tooltip: T("status.diffTip"), Priority: 3},
					nav.StatusItem{Text: dd.leftName + " ↔ " + dd.rightName, Priority: 2},
					nav.StatusItem{Text: changeName(dd.change.Code), Priority: 1},
				)
			}
		case c.Err != nil:
		case d.image != nil || t.showsDrawing():
			sz := pictureSize(d)
			trailing = append(trailing,
				nav.StatusItem{Text: fmt.Sprintf("%.0f × %.0f", sz.W, sz.H), Tooltip: T("status.imageSizeTip"), Priority: 3},
				nav.StatusItem{Text: sizeText(c.Size), Priority: 1},
				nav.StatusItem{Text: fmt.Sprintf("%.0f%%", t.scale*100), Tooltip: T("status.zoomTip"), Priority: 2},
			)
		case t.showsMarkdown():
			eol := "LF"
			if c.CRLF {
				eol = "CRLF"
			}
			trailing = append(trailing,
				nav.StatusItem{Text: T("status.headings", len(d.headings)), Priority: 2},
				nav.StatusItem{Text: encodingLabel(c.Encoding), Priority: 2},
				nav.StatusItem{Text: eol, Priority: 1},
				nav.StatusItem{Text: T("status.mdPreview"), Tooltip: T("status.mdTip"), Priority: 2},
			)
		case t.showsSheet():
			eol := "LF"
			if c.CRLF {
				eol = "CRLF"
			}
			trailing = append(trailing,
				nav.StatusItem{Text: T("status.sheetSize", len(d.sheet.rows), len(d.sheet.header)), Priority: 3},
				nav.StatusItem{Text: encodingLabel(c.Encoding), Priority: 2},
				nav.StatusItem{Text: eol, Priority: 1},
				nav.StatusItem{Text: T("status.sheetPreview"), Tooltip: T("status.sheetTip"), Priority: 2},
			)
		case !c.Binary || c.Forced:
			trailing = append(trailing, nav.StatusItem{Text: caretText(&d.ctrl, t.view.Selection()), Priority: 3})
			if p, loading := d.loadProgress(); loading {
				trailing = append(trailing, nav.StatusItem{Text: T("status.loading", int(p*100)), Priority: 1})
			}
			if p, ok := d.saveProgress(); ok {
				trailing = append(trailing, nav.StatusItem{Text: T("status.saving", p), Priority: 1})
			}
			trailing = append(trailing, nav.StatusItem{Text: encodingLabel(c.Encoding), Priority: 2})
			eol := "LF"
			if c.CRLF {
				eol = "CRLF"
			}
			trailing = append(trailing,
				nav.StatusItem{Text: eol, Priority: 1},
				nav.StatusItem{Text: languageOf(d.path), Priority: 2},
			)
			switch {
			case !d.canEdit():
				trailing = append(trailing, nav.StatusItem{Text: T("status.readOnly"), Priority: 1})
			case d.dirty && !d.saving():
				// While it saves, "Saving 42%" says it all.
				trailing = append(trailing, nav.StatusItem{Text: T("status.unsaved"), Tooltip: autosaveTip(a), Priority: 2})
			}
		}
	}
	return nav.StatusBar{Leading: leading, Trailing: trailing}
}

// caretText says where the caret is, and how much is selected.
func caretText(c *editor.Controller, sel editor.TextRange) string {
	line := sel.End.Line
	col := 1
	if line < c.LineCount() {
		s := c.Line(line)
		col = utf8.RuneCountInString(s[:min(sel.End.Column, len(s))]) + 1
	}
	t := T("status.caret", line+1, col)
	if !sel.Empty() {
		n := utf8.RuneCountInString(c.TextIn(sel.Ordered()))
		t += T("status.selected", n)
	}
	return t
}

// encodingLabel is the name of an encoding as the status bar shows it.
func encodingLabel(enc string) string {
	if enc == textfile.UTF8BOM {
		return T("enc.utf8bom")
	}
	return enc
}
