// Renders the Field Guide logo to PNG at every size the app, tray, installer and extension
// use, and writes peregrine.ico. Uses the Playwright Chromium that the frontend tests already
// install; no extra dependency.
//
//   cd desktop/frontend && node ../../assets/logo/render.mjs
//
// Sizes up to 32 px are drawn from peregrine-small.svg (heavier shapes, no fine
// detail); larger sizes from peregrine.svg.
import { createRequire } from "node:module";
import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

// Resolve Playwright from the current directory (desktop/frontend), not from this file.
const { chromium } = createRequire(join(process.cwd(), "x.js"))("@playwright/test");
const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const big = readFileSync(join(here, "peregrine.svg"), "utf8");
const small = readFileSync(join(here, "peregrine-small.svg"), "utf8");
const svgFor = (size) => (size <= 32 ? small : big);

const browser = await chromium.launch();
const page = await browser.newPage({ deviceScaleFactor: 1 });

async function render(size) {
  const svg = svgFor(size).replace("<svg ", `<svg width="${size}" height="${size}" `);
  await page.setViewportSize({ width: size, height: size });
  await page.setContent(
    `<html><body style="margin:0;background:transparent">${svg}</body></html>`,
  );
  return page.locator("svg").screenshot({ omitBackground: true });
}

const png = {};
for (const size of [16, 24, 32, 48, 64, 128, 256, 512, 1024]) {
  png[size] = await render(size);
}

const out = (p, data) => {
  mkdirSync(dirname(join(root, p)), { recursive: true });
  writeFileSync(join(root, p), data);
};
for (const [size, data] of Object.entries(png)) out(`assets/logo/png/icon-${size}.png`, data);
out("desktop/build/appicon.png", png[1024]);
out("desktop/build/torrentfile.png", png[1024]); // .torrent file association icon
out("desktop/build/tray.png", png[64]);
for (const size of [16, 32, 48, 128]) out(`extension/icons/icon-${size}.png`, png[size]);
out("desktop/frontend/src/logo.svg", small); // shown at 22-48 px in the app

// ICO with PNG-compressed entries (supported since Windows Vista).
function ico(entries) {
  const head = Buffer.alloc(6 + 16 * entries.length);
  head.writeUInt16LE(0, 0);
  head.writeUInt16LE(1, 2);
  head.writeUInt16LE(entries.length, 4);
  let offset = head.length;
  entries.forEach(([size, data], i) => {
    const o = 6 + 16 * i;
    head.writeUInt8(size >= 256 ? 0 : size, o);
    head.writeUInt8(size >= 256 ? 0 : size, o + 1);
    head.writeUInt16LE(1, o + 4); // planes
    head.writeUInt16LE(32, o + 6); // bits per pixel
    head.writeUInt32LE(data.length, o + 8);
    head.writeUInt32LE(offset, o + 12);
    offset += data.length;
  });
  return Buffer.concat([head, ...entries.map(([, d]) => d)]);
}
const icoData = ico([16, 24, 32, 48, 64, 128, 256].map((s) => [s, png[s]]));
out("assets/logo/peregrine.ico", icoData);
out("desktop/build/windows/icon.ico", icoData);
out("desktop/build/windows/torrentfile.ico", icoData);

// Preview sheet: every size on paper, white and a dark taskbar.
const cells = [16, 24, 32, 48, 64, 128]
  .map((s) => `<img src="data:image/png;base64,${png[s].toString("base64")}" width="${s}" height="${s}">`)
  .join("");
const zoom = [16, 32]
  .map(
    (s) =>
      `<img src="data:image/png;base64,${png[s].toString("base64")}" width="${s * 6}" height="${s * 6}" style="image-rendering:pixelated">`,
  )
  .join("");
await page.setViewportSize({ width: 760, height: 420 });
await page.setContent(`<html><body style="margin:0;font:14px Georgia,serif">
  ${["#ffffff", "#efe8d6", "#202020", "#1c1914"]
    .map(
      (bg) =>
        `<div style="display:flex;gap:18px;align-items:center;padding:12px 18px;background:${bg};color:${bg < "#8" ? "#eee" : "#222"}">
          <span style="width:70px">${bg}</span>${cells}</div>`,
    )
    .join("")}
  <div style="display:flex;gap:24px;align-items:center;padding:12px 18px;background:#ddd">
    <span style="width:70px">16 / 32 px, zoomed ×6</span>${zoom}</div>
</body></html>`);
const preview = join(process.env.PREVIEW_DIR || tmpdir(), "peregrine-icon-preview.png");
await page.screenshot({ path: preview, fullPage: true });
await browser.close();
console.log(`icons written; preview: ${preview}`);
