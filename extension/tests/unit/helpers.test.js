import { test } from 'node:test';
import assert from 'node:assert/strict';
import { buildCookieHeader } from '../../src/cookies.js';
import { classifyMedia, responseSize, header, addMedia, mediaKey, MAX_MEDIA_PER_TAB, MIN_MEDIA_SIZE } from '../../src/media.js';
import { dedupeLinks, downloadableLinks, MAX_LINKS } from '../../src/links.js';

test('buildCookieHeader orders by path length and skips bad cookies', () => {
  assert.equal(buildCookieHeader([]), '');
  assert.equal(buildCookieHeader(undefined), '');
  const h = buildCookieHeader([
    { name: 'a', value: '1', path: '/' },
    { name: 'b', value: '2', path: '/deep/path' },
    { name: 'c', value: '3', path: '/deep' },
    { name: 'a', value: 'dup', path: '/' },
    { name: 'evil', value: 'x;y', path: '/' },
    { name: 'nl', value: 'x\r\nSet: 1', path: '/' },
    { name: 'nopath', value: '4' },
    { name: '', value: 'bare', path: '/' },
    null,
    { name: 5, value: 'num' },
  ]);
  assert.equal(h, 'b=2; c=3; a=1; nopath=4; bare');
});

test('responseSize prefers Content-Range total', () => {
  assert.equal(responseSize(206, '2', 'bytes 0-1/12345'), 12345);
  assert.equal(responseSize(206, '2', ''), 0);
  assert.equal(responseSize(200, '999', ''), 999);
  assert.equal(responseSize(200, '', ''), 0);
  assert.equal(responseSize(200, 'abc', undefined), 0);
});

test('header lookup is case-insensitive', () => {
  assert.equal(header([{ name: 'Content-Type', value: 'video/mp4' }], 'content-type'), 'video/mp4');
  assert.equal(header(undefined, 'content-type'), '');
  assert.equal(header([{ name: 'X' }], 'x'), '');
});

test('classifyMedia: direct files, streams, segments and noise', () => {
  const big = 5 * 1024 * 1024;
  assert.equal(classifyMedia({ url: 'https://v.com/a/movie.mp4', mime: 'video/mp4', size: big }).kind, 'file');
  assert.equal(classifyMedia({ url: 'https://v.com/play?id=1', mime: 'audio/mpeg', size: 0 }).kind, 'file');
  assert.equal(classifyMedia({ url: 'https://v.com/song.mp3', mime: 'application/octet-stream', size: big }).kind, 'file');
  assert.equal(classifyMedia({ url: 'https://v.com/clip.webm', mime: '', size: 0 }).filename, 'clip.webm');
  assert.equal(classifyMedia({ url: 'https://v.com/live/index.m3u8?t=1', mime: 'application/vnd.apple.mpegurl' }).kind, 'hls');
  assert.equal(classifyMedia({ url: 'https://v.com/master', mime: 'application/x-mpegURL' }).kind, 'hls');
  assert.equal(classifyMedia({ url: 'https://v.com/manifest.mpd', mime: 'application/dash+xml' }).kind, 'dash');
  assert.equal(classifyMedia({ url: 'https://v.com/seg-001.ts', mime: 'video/mp2t', size: big }), null);
  assert.equal(classifyMedia({ url: 'https://v.com/chunk.m4s', mime: 'video/iso.segment', size: big }), null);
  assert.equal(classifyMedia({ url: 'https://v.com/click.mp3', mime: 'audio/mpeg', size: MIN_MEDIA_SIZE - 1 }), null);
  assert.equal(classifyMedia({ url: 'https://v.com/page.html', mime: 'text/html', size: big }), null);
  assert.equal(classifyMedia({ url: 'https://v.com/error.mp4', mime: 'text/html', size: big }), null);
  assert.equal(classifyMedia({ url: 'https://v.com/a.mp4', mime: 'video/mp4', statusCode: 404 }), null);
  assert.equal(classifyMedia({ url: 'blob:https://v.com/1', mime: 'video/mp4' }), null);
  assert.equal(classifyMedia({ url: 'nope' }), null);
  assert.equal(classifyMedia(null), null);
});

test('addMedia de-duplicates (ignoring range params) and caps the list', () => {
  let list = [];
  list = addMedia(list, { url: 'https://v.com/v.mp4?range=0-100&id=1' });
  list = addMedia(list, { url: 'https://v.com/v.mp4?range=100-200&id=1' });
  assert.equal(list.length, 1);
  assert.equal(mediaKey('https://v.com/v.mp4?id=1&range=1-2#t'), 'https://v.com/v.mp4?id=1');
  for (let i = 0; i < MAX_MEDIA_PER_TAB + 10; i++) list = addMedia(list, { url: `https://v.com/${i}.mp4` });
  assert.equal(list.length, MAX_MEDIA_PER_TAB);
  assert.equal(list.at(-1).url, `https://v.com/${MAX_MEDIA_PER_TAB + 9}.mp4`);
});

test('dedupeLinks resolves, filters schemes and removes duplicates', () => {
  const out = dedupeLinks([
    '/files/a.zip', 'https://site.com/files/a.zip#x', 'b.zip', 'javascript:alert(1)', 'mailto:me@x.com',
    'data:text/plain,hi', 'ftp://ftp.site.com/c.iso', '  https://site.com/d.pdf  ', 'http://[bad', 42, null,
  ], 'https://site.com/dir/page.html');
  assert.deepEqual(out, ['https://site.com/files/a.zip', 'https://site.com/dir/b.zip', 'ftp://ftp.site.com/c.iso', 'https://site.com/d.pdf']);
});

test('downloadableLinks keeps listed types and caps the count', () => {
  const urls = ['https://s.com/a.zip', 'https://s.com/page', 'https://s.com/b.html', 'https://s.com/c.EXE?x=1'];
  assert.deepEqual(downloadableLinks(urls, ['zip', 'exe']), ['https://s.com/a.zip', 'https://s.com/c.EXE?x=1']);
  const many = Array.from({ length: MAX_LINKS + 20 }, (_, i) => `https://s.com/${i}.zip`);
  assert.equal(downloadableLinks(many, ['zip']).length, MAX_LINKS);
});
