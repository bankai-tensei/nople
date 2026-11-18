//go:build !windows

package trash

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Move moves the file or folder at path to the trash: ~/.Trash on
// macOS, the trash of freedesktop.org elsewhere, with the note that lets the
// file manager put it back.
func Move(path string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if runtime.GOOS == "darwin" {
		return os.Rename(abs, freeName(filepath.Join(home, ".Trash"), filepath.Base(abs)))
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		base = filepath.Join(home, ".local", "share")
	}
	files, info := filepath.Join(base, "Trash", "files"), filepath.Join(base, "Trash", "info")
	if err := os.MkdirAll(files, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(info, 0o700); err != nil {
		return err
	}
	to := freeName(files, filepath.Base(abs))
	note := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n",
		(&url.URL{Path: abs}).EscapedPath(), time.Now().Format("2006-01-02T15:04:05"))
	if err := os.WriteFile(filepath.Join(info, filepath.Base(to)+".trashinfo"), []byte(note), 0o600); err != nil {
		return err
	}
	return os.Rename(abs, to)
}

// freeName is dir/name, or with a number before the extension if that is
// taken.
func freeName(dir, name string) string {
	p := filepath.Join(dir, name)
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 2; ; i++ {
		if _, err := os.Lstat(p); os.IsNotExist(err) {
			return p
		}
		p = filepath.Join(dir, fmt.Sprintf("%s %d%s", stem, i, ext))
	}
}
