// Package i18n is the text of the UI in each language Nople ships, and the
// choice of the language from the settings or the OS.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"
	"sync"

	"golang.org/x/text/language"
)

// The text of the UI is in catalogs, one JSON file per language in locales/:
// a key to its text, which may hold the verbs of fmt (%s, %d, and %[2]d to
// take them in another order). "_name" is the name of the language in
// itself, for the menu, and "_locale" its BCP 47 tag, which gowaku's own
// words follow. A language is added by adding its file; a key it lacks
// falls back to English, and then to the key itself.

//go:embed locales/*.json
var localeFiles embed.FS

// catalog is the text of one language.
type catalog struct {
	id     string // the name of the file, such as "ja"
	name   string
	locale language.Tag
	text   map[string]string
}

var catalogs = struct {
	once sync.Once
	all  map[string]*catalog
	ids  []string
}{}

// loadCatalogs reads the catalogs that ship with the program.
func loadCatalogs() {
	catalogs.once.Do(func() {
		catalogs.all = map[string]*catalog{}
		ents, _ := localeFiles.ReadDir("locales")
		for _, e := range ents {
			data, err := localeFiles.ReadFile(path.Join("locales", e.Name()))
			if err != nil {
				continue
			}
			var m map[string]string
			if err := json.Unmarshal(data, &m); err != nil {
				panic("nople: locales/" + e.Name() + ": " + err.Error())
			}
			id := strings.TrimSuffix(e.Name(), ".json")
			c := &catalog{id: id, name: m["_name"], text: m}
			if c.name == "" {
				c.name = id
			}
			c.locale = language.Make(m["_locale"])
			if c.locale == language.Und {
				c.locale = language.Make(id)
			}
			catalogs.all[id] = c
			catalogs.ids = append(catalogs.ids, id)
		}
		slices.Sort(catalogs.ids)
	})
}

// Languages are the ids of the catalogs, in order.
func Languages() []string {
	loadCatalogs()
	return catalogs.ids
}

// Name is the name of the language id in itself.
func Name(id string) string {
	loadCatalogs()
	if c := catalogs.all[id]; c != nil {
		return c.name
	}
	return id
}

// current is the language the UI shows, set on the UI goroutine.
var current *catalog

// Set makes the UI show the language id: "system" or "" picks the
// one of the OS, and one there is no catalog of picks English.
func Set(id string) {
	loadCatalogs()
	if id == "" || id == "system" {
		id = systemLanguage()
	}
	c := catalogs.all[id]
	if c == nil {
		c = catalogs.all["en"]
	}
	current = c
}

// Current is the id of the language the UI shows.
func Current() string {
	if current == nil {
		Set("system")
	}
	return current.id
}

// Locale is the locale of the UI, which gowaku's own words follow.
func Locale() language.Tag {
	if current == nil {
		Set("system")
	}
	return current.locale
}

// systemLanguage is the catalog closest to the language of the OS, or the
// one NOPLE_LANG names, as "ja" or "en", which stands in for the OS.
func systemLanguage() string {
	loadCatalogs()
	if id := os.Getenv("NOPLE_LANG"); id != "" {
		if _, ok := catalogs.all[id]; ok {
			return id
		}
	}
	tags := make([]language.Tag, 0, len(catalogs.ids))
	for _, id := range catalogs.ids {
		tags = append(tags, catalogs.all[id].locale)
	}
	if len(tags) == 0 {
		return "en"
	}
	m := language.NewMatcher(tags)
	_, i, conf := m.Match(osLanguages()...)
	if conf == language.No {
		return "en"
	}
	return catalogs.ids[i]
}

// osLanguages are the languages the user set in the OS, best first.
func osLanguages() []language.Tag {
	var tags []language.Tag
	for _, s := range osUILanguages() {
		if t, err := language.Parse(s); err == nil {
			tags = append(tags, t)
		}
	}
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(env); v != "" && v != "C" && v != "POSIX" {
			v, _, _ = strings.Cut(v, ".")
			if t, err := language.Parse(strings.ReplaceAll(v, "_", "-")); err == nil {
				tags = append(tags, t)
			}
		}
	}
	return tags
}

// T is the text of key in the language of the UI. With args, the text is a
// format of fmt for them.
func T(key string, args ...any) string {
	if current == nil {
		Set("system")
	}
	s, ok := current.text[key]
	if !ok {
		if en := catalogs.all["en"]; en != nil {
			s, ok = en.text[key]
		}
	}
	if !ok {
		s = key
	}
	if len(args) > 0 {
		return sprintf(s, args...)
	}
	return s
}

// sprintf is fmt.Sprintf; T with args uses it.
var sprintf = fmt.Sprintf

// Has reports whether key has text in English, the language every key has.
func Has(key string) bool {
	loadCatalogs()
	en := catalogs.all["en"]
	_, ok := en.text[key]
	return ok
}

// Keys are the keys of the English catalog, without the ones of its
// metadata.
func Keys() []string {
	loadCatalogs()
	var ks []string
	for k := range catalogs.all["en"].text {
		if !strings.HasPrefix(k, "_") {
			ks = append(ks, k)
		}
	}
	slices.Sort(ks)
	return ks
}
