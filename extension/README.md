# Peregrine browser extension

Manifest V3 extension for Chromium browsers (Chrome, Edge, Brave, Opera, Vivaldi). It hands
browser downloads and detected media to the Peregrine desktop app on the same computer.

Plain JavaScript ES modules. No bundler, no dependencies at runtime, no remote code.

## Load it unpacked (development)

1. Install the native messaging host (once per machine, see below).
2. Open `chrome://extensions` (or `edge://extensions`, `brave://extensions`, ...).
3. Turn on **Developer mode**, click **Load unpacked**, and pick this `extension/` folder.

The `"key"` in `manifest.json` pins the unpacked extension ID to
**`bcnjpgddmjlffmcfgnfjaadkdjphdhfd`**, whatever folder you load it from. The host only accepts
that ID. The private key was thrown away: an unpacked ID depends only on the public key. Store
builds get their own IDs from the Chrome Web Store and Edge Add-ons. Add those IDs to
`builtinExtensionIDs` in `cmd/peregrine-host/origin.go`. Until then, put them in
`<config>/Peregrine/allowed-extensions.json` (`["id1", "id2"]`) and run `install` again.

## How it talks to the app

```
extension  --chrome.runtime.sendNativeMessage-->  peregrine-host (stdio, 4-byte length + JSON)
           <--------------- reply ---------------  |  POST http://127.0.0.1:<port>/rpc + bearer token
                                                   v  (port/token from <config>/Peregrine/ipc.json)
                                                Peregrine app
```

- Message: `{"id": n, "method": "...", "params": {...}}`. Reply: `{"id": n, "result": ...}` or
  `{"id": n, "error": {"code", "message"}}`.
- The host only allows these methods: `ping`, `version`, `capture`, `add`, `getSettings`,
  `list`. It drops the download folder and any header except
  Cookie/Referer/User-Agent/Accept-Language. It only accepts http(s)/ftp URLs. `getSettings` and
  `list` replies are trimmed (no folders, proxy settings or URLs).
- `capture`, `add` and `version` start the app if it is not running. The host waits up to
  15 s for it. `ping`, `getSettings` and `list` never start it.
- If the host or app is not available (`host_unavailable` / `app_unavailable`), the browser
  keeps the download.

### Native messaging host

```sh
go build -o <install-dir>/peregrine-host(.exe) ./cmd/peregrine-host
<install-dir>/peregrine-host install            # all browsers; or --browsers chrome,edge
<install-dir>/peregrine-host uninstall
```

Put the host next to the app (`Peregrine.exe` on Windows, inside
`Peregrine.app/Contents/MacOS/` on macOS) so it can start the app. `install` writes
`com.peregrine.host.json` and registers it:

- Windows: `HKCU\Software\{Google\Chrome, Microsoft\Edge, BraveSoftware\Brave-Browser, Chromium, Vivaldi}\NativeMessagingHosts\com.peregrine.host`.
  The manifest goes in `%APPDATA%\Peregrine`. Opera reads Chrome's key.
- macOS: `~/Library/Application Support/<browser>/NativeMessagingHosts/`.
- Linux (best effort): `~/.config/<browser>/NativeMessagingHosts/`.

The host logs to `<config>/Peregrine/peregrine-host.log`, with query strings redacted.

## What it does

- **Download capture** (`downloads.onCreated`): the logic is in `src/decide.js`. A download
  is captured when its file type is in the list, or when its size is at least the minimum
  size (the app's `minExtSize`, default 1 MB, or your own value in Settings). Unknown size
  plus unknown type stays in the browser. `blob:`, `data:`, `file:` and `filesystem:` URLs
  and excluded sites are skipped.
  - Capture flow: pause the browser download, send `capture` (URL, cookies, referrer,
    user agent, MIME type, size). If the app accepts it, cancel and erase the browser
    download. Otherwise resume it, or start it again with `downloads.download` and remember
    the URL so it is not captured a second time.
- **Skip next download**: a switch in the popup. Extensions cannot see modifier keys on a
  download click, so there is no "hold Alt to bypass".
- **Context menus**:
  - "Download with Peregrine" works on links, images, video and audio.
  - "Download all links with Peregrine" collects the page's links, removes duplicates and
    keeps only listed file types (100 at most). It asks for confirmation, then sends `add`
    for each link.
- **Media detection**: a non-blocking `webRequest.onHeadersReceived` listener records the
  `video/*` and `audio/*` responses for each tab, plus `.mp4`, `.webm` and `.mp3` files.
  `.m3u8`/`.mpd` are flagged as streams, and segments and tiny files are ignored. The list
  is stored in `chrome.storage.session` and cleared on navigation and when the tab closes.
  The badge shows the count. Streams appear in the popup but are disabled ("Stream support
  coming soon").
- **i18n**: English and Arabic (`_locales`), right-to-left when the UI language is Arabic.

## Permissions and why

| Permission | Why |
|---|---|
| `downloads` | See new browser downloads, cancel/erase the ones Peregrine takes, restart them if the app is unavailable. |
| `nativeMessaging` | The only channel to the Peregrine app (`com.peregrine.host`). |
| `contextMenus` | "Download with Peregrine" / "Download all links with Peregrine". |
| `cookies` | Send the download's cookies to the app so logged-in downloads work. |
| `storage` | Your settings (local) and per-tab media lists (session, cleared on browser exit). |
| `webRequest` | Read response headers (type/size) to detect media. Read-only, non-blocking. |
| `scripting` | Read the links on the current page for "Download all links", and show its confirm dialog. Runs only when you click that menu item. |
| `<all_urls>` | Cookies, media detection and link collection must work on any site you download from. |

## Tests

```sh
cd extension
npm install
npm test            # unit tests (node --test): decide, cookies, media, links, manifest/locales
npm run test:e2e    # Playwright: real extension + real host + fake app
```

The e2e test builds `peregrine-host` and `cmd/peregrine-host/internal/fakeapp` with Go. It
uses a temporary `PEREGRINE_HOME` and registers the host for the test browser: on Windows
it writes the Chrome and Chromium HKCU keys, backs up any existing values and restores them
afterwards. It then loads the extension into Playwright's Chromium in new headless mode
(`HEADED=1` to watch). The test checks five things:

1. A clicked `.zip` reaches the app with the right URL, Referer, Cookie and User-Agent, and
   the browser keeps no copy.
2. With the app stopped, the browser download completes (fallback).
3. Media detection finds direct files and flags streams.
4. The popup and options pages work.
5. The host log contains no secrets.
