// Pure decision logic: should a browser download be handed to Peregrine?
// No chrome.* calls here so it can be unit-tested with `node --test`.

import { DEFAULT_EXTENSIONS, DEFAULT_MIN_SIZE, DEFAULT_SETTINGS } from './defaults.js';

const CAPTURE_SCHEMES = new Set(['http:', 'https:', 'ftp:']);

// MIME types that identify a capturable type when the filename is not known yet.
const MIME_TO_EXT = {
  'application/zip': 'zip',
  'application/x-zip-compressed': 'zip',
  'application/vnd.rar': 'rar',
  'application/x-rar-compressed': 'rar',
  'application/x-7z-compressed': '7z',
  'application/gzip': 'gz',
  'application/x-gzip': 'gz',
  'application/x-tar': 'tar',
  'application/x-bzip2': 'bz2',
  'application/x-xz': 'xz',
  'application/x-msdownload': 'exe',
  'application/x-msdos-program': 'exe',
  'application/vnd.microsoft.portable-executable': 'exe',
  'application/x-msi': 'msi',
  'application/x-ms-installer': 'msi',
  'application/x-apple-diskimage': 'dmg',
  'application/x-iso9660-image': 'iso',
  'application/vnd.android.package-archive': 'apk',
  'application/vnd.debian.binary-package': 'deb',
  'application/x-rpm': 'rpm',
  'application/pdf': 'pdf',
  'application/epub+zip': 'epub',
  'application/x-bittorrent': 'torrent',
  'video/mp4': 'mp4',
  'video/x-matroska': 'mkv',
  'video/x-msvideo': 'avi',
  'video/quicktime': 'mov',
  'video/webm': 'webm',
  'audio/mpeg': 'mp3',
  'audio/flac': 'flac',
  'audio/x-flac': 'flac',
  'audio/wav': 'wav',
  'audio/aac': 'aac',
  'audio/mp4': 'm4a',
  'audio/ogg': 'ogg',
};

