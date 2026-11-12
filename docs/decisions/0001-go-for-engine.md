# 0001 — Go for the download engine

- Status: accepted
- Date: 2026-11-12

## Context
We need a fast, memory-safe, cross-platform engine that AI assistants can write reliably,
reusable on desktop and Android.

## Decision
Write the engine in Go, standard library first (`net/http`, goroutines, `context`).

## Alternatives considered
- Rust (tokio/reqwest) — slightly smaller binaries and compile-time race safety, but slower
  compiles and harder to vibe-code. Was the first proposal; replaced at user's request.
- C++ — not memory-safe; hard to keep secure.
- Node/Electron — too heavy for a "fast" app.

## Consequences
- Fast iteration, strong networking stdlib, easy cross-compiling.
- Data races are possible → `go test -race` is mandatory in CI.
- Pure-Go SQLite (`modernc.org/sqlite`) keeps CGO out of the engine for easy cross builds.
- Mature torrent library available (`anacrolix/torrent`).
