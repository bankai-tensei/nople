package app

import (
	"image"
	_ "image/gif"  // registers GIF with image.Decode
	_ "image/jpeg" // registers JPEG with image.Decode
	_ "image/png"  // registers PNG with image.Decode
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bankai-tensei/nople/internal/bigfile"
	"github.com/bankai-tensei/nople/internal/syntax"
	"github.com/bankai-tensei/nople/internal/textfile"

	_ "golang.org/x/image/bmp"  // registers BMP with image.Decode
	_ "golang.org/x/image/webp" // registers WebP with image.Decode

	"github.com/bankai-tensei/gowaku/markdown"
	"github.com/bankai-tensei/gowaku/paint"
	"github.com/bankai-tensei/gowaku/widgets/editor"
)

type document struct {
	path    string
	ctrl    editor.Controller
	content textfile.Content
	// image is the picture of an image file, which shows instead of text.
	image *paint.Image
	// svg is the drawing of an SVG file, which shows instead of its text
	// unless the tab shows the source. svgErr is why it could not be read.
	svg    *paint.SVG
	svgErr error
	// hl is the colors of the text, once lexing finished; pal is the
	// palette they show in, that of the theme of the last build.
	hl  *syntax.Highlight
	pal *palette
	// markdown means the file is Markdown, which shows rendered unless the
	// tab shows the source; headings are its headings, for the outline.
	markdown bool
	headings []markdown.Heading
	// sheet is a CSV or TSV file read as a table, which shows unless the
	// tab shows the source.
	sheet *sheet
	// diff makes the document a diff tab of a change of git; marks are the
	// lines of an ordinary file that differ from the index.
	diff  *diffDoc
	marks *marks
	// gone means the file was deleted on disk after it opened.
	gone bool
	// dirty means the text has changes not saved; conflict that the file
	// changed on disk meanwhile; saveErr is why the last save failed.
	// editGen counts the edits, for the timers that follow them.
	dirty    bool
	conflict bool
	saveErr  error
	editGen  int
	// big shows a large text file without reading it all (see bigdoc.go);
	// wantBig says that the file is one, before newDocument opens it.
	big     *bigfile.Doc
	wantBig bool
	// bighl is the colors of a large file, by the blocks in view; tracked
	// says git tracks the file, whose changes a large file does not show.
	bighl   *bigHighlight
	tracked bool
	// A large file saves on a goroutine of its own (see saveBig): saveStart
	// is when the save that runs began, saveWritten and saveTotal how far it
	// is, and afterSave what waits for it to end.
	saveStart   time.Time
	saveWritten int64
	saveTotal   int64
	afterSave   []func(error)
}

// newDocument reads the file at path into a new document, and starts what
// follows its text.
func (a *app) newDocument(path string) *document {
	d := openDocument(path)
	if d.wantBig {
		a.openBig(d)
	}
	a.startHighlight(d)
	a.startMarks(d)
	return d
}

// openDocument reads the file at path into a new document. A large text file
// is not read: it opens through bigfile once the app has it (see
// newDocument).
func openDocument(path string) *document {
	d := &document{path: path}
	if isImagePath(path) {
		d.image, d.content = readImage(path)
		return d
	}
	if c, ok := bigContent(path); ok {
		d.content = c
		d.wantBig = !c.Binary
		return d
	}
	d.content = textfile.Read(path, false)
	d.ctrl.SetText(d.content.Text)
	if isMarkdownPath(path) && d.content.Err == nil && !d.content.Binary {
		d.markdown = true
		d.headings = markdown.Headings(d.content.Text)
	}
	if isSVGPath(path) && d.content.Err == nil && !d.content.Binary {
		d.svg, d.svgErr = paint.ParseSVG([]byte(d.content.Text))
	}
	if sep := sheetSeparator(path); sep != 0 && d.content.Err == nil && !d.content.Binary {
		// The text of the controller, whose line breaks are \n, is what an
		// edit of a cell finds its field in.
		d.sheet, _ = parseSheet(d.ctrl.Text(), sep)
	}
	return d
}

// openAsText reads a binary file again as text, as the user asked.
func (d *document) openAsText() {
	d.content = textfile.Read(d.path, true)
	d.ctrl.SetText(d.content.Text)
}

// title is the label of the tab.
func (d *document) title() string {
	if d.diff != nil {
		return diffTitle(d)
	}
	return filepath.Base(d.path)
}

// languageOf names the language of a file for the status bar, from its name.
func languageOf(path string) string {
	name := strings.ToLower(filepath.Base(path))
	switch name {
	case "makefile", "gnumakefile":
		return "Makefile"
	case "dockerfile":
		return "Dockerfile"
	case "go.mod", "go.sum", "go.work":
		return "Go Module"
	case ".gitignore", ".gitattributes":
		return "Ignore"
	}
	if l, ok := languages[filepath.Ext(name)]; ok {
		return l
	}
	if syntax.IsLogPath(name) {
		return "Log"
	}
	return T("lang.plain")
}

// languages are the names of languages by extension.
var languages = map[string]string{
	".go": "Go", ".md": "Markdown", ".markdown": "Markdown",
	".js": "JavaScript", ".mjs": "JavaScript", ".cjs": "JavaScript", ".jsx": "JavaScript JSX",
	".ts": "TypeScript", ".tsx": "TypeScript JSX", ".json": "JSON", ".jsonc": "JSON with Comments",
	".html": "HTML", ".htm": "HTML", ".css": "CSS", ".scss": "SCSS",
	".py": "Python", ".rs": "Rust", ".c": "C", ".h": "C", ".cpp": "C++", ".cc": "C++", ".hpp": "C++",
	".cs": "C#", ".java": "Java", ".kt": "Kotlin", ".swift": "Swift", ".rb": "Ruby", ".php": "PHP",
	".sh": "Shell", ".bash": "Shell", ".ps1": "PowerShell", ".bat": "Batch", ".cmd": "Batch",
	".yaml": "YAML", ".yml": "YAML", ".toml": "TOML", ".ini": "INI", ".xml": "XML", ".svg": "SVG",
	".sql": "SQL", ".lua": "Lua", ".txt": T("lang.plain"), ".hlsl": "HLSL", ".glsl": "GLSL",
	".csv": "CSV", ".tsv": "TSV", ".wgsl": "WGSL", ".zig": "Zig", ".dart": "Dart", ".vue": "Vue", ".svelte": "Svelte",
}

// imageExts are the extensions of the pictures that show as images.
var imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true, ".bmp": true}

// isImagePath reports whether the file at path shows as a picture.
func isImagePath(path string) bool { return imageExts[strings.ToLower(filepath.Ext(path))] }

// isSVGPath reports whether the file at path is an SVG drawing, which shows
// drawn and also has a source to read.
func isSVGPath(path string) bool { return strings.EqualFold(filepath.Ext(path), ".svg") }

// readImage decodes the picture at path. GIF shows its first frame.
func readImage(path string) (*paint.Image, textfile.Content) {
	f, err := os.Open(path)
	if err != nil {
		return nil, textfile.Content{Err: err}
	}
	defer f.Close() //nolint:errcheck // only read
	c := textfile.Content{}
	if st, err := f.Stat(); err == nil {
		c.Size = st.Size()
		if c.Size > textfile.MaxOpenSize {
			c.Err = textfile.ErrTooLarge
			return nil, c
		}
	}
	m, _, err := image.Decode(f)
	if err != nil {
		c.Err = err
		return nil, c
	}
	return paint.NewImage(m), c
}
