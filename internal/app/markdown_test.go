package app

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bankai-tensei/gowaku/theme"
)

// readme is a Markdown document with what a README has.
const readme = "# Nople のサンプル\n\n" +
	"コードを読むための**小さな**プロジェクト。[main.go](main.go) から読む。\n\n" +
	"## 使い方\n\n```go\nfunc main() {\n\tfmt.Println(\"hi\") // greet\n}\n```\n\n" +
	"- [文字列の関数](internal/util/strings.go)\n- [サイト](https://example.com/)\n- [下へ](#おわりに)\n\n" +
	"![ロゴ](logo.png)\n\n| 名前 | 中身 |\n|---|---|\n| main.go | 入口 |\n\n" +
	"## おわりに\n\n> 引用もこのとおり。\n"

// openReadme opens a sample folder whose README.md is readme, with a picture
// beside it, and opens the README.
func openReadme(t *testing.T, dark bool) (*harness, string) {
	t.Helper()
	h, dir := openSample(t, dark)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	m := image.NewNRGBA(image.Rect(0, 0, 96, 32))
	for y := range 32 {
		for x := range 96 {
			m.SetNRGBA(x, y, color.NRGBA{uint8(x * 2), 0x80, 0xE0, 0xFF})
		}
	}
	f, err := os.Create(filepath.Join(dir, "logo.png"))
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(f, m)
	f.Close()
	h.app.openFile(filepath.Join(dir, "README.md"), false)
	h.settle()
	return h, dir
}

func TestMarkdown_Rendered(t *testing.T) {
	h, dir := openReadme(t, false)
	tb := h.app.currentTab()
	if !tb.showsMarkdown() {
		t.Fatal("README.md should show rendered")
	}
	for _, s := range []string{"Nople のサンプル", "使い方", "おわりに", "Markdown プレビュー", "見出し 3"} {
		if !h.has(s) {
			t.Errorf("%q should show:\n%s", s, h.tt.Dump())
		}
	}
	if h.has("# Nople のサンプル") {
		t.Error("the marks of a heading should not show")
	}
	h.shot("phase4-markdown")

	// A link to a file opens it in a tab; one to the web goes to the browser.
	h.app.followLink(tb.id, "https://example.com/")
	if len(h.sh.urls) != 1 || h.sh.urls[0] != "https://example.com/" {
		t.Errorf("urls = %v", h.sh.urls)
	}
	h.app.followLink(tb.id, "javascript:alert(1)")
	if len(h.sh.urls) != 1 {
		t.Errorf("a script link should not be followed, urls = %v", h.sh.urls)
	}
	h.app.followLink(tb.id, "internal/util/strings.go")
	h.settle()
	if got := h.app.currentTab().doc.path; got != filepath.Join(dir, "internal", "util", "strings.go") {
		t.Errorf("the link should open strings.go, current = %s", got)
	}

	// Ctrl+Shift+V shows the source, in that tab only.
	h.app.activate(tb.id)
	h.app.toggleSource()
	h.settle()
	if tb.showsMarkdown() || h.has("Markdown プレビュー") {
		t.Errorf("the source should show:\n%s", h.tt.Dump())
	}
	if tb.doc.hl == nil {
		t.Error("the source of Markdown should be colored")
	}
	h.shot("phase4-markdown-source")
}

func TestMarkdown_Outline(t *testing.T) {
	h, _ := openReadme(t, true)
	h.app.showPanel(outlineID)
	h.settle()
	for _, s := range []string{"Nople のサンプル", "使い方", "おわりに"} {
		if n := len(h.tt.FindAll(s)); n < 2 {
			t.Errorf("%q should show in the outline and the document, found %d", s, n)
		}
	}
	h.shot("phase4-outline-dark")
}

func TestHighlightCode_SpellsCode(t *testing.T) {
	p := paletteOf(theme.Default().Light)
	code := "func main() {\n\tfmt.Println(\"hi\") // greet\n}\n"
	spans := highlightCode(p, "go", code)
	if spans == nil {
		t.Fatal("go code should be colored")
	}
	var b strings.Builder
	colored := false
	for _, s := range spans {
		b.WriteString(s.Text)
		colored = colored || s.Style.Color.A > 0
	}
	if b.String() != code || !colored {
		t.Errorf("spans spell %q, colored = %v", b.String(), colored)
	}
	if highlightCode(p, "nosuchlanguage", code) != nil || highlightCode(p, "", code) != nil {
		t.Error("an unknown language should stay in one color")
	}
}

// TestMarkdown_Long times a document of thousands of lines (PLAN §5 #8).
func TestMarkdown_Long(t *testing.T) {
	h, dir := openSample(t, false)
	var b strings.Builder
	for i := range 600 {
		fmt.Fprintf(&b, "## 節 %d\n\n本文の段落です。**強調**と `code` と [リンク](#節-%d) を含む。日本語の文が折り返すまで続く。\n\n", i, i)
		fmt.Fprintf(&b, "```go\nfunc f%d() int { return %d }\n```\n\n- 項目 a\n- 項目 b\n\n", i, i)
	}
	src := b.String()
	path := filepath.Join(dir, "long.md")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	h.app.openFile(path, false)
	h.settle()
	t.Logf("%d lines open and draw in %v", strings.Count(src, "\n"), time.Since(start))
	if !h.has("節 0") {
		t.Errorf("the first heading should show")
	}
	start = time.Now()
	h.app.currentTab().md.ShowHeading("節-599")
	h.settle()
	t.Logf("jump to the last heading in %v", time.Since(start))
	if !h.has("節 599") {
		t.Errorf("the last heading should show after the jump")
	}
}

// TestMarkdown_CopyCode checks that the Copy button of a code block puts the
// code on the clipboard, tabs and all.
func TestMarkdown_CopyCode(t *testing.T) {
	h, _ := openReadme(t, false)
	label := "Copy"
	if h.has("コピー") {
		label = "コピー"
	}
	h.tt.Tap(label)
	h.settle()
	if got, want := h.tt.Clipboard(), "func main() {\n\tfmt.Println(\"hi\") // greet\n}\n"; got != want && got != strings.TrimSuffix(want, "\n") {
		t.Errorf("clipboard = %q", got)
	}
}
