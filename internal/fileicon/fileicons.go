package fileicon

import (
	"path/filepath"
	"strings"

	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/paint"
	"github.com/bankai-tensei/gowaku/widgets/basic"
)

// A file icon is a glyph that tells the kind of the file, in the color of its
// language or kind: </> for code, {} for data, a picture for an image, and so
// on. At 16 pixels a glyph on a page would be a speck, so it is drawn alone,
// as large as it goes, as the icon themes of editors do. The tab of the dock
// shows the same glyph in its ink.

// fileKind is a kind of file: its glyph, on the grid of 24 inside the page.
type fileKind struct {
	glyph string
	// filledGlyph draws the glyph as a shape, not a line.
	filledGlyph bool
}

// The kinds of file.
var (
	kindCode     = fileKind{glyph: "M9.5 12l-2.5 2.5 2.5 2.5 M15 12l2.5 2.5-2.5 2.5 M13.25 11l-2 7"}
	kindMarkup   = fileKind{glyph: "M10.5 11.5l-3.25 3.25 3.25 3.25 M14 11.5l3.25 3.25-3.25 3.25"}
	kindStyle    = fileKind{glyph: "M10.75 11l-1 7.5 M14.75 11l-1 7.5 M8.5 13.25h8 M8 16.25h8"}
	kindData     = fileKind{glyph: "M10 11c-1.5 0-1.5 1-1.5 2s-.75 1.75-1.5 1.75c.75 0 1.5.75 1.5 1.75s0 2 1.5 2 M14.5 11c1.5 0 1.5 1 1.5 2s.75 1.75 1.5 1.75c-.75 0-1.5.75-1.5 1.75s0 2-1.5 2"}
	kindConfig   = fileKind{glyph: "M8 12.5h8.5 M8 16.5h8.5 M10.5 11v3 M14 15v3"}
	kindMarkdown = fileKind{glyph: "M7.5 18.5v-6.5l2.25 3 2.25-3v6.5 M15.25 12v6.5 M13.5 16.75l1.75 1.75 1.75-1.75"}
	kindText     = fileKind{glyph: "M8 12h8.5 M8 15h8.5 M8 18h5.5"}
	kindImage    = fileKind{glyph: "M7.5 18.5l3.25-4.5 2.25 2.75 1.5-1.75 3 3.5z M15.5 10.5a1.25 1.25 0 1 0 0 2.5a1.25 1.25 0 1 0 0-2.5z", filledGlyph: true}
	kindVector   = fileKind{glyph: "M12.25 10l3.75 5.25-3.75 4-3.75-4z M12.25 10v5.5 M12.25 15.5h0.01"}
	kindShell    = fileKind{glyph: "M8 12l3 2.5-3 2.5 M12.5 18h4.5"}
	kindLock     = fileKind{glyph: "M9 15h6.5v4H9z M10.25 15v-1.75a2 2 0 0 1 4 0V15"}
	kindGit      = fileKind{glyph: "M9.5 11v8 M15.5 11.5a1.5 1.5 0 1 0 0 3a1.5 1.5 0 1 0 0-3z M15.5 14.5c0 2.5-6 1.75-6 4"}
	kindPackage  = fileKind{glyph: "M8 13l4.25-2 4.25 2v4.5l-4.25 2-4.25-2z M8 13l4.25 2 4.25-2 M12.25 15v4.5"}
	kindBuild    = fileKind{glyph: "M10 11.5v7l5.5-3.5z", filledGlyph: true}
	kindDatabase = fileKind{glyph: "M8.25 12.5a4 1.5 0 1 0 8 0a4 1.5 0 1 0-8 0 M8.25 12.5v5a4 1.5 0 0 0 8 0v-5 M8.25 15a4 1.5 0 0 0 8 0"}
	kindArchive  = fileKind{glyph: "M8 10.5h8.5v8.5H8z M12.25 10.5v1 M12.25 12.75v1 M11.25 15h2v2h-2z"}
	kindLicense  = fileKind{glyph: "M12.25 10.75a2.5 2.5 0 1 0 0 5a2.5 2.5 0 1 0 0-5z M10.75 15.5l-1 3.5 2.5-1.25 2.5 1.25-1-3.5"}
	kindInfo     = fileKind{glyph: "M12.25 10a4.75 4.75 0 1 0 0 9.5a4.75 4.75 0 1 0 0-9.5z M12.25 12.5h0.01 M12.25 14.5v2.5"}
	kindKey      = fileKind{glyph: "M10 13.25a2 2 0 1 0 0 4a2 2 0 1 0 0-4z M12 15.25h5 M15.5 15.25v2"}
	kindFont     = fileKind{glyph: "M8.5 18.5l3.75-7.5 3.75 7.5 M10 15.75h4.5"}
	kindAudio    = fileKind{glyph: "M10.25 16a1.75 1.75 0 1 0 0 3.5a1.75 1.75 0 1 0 0-3.5z M12 17.75v-6.75l4-1.25v5.75", filledGlyph: false}
	kindVideo    = fileKind{glyph: "M7.75 12.5h6v5h-6z M13.75 14l3-1.75v5.5l-3-1.75"}
	kindTest     = fileKind{glyph: "M10.75 11h3 M11.25 11v3.25l-3.25 4.75h8.5l-3.25-4.75V11"}
	kindBinary   = fileKind{glyph: "M8.5 12v6 M11.5 12h2.5v6h-2.5z M16.5 12v6"}
	kindPDF      = fileKind{glyph: "M8 10h6l3 3v7.5H8z M14 10v3h3 M10 16h5 M10 18.5h5"}
)

