// Human-readable formatting helpers. Pure functions (unit-tested in format.test.ts).

const UNITS = ["B", "KB", "MB", "GB", "TB", "PB"];

/** formatBytes(1536) -> "1.5 KB". Negative or NaN -> "?" (unknown). Binary (1024) units. */
export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return "?";
  if (n < 1024) return `${Math.round(n)} B`;
  let i = 0;
  let v = n;
  while (v >= 1024 && i < UNITS.length - 1) {
    v /= 1024;
    i++;
  }
  const digits = v >= 100 ? 0 : v >= 10 ? 1 : 2;
  return `${trimZeros(v.toFixed(digits))} ${UNITS[i]}`;
}

function trimZeros(s: string): string {
  return s.includes(".") ? s.replace(/\.?0+$/, "") : s;
}

/** formatSpeed(2_097_152) -> "2 MB/s"; 0 -> "". */
export function formatSpeed(bps: number): string {
  if (!Number.isFinite(bps) || bps <= 0) return "";
  return `${formatBytes(bps)}/s`;
}

/** formatETA(3725) -> "1h 02m"; 185 -> "3m 05s"; 42 -> "42s"; <0 -> "". */
export function formatETA(sec: number): string {
  if (!Number.isFinite(sec) || sec < 0) return "";
  sec = Math.round(sec);
  if (sec < 60) return `${sec}s`;
  const pad = (x: number) => String(x).padStart(2, "0");
  if (sec < 3600) return `${Math.floor(sec / 60)}m ${pad(sec % 60)}s`;
  if (sec < 86400) return `${Math.floor(sec / 3600)}h ${pad(Math.floor((sec % 3600) / 60))}m`;
  return `${Math.floor(sec / 86400)}d ${Math.floor((sec % 86400) / 3600)}h`;
}

/** Progress in [0, 100], or -1 when the total size is unknown. */
export function percent(downloaded: number, size: number): number {
  if (size <= 0) return -1;
  return Math.max(0, Math.min(100, (downloaded / size) * 100));
}

/** formatPercent(42.345) -> "42.3%"; whole numbers without decimals. */
export function formatPercent(p: number): string {
  if (p < 0) return "";
  if (p >= 100) return "100%";
  return `${p.toFixed(1).replace(/\.0$/, "")}%`;
}

/** Parses "512K", "2M", "1.5 MB", "1000" (bytes/s) into a number of bytes. 0/"" = unlimited. */
export function parseSize(input: string): number | null {
  const s = input.trim().toUpperCase().replace(/\/S$/, "");
  if (s === "") return 0;
  const m = /^(\d+(?:\.\d+)?)\s*([KMGT]?)(I?B)?$/.exec(s);
  if (!m) return null;
  const mult = { "": 1, K: 1024, M: 1024 ** 2, G: 1024 ** 3, T: 1024 ** 4 }[m[2] as "" | "K" | "M" | "G" | "T"];
  return Math.round(parseFloat(m[1]) * mult);
}
