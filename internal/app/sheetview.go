package app

import (
	"math"
	"strconv"

	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/input"
	"github.com/bankai-tensei/gowaku/layout"
	"github.com/bankai-tensei/gowaku/paint"
	"github.com/bankai-tensei/gowaku/text"
	"github.com/bankai-tensei/gowaku/theme"
	"github.com/bankai-tensei/gowaku/widgets/basic"
	"github.com/bankai-tensei/gowaku/widgets/collection"
	"github.com/bankai-tensei/gowaku/widgets/control"
	"github.com/bankai-tensei/gowaku/widgets/editor"
	"github.com/bankai-tensei/gowaku/widgets/field"
	"github.com/bankai-tensei/gowaku/widgets/interact"
)

// The measures of a column of a table, past the text it holds.
const (
	// sheetCellPad is the room the table leaves at each side of a cell.
	sheetCellPad = theme.Space12
	// sheetSortRoom is the room of the arrow of a sorted column.
	sheetSortRoom = 20
	// sheetMinWidth and sheetMaxWidth bound the width a column starts
	// with: a long cell is cut, and the user drags the edge for more.
	sheetMinWidth = 56
	sheetMaxWidth = 360
	// sheetMeasureRows is how many rows the widths of the columns follow:
	// measuring all of a large file would take long.
	sheetMeasureRows = 500
)

// sheetRow is a row of a table as it shows: pos is where it shows, which
// gives its band, and idx is its index in the file.
type sheetRow struct{ pos, idx int }

// showsSheet reports whether the tab shows a CSV or TSV file as a table
// rather than its text.
func (t *tab) showsSheet() bool { return t.doc.sheet != nil && !t.source }

// sheetOrder returns the rows of the table of tab t in the order it sorts
// them, sorting them again when the file changed.
func (t *tab) sheetOrder() []int {
	s := t.doc.sheet
	if t.sorted != s {
		t.sorted = s
		t.order = nil
		if t.sort.Order != collection.SortNone {
			t.order = s.order(t.sort.Column-1, t.sort.Order == collection.SortDescending)
		}
	}
	return t.order
}

// sortSheet sorts the table of tab t as its header asked.
func (a *app) sortSheet(t *tab, s collection.TableSort) {
	t.sort, t.sorted = s, nil
	a.refresh()
}

// sheetView shows the CSV or TSV document of tab t as a table: the header
// stays at the top and the row numbers at the left, the rows are banded,
// numbers sit at the end of their cells, and a click on a header sorts.
// Double click or Enter on a cell edits it in place; on the row number it
// shows the row in the source.
func sheetView(th *theme.Theme, shaper *text.Shaper, a *app, t *tab) core.Widget {
	s := t.doc.sheet
	if len(s.header) == 0 {
		return message(th, T("sheet.empty"), T("sheet.emptyDetail"))
	}
	widths := sheetWidths(s, th, shaper)
	order := t.sheetOrder()
	body := th.Text.Body.WithColor(th.Color.Ink)
	band := th.Color.Fill.WithAlpha(th.Color.Fill.A * 0.6)
	cell := func(r sheetRow, content string, st text.Style, trailing bool) core.Widget {
		align := layout.AlignCenterLeft
		if trailing {
			align = layout.AlignCenterRight
		}
		st.MaxLines = 1
		label := basic.Align{Alignment: align, Child: basic.Text{Content: content, Style: st}}
		if r.pos%2 == 0 {
			return label
		}
		return basic.Stack{Fit: layout.StackExpand, Children: []core.Widget{
			control.CustomPaint{Painter: bandPainter{band}},
			label,
		}}
	}
	cols := make([]collection.TableColumn[sheetRow], 0, len(s.header)+1)
	numberStyle := th.Text.Body.WithColor(th.Color.Ink2)
	cols = append(cols, collection.TableColumn[sheetRow]{
		Title: "#", Width: widths[0], MinWidth: widths[0], Trailing: true, Sortable: true,
		Kind: collection.CellWidget,
		Text: func(r sheetRow) string { return strconv.Itoa(r.idx + 1) },
		Widget: func(r sheetRow) core.Widget {
			return cell(r, strconv.Itoa(r.idx+1), numberStyle, true)
		},
	})
	for j, title := range s.header {
		if title == "" {
			title = columnName(j)
		}
		numeric := s.numeric[j]
		cols = append(cols, collection.TableColumn[sheetRow]{
			Title: title, Width: widths[j+1], Trailing: numeric, Sortable: true,
			Kind: collection.CellWidget,
			Text: func(r sheetRow) string { return s.rows[r.idx][j] },
			Widget: func(r sheetRow) core.Widget {
				if e := t.edit; e != nil && e.idx == r.idx && e.col == j {
					return a.cellEditor(th, t, e, title, body)
				}
				w := cell(r, s.rows[r.idx][j], body, numeric)
				if k := t.edited; k != nil && k.idx == r.idx && k.col == j {
					// The keyboard stays on the cell just edited, for the arrows.
					return interact.Focus{Enabled: true, Autofocus: true, Child: w, OnFocusChange: func(f bool) {
						if !f && t.edited == k {
							t.edited = nil
						}
					}}
				}
				return w
			},
		})
	}
	id := t.id
	return collection.Table[sheetRow]{
		RowCount: len(s.rows),
		Row: func(i int) sheetRow {
			if order != nil {
				return sheetRow{i, order[i]}
			}
			return sheetRow{i, i}
		},
		Columns:    cols,
		Label:      T("sheet.label", t.doc.title()),
		Frozen:     1,
		Controller: &t.table,
		RowKey:     func(r sheetRow) core.Key { return r.idx },
		Sort:       t.sort,
		OnSort:     func(ns collection.TableSort) { a.sortSheet(t, ns) },
		OnActivate: func(i int) { a.activateSheet(id, i) },
	}
}

