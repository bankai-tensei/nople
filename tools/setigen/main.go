// Command setigen writes internal/fileicon/seti_icons.go: the glyphs of the icon theme Seti,
// which VS Code ships with, for the source files of programming languages
// only. Nople draws its own icons for every other file; this keeps only the
// logos of languages, as paths, and none of the font or the rest of the
// theme. Seti is under the MIT license (Jesse Weed), kept in
// assets/seti/LICENSE.txt.
//
//	go run ./tools/setigen <folder of theme-seti/icons> > internal/fileicon/seti_icons.go
//
// The folder is the icons folder of the theme-seti extension of VS Code, or
// of an editor built on it.
package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// sourceLanguages are the extensions of source files and the language VS
// Code gives them, for the languageIds of the theme.
var sourceLanguages = map[string]string{
	"go": "go", "rs": "rust", "py": "python", "pyi": "python", "pyw": "python",
	"js": "javascript", "mjs": "javascript", "cjs": "javascript", "jsx": "javascriptreact",
	"ts": "typescript", "mts": "typescript", "cts": "typescript", "tsx": "typescriptreact",
	"c": "c", "h": "c", "cpp": "cpp", "cc": "cpp", "cxx": "cpp", "hpp": "cpp", "hh": "cpp", "hxx": "cpp", "cu": "cuda-cpp",
	"cs": "csharp", "java": "java", "kt": "kotlin", "kts": "kotlin", "swift": "swift", "rb": "ruby", "php": "php",
	"lua": "lua", "dart": "dart", "html": "html", "htm": "html", "css": "css", "scss": "scss", "sass": "sass",
	"less": "less", "styl": "stylus", "sh": "shellscript", "bash": "shellscript", "zsh": "shellscript", "fish": "shellscript",
	"ps1": "powershell", "psm1": "powershell", "psd1": "powershell", "bat": "bat", "cmd": "bat",
	"r": "r", "pl": "perl", "pm": "perl", "hs": "haskell", "ex": "elixir", "exs": "elixir",
	"elm": "elm", "erb": "erb", "groovy": "groovy", "jl": "julia", "vue": "vue", "svelte": "", "ml": "ocaml", "mli": "ocaml",
	"fs": "fsharp", "fsx": "fsharp", "clj": "clojure", "cljs": "clojure", "coffee": "coffeescript",
	"m": "objective-c", "mm": "objective-cpp", "hx": "haxe", "res": "rescript", "vala": "vala", "gd": "godot",
	// Languages the theme knows by extension only.
	"scala": "", "sc": "", "zig": "", "nim": "", "nims": "", "cr": "", "d": "", "asm": "", "s": "", "wat": "",
	"graphql": "", "gql": "", "prisma": "", "sol": "", "tf": "terraform", "purs": "", "re": "", "rei": "",
	"liquid": "", "twig": "", "ejs": "", "slim": "", "tpl": "", "cfm": "", "cfc": "", "tex": "latex", "sty": "latex",
	"pug": "jade", "jade": "jade", "haml": "haml", "mustache": "mustache", "njk": "nunjucks", "jinja": "jinja", "j2": "jinja",
	"resi": "rescript", "fsi": "fsharp", "vapi": "vala", "cljc": "clojure", "litcoffee": "coffeescript", "pyx": "", "ipynb": "",
	"apex": "", "cls": "", "bsl": "", "hack": "", "ls": "", "pp": "", "pro": "", "cake": "", "eex": "", "leex": "", "heex": "",
	"dockerignore": "", "mdx": "", "astro": "", "hbs": "handlebars", "handlebars": "handlebars", "cshtml": "razor", "razor": "razor", "bicep": "bicep",
}

// sourceNames are the files of Docker, known by their name, and their language.
var sourceNames = map[string]string{
	"dockerfile": "dockerfile", "containerfile": "dockerfile",
	"docker-compose.yml": "dockercompose", "docker-compose.yaml": "dockercompose",
	"compose.yml": "dockercompose", "compose.yaml": "dockercompose",
}

