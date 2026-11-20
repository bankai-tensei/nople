//go:build linux || darwin

package pty

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

// PTY is a shell running on a pseudo terminal: what the shell writes comes
// out of Read, and what is written to it the shell reads as typed.
type PTY struct {
	master *os.File
	cmd    *exec.Cmd
}

// DefaultShell is the shell a terminal starts: $SHELL, else /bin/sh.
func DefaultShell() []string {
	if s := os.Getenv("SHELL"); s != "" {
		return []string{s}
	}
	return []string{"/bin/sh"}
}

// Start runs argv in dir on a pseudo terminal of cols by rows.
func Start(argv []string, dir string, cols, rows int) (*PTY, error) {
	master, slaveName, err := openPTY()
	if err != nil {
		return nil, err
	}
	slave, err := os.OpenFile(slaveName, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close() //nolint:errcheck // the error of the start is the one to report
		return nil, err
	}
	defer slave.Close() //nolint:errcheck // the shell has its own copy
	// A size that does not take leaves the terminal at its default size.
	_ = setSize(master, cols, rows) //nolint:errcheck // see above
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		_ = master.Close() //nolint:errcheck // the error of the start is the one to report
		return nil, err
	}
	return &PTY{master: master, cmd: cmd}, nil
}

func setSize(f *os.File, cols, rows int) error {
	return unix.IoctlSetWinsize(int(f.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(max(cols, 1)), Row: uint16(max(rows, 1))})
}

// Read reads what the shell wrote.
func (p *PTY) Read(b []byte) (int, error) { return p.master.Read(b) }

// Write sends input to the shell.
func (p *PTY) Write(b []byte) (int, error) { return p.master.Write(b) }

// Resize tells the terminal its new size, which signals the shell.
func (p *PTY) Resize(cols, rows int) error { return setSize(p.master, cols, rows) }

// Wait waits for the shell to end.
func (p *PTY) Wait() error { return p.cmd.Wait() }

// Close ends the shell and the terminal.
func (p *PTY) Close() error {
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Signal(syscall.SIGHUP) //nolint:errcheck // the shell may have ended already
	}
	return p.master.Close()
}
