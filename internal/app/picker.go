package app

import (
	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/input"
	"github.com/bankai-tensei/gowaku/layout"
	"github.com/bankai-tensei/gowaku/semantics"
	"github.com/bankai-tensei/gowaku/theme"
	"github.com/bankai-tensei/gowaku/widgets/basic"
	"github.com/bankai-tensei/gowaku/widgets/field"
	"github.com/bankai-tensei/gowaku/widgets/interact"
	"github.com/bankai-tensei/gowaku/widgets/overlay"
)

// Measures of a picker.
const (
	pickerWidth = 600
	pickerRows  = 10
	pickerRow   = 30
)

// pickerItem is one row of a picker.
type pickerItem struct {
	label string
	// marks are the byte ranges of label that matched, in pairs.
	marks []int
	// detail is shown dimmed after the label, such as the folder of a file.
	detail string
	run    func()
}

// picker is a dialog with a field and the rows that match what is typed in
// it, as Ctrl+P and Ctrl+G open: Up and Down choose, Enter runs, Escape
// closes. Unlike the command palette it asks items for the rows of each query,
// so that it can search thousands of files and show only the best.
type picker struct {
	core.StatefulBase
	placeholder string
	// items returns the rows for query, best first, and a line that tells
	// about them, such as how many there are; empty shows nothing.
	items   func(query string) (rows []pickerItem, note string)
	dismiss *func()
}

// showPicker opens p as a dialog over the window of ctx.
func showPicker(ctx core.BuildContext, p picker) {
	var dismiss func()
	p.dismiss = &dismiss
	overlay.ShowDialog(ctx, func(d func()) core.Widget {
		dismiss = d
		return overlay.Dialog{Glass: true, Width: pickerWidth, Content: p}
	})
}

type pickerState struct {
	core.StateOf[picker]
	query    string
	selected int
	field    field.InputController
	rows     []pickerItem
	note     string
}

// CreateState implements core.StatefulWidget.
func (picker) CreateState() core.State { return &pickerState{} }

// key moves the selection with the arrows.
func (s *pickerState) key(e *core.KeyEvent) bool {
	if e.Modifiers != 0 {
		return false
	}
	switch e.Key {
	case input.KeyDown:
		s.SetState(func() { s.selected = min(s.selected+1, max(len(s.rows)-1, 0)) })
	case input.KeyUp:
		s.SetState(func() { s.selected = max(s.selected-1, 0) })
	default:
		return false
	}
	return true
}

// run closes the picker and runs row i.
func (s *pickerState) run(i int) {
	if i < 0 || i >= len(s.rows) || s.rows[i].run == nil {
		return
	}
	r := s.rows[i].run
	if d := *s.Widget().dismiss; d != nil {
		d()
	}
	r()
}

// Build implements core.State.
func (s *pickerState) Build(ctx core.BuildContext) core.Widget {
	th := basic.ThemeOf(ctx)
	w := s.Widget()
	s.rows, s.note = w.items(s.query)
	s.selected = min(s.selected, max(len(s.rows)-1, 0))
	first := max(0, s.selected-pickerRows+1)
	list := make([]core.Widget, 0, pickerRows+1)
	for i := first; i < len(s.rows) && i < first+pickerRows; i++ {
		list = append(list, s.row(th, i))
	}
	if s.note != "" {
		list = append(list, basic.Padding{Insets: geom.Symmetric(theme.Space10, theme.Space4), Child: basic.Text{
			Content: s.note, Style: th.Text.Caption.WithColor(th.Color.Ink2),
		}})
	}
	return interact.Focus{
		OnKey: s.key,
		Child: basic.Column{Size: layout.MainMin, Cross: layout.CrossStretch, Spacing: theme.Space8, Children: []core.Widget{
			field.Input{
				Value: s.query, Placeholder: w.placeholder, Controller: &s.field, Autofocus: true,
				OnChange: func(v string) { s.SetState(func() { s.query, s.selected = v, 0 }) },
				OnSubmit: func(string) { s.run(s.selected) },
			},
			interact.Semantics{Role: semantics.RoleList, Child: basic.Column{
				Size: layout.MainMin, Cross: layout.CrossStretch, Spacing: theme.Space2, Children: list,
			}},
		}},
	}
}

// row draws row i, lit when it is the selected one.
func (s *pickerState) row(th *theme.Theme, i int) core.Widget {
	it := s.rows[i]
	selected := i == s.selected
	if selected {
		// The text of the lit row reads on its tint.
		th = th.OnTint()
	}
	body := th.Text.Body.WithColor(th.Color.Ink)
	spans := field.FuzzySpans(nil, it.label, it.marks, body, field.FuzzyHighlight(body, th.Color.AccentInk))
	var state semantics.State
	if selected {
		state |= semantics.Selected
	}
	return interact.Pressable{
		Label: it.label, Value: it.detail, Role: semantics.RoleListItem, State: state, SkipTab: true,
		OnPress: func() { s.run(i) },
		Build: func(ps interact.PressState) core.Widget {
			var ground geom.Color
			switch {
			case selected:
				ground = th.Color.AccentTint
			case ps.Hovered:
				ground = th.Color.Fill
			}
			parts := []core.Widget{basic.RichText{Spans: spans, Style: body}}
			if it.detail != "" {
				parts = append(parts, basic.Expanded{Child: basic.Text{
					Content: it.detail, Style: th.Text.Caption.WithColor(th.Color.Ink2),
				}})
			}
			return basic.Container{
				Height: pickerRow, Color: ground, Radius: th.Radius.Control, Padding: geom.Symmetric(theme.Space10, 0),
				Child: basic.Row{Cross: layout.CrossCenter, Spacing: theme.Space12, Children: parts},
			}
		},
	}
}
