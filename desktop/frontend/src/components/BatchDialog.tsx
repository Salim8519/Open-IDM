import { useMemo, useState } from "react";
import { FolderOpen, Play, Plus } from "lucide-react";
import { Dialog } from "./Dialog";
import { useI18n } from "../i18n";
import { Client } from "../lib/api";
import { parseBatch } from "../lib/batch";
import { Settings } from "../lib/types";

interface Props {
  client: Client;
  settings: Settings | null;
  initialText?: string;
  onClose: () => void;
  onDone: (n: number) => void;
}

export function BatchDialog({ client, settings, initialText = "", onClose, onDone }: Props) {
  const { t } = useI18n();
  const [text, setText] = useState(initialText);
  const [dir, setDir] = useState("");
  const [connections, setConnections] = useState(settings?.connections || 8);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const links = useMemo(() => parseBatch(text), [text]);

  const browse = async () => {
    try {
      const r = await client.pickFolder(dir || settings?.downloadDir || "");
      if (r.dir) setDir(r.dir);
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const submit = async (paused: boolean) => {
    if (!links.length) return;
    setBusy(true);
    setError("");
    let n = 0;
    try {
      for (const url of links) {
        await client.add({ url, dir: dir.trim() || undefined, connections, paused });
        n++;
      }
      onDone(n);
      onClose();
    } catch (err) {
      setError((err as Error).message);
      setBusy(false);
      if (n) onDone(n);
    }
  };

  const preview = links.slice(0, 6);
  return (
    <Dialog
      title={t("batch.title")}
      onClose={onClose}
      wide
      testId="batch-dialog"
      footer={
        <>
          <button className="btn ghost" onClick={onClose}>
            {t("add.cancel")}
          </button>
          <button className="btn" disabled={busy || !links.length} onClick={() => submit(true)} data-testid="batch-paused">
            <Plus size={14} /> {t("add.addPaused")}
          </button>
          <button className="btn primary" disabled={busy || !links.length} onClick={() => submit(false)} data-testid="batch-start">
            <Play size={14} /> {t("batch.add", { n: links.length })}
          </button>
        </>
      }
    >
      <div className="form">
        <p className="muted">{t("batch.hint")}</p>
        <label className="field">
          <span>{t("batch.links")}</span>
          <textarea
            rows={7}
            value={text}
            onChange={(e) => setText(e.target.value)}
            spellCheck={false}
            className="mono"
            placeholder={"https://example.com/file[001-100].zip"}
            data-testid="batch-text"
            autoFocus
          />
        </label>
        <div className="batch-preview" aria-live="polite">
          <strong data-testid="batch-count">{t("batch.count", { n: links.length })}</strong>
          <ul>
            {preview.map((l) => (
              <li key={l} className="mono ellipsis">
                {l}
              </li>
            ))}
            {links.length > preview.length && <li className="muted">{t("batch.more", { n: links.length - preview.length })}</li>}
          </ul>
        </div>
        <div className="field-row">
          <label className="field grow">
            <span>{t("add.folder")}</span>
            <div className="row">
              <input
                value={dir}
                onChange={(e) => setDir(e.target.value)}
                placeholder={t("add.folderDefault", { dir: settings?.downloadDir ?? "" })}
              />
              <button type="button" className="btn" onClick={browse}>
                <FolderOpen size={14} /> {t("add.browse")}
              </button>
            </div>
          </label>
          <label className="field" style={{ minWidth: 160 }}>
            <span>
              {t("add.connections")}: <strong>{connections}</strong>
            </span>
            <input type="range" min={1} max={32} value={connections} onChange={(e) => setConnections(Number(e.target.value))} />
          </label>
        </div>
        {error && (
          <p className="form-error" role="alert">
            {error}
          </p>
        )}
      </div>
    </Dialog>
  );
}
