package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bankai-tensei/nople/internal/config"
	"github.com/bankai-tensei/nople/internal/workspace"

	"github.com/bankai-tensei/gowaku/gowakutest"
	"github.com/bankai-tensei/gowaku/widgets/editor"
)

// harness runs the window of an app in a test.
type harness struct {
	t   *testing.T
	sh  *localShell
	app *app
	tt  *gowakutest.Tester
}

// newHarness mounts the window with nothing open.
func newHarness(t *testing.T, dark bool) *harness {
	t.Helper()
	sh := &localShell{}
	a := newApp(sh)
	tt := gowakutest.New(t, windowView{app: a}, gowakutest.Options{Width: windowWidth, Height: windowHeight, Dark: dark, Settle: 300 * time.Millisecond})
	h := &harness{t: t, sh: sh, app: a, tt: tt}
	h.settle()
	return h
}

// settle runs what was posted and draws until the window rests.
func (h *harness) settle() {
	h.sh.flush()
	for range 20 {
		h.tt.Settle()
		if !h.sh.flush() {
			return
		}
	}
}

// has reports whether something on screen is labeled label.
func (h *harness) has(label string) bool {
	_, ok := h.tt.Find(label)
	return ok
}

// shot saves a screenshot under docs/shots when NOPLE_SHOTS is set.
func (h *harness) shot(name string) {
	if os.Getenv("NOPLE_SHOTS") == "" {
		return
	}
	h.tt.SaveScreenshot(filepath.Join(repoRoot, "docs", "shots", name+".png"))
}

