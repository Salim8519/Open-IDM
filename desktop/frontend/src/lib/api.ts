import { pickTransport, Transport } from "./transport";
import { AddRequest, BrowserKey, BrowserStatus, Capture, Download, PeregrineEvent, ProbeInfo, ScheduleStatus, Settings, UpdateInfo } from "./types";

/** Typed client for docs/API.md. All UI code goes through this. */
export function createClient(t: Transport) {
  return {
    transport: t,
    version: () => t.call<{ version: string }>("version"),
    list: () => t.call<Download[]>("list"),
    get: (id: string) => t.call<Download>("get", { id }),
    add: (r: AddRequest) => t.call<Download>("add", r),
    probe: (url: string, headers?: Record<string, string>) => t.call<ProbeInfo>("probe", { url, headers }),
    pause: (id: string) => t.call<boolean>("pause", { id }),
    resume: (id: string) => t.call<boolean>("resume", { id }),
    restart: (id: string) => t.call<boolean>("restart", { id }),
    remove: (id: string, deleteFile = false) => t.call<boolean>("remove", { id, deleteFile }),
    updateURL: (id: string, url: string, headers?: Record<string, string>) =>
      t.call<boolean>("updateURL", { id, url, headers }),
    setSpeedLimit: (id: string, bps: number) => t.call<boolean>("setSpeedLimit", { id, bps }),
    move: (id: string, delta: number) => t.call<boolean>("move", { id, delta }),
    pauseAll: () => t.call<boolean>("pauseAll"),
    resumeAll: () => t.call<boolean>("resumeAll"),
    clearCompleted: () => t.call<boolean>("clearCompleted"),
    getSettings: () => t.call<Settings>("getSettings"),
    updateSettings: (s: Partial<Settings>) => t.call<Settings>("updateSettings", s),
    scheduleStatus: () => t.call<ScheduleStatus>("scheduleStatus"),
    capture: (c: Capture) => t.call<{ queued: boolean; id?: string; prompted?: boolean }>("capture", c),
    openFile: (id: string) => t.call<boolean>("openFile", { id }),
    showInFolder: (id: string) => t.call<boolean>("showInFolder", { id }),
    pickFolder: (current: string) => t.call<{ dir: string }>("pickFolder", { current }),
    browserStatus: () => t.call<BrowserStatus>("browserStatus"),
    openExtensionsPage: (browser: BrowserKey) => t.call<boolean>("openExtensionsPage", { browser }),
    showExtensionFolder: () => t.call<boolean>("showExtensionFolder"),
    checkUpdate: () => t.call<UpdateInfo>("checkUpdate"),
    updateStatus: () => t.call<UpdateInfo | null>("updateStatus"),
    openUpdatePage: () => t.call<boolean>("openUpdatePage"),
    readClipboard: () => t.call<{ text: string }>("readClipboard"),
    subscribe: (fn: (e: PeregrineEvent) => void) => t.subscribe(fn),
  };
}

export type Client = ReturnType<typeof createClient>;

export const api: Client = createClient(pickTransport());
