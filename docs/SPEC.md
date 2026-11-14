# Peregrine — Product Specification

Status: draft v0.1 · Owner: project maintainer · Name: Peregrine (ADR 0007).

## 1. Vision
A download manager that is **faster than IDM, cleaner than FDM, and free forever**:
open source (GPL-3.0), no ads, no telemetry, no nagging. Desktop-first: Windows and macOS
(see ADR 0005).

## 2. Goals and non-goals
**Goals**
- Max out the user's bandwidth with multi-connection segmented downloads.
- Never lose a download: survive crashes, reboots, network drops, sleep.
- Capture downloads and videos from the browser with one click.
- Run on Windows 10+ and macOS 12+ (Intel and Apple Silicon).
- Secure by default.

**Non-goals (for now)**
- Android (deferred, ADR 0005) and iOS (platform does not allow this class of app).
- Official Linux support (it builds, but is not tested or supported in v1).
- DRM-protected streams (we will never circumvent DRM).
- Cloud accounts / sync.

## 3. Platforms and components
| Component | Tech | Phase |
|---|---|---|
| Engine (library) | Go | 1 |
| CLI `peregrine` | Go | 1 |
| Desktop app (Windows, macOS) | Wails + React + TS | 2 |
| Browser extension | Manifest V3, Chromium (Chrome/Edge/Brave/Opera/Vivaldi) | 3 |
| Video & torrent | yt-dlp sidecar, anacrolix/torrent | 4 |
| Later | Firefox extension, Safari extension, official Linux | 5 |
| Maybe later | Android (ADR 0003, superseded) | — |

## 4. Engine requirements

### 4.1 Downloading
- **E1 Segmented download:** split a file into N segments (default 8, max 32) fetched in
  parallel with HTTP `Range`.
- **E2 Dynamic segmentation:** when a connection finishes, split the largest remaining
  segment in half and give the new half to the free connection (IDM-style). No segment
  smaller than 256 KiB.
- **E3 Fallback:** if the server does not support `Range` (no `Accept-Ranges: bytes` / 200
  instead of 206), download with one connection and mark as "not resumable".
- **E4 Protocols:** HTTP/1.1, HTTP/2, HTTPS; HTTP/3 (quic-go) optional; FTP/FTPS (phase 4).
- **E5 Retries:** per-segment retry with exponential backoff + jitter (default 10 tries,
  capped at 60 s), then pause with an error; never restart from zero if resumable.
- **E6 Validation on resume:** compare `ETag` / `Last-Modified` / size; if the file changed
  on the server, ask the user (restart or keep).
- **E7 Expired links:** if a resume gets 403/410, allow "refresh address" (user pastes a new
  URL or the extension re-captures it) keeping progress.
- **E8 Headers/auth:** custom headers, cookies and referer from the browser; basic/digest auth.
- **E9 Proxy:** system proxy, HTTP/HTTPS/SOCKS5 proxy, per-download override.

### 4.2 Reliability and state
- **R1** State stored in SQLite (WAL). Segment progress flushed at least every 2 s and on
  pause/quit.
- **R2** Data written to `<name>.peregrine.part`; on completion: fsync → verify → atomic rename.
- **R3** On startup, unfinished downloads restore exactly; partially written tail of a
  segment is re-verified by size, never trusted blindly.
- **R4** Disk-full, permission-denied, and file-locked errors pause the download with a
  clear message instead of corrupting data.
- **R5** Pre-allocate file size when known (sparse where supported).

### 4.3 Queue and control
- **Q1** Queues with max concurrent downloads (default 3); drag to reorder.
- **Q2** Global and per-download speed limit.
- **Q3** Scheduler: start/stop queue at times on chosen days (window may cross midnight);
  at the stop time running downloads return to the queue with their progress; action after
  the queue finishes (nothing, quit, sleep, shutdown) after a 60 s cancellable countdown.
- **Q4** Categories by file type → target folders (Video, Music, Documents, Archives, Programs).
- **Q5** Duplicate detection: same URL or same target filename → ask (rename / overwrite / skip).

### 4.4 Integrity
- **I1** Optional checksum check (MD5/SHA-1/SHA-256/SHA-512) entered by user or detected.
- **I2** Show the final SHA-256 of every completed file.

### 4.5 Performance targets
- Saturate a 1 Gbps link on a single large file from a capable server.
- Idle CPU < 1%; RAM < 80 MB for the desktop app with 100 queued items. RAM counts the app
  and its WebView processes (private working set). Measure with `scripts/footprint.ps1`
  (Windows); see ADR 0015 for the latest numbers.
- Startup to usable window < 1 s on a modern machine.

## 5. Desktop app (Phase 2)
- Main list: name, size, progress, speed (live), time left (steady countdown, "Waiting…" when no data arrives; ADR 0019), status; filter by category/status; search.
- Add dialog: URL(s), folder, filename, connections, checksum, start now / add to queue.
- Batch add: paste many links; pattern links (`file[001-100].zip`).
- Clipboard monitor (opt-in): a copied downloadable link opens the add dialog. This is the
  capture method for browsers without our extension (e.g. Safari, Firefox in v1).
