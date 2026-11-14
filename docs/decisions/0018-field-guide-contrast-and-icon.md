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
- Field-only rules: rust rule on the leading edge of the selected row and sidebar item
  (mirrored in RTL), table headers in muted ink with an ink rule, 5 px progress bars,
  dashed edge on disabled buttons.
- Arabic script (ar, fa, ur) in Field Guide: Naskh at 14.5 px / weight 500 for body text and
  13 px for the small labels (was 14 px / 400 and 11.5–12 px), where the hairlines broke up.
- `src/theme-contrast.test.ts` reads the tokens from `styles.css` and checks every pair below,
  so a colour change that breaks the floor fails `vitest`.
- **Icon**: `assets/logo/peregrine.svg` is now the field-guide edition: the same diving falcon,
  in ink on a paper tile with an ink edge and a faint plate rule, landing on a rust tray.
  `peregrine-small.svg` is a heavier version without fine detail for 16–32 px.
  `assets/logo/render.mjs` (Playwright, already a dev dependency) renders every PNG and the
  multi-size `.ico` for the app, tray, installer, `.torrent` association and extension.
  The blue gradient files are removed; nothing uses them.
- **Extension**: popup and options use the same tokens and serif face, day/night via
  `prefers-color-scheme`, the switch track has an ink edge so "off" is visible, and the
  bundled Noto Naskh Arabic subset (OFL text alongside) keeps Arabic crisp without a network
  request.

## Measured ratios
Worst case across the surfaces each colour sits on (computed from the token values; the test
recomputes them).

| Pair (worst surface) | Min | Day before | Day after | Night before | Night after |
|---|---|---|---|---|---|
| Body text | 4.5 | 9.54 | 10.41 | 9.73 | 10.18 |
| Muted text (sidebar items, numbers, status bar) | 4.5 | 3.66 | 5.78 | 4.57 | 6.14 |
| Faint text (table headers, counts, labels, placeholders) | 4.5 | 1.83 | 4.68 | 2.21 | 4.88 |
| Rust text (section labels, links, "Downloading") | 4.5 | 4.63 | 5.58 | 5.94 | 6.20 |
| OK text | 4.5 | 4.16 | 5.27 | 6.79 | 7.42 |
| Warn text ("Paused", expired link) | 4.5 | 3.23 | 5.38 | 6.95 | 7.37 |
| Error text | 4.5 | 4.15 | 5.03 | 4.86 | 5.28 |
| Primary button text (rest and hover) | 4.5 | 5.91 | 6.97 | 4.97 | 6.33 |
| Danger button text | 4.5 | 6.06 | 7.14 | 2.74 | 7.07 |
| Disabled button text and icon | 3 | 1.88 | 3.47 | 2.53 | 3.60 |
| Input / button / dialog edge | 3 | 1.38 | 3.22 | 1.29 | 3.25 |
| Focus ring | 3 | 4.51 | 5.32 | 5.49 | 6.06 |
| Progress / segment fill, active | 3 | 3.98 | 4.38 | 4.85 | 4.97 |
| Progress / segment fill, done | 3 | 1.92 | 3.20 | 2.37 | 3.71 |
| Progress fill, paused / queued | 3 | 1.92 | 4.60 | 2.37 | 4.94 |
| Progress fill, error | 3 | 3.93 | 4.49 | 4.90 | 5.03 |

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