// fileType is what a file is: its kind and the color of its page.
type fileType struct {
	kind  fileKind
	color geom.Color
}

// Colors of pages, by language and kind. They are mid tones, which carry the
// white glyph and read on both themes.
var (
	colGo       = geom.Hex(0x00A7D0)
	colPython   = geom.Hex(0x3B77A8)
	colRust     = geom.Hex(0xCE6A3E)
	colJS       = geom.Hex(0xC9A227)
	colTS       = geom.Hex(0x3178C6)
	colReact    = geom.Hex(0x1FA2C4)
	colC        = geom.Hex(0x5C6BC0)
	colCpp      = geom.Hex(0x00599C)
	colCSharp   = geom.Hex(0x7E57C2)
	colJava     = geom.Hex(0xD9573B)
	colKotlin   = geom.Hex(0x8E5BD6)
	colSwift    = geom.Hex(0xF05138)
	colRuby     = geom.Hex(0xCC342D)
	colPHP      = geom.Hex(0x7377AD)
	colLua      = geom.Hex(0x3D5AFE)
	colDart     = geom.Hex(0x0E8ACB)
	colZig      = geom.Hex(0xE0922F)
	colShader   = geom.Hex(0x5E8C31)
	colHTML     = geom.Hex(0xE44D26)
	colVue      = geom.Hex(0x41B883)
	colSvelte   = geom.Hex(0xFF3E00)
	colCSS      = geom.Hex(0x563D7C)
	colSass     = geom.Hex(0xC6538C)
	colJSON     = geom.Hex(0xCB8B3B)
	colYAML     = geom.Hex(0xA34E5A)
	colTOML     = geom.Hex(0x9C4221)
	colXML      = geom.Hex(0x8D6E63)
	colMarkdown = geom.Hex(0x519ABA)
	colText     = geom.Hex(0x7A8591)
	colImage    = geom.Hex(0x8E6CC0)
	colSVG      = geom.Hex(0xF29A2E)
	colShell    = geom.Hex(0x4E9A06)
	colPS       = geom.Hex(0x2B6CB0)
	colGit      = geom.Hex(0xE8533A)
	colLock     = geom.Hex(0x8A8F98)
	colSQL      = geom.Hex(0xD08A00)
	colArchive  = geom.Hex(0x8D6E63)
	colLicense  = geom.Hex(0xC79B2E)
	colInfo     = geom.Hex(0x3E8ED0)
	colEnv      = geom.Hex(0xB8A13A)
	colFont     = geom.Hex(0xD0587B)
	colMedia    = geom.Hex(0xD0587B)
	colTest     = geom.Hex(0x43A047)
	colBinary   = geom.Hex(0x6B7280)
	colPDF      = geom.Hex(0xD32F2F)
	colDocker   = geom.Hex(0x2496ED)
	colMake     = geom.Hex(0x6D8086)
)

