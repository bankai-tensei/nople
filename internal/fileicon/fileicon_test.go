package fileicon

import (
	"testing"
)

func TestTypeOf(t *testing.T) {
	cases := map[string]fileKind{
		"main.go": kindCode, "main_test.go": kindTest, "go.mod": kindPackage, "go.sum": kindLock,
		"README.md": kindInfo, "docs/PLAN.md": kindMarkdown, "LICENSE": kindLicense, ".gitignore": kindGit,
		"logo.svg": kindVector, "icon.png": kindImage, "app.tsx": kindCode, "style.css": kindStyle,
		"data.json": kindData, "ci.yml": kindConfig, "run.ps1": kindShell, "Makefile": kindBuild,
		"Dockerfile": kindPackage, ".env.local": kindKey, "schema.sql": kindDatabase, "a.zip": kindArchive,
		"x.test.ts": kindTest, "types.d.ts": kindData, "nople.exe": kindBinary, "spec.pdf": kindPDF,
	}
	for name, want := range cases {
		got, ok := typeOf(name)
		if !ok || got.kind.glyph != want.glyph {
			t.Errorf("%s: kind %q, want %q", name, got.kind.glyph, want.glyph)
		}
	}
	if _, ok := typeOf("weird.qqq"); ok {
		t.Error("an unknown extension has no type")
	}
	// Every glyph parses, as a shape and as a line.
	for _, ft := range typesByExt {
		glyphOf(ft.kind)
		TabIcon("x" + ".go")
	}
}
