package safewrite

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

const isWindows = true

// keepOwner does nothing on Windows, where the new file takes the security of
// its folder, as a file saved by any program does.
func keepOwner(*os.File, os.FileInfo) {}

// keepAttributes gives the file at tmp the hidden, system and not-indexed
// attributes of target, which a rename would lose.
func keepAttributes(tmp, target string) {
	tp, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return
	}
	old, err := windows.GetFileAttributes(tp)
	if err != nil {
		return
	}
	keep := old & (windows.FILE_ATTRIBUTE_HIDDEN | windows.FILE_ATTRIBUTE_SYSTEM | windows.FILE_ATTRIBUTE_NOT_CONTENT_INDEXED)
	if keep == 0 {
		return
	}
	np, err := windows.UTF16PtrFromString(tmp)
	if err != nil {
		return
	}
	cur, err := windows.GetFileAttributes(np)
	if err != nil {
		return
	}
	_ = windows.SetFileAttributes(np, cur|keep) //nolint:errcheck // the attributes are a courtesy
}

// links returns the number of hard links of the file at path, or 0.
func links(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info); err != nil {
		return 0
	}
	return int(info.NumberOfLinks)
}

// syncDir does nothing on Windows, where a rename is kept with the file.
func syncDir(string) {}

// rename renames old to new, trying again for a while when the file is in
// use: a virus scanner, the indexer of Windows or a watch of Nople's own may
// hold it open for a moment, and Windows does not let an open file be
// replaced.
func rename(old, new string) error {
	var err error
	for try := 0; try < 40; try++ {
		if err = os.Rename(old, new); err == nil || !inUse(err) {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
	return err
}

// inUse reports whether err says a file is held open by another program.
func inUse(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
