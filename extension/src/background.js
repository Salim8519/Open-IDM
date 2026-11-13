// Peregrine background service worker (Manifest V3, ES module).
//
// - Intercepts browser downloads and hands them to the Peregrine app via native messaging.
// - Context menus: "Download with Peregrine" and "Download all links with Peregrine".
// - Detects media responses per tab (non-blocking webRequest) for the popup.

import { normalizeSettings, effectiveMinSize, shouldIntercept, fileExtension } from './decide.js';
import { buildCookieHeader } from './cookies.js';
import { classifyMedia, addMedia, header, responseSize } from './media.js';
import { dedupeLinks, downloadableLinks } from './links.js';
import { callNative, isUnavailable } from './native.js';

const FALLBACK_TTL_MS = 2 * 60 * 1000;
const APP_SETTINGS_TTL_MS = 10 * 60 * 1000;
const MENU_LINK = 'peregrine-link';
const MENU_ALL = 'peregrine-all';
const MENU_VIDEO = 'peregrine-video';
const t = (key, subs) => chrome.i18n.getMessage(key, subs) || key;

// ---------- settings ----------

async function loadSettings() {
  const { settings } = await chrome.storage.local.get('settings');
  return normalizeSettings(settings);
}

async function saveSettings(patch) {
  const next = normalizeSettings({ ...(await loadSettings()), ...patch });
  await chrome.storage.local.set({ settings: next });
  await updateTitle(next);
  return next;
}

async function updateTitle(settings) {
  const s = settings || (await loadSettings());
  await chrome.action.setTitle({ title: s.enabled ? t('extName') : t('titleOff') });
}

let appSettingsFetching = null;

/** Refresh the app's minExtSize (without starting the app). */
function refreshAppSettings() {
  if (!appSettingsFetching) {
    appSettingsFetching = callNative('getSettings', undefined, { timeoutMs: 5000 })
      .then((s) => {
        if (s && Number.isFinite(s.minExtSize)) {
          return chrome.storage.local.set({ appMinSize: s.minExtSize, appSettingsAt: Date.now() });
        }
      })
      .catch(() => {})
      .finally(() => {
        appSettingsFetching = null;
      });
  }
  return appSettingsFetching;
}

// ---------- fallback memory (avoid intercepting our own re-download) ----------

const fallbackMem = new Map();

async function loadFallback() {
  const { fallbackUrls = {} } = await chrome.storage.session.get('fallbackUrls');
  const now = Date.now();
  for (const [u, ts] of Object.entries(fallbackUrls)) if (now - ts < FALLBACK_TTL_MS) fallbackMem.set(u, ts);
  for (const [u, ts] of fallbackMem) if (now - ts >= FALLBACK_TTL_MS) fallbackMem.delete(u);
  return new Set(fallbackMem.keys());
}

async function rememberFallback(url) {
  fallbackMem.set(url, Date.now());
  await chrome.storage.session.set({ fallbackUrls: Object.fromEntries(fallbackMem) });
}

async function browserDownload(url) {
  await rememberFallback(url);
  return chrome.downloads.download({ url, conflictAction: 'uniquify' });
}

// ---------- capture ----------

function basename(p) {
  return p ? String(p).split(/[\\/]/).pop() : '';
}

async function buildCapture({ url, referrer, filename, mime, size, pageUrl, kind }) {
  const headers = { 'User-Agent': navigator.userAgent };
  try {
    const cookie = buildCookieHeader(await chrome.cookies.getAll({ url }));
    if (cookie) headers.Cookie = cookie;
  } catch {
    /* no cookies for this URL */
  }
  if (referrer) headers.Referer = referrer;
  const c = { url, headers, source: 'extension' };
  if (kind) c.kind = kind;
  if (filename) c.filename = filename;
  if (mime) c.mimeType = mime;
  if (size > 0) c.size = size;
  if (pageUrl || referrer) c.pageUrl = pageUrl || referrer;
  return c;
}

// How long the browser's download may wait for the app (it may need a few seconds to start)
// before we let the browser carry on by itself.
const DECIDE_TIMEOUT_MS = 25_000;

/**
 * Decides whether Peregrine takes a download. Runs in onDeterminingFilename, i.e. before the
 * browser picks a file name, shows its download bubble or opens a Save as dialog, so a
 * captured download never appears as a browser download (it used to start in parallel).
 * Returns true when the app took it.
 */
