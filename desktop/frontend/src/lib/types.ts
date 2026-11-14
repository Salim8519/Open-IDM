// Types mirror engine/types.go and internal/api (see docs/API.md).

export type Status = "queued" | "downloading" | "paused" | "error" | "completed";

export interface Segment {
  start: number;
  end: number; // -1 = until EOF
  done: number;
}

export interface Download {
  id: string;
  url: string;
  dir: string;
  filename: string;
  size: number; // -1 unknown
  downloaded: number;
  status: Status;
  error?: string;
  resumable: boolean;
  connections: number;
  active: number;
  speed: number;
  kind?: DownloadKind;
  isDir?: boolean; // output is a folder (multi-file torrent)
  eta: number; // seconds, -1 unknown
  checksum?: string;
  sha256?: string;
  speedLimit?: number;
  category: string;
  segments?: Segment[];
  createdAt: string;
  completedAt?: string;
  order: number;
}

export interface AddRequest {
  url: string;
  dir?: string;
  filename?: string;
  connections?: number;
  headers?: Record<string, string>;
  checksum?: string;
  speedLimit?: number;
  paused?: boolean;
  kind?: DownloadKind;
}

/** Which downloader handles a link ("" or "http" = the built-in segmented HTTP engine). */
export type DownloadKind = "" | "http" | "ftp" | "hls" | "dash" | "video" | "torrent";

export interface ProbeInfo {
  url: string;
  filename: string;
  size: number;
  resumable: boolean;
  category: string;
  kind?: DownloadKind;
}

export interface Settings {
  downloadDir: string;
  useCategories: boolean;
  maxActive: number;
  connections: number;
  globalLimit: number;
  proxy: string;
  userAgent: string;
  maxRetries: number;
  clipboardWatch: boolean;
  autoStartExt: boolean;
  theme: string; // "field" (default) | "field-day" | "field-night" | Classic: "system" | "light" | "dark"
  uiVersion?: number; // engine bookkeeping for one-time settings migrations
  language: string; // a code from i18n LANGUAGES, e.g. "en", "ar", "zh"
  minExtSize: number;
  schedule: Schedule;
  closeToTray: boolean;
  launchAtLogin: boolean;
  notify: boolean;
  updateCheck: boolean;
}

export type WhenDone = "none" | "quit" | "sleep" | "shutdown";

/** When the queue may run (docs/API.md: Settings.schedule). Times are local "HH:MM". */
export interface Schedule {
  enabled: boolean;
  start: string;
  stop: string; // "" = no stop time (24-hour window)
  days: number[] | null; // 0 = Sunday … 6 = Saturday; empty/null = every day
  whenDone: WhenDone;
}

export interface ScheduleStatus {
  enabled: boolean;
  open: boolean;
  nextStart?: string; // RFC 3339
  nextStop?: string;
}

export interface Capture extends AddRequest {
  source: string;
  pageUrl?: string;
  size?: number;
  mimeType?: string;
}

export interface PeregrineEvent {
  type: "added" | "updated" | "progress" | "removed" | "settings" | "capture" | "browser" | "schedule" | "queueDone" | "update";
  download?: Download;
  downloads?: Download[];
  id?: string;
  settings?: Settings;
  schedule?: ScheduleStatus;
  capture?: Capture;
  browser?: BrowserStatus;
  update?: UpdateInfo;
}

/** Result of an update check (docs/API.md: checkUpdate, ADR 0020). */
export interface UpdateInfo {
  current: string;
  latest: string;
  newer: boolean;
  url: string;
  checkedAt: string;
}

/** Whether the browser extension has reached the app (docs/API.md: browserStatus). */
export interface BrowserStatus {
  connected: boolean;
  lastSeen?: number;
  extensionDir?: string;
  extensionId: string;
}

export type BrowserKey = "chrome" | "edge" | "brave";

export class RPCError extends Error {
  code: string;
  constructor(code: string, message: string) {
    super(message);
    this.code = code;
  }
}
