import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowDown,
  ArrowUp,
  CirclePause,
  CirclePlay,
  Copy,
  Eraser,
  ExternalLink,
  FolderOpen,
  Gauge,
  Inbox,
  ListPlus,
  Pause,
  Play,
  Plus,
  RefreshCw,
  RotateCcw,
  Search,
  Settings as SettingsIcon,
  Trash,
} from "lucide-react";
import { api } from "./lib/api";
import { useEngine, Transition } from "./lib/store";
import { Capture, Download, Settings } from "./lib/types";
import { I18nProvider, isRTL, Lang, translate } from "./i18n";
import { StringKey } from "./i18n/en";
import {
  canPause,
  canResume,
  CATEGORIES,
  compareDownloads,
  matchesStatus,
  SortKey,
  STATUS_FILTERS,
  StatusFilter,
} from "./lib/download";
import { looksDownloadable, parseBatch } from "./lib/batch";
import { formatSpeed } from "./lib/format";
import { Sidebar } from "./components/Sidebar";
import { DownloadTable } from "./components/DownloadTable";
import { DetailPanel } from "./components/DetailPanel";
import { ContextMenu, MenuItem } from "./components/ContextMenu";
import { AddDialog, AddInitial } from "./components/AddDialog";
import { BatchDialog } from "./components/BatchDialog";
import { SettingsDialog } from "./components/SettingsDialog";
import { RefreshLinkDialog, RemoveDialog, SpeedLimitDialog } from "./components/SmallDialogs";
import { Toasts, useToasts } from "./components/Toasts";
import logo from "./logo.svg";

type Modal =
  | { type: "add"; initial: AddInitial }
  | { type: "batch"; text?: string }
  | { type: "settings" }
  | { type: "remove"; ids: string[]; deleteFile: boolean }
  | { type: "refresh"; id: string }
  | { type: "limit"; id: string }
  | null;

function useEffectiveTheme(theme: string | undefined): "light" | "dark" {
  const query = "(prefers-color-scheme: dark)";
  const [sysDark, setSysDark] = useState(() => window.matchMedia?.(query).matches ?? true);
  useEffect(() => {
    const mq = window.matchMedia?.(query);
    if (!mq) return;
    const on = (e: MediaQueryListEvent) => setSysDark(e.matches);
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, []);
  if (theme === "light" || theme === "dark") return theme;
  return sysDark ? "dark" : "light";
}

const isTyping = (el: EventTarget | null) =>
  el instanceof HTMLElement && (el.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName));

