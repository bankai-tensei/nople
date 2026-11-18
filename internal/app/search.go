package app

import (
	"fmt"
	"github.com/bankai-tensei/nople/internal/textfile"
	"github.com/bankai-tensei/nople/internal/workspace"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/layout"
	"github.com/bankai-tensei/gowaku/theme"
	"github.com/bankai-tensei/gowaku/widgets/basic"
	"github.com/bankai-tensei/gowaku/widgets/collection"
	"github.com/bankai-tensei/gowaku/widgets/control"
	"github.com/bankai-tensei/gowaku/widgets/editor"
	"github.com/bankai-tensei/gowaku/widgets/field"
)

// Limits of the search of the folder.
const (
	// maxHits is the most matches a search lists.
	maxHits = 5000
	// maxSearchedSize is the largest file that is searched.
	maxSearchedSize = 4 << 20
	// searchBatch is how many files are searched between reports to the UI.
	searchBatch = 64
	// hitContext is how much of a long line shows around a match, in
	// characters before it.
	hitContext = 24
)

// searchOptions are what to look for.
type searchOptions struct {
	query     string
	matchCase bool
	word      bool
	regex     bool
}

// pattern compiles the options to a regular expression.
func (o searchOptions) pattern() (*regexp.Regexp, error) {
	p := o.query
	if !o.regex {
		p = regexp.QuoteMeta(p)
	}
	if o.word {
		p = `\b(?:` + p + `)\b`
	}
	if !o.matchCase {
		p = "(?i)" + p
	}
	return regexp.Compile(p)
}

// hit is one match in a file.
type hit struct {
	// line is from 0; start and end are byte offsets in it.
	line, start, end int
	// col is the column of the match in characters, from 1.
	col int
	// text is the line, cut around the match if it is long, and trimmed.
	text string
}

// fileHits are the matches in one file.
type fileHits struct {
	path string
	hits []hit
}

// searchState is the search of the folder: what was asked, and what was
// found so far.
type searchState struct {
	opts searchOptions
	// gen counts the searches, so that the reports of an old one are
	// dropped.
	gen     int
	live    atomic.Int64
	running bool
	err     error
	files   []fileHits
	byPath  map[string]*fileHits
	count   int
	scanned int
	// cut means the search stopped at maxHits.
	cut  bool
	tree collection.TreeController[string]
}

// search starts a search of the folder for o, stopping the one before.
func (a *app) search(o searchOptions) {
	s := &a.find
	s.gen++
	gen := s.gen
	s.live.Store(int64(gen))
	s.opts = o
	s.files, s.byPath, s.count, s.scanned, s.cut, s.err, s.running = nil, map[string]*fileHits{}, 0, 0, false, nil, false
	defer a.refresh()
	if a.ws == nil || o.query == "" {
		return
	}
	re, err := o.pattern()
	if err != nil {
		s.err = err
		return
	}
	s.running = true
	root, gitignore := a.ws.Root, a.ws.Gitignore
	stopped := func() bool { return s.live.Load() != int64(gen) }
	a.shell.async(func() func() {
		var batch []fileHits
		files, count := 0, 0
		flush := func() {
			b, n := batch, files
			batch = nil
			a.shell.post(func() { a.searchFound(gen, b, n) })
		}
		workspace.Walk(root, gitignore, stopped, func(p string) bool {
			files++
			if fh := searchFile(p, re, maxHits-count); len(fh.hits) > 0 {
				batch = append(batch, fh)
				count += len(fh.hits)
			}
			if files%searchBatch == 0 {
				flush()
			}
			return count >= maxHits
		})
		flush()
		cut := count >= maxHits
		return func() { a.searchDone(gen, cut) }
	})
}

// searchFound takes in matches of search gen.
func (a *app) searchFound(gen int, batch []fileHits, scanned int) {
	s := &a.find
	if gen != s.gen {
		return
	}
	for _, fh := range batch {
		s.files = append(s.files, fh)
		s.count += len(fh.hits)
	}
	s.byPath = make(map[string]*fileHits, len(s.files))
	for i := range s.files {
		s.byPath[s.files[i].path] = &s.files[i]
		s.tree.Expand(s.files[i].path)
	}
	s.scanned = scanned
	a.refresh()
}

// searchDone ends search gen.
func (a *app) searchDone(gen int, cut bool) {
	if s := &a.find; gen == s.gen {
		s.running, s.cut = false, cut
		a.refresh()
	}
}