// sheetEdit is a cell of a table being edited: row idx of the file and
// column col, with the text in the editor.
type sheetEdit struct {
	idx, col int
	text     string
	input    field.InputController
}

// activateSheet answers a double click or Enter on row i of the table of
// tab id: the cell in focus opens to be edited, and the row number shows
// the row in the source. A file that cannot be saved edits nothing.
func (a *app) activateSheet(id string, i int) {
	t := a.tabs[id]
	if t == nil || !t.showsSheet() {
		return
	}
	_, col := t.table.FocusedCell()
	d := t.doc
	if col <= 0 || col > len(d.sheet.header) {
		a.showSheetRow(id, i)
		return
	}
	if !d.canEdit() || d.saving() {
		return
	}
	idx := i
	if order := t.sheetOrder(); order != nil {
		idx = order[i]
	}
	e := &sheetEdit{idx: idx, col: col - 1, text: d.sheet.rows[idx][col-1]}
	// The caret starts at the end, as in a spreadsheet.
	e.input.SetSelection(len(e.text), len(e.text))
	t.edit = e
	t.edited = nil
	a.refresh()
}

// cellEditor is the field that edits the cell of e in place: Enter keeps
// the text, and so does the keyboard leaving it; Escape drops it.
func (a *app) cellEditor(th *theme.Theme, t *tab, e *sheetEdit, title string, st text.Style) core.Widget {
	// The keys the field leaves come here before the table, which would take
	// the keyboard back on Escape and so keep the edit.
	return interact.Focus{
		OnKey: func(k *core.KeyEvent) bool {
			if k.Key != input.KeyEscape || k.Modifiers != 0 {
				return false
			}
			a.endCellEdit(t, e, false)
			return true
		},
		Child: basic.Stack{Fit: layout.StackExpand, Children: []core.Widget{
			control.CustomPaint{Painter: editPainter{fill: th.Color.Field, ring: th.Color.Accent}},
			basic.Align{Alignment: layout.AlignCenterLeft, Child: field.Input{
				Value: e.text, Controller: &e.input, Label: title, HideLabel: true, Bare: true, Autofocus: true, Style: st,
				OnChange: func(s string) { e.text = s },
				OnSubmit: func(s string) { e.text = s; a.endCellEdit(t, e, true) },
				Extras: field.Extras{OnFocus: func(focused bool) {
					if !focused {
						a.endCellEdit(t, e, true)
					}
				}},
			}},
		}},
	}
}

