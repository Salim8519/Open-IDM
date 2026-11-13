// Makes the Chrome Web Store / Edge Add-ons images in docs/store/images:
// three 1280x800 screenshots of the real extension and app, a 440x280 tile and a 1400x560
// marquee. Run from extension/:  node tests/store/shots.mjs
//
// Like the e2e tests it registers the native messaging host for the test browser (restored
// afterwards). The app side is the fake app from the host tests, reporting the real version.
import { chromium } from '@playwright/test';
import { spawn } from 'node:child_process';
import { createServer } from 'node:http';
import { mkdirSync, readFileSync, rmSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { EXTENSION_ID, extDir, makeTemp, buildBinaries, registerHost, removeTemp } from '../e2e/harness.js';

const repo = resolve(extDir, '..');
const out = join(repo, 'docs', 'store', 'images');
const version = JSON.parse(readFileSync(join(extDir, 'manifest.json'), 'utf8')).version;
const dataURL = (file, mime = 'image/png') => `data:${mime};base64,${readFileSync(file).toString('base64')}`;
const logo = dataURL(join(repo, 'assets', 'logo', 'peregrine.svg'), 'image/svg+xml');

// A small site with a video and a live stream, so the popup has something to list.
function startSite() {
  const page = `<!doctype html><meta charset="utf-8"><title>Product tour</title>
<video src="/media/product-tour-1080p.mp4" preload="auto" muted></video>
<audio src="/media/podcast-episode-42.mp3" preload="auto"></audio>
<script>fetch('/media/live/master.m3u8')</script>`;
  const server = createServer((req, res) => {
    const u = new URL(req.url, 'http://x');
    const types = { '.mp4': ['video/mp4', 48_600_000], '.mp3': ['audio/mpeg', 31_400_000], '.m3u8': ['application/vnd.apple.mpegurl', 420] };
    const ext = Object.keys(types).find((e) => u.pathname.endsWith(e));
    if (!ext) {
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      return res.end(page);
    }
    const [mime, size] = types[ext];
    // Headers announce the real size; the body is cut short, enough for the browser to probe.
    res.writeHead(200, { 'Content-Type': mime, 'Content-Length': String(size) });
    res.end(Buffer.alloc(1024));
  });
  return new Promise((r) => server.listen(0, '127.0.0.1', () => r({ server, base: `http://127.0.0.1:${server.address().port}` })));
}

function startApp(fakeExe, home) {
  const child = spawn(fakeExe, ['-version', version], { env: { ...process.env, PEREGRINE_HOME: home }, stdio: ['pipe', 'pipe', 'inherit'] });
  return new Promise((res, rej) => {
    child.stdout.on('data', (d) => String(d).includes('READY') && res(child));
    child.on('exit', (c) => rej(new Error(`fakeapp exited ${c}`)));
  });
}

// One store screenshot: headline on the left, the real UI capture on the right.
function frame(title, body, shot, wide) {
  return `<!doctype html><meta charset="utf-8"><style>
  *{box-sizing:border-box;margin:0}
  body{width:1280px;height:800px;background:#0f1013;color:#ececee;font-family:"Segoe UI Variable Text","Segoe UI",system-ui,sans-serif;
    display:grid;grid-template-columns:${wide ? '330px 1fr' : '470px 1fr'};align-items:center;overflow:hidden}
  .copy{padding:0 36px 0 64px}
  .brand{display:flex;align-items:center;gap:10px;font-size:17px;font-weight:600;color:#9a9aa2;margin-bottom:28px}
  .brand img{width:30px;height:30px}
  h1{font-size:${wide ? 32 : 38}px;line-height:1.15;font-weight:650;letter-spacing:-.01em;margin-bottom:16px}
  p{font-size:18px;line-height:1.5;color:#9a9aa2}
  .shot{display:flex;justify-content:center;align-items:center;height:100%;padding:${wide ? '40px 40px 40px 0' : '0 64px 0 0'}}
  .shot img{max-width:100%;max-height:720px;border-radius:8px;box-shadow:0 20px 60px rgba(0,0,0,.55),0 0 0 1px #2d2d32}
</style><div class="copy"><div class="brand"><img src="${logo}">Peregrine</div><h1>${title}</h1><p>${body}</p></div>
<div class="shot"><img src="${shot}"></div>`;
}

const tile = (w, h, big) => `<!doctype html><meta charset="utf-8"><style>
  *{margin:0}body{width:${w}px;height:${h}px;background:linear-gradient(135deg,#0b1020 0%,#132457 100%);color:#fff;
  font-family:"Segoe UI Variable Text","Segoe UI",system-ui,sans-serif;display:flex;align-items:center;gap:${big ? 40 : 22}px;padding:0 ${big ? 110 : 36}px;overflow:hidden}
  img{width:${big ? 190 : 104}px;height:${big ? 190 : 104}px}
  h1{font-size:${big ? 64 : 36}px;font-weight:650;letter-spacing:-.01em}
  p{font-size:${big ? 26 : 16}px;color:#b9c6ea;margin-top:${big ? 10 : 6}px;line-height:1.35}
</style><img src="${logo}"><div><h1>Peregrine</h1><p>Fast, free download manager.<br>Your browser's downloads, sped up.</p></div>`;

async function render(browser, html, w, h, file) {
  const page = await browser.newPage();
  await page.setViewportSize({ width: w, height: h });
  await page.setContent(html, { waitUntil: 'load' });
  await page.screenshot({ path: join(out, file) });
  await page.close();
}

mkdirSync(out, { recursive: true });
const dirs = makeTemp();
let unregister = () => {};
let app;
let site;
let context;
try {
  const bins = buildBinaries(dirs.bin);
  unregister = registerHost(bins.hostExe, dirs);
  app = await startApp(bins.fakeExe, dirs.home);
  site = await startSite();
  context = await chromium.launchPersistentContext(dirs.profile, {
    channel: 'chromium',
    headless: true,
    args: [`--disable-extensions-except=${extDir}`, `--load-extension=${extDir}`],
    env: { ...process.env, PEREGRINE_HOME: dirs.home },
    colorScheme: 'dark',
  });
  let sw = context.serviceWorkers().find((w) => w.url().startsWith('chrome-extension://'));
  if (!sw) sw = await context.waitForEvent('serviceworker');

  // Popup with media found on a page. The popup asks for "the active tab"; when opened as a
  // page for the capture, it is pointed at the media tab instead.
  const media = await context.newPage();
  await media.goto(site.base + '/');
  await media.waitForTimeout(2500);
  const tabId = await sw.evaluate(async (u) => (await chrome.tabs.query({ url: u + '/' }))[0].id, site.base);
  const popup = await context.newPage();
  await popup.setViewportSize({ width: 364, height: 600 });
  await popup.addInitScript((id) => {
    const q = chrome.tabs.query.bind(chrome.tabs);
    chrome.tabs.query = async (info) => (info && info.active ? [await chrome.tabs.get(id)] : q(info));
  }, tabId);
  await popup.goto(`chrome-extension://${EXTENSION_ID}/popup.html`);
  await popup.locator('#statusText').filter({ hasText: 'Connected' }).waitFor();
  await popup.waitForTimeout(800);
  const popupShot = join(dirs.tmp, 'popup.png');
  await popup.locator('body').screenshot({ path: popupShot, scale: 'device' });

  const options = await context.newPage();
  await options.setViewportSize({ width: 820, height: 900 });
  await options.goto(`chrome-extension://${EXTENSION_ID}/options.html`);
  await options.waitForTimeout(800);
  const optionsShot = join(dirs.tmp, 'options.png');
  await options.screenshot({ path: optionsShot });

  const r = (html, w, h, f) => render(context, html, w, h, f);
  await r(frame('Your browser’s downloads, sped up', 'Peregrine takes over downloads from Chrome and Edge and splits each file into many connections. Pause, resume, and never lose progress.', dataURL(join(repo, 'docs', 'screenshots', 'main-dark.png')), true), 1280, 800, 'screenshot-1-app.png');
  await r(frame('Grab video and audio from the page', 'The toolbar button lists media found on the page. One click sends it to Peregrine, with your login cookies.', dataURL(popupShot)), 1280, 800, 'screenshot-2-popup.png');
  await r(frame('You choose what it takes over', 'File types, minimum size and sites to skip. “Skip next download” hands one file back to the browser.', dataURL(optionsShot)), 1280, 800, 'screenshot-3-options.png');
  await r(tile(440, 280, false), 440, 280, 'promo-small-440x280.png');
  await r(tile(1400, 560, true), 1400, 560, 'promo-marquee-1400x560.png');
  console.log('store images written to', out);
} finally {
  await context?.close().catch(() => {});
  if (app && app.exitCode === null) {
    app.stdin.end();
    app.kill();
  }
  site?.server.close();
  try {
    unregister();
  } finally {
    rmSync(join(dirs.home, 'ipc.json'), { force: true });
    removeTemp(dirs);
  }
}
