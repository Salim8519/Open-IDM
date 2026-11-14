# 0020 — Update check (step 1 of signed auto-update)

- Status: accepted
- Date: 2026-11-14

## Context
Users only find out about a new version by visiting the website. The spec asks for signed
auto-update that the user can disable, and ADR 0008 forbids an auto-updater until a
signature-verifying design has its own ADR. Telling the user that a new version exists is
useful on its own and needs no signing keys, so we ship it first and keep installing for later.

## Decision
- The desktop app asks `https://api.github.com/repos/Salim8519/Peregrine/releases/latest`
  for the latest published release: 20 s after start, then once a day while it runs (an hour
  later after a failure). The setting **Check for new versions automatically** (`updateCheck`)
  is on by default and turns it off. "Check now" in Settings runs it by hand.
- The request is a plain GET with a fixed `User-Agent: Peregrine-update-check`, no cookies,
  no query, no IDs, through the user's proxy setting. Only `tag_name`, `draft` and
  `prerelease` are read, from at most 1 MiB of response.
- Only plain versions (`v1.2.3`) are accepted. The release-page link is built by the app
  from that version (`https://github.com/Salim8519/Peregrine/releases/tag/v1.2.3`); nothing
  from the response is ever opened. Dev builds with a non-release version never report updates.
- A newer version shows a pill in the status bar and in Settings; clicking it opens the release
  page in the browser (`openUpdatePage`, which only opens the link from the last check). When
  the window is hidden or not focused, a system notification is shown once per version.
- Nothing is downloaded or installed. The user downloads the release like the first time,
  where SHA-256 sums and provenance attestations already apply (ADR 0008).
- `checkUpdate`, `updateStatus` and `openUpdatePage` are not on the native host allowlist.

## Alternatives considered
- Ask our website (`/api/dl`) instead of GitHub: one more service in the path, and the website
  itself reads GitHub; the release data is the same.
- Download and install automatically now: needs update signing keys, a signed manifest and,
  on macOS, notarization. That is step 2 and gets its own ADR.
- Off by default: the spec says the user can disable it, and an unpatched download manager is
  the larger risk; the check sends nothing about the user.

## Consequences
- One small HTTPS request to api.github.com per day is the only traffic besides downloads.
  SPEC, SECURITY.md and SECURITY-MODEL.md (T24) say so.
- GitHub's unauthenticated limit (60 requests/hour per IP) is far above one per day; a 403
  is treated like being offline.
- Step 2 (signed download and install, Windows first) builds on `internal/update`.
