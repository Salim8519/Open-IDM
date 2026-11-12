// Link helpers for "Download all links" (pure).

import { fileExtension } from './decide.js';

export const MAX_LINKS = 100;

/**
 * Resolve, filter and de-duplicate raw hrefs.
 * Keeps http(s)/ftp only, drops fragments, and keeps the first occurrence.
 */
export function dedupeLinks(hrefs, baseUrl) {
  const seen = new Set();
  const out = [];
  for (const raw of hrefs || []) {
    if (typeof raw !== 'string' || raw.length > 8192) continue;
    let u;
    try {
      u = new URL(raw.trim(), baseUrl);
    } catch {
      continue;
    }
    if (!['http:', 'https:', 'ftp:'].includes(u.protocol)) continue;
    u.hash = '';
    const s = u.toString();
    if (!seen.has(s)) {
      seen.add(s);
      out.push(s);
    }
  }
  return out;
}

/** Links whose file type is in the capture list, capped at MAX_LINKS. */
export function downloadableLinks(urls, extensions) {
  return urls.filter((u) => {
    const e = fileExtension(u);
    return e && extensions.includes(e);
  }).slice(0, MAX_LINKS);
}
