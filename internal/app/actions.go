package app

import (
	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/gowaku/input"
	"github.com/bankai-tensei/gowaku/widgets/interact"
	"github.com/bankai-tensei/gowaku/widgets/overlay"
	"github.com/bankai-tensei/nople/internal/i18n"
)

const (
	ctrl      = input.ModControl
	ctrlShift = input.ModControl | input.ModShift
)

// shortcut is a key with modifiers.
func shortcut(k input.KeyCode, m input.Modifiers) interact.Shortcut {
	return interact.Shortcut{Key: k, Modifiers: m}
}

// actions are the commands of the window, which the menus and the shortcuts
// share.
type actions struct {
	openFolder, closeFolder, reload, closeTab, keepTab, splitRight, quit interact.Action
	explorer, search, scm, outline, source                               interact.Action
	wrap, bigger, smaller, normal                                        interact.Action
	quickOpen, gotoLine, palette                                         interact.Action
	newTerminal, toggleTerminal                                          interact.Action
	// biggerSemi, toggleTerminalAt and toggleTerminalUS are second keys
	// that no menu shows: on a JIS layout the ;+ key is KeySemicolon and
	// VS Code toggles the terminal with Ctrl+@, which a US layout types as
	// Ctrl+Shift+2.
	biggerSemi, toggleTerminalAt, toggleTerminalUS interact.Action
	save, saveAll, settings, settingsJSON          interact.Action
	newFile, newFolder                             interact.Action
	// languages choose the language of the UI: the system one, then each
	// catalog.
	languages []interact.Action
	// biggerPlus is Ctrl++ on the number pad and Ctrl+Shift+=, the second
	// way to make the text bigger.
	biggerPlus interact.Action
}

// newActions makes the commands for the state of a now.
func newActions(a *app) actions {
	var acts actions
	acts.openFolder = interact.Action{Label: T("menu.openFolder"), Shortcut: shortcut(input.KeyO, ctrl), OnInvoke: a.openFolderDialog}
	acts.closeFolder = interact.Action{Label: T("menu.closeFolder")}
	acts.reload = interact.Action{Label: T("menu.reloadFolder")}
	acts.closeTab = interact.Action{Label: T("menu.closeTab"), Shortcut: shortcut(input.KeyW, ctrl)}
	acts.keepTab = interact.Action{Label: T("menu.keepTab")}
	acts.splitRight = interact.Action{Label: T("menu.splitRight"), Shortcut: shortcut(input.KeyBackslash, ctrl)}
	acts.quit = interact.Action{Label: T("menu.quit"), OnInvoke: a.quit}
	if a.ws != nil {
		acts.closeFolder.OnInvoke = a.closeFolder
		acts.reload.OnInvoke = a.reloadFolder
	}
	if t := a.currentTab(); t != nil {
		id := t.id
		acts.closeTab.OnInvoke = func() { a.closeTabAsking(id) }
		acts.splitRight.OnInvoke = a.splitRight
		if t.preview {
			acts.keepTab.OnInvoke = func() { a.keep(id) }
		}
	}

	acts.explorer = interact.Action{Label: T("menu.explorer"), Shortcut: shortcut(input.KeyE, ctrlShift), OnInvoke: func() { a.showPanel(explorerID) }}
	acts.search = interact.Action{Label: T("menu.search"), Shortcut: shortcut(input.KeyF, ctrlShift), OnInvoke: func() { a.showPanel(searchID) }}
	acts.scm = interact.Action{Label: T("menu.scm"), Shortcut: shortcut(input.KeyG, ctrlShift), OnInvoke: func() { a.readRepo(); a.showPanel(scmID) }}
	acts.outline = interact.Action{Label: T("menu.outline"), Shortcut: shortcut(input.KeyO, ctrlShift), OnInvoke: func() { a.showPanel(outlineID) }}
	acts.source = interact.Action{Label: T("menu.showSource"), Shortcut: shortcut(input.KeyV, ctrlShift), Checkable: true}
	if t := a.currentTab(); t != nil && t.hasSource() {
		acts.source.Checked = t.source
		acts.source.OnInvoke = a.toggleSource
	}
	acts.wrap = interact.Action{Label: T("menu.wrap"), Shortcut: shortcut(input.KeyZ, input.ModAlt), Checkable: true, Checked: a.wrap,
		OnInvoke: func() { a.setWrap(!a.wrap) }}
	acts.bigger = interact.Action{Label: T("menu.bigger"), Shortcut: shortcut(input.KeyEqual, ctrl), OnInvoke: func() { a.zoom(1) }}
	acts.biggerPlus = interact.Action{Label: T("menu.biggerPlain"), Shortcut: shortcut(input.KeyEqual, ctrlShift), OnInvoke: func() { a.zoom(1) }}
	acts.biggerSemi = interact.Action{Label: T("menu.bigger"), Shortcut: shortcut(input.KeySemicolon, ctrl), OnInvoke: func() { a.zoom(1) }}
	acts.smaller = interact.Action{Label: T("menu.smaller"), Shortcut: shortcut(input.KeyMinus, ctrl), OnInvoke: func() { a.zoom(-1) }}
	acts.normal = interact.Action{Label: T("menu.normalSize"), Shortcut: shortcut(input.Key0, ctrl), OnInvoke: func() { a.zoom(0) }}
	acts.newFile = interact.Action{Label: T("menu.newFile"), Shortcut: shortcut(input.KeyN, ctrl)}
	acts.newFolder = interact.Action{Label: T("menu.newFolder")}
	if a.ws != nil {
		acts.newFile.OnInvoke = a.newFile
		acts.newFolder.OnInvoke = a.newFolder
	}
	choice := a.settings.Language
	if choice == "" {
		choice = "system"
	}
	for _, id := range append([]string{"system"}, i18n.Languages()...) {
		label := T("lang.system")
		if id != "system" {
			label = i18n.Name(id)
		}
		acts.languages = append(acts.languages, interact.Action{
			Label: label, Checkable: true, Checked: choice == id,
			OnInvoke: func() { a.chooseLanguage(id) },
		})
	}
	acts.save = interact.Action{Label: T("menu.save"), Shortcut: shortcut(input.KeyS, ctrl)}
	acts.saveAll = interact.Action{Label: T("menu.saveAll")}
	if t := a.currentTab(); t != nil && t.doc.canEdit() {
		acts.save.OnInvoke = a.saveCurrent
	}
	if len(a.dirtyDocs()) > 0 {
		acts.saveAll.OnInvoke = func() { a.saveAll() }
	}
	acts.settings = interact.Action{Label: T("menu.settings"), Shortcut: shortcut(input.KeyComma, ctrl), OnInvoke: a.openSettingsTab}
	acts.settingsJSON = interact.Action{Label: T("menu.settingsJSON"), OnInvoke: a.openSettings}
	acts.newTerminal = interact.Action{Label: T("menu.newTerminal"), OnInvoke: a.newTerminal}
	acts.toggleTerminal = interact.Action{Label: T("menu.toggleTerminal"), Shortcut: shortcut(input.KeyJ, ctrl), OnInvoke: a.toggleTerminal}
	acts.toggleTerminalAt = interact.Action{Label: T("menu.toggleTerminal"), Shortcut: shortcut(input.KeyAt, ctrl), OnInvoke: a.toggleTerminal}
	acts.toggleTerminalUS = interact.Action{Label: T("menu.toggleTerminal"), Shortcut: shortcut(input.Key2, ctrlShift), OnInvoke: a.toggleTerminal}
	acts.quickOpen = interact.Action{Label: T("menu.gotoFile"), Shortcut: shortcut(input.KeyP, ctrl), OnInvoke: a.quickOpen}
	acts.gotoLine = interact.Action{Label: T("menu.gotoLine"), Shortcut: shortcut(input.KeyG, ctrl)}
	if t := a.currentTab(); t != nil && t.hasText() {
		acts.gotoLine.OnInvoke = a.gotoLinePicker
	}
	return acts
}

