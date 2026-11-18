package syntax

import (
	"path/filepath"
	"testing"
)

// kindsOf returns the Kind of each byte of line i, by the spans of h.
func kindsAt(h *Highlight, line, col int) Kind {
	for _, sp := range h.Lines[line] {
		if int(sp.Start) <= col && col < int(sp.End) {
			return sp.Kind
		}
	}
	return KindNone
}

func TestLex_MultilineCommentAndString(t *testing.T) {
	src := "package main\n\n/* a comment\n   that runs on */\nvar s = `raw\nstring`\nfunc f() int { return 42 }\n"
	h := Lex(LexerOf("x.go", src), src)
	if h == nil {
		t.Fatal("no highlight")
	}
	cases := []struct {
		line, col int
		want      Kind
	}{
		{0, 0, KindKeyword},  // package
		{2, 3, KindComment},  // a comment
		{3, 5, KindComment},  // that runs on: inside the comment begun a line above
		{4, 9, KindString},   // `raw
		{5, 0, KindString},   // string`: the raw string goes on
		{6, 0, KindKeyword},  // func
		{6, 5, KindFunction}, // f
		{6, 9, KindType},     // int
		{6, 22, KindNumber},  // 42
	}
	for _, c := range cases {
		if got := kindsAt(h, c.line, c.col); got != c.want {
			t.Errorf("line %d col %d: Kind %d, want %d (%v)", c.line, c.col, got, c.want, h.Lines[c.line])
		}
	}
}

func TestLex_CRLF(t *testing.T) {
	src := "// one\r\nx := 1\r\n"
	h := Lex(LexerOf("a.go", src), src)
	if len(h.Lines) != 3 || kindsAt(h, 1, 5) != KindNumber {
		t.Errorf("lines = %v", h.Lines)
	}
}

func TestLexerOf(t *testing.T) {
	for _, name := range []string{"a.go", "b.py", "Makefile", "c.ts", "d.rs", "e.json", "Dockerfile"} {
		if LexerOf(filepath.Join("p", name), "") == nil {
			t.Errorf("no lexer for %s", name)
		}
	}
}

func TestCode_SpellsCode(t *testing.T) {
	code := "func main() {\n\tfmt.Println(\"hi\") // greet\n}\n"
	toks := Code("go", code)
	got := ""
	kinds := 0
	for _, tok := range toks {
		got += tok.Text
		if tok.Kind != KindNone {
			kinds++
		}
	}
	if got != code || kinds == 0 {
		t.Errorf("tokens spell %q with %d kinds", got, kinds)
	}
	if Code("nosuchlanguage", code) != nil || Code("", code) != nil {
		t.Error("an unknown language has no tokens")
	}
}
