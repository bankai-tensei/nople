package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bankai-tensei/nople/internal/textfile"

	"golang.org/x/text/encoding/japanese"

	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/input"
	"github.com/bankai-tensei/gowaku/widgets/editor"
)

// openSample opens the sample folder in a new harness.
func openSample(t *testing.T, dark bool) (*harness, string) {
	t.Helper()
	dir := sampleFolder(t)
	h := newHarness(t, dark)
	if err := h.app.openFolder(dir); err != nil {
		t.Fatal(err)
	}
	h.settle()
	h.app.dock.HideFlyout()
	h.settle()
	return h, dir
}

// at is a position in a text.
func at(line, col int) editor.TextPosition { return editor.TextPosition{Line: line, Column: col} }

func TestTabs_KeepTheirPlace(t *testing.T) {
	h, dir := openSample(t, false)
	h.app.openFile(filepath.Join(dir, "main.go"), false)
	h.settle()
	mainTab := h.app.current
	h.app.currentTab().view.SetSelection(editor.TextRange{Start: at(5, 0), End: at(5, 4)})
	h.app.openFile(filepath.Join(dir, "go.mod"), false)
	h.settle()
	if h.has("行 6、列 5（4 文字を選択）") {
		t.Fatal("the status bar should follow the new tab")
	}
	h.app.activate(mainTab)
	h.settle()
	if !h.has("行 6、列 5（4 文字を選択）") {
		t.Errorf("going back to a tab should bring back its selection:\n%s", h.tt.Dump())
	}
	// Opening the file again shows its tab rather than a second one.
	h.app.openFile(filepath.Join(dir, "main.go"), false)
	h.settle()
	if len(h.app.tabs) != 2 || h.app.current != mainTab {
		t.Errorf("an open file should show in its tab: tabs = %d", len(h.app.tabs))
	}
}

func TestTabs_SplitRightSharesTheText(t *testing.T) {
	h, dir := openSample(t, false)
	h.app.openFile(filepath.Join(dir, "main.go"), true)
	h.settle()
	left := h.app.currentTab()
	left.view.SetSelection(editor.TextRange{Start: at(6, 1), End: at(6, 1)})
	h.app.splitRight()
	h.settle()
	right := h.app.currentTab()
	if right == left || right.doc != left.doc {
		t.Fatalf("the split should be a new tab of the same document")
	}
	if left.preview {
		t.Error("splitting should keep the preview tab")
	}
	if got := right.view.Selection(); got.End != at(6, 1) {
		t.Errorf("the new tab should start at the caret of the old one, got %+v", got)
	}
	root := h.app.dock.Arrangement().Root
	if len(root.Children) != 2 {
		t.Fatalf("the split should make two groups side by side: %+v", root)
	}
	// Each keeps its own selection.
	right.view.SetSelection(editor.TextRange{Start: at(0, 0), End: at(0, 7)})
	h.settle()
	if left.view.Selection().End != at(6, 1) {
		t.Error("selecting in one pane moved the other")
	}
	h.shot("phase2-split")
	// Closing one pane leaves the document to the other.
	h.app.closeTab(right.id)
	h.settle()
	if h.app.tabs[left.id] == nil || h.app.current != left.id {
		t.Errorf("closing the right pane should leave the left current")
	}
}

func TestTabs_Keys(t *testing.T) {
	h, dir := openSample(t, false)
	h.app.openFile(filepath.Join(dir, "main.go"), false)
	h.app.openFile(filepath.Join(dir, "go.mod"), false)
	h.settle()
	h.tt.Tap("go.mod")
	h.settle()
	h.tt.Key(input.KeyTab, input.ModControl)
	h.settle()
	if got := h.app.currentTab().doc.title(); got != "main.go" {
		t.Errorf("Ctrl+Tab should go to the other tab, current = %s", got)
	}
	h.tt.Key(input.KeyW, input.ModControl)
	h.settle()
	if len(h.app.tabs) != 1 || h.app.currentTab().doc.title() != "go.mod" {
		t.Errorf("Ctrl+W should close the current tab, tabs = %d", len(h.app.tabs))
	}
}

