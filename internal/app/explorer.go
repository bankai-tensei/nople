package app

import (
	"path/filepath"

	"github.com/bankai-tensei/nople/internal/fileicon"

	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/layout"
	"github.com/bankai-tensei/gowaku/theme"
	"github.com/bankai-tensei/gowaku/widgets/basic"
	"github.com/bankai-tensei/gowaku/widgets/collection"
	"github.com/bankai-tensei/gowaku/widgets/control"
	"github.com/bankai-tensei/gowaku/widgets/interact"
)

// explorerView is the panel of the files of the open folder.
type explorerView struct {
	core.StatelessBase
	app *app
}

// Build implements core.StatelessWidget.
func (v explorerView) Build(ctx core.BuildContext) core.Widget {
	a := v.app
	th := basic.ThemeOf(ctx)
	ws := a.ws
	if ws == nil {
		return basic.Container{Color: th.Color.Sidebar, Child: basic.Padding{Insets: geom.All(theme.Space16), Child: basic.Column{
			Size: layout.MainMin, Cross: layout.CrossStart, Spacing: theme.Space12,
			Children: []core.Widget{
				basic.Text{Content: T("explorer.noFolder"), Style: th.Text.Body.WithColor(th.Color.Ink2)},
				control.Button{Label: T("common.openFolder"), Style: control.ButtonPrimary, OnPress: a.openFolderDialog},
			},
		}}}
	}
	plain := th.Color.Ink3
	var body core.Widget
	switch {
	case ws.Failed[ws.Root] != nil:
		body = basic.Padding{Insets: geom.All(theme.Space16), Child: basic.Text{
			Content: T("explorer.unreadable", ws.Failed[ws.Root].Error()), Style: th.Text.Caption.WithColor(th.Color.Danger),
		}}
	case !ws.Loaded(ws.Root):
		body = basic.Padding{Insets: geom.All(theme.Space16), Child: basic.Text{
			Content: T("explorer.loading"), Style: th.Text.Caption.WithColor(th.Color.Ink2),
		}}
	default:
		body = collection.Tree[string]{
			Roots:       ws.Children[ws.Root],
			Multiselect: true,
			OnRename: func(p, name string) {
				if err := a.rename(p, name); err != nil {
					a.toast(err.Error())
				}
			},
			OnMove:      a.treeMove,
			CanDrop:     a.canDrop,
			ChildrenOf:  func(p string) []string { return ws.Children[p] },
			HasChildren: ws.IsDir,
			LabelOf:     filepath.Base,
			IconOf: func(p string) core.Widget {
				if r := a.repo; r != nil && !ws.IsDir(p) {
					if code, ok := r.ByPath[p]; ok {
						// A changed file takes the color of its change.
						return fileicon.Widget(p, plain, statusColor(th, code), th.Brightness == theme.BrightnessDark)
					}
				}
				return fileIcon(p, ws.IsDir(p), plain, th.Brightness == theme.BrightnessDark)
			},
			Label:      T("explorer.filesOf", ws.Name()),
			Controller: &a.tree,
			OnLoad:     a.loadDir,
			OnSelectionChange: func(sel []string) {
				if len(sel) == 1 && !ws.IsDir(sel[0]) {
					a.openFile(sel[0], true)
				}
			},
			OnActivate: func(p string) {
				if ws.IsDir(p) {
					a.tree.SetExpanded(p, !a.tree.Expanded(p))
					return
				}
				a.openFile(p, false)
				// Opened on purpose, the file is what the user reads next.
				a.focusEditor(a.currentTab())
				a.dock.HideFlyout()
			},
		}
	}
	header := basic.Padding{Insets: geom.Symmetric(theme.Space12, theme.Space8), Child: basic.Text{
		Content: ws.Name(), Style: th.Text.Section.WithColor(th.Color.Ink2),
	}}
	return basic.Container{Color: th.Color.Sidebar, Child: basic.Column{Cross: layout.CrossStretch, Children: []core.Widget{
		header,
		basic.Expanded{Child: basic.Builder{Content: func(ctx core.BuildContext) core.Widget {
			a.explorerCtx, a.explorerCtxOK = ctx, true
			// A right click on a row selects it before the menu opens, and
			// the menu key and Shift+F10 open it too (PLAN §5 #20). The
			// detector is for the space below the rows.
			if tr, ok := body.(collection.Tree[string]); ok {
				tr.OnContextMenu = func(_ string, pos geom.Offset) { a.openExplorerMenu(ctx, pos) }
				body = tr
			}
			return interact.GestureDetector{
				OnSecondaryTapAt: func(t layout.Tap) { a.openExplorerMenu(ctx, t.Pos) },
				Child:            interact.Focus{OnKey: a.explorerKeys, Child: body},
			}
		}}},
	}}}
}

// welcomeView is the page in the middle while no document is open.
type welcomeView struct {
	core.StatelessBase
	app *app
}

// Build implements core.StatelessWidget.
func (v welcomeView) Build(ctx core.BuildContext) core.Widget {
	a := v.app
	th := basic.ThemeOf(ctx)
	lines := []core.Widget{
		wordmark(th, 180),
		basic.Text{Content: T("welcome.tagline"), Style: th.Text.Body.WithColor(th.Color.Ink2)},
		basic.SizedBox{Height: theme.Space8},
	}
	if a.ws == nil {
		lines = append(lines, control.Button{Label: T("common.openFolder"), Style: control.ButtonPrimary, OnPress: a.openFolderDialog})
	}
	lines = append(lines,
		basic.SizedBox{Height: theme.Space8},
		shortcutHint(th, "Ctrl+O", T("common.openFolder")),
		shortcutHint(th, "Ctrl+Shift+E", T("panel.explorer")),
		shortcutHint(th, "Ctrl+W", T("welcome.closeTab")),
	)
	return basic.Container{Color: th.Color.Canvas, Child: basic.Center{Child: basic.VStack(lines...)}}
}

// shortcutHint is one line of the keys on the welcome page.
func shortcutHint(th *theme.Theme, keys, what string) core.Widget {
	return basic.Row{Size: layout.MainMin, Spacing: theme.Space12, Children: []core.Widget{
		basic.SizedBox{Width: 110, Child: basic.Text{Content: keys, Style: th.Text.Caption.WithColor(th.Color.AccentInk)}},
		basic.Text{Content: what, Style: th.Text.Caption.WithColor(th.Color.Ink2)},
	}}
}
