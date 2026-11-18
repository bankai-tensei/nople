package app

import (
	"log"
	"os"
	"path/filepath"
)

// openFromOS opens the paths that the OS hands over while Nople runs, as
// Finder does on macOS (internal/macopen). It runs on the UI goroutine.
// Before the window is built, the first path takes the place of the one on
// the command line and the other files wait in startMore; afterwards they
// open at once.
func (a *app) openFromOS(paths []string) {
	if len(paths) == 0 {
		return
	}
	if a.view == nil {
		first := paths[0]
		if st, err := os.Stat(first); err == nil && !st.IsDir() {
			a.startFolder, a.startFile = "", first
		} else {
			a.startFolder, a.startFile = first, ""
		}
		a.startMore = append(a.startMore, paths[1:]...)
		return
	}
	a.shell.post(func() { a.openPaths(paths) })
}

// openPaths opens each of paths: a folder in place of the one open now, a
// file in a tab.
func (a *app) openPaths(paths []string) {
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		st, err := os.Stat(abs)
		switch {
		case err != nil:
			log.Printf("nople: %v", err)
		case st.IsDir():
			if err := a.openFolder(abs); err != nil {
				log.Printf("nople: %v", err)
			}
		default:
			a.openFile(abs, false)
		}
	}
}
