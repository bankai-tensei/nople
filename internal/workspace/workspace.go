package workspace

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Workspace is the folder that is open, as far as the explorer has read it.
// It belongs to the UI goroutine; listing a folder runs off it, through
// listDir, and comes back with setListing.
type Workspace struct {
	Root string
	// Children are the entries of each folder read so far, by its path.
	Children map[string][]string
	// Dirs are the paths known to be folders.
	Dirs map[string]bool
	// Loading are the folders being read now.
	Loading map[string]bool
	// Failed are the folders that could not be read, with why.
	Failed map[string]error
	// Rules are the .gitignore rules of the folders read so far.
	Rules IgnoreRules
	// Gitignore says whether what the .gitignore files ignore is left out.
	// The .git folder is left out either way.
	Gitignore bool
}

// New starts a workspace at root, with nothing read yet. gitignore says
// whether it leaves out what the .gitignore files ignore.
func New(root string, gitignore bool) *Workspace {
	return &Workspace{
		Root:      root,
		Gitignore: gitignore,
		Children:  make(map[string][]string),
		Dirs:      map[string]bool{root: true},
		Loading:   make(map[string]bool),
		Failed:    make(map[string]error),
	}
}

// Name is what the explorer calls the workspace.
func (w *Workspace) Name() string { return filepath.Base(w.Root) }

// rel returns path relative to the root, with slashes.
func (w *Workspace) Rel(path string) string {
	r, err := filepath.Rel(w.Root, path)
	if err != nil || r == "." {
		return ""
	}
	return filepath.ToSlash(r)
}

// IsDir reports whether path is a folder the workspace knows of.
func (w *Workspace) IsDir(path string) bool { return w.Dirs[path] }

// Loaded reports whether the folder at path has been read.
func (w *Workspace) Loaded(path string) bool {
	_, ok := w.Children[path]
	return ok
}

// Listing is what reading one folder found.
type Listing struct {
	Dir string
	// Entries are the paths in the folder that are not ignored: folders
	// first, then files, each by name.
	Entries []string
	Dirs    map[string]bool
	// Rules are those of the .gitignore in the folder, if it has one.
	Rules IgnoreRules
	Err   error
}

// alwaysHidden are names the explorer never shows.
var alwaysHidden = map[string]bool{".git": true}

// ListDir reads the folder dir of the workspace at root. If gitignore, it
// leaves out what the rules, with those of its own .gitignore, ignore;
// otherwise it reads no .gitignore and leaves out only the .git folder. It
// runs off the UI goroutine, so it takes the rules as they were when it
// started.
func ListDir(root, dir string, rules IgnoreRules, gitignore bool) Listing {
	l := Listing{Dir: dir, Dirs: make(map[string]bool)}
	ents, err := os.ReadDir(dir)
	if err != nil {
		l.Err = err
		return l
	}
	relDir := ""
	if r, err := filepath.Rel(root, dir); err == nil && r != "." {
		relDir = filepath.ToSlash(r)
	}
	if !gitignore {
		rules = nil
	} else if data, err := os.ReadFile(filepath.Join(dir, ".gitignore")); err == nil {
		l.Rules = parseIgnore(relDir, data)
		rules = append(slices.Clip(rules), l.Rules...)
	}
	type entry struct {
		path string
		dir  bool
	}
	var list []entry
	for _, e := range ents {
		name := e.Name()
		if alwaysHidden[name] {
			continue
		}
		p := filepath.Join(dir, name)
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			if st, err := os.Stat(p); err == nil {
				isDir = st.IsDir()
			}
		}
		rel := name
		if relDir != "" {
			rel = relDir + "/" + name
		}
		if rules.ignored(rel, isDir) {
			continue
		}
		list = append(list, entry{p, isDir})
	}
	slices.SortFunc(list, func(a, b entry) int {
		if a.dir != b.dir {
			if a.dir {
				return -1
			}
			return 1
		}
		an, bn := filepath.Base(a.path), filepath.Base(b.path)
		if c := cmp.Compare(strings.ToLower(an), strings.ToLower(bn)); c != 0 {
			return c
		}
		return cmp.Compare(an, bn)
	})
	l.Entries = make([]string, len(list))
	for i, e := range list {
		l.Entries[i] = e.path
		if e.dir {
			l.Dirs[e.path] = true
		}
	}
	return l
}

// SetListing takes in what listDir found.
func (w *Workspace) SetListing(l Listing) {
	delete(w.Loading, l.Dir)
	if l.Err != nil {
		w.Failed[l.Dir] = l.Err
		w.Children[l.Dir] = nil
		return
	}
	delete(w.Failed, l.Dir)
	w.Children[l.Dir] = l.Entries
	for p := range l.Dirs {
		w.Dirs[p] = true
	}
	w.Rules = append(w.Rules, l.Rules...)
}
