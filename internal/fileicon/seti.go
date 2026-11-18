package fileicon

import (
	"path/filepath"
	"strings"

	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/paint"
)

// The source files of programming languages, and the files of Docker, show
// the logo of their language, as VS Code shows them with its icon theme Seti. Only those
// glyphs are kept, as paths in seti_icons.go, which tools/setigen writes;
// every other file takes an icon of Nople's own (fileicons.go). Seti is
// under the MIT license (Jesse Weed): see assets/seti/LICENSE.txt.

// setiIconData is the glyph of a source file and its colors.
type setiIconData struct {
	glyph       string
	dark, light uint32
}

// setiPaths are the glyphs, parsed once.
var setiPaths = map[string]*paint.Path{}

// setiIcon is the logo of the language of the source file at path, and its
// color in the dark or the light theme; false for a file that is no source.
func setiIcon(path string, dark bool) (*paint.Path, geom.Color, bool) {
	name := strings.ToLower(filepath.Base(path))
	icon, ok := setiNames[name]
	if !ok && (strings.HasPrefix(name, "dockerfile.") || strings.HasSuffix(name, ".dockerfile")) {
		// Dockerfile.dev and the like are Dockerfiles too.
		icon, ok = setiNames["dockerfile"]
	}
	if !ok {
		icon, ok = setiIcons[strings.TrimPrefix(filepath.Ext(name), ".")]
	}
	if !ok {
		return nil, geom.Color{}, false
	}
	p, ok := setiPaths[icon.glyph]
	if !ok {
		parsed, err := paint.ParseSVGPath(setiGlyphs[icon.glyph])
		if err != nil {
			parsed = nil
		}
		p = parsed
		setiPaths[icon.glyph] = p
	}
	if p == nil {
		return nil, geom.Color{}, false
	}
	c := icon.light
	if dark {
		c = icon.dark
	}
	return p, geom.Hex(c), true
}
