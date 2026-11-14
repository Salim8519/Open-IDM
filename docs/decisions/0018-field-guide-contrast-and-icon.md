# 0018 — Field Guide contrast floor, field-guide icon and extension look

- Status: accepted (supersedes the colour values of 0017 and the logo colours of 0007)
- Date: 2026-11-14

## Context
The first Field Guide pass (ADR 0017) copied the website's palette. In the app, several
things blended into the paper: grey labels and table headers, disabled toolbar buttons
(32% opacity), input and button edges, finished/paused progress bars against their track,
amber "Paused" text, and white text on the night edition's danger button. Measured against
WCAG 2.2, 41 pairs failed AA. The owner asked that buttons, icons and text always stand out
from the paper, and that the app icon and the browser extension share the look.

## Decision
- **Contrast floor**, both editions: 4.5:1 for every text colour on every surface it is used
  on (paper, sidebar, raised panels, inputs, selected and hovered rows); 3:1 for icons,
  progress/segment fills against the track, control edges, the focus ring and disabled
  controls. Same paper, ink and rust identity — only stronger.
- New tokens: `--edge` (ink edge for inputs, plain buttons, dialogs, menus, toasts; the
  existing `--line` stays for decorative dividers), `--disabled` (quieter ink for disabled
  controls instead of a see-through fade) and `--on-err` (text on the danger colour).
  Classic defines them too but does not use them.
- Palette: the website's final tokens (warm paper #f5eddb, ink #211b12, rust #a8481a /
  night #f29a62, ok-green #27692b / #9fd67a), plus a **sun-yellow highlight** (#f4c34f /
  #e8b33c, always with ink text) for the current sidebar item and the selected row, and six
  field-guide hues for file-category icons (clay, plum, sea, ochre, indigo, moss), each
  4.5:1 on the sidebar, paper and selected row.
- Field-only rules: rust rule on the leading edge of the selected row and sidebar item
  (mirrored in RTL), labels and headings (section labels, table headers, field labels,
  legends, primary buttons) in bold ink, 5 px progress bars, dashed edge on disabled buttons.
- Arabic script (ar, fa, ur) in Field Guide: Naskh at 14.5 px / weight 500 for body text,
  700 for labels and headings, 13 px for the small labels (was 14 px / 400 and 11.5–12 px),
  where the hairlines broke up.
- `src/theme-contrast.test.ts` reads the tokens from `styles.css` and checks every pair below,
  so a colour change that breaks the floor fails `vitest`.
- **Icon**: `assets/logo/peregrine.svg` is now the field-guide edition: the same diving falcon,
  in ink (#211b12) on a paper tile (#f5eddb) with an ink edge and a faint plate rule, landing
  on a rust tray (#a8481a).
  `peregrine-small.svg` is a heavier version without fine detail for 16–32 px.
  `assets/logo/render.mjs` (Playwright, already a dev dependency) renders every PNG and the
  multi-size `.ico` for the app, tray, installer, `.torrent` association and extension.
  The blue gradient files are removed; nothing uses them.
- **Extension**: popup and options use the same tokens and serif face, section labels as a
  sun-yellow sticker (as on the website), day/night via
  `prefers-color-scheme`, the switch track has an ink edge so "off" is visible, and the
  bundled Noto Naskh Arabic subset (OFL text alongside) keeps Arabic crisp without a network
  request.

## Measured ratios
Worst case across the surfaces each colour sits on (computed from the token values; the test
recomputes them).

Before = the first Field Guide pass (website palette v1); after = this ADR (website palette
v2, shared with peregrinedownloader.app).

| Pair (worst surface) | Min | Day before | Day after | Night before | Night after |
|---|---|---|---|---|---|
| Body text | 4.5 | 9.54 | 12.10 | 9.73 | 9.00 |
| Muted text (sidebar items, numbers, status bar) | 4.5 | 3.66 | 5.51 | 4.57 | 5.14 |
| Faint text (table headers, counts, labels, placeholders) | 4.5 | 1.83 | 4.55 | 2.21 | 4.90 |
| Rust text (section labels, links, "Downloading") | 4.5 | 4.63 | 5.22 | 5.94 | 5.53 |
| OK text | 4.5 | 4.16 | 5.10 | 6.79 | 6.74 |
| Warn text ("Paused", expired link) | 4.5 | 3.23 | 5.44 | 6.95 | 5.96 |
| Error text | 4.5 | 4.15 | 4.88 | 4.86 | 4.76 |
| Primary button text (rest and hover) | 4.5 | 5.91 | 4.88 | 4.97 | 7.99 |
| Danger button text | 4.5 | 6.06 | 6.15 | 2.74 | 7.30 |
| Ink on the sun highlight (current sidebar item) | 4.5 | — | 10.38 | — | 9.13 |
| Category icon hues (worst of six) | 4.5 | — | 4.52 | — | 5.12 |
| Disabled button text and icon | 3 | 1.88 | 3.38 | 2.53 | 3.66 |
| Input / button / dialog edge | 3 | 1.38 | 3.38 | 1.29 | 3.22 |
| Focus ring | 3 | 4.51 | 3.91 | 5.49 | 7.61 |
| Progress / segment fill, active | 3 | 3.98 | 3.46 | 4.85 | 6.57 |
| Progress / segment fill, done | 3 | 1.92 | 3.50 | 2.37 | 3.90 |
| Progress fill, paused / queued | 3 | 1.92 | 4.38 | 2.37 | 5.87 |
| Progress fill, error | 3 | 3.93 | 4.36 | 4.90 | 5.19 |

Surfaces checked: paper, sidebar, raised panel, input field, selected row, selected sidebar
item, hovered row (status colours: paper, sidebar, raised, selected row). The extension uses
the same values on paper and raised cards; its switch track edge is the `--edge` pair.

## Alternatives considered
- Keep the website palette and only fix the worst pairs — the same faint greys are used
  for a dozen elements; a token-level fix is consistent and testable.
- Make `--line` itself 3:1 — every row divider would turn into a heavy ruled grid; a
  separate `--edge` keeps dividers quiet and controls visible.
- Generate the icon with an external tool (ImageMagick, Inkscape) — not on every
  contributor's machine; the Playwright Chromium is already installed for the e2e tests.

## Consequences
- New Field Guide colours must keep the floor; `vitest` enforces it for the tokens.
- Re-run `node ../../assets/logo/render.mjs` from `desktop/frontend` after changing the logo SVGs.
- The store screenshots in `docs/store/images` still show the old extension look and logo
  until they are regenerated (`extension/tests/store/shots.mjs`).
