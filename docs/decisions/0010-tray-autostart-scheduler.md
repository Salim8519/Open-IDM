# 0010 — Tray, start at sign-in and the scheduler's power actions

- Status: accepted
- Date: 2026-11-13

## Context
A download manager has to keep working with its window closed, start with the computer,
and tell the user when something finishes. Wails v2 has native notifications but no working
system tray (`menu.TrayMenu` exists but is not wired up). The scheduler (Q3) also needs an
"after the queue is done" action, which can sleep or shut down the computer.

## Decision
- **Tray:** `fyne.io/systray` (one new dependency, BSD-3; its Linux D-Bus dependency is
  already in our tree through Wails). On Windows and Linux it runs its own loop on a locked
  OS thread; on macOS it uses the external-loop mode and is created on the main queue,
  because Wails owns NSApp.
- **Close button:** Windows/Linux hide to the tray when "close to tray" is on and the tray is
  actually up (so the app can never become invisible). macOS uses Wails' `HideWindowOnClose`,
  i.e. the normal Mac behaviour: the Dock icon brings it back, Cmd+Q quits.
- **Notifications:** Wails' built-in API, only while the window is unfocused or hidden.
- **Start at sign-in:** our own small `desktop/autostart` package, per user and without admin
  rights: HKCU `Run` value (Windows), `~/Library/LaunchAgents/com.peregrine.app.plist`
  (macOS), `~/.config/autostart/peregrine.desktop` (Linux), each starting the app with
  `--hidden`. The entry is rewritten on every start while on, so it follows updates/moves.
- **Scheduler:** lives in the engine as a pure time-window check (tested with a fake clock),
  stored in `Settings.schedule`. The engine only emits `queueDone`; the desktop app performs
  quit/sleep/shutdown after a 60 s countdown the user can cancel.

## Alternatives considered
- `getlantern/systray` — older parent of the fyne fork, less maintained, no external-loop mode.
- Waiting for Wails v3 (has a tray) — still alpha; migrating the app is a larger change.
- Third-party autostart libraries — small enough to own; fewer dependencies, easier to audit.
- `SMAppService` login items on macOS — needs a signed app; LaunchAgents work unsigned.
- Power actions in the engine — would put OS control into the library; it stays UI-only.

## Consequences
- macOS tray and notifications can only be checked by hand (CI builds but cannot click);
  macOS notifications may stay unavailable until the app is signed.
- On Linux desktops without a StatusNotifierItem host (plain GNOME) there is no tray icon;
  closing then quits, and a `--hidden` start shows the window after 5 s.
- New threats T16 (power actions) and T17 (auto-start) in docs/SECURITY-MODEL.md.
