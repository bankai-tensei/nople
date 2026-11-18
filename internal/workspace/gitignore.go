package workspace

import (
	"bufio"
	"bytes"
	"path"
	"strings"
)

// ignoreRule is one line of a .gitignore file.
type ignoreRule struct {
	// base is the folder of the .gitignore, relative to the root of the
	// workspace with slashes, "" for the root itself.
	base string
	// segments are the pattern split at its slashes.
	segments []string
	// negate is a rule that starts with !, which takes a path back.
	negate bool
	// dirOnly is a rule that ends with /, which matches folders only.
	dirOnly bool
	// anchored is a rule with a slash before its end, which matches from base
	// rather than at any depth.
	anchored bool
}

// IgnoreRules are the rules of the .gitignore files read so far, in the order
// git applies them: those of a parent folder before those of its children, and
// within a file from top to bottom. The last rule that matches decides.
type IgnoreRules []ignoreRule

// parseIgnore reads the .gitignore of the folder base.
func parseIgnore(base string, data []byte) IgnoreRules {
	var rules IgnoreRules
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		// Trailing spaces end a pattern unless escaped with a backslash.
		for strings.HasSuffix(line, " ") && !strings.HasSuffix(line, "\\ ") {
			line = line[:len(line)-1]
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r := ignoreRule{base: base}
		if strings.HasPrefix(line, "!") {
			r.negate = true
			line = line[1:]
		} else if strings.HasPrefix(line, `\!`) || strings.HasPrefix(line, `\#`) {
			line = line[1:]
		}
		if strings.HasSuffix(line, "/") {
			r.dirOnly = true
			line = strings.TrimRight(line, "/")
		}
		if line == "" {
			continue
		}
		if strings.Contains(line, "/") {
			r.anchored = true
			line = strings.TrimPrefix(line, "/")
		}
		r.segments = strings.Split(line, "/")
		rules = append(rules, r)
	}
	return rules
}

// ignored reports whether the path rel, relative to the root with slashes,
// is ignored. dir says whether it is a folder.
func (rs IgnoreRules) ignored(rel string, dir bool) bool {
	ignored := false
	for i := range rs {
		r := &rs[i]
		if r.negate != ignored {
			// The rule could not change the answer.
			continue
		}
		if r.matches(rel, dir) {
			ignored = !r.negate
		}
	}
	return ignored
}

// matches reports whether r matches rel.
func (r *ignoreRule) matches(rel string, dir bool) bool {
	if r.dirOnly && !dir {
		return false
	}
	if r.base != "" {
		if !strings.HasPrefix(rel, r.base+"/") {
			return false
		}
		rel = rel[len(r.base)+1:]
	}
	if !r.anchored {
		return matchSegment(r.segments[0], path.Base(rel))
	}
	return matchGlob(r.segments, strings.Split(rel, "/"))
}

// matchGlob matches the segments of a path against those of a pattern, where
// a segment ** stands for any number of segments.
func matchGlob(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			rest := pat[1:]
			if len(rest) == 0 {
				return true
			}
			for i := range name {
				if matchGlob(rest, name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 || !matchSegment(pat[0], name[0]) {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}

// matchSegment matches one segment, with * ? and [...] as path.Match reads
// them. A malformed pattern matches nothing.
func matchSegment(pat, name string) bool {
	ok, err := path.Match(pat, name)
	return err == nil && ok
}
