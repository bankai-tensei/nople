package app

import (
	"slices"
	"testing"

	"github.com/bankai-tensei/gowaku/layout"
	"github.com/bankai-tensei/gowaku/widgets/nav"
)

func TestPlaceInEditors_ReplacesWelcome(t *testing.T) {
	a := placeInEditors(defaultArrangement(), "doc:1", welcomeID)
	if got := a.Root.Panels; !slices.Equal(got, []string{"doc:1"}) {
		t.Fatalf("root panels = %v, want [doc:1]", got)
	}
	if len(a.Hidden) != len(sidebarIDs) {
		t.Errorf("hidden = %v, want the sidebar kept", a.Hidden)
	}
	a = placeInEditors(a, "doc:2", "doc:1")
	if got := a.Root.Panels; !slices.Equal(got, []string{"doc:1", "doc:2"}) || a.Root.Active != "doc:2" {
		t.Errorf("root = %+v, want doc:2 after doc:1 and active", a.Root)
	}
}

func TestPlaceInEditors_BesidePinnedSidebar(t *testing.T) {
	// The explorer is pinned, and the editors are gone: a document must not
	// join the explorer's group.
	a := nav.DockArrangement{Root: nav.DockNode{Panels: []string{explorerID}}}
	a = placeInEditors(a, welcomeID, "")
	if len(a.Root.Children) != 2 || a.Root.Axis != layout.Horizontal {
		t.Fatalf("root = %+v, want a split", a.Root)
	}
	if !slices.Equal(a.Root.Children[0].Panels, []string{explorerID}) || !slices.Equal(a.Root.Children[1].Panels, []string{welcomeID}) {
		t.Errorf("children = %+v", a.Root.Children)
	}
}

func TestPrune(t *testing.T) {
	n := nav.DockNode{Children: []nav.DockNode{
		{Panels: []string{"a"}},
		{Children: []nav.DockNode{{}, {Panels: []string{"b"}}}},
		{},
	}, Weights: []float32{1, 2, 3}}
	got := prune(n)
	if len(got.Children) != 2 || !slices.Equal(got.Weights, []float32{1, 2}) {
		t.Fatalf("prune = %+v", got)
	}
	if !slices.Equal(got.Children[1].Panels, []string{"b"}) {
		t.Errorf("the split of one child should become the child: %+v", got.Children[1])
	}
}
