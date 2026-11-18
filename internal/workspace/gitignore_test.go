package workspace

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestIgnoreRules(t *testing.T) {
	rules := parseIgnore("", []byte("# comment\n*.log\n!keep.log\nbuild/\n/root.txt\ndocs/**/*.tmp\n\n"))
	rules = append(rules, parseIgnore("sub", []byte("local\n/only-here\n"))...)
	cases := []struct {
		rel  string
		dir  bool
		want bool
	}{
		{"x.log", false, true},
		{"deep/er/x.log", false, true},
		{"keep.log", false, false},
		{"build", true, true},
		{"build", false, false},
		{"src/build", true, true},
		{"root.txt", false, true},
		{"src/root.txt", false, false},
		{"docs/a/b/c.tmp", false, true},
		{"docs/c.tmp", false, true},
		{"other/c.tmp", false, false},
		{"sub/local", false, true},
		{"sub/x/local", true, true},
		{"local", false, false},
		{"sub/only-here", false, true},
		{"sub/x/only-here", false, false},
		{"main.go", false, false},
	}
	for _, c := range cases {
		if got := rules.ignored(c.rel, c.dir); got != c.want {
			t.Errorf("ignored(%q, dir=%v) = %v, want %v", c.rel, c.dir, got, c.want)
		}
	}
}

func TestListDir_Gitignore(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{".git", "build", "src"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string]string{".gitignore": "build/\n*.log\n", "debug.log": "", "main.go": ""} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	names := func(l Listing) []string {
		var got []string
		for _, p := range l.Entries {
			got = append(got, filepath.Base(p))
		}
		return got
	}
	on := ListDir(root, root, nil, true)
	if got := names(on); !slices.Equal(got, []string{"src", ".gitignore", "main.go"}) {
		t.Errorf("with .gitignore, got %v", got)
	}
	if len(on.Rules) == 0 {
		t.Error("with .gitignore, its rules should come back")
	}
	off := ListDir(root, root, on.Rules, false)
	if got := names(off); !slices.Equal(got, []string{"build", "src", ".gitignore", "debug.log", "main.go"}) {
		t.Errorf("without .gitignore, got %v", got)
	}
	if len(off.Rules) != 0 {
		t.Error("without .gitignore, no rules should be read")
	}
}
