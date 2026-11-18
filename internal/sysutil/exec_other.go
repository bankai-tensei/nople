//go:build !windows

package sysutil

import "os/exec"

// HideWindow does nothing outside Windows, where a console program opens no
// window.
func HideWindow(*exec.Cmd) {}
