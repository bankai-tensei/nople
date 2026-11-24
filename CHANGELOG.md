# Changelog

All notable changes to Nople are recorded here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Changed

- Nople builds on gowaku v0.2.1. The folder dialog opens in front of Nople's window, and on Linux the numeric keypad types digits with Num Lock on.
- Windows: Nople no longer puts itself among the programs that open `.bat` and `.cmd` files, and registering takes out what an earlier version put there. With Nople among them, a double-click on a batch file asked which program to open it with instead of running it.

### Added

- The repository runs CI on every push and pull request (vet, tests on Linux and Windows, golangci-lint, and builds for macOS and arm64), and has issue templates, a security policy, contributing guides and third-party notices.

## [1.0.0] - 2026-11-18

The first public release, for Windows, Linux and macOS (Apple silicon).

[Unreleased]: https://github.com/bankai-tensei/nople/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/bankai-tensei/nople/releases/tag/v1.0.0
