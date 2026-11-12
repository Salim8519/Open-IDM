# 0002 — Wails + React + TypeScript for the desktop UI

- Status: accepted
- Date: 2026-11-12

## Context
One modern-looking UI for Windows, macOS and Linux, calling the Go engine directly, with a
small install size.

## Decision
Use Wails (system webview) with a React + TypeScript frontend.

## Alternatives considered
- Fyne — pure Go and also runs on Android, but looks less polished; UI quality is a key
  differentiator against IDM/FDM.
- Electron — ~150 MB installs, high RAM.
- Tauri — needs a Rust backend; conflicts with ADR 0001.

## Consequences
- Small installers, native webview (WebView2 on Windows, WebKit on macOS/Linux).
- Linux depends on WebKitGTK being present; document it in install notes.
- Android needs a separate UI (ADR 0003).
