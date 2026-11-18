package app

import (
	"errors"
	"fmt"
	"github.com/bankai-tensei/nople/internal/textfile"
	"os"
	"path/filepath"
	"strings"

	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/layout"
	"github.com/bankai-tensei/gowaku/text"
	"github.com/bankai-tensei/gowaku/theme"
	"github.com/bankai-tensei/gowaku/widgets/basic"
	"github.com/bankai-tensei/gowaku/widgets/control"
	"github.com/bankai-tensei/gowaku/widgets/editor"
	"github.com/bankai-tensei/gowaku/widgets/nav"
)

// docView is the content of one tab: the path, a notice if the file needs
// one, and the text read only, or the picture.
type docView struct {
	core.StatelessBase
	app *app
	tab *tab
}

// Build implements core.StatelessWidget.
func (v docView) Build(ctx core.BuildContext) core.Widget {
	a, t := v.app, v.tab
	th := basic.ThemeOf(ctx)
	if t == nil {
		return basic.SizedBox{}
	}
	d := t.doc
	if d.svg != nil && d.svg.HasText() {
		// The text of the drawing needs fonts: the shaper of the window sets it
		// once, and the same shaper again does nothing (ADR 0113).
		if shaper, ok := core.DependOn[*text.Shaper](ctx); ok {
			_ = d.svg.SetText(shaper)
		}
	}
	if p := paletteOf(th); d.pal != p {
		// The theme changed, or this is the first build: the colors follow.
		d.pal = p
		if d.hl != nil {
			d.ctrl.Redecorate()
		}
	}
	var body core.Widget
	switch c := d.content; {
	case d.diff != nil:
		body = diffView(th, a, t)
	case c.Err != nil:
		body = message(th, T("doc.cannotOpen"), errorText(c.Err))
	case d.image != nil, t.showsDrawing():
		body = pictureView(th, a, t)
	case t.showsMarkdown():
		body = markdownView(th, a, t)
	case t.showsSheet():
		shaper, _ := core.DependOn[*text.Shaper](ctx)
		body = sheetView(th, shaper, a, t)
	case c.Binary:
		body = message(th, T("common.binary"), T("doc.binaryDetail", sizeText(c.Size)))
	default:
		body = editor.Editor{
			Controller:           &d.ctrl,
			View:                 &t.view,
			Label:                d.title(),
			Style:                text.Style{Family: a.settings.FontFamily, Size: a.fontSize},
			Wrap:                 a.wrap,
			ReadOnly:             !d.canEdit() || d.saving(),
			OnChange:             func() { a.edited(d) },
			LineNumbers:          true,
			HighlightCurrentLine: true,
			Gutter:               markGutter(th, d),
			GutterWidth:          gutterWidth,
			Decorations: func(i int, s string, dst []editor.Decoration) []editor.Decoration {
				if d.bighl != nil {
					return a.decorateBig(d, i, s, dst)
				}
				return decorate(d.hl, d.pal, i, s, dst)
			},
		}
	}
	return basic.Container{Color: th.Color.Canvas, Child: basic.Column{Cross: layout.CrossStretch, Children: []core.Widget{
		basic.Padding{Insets: geom.Symmetric(theme.Space8, 0), Child: nav.Breadcrumb{
			Items: breadcrumb(a, d.path), Label: T("doc.location"),
		}},
		basic.Container{Height: 1, Color: th.Color.Hair},
		notice(a, t),
		basic.Expanded{Child: body},
	}}}
}

