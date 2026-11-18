package app

import (
	"github.com/bankai-tensei/nople/internal/git"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bankai-tensei/gowaku/widgets/editor"
)

// gitSample makes the sample folder a repository with one commit, then
// changes it: main.go edited and staged, strings.go edited, README.md
// deleted, new.txt untracked.
func gitSample(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := sampleFolder(t)
	git := func(args ...string) {
		t.Helper()
		if out, err := git.Run(dir, args...); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	git("-c", "user.name=t", "-c", "user.email=t@example.com", "add", ".")
	git("-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "first")
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.go", "package main\n\nimport \"fmt\"\n\n// main greets.\nfunc main() {\n\tfmt.Println(\"こんにちは、Nople\")\n\tfmt.Println(\"bye\")\n}\n")
	git("add", "main.go")
	write("internal/util/strings.go", "package util\n\n// Reverse returns s backwards, rune by rune.\nfunc Reverse(s string) string {\n\tr := []rune(s)\n\tfor i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {\n\t\tr[i], r[j] = r[j], r[i]\n\t}\n\treturn string(r)\n}\n\n// Upper is new.\nfunc Upper() {}\n")
	os.Remove(filepath.Join(dir, "README.md"))
	write("new.txt", "fresh\n")
	return dir
}

func TestRepo_Status(t *testing.T) {
	dir := gitSample(t)
	r, err := git.Read(dir)
	if err != nil || r == nil {
		t.Fatalf("readRepo: %v %v", r, err)
	}
	if r.Branch != "main" {
		t.Errorf("branch = %q", r.Branch)
	}
	if len(r.Staged) != 1 || r.Staged[0].Path != filepath.Join(dir, "main.go") || r.Staged[0].Code != 'M' {
		t.Errorf("staged = %+v", r.Staged)
	}
	codes := map[string]byte{}
	for _, c := range r.Unstaged {
		rel, _ := filepath.Rel(dir, c.Path)
		codes[filepath.ToSlash(rel)] = c.Code
	}
	want := map[string]byte{"internal/util/strings.go": 'M', "README.md": 'D', "new.txt": 'U'}
	for k, v := range want {
		if codes[k] != v {
			t.Errorf("unstaged %s = %q, want %q (%v)", k, codes[k], v, codes)
		}
	}
	if r2, _ := git.Read(t.TempDir()); r2 != nil {
		t.Error("a folder outside a repository should have none")
	}
}

func TestSCM_PanelAndDiff(t *testing.T) {
	dir := gitSample(t)
	h := newHarness(t, false)
	if err := h.app.openFolder(dir); err != nil {
		t.Fatal(err)
	}
	h.settle()
	h.app.dock.HideFlyout()
	h.settle()
	if h.app.repo == nil {
		t.Fatalf("the repository should be read, err = %v", h.app.repoErr)
	}
	if !h.has("ブランチ main") {
		t.Errorf("the status bar should name the branch:\n%s", h.tt.Dump())
	}
	h.app.showPanel(scmID)
	h.settle()
	for _, s := range []string{"ステージ済みの変更（1）", "変更（3）", "strings.go — internal/util", "new.txt"} {
		if !h.has(s) {
			t.Errorf("the panel should show %q:\n%s", s, h.tt.Dump())
		}
	}
	h.shot("phase6-scm")

	// The unstaged change of strings.go: two lines added, one changed.
	var c git.Change
	for _, x := range h.app.repo.Unstaged {
		if filepath.Base(x.Path) == "strings.go" {
			c = x
		}
	}
	h.app.openDiff(c, false)
	h.settle()
	h.app.dock.HideFlyout()
	h.settle()
	tb := h.app.currentTab()
	dd := tb.doc.diff
	if dd == nil || !dd.ready {
		t.Fatalf("the diff should load: %+v", dd)
	}
	if dd.added != 4 || dd.gone != 1 {
		t.Errorf("+%d −%d, want +4 −1 (%v)", dd.added, dd.gone, dd.hunks)
	}
	if _, ok := dd.rightIntra[2]; !ok {
		t.Errorf("the changed comment should mark what changed in it: %v", dd.rightIntra)
	}
	if !h.has("strings.go（作業ツリー）") || !h.has("+4 −1") {
		t.Errorf("the tab and the status bar should tell the diff:\n%s", h.tt.Dump())
	}
	h.shot("phase6-diff")

	// The two sides scroll together.
	dd.lv.OnScroll(editor.ScrollPosition{Line: 9})
	if got := dd.rv.Scroll().Line; got != 9 {
		t.Errorf("line 9 of the left is beside line %d of the right, want 9", got)
	}
	dd.rv.OnScroll(editor.ScrollPosition{Line: 12})
	if got := dd.lv.Scroll().Line; got > 10 {
		t.Errorf("line 12 of the right, which was added, is beside line %d of the left", got)
	}

	// Opening the same change again shows its tab.
	n := len(h.app.tabs)
	h.app.openDiff(c, false)
	if len(h.app.tabs) != n {
		t.Error("the same diff should open in its tab")
	}

	// A new file and a deleted one show against nothing.
	for _, x := range h.app.repo.Unstaged {
		h.app.openDiff(x, false)
		h.settle()
		if d := h.app.currentTab().doc.diff; !d.ready || d.err != nil {
			t.Errorf("%s: ready = %v err = %v", x.Path, d.ready, d.err)
		}
	}

	// An ordinary tab marks its changed lines in the gutter.
	h.app.openFile(filepath.Join(dir, "internal", "util", "strings.go"), false)
	h.settle()
	m := h.app.currentTab().doc.marks
	if m == nil || m.line[2] != markModified || m.line[11] != markAdded {
		t.Errorf("marks = %+v", m)
	}
	h.shot("phase6-gutter")
}