async function decideAndCapture(item) {
  const [settings, local, session, fallback] = await Promise.all([
    loadSettings(),
    chrome.storage.local.get(['appMinSize', 'appSettingsAt']),
    chrome.storage.session.get('skipNext'),
    loadFallback(),
  ]);
  if (!local.appSettingsAt || Date.now() - local.appSettingsAt > APP_SETTINGS_TTL_MS) refreshAppSettings();

  const decision = shouldIntercept(item, settings, {
    selfId: chrome.runtime.id,
    minSize: effectiveMinSize(settings, local.appMinSize),
    skipNext: !!session.skipNext,
    recentFallback: fallback,
  });
  if (decision.reason === 'skip-next') {
    await chrome.storage.session.set({ skipNext: false });
    return false;
  }
  if (!decision.capture) return false;

  const url = item.finalUrl || item.url;
  const capture = await buildCapture({
    url,
    referrer: item.referrer,
    filename: basename(item.filename),
    mime: item.mime,
    size: item.totalBytes > 0 ? item.totalBytes : item.fileSize,
  });
  try {
    await callNative('capture', capture);
  } catch (err) {
    console.info('Peregrine: leaving download to the browser:', err.code || err.message);
    return false;
  }
  return true;
}

/** The app accepted the download: remove the browser's copy (and its file, if it finished). */
async function dropBrowserCopy(id) {
  const [cur] = await chrome.downloads.search({ id }).catch(() => []);
  if (cur && cur.state === 'complete') await chrome.downloads.removeFile(id).catch(() => {});
  else await chrome.downloads.cancel(id).catch(() => {});
  await chrome.downloads.erase({ id }).catch(() => {});
}

// Downloads the early hook has seen. If it never fires for one (some download paths skip
// the extension naming step, e.g. under automation), onCreated takes over shortly after.
const seenEarly = new Set();
const EARLY_GRACE_MS = 500;

chrome.downloads.onCreated.addListener((item) => {
  setTimeout(() => {
    if (seenEarly.delete(item.id)) return;
    lateCapture(item).catch((e) => console.warn('Peregrine: onCreated failed', e));
  }, EARLY_GRACE_MS);
});

/** Fallback path: hold the browser's copy, ask the app, then drop or release it. */
async function lateCapture(item) {
  const [cur] = await chrome.downloads.search({ id: item.id }).catch(() => []);
  if (!cur || cur.state !== 'in_progress') return;
  await chrome.downloads.pause(item.id).catch(() => {});
  // Keep the name known at creation: by now the browser may have given the file a temporary name.
  if (await decideAndCapture({ ...cur, filename: item.filename })) {
    await dropBrowserCopy(item.id);
  } else {
    await chrome.downloads.resume(item.id).catch(() => {});
  }
}

chrome.downloads.onDeterminingFilename.addListener((item, suggest) => {
  seenEarly.add(item.id);
  setTimeout(() => seenEarly.delete(item.id), 60_000);
  let settled = false;
  // Let the browser continue with its own name (nothing taken, app missing, or timeout).
  const release = () => {
    if (settled) return;
    settled = true;
    try {
      suggest();
    } catch {
      /* download already gone */
    }
  };
  const timer = setTimeout(release, DECIDE_TIMEOUT_MS);
  decideAndCapture(item)
    .then(async (taken) => {
      if (!taken) return release();
      if (settled) return dropBrowserCopy(item.id); // the app answered after the timeout
      settled = true;
      await dropBrowserCopy(item.id);
    })
    .catch((e) => {
      console.warn('Peregrine: download decision failed', e);
      release();
    })
    .finally(() => clearTimeout(timer));
  return true; // we call suggest() asynchronously
});

/** Send a single URL to the app (context menu / popup). */
async function captureUrl({ url, referrer, pageUrl, filename, mime, size, kind }) {
  const c = await buildCapture({ url, referrer, pageUrl, filename, mime, size, kind });
  return callNative('capture', c);
}

// ---------- context menus ----------

function setupMenus() {
  chrome.contextMenus.removeAll(() => {
    chrome.contextMenus.create({ id: MENU_LINK, title: t('menuDownload'), contexts: ['link', 'image', 'video', 'audio'] });
    chrome.contextMenus.create({ id: MENU_ALL, title: t('menuDownloadAll'), contexts: ['page'] });
    chrome.contextMenus.create({ id: MENU_VIDEO, title: t('menuDownloadVideo'), contexts: ['page', 'video'] });
  });
}

