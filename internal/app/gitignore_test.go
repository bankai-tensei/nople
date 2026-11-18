package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/bankai-tensei/nople/internal/config"
)

// TestGitignoreSetting turns the setting of .gitignore on and off with the
// folder open, and sees the explorer, quick open and the search follow it at
// once, with .git hidden either way.
func TestGitignoreSetting(t *testing.T) {
	dir := sampleFolder(t)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "debug.log"), []byte("needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, false)
	h.app.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	if h.app.settings.Gitignore {
		t.Fatal(".gitignore should not be followed by default")
	}
	if err := h.app.openFolder(dir); err != nil {
		t.Fatal(err)
	}
	h.app.search(searchOptions{query: "needle"})
	h.settle()

	searched := func() []string {
		var got []string
		for _, fh := range h.app.find.files {
			got = append(got, filepath.Base(fh.path))
		}
		return got
	}
	check := func(gitignore bool) {
		t.Helper()
		for _, name := range []string{"debug.log", "build"} {
			if h.has(name) == gitignore {
				t.Errorf("gitignore %v: the explorer shows %s = %v", gitignore, name, !gitignore)
			}
		}
		if h.has(".git") {
			t.Errorf("gitignore %v: the explorer should leave out .git", gitignore)
		}
		if !h.app.files.ready {
			t.Fatalf("gitignore %v: quick open should have its files", gitignore)
		}
		for _, rel := range []string{"debug.log", "build/out.txt"} {
			if slices.Contains(h.app.files.rel, rel) == gitignore {
				t.Errorf("gitignore %v: quick open lists %s = %v: %v", gitignore, rel, !gitignore, h.app.files.rel)
			}
		}
		got := searched()
		if !slices.Contains(got, "notes.txt") || slices.Contains(got, "debug.log") == gitignore {
			t.Errorf("gitignore %v: the search found %v", gitignore, got)
		}
	}
	check(false)

	// Turned on, what .gitignore ignores goes at once.
	h.app.changeSettings(func(s *config.Settings) { s.Gitignore = true })
	h.settle()
	check(true)
	if s, err := config.Load(h.app.settingsPath); err != nil || !s.Gitignore {
		t.Errorf("the setting should be written: %+v, %v", s, err)
	}

	// Turned off by editing the file, it comes back.
	s := h.app.settings
	s.Gitignore = false
	if err := config.Save(h.app.settingsPath, s); err != nil {
		t.Fatal(err)
	}
	h.app.applySettings()
	h.settle()
	check(false)
}
