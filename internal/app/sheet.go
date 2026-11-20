package app

import (
	"cmp"
	"encoding/csv"
	"errors"
	"io"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// sheet is a CSV or TSV file read as a table: the first record is the
// header, the rest are the rows. Every row has as many cells as the widest
// record, so a short record shows empty cells at its end.
type sheet struct {
	header []string
	// rows are the cells as they show: a line break inside a field shows
	// as a mark, since a row is one line high.
	rows [][]string
	// lines are where each row starts in the source, from 0.
	lines []int
	// spans are where the fields of each row are in the text it was read
	// from, as byte offsets, quotes included; ends are where the last field
	// of each row ends. A row shorter than the header has fewer spans.
	spans [][]fieldSpan
	ends  []int
	sep   rune
	// numeric marks the columns whose cells are all numbers, or empty:
	// they show at the end of the cell and sort by value.
	numeric []bool
	// values are the numbers of the numeric columns, for sorting.
	values [][]float64
	// widths are the widths of the columns as the font of widthsFor
	// measured them, the column of row numbers first.
	widths    []float32
	widthsFor any
}

// sheetSeparator returns the separator of the fields of the file at path,
// or 0 if it is no table.
func sheetSeparator(path string) rune {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv":
		return ','
	case ".tsv", ".tab":
		return '\t'
	}
	return 0
}

// fieldSpan is where a field is in the text: from start up to end. quoted
// says it is written in quotes.
type fieldSpan struct {
	start, end int
	quoted     bool
}

// lineBreakMark stands for a line break inside a field.
const lineBreakMark = " ↵ "

