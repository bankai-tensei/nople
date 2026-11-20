// Package textfile reads a file as text and writes it back as it was: it
// tells text from binary, finds the encoding (UTF-8, with or without a BOM,
// or Shift_JIS) and the line breaks, and encodes the text again in them.
package textfile

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
)

// Limits on what opens as text.
const (
	// MaxOpenSize is the largest file that opens at all.
	MaxOpenSize = 64 << 20
	// LargeSize is the size above which a file opens with a warning.
	LargeSize = 8 << 20
	// sniffSize is how much of a file decides whether it is binary.
	sniffSize = 8000
)

// Encodings of a text file.
const (
	UTF8    = "UTF-8"
	UTF8BOM = "UTF-8 BOM"
	SJIS    = "Shift_JIS"
)

// Content is what reading a file found.
type Content struct {
	// Text is the text, decoded from Encoding, with its line breaks; CRLF
	// means they are CR LF.
	Text     string
	Encoding string
	CRLF     bool
	// Binary means the file holds no text to show.
	Binary bool
	// Forced means a binary file shows as text because the user asked.
	Forced bool
	// Lossy means bytes that are no text in UTF-8 or Shift_JIS were
	// replaced, so that the text cannot be written back as it was.
	Lossy bool
	// Large means the file is big enough to be slow.
	Large bool
	Size  int64
	Err   error
}

// ErrTooLarge means a file is beyond MaxOpenSize.
var ErrTooLarge = errors.New("too large")

// Read reads the file at path as text. A binary file reads as
// nothing unless force.
func Read(path string, force bool) Content {
	f, err := os.Open(path)
	if err != nil {
		return Content{Err: err}
	}
	defer f.Close() //nolint:errcheck // only read
	st, err := f.Stat()
	if err != nil {
		return Content{Err: err}
	}
	c := Content{Size: st.Size()}
	if st.IsDir() {
		c.Err = fmt.Errorf("%s: is a folder", path)
		return c
	}
	if st.Size() > MaxOpenSize {
		c.Err = ErrTooLarge
		return c
	}
	data, err := io.ReadAll(f)
	if err != nil {
		c.Err = err
		return c
	}
	c.Large = len(data) > LargeSize
	if force {
		c.Forced = true
		// NUL shows as its picture, which a reader can see.
		DecodeText(bytes.ReplaceAll(data, []byte{0}, []byte("␀")), &c)
		return c
	}
	Decode(data, &c)
	return c
}

// Decode finds the encoding and line breaks of data, and sets the text
// of c.
func Decode(data []byte, c *Content) {
	if IsBinary(data) {
		c.Binary = true
		return
	}
	DecodeText(data, c)
}

// DecodeText decodes data as text, whatever it holds.
func DecodeText(data []byte, c *Content) {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		data = data[3:]
		c.Encoding = UTF8BOM
	case utf8.Valid(data):
		c.Encoding = UTF8
	default:
		if dec, err := japanese.ShiftJIS.NewDecoder().Bytes(data); err == nil && utf8.Valid(dec) {
			data = dec
			c.Encoding = SJIS
			// The decoder puts U+FFFD for bytes that are no Shift_JIS.
			c.Lossy = bytes.ContainsRune(dec, utf8.RuneError)
		} else {
			// Neither: show it as UTF-8, with the bad bytes replaced.
			c.Encoding = UTF8
			c.Lossy = true
			data = bytes.ToValidUTF8(data, []byte("�"))
		}
	}
	c.Text = string(data)
	c.CRLF = strings.Contains(c.Text, "\r\n")
}

// IsBinary reports whether data looks like a file that is not text: it has a
// NUL byte near the start, which text in UTF-8 or Shift_JIS never has.
// UTF-16 also has them, and is not read yet.
func IsBinary(data []byte) bool {
	return bytes.IndexByte(data[:min(len(data), sniffSize)], 0) >= 0
}

// JoinLines puts the line breaks of the file into text, which has \n.
func JoinLines(text string, crlf bool) string {
	if crlf {
		return strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}

// Encode turns text into the bytes of the file in encoding.
func Encode(text, encoding string) ([]byte, error) {
	switch encoding {
	case UTF8BOM:
		return append([]byte{0xEF, 0xBB, 0xBF}, text...), nil
	case SJIS:
		b, err := japanese.ShiftJIS.NewEncoder().Bytes([]byte(text))
		if err != nil {
			return nil, ErrNotSJIS
		}
		return b, nil
	}
	return []byte(text), nil
}

// ErrNotSJIS means a text holds characters Shift_JIS has not.
var ErrNotSJIS = errors.New("textfile: not representable in Shift_JIS")
