package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bankai-tensei/nople/internal/config"
	"github.com/bankai-tensei/nople/internal/i18n"
	"github.com/bankai-tensei/nople/internal/safewrite"
	"github.com/bankai-tensei/nople/internal/textfile"

	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/gowaku/markdown"
	"github.com/bankai-tensei/gowaku/paint"
	"github.com/bankai-tensei/gowaku/widgets/control"
	"github.com/bankai-tensei/gowaku/widgets/overlay"
)

// applySettings reads the settings file again and puts it into effect.
func (a *app) applySettings() {
	s, err := config.Load(a.settingsPath)
	a.settingsErr = nil
	if err != nil {
		a.settingsErr = errors.New(T("settings.unreadable", errors.Unwrap(err)))
	}
	if err != nil {
		a.refresh()
		return
	}
	old := a.settings
	a.settings = s
	i18n.Set(s.Language)
	if s.FontSize != old.FontSize {
		a.fontSize = s.FontSize
	}
	if s.Wrap != old.Wrap {
		a.wrap = s.Wrap
	}
	a.shellCmd = strings.Fields(s.Shell)
	if a.shellOverride != nil {
		a.shellCmd = a.shellOverride
	}
	if s.Gitignore != old.Gitignore {
		a.followGitignore()
	}
	a.refresh()
}

// openSettings opens the settings file in a tab, writing the defaults first
// if there is none.
func (a *app) openSettings() {
	p := a.settingsPath
	if p == "" {
		return
	}
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		data, _ := json.MarshalIndent(a.settings, "", "\t")
		_ = os.MkdirAll(filepath.Dir(p), 0o755) //nolint:errcheck // if it fails, so does the write below
		if err := os.WriteFile(p, append(data, '\n'), 0o644); err != nil {
			return
		}
	}
	a.watchDir(filepath.Dir(p))
	a.openFile(p, false)
}

// canEdit reports whether the document can be edited and saved: not a
// picture, a diff, a binary file shown as text, or text whose bytes could not
// be read and would not be written back as they were.
func (d *document) canEdit() bool {
	c := d.content
	return d.diff == nil && d.image == nil && c.Err == nil && !c.Binary && !c.Forced && !c.Lossy
}

// edited takes a change of the text of d by the user.
func (a *app) edited(d *document) {
	if !d.dirty {
		d.dirty = true
		a.refresh()
	}
	d.editGen++
	gen := d.editGen
	// The colors, the marks and the headings follow the text a moment
	// after typing stops.
	a.later(300*time.Millisecond, func() {
		if d.editGen == gen {
			a.refill(d)
		}
	})
	// A large file is not saved while typing pauses: its save runs on the UI
	// goroutine and takes a second a gigabyte, so that every pause would
	// freeze the window. Ctrl+S and leaving the tab still save it.
	if a.settings.Autosave == config.AutosaveAfterDelay && d.big == nil {
		a.later(time.Duration(a.settings.AutosaveDelay)*time.Millisecond, func() {
			if d.editGen == gen && d.dirty && a.isOpen(d) {
				_ = a.save(d) //nolint:errcheck // a failure shows in the bar of its tab
			}
		})
	}
}

// later runs fn on the UI goroutine after d.
func (a *app) later(d time.Duration, fn func()) {
	if a.timers != nil {
		a.timers(d, fn)
		return
	}
	time.AfterFunc(d, func() { a.shell.post(fn) })
}

// isOpen reports whether a tab still shows d.
func (a *app) isOpen(d *document) bool { return len(a.tabsOf(d)) > 0 }

