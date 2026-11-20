# Contributing to Nople

[日本語](CONTRIBUTING.ja.md)

Thank you for helping. Bug reports, small fixes and documentation are all welcome. For a new feature, open an issue first so that we can agree on it before the code is written: Nople leaves out a good deal on purpose (see "What Nople leaves out" in the [README](README.md)), so a feature that fits another editor may not fit here.

## Before you write code

Read [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md): how the code is laid out. The rules that matter most:

- **Nothing to set up.** A folder opens with everything ready; a feature that needs configuring first is a hard sell.
- **Read-only Git.** Nople never writes to a repository, and runs git without its optional locks.
- **Files stay as they were.** Encoding and line breaks are written back as they were read, and saves go through a temporary file beside the original ([internal/safewrite](internal/safewrite)).
- **Idle means idle.** Nothing polls or redraws while nothing changes, so that Nople uses no CPU when left alone.
- **No cgo.** Nople builds for every system with `CGO_ENABLED=0`.

## Commands

Check and test, as CI does:

```bash
gofmt -s -l . && go vet ./... && golangci-lint run
```

```bash
GOOS=linux golangci-lint run && GOOS=darwin golangci-lint run
```

```bash
go test ./...
```

```bash
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...
```

The tests run the whole UI without a window, so they need no GPU. `NOPLE_SHOTS=1` writes screenshots to `docs/shots/`, which helps when you change what you see.

Large files have checks of their own that write gigabytes, so they run only when asked; see the top of [internal/bigfile/verify_test.go](internal/bigfile/verify_test.go). Run them when you change how large files are read or saved. The numbers of [docs/PERFORMANCE.md](docs/PERFORMANCE.md) come from measures you can run again the same way.

To work on gowaku at the same time, see "Build" in the README for a `go.work`.

## Documentation

- Doc comments are in English.
- User documentation is in English and Japanese: `X.md` is English and `X.ja.md` is Japanese, and so are the words of the UI, in [internal/i18n/locales](internal/i18n/locales). Change both when you can; if you can write only one, say so in the pull request and we will translate it.

## Commits and pull requests

- Commit messages are a summary line in the imperative, then why, wrapped at 72 columns.
- One commit, one intent. Formatting-only changes go in their own commit.
- A pull request that changes what you see includes a screenshot, light and dark when it matters.
- CI must pass.

## Reporting bugs

Open an issue with the OS and its version, the Nople version (`nople -version`), what you opened and did, and what you expected. For text and IME bugs, add the input method and the text. For security problems, see [SECURITY.md](SECURITY.md).

By contributing, you agree that your contributions are licensed under the [MIT License](LICENSE).
