// Build a Cookie request header from chrome.cookies.getAll() results (pure).

/**
 * Orders cookies like a browser (RFC 6265 §5.4: longer paths first, then older first),
 * drops duplicates by name+path, and skips cookies that would break the header.
 */
export function buildCookieHeader(cookies) {
  if (!Array.isArray(cookies) || cookies.length === 0) return '';
  const seen = new Set();
  const list = cookies
    .filter((c) => c && typeof c.name === 'string' && typeof c.value === 'string')
    .filter((c) => !/[;\r\n\0]/.test(c.name + c.value) && (c.name !== '' || c.value !== ''))
    .map((c, i) => ({ c, i }))
    .sort((a, b) => (b.c.path || '/').length - (a.c.path || '/').length || a.i - b.i);
  const parts = [];
  for (const { c } of list) {
    const key = c.name + '\u0000' + (c.path || '/');
    if (seen.has(key)) continue;
    seen.add(key);
    parts.push(c.name === '' ? c.value : `${c.name}=${c.value}`);
  }
  return parts.join('; ');
}
