package fileicon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestShellIcon checks that the .ico holds a PNG of every size, and that the
// glyph shows on the page: an icon of a known kind differs from the page
// alone. With NOPLE_SHOTS=1 it writes a sheet of icons to docs/shots.
func TestShellIcon(t *testing.T) {
	exts := []string{".go", ".md", ".json", ".ts", ".py", ".rs", ".yaml", ".txt", ".sh", ".css", ".html", ".gitignore", ".unknown"}
	plain := shellPicture(48, nil, shellPaper)
	for _, ext := range exts {
		data := ShellIcon(ext)
		var head [3]uint16
		binary.Read(bytes.NewReader(data), binary.LittleEndian, &head)
		if head[1] != 1 || int(head[2]) != len(shellSizes) {
			t.Fatalf("%s: header %v", ext, head)
		}
		glyph, c := shellGlyph(ext)
		got := shellPicture(48, glyph, c)
		known := ext != ".unknown"
		if same := bytes.Equal(got.Pix, plain.Pix); same == known {
			t.Errorf("%s: icon is the page alone: %v, want %v", ext, same, !known)
		}
	}
	if os.Getenv("NOPLE_SHOTS") == "" {
		return
	}
	sizes := []int{256, 48, 32, 16}
	w := 0
	for _, n := range sizes {
		w += n + 8
	}
	sheet := image.NewRGBA(image.Rect(0, 0, w, len(exts)*264))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(image.White.C), image.Point{}, draw.Src)
	for i, ext := range exts {
		glyph, c := shellGlyph(ext)
		x := 0
		for _, n := range sizes {
			pic := shellPicture(n, glyph, c)
			draw.Draw(sheet, image.Rect(x, i*264, x+n, i*264+n), pic, image.Point{}, draw.Over)
			x += n + 8
		}
	}
	dir := filepath.Join("..", "..", "docs", "shots")
	os.MkdirAll(dir, 0o755)
	f, err := os.Create(filepath.Join(dir, "shell-icons.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, sheet)
}

// TestMacIcon checks that the .icns says its length and holds a PNG of each
// of its types, at the size of the type.
func TestMacIcon(t *testing.T) {
	data := MacIcon(".go")
	if string(data[:4]) != "icns" || int(binary.BigEndian.Uint32(data[4:8])) != len(data) {
		t.Fatalf("header %q, length %d of %d", data[:4], binary.BigEndian.Uint32(data[4:8]), len(data))
	}
	rest := data[8:]
	for _, want := range macIconTypes {
		if len(rest) < 8 {
			t.Fatalf("no %s", want.typ)
		}
		n := int(binary.BigEndian.Uint32(rest[4:8]))
		if string(rest[:4]) != want.typ || n > len(rest) {
			t.Fatalf("chunk %q of %d, want %s", rest[:4], n, want.typ)
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(rest[8:n]))
		if err != nil || cfg.Width != want.n || cfg.Height != want.n {
			t.Errorf("%s: %dx%d, %v; want %d", want.typ, cfg.Width, cfg.Height, err, want.n)
		}
		rest = rest[n:]
	}
	if len(rest) != 0 {
		t.Errorf("%d bytes after the last chunk", len(rest))
	}
}
