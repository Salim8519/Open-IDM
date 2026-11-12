import { Download, Status } from "./types";

/** True when a failed download looks like an expired/forbidden link (fix: refresh the URL). */
export function isExpired(d: Download): boolean {
  return d.status === "error" && /expired|access was denied|403|401|forbidden/i.test(d.error ?? "");
}

export const STATUS_FILTERS = ["all", "downloading", "queued", "paused", "completed", "error"] as const;
export type StatusFilter = (typeof STATUS_FILTERS)[number];

export const CATEGORIES = ["Video", "Music", "Documents", "Compressed", "Programs", "Images", "Disk Images", "Other"] as const;

export function matchesStatus(d: Download, f: StatusFilter): boolean {
  return f === "all" || d.status === (f as Status);
}

export type SortKey = "order" | "name" | "size" | "progress" | "speed" | "eta" | "status";

const statusRank: Record<Status, number> = { downloading: 0, queued: 1, paused: 2, error: 3, completed: 4 };

export function compareDownloads(a: Download, b: Download, key: SortKey): number {
  switch (key) {
    case "name":
      return a.filename.localeCompare(b.filename, undefined, { numeric: true, sensitivity: "base" });
    case "size":
      return a.size - b.size;
    case "progress": {
      const pa = a.size > 0 ? a.downloaded / a.size : a.status === "completed" ? 1 : 0;
      const pb = b.size > 0 ? b.downloaded / b.size : b.status === "completed" ? 1 : 0;
      return pa - pb;
    }
    case "speed":
      return a.speed - b.speed;
    case "eta":
      return (a.eta < 0 ? Infinity : a.eta) - (b.eta < 0 ? Infinity : b.eta);
    case "status":
      return statusRank[a.status] - statusRank[b.status];
    default:
      return a.order - b.order;
  }
}

export function canPause(d: Download) {
  return d.status === "downloading" || d.status === "queued";
}

export function canResume(d: Download) {
  return d.status === "paused" || d.status === "error";
}
