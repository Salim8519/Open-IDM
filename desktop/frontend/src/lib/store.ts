import { useCallback, useEffect, useRef, useState } from "react";
import { Client } from "./api";
import { Capture, Download, PeregrineEvent, ScheduleStatus, Settings, UpdateInfo } from "./types";

export interface Transition {
  download: Download;
  from: Download["status"] | undefined;
}

interface Handlers {
  onCapture?: (c: Capture) => void;
  onTransition?: (t: Transition) => void;
}

/**
 * Keeps a live map of downloads + settings in sync with the engine: initial list(), then
 * events (added/updated/progress/removed/settings/capture).
 */
export function useEngine(client: Client, handlers: Handlers) {
  const [downloads, setDownloads] = useState<Map<string, Download>>(new Map());
  const [settings, setSettings] = useState<Settings | null>(null);
  const [schedule, setSchedule] = useState<ScheduleStatus | null>(null);
  const [version, setVersion] = useState("");
  const [update, setUpdate] = useState<UpdateInfo | null>(null);
  const [connected, setConnected] = useState(true);
  const h = useRef(handlers);
  h.current = handlers;
  const statuses = useRef(new Map<string, Download["status"]>());

  const upsert = useCallback((list: Download[], replace = false) => {
    for (const d of list) {
      const prev = statuses.current.get(d.id);
      statuses.current.set(d.id, d.status);
      if (prev !== d.status && prev !== undefined) h.current.onTransition?.({ download: d, from: prev });
    }
    setDownloads((old) => {
      const next = replace ? new Map<string, Download>() : new Map(old);
      for (const d of list) next.set(d.id, d);
      return next;
    });
  }, []);

  const refresh = useCallback(async () => {
    try {
      const list = await client.list();
      statuses.current = new Map(list.map((d) => [d.id, d.status]));
      upsert(list, true);
      setConnected(true);
    } catch {
      setConnected(false);
    }
  }, [client, upsert]);

  useEffect(() => {
    const off = client.subscribe((e: PeregrineEvent) => {
      switch (e.type) {
        case "added":
        case "updated":
          if (e.download) upsert([e.download]);
          break;
        case "progress":
          if (e.downloads) upsert(e.downloads);
          break;
        case "removed":
          if (e.id) {
            statuses.current.delete(e.id);
            setDownloads((old) => {
              const next = new Map(old);
              next.delete(e.id!);
              return next;
            });
          }
          break;
        case "settings":
          if (e.settings) setSettings(e.settings);
          break;
        case "schedule":
          if (e.schedule) setSchedule(e.schedule);
          break;
        case "capture":
          if (e.capture) h.current.onCapture?.(e.capture);
          break;
        case "update":
          if (e.update) setUpdate(e.update);
          break;
      }
    });
    void refresh();
    client.getSettings().then(setSettings, () => setConnected(false));
    client.scheduleStatus().then(setSchedule, () => undefined);
    client.version().then((v) => setVersion(v.version), () => undefined);
    client.updateStatus().then((u) => u && setUpdate(u), () => undefined);
    return off;
  }, [client, refresh, upsert]);

  return { downloads, settings, setSettings, schedule, version, update, connected, refresh };
}