/** Lower-case extension (no dot) of a filename or URL path, or '' if none. */
export function fileExtension(nameOrUrl) {
  if (!nameOrUrl) return '';
  let path = String(nameOrUrl);
  try {
    if (/^[a-z][a-z0-9+.-]*:/i.test(path)) path = decodeURIComponent(new URL(path).pathname);
  } catch {
    /* not a URL or bad escape: use as-is */
  }
  path = path.split(/[?#]/)[0];
  const base = path.split(/[\\/]/).pop() || '';
  const dot = base.lastIndexOf('.');
  if (dot <= 0 || dot === base.length - 1) return '';
  const ext = base.slice(dot + 1).toLowerCase();
  return /^[a-z0-9]{1,10}$/.test(ext) ? ext : '';
}

export function mimeExtension(mime) {
  if (!mime) return '';
  return MIME_TO_EXT[String(mime).split(';')[0].trim().toLowerCase()] || '';
}

/** Parse "zip, rar 7z\n.mp4" into a clean, de-duplicated list. */
export function parseExtensionList(text) {
  const out = [];
  for (const raw of String(text || '').split(/[\s,;]+/)) {
    const e = raw.trim().toLowerCase().replace(/^\*?\./, '');
    if (/^[a-z0-9]{1,10}$/.test(e) && !out.includes(e)) out.push(e);
  }
  return out;
}

/** Parse one site per line ("example.com", "https://x.example.org/path", "*.cdn.net"). */
export function parseSiteList(text) {
  const out = [];
  for (let line of String(text || '').split(/\r?\n/)) {
    line = line.trim().toLowerCase();
    if (!line || line.startsWith('#')) continue;
    try {
      if (/^[a-z][a-z0-9+.-]*:\/\//.test(line)) line = new URL(line).hostname;
    } catch {
      continue;
    }
    line = line.replace(/^\*\./, '').replace(/^\.+|\.+$/g, '').split('/')[0].split(':')[0];
    if (/^[a-z0-9.-]+$/.test(line) && line.includes('.') && !out.includes(line)) out.push(line);
    else if (line === 'localhost' && !out.includes(line)) out.push(line);
  }
  return out;
}

/** True if host equals site or is a subdomain of it. */
export function hostMatches(host, site) {
  if (!host || !site) return false;
  host = host.toLowerCase().replace(/\.$/, '');
  site = site.toLowerCase();
  return host === site || host.endsWith('.' + site);
}

function hostOf(url) {
  try {
    return new URL(url).hostname;
  } catch {
    return '';
  }
}

export function isExcluded(urls, sites) {
  if (!sites || !sites.length) return false;
  return urls.some((u) => {
    const h = hostOf(u);
    return h && sites.some((s) => hostMatches(h, s));
  });
}

/** Merge stored settings with defaults and drop anything malformed. */
export function normalizeSettings(stored) {
  const s = { ...DEFAULT_SETTINGS, ...(stored || {}) };
  s.enabled = s.enabled !== false;
  s.extensions = Array.isArray(s.extensions) ? parseExtensionList(s.extensions.join(' ')) : DEFAULT_EXTENSIONS;
  s.excludedSites = Array.isArray(s.excludedSites) ? parseSiteList(s.excludedSites.join('\n')) : [];
  const mb = Number(s.minSizeMB);
  s.minSizeMB = s.minSizeMB === null || s.minSizeMB === '' || !Number.isFinite(mb) || mb < 0 ? null : mb;
  return s;
}

/** Effective minimum size in bytes: user override, else the app's minExtSize, else 1 MB. */
export function effectiveMinSize(settings, appMinSize) {
  if (settings.minSizeMB !== null && settings.minSizeMB !== undefined) return Math.round(settings.minSizeMB * 1024 * 1024);
  if (Number.isFinite(appMinSize) && appMinSize > 0) return appMinSize;
  return DEFAULT_MIN_SIZE;
}

/**
 * Decide whether to intercept a chrome.downloads item.
 * @param item  DownloadItem-like: {url, finalUrl, filename, mime, fileSize, totalBytes, referrer, byExtensionId, state}
 * @param settings normalized settings
 * @param ctx {selfId, minSize, skipNext, recentFallback: Set<string>}
 * @returns {{capture: boolean, reason: string}}
 */
export function shouldIntercept(item, settings, ctx = {}) {
  const no = (reason) => ({ capture: false, reason });
  if (!item || !item.url) return no('invalid');
  if (!settings.enabled) return no('disabled');
  if (ctx.selfId && item.byExtensionId === ctx.selfId) return no('own-download');
  const url = item.finalUrl || item.url;
  if (ctx.recentFallback && (ctx.recentFallback.has(url) || ctx.recentFallback.has(item.url))) return no('fallback');
  if (item.state && item.state !== 'in_progress') return no('not-in-progress');
  let scheme;
  try {
    scheme = new URL(url).protocol;
  } catch {
    return no('invalid');
  }
  if (!CAPTURE_SCHEMES.has(scheme)) return no('scheme'); // blob:, data:, file:, filesystem:, chrome: ...
  if (ctx.skipNext) return no('skip-next');
  if (isExcluded([url, item.url, item.referrer].filter(Boolean), settings.excludedSites)) return no('excluded');

  const ext = fileExtension(item.filename) || fileExtension(url) || mimeExtension(item.mime);
  if (ext && settings.extensions.includes(ext)) return { capture: true, reason: 'type' };

  const size = item.totalBytes > 0 ? item.totalBytes : item.fileSize > 0 ? item.fileSize : 0;
  const min = ctx.minSize > 0 ? ctx.minSize : DEFAULT_MIN_SIZE;
  if (size > 0) return size >= min ? { capture: true, reason: 'size' } : no('small');
  return no('unknown');
}
