// Package macopen hears the files and folders that Finder asks Nople to
// open on macOS: through "Open With", a double click once Nople is the
// default, or a drop on the icon in the Dock. Finder does not put them on
// the command line as the other systems do, but sends an Apple Event, to the
// app that it starts or to the one already running.
package macopen
