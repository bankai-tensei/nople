package app

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bankai-tensei/gowaku/input"
)

// waitFor settles the window until ok, or fails after a few seconds.
func (h *harness) waitFor(what string, ok func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			h.t.Fatalf("timed out waiting for %s; screens:\n%s", what, h.screens())
		}
		time.Sleep(20 * time.Millisecond)
		h.settle()
	}
}

func TestTerminal_RunsShell(t *testing.T) {
	h, dir := openSample(t, false)
	if runtime.GOOS == "windows" {
		h.app.shellCmd = []string{"cmd.exe", "/q", "/k", "prompt $g"}
	} else {
		h.app.shellCmd = []string{"/bin/sh"}
	}
	h.app.newTerminal()
	h.settle()
	if len(h.app.termOrder) != 1 {
		t.Fatal("a terminal should open")
	}
	term := h.app.terms[h.app.termOrder[0]]
	if term.err != nil {
		t.Fatalf("the shell should start: %v", term.err)
	}
	if term.ro == nil || !term.ro.focused {
		t.Error("a new terminal should take the keyboard")
	}
	h.waitFor("the prompt", func() bool { return strings.Contains(term.vt.ScreenText(), ">") })

	// Typing goes to the shell, and Enter runs it.
	h.tt.Type("echo nople-は-ok")
	h.tt.Key(input.KeyEnter, 0)
	h.waitFor("the echo", func() bool { return strings.Count(term.vt.ScreenText(), "nople-は-ok") >= 2 })
	h.shot("phase7-terminal")

	// The size follows the panel.
	if term.vt.Cols < 40 || term.vt.Rows < 3 {
		t.Errorf("the grid should fill the panel: %d×%d", term.vt.Cols, term.vt.Rows)
	}

	// Ctrl+J slides the panel down and brings it back.
	h.tt.Key(input.KeyJ, input.ModControl)
	h.settle()
	hidden := false
	for _, x := range h.app.dock.Arrangement().Hidden {
		if strings.Contains(strings.Join(x.Origin, ","), term.id) || x.ID == term.id {
			hidden = true
		}
	}
	if !hidden {
		t.Errorf("Ctrl+J should hide the terminal: %+v", h.app.dock.Arrangement())
	}
	h.app.toggleTerminal()
	h.settle()

	// A new file still opens among the editors, not with the terminals.
	h.app.openFile(dir+"/main.go", false)
	h.settle()
	if g := groupOf(h.app.dock.Arrangement(), h.app.current); g == nil || slicesHasTerminal(g) {
		t.Errorf("the file should open apart from the terminal, group = %v", g)
	}

	h.app.closeTerminals()
	if len(h.app.terms) != 0 {
		t.Error("closing should end the terminals")
	}
}

func slicesHasTerminal(ids []string) bool {
	for _, id := range ids {
		if isTerminal(id) {
			return true
		}
	}
	return false
}

func TestKeySequence(t *testing.T) {
	cases := []struct {
		k                    input.KeyCode
		ctrl, shift, alt, ac bool
		want                 string
	}{
		{input.KeyUp, false, false, false, false, "\x1b[A"},
		{input.KeyUp, false, false, false, true, "\x1bOA"},
		{input.KeyRight, true, false, false, false, "\x1b[1;5C"},
		{input.KeyC, true, false, false, false, "\x03"},
		{input.KeyB, false, false, true, false, "\x1bb"},
		{input.KeyDelete, false, false, false, false, "\x1b[3~"},
		{input.KeyF5, false, false, false, false, "\x1b[15~"},
		{input.KeyTab, false, true, false, false, "\x1b[Z"},
		{input.KeyA, false, false, false, false, ""},
	}
	for _, c := range cases {
		if got := keySequence(c.k, c.ctrl, c.shift, c.alt, c.ac); got != c.want {
			t.Errorf("%v ctrl=%v shift=%v alt=%v: %q, want %q", c.k, c.ctrl, c.shift, c.alt, got, c.want)
		}
	}
}

func TestTerminal_Colors(t *testing.T) {
	h, _ := openSample(t, true)
	h.app.shellCmd = []string{"no-such-shell-for-nople"}
	h.app.newTerminal()
	h.settle()
	term := h.app.terms[h.app.termOrder[0]]
	if term.err == nil {
		t.Fatal("a missing shell should fail to start")
	}
	// Draw a palette and text straight into the screen.
	var b strings.Builder
	b.WriteString("\x1b[2J\x1b[H")
	for i := range 16 {
		b.WriteString("\x1b[48;5;" + itoa(i) + "m  ")
	}
	b.WriteString("\x1b[0m\r\n\x1b[1;32mgreen bold\x1b[0m \x1b[3;38;2;255;128;0mitalic orange\x1b[0m \x1b[4munderline\x1b[0m \x1b[7minverse\x1b[0m\r\n日本語の全角もセル 2 つ分 | ascii\r\n")
	term.output([]byte(b.String()))
	h.settle()
	h.shot("phase7-colors-dark")
	if !strings.Contains(term.vt.ScreenText(), "日本語の全角") {
		t.Errorf("screen = %q", term.vt.ScreenText())
	}
}

func itoa(i int) string {
	return strings.TrimSpace(strings.Repeat(" ", 0) + string(rune('0'+i/10)) + string(rune('0'+i%10)))
}

// screens are the texts of the terminals, for a failure.
func (h *harness) screens() string {
	var sb strings.Builder
	for _, id := range h.app.termOrder {
		t := h.app.terms[id]
		sb.WriteString(id + " exited=" + map[bool]string{true: "yes", false: "no"}[t.exited] + ":\n" + t.vt.ScreenText() + "\n")
	}
	return sb.String()
}