- Tray (Windows, Linux) / menu-bar (macOS) icon with open, pause all, resume all, schedule
  on/off and quit; closing the window keeps the app in the tray (setting; on macOS the close
  button hides the app as usual). System notifications on finish/error while the window is
  not focused (setting).
- Start when the user signs in (opt-in), hidden in the tray: HKCU Run key (Windows),
  LaunchAgent (macOS), XDG autostart (Linux).
- Themes: Field Guide (default; day and night editions that follow the system) and
  Classic (light/dark/system), chosen in Settings; RTL support (Arabic, Persian, Urdu); 16 UI languages
  (`desktop/frontend/src/i18n`, tray/notification text in `desktop/locales`).
- Settings: connections, speed limits, folders, proxy, scheduler, updates, extension pairing.
- Signed auto-update (user can disable).

## 6. Browser extension (Phase 3)
The extension is the only secure, reliable way to send clicked browser downloads to the app
(proxy, folder-watching and code-injection approaches were rejected in ADR 0005).
- One Manifest V3 codebase for Chrome, Edge, Brave, Opera, Vivaldi.
- The app installer registers the Native Messaging host for each installed Chromium browser
  (Windows registry keys; macOS `NativeMessagingHosts` folders).
- Intercept browser downloads (`downloads.onCreated`) above a size threshold or by file type,
  cancel the browser's copy and hand off URL + cookies + referer + headers to the app.
- Detect media (HLS `.m3u8`, DASH `.mpd`, direct video) on the page and show a "Download"
  button / popup list.
- Right-click "Download with Peregrine" / "Download all links".
- Communicates only via Native Messaging; the host only accepts our extension IDs.
- Minimal permissions; no remote code; no data leaves the machine.
- If the app is not running/installed, fall back to the browser's own download.

## 7. Video, streams, FTP and torrent (Phase 4)
All are plug-ins behind `engine.Protocol` (kind per download), registered by
`internal/protocols` for the app, the CLI and the test servers.
- **FTP/FTPS** (`ftp://`, `ftps://`): own client, TLS when offered, resume with `REST`
  (ADR 0011).
- **HLS/DASH** (`.m3u8`, `.mpd`): native segment downloader for unencrypted streams, live
  HLS recording, remuxed with ffmpeg; DRM is refused (ADR 0012).
- **Video sites** (YouTube and other known hosts, or any page marked "This is a video
  page"): bundled yt-dlp sidecar, with ffmpeg for merging. Both are pinned and checksum-
  verified in the release build (ADR 0013).
- **BitTorrent** via `anacrolix/torrent`: magnet links, `.torrent` URLs and local
  `.torrent` files (double-click / file association, drag onto the window). Peregrine
  registers as the handler for `magnet:` and `.torrent`. No seeding after completion;
  upload while downloading is capped (ADR 0014). Later: choose files inside a torrent,
  seeding ratio option.

## 8. Later platforms (Phase 5 and beyond)
- Firefox extension: same code as the Chromium one with small manifest/API changes.
- Safari extension: Safari Web Extension packaged in an Xcode wrapper app (needs Apple
  Developer account).
- Official Linux: AppImage/deb/rpm, tested in CI.
- Android: deferred. Plan kept in ADR 0003. Keep `engine/` free of desktop-only code.

## 9. Security and privacy
- No telemetry or analytics. No network traffic except user downloads and the update check.
- Updates signed (minisign / platform signing) and verified before install.
- Filename sanitization: strip path separators, `..`, control chars, Windows reserved names
  (`CON`, `NUL`, `COM1`…), trailing dots/spaces; limit length.
- Windows Mark-of-the-Web on completed files; macOS quarantine attribute.
- Credentials (proxy/auth) stored in the OS keychain (Windows Credential Manager,
  macOS Keychain), never in plain SQLite.
- Logs redact query strings, cookies and auth headers.
- Code signing: Windows via SignPath (free for OSS); macOS notarization (Apple Developer account).

## 10. Quality
- Engine tests run against `internal/testsrv`, which simulates: dropped connections,
  throttling, no-Range servers, wrong Content-Length, changing files, redirects, 403 on
  resume, slow first byte.
- `go test -race ./...`, `go vet`, `staticcheck` in CI on Windows and macOS (Linux job
  for the engine only, to keep it portable).
- Fuzz tests for filename sanitization, URL parsing and header parsing.
- Release builds are reproducible and published by GitHub Actions with checksums.

## 11. Open questions
- Final product name.
- ~~Whether to ship ffmpeg/yt-dlp bundled or download on first use.~~ Bundled (ADR 0013).
- HTTP/3 on by default or opt-in.
