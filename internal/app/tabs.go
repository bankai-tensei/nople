package app

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/bankai-tensei/nople/internal/config"

	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/markdown"
	"github.com/bankai-tensei/gowaku/widgets/collection"
	"github.com/bankai-tensei/gowaku/widgets/editor"
	"github.com/bankai-tensei/gowaku/widgets/interact"
	"github.com/bankai-tensei/gowaku/widgets/nav"
)

// tab is one tab of the dock that shows a document. It keeps what is its own
// as the user switches between tabs: the selection and scroll position of its
// editor, the pan and zoom of its picture.
type tab struct {
	// id is the ID of its panel in the dock.
	id  string
	doc *document
	// preview marks the tab that a single click in the explorer opened, which
	// the next such click reuses. Opening it on purpose keeps it.
	preview bool

	view editor.View
	// md scrolls the rendered Markdown to a heading, for the outline.
	md     markdown.Controller
	viewer interact.ViewerController
	// scale is the zoom of the picture, as the viewer last told.
	scale float32
	// source shows the text of a file that shows drawn, such as an SVG.
	// Each tab keeps its own, so a split can show both.
	source bool
	// table holds the selection and the columns of a CSV file shown as a
	// table; sort is how it is sorted, and order its rows in that order,
	// sorted from the sheet sorted.
	table  collection.TableController
	sort   collection.TableSort
	order  []int
	sorted *sheet
	// edit is the cell of the table being edited, and edited the one that
	// was, which keeps the keyboard until it leaves.
	edit   *sheetEdit
	edited *sheetCell
	// noticeClosed means the user closed the notice above the text.
	noticeClosed bool
}

// showsDrawing reports whether the tab shows an SVG file drawn rather than
// its text.
func (t *tab) showsDrawing() bool { return t.doc.svg != nil && !t.source }

// focusEditor gives the keyboard to the editor of tab t, when it shows its
// text: now, or when the editor is first laid out.
func (a *app) focusEditor(t *tab) {
	if t != nil && t.hasText() && !t.showsDrawing() && !t.showsMarkdown() && !t.showsSheet() {
		t.view.Focus()
	}
}

// hasSource reports whether the tab can switch between drawn and source.
func (t *tab) hasSource() bool { return t.doc.svg != nil || t.doc.markdown || t.doc.sheet != nil }

// showsMarkdown reports whether the tab shows a Markdown file rendered rather
// than its text.
func (t *tab) showsMarkdown() bool { return t.doc.markdown && !t.source }

// docOf returns the open document of the file at path, or nil.
func (a *app) docOf(path string) *document {
	for _, id := range a.order {
		if d := a.tabs[id].doc; d.diff == nil && samePath(d.path, path) {
			return d
		}
	}
	return nil
}

// tabsOf returns the IDs of the tabs that show d.
func (a *app) tabsOf(d *document) []string {
	var ids []string
	for _, id := range a.order {
		if a.tabs[id].doc == d {
			ids = append(ids, id)
		}
	}
	return ids
}

// newTab makes a tab of d, which tells the status bar when its caret or zoom
// changes, and registers it.
func (a *app) newTab(d *document, preview bool) *tab {
	a.nextTab++
	t := &tab{id: fmt.Sprintf("doc:%d", a.nextTab), doc: d, preview: preview, scale: 1}
	id := t.id
	t.view.OnSelectionChange = func(editor.TextRange) {
		if a.current == id {
			a.refreshStatus()
		}
	}
	a.tabs[id] = t
	a.order = append(a.order, id)
	if preview {
		a.previewID = id
	}
	return t
}

// openFile shows the file at path in a tab. A preview opens it in the
// preview tab, which the next preview reuses; otherwise the tab stays. A file
// that is open already shows in its tab, in the group of the current tab if
// it has one there.
func (a *app) openFile(path string, preview bool) {
	if d := a.docOf(path); d != nil {
		id := a.nearestTab(d)
		if !preview {
			a.keep(id)
		}
		a.activate(id)
		return
	}
	d := a.newDocument(path)
	a.place(d, preview)
}

// place shows the new document d in a tab: the preview tab if preview and
// there is one, else a new tab beside the current one.
func (a *app) place(d *document, preview bool) {
	if d.diff == nil {
		a.watchDir(filepath.Dir(d.path))
	}
	if d.diff != nil {
		d.diff.linkScroll()
	}
	if preview && a.previewID != "" {
		// The preview tab shows the new file in place of the old. The tab is
		// made anew under the same ID, so that its editor and viewer start
		// over, and the dock keeps it where it is.
		old := a.tabs[a.previewID]
		t := &tab{id: old.id, doc: d, preview: true, scale: 1}
		t.view.OnSelectionChange = old.view.OnSelectionChange
		a.tabs[t.id] = t
		a.activate(t.id)
		return
	}
	anchor := a.anchor()
	t := a.newTab(d, preview)
	// The dock takes a panel added in this frame (ADR 0109), so the new tab
	// goes beside the editors at once, never into the sidebar's group, and
	// an explorer that slid out stays out.
	a.dock.Move(t.id, anchor, nav.DockCenter)
	a.activate(t.id)
}

