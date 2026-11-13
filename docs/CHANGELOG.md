# Changelog

Format: [Keep a Changelog](https://keepachangelog.com/). Newest first.

## Unreleased

## [0.1.1] - 2026-11-13

### Added
- **Store-ready extension package:** `Peregrine-<version>-chromium-store-upload.zip` (no
  manifest key) plus the full listing, permission justifications and images in `docs/store/`.
- **"Connect your browser" guide** on first launch: opens the extensions page of Chrome, Edge
  or Brave, shows the bundled extension folder for *Load unpacked*, and turns green by itself
  when the extension connects. Reopen it from Settings → Browser & clipboard.
- The installer, portable zip and macOS app now include the unpacked extension.

### Changed
- The repository is now `github.com/Salim8519/Peregrine` (old links redirect).

## [0.1.0] - 2026-11-13
First public preview. Windows and macOS builds are not code-signed yet.

### Added
- **Engine:** multi-connection segmented downloads (up to 32) with dynamic segmentation,
  crash-safe resume (SQLite, progress saved every second), retries with backoff, fallback for
  servers without range support, detection of files that changed on the server, refreshing
  expired links, global and per-download speed limits, queue with max active downloads,
  categories, HTTP/SOCKS5 proxy, SHA-256 of every file and checksum verification, safe
  filenames, Mark-of-the-Web (Windows) / quarantine (macOS).
- **Desktop app** (Windows, macOS): download list with filters, search and segment map, add and
  batch-add dialogs, context menu, settings, clipboard monitor, keyboard shortcuts, light/dark
  themes, English and Arabic (RTL).
- **Browser extension** (Chrome, Edge, Brave, Opera, Vivaldi): catches downloads with cookies
  and referrer, "Download with Peregrine" menus, media detection, falls back to the browser when
  the app is unavailable. Talks to the app only through a local native messaging host.
- **CLI:** `peregrine get` (resumable), `add`, `list`, `serve`.
- Logo and brand (ADR 0007), GPL-3.0 license, CI on Windows/macOS/Linux, release pipeline.
- Calm, tool-like interface: neutral colours with one accent, live per-connection progress in
  every row, speed graph in the status bar.

### Security
- Built with Go 1.27.4 (Go 1.26.0 had 30 known standard-library vulnerabilities).
- govulncheck, npm audit and CodeQL on every push and weekly; Dependabot updates.
- Release files carry SHA-256 sums and a signed build provenance attestation.
- Strict Content Security Policy in the app; threat model in docs/SECURITY-MODEL.md (ADR 0008).

- Scope narrowed to Windows + macOS + Chromium extension; Android deferred, Linux unofficial
  (ADR 0005, supersedes ADR 0003).
- Project plan: spec, roadmap, AI rules (CLAUDE.md), ADRs 0001–0004 (Go engine, Wails desktop,
  Android via gomobile, GPL-3.0 proposed).
