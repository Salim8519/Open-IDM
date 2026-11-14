# Changelog

Format: [Keep a Changelog](https://keepachangelog.com/). Newest first.

## Unreleased

### Fixed
- Time left now follows a lasting speed change within a few seconds. In 0.4.1, halving the
  speed (a slower server, a speed limit, fewer peers) left the time far too low: it showed
  2 s when about 6 s were left.

## [0.4.2] - 2026-11-14

### Added
- Update check: Peregrine asks GitHub once a day whether a newer version is out and shows it
  in the status bar (and as a system notification when the window is hidden). Click it to
  open the release page. Settings has "Check now" and a switch to turn the check off. It
  sends nothing about you and installs nothing by itself.

### Changed
- Browser extension: sizes read like the app ("1 MB" instead of "1.0 MB").

## [0.4.1] - 2026-11-14

### Changed
- Field Guide is warmer and bolder, with the same colours as the website: bold labels and
  headings (Arabic body text a little heavier too), a sun-yellow highlight on the current
  sidebar item and the selected download, and file types in their own colours. Every pair
  still meets WCAG AA contrast.
- The app and extension icon use the website's exact paper, ink and rust colours. The
  extension's section labels are a sun-yellow sticker, as on the website.
- File-type icons are coloured in every theme (Classic too): in the download list, the
  sidebar, the details pane and the Add dialog. The sidebar's status icons use their status
  colour (green for completed, red for errors, and so on).
- All text is bigger: about 1.5 px larger everywhere (Arabic, Persian and Urdu too), with
  taller rows and buttons to match. The extension popup is a little bigger as well.
- **Steadier time left.** The speed still shows what is happening right now, but the time
  left is based on a longer average and counts down like a clock; it only corrects itself
  when the speed really changes for a few seconds, so it no longer jumps between, say,
  3 and 5 minutes with every burst. It is rounded to read calmly (10 s steps under 10
  minutes, whole minutes under an hour, 5-minute steps above).
- When no data arrives for a few seconds, the time left says **"Waiting…"** (reconnecting,
  or a torrent looking for peers) instead of going blank or showing a huge number.
- Torrents show their number of **peers** in the details instead of a misleading
  "1 segment · 4 active of 8".

### Fixed
- **Windows: "Show in folder" did nothing.** It opened an invisible Explorer window (one per
  click); the folder now opens in front with the file selected. "Open" had the same flag
  removed.

## [0.4.0] - 2026-11-14

### Added
- New default look, **Field Guide**: warm paper and ink with a serif face, in a day and a
  night edition that follow your system. Arabic, Persian and Urdu use the Noto Naskh Arabic
  font, bundled with the app. The previous look is still available as **Classic** in
  Settings → Theme. If you never changed the theme, you get Field Guide after updating; if
  you picked light or dark, nothing changes. Text, buttons, icons and input edges stand out
  clearly from the paper in both editions (every pair meets WCAG AA contrast); disabled
  buttons stay legible, just quieter.
- New icon in the same style: the diving falcon in ink on paper with a rust tray, for the
  app, the tray, the installer, .torrent files and the browser extension. Redrawn for 16 px
  so it stays readable in the taskbar and the browser toolbar.
- The browser extension's popup and settings page use the Field Guide look too (day and
  night follow your browser), in all 16 languages, right-to-left included.
- Project website at https://peregrinedownloader.app with download links that always
  point to the newest release (Windows, macOS, Linux CLI, browser extension), in English
  and Arabic.

## [0.3.1] - 2026-11-14

### Changed
- **Windows:** the app uses about half the memory (~75 MB instead of ~130–150 MB, window
  open or in the tray) and less CPU when idle. WebView2 now runs without a GPU process,
  and the speed graph no longer redraws the window every second when nothing is
  downloading.
- **Windows:** in the tray with nothing downloading, Peregrine now gives unused memory
  back to Windows after 30 seconds: ~31 MB instead of ~75 MB. Opening the window or a
  captured download is just as fast.
- Less CPU during big downloads. Only the rows that changed are redrawn, and the hidden
  window no longer draws progress nobody sees. In tests with 4 fast downloads and 100
  items, CPU dropped by about half with the window open, and almost to zero in the tray.
- **Windows:** the installer is about half the size (~63 MB instead of 120 MB) thanks to
  stronger compression. The installed files are the same.
- The browser-extension helper (`peregrine-host`) is a third smaller (7 MB instead of
  10.5 MB). It no longer includes the download engine.

### Fixed
- **Windows:** 0.3.0 would not start ("libstdc++-6.dll / libgcc_s_seh-1.dll was not
  found"). The Windows app is built without cgo again, and the release now fails if the exe
  needs MinGW DLLs.
- **Windows:** updating while Peregrine was running in the tray (or the browser extension
  was connected) could fail with "Error opening file for writing" or leave the old version
  running. Setup and uninstall now close Peregrine first, after asking.

## [0.3.0] - 2026-11-14

### Added
- **BitTorrent:** magnet links, `.torrent` links and `.torrent` files from your computer
  (double-click once Peregrine is the handler, or drag onto the window). Pause and resume
  keep the metadata, so a magnet does not have to find peers for it again. Nothing is
  seeded after a download finishes. Windows asks once to allow Peregrine through the
  firewall when the first torrent starts.
- **Video sites:** download the video from YouTube and other video pages (bundled yt-dlp
  and ffmpeg). Right-click a page → "Download video with Peregrine", the popup's "Download
  the video on this page" button, or tick "This is a video page" in Add download.
- **Streams:** HLS (`.m3u8`, including live recording) and DASH (`.mpd`) download and
  merge into one `.mp4`. The extension's media list can now download them.
- **FTP and FTPS** links, with resume.
- A small badge in the list shows the download type (Torrent, Video site, Stream, FTP).

## [0.2.1] - 2026-11-14

### Fixed
- **The desktop app no longer freezes** after the first download changes state (v0.2.0):
  buttons like Clear completed did nothing and the list stopped updating. The new tray and
  notification code called back into the engine while it was busy; it now runs separately.

## [0.2.0] - 2026-11-14

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
- **Browser extension in 16 languages:** the 14 new app languages (Chinese (Simplified),
  French, German, Hindi, Indonesian, Italian, Japanese, Korean, Persian, Portuguese (Brazil),
  Russian, Spanish, Turkish, Urdu) now also translate the extension's menus, popup and
  settings, following the browser's language (Persian and Urdu right-to-left).

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