// refill makes what is made from the text of d again from what it is now.
func (a *app) refill(d *document) {
	if d.big != nil {
		// A large file colors the blocks in view again; nothing else is
		// made from its text.
		a.resetBigHighlight(d)
		a.refresh()
		return
	}
	text := d.ctrl.Text()
	d.content.Text = textfile.JoinLines(text, d.content.CRLF)
	if d.markdown {
		d.headings = markdown.Headings(text)
	}
	if sep := sheetSeparator(d.path); sep != 0 {
		old := d.sheet
		d.sheet, _ = parseSheet(text, sep)
		if old != nil && d.sheet != nil && len(old.header) == len(d.sheet.header) {
			// The columns keep their widths as the cells change.
			d.sheet.widths, d.sheet.widthsFor = old.widths, old.widthsFor
		}
	}
	if isSVGPath(d.path) {
		if s, err := paint.ParseSVG([]byte(text)); err == nil {
			d.svg, d.svgErr = s, nil
		} else {
			d.svgErr = err
		}
	}
	a.startHighlight(d)
	a.startMarks(d)
	a.refresh()
}

// save writes the text of d to its file, in the encoding and with the line
// breaks it was read with.
func (a *app) save(d *document) error {
	if !d.canEdit() {
		return nil
	}
	if d.big != nil {
		// On a goroutine of its own: the save ends later (see saveThen).
		if err := a.saveBig(d, nil); err != nil {
			d.saveErr = err
			a.refresh()
			return err
		}
		return nil
	}
	text := textfile.JoinLines(d.ctrl.Text(), d.content.CRLF)
	data, err := textfile.Encode(text, d.content.Encoding)
	if errors.Is(err, textfile.ErrNotSJIS) {
		err = errors.New(T("save.sjis"))
	}
	if err == nil {
		// Through a file beside it, so that a save cut short leaves the
		// file as it was.
		err = safewrite.WriteFile(d.path, data, 0o644)
	}
	if err != nil {
		d.saveErr = err
		a.refresh()
		return err
	}
	d.saveErr, d.conflict, d.gone = nil, false, false
	d.content.Text, d.content.Size = text, int64(len(data))
	if d.dirty {
		d.dirty = false
		a.refill(d)
	}
	a.refresh()
	return nil
}

// saveThen saves d, as save does, and runs then with the error of the save
// once it ended: at once for a small file, at the end of its save for a large
// one, which saves on a goroutine of its own.
func (a *app) saveThen(d *document, then func(error)) {
	if d.big == nil || !d.canEdit() {
		then(a.save(d))
		return
	}
	if err := a.saveBig(d, then); err != nil {
		d.saveErr = err
		a.refresh()
		then(err)
	}
}

// saveAllThen saves every document that has changes, as saveAll does, and
// runs then with the first error once every save ended.
func (a *app) saveAllThen(then func(error)) {
	first := a.saveAll()
	saved := a.dirtyDocs()
	a.whenSaved(func() {
		for _, d := range saved {
			if first == nil && d.dirty && d.saveErr != nil {
				first = d.saveErr
			}
		}
		then(first)
	})
}

// saveCurrent saves the document of the current tab.
func (a *app) saveCurrent() {
	if t := a.currentTab(); t != nil && t.doc.canEdit() {
		_ = a.save(t.doc) //nolint:errcheck // a failure shows in the bar of its tab
	}
}

