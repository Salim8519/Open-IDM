# 0015 — Lighter desktop app: memory, installer and native host size

- Status: accepted
- Date: 2026-11-14

## Context
SPEC 4.5 asks for idle CPU < 1% and RAM < 80 MB, but nobody had measured it. We measured
on Windows 11 with `scripts/footprint.ps1`, which counts private working set (Task
Manager's "Memory") over the app and every WebView2 process it starts:

| Build (0.3.0, 4 items in the list) | Window open | Hidden in tray |
|---|---|---|
| Before | 127–153 MB | 132–156 MB |
| After  | ~78 MB | ~75 MB |

The Go engine is only ~10 MB. The rest is WebView2, and it stays alive while the window is
hidden in the tray. The biggest single process was WebView2's GPU process (~45 MB). Idle
CPU was mostly React re-rendering the whole app once a second for the status-bar speed
graph, even with nothing downloading.

## Decision
- Start WebView2 with the GPU disabled (`windows.Options.WebviewGpuIsDisabled`). Our UI is
  a flat list with a few short CSS animations, and software rendering is enough for it.
- The speed graph's once-a-second tick returns the same array when idle and the graph is
  already flat (`nextSpeedHistory`), so React skips the re-render.
- Measure with `scripts/footprint.ps1` before a release, and when changing the UI shell.

## Alternatives considered
- Unload the React UI while hidden in the tray. A test build that renders nothing came to
  66 MB, only ~12 MB less than the GPU-off build. Browser captures and the when-done
  countdown arrive as events while hidden, so an unloaded UI would have to buffer and
  replay them. Not worth the risk for 12 MB.
- Suspend the hidden WebView (`TrySuspend`, `MemoryUsageTargetLevel`). Wails v2 does not
  expose the WebView2 controller, so this needs a Wails fork. Revisit with Wails v3.
- Split into a tray engine process plus a UI process started on demand. That would bring
  the tray cost down to ~10 MB, but it is a large change (two processes, IPC, single
  instance). Keep as a future option if the tray footprint matters more.

## Consequences
- ~50% less memory and about half the idle CPU on Windows, with or without the window.
- Heavy scrolling or animations use the CPU instead of the GPU. Fine for this UI. If we
  ever add rich visuals, measure again before turning the GPU back on.
- macOS (WKWebView) is unaffected by the flag and has not been measured yet.
- WebView2's floor (~66 MB) means the window-open target of 80 MB leaves little headroom,
  so UI additions should stay light.

## Addendum: give memory back while hidden and idle (same day)

At ~75 MB in the tray we were at WebView2's floor. Closing the WebView while hidden (a
tray/UI process split) would cost ~0.5–1 s every time a captured download opens the Add
dialog, so it was rejected. Instead:

- **Trim while hidden and idle** (`desktop/trim*.go`). Once the window has been hidden for
  30 s with nothing downloading and no when-done countdown, the app calls
  `debug.FreeOSMemory` and, on Windows, empties the working set of its own process and of
  its `msedgewebview2.exe` descendants (`SetProcessWorkingSetSizeEx(-1, -1)`). It repeats
  at most every 5 minutes while still hidden. Nothing is closed or reloaded. Errors are
  only logged, and a panic is recovered. No files are involved.
- **Measured** with the 0.3.0 profile (4 items), same scripts as above:
  - In the tray: 75 MB before, 31 MB after (~80 s after hiding). Idle CPU still near 0.
  - Captured link → Add dialog: screenshots at fixed times show the dialog appearing by
    150 ms and fully drawn by 300 ms, with and without trimming. No visible difference.
- **Honest caveat:** this lowers private working set (Task Manager's "Memory") and really
  frees physical RAM for other apps. The pages come back from the standby list or the
  page file as the app touches them, so the number creeps up between trims.
- macOS: only the Go heap is returned. The WebView part is Windows-only for now.

## Addendum: heavy downloads (same day)

Measured with a local server, 4 downloads × 8 connections at 32 MB/s in total, and 100 items
in the list, in a throwaway profile:

| | Window open | Hidden in tray |
|---|---|---|
| Before | 172–175 MB, 20–23% of one core | 167–173 MB, 19–52% |
| After | 104–160 MB, 10–14% | 94–97 MB, ~0.4% UI (engine 0.3–13%) |

The engine itself takes a steady 2–5% of one core at that rate, with short spikes (ramp-up,
about once a minute). The UI was the waste:

- **No progress events to a hidden window** (`desktop/uievents.go`, `forwardToUI`). While
  hidden in the tray, the 500 ms progress ticks are not sent to the WebView. Status
  changes, captured links, settings and everything else still are. The first tick after
  showing updates the numbers within 0.5 s. The hidden flag is mirrored in an
  `atomic.Bool`, because engine subscribers run under the engine lock and `life.mu` is
  sometimes held while calling the engine (lock-order safety).
- **Only changed rows redraw** (`DownloadTable.tsx`). Rows are a memoized component with
  one stable handler object, and a progress tick only replaces the active downloads'
  objects. Before, every tick redrew all 100 rows.
- **The speed graph skips stale samples** (`nextSpeedHistory`). With no update for 1.5 s,
  as while hidden, it does not repeat the last speed as a flat line.

## Addendum: download and disk size (same day)

The 0.3.0 Windows installer was 120.5 MB. Its contents, compressed: ffmpeg 38.8 MB,
ffprobe 38.7 MB, yt-dlp 23.4 MB, Peregrine.exe 12.5 MB, peregrine-host.exe 4.7 MB,
WebView2 bootstrapper 1.7 MB. NSIS was using its default zlib compression.

Decisions:
- **Installer: `SetCompressor /SOLID lzma` with a 128 MB dictionary.** ffmpeg.exe and
  ffprobe.exe are almost the same static binary. A dictionary larger than ffmpeg.exe
  (~100 MB) lets ffprobe compress almost for free. With the exact 0.3.0 files the installer
  goes from 120.5 MB to 63.2 MB, and every unpacked file is byte-identical. Costs: the
  release build takes ~2.5 min longer to compress, and setup needs ~128 MB of RAM and a
  few more seconds to unpack. Nothing changes in the installed app.
- **Native messaging host: no engine import.** It pulled in the engine and SQLite only to
  reuse a few message types. The discovery file and RPC client moved to `internal/ipc`
  (standard library only; `internal/api` keeps aliases), and the host keeps its own copies
  of the request and capture fields the extension may set (`cmd/peregrine-host/wire.go`).
  `TestWireTypesMatchApp` fails if a field drifts from `engine.Request` or `api.Capture`.
  Stripped size: 10.5 MB to 7.1 MB. The rest is Go's `net/http` client. A hand-written
  HTTP client would save ~3 MB more, but it is security-sensitive code to maintain for a
  small gain.

Considered for later:
- **Remux-only ffmpeg built in CI.** We only use `-c copy` remuxing, and yt-dlp only
  merges streams. A minimal build (~5 MB) would cut the installer to ~25–30 MB and the
  install from ~265 MB to ~70 MB. It needs its own CI job and testing on the main video
  sites.
- **Drop ffprobe.** Our code never calls it, and yt-dlp falls back to ffmpeg without it.
  With LZMA it only adds a few MB to the download, but ~100 MB on disk. Decide together with
  the remux-only build.
- **Download the video tools on first use.** Rejected for now: it slows the first video
  download, and it is a new kind of network call (see CLAUDE.md).
- **UPX-packing the executables.** Rejected: antivirus tools often flag packed exes as
  false positives.
