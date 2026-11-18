//go:build unix

package bigfile

import (
	"os"
	"syscall"
)

// holdOpen says whether a File stays open while it is shown. It does here:
// the file can be renamed over and deleted while open, and the text still
// reads the file it opened.
const holdOpen = true

// dup returns a file of its own over the open file f, for another goroutine,
// which closing f does not close.
func dup(f *os.File) (*os.File, error) {
	fd, err := syscall.Dup(int(f.Fd()))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), f.Name()), nil
}
