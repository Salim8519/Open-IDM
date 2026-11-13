import { localize, t, formatSize, send } from './i18n.js';

localize();

const $ = (id) => document.getElementById(id);

async function refreshStatus() {
  const dot = $('statusDot');
  const text = $('statusText');
  const start = $('startBtn');
  const r = await send({ type: 'ping' });
  start.hidden = true;
  if (r && r.ok && r.app) {
    dot.className = 'dot ok';
    text.textContent = t('statusConnected', [r.appVersion || '?']);
  } else if (r && r.ok) {
    dot.className = 'dot warn';
    text.textContent = t('statusNotRunning');
    start.hidden = false;
  } else {
    dot.className = 'dot err';
    text.textContent = t('statusNotInstalled');
  }
  document.body.dataset.status = dot.className.replace('dot ', '');
}

$('startBtn').addEventListener('click', async () => {
  const btn = $('startBtn');
  btn.disabled = true;
  btn.textContent = t('starting');
  await send({ type: 'startApp' });
  btn.disabled = false;
  btn.textContent = t('startApp');
  await refreshStatus();
});

async function loadToggles() {
  const s = await send({ type: 'getState' });
  $('enabled').checked = s.settings.enabled;
  $('skipNext').checked = s.skipNext;
}

$('enabled').addEventListener('change', (e) => send({ type: 'saveSettings', settings: { enabled: e.target.checked } }));
$('skipNext').addEventListener('change', (e) => send({ type: 'setSkipNext', value: e.target.checked }));

$('optionsLink').addEventListener('click', (e) => {
  e.preventDefault();
  chrome.runtime.openOptionsPage();
  window.close();
});

async function loadMedia() {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  if (!tab || tab.id === undefined) return;
  const { media = [] } = await send({ type: 'getMedia', tabId: tab.id });
  const list = $('mediaList');
  list.replaceChildren();
  $('mediaEmpty').hidden = media.length > 0;
  for (const m of [...media].reverse()) {
    const li = document.createElement('li');
    const info = document.createElement('div');
    info.className = 'media-info';
    const name = document.createElement('div');
    name.className = 'media-name';
    name.textContent = m.filename;
    name.title = m.url;
    const meta = document.createElement('div');
    meta.className = 'media-meta';
    if (m.kind !== 'file') {
      const tag = document.createElement('span');
      tag.className = 'tag';
      tag.textContent = m.kind.toUpperCase();
      meta.append(tag);
    }
    meta.append([m.mime, formatSize(m.size)].filter(Boolean).join(' · '));
    info.append(name, meta);

    const btn = document.createElement('button');
    btn.textContent = t('download');
    btn.className = 'primary';
    btn.addEventListener('click', async () => {
      btn.disabled = true;
      const r = await send({ type: 'downloadMedia', tabId: tab.id, url: m.url });
      btn.textContent = r && r.ok ? t('sent') : t('failed');
      if (!(r && r.ok)) {
        btn.disabled = false;
        refreshStatus();
      }
    });
    li.append(info, btn);
    list.append(li);
  }
}

// "Download the video on this page" (YouTube and other video sites, via the app's video downloader).
async function setupPageVideo() {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  const btn = $('pageVideoBtn');
  if (!tab || tab.id === undefined || !/^https?:\/\//i.test(tab.url || '')) return;
  btn.hidden = false;
  btn.addEventListener('click', async () => {
    btn.disabled = true;
    const r = await send({ type: 'downloadPageVideo', tabId: tab.id });
    btn.textContent = r && r.ok ? t('sent') : t('failed');
    if (!(r && r.ok)) {
      btn.disabled = false;
      refreshStatus();
    }
  });
}

loadToggles();
setupPageVideo();
loadMedia();
refreshStatus();