// endCellEdit closes the editor of e, writing its text into the field of
// the source when keep says so. Only that field changes, and Ctrl+Z in the
// source takes the change back.
func (a *app) endCellEdit(t *tab, e *sheetEdit, keep bool) {
	if t.edit != e {
		return
	}
	t.edit = nil
	// The cell takes the keyboard back once the field is gone: a cell that
	// took it while the field had it would lose it with the field.
	cell := &sheetCell{e.idx, e.col}
	a.shell.post(func() {
		if t.edit == nil {
			t.edited = cell
			a.refresh()
		}
	})
	d := t.doc
	s := d.sheet
	if keep && s != nil && d.canEdit() && !d.saving() && e.idx < len(s.rows) && e.text != s.rows[e.idx][e.col] {
		start, end, text := s.fieldEdit(e.idx, e.col, e.text)
		d.ctrl.Replace(editor.TextRange{Start: d.ctrl.PositionAt(start), End: d.ctrl.PositionAt(end)}, text)
		a.edited(d)
		a.refill(d)
		return
	}
	a.refresh()
}

// sheetCell is a cell of a table: row idx of the file and column col.
type sheetCell struct{ idx, col int }

// showSheetRow shows row i of the table of tab id, as it shows, in the
// source.
func (a *app) showSheetRow(id string, i int) {
	t := a.tabs[id]
	if t == nil || !t.showsSheet() {
		return
	}
	s := t.doc.sheet
	idx := i
	if order := t.sheetOrder(); order != nil {
		idx = order[i]
	}
	if idx < 0 || idx >= len(s.lines) {
		return
	}
	a.gotoRange(t, s.lines[idx], 0, 0)
}

// sheetWidths returns the widths the columns of s start with, the column
// of row numbers first: as wide as the widest of the first rows, and the
// header, within bounds. They are measured again when the font changes.
func sheetWidths(s *sheet, th *theme.Theme, shaper *text.Shaper) []float32 {
	body := th.Text.Body
	head := th.Text.Caption
	head.Weight = th.Text.Headline.Weight
	key := sheetFont{shaper, body.Family, head.Family, body.Size, head.Size, head.Weight}
	if s.widths != nil && s.widthsFor == key {
		return s.widths
	}
	measure := func(str string, st text.Style) float32 {
		if str == "" || shaper == nil {
			return 0
		}
		return shaper.Shape(str, st).Layout(float32(math.Inf(1))).Size().W
	}
	pad := float32(2*sheetCellPad + sheetSortRoom)
	widths := make([]float32, len(s.header)+1)
	widths[0] = max(measure(strconv.Itoa(len(s.rows)), body), measure("#", head)) + pad
	for j, title := range s.header {
		if title == "" {
			title = columnName(j)
		}
		w := measure(title, head)
		for _, row := range s.rows[:min(len(s.rows), sheetMeasureRows)] {
			w = max(w, measure(row[j], body))
			if w >= sheetMaxWidth {
				break
			}
		}
		widths[j+1] = min(max(w+pad, sheetMinWidth), sheetMaxWidth)
	}
	s.widths, s.widthsFor = widths, key
	return widths
}

// sheetFont is what the widths of the columns of a table depend on.
type sheetFont struct {
	shaper           *text.Shaper
	body, head       string
	bodySize, headSz float32
	weight           uint16
}

// editPainter grounds a cell being edited, past the room the table leaves
// at its sides, and rings it in the accent.
type editPainter struct{ fill, ring geom.Color }

// Paint implements layout.Painter.
func (p editPainter) Paint(cv *paint.Canvas, sz geom.Size) {
	box := geom.Rect{X: -sheetCellPad + 1, Y: -1, W: sz.W + 2*sheetCellPad - 2, H: sz.H + 2}
	cv.DrawRRect(geom.RRectAll(box, 2), paint.Paint{Color: p.fill})
	cv.DrawRRect(geom.RRectAll(box, 2), paint.Paint{Stroke: &paint.Stroke{Width: 2, Color: p.ring}})
}

// bandPainter fills the whole cell around it, past the room the table
// leaves at its sides, to band a row.
type bandPainter struct{ color geom.Color }

// Paint implements layout.Painter. The table cuts what a cell paints to
// the cell.
func (p bandPainter) Paint(cv *paint.Canvas, sz geom.Size) {
	box := geom.Rect{X: -sheetCellPad, Y: -sheetCellPad, W: sz.W + 2*sheetCellPad, H: sz.H + 2*sheetCellPad}
	cv.DrawRRect(geom.RRectAll(box, 0), paint.Paint{Color: p.color})
}
