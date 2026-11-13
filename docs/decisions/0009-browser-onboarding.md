# 0009 — First-run "Connect your browser" guide; store install later

- Status: accepted
- Date: 2026-11-13

## Context
The extension is what makes Peregrine catch browser downloads, but most people will not find
`chrome://extensions` on their own. Until the extension is listed in the Chrome Web Store
and Edge Add-ons, Chromium only accepts it as an unpacked folder in Developer mode. Chrome on
Windows and macOS refuses to install extensions pushed from outside the store (registry or
preferences entries only work for store-hosted extensions), so the app cannot install it
silently, and should not try.

## Decision
- Ship the unpacked extension with the app (`extension/` next to `Peregrine.exe` in the
  installer and portable zip, `Contents/Resources/extension` in the macOS app).
- On first launch the app shows a three-step guide: open the extensions page (buttons for
  Chrome, Edge, Brave, plus the address to paste), turn on Developer mode, Load unpacked with
  a "Show folder" button. It is shown once; Settings → Browser & clipboard reopens it.
- The guide turns green by itself: when the extension talks to the native host (`ping`, and
  `getSettings` which runs on install and browser start) the host calls the app's
  `extensionHello`. The app stores only the time in `browser.json` and emits a `browser` event.
- When the extension is in the stores: the installer writes the store's external-extension
  entries (`Software\Google\Chrome\Extensions\<id>` with the store update URL, and the Edge
  equivalent), so the browser itself asks "New extension added — enable?". The guide then
  shows a store link instead of the unpacked steps.

## Alternatives considered
- Silent install through policy (`ExtensionInstallForcelist`) — that is an enterprise lockdown
  feature, shows "managed by your organization" and is the behaviour of adware. Rejected.
- No guide, only README instructions — most users never connect the extension.
- Polling for the extension from the app — no way to see browser state from outside; the host
  check-in is exact and needs no new permission.

## Consequences
- New API methods `browserStatus`, `extensionHello`, `openExtensionsPage`, `showExtensionFolder`.
  None are on the host's extension allowlist; `openExtensionsPage` only maps a fixed browser key
  to a fixed program and URL.
- Developer mode shows Chrome's "disable developer mode extensions" banner on some versions;
  publishing to the stores (Phase 3) removes it.
