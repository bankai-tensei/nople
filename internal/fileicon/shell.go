package fileicon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"strings"

	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/paint"
	"golang.org/x/image/vector"
)

// The icons the shell of the OS shows for the files Nople opens, as
// Explorer draws them once Nople is the program of their type: a white page
// with its corner folded, and on it the logo of the language or the glyph
// of the kind, in its color, as the explorer of Nople shows it. A file of
// no known kind gets the page alone.

// shellSizes are the sizes of the pictures in an icon, the ones Explorer
// asks for at 100 to 200 percent scale.
var shellSizes = []int{16, 20, 24, 32, 40, 48, 64, 256}

// The page and its fold, on the grid of 24.
var (
	shellPage = filled("M6.5 2H14l6 6v12a2 2 0 0 1-2 2H6.5a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2z")
	shellFold = filled("M14 2l6 6h-4.5a1.5 1.5 0 0 1-1.5-1.5z")
	// shellEdge is the outline of the page and of its fold, which the
	// icon strokes at a width of its size.
	shellEdge = filled("M6.5 2H14l6 6v12a2 2 0 0 1-2 2H6.5a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2z M14 2v4.5a1.5 1.5 0 0 0 1.5 1.5H20")
)

// Colors of the page.
var (
	shellPaper = geom.Hex(0xFFFFFF)
	shellLine  = geom.Hex(0x9AA3AD)
	shellFoldC = geom.Hex(0xE3E7EB)
)

// shellGlyph is the logo or glyph of files with the extension ext (with its
// dot), fitted to the grid of 24, and its color; nil for an extension of no
// known kind.
func shellGlyph(ext string) (*paint.Path, geom.Color) {
	ext = strings.ToLower(ext)
	// A file named only by its extension, as .gitignore, is known by its
	// whole name.
	for _, name := range []string{"file" + ext, ext} {
		if p, c, ok := languageIcon(name, false); ok {
			return fitGlyph(p), c
		}
		if t, ok := typeOf(name); ok {
			if g := glyphOf(t.kind); g != nil {
				return g, t.color
			}
		}
	}
	return nil, geom.Color{}
}

// ShellIcon is the .ico file of the icon of files with the extension ext,
// such as ".go"; "" is the icon of a file of no known kind.
func ShellIcon(ext string) []byte {
	glyph, c := shellGlyph(ext)
	pics := make([][]byte, len(shellSizes))
	for i, n := range shellSizes {
		pic := shellPicture(n, glyph, c)
		if n < 256 {
			pics[i] = dib(pic)
			continue
		}
		var buf bytes.Buffer
		png.Encode(&buf, pic)
		pics[i] = buf.Bytes()
	}
	return ico(shellSizes, pics)
}

// macIconTypes are the types of the pictures in a .icns file, each a PNG of
// its size: 16, 32, 128, 256 and 512 points, at 1x and at 2x.
var macIconTypes = []struct {
	typ string
	n   int
}{
	{"icp4", 16}, {"ic11", 32}, {"icp5", 32}, {"ic12", 64},
	{"ic07", 128}, {"ic13", 256}, {"ic08", 256}, {"ic14", 512},
	{"ic09", 512}, {"ic10", 1024},
}

