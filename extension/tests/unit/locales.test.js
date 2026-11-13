// Every locale in _locales must match English: same keys, same placeholders (names and
// contents), every $NAME$ the English message uses, the product name, and store limits.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const dir = join(dirname(fileURLToPath(import.meta.url)), '..', '..', '_locales');
const load = (code) => JSON.parse(readFileSync(join(dir, code, 'messages.json'), 'utf8'));
const en = load('en');
const codes = readdirSync(dir).filter((c) => c !== 'en').sort();
const refs = (s) => [...s.matchAll(/\$([A-Za-z0-9_]+)\$/g)].map((m) => m[1].toLowerCase()).sort();

// The desktop app's languages (desktop/frontend/src/i18n), in Chrome's locale codes.
const EXPECTED = ['ar', 'de', 'es', 'fa', 'fr', 'hi', 'id', 'it', 'ja', 'ko', 'pt_BR', 'ru', 'tr', 'ur', 'zh_CN'];

test('the extension ships every desktop app language', () => {
  assert.deepEqual(codes, EXPECTED);
});

for (const code of codes) {
  test(`${code} matches the English keys and placeholders`, () => {
    const loc = load(code);
    assert.deepEqual(Object.keys(loc).sort(), Object.keys(en).sort());
    for (const [k, base] of Object.entries(en)) {
      const m = loc[k];
      assert.equal(typeof m.message, 'string', `${code}.${k}`);
      assert.ok(m.message.trim(), `${code}.${k} is empty`);
      assert.deepEqual(m.placeholders || {}, base.placeholders || {}, `${code}.${k} placeholders`);
      assert.deepEqual(refs(m.message), refs(base.message), `${code}.${k} uses different $NAME$s`);
      if (base.message.includes('Peregrine')) assert.ok(m.message.includes('Peregrine'), `${code}.${k} lost "Peregrine"`);
    }
    // Chrome Web Store limits for the manifest name and description.
    assert.ok([...loc.extName.message].length <= 75, `${code}.extName is too long`);
    assert.ok([...loc.extDescription.message].length <= 132, `${code}.extDescription is too long`);
  });
}
