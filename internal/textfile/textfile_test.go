package textfile

import (
	"testing"

	"golang.org/x/text/encoding/japanese"
)

func TestDecodeContent(t *testing.T) {
	sjis, err := japanese.ShiftJIS.NewEncoder().Bytes([]byte("こんにちは\r\n世界\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		data     []byte
		text     string
		encoding string
		crlf     bool
		binary   bool
	}{
		{"utf8", []byte("package main\n"), "package main\n", UTF8, false, false},
		{"bom", []byte("\xEF\xBB\xBFhello\r\n"), "hello\r\n", UTF8BOM, true, false},
		{"sjis", sjis, "こんにちは\r\n世界\r\n", SJIS, true, false},
		{"binary", []byte("PNG\x00\x01\x02"), "", "", false, true},
	}
	for _, c := range cases {
		var got Content
		Decode(c.data, &got)
		if got.Text != c.text || got.Encoding != c.encoding || got.CRLF != c.crlf || got.Binary != c.binary {
			t.Errorf("%s: got %+v", c.name, got)
		}
	}
}
