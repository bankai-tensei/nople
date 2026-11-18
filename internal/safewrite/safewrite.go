// Package safewrite writes a file so that a write that fails halfway, or a
// crash or a power cut in the middle of it, leaves the file as it was: the
// new contents go to a temporary file beside it, which is synced to the disk
// and then takes the place of the file in one rename.
//
// The new file keeps what the old one had that the rename would lose: its
// permissions, its owner where the process may set it, and on Windows its
// hidden and system attributes. A symbolic link is followed, so that the file
// it points to is replaced and the link stays. A file with other hard links,
// which a rename would part from them, is written in place, as is a file the
// rename cannot replace, as one that another program holds open on Windows.
package safewrite

import (
	"os"
	"path/filepath"
)

// Target returns the file that writing to path changes: the file a symbolic
// link at path points to, or path itself.
func Target(path string) string {
	if p, err := filepath.EvalSymlinks(path); err == nil {
		return p
	}
	return path
}

// Temp makes the temporary file for writing target, in its folder, and
// readies it to take the place of target: with the permissions of target, or
// perm for a new file, and its owner and attributes where it can.
func Temp(target string, perm os.FileMode) (*os.File, error) {
	tmp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".*.tmp")
	if err != nil {
		return nil, err
	}
	mode := perm
	st, statErr := os.Stat(target)
	if statErr == nil {
		mode = st.Mode().Perm()
	}
	// Windows takes only the read-only bit, and a file being written has it
	// off; elsewhere the mode counts.
	if err := tmp.Chmod(mode); err != nil && !isWindows {
		Abort(tmp)
		return nil, err
	}
	if statErr == nil {
		keepOwner(tmp, st)
	}
	return tmp, nil
}

// Commit syncs tmp to the disk, closes it, and makes it take the place of
// target. When the rename fails, as when another program holds target open on
// Windows, tmp goes, and Commit returns the error of the rename for the caller
// to decide.
func Commit(tmp *os.File, target string) error {
	if err := tmp.Sync(); err != nil {
		Abort(tmp)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name()) //nolint:errcheck // a file of ours left over at worst
		return err
	}
	keepAttributes(tmp.Name(), target)
	if err := rename(tmp.Name(), target); err != nil {
		_ = os.Remove(tmp.Name()) //nolint:errcheck // the error of Rename is the one to report
		return err
	}
	syncDir(filepath.Dir(target))
	return nil
}

// Abort closes and removes tmp.
func Abort(tmp *os.File) {
	_ = tmp.Close()           //nolint:errcheck // the error before is the one to report
	_ = os.Remove(tmp.Name()) //nolint:errcheck // a file of ours left over at worst
}

// SharesLinks reports whether the file at path has other hard links, which
// replacing it would part from it.
func SharesLinks(path string) bool { return links(path) > 1 }

// WriteFile writes data to the file at path, as os.WriteFile does but
// through a temporary file (see the package). A new file gets perm.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	target := Target(path)
	if SharesLinks(target) {
		return os.WriteFile(target, data, perm)
	}
	tmp, err := Temp(target, perm)
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		Abort(tmp)
		return err
	}
	if err := Commit(tmp, target); err != nil {
		// The file cannot be replaced, but may still be written: as before,
		// in place.
		return writeInPlace(target, data, perm)
	}
	return nil
}

// writeInPlace writes data over the file at path, and syncs it.
func writeInPlace(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close() //nolint:errcheck // the error of the write is the one to report
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close() //nolint:errcheck // the error of the sync is the one to report
		return err
	}
	return f.Close()
}
