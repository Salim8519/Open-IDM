# CLAUDE.md — rules for every AI session

Peregrine (repo: Salim8519/Open-IDM; the name is settled in ADR 0007)
is a free, open-source, fast, secure download manager for Windows and macOS, plus a
Chromium browser extension (ADR 0005). It is a vibe-coded project: these docs are the
project's memory.

## Read first, every session
1. `docs/SPEC.md` — what we are building (source of truth).
2. `docs/ROADMAP.md` — current phase and the next unchecked task.
3. `docs/decisions/` — why things are the way they are. Do not reverse a decision without
   writing a new ADR that supersedes it.

## Keep the memory up to date (mandatory)
- Finished a roadmap task → tick it in `docs/ROADMAP.md`.
- Made a non-trivial choice (library, format, protocol, architecture) → add
  `docs/decisions/NNNN-short-title.md` using `docs/decisions/TEMPLATE.md`.
- User-visible change → add a line to `docs/CHANGELOG.md` under `Unreleased`.
- Spec changed → update `docs/SPEC.md` in the same commit as the code.

## Stack (see ADRs 0001, 0002, 0004, 0005)
- Engine: Go (latest stable), standard library first. Module root: repo root.
- Desktop (Windows + macOS): Wails + React + TypeScript — the only UI codebase.
- Extension: Manifest V3 for Chromium browsers, talks to the app via Native Messaging.
- Android is deferred: keep `engine/` free of desktop-only code so it stays possible.
- State: SQLite in WAL mode (`modernc.org/sqlite`, pure Go, no CGO).

## Layout
```
engine/           Go library — no UI code, no globals, no os.Exit
cmd/peregrine/    CLI (get, add, list, serve)
cmd/peregrine-host/ Native Messaging host (bridge between extension and app)
internal/testsrv/ "evil" HTTP test server (drops, throttles, no-Range, lies about size)
desktop/          Wails app (React UI in desktop/frontend)
extension/        Chromium MV3 extension
docs/             spec, roadmap, decisions, changelog
```

## Coding rules
- `engine/` must never import UI, Wails, or platform UI packages.
- All long operations take a `context.Context` and stop cleanly when it is cancelled.
- No panics across package boundaries; return wrapped errors (`fmt.Errorf("...: %w", err)`).
- Never write downloaded bytes to the final filename until complete: use `<name>.peregrine.part`,
  then verify and rename atomically.
- Persist progress often enough that a crash loses at most a few seconds of work.
- No telemetry, no analytics, no network calls except the user's downloads and the
  signed update check (which the user can disable).
- Keep dependencies few; every new dependency needs a one-line reason in the PR/commit.

## Testing rules (non-negotiable)
- Every engine feature has a test against `internal/testsrv`.
- Always run `go test -race ./...` — data races are the main Go risk.
- `go vet ./...` and `staticcheck ./...` must be clean.
- A bug fix starts with a failing test that reproduces it.

## Security rules
- Validate and sanitize every filename from servers/URLs (no path traversal, no reserved
  Windows names, no control chars).
- Native Messaging host accepts only our extension IDs.
- Windows: set Mark-of-the-Web (Zone.Identifier) on completed files.
- Never log full URLs with credentials or tokens; redact query strings in logs.
- Updates must be signature-verified before install.

## Security (see docs/SECURITY-MODEL.md, ADR 0008)
- Any new input, network listener, file write, API method or dependency: check it against
  the threat model and update `docs/SECURITY-MODEL.md` in the same change.
- New API methods are NOT exposed to the browser extension unless added to the host allowlist.
- Run `govulncheck ./...` (root and `desktop/`) before a release.

## Never
- Never commit secrets, keystores, or signing keys.
- Never add ads, bundled offers, or tracking.
- Never weaken a test to make it pass.
