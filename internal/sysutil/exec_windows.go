package sysutil

import (
	"os/exec"
	"syscall"
)

// HideWindow keeps a console program such as git from opening a console
// window of its own, which a program without one, like nople.exe, would show.
func HideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