type theme struct {
	IconDefinitions map[string]struct {
		FontCharacter string `json:"fontCharacter"`
		FontColor     string `json:"fontColor"`
	} `json:"iconDefinitions"`
	File           string            `json:"file"`
	FileExtensions map[string]string `json:"fileExtensions"`
	LanguageIDs    map[string]string `json:"languageIds"`
	Light          struct {
		FileExtensions map[string]string `json:"fileExtensions"`
		LanguageIDs    map[string]string `json:"languageIds"`
	} `json:"light"`
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: setigen <folder of theme-seti/icons>")
	}
	dir := os.Args[1]
	data, err := os.ReadFile(filepath.Join(dir, "vs-seti-icon-theme.json"))
	if err != nil {
		log.Fatal(err)
	}
	var t theme
	if err := json.Unmarshal(data, &t); err != nil {
		log.Fatal(err)
	}
	woff, err := os.ReadFile(filepath.Join(dir, "seti.woff"))
	if err != nil {
		log.Fatal(err)
	}
	ttf, err := woffToSFNT(woff)
	if err != nil {
		log.Fatal(err)
	}
	font, err := sfnt.Parse(ttf)
	if err != nil {
		log.Fatal(err)
	}
	var buf sfnt.Buffer

	resolve := func(ext string, exts, langs map[string]string) string {
		if id, ok := exts[ext]; ok {
			return id
		}
		if lang := sourceLanguages[ext]; lang != "" {
			return langs[lang]
		}
		return ""
	}
	type entry struct {
		ext, glyph  string
		dark, light uint64
	}
	var entries, named []entry
	paths := map[string]string{}
	// add makes the entry of key from the icons id and lightID, if its glyph
	// draws.
	add := func(list *[]entry, key, id, lightID string) {
		if id == "" || id == t.File {
			return
		}
		if lightID == "" {
			lightID = id + "_light"
		}
		def := t.IconDefinitions[id]
		lightDef, ok := t.IconDefinitions[lightID]
		if !ok {
			lightDef = def
		}
		code, err := strconv.ParseUint(strings.TrimPrefix(def.FontCharacter, "\\"), 16, 32)
		if err != nil {
			return
		}
		d, ok := paths[def.FontCharacter]
		if !ok {
			d = glyphPath(font, &buf, rune(code))
			paths[def.FontCharacter] = d
		}
		if d == "" {
			return
		}
		dark, _ := strconv.ParseUint(strings.TrimPrefix(def.FontColor, "#"), 16, 32)
		light, _ := strconv.ParseUint(strings.TrimPrefix(lightDef.FontColor, "#"), 16, 32)
		*list = append(*list, entry{key, strings.TrimPrefix(def.FontCharacter, "\\"), dark, light})
	}
	for name, lang := range sourceNames {
		add(&named, name, t.LanguageIDs[lang], t.Light.LanguageIDs[lang])
	}
	for ext := range sourceLanguages {
		id := resolve(ext, t.FileExtensions, t.LanguageIDs)
		if id == "" || id == t.File {
			continue
		}
		lightID := resolve(ext, t.Light.FileExtensions, t.Light.LanguageIDs)
		add(&entries, ext, id, lightID)
	}
	byKey := func(a, b entry) int { return strings.Compare(a.ext, b.ext) }
	slices.SortFunc(entries, byKey)
	slices.SortFunc(named, byKey)

	var out bytes.Buffer
	out.WriteString("// Code generated by tools/setigen from the icon theme Seti (MIT, Jesse Weed;\n")
	out.WriteString("// see assets/seti/LICENSE.txt). DO NOT EDIT.\n\npackage fileicon\n\n")
	out.WriteString("// setiGlyphs are the logos of languages, by the code of their glyph in the\n")
	out.WriteString("// font: SVG path data on the grid of 24.\n")
	out.WriteString("var setiGlyphs = map[string]string{\n")
	var codes []string
	for _, e := range append(slices.Clone(entries), named...) {
		if !slices.Contains(codes, e.glyph) {
			codes = append(codes, e.glyph)
		}
	}
	slices.Sort(codes)
	for _, c := range codes {
		fmt.Fprintf(&out, "\t%q: %q,\n", c, paths["\\"+c])
	}
	out.WriteString("}\n\n")
	out.WriteString("// setiIcons are the glyphs of source files by extension, and their colors in\n")
	out.WriteString("// the dark and light themes.\n")
	out.WriteString("var setiIcons = map[string]setiIconData{\n")
	for _, e := range entries {
		fmt.Fprintf(&out, "\t%q: {glyph: %q, dark: 0x%06X, light: 0x%06X},\n", e.ext, e.glyph, e.dark, e.light)
	}
	out.WriteString("}\n\n")
	out.WriteString("// setiNames are the glyphs of the files of Docker, by their whole name in\n")
	out.WriteString("// lower case.\n")
	out.WriteString("var setiNames = map[string]setiIconData{\n")
	for _, e := range named {
		fmt.Fprintf(&out, "\t%q: {glyph: %q, dark: 0x%06X, light: 0x%06X},\n", e.ext, e.glyph, e.dark, e.light)
	}
	out.WriteString("}\n")
	src, err := format.Source(out.Bytes())
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(src)
}

