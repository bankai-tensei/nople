// Command nople is a desktop editor for reading and writing code: it opens a
// folder, shows its files in a tree and each file in a tab. See README.md.
//
//	nople [folder|file]        opens the folder, or the file over what was open last time
//	nople -quit-after 5s       quits on its own
//	nople -shell "cmd.exe"     starts terminals with cmd.exe (also NOPLE_SHELL)
//	nople -fresh               starts with nothing open, and saves no session
//	nople -register            puts "Open with Nople" and the file types into the shell (Windows)
//	nople -unregister          takes them out again
//	nople -version             prints the version
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/bankai-tensei/nople/internal/app"
)

// version is the version of the build, which the packaging sets with
// -ldflags "-X main.version=1.2.3".
var version = "dev"

// The icon of nople.exe; see nople.rc. Needs windres (MinGW or TDM-GCC).
//
//go:generate windres -i nople.rc -o rsrc_windows_amd64.syso -O coff

func main() {
	var o app.Options
	var register, unregister, showVersion bool
	flag.DurationVar(&o.QuitAfter, "quit-after", 0, "quit after this long; 0 waits for the window to close")
	flag.BoolVar(&o.Fresh, "fresh", false, "start with nothing open, not with what was open last time")
	flag.StringVar(&o.Shell, "shell", os.Getenv("NOPLE_SHELL"), "the shell of the terminal, with its arguments; empty takes that of the settings")
	flag.BoolVar(&register, "register", false, `put "Open with Nople" and the file types Nople opens into the shell of the current user, then quit (Windows)`)
	flag.BoolVar(&unregister, "unregister", false, "take out of the shell what -register put there, then quit (Windows)")
	flag.BoolVar(&showVersion, "version", false, "print the version, then quit")
	flag.Parse()
	switch {
	case showVersion:
		fmt.Println("nople", version)
		os.Exit(0)
	case register:
		exit(app.RegisterShell())
	case unregister:
		exit(app.UnregisterShell())
	}
	// The shell hands a drive over as "C:\", which reaches os.Args as C:"
	// since the backslash escapes the quote; bare C: would be the current
	// folder of the drive.
	o.Path = flag.Arg(0)
	if p, ok := strings.CutSuffix(o.Path, `:"`); ok {
		o.Path = p + `:\`
	}
	if err := app.Run(o); err != nil {
		log.Fatal(err)
	}
}

// exit ends a run that only changed the shell. nople.exe has no console,
// so the outcome is also in its exit code.
func exit(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "nople:", err)
		os.Exit(1)
	}
	os.Exit(0)
}
