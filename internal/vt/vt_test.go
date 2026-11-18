package vt

import (
	"strings"
	"testing"
)

func TestVT_TextAndWrap(t *testing.T) {
	v := New(10, 3, nil)
	v.Write([]byte("hello\r\nworld 日本語です"))
	want := "hello\nworld 日本\n語です"
	if got := v.ScreenText(); got != want {
		t.Errorf("screen =\n%q\nwant\n%q", got, want)
	}
	// The wide character that does not fit wraps whole.
	if c := v.Lines[1].Cells[6]; c.R != '日' || c.Attr&AttrWide == 0 || v.Lines[1].Cells[7].Attr&AttrWideTail == 0 {
		t.Errorf("cell = %+v", c)
	}
	// Scrolling keeps what left the top.
	v.Write([]byte("\r\nnext"))
	if len(v.Scrollback) != 1 || LineText(v.Scrollback[0], 0, 10) != "hello" {
		t.Errorf("scrollback = %d", len(v.Scrollback))
	}
}

func TestVT_CursorAndErase(t *testing.T) {
	v := New(10, 3, nil)
	v.Write([]byte("abcdefghij\x1b[2;3Hxy\x1b[1;5H\x1b[K\x1b[3;1Hzz\x1b[1D\x1b[P"))
	want := "abcd\n  xy\nz"
	if got := v.ScreenText(); got != want {
		t.Errorf("screen =\n%q\nwant\n%q", got, want)
	}
	v.Write([]byte("\x1b[2J"))
	if strings.TrimSpace(v.ScreenText()) != "" {
		t.Errorf("2J should clear: %q", v.ScreenText())
	}
}

func TestVT_ColorsAndModes(t *testing.T) {
	var replies []string
	v := New(20, 4, func(b []byte) { replies = append(replies, string(b)) })
	v.Write([]byte("\x1b[1;31mR\x1b[38;5;208mO\x1b[38;2;10;20;30;48;2;1;2;3mT\x1b[0mn"))
	l := v.Lines[0].Cells
	if l[0].FG != indexed(1) || l[0].Attr&AttrBold == 0 || l[1].FG != indexed(208) || l[2].FG != rgb(10, 20, 30) || l[2].BG != rgb(1, 2, 3) || l[3].FG != ColorDefault {
		t.Errorf("cells = %+v", l[:4])
	}
	v.Write([]byte("\x1b[6n\x1b]0;my title\x07"))
	if len(replies) != 1 || replies[0] != "\x1b[1;5R" || v.Title != "my title" {
		t.Errorf("replies = %q title = %q", replies, v.Title)
	}
	// The alternate screen comes and goes, and the main one is as it was.
	v.Write([]byte("\x1b[?1049h\x1b[Halt"))
	if !strings.HasPrefix(v.ScreenText(), "alt") {
		t.Errorf("alt screen = %q", v.ScreenText())
	}
	v.Write([]byte("\x1b[?1049l"))
	if !strings.HasPrefix(v.ScreenText(), "ROTn") {
		t.Errorf("main screen = %q", v.ScreenText())
	}
	// A sequence cut between two writes still parses, and so does UTF-8.
	v.Write([]byte("\r\n\x1b[3"))
	v.Write([]byte("2mG"))
	b := []byte("あ")
	v.Write(b[:1])
	v.Write(b[1:])
	if v.Lines[1].Cells[0].FG != indexed(2) || v.Lines[1].Cells[1].R != 'あ' {
		t.Errorf("line = %+v", v.Lines[1].Cells[:3])
	}
}

func TestVT_ScrollRegionAndResize(t *testing.T) {
	v := New(5, 4, nil)
	v.Write([]byte("1\r\n2\r\n3\r\n4\x1b[2;3r\x1b[3;1H\n"))
	if got := v.ScreenText(); got != "1\n3\n\n4" {
		t.Errorf("region scroll =\n%q", got)
	}
	v.Resize(3, 2)
	if v.Cols != 3 || v.Rows != 2 || len(v.Lines) != 2 {
		t.Errorf("resize: %d×%d %d lines", v.Cols, v.Rows, len(v.Lines))
	}
}
