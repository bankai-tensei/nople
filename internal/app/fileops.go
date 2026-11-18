package app

import (
	"errors"
	"fmt"
	"github.com/bankai-tensei/nople/internal/trash"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/input"
	"github.com/bankai-tensei/gowaku/layout"
	"github.com/bankai-tensei/gowaku/theme"
	"github.com/bankai-tensei/gowaku/widgets/basic"
	"github.com/bankai-tensei/gowaku/widgets/collection"
	"github.com/bankai-tensei/gowaku/widgets/control"
	"github.com/bankai-tensei/gowaku/widgets/field"
	"github.com/bankai-tensei/gowaku/widgets/interact"
	"github.com/bankai-tensei/gowaku/widgets/overlay"
)

// The explorer creates, renames, moves and deletes files and folders. A
// deleted one goes to the trash of the OS, from where it comes back. Tabs of
// the files follow a rename or a move, and close with a delete.

// selectedPaths are the rows selected in the explorer, inside the folder.
func (a *app) selectedPaths() []string {
	if a.ws == nil {
		return nil
	}
	var ps []string
	for _, p := range a.tree.Selection() {
		if within(a.ws.Root, p) && p != a.ws.Root {
			ps = append(ps, p)
		}
	}
	return ps
}

// within reports whether p is root or inside it.
func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// targetDir is the folder a new file goes in: the selected folder, the folder
// of the selected file, or the root.
func (a *app) targetDir() string {
	if a.ws == nil {
		return ""
	}
	sel := a.selectedPaths()
	if len(sel) == 0 {
		return a.ws.Root
	}
	p := sel[len(sel)-1]
	if a.ws.IsDir(p) {
		return p
	}
	return filepath.Dir(p)
}

// checkName refuses a name that is no name of one file: empty, with a
// separator, or one the OS reserves.
func checkName(name string) error {
	switch {
	case strings.TrimSpace(name) == "":
		return errors.New(T("name.empty"))
	case name == "." || name == "..":
		return errors.New(T("name.reserved"))
	case strings.ContainsAny(name, `\/:*?"<>|`):
		return errors.New(T("name.badChars"))
	}
	return nil
}