func TestNotices_BinaryAndLarge(t *testing.T) {
	h, dir := openSample(t, false)
	h.app.openFile(filepath.Join(dir, "assets", "logo.bin"), false)
	h.settle()
	if !h.has("テキストとして開く") {
		t.Fatalf("a binary file should offer to open as text:\n%s", h.tt.Dump())
	}
	h.shot("phase2-binary")
	h.tt.Tap("テキストとして開く")
	h.settle()
	d := h.app.currentTab().doc
	if !d.content.Forced || !strings.Contains(d.ctrl.Text(), "PNG␀␀") {
		t.Errorf("opening as text should show the bytes, got %q", d.ctrl.Text())
	}

	// A large Markdown file goes without its outline, and says so.
	big := filepath.Join(dir, "big.md")
	line := strings.Repeat("x", 99) + "\n"
	if err := os.WriteFile(big, []byte(strings.Repeat(line, (textfile.LargeSize/len(line))+10)), 0o644); err != nil {
		t.Fatal(err)
	}
	h.app.openFile(big, false)
	h.settle()
	title := "サイズが大きいため、一部の表示を省いています（" + sizeText(h.app.currentTab().doc.content.Size) + "）"
	if !h.has(title) || !h.has("アウトラインは非表示になりました。") {
		t.Fatalf("a large Markdown file should say it has no outline:\n%s", h.tt.Dump())
	}
	h.app.closeNotice(h.app.current)
	h.settle()
	if h.has(title) {
		t.Error("the warning should close")
	}
}

func TestImage_PanAndZoom(t *testing.T) {
	h, dir := openSample(t, true)
	m := image.NewNRGBA(image.Rect(0, 0, 64, 32))
	for y := range 32 {
		for x := range 64 {
			m.Set(x, y, color.NRGBA{uint8(x * 4), uint8(y * 8), 200, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, m); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "assets", "tile.png")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	h.app.openFile(path, false)
	h.settle()
	tb := h.app.currentTab()
	if tb.doc.image == nil {
		t.Fatalf("a PNG should open as a picture: %v", tb.doc.content.Err)
	}
	if !h.has("64 × 32") || !h.has("100%") {
		t.Errorf("the status bar should tell the size and zoom:\n%s", h.tt.Dump())
	}
	h.shot("phase2-image")
	// The focus of a zoom is in the viewer's own coordinates: about its
	// middle (1252 by 679 here), the picture stays in the middle.
	tb.viewer.ZoomBy(4, geom.Offset{X: 626, Y: 340}, false)
	h.settle()
	if !h.has("400%") {
		t.Errorf("zooming should show in the status bar:\n%s", h.tt.Dump())
	}
	h.shot("phase2-image-zoom")

	// The wheel zooms without Ctrl, as in an image viewer.
	before := tb.scale
	h.tt.Scroll(tb.doc.title(), 0, 3, 0)
	h.settle()
	if tb.scale <= before {
		t.Errorf("turning the wheel away should zoom in: %v -> %v", before, tb.scale)
	}
}

func TestShiftJIS(t *testing.T) {
	h, dir := openSample(t, false)
	data, err := japanese.ShiftJIS.NewEncoder().Bytes([]byte("日本語のメモ\r\n二行目\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "memo.txt")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	h.app.openFile(path, false)
	h.settle()
	if !h.has("Shift_JIS") || !h.has("CRLF") {
		t.Errorf("the status bar should tell the encoding:\n%s", h.tt.Dump())
	}
	if got := h.app.currentTab().doc.ctrl.Line(0); got != "日本語のメモ" {
		t.Errorf("line 0 = %q", got)
	}
}
