package app

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/theme"
)

// luminance is the relative luminance of c, as WCAG 2 defines it.
func luminance(c geom.Color) float64 {
	f := func(v float32) float64 {
		x := float64(v)
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*f(c.R) + 0.7152*f(c.G) + 0.0722*f(c.B)
}

func TestPalette_Contrast(t *testing.T) {
	set := theme.Default()
	for _, th := range []*theme.Theme{set.Light, set.Dark} {
		p := paletteOf(th)
		bg := luminance(th.Color.Canvas)
		for k, d := range p {
			if d.Color == (geom.Color{}) {
				continue
			}
			l := luminance(d.Color)
			ratio := (max(l, bg) + 0.05) / (min(l, bg) + 0.05)
			if ratio < 4.5 {
				t.Errorf("%v: kind %d reads at %.2f:1 on the canvas", th.Brightness, k, ratio)
			}
		}
	}
}

func TestWindow_Highlight(t *testing.T) {
	for _, dark := range []bool{false, true} {
		h, _ := openSample(t, dark)
		// A real file of some length: this one.
		src, err := filepath.Abs("highlight.go")
		if err != nil {
			t.Fatal(err)
		}
		h.app.openFile(src, false)
		h.settle()
		d := h.app.currentTab().doc
		if d.hl == nil || d.pal == nil {
			t.Fatalf("highlight.go should be colored, hl = %v pal = %v", d.hl != nil, d.pal != nil)
		}
		h.app.dock.HideFlyout()
		h.settle()
		name := "phase3-go"
		if dark {
			name += "-dark"
		}
		h.shot(name)
	}
}
