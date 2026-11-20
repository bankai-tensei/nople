package pty

import (
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

// openPTY opens a new pseudo terminal, and returns its master and the path
// of its slave.
func openPTY() (*os.File, string, error) {
	f, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", err
	}
	if err := unix.IoctlSetPointerInt(int(f.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		_ = f.Close() //nolint:errcheck // the error of the ioctl is the one to report
		return nil, "", err
	}
	n, err := unix.IoctlGetInt(int(f.Fd()), unix.TIOCGPTN)
	if err != nil {
		_ = f.Close() //nolint:errcheck // the error of the ioctl is the one to report
		return nil, "", err
	}
	return f, "/dev/pts/" + strconv.Itoa(n), nil
}
