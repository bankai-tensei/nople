package app

import (
	"github.com/bankai-tensei/nople/internal/i18n"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestMain runs the tests in Japanese, which they look for on screen,
// whatever the language of the machine.
func TestMain(m *testing.M) {
	i18n.Set("ja")
	os.Exit(m.Run())
}

func TestCatalogs_KeysInCode(t *testing.T) {
	re := regexp.MustCompile(`\bT\("([^"]+)"`)
	used := map[string]bool{}
	filepath.WalkDir(filepath.Join(repoRoot, "internal"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		data, _ := os.ReadFile(p)
		for _, m := range re.FindAllStringSubmatch(string(data), -1) {
			used[m[1]] = true
			if !i18n.Has(m[1]) {
				t.Errorf("%s: T(%q) has no text", p, m[1])
			}
		}
		return nil
	})
	for _, k := range i18n.Keys() {
		if !used[k] {
			t.Errorf("key %q is in no T call", k)
		}
	}
}

func TestLanguage_Switch(t *testing.T) {
	defer i18n.Set("ja")
	h, dir := openSample(t, false)
	h.app.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	h.app.openFile(filepath.Join(dir, "main.go"), false)
	h.settle()
	h.app.chooseLanguage("en")
	h.settle()
	for _, s := range []string{"File", "View", "Ln 1, Col 1"} {
		if !h.has(s) {
			t.Errorf("%q should show in English:\n%s", s, h.tt.Dump())
		}
	}
	if h.has("ファイル(F)") {
		t.Error("the Japanese menu should go")
	}
	data, _ := os.ReadFile(h.app.settingsPath)
	if !strings.Contains(string(data), `"language": "en"`) {
		t.Errorf("the choice should be saved: %s", data)
	}
	h.shot("i18n-en")
	if T("search.found", 3, 12) != "12 results in 3 files" {
		t.Errorf("reordered verbs: %q", T("search.found", 3, 12))
	}
	i18n.Set("ja")
	if T("search.found", 3, 12) != "3 ファイルに 12 件" {
		t.Errorf("ja: %q", T("search.found", 3, 12))
	}
	i18n.Set("xx")
	if i18n.Current() != "en" {
		t.Error("a language with no catalog falls back to English")
	}
	i18n.Set("system")
	if !slices.Contains(i18n.Languages(), i18n.Current()) {
		t.Error("the system language should resolve to a catalog")
	}
}
