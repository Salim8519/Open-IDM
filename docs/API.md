# Peregrine RPC API

One method-based API (`internal/api.Service.Call`) used by every client:

| Client | Transport |
|---|---|
| Desktop UI (Wails) | Go binding `App.Call(method, paramsJSON) -> resultJSON`; events via Wails runtime event `peregrine:event` |
| Browser-mode UI, e2e tests | `POST http://127.0.0.1:<port>/rpc` + SSE `GET /events?token=` |
| Native messaging host, CLI | same HTTP API; port + token from `<config>/Peregrine/ipc.json` |

`<config>` = `%APPDATA%` (Windows), `~/Library/Application Support` (macOS). Override with `PEREGRINE_HOME`.

## HTTP
- Bind: `127.0.0.1` only. Host header must be `127.0.0.1`/`localhost` (DNS-rebinding guard).
  Cross-origin requests are rejected (Origin must equal the server origin).
- Auth: `Authorization: Bearer <token>` or `?token=<token>`.
- Request: `{"method": "add", "params": {...}}`
- Response: `{"result": ...}` or `{"error": {"code": "not_found|bad_params|unknown_method|unsupported|error", "message": "..."}}`

## Methods
| Method | Params | Result |
|---|---|---|
| `version` | – | `{version}` |
| `list` | – | `Download[]` (queue order) |
| `get` | `{id}` | `Download` |
| `add` | `Request` | `Download` |
| `probe` | `{url, headers?}` | `{url, filename, size, resumable, category}` |
| `pause` / `resume` / `restart` | `{id}` | `true` |
| `remove` | `{id, deleteFile?}` | `true` |
| `updateURL` | `{id, url, headers?}` | `true` (refresh an expired link, keeps progress) |
| `setSpeedLimit` | `{id, bps}` | `true` (0 = unlimited) |
| `move` | `{id, delta}` | `true` (-1 up, +1 down) |
| `pauseAll` / `resumeAll` / `clearCompleted` | – | `true` |
| `getSettings` | – | `Settings` |
| `scheduleStatus` | – | `{enabled, open, nextStart?, nextStop?}` (RFC 3339 times; `open` = downloads may run now) |
| `updateSettings` | partial `Settings` | `Settings` |
| `capture` | `Capture` (Request + `source`, `pageUrl`, `size`, `mimeType`) | `{queued, id?}` or `{prompted: true}` |
| `openFile` / `showInFolder` | `{id}` | `true` (desktop only, else `unsupported`) |
| `pickFolder` | `{current}` | `{dir}` (desktop only) |
| `readClipboard` | – | `{text}` (desktop only) |
| `browserStatus` | – | `{connected, lastSeen?, extensionDir?, extensionId}` |
| `extensionHello` | – | same as `browserStatus`; called by the native host when the extension checks in (not callable by the extension) |
| `openExtensionsPage` | `{browser: "chrome"\|"edge"\|"brave"}` | `true` (desktop only) |
| `showExtensionFolder` | – | `true`; opens the bundled extension folder (desktop only) |

`Request`: `{url, kind?, dir?, filename?, connections?, headers?, checksum?, speedLimit?, paused?}` —
`headers` may carry `Cookie`, `Referer`, `User-Agent` from the browser. Headers are never
returned to clients. `kind` forces a downloader (`http`, `ftp`, `hls`, `dash`, `video`,
`torrent`); without it the URL decides (magnet / `.torrent` → torrent, `.m3u8` → hls,
`.mpd` → dash, known video hosts → video, `ftp(s)://` → ftp, else http). Local `.torrent`
files are `file:///…/x.torrent` URLs; the native host never forwards `file:` URLs.

`Download`: see `engine/types.go` (`id, url, dir, filename, size, downloaded, status, error,
resumable, connections, active, speed, eta, sha256, category, segments, createdAt, completedAt, order,
kind, isDir`). `isDir` is true when the result is a folder (multi-file torrents). A URL's
password is never returned, nor a downloader's internal resume state.
Status: `queued | downloading | paused | error | completed`.

## Events
JSON objects: `{type, download?, downloads?, id?, settings?, schedule?, capture?, browser?}`
- `added`, `updated` → `download`
- `progress` → `downloads` (batch of active downloads, every 500 ms)
- `removed` → `id`
- `settings` → `settings`
- `schedule` → `schedule` (the schedule window opened or closed, or the schedule changed)
- `queueDone` (the queue ran empty after at least one download completed; the desktop app
  runs the schedule's `whenDone` action after a 60 s cancellable countdown)

`Settings.schedule`: `{enabled, start: "HH:MM", stop: "HH:MM" | "", days: [0-6] (0 = Sunday,
empty = every day), whenDone: "none" | "quit" | "sleep" | "shutdown"}`. Outside the window
queued downloads wait; at the stop time running downloads go back to the queue with their
progress. Without a stop time the window lasts 24 hours. Desktop-only settings stored by the
engine: `closeToTray`, `launchAtLogin`, `notify`.

Desktop-only (Wails, not RPC): bindings `App.SetFocused(bool)` and `App.CancelPowerAction()`;
event `peregrine:power` `{action, seconds}` (a when-done countdown started; `action: ""` =
cancelled or done).
- `capture` → `capture` (extension offered a download; UI shows the add dialog)
- `browser` → `browser` (the extension checked in; closes the "Connect your browser" guide)
