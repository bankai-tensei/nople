package app

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/bankai-tensei/nople/internal/textfile"

	"github.com/bankai-tensei/gowaku/input"
	"github.com/bankai-tensei/gowaku/widgets/collection"
	"golang.org/x/text/encoding/japanese"
)

// sales is a CSV file with what makes CSV hard: quoted fields with commas,
// quotes and line breaks in them, a short record, and columns of numbers.
const sales = "商品,地域,数量,売上,メモ\r\n" +
	"りんご,東京,12,\"1,200\",\"甘い, 赤い\"\r\n" +
	"みかん,大阪,8,640,\"「\"\"小粒\"\"」\"\r\n" +
	"ぶどう,名古屋,30,\"4,500\",\"二行目へ\r\n続く\"\r\n" +
	"メロン,福岡,1,3000\r\n" +
	"もも,札幌,,980,\r\n"

func TestParseSheet_Fields(t *testing.T) {
	s, err := parseSheet(sales, ',')
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"商品", "地域", "数量", "売上", "メモ"}; !slices.Equal(s.header, want) {
		t.Errorf("header = %q", s.header)
	}
	if len(s.rows) != 5 {
		t.Fatalf("%d rows, want 5", len(s.rows))
	}
	for i, want := range [][]string{
		{"りんご", "東京", "12", "1,200", "甘い, 赤い"},
		{"みかん", "大阪", "8", "640", "「\"小粒\"」"},
		{"ぶどう", "名古屋", "30", "4,500", "二行目へ" + lineBreakMark + "続く"},
		{"メロン", "福岡", "1", "3000", ""},
		{"もも", "札幌", "", "980", ""},
	} {
		if !slices.Equal(s.rows[i], want) {
			t.Errorf("row %d = %q, want %q", i, s.rows[i], want)
		}
	}
	// The row after the field of two lines starts a line further down.
	if want := []int{1, 2, 3, 5, 6}; !slices.Equal(s.lines, want) {
		t.Errorf("lines = %v, want %v", s.lines, want)
	}
	if want := []bool{false, false, true, true, false}; !slices.Equal(s.numeric, want) {
		t.Errorf("numeric = %v, want %v", s.numeric, want)
	}
}

func TestParseSheet_Edges(t *testing.T) {
	s, err := parseSheet("", ',')
	if err != nil || len(s.header) != 0 || len(s.rows) != 0 {
		t.Errorf("an empty file: %v, %+v", err, s)
	}
	s, err = parseSheet("a\tb\n1\t\"x\ty\"\n", '\t')
	if err != nil || !slices.Equal(s.rows[0], []string{"1", "x\ty"}) {
		t.Errorf("tsv: %v, %q", err, s.rows)
	}
	// A quote inside a field that is not quoted stays as it is.
	s, err = parseSheet("a,b\n5\" disk,ok\n", ',')
	if err != nil || s.rows[0][0] != "5\" disk" {
		t.Errorf("a bare quote: %v, %q", err, s.rows)
	}
	// A quoted field that never ends runs to the end.
	s, err = parseSheet("a,b\n\"never ends,1\n", ',')
	if err != nil || len(s.rows) != 1 || s.rows[0][0] != "never ends,1"+lineBreakMark {
		t.Errorf("an open quote: %v, %q", err, s.rows)
	}
}

func TestParseNumber(t *testing.T) {
	for s, want := range map[string]float64{
		"12": 12, "-3.5": -3.5, "+7": 7, "1,234,567.5": 1234567.5, "¥1,200": 1200,
		"$-": 0, "-$4": -4, "50%": 50, "1e3": 1000, ".5": 0.5,
	} {
		v, ok := parseNumber(s)
		if s == "$-" {
			if ok {
				t.Errorf("%q should not be a number", s)
			}
			continue
		}
		if !ok || v != want {
			t.Errorf("parseNumber(%q) = %v, %v; want %v", s, v, ok, want)
		}
	}
	for _, s := range []string{"abc", "1,2", "12,34", "0x1F", "1_000", "NaN", "inf", "2026-11-18", "%"} {
		if _, ok := parseNumber(s); ok {
			t.Errorf("%q should not be a number", s)
		}
	}
}

