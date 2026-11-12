// Classify network responses as downloadable media (pure).

import { fileExtension } from './decide.js';

const DIRECT_EXT = new Set(['mp4', 'webm', 'mkv', 'mov', 'm4v', 'mp3', 'm4a', 'aac', 'flac', 'ogg', 'oga', 'opus', 'wav']);
const HLS_MIME = new Set(['application/vnd.apple.mpegurl', 'application/x-mpegurl', 'audio/mpegurl', 'audio/x-mpegurl']);
const DASH_MIME = 'application/dash+xml';
// Stream fragments are not useful on their own.
const SEGMENT_EXT = new Set(['ts', 'm4s', 'aac_seg']);
const SEGMENT_MIME = new Set(['video/mp2t', 'video/iso.segment', 'audio/iso.segment']);

export const MIN_MEDIA_SIZE = 256 * 1024; // ignore UI sounds and tiny clips when size is known
export const MAX_MEDIA_PER_TAB = 50;

export function header(headers, name) {
  if (!Array.isArray(headers)) return '';
  const h = headers.find((x) => x && typeof x.name === 'string' && x.name.toLowerCase() === name);
  return h ? String(h.value ?? '') : '';
}

/** Total size from Content-Range ("bytes 0-1/12345") or Content-Length; 0 if unknown. */
export function responseSize(statusCode, contentLength, contentRange) {
  const m = /\/(\d+)\s*$/.exec(contentRange || '');
  if (m) return Number(m[1]);
  if (statusCode === 206) return 0; // partial body without a total
  const n = Number(contentLength);
  return Number.isFinite(n) && n > 0 ? n : 0;
}

/**
 * @param r {url, mime, size, type, statusCode}
 * @returns {null | {url, kind: 'file'|'hls'|'dash', mime, size, filename}}
 */
export function classifyMedia(r) {
  if (!r || !r.url) return null;
  let u;
  try {
    u = new URL(r.url);
  } catch {
    return null;
  }
  if (u.protocol !== 'http:' && u.protocol !== 'https:') return null;
  if (r.statusCode && (r.statusCode < 200 || r.statusCode >= 300)) return null;
  const mime = String(r.mime || '').split(';')[0].trim().toLowerCase();
  const ext = fileExtension(r.url);
  const size = r.size > 0 ? r.size : 0;
  const filename = mediaFilename(r.url, ext);

  if (ext === 'm3u8' || HLS_MIME.has(mime)) return { url: r.url, kind: 'hls', mime, size, filename };
  if (ext === 'mpd' || mime === DASH_MIME) return { url: r.url, kind: 'dash', mime, size, filename };
  if (SEGMENT_EXT.has(ext) || SEGMENT_MIME.has(mime)) return null;

  const isMediaMime = mime.startsWith('video/') || mime.startsWith('audio/');
  if (!isMediaMime && !DIRECT_EXT.has(ext)) return null;
  // A known media extension served as HTML/JSON/text is an error page or API reply.
  if (!isMediaMime && /^(text\/|application\/json)/.test(mime)) return null;
  if (size > 0 && size < MIN_MEDIA_SIZE) return null;
  return { url: r.url, kind: 'file', mime, size, filename };
}

export function mediaFilename(url, ext) {
  try {
    const base = decodeURIComponent(new URL(url).pathname.split('/').pop() || '');
    if (base) return base.slice(0, 200);
  } catch {
    /* fall through */
  }
  return ext ? `media.${ext}` : 'media';
}

/** Identity used to de-duplicate media entries (ignore byte-range query noise). */
export function mediaKey(url) {
  try {
    const u = new URL(url);
    for (const p of ['range', 'rn', 'rbuf', 'bytestart', 'byteend']) u.searchParams.delete(p);
    u.hash = '';
    return u.toString();
  } catch {
    return url;
  }
}

/** Add an entry to a tab's list: de-duplicated, newest last, capped. Returns a new array. */
export function addMedia(list, entry) {
  const key = mediaKey(entry.url);
  const next = (list || []).filter((e) => mediaKey(e.url) !== key);
  next.push(entry);
  return next.slice(-MAX_MEDIA_PER_TAB);
}
