// Builds the static site into dist/: one page per language in src/i18n, plus public/ files
// and a sitemap with hreflang links. No dependencies; run with `node build.mjs`.
// To add a language: copy src/i18n/en.json to <code>.json, translate it, set "path".
import { readFileSync, writeFileSync, mkdirSync, readdirSync, rmSync, cpSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { render } from "./src/page.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const out = join(here, "dist");
const SITE = "https://peregrinedownloader.app";
// Shown until /api/dl?p=info answers; keep roughly current.
const VERSION = "0.4.2";

const langs = readdirSync(join(here, "src/i18n"))
  .filter((f) => f.endsWith(".json"))
  .map((f) => JSON.parse(readFileSync(join(here, "src/i18n", f), "utf8")))
  .sort((a, b) => (a.lang === "en" ? -1 : b.lang === "en" ? 1 : a.lang.localeCompare(b.lang)));

rmSync(out, { recursive: true, force: true });
mkdirSync(out, { recursive: true });
cpSync(join(here, "public"), out, { recursive: true });

for (const t of langs) {
  const file = t.path === "/" ? join(out, "index.html") : join(out, t.path.replace(/^\//, ""), "index.html");
  mkdirSync(dirname(file), { recursive: true });
  writeFileSync(file, render(t, langs, VERSION));
}

const today = new Date().toISOString().slice(0, 10);
const loc = (p) => SITE + (p === "/" ? "/" : p);
const alts = langs.map((l) => `    <xhtml:link rel="alternate" hreflang="${l.lang}" href="${loc(l.path)}"/>`).join("\n");
writeFileSync(join(out, "sitemap.xml"), `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">
${langs.map((l) => `  <url>
    <loc>${loc(l.path)}</loc>
    <lastmod>${today}</lastmod>
${alts}
  </url>`).join("\n")}
</urlset>
`);

console.log(`built ${langs.map((l) => l.lang).join(", ")} -> dist/`);
