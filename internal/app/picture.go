package app

import (
	"image"
	"image/color"

	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/paint"
	"github.com/bankai-tensei/gowaku/semantics"
	"github.com/bankai-tensei/gowaku/theme"
	"github.com/bankai-tensei/gowaku/widgets/basic"
	"github.com/bankai-tensei/gowaku/widgets/control"
	"github.com/bankai-tensei/gowaku/widgets/interact"
)

// minDrawingSize is the least length of the longer side of an SVG drawing on
// screen at 100%: an icon of 24 would be a speck, and a drawing stays sharp at
// any size.
const minDrawingSize = 256

// pictureSize is the size of the picture of d: its pixels, or the size an
// SVG drawing asks for.
func pictureSize(d *document) geom.Size {
	switch {
	case d.image != nil:
		return d.image.Size()
	case d.svg != nil:
		w, h := d.svg.Width, d.svg.Height
		if w <= 0 || h <= 0 {
			w, h = d.svg.ViewBox.W, d.svg.ViewBox.H
		}
		return geom.Size{W: w, H: h}
	}
	return geom.Size{}
}

// shownSize is the size the picture of d takes at 100%, before it shrinks to
// fit the tab: an image its pixels, a small drawing scaled up.
func shownSize(d *document) geom.Size {
	sz := pictureSize(d)
	if d.svg != nil && sz.W > 0 && sz.H > 0 {
		if long := max(sz.W, sz.H); long < minDrawingSize {
			k := minDrawingSize / long
			sz = geom.Size{W: sz.W * k, H: sz.H * k}
		}
	}
	return sz
}

// pictureView shows the picture of tab t, an image or an SVG drawing, which
// the user pans and zooms. It starts at shownSize, or smaller to fit the tab,
// in the middle, over a checkerboard that shows where it is transparent.
func pictureView(th *theme.Theme, a *app, t *tab) core.Widget {
	d := t.doc
	sz := shownSize(d)
	id := t.id
	return basic.Container{Color: th.Color.Sidebar, Child: interact.InteractiveViewer{
		Controller:    &t.viewer,
		MinScale:      0.05,
		MaxScale:      32,
		Constrained:   true,
		DoubleTapZoom: true,
		WheelZooms:    true,
		Keys:          interact.ViewerKeysZoom,
		Label:         d.title(),
		OnChange:      func(scale float32, off geom.Offset) { a.zoomed(id, scale, off) },
		Child: basic.Center{Child: basic.ConstrainedBox{MaxWidth: sz.W, MaxHeight: sz.H, Child: interact.Semantics{
			Role: semantics.RoleImage, Label: d.title(),
			Child: control.CustomPaint{Painter: picturePainter{img: d.image, svg: d.svg, size: sz}},
		}}},
	}}
}

// The checkerboard behind a picture: squares of checkerSquare, drawn as tiles
// of one small image so that a large picture takes few draws.
const (
	checkerSquare = 8
	checkerTile   = 8 * checkerSquare
)

var checkerImage = func() *paint.Image {
	m := image.NewNRGBA(image.Rect(0, 0, checkerTile, checkerTile))
	light, dark := color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}, color.NRGBA{0xE4, 0xE4, 0xE7, 0xFF}
	for y := range checkerTile {
		for x := range checkerTile {
			c := light
			if (x/checkerSquare+y/checkerSquare)%2 == 1 {
				c = dark
			}
			m.SetNRGBA(x, y, c)
		}
	}
	return paint.NewImage(m)
}()

// picturePainter draws an image or an SVG drawing as large as fits its box
// at the proportions of size, in the middle, over the checkerboard.
type picturePainter struct {
	img  *paint.Image
	svg  *paint.SVG
	size geom.Size
}

// Paint implements layout.Painter.
func (p picturePainter) Paint(cv *paint.Canvas, sz geom.Size) {
	if p.size.W <= 0 || p.size.H <= 0 {
		return
	}
	k := min(sz.W/p.size.W, sz.H/p.size.H)
	w, h := p.size.W*k, p.size.H*k
	dst := geom.Rect{X: (sz.W - w) / 2, Y: (sz.H - h) / 2, W: w, H: h}
	cv.Save()
	cv.ClipRect(dst)
	for y := dst.Y; y < dst.Y+dst.H; y += checkerTile {
		for x := dst.X; x < dst.X+dst.W; x += checkerTile {
			cv.DrawImageQuality(checkerImage, geom.Rect{X: x, Y: y, W: checkerTile, H: checkerTile}, 1, paint.FilterNearest)
		}
	}
	cv.Restore()
	switch {
	case p.img != nil:
		cv.DrawImage(p.img, dst, 1)
	case p.svg != nil:
		p.svg.Draw(cv, dst, geom.Color{A: 1})
	}
}
