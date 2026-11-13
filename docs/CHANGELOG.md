# Changelog

Format: [Keep a Changelog](https://keepachangelog.com/). Newest first.

## Unreleased

### Added
- **Scheduler:** download only between set times on chosen days (overnight windows work).
  At the stop time running downloads go back to the queue and continue in the next window.
  Optionally quit, sleep or shut down when everything is done, after a 60-second countdown
  you can cancel. The status bar shows the schedule with a one-click "Turn off".
- **Tray / menu-bar icon:** open, pause all, resume all, turn the schedule on/off, quit; the
  tooltip shows active downloads and speed. Closing the window keeps Peregrine running in
  the tray (Windows, Linux; can be turned off). On macOS the close button hides the app.
- **System notifications** when a download finishes or fails while the window is in the
  background (can be turned off).
- **Start when I sign in** (Windows, macOS, Linux; off by default): starts in the tray
  without a window.
- **14 more languages:** Chinese (Simplified), French, German, Hindi, Indonesian, Italian,
  Japanese, Korean, Persian, Portuguese (Brazil), Russian, Spanish, Turkish and Urdu (Persian
  and Urdu right-to-left). Machine-assisted; corrections from native speakers are welcome.

### Changed
- Toolbar buttons show clearly when they can be used: disabled buttons are faded, usable
  ones are full strength, and Resume / Pause / Remove light up green / amber / red as soon as
  the selection allows them; stronger hover and keyboard focus.

### Fixed
- **Browser downloads no longer start in the browser first.** The extension now decides
  before Chrome/Edge/Brave name the file, show their download bubble or open Save as; a
  download Peregrine takes never appears in the browser, and one it leaves continues
  without restarting. (Before, the browser's download started and was cancelled a moment
  later, so both seemed to run.)
- **The Add dialog comes to the front** when the browser sends a download, instead of only
  flashing the Peregrine button in the taskbar (Windows).

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
