// Package app is the window of Nople: the explorer, the tabs of documents,
// the panels of search, source control and the terminal, and the menus that
// drive them. The parts that do not draw live in the other packages of
// internal: the terminal emulator, the diff, git, the highlighter, the
// translations, the settings and so on.
package app

import (
	"github.com/bankai-tensei/nople/internal/config"
	"github.com/bankai-tensei/nople/internal/i18n"
	"github.com/bankai-tensei/nople/internal/macopen"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bankai-tensei/gowaku"
)

// Options are what the command line asks of a run.
type Options struct {
	// Path is the folder to open, or a file to open in a tab over what was
	// open last time; empty opens what was open last time.
	Path string
	// Shell overrides the shell of the settings, with its arguments.
	Shell string
	// Fresh starts with nothing open and keeps no session.
	Fresh bool
	// QuitAfter quits on its own after this long, if it is not zero.
	QuitAfter time.Duration
}

// Run shows the window until it closes.
func Run(o Options) error {
	if o.QuitAfter > 0 {
		time.AfterFunc(o.QuitAfter, gowaku.Quit)
	}
	sh := &gowakuShell{}
	a := newApp(sh)
	a.trace = newStartupTrace()
	// The folder opens once Run runs, as reading it posts back to the UI.
	if o.Path != "" {
		if st, err := os.Stat(o.Path); err == nil && !st.IsDir() {
			a.startFile, _ = filepath.Abs(o.Path)
		} else {
			a.startFolder = o.Path
		}
	}
	a.settingsPath = config.DefaultPath()
	if o.Shell != "" {
		a.shellOverride = strings.Fields(o.Shell)
	}
	a.applySettings()
	a.wrap, a.fontSize = a.settings.Wrap, a.settings.FontSize
	a.startWatching()
	if !o.Fresh {
		a.sessionPath = defaultSessionPath()
	}
	// Finder hands over what to open in Apple Events, not on the command
	// line; the first may come before the window is built.
	macopen.Listen(a.openFromOS)
	err := gowaku.Run(gowaku.App{
		Title: a.windowTitle(), Width: windowWidth, Height: windowHeight,
		// The app_id or WM_CLASS that matches the window with nople.desktop.
		AppID:          "nople",
		Icon:           windowIcon(),
		Locale:         i18n.Locale(),
		Home:           windowView{app: a},
		OnCloseRequest: a.confirmClose,
	})
	if err := a.saveSession(); err != nil {
		log.Printf("nople: the session was not saved: %v", err)
	}
	a.stopWatching()
	a.closeTerminals()
	return err
}