// glyphPath is the outline of the glyph r as SVG path data, in the middle of
// the grid of 24, at 150% of the text as VS Code draws it.
func glyphPath(f *sfnt.Font, buf *sfnt.Buffer, r rune) string {
	gi, err := f.GlyphIndex(buf, r)
	if err != nil || gi == 0 {
		return ""
	}
	const ppem = 36
	segs, err := f.LoadGlyph(buf, gi, fixed.I(ppem), nil)
	if err != nil || len(segs) == 0 {
		return ""
	}
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, s := range segs {
		n := map[sfnt.SegmentOp]int{sfnt.SegmentOpMoveTo: 1, sfnt.SegmentOpLineTo: 1, sfnt.SegmentOpQuadTo: 2, sfnt.SegmentOpCubeTo: 3}[s.Op]
		for _, a := range s.Args[:n] {
			x, y := float64(a.X)/64, float64(a.Y)/64
			minX, maxX = math.Min(minX, x), math.Max(maxX, x)
			minY, maxY = math.Min(minY, y), math.Max(maxY, y)
		}
	}
	k := 1.0
	w, h := maxX-minX, maxY-minY
	switch {
	case math.Max(w, h) > 23:
		k = 23 / math.Max(w, h)
	case w > 1.6*h:
		// A wide logo, such as that of Go, would be a sliver at the size of
		// the others: it takes the whole width.
		k = 22 / w
	}
	cx, cy := (minX+maxX)/2, (minY+maxY)/2
	pt := func(a fixed.Point26_6) string {
		x := (float64(a.X)/64-cx)*k + 12
		y := (float64(a.Y)/64-cy)*k + 12
		return strconv.FormatFloat(math.Round(x*100)/100, 'f', -1, 64) + " " + strconv.FormatFloat(math.Round(y*100)/100, 'f', -1, 64)
	}
	var sb strings.Builder
	for _, s := range segs {
		switch s.Op {
		case sfnt.SegmentOpMoveTo:
			if sb.Len() > 0 {
				sb.WriteString("Z")
			}
			sb.WriteString("M" + pt(s.Args[0]))
		case sfnt.SegmentOpLineTo:
			sb.WriteString("L" + pt(s.Args[0]))
		case sfnt.SegmentOpQuadTo:
			sb.WriteString("Q" + pt(s.Args[0]) + " " + pt(s.Args[1]))
		case sfnt.SegmentOpCubeTo:
			sb.WriteString("C" + pt(s.Args[0]) + " " + pt(s.Args[1]) + " " + pt(s.Args[2]))
		}
	}
	sb.WriteString("Z")
	return sb.String()
}

// woffToSFNT unpacks a WOFF 1 font into the TrueType or OpenType file it
// wraps, which sfnt reads.
func woffToSFNT(w []byte) ([]byte, error) {
	be := binary.BigEndian
	if len(w) < 44 || string(w[:4]) != "wOFF" {
		return nil, errors.New("not a WOFF font")
	}
	flavor := be.Uint32(w[4:])
	n := int(be.Uint16(w[12:]))
	type table struct {
		tag      uint32
		data     []byte
		checksum uint32
	}
	tables := make([]table, n)
	for i := range n {
		e := w[44+20*i:]
		if len(e) < 20 {
			return nil, errors.New("WOFF table directory cut short")
		}
		off, comp, orig := be.Uint32(e[4:]), be.Uint32(e[8:]), be.Uint32(e[12:])
		if int(off)+int(comp) > len(w) {
			return nil, errors.New("WOFF table out of range")
		}
		data := w[off : off+comp]
		if comp < orig {
			r, err := zlib.NewReader(bytes.NewReader(data))
			if err != nil {
				return nil, err
			}
			data, err = io.ReadAll(io.LimitReader(r, int64(orig)))
			if err != nil {
				return nil, err
			}
		}
		tables[i] = table{tag: be.Uint32(e), data: data, checksum: be.Uint32(e[16:])}
	}
	var out bytes.Buffer
	head := make([]byte, 12)
	be.PutUint32(head, flavor)
	be.PutUint16(head[4:], uint16(n))
	sr, es := 1, 0
	for sr*2 <= n {
		sr *= 2
		es++
	}
	be.PutUint16(head[6:], uint16(sr*16))
	be.PutUint16(head[8:], uint16(es))
	be.PutUint16(head[10:], uint16(n*16-sr*16))
	out.Write(head)
	off := 12 + 16*n
	records := make([]byte, 16*n)
	for i, t := range tables {
		r := records[16*i:]
		be.PutUint32(r, t.tag)
		be.PutUint32(r[4:], t.checksum)
		be.PutUint32(r[8:], uint32(off))
		be.PutUint32(r[12:], uint32(len(t.data)))
		off += (len(t.data) + 3) &^ 3
	}
	out.Write(records)
	for _, t := range tables {
		out.Write(t.data)
		for out.Len()%4 != 0 {
			out.WriteByte(0)
		}
	}
	return out.Bytes(), nil
}