// saveAll saves every document that has changes. It reports the first error.
func (a *app) saveAll() error {
	var first error
	seen := map[*document]bool{}
	for _, id := range a.order {
		d := a.tabs[id].doc
		if seen[d] || !d.dirty {
			continue
		}
		seen[d] = true
		if err := a.save(d); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// dirtyDocs are the documents with changes not saved.
func (a *app) dirtyDocs() []*document {
	var ds []*document
	seen := map[*document]bool{}
	for _, id := range a.order {
		if d := a.tabs[id].doc; d.dirty && !seen[d] {
			seen[d] = true
			ds = append(ds, d)
		}
	}
	return ds
}

// revert reads d from disk again, dropping its changes.
func (a *app) revert(d *document) {
	if d.saving() {
		// The text it writes stays until the save ended.
		return
	}
	d.dirty, d.conflict = false, false
	if d.big != nil {
		a.revertBig(d)
		a.refresh()
		return
	}
	d.content.Text = "\x00" // differ from anything on disk, so that it reloads
	a.reloadDoc(d)
	a.refresh()
}

// leaving runs when the user leaves the tab id for another: with autosave on
// a change of focus, its changes are saved.
func (a *app) leaving(id string) {
	if a.settings.Autosave != config.AutosaveFocusChange {
		return
	}
	if t := a.tabs[id]; t != nil && t.doc.dirty {
		_ = a.save(t.doc) //nolint:errcheck // a failure shows in the bar of its tab
	}
}

// askSave asks whether to save the changes of d, then runs done with
// whether to go on (Cancel answers false). withCancel offers Cancel. save
// saves and runs its argument with the error once the save ended, which for a
// large file is later.
func (a *app) askSave(names string, withCancel bool, save func(then func(error)), done func(goOn bool)) {
	saved := func(err error) { done(err == nil) }
	if !a.ctxOK {
		save(saved)
		return
	}
	overlay.ShowDialog(a.ctx, func(dismiss func()) core.Widget {
		actions := []core.Widget{
			control.Button{Label: T("save.yes"), Style: control.ButtonPrimary, OnPress: func() {
				dismiss()
				save(saved)
			}},
			control.Button{Label: T("save.no"), OnPress: func() {
				dismiss()
				done(true)
			}},
		}
		if withCancel {
			actions = append(actions, control.Button{Label: T("common.cancel"), OnPress: func() {
				dismiss()
				done(false)
			}})
		}
		return overlay.Dialog{
			Title:   T("save.ask", names),
			Message: T("save.askDetail"),
			Actions: actions,
		}
	})
}

// closeTabAsking closes tab id, asking first if it is the last tab of a
// document with changes and autosave is off. With autosave on, the changes
// are saved.
func (a *app) closeTabAsking(id string) {
	t := a.tabs[id]
	if t == nil {
		return
	}
	d := t.doc
	last := len(a.tabsOf(d)) <= 1
	if last && d.saving() {
		// The last tab of a file that saves closes once the save ended, if
		// the user waits for it.
		a.askWait(d.title(), func(then func()) { a.afterSaved(d, then) }, func() { a.closeTabAsking(id) })
		return
	}
	if !d.dirty || !last {
		a.closeTab(id)
		return
	}
	if a.settings.Autosave != config.AutosaveOff {
		// A large file closes once it is saved, its tab showing how far.
		a.saveThen(d, func(error) { a.closeTab(id) })
		return
	}
	a.askSave(d.title(), true, func(then func(error)) { a.saveThen(d, then) }, func(goOn bool) {
		if goOn {
			d.dirty = false // saved, or dropped
			a.closeTab(id)
		}
	})
}

// tabCloseRequest decides whether the dock closes panel id itself. The last
// tab of a document with changes, autosave off, stays until the user answers:
// closeTabAsking then closes it, or keeps it on Cancel.
func (a *app) tabCloseRequest(id string) bool {
	t := a.tabs[id]
	if t == nil || len(a.tabsOf(t.doc)) > 1 {
		return true
	}
	d := t.doc
	// A save that runs, or a large file that autosave saves now, keeps its
	// last tab until the save ended.
	waits := d.saving() || d.dirty && d.big != nil && a.settings.Autosave != config.AutosaveOff
	if !waits && (!d.dirty || a.settings.Autosave != config.AutosaveOff) {
		return true
	}
	a.closeTabAsking(id)
	return false
}

// closedDirty handles a document whose last tab closed while it had changes:
// with autosave they are saved; without, the user is asked, as the tab cannot
// come back. The dock asks first (tabCloseRequest), so this is what is left
// for a tab that closes another way.
func (a *app) closedDirty(d *document) {
	if a.settings.Autosave != config.AutosaveOff {
		_ = a.save(d) //nolint:errcheck // a failure shows in the bar of its tab
		return
	}
	a.askSave(d.title(), false, func(then func(error)) { a.saveThen(d, then) }, func(bool) {})
}

// askWait asks whether to close once the save of names ended, and on yes runs
// then when wait says it did. Without a window to ask in, it waits.
func (a *app) askWait(names string, wait func(func()), then func()) {
	if !a.ctxOK {
		wait(then)
		return
	}
	overlay.ShowDialog(a.ctx, func(dismiss func()) core.Widget {
		return overlay.Dialog{
			Title:   T("save.waitTitle", names),
			Message: T("save.waitDetail"),
			Actions: []core.Widget{
				control.Button{Label: T("save.waitClose"), Style: control.ButtonPrimary, OnPress: func() {
					dismiss()
					wait(then)
				}},
				control.Button{Label: T("common.cancel"), OnPress: dismiss},
			},
		}
	})
}

// docNames lists the titles of ds for a dialog, the first three and how many
// more.
func docNames(ds []*document) string {
	names := make([]string, len(ds))
	for i, d := range ds {
		names[i] = d.title()
	}
	if len(names) > 3 {
		return T("save.manyFiles", strings.Join(names[:3], "、"), len(names)-3)
	}
	return strings.Join(names, "、")
}

// quit ends the app as the close button of the window does: unsaved changes
// are saved or asked about, and a save that runs ends first. A quit that
// waits for an answer or a save closes the window once it has one.
func (a *app) quit() {
	if a.confirmClose() {
		a.shell.quit()
	}
}

// confirmClose decides whether the window closes: with autosave every change
// is saved first; without, the user is asked, and the window closes after an
// answer that is not Cancel.
func (a *app) confirmClose() bool {
	if saving := a.savingDocs(); len(saving) > 0 {
		// The window closes once every save that runs ended, if the user
		// waits; what is still unsaved then is asked about as ever.
		a.askWait(docNames(saving), a.whenSaved, a.shell.requestClose)
		return false
	}
	dirty := a.dirtyDocs()
	if len(dirty) == 0 {
		return true
	}
	if a.settings.Autosave != config.AutosaveOff {
		if slices.ContainsFunc(dirty, func(d *document) bool { return d.big != nil }) {
			// A large file saves on a goroutine of its own: the window
			// closes once it is saved.
			a.saveAllThen(func(err error) {
				if err == nil {
					a.shell.requestClose()
				}
			})
			return false
		}
		if a.saveAll() == nil {
			return true
		}
		// A file that could not be saved keeps the window open, with its
		// error shown.
		return false
	}
	a.askSave(docNames(dirty), true, a.saveAllThen, func(goOn bool) {
		if goOn {
			for _, d := range a.dirtyDocs() {
				d.dirty = false
			}
			a.shell.requestClose()
		}
	})
	return false
}

// tabTitle is the label of tab t, with a dot while it has changes.
func tabTitle(t *tab) string {
	title := t.doc.title()
	if p, ok := t.doc.saveProgress(); ok {
		// How far comes first, so that a narrow tab still shows it.
		title = T("tab.saving", p, title)
	}
	if t.doc.dirty {
		return "● " + title
	}
	return title
}

// autosaveTip says when changes are saved.
func autosaveTip(a *app) string {
	switch a.settings.Autosave {
	case config.AutosaveAfterDelay:
		return T("autosave.afterDelay", float64(a.settings.AutosaveDelay)/1000)
	case config.AutosaveFocusChange:
		return T("autosave.focus")
	}
	return T("autosave.off")
}

// saveSettings writes the settings to their file.
func (a *app) saveSettings() error {
	if a.settingsPath == "" {
		return nil
	}
	return config.Save(a.settingsPath, a.settings)
}

// chooseLanguage shows the UI in the language id ("system" follows the OS)
// and keeps the choice in the settings.
func (a *app) chooseLanguage(id string) {
	a.settings.Language = id
	i18n.Set(id)
	if err := a.saveSettings(); err != nil {
		a.toast(err.Error())
	}
	a.refresh()
}