function isWebUrl(u) {
  return /^(https?|ftps?):\/\//i.test(u || '');
}

/** Links Peregrine can take from a context menu: web/FTP links and BitTorrent magnet links. */
function isCapturable(u) {
  if (isWebUrl(u)) return true;
  if (!/^magnet:\?/i.test(u || '')) return false;
  return new URLSearchParams(u.slice(u.indexOf('?') + 1)).getAll('xt').some((x) => /^urn:bt(ih|mh):/i.test(x));
}

async function pageDialog(tabId, kind, message) {
  try {
    const [res] = await chrome.scripting.executeScript({
      target: { tabId },
      func: (k, m) => (k === 'confirm' ? window.confirm(m) : (window.alert(m), true)),
      args: [kind, message],
    });
    return res ? res.result : false;
  } catch {
    return false;
  }
}

async function downloadAllLinks(tab) {
  if (!tab || tab.id === undefined || !isWebUrl(tab.url)) return;
  let hrefs = [];
  try {
    const [res] = await chrome.scripting.executeScript({
      target: { tabId: tab.id },
      func: () => Array.from(document.links, (a) => a.href).slice(0, 5000),
    });
    hrefs = (res && res.result) || [];
  } catch (e) {
    console.warn('Peregrine: cannot read links on this page', e);
    return;
  }
  const settings = await loadSettings();
  const links = downloadableLinks(dedupeLinks(hrefs, tab.url), settings.extensions);
  if (links.length === 0) {
    await pageDialog(tab.id, 'alert', t('noLinks'));
    return;
  }
  if (!(await pageDialog(tab.id, 'confirm', t('confirmDownloadAll', [String(links.length)])))) return;
  for (const url of links) {
    const c = await buildCapture({ url, referrer: tab.url });
    try {
      await callNative('add', { url: c.url, headers: c.headers });
    } catch (err) {
      if (isUnavailable(err)) {
        await pageDialog(tab.id, 'alert', t('appUnavailable'));
        return;
      }
    }
  }
}

