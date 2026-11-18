# Nople language files

The text of the UI lives in one JSON file per language (`en.json`, `ja.json`). The files are embedded in the executable.

## Adding a language

1. Copy `en.json` to a file named after the language (for example `ko.json` or `zh-Hans.json`).
2. Set `"_name"` to the name of the language in that language (it appears in the menu, for example `"한국어"`), and `"_locale"` to its BCP 47 tag (for example `"ko"`). gowaku's own text, such as dialog buttons, follows this tag.
3. Translate the values. Do not change the keys.
4. Run `go test ./...`. `TestCatalogs_Agree` checks that no keys are missing or extra, and that the format verbs (`%s`, `%d`, …) match the English ones.

The new file appears by itself in **View → Language** and in the `"language"` setting.

## Format

Values are Go `fmt` format strings. To change the order of arguments, number them like `%[2]d` (for example `"%[2]d results in %[1]d files"`).

A key with no translation falls back to English, and a key missing from English too shows as the key itself.

## Using it from code

`T("key")`, or `T("key", args...)` with arguments. Add new keys to both `ja.json` and `en.json`. `TestCatalogs_KeysInCode` checks that every key used with `T` exists and that no key goes unused.
