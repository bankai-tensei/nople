package app

import (
	"github.com/bankai-tensei/nople/internal/git"
	"github.com/bankai-tensei/nople/internal/textfile"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/bankai-tensei/gowaku/widgets/editor"
)

// watchDelay gathers the events of a burst, as an editor that saves by
// writing a new file and renaming it makes several.
const watchDelay = 150 * time.Millisecond

// watcher tells the app what changed on disk in the folders it watches: the
// folders the explorer read and those of the open files.
type watcher struct {
	fs      *fsnotify.Watcher
	mu      sync.Mutex
	changed map[string]fsnotify.Op
	timer   *time.Timer
	dirs    map[string]bool
}

// startWatching watches the open folder, stopping the watch of the one
// before. Without a watcher (the OS refused one) files just do not reload.
func (a *app) startWatching() {
	a.stopWatching()
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	w := &watcher{fs: fw, changed: map[string]fsnotify.Op{}, dirs: map[string]bool{}}
	a.watch = w
	go func() {
		for {
			select {
			case e, ok := <-fw.Events:
				if !ok {
					return
				}
				w.mu.Lock()
				w.changed[filepath.Clean(e.Name)] |= e.Op
				if w.timer == nil {
					w.timer = time.AfterFunc(watchDelay, func() {
						w.mu.Lock()
						changed := w.changed
						w.changed, w.timer = map[string]fsnotify.Op{}, nil
						w.mu.Unlock()
						a.shell.post(func() {
							if a.watch == w {
								a.diskChanged(changed)
							}
						})
					})
				}
				w.mu.Unlock()
			case _, ok := <-fw.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	if a.settingsPath != "" {
		os.MkdirAll(filepath.Dir(a.settingsPath), 0o755)
		a.watchDir(filepath.Dir(a.settingsPath))
	}
	if a.ws != nil {
		a.watchDir(a.ws.Root)
		for dir := range a.ws.Children {
			a.watchDir(dir)
		}
	}
	for _, id := range a.order {
		a.watchDir(filepath.Dir(a.tabs[id].doc.path))
	}
}

// stopWatching ends the watch.
func (a *app) stopWatching() {
	if a.watch != nil {
		a.watch.fs.Close()
		a.watch = nil
	}
}

// watchDir adds the folder dir to the watch.
func (a *app) watchDir(dir string) {
	w := a.watch
	if w == nil || w.dirs[dir] {
		return
	}
	if err := w.fs.Add(dir); err == nil {
		w.dirs[dir] = true
	}
}

// diskChanged takes in what changed on disk: open files read again, folders
// of the explorer list again, and git tells its state again.
func (a *app) diskChanged(changed map[string]fsnotify.Op) {
	ws := a.ws
	gitChanged, treeChanged := false, false
	relist := map[string]bool{}
	for p, op := range changed {
		// Any change may change what git says; one in .git (the index, a
		// ref) changes nothing else.
		gitChanged = true
		if ws != nil && isInGitDir(ws.Root, a.repo, p) {
			continue
		}
		if op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
			treeChanged = true
			if dir := filepath.Dir(p); ws != nil && ws.Loaded(dir) {
				relist[dir] = true
			}
		}
	}
	if _, ok := changed[filepath.Clean(a.settingsPath)]; ok && a.settingsPath != "" {
		a.applySettings()
	}
	for _, id := range a.order {
		d := a.tabs[id].doc
		if d.diff != nil {
			continue
		}
		if _, ok := changed[filepath.Clean(d.path)]; ok {
			a.reloadDoc(d)
		}
	}
	if ws != nil {
		for dir := range relist {
			delete(ws.Children, dir)
			a.loadDir(dir)
		}
	}
	if treeChanged {
		a.listFiles()
	}
	if gitChanged && a.repo != nil {
		a.readRepo()
	}
	a.refresh()
}

// isInGitDir reports whether p is inside the .git folder of the repository
// or of the folder.
func isInGitDir(root string, r *git.Repo, p string) bool {
	for _, base := range []string{root, func() string {
		if r != nil {
			return r.Root
		}
		return root
	}()} {
		g := filepath.Join(base, ".git")
		if p == g || strings.HasPrefix(p, g+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// reloadDoc reads d again from disk, keeping the place of each of its tabs.
// A file that is gone keeps its text and says so.
func (a *app) reloadDoc(d *document) {
	if d.big != nil {
		// bigfile tells what changed, to bigChanged.
		d.big.Check()
		return
	}
	fresh := openDocument(d.path)
	if fresh.wantBig && fresh.content.Err == nil {
		// The file grew large: it opens through bigfile, unless it has
		// changes to keep.
		if d.dirty {
			d.conflict = true
			return
		}
		d.gone = false
		a.keepPlaces(d, func() {
			d.content, d.wantBig = fresh.content, true
			d.image, d.svg, d.svgErr, d.markdown, d.headings, d.hl, d.marks = nil, nil, nil, false, nil, nil, nil
			d.sheet = nil
			a.openBig(d)
		})
		return
	}
	if fresh.content.Err != nil {
		d.gone = true
		for _, id := range a.tabsOf(d) {
			a.tabs[id].noticeClosed = false
		}
		return
	}
	d.gone = false
	if fresh.image == nil && d.image == nil && fresh.content.Text == d.content.Text && fresh.content.Binary == d.content.Binary {
		return
	}
	if d.dirty {
		// The changes here and those on disk meet: the user decides.
		d.conflict = true
		return
	}
	if d.image != nil && fresh.image != nil {
		d.image, d.content = fresh.image, fresh.content
		return
	}
	type place struct {
		t      *tab
		scroll editorScroll
	}
	var places []place
	for _, id := range a.tabsOf(d) {
		t := a.tabs[id]
		places = append(places, place{t, editorScroll{t.view.Selection(), t.view.Scroll()}})
	}
	forced := d.content.Forced
	d.content, d.image = fresh.content, fresh.image
	if forced && d.content.Binary {
		d.content = textfile.Read(d.path, true)
	}
	d.ctrl.SetText(d.content.Text)
	d.svg, d.svgErr = fresh.svg, fresh.svgErr
	d.markdown, d.headings = fresh.markdown, fresh.headings
	d.sheet = fresh.sheet
	d.hl = nil
	a.startHighlight(d)
	a.startMarks(d)
	for _, p := range places {
		p.t.view.SetSelection(p.scroll.sel)
		p.t.view.SetScroll(p.scroll.pos)
	}
}

// editorScroll is the place of an editor: its selection and its scroll.
type editorScroll struct {
	sel editor.TextRange
	pos editor.ScrollPosition
}
