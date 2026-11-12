# 0005 — Desktop-first: Windows + macOS + Chromium extension; Android deferred

- Status: accepted
- Date: 2026-11-12
- Supersedes: 0003 (Android via gomobile)

## Context
Supporting phones now spreads a small, vibe-coded project too thin. Download managers are
used mostly on desktop. The maintainer also asked whether browser downloads can be captured
without any extension.

## Decision
- **Official v1 platforms:** Windows 10+ and macOS 12+.
- **Browser capture:** one Manifest V3 extension for Chromium browsers (Chrome, Edge, Brave,
  Opera, Vivaldi), talking to the app over Native Messaging. **Required** for click-to-capture.
- **Fallback without the extension:** opt-in clipboard monitor (copy a link → add dialog).
- **Linux:** builds from the same code, but unofficial (not tested or supported in v1).
- **Later:** Firefox extension (same code, small changes), Safari (needs an Xcode wrapper app).
- **Maybe later:** Android. Keep `engine/` free of desktop-only code so it stays possible.

## Alternatives considered (for capture without an extension)
- Watch the Downloads folder — the browser has already started, the real URL and cookies are
  not available, so authenticated links fail. Unreliable.
- Local proxy — sees only hostnames for HTTPS; seeing URLs needs a man-in-the-middle root
  certificate, which is a serious security hole. Rejected.
- Injecting code into browser processes — blocked by browsers, flagged by antivirus. Rejected.

## Consequences
- One UI codebase: React in Wails for both desktop OSes. No Kotlin, no second UI.
- The extension is small but must be published to the Chrome Web Store (one-time $5) and
  Edge Add-ons (free).
- The installer must register the Native Messaging host for each Chromium browser.
- Safari users get the clipboard fallback until a Safari extension exists.