// nearestTab returns the tab of d to show: the current one, else one in the
// group of the current tab, else the first.
func (a *app) nearestTab(d *document) string {
	ids := a.tabsOf(d)
	if slices.Contains(ids, a.current) {
		return a.current
	}
	group := groupOf(a.dock.Arrangement(), a.current)
	for _, id := range ids {
		if slices.Contains(group, id) {
			return id
		}
	}
	return ids[0]
}

// splitRight shows the document of the current tab in a new tab at the right
// of its group. The two share the text and keep their own place in it; the
// new one starts where the old one is.
func (a *app) splitRight() {
	cur := a.currentTab()
	if cur == nil || cur.doc.diff != nil {
		return
	}
	a.keep(cur.id)
	t := a.newTab(cur.doc, false)
	t.view.SetSelection(cur.view.Selection())
	t.view.SetScroll(cur.view.Scroll())
	a.dock.Move(t.id, cur.id, nav.DockRight)
	a.activate(t.id)
}

// keep turns the preview tab id into an ordinary one.
func (a *app) keep(id string) {
	if t := a.tabs[id]; t != nil && t.preview {
		t.preview = false
		a.previewID = ""
		a.refresh()
	}
}

// anchor is the panel a new tab opens beside: the current tab, or the last
// one opened, or the welcome page.
func (a *app) anchor() string {
	arr := a.dock.Arrangement()
	inEditors := func(id string) bool {
		g := groupOf(arr, id)
		return g != nil && !slices.ContainsFunc(g, isSidebar) && !slices.ContainsFunc(g, isTerminal)
	}
	if a.current != "" && inEditors(a.current) {
		return a.current
	}
	for i := len(a.order) - 1; i >= 0; i-- {
		if inEditors(a.order[i]) {
			return a.order[i]
		}
	}
	return welcomeID
}

// activate selects tab id and makes it current.
func (a *app) activate(id string) {
	if a.current != id && a.current != "" {
		a.leaving(a.current)
	}
	a.current = id
	a.dock.Activate(id)
	a.refresh()
}

// closeTab closes tab id.
func (a *app) closeTab(id string) {
	if a.tabs[id] == nil {
		return
	}
	a.dock.Close(id)
	a.tabClosed(id)
}

// tabClosed forgets tab id, which has closed. Its document goes with its last
// tab.
func (a *app) tabClosed(id string) {
	if a.tabs[id] == nil {
		return
	}
	d := a.tabs[id].doc
	defer func() {
		if a.isOpen(d) {
			return
		}
		if d.dirty {
			a.closedDirty(d)
		}
		if d.saving() {
			a.closing = append(a.closing, d)
		}
		// A large file lets go of its file once nothing waits to save it.
		if d.big != nil && (!d.dirty || a.settings.Autosave != config.AutosaveOff) {
			d.big.Close()
		}
	}()
	delete(a.tabs, id)
	a.order = slices.DeleteFunc(a.order, func(s string) bool { return s == id })
	if a.previewID == id {
		a.previewID = ""
	}
	arr := a.dock.Arrangement()
	arr.Root = prune(removePanel(cloneNode(arr.Root), id))
	if a.current == id {
		a.current = ""
		for _, act := range activeOf(arr) {
			if a.tabs[act] != nil {
				a.current = act
			}
		}
	}
	if len(a.tabs) == 0 && !a.settingsOpen {
		// The welcome page takes the place of the editors.
		a.dock.SetArrangement(placeInEditors(arr, welcomeID, ""))
	}
	a.refresh()
}

// closeAll closes every tab.
func (a *app) closeAll() {
	for _, id := range slices.Clone(a.order) {
		a.closeTab(id)
	}
}

// arrangementChanged follows a change the user made in the dock: a tab
// selected in another group may be the one the user works in now.
func (a *app) arrangementChanged(arr nav.DockArrangement) {
	act := activeOf(arr)
	if slices.Contains(act, a.current) {
		return
	}
	for _, id := range act {
		if a.tabs[id] != nil {
			a.current = id
			a.refresh()
			return
		}
	}
}

// focusTab makes tab id current, as the focus went into it.
func (a *app) focusTab(id string) {
	if a.current != id && a.tabs[id] != nil {
		if a.current != "" {
			a.leaving(a.current)
		}
		a.current = id
		a.refresh()
	}
}

// openAsText shows the binary document of tab id as text, in all its tabs.
func (a *app) openAsText(id string) {
	if t := a.tabs[id]; t != nil {
		t.doc.openAsText()
		a.refresh()
	}
}

// closeNotice takes away the notice above the text of tab id.
func (a *app) closeNotice(id string) {
	if t := a.tabs[id]; t != nil {
		t.noticeClosed = true
		a.refresh()
	}
}

// zoomed takes the zoom of the picture of tab id from its viewer.
func (a *app) zoomed(id string, scale float32, _ geom.Offset) {
	if t := a.tabs[id]; t != nil && t.scale != scale {
		t.scale = scale
		if a.current == id {
			a.refreshStatus()
		}
	}
}

// toggleSource switches the current tab between its drawing, its rendered
// Markdown or its table, and its text.
func (a *app) toggleSource() {
	if t := a.currentTab(); t != nil && t.hasSource() {
		t.source = !t.source
		a.refresh()
	}
}
