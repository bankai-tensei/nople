package app

import (
	"fmt"
	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/nople/internal/config"
	"github.com/bankai-tensei/nople/internal/git"
	"github.com/bankai-tensei/nople/internal/workspace"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/bankai-tensei/gowaku"
	"github.com/bankai-tensei/gowaku/widgets/collection"
	"github.com/bankai-tensei/gowaku/widgets/nav"
)

// Sizes of the editor text.
const (
	minFontSize = 8
	maxFontSize = 40
)

// app is the state of the window: the folder that is open and the documents
// in its tabs. It belongs to the UI goroutine; the window's State shows it.
type app struct {
	shell shell
	// ws is the open folder, or nil.
	ws *workspace.Workspace

	// tabs are the open tabs by the ID of their panel, and order the IDs in
	// the order they opened. Tabs of one file share its document.
	tabs    map[string]*tab
	order   []string
	nextTab int
	// current is the tab the user works in, or "".
	current string
	// previewID is the preview tab, or "".
	previewID string
	// closing are the documents whose last tab closed while their save ran,
	// until it ends, so that quitting waits for it.
	closing []*document

	dock nav.DockController
	tree collection.TreeController[string]

	wrap     bool
	fontSize float32

	// view and status are the States that show the app while they are in
	// the tree.
	view   *windowState
	status *statusState
	// startFolder is the folder to open once the window shows.
	startFolder string
	// startFile is the file to open in a tab once the window shows, over
	// what the session had open.
	startFile string
	// startMore are the files to open in tabs after startFile or
	// startFolder, when Finder hands over more than one at the start.
	startMore []string

	// files are the files of the folder for quick open, and find the
	// search of the folder.
	files fileList
	find  searchState
	// repo is the Git repository of the folder, or nil; repoErr is why
	// git could not read it. scmTree is the tree of its changes.
	// terms are the terminals by the ID of their panel, in termOrder;
	// shellCmd is the shell they start, or nil for the default.
	terms     map[string]*terminal
	termOrder []string
	nextTerm  int
	shellCmd  []string
	repo      *git.Repo
	repoErr   error
	scmTree   collection.TreeController[string]
	// watch tells of changes on disk; sessionPath is where the session is
	// kept, empty for none.
	watch       *watcher
	sessionPath string
	// settings are those of settingsPath, or the defaults; settingsErr is
	// why the file did not read. shellOverride is the shell of -shell.
	settings      config.Settings
	settingsPath  string
	settingsErr   error
	shellOverride []string
	// settingsOpen means the tab of the settings is open.
	settingsOpen bool
	// explorerCtx is a context at the place of the explorer panel, from its
	// last build: menus and dialogs opened from it while it is the flyout
	// keep the flyout out (PLAN §5 #21).
	explorerCtx   core.BuildContext
	explorerCtxOK bool
	// trash, if set, replaces trash.Move, for tests.
	trash func(path string) error
	// timers, if set, replaces time.AfterFunc, for tests.
	timers func(d time.Duration, fn func())
	// trace measures the start, when the environment asks (see startup.go).
	trace *startupTrace
	// ctx is a context in the window under the overlay, for dialogs.
	ctx   core.BuildContext
	ctxOK bool
}

// newApp makes the application with nothing open.
func newApp(sh shell) *app {
	a := &app{shell: sh, tabs: make(map[string]*tab), terms: make(map[string]*terminal), fontSize: config.DefaultFontSize, settings: config.Default()}
	a.dock.SetArrangement(defaultArrangement())
	return a
}

// refresh builds the window again.
func (a *app) refresh() {
	if a.view != nil {
		a.view.SetState(func() {})
	}
	a.refreshStatus()
}

// refreshStatus builds the status bar again.
func (a *app) refreshStatus() {
	if a.status != nil {
		a.status.SetState(func() {})
	}
}

// currentTab returns the tab the user works in, or nil.
func (a *app) currentTab() *tab { return a.tabs[a.current] }

// windowTitle is what the title bar of the OS says.
func (a *app) windowTitle() string {
	title := "Nople"
	if a.ws != nil {
		title = a.ws.Name() + " — " + title
	}
	if t := a.currentTab(); t != nil {
		title = tabTitle(t) + " — " + title
	}
	return title
}

