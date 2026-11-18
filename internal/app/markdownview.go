package app

import (
	"github.com/bankai-tensei/nople/internal/syntax"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/layout"
	"github.com/bankai-tensei/gowaku/markdown"
	"github.com/bankai-tensei/gowaku/text"
	"github.com/bankai-tensei/gowaku/theme"
	"github.com/bankai-tensei/gowaku/widgets/basic"
	"github.com/bankai-tensei/gowaku/widgets/collection"
	"github.com/bankai-tensei/gowaku/widgets/field"
)

// readingWidth is the widest a rendered document runs: lines longer than
// this are hard to read.
const readingWidth = 860

// isMarkdownPath reports whether the file at path is Markdown, which shows
// rendered.
func isMarkdownPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".mdown", ".mkd":
		return true
	}
	return false
}

// markdownView shows the document of tab t rendered: its text selects and
// copies, links open in a tab or the browser, pictures come from beside it,
// and code blocks are colored as the editor colors code.
func markdownView(th *theme.Theme, a *app, t *tab) core.Widget {
	d := t.doc
	id := t.id
	p := paletteOf(th)
	return basic.Container{Color: th.Color.Canvas, Child: field.SelectionArea{Child: basic.Scroll{
		Child: basic.Center{Child: basic.ConstrainedBox{MaxWidth: readingWidth, Child: basic.Padding{
			Insets: geom.Symmetric(theme.Space28, theme.Space24),
			Child: markdown.Markdown{
				Source:     d.content.Text,
				Controller: &t.md,
				Images:     os.DirFS(filepath.Dir(d.path)),
				Highlight:  func(lang, code string) []text.Span { return highlightCode(p, lang, code) },
				OnLinkTap:  func(dest string) { a.followLink(id, dest) },
				CopyButton: true,
			},
		}}},
	}}}
}

// highlightCode colors a code block of a Markdown document in the language of
// its info string, or returns nil to leave it in one color.
func highlightCode(p *palette, lang, code string) []text.Span {
	toks := syntax.Code(lang, code)
	if toks == nil {
		return nil
	}
	spans := make([]text.Span, len(toks))
	for i, tok := range toks {
		d := p[tok.Kind]
		st := text.Style{Color: d.Color, Italic: d.Italic}
		if d.Bold {
			st.Weight = 700
		}
		spans[i] = text.Span{Text: tok.Text, Style: st}
	}
	return spans
}

// followLink follows a link of the Markdown document of tab id: an address of
// the web opens in the browser of the OS; a path opens beside the document in
// a tab, at the heading its fragment names.
func (a *app) followLink(id, dest string) {
	t := a.tabs[id]
	if t == nil {
		return
	}
	u, err := url.Parse(dest)
	if err != nil {
		return
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto":
		a.shell.openURL(u.String())
		return
	case "":
	default:
		// file:, javascript: and the like are not followed.
		return
	}
	if u.Path == "" {
		return
	}
	path := filepath.FromSlash(u.Path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(t.doc.path), path)
	}
	st, err := os.Stat(path)
	if err != nil {
		return
	}
	if st.IsDir() {
		a.revealFolder(path)
		return
	}
	a.openFile(path, false)
	if u.Fragment != "" {
		if cur := a.currentTab(); cur != nil && cur.showsMarkdown() {
			// The controller waits for the document if it has not shown yet.
			cur.md.ShowHeading(u.Fragment)
		}
	}
}

// revealFolder opens the folder at path in the explorer, and its parents.
func (a *app) revealFolder(path string) {
	if a.ws == nil {
		return
	}
	rel := a.ws.Rel(path)
	if rel == "" || strings.HasPrefix(rel, "../") {
		return
	}
	cur := a.ws.Root
	for _, part := range strings.Split(rel, "/") {
		cur = filepath.Join(cur, part)
		a.tree.Expand(cur)
	}
	a.showPanel(explorerID)
}

// outlineView lists the headings of the Markdown document of the current tab;
// a heading pressed comes into view.
type outlineView struct {
	core.StatelessBase
	app *app
}

// Build implements core.StatelessWidget.
func (v outlineView) Build(ctx core.BuildContext) core.Widget {
	a := v.app
	th := basic.ThemeOf(ctx)
	t := a.currentTab()
	if t == nil || !t.showsMarkdown() {
		return placeholder(ctx, T("outline.empty"))
	}
	hs := t.doc.headings
	if len(hs) == 0 {
		return placeholder(ctx, T("outline.none"))
	}
	top := 6
	for _, h := range hs {
		top = min(top, h.Level)
	}
	rows := make([]core.Widget, 0, len(hs))
	for _, h := range hs {
		slug := h.Slug
		rows = append(rows, basic.Padding{Insets: geom.Insets{Left: float32(h.Level-top) * theme.Space12}, Child: outlineItem(h.Text, func() {
			t.md.ShowHeading(slug)
		})})
	}
	return basic.Container{Color: th.Color.Sidebar, Child: basic.Scroll{Child: basic.Padding{
		Insets: geom.Symmetric(theme.Space8, theme.Space8),
		Child:  basic.Column{Cross: layout.CrossStretch, Children: rows},
	}}}
}

// outlineItem is one heading of the outline.
func outlineItem(label string, onPress func()) core.Widget {
	return collection.SidebarItem{Label: label, OnPress: onPress}
}
