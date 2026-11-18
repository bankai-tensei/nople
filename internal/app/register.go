package app

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/bankai-tensei/nople/internal/config"
	"github.com/bankai-tensei/nople/internal/fileicon"
	"github.com/bankai-tensei/nople/internal/i18n"
	"github.com/bankai-tensei/nople/internal/shellreg"
)

// RegisterShell puts the running nople.exe into the shell of the current
// user, in the language of the settings: "Open with Nople" on the context
// menus, and Nople among the programs that open text and source files, each
// extension with an icon of its type, which it writes to shellIconDir.
func RegisterShell() error {
	if runtime.GOOS != "windows" {
		// Only Windows has this shell; shellreg says so.
		return shellreg.Register("", shellreg.Labels{}, "")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	dir, err := shellIconDir()
	if err != nil {
		return err
	}
	if err := writeShellIcons(dir); err != nil {
		return err
	}
	s, _ := config.Load(config.DefaultPath())
	i18n.Set(s.Language)
	return shellreg.Register(exe, shellreg.Labels{
		Open:        T("shell.open"),
		Description: T("welcome.tagline"),
		TypeName: func(ext string) string {
			return T("shell.fileType", strings.ToUpper(strings.TrimPrefix(ext, ".")))
		},
	}, dir)
}

// UnregisterShell takes out of the shell what RegisterShell put there, and
// the icons it wrote.
func UnregisterShell() error {
	err := shellreg.Unregister()
	if dir, derr := shellIconDir(); derr == nil {
		err = errors.Join(err, os.RemoveAll(dir))
	}
	return err
}

// shellIconDir is the folder of the icons of the file types, in the local
// application data of the user: %LOCALAPPDATA%\Nople\FileIcons.
func shellIconDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Nople", "FileIcons"), nil
}

// writeShellIcons writes the icon of every extension of shellreg, and that
// of a file of no kind, into dir.
func writeShellIcons(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, ext := range append([]string{""}, shellreg.Extensions...) {
		if err := os.WriteFile(shellreg.IconFile(dir, ext), fileicon.ShellIcon(ext), 0o644); err != nil {
			return err
		}
	}
	return nil
}
