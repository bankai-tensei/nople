package app

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/bankai-tensei/nople/assets"

	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/paint"
	"github.com/bankai-tensei/gowaku/semantics"
	"github.com/bankai-tensei/gowaku/theme"
	"github.com/bankai-tensei/gowaku/widgets/control"
	"github.com/bankai-tensei/gowaku/widgets/interact"
)

// The wordmark, in ink for the light theme and in paper for the dark one.
var (
	logoLight = mustSVG(assets.LogoLight)
	logoDark  = mustSVG(assets.LogoDark)
)

// mustSVG reads a drawing that ships with the program.
func mustSVG(data []byte) *paint.SVG {
	s, err := paint.ParseSVG(data)
	if err != nil {
		panic("nople: " + err.Error())
	}
	return s
}

// wordmark draws the logo of Nople, width wide, at the proportions of its
// view box. basic.SVG fits a drawing into a square, which suits an icon but
// leaves room above and below a wide logo, so it draws here by itself.
func wordmark(th *theme.Theme, width float32) core.Widget {
	s := logoLight
	if th.Brightness == theme.BrightnessDark {
		s = logoDark
	}
	vb := s.ViewBox
	height := width * vb.H / vb.W
	return interact.Semantics{Role: semantics.RoleImage, Label: "Nople", Child: control.CustomPaint{
		Width: width, Height: height,
		Painter: svgPainter{svg: s},
	}}
}

// svgPainter draws an SVG drawing over the whole of its box.
type svgPainter struct{ svg *paint.SVG }

// Paint implements layout.Painter.
func (p svgPainter) Paint(cv *paint.Canvas, sz geom.Size) {
	p.svg.Draw(cv, geom.RectFromSize(sz), geom.Color{})
}

// The icon of the window, which the title bar and the taskbar show (ADR 0111).
// The OS picks its sizes from this one picture.

// windowIcon decodes the icon of the window. On macOS the icon is the one of
// the application in the Dock: inside Nople.app it is nil, so that the Dock
// keeps the icon of the bundle, which leaves the margin of an icon of macOS
// round the shape, where icon.png fills the whole square and would show
// larger than the icons beside it while Nople runs.
func windowIcon() []image.Image {
	if runtime.GOOS == "darwin" && inAppBundle() {
		return nil
	}
	m, err := png.Decode(bytes.NewReader(assets.Icon))
	if err != nil {
		panic("nople: icon: " + err.Error())
	}
	return []image.Image{m}
}

// inAppBundle reports whether the running program is the executable of a
// macOS application bundle, also when it was started through a link to it,
// as /usr/local/bin/nople.
func inAppBundle() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	return strings.HasSuffix(filepath.Dir(exe), ".app/Contents/MacOS")
}
