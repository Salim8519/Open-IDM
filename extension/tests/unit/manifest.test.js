// Static checks: manifest sanity (locale parity is in locales.test.js), every i18n key used is defined,
// no remote code, and the fixed extension ID matches the "key".
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, existsSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (p) => readFileSync(join(root, p), 'utf8');
const manifest = JSON.parse(read('manifest.json'));
const en = JSON.parse(read('_locales/en/messages.json'));
export const EXTENSION_ID = 'bcnjpgddmjlffmcfgnfjaadkdjphdhfd';

test('extension ID derives from the manifest key', () => {
  const der = Buffer.from(manifest.key, 'base64');
  const hex = createHash('sha256').update(der).digest('hex').slice(0, 32);
  const id = [...hex].map((c) => String.fromCharCode(97 + parseInt(c, 16))).join('');
  assert.equal(id, EXTENSION_ID);
  // The host allowlist must contain the same ID.
  const origin = readFileSync(join(root, '..', 'cmd', 'peregrine-host', 'origin.go'), 'utf8');
  assert.ok(origin.includes(`"${EXTENSION_ID}"`), 'host allowlist is missing the extension ID');
});

test('manifest is MV3 with the expected permissions and files', () => {
  assert.equal(manifest.manifest_version, 3);
  assert.equal(manifest.default_locale, 'en');
  assert.deepEqual([...manifest.permissions].sort(), ['contextMenus', 'cookies', 'downloads', 'nativeMessaging', 'scripting', 'storage', 'webRequest']);
  assert.deepEqual(manifest.host_permissions, ['<all_urls>']);
  assert.match(manifest.content_security_policy.extension_pages, /script-src 'self';/);
  for (const p of [manifest.background.service_worker, manifest.action.default_popup, manifest.options_ui.page,
    ...Object.values(manifest.icons), ...Object.values(manifest.action.default_icon)]) {
    assert.ok(existsSync(join(root, p)), `missing ${p}`);
  }
});

test('every i18n key referenced in code and HTML exists', () => {
  const files = ['popup.html', 'options.html', 'manifest.json', ...readdirSync(join(root, 'src')).map((f) => `src/${f}`)];
  const used = new Set();
  for (const f of files) {
    const s = read(f);
    for (const m of s.matchAll(/data-i18n(?:-title|-placeholder)?="([A-Za-z0-9_]+)"/g)) used.add(m[1]);
    for (const m of s.matchAll(/__MSG_([A-Za-z0-9_]+)__/g)) used.add(m[1]);
    for (const m of s.matchAll(/\bt\('([A-Za-z0-9_]+)'/g)) used.add(m[1]);
  }
  assert.ok(used.size > 20);
  for (const k of used) assert.ok(en[k], `missing message "${k}"`);
});

test('no remote code or inline scripts', () => {
  for (const f of ['popup.html', 'options.html']) {
    const s = read(f);
    assert.doesNotMatch(s, /<script(?![^>]*\bsrc="src\/)/, `${f} has an inline or foreign script`);
    assert.doesNotMatch(s, /https?:\/\//, `${f} references a remote resource`);
    assert.doesNotMatch(s, /\son[a-z]+=/i, `${f} has an inline event handler`);
  }
  for (const f of readdirSync(join(root, 'src'))) {
    const s = read(`src/${f}`);
    assert.doesNotMatch(s, /\beval\(|new Function\(|importScripts\(|\bimport\(\s*['"`]https?:/, `${f} loads code dynamically`);
    assert.doesNotMatch(s, /\bfetch\(|XMLHttpRequest|WebSocket\(/, `${f} makes network requests`);
  }
});
