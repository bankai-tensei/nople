package pty

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// PTY is a shell running in a pseudo console of Windows (ConPTY): what the
// shell writes comes out of Read as text with VT escapes, and what is written
// to it the shell reads as typed.
type PTY struct {
	hpc     windows.Handle
	in      *os.File // to the shell
	out     *os.File // from the shell
	process windows.Handle
	once    sync.Once
}

// DefaultShell is the shell a terminal starts: PowerShell 7 if it is
// installed, else Windows PowerShell.
func DefaultShell() []string {
	if p, err := exec.LookPath("pwsh.exe"); err == nil {
		return []string{p, "-NoLogo"}
	}
	return []string{"powershell.exe", "-NoLogo"}
}

// Start runs argv in dir in a pseudo console of cols by rows.
func Start(argv []string, dir string, cols, rows int) (*PTY, error) {
	var ptyIn, inWrite, outRead, ptyOut windows.Handle
	if err := windows.CreatePipe(&ptyIn, &inWrite, nil, 0); err != nil {
		return nil, err
	}
	if err := windows.CreatePipe(&outRead, &ptyOut, nil, 0); err != nil {
		windows.CloseHandle(ptyIn)
		windows.CloseHandle(inWrite)
		return nil, err
	}
	var hpc windows.Handle
	size := windows.Coord{X: int16(max(cols, 1)), Y: int16(max(rows, 1))}
	err := windows.CreatePseudoConsole(size, ptyIn, ptyOut, 0, &hpc)
	// The console has its own copies of its ends of the pipes.
	windows.CloseHandle(ptyIn)
	windows.CloseHandle(ptyOut)
	if err != nil {
		windows.CloseHandle(inWrite)
		windows.CloseHandle(outRead)
		return nil, err
	}
	p := &PTY{hpc: hpc, in: os.NewFile(uintptr(inWrite), "pty-in"), out: os.NewFile(uintptr(outRead), "pty-out")}

	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		p.Close()
		return nil, err
	}
	defer attrs.Delete()
	// The attribute is the handle itself, not a pointer to it.
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, *(*unsafe.Pointer)(unsafe.Pointer(&hpc)), unsafe.Sizeof(hpc)); err != nil {
		p.Close()
		return nil, err
	}
	si := windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.Cb = uint32(unsafe.Sizeof(si))
	// Null standard handles, so that the shell takes those of the console
	// and not the ones of this process, which may be pipes (as under go
	// test).
	si.Flags |= windows.STARTF_USESTDHANDLES
	line, err := windows.UTF16PtrFromString(commandLine(argv))
	if err != nil {
		p.Close()
		return nil, err
	}
	var wdir *uint16
	if dir != "" {
		if wdir, err = windows.UTF16PtrFromString(dir); err != nil {
			p.Close()
			return nil, err
		}
	}
	var pi windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT)
	if err := windows.CreateProcess(nil, line, nil, nil, false, flags, nil, wdir, &si.StartupInfo, &pi); err != nil {
		p.Close()
		return nil, err
	}
	windows.CloseHandle(pi.Thread)
	p.process = pi.Process
	return p, nil
}

// commandLine joins argv as Windows parses a command line.
func commandLine(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = windows.EscapeArg(a)
	}
	return strings.Join(parts, " ")
}

// Read reads what the shell wrote. It returns an error once the console
// closed.
func (p *PTY) Read(b []byte) (int, error) { return p.out.Read(b) }

// Write sends input to the shell.
func (p *PTY) Write(b []byte) (int, error) { return p.in.Write(b) }

// Resize tells the console its new size.
func (p *PTY) Resize(cols, rows int) error {
	return windows.ResizePseudoConsole(p.hpc, windows.Coord{X: int16(max(cols, 1)), Y: int16(max(rows, 1))})
}

// Wait waits for the shell to end.
func (p *PTY) Wait() error {
	if p.process == 0 {
		return errors.New("no process")
	}
	_, err := windows.WaitForSingleObject(p.process, windows.INFINITE)
	return err
}

// Close ends the console, which ends the shell, and frees what it holds.
func (p *PTY) Close() error {
	p.once.Do(func() {
		if p.hpc != 0 {
			windows.ClosePseudoConsole(p.hpc)
		}
		p.in.Close()
		if p.process != 0 {
			windows.TerminateProcess(p.process, 1)
			windows.CloseHandle(p.process)
		}
		// out closes last: closing the console flushes into it.
		p.out.Close()
	})
	return nil
}
