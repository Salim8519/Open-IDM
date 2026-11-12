# 0003 — Android via gomobile + Kotlin/Compose

- Status: superseded by 0005 (Android deferred; kept for when it comes back)
- Date: 2026-11-12

## Context
Android must reuse the Go engine and survive Android's background restrictions.

## Decision
- `gomobile bind` the `engine/mobile` package into `engine.aar` (arm64, arm, x86_64; API 24+).
- Native UI in Kotlin + Jetpack Compose, built with Gradle.
- Background work: user-initiated data transfer jobs on Android 14+, foreground service with
  a notification on older versions.
- Release pipeline in GitHub Actions: per-ABI signed APKs + AAB; keystore in GitHub Secrets.

## Alternatives considered
- Fyne mobile — single codebase but weaker UI and background-service integration.
- Wails mobile — not mature enough to depend on.

## Consequences
- `engine/mobile` must expose only gomobile-friendly types (strings, numbers, bytes,
  simple interfaces for callbacks).
- Android UI is a second codebase; keep logic in the engine, UI thin.
- Losing the signing keystore means no updates for that app ID — back it up offline.