// searchFile finds re in the file at path, at most limit times. A binary or
// huge file has no matches.
func searchFile(path string, re *regexp.Regexp, limit int) fileHits {
	fh := fileHits{path: path}
	st, err := os.Stat(path)
	if err != nil || st.Size() > maxSearchedSize || isImagePath(path) {
		return fh
	}
	data, err := os.ReadFile(path)
	if err != nil || textfile.IsBinary(data) {
		return fh
	}
	var c textfile.Content
	textfile.DecodeText(data, &c)
	line := 0
	for rest := c.Text; rest != "" && len(fh.hits) < limit; line++ {
		s := rest
		if i := strings.IndexByte(rest, '\n'); i >= 0 {
			s, rest = rest[:i], rest[i+1:]
		} else {
			rest = ""
		}
		s = strings.TrimSuffix(s, "\r")
		for _, m := range re.FindAllStringIndex(s, -1) {
			if m[0] == m[1] {
				continue
			}
			fh.hits = append(fh.hits, hit{line: line, start: m[0], end: m[1], col: utf8.RuneCountInString(s[:m[0]]) + 1, text: hitText(s, m[0])})
			if len(fh.hits) == limit {
				break
			}
		}
	}
	return fh
}

// hitText is the line s as a row of the results shows it: without its
// indent, and from a little before the match at start if the line is long.
func hitText(s string, start int) string {
	from := len(s) - len(strings.TrimLeft(s, " \t"))
	if utf8.RuneCountInString(s[from:start]) > hitContext {
		// Back hitContext characters from the match.
		i := start
		for n := 0; n < hitContext && i > from; n++ {
			_, size := utf8.DecodeLastRuneInString(s[:i])
			i -= size
		}
		from = i
		return "…" + truncate(s[from:], 200)
	}
	return truncate(s[from:], 200)
}

// truncate cuts s to n characters.
func truncate(s string, n int) string {
	for i := range s {
		if n == 0 {
			return s[:i] + "…"
		}
		n--
	}
	return s
}

// The keys of the tree of results: a file is its path; a match is its path,
// a tab, and its index in the file.
func hitKey(path string, i int) string { return path + "\t" + strconv.Itoa(i) }

// splitHitKey parses a key of a match; ok is false for a file.
func splitHitKey(k string) (path string, i int, ok bool) {
	p, n, found := strings.Cut(k, "\t")
	if !found {
		return k, 0, false
	}
	i, err := strconv.Atoi(n)
	return p, i, err == nil
}

// openHit shows the match of key k in a tab, selected.
func (a *app) openHit(k string, preview bool) {
	p, i, ok := splitHitKey(k)
	fh := a.find.byPath[p]
	if !ok || fh == nil || i >= len(fh.hits) {
		return
	}
	h := fh.hits[i]
	a.openFile(p, preview)
	if t := a.currentTab(); t != nil && samePath(t.doc.path, p) {
		a.gotoRange(t, h.line, h.start, h.end)
		if !preview {
			a.focusEditor(t)
		}
	}
}

// searchView is the panel of the search of the folder.
type searchView struct {
	core.StatefulBase
	app *app
}

type searchViewState struct {
	core.StateOf[searchView]
	query field.InputController
}

// CreateState implements core.StatefulWidget.
func (searchView) CreateState() core.State { return &searchViewState{} }

