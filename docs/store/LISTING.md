# Store listing: Chrome Web Store and Edge Add-ons

Everything needed to publish the extension. Copy each field into the store form.
Images are in [`images/`](images/) (regenerate with `node tests/store/shots.mjs` in `extension/`).

**Upload file:** `Peregrine-<version>-chromium-store-upload.zip` from the release (same files
as the normal extension zip, without the manifest `key`; the stores reject `key` and assign
their own extension ID). Locally: `bash scripts/stage-extension.sh <dir> --store`, then zip
the folder's contents.

## Listing

| Field | Value |
|---|---|
| Name | Peregrine Download Manager *(from the manifest)* |
| Summary (≤132) | Send browser downloads and media to the Peregrine download manager on your computer. *(from the manifest)* |
| Category | Chrome: Productivity → Tools. Edge: Productivity |
| Languages | English, Arabic |
| Homepage | https://github.com/Salim8519/Peregrine |
| Support | https://github.com/Salim8519/Peregrine/issues |
| Privacy policy | https://github.com/Salim8519/Peregrine/blob/main/extension/PRIVACY.md |
| Icon | `extension/icons/icon-128.png` |
| Screenshots (1280×800) | `images/screenshot-1-app.png`, `images/screenshot-2-popup.png`, `images/screenshot-3-options.png` |
| Small promo tile (440×280) | `images/promo-small-440x280.png` |
| Marquee (1400×560, optional) | `images/promo-marquee-1400x560.png` |

### Description

```
Peregrine is a free, open-source download manager for Windows and macOS — an alternative to IDM and FDM. This extension connects your browser to the Peregrine app, so your downloads get its speed and reliability.

What it does
• Takes over downloads you start in the browser and sends them to Peregrine, which splits each file into up to 32 connections and keeps them busy until the end.
• Keeps your login: the cookies for the download are passed along, so files behind a sign-in still work.
• Lists the video and audio on the current page in the toolbar menu; one click sends a file to Peregrine.
• Right-click any link: "Download with Peregrine", or "Download all links with Peregrine" for a whole page.
• You choose what is taken over: file types, minimum size, sites to skip, and "Skip next download" to hand one file back to the browser.
• If the app is not installed or not running, the browser downloads the file itself — nothing is lost.

Requires the Peregrine desktop app (free): https://github.com/Salim8519/Peregrine/releases

Privacy
No analytics, no tracking, no ads, no remote code. Download details go only to the Peregrine app on your own computer, through the browser's native messaging channel. Nothing is sent to the developers or anyone else.

Open source (GPL-3.0): https://github.com/Salim8519/Peregrine
```

## Privacy practices tab

**Single purpose**
```
Hand the downloads a user starts in the browser (and media files they pick from the page) to the Peregrine download manager app installed on the same computer.
```

**Permission justifications**

| Permission | Justification |
|---|---|
| `downloads` | To see when the user starts a download, cancel the browser's copy and pass the URL to the Peregrine app instead (or let the browser continue when the app is unavailable). |
| `nativeMessaging` | The only channel to the Peregrine desktop app on the same computer (host `com.peregrine.host`). |
| `cookies` | Read the cookies for the download URL only, so the app can download files that need the user's sign-in. They go only to the local app. |
| `contextMenus` | Adds "Download with Peregrine" and "Download all links with Peregrine" to the right-click menu. |
| `storage` | Saves the user's settings (on/off, file types, minimum size, excluded sites) and the per-tab list of media files. |
| `webRequest` | Reads response headers (type and size) of requests the browser already makes, to list video and audio files in the popup. It does not block or change requests. |
| `scripting` | When the user picks "Download all links with Peregrine", collects the link addresses on that page. Runs only on that user action. |
| Host permission `<all_urls>` | Downloads and media can come from any website; cookies and response headers are needed for whatever site the download is on. |

**Remote code:** No, I am not using remote code. (All scripts are in the package; the CSP forbids anything else.)

**Data usage** — declare honestly; the extension handles these, but only on the user's device:
- ☑ Authentication information *(cookies for the download URL, passed to the local app)*
- ☑ Web history *(the URLs of downloads the user sends to the app)*
- ☑ Website content *(link addresses when the user chooses "Download all links")*

And tick all three certifications: not sold to third parties; not used or transferred for
purposes unrelated to the single purpose; not used to determine creditworthiness.

**Notes for the reviewer** (Chrome: "Test instructions"; Edge: "Notes for certification")
```
The extension talks to the Peregrine desktop app through native messaging. To test the full flow, install the app (Windows setup.exe or macOS DMG) from https://github.com/Salim8519/Peregrine/releases, start it, then click a download link. Without the app the extension stays out of the way: the popup shows "App not running" and the browser downloads files normally. No account or login is needed.
```

## Publishing steps (maintainer)

1. **Chrome Web Store** — https://chrome.google.com/webstore/devconsole : sign in, pay the
   one-time $5 registration fee, turn on 2-step verification. *Add new item* → upload the
   store zip → fill *Store listing* and *Privacy practices* from this file → *Submit for review*
   (usually a few days; `<all_urls>` and `cookies` can mean a longer review).
2. **Edge Add-ons** — https://partner.microsoft.com/dashboard/microsoftedge : free
   registration. *Create new extension* → upload the same zip → same text → *Publish*.
3. Send the two **extension IDs** the stores show (32 letters a–p). Then the next release:
   - adds both IDs to the native host's allowlist (`cmd/peregrine-host/origin.go`) —
     until then the store-installed extension cannot reach the app;
   - makes the installer register the store extensions, so Chrome and Edge show their own
     "New extension added — enable?" prompt (ADR 0009);
   - switches the app's "Connect your browser" guide to store links.