// typesByExt are the types of files by extension.
var typesByExt = map[string]fileType{
	".go": {kindCode, colGo}, ".py": {kindCode, colPython}, ".pyi": {kindCode, colPython},
	".rs": {kindCode, colRust}, ".js": {kindCode, colJS}, ".mjs": {kindCode, colJS}, ".cjs": {kindCode, colJS},
	".jsx": {kindCode, colReact}, ".ts": {kindCode, colTS}, ".mts": {kindCode, colTS}, ".cts": {kindCode, colTS},
	".tsx": {kindCode, colReact}, ".c": {kindCode, colC}, ".h": {kindCode, colC},
	".cpp": {kindCode, colCpp}, ".cc": {kindCode, colCpp}, ".cxx": {kindCode, colCpp}, ".hpp": {kindCode, colCpp}, ".hh": {kindCode, colCpp},
	".cs": {kindCode, colCSharp}, ".java": {kindCode, colJava}, ".kt": {kindCode, colKotlin}, ".kts": {kindCode, colKotlin},
	".swift": {kindCode, colSwift}, ".rb": {kindCode, colRuby}, ".php": {kindCode, colPHP}, ".lua": {kindCode, colLua},
	".dart": {kindCode, colDart}, ".zig": {kindCode, colZig}, ".hlsl": {kindCode, colShader}, ".glsl": {kindCode, colShader},
	".wgsl": {kindCode, colShader}, ".frag": {kindCode, colShader}, ".vert": {kindCode, colShader},
	".html": {kindMarkup, colHTML}, ".htm": {kindMarkup, colHTML}, ".vue": {kindMarkup, colVue}, ".svelte": {kindMarkup, colSvelte},
	".xml": {kindMarkup, colXML}, ".xaml": {kindMarkup, colXML}, ".plist": {kindMarkup, colXML},
	".css": {kindStyle, colCSS}, ".scss": {kindStyle, colSass}, ".sass": {kindStyle, colSass}, ".less": {kindStyle, colCSS},
	".json": {kindData, colJSON}, ".jsonc": {kindData, colJSON}, ".json5": {kindData, colJSON}, ".ndjson": {kindData, colJSON},
	".yaml": {kindConfig, colYAML}, ".yml": {kindConfig, colYAML}, ".toml": {kindConfig, colTOML},
	".ini": {kindConfig, colText}, ".cfg": {kindConfig, colText}, ".conf": {kindConfig, colText}, ".properties": {kindConfig, colText},
	".editorconfig": {kindConfig, colText},
	".md":           {kindMarkdown, colMarkdown}, ".markdown": {kindMarkdown, colMarkdown}, ".mdx": {kindMarkdown, colMarkdown},
	".txt": {kindText, colText}, ".log": {kindText, colText}, ".rst": {kindText, colText}, ".csv": {kindData, colShell}, ".tsv": {kindData, colShell},
	".png": {kindImage, colImage}, ".jpg": {kindImage, colImage}, ".jpeg": {kindImage, colImage}, ".gif": {kindImage, colImage},
	".webp": {kindImage, colImage}, ".bmp": {kindImage, colImage}, ".ico": {kindImage, colImage}, ".tif": {kindImage, colImage}, ".tiff": {kindImage, colImage},
	".svg": {kindVector, colSVG},
	".sh":  {kindShell, colShell}, ".bash": {kindShell, colShell}, ".zsh": {kindShell, colShell}, ".fish": {kindShell, colShell},
	".ps1": {kindShell, colPS}, ".psm1": {kindShell, colPS}, ".bat": {kindShell, colPS}, ".cmd": {kindShell, colPS},
	".lock": {kindLock, colLock}, ".sum": {kindLock, colLock},
	".sql": {kindDatabase, colSQL}, ".db": {kindDatabase, colSQL}, ".sqlite": {kindDatabase, colSQL},
	".zip": {kindArchive, colArchive}, ".tar": {kindArchive, colArchive}, ".gz": {kindArchive, colArchive}, ".tgz": {kindArchive, colArchive},
	".7z": {kindArchive, colArchive}, ".rar": {kindArchive, colArchive}, ".xz": {kindArchive, colArchive}, ".zst": {kindArchive, colArchive},
	".ttf": {kindFont, colFont}, ".otf": {kindFont, colFont}, ".woff": {kindFont, colFont}, ".woff2": {kindFont, colFont},
	".mp3": {kindAudio, colMedia}, ".wav": {kindAudio, colMedia}, ".ogg": {kindAudio, colMedia}, ".flac": {kindAudio, colMedia}, ".m4a": {kindAudio, colMedia},
	".mp4": {kindVideo, colMedia}, ".mov": {kindVideo, colMedia}, ".webm": {kindVideo, colMedia}, ".mkv": {kindVideo, colMedia}, ".avi": {kindVideo, colMedia},
	".exe": {kindBinary, colBinary}, ".dll": {kindBinary, colBinary}, ".so": {kindBinary, colBinary}, ".dylib": {kindBinary, colBinary},
	".bin": {kindBinary, colBinary}, ".o": {kindBinary, colBinary}, ".a": {kindBinary, colBinary}, ".syso": {kindBinary, colBinary},
	".wasm": {kindBinary, colBinary}, ".class": {kindBinary, colBinary},
	".pdf": {kindPDF, colPDF},
	".pem": {kindKey, colEnv}, ".key": {kindKey, colEnv}, ".crt": {kindKey, colEnv},
	".rc": {kindConfig, colText}, ".mod": {kindPackage, colGo},
}

