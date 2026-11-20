// Package git reads the Git repository of the open folder by running git:
// the branch and the files that changed.
package git

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bankai-tensei/nople/internal/sysutil"
)

// Repo is the Git repository the open folder is in, as git last told.
type Repo struct {
	// Root is the top of the work tree.
	Root string
	// Branch is the name of the branch, or a short hash when the head is
	// detached; empty before the first commit is read.
	Branch string
	// Staged and Unstaged are the changes of the index and of the work
	// tree, untracked files with the second, each by path.
	Staged, Unstaged []Change
	// ByPath is the state of each changed file, for the explorer, the
	// worst of its two.
	ByPath map[string]byte
}

// Change is a file that differs.
type Change struct {
	// Path is the file now; Orig the one it was renamed or copied from.
	Path, Orig string
	// Code is git's letter: M, A, D, R, C, T, or U for an untracked file and
	// ! for a conflict.
	Code   byte
	Staged bool
}

// ErrNoGit means git is not installed or is not on the PATH.
var ErrNoGit = errors.New("git: not found")

// Run runs git in dir and returns what it wrote. It takes none of the locks
// git takes only to save work for later, as status does to refresh the index:
// Nople only reads, and the lock file status makes and removes in .git would
// show as a change in the folder Nople watches, which reads git again, on and
// on.
func Run(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", dir, "--no-optional-locks", "-c", "core.quotepath=off"}, args...)...)
	sysutil.HideWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, ErrNoGit
	}
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return out, errors.New(msg)
		}
		return out, err
	}
	return out, nil
}

// Read finds the repository of the folder dir and reads its state. It
// returns nil, nil for a folder that is in no repository.
func Read(dir string) (*Repo, error) {
	out, err := Run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		if errors.Is(err, ErrNoGit) {
			return nil, err
		}
		return nil, nil
	}
	top := rootAs(filepath.Clean(dir), filepath.Clean(strings.TrimSpace(string(out))))
	r := &Repo{Root: top}
	out, err = Run(r.Root, "status", "--porcelain=v2", "--branch", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	r.parseStatus(out)
	return r, nil
}

// parseStatus reads the output of git status --porcelain=v2 --branch -z.
func (r *Repo) parseStatus(out []byte) {
	r.ByPath = map[string]byte{}
	var oid string
	fields := strings.Split(string(out), "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if f == "" {
			continue
		}
		abs := func(p string) string { return filepath.Join(r.Root, filepath.FromSlash(p)) }
		switch f[0] {
		case '#':
			if v, ok := strings.CutPrefix(f, "# branch.head "); ok && v != "(detached)" {
				r.Branch = v
			}
			if v, ok := strings.CutPrefix(f, "# branch.oid "); ok && v != "(initial)" {
				oid = v
			}
		case '1', '2':
			// 1 XY sub mH mI mW hH hI path; 2 adds a score, and the old path
			// in the next field.
			n := 9
			if f[0] == '2' {
				n = 10
			}
			parts := strings.SplitN(f, " ", n)
			if len(parts) < n {
				continue
			}
			xy, path := parts[1], abs(parts[n-1])
			orig := ""
			if f[0] == '2' && i+1 < len(fields) {
				i++
				orig = abs(fields[i])
			}
			if xy[0] != '.' {
				r.Staged = append(r.Staged, Change{Path: path, Orig: orig, Code: xy[0], Staged: true})
			}
			if xy[1] != '.' {
				r.Unstaged = append(r.Unstaged, Change{Path: path, Code: xy[1]})
			}
		case 'u':
			parts := strings.SplitN(f, " ", 11)
			if len(parts) == 11 {
				r.Unstaged = append(r.Unstaged, Change{Path: abs(parts[10]), Code: '!'})
			}
		case '?':
			r.Unstaged = append(r.Unstaged, Change{Path: abs(f[2:]), Code: 'U'})
		}
	}
	if r.Branch == "" && len(oid) >= 7 {
		r.Branch = oid[:7]
	}
	byPath := func(a, b Change) int { return strings.Compare(a.Path, b.Path) }
	slices.SortFunc(r.Staged, byPath)
	slices.SortFunc(r.Unstaged, byPath)
	for _, c := range append(slices.Clone(r.Staged), r.Unstaged...) {
		if worse(c.Code, r.ByPath[c.Path]) {
			r.ByPath[c.Path] = c.Code
		}
	}
}

// worse reports whether code a says more than b, for the one mark of a file:
// a conflict, then a deletion, then the rest.
func worse(a, b byte) bool {
	rank := func(c byte) int { return strings.IndexByte("MTRCAUD!", c) }
	return rank(a) > rank(b)
}

// Rel is the path of a file of the repository as git names it.
func (r *Repo) Rel(path string) string {
	rel, err := filepath.Rel(r.Root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// Show returns the text of a file at a revision: "HEAD", or "" for the index.
// A file that is not there reads as empty.
func (r *Repo) Show(rev, path string) (string, bool) {
	out, err := Run(r.Root, "show", rev+":"+r.Rel(path))
	if err != nil {
		return "", false
	}
	return string(out), true
}

// Tracked reports whether git tracks the file at path: it is in the index,
// not new, ignored or outside the repository.
func (r *Repo) Tracked(path string) bool {
	_, err := Run(r.Root, "ls-files", "--error-unmatch", "--", r.Rel(path))
	return err == nil
}

// rootAs spells top, the root of a repository as git names it, as the folder
// dir inside it is spelled, so that the paths of git are those of the
// explorer. On Windows they may differ in case, and dir may name a folder by
// its short name, as C:\Users\RUNNER~1, where git gives the long one.
func rootAs(dir, top string) string {
	if p, ok := prefixAs(dir, top); ok {
		return p
	}
	long, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return top
	}
	long = filepath.Clean(long)
	if _, ok := prefixAs(long, top); !ok {
		return top
	}
	// The part of the folder below the root, as the long name spells it,
	// ends dir too, as the short names are of the folders above it.
	below := long[len(top):]
	if len(below) <= len(dir) && strings.EqualFold(dir[len(dir)-len(below):], below) {
		return dir[:len(dir)-len(below)]
	}
	return top
}

// prefixAs returns the start of path that is the folder root, spelled as
// path spells it, when root holds path or is it.
func prefixAs(path, root string) (string, bool) {
	if len(root) > len(path) || !strings.EqualFold(path[:len(root)], root) {
		return "", false
	}
	if len(path) > len(root) && !os.IsPathSeparator(path[len(root)]) && !os.IsPathSeparator(root[len(root)-1]) {
		return "", false
	}
	return path[:len(root)], true
}