// extra are the shortcuts that no menu shows.
func (acts actions) extra() []interact.Action {
	return []interact.Action{acts.biggerPlus, acts.biggerSemi, acts.toggleTerminalAt, acts.toggleTerminalUS}
}

// all are the commands the command palette finds: those of the menus.
func (acts actions) all() []interact.Action {
	return append([]interact.Action{
		acts.newFile, acts.newFolder,
		acts.openFolder, acts.reload, acts.closeFolder, acts.save, acts.saveAll, acts.settings, acts.settingsJSON, acts.keepTab, acts.splitRight, acts.closeTab, acts.quit,
		acts.quickOpen, acts.gotoLine,
		acts.newTerminal, acts.toggleTerminal,
		acts.explorer, acts.search, acts.scm, acts.outline, acts.source, acts.wrap, acts.bigger, acts.smaller, acts.normal,
	}, acts.languages...)
}

// menuBar is the menu bar of the window.
func (acts actions) menuBar() core.Widget {
	item := func(a interact.Action) overlay.MenuItem { return overlay.MenuItem{Action: a} }
	var langItems []overlay.MenuItem
	for _, l := range acts.languages {
		langItems = append(langItems, item(l))
	}
	sep := overlay.MenuItem{Separator: true}
	return overlay.MenuBar{Items: []overlay.MenuItem{
		{Action: interact.Action{Label: T("menu.file")}, Items: []overlay.MenuItem{
			item(acts.newFile), item(acts.newFolder),
			sep, item(acts.openFolder), item(acts.reload), item(acts.closeFolder),
			sep, item(acts.save), item(acts.saveAll),
			sep, item(acts.keepTab), item(acts.splitRight), item(acts.closeTab),
			sep, item(acts.settings), item(acts.settingsJSON),
			sep, item(acts.quit),
		}},
		{Action: interact.Action{Label: T("menu.go")}, Items: []overlay.MenuItem{
			item(acts.quickOpen), item(acts.gotoLine), sep, item(acts.palette),
		}},
		{Action: interact.Action{Label: T("menu.terminal")}, Items: []overlay.MenuItem{
			item(acts.newTerminal), item(acts.toggleTerminal),
		}},
		{Action: interact.Action{Label: T("menu.view")}, Items: []overlay.MenuItem{
			item(acts.explorer), item(acts.search), item(acts.scm), item(acts.outline),
			sep, item(acts.source),
			sep, item(acts.wrap),
			sep, item(acts.bigger), item(acts.smaller), item(acts.normal),
			sep, {Action: interact.Action{Label: T("menu.language")}, Items: langItems},
		}},
	}}
}
