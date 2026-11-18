package safewrite

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// only checks that the folder holds just the names given.
func only(t *testing.T, dir string, names ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(names) {
		var got []string
		for _, e := range entries {
			got = append(got, e.Name())
		}
		t.Errorf("the folder holds %v, want %v", got, names)
	}
}

func TestWriteFile_NewAndReplaced(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	if err := WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("second, longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != "second, longer" {
		t.Errorf("got %q", got)
	}
	only(t, dir, "doc.txt")
	if runtime.GOOS != "windows" {
		// The mode of the file stays, not the one asked for a new file.
		if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
			t.Errorf("mode %v, want 0600", st.Mode().Perm())
		}
	}
}

func TestWriteFile_ACutWriteLeavesTheFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(path, []byte("as it was"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A write that stops before Commit, as a crash would.
	tmp, err := Temp(path, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	tmp.WriteString("half of the new")
	Abort(tmp)
	if got := read(t, path); got != "as it was" {
		t.Errorf("got %q", got)
	}
	only(t, dir, "doc.txt")
}

func TestWriteFile_FollowsASymbolicLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	link := filepath.Join(dir, "link.txt")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
	if err := WriteFile(link, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := read(t, target); got != "new" {
		t.Errorf("the target holds %q", got)
	}
	if st, err := os.Lstat(link); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link should stay a link: %v %v", st.Mode(), err)
	}
}

func TestWriteFile_KeepsHardLinks(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(a, b); err != nil {
		t.Skipf("no hard links here: %v", err)
	}
	if err := WriteFile(a, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if read(t, a) != "new" || read(t, b) != "new" {
		t.Errorf("both links should hold the new text: %q %q", read(t, a), read(t, b))
	}
}

func TestWriteFile_AFileHeldOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	// On Windows, Go opens files without FILE_SHARE_DELETE, so that the
	// rename cannot replace it, and the file is written in place.
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := WriteFile(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != "new" {
		t.Errorf("got %q", got)
	}
	only(t, dir, "doc.txt")
}
