package pty

import (
	"bytes"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// openPTY opens a new pseudo terminal, and returns its master and the path
// of its slave.
func openPTY() (*os.File, string, error) {
	f, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", err
	}
	fd := f.Fd()
	if err := unix.IoctlSetInt(int(fd), unix.TIOCPTYGRANT, 0); err != nil {
		_ = f.Close() //nolint:errcheck // the error of the ioctl is the one to report
		return nil, "", err
	}
	if err := unix.IoctlSetInt(int(fd), unix.TIOCPTYUNLK, 0); err != nil {
		_ = f.Close() //nolint:errcheck // the error of the ioctl is the one to report
		return nil, "", err
	}
	var name [128]byte
	// x/sys has no wrapper for an ioctl that fills a buffer of the caller.
	if _, _, e := unix.Syscall(unix.SYS_IOCTL, fd, unix.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))); e != 0 { //nolint:staticcheck // see above
		_ = f.Close() //nolint:errcheck // the error of the ioctl is the one to report
		return nil, "", e
	}
	return f, string(name[:bytes.IndexByte(name[:], 0)]), nil
}