func TestSheet_Order(t *testing.T) {
	s, _ := parseSheet(sales, ',')
	// 数量 sorts by value, and the empty cell comes last both ways.
	if got := s.order(2, false); !slices.Equal(got, []int{3, 1, 0, 2, 4}) {
		t.Errorf("up by 数量 = %v", got)
	}
	if got := s.order(2, true); !slices.Equal(got, []int{2, 0, 1, 3, 4}) {
		t.Errorf("down by 数量 = %v", got)
	}
	// 売上 holds 1,200 and 3000: by value, not as text.
	if got := s.order(3, false); !slices.Equal(got, []int{1, 4, 0, 3, 2}) {
		t.Errorf("up by 売上 = %v", got)
	}
	if got := s.order(-1, true); !slices.Equal(got, []int{4, 3, 2, 1, 0}) {
		t.Errorf("down by row number = %v", got)
	}
}

func TestColumnName(t *testing.T) {
	for j, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 27: "AB", 701: "ZZ", 702: "AAA"} {
		if got := columnName(j); got != want {
			t.Errorf("columnName(%d) = %q, want %q", j, got, want)
		}
	}
}

// TestSheet_Encodings reads tables with a BOM and in Shift_JIS, as Excel
// writes them, through textfile.
func TestSheet_Encodings(t *testing.T) {
	dir := t.TempDir()
	bom := filepath.Join(dir, "bom.csv")
	os.WriteFile(bom, append([]byte{0xEF, 0xBB, 0xBF}, sales...), 0o644)
	sjisText, err := japanese.ShiftJIS.NewEncoder().String(sales)
	if err != nil {
		t.Fatal(err)
	}
	sjis := filepath.Join(dir, "sjis.csv")
	os.WriteFile(sjis, []byte(sjisText), 0o644)
	for path, enc := range map[string]string{bom: textfile.UTF8BOM, sjis: textfile.SJIS} {
		d := openDocument(path)
		if d.content.Encoding != enc || d.sheet == nil {
			t.Fatalf("%s: encoding %s, sheet %v", filepath.Base(path), d.content.Encoding, d.sheet)
		}
		if d.sheet.header[0] != "商品" || d.sheet.rows[1][4] != "「\"小粒\"」" {
			t.Errorf("%s: header %q, rows %q", filepath.Base(path), d.sheet.header, d.sheet.rows)
		}
	}
}

// openSales opens a sample folder with sales.csv in it, and the file.
func openSales(t *testing.T, dark bool) (*harness, string) {
	t.Helper()
	h, dir := openSample(t, dark)
	path := filepath.Join(dir, "sales.csv")
	if err := os.WriteFile(path, []byte(sales), 0o644); err != nil {
		t.Fatal(err)
	}
	h.app.openFile(path, false)
	h.settle()
	return h, path
}

func TestSheet_ShowsTable(t *testing.T) {
	h, _ := openSales(t, false)
	tb := h.app.currentTab()
	if !tb.showsSheet() {
		t.Fatal("sales.csv should show as a table")
	}
	for _, s := range []string{"商品", "売上", "りんご", "1,200", "甘い, 赤い", "「\"小粒\"」", "表プレビュー", "5 行 × 5 列"} {
		if !h.has(s) {
			t.Errorf("%q should show:\n%s", s, h.tt.Dump())
		}
	}
	if h.has("\"甘い, 赤い\"") {
		t.Error("the quotes of a field should not show")
	}
	h.shot("csv-table")

	// A click on a header sorts; the next one sorts the other way.
	h.app.sortSheet(tb, collection.TableSort{Column: 4, Order: collection.SortDescending})
	h.settle()
	if got := tb.sheetOrder(); !slices.Equal(got, []int{2, 3, 0, 4, 1}) {
		t.Errorf("down by 売上 = %v", got)
	}

	// Ctrl+Shift+V shows the source, in that tab only.
	h.app.toggleSource()
	h.settle()
	if tb.showsSheet() || h.has("表プレビュー") {
		t.Errorf("the source should show:\n%s", h.tt.Dump())
	}
	h.shot("csv-source")
	h.app.toggleSource()
	h.settle()
	if !tb.showsSheet() {
		t.Error("Ctrl+Shift+V again should show the table")
	}

	// Enter or a double click on a row number shows the row in the source:
	// the first row sorted, ぶどう, is on line 4.
	tb.table.SetFocusedCell(0, 0)
	h.app.activateSheet(tb.id, 0)
	h.settle()
	if !tb.source {
		t.Fatal("the row should show in the source")
	}
	if got := tb.view.Selection().Start.Line; got != 3 {
		t.Errorf("the caret is on line %d, want 3", got)
	}
}

