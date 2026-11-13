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
}

export interface ProbeInfo {
  url: string;
  filename: string;
  size: number;
  resumable: boolean;
  category: string;
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
  theme: string; // "system" | "light" | "dark"
  language: string; // "en" | "ar"
  minExtSize: number;
}

export interface Capture extends AddRequest {
  source: string;
  pageUrl?: string;
  size?: number;
  mimeType?: string;
}

export interface PeregrineEvent {
  type: "added" | "updated" | "progress" | "removed" | "settings" | "capture" | "browser";
  download?: Download;
  downloads?: Download[];
  id?: string;
  settings?: Settings;
  capture?: Capture;
  browser?: BrowserStatus;
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
