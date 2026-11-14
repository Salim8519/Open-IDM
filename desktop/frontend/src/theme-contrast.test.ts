// Field Guide contrast guard (ADR 0018): reads the theme tokens straight from styles.css
// and checks every text/background and icon/fill/edge pair against WCAG AA, in both
// editions. Change a colour so that a pair drops below the line and this test fails.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const css = readFileSync(fileURLToPath(new URL("./styles.css", import.meta.url)), "utf8");

function tokens(theme: string): Record<string, string> {
  const head = `:root[data-theme="${theme}"] {`;
  // the last block with this exact selector holds the colours (the first is shared font/radius)
  const start = css.lastIndexOf(head);
  if (start < 0) throw new Error(`no token block for ${theme}`);
  const block = css.slice(start + head.length, css.indexOf("}", start));
  const out: Record<string, string> = {};
  for (const [, k, v] of block.matchAll(/--([\w-]+):\s*([^;]+);/g)) out[k] = v.trim();
  return out;
}

type RGB = [number, number, number];
function parse(c: string): { rgb: RGB; a: number } {
  const hex = /^#([0-9a-f]{6})$/i.exec(c);
  if (hex) return { rgb: [0, 2, 4].map((i) => parseInt(hex[1].slice(i, i + 2), 16)) as RGB, a: 1 };
  const rgba = /^rgba\((\d+),\s*(\d+),\s*(\d+),\s*([\d.]+)\)$/.exec(c);
  if (rgba) return { rgb: [+rgba[1], +rgba[2], +rgba[3]], a: +rgba[4] };
  throw new Error(`unsupported colour ${c}`);
}
/** Paints `top` (possibly translucent) over an opaque `under`. */
function over(top: string, under: string): string {
  const t = parse(top);
  const u = parse(under).rgb;
  const mix = t.rgb.map((v, i) => Math.round(v * t.a + u[i] * (1 - t.a)));
  return "#" + mix.map((v) => v.toString(16).padStart(2, "0")).join("");
}
function luminance(c: string): number {
  const [r, g, b] = parse(c).rgb.map((v) => {
    const s = v / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}
export function ratio(a: string, b: string): number {
  const x = luminance(a);
  const y = luminance(b);
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

const TEXT = 4.5; // normal text
const UI = 3; // icons, fills, control edges, focus ring, disabled controls

for (const theme of ["field-day", "field-night"]) {
  describe(`${theme} contrast`, () => {
    const t = tokens(theme);
    const surfaces: Record<string, string> = {
      bg: t.bg,
      side: t.side,
      raised: t.raised,
      field: t.field,
      "selected row": over(t.sel, t.bg),
      "selected sidebar item": over(t.sel, t.side),
      "hovered row": over(t.hover, t.bg),
    };
    const pairs: [string, string, string[], number][] = [
      ["text", t.text, Object.keys(surfaces), TEXT],
      ["muted", t.muted, Object.keys(surfaces), TEXT],
      ["faint", t.faint, Object.keys(surfaces), TEXT],
      ["accent-text", t["accent-text"], Object.keys(surfaces), TEXT],
      ["ok", t.ok, ["bg", "side", "raised", "selected row"], TEXT],
      ["warn", t.warn, ["bg", "side", "raised", "selected row"], TEXT],
      ["err", t.err, ["bg", "side", "raised", "selected row"], TEXT],
      ["disabled", t.disabled, ["bg", "side", "raised"], UI],
      ["edge", t.edge, ["bg", "side", "raised", "field"], UI],
      ["focus ring (accent)", t.accent, ["bg", "side", "raised", "field"], UI],
    ];
    for (const [name, fg, on, min] of pairs) {
      for (const s of on) {
        it(`${name} on ${s} ≥ ${min}`, () => {
          expect(ratio(fg, surfaces[s])).toBeGreaterThanOrEqual(min);
        });
      }
    }
    it("primary button text ≥ 4.5 at rest and on hover", () => {
      expect(ratio(t["on-accent"], t["accent-strong"])).toBeGreaterThanOrEqual(TEXT);
      expect(ratio(t["on-accent"], t.accent)).toBeGreaterThanOrEqual(TEXT);
    });
    it("danger button text ≥ 4.5", () => {
      expect(ratio(t["on-err"], t.err)).toBeGreaterThanOrEqual(TEXT);
    });
    it("progress and segment fills ≥ 3 against the track", () => {
      for (const fill of [t.accent, t["done-fill"], t.faint, t.err]) {
        expect(ratio(fill, t.track)).toBeGreaterThanOrEqual(UI);
      }
    });
  });
}