func TestSheet_FollowsEdits(t *testing.T) {
	h, _ := openSales(t, false)
	tb := h.app.currentTab()
	d := tb.doc
	h.app.toggleSource()
	h.settle()
	d.ctrl.SetText(strings.Replace(d.ctrl.Text(), "りんご", "林檎", 1))
	h.app.refill(d)
	h.app.toggleSource()
	h.settle()
	if !h.has("林檎") {
		t.Errorf("the table should follow the text:\n%s", h.tt.Dump())
	}
}

func TestSheet_DarkAndWide(t *testing.T) {
	h, dir := openSample(t, true)
	var b strings.Builder
	cols := []string{"id", "name", "email", "city", "country", "score", "rank", "joined", "plan", "notes", "lat", "lng"}
	b.WriteString(strings.Join(cols, "\t") + "\n")
	for i := range 200 {
		b.WriteString(strings.Join([]string{
			strconv.Itoa(i + 1), "ユーザー " + strconv.Itoa(i), "user" + strconv.Itoa(i) + "@example.com", "横浜", "Japan",
			strconv.Itoa(i * 37 % 1000), strconv.Itoa(i%10 + 1), "2026-11-18", "Pro", "とても長いメモがここに入ります。列の幅には上限があります。",
			"35.4437", "139.6380",
		}, "\t") + "\n")
	}
	path := filepath.Join(dir, "users.tsv")
	os.WriteFile(path, []byte(b.String()), 0o644)
	h.app.openFile(path, false)
	h.settle()
	tb := h.app.currentTab()
	if !tb.showsSheet() || !h.has("user0@example.com") || !h.has("200 行 × 12 列") {
		t.Errorf("users.tsv should show as a table:\n%s", h.tt.Dump())
	}
	s := tb.doc.sheet
	if !s.numeric[0] || !s.numeric[10] || s.numeric[7] || s.numeric[1] {
		t.Errorf("numeric = %v", s.numeric)
	}
	// The row numbers come first.
	w := s.widths
	if w[11] >= w[3] || w[10] != sheetMaxWidth {
		t.Errorf("widths should follow the cells, within bounds: %v", w)
	}
	h.shot("csv-table-dark")
}

// TestSheet_EditInPlace edits cells of the table in place: a double click
// opens the cell, Enter writes it into its field of the source alone, quoted
// as it has to be, and Escape drops the edit.
func TestSheet_EditInPlace(t *testing.T) {
	h, _ := openSales(t, false)
	tb := h.app.currentTab()
	d := tb.doc
	before := d.ctrl.Text()

	h.tt.DoubleTap("りんご")
	h.settle()
	if tb.edit == nil || !tb.showsSheet() {
		t.Fatalf("a double click should edit the cell in the table, edit = %v, source = %v", tb.edit, tb.source)
	}
	h.shot("csv-edit")
	for range 3 {
		h.tt.Key(input.KeyBackspace, 0)
	}
	h.tt.Type("紅玉, 青森")
	h.tt.Key(input.KeyEnter, 0)
	h.settle()
	if tb.edit != nil || !tb.showsSheet() || !d.dirty {
		t.Fatalf("Enter should keep the edit in the table, edit = %v, dirty = %v", tb.edit, d.dirty)
	}
	want := strings.Replace(before, "りんご,", "\"紅玉, 青森\",", 1)
	if got := d.ctrl.Text(); got != want {
		t.Errorf("only the field should change:\n got %q\nwant %q", got, want)
	}
	if !h.has("紅玉, 青森") {
		t.Errorf("the table should show the new text:\n%s", h.tt.Dump())
	}
	h.shot("csv-edited")

	// The keyboard stays in the table: Down goes to the next row, and Enter
	// edits its cell.
	h.tt.Key(input.KeyDown, 0)
	h.settle()
	if row, col := tb.table.FocusedCell(); row != 1 || col != 1 {
		t.Errorf("Down should go to row 1, column 1, at %d, %d", row, col)
	}
	h.tt.Key(input.KeyEnter, 0)
	h.settle()
	if e := tb.edit; e == nil || e.idx != 1 || e.col != 0 {
		t.Fatalf("Enter should edit みかん, edit = %+v", e)
	}
	h.tt.Key(input.KeyEscape, 0)
	h.settle()

	// Escape drops what was typed.
	h.tt.DoubleTap("東京")
	h.settle()
	h.tt.Type("都")
	h.tt.Key(input.KeyEscape, 0)
	h.settle()
	if tb.edit != nil || strings.Contains(d.ctrl.Text(), "東京都") {
		t.Errorf("Escape should drop the edit, edit = %v", tb.edit)
	}

	// Undo in the source takes the edit back.
	d.ctrl.Undo()
	if d.ctrl.Text() != before {
		t.Errorf("undo should bring the text back, got %q", d.ctrl.Text())
	}
}

