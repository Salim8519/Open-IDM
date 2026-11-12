import { useState } from "react";
import { Dialog } from "./Dialog";
import { useI18n } from "../i18n";
import { Download } from "../lib/types";
import { isHttpURL } from "../lib/batch";
import { formatBytes, parseSize } from "../lib/format";

export function RemoveDialog({
  items,
  deleteFile: initialDelete,
  onConfirm,
  onClose,
}: {
  items: Download[];
  deleteFile: boolean;
  onConfirm: (deleteFile: boolean) => void;
  onClose: () => void;
}) {
  const { t } = useI18n();
  const [del, setDel] = useState(initialDelete);
  const anyCompleted = items.some((d) => d.status === "completed");
  return (
    <Dialog
      title={t("remove.title")}
      onClose={onClose}
      testId="remove-dialog"
      footer={
        <>
          <button className="btn ghost" onClick={onClose}>
            {t("add.cancel")}
          </button>
          <button
            className="btn danger"
            autoFocus
            data-testid="remove-confirm"
            onClick={() => {
              onConfirm(del);
              onClose();
            }}
          >
            {t("remove.confirm")}
          </button>
        </>
      }
    >
      <p>{items.length === 1 ? t("remove.one", { name: items[0].filename }) : t("remove.many", { n: items.length })}</p>
      {anyCompleted && (
        <label className="check">
          <input type="checkbox" checked={del} onChange={(e) => setDel(e.target.checked)} data-testid="remove-delete-file" />
          {t("remove.deleteFile")}
        </label>
      )}
      <p className="muted small">{t("remove.partNote")}</p>
    </Dialog>
  );
}

export function RefreshLinkDialog({
  d,
  onSubmit,
  onClose,
}: {
  d: Download;
  onSubmit: (url: string) => Promise<void>;
  onClose: () => void;
}) {
  const { t } = useI18n();
  const [url, setUrl] = useState(d.url);
  const [error, setError] = useState("");
  const submit = async () => {
    if (!isHttpURL(url.trim())) {
      setError(t("add.invalidUrl"));
      return;
    }
    try {
      await onSubmit(url.trim());
      onClose();
    } catch (err) {
      setError((err as Error).message);
    }
  };
  return (
    <Dialog
      title={t("refresh.title")}
      onClose={onClose}
      testId="refresh-dialog"
      footer={
        <>
          <button className="btn ghost" onClick={onClose}>
            {t("add.cancel")}
          </button>
          <button className="btn primary" onClick={submit} data-testid="refresh-save">
            {t("refresh.save")}
          </button>
        </>
      }
    >
      <form
        className="form"
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <p className="muted">{t("detail.expiredHint")}</p>
        <label className="field">
          <span>{t("refresh.newUrl")}</span>
          <input
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            spellCheck={false}
            className="mono"
            autoFocus
            onFocus={(e) => e.target.select()}
            data-testid="refresh-url"
          />
        </label>
        {error && (
          <p className="form-error" role="alert">
            {error}
          </p>
        )}
      </form>
    </Dialog>
  );
}

export function SpeedLimitDialog({
  d,
  onSubmit,
  onClose,
}: {
  d: Download;
  onSubmit: (bps: number) => Promise<void>;
  onClose: () => void;
}) {
  const { t } = useI18n();
  const [text, setText] = useState(d.speedLimit ? formatBytes(d.speedLimit).replace(" ", "").replace(/B$/, "") : "");
  const [error, setError] = useState("");
  const submit = async () => {
    const v = parseSize(text);
    if (v === null) {
      setError(t("limit.invalid"));
      return;
    }
    try {
      await onSubmit(v);
      onClose();
    } catch (err) {
      setError((err as Error).message);
    }
  };
  return (
    <Dialog
      title={t("limit.title")}
      onClose={onClose}
      testId="limit-dialog"
      footer={
        <>
          <button className="btn ghost" onClick={onClose}>
            {t("add.cancel")}
          </button>
          <button className="btn primary" onClick={submit} data-testid="limit-save">
            {t("limit.save")}
          </button>
        </>
      }
    >
      <form
        className="form"
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <p className="muted">{d.filename}</p>
        <label className="field">
          <span>{t("limit.hint")}</span>
          <input value={text} onChange={(e) => setText(e.target.value)} placeholder={t("detail.unlimited")} autoFocus data-testid="limit-input" />
        </label>
        {error && (
          <p className="form-error" role="alert">
            {error}
          </p>
        )}
      </form>
    </Dialog>
  );
}