// createIn makes the file or folder rel in dir: rel may have folders before
// its name, as in "cmd/tool/main.go", which are made too. It returns the
// path made.
func createIn(dir, rel string, folder bool) (string, error) {
	rel = strings.TrimSpace(strings.ReplaceAll(rel, `\`, "/"))
	parts := strings.Split(strings.Trim(rel, "/"), "/")
	for _, p := range parts {
		if err := checkName(p); err != nil {
			return "", err
		}
	}
	path := filepath.Join(append([]string{dir}, parts...)...)
	if _, err := os.Lstat(path); err == nil {
		return "", fmt.Errorf(T("name.exists"), rel)
	}
	if folder {
		return path, os.MkdirAll(path, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	return path, f.Close()
}

// relist reads the folders dirs again for the explorer, at once rather than
// when the watch tells.
func (a *app) relist(dirs ...string) {
	ws := a.ws
	if ws == nil {
		return
	}
	for _, d := range dirs {
		if ws.Loaded(d) {
			delete(ws.Children, d)
			a.loadDir(d)
		}
	}
	a.listFiles()
}

// newFile asks for the name of a new file in the target folder, makes it and
// opens it.
func (a *app) newFile() { a.askCreate(false) }

// newFolder asks for the name of a new folder in the target folder and makes
// it.
func (a *app) newFolder() { a.askCreate(true) }

func (a *app) askCreate(folder bool) {
	dir := a.targetDir()
	if dir == "" {
		return
	}
	title, hint := T("create.file"), T("create.fileHint")
	if folder {
		title, hint = T("create.folder"), T("create.folderHint")
	}
	where := a.ws.Rel(dir)
	if where == "" {
		where = a.ws.Name()
	}
	a.askName(T("create.titleIn", title, where), hint, "", func(name string) error {
		p, err := createIn(dir, name, folder)
		if err != nil {
			return err
		}
		a.tree.Expand(dir)
		a.relist(dir, filepath.Dir(p))
		for d := filepath.Dir(p); within(dir, d) && d != dir; d = filepath.Dir(d) {
			a.tree.Expand(d)
		}
		a.tree.Select(p)
		if !folder {
			// The new file is what the user works on next.
			a.openFile(p, false)
			a.dock.HideFlyout()
		}
		return nil
	})
}

// askName shows a dialog with a field for a name; ok gets the name and
// returns why it cannot be used, which keeps the dialog open.
func (a *app) askName(title, hint, value string, ok func(name string) error) {
	if !a.ctxOK {
		return
	}
	overlay.ShowDialog(a.explorerDialogCtx(), func(dismiss func()) core.Widget {
		return nameDialog{title: title, hint: hint, value: value, ok: ok, dismiss: dismiss}
	})
}

// nameDialog is the dialog of askName.
type nameDialog struct {
	core.StatefulBase
	title, hint, value string
	ok                 func(string) error
	dismiss            func()
}

type nameDialogState struct {
	core.StateOf[nameDialog]
	field field.InputController
	text  string
	err   string
}

// CreateState implements core.StatefulWidget.
func (nameDialog) CreateState() core.State { return &nameDialogState{} }

// InitState implements core.State.
func (s *nameDialogState) InitState() {
	s.text = s.Widget().value
	s.field.SelectAll()
	s.field.Focus()
}

func (s *nameDialogState) submit() {
	w := s.Widget()
	if err := w.ok(s.text); err != nil {
		s.SetState(func() { s.err = err.Error() })
		return
	}
	w.dismiss()
}

// Build implements core.State.
func (s *nameDialogState) Build(ctx core.BuildContext) core.Widget {
	th := basic.ThemeOf(ctx)
	w := s.Widget()
	content := []core.Widget{field.Input{
		Value: w.value, Placeholder: w.hint, Controller: &s.field,
		OnChange: func(v string) { s.text, s.err = v, "" },
		OnSubmit: func(string) { s.submit() },
	}}
	if s.err != "" {
		content = append(content, basic.Text{Content: s.err, Style: th.Text.Caption.WithColor(th.Color.Danger)})
	}
	return overlay.Dialog{
		Title: w.title, Width: 460,
		Content: basic.Column{Size: layout.MainMin, Cross: layout.CrossStretch, Spacing: theme.Space8, Children: content},
		Actions: []core.Widget{
			control.Button{Label: T("create.ok"), Style: control.ButtonPrimary, OnPress: s.submit},
			control.Button{Label: T("common.cancel"), OnPress: w.dismiss},
		},
	}
}

// rename renames the file or folder at p to name, in its folder.
func (a *app) rename(p, name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	to := filepath.Join(filepath.Dir(p), name)
	if to == p {
		return nil
	}
	if _, err := os.Lstat(to); err == nil && !strings.EqualFold(to, p) {
		return fmt.Errorf(T("name.exists"), name)
	}
	if err := os.Rename(p, to); err != nil {
		return err
	}
	a.moved(p, to)
	a.relist(filepath.Dir(p))
	a.tree.Select(to)
	return nil
}

// renameSelected starts editing the name of the selected row in place.
func (a *app) renameSelected() {
	if sel := a.selectedPaths(); len(sel) == 1 {
		a.tree.Rename(sel[0])
	}
}

// moveInto moves the files and folders paths into the folder dir.
func (a *app) moveInto(paths []string, dir string) error {
	var errs []string
	dirs := []string{dir}
	for _, p := range paths {
		to := filepath.Join(dir, filepath.Base(p))
		switch {
		case to == p:
			continue
		case within(p, dir):
			errs = append(errs, T("move.intoItself", filepath.Base(p)))
			continue
		}
		if _, err := os.Lstat(to); err == nil {
			errs = append(errs, T("move.exists", filepath.Base(p)))
			continue
		}
		if err := os.Rename(p, to); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		a.moved(p, to)
		dirs = append(dirs, filepath.Dir(p))
	}
	a.tree.Expand(dir)
	a.relist(dirs...)
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}
	return nil
}

// moved makes the tabs and the explorer follow a file or folder that went
// from old to to: the documents inside it take their new paths.
func (a *app) moved(old, to string) {
	seen := map[*document]bool{}
	for _, id := range a.order {
		d := a.tabs[id].doc
		if seen[d] {
			continue
		}
		seen[d] = true
		if np, ok := rebase(d.path, old, to); ok {
			d.path = np
			a.watchDir(filepath.Dir(np))
		}
	}
	// What was open in the tree stays open under its new name.
	if a.ws != nil {
		for dir := range a.ws.Children {
			if np, ok := rebase(dir, old, to); ok && a.tree.Expanded(dir) {
				a.tree.Expand(np)
			}
		}
	}
	a.refresh()
}

// rebase is p moved from under old to under to, if it was there.
func rebase(p, old, to string) (string, bool) {
	if samePath(p, old) {
		return to, true
	}
	if within(old, p) {
		rel, _ := filepath.Rel(old, p)
		return filepath.Join(to, rel), true
	}
	return "", false
}

// deleteSelected asks, then moves the selected files and folders to the
// trash.
func (a *app) deleteSelected() {
	sel := a.selectedPaths()
	if len(sel) == 0 || !a.ctxOK {
		return
	}
	what := T("delete.quoted", filepath.Base(sel[0]))
	if len(sel) > 1 {
		what = T("delete.many", what, len(sel)-1)
	}
	overlay.ShowDialog(a.explorerDialogCtx(), func(dismiss func()) core.Widget {
		return overlay.Dialog{
			Title:   T("delete.ask", what),
			Message: T("delete.askDetail"),
			Actions: []core.Widget{
				control.Button{Label: T("delete.ok"), Style: control.ButtonPrimary, OnPress: func() {
					dismiss()
					if err := a.trashPaths(sel); err != nil {
						a.toast(T("delete.failed", err.Error()))
					}
				}},
				control.Button{Label: T("common.cancel"), OnPress: dismiss},
			},
		}
	})
}

// trashPaths moves paths to the trash, closing the tabs of what was in them.
func (a *app) trashPaths(paths []string) error {
	var errs []string
	var dirs []string
	for _, p := range paths {
		trash := trash.Move
		if a.trash != nil {
			trash = a.trash
		}
		if err := trash(p); err != nil {
			errs = append(errs, filepath.Base(p)+": "+err.Error())
			continue
		}
		dirs = append(dirs, filepath.Dir(p))
		for _, id := range slices.Clone(a.order) {
			if t := a.tabs[id]; t != nil && t.doc.diff == nil {
				if _, ok := rebase(t.doc.path, p, p); ok {
					t.doc.dirty = false // it is gone on purpose
					a.closeTab(id)
				}
			}
		}
	}
	a.tree.ClearSelection()
	a.relist(dirs...)
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "、"))
	}
	return nil
}

// toast tells something for a moment.
func (a *app) toast(msg string) {
	if a.ctxOK {
		overlay.ShowToast(a.ctx, overlay.Toast{Message: msg, Duration: 5 * time.Second})
	}
}

// explorerMenu are the items of the menu of the explorer.
func (a *app) explorerMenu() []overlay.MenuItem {
	one := len(a.selectedPaths()) == 1
	some := len(a.selectedPaths()) > 0
	item := func(label string, sc interact.Shortcut, on bool, fn func()) overlay.MenuItem {
		act := interact.Action{Label: label, Shortcut: sc}
		if on {
			act.OnInvoke = fn
		}
		return overlay.MenuItem{Action: act}
	}
	return []overlay.MenuItem{
		item(T("menu.newFile"), shortcut(input.KeyN, ctrl), a.ws != nil, a.newFile),
		item(T("menu.newFolderCtx"), interact.Shortcut{}, a.ws != nil, a.newFolder),
		{Separator: true},
		item(T("menu.rename"), shortcut(input.KeyF2, 0), one, a.renameSelected),
		item(T("menu.delete"), shortcut(input.KeyDelete, 0), some, a.deleteSelected),
	}
}

// explorerKeys are the keys of the explorer while a row has the focus:
// Delete deletes; F2 the tree itself takes.
func (a *app) explorerKeys(e *core.KeyEvent) bool {
	if e.Key == input.KeyDelete && e.Modifiers == 0 {
		a.deleteSelected()
		return true
	}
	return false
}

// treeMove takes a drag in the explorer: the rows go into the folder they
// were dropped in or beside.
func (a *app) treeMove(m collection.TreeMove[string]) {
	dir := a.ws.Root
	if !m.AtRoot {
		dir = m.Parent
	}
	if err := a.moveInto(m.Nodes, dir); err != nil {
		a.toast(err.Error())
	}
}

// canDrop refuses a move that would change nothing.
func (a *app) canDrop(m collection.TreeMove[string]) bool {
	dir := a.ws.Root
	if !m.AtRoot {
		dir = m.Parent
	}
	for _, p := range m.Nodes {
		if filepath.Dir(p) != dir {
			return true
		}
	}
	return false
}

// explorerDialogCtx is where a dialog of the explorer opens: at the place of
// the explorer while it is the flyout, so that the dock keeps the flyout out
// (PLAN §5 #21), else on the window.
func (a *app) explorerDialogCtx() core.BuildContext {
	if a.explorerCtxOK && a.dock.Flyout() == explorerID {
		return a.explorerCtx
	}
	return a.ctx
}

// openExplorerMenu opens the menu of the explorer at pos of ctx, a context at
// the place of the explorer.
func (a *app) openExplorerMenu(ctx core.BuildContext, pos geom.Offset) {
	overlay.ShowPopupMenu(ctx, pos, a.explorerMenu())
}
