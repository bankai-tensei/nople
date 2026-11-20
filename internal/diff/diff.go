// Package diff finds where two texts differ, line by line and within a line,
// for the changes Git shows.
package diff

import (
	"strings"
	"unicode/utf8"
)

// maxDiffEdits bounds the work of lineDiff: past this many differing lines
// the rest of the files counts as one change.
const maxDiffEdits = 2000

// Hunk is a stretch where two texts differ: the lines from oldStart for
// oldLen in the old text became those from newStart for newLen in the new.
// One of the lengths may be 0.
type Hunk struct {
	OldStart, OldLen, NewStart, NewLen int
}

// Split splits a text into its lines, without their line breaks; a text
// that ends with a line break has no empty last line.
func Split(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}

// Lines finds the hunks that turn the lines a into b, by the algorithm of
// Myers: the fewest lines deleted and inserted.
func Lines(a, b []string) []Hunk {
	// The lines both start and end with are the same, and need no search.
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	ma, mb := a[pre:len(a)-suf], b[pre:len(b)-suf]
	hs := myers(ma, mb)
	for i := range hs {
		hs[i].OldStart += pre
		hs[i].NewStart += pre
	}
	return hs
}

// myers diffs a and b, which differ at their ends.
func myers(a, b []string) []Hunk {
	n, m := len(a), len(b)
	switch {
	case n == 0 && m == 0:
		return nil
	case n == 0 || m == 0:
		return []Hunk{{0, n, 0, m}}
	}
	total := n + m
	limit := min(total, maxDiffEdits)
	off := total + 1
	v := make([]int, 2*total+4)
	// trace[d] is v as it was before step d, at the diagonals -d-1 to d+1,
	// which are all that the walk back reads: so the memory grows with the
	// square of the edits and not with the size of the files.
	var trace [][]int
	found := false
	for d := 0; d <= limit && !found; d++ {
		trace = append(trace, append([]int(nil), v[off-d-1:off+d+2]...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || k != d && v[off+k-1] < v[off+k+1] {
				x = v[off+k+1]
			} else {
				x = v[off+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[off+k] = x
			if x >= n && y >= m {
				found = true
				break
			}
		}
	}
	if !found {
		// Too different to be worth the search: one change.
		return []Hunk{{0, n, 0, m}}
	}
	// Walk back through the trace, from the end, marking each line.
	delA := make([]bool, n)
	insB := make([]bool, m)
	x, y := n, m
	for d := len(trace) - 1; d >= 0 && (x > 0 || y > 0); d-- {
		w := trace[d]
		at := func(k int) int { return w[k+d+1] }
		k := x - y
		var pk int
		if k == -d || k != d && at(k-1) < at(k+1) {
			pk = k + 1
		} else {
			pk = k - 1
		}
		px := at(pk)
		py := px - pk
		for x > px && y > py && x > 0 && y > 0 {
			x--
			y--
		}
		if d == 0 {
			break
		}
		if x == px {
			insB[py] = true
		} else {
			delA[px] = true
		}
		x, y = px, py
	}
	// Gather the marks into hunks.
	var hs []Hunk
	i, j := 0, 0
	for i < n || j < m {
		if i < n && j < m && !delA[i] && !insB[j] {
			i++
			j++
			continue
		}
		h := Hunk{OldStart: i, NewStart: j}
		for i < n && delA[i] {
			i++
		}
		for j < m && insB[j] {
			j++
		}
		h.OldLen, h.NewLen = i-h.OldStart, j-h.NewStart
		if h.OldLen == 0 && h.NewLen == 0 {
			// Out of step, which a correct trace never is: take the rest.
			hs = append(hs, Hunk{i, n - i, j, m - j})
			break
		}
		hs = append(hs, h)
	}
	return hs
}

// Intraline is the part of the line a and of b that differs, as byte
// ranges, found by their common start and end; a line that is all changed
// gives no ranges.
func Intraline(a, b string) (as, ae, bs, be int, ok bool) {
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	// The ranges start and end between characters.
	for p > 0 && (p < len(a) && !utf8.RuneStart(a[p]) || p < len(b) && !utf8.RuneStart(b[p])) {
		p--
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	for s > 0 && (!utf8.RuneStart(a[len(a)-s]) || !utf8.RuneStart(b[len(b)-s])) {
		s--
	}
	if p == 0 && s == 0 {
		return 0, 0, 0, 0, false
	}
	return p, len(a) - s, p, len(b) - s, true
}

// LineMap maps line numbers of the old text to the new one and back, for
// two editors that scroll together.
type LineMap struct {
	Hunks []Hunk
}

// ToNew is the line of the new text beside line l of the old one: the same
// line in a stretch that did not change, and a line in proportion in one
// that did.
func (lm LineMap) ToNew(l float64) float64 { return lm.mapLine(l, false) }

// ToOld is the other way.
func (lm LineMap) ToOld(l float64) float64 { return lm.mapLine(l, true) }

func (lm LineMap) mapLine(l float64, back bool) float64 {
	shift := 0.0
	for _, h := range lm.Hunks {
		fs, fl, ts, tl := h.OldStart, h.OldLen, h.NewStart, h.NewLen
		if back {
			fs, fl, ts, tl = ts, tl, fs, fl
		}
		if l < float64(fs) {
			break
		}
		if l < float64(fs+fl) {
			return float64(ts) + (l-float64(fs))*float64(tl)/float64(fl)
		}
		shift = float64(ts+tl) - float64(fs+fl)
	}
	return l + shift
}
