# Architecture

How the code of Nople is laid out, and the decisions that are not obvious from reading it.

## Layout

`main.go` only reads the flags and calls `app.Run`. Everything else is under `internal/`.

| Package | What it does |
|---|---|
| `internal/app` | The window and all of the UI: explorer, tabs, document views, the search / source control / outline panels, the terminal panel, settings tab, menus, session, file operations. `app.Run(Options)` is the entry point |
| `internal/vt` | The terminal screen: VT escape parsing, the cell grid, scrollback |
| `internal/pty` | Pseudo terminals: ConPTY on Windows, `/dev/ptmx` on Linux, `TIOCPTYGNAME` on macOS |
| `internal/diff` | Line diff (Myers), in-line changes, and the line map between the two sides |
| `internal/git` | Runs `git` and reads the state and files of a repository |
| `internal/syntax` | Lexes with chroma and sorts tokens into color kinds |
| `internal/textfile` | Reading and writing text: binary detection, UTF-8 / BOM / Shift_JIS, line endings |
| `internal/workspace` | Reading folders, `.gitignore`, walking the whole tree |
| `internal/i18n` | Language files (`locales/*.json`) and `T` |
| `internal/config` | The settings (`settings.json`) and how they are read and written |
| `internal/fileicon` | File icons: Seti logos for source files and Nople's own symbols for the rest |
| `internal/trash` | Moving files to the OS trash |
| `internal/sysutil` | Starting commands without a console window |
| `assets` | Embedded pictures (word logo, window icon) and the Seti license |
| `tools/setigen` | Generates `internal/fileicon/seti_icons.go` from the Seti glyphs |

The packages under `internal/` other than `app` do not know about the UI. Text they need translated is either translated with `i18n.T` or returned as a sentinel error (`git.ErrNoGit`, `textfile.ErrNotSJIS`) that `app` translates.

Tests live beside each package. The `app` tests are split by feature (`tabs_test.go`, `find_test.go`, `edit_test.go`, …) and drive the UI without a window through `gowakutest`.

## UI library

The UI is built on [gowaku](https://github.com/bankai-tensei/gowaku). The main pieces Nople uses:

| Feature | gowaku widget |
|---|---|
| Overall layout, tabs, side strip | `nav.DockLayout` with `AutoHide` |
| Explorer, search results, source control | `collection.Tree` |
| Editor and diff | `editor.Editor`, `editor.Controller`, `editor.View` |
| Markdown | `markdown.Markdown` inside `field.SelectionArea` and `basic.Scroll` |
| Images and SVG | `interact.InteractiveViewer` |
| Menus, command palette | `overlay.MenuBar`, `field.CommandPalette` |
| Status bar, info bars, breadcrumbs | `nav.StatusBar`, `control.InfoBar`, `nav.Breadcrumb` |
| Terminal | A custom `RenderObject` that lays glyphs on a cell grid |

## Documents and tabs

A *document* (`document.go`) is a file: its path, one `editor.Controller`, and what was read. A *tab* (`tabs.go`) is one view of it, with its own `editor.View`. Splitting a file to the right adds a second tab on the same document, so the text is shared while selection and scroll position are not.

## Syntax highlighting

chroma can only start lexing from a named state, not resume a state stack in the middle of a file, so per-line incremental lexing is not possible. Nople lexes the whole document once, off the UI goroutine, and hands the result to `Decorations`. That is about 1 MB/s; until it finishes the text is shown uncolored, and files over 4 MB are not colored at all. After an edit, the whole document is lexed again 300 ms after typing stops.

## Git

Nople runs `git` rather than embedding a library: `rev-parse --show-toplevel` to find the repository and `status --porcelain=v2 --branch -z` to read it. On Windows, commands start with `CREATE_NO_WINDOW`. Git state is reread whenever the folder watcher sees a change, including inside `.git`.

The diff is Nople's own Myers implementation. The two editors of a diff tab sync through each other's `OnScroll` and `SetScroll`; `SetScroll` does not notify, so there is no feedback loop. Lines are mapped through the diff, so unchanged regions line up exactly and changed regions proportionally. Because gowaku cannot yet insert blank space between lines, the two sides do not always sit at the same height around large changes.

## Terminal

On Windows the shell runs under ConPTY. If the parent's standard handles are pipes (as under `go test`), the child inherits them, so Nople passes empty handles with `STARTF_USESTDHANDLES`. `internal/vt` parses the output into a cell grid with scrollback; wide characters take two cells. The view shapes each glyph once and rebuilds a row only when its version changes.

## Files on disk

`watch.go` watches the open folder, every folder the explorer has read, and the folders of open files with fsnotify, batching events for 150 ms. A changed file is reloaded keeping each tab's selection and scroll; if it has unsaved edits, Nople keeps them and shows an info bar offering to reload from disk instead. Saving writes back in the encoding and line endings the file was read with, and refuses (with an info bar) if a character cannot be written in Shift_JIS. Text that contained undecodable bytes stays read-only, since writing it back would corrupt it.

## Regenerating the Seti icons

`internal/fileicon/seti_icons.go` is generated. Only the glyphs used for programming-language source files (and Docker files) are kept, as SVG paths:

```bash
go run ./tools/setigen <path to vscode's extensions/theme-seti/icons>
```
