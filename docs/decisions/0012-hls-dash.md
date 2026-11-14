# 0012 — HLS and DASH stream downloads

- Status: accepted
- Date: 2026-11-14

## Context
Most video on the web is not one file but a stream: an HLS playlist (`.m3u8`) or a DASH
manifest (`.mpd`) that lists many small segments, sometimes encrypted (AES-128), sometimes
with video and audio in separate tracks, sometimes live. Users expect a download manager
to save such a stream as one playable file. The engine now has a protocol plug-in point
(`engine.Protocol`), which owns the queue, pause, the `.peregrine.part` file, its rename,
checksums and speed limits; a protocol only produces the bytes.

## Decision
- New package `engine/proto/stream` with two protocols, kinds `hls` and `dash`, matched by
  an http(s) URL whose path ends in `.m3u8` / `.mpd` (query string ignored).
- **Parsing with the standard library only:** a small line parser for M3U8 and
  `encoding/xml` for the MPD subset we need (SegmentTemplate with `$Number$`/`$Time$` and
  SegmentTimeline, SegmentList, SegmentBase/BaseURL as one file fetched in byte ranges).
  No new dependency.
- **Choice of tracks:** HLS takes the highest-bandwidth variant and its separate audio
  rendition when it has one (DEFAULT, then AUTOSELECT); without ffmpeg it prefers a variant
  with muxed audio. DASH takes the best video and the best audio representation of the first
  period. DRM (SAMPLE-AES, ContentProtection) is refused with a clear message.
- **Download:** segments are fetched 4 at a time per track (option), retried with backoff,
  and written in order. AES-128 is decrypted in memory (key cached, IV from the tag or the
  media sequence number). EXT-X-MAP / DASH initialization is written once, before the first
  segment that uses it.
- **Resume:** per track, the sequence number of the next segment and the bytes written are
  saved with `Job.SaveState`; the next run truncates each file to that length and continues.
- **Live HLS:** the playlist is reloaded every target duration until EXT-X-ENDLIST. A pause
  or stop ends the recording: the next run only finalizes the file. Repeated reload failures
  or a playlist that stops growing also end it, so a dead stream cannot loop forever.
  Live DASH is not supported yet.
- **Output and ffmpeg:** without ffmpeg, HLS is saved as the concatenated segments (`.ts`,
  or `.mp4` for fragmented MP4). With ffmpeg, the result is remuxed (`-c copy`, no
  re-encoding) into `.mp4`. Two tracks (DASH, HLS with separate audio) need ffmpeg to mux;
  if it is missing the download fails before fetching anything, with a message telling the
  user to install it. ffmpeg is not bundled: it is found via an option (path), else next to
  the executable, else on PATH, and run with `exec.CommandContext` (no shell, paths as
  separate arguments, hidden console window on Windows).
- The second track and ffmpeg's output live in `<name>.peregrine.tmp/` next to the part
  file (never the system temp folder) and are removed when the download finishes.

## Alternatives considered
- `grafov/m3u8` or a DASH library — a dependency for a few hundred lines of parsing we can
  test ourselves; our needs (download, not playback) are a small subset.
- Hand the whole job to ffmpeg (`ffmpeg -i playlist.m3u8`) — no pause/resume, no speed
  limit, no progress, no parallel segments, and the browser's cookies would have to be
  passed on a command line.
- Bundle ffmpeg — tens of MB and LGPL/GPL build questions; we can revisit for releases.
- Write segments as separate files and concatenate at the end — doubles disk use and I/O;
  in-order writes into the part file make resume a simple truncate.

## Consequences
- Streams are downloadable from the extension and the CLI once the app registers the two
  protocols (`stream.NewHLS(stream.Options{...})`, `stream.NewDASH(...)`).
- Separate-track streams depend on a user-installed ffmpeg until we decide to ship one.
- A removed paused download may leave its `.peregrine.tmp/` folder (only for two-track
  streams); the engine only deletes the part file. A future engine hook can clean it.
- Security: playlist URIs are resolved against the playlist and limited to http/https;
  playlists are capped at 10 MB, segments at 256 MB and 200 000 per track; Cookie and
  Authorization go only to the playlist's host and its subdomains; no URL is logged.
- Only the first DASH period is downloaded; multi-period (ad-inserted) manifests are cut.
