// Redirects /download/<platform> to the matching file in the newest GitHub release
// (pre-releases included, drafts skipped). The CDN caches each answer for 2 minutes,
// so GitHub's unauthenticated rate limit is never a problem. If GitHub cannot be
// reached, it falls back to the last release known when this file was written.

const REPO = "Salim8519/Peregrine";
const FALLBACK_TAG = "v0.4.2";

// platform -> function(version) => asset file name
const ASSETS = {
  windows: (v) => `Peregrine-${v}-windows-x64-setup.exe`,
  "windows-portable": (v) => `Peregrine-${v}-windows-x64-portable.zip`,
  macos: (v) => `Peregrine-${v}-macos-universal.dmg`,
  linux: (v) => `peregrine-cli-${v}-linux-amd64.tar.gz`,
  "linux-arm64": (v) => `peregrine-cli-${v}-linux-arm64.tar.gz`,
  extension: (v) => `Peregrine-${v}-chromium-extension.zip`,
  checksums: () => "SHA256SUMS.txt",
};

let memo = { at: 0, release: null };

async function newestRelease() {
  if (memo.release && Date.now() - memo.at < 60 * 1000) return memo.release;
  const res = await fetch(`https://api.github.com/repos/${REPO}/releases?per_page=10`, {
    headers: { Accept: "application/vnd.github+json", "User-Agent": "peregrine-website" },
  });
  if (!res.ok) throw new Error(`github ${res.status}`);
  const list = await res.json();
  const rel = list.find((r) => !r.draft && Array.isArray(r.assets) && r.assets.length > 0);
  if (!rel) throw new Error("no release");
  memo = { at: Date.now(), release: rel };
  return rel;
}

export default async function handler(req, res) {
  const p = String(req.query.p || "");

  let tag = FALLBACK_TAG;
  let assets = null;
  let published = null;
  try {
    const rel = await newestRelease();
    tag = rel.tag_name;
    published = rel.published_at;
    assets = rel.assets;
  } catch {
    // keep the fallback
  }
  const version = tag.replace(/^v/, "");

  res.setHeader("Cache-Control", "public, s-maxage=120, stale-while-revalidate=600");

  if (p === "info") {
    res.status(200).json({ version, tag, published });
    return;
  }

  const make = ASSETS[p];
  if (!make) {
    res.redirect(302, `https://github.com/${REPO}/releases`);
    return;
  }
  const name = make(version);
  const found = assets ? assets.find((a) => a.name === name) : null;
  const url = found
    ? found.browser_download_url
    : assets
      ? `https://github.com/${REPO}/releases/tag/${tag}` // asset missing in this release
      : `https://github.com/${REPO}/releases/download/${tag}/${name}`;
  res.redirect(302, url);
}
