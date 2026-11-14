# 0013 — Video-site downloads through a bundled yt-dlp

- Status: accepted
- Date: 2026-11-14

## Context
Users expect a download manager to save videos from YouTube, Vimeo, X/Twitter, TikTok,
Instagram, Facebook, Twitch, SoundCloud, Bilibili and similar sites. Those pages do not link
to a file: each site needs its own extractor (signatures, JSON APIs, HLS/DASH manifests,
separate video and audio streams to merge), and the sites change weekly. Writing and
maintaining that in Go is out of reach for this project. yt-dlp already supports about 1800
sites, is actively maintained, and is public domain. Commit 02d26f6 added the
`engine.Protocol` plug-in point for exactly this kind of source.

## Decision
- New package `engine/proto/video` (kind `"video"`) runs **yt-dlp as a separate program**
  (a sidecar), never as a library and never through a shell: `exec.CommandContext`, each
  argument separate, the page URL always last after `--` so it can never be read as an
  option. On Windows the console window is hidden and the process runs in a kill-on-close
  job object; on macOS/Linux it gets its own process group. Pausing cancels the context,
  which kills yt-dlp and any ffmpeg it started.
- **Matching:** a built-in list of well-known video hosts (and their subdomains) is
  matched automatically; any other page can be forced with `Kind: "video"`.
- **Options we always pass:** `--no-config` (a user or portable yt-dlp config cannot add
  options such as `--exec`), `--no-playlist` (default), `--no-mtime`, `--encoding utf-8`,
  `-f bv*+ba/b --merge-output-format mp4/mkv --ffmpeg-location <ffmpeg>` when ffmpeg is
  found (otherwise `-f b`, the best single file, since merging needs ffmpeg),
  `--limit-rate` from the engine's speed limit, and `--referer`/`--user-agent` from the
  browser's headers. `--restrict-filenames` is an option.
- **Naming:** a first call with `--print filename` gives the title and final extension;
  the protocol calls `Job.SetName` while the job is fresh and saves `{"named":true}` so a
  resumed job does not ask again.
- **Work folder and resume:** yt-dlp downloads into `<PartPath>.d/` with a fixed
  `media.%(ext)s` template; its own `.part` files make pause/resume continue (the next Run
  reuses the folder; `--continue` is yt-dlp's default). At the end the single result file
  is renamed to `PartPath` and the folder removed; the engine then hashes, renames and
  marks the file as usual.
- **Progress:** `--newline --progress-template` prints a tagged, machine-readable line per
  update; streams of a merged download are summed into `SetDownloaded`/`SetSize`.
- **Errors:** the last `ERROR:` line from stderr becomes the message, without colors,
  without the `[extractor] id:` prefix, and with URLs stripped of credentials, query
  strings and fragments (they can carry tokens).
- **Cookies:** a `Cookie` header from the browser is written as a Netscape cookies file
  for that host (mode 0600, inside the work folder, or a temporary probe folder for the
  naming call), passed with `--cookies`, and deleted as soon as yt-dlp exits; stale ones
  from a crash are removed on the next Run.
- **Finding the tools:** constructor options (`YtDlpPath`, `FFmpegPath`); otherwise next to
  the running executable or in the macOS app's `Contents/Resources`, then `PATH` (and on
  macOS Homebrew's folders, which Finder-started apps do not have on `PATH`). Without
  yt-dlp the download fails with a clear message (`ErrNoYtDlp`).
- **Bundling:** the release workflow downloads pinned builds from their official release
  URLs, verifies hard-coded SHA-256 sums, checks the yt-dlp version and that ffmpeg is an
  (L)GPL build, and puts them next to the app: `yt-dlp.exe`, `ffmpeg.exe`, `ffprobe.exe` in
  the install/portable folder on Windows; `yt-dlp`, `ffmpeg`, `ffprobe` (universal, via
  `lipo`) in `Peregrine.app/Contents/MacOS` on macOS. Pinned: yt-dlp 2026.10.26; ffmpeg
  9.0.2 from gyan.dev "essentials" (Windows) and ffmpeg.martin-riedl.de (macOS).
- **Licenses:** yt-dlp is under the Unlicense (public domain), fine to ship with GPL-3.0
  code. The ffmpeg builds are GPL builds (an LGPL build would also be acceptable); they are
  separate programs we run, not linked, and GPL is compatible with our GPL-3.0 (ADR 0004)
  in any case. Packages include `ffmpeg-LICENSE.txt` and `THIRD-PARTY-NOTICES.txt` with
  the versions, build URLs and where to get the source, as the GPL requires.
- Tests use a fake yt-dlp (`engine/proto/video/internal/fakeytdlp`) built at test time;
  nothing touches the network.

## Alternatives considered
- Pure-Go extractors (e.g. `kkdai/youtube`) — one site each, break often, and would put
  site-specific scraping in our code base.
- Linking yt-dlp through an embedded Python — huge, CGO, and a second runtime to secure.
- Asking users to install yt-dlp themselves — works (PATH is still searched) but is too
  much friction for the default case.
- Auto-updating yt-dlp at run time (`yt-dlp -U`) — would be an unsigned download and a
  network call outside the user's downloads; we ship a pinned, verified build instead and
  update it with releases.

## Consequences
- Releases are bigger (about 80–150 MB more per platform) and must bump yt-dlp regularly,
  because sites break old versions. Each bump needs new pinned SHA-256 sums.
- yt-dlp and ffmpeg run with the user's rights and parse untrusted web content; that is
  now part of our attack surface (see SECURITY-MODEL). We reduce it with `--no-config`,
  arguments without a shell, the URL after `--`, and checksum-pinned binaries.
- Browser cookies are on disk briefly while yt-dlp runs (0600, deleted afterwards).
- The engine proxy setting is not passed to yt-dlp yet; yt-dlp uses the system proxy
  environment variables. Speed-limit changes apply at the next start/resume of a video.
- A removed (not completed) video download can leave its `<name>.peregrine.part.d` work
  folder behind until the engine's removal also deletes it.
- A playlist URL with `Playlist` on still has to produce one file; real playlist support
  (a folder of files) is future work.
