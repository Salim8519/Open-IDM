# 0008 — Security baseline and supply-chain checks

- Status: accepted
- Date: 2026-11-13

## Context
The maintainer asked for the app to be "secured and anti-backdoor". A download manager is a
high-value target: it handles browser cookies, writes files from untrusted servers, runs a
local API and is installed on many machines. govulncheck found that building with Go 1.26.0
pulled in 30 known standard-library vulnerabilities fixed in later patch releases.

## Decision
- Build with the latest patched Go (1.27.4 at time of writing) and keep it current.
- CI runs govulncheck, npm audit and CodeQL (security-extended) on every push and weekly;
  Dependabot opens update PRs that must pass all checks.
- Releases are built only in GitHub Actions from tags and carry SHA-256 sums plus a signed
  build provenance attestation (`actions/attest-build-provenance`).
- The desktop UI ships a strict Content Security Policy.
- No telemetry, remote config, eval/dynamic code, plugins, and no auto-updater until a
  signature-verifying design is accepted in its own ADR.
- `docs/SECURITY-MODEL.md` is the threat model; every new input, listener, file write or
  dependency must be checked against it, with a test where possible.

## Consequences
- A security regression in a dependency shows up as a failing check within a week.
- Users can verify a release was not built or modified outside CI.
- Code signing (SignPath, Apple notarization) is still needed to remove OS warnings.
