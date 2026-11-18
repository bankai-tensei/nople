package app

import (
	"github.com/bankai-tensei/nople/internal/syntax"

	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/theme"
	"github.com/bankai-tensei/gowaku/widgets/editor"
)

// palette is the look of each kind in one theme.
type palette [syntax.KindCount]editor.Decoration

// Palettes of the light and dark themes, made once.
var lightPalette, darkPalette *palette

// paletteOf returns the palette of th. Comments take the supporting ink of the
// theme and keywords its accent as text, so that the code sits in the window;
// the other hues read at 4.5:1 or more on the canvas of each theme.
func paletteOf(th *theme.Theme) *palette {
	dark := th.Brightness == theme.BrightnessDark
	if dark && darkPalette != nil {
		return darkPalette
	}
	if !dark && lightPalette != nil {
		return lightPalette
	}
	hue := func(light, dark uint32) geom.Color {
		if th.Brightness == theme.BrightnessDark {
			return geom.Hex(dark)
		}
		return geom.Hex(light)
	}
	c := th.Color
	p := &palette{
		syntax.KindComment:   {Color: c.Ink2, Italic: true},
		syntax.KindPreproc:   {Color: hue(0x9A3412, 0xFDBA74)},
		syntax.KindKeyword:   {Color: c.AccentInk, Bold: true},
		syntax.KindType:      {Color: hue(0x0F766E, 0x5EEAD4)},
		syntax.KindString:    {Color: hue(0x15803D, 0x86EFAC)},
		syntax.KindEscape:    {Color: hue(0xA16207, 0xFDE68A), Bold: true},
		syntax.KindNumber:    {Color: hue(0xB45309, 0xFCD34D)},
		syntax.KindFunction:  {Color: hue(0x7E22CE, 0xD8B4FE)},
		syntax.KindTag:       {Color: hue(0xBE123C, 0xFDA4AF)},
		syntax.KindAttribute: {Color: hue(0xB45309, 0xFCD34D)},
		syntax.KindBuiltin:   {Color: hue(0x0F766E, 0x5EEAD4)},
		syntax.KindHeading:   {Color: c.AccentInk, Bold: true},
		syntax.KindInserted:  {Color: hue(0x15803D, 0x86EFAC)},
		syntax.KindDeleted:   {Color: hue(0xB91C1C, 0xFCA5A5)},
		syntax.KindEmph:      {Italic: true},
		syntax.KindStrong:    {Bold: true},
	}
	if dark {
		darkPalette = p
	} else {
		lightPalette = p
	}
	return p
}

// decorate appends the decorations of line i, whose text is s, to dst.
func decorate(h *syntax.Highlight, p *palette, i int, s string, dst []editor.Decoration) []editor.Decoration {
	if h == nil || p == nil || i >= len(h.Lines) {
		return dst
	}
	for _, sp := range h.Lines[i] {
		start, end := int(sp.Start), min(int(sp.End), len(s))
		if start >= end {
			continue
		}
		d := p[sp.Kind]
		d.Start, d.End = start, end
		dst = append(dst, d)
	}
	return dst
}

// startHighlight lexes the text of d off the UI goroutine, and colors it
// when done. A binary file, a picture and a huge text stay plain.
func (a *app) startHighlight(d *document) {
	c := d.content
	if d.diff != nil || d.big != nil || d.wantBig || c.Err != nil || c.Binary || c.Forced || d.image != nil || len(c.Text) > syntax.MaxSize {
		return
	}
	lex := lexerOf(d.path, c.Text)
	if lex == nil {
		return
	}
	text := d.ctrl.Text()
	a.shell.async(func() func() {
		h := lex(text)
		return func() {
			d.hl = h
			d.ctrl.Redecorate()
		}
	})
}

// lexerOf returns what colors the text of the file at path, judged by its
// name or else by sample, a part of its text: the colors of a log, or a lexer
// of chroma; nil for a text that stays plain.
func lexerOf(path, sample string) func(text string) *syntax.Highlight {
	if syntax.IsLogPath(path) {
		return syntax.LexLog
	}
	l := syntax.LexerOf(path, sample)
	if l == nil {
		return nil
	}
	return func(text string) *syntax.Highlight { return syntax.Lex(l, text) }
}