func TestSheet_FieldEdit(t *testing.T) {
	src := "a,b,c\n1,\"x\ny\",3\nshort\n"
	s, err := parseSheet(src, ',')
	if err != nil {
		t.Fatal(err)
	}
	apply := func(idx, j int, v string) string {
		start, end, text := s.fieldEdit(idx, j, v)
		return src[:start] + text + src[end:]
	}
	for _, c := range []struct {
		idx, j  int
		v, want string
	}{
		{0, 0, "9", "a,b,c\n9,\"x\ny\",3\nshort\n"},
		{0, 1, "z", "a,b,c\n1,\"z\",3\nshort\n"},
		{0, 0, "a,b", "a,b,c\n\"a,b\",\"x\ny\",3\nshort\n"},
		{0, 1, "x" + lineBreakMark + "w", "a,b,c\n1,\"x\nw\",3\nshort\n"},
		{0, 2, "say \"hi\"", "a,b,c\n1,\"x\ny\",\"say \"\"hi\"\"\"\nshort\n"},
		{1, 0, "long", "a,b,c\n1,\"x\ny\",3\nlong\n"},
		{1, 2, "end", "a,b,c\n1,\"x\ny\",3\nshort,,end\n"},
	} {
		if got := apply(c.idx, c.j, c.v); got != c.want {
			t.Errorf("edit (%d, %d) to %q:\n got %q\nwant %q", c.idx, c.j, c.v, got, c.want)
		}
	}
	// A file that quotes every field keeps quoting the fields edited, also
	// one added at the end of a short row.
	quoted := "\"a\",\"b\",\"c\"\r\n\"1\",\"5\",\"x\"\r\n\"2\"\r\n"
	s, _ = parseSheet(quoted, ',')
	for _, c := range []struct {
		idx, j  int
		v, want string
	}{
		{0, 1, "4", "\"a\",\"b\",\"c\"\r\n\"1\",\"4\",\"x\"\r\n\"2\"\r\n"},
		{0, 2, "", "\"a\",\"b\",\"c\"\r\n\"1\",\"5\",\"\"\r\n\"2\"\r\n"},
		{1, 2, "z", "\"a\",\"b\",\"c\"\r\n\"1\",\"5\",\"x\"\r\n\"2\",,\"z\"\r\n"},
	} {
		start, end, text := s.fieldEdit(c.idx, c.j, c.v)
		if got := quoted[:start] + text + quoted[end:]; got != c.want {
			t.Errorf("quoted edit (%d, %d) to %q:\n got %q\nwant %q", c.idx, c.j, c.v, got, c.want)
		}
	}

	// A last row without a line break, and CRLF.
	s, _ = parseSheet("a\tb\r\n1\t2", '\t')
	if start, end, text := s.fieldEdit(0, 1, "x y"); start != 7 || end != 8 || text != "x y" {
		t.Errorf("tsv edit = %d, %d, %q", start, end, text)
	}
}
