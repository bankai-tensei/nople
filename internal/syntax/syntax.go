// Package syntax colors code: it lexes a text with chroma and sorts its
// tokens into the few kinds that have colors, kept line by line for an
// editor that asks for a line at a time.
package syntax

import (
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// MaxSize is the largest text that gets colors: chroma lexes the
// whole of it, which for many megabytes takes seconds.
const MaxSize = 4 << 20

// Kind is what a stretch of code is, which decides its color.
type Kind uint8

const (
	KindNone Kind = iota
	KindComment
	KindPreproc
	KindKeyword
	KindType
	KindString
	KindEscape
	KindNumber
	KindFunction
	KindTag
	KindAttribute
	KindBuiltin
	KindHeading
	KindInserted
	KindDeleted
	KindEmph
	KindStrong
	KindCount
)

// Span is one colored stretch of a line, in bytes.
type Span struct {
	Start, End int32
	Kind       Kind
}

// Highlight is the colors of a text: the spans of each line. chroma can
// start lexing only from the top of a text, not from the state of a line, so
// the whole text is lexed at once, off the UI goroutine, and kept by line;
// the editor then asks for a line at a time.
type Highlight struct {
	Lines [][]Span
}

// LexerOf finds the lexer of the file at path, from its name and else from
// its text; nil if there is none.
func LexerOf(path, text string) chroma.Lexer {
	l := lexers.Match(filepath.Base(path))
	if l == nil {
		l = lexers.Analyse(text)
	}
	if l == nil {
		return nil
	}
	return chroma.Coalesce(l)
}

// Lex colors text with l. A text the lexer cannot read gets no colors.
func Lex(l chroma.Lexer, text string) *Highlight {
	it, err := l.Tokenise(nil, text)
	if err != nil {
		return nil
	}
	h := &Highlight{Lines: make([][]Span, 1, strings.Count(text, "\n")+1)}
	line, col := 0, 0
	for tok := it(); tok != chroma.EOF; tok = it() {
		k := KindOf(tok.Type)
		v := tok.Value
		for {
			nl := strings.IndexByte(v, '\n')
			part := v
			if nl >= 0 {
				part = v[:nl]
			}
			if k != KindNone && part != "" {
				// A \r before the line break is no part of the line the
				// editor shows, and coloring it is harmless.
				h.Lines[line] = append(h.Lines[line], Span{int32(col), int32(col + len(part)), k})
			}
			col += len(part)
			if nl < 0 {
				break
			}
			line++
			col = 0
			h.Lines = append(h.Lines, nil)
			v = v[nl+1:]
		}
	}
	return h
}

// KindOf sorts a token of chroma into the few kinds that have colors.
func KindOf(t chroma.TokenType) Kind {
	switch {
	case t == chroma.CommentPreproc || t == chroma.CommentPreprocFile || t == chroma.NameDecorator:
		return KindPreproc
	case t.InCategory(chroma.Comment):
		return KindComment
	case t == chroma.KeywordType:
		return KindType
	case t == chroma.KeywordConstant:
		return KindNumber
	case t.InCategory(chroma.Keyword), t == chroma.OperatorWord:
		return KindKeyword
	case t == chroma.LiteralStringEscape || t == chroma.LiteralStringInterpol:
		return KindEscape
	case t.InSubCategory(chroma.LiteralString):
		return KindString
	case t.InSubCategory(chroma.LiteralNumber), t == chroma.NameConstant:
		return KindNumber
	case t == chroma.NameFunction || t == chroma.NameFunctionMagic:
		return KindFunction
	case t == chroma.NameClass:
		return KindType
	case t == chroma.NameTag:
		return KindTag
	case t == chroma.NameAttribute:
		return KindAttribute
	case t == chroma.NameBuiltin || t == chroma.NameBuiltinPseudo:
		return KindBuiltin
	case t == chroma.GenericHeading || t == chroma.GenericSubheading:
		return KindHeading
	case t == chroma.GenericInserted:
		return KindInserted
	case t == chroma.GenericDeleted:
		return KindDeleted
	case t == chroma.GenericEmph:
		return KindEmph
	case t == chroma.GenericStrong:
		return KindStrong
	}
	return KindNone
}

// Token is a stretch of code and its kind.
type Token struct {
	Text string
	Kind Kind
}

// Code lexes code in the language lang, a name chroma knows such as "go",
// into tokens that spell code exactly; nil if lang is unknown or code too
// large. A lexer may add a line break at the end, which is cut.
func Code(lang, code string) []Token {
	if lang == "" || len(code) > MaxSize {
		return nil
	}
	l := lexers.Get(lang)
	if l == nil {
		return nil
	}
	it, err := chroma.Coalesce(l).Tokenise(nil, code)
	if err != nil {
		return nil
	}
	var toks []Token
	rest := code
	for tok := it(); tok != chroma.EOF && rest != ""; tok = it() {
		v := tok.Value
		if len(v) > len(rest) {
			v = rest
		}
		if !strings.HasPrefix(rest, v) {
			return nil
		}
		rest = rest[len(v):]
		if v != "" {
			toks = append(toks, Token{v, KindOf(tok.Type)})
		}
	}
	if rest != "" {
		toks = append(toks, Token{Text: rest})
	}
	return toks
}
