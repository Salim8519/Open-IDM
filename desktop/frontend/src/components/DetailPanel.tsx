import { useState } from "react";
import { Check, Copy, RefreshCw, X } from "lucide-react";
import { Download } from "../lib/types";
import { useI18n } from "../i18n";
import { ETA_WAITING, formatBytes, formatETA, formatSpeed } from "../lib/format";
import { isExpired } from "../lib/download";
import { SegmentMap } from "./SegmentMap";
import { CategoryIcon } from "./FileIcon";
import { ProgressBar } from "./ProgressBar";

interface Props {
  d: Download;
  onRefreshLink: (d: Download) => void;
  onClose: () => void;
}

export function DetailPanel({ d, onRefreshLink, onClose }: Props) {
  const { t } = useI18n();
  const [copied, setCopied] = useState(false);
  const expired = isExpired(d);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(d.url);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* clipboard may be unavailable */
    }
  };

  return (
    <section className="detail" aria-label={d.filename} data-testid="detail-panel">
      <div className="detail-title">
        <CategoryIcon category={d.category} size={20} />
        <h3 title={d.filename}>{d.filename}</h3>
        <span className={`badge status-${d.status}`}>{expired ? t("status.expired") : t(`status.${d.status}`)}</span>
        <button className="icon-btn" onClick={onClose} aria-label={t("detail.close")} title={t("detail.close")}>
          <X size={16} />
        </button>
      </div>

      <div className="detail-grid">
        <div className="detail-main">
          <SegmentMap d={d} />
          <div className="detail-progress">
            <ProgressBar d={d} />
            <span className="muted detail-numbers" dir="ltr">
              <bdi>{formatBytes(d.downloaded)}</bdi> / <bdi>{d.size >= 0 ? formatBytes(d.size) : t("detail.unknown")}</bdi>
              {d.speed > 0 ? <> · <bdi>{formatSpeed(d.speed)}</bdi></> : ""}
              {d.eta >= 0 && d.status === "downloading" ? <> · <bdi>{formatETA(d.eta)}</bdi></> : ""}
              {d.eta === ETA_WAITING && d.status === "downloading" ? (
                <> · <span className="eta-waiting" title={t("eta.waitingHint")}>{t("eta.waiting")}</span></>
              ) : (
                ""
              )}
            </span>
          </div>

          {d.status === "error" && (
            <div className={`error-box${expired ? " expired" : ""}`} role="alert" data-testid="error-box">
              <div>
                <strong>{expired ? t("status.expired") : t("detail.error")}</strong>
                <p className="error-text">{d.error}</p>
                {expired && <p className="muted">{t("detail.expiredHint")}</p>}
              </div>
              <button className="btn" onClick={() => onRefreshLink(d)}>
                <RefreshCw size={14} /> {t("detail.refreshLink")}
              </button>
            </div>
          )}
        </div>

        <dl className="detail-props">
          <dt>{t("detail.url")}</dt>
          <dd className="url-row">
            <span className="mono ellipsis" title={d.url} data-testid="detail-url">
              {d.url}
            </span>
            <button className="icon-btn" onClick={copy} aria-label={t("detail.copy")} title={t("detail.copy")}>
              {copied ? <Check size={14} /> : <Copy size={14} />}
            </button>
          </dd>
          <dt>{t("detail.folder")}</dt>
          <dd className="mono ellipsis" title={d.dir}>
            {d.dir}
          </dd>
          <dt>{t("detail.resumable")}</dt>
          <dd>{d.resumable ? t("detail.yes") : d.size < 0 ? t("detail.unknown") : t("detail.no")}</dd>
          {d.kind === "torrent" ? (
            <>
              <dt>{t("detail.peers")}</dt>
              <dd>{d.status === "downloading" ? d.active : "—"}</dd>
            </>
          ) : (
            <>
              <dt>{t("detail.connections")}</dt>
              <dd>{d.connections}</dd>
            </>
          )}
          <dt>{t("detail.speedLimit")}</dt>
          <dd><bdi>{d.speedLimit ? formatSpeed(d.speedLimit) : t("detail.unlimited")}</bdi></dd>
          {d.checksum && (
            <>
              <dt>{t("detail.checksum")}</dt>
              <dd className="mono ellipsis" title={d.checksum}>
                {d.checksum}
              </dd>
            </>
          )}
          {d.sha256 && (
            <>
              <dt>{t("detail.sha256")}</dt>
              <dd className="mono break" data-testid="sha256">
                {d.sha256}
              </dd>
            </>
          )}
          <dt>{t("detail.added")}</dt>
          <dd>{new Date(d.createdAt).toLocaleString()}</dd>
          {d.completedAt && (
            <>
              <dt>{t("detail.finished")}</dt>
              <dd>{new Date(d.completedAt).toLocaleString()}</dd>
            </>
          )}
        </dl>
      </div>
    </section>
  );
}
