import { useEffect, useRef, useState } from "react";
import { FolderOpen, LoaderCircle, Play, Plus } from "lucide-react";
import { Dialog } from "./Dialog";
import { CategoryIcon } from "./FileIcon";
import { useI18n } from "../i18n";
import { Client } from "../lib/api";
import { formatBytes } from "../lib/format";
import { isHttpURL } from "../lib/batch";
import { Capture, ProbeInfo, Settings } from "../lib/types";

export interface AddInitial extends Partial<Capture> {}

interface Props {
  client: Client;
  settings: Settings | null;
  initial: AddInitial;
  onClose: () => void;
}

export function AddDialog({ client, settings, initial, onClose }: Props) {
  const { t } = useI18n();
  const [url, setUrl] = useState(initial.url ?? "");
  const [dir, setDir] = useState(initial.dir ?? "");
  const [filename, setFilename] = useState(initial.filename ?? "");
  const [connections, setConnections] = useState(initial.connections || settings?.connections || 8);
  const [checksum, setChecksum] = useState(initial.checksum ?? "");
  const [probe, setProbe] = useState<ProbeInfo | null>(null);
  const [probing, setProbing] = useState(false);
  const [probeError, setProbeError] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const seq = useRef(0);
  const headers = initial.headers;

  // Debounced probe: show name/size/resumable as the user types or pastes.
  useEffect(() => {
    const u = url.trim();
    setProbe(null);
    setProbeError("");
    if (!isHttpURL(u)) {
      setProbing(false);
      return;
    }
    const my = ++seq.current;
    setProbing(true);
    const timer = setTimeout(() => {
      client.probe(u, headers).then(
        (p) => {
          if (seq.current !== my) return;
          setProbe(p);
          setProbing(false);
        },
        (err: Error) => {
          if (seq.current !== my) return;
          setProbeError(err.message);
          setProbing(false);
        },
      );
    }, 450);
    return () => clearTimeout(timer);
  }, [url, client, headers]);

  const browse = async () => {
    try {
      const r = await client.pickFolder(dir || settings?.downloadDir || "");
      if (r.dir) setDir(r.dir);
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const submit = async (paused: boolean) => {
    const u = url.trim();
    if (!isHttpURL(u)) {
      setError(t("add.invalidUrl"));
      return;
    }
    setBusy(true);
    setError("");
    try {
      await client.add({
        url: u,
        dir: dir.trim() || undefined,
        // Use the probed name: a paused download is not probed by the engine until it starts.
        filename: filename.trim() || (probe && probe.url === u ? probe.filename : undefined) || undefined,
        connections,
        checksum: checksum.trim() || undefined,
        headers,
        paused,
      });
      onClose();
    } catch (err) {
      setError((err as Error).message);
      setBusy(false);
    }
  };

  const sourceNote =
    initial.source === "extension" ? t("add.fromExtension") : initial.source === "clipboard" ? t("add.fromClipboard") : "";

  return (
    <Dialog
      title={t("add.title")}
      onClose={onClose}
      testId="add-dialog"
      footer={
        <>
          <button className="btn ghost" onClick={onClose}>
            {t("add.cancel")}
          </button>
          <button className="btn" onClick={() => submit(true)} disabled={busy} data-testid="add-paused">
            <Plus size={14} /> {t("add.addPaused")}
          </button>
          <button className="btn primary" onClick={() => submit(false)} disabled={busy} data-testid="add-start">
            <Play size={14} /> {t("add.startNow")}
          </button>
        </>
      }
    >
      <form
        className="form"
        onSubmit={(e) => {
          e.preventDefault();
          void submit(false);
        }}
      >
        {sourceNote && <p className="source-note">{sourceNote}{initial.pageUrl ? ` · ${initial.pageUrl}` : ""}</p>}
        <label className="field">
          <span>{t("add.url")}</span>
          <input
            type="url"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder={t("add.urlPlaceholder")}
            spellCheck={false}
            autoFocus
            data-testid="add-url"
          />
        </label>

        <div className="probe-card" aria-live="polite" data-testid="probe">
          {probing && (
            <span className="muted">
              <LoaderCircle size={14} className="spin" /> {t("add.checking")}
            </span>
          )}
          {probe && (
            <>
              <CategoryIcon category={probe.category} size={22} />
              <div className="probe-info">
                <strong className="ellipsis" data-testid="probe-name">
                  {probe.filename}
                </strong>
                <span className="muted">
                  <span data-testid="probe-size">{probe.size >= 0 ? formatBytes(probe.size) : t("add.unknownSize")}</span>
                  {" · "}
                  <span className={probe.resumable ? "ok" : "warn"}>
                    {probe.resumable ? t("add.resumable") : t("add.notResumable")}
                  </span>
                </span>
              </div>
            </>
          )}
          {probeError && <span className="warn">{t("add.probeFailed", { error: probeError })}</span>}
          {!probing && !probe && !probeError && initial.size ? (
            <span className="muted">{formatBytes(initial.size)}</span>
          ) : null}
        </div>

        <label className="field">
          <span>{t("add.folder")}</span>
          <div className="row">
            <input
              value={dir}
              onChange={(e) => setDir(e.target.value)}
              placeholder={t("add.folderDefault", { dir: settings?.downloadDir ?? "" })}
              spellCheck={false}
              data-testid="add-dir"
            />
            <button type="button" className="btn" onClick={browse} data-testid="add-browse">
              <FolderOpen size={14} /> {t("add.browse")}
            </button>
          </div>
        </label>

        <label className="field">
          <span>{t("add.filename")}</span>
          <input
            value={filename}
            onChange={(e) => setFilename(e.target.value)}
            placeholder={probe?.filename || t("add.filenamePlaceholder")}
            spellCheck={false}
            data-testid="add-filename"
          />
        </label>

        <div className="field-row">
          <label className="field grow">
            <span>
              {t("add.connections")}: <strong data-testid="add-conn-value">{connections}</strong>
            </span>
            <input
              type="range"
              min={1}
              max={32}
              value={connections}
              onChange={(e) => setConnections(Number(e.target.value))}
              data-testid="add-connections"
            />
          </label>
          <label className="field grow">
            <span>{t("add.checksum")}</span>
            <input
              value={checksum}
              onChange={(e) => setChecksum(e.target.value)}
              placeholder={t("add.checksumPlaceholder")}
              spellCheck={false}
              className="mono"
              data-testid="add-checksum"
            />
          </label>
        </div>
        {error && (
          <p className="form-error" role="alert">
            {error}
          </p>
        )}
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