// --- the folder

// openFolder opens the folder at dir in place of the one open now, closing
// its documents.
func (a *app) openFolder(dir string) error { return a.openFolderAs(dir, true) }

// openFolderAs opens the folder dir, sliding the explorer out if
// showExplorer.
func (a *app) openFolderAs(dir string, showExplorer bool) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("%s is not a folder", abs)
	}
	a.closeAll()
	a.ws = workspace.New(abs, a.settings.Gitignore)
	a.tree = collection.TreeController[string]{}
	a.loadDir(abs)
	a.listFiles()
	a.readRepo()
	a.search(searchOptions{})
	a.startWatching()
	switch {
	case !showExplorer:
	case slices.ContainsFunc(a.dock.Arrangement().Hidden, func(h nav.DockHidden) bool { return h.ID == explorerID }):
		a.shell.post(func() { a.dock.Show(explorerID) })
	default:
		a.dock.Activate(explorerID)
	}
	a.refresh()
	return nil
}

// closeFolder closes the folder and its documents.
func (a *app) closeFolder() {
	a.closeAll()
	a.ws = nil
	a.listFiles()
	a.readRepo()
	if a.watch != nil {
		a.startWatching()
	}
	a.search(searchOptions{})
	a.refresh()
}

// openFolderDialog asks for a folder to open.
func (a *app) openFolderDialog() {
	d := gowaku.FileDialog{Title: T("common.openFolder"), Directory: true}
	if a.ws != nil {
		d.Folder = filepath.Dir(a.ws.Root)
	}
	a.shell.fileDialog(d, func(paths []string, err error) {
		if err != nil || len(paths) == 0 {
			return
		}
		a.openFolder(paths[0])
	})
}

// loadDir reads the folder dir off the UI goroutine, unless it is read or
// being read.
func (a *app) loadDir(dir string) {
	ws := a.ws
	if ws == nil || ws.Loading[dir] || ws.Loaded(dir) {
		return
	}
	ws.Loading[dir] = true
	root, rules, gitignore := ws.Root, slices.Clip(ws.Rules), ws.Gitignore
	a.shell.async(func() func() {
		l := workspace.ListDir(root, dir, rules, gitignore)
		return func() {
			if a.ws != ws {
				return
			}
			ws.SetListing(l)
			a.watchDir(l.Dir)
			a.refresh()
		}
	})
}

// reloadFolder reads the open folders again.
func (a *app) reloadFolder() {
	ws := a.ws
	if ws == nil {
		return
	}
	fresh := workspace.New(ws.Root, a.settings.Gitignore)
	a.ws = fresh
	a.loadDir(fresh.Root)
	a.listFiles()
	a.readRepo()
	a.startWatching()
	for dir := range ws.Children {
		if dir != fresh.Root && a.tree.Expanded(dir) {
			a.loadDir(dir)
		}
	}
	a.refresh()
}

// followGitignore reads the open folder again after the setting of .gitignore
// changed, so the explorer, quick open and the search follow it at once.
func (a *app) followGitignore() {
	if a.ws == nil {
		return
	}
	a.reloadFolder()
	if a.find.opts.query != "" {
		a.search(a.find.opts)
	}
}

// --- the view

// setWrap breaks long lines or lets them run on.
func (a *app) setWrap(on bool) {
	a.wrap = on
	a.refresh()
}

// zoom changes the size of the text by steps; zero goes back to the default.
func (a *app) zoom(step int) {
	switch {
	case step == 0:
		a.fontSize = config.DefaultFontSize
	case step > 0:
		a.fontSize = min(a.fontSize+1, maxFontSize)
	default:
		a.fontSize = max(a.fontSize-1, minFontSize)
	}
	a.refresh()
}

// showPanel shows the sidebar panel id: slid out if it is hidden, selected
// if it is docked.
func (a *app) showPanel(id string) { a.dock.Activate(id) }

// samePath reports whether a and b name the same file.
func samePath(a, b string) bool {
	if a == b {
		return true
	}
	ca, errA := filepath.Abs(a)
	cb, errB := filepath.Abs(b)
	return errA == nil && errB == nil && pathEqual(ca, cb)
}