chrome.contextMenus.onClicked.addListener((info, tab) => {
  (async () => {
    if (info.menuItemId === MENU_LINK) {
      const url = info.linkUrl || info.srcUrl;
      if (!isCapturable(url)) return;
      try {
        await captureUrl({ url, referrer: info.pageUrl || (tab && tab.url) });
      } catch (err) {
        console.info('Peregrine: context-menu fallback to browser:', err.code || err.message);
        if (isWebUrl(url)) await browserDownload(url);
      }
    } else if (info.menuItemId === MENU_ALL) {
      await downloadAllLinks(tab);
    } else if (info.menuItemId === MENU_VIDEO) {
      const page = info.pageUrl || (tab && tab.url);
      if (!/^https?:\/\//i.test(page || '')) return;
      await captureUrl({ url: page, referrer: page, kind: 'video' }).catch(async (err) => {
        if (isUnavailable(err) && tab && tab.id !== undefined) await pageDialog(tab.id, 'alert', t('appUnavailable'));
      });
    }
  })().catch((e) => console.warn('Peregrine: menu action failed', e));
});

// ---------- media detection ----------

const mediaCache = new Map(); // tabId -> entries
const mediaKeyFor = (tabId) => `media:${tabId}`;

async function getTabMedia(tabId) {
  if (!mediaCache.has(tabId)) {
    const k = mediaKeyFor(tabId);
    const stored = (await chrome.storage.session.get(k))[k] || [];
    if (!mediaCache.has(tabId)) mediaCache.set(tabId, stored);
  }
  return mediaCache.get(tabId);
}

function setBadge(tabId, count) {
  chrome.action.setBadgeText({ tabId, text: count ? String(Math.min(count, 99)) : '' }).catch(() => {});
}

async function addTabMedia(tabId, entry) {
  const next = addMedia(await getTabMedia(tabId), entry);
  mediaCache.set(tabId, next);
  setBadge(tabId, next.length);
  await chrome.storage.session.set({ [mediaKeyFor(tabId)]: next });
}

async function clearTabMedia(tabId) {
  mediaCache.set(tabId, []);
  setBadge(tabId, 0);
  await chrome.storage.session.remove(mediaKeyFor(tabId));
}

const SKIP_TYPES = new Set(['image', 'stylesheet', 'script', 'font', 'ping', 'csp_report', 'websocket', 'webtransport', 'webbundle']);

chrome.webRequest.onHeadersReceived.addListener(
  (d) => {
    if (d.tabId < 0 || SKIP_TYPES.has(d.type)) return;
    (async () => {
      if (d.type === 'main_frame') await clearTabMedia(d.tabId);
      const size = responseSize(d.statusCode, header(d.responseHeaders, 'content-length'), header(d.responseHeaders, 'content-range'));
      const m = classifyMedia({ url: d.url, mime: header(d.responseHeaders, 'content-type'), size, type: d.type, statusCode: d.statusCode });
      if (m) await addTabMedia(d.tabId, { ...m, ts: Date.now() });
    })().catch(() => {});
  },
  { urls: ['<all_urls>'] },
  ['responseHeaders'],
);

chrome.tabs.onRemoved.addListener((tabId) => {
  mediaCache.delete(tabId);
  chrome.storage.session.remove(mediaKeyFor(tabId)).catch(() => {});
});

// ---------- messages from the popup / options page ----------

function fromOwnPage(sender) {
  return sender.id === chrome.runtime.id && typeof sender.url === 'string' && sender.url.startsWith(`chrome-extension://${chrome.runtime.id}/`);
}

async function onMessage(msg) {
  switch (msg && msg.type) {
    case 'getState': {
      const [settings, local, session] = await Promise.all([
        loadSettings(),
        chrome.storage.local.get('appMinSize'),
        chrome.storage.session.get('skipNext'),
      ]);
      return { settings, appMinSize: local.appMinSize ?? null, skipNext: !!session.skipNext };
    }
    case 'saveSettings':
      return { settings: await saveSettings(msg.settings || {}) };
    case 'setSkipNext':
      await chrome.storage.session.set({ skipNext: !!msg.value });
      return { skipNext: !!msg.value };
    case 'ping':
      try {
        const r = await callNative('ping', undefined, { timeoutMs: 8000 });
        if (r && r.app) refreshAppSettings();
        return { ok: true, ...r };
      } catch (err) {
        return { ok: false, error: err.code || 'error' };
      }
    case 'startApp':
      try {
        const r = await callNative('version');
        refreshAppSettings();
        return { ok: true, appVersion: r && r.version };
      } catch (err) {
        return { ok: false, error: err.code || 'error' };
      }
    case 'downloadPageVideo': {
      let page;
      try {
        page = (await chrome.tabs.get(Number(msg.tabId))).url;
      } catch {
        return { ok: false, error: 'not_found' };
      }
      if (!/^https?:\/\//i.test(page || '')) return { ok: false, error: 'not_a_page' };
      try {
        await captureUrl({ url: page, referrer: page, kind: 'video' });
        return { ok: true };
      } catch (err) {
        return { ok: false, error: err.code || 'error' };
      }
    }
    case 'getMedia':
      return { media: await getTabMedia(Number(msg.tabId)) };
    case 'downloadMedia': {
      const tabId = Number(msg.tabId);
      const entry = (await getTabMedia(tabId)).find((e) => e.url === msg.url);
      if (!entry) return { ok: false, error: 'not_found' };
      let pageUrl;
      try {
        pageUrl = (await chrome.tabs.get(tabId)).url;
      } catch {
        /* tab closed */
      }
      try {
        // Streams (HLS/DASH) go to the stream downloader; plain files to the normal engine.
        const kind = entry.kind === 'hls' || entry.kind === 'dash' ? entry.kind : undefined;
        await captureUrl({ url: entry.url, referrer: pageUrl, filename: kind ? undefined : entry.filename, mime: entry.mime, size: entry.size, kind });
        return { ok: true };
      } catch (err) {
        return { ok: false, error: err.code || 'error' };
      }
    }
  }
  return { ok: false, error: 'unknown_message' };
}

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (!fromOwnPage(sender)) return false;
  onMessage(msg).then(sendResponse, (e) => sendResponse({ ok: false, error: String(e && e.message) }));
  return true;
});

// ---------- lifecycle ----------

chrome.runtime.onInstalled.addListener(() => {
  setupMenus();
  chrome.action.setBadgeBackgroundColor({ color: '#2F6BFF' }).catch(() => {});
  updateTitle().catch(() => {});
  refreshAppSettings();
});

chrome.runtime.onStartup.addListener(() => {
  updateTitle().catch(() => {});
  refreshAppSettings();
});

// Exposed for debugging from the service worker console and for e2e tests.
globalThis.peregrine = { loadSettings, saveSettings, fileExtension };
