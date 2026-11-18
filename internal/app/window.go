package app

import (
	"github.com/bankai-tensei/nople/internal/fileicon"
	"github.com/bankai-tensei/nople/internal/i18n"
	"log"
	"os"

	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/input"
	"github.com/bankai-tensei/gowaku/layout"
	"github.com/bankai-tensei/gowaku/paint"
	"github.com/bankai-tensei/gowaku/theme"
	"github.com/bankai-tensei/gowaku/widgets/basic"
	"github.com/bankai-tensei/gowaku/widgets/field"
	"github.com/bankai-tensei/gowaku/widgets/interact"
	"github.com/bankai-tensei/gowaku/widgets/nav"
)

// windowView is the root of the window.
type windowView struct {
	core.StatefulBase
	app *app
}

type windowState struct {
	core.StateOf[windowView]
	// title is what the window was last called, and dropping that files are
	// dragged over it.
	title    string
	dropping bool
}

// CreateState implements core.StatefulWidget.
func (windowView) CreateState() core.State { return &windowState{} }

// InitState implements core.State.
func (s *windowState) InitState() {
	a := s.Widget().app
	a.view = s
	if dir := a.startFolder; dir != "" {
		a.startFolder = ""
		a.shell.post(func() {
			if err := a.openFolder(dir); err != nil {
				log.Printf("nople: %v", err)
			}
		})
	} else if file := a.startFile; file != "" {
		a.startFile = ""
		a.shell.post(func() {
			a.restoreSession()
			a.openFile(file, false)
		})
	} else if a.sessionPath != "" {
		a.shell.post(func() { a.restoreSession() })
	}
	if more := a.startMore; len(more) > 0 {
		a.startMore = nil
		a.shell.post(func() { a.openPaths(more) })
	}
	a.trace.opened(a.shell.post)
}

// Dispose implements core.State.
func (s *windowState) Dispose() {
	if a := s.Widget().app; a.view == s {
		a.view = nil
	}
}

// Build implements core.State.
func (s *windowState) Build(ctx core.BuildContext) core.Widget {
	a := s.Widget().app
	a.trace.builtOnce(a.shell.post)
	md := basic.MediaQueryOf(ctx)
	md.Locale = i18n.Locale()
	var child core.Widget = basic.Builder{Content: func(ctx core.BuildContext) core.Widget {
		return s.content(ctx, a)
	}}
	// A theme of the settings other than the system one takes the place of
	// the one the window follows.
	switch a.settings.Theme {
	case "light":
		child = basic.Theme{Data: theme.Default().Light, Child: child}
	case "dark":
		child = basic.Theme{Data: theme.Default().Dark, Child: child}
	}
	return basic.MediaQuery{Data: md, Child: child}
}

// content is the window under its MediaQuery.
func (s *windowState) content(ctx core.BuildContext, a *app) core.Widget {
	th := basic.ThemeOf(ctx)
	if title := a.windowTitle(); title != s.title {
		s.title = title
		a.shell.setTitle(ctx, title)
	}
	acts := newActions(a)
	return field.CommandPalette{Actions: acts.all(), RecentFirst: true, Placeholder: T("palette.placeholder"), Child: basic.Builder{Content: func(ctx core.BuildContext) core.Widget {
		a.ctx, a.ctxOK = ctx, true
		acts.palette = field.CommandPaletteAction(ctx)
		acts.palette.Label = T("menu.palette")
		return s.frame(th, a, acts)
	}}}
}

// frame is the window inside its command palette.
func (s *windowState) frame(th *theme.Theme, a *app, acts actions) core.Widget {
	body := basic.Column{Cross: layout.CrossStretch, Children: []core.Widget{
		basic.Container{Color: th.Color.Sidebar, Child: acts.menuBar()},
		basic.Container{Height: 1, Color: th.Color.Hair},
		basic.Expanded{Child: s.dock(a)},
		statusView{app: a},
	}}
	// A ground of the theme under everything, as the window paints that of the
	// theme of the OS, which the theme of the settings may not be.
	layers := []core.Widget{basic.Container{Color: th.Color.Sidebar, Child: body}}
	if s.dropping {
		layers = append(layers, basic.IgnorePointer{Child: dropHint(th)})
	}
	return interact.Actions{
		Actions: acts.extra(),
		Child: interact.DragTarget{
			Kinds:   input.DataFiles,
			OnHover: func(over bool) { s.SetState(func() { s.dropping = over }) },
			OnDrop:  func(d input.DragData, _ geom.Offset) { s.drop(d.Files) },
			Child:   basic.Stack{Fit: layout.StackExpand, Children: layers},
		},
	}
}

