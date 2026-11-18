package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileIcons_Gallery(t *testing.T) {
	names := []string{
		"main.go", "main_test.go", "go.mod", "go.sum", "README.md", "CHANGELOG.md", "LICENSE", ".gitignore", ".env",
		"app.tsx", "index.js", "util.ts", "lib.rs", "tool.py", "Main.java", "view.vue", "page.html", "site.css", "theme.scss", "main.c", "app.cpp", "App.cs", "main.swift", "app.rb", "index.php", "init.lua", "main.dart", "Main.kt", "main.zig",
		"data.json", "ci.yml", "Cargo.toml", "Makefile", "Dockerfile", "Dockerfile.dev", "compose.yaml", ".dockerignore", "run.ps1", "build.sh", "schema.sql",
		"logo.svg", "photo.png", "font.ttf", "song.mp3", "clip.mp4", "bundle.zip", "spec.pdf", "nople.exe", "notes.txt", "weird.qqq",
	}
	for _, dark := range []bool{false, true} {
		dir := filepath.Join(t.TempDir(), "icons")
		os.MkdirAll(filepath.Join(dir, "src"), 0o755)
		for _, n := range names {
			os.WriteFile(filepath.Join(dir, n), []byte("x\n"), 0o644)
		}
		h := newHarness(t, dark)
		if err := h.app.openFolder(dir); err != nil {
			t.Fatal(err)
		}
		h.settle()
		h.app.dock.Pin(explorerID)
		for _, n := range []string{"main.go", "README.md", "data.json", "logo.svg", "Makefile", "site.css"} {
			h.app.openFile(filepath.Join(dir, n), false)
		}
		h.settle()
		name := "icons"
		if dark {
			name += "-dark"
		}
		h.shot(name)
	}
}
