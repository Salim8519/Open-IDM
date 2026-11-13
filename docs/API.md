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
| `updateSettings` | partial `Settings` | `Settings` |
| `capture` | `Capture` (Request + `source`, `pageUrl`, `size`, `mimeType`) | `{queued, id?}` or `{prompted: true}` |
| `openFile` / `showInFolder` | `{id}` | `true` (desktop only, else `unsupported`) |
| `pickFolder` | `{current}` | `{dir}` (desktop only) |
| `readClipboard` | – | `{text}` (desktop only) |
| `browserStatus` | – | `{connected, lastSeen?, extensionDir?, extensionId}` |
| `extensionHello` | – | same as `browserStatus`; called by the native host when the extension checks in (not callable by the extension) |
| `openExtensionsPage` | `{browser: "chrome"\|"edge"\|"brave"}` | `true` (desktop only) |
| `showExtensionFolder` | – | `true`; opens the bundled extension folder (desktop only) |

`Request`: `{url, dir?, filename?, connections?, headers?, checksum?, speedLimit?, paused?}` —
`headers` may carry `Cookie`, `Referer`, `User-Agent` from the browser. Headers are never
returned to clients.

`Download`: see `engine/types.go` (`id, url, dir, filename, size, downloaded, status, error,
resumable, connections, active, speed, eta, sha256, category, segments, createdAt, completedAt, order`).
Status: `queued | downloading | paused | error | completed`.

## Events
JSON objects: `{type, download?, downloads?, id?, settings?, capture?, browser?}`
- `added`, `updated` → `download`
- `progress` → `downloads` (batch of active downloads, every 500 ms)
- `removed` → `id`
- `settings` → `settings`
- `capture` → `capture` (extension offered a download; UI shows the add dialog)
- `browser` → `browser` (the extension checked in; closes the "Connect your browser" guide)
