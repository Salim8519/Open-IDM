# Roadmap

Scope: Windows + macOS + Chromium extension (ADR 0005). Tick tasks as they land. Work top to
bottom; a phase is done when every box is ticked and CI is green on Windows and macOS.

## Phase 0 — Plan and skeleton
- [x] Spec, CLAUDE.md, roadmap, first ADRs
- [x] Scope decision: desktop-first, extension required, Android deferred (ADR 0005)
- [x] Choose final product name: Peregrine (ADR 0007)
- [x] Confirm license (ADR 0004)
- [x] Go module, folder layout, `.gitignore`, LICENSE
- [x] GitHub Actions: build + `go test -race` + vet + staticcheck on Windows, macOS (+ Linux engine-only)
- [x] `internal/testsrv` evil test server (first version)

## Phase 1 — Engine + CLI (the heart: speed and reliability)
- [x] Single-connection download to `.peregrine.part` + atomic rename (R2)
- [x] Probe: size, `Accept-Ranges`, ETag, Last-Modified, filename from headers/URL
- [x] Filename sanitization + fuzz test
- [x] Segmented download with N connections (E1)
- [x] Dynamic segmentation (E2)
- [x] No-Range fallback (E3)
- [x] Retries with backoff (E5)
- [x] SQLite state + crash-safe resume (R1, R3)
- [x] Resume validation and changed-file detection (E6)
- [x] Speed limiter (global + per download) (Q2)
- [x] Queue with max concurrency (Q1)
- [x] Proxy support (E9): system, HTTP, SOCKS5
- [x] Checksum verify (I1, I2)
- [x] CLI `peregrine get/add/list/serve`
- [ ] Benchmark: saturate 1 Gbps locally; compare with aria2, IDM and FDM on real servers (next)

## Phase 2 — Desktop app (Windows + macOS)
v0.1.0 preview released 2026-11-13 (unsigned): https://github.com/Salim8519/Open-IDM/releases/tag/v0.1.0
- [x] Wails + React shell, engine wired in
- [x] Main list, add dialog, batch add
- [x] Categories and folders (Q4), duplicate handling (Q5)
- [ ] Tray / menu-bar icon, native notifications (in-app toasts exist)
- [x] Clipboard monitor (capture without extension)
- [x] Settings screen
- [ ] Scheduler (Q3) — next (asked for by the maintainer)
- [x] End-to-end tests: UI ↔ real engine (Playwright), browser ↔ extension ↔ host ↔ app (Playwright), CLI kill-and-resume
- [x] i18n EN + AR with RTL
- [x] Windows installer (NSIS) + portable zip + macOS DMG (universal) — built by the release workflow
- [ ] Signed auto-update
- [ ] Code signing: SignPath (Windows), Apple notarization (macOS)
- [ ] **v1.0 beta release**

## Security baseline
- [x] Threat model (docs/SECURITY-MODEL.md), SECURITY.md, ADR 0008
- [x] govulncheck + npm audit + CodeQL in CI, Dependabot
- [x] Signed build provenance + SHA-256 sums on releases
- [x] Strict CSP in the desktop UI

## Phase 3 — Chromium extension (Chrome, Edge, Brave, Opera, Vivaldi)
- [x] Native Messaging host + installer registration per browser (Windows registry, macOS folders)
- [x] Download interception with cookies/referer handoff; fall back to browser if app absent
- [x] Context menu: "Download with Peregrine", "Download all links"
- [x] Media detection popup (direct video; HLS/DASH listed, download in Phase 4)
- [ ] Publish: Chrome Web Store, Edge Add-ons
- [ ] **v1.0 release**

## Phase 4 — Video and torrent
- [ ] HLS/DASH downloader + ffmpeg merge
- [ ] yt-dlp sidecar integration
- [ ] FTP/FTPS
- [ ] BitTorrent + magnet

## Phase 5 — More browsers and platforms
- [ ] Firefox extension
- [ ] Safari extension (Xcode wrapper)
- [ ] Official Linux builds (AppImage/deb/rpm)

## Maybe later
- [ ] Android (plan in ADR 0003)
