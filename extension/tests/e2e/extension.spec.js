// End-to-end: real extension + real native messaging host + fake Peregrine app.
//
// Registers com.peregrine.host for the test browser (Windows: HKCU keys for Chrome and
// Chromium, restored afterwards), so run it on a dev machine or CI, not in parallel with
// a real Peregrine install that you care about.
import { test, expect, chromium } from '@playwright/test';
import { join } from 'node:path';
import { mkdirSync, writeFileSync } from 'node:fs';
import {
  EXTENSION_ID, extDir, makeTemp, buildBinaries, registerHost, startFakeApp, startSite, removeTemp,
} from './harness.js';

test.describe.configure({ mode: 'serial' });

let dirs;
let bins;
let unregister = () => {};
let site;
let app;
let context;
let sw;

test.beforeAll(async () => {
  test.setTimeout(240_000);
  dirs = makeTemp();
  bins = buildBinaries(dirs.bin);
  unregister = registerHost(bins.hostExe, dirs);
  site = await startSite();
  app = await startFakeApp(bins.fakeExe, dirs);
  // Downloads the browser keeps go to a temp folder, never the real Downloads folder.
  mkdirSync(join(dirs.profile, 'Default'), { recursive: true });
  writeFileSync(
    join(dirs.profile, 'Default', 'Preferences'),
    JSON.stringify({ download: { default_directory: join(dirs.home, 'browser-downloads'), prompt_for_download: false } }),
  );
  context = await chromium.launchPersistentContext(dirs.profile, {
    channel: 'chromium', // full Chromium in new headless mode: supports extensions
    headless: !process.env.HEADED,
    args: [`--disable-extensions-except=${extDir}`, `--load-extension=${extDir}`],
    env: { ...process.env, PEREGRINE_HOME: dirs.home },
    acceptDownloads: true,
  });
  sw = context.serviceWorkers().find((w) => w.url().startsWith('chrome-extension://'));
  if (!sw) sw = await context.waitForEvent('serviceworker');
});

test.afterAll(async () => {
  await context?.close().catch(() => {});
  await app?.stop().catch(() => {});
  await site?.close().catch(() => {});
  try {
    unregister();
  } finally {
    if (dirs) removeTemp(dirs);
  }
});

const searchDownloads = (pattern) =>
  sw.evaluate((p) => chrome.downloads.search({ urlRegex: p }), pattern);

test('extension loads with the fixed ID', async () => {
  expect(new URL(sw.url()).host).toBe(EXTENSION_ID);
});

test('popup shows the connection to the app', async () => {
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.goto(`chrome-extension://${EXTENSION_ID}/popup.html`);
  await expect(page.locator('#statusText')).toHaveText('Connected to Peregrine v9.9.9-fake');
  await expect(page.locator('#enabled')).toBeChecked();
  expect(errors).toEqual([]);
  await page.close();
});

test('options page loads and saves', async () => {
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.goto(`chrome-extension://${EXTENSION_ID}/options.html`);
  await expect(page.locator('#extensions')).toHaveValue(/\bzip\b/);
  await page.fill('#excluded', 'excluded.example\n');
  await page.click('button[type=submit]');
  await expect(page.locator('#saved')).toHaveText('Saved');
  const stored = await sw.evaluate(() => chrome.storage.local.get('settings'));
  expect(stored.settings.excludedSites).toEqual(['excluded.example']);
  expect(errors).toEqual([]);
  await page.close();
});

