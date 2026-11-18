package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bankai-tensei/gowaku/input"
)

func TestSVG_OpensDrawn(t *testing.T) {
	h, _ := openSample(t, false)
	logo, err := filepath.Abs(filepath.Join(repoRoot, "assets", "logo.svg"))
	if err != nil {
		t.Fatal(err)
	}
	h.app.openFile(logo, false)
	h.settle()
	tb := h.app.currentTab()
	if !tb.showsDrawing() {
		t.Fatalf("an SVG should open drawn: svgErr = %v", tb.doc.svgErr)
	}
	if !h.has("363 × 155") || !h.has("100%") {
		t.Errorf("the status bar should tell the size of the drawing:\n%s", h.tt.Dump())
	}
	if h.has("logo.svg") && tb.doc.ctrl.LineCount() < 2 {
		t.Error("the source should be read too, for Ctrl+Shift+V")
	}
	h.shot("phase2-svg")

	// Ctrl+Shift+V shows the source, and again the drawing.
	h.tt.Key(input.KeyV, input.ModControl|input.ModShift)
	h.settle()
	if tb.showsDrawing() || !h.has("行 1、列 1") {
		t.Fatalf("Ctrl+Shift+V should show the source:\n%s", h.tt.Dump())
	}
	h.shot("phase2-svg-source")
	h.tt.Key(input.KeyV, input.ModControl|input.ModShift)
	h.settle()
	if !tb.showsDrawing() {
		t.Error("Ctrl+Shift+V again should show the drawing")
	}
}

func TestSVG_SmallIconDarkAndSkipped(t *testing.T) {
	h, dir := openSample(t, true)
	icon, err := filepath.Abs(filepath.Join(repoRoot, "assets", "icon.svg"))
	if err != nil {
		t.Fatal(err)
	}
	h.app.openFile(icon, false)
	h.settle()
	if !h.app.currentTab().showsDrawing() {
		t.Fatal("icon.svg should open drawn")
	}
	h.shot("phase2-svg-icon-dark")

	// Text draws with the fonts of the window, and needs no notice.
	small := filepath.Join(dir, "badge.svg")
	body := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24">` +
		`<circle cx="12" cy="12" r="10" fill="#3B82F6"/><text x="6" y="17" font-size="14" fill="#fff">A</text></svg>`
	if err := os.WriteFile(small, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	h.app.openFile(small, false)
	h.settle()
	if d := h.app.currentTab().doc; d.svg.Skipped != 0 || h.has("描けない要素") {
		t.Errorf("text should draw, skipped = %d:\n%s", d.svg.Skipped, h.tt.Dump())
	}
	if got := shownSize(h.app.currentTab().doc); got.W != minDrawingSize || got.H != minDrawingSize {
		t.Errorf("a small drawing should show at %d, got %v", minDrawingSize, got)
	}
	h.shot("phase2-svg-text")

	// A pattern, which gowaku leaves out, says so.
	patterned := filepath.Join(dir, "pattern.svg")
	body = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24">` +
		`<defs><pattern id="p" width="4" height="4" patternUnits="userSpaceOnUse"><rect width="2" height="2"/></pattern></defs>` +
		`<rect width="24" height="24" fill="url(#p) red"/></svg>`
	if err := os.WriteFile(patterned, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	h.app.openFile(patterned, false)
	h.settle()
	if !h.has("描けない要素を 1 個省いています") {
		t.Errorf("a drawing with skipped elements should say so:\n%s", h.tt.Dump())
	}
}

func TestSVG_BrokenShowsSource(t *testing.T) {
	h, dir := openSample(t, false)
	bad := filepath.Join(dir, "broken.svg")
	if err := os.WriteFile(bad, []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"><path d=\"M 0 0 L\"/></svg"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.app.openFile(bad, false)
	h.settle()
	tb := h.app.currentTab()
	if tb.showsDrawing() || !h.has("SVG を描けません") {
		t.Errorf("a broken SVG should show its source with a warning:\n%s", h.tt.Dump())
	}
}
