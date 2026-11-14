# Security model

What Peregrine defends against, how, and what it deliberately does not. Update this file
whenever a change adds a new input, network listener, file write or dependency.

## Assets
1. The user's computer (no code execution by attackers).
2. The user's files outside the download folder.
3. Browser secrets passed to Peregrine (cookies, auth headers, signed URLs).
4. The integrity of Peregrine itself (releases, updates, dependencies).

## Trust boundaries and threats

| # | Boundary | Threat | Mitigation | Where | Test |
|---|---|---|---|---|---|
| T1 | Remote server → engine | Path traversal / overwrite via `Content-Disposition` or URL name | `SanitizeFilename`; files only inside the chosen folder; unique names, never overwrite | `engine/names.go` | `TestSanitizeFilename`, `FuzzSanitizeFilename`, `TestDuplicateNamesGetUniquePaths` |
| T2 | Remote server → engine | Spoofed extension (`evil‮txt.exe`), reserved names (`CON`) | Bidi controls stripped, reserved names prefixed | `engine/names.go` | `TestSanitizeFilename` |
| T3 | Remote server → engine | Malicious content run silently | Mark-of-the-Web / quarantine attribute; never auto-open | `engine/zone_*.go` | manual |
| T4 | Remote server → engine | Swapped file mid-download (corruption, injection) | ETag / Last-Modified / size check + `If-Range`; optional checksum | `engine/task.go` | `TestRemoteChangedDetected`, `TestChecksumVerification` |
| T5 | Remote server → engine | Cookie leak through redirect to another host | `Cookie`/`Authorization` dropped on cross-host redirect | `engine/http.go` | — |
| T6 | Remote server → engine | Resource exhaustion (endless retries, huge sizes) | Retry cap with backoff; 403/410 not retried; disk errors stop the download | `engine/task.go` | `TestExpiredLinkIsNotRetriedForever` |
| T7 | Any website → local API | Drive-by requests from a web page (CSRF) or DNS rebinding | 127.0.0.1 only; bearer token; Origin must equal own origin; Host must be loopback | `internal/api/server.go` | `TestAuthAndHostGuard` |
| T8 | Local API → clients | Secrets leaked to UI/extension | Headers never serialized to clients | `engine/manager.go` | `TestHeadersNeverReturned` |
| T9 | Browser → native host | Another extension or page driving Peregrine | Extension-ID allowlist; method allowlist; scheme + header allowlist; `dir` dropped; params are decoded into the host's own narrow types (`wire.go`), so fields outside them never reach the app | `cmd/peregrine-host`, `internal/ipc` | host unit tests, `TestWireTypesMatchApp`, extension e2e |
| T10 | Native host → browser | Settings disclosure (folders, proxy credentials) | Only `minExtSize`, `autoStartExt`, `language` returned | `cmd/peregrine-host` | host unit tests |
| T15 | UI / local API → OS | Launching arbitrary programs through "open extensions page" | Browser key checked against a fixed table; program and URL come from code, never from the caller; `extensionHello` only stores a timestamp and is not on the host allowlist | `internal/api/browser.go`, `desktop/browser_*.go` | `TestOpenExtensionsPageAllowlist`, `TestMethodAllowlist` |
| T16 | Local API → OS | Sleeping or shutting down the computer on someone else's behalf | Only the scheduler's when-done action can do it: needs the schedule on, the queue empty after a completion, and a 60 s countdown the user can cancel; fixed commands, no caller input; `updateSettings` is not on the extension allowlist and the host never returns `schedule` | `desktop/lifecycle.go`, `desktop/power_*.go` | `TestScheduleValidation`, `TestMethodAllowlist` |
| T17 | App → OS start-up | Persistent auto-start abused to run something else | Per-user entry only (HKCU Run / LaunchAgent / XDG autostart), written only when the user turns on "Start when I sign in"; path is the app's own executable, name and flag are constants; names with path characters refused | `desktop/autostart` | `TestSetAndRemove`, `TestRejectsBadNames` |
| T18 | FTP server → engine | Password sent in clear, FTP bounce, hangs | `AUTH TLS` when offered; a failed handshake never falls back to plain FTP when a password is set; data connection only to the control connection's IP (PASV address ignored); deadlines on every reply/read; passwords scrubbed from errors and logs | `engine/proto/ftp` | `engine/proto/ftp` tests (ftpserverlib) |
| T19 | Stream server → engine | Endless live stream, malicious playlist, command injection via ffmpeg | Live recording ends on pause or a stalled playlist; DRM refused; segment URLs resolved against the playlist; ffmpeg run with `exec.CommandContext`, separate arguments, no shell; scratch files only in `<name>.peregrine.tmp` beside the download | `engine/proto/stream` | `engine/proto/stream` tests |
| T20 | Web page → yt-dlp sidecar | Option injection through the URL, user config running `--exec`, cookie leak | URL passed last after `--`; `--no-config`; fixed argument list; cookies written to a 0600 file in the work folder and deleted when yt-dlp exits; errors stripped of query strings and credentials; yt-dlp/ffmpeg pinned with SHA-256 checked in the release workflow | `engine/proto/video`, `.github/workflows/release.yml` | `engine/proto/video` tests (fake yt-dlp) |
| T21 | BitTorrent peers → engine | Path traversal from file names inside a torrent; huge `.torrent`; open port | Every path element sanitized and kept inside the part folder (`SafeRelPath`); `.torrent` capped at 20 MB (HTTP and local); the listening port (TCP/UDP 46881) opens only when a torrent starts and closes with the app; no seeding after completion, upload capped | `engine/proto/torrent` | `TestMaliciousPaths`, `TestSafeRelPath`, `TestBadSources` |
| T22 | OS file open / drop → app | A crafted argument or drop opening something other than a torrent | Only existing regular files ending in `.torrent` are accepted, and only parsed as torrent metadata; `file://` URLs with a host (network shares) are refused; the native host never forwards `file:` URLs (scheme allowlist) | `desktop/app.go`, `engine/proto/torrent` | `TestURLArgTorrentFile`, `TestLocalTorrentFile`, `TestMatch` |
| T11 | Server text → UI | XSS through file names or errors | React text rendering; strict CSP (no remote/inline scripts, no eval) | `desktop/frontend/index.html` | — |
| T12 | Logs | Tokens in URLs written to disk | Query strings and credentials redacted | `engine/manager.go` (`redact`), host logging | `TestRedact`, extension e2e log test |
| T13 | Supply chain | Vulnerable or malicious dependency | Lockfiles; govulncheck, npm audit, CodeQL on every push + weekly; Dependabot PRs gated by CI | `.github/workflows/security.yml` | CI |
| T14 | Release | Tampered binaries | Built only in GitHub Actions from tags; SHA-256 sums; signed provenance attestation | `.github/workflows/release.yml` | `gh attestation verify` |
| T23 | Website → visitor | Download links sent somewhere else; injected script; visitor tracking | `/download/*` only redirects to assets of `Salim8519/Peregrine` releases on github.com (fixed repo, fixed file-name list, no user-supplied URL); strict CSP, HSTS, `X-Frame-Options: DENY`; no third-party requests or analytics; DNS-only records at Cloudflare so TLS ends at Vercel | `website/`, ADR 0016 | manual check after deploy |

