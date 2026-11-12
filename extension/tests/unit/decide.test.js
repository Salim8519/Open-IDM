import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  shouldIntercept, normalizeSettings, effectiveMinSize, fileExtension, mimeExtension,
  parseExtensionList, parseSiteList, hostMatches, isExcluded,
} from '../../src/decide.js';
import { DEFAULT_SETTINGS } from '../../src/defaults.js';

const S = normalizeSettings({});
const MB = 1024 * 1024;
const ctx = { selfId: 'self', minSize: MB, recentFallback: new Set() };
const item = (o) => ({ url: 'https://example.com/file.zip', state: 'in_progress', referrer: 'https://example.com/', ...o });

test('fileExtension handles URLs, names, queries and junk', () => {
  assert.equal(fileExtension('https://x.com/a/b/Setup.EXE?x=1#y'), 'exe');
  assert.equal(fileExtension('C:\\Users\\me\\Downloads\\movie.mkv'), 'mkv');
  assert.equal(fileExtension('https://x.com/dir.v2/download'), '');
  assert.equal(fileExtension('https://x.com/archive.tar.gz'), 'gz');
  assert.equal(fileExtension('https://x.com/%E2%9C%93.pdf'), 'pdf');
  assert.equal(fileExtension('https://x.com/bad%E0%A4%A.zip'), 'zip');
  assert.equal(fileExtension('.hidden'), '');
  assert.equal(fileExtension('trailing.'), '');
  assert.equal(fileExtension(''), '');
  assert.equal(fileExtension(undefined), '');
  assert.equal(fileExtension('https://x.com/a.thisistoolongext'), '');
});

test('mimeExtension maps known types only', () => {
  assert.equal(mimeExtension('application/zip'), 'zip');
  assert.equal(mimeExtension('Video/MP4; codecs="avc1"'), 'mp4');
  assert.equal(mimeExtension('text/html'), '');
  assert.equal(mimeExtension(''), '');
});

test('captures by file type regardless of size', () => {
  assert.deepEqual(shouldIntercept(item(), S, ctx), { capture: true, reason: 'type' });
  assert.equal(shouldIntercept(item({ url: 'https://x.com/dl?id=1', filename: 'C:\\d\\report.PDF' }), S, ctx).reason, 'type');
  assert.equal(shouldIntercept(item({ url: 'https://x.com/dl?id=1', mime: 'application/x-msdownload' }), S, ctx).reason, 'type');
  assert.equal(shouldIntercept(item({ finalUrl: 'https://cdn.x.com/real.iso', url: 'https://x.com/go' }), S, ctx).reason, 'type');
});

test('captures by size when the type is unknown', () => {
  const unk = { url: 'https://x.com/dl?id=1', mime: 'application/octet-stream' };
  assert.equal(shouldIntercept(item({ ...unk, totalBytes: 2 * MB }), S, ctx).capture, true);
  assert.equal(shouldIntercept(item({ ...unk, fileSize: MB }), S, ctx).reason, 'size');
  assert.deepEqual(shouldIntercept(item({ ...unk, totalBytes: MB - 1 }), S, ctx), { capture: false, reason: 'small' });
  // unknown size + unknown type: leave it to the browser
  assert.deepEqual(shouldIntercept(item({ ...unk, totalBytes: 0, fileSize: -1 }), S, ctx), { capture: false, reason: 'unknown' });
  assert.equal(shouldIntercept(item({ url: 'https://x.com/page.html', totalBytes: 10 }), S, ctx).reason, 'small');
});

test('skips non-web schemes', () => {
  for (const url of ['blob:https://x.com/123', 'data:application/zip;base64,AAA', 'file:///C:/a.zip', 'filesystem:https://x.com/temporary/a.zip', 'chrome://downloads', 'javascript:void(0)']) {
    assert.equal(shouldIntercept(item({ url }), S, ctx).reason, 'scheme', url);
  }
  assert.equal(shouldIntercept(item({ url: 'ftp://x.com/a.zip' }), S, ctx).capture, true);
  assert.equal(shouldIntercept(item({ url: 'not a url' }), S, ctx).reason, 'invalid');
  assert.equal(shouldIntercept(null, S, ctx).reason, 'invalid');
});