// typesByName are the types of files by their whole name, before the
// extension: manifests, build files and the files of git.
var typesByName = map[string]fileType{
	"go.mod": {kindPackage, colGo}, "go.work": {kindPackage, colGo}, "go.sum": {kindLock, colLock}, "go.work.sum": {kindLock, colLock},
	"package.json": {kindPackage, colJS}, "package-lock.json": {kindLock, colLock}, "yarn.lock": {kindLock, colLock}, "pnpm-lock.yaml": {kindLock, colLock},
	"tsconfig.json": {kindConfig, colTS}, "jsconfig.json": {kindConfig, colJS},
	"cargo.toml": {kindPackage, colRust}, "cargo.lock": {kindLock, colLock},
	"pyproject.toml": {kindPackage, colPython}, "requirements.txt": {kindPackage, colPython}, "setup.py": {kindPackage, colPython},
	"gemfile": {kindPackage, colRuby}, "gemfile.lock": {kindLock, colLock}, "composer.json": {kindPackage, colPHP},
	"dockerfile": {kindPackage, colDocker}, "docker-compose.yml": {kindConfig, colDocker}, "docker-compose.yaml": {kindConfig, colDocker},
	"compose.yml": {kindConfig, colDocker}, "compose.yaml": {kindConfig, colDocker},
	"makefile": {kindBuild, colMake}, "gnumakefile": {kindBuild, colMake}, "justfile": {kindBuild, colMake}, "taskfile.yml": {kindBuild, colMake},
	"cmakelists.txt": {kindBuild, colMake}, "build.gradle": {kindBuild, colKotlin}, "build.gradle.kts": {kindBuild, colKotlin},
	".gitignore": {kindGit, colGit}, ".gitattributes": {kindGit, colGit}, ".gitmodules": {kindGit, colGit}, ".gitkeep": {kindGit, colGit},
	".dockerignore": {kindConfig, colDocker}, ".editorconfig": {kindConfig, colText},
	".prettierrc": {kindConfig, colText}, ".eslintrc": {kindConfig, colText}, ".npmrc": {kindConfig, colText},
	"license": {kindLicense, colLicense}, "license.md": {kindLicense, colLicense}, "license.txt": {kindLicense, colLicense},
	"licence": {kindLicense, colLicense}, //nolint:misspell // the name of a file, as some projects spell it
	"copying": {kindLicense, colLicense}, "notice": {kindLicense, colLicense},
	"claude.md": {kindInfo, colInfo}, "agents.md": {kindInfo, colInfo}, "changelog.md": {kindInfo, colInfo}, "contributing.md": {kindInfo, colInfo},
}

// typeOf is the type of the file at path.
func typeOf(path string) (fileType, bool) {
	name := strings.ToLower(filepath.Base(path))
	if t, ok := typesByName[name]; ok {
		return t, true
	}
	switch {
	case strings.HasPrefix(name, "readme"):
		return fileType{kindInfo, colInfo}, true
	case name == ".env" || strings.HasPrefix(name, ".env."):
		return fileType{kindKey, colEnv}, true
	case strings.HasSuffix(name, "_test.go"), strings.Contains(name, ".test."), strings.Contains(name, ".spec."),
		strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py"):
		return fileType{kindTest, colTest}, true
	case strings.HasPrefix(name, "dockerfile."), strings.HasSuffix(name, ".dockerfile"):
		return fileType{kindPackage, colDocker}, true
	case strings.HasSuffix(name, ".d.ts"):
		return fileType{kindData, colTS}, true
	}
	t, ok := typesByExt[filepath.Ext(name)]
	return t, ok
}

// glyphs are the paths of the kinds, made once.
var (
	glyphs = map[string]*paint.Path{}
)

