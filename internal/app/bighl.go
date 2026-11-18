package app

import (
	"strings"

	"github.com/bankai-tensei/nople/internal/syntax"

	"github.com/bankai-tensei/gowaku/widgets/editor"
)

// A large file is colored by blocks of lines, as they come into view: each
// block is lexed off the UI goroutine, from some lines before it, so that a
// comment or string that opened just above is known. A block of lines too
// long to lex in a moment, as a file of a single line of JSON, stays plain.
const (
	hlBlock   = 256
	hlContext = 64
	// hlMaxBytes is the most text one block lexes.
	hlMaxBytes = 1 << 20
	// hlBlocks is how many blocks a file keeps colored.
	hlBlocks = 256
)

// bigHighlight is the colors of a large file, by block.
type bigHighlight struct {
	lex    func(text string) *syntax.Highlight
	blocks map[int]*hlBlockColors
	// gen counts the times the colors were thrown away, so that a lexing
	// that ends after does not come back.
	gen int
}

// hlBlockColors is the colors of a block, or nil h while it is lexed. upto is
// the number of lines the text had when it was asked for, so that a block cut
// short by the end of a file still loading is lexed again once it grew.
type hlBlockColors struct {
	h    *syntax.Highlight
	upto int
}

// startBigHighlight readies the colors of the large file of d, choosing the
// lexer from its name or its first lines.
func (a *app) startBigHighlight(d *document) {
	var sample strings.Builder
	for i := 0; i < min(d.ctrl.LineCount(), 200) && sample.Len() < 64<<10; i++ {
		sample.WriteString(d.ctrl.Line(i))
		sample.WriteByte('\n')
	}
	lex := lexerOf(d.path, sample.String())
	if lex == nil {
		d.bighl = nil
		return
	}
	d.bighl = &bigHighlight{lex: lex, blocks: map[int]*hlBlockColors{}}
}

// resetBigHighlight throws the colors of d away, after its lines moved, and
// has the editors ask for them again.
func (a *app) resetBigHighlight(d *document) {
	if d.bighl == nil {
		return
	}
	d.bighl.gen++
	clear(d.bighl.blocks)
	d.ctrl.Redecorate()
}

// decorateBig appends the decorations of line i of the large file of d to
// dst, and asks for the colors of its block when it has none yet.
func (a *app) decorateBig(d *document, i int, s string, dst []editor.Decoration) []editor.Decoration {
	bh := d.bighl
	b := i / hlBlock
	blk := bh.blocks[b]
	if blk != nil && blk.h == nil {
		return dst // being lexed
	}
	if blk != nil && (i-b*hlBlock < len(blk.h.Lines) || blk.upto >= d.ctrl.LineCount()) {
		return decorate(blk.h, d.pal, i-b*hlBlock, s, dst)
	}
	a.lexBlock(d, b)
	return dst
}

// lexBlock lexes block b of the large file of d off the UI goroutine.
func (a *app) lexBlock(d *document, b int) {
	bh := d.bighl
	if len(bh.blocks) >= hlBlocks {
		clear(bh.blocks)
	}
	n := d.ctrl.LineCount()
	first := b * hlBlock
	from, to := max(0, first-hlContext), min(n, first+hlBlock)
	blk := &hlBlockColors{upto: n}
	bh.blocks[b] = blk
	var text strings.Builder
	for i := from; i < to; i++ {
		text.WriteString(d.ctrl.Line(i))
		if text.Len() > hlMaxBytes {
			blk.h = &syntax.Highlight{}
			blk.upto = n + 1<<30 // never again
			return
		}
		if i < to-1 {
			text.WriteByte('\n')
		}
	}
	gen, lex, src := bh.gen, bh.lex, text.String()
	a.shell.async(func() func() {
		h := lex(src)
		return func() {
			if d.bighl != bh || bh.gen != gen || bh.blocks[b] != blk {
				return
			}
			// The lines before the block were only for the state of the lexer.
			if h == nil || len(h.Lines) < first-from {
				h = &syntax.Highlight{}
			} else {
				h = &syntax.Highlight{Lines: h.Lines[first-from:]}
			}
			blk.h = h
			d.ctrl.RedecorateLines(first, to-1)
		}
	})
}
