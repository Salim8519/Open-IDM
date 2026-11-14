# Security

## Reporting a vulnerability

Please **do not open a public issue** for security problems. Use GitHub's private
[security advisory form](https://github.com/Salim8519/Peregrine/security/advisories/new) instead.
Include what you found, how to reproduce it and the affected version. You will get a reply
as soon as possible; fixes for confirmed issues ship in a patch release with credit to the
reporter unless you prefer otherwise.

## Supported versions

Peregrine is in early preview (0.x). Only the latest release receives security fixes.

## Verifying a download

Every release file is built by GitHub Actions from a tagged commit — never on a developer
machine — and comes with:

- `SHA256SUMS.txt` — check the file was not corrupted or swapped:
  `sha256sum -c SHA256SUMS.txt --ignore-missing` (or `Get-FileHash` on Windows).
- A signed **build provenance attestation** proving which workflow and commit produced it:
  `gh attestation verify <file> -R Salim8519/Peregrine`

If either check fails, do not run the file and please report it.

## How Peregrine protects you

The full threat model is in [docs/SECURITY-MODEL.md](docs/SECURITY-MODEL.md). In short:

- **No backdoor surface:** no telemetry, no accounts, no remote configuration, no plugin
  system and (for now) no auto-updater. Peregrine only connects to the servers of the
  downloads you start, plus one small daily request to GitHub to see whether a newer
  version exists (it sends nothing about you; turn it off in Settings).
- **Local API is locked down:** it listens on `127.0.0.1` only, requires a random 256-bit token
  stored in your user profile, rejects other websites (Origin check) and DNS-rebinding
  attacks (Host check).
- **The browser bridge is an allowlist:** only Peregrine's extension ID may connect; only six
  methods are allowed; only http(s)/ftp links and a few headers (Cookie, Referer,
  User-Agent, Accept-Language) pass through; download folders and proxy settings are never
  exposed to the browser.
- **Downloaded files can't escape their folder:** server-supplied names are sanitized
  (no `..`, path separators, reserved Windows names, control or bidi-spoofing characters).
  Files are written to `*.peregrine.part` and only renamed after they are complete and
  hashed. Windows Mark-of-the-Web / macOS quarantine is applied so the OS still warns
  before running a downloaded program.
- **The UI cannot run remote code:** a strict Content Security Policy allows only the app's
  own scripts; all text from servers (file names, errors) is rendered as text.
- **Secrets stay private:** cookies and auth headers are never sent back to the UI and are
  stripped when a download redirects to a different host; URLs are logged without query
  strings.
- **Supply chain:** dependencies are pinned with lockfiles (`go.sum`, `package-lock.json`),
  scanned on every push and weekly (govulncheck, npm audit, CodeQL), and updated through
  reviewed Dependabot pull requests. Builds use the latest patched Go toolchain.