// notice is the bar above the text that warns of a file that is binary or
// large. A closed bar takes no room.
func notice(a *app, t *tab) control.InfoBar {
	c := t.doc.content
	id := t.id
	closeIt := func() { a.closeNotice(id) }
	switch {
	case t.doc.saveErr != nil:
		return control.InfoBar{
			Open: true, Severity: control.SeverityError,
			Title: T("doc.saveFailed"), Message: t.doc.saveErr.Error(),
			Action: T("doc.saveAgain"), OnAction: func() { a.save(t.doc) },
		}
	case t.doc.conflict:
		detail := T("doc.conflictDetail")
		if t.doc.big != nil {
			// A large file is not saved over one changed on disk.
			detail = T("doc.conflictBigDetail")
		}
		return control.InfoBar{
			Open: true, Severity: control.SeverityWarning,
			Title: T("doc.conflict"), Message: detail,
			Action: T("doc.reloadFromDisk"), OnAction: func() { a.revert(t.doc) },
		}
	case a.settingsErr != nil && samePath(t.doc.path, a.settingsPath):
		return control.InfoBar{
			Open: true, Severity: control.SeverityError,
			Title: T("doc.settingsBroken"), Message: a.settingsErr.Error(),
		}
	case t.doc.gone:
		return control.InfoBar{
			Open: !t.noticeClosed, Severity: control.SeverityWarning,
			Title: T("doc.deleted"), Message: T("doc.deletedDetail"),
			OnClose: closeIt,
		}
	case c.Err != nil || t.doc.image != nil || t.doc.diff != nil:
		return control.InfoBar{}
	case t.doc.svgErr != nil:
		return control.InfoBar{
			Open: !t.noticeClosed, Severity: control.SeverityWarning,
			Title: T("doc.svgBroken"), Message: T("doc.svgShowingSource") + t.doc.svgErr.Error(),
			OnClose: closeIt,
		}
	case t.showsDrawing() && t.doc.svg.Skipped > 0:
		return control.InfoBar{
			Open: !t.noticeClosed, Severity: control.SeverityInfo,
			Title:   T("doc.svgSkipped", t.doc.svg.Skipped),
			Message: T("doc.svgSkippedDetail"),
			OnClose: closeIt,
		}
	case t.showsDrawing():
		return control.InfoBar{}

	case c.Binary:
		return control.InfoBar{
			Open: true, Severity: control.SeverityWarning,
			Title: T("common.binary"), Message: T("doc.binaryNotice"),
			Action: T("doc.openAsText"), OnAction: func() { a.openAsText(id) },
		}
	case c.Forced:
		return control.InfoBar{
			Open: !t.noticeClosed, Severity: control.SeverityInfo,
			Title: T("doc.forced"), Message: T("doc.forcedDetail"),
			OnClose: closeIt,
		}
	case c.Large:
		// A large file read as it is shown is no trouble, only something to
		// know; one read whole may be slow.
		if t.doc.big == nil {
			return control.InfoBar{
				Open: !t.noticeClosed, Severity: control.SeverityWarning,
				Title: T("doc.largeSlow", sizeText(c.Size)), OnClose: closeIt,
			}
		}
		// What a large file goes without, of what it would show.
		var left []string
		if t.doc.tracked {
			left = append(left, T("doc.largeNoGit"))
		}
		if isMarkdownPath(t.doc.path) {
			left = append(left, T("doc.largeNoOutline"))
		}
		if len(left) == 0 {
			// It goes without nothing it would show: nothing to say.
			return control.InfoBar{}
		}
		return control.InfoBar{
			Open: !t.noticeClosed, Severity: control.SeverityInfo,
			Title: T("doc.large", sizeText(c.Size)), Message: strings.Join(left, "\n"),
			OnClose: closeIt,
		}
	}
	return control.InfoBar{}
}

// breadcrumb splits path into the folders from the root of the workspace.
func breadcrumb(a *app, path string) []nav.BreadcrumbItem {
	var parts []string
	if a.ws != nil {
		if rel := a.ws.Rel(path); rel != "" && !strings.HasPrefix(rel, "../") {
			parts = append([]string{a.ws.Name()}, strings.Split(rel, "/")...)
		}
	}
	if parts == nil {
		parts = strings.Split(filepath.ToSlash(path), "/")
	}
	items := make([]nav.BreadcrumbItem, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			items = append(items, nav.BreadcrumbItem{Label: p})
		}
	}
	return items
}

// message fills a tab that has no text to show.
func message(th *theme.Theme, title, detail string) core.Widget {
	return basic.Center{Child: basic.Column{Size: layout.MainMin, Spacing: theme.Space8, Cross: layout.CrossCenter, Children: []core.Widget{
		basic.Text{Content: title, Style: th.Text.Title.WithColor(th.Color.Ink)},
		basic.Text{Content: detail, Style: th.Text.Body.WithColor(th.Color.Ink2)},
	}}}
}

// errorText is the Japanese for why a file did not open.
func errorText(err error) string {
	switch {
	case errors.Is(err, textfile.ErrTooLarge):
		return T("err.tooLarge", sizeText(textfile.MaxOpenSize))
	case errors.Is(err, os.ErrNotExist):
		return T("err.notExist")
	case errors.Is(err, os.ErrPermission):
		return T("err.permission")
	}
	return err.Error()
}

// sizeText writes a size of a file.
func sizeText(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return T("size.bytes", n)
}
