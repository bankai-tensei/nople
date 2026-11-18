package bigfile

import "github.com/bankai-tensei/gowaku/widgets/editor"

// reloadReach is how far, in lines, ReloadKeepingEdits looks for the place of
// an edit around where it was.
const reloadReach = 1 << 17

// Conflict is an edit that ReloadKeepingEdits could not place in the file as
// it now is, because the lines it replaced, or those around them, are no
// longer there as they were.
type Conflict struct {
	// Line is where the edit was, as a line of the file before the change.
	Line int
	// Removed are the lines of the file the edit replaced, nil when they were
	// too many to keep, and Inserted the lines it put in their place.
	Removed, Inserted []string
}

// ReloadKeepingEdits shows the file again as it is now on disk, with the
// edits made to the text placed in it: each edit goes where the lines it
// replaced are found, with the two lines on each side of them, near where it
// was, as a patch is applied. It suits a file that another program changed
// while the text holds edits, however it changed.
//
// It returns the edits it could not place. With partial, the text then shows
// the file with the others; without it, the text stays as it was when any
// edit cannot be placed, for the application to ask the user. The undo
// history goes; the selections and the scroll move as the edits placed
// moved, as Controller.Rebase moves them. ReloadKeepingEdits reads the
// whole new file before it returns, and returns ErrLoading while the file is
// still being indexed.
func (d *Doc) ReloadKeepingEdits(partial bool) ([]Conflict, error) {
	if d.saving != nil {
		return nil, ErrSaving
	}
	if !d.f.done {
		return nil, ErrLoading
	}
	s, err := d.load(d.f.path, true)
	if err != nil {
		return nil, err
	}
	edits := d.ctrl.Edits()
	placed, conflicts := place(s, edits)
	if len(conflicts) > 0 && !partial {
		_ = s.Close() //nolint:errcheck // only read
		return conflicts, nil
	}
	if d.f != s {
		_ = d.f.Close() //nolint:errcheck // a file only read, which s replaces
	}
	d.f, d.told, d.loadErr = s, Unchanged, nil
	d.ctrl.Rebase(s, placed)
	d.watch()
	return conflicts, nil
}

// place finds where each of edits goes in s: the line from which s holds the
// lines the edit removed, after the lines before them and followed by those
// after them, in order, as near where it was, moved by the edits before, as
// can be.
func place(s editor.LineSource, edits []editor.SourceEdit) ([]editor.PlacedEdit, []Conflict) {
	var placed []editor.PlacedEdit
	var conflicts []Conflict
	drift, low := 0, 0
	for k, e := range edits {
		at := -1
		if e.Removed != nil || e.From == e.To {
			at = find(s, e.Before, e.Removed, e.After, e.From+drift, low)
		}
		if at < 0 {
			conflicts = append(conflicts, Conflict{Line: e.From, Removed: e.Removed, Inserted: e.Inserted})
			continue
		}
		placed = append(placed, editor.PlacedEdit{Edit: k, At: at})
		drift = at - e.From
		low = at + len(e.Removed)
	}
	return placed, conflicts
}

// find returns the line from which s holds removed, after before and followed
// by after, at least at low and as near near as can be, or -1 when there is
// none within reloadReach.
func find(s editor.LineSource, before, removed, after []string, near, low int) int {
	n := s.Len()
	fits := func(p int) bool {
		if p < low || p-len(before) < 0 || p+len(removed)+len(after) > n {
			return false
		}
		for i, l := range removed {
			if s.Line(p+i) != l {
				return false
			}
		}
		for i, l := range before {
			if s.Line(p-len(before)+i) != l {
				return false
			}
		}
		for i, l := range after {
			if s.Line(p+len(removed)+i) != l {
				return false
			}
		}
		return true
	}
	near = min(max(near, low), n)
	for dist := 0; dist <= reloadReach; dist++ {
		if fits(near + dist) {
			return near + dist
		}
		if dist > 0 && fits(near-dist) {
			return near - dist
		}
		if near+dist > n && near-dist < low {
			break
		}
	}
	return -1
}
