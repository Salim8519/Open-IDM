// Test harness for the extension e2e tests: builds the host and fake app, registers the
// host for the test browser, serves a local site and runs the fake Peregrine app.
import { execFileSync, spawn } from 'node:child_process';
import { mkdtempSync, mkdirSync, rmSync, readFileSync, existsSync, copyFileSync } from 'node:fs';
import { createServer } from 'node:http';
import { tmpdir } from 'node:os';
import { join, resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

export const EXTENSION_ID = 'bcnjpgddmjlffmcfgnfjaadkdjphdhfd';
export const extDir = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const repo = resolve(extDir, '..');
const isWin = process.platform === 'win32';
const exe = isWin ? '.exe' : '';

// Chrome for Testing on Windows reads Google\Chrome's key; Chromium builds read Chromium's.
const REG_BROWSERS = ['chrome', 'chromium'];
const REG_KEYS = [
  'HKCU\\Software\\Google\\Chrome\\NativeMessagingHosts\\com.peregrine.host',
  'HKCU\\Software\\Chromium\\NativeMessagingHosts\\com.peregrine.host',
];

export function makeTemp() {
  const tmp = mkdtempSync(join(tmpdir(), 'peregrine-e2e-'));
  const dirs = { tmp, home: join(tmp, 'home'), bin: join(tmp, 'bin'), profile: join(tmp, 'profile'), fakeHome: join(tmp, 'fakehome') };
  for (const d of Object.values(dirs)) mkdirSync(d, { recursive: true });
  return dirs;
}

export function buildBinaries(bin) {
  const hostExe = join(bin, `peregrine-host${exe}`);
  const fakeExe = join(bin, `fakeapp${exe}`);
  execFileSync('go', ['build', '-o', hostExe, './cmd/peregrine-host'], { cwd: repo, stdio: 'inherit' });
  execFileSync('go', ['build', '-o', fakeExe, './cmd/peregrine-host/internal/fakeapp'], { cwd: repo, stdio: 'inherit' });
  return { hostExe, fakeExe };
}

function regQuery(key) {
  try {
    const out = execFileSync('reg', ['query', key, '/ve'], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] });
    const m = /REG_SZ\s+(.*)$/m.exec(out);
    return m ? m[1].trim() : '';
  } catch {
    return null; // key does not exist
  }
}

/** Register the host for the test browser. Returns a cleanup function. */
export function registerHost(hostExe, dirs) {
  const env = { ...process.env, PEREGRINE_HOME: dirs.home };
  if (isWin) {
    const backups = REG_KEYS.map((k) => [k, regQuery(k)]);
    execFileSync(hostExe, ['install', '--browsers', REG_BROWSERS.join(',')], { env, stdio: 'inherit' });
    return () => {
      try {
        execFileSync(hostExe, ['uninstall', '--browsers', REG_BROWSERS.join(',')], { env, stdio: 'inherit' });
      } finally {
        for (const [k, v] of backups) {
          if (v !== null) execFileSync('reg', ['add', k, '/ve', '/d', v, '/f'], { stdio: 'ignore' });
        }
      }
    };
  }
  // macOS/Linux: install into a fake home, then copy the manifest into the profile's own
  // NativeMessagingHosts folder, which Chromium also reads. Nothing outside tmp is touched.
  execFileSync(hostExe, ['install', '--browsers', 'chromium'], { env: { ...env, PEREGRINE_REG_ROOT: dirs.fakeHome }, stdio: 'inherit' });
  const src = process.platform === 'darwin'
    ? join(dirs.fakeHome, 'Library', 'Application Support', 'Chromium', 'NativeMessagingHosts', 'com.peregrine.host.json')
    : join(dirs.fakeHome, '.config', 'chromium', 'NativeMessagingHosts', 'com.peregrine.host.json');
  mkdirSync(join(dirs.profile, 'NativeMessagingHosts'), { recursive: true });
  copyFileSync(src, join(dirs.profile, 'NativeMessagingHosts', 'com.peregrine.host.json'));
  return () => {};
}