// parseSheet reads src, decoded text with its BOM gone, as fields separated
// by sep. Quotes that are not quite right are taken as they are, as
// spreadsheets read them: a bare quote stays in its field, and a quoted
// field that never ends runs to the end of the file. The error is that of
// reading, which a string does not fail.
func parseSheet(src string, sep rune) (*sheet, error) {
	r := csv.NewReader(strings.NewReader(src))
	r.Comma = sep
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	s := &sheet{sep: sep}
	// starts are where the lines of src start, to turn the places of
	// FieldPos into offsets.
	starts := []int{0}
	for i := range len(src) {
		if src[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	at := func(line, col int) int { return starts[line-1] + col - 1 }
	var recs [][]string
	width := 0
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		line, _ := r.FieldPos(0)
		spans := make([]fieldSpan, len(rec))
		for i := range rec {
			spans[i].start = at(r.FieldPos(i))
			spans[i].quoted = spans[i].start < len(src) && src[spans[i].start] == '"'
			if i > 0 {
				spans[i-1].end = spans[i].start - utf8.RuneLen(sep)
			}
		}
		end := int(r.InputOffset())
		if end > 0 && src[end-1] == '\n' {
			end--
			if end > 0 && src[end-1] == '\r' {
				end--
			}
		}
		spans[len(rec)-1].end = end
		s.spans = append(s.spans, spans)
		s.ends = append(s.ends, end)
		for i, f := range rec {
			if strings.ContainsAny(f, "\r\n") {
				f = strings.ReplaceAll(f, "\r\n", "\n")
				rec[i] = strings.ReplaceAll(f, "\n", lineBreakMark)
			}
		}
		recs = append(recs, rec)
		s.lines = append(s.lines, line-1)
		width = max(width, len(rec))
	}
	for i, rec := range recs {
		if len(rec) < width {
			recs[i] = append(rec, make([]string, width-len(rec))...)
		}
	}
	if len(recs) == 0 {
		return s, nil
	}
	s.header, s.rows, s.lines = recs[0], recs[1:], s.lines[1:]
	s.spans, s.ends = s.spans[1:], s.ends[1:]
	s.numeric = make([]bool, width)
	s.values = make([][]float64, width)
	for j := range width {
		s.values[j], s.numeric[j] = columnValues(s.rows, j)
	}
	return s, nil
}

// columnValues returns the numbers of column j, if every cell of it is a
// number or empty and one at least is a number. An empty cell is NaN.
func columnValues(rows [][]string, j int) ([]float64, bool) {
	vals := make([]float64, len(rows))
	found := false
	for i, row := range rows {
		c := strings.TrimSpace(row[j])
		if c == "" {
			vals[i] = math.NaN() // empty
			continue
		}
		v, ok := parseNumber(c)
		if !ok {
			return nil, false
		}
		vals[i], found = v, true
	}
	if !found {
		return nil, false
	}
	return vals, true
}

// parseNumber reads a number as a table writes it: a sign, a mark of money
// before it or a percent after it, and commas between thousands.
func parseNumber(s string) (float64, bool) {
	s = strings.TrimSuffix(s, "%")
	for _, m := range []string{"¥", "￥", "$", "€", "£"} {
		if rest, ok := strings.CutPrefix(s, m); ok {
			s = rest
			break
		}
		if rest, ok := strings.CutPrefix(s, "-"+m); ok {
			s = "-" + rest
			break
		}
	}
	if s == "" {
		return 0, false
	}
	if strings.Contains(s, ",") {
		if !thousands(s) {
			return 0, false
		}
		s = strings.ReplaceAll(s, ",", "")
	}
	switch strings.ToLower(strings.TrimLeft(s, "+-")) {
	case "inf", "infinity", "nan":
		// Words are text, though ParseFloat reads them.
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || strings.ContainsAny(s, "xXpP_") {
		// ParseFloat also reads hexadecimal and underscores.
		return 0, false
	}
	return v, true
}

// thousands reports whether the commas of s fall between groups of three
// digits, as 1,234,567.8 has them.
func thousands(s string) bool {
	s = strings.TrimLeft(s, "+-")
	whole, _, _ := strings.Cut(s, ".")
	groups := strings.Split(whole, ",")
	if groups[0] == "" || len(groups[0]) > 3 {
		return false
	}
	for _, g := range groups[1:] {
		if len(g) != 3 {
			return false
		}
	}
	return true
}

// order returns the rows of s, as indexes, sorted by column j (of the
// columns of the file) up or down. Empty cells come last either way; rows
// that tie keep their order.
func (s *sheet) order(j int, desc bool) []int {
	idx := make([]int, len(s.rows))
	for i := range idx {
		idx[i] = i
	}
	if j < 0 || j >= len(s.header) {
		if desc {
			slices.Reverse(idx)
		}
		return idx
	}
	if s.numeric[j] {
		vals := s.values[j]
		slices.SortStableFunc(idx, func(a, b int) int {
			va, vb := vals[a], vals[b]
			switch ea, eb := math.IsNaN(va), math.IsNaN(vb); {
			case ea || eb:
				return cmpBool(ea, eb)
			case desc:
				return cmp.Compare(vb, va)
			}
			return cmp.Compare(va, vb)
		})
		return idx
	}
	slices.SortStableFunc(idx, func(a, b int) int {
		ca, cb := s.rows[a][j], s.rows[b][j]
		switch ea, eb := ca == "", cb == ""; {
		case ea || eb:
			return cmpBool(ea, eb)
		case desc:
			return strings.Compare(cb, ca)
		}
		return strings.Compare(ca, cb)
	})
	return idx
}

// cmpBool orders false before true.
func cmpBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// columnName is the name of column j when its header is empty: A, B, …, Z,
// AA, as a spreadsheet names them.
func columnName(j int) string {
	var b []byte
	for j++; j > 0; j = (j - 1) / 26 {
		b = append([]byte{byte('A' + (j-1)%26)}, b...)
	}
	return string(b)
}

// fieldEdit returns how to put value in place of the field of row idx and
// column j in the text the sheet was read from: the bytes from start up to
// end become text. Only that field changes, in quotes if it was in quotes
// or has to be, so that a file that quotes every field still does; a field
// past the end of a short row comes after separators that reach it, quoted
// as the last field of the row is. A line break mark in value is a line
// break.
func (s *sheet) fieldEdit(idx, j int, value string) (start, end int, text string) {
	value = strings.ReplaceAll(value, lineBreakMark, "\n")
	spans := s.spans[idx]
	if j < len(spans) {
		return spans[j].start, spans[j].end, quoteField(value, s.sep, spans[j].quoted)
	}
	end = s.ends[idx]
	quoted := len(spans) > 0 && spans[len(spans)-1].quoted
	return end, end, strings.Repeat(string(s.sep), j-len(spans)+1) + quoteField(value, s.sep, quoted)
}

// quoteField writes value as a field separated by sep, in quotes when
// always says so, or when it holds the separator, a quote or a line break,
// or starts with a space, as encoding/csv writes it.
func quoteField(value string, sep rune, always bool) string {
	if !always && !strings.ContainsRune(value, sep) && !strings.ContainsAny(value, "\"\r\n") && !strings.HasPrefix(value, " ") {
		return value
	}
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