test('clicking a .zip link hands it to Peregrine with cookies and referer', async () => {
  const page = await context.newPage();
  await page.goto(site.base + '/');
  await page.click('#zip');

  const url = site.base + '/files/test.zip';
  await expect.poll(() => app.calls().filter((c) => c.method === 'capture').length, { timeout: 20_000 }).toBe(1);
  const cap = app.calls().find((c) => c.method === 'capture').params;
  expect(cap.url).toBe(url);
  expect(cap.source).toBe('extension');
  expect(cap.filename).toBeFalsy(); // not known yet at onCreated; the app probes it
  expect(cap.headers.Referer).toBe(site.base + '/');
  expect(cap.headers.Cookie).toBe('sess=abc123');
  expect(cap.headers['User-Agent']).toMatch(/Mozilla\/5\.0/);
  expect(cap.pageUrl).toBe(site.base + '/');

  // The browser must not keep its own copy.
  await expect.poll(async () => (await searchDownloads('test\\.zip')).length, { timeout: 10_000 }).toBe(0);
  await page.close();
});

test('a captured download never shows up as a browser download first', async () => {
  // Record every file name the browser gives a download. A name means the browser has
  // committed to its own download (bubble/shelf, Save as dialog, file on disk).
  await sw.evaluate(() => {
    globalThis.__urls = {};
    globalThis.__named = [];
    chrome.downloads.onCreated.addListener((d) => {
      globalThis.__urls[d.id] = d.url;
    });
    chrome.downloads.onChanged.addListener((d) => {
      if (d.filename && d.filename.current) globalThis.__named.push(d.id);
    });
  });
  const before = app.calls().filter((c) => c.method === 'capture').length;
  const page = await context.newPage();
  // Use the browser's normal download handling, as in Brave or Chrome (Playwright otherwise
  // handles downloads itself, which skips the extension's naming step).
  const cdp = await context.newCDPSession(page);
  await cdp.send('Browser.setDownloadBehavior', { behavior: 'default' });
  await page.goto(site.base + '/');
  await page.click('#zip');
  await expect.poll(() => app.calls().filter((c) => c.method === 'capture').length, { timeout: 20_000 }).toBe(before + 1);
  await expect.poll(async () => (await searchDownloads('test[.]zip')).length, { timeout: 10_000 }).toBe(0);
  const { urls, named } = await sw.evaluate(() => ({ urls: globalThis.__urls, named: globalThis.__named }));
  const ours = Object.keys(urls).filter((id) => /test[.]zip/.test(urls[id])).map(Number);
  expect(ours.length).toBe(1);
  expect(named.filter((id) => ours.includes(id))).toEqual([]);
  await page.close();
});

test('media responses are listed per tab, streams flagged', async () => {
  const page = await context.newPage();
  await page.goto(site.base + '/media.html');
  const media = async () => {
    const all = await sw.evaluate(() => chrome.storage.session.get(null));
    return Object.entries(all).filter(([k]) => k.startsWith('media:')).flatMap(([, v]) => v);
  };
  await expect.poll(async () => (await media()).map((m) => `${m.kind}:${m.filename}`).sort(), { timeout: 15_000 })
    .toEqual(['file:clip.mp4', 'hls:index.m3u8']);
  await page.close();
});

test('without the app, the browser keeps the download (fallback)', async () => {
  await app.stop();
  const before = app.calls().length;
  const page = await context.newPage();
  await page.goto(site.base + '/');
  await page.click('#fallback');
  await expect
    .poll(async () => (await searchDownloads('fallback\\.zip')).map((d) => d.state).join(','), { timeout: 30_000 })
    .toBe('complete');
  const [item] = await searchDownloads('fallback\\.zip');
  expect(item.bytesReceived).toBe(24 * 64 * 1024);
  expect(app.calls().length).toBe(before);
  await page.close();
});

test('popup offers to start the app when it is not running', async () => {
  const page = await context.newPage();
  await page.goto(`chrome-extension://${EXTENSION_ID}/popup.html`);
  await expect(page.locator('#statusText')).toHaveText('App not running');
  await expect(page.locator('#startBtn')).toBeVisible();
  await page.close();
});

test('host log never contains query strings or cookies', async () => {
  const { readFileSync } = await import('node:fs');
  const log = readFileSync(join(dirs.home, 'peregrine-host.log'), 'utf8');
  expect(log).toContain('capture');
  expect(log).not.toContain('abc123');
});