/** Start the fake app; resolves once it has written ipc.json. */
export async function startFakeApp(fakeExe, dirs) {
  const record = join(dirs.tmp, 'calls.jsonl');
  const child = spawn(fakeExe, ['-record', record], { env: { ...process.env, PEREGRINE_HOME: dirs.home }, stdio: ['pipe', 'pipe', 'inherit'] });
  await new Promise((res, rej) => {
    let buf = '';
    child.stdout.on('data', (d) => {
      buf += d;
      if (buf.includes('READY')) res();
    });
    child.on('exit', (code) => rej(new Error(`fakeapp exited with ${code}`)));
    setTimeout(() => rej(new Error('fakeapp did not start')), 15000);
  });
  return {
    record,
    calls() {
      if (!existsSync(record)) return [];
      return readFileSync(record, 'utf8').split('\n').filter(Boolean).map((l) => JSON.parse(l));
    },
    async stop() {
      if (child.exitCode !== null) return;
      const exited = new Promise((r) => child.once('exit', r));
      child.stdin.end();
      await Promise.race([exited, new Promise((r) => setTimeout(r, 5000))]);
      if (child.exitCode === null) child.kill();
      rmSync(join(dirs.home, 'ipc.json'), { force: true });
    },
  };
}

const PAGE = `<!doctype html><html><head><meta charset="utf-8"><title>Peregrine e2e</title></head><body>
<h1>Downloads</h1>
<p><a id="zip" href="/files/test.zip">test.zip</a></p>
<p><a id="fallback" href="/files/fallback.zip">fallback.zip</a></p>
<p><a id="page" href="/other.html">not a download</a></p>
</body></html>`;

const MEDIA_PAGE = `<!doctype html><html><head><meta charset="utf-8"><title>media</title></head><body>
<video id="v" src="/media/clip.mp4" preload="auto" muted></video>
<script>fetch('/media/live/index.m3u8');</script>
</body></html>`;

/** Local website: sets a session cookie, serves slow .zip downloads and some media. */
export async function startSite() {
  const requests = [];
  const server = createServer((req, res) => {
    requests.push({ url: req.url, headers: req.headers });
    const u = new URL(req.url, 'http://x');
    if (u.pathname === '/') {
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8', 'Set-Cookie': 'sess=abc123; Path=/; HttpOnly' });
      return res.end(PAGE);
    }
    if (u.pathname === '/media.html') {
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      return res.end(MEDIA_PAGE);
    }
    if (u.pathname.startsWith('/files/')) {
      const name = u.pathname.split('/').pop();
      const chunk = Buffer.alloc(64 * 1024, 7);
      const chunks = 24; // 1.5 MB over ~2.4 s, so the browser is still downloading when we pause it
      res.writeHead(200, {
        'Content-Type': 'application/zip',
        'Content-Disposition': `attachment; filename="${name}"`,
        'Content-Length': String(chunk.length * chunks),
      });
      let i = 0;
      const timer = setInterval(() => {
        if (res.destroyed) return clearInterval(timer);
        res.write(chunk);
        if (++i >= chunks) {
          clearInterval(timer);
          res.end();
        }
      }, 100);
      req.on('close', () => clearInterval(timer));
      return;
    }
    if (u.pathname === '/media/clip.mp4') {
      const body = Buffer.alloc(400 * 1024, 1);
      res.writeHead(200, { 'Content-Type': 'video/mp4', 'Content-Length': String(body.length), 'Accept-Ranges': 'none' });
      return res.end(body);
    }
    if (u.pathname === '/media/live/index.m3u8') {
      res.writeHead(200, { 'Content-Type': 'application/vnd.apple.mpegurl' });
      return res.end('#EXTM3U\n#EXT-X-ENDLIST\n');
    }
    res.writeHead(404, { 'Content-Type': 'text/plain' });
    res.end('not found');
  });
  await new Promise((r) => server.listen(0, '127.0.0.1', r));
  const base = `http://127.0.0.1:${server.address().port}`;
  return { base, requests, close: () => new Promise((r) => server.close(r)) };
}

export function removeTemp(dirs) {
  try {
    rmSync(dirs.tmp, { recursive: true, force: true, maxRetries: 5, retryDelay: 300 });
  } catch {
    /* browser may still hold files on Windows; temp dir is cleaned by the OS later */
  }
}
