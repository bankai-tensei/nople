package i18n

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// verbs are the verbs of fmt in s, in order of their arguments.
var verbRe = regexp.MustCompile(`%(\[\d+\])?[-+# 0]*\d*(\.\d+)?[a-zA-Z%]`)

func verbs(s string) []string {
	var vs []string
	for _, m := range verbRe.FindAllString(s, -1) {
		if m == "%%" {
			continue
		}
		// %[2]d and %d say the same verb; the order is the translator's.
		v := m[len(m)-1:]
		vs = append(vs, v)
	}
	slices.Sort(vs)
	return vs
}

func TestCatalogs_Agree(t *testing.T) {
	loadCatalogs()
	en := catalogs.all["en"]
	if en == nil || catalogs.all["ja"] == nil {
		t.Fatal("en and ja should ship")
	}
	for _, id := range Languages() {
		c := catalogs.all[id]
		for k, v := range c.text {
			if strings.HasPrefix(k, "_") {
				continue
			}
			ev, ok := en.text[k]
			if !ok {
				t.Errorf("%s: key %q is not in en", id, k)
				continue
			}
			if !slices.Equal(verbs(v), verbs(ev)) {
				t.Errorf("%s: %q has verbs %v, en %v", id, k, verbs(v), verbs(ev))
			}
		}
		for k := range en.text {
			if _, ok := c.text[k]; !ok && !strings.HasPrefix(k, "_") {
				t.Errorf("%s lacks %q", id, k)
			}
		}
	}
}

// The language of the OS is found without the environment, as when Finder
// starts the app (on Windows and macOS, which say it in a setting).
func TestSystemLanguage_WithoutEnvironment(t *testing.T) {
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		t.Setenv(env, "")
	}
	if len(osUILanguages()) == 0 {
		t.Skip("the OS says no language of its own")
	}
	t.Logf("OS languages %v, picked %s", osUILanguages(), systemLanguage())
	if len(osLanguages()) == 0 {
		t.Errorf("osLanguages() empty with OS languages %v", osUILanguages())
	}
}
