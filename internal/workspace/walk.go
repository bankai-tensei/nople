package workspace

import (
	"os"
	"slices"
)

// maxWalkDepth bounds how deep walkFiles goes, against links that loop.
const maxWalkDepth = 64

// Walk calls visit with every file of the workspace at root, folder by
// folder in the order of the explorer, leaving out what its .gitignore files
// ignore if gitignore. It stops when visit or stop returns true. It runs off
// the UI goroutine.
func Walk(root string, gitignore bool, stop func() bool, visit func(path string) bool) {
	var walk func(dir string, rules IgnoreRules, depth int) bool
	walk = func(dir string, rules IgnoreRules, depth int) bool {
		if depth > maxWalkDepth || stop() {
			return true
		}
		l := ListDir(root, dir, rules, gitignore)
		rules = append(slices.Clip(rules), l.Rules...)
		for _, p := range l.Entries {
			if !l.Dirs[p] {
				continue
			}
			// A link to a folder is not followed: it may loop, and what it
			// points at is usually elsewhere in the tree.
			if st, err := os.Lstat(p); err == nil && st.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if walk(p, rules, depth+1) {
				return true
			}
		}
		for _, p := range l.Entries {
			if !l.Dirs[p] && visit(p) {
				return true
			}
		}
		return false
	}
	walk(root, nil, 0)
}
