//go:build unix

package safewrite

import (
	"os"
	"syscall"
)

const isWindows = false

// keepOwner gives tmp the owner and group of the file st describes, where the
// process may, as root may for the file of another user.
func keepOwner(tmp *os.File, st os.FileInfo) {
	if s, ok := st.Sys().(*syscall.Stat_t); ok {
		_ = tmp.Chown(int(s.Uid), int(s.Gid)) //nolint:errcheck // only root may, and only it needs to
	}
}

// keepAttributes does nothing here: the mode is all there is.
func keepAttributes(_, _ string) {}

// links returns the number of hard links of the file at path, or 0.
func links(path string) int {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if s, ok := st.Sys().(*syscall.Stat_t); ok {
		return int(s.Nlink)
	}
	return 0
}

// syncDir syncs the folder dir, so that the rename is on the disk too.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()  //nolint:errcheck // the file itself is synced; this is the name
		_ = d.Close() //nolint:errcheck // only read
	}
}

// rename renames from to to; a file held open can be renamed over here.
func rename(from, to string) error { return os.Rename(from, to) }