// Build implements core.State.
func (s *searchViewState) Build(ctx core.BuildContext) core.Widget {
	a := s.Widget().app
	th := basic.ThemeOf(ctx)
	f := &a.find
	o := f.opts
	if a.ws == nil {
		return placeholder(ctx, T("search.noFolder"))
	}
	toggle := func(label string, on bool, set func(*searchOptions)) core.Widget {
		return control.Chip{Label: label, Selected: on, OnPress: func() {
			n := f.opts
			n.query = s.query.Text()
			set(&n)
			a.search(n)
		}}
	}
	controls := basic.Column{Size: layout.MainMin, Cross: layout.CrossStretch, Spacing: theme.Space8, Children: []core.Widget{
		field.Input{
			Value: o.query, Placeholder: T("panel.search"), Label: T("search.label"), Controller: &s.query,
			OnSubmit: func(v string) {
				n := f.opts
				n.query = v
				a.search(n)
			},
		},
		basic.Row{Spacing: theme.Space6, Children: []core.Widget{
			toggle("Aa", o.matchCase, func(n *searchOptions) { n.matchCase = !n.matchCase }),
			toggle(T("search.word"), o.word, func(n *searchOptions) { n.word = !n.word }),
			toggle(".*", o.regex, func(n *searchOptions) { n.regex = !n.regex }),
		}},
		searchStatus(th, f),
	}}
	var results core.Widget = basic.SizedBox{}
	if len(f.files) > 0 {
		results = collection.Tree[string]{
			Roots: func() []string {
				r := make([]string, len(f.files))
				for i := range f.files {
					r[i] = f.files[i].path
				}
				return r
			}(),
			ChildrenOf: func(k string) []string {
				fh := f.byPath[k]
				if fh == nil {
					return nil
				}
				ks := make([]string, len(fh.hits))
				for i := range fh.hits {
					ks[i] = hitKey(k, i)
				}
				return ks
			},
			HasChildren: func(k string) bool { _, _, ok := splitHitKey(k); return !ok },
			LabelOf: func(k string) string {
				p, i, ok := splitHitKey(k)
				if !ok {
					rel := a.ws.Rel(p)
					return fmt.Sprintf("%s（%d）", rel, len(f.byPath[p].hits))
				}
				hs := f.byPath[p].hits
				h := hs[i]
				if i > 0 && hs[i-1].line == h.line || i+1 < len(hs) && hs[i+1].line == h.line {
					// Several matches on the line tell apart by their column.
					line := h.text
					return fmt.Sprintf("%d:%d: %s", h.line+1, h.col, line)
				}
				return fmt.Sprintf("%d: %s", h.line+1, h.text)
			},
			IconOf: func(k string) core.Widget {
				if _, _, ok := splitHitKey(k); ok {
					return nil
				}
				return fileIcon(k, false, th.Color.Ink3, th.Brightness == theme.BrightnessDark)
			},
			Label:      T("search.results"),
			Controller: &f.tree,
			OnSelectionChange: func(sel []string) {
				if len(sel) == 1 {
					a.openHit(sel[0], true)
				}
			},
			OnActivate: func(k string) {
				if _, _, ok := splitHitKey(k); !ok {
					f.tree.SetExpanded(k, !f.tree.Expanded(k))
					return
				}
				a.openHit(k, false)
				a.dock.HideFlyout()
			},
		}
	}
	return basic.Container{Color: th.Color.Sidebar, Child: basic.Column{Cross: layout.CrossStretch, Children: []core.Widget{
		basic.Padding{Insets: geom.All(theme.Space12), Child: controls},
		basic.Expanded{Child: results},
	}}}
}

// searchStatus is the line under the field: how the search went, after a
// spinner while it runs.
func searchStatus(th *theme.Theme, f *searchState) core.Widget {
	summary := basic.Text{Content: searchSummary(f), Style: th.Text.Caption.WithColor(th.Color.Ink2)}
	if !f.running {
		return summary
	}
	return basic.Row{Cross: layout.CrossCenter, Spacing: theme.Space6, Children: []core.Widget{
		control.ProgressRing{Indeterminate: true, Size: 12, Thickness: 1.5},
		basic.Expanded{Child: summary},
	}}
}

// searchSummary says how the search went.
func searchSummary(f *searchState) string {
	switch {
	case f.err != nil:
		return T("search.badRegex", f.err.Error())
	case f.opts.query == "":
		return T("search.hint")
	case f.running && f.count == 0:
		return T("search.scanning", f.scanned)
	case f.running:
		return T("search.foundScanning", len(f.files), f.count)
	case f.count == 0:
		return T("common.notFound")
	case f.cut:
		return T("search.foundCut", len(f.files), f.count)
	}
	return T("search.found", len(f.files), f.count)
}

// gotoRange selects from byte start to end of line in tab t, as gotoLine
// does with characters.
func (a *app) gotoRange(t *tab, line, start, end int) {
	if !t.hasText() {
		return
	}
	if t.showsDrawing() || t.showsMarkdown() || t.showsSheet() {
		t.source = true
	}
	c := &t.doc.ctrl
	line = min(max(line, 0), c.LineCount()-1)
	t.view.SetSelection(editor.TextRange{
		Start: editor.TextPosition{Line: line, Column: start},
		End:   editor.TextPosition{Line: line, Column: end},
	})
	a.activate(t.id)
}
