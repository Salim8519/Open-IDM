# 0017 — Field Guide theme as the default look

- Status: accepted
- Date: 2026-11-14

## Context
The website was redesigned as a naturalist's field guide entry ("The Digital Peregrine",
ADR 0016). The owner wanted the app to share that identity instead of the neutral
graphite look, while keeping the old look available.

## Decision
- New theme "Field Guide", the default: warm paper and ink, a serif text face, rust as the
  single accent, small-caps section labels, square corners. Two editions: day (paper) and
  night (the same page under a lamp). `theme: "field"` follows the system's day/night;
  `"field-day"` and `"field-night"` pin one edition.
- The previous look is kept as "Classic" (`"system"`, `"light"`, `"dark"`).
- Implemented only with CSS custom properties on `<html data-theme>` (`field-day`,
  `field-night`, `light`, `dark`), so components are unchanged.
- Arabic, Persian and Urdu use a bundled Arabic subset of Noto Naskh Arabic (94 KB, SIL OFL,
  `unicode-range`, no network). Latin uses the system serif (Georgia and fallbacks); other
  scripts fall back to the system's fonts. The OFL text ships in THIRD-PARTY-NOTICES.
- Settings gain `uiVersion`. Settings saved before this change (no `uiVersion`) that still
  have the old default `"system"` move once to `"field"`; an explicit light/dark choice is
  kept. Settings written by the UI are always stamped current, so a later Classic choice
  is never migrated away. Unknown theme values fall back to `"field"`.

## Alternatives considered
- Replace the Classic look entirely — some people prefer a neutral tool UI; keeping it
  costs only one token block.
- A serif web font for Latin too — more weight and a network/licensing step for little gain;
  Georgia-class system serifs are on every desktop the app supports.
- Migrate every existing install to Field Guide — would override people who deliberately
  picked light or dark.

## Consequences
- Every new colour must be defined in all four token blocks.
- The `field` value is the engine default; tests cover the default and the migration.
- The first paint uses the last theme remembered in `localStorage`, so there is no flash of
  the wrong theme at launch.