| T24 | GitHub release API → app | Fake "update" sending the user to a malicious page; tracking through the check | Only `tag_name` read (1 MiB cap); strict `vX.Y.Z` pattern; the release-page URL is built from the version and points at the fixed repo, nothing from the response is opened; drafts/prereleases ignored; fixed User-Agent, no cookies/IDs; user can turn it off; nothing is downloaded or installed; `checkUpdate`/`updateStatus`/`openUpdatePage` not on the host allowlist | `internal/update`, `internal/api/update.go`, ADR 0020 | `internal/update` tests, `TestCheckUpdateFlow`, `TestMethodAllowlist`, `e2e/update.spec.ts` |

## Out of scope (documented, not defended)
- **Malware running as the same user.** It can already read the token file, the database
  and the browser's own cookies; no local app can defend against that.
- **Unsigned binaries.** Until Windows code signing (SignPath) and Apple notarization are set
  up, the OS shows a warning on first launch. Provenance attestations are the interim proof.
- **What you choose to download.** Peregrine does not scan files for viruses; Windows
  Defender / macOS XProtect do, helped by the Mark-of-the-Web / quarantine flag.

## Rules for contributors (and AI assistants)
- Never add telemetry, remote config, eval, dynamic code loading or a plugin system.
- Any new API method must be reviewed against T7–T10 and, if the extension needs it, added
  to the host allowlist explicitly.
- Any new file write must go through the sanitizer and stay inside the download folder.
- An auto-updater (roadmap) must verify signatures before installing — ADR required first.
