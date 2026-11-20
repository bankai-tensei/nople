package app

import (
	"sync"

	"github.com/bankai-tensei/gowaku"
	"github.com/bankai-tensei/gowaku/core"
)

// Size of a new window.
const (
	windowWidth  = 1280
	windowHeight = 800
)

// shell is what the application asks of gowaku outside its widgets, so that
// the tests run without Run.
type shell interface {
	// post runs fn on the UI goroutine. It is safe to call from any goroutine.
	post(fn func())
	// async runs work off the UI goroutine, then what it returns on it.
	async(work func() (done func()))
	fileDialog(d gowaku.FileDialog, done func(paths []string, err error))
	setTitle(ctx core.BuildContext, title string)
	// openURL opens an address of the web in the browser of the OS.
	openURL(u string)
	// requestClose asks the window to close, as its close button does.
	requestClose()
	quit()
}

// gowakuShell is the shell of Run.
type gowakuShell struct {
	// window is the window, once a build told it.
	window *gowaku.WindowHandle
}

func (*gowakuShell) post(fn func()) { gowaku.Post(fn) }

func (*gowakuShell) async(work func() func()) {
	go func() {
		done := work()
		gowaku.Post(done)
	}()
}

// fileDialog shows the dialog in front of the window, which a build told.
func (s *gowakuShell) fileDialog(d gowaku.FileDialog, done func([]string, error)) {
	if s.window == nil {
		done(nil, gowaku.ErrNoFileDialog)
		return
	}
	s.window.ShowFileDialog(d, done)
}

func (s *gowakuShell) setTitle(ctx core.BuildContext, title string) {
	h := gowaku.WindowOf(ctx)
	s.window = h
	// The title changes after the build, not during it.
	gowaku.Post(func() { h.SetTitle(title) })
}

func (*gowakuShell) quit() { gowaku.Quit() }

func (*gowakuShell) openURL(u string) { _ = gowaku.OpenURL(u) }

func (s *gowakuShell) requestClose() {
	if s.window != nil {
		s.window.RequestClose()
	}
}

// localShell runs the application without Run, for tests: what gowaku would
// post waits in a queue until flush, work meant for another goroutine runs at
// once, and the file dialog answers with dialogPaths.
type localShell struct {
	mu    sync.Mutex
	queue []func()

	dialogs        []gowaku.FileDialog
	dialogPaths    []string
	title          string
	urls           []string
	closeRequested bool
	quitted        bool
}

func (l *localShell) post(fn func()) {
	l.mu.Lock()
	l.queue = append(l.queue, fn)
	l.mu.Unlock()
}

func (l *localShell) async(work func() func()) { l.post(work()) }

func (l *localShell) fileDialog(d gowaku.FileDialog, done func([]string, error)) {
	l.dialogs = append(l.dialogs, d)
	paths := l.dialogPaths
	l.post(func() { done(paths, nil) })
}

func (l *localShell) setTitle(_ core.BuildContext, title string) { l.title = title }

func (l *localShell) quit() { l.quitted = true }

// flush runs what was posted, and what that posts in turn, and reports
// whether there was anything.
func (l *localShell) flush() bool {
	ran := false
	for range 100 {
		l.mu.Lock()
		q := l.queue
		l.queue = nil
		l.mu.Unlock()
		if len(q) == 0 {
			return ran
		}
		for _, fn := range q {
			fn()
		}
		ran = true
	}
	return ran
}
func (l *localShell) openURL(u string) { l.urls = append(l.urls, u) }
func (l *localShell) requestClose()    { l.closeRequested = true }
