import { localize, t, formatSize, send } from './i18n.js';
import { DEFAULT_SETTINGS, DEFAULT_MIN_SIZE } from './defaults.js';
import { parseExtensionList, parseSiteList } from './decide.js';

localize();

const $ = (id) => document.getElementById(id);

function fill(settings, appMinSize) {
  $('enabled').checked = settings.enabled;
  $('minSize').value = settings.minSizeMB ?? '';
  $('minSize').placeholder = String(+((appMinSize || DEFAULT_MIN_SIZE) / 1048576).toFixed(2));
  $('minSizeHint').textContent = t('optMinSizeHint', [formatSize(appMinSize || DEFAULT_MIN_SIZE)]);
  $('extensions').value = settings.extensions.join(' ');
  $('excluded').value = settings.excludedSites.join('\n');
}

function flash() {
  $('saved').textContent = t('saved');
  setTimeout(() => ($('saved').textContent = ''), 2000);
}

let appMinSize = null;

async function load() {
  const s = await send({ type: 'getState' });
  appMinSize = s.appMinSize;
  fill(s.settings, appMinSize);
}

$('form').addEventListener('submit', async (e) => {
  e.preventDefault();
  const raw = $('minSize').value.trim();
  const { settings } = await send({
    type: 'saveSettings',
    settings: {
      enabled: $('enabled').checked,
      minSizeMB: raw === '' ? null : Number(raw),
      extensions: parseExtensionList($('extensions').value),
      excludedSites: parseSiteList($('excluded').value),
    },
  });
  fill(settings, appMinSize);
  flash();
});

$('reset').addEventListener('click', async () => {
  const { settings } = await send({ type: 'saveSettings', settings: { ...DEFAULT_SETTINGS } });
  fill(settings, appMinSize);
  flash();
});

load();