// glyphBox is the side of the square, on the grid of 24, that a glyph is
// scaled to fill: at 16 pixels a glyph on a page would be a speck, so the
// icon is the glyph alone, as large as it goes.
const glyphBox = 18

// fitGlyph moves and scales the glyph p so that its longer side is glyphBox,
// in the middle of the grid of 24.
func fitGlyph(p *paint.Path) *paint.Path {
	b := p.Bounds()
	k := float32(glyphBox) / max(b.W, b.H)
	cx, cy := b.X+b.W/2, b.Y+b.H/2
	at := func(o geom.Offset) (float32, float32) { return (o.X-cx)*k + 12, (o.Y-cy)*k + 12 }
	q := &paint.Path{}
	p.Walk(
		func(o geom.Offset) { q.MoveTo(at(o)) },
		func(o geom.Offset) { q.LineTo(at(o)) },
		func(c, o geom.Offset) {
			x1, y1 := at(c)
			x2, y2 := at(o)
			q.QuadTo(x1, y1, x2, y2)
		},
		func(a, b, o geom.Offset) {
			x1, y1 := at(a)
			x2, y2 := at(b)
			x3, y3 := at(o)
			q.CubeTo(x1, y1, x2, y2, x3, y3)
		},
		q.Close,
	)
	return q
}

// glyphOf is the path of the glyph of k, fitted to the grid, stroked or
// filled; nil for a kind without one.
func glyphOf(k fileKind) *paint.Path {
	if k.glyph == "" {
		return nil
	}
	if p, ok := glyphs[k.glyph]; ok {
		return p
	}
	parsed, err := paint.ParseSVGPath(k.glyph)
	if err != nil {
		panic(err)
	}
	p := fitGlyph(parsed)
	if !k.filledGlyph {
		p = p.Stroke(paint.StrokeStyle{Width: 2.25, Cap: paint.CapRound, Join: paint.JoinRound})
	}
	glyphs[k.glyph] = p
	return p
}

// TabIcon is the icon of the tab of the file at path: the glyph of its
// kind, which the tab draws in its ink; the page for a file of no kind.
func TabIcon(path string) *paint.Path {
	if p, _, ok := languageIcon(path, true); ok {
		return p
	}
	t, ok := typeOf(path)
	if !ok {
		return iconFile
	}
	if g := glyphOf(t.kind); g != nil {
		return g
	}
	return iconFile
}

// Widget is the icon of a file: the page in color, its glyph in
// white. A color other than zero takes the place of that of the type, as for
// a file that git marks.
func Widget(path string, plain, override geom.Color, dark bool) core.Widget {
	if p, c, ok := languageIcon(path, dark); ok {
		if override != (geom.Color{}) {
			c = override
		}
		return basic.Icon{Path: p, Size: 16, Color: c}
	}
	t, ok := typeOf(path)
	color := t.color
	if !ok {
		color = plain
	}
	if override != (geom.Color{}) {
		color = override
	}
	if g := glyphOf(t.kind); g != nil {
		return basic.Icon{Path: g, Size: 16, Color: color}
	}
	return basic.Icon{Path: iconFile, Size: 16, Color: color}
}

// goIcon is the icon of Go source: the letters GO, as large as the box
// holds. The logo of Go in Seti is three times as wide as it is tall, and at
// 16 pixels its letters shrink to a squiggle.
var goIcon = func() *paint.Path {
	p, err := paint.ParseSVGPath("M10.4 9a4.75 4.75 0 1 0 0.85 4.75H8 M17.75 7.25a4.75 4.75 0 1 0 0 9.5a4.75 4.75 0 1 0 0-9.5z")
	if err != nil {
		panic(err)
	}
	return p.Stroke(paint.StrokeStyle{Width: 2.5, Cap: paint.CapRound, Join: paint.JoinRound})
}()

// Colors of the icon of Go: the blue of Go, darker on a light ground.
var (
	goDark  = geom.Hex(0x00ADD8)
	goLight = geom.Hex(0x007D9C)
)

// languageIcon is the icon of the language of a source file: Go's own, else
// the logo Seti draws.
func languageIcon(path string, dark bool) (*paint.Path, geom.Color, bool) {
	if strings.EqualFold(filepath.Ext(path), ".go") {
		if dark {
			return goIcon, goDark, true
		}
		return goIcon, goLight, true
	}
	return setiIcon(path, dark)
}
