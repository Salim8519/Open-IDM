# Contributing to Peregrine

Thanks for helping. Peregrine is a free, open-source download manager for Windows and macOS,
plus a Chromium browser extension. Bug reports, fixes, tests and docs are all welcome.

## Before you start

- **Security problem?** Do not open an issue. Follow [SECURITY.md](SECURITY.md).
- **Bug or idea?** Search the [issues](https://github.com/Salim8519/Peregrine/issues) first,
  then open one with the matching template.
- **Bigger change** (new protocol, new dependency, new API method, UI redesign, anything that
  touches the security model)? Open an issue and agree on the approach **before** writing code.
  It saves you from a PR that can't be merged.
- **Small fixes** (typos, clear bugs, missing tests) can go straight to a PR.

Read these first, they are the project's memory:

1. [docs/SPEC.md](docs/SPEC.md): what we are building.
2. [docs/ROADMAP.md](docs/ROADMAP.md): what is planned and in what order.
3. [docs/decisions/](docs/decisions/): why things are the way they are.
4. [docs/SECURITY-MODEL.md](docs/SECURITY-MODEL.md): the threat model.

## What we won't accept

- Telemetry, analytics, ads, bundled offers, or any network call that isn't the user's own
  download or the signed update check.
- Reversing a recorded decision without a new ADR that supersedes it (see below).
- Weakening or deleting a test so a change passes.
- New dependencies without a clear reason. Prefer the Go standard library.
- An Android or iOS app, or official Linux support, for now. Linux builds from the same
  code but isn't tested or supported yet (see [ADR 0005](docs/decisions/0005-desktop-first-scope.md)).
  Small fixes that keep the Linux build working are welcome.

## Set up

Requirements: Go (version in `go.mod`), Node 22+, and
[Wails v2](https://wails.io) for the desktop app.

```bash
git clone https://github.com/Salim8519/Peregrine.git
cd Peregrine
go test -race ./...
```

## Project layout

```
engine/             Go library: no UI code, no globals, no os.Exit
cmd/peregrine/      CLI (get, add, list, serve)
cmd/peregrine-host/ Native Messaging host (bridge between extension and app)
internal/testsrv/   "evil" HTTP test server (drops, throttles, no-Range, lies about size)
desktop/            Wails app (React UI in desktop/frontend)
extension/          Chromium MV3 extension
docs/               spec, roadmap, decisions, changelog
```

## Run the checks CI runs

Your PR must pass all of them. Run what applies to your change before pushing.

**Go (engine, CLI, host)**

```bash
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@latest ./...
go test -race ./...
```

**Desktop UI**

```bash
cd desktop/frontend
npm ci && npx playwright install chromium
npm run typecheck && npm test
npm run build
npm run test:e2e
```

**Browser extension**

```bash
cd extension
npm ci && npx playwright install chromium
npm test
npm run test:e2e
```

**Desktop app build**

```bash
cd desktop && wails build
```

## Coding rules

- `engine/` must never import UI, Wails or platform UI packages.
- Long operations take a `context.Context` and stop cleanly when it is cancelled.
- No panics across package boundaries. Return wrapped errors: `fmt.Errorf("...: %w", err)`.
- Never write downloaded bytes to the final filename until the download is complete. Write
  to `<name>.peregrine.part`, verify, then rename.
- Sanitize every filename that comes from a server or URL.
- Never log full URLs with credentials or tokens. Redact query strings.
- Format Go code with `gofmt`.

## Tests

- Every engine feature has a test against `internal/testsrv`.
- **A bug fix starts with a failing test** that reproduces the bug.
- UI changes need a unit or Playwright test where it's practical.

## Keep the docs up to date

In the same PR as the code:

- **User-visible change:** add a line to [docs/CHANGELOG.md](docs/CHANGELOG.md) under
  `Unreleased`.
- **Spec change:** update [docs/SPEC.md](docs/SPEC.md).
- **Non-trivial choice** (library, format, protocol, architecture): add
  `docs/decisions/NNNN-short-title.md` using [the template](docs/decisions/TEMPLATE.md).
- **New input, network listener, file write, API method or dependency:** check it against
  [docs/SECURITY-MODEL.md](docs/SECURITY-MODEL.md) and update it. New API methods are not
  exposed to the browser extension unless they are added to the host allowlist.
- **Finished a roadmap task:** tick it in [docs/ROADMAP.md](docs/ROADMAP.md).

## Pull requests

- Branch from `main`, keep the PR focused on one thing.
- Write a clear title and fill in the PR template.
- Link the issue it fixes (`Fixes #123`).
- CI on PRs from first-time contributors runs after a maintainer approves it.
- A maintainer reviews every PR. Expect questions, it's not a rejection.

## AI-assisted contributions

Peregrine itself is built with AI help, so AI-assisted PRs are fine. You are responsible for
the result: read and understand every line, run the checks yourself, and say in the PR that
you used AI. The rules in [CLAUDE.md](CLAUDE.md) apply to AI tools working on this repo.

## License

By contributing, you agree that your contribution is licensed under
[GPL-3.0-or-later](LICENSE), the same license as the project.

## Code of conduct

Be kind. See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).
