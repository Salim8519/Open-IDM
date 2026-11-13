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
| T9 | Browser → native host | Another extension or page driving Peregrine | Extension-ID allowlist; method allowlist; scheme + header allowlist; `dir` dropped | `cmd/peregrine-host` | host unit tests, extension e2e |
| T10 | Native host → browser | Settings disclosure (folders, proxy credentials) | Only `minExtSize`, `autoStartExt`, `language` returned | `cmd/peregrine-host` | host unit tests |
| T15 | UI / local API → OS | Launching arbitrary programs through "open extensions page" | Browser key checked against a fixed table; program and URL come from code, never from the caller; `extensionHello` only stores a timestamp and is not on the host allowlist | `internal/api/browser.go`, `desktop/browser_*.go` | `TestOpenExtensionsPageAllowlist`, `TestMethodAllowlist` |
| T11 | Server text → UI | XSS through file names or errors | React text rendering; strict CSP (no remote/inline scripts, no eval) | `desktop/frontend/index.html` | — |
| T12 | Logs | Tokens in URLs written to disk | Query strings and credentials redacted | `engine/manager.go` (`redact`), host logging | `TestRedact`, extension e2e log test |
| T13 | Supply chain | Vulnerable or malicious dependency | Lockfiles; govulncheck, npm audit, CodeQL on every push + weekly; Dependabot PRs gated by CI | `.github/workflows/security.yml` | CI |
| T14 | Release | Tampered binaries | Built only in GitHub Actions from tags; SHA-256 sums; signed provenance attestation | `.github/workflows/release.yml` | `gh attestation verify` |

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
