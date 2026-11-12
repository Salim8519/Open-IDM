// Batch-add helpers: expand pattern links like "file[001-100].zip" and parse pasted text.

/** Hard cap so a typo like [1-99999999] can't freeze the UI. */
export const MAX_EXPANSION = 5000;

const RANGE = /\[(\d+)-(\d+)\]|\[([a-zA-Z])-([a-zA-Z])\]/;

/**
 * Expands the first [a-b] range in s and recurses for the rest (cartesian product).
 * Numeric ranges keep the zero padding of the start token: [001-003] -> 001, 002, 003.
 * Letter ranges: [a-c] -> a, b, c. Descending ranges count down.
 */
export function expandPattern(s: string, limit = MAX_EXPANSION): string[] {
  const m = RANGE.exec(s);
  if (!m) return [s];
  const before = s.slice(0, m.index);
  const after = s.slice(m.index + m[0].length);
  const parts: string[] = [];
  if (m[1] !== undefined) {
    const a = parseInt(m[1], 10);
    const b = parseInt(m[2], 10);
    const width = m[1].length > 1 && m[1].startsWith("0") ? m[1].length : 0;
    const step = a <= b ? 1 : -1;
    for (let i = a; step > 0 ? i <= b : i >= b; i += step) {
      parts.push(width ? String(i).padStart(width, "0") : String(i));
      if (parts.length >= limit) break;
    }
  } else {
    const a = m[3].charCodeAt(0);
    const b = m[4].charCodeAt(0);
    const step = a <= b ? 1 : -1;
    for (let i = a; step > 0 ? i <= b : i >= b; i += step) parts.push(String.fromCharCode(i));
  }
  const out: string[] = [];
  for (const p of parts) {
    for (const rest of expandPattern(after, limit)) {
      out.push(before + p + rest);
      if (out.length >= limit) return out;
    }
  }
  return out;
}

/** True for http(s) URLs. */
export function isHttpURL(s: string): boolean {
  try {
    const u = new URL(s);
    return (u.protocol === "http:" || u.protocol === "https:") && !!u.host;
  } catch {
    return false;
  }
}

/**
 * Parses pasted text: one link per line (or whitespace separated), expands patterns,
 * drops non-http(s) entries and duplicates, keeps order.
 */
export function parseBatch(text: string, limit = MAX_EXPANSION): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const token of text.split(/\s+/)) {
    const t = token.trim();
    if (!t) continue;
    for (const u of expandPattern(t, limit)) {
      if (!isHttpURL(u) || seen.has(u)) continue;
      seen.add(u);
      out.push(u);
      if (out.length >= limit) return out;
    }
  }
  return out;
}

const DOWNLOADABLE =
  /\.(zip|rar|7z|tar|gz|tgz|bz2|xz|zst|iso|img|exe|msi|dmg|pkg|deb|rpm|apk|appimage|mp4|mkv|avi|mov|webm|flv|wmv|m4v|mp3|flac|wav|aac|ogg|m4a|opus|pdf|epub|docx?|xlsx?|pptx?|csv|bin|torrent|jpe?g|png|gif|webp)$/i;

/** Heuristic for the clipboard monitor: an http(s) link whose path ends in a file extension. */
export function looksDownloadable(s: string): boolean {
  const t = s.trim();
  if (!isHttpURL(t) || /\s/.test(t)) return false;
  const u = new URL(t);
  return DOWNLOADABLE.test(decodeURIComponent(u.pathname)) || /[?&](download|dl)=/i.test(u.search);
}

/** Last path segment of a URL, for display before probing. */
export function nameFromURL(s: string): string {
  try {
    const u = new URL(s);
    const seg = u.pathname.split("/").filter(Boolean).pop();
    return seg ? decodeURIComponent(seg) : u.host;
  } catch {
    return s;
  }
}
