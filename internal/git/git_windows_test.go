package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// shortName returns the short name of Windows for path, or "" when it has
// none.
func shortName(t *testing.T, path string) string {
	t.Helper()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetShortPathName(p, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// TestRead_AFolderByItsShortName reads a repository opened by the short name
// of its folder, as the temp folder of a CI runner is, and finds its files
// under that name.
func TestRead_AFolderByItsShortName(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	long := filepath.Join(t.TempDir(), "a folder with a long name")
	if err := os.MkdirAll(filepath.Join(long, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	short := shortName(t, long)
	if short == "" || short == long {
		t.Skip("short names are off on this volume")
	}
	if _, err := Run(long, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(long, "sub", "new.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{short, filepath.Join(short, "sub")} {
		r, err := Read(dir)
		if err != nil || r == nil {
			t.Fatalf("Read(%s): %v %v", dir, r, err)
		}
		if r.Root != short {
			t.Errorf("Read(%s): root %s, want %s", dir, r.Root, short)
		}
		if code := r.ByPath[filepath.Join(short, "sub", "new.txt")]; code != 'U' {
			t.Errorf("Read(%s): the new file is %q under the short name; ByPath %v", dir, code, r.ByPath)
		}
	}
}