test('disabled, own downloads, fallback memory, skip-next and state', () => {
  assert.equal(shouldIntercept(item(), normalizeSettings({ enabled: false }), ctx).reason, 'disabled');
  assert.equal(shouldIntercept(item({ byExtensionId: 'self' }), S, ctx).reason, 'own-download');
  assert.equal(shouldIntercept(item({ byExtensionId: 'other' }), S, ctx).capture, true);
  const fb = { ...ctx, recentFallback: new Set(['https://example.com/file.zip']) };
  assert.equal(shouldIntercept(item(), S, fb).reason, 'fallback');
  assert.equal(shouldIntercept(item(), S, { ...ctx, skipNext: true }).reason, 'skip-next');
  // skip-next is not consumed by downloads we would not capture anyway
  assert.equal(shouldIntercept(item({ url: 'blob:https://x/1' }), S, { ...ctx, skipNext: true }).reason, 'scheme');
  assert.equal(shouldIntercept(item({ state: 'complete' }), S, ctx).reason, 'not-in-progress');
  assert.equal(shouldIntercept(item({ state: 'interrupted' }), S, ctx).reason, 'not-in-progress');
});

test('excluded sites match host, subdomains and referrer', () => {
  const s = normalizeSettings({ excludedSites: ['example.com'] });
  assert.equal(shouldIntercept(item(), s, ctx).reason, 'excluded');
  assert.equal(shouldIntercept(item({ url: 'https://dl.example.com/a.zip', referrer: '' }), s, ctx).reason, 'excluded');
  assert.equal(shouldIntercept(item({ url: 'https://cdn.other.net/a.zip', referrer: 'https://www.example.com/page' }), s, ctx).reason, 'excluded');
  assert.equal(shouldIntercept(item({ url: 'https://notexample.com/a.zip', referrer: '' }), s, ctx).capture, true);
  assert.equal(hostMatches('a.b.example.com', 'example.com'), true);
  assert.equal(hostMatches('badexample.com', 'example.com'), false);
  assert.equal(hostMatches('EXAMPLE.com.', 'example.com'), true);
  assert.equal(isExcluded(['::bad::'], ['example.com']), false);
});

test('parsers clean user input', () => {
  assert.deepEqual(parseExtensionList('ZIP, .rar;*.7z\nmp4 mp4 bad/ext toolongextension'), ['zip', 'rar', '7z', 'mp4']);
  assert.deepEqual(
    parseSiteList('Example.com\n https://www.Foo.org/path?q=1 \n*.cdn.net\n# comment\n\nnot a site\nlocalhost\nnodot:8080'),
    ['example.com', 'www.foo.org', 'cdn.net', 'localhost'],
  );
});

test('normalizeSettings fills defaults and rejects junk', () => {
  assert.deepEqual(normalizeSettings(undefined), { ...DEFAULT_SETTINGS, extensions: [...DEFAULT_SETTINGS.extensions] });
  const n = normalizeSettings({ enabled: 0, extensions: 'zip', excludedSites: null, minSizeMB: '-3' });
  assert.equal(n.enabled, true); // only an explicit false disables
  assert.deepEqual(n.extensions, DEFAULT_SETTINGS.extensions);
  assert.deepEqual(n.excludedSites, []);
  assert.equal(n.minSizeMB, null);
  assert.equal(normalizeSettings({ minSizeMB: '2.5' }).minSizeMB, 2.5);
  assert.equal(normalizeSettings({ enabled: false }).enabled, false);
});

test('effectiveMinSize: user override, then app value, then 1 MB', () => {
  assert.equal(effectiveMinSize(normalizeSettings({ minSizeMB: 5 }), 123), 5 * MB);
  assert.equal(effectiveMinSize(normalizeSettings({ minSizeMB: 0 }), 123), 0);
  assert.equal(effectiveMinSize(S, 3 * MB), 3 * MB);
  assert.equal(effectiveMinSize(S, null), MB);
  assert.equal(effectiveMinSize(S, 0), MB);
});

test('minSize 0 from context falls back to the default', () => {
  const unk = item({ url: 'https://x.com/dl', totalBytes: 10 });
  assert.equal(shouldIntercept(unk, S, { ...ctx, minSize: 0 }).reason, 'small');
});