export default function App() {
  const { toasts, push, dismiss } = useToasts();
  const [modal, setModal] = useState<Modal>(null);
  const langRef = useRef<Lang>("en");
  const tr = (k: StringKey, v?: Record<string, string | number>) => translate(langRef.current, k, v);

  const onCapture = useCallback((c: Capture) => setModal({ type: "add", initial: c }), []);
  const onTransition = useCallback(
    ({ download: d, from }: Transition) => {
      if (d.status === "completed" && from !== "completed") {
        push({
          kind: "success",
          title: tr("toast.completed"),
          body: d.filename,
          action: api.transport.kind === "wails" ? { label: tr("ctx.open"), run: () => void api.openFile(d.id) } : undefined,
        });
      } else if (d.status === "error" && from !== "error") {
        push({ kind: "error", title: tr("toast.failed"), body: `${d.filename}: ${d.error ?? ""}`, timeout: 8000 });
      }
    },
    [push],
  );
  const { downloads, settings, setSettings, version, connected, refresh } = useEngine(api, { onCapture, onTransition });

  const lang: Lang = settings?.language === "ar" ? "ar" : "en";
  langRef.current = lang;
  const theme = useEffectiveTheme(settings?.theme);

  useEffect(() => {
    const html = document.documentElement;
    html.lang = lang;
    html.dir = isRTL(lang) ? "rtl" : "ltr";
    html.dataset.theme = theme;
  }, [lang, theme]);

  // ---- filtering, search, sort ----
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [category, setCategory] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [sortKey, setSortKey] = useState<SortKey>("order");
  const [sortDir, setSortDir] = useState<1 | -1>(1);

  const all = useMemo(() => Array.from(downloads.values()).sort((a, b) => a.order - b.order), [downloads]);

  const statusCounts = useMemo(() => {
    const c = Object.fromEntries(STATUS_FILTERS.map((s) => [s, 0])) as Record<StatusFilter, number>;
    for (const d of all) {
      c.all++;
      c[d.status as StatusFilter]++;
    }
    return c;
  }, [all]);
  const categoryCounts = useMemo(() => {
    const c: Record<string, number> = Object.fromEntries(CATEGORIES.map((k) => [k, 0]));
    for (const d of all) c[d.category] = (c[d.category] ?? 0) + 1;
    return c;
  }, [all]);

  const rows = useMemo(() => {
    const q = search.trim().toLowerCase();
    const out = all.filter(
      (d) =>
        matchesStatus(d, statusFilter) &&
        (category === null || d.category === category) &&
        (!q || d.filename.toLowerCase().includes(q) || d.url.toLowerCase().includes(q)),
    );
    if (sortKey !== "order" || sortDir !== 1) out.sort((a, b) => compareDownloads(a, b, sortKey) * sortDir);
    return out;
  }, [all, statusFilter, category, search, sortKey, sortDir]);

  const onSort = (k: SortKey) => {
    if (k === sortKey) {
      if (sortDir === 1) setSortDir(-1);
      else {
        setSortKey("order");
        setSortDir(1);
      }
    } else {
      setSortKey(k);
      setSortDir(1);
    }
  };

  // ---- selection ----
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [focusId, setFocusId] = useState<string | null>(null);
  const anchor = useRef<string | null>(null);

  useEffect(() => {
    // drop ids that no longer exist
    setSelected((s) => {
      const n = new Set([...s].filter((id) => downloads.has(id)));
      return n.size === s.size ? s : n;
    });
    if (focusId && !downloads.has(focusId)) setFocusId(null);
  }, [downloads, focusId]);

  const onRowClick = (e: React.MouseEvent, id: string) => {
    if (e.shiftKey && anchor.current) {
      const ids = rows.map((r) => r.id);
      const a = ids.indexOf(anchor.current);
      const b = ids.indexOf(id);
      if (a >= 0 && b >= 0) {
        const [lo, hi] = a < b ? [a, b] : [b, a];
        setSelected(new Set(ids.slice(lo, hi + 1)));
        setFocusId(id);
        return;
      }
    }
    if (e.ctrlKey || e.metaKey) {
      setSelected((s) => {
        const n = new Set(s);
        if (n.has(id)) n.delete(id);
        else n.add(id);
        return n;
      });
    } else {
      setSelected(new Set([id]));
    }
    anchor.current = id;
    setFocusId(id);
  };

  const selectedList = useMemo(() => all.filter((d) => selected.has(d.id)), [all, selected]);
  const detail = focusId && selected.has(focusId) ? downloads.get(focusId) : selectedList.length === 1 ? selectedList[0] : undefined;

  // ---- actions ----
  const run = useCallback(
    async (fn: () => Promise<unknown>) => {
      try {
        await fn();
      } catch (err) {
        push({ kind: "error", title: (err as Error).message, timeout: 6000 });
      }
    },
    [push],
  );
  const pauseList = (list: Download[]) => run(() => Promise.all(list.filter(canPause).map((d) => api.pause(d.id))));
  const resumeList = (list: Download[]) => run(() => Promise.all(list.filter(canResume).map((d) => api.resume(d.id))));
  const askRemove = (list: Download[], deleteFile = false) => {
    if (list.length) setModal({ type: "remove", ids: list.map((d) => d.id), deleteFile });
  };
  const copyLink = async (d: Download) => {
    try {
      await navigator.clipboard.writeText(d.url);
      push({ kind: "info", title: tr("toast.copied"), timeout: 2000 });
    } catch {
      /* ignore */
    }
  };
  const openAdd = (initial: AddInitial = {}) => setModal({ type: "add", initial });

  // ---- context menu ----
  const [menu, setMenu] = useState<{ x: number; y: number; id: string } | null>(null);
  const onRowContext = (e: React.MouseEvent, id: string) => {
    e.preventDefault();
    if (!selected.has(id)) {
      setSelected(new Set([id]));
      anchor.current = id;
    }
    setFocusId(id);
    setMenu({ x: e.clientX, y: e.clientY, id });
  };
  const menuItems = (d: Download): MenuItem[] => {
    const list = selected.has(d.id) ? selectedList : [d];
    const done = d.status === "completed";
    const desktop = api.transport.kind === "wails";
    return [
      { id: "open", label: tr("ctx.open"), icon: <ExternalLink size={14} />, disabled: !done || !desktop, onSelect: () => run(() => api.openFile(d.id)) },
      { id: "folder", label: tr("ctx.showInFolder"), icon: <FolderOpen size={14} />, disabled: !desktop, onSelect: () => run(() => api.showInFolder(d.id)) },
      { id: "copy", label: tr("ctx.copyLink"), icon: <Copy size={14} />, onSelect: () => void copyLink(d) },
      { id: "pause", label: tr("toolbar.pause"), icon: <Pause size={14} />, separatorBefore: true, disabled: !list.some(canPause), onSelect: () => void pauseList(list) },
      { id: "resume", label: tr("toolbar.resume"), icon: <Play size={14} />, disabled: !list.some(canResume), onSelect: () => void resumeList(list) },
      { id: "refresh", label: tr("ctx.refreshLink"), icon: <RefreshCw size={14} />, disabled: done, onSelect: () => setModal({ type: "refresh", id: d.id }) },
      { id: "restart", label: tr("ctx.restart"), icon: <RotateCcw size={14} />, onSelect: () => run(() => api.restart(d.id)) },
      { id: "up", label: tr("ctx.moveUp"), icon: <ArrowUp size={14} />, separatorBefore: true, onSelect: () => run(() => api.move(d.id, -1)) },
      { id: "down", label: tr("ctx.moveDown"), icon: <ArrowDown size={14} />, onSelect: () => run(() => api.move(d.id, 1)) },
      { id: "limit", label: tr("ctx.speedLimit"), icon: <Gauge size={14} />, disabled: done, onSelect: () => setModal({ type: "limit", id: d.id }) },
      { id: "remove", label: tr("ctx.remove"), icon: <Trash size={14} />, separatorBefore: true, danger: true, onSelect: () => askRemove(list) },
      { id: "remove-delete", label: tr("ctx.removeDelete"), icon: <Trash size={14} />, danger: true, onSelect: () => askRemove(list, true) },
    ];
  };

  // ---- keyboard shortcuts & paste ----
  const stateRef = useRef({ modal, selectedList, rows });
  stateRef.current = { modal, selectedList, rows };
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const { modal, selectedList, rows } = stateRef.current;
      if (modal) return;
      const mod = e.ctrlKey || e.metaKey;
      if (mod && e.key.toLowerCase() === "n") {
        e.preventDefault();
        openAdd();
        return;
      }
      if (isTyping(e.target)) return;
      if (mod && e.key.toLowerCase() === "a") {
        e.preventDefault();
        setSelected(new Set(rows.map((r) => r.id)));
      } else if (e.key === "Delete") {
        e.preventDefault();
        askRemove(selectedList);
      } else if (e.key === " " && selectedList.length) {
        e.preventDefault();
        if (selectedList.some(canPause)) void pauseList(selectedList);
        else void resumeList(selectedList);
      } else if (e.key === "Escape") {
        setSelected(new Set());
      }
    };
    const onPaste = (e: ClipboardEvent) => {
      if (stateRef.current.modal || isTyping(e.target)) return;
      const text = e.clipboardData?.getData("text") ?? "";
      const links = parseBatch(text);
      if (links.length === 1) openAdd({ url: links[0] });
      else if (links.length > 1) setModal({ type: "batch", text });
    };
    window.addEventListener("keydown", onKey);
    window.addEventListener("paste", onPaste);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("paste", onPaste);
    };
  }, []);

  // ---- clipboard monitor ----
  const urlsRef = useRef(new Set<string>());
  urlsRef.current = new Set(all.map((d) => d.url));
  useEffect(() => {
    if (!settings?.clipboardWatch) return;
    let last: string | null = null;
    let stopped = false;
    const tick = async () => {
      try {
        const { text } = await api.readClipboard();
        const s = (text ?? "").trim();
        if (last === null) {
          last = s; // don't offer whatever was on the clipboard before monitoring started
          return;
        }
        if (s === last) return;
        last = s;
        if (looksDownloadable(s) && !urlsRef.current.has(s)) {
          push({
            kind: "link",
            title: tr("toast.clipboard"),
            body: s,
            timeout: 12000,
            action: { label: tr("toast.download"), run: () => setModal({ type: "add", initial: { url: s, source: "clipboard" } }) },
          });
        }
      } catch {
        stopped = true; // unsupported in this mode
      }
    };
    void tick();
    const id = setInterval(() => !stopped && void tick(), 1500);
    return () => clearInterval(id);
  }, [settings?.clipboardWatch, push]);

  const totalSpeed = all.reduce((s, d) => s + (d.status === "downloading" ? d.speed : 0), 0);
  const activeCount = statusCounts.downloading;
  const modalDownload = modal && (modal.type === "refresh" || modal.type === "limit") ? downloads.get(modal.id) : undefined;
  const t = tr;

  return (
    <I18nProvider lang={lang}>
      <div className="app">
        <header className="topbar">
          <div className="brand">
            <img src={logo} alt="" width={26} height={26} />
            <span className="brand-name">{t("app.name")}</span>
          </div>
          <div className="toolbar" role="toolbar" aria-label="Toolbar">
            <button className="tb-btn primary" onClick={() => openAdd()} title={t("toolbar.addTitle")} data-testid="tb-add">
              <Plus size={16} />
              <span>{t("toolbar.add")}</span>
            </button>
            <button className="tb-btn" onClick={() => setModal({ type: "batch" })} title={t("toolbar.batchTitle")} data-testid="tb-batch">
              <ListPlus size={16} />
              <span>{t("toolbar.batch")}</span>
            </button>
            <span className="tb-sep" />
            <button
              className="tb-btn"
              onClick={() => void resumeList(selectedList)}
              disabled={!selectedList.some(canResume)}
              title={t("toolbar.resume")}
              data-testid="tb-resume"
            >
              <Play size={16} />
              <span>{t("toolbar.resume")}</span>
            </button>
            <button
              className="tb-btn"
              onClick={() => void pauseList(selectedList)}
              disabled={!selectedList.some(canPause)}
              title={t("toolbar.pause")}
              data-testid="tb-pause"
            >
              <Pause size={16} />
              <span>{t("toolbar.pause")}</span>
            </button>
            <button
              className="tb-btn"
              onClick={() => askRemove(selectedList)}
              disabled={!selectedList.length}
              title={t("toolbar.remove")}
              data-testid="tb-remove"
            >
              <Trash size={16} />
              <span>{t("toolbar.remove")}</span>
            </button>
            <span className="tb-sep" />
            <button className="tb-btn" onClick={() => run(api.pauseAll)} title={t("toolbar.pauseAll")} data-testid="tb-pause-all">
              <CirclePause size={16} />
              <span>{t("toolbar.pauseAll")}</span>
            </button>
            <button className="tb-btn" onClick={() => run(api.resumeAll)} title={t("toolbar.resumeAll")} data-testid="tb-resume-all">
              <CirclePlay size={16} />
              <span>{t("toolbar.resumeAll")}</span>
            </button>
            <button
              className="tb-btn"
              onClick={() => run(api.clearCompleted)}
              disabled={!statusCounts.completed}
              title={t("toolbar.clearCompleted")}
              data-testid="tb-clear"
            >
              <Eraser size={16} />
              <span>{t("toolbar.clearCompleted")}</span>
            </button>
          </div>
          <div className="search">
            <Search size={14} aria-hidden="true" />
            <input
              type="search"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t("toolbar.search")}
              aria-label={t("toolbar.search")}
              data-testid="search"
            />
          </div>
          <button
            className="tb-btn icon-only"
            onClick={() => setModal({ type: "settings" })}
            disabled={!settings}
            title={t("toolbar.settings")}
            aria-label={t("toolbar.settings")}
            data-testid="tb-settings"
          >
            <SettingsIcon size={18} />
          </button>
        </header>

        <div className="body">
          <Sidebar
            status={statusFilter}
            category={category}
            statusCounts={statusCounts}
            categoryCounts={categoryCounts}
            onStatus={setStatusFilter}
            onCategory={setCategory}
          />
          <main className="main">
            <div className="table-wrap" onMouseDown={(e) => e.target === e.currentTarget && setSelected(new Set())}>
              {rows.length > 0 ? (
                <DownloadTable
                  rows={rows}
                  selected={selected}
                  sortKey={sortKey}
                  sortDir={sortDir}
                  onSort={onSort}
                  onRowClick={onRowClick}
                  onRowContext={onRowContext}
                  onRowOpen={(d) => {
                    if (d.status === "completed" && api.transport.kind === "wails") void run(() => api.openFile(d.id));
                    else if (canResume(d)) void resumeList([d]);
                  }}
                />
              ) : (
                <div className="empty" data-testid="empty">
                  <Inbox size={44} strokeWidth={1.4} aria-hidden="true" />
                  {all.length === 0 ? (
                    <>
                      <h2>{t("empty.title")}</h2>
                      <p>{t("empty.hint")}</p>
                      <button className="btn primary" onClick={() => openAdd()}>
                        <Plus size={14} /> {t("toolbar.add")}
                      </button>
                    </>
                  ) : (
                    <h2>{t("empty.noMatch")}</h2>
                  )}
                </div>
              )}
            </div>
            {detail && (
              <DetailPanel d={detail} onRefreshLink={(d) => setModal({ type: "refresh", id: d.id })} onClose={() => setSelected(new Set())} />
            )}
          </main>
        </div>

        <footer className="statusbar">
          <span className={`dot${connected ? " on" : ""}`} aria-hidden="true" />
          {connected ? (
            <>
              <span data-testid="sb-speed">{t("statusbar.speed", { speed: formatSpeed(totalSpeed) || "0 B/s" })}</span>
              <span>{t("statusbar.active", { n: activeCount })}</span>
            </>
          ) : (
            <span className="warn">{t("statusbar.disconnected")}</span>
          )}
          <span className="grow" />
          <span className="muted">
            {t("app.name")} {version}
          </span>
        </footer>

        {menu && downloads.get(menu.id) && (
          <ContextMenu x={menu.x} y={menu.y} label={t("ctx.label")} items={menuItems(downloads.get(menu.id)!)} onClose={() => setMenu(null)} />
        )}

        {modal?.type === "add" && <AddDialog client={api} settings={settings} initial={modal.initial} onClose={() => setModal(null)} />}
        {modal?.type === "batch" && (
          <BatchDialog
            client={api}
            settings={settings}
            initialText={modal.text}
            onClose={() => setModal(null)}
            onDone={(n) => push({ kind: "info", title: t("batch.added", { n }), timeout: 3000 })}
          />
        )}
        {modal?.type === "settings" && settings && (
          <SettingsDialog
            client={api}
            settings={settings}
            version={version}
            onClose={() => setModal(null)}
            onSaved={(s: Settings) => {
              setSettings(s);
              push({ kind: "success", title: translate(s.language === "ar" ? "ar" : "en", "settings.saved"), timeout: 2500 });
            }}
          />
        )}
        {modal?.type === "remove" && (
          <RemoveDialog
            items={modal.ids.map((id) => downloads.get(id)).filter((d): d is Download => !!d)}
            deleteFile={modal.deleteFile}
            onClose={() => setModal(null)}
            onConfirm={(del) => void run(() => Promise.all(modal.ids.map((id) => api.remove(id, del))))}
          />
        )}
        {modal?.type === "refresh" && modalDownload && (
          <RefreshLinkDialog
            d={modalDownload}
            onClose={() => setModal(null)}
            onSubmit={async (url) => {
              await api.updateURL(modalDownload.id, url);
              await api.resume(modalDownload.id);
            }}
          />
        )}
        {modal?.type === "limit" && modalDownload && (
          <SpeedLimitDialog d={modalDownload} onClose={() => setModal(null)} onSubmit={async (bps) => {
              await api.setSpeedLimit(modalDownload.id, bps);
              await refresh(); // setSpeedLimit emits no event
            }} />
        )}

        <Toasts toasts={toasts} dismiss={dismiss} />
      </div>
    </I18nProvider>
  );
}

