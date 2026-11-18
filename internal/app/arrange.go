package app

import (
	"slices"

	"github.com/bankai-tensei/gowaku/layout"
	"github.com/bankai-tensei/gowaku/widgets/nav"
)

// IDs of the panels that are not documents.
const (
	explorerID = "explorer"
	searchID   = "search"
	scmID      = "scm"
	outlineID  = "outline"
	welcomeID  = "welcome"
)

// sidebarIDs are the panels of the strip at the left, in order.
var sidebarIDs = []string{explorerID, searchID, scmID, outlineID}

// isSidebar reports whether id is a panel of the sidebar.
func isSidebar(id string) bool { return slices.Contains(sidebarIDs, id) }

// Sizes of the sidebar.
const (
	sidebarWidth = 280
	// sidebarShare is the part of the width a pinned sidebar takes when it
	// docks back along the left edge.
	sidebarShare = 0.22
)

// defaultArrangement has the welcome page in the middle and the sidebar
// panels hidden on the strip at the left, each on its own, so that each pins
// back by itself.
func defaultArrangement() nav.DockArrangement {
	a := nav.DockArrangement{Root: nav.DockNode{Panels: []string{welcomeID}}}
	for _, id := range sidebarIDs {
		a.Hidden = append(a.Hidden, nav.DockHidden{
			ID: id, Side: nav.DockLeft, Size: sidebarWidth, Origin: []string{id}, Share: sidebarShare,
		})
	}
	return a
}

// placeInEditors puts the panel id among the editors: in the group of anchor
// after it, or, without one, in a new group at the right of the whole layout.
// A welcome page in that group goes. It returns the new arrangement.
func placeInEditors(a nav.DockArrangement, id, anchor string) nav.DockArrangement {
	a.Root = removePanel(cloneNode(a.Root), id)
	a.Closed = slices.DeleteFunc(slices.Clone(a.Closed), func(s string) bool { return s == id })
	if anchor != "" && anchor != id {
		if g := findGroup(&a.Root, anchor); g != nil {
			i := slices.Index(g.Panels, anchor)
			g.Panels = slices.Insert(g.Panels, i+1, id)
			g.Active = id
			if id != welcomeID {
				g.Panels = slices.DeleteFunc(g.Panels, func(s string) bool { return s == welcomeID })
			}
			a.Root = prune(a.Root)
			return a
		}
	}
	group := nav.DockNode{Panels: []string{id}, Active: id}
	a.Root = prune(a.Root)
	if isEmpty(a.Root) {
		a.Root = group
	} else {
		a.Root = nav.DockNode{
			Axis:     layout.Horizontal,
			Children: []nav.DockNode{a.Root, group},
			Weights:  []float32{sidebarShare, 1 - sidebarShare},
		}
	}
	a.Maximized = ""
	return a
}

// groupOf returns the panels of the group that holds id, or nil.
func groupOf(a nav.DockArrangement, id string) []string {
	if g := findGroup(&a.Root, id); g != nil {
		return g.Panels
	}
	return nil
}

// activeOf returns the panel each group shows.
func activeOf(a nav.DockArrangement) []string {
	var out []string
	var walk func(n *nav.DockNode)
	walk = func(n *nav.DockNode) {
		if len(n.Children) == 0 {
			if len(n.Panels) > 0 {
				act := n.Active
				if !slices.Contains(n.Panels, act) {
					act = n.Panels[0]
				}
				out = append(out, act)
			}
			return
		}
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	walk(&a.Root)
	return out
}

// findGroup returns the group under n that holds id.
func findGroup(n *nav.DockNode, id string) *nav.DockNode {
	if len(n.Children) == 0 {
		if slices.Contains(n.Panels, id) {
			return n
		}
		return nil
	}
	for i := range n.Children {
		if g := findGroup(&n.Children[i], id); g != nil {
			return g
		}
	}
	return nil
}

// cloneNode copies n deeply, so that changing it leaves the arrangement it
// came from alone.
func cloneNode(n nav.DockNode) nav.DockNode {
	n.Panels = slices.Clone(n.Panels)
	n.Weights = slices.Clone(n.Weights)
	if n.Children != nil {
		kids := make([]nav.DockNode, len(n.Children))
		for i, c := range n.Children {
			kids[i] = cloneNode(c)
		}
		n.Children = kids
	}
	return n
}

// removePanel takes id out of every group under n.
func removePanel(n nav.DockNode, id string) nav.DockNode {
	if len(n.Children) == 0 {
		n.Panels = slices.DeleteFunc(n.Panels, func(s string) bool { return s == id })
		return n
	}
	for i := range n.Children {
		n.Children[i] = removePanel(n.Children[i], id)
	}
	return n
}

// isEmpty reports whether n holds no panel.
func isEmpty(n nav.DockNode) bool {
	if len(n.Children) == 0 {
		return len(n.Panels) == 0
	}
	for _, c := range n.Children {
		if !isEmpty(c) {
			return false
		}
	}
	return true
}

// prune drops the empty groups under n, with their weights, and replaces a
// split left with one child by that child.
func prune(n nav.DockNode) nav.DockNode {
	if len(n.Children) == 0 {
		return n
	}
	var kids []nav.DockNode
	var weights []float32
	even := len(n.Weights) != len(n.Children)
	for i, c := range n.Children {
		c = prune(c)
		if isEmpty(c) {
			continue
		}
		kids = append(kids, c)
		if !even {
			weights = append(weights, n.Weights[i])
		}
	}
	switch len(kids) {
	case 0:
		return nav.DockNode{}
	case 1:
		return kids[0]
	}
	n.Children, n.Weights = kids, weights
	return n
}