// sampleFolder makes a small project to open.
func sampleFolder(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "sample")
	files := map[string]string{
		"main.go":                  "package main\n\nimport \"fmt\"\n\n// main says hello.\nfunc main() {\n\tfmt.Println(\"こんにちは、Nople\")\n}\n",
		"README.md":                "# sample\n\nA project to read.\n",
		"go.mod":                   "module sample\n\ngo 1.27\n",
		".gitignore":               "build/\n*.log\n",
		"debug.log":                "noise\n",
		"build/out.txt":            "built\n",
		"internal/util/strings.go": "package util\n\n// Reverse returns s backwards.\nfunc Reverse(s string) string {\n\tr := []rune(s)\n\tfor i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {\n\t\tr[i], r[j] = r[j], r[i]\n\t}\n\treturn string(r)\n}\n",
		"assets/logo.bin":          "PNG\x00\x00\x01",
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestWindow_Empty(t *testing.T) {
	h := newHarness(t, false)
	if !h.has("フォルダーを開く") {
		t.Errorf("the welcome page should offer to open a folder:\n%s", h.tt.Dump())
	}
	if got := h.sh.title; got != "Nople" {
		t.Errorf("title = %q, want Nople", got)
	}
	h.shot("phase1-empty")
}

func TestWindow_EmptyDark(t *testing.T) {
	h := newHarness(t, true)
	if !h.has("Nople") {
		t.Errorf("the wordmark should be named for screen readers:\n%s", h.tt.Dump())
	}
	h.shot("phase1-empty-dark")
}

func TestWindow_OpenFolderAndRead(t *testing.T) {
	dir := sampleFolder(t)
	h := newHarness(t, false)
	if err := h.app.openFolder(dir); err != nil {
		t.Fatal(err)
	}
	h.settle()
	if h.app.dock.Flyout() != explorerID {
		t.Errorf("the explorer should slide out when a folder opens, flyout = %q", h.app.dock.Flyout())
	}
	for _, name := range []string{"main.go", "README.md", "internal", ".gitignore"} {
		if !h.has(name) {
			t.Errorf("the explorer should show %s:\n%s", name, h.tt.Dump())
		}
	}
	for _, name := range []string{"debug.log", "build"} {
		if !h.has(name) {
			t.Errorf("the explorer should show %s, as .gitignore is not followed by default", name)
		}
	}

	// A click opens a preview tab; the next click reuses it.
	h.tt.Tap("main.go")
	h.settle()
	if len(h.app.tabs) != 1 || h.app.previewID == "" {
		t.Fatalf("a click should open one preview tab, docs = %d preview = %q", len(h.app.tabs), h.app.previewID)
	}
	if h.app.dock.Flyout() != explorerID {
		t.Errorf("a preview should leave the explorer out, flyout = %q", h.app.dock.Flyout())
	}
	first := h.app.previewID
	h.shot("phase1-preview")
	h.tt.Tap("README.md")
	h.settle()
	if len(h.app.tabs) != 1 || h.app.previewID != first || h.app.currentTab().doc.title() != "README.md" {
		t.Errorf("the second click should reuse the preview tab: docs = %d, current = %v", len(h.app.tabs), h.app.currentTab())
	}
	h.app.dock.HideFlyout()
	h.settle()
	h.shot("phase1-preview-tab")

	// Opening on purpose keeps the tab, and the next file gets its own.
	h.app.openFile(filepath.Join(dir, "README.md"), false)
	h.app.openFile(filepath.Join(dir, "go.mod"), false)
	h.settle()
	if len(h.app.tabs) != 2 || h.app.previewID != "" {
		t.Errorf("two kept tabs expected, docs = %d preview = %q", len(h.app.tabs), h.app.previewID)
	}
	if got := h.sh.title; got != "go.mod — sample — Nople" {
		t.Errorf("title = %q", got)
	}
	if !h.has("行 1、列 1") || !h.has("UTF-8") || !h.has("Go Module") {
		t.Errorf("the status bar should tell the caret and the file:\n%s", h.tt.Dump())
	}

	// The editor is read only.
	tb := h.app.currentTab()
	d := tb.doc
	// The status bar follows the caret as soon as it moves.
	tb.view.SetSelection(editor.TextRange{Start: editor.TextPosition{Line: 2, Column: 0}, End: editor.TextPosition{Line: 2, Column: 4}})
	h.settle()
	if !h.has("行 3、列 5（4 文字を選択）") {
		t.Errorf("the status bar should tell the moved caret:\n%s", h.tt.Dump())
	}
	// Since phase 9 the text can be edited; typing makes it unsaved.
	h.app.settings.Autosave = config.AutosaveOff
	h.tt.Tap(d.title())
	h.tt.Type("xyz")
	h.settle()
	if !strings.Contains(d.ctrl.Text(), "xyz") || !d.dirty {
		t.Errorf("typing should edit the document")
	}
	d.dirty = false

	// Closing the last tab brings the welcome page back.
	h.app.closeAll()
	h.settle()
	if len(h.app.tabs) != 0 || !h.has("ようこそ") {
		t.Errorf("closing every tab should bring back the welcome page:\n%s", h.tt.Dump())
	}
}

func TestWindow_LazyFolderAndBinary(t *testing.T) {
	dir := sampleFolder(t)
	h := newHarness(t, true)
	if err := h.app.openFolder(dir); err != nil {
		t.Fatal(err)
	}
	h.settle()
	internal := filepath.Join(dir, "internal")
	if h.app.ws.Loaded(internal) {
		t.Fatal("a closed folder should not be read yet")
	}
	h.app.tree.Expand(internal)
	h.settle()
	h.app.tree.Expand(filepath.Join(internal, "util"))
	h.settle()
	if !h.has("strings.go") {
		t.Fatalf("opening a folder should read it:\n%s", h.tt.Dump())
	}
	h.app.openFile(filepath.Join(internal, "util", "strings.go"), false)
	h.settle()
	h.shot("phase1-dark")

	h.app.openFile(filepath.Join(dir, "assets", "logo.bin"), false)
	h.settle()
	if !h.has("バイナリファイルです") {
		t.Errorf("a binary file should say so:\n%s", h.tt.Dump())
	}
}

func TestWindow_PinnedExplorerKeepsEditorsApart(t *testing.T) {
	dir := sampleFolder(t)
	h := newHarness(t, false)
	if err := h.app.openFolder(dir); err != nil {
		t.Fatal(err)
	}
	h.settle()
	h.app.dock.Pin(explorerID)
	h.settle()
	h.app.openFile(filepath.Join(dir, "main.go"), false)
	h.settle()
	arr := h.app.dock.Arrangement()
	if g := groupOf(arr, explorerID); len(g) != 1 {
		t.Errorf("the explorer's group should hold only the explorer, got %v", g)
	}
	h.shot("phase1-pinned")
	// Closing the document puts the welcome page beside the explorer, not in
	// its group.
	h.app.closeTab(h.app.current)
	h.settle()
	arr = h.app.dock.Arrangement()
	if g := groupOf(arr, welcomeID); len(g) != 1 {
		t.Errorf("the welcome page should have a group of its own, got %v (%+v)", g, arr.Root)
	}
}

func TestBreadcrumb(t *testing.T) {
	a := newApp(&localShell{})
	root := filepath.Join(t.TempDir(), "proj")
	a.ws = workspace.New(root, false)
	var got []string
	for _, it := range breadcrumb(a, filepath.Join(root, "a", "b.go")) {
		got = append(got, it.Label)
	}
	if strings.Join(got, "/") != "proj/a/b.go" {
		t.Errorf("breadcrumb = %v", got)
	}
}

// repoRoot is the top of the repository, two folders above this package.
var repoRoot = filepath.Join("..", "..")
