package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/bankai-tensei/nople/internal/safewrite"

	"github.com/bankai-tensei/gowaku/widgets/editor"
	"github.com/bankai-tensei/gowaku/widgets/nav"
)

// session is what Nople keeps between runs: the folder, the tabs with their
// places, the layout of the dock and the look of the text.
type session struct {
	Version     int                  `json:"version"`
	Folder      string               `json:"folder,omitempty"`
	Wrap        bool                 `json:"wrap,omitempty"`
	FontSize    float32              `json:"fontSize,omitempty"`
	Tabs        []sessionTab         `json:"tabs,omitempty"`
	Current     string               `json:"current,omitempty"`
	Arrangement *nav.DockArrangement `json:"arrangement,omitempty"`
}

// sessionTab is one tab: its file and its place in it.
type sessionTab struct {
	ID      string                `json:"id"`
	Path    string                `json:"path"`
	Preview bool                  `json:"preview,omitempty"`
	Source  bool                  `json:"source,omitempty"`
	Sel     editor.TextRange      `json:"sel"`
	Scroll  editor.ScrollPosition `json:"scroll"`
}

// defaultSessionPath is the file of the session in the folder of settings of
// the user.
func defaultSessionPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "Nople", "session.json")
}

// saveSession writes the session to a.sessionPath. Diff tabs and terminals
// are left out: they are made from the state of git and of the shell, which
// will have changed.
func (a *app) saveSession() error {
	if a.sessionPath == "" {
		return nil
	}
	s := session{Version: 1, Wrap: a.wrap, FontSize: a.fontSize}
	if a.ws != nil {
		s.Folder = a.ws.Root
	}
	keep := map[string]bool{}
	for _, id := range a.order {
		t := a.tabs[id]
		if t.doc.diff != nil {
			continue
		}
		keep[id] = true
		s.Tabs = append(s.Tabs, sessionTab{
			ID: id, Path: t.doc.path, Preview: t.preview, Source: t.source,
			Sel: t.view.Selection(), Scroll: t.view.Scroll(),
		})
	}
	if keep[a.current] {
		s.Current = a.current
	}
	arr := a.dock.Arrangement()
	arr = keepPanels(arr, func(id string) bool { return keep[id] || isSidebar(id) || id == welcomeID })
	s.Arrangement = &arr
	data, err := json.MarshalIndent(s, "", "\t")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(a.sessionPath), 0o755); err != nil {
		return err
	}
	return safewrite.WriteFile(a.sessionPath, data, 0o644)
}

// keepPanels takes out of arr every panel that keep refuses.
func keepPanels(arr nav.DockArrangement, keep func(id string) bool) nav.DockArrangement {
	root := cloneNode(arr.Root)
	var drop []string
	var walk func(n *nav.DockNode)
	walk = func(n *nav.DockNode) {
		for _, id := range n.Panels {
			if !keep(id) {
				drop = append(drop, id)
			}
		}
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	walk(&root)
	for _, id := range drop {
		root = removePanel(root, id)
	}
	arr.Root = prune(root)
	arr.Hidden = slices.DeleteFunc(slices.Clone(arr.Hidden), func(h nav.DockHidden) bool { return !keep(h.ID) })
	arr.Closed = slices.DeleteFunc(slices.Clone(arr.Closed), func(id string) bool { return !keep(id) })
	if arr.Maximized != "" && !keep(arr.Maximized) {
		arr.Maximized = ""
	}
	return arr
}

// restoreSession opens what the session at a.sessionPath had open. It
// reports whether there was a session with a folder that still exists.
func (a *app) restoreSession() bool {
	if a.sessionPath == "" {
		return false
	}
	data, err := os.ReadFile(a.sessionPath)
	if err != nil {
		return false
	}
	var s session
	if err := json.Unmarshal(data, &s); err != nil || s.Version != 1 {
		return false
	}
	a.wrap = s.Wrap
	if s.FontSize > 0 {
		a.fontSize = s.FontSize
	}
	if s.Folder == "" {
		a.refresh()
		return false
	}
	if st, err := os.Stat(s.Folder); err != nil || !st.IsDir() {
		return false
	}
	if err := a.openFolderAs(s.Folder, false); err != nil {
		return false
	}
	// The tabs come back under their IDs, which the arrangement names.
	opened := map[string]bool{}
	for _, st := range s.Tabs {
		if _, err := os.Stat(st.Path); err != nil || a.tabs[st.ID] != nil || !strings.HasPrefix(st.ID, "doc:") {
			continue
		}
		d := a.docOf(st.Path)
		if d == nil {
			d = a.newDocument(st.Path)
			a.watchDir(filepath.Dir(d.path))
		}
		t := &tab{id: st.ID, doc: d, preview: st.Preview, source: st.Source && (d.svg != nil || d.markdown || d.sheet != nil), scale: 1}
		id := t.id
		t.view.OnSelectionChange = func(editor.TextRange) {
			if a.current == id {
				a.refreshStatus()
			}
		}
		t.view.SetSelection(st.Sel)
		t.view.SetScroll(st.Scroll)
		a.tabs[id] = t
		a.order = append(a.order, id)
		if t.preview {
			if a.previewID != "" {
				t.preview = false
			} else {
				a.previewID = id
			}
		}
		if n, err := strconv.Atoi(strings.TrimPrefix(id, "doc:")); err == nil {
			a.nextTab = max(a.nextTab, n)
		}
		opened[id] = true
	}
	if s.Arrangement != nil {
		arr := keepPanels(*s.Arrangement, func(id string) bool {
			return opened[id] || isSidebar(id) || id == welcomeID && len(opened) == 0
		})
		// A sidebar panel that was not in the session goes on the strip.
		for _, h := range defaultArrangement().Hidden {
			if !panelIn(arr, h.ID) {
				arr.Hidden = append(arr.Hidden, h)
			}
		}
		if len(opened) == 0 && !panelIn(arr, welcomeID) {
			arr = placeInEditors(arr, welcomeID, "")
		}
		// Tabs the arrangement lost go among the editors.
		for _, id := range a.order {
			if !panelIn(arr, id) {
				arr = placeInEditors(arr, id, a.anchorIn(arr))
			}
		}
		a.dock.SetArrangement(arr)
	} else {
		arr := a.dock.Arrangement()
		for _, id := range a.order {
			arr = placeInEditors(arr, id, a.anchorIn(arr))
		}
		a.dock.SetArrangement(arr)
	}
	if opened[s.Current] {
		a.current = s.Current
		a.dock.Activate(s.Current)
	} else if len(a.order) > 0 {
		a.current = a.order[len(a.order)-1]
	}
	a.refresh()
	return true
}

// panelIn reports whether the panel id is anywhere in arr.
func panelIn(arr nav.DockArrangement, id string) bool {
	if findGroup(&arr.Root, id) != nil || slices.Contains(arr.Closed, id) {
		return true
	}
	return slices.ContainsFunc(arr.Hidden, func(h nav.DockHidden) bool { return h.ID == id || slices.Contains(h.Origin, id) })
}

// anchorIn is a tab already among the editors of arr, or "".
func (a *app) anchorIn(arr nav.DockArrangement) string {
	for i := len(a.order) - 1; i >= 0; i-- {
		if g := groupOf(arr, a.order[i]); g != nil && !slices.ContainsFunc(g, isSidebar) {
			return a.order[i]
		}
	}
	return ""
}
