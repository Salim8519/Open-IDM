# 0016 — Project website on Vercel at peregrinedownloader.app

- Status: accepted
- Date: 2026-11-14

## Context
Peregrine needs a public home page that ranks in search, explains the app in one screen
and always offers the newest build. The domain `peregrinedownloader.app` is registered
at Cloudflare. All releases so far are GitHub pre-releases, so GitHub's
`/releases/latest` URL returns 404 and cannot be used for download links.

## Decision
- The site lives in `website/`: one hand-written static `index.html`, no framework, no
  web fonts, no third-party requests, no analytics. SEO through plain HTML, meta/Open
  Graph tags, JSON-LD (`SoftwareApplication` + `FAQPage`), `robots.txt` and `sitemap.xml`.
- Hosted on Vercel (free Hobby plan; the project is non-commercial), Git-connected with
  root directory `website/`, so a push to `main` deploys it.
- `/download/<platform>` is a small Vercel function (`website/api/dl.js`) that reads the
  newest non-draft release from the GitHub API and redirects to the matching asset.
  Responses are CDN-cached for 10 minutes; if GitHub is unreachable it falls back to the
  last known tag written in the file.
- DNS stays at Cloudflare with records set to "DNS only" so Vercel issues the TLS
  certificate: `@` and `www` are CNAMEs to the project's `*.vercel-dns-017.com` host
  (Cloudflare flattens the apex). `www` 308-redirects to the apex, which is canonical.
- Vercel project: `peregrine-website` (Hobby team `salim-8196`), production alias
  `peregrine-website-sigma.vercel.app`.

## Alternatives considered
- GitHub Pages — free too, but no server-side redirect for "newest pre-release" links.
- Cloudflare Pages — would keep everything in one account; Vercel was the user's choice.
- Hard-coded versioned links — break or go stale on every release.
- Client-side call to api.github.com — leaks every visitor's IP to a third party.

## Consequences
- The release asset names in `.github/workflows/release.yml` and `website/api/dl.js` must
  stay in sync. Bump `FALLBACK_TAG` and `softwareVersion` in `index.html` occasionally.
- The comparison table makes claims about other products; keep it factual and dated.
