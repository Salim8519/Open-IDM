# 0007 — Product name: Peregrine

- Status: accepted
- Date: 2026-11-13

## Context
"Open-IDM" was a working name; "IDM" is a trademark of Tonec Inc. The maintainer asked
Claude to choose. "Swoop" was considered but at least three download managers on GitHub
already use it.

## Decision
The product is **Peregrine** — after the peregrine falcon, the fastest animal on Earth, which
hunts by diving. The logo is a falcon in a head-first dive drawn as a download arrow over a
tray (`assets/logo/peregrine.svg`). Brand colors: #2F6BFF → #22D3EE gradient, dark #0B1020.

- Binaries: `Peregrine` (desktop app), `peregrine` (CLI), `peregrine-host` (native messaging).
- Native messaging host name: `com.peregrine.host`. Data folder: `<config>/Peregrine`.
- The GitHub repository keeps its URL (`Salim8519/Open-IDM`) for now; renaming it later is
  safe because GitHub redirects old URLs, but the Go module path would need updating.

## Consequences
- Search found no existing download manager named Peregrine (2026-11-13). A formal trademark
  search was not done.