// dock is the layout of the panels: the sidebar on the strip at the left and
// the documents in the middle.
func (s *windowState) dock(a *app) core.Widget {
	panels := []nav.DockPanel{
		{ID: explorerID, Title: T("panel.explorer"), Icon: iconFiles, Build: func(core.BuildContext) core.Widget {
			return explorerView{app: a}
		}},
		{ID: searchID, Title: T("panel.search"), Icon: iconSearch, Build: func(core.BuildContext) core.Widget {
			return searchView{app: a}
		}},
		{ID: scmID, Title: T("panel.scm"), Icon: iconBranch, Build: func(core.BuildContext) core.Widget {
			return scmView{app: a}
		}},
		{ID: outlineID, Title: T("panel.outline"), Icon: iconOutline, Build: func(core.BuildContext) core.Widget {
			return outlineView{app: a}
		}},
	}
	if a.settingsOpen {
		panels = append(panels, nav.DockPanel{ID: settingsID, Title: T("settings.title"), Icon: iconSettings, Closable: true,
			Build: func(core.BuildContext) core.Widget { return settingsView{app: a} }})
	}
	if len(a.tabs) == 0 && !a.settingsOpen {
		panels = append(panels, nav.DockPanel{ID: welcomeID, Title: T("panel.welcome"), Icon: iconHome, Build: func(core.BuildContext) core.Widget {
			return welcomeView{app: a}
		}})
	}
	for _, id := range a.order {
		t := a.tabs[id]
		panels = append(panels, nav.DockPanel{ID: id, Title: tabTitle(t), Icon: fileicon.TabIcon(t.doc.path), Closable: true, Italic: t.preview,
			Build: func(core.BuildContext) core.Widget {
				return focusWithin{
					onChange: func(within bool) {
						if within {
							a.focusTab(id)
						}
					},
					child: docView{app: a, tab: a.tabs[id]},
				}
			}})
	}
	for _, id := range a.termOrder {
		t := a.terms[id]
		panels = append(panels, nav.DockPanel{ID: id, Title: t.title(), Icon: iconTerminal, Closable: true,
			Build: func(core.BuildContext) core.Widget { return terminalView{app: a, term: t} }})
	}
	return nav.DockLayout{
		Panels:     panels,
		Controller: &a.dock,
		Default:    defaultArrangement(),
		Label:      T("panel.editors"),
		AutoHide:   true,
		// The × of a tab, Ctrl+W and the middle button ask first about a
		// document that would lose its changes (PLAN §5 #19).
		OnCloseRequest: a.tabCloseRequest,
		OnClose: func(id string) {
			if isTerminal(id) {
				a.termClosed(id)
				return
			}
			if id == settingsID {
				a.settingsClosed()
				return
			}
			a.tabClosed(id)
		},
		OnChange: a.arrangementChanged,
	}
}

// drop opens what is dropped on the window: a folder as the workspace, files
// in tabs.
func (s *windowState) drop(files []string) {
	s.SetState(func() { s.dropping = false })
	a := s.Widget().app
	for _, f := range files {
		st, err := os.Stat(f)
		if err != nil {
			continue
		}
		if st.IsDir() {
			a.openFolder(f)
			return
		}
	}
	for _, f := range files {
		a.openFile(f, false)
	}
}

// dropHint covers the window while files are dragged over it.
func dropHint(th *theme.Theme) core.Widget {
	return basic.Padding{Insets: geom.All(theme.Space8), Child: basic.Container{
		Color: th.Color.AccentTint, Radius: th.Radius.Group,
		Border: paint.Stroke{Width: 2, Color: th.Color.AccentInk},
		Child: basic.Center{Child: basic.Text{
			Content: T("drop.hint"), Style: th.Text.Title.WithColor(th.Color.AccentInk),
		}},
	}}
}

// placeholder is a panel that says what it will hold.
func placeholder(ctx core.BuildContext, msg string) core.Widget {
	th := basic.ThemeOf(ctx)
	return basic.Container{Color: th.Color.Sidebar, Child: basic.Padding{Insets: geom.All(theme.Space16), Child: basic.Text{
		Content: msg, Style: th.Text.Caption.WithColor(th.Color.Ink2),
	}}}
}