// MacIcon is the .icns file of the icon of files with the extension ext, as
// ShellIcon is the .ico one: the same page and glyph, for the document types
// of Nople.app, which Finder shows once Nople opens the files.
func MacIcon(ext string) []byte {
	glyph, c := shellGlyph(ext)
	pngs := map[int][]byte{}
	var body bytes.Buffer
	for _, t := range macIconTypes {
		pic, ok := pngs[t.n]
		if !ok {
			var buf bytes.Buffer
			png.Encode(&buf, shellPicture(t.n, glyph, c))
			pic = buf.Bytes()
			pngs[t.n] = pic
		}
		body.WriteString(t.typ)
		binary.Write(&body, binary.BigEndian, uint32(8+len(pic)))
		body.Write(pic)
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	return out.Bytes()
}

// shellPicture draws the icon at n by n pixels.
func shellPicture(n int, glyph *paint.Path, c geom.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	k := float32(n) / 24
	page := place{s: 1}
	fill(img, shellPage, k, page, shellPaper)
	fill(img, shellFold, k, page, shellFoldC)
	// About a pixel at small sizes, thinner than the grid's unit at large.
	edge := shellEdge.Stroke(paint.StrokeStyle{Width: max(1, float32(n)/56) / k, Join: paint.JoinRound})
	fill(img, edge, k, page, shellLine)
	if glyph != nil {
		// The glyph sits in the page below its fold, as large as the
		// width of the page lets it; small icons give it more of the page.
		box, cy := float32(11), float32(14.25)
		if n <= 24 {
			box, cy = 13, 14
		}
		fill(img, glyph, k, fitTo(glyph, box, 12.25, cy), c)
	}
	return img
}

// place scales a path on the grid by s, then moves it by dx, dy.
type place struct{ s, dx, dy float32 }

// fitTo places the path p so that its longer side is box, around the point
// x, y.
func fitTo(p *paint.Path, box, x, y float32) place {
	b := p.Bounds()
	s := box / max(b.W, b.H)
	return place{s: s, dx: x - (b.X+b.W/2)*s, dy: y - (b.Y+b.H/2)*s}
}

// fill fills the path p, placed by m and scaled by k from the grid to the
// pixels, in c over img.
func fill(img *image.RGBA, p *paint.Path, k float32, m place, c geom.Color) {
	n := img.Bounds().Dx()
	r := vector.NewRasterizer(n, n)
	at := func(o geom.Offset) (float32, float32) {
		return (o.X*m.s + m.dx) * k, (o.Y*m.s + m.dy) * k
	}
	open := false
	p.Walk(
		func(o geom.Offset) {
			if open {
				r.ClosePath()
			}
			open = true
			r.MoveTo(at(o))
		},
		func(o geom.Offset) { r.LineTo(at(o)) },
		func(cp, o geom.Offset) {
			x1, y1 := at(cp)
			x2, y2 := at(o)
			r.QuadTo(x1, y1, x2, y2)
		},
		func(a, b, o geom.Offset) {
			x1, y1 := at(a)
			x2, y2 := at(b)
			x3, y3 := at(o)
			r.CubeTo(x1, y1, x2, y2, x3, y3)
		},
		func() {
			r.ClosePath()
			open = false
		},
	)
	if open {
		r.ClosePath()
	}
	r.Draw(img, img.Bounds(), image.NewUniform(c.Std()), image.Point{})
}

// dib is the picture img as an icon holds it below 256 pixels: a 32-bit
// bitmap, its rows from the bottom up, in blue, green, red and straight
// alpha, then the mask of 1 bit a pixel, which the alpha makes moot. PNG
// holds only the picture of 256, as readers older than the shell of Vista,
// such as System.Drawing, take a PNG of a smaller size for a bitmap.
func dib(img *image.RGBA) []byte {
	n := img.Bounds().Dx()
	maskRow := (n + 31) / 32 * 4
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, struct {
		Size                  uint32
		Width, Height         int32
		Planes, Bits          uint16
		Compression, ImgSize  uint32
		XPerM, YPerM          int32
		ColorsUsed, Important uint32
	}{40, int32(n), int32(2 * n), 1, 32, 0, uint32(n*n*4 + maskRow*n), 0, 0, 0, 0})
	for y := n - 1; y >= 0; y-- {
		for x := 0; x < n; x++ {
			i := img.PixOffset(x, y)
			r, g, b, a := img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]
			if a != 0 && a != 0xFF {
				// image.RGBA is premultiplied; the icon is not.
				r, g, b = unpremultiply(r, a), unpremultiply(g, a), unpremultiply(b, a)
			}
			buf.Write([]byte{b, g, r, a})
		}
	}
	buf.Write(make([]byte, maskRow*n))
	return buf.Bytes()
}

// unpremultiply is the straight value of the premultiplied v at alpha a.
func unpremultiply(v, a uint8) uint8 { return uint8(min(255, (int(v)*255+int(a)/2)/int(a))) }

// ico packs the pictures of the sizes, bitmaps or PNG, into an .ico file.
func ico(sizes []int, pics [][]byte) []byte {
	var buf bytes.Buffer
	le := binary.LittleEndian
	binary.Write(&buf, le, [3]uint16{0, 1, uint16(len(pics))})
	offset := 6 + 16*len(pics)
	for i, n := range sizes {
		side := uint8(n)
		if n >= 256 {
			side = 0
		}
		binary.Write(&buf, le, struct {
			W, H, Colors, Reserved uint8
			Planes, Bits           uint16
			Size, Offset           uint32
		}{side, side, 0, 0, 1, 32, uint32(len(pics[i])), uint32(offset)})
		offset += len(pics[i])
	}
	for _, p := range pics {
		buf.Write(p)
	}
	return buf.Bytes()
}
