import { Download, Segment } from "../lib/types";
import { useI18n } from "../i18n";
import { formatBytes } from "../lib/format";

/** Segments to draw: the engine's list, or one synthetic segment when it has none. */
export function displaySegments(d: Download): Segment[] {
  if (d.segments && d.segments.length && d.size > 0) return d.segments;
  if (d.status === "completed") return [{ start: 0, end: Math.max(d.size, 1), done: Math.max(d.size, 1) }];
  if (d.size > 0) return [{ start: 0, end: d.size, done: Math.min(d.downloaded, d.size) }];
  return [];
}

/**
 * IDM-style segment map: one block per byte range, positioned by offset, filled by the bytes
 * already on disk. Logical properties make it mirror in RTL.
 */
export function SegmentMap({ d }: { d: Download }) {
  const { t } = useI18n();
  const segs = displaySegments(d);
  const total = d.size > 0 ? d.size : segs.reduce((m, s) => Math.max(m, s.end), 0);
  const running = d.status === "downloading";

  return (
    <div className="segmap-wrap">
      <div className="segmap-head">
        <span className="label">{t("detail.segments")}</span>
        <span className="muted">
          {segs.length > 0 ? t("detail.segmentsHint", { n: segs.length }) : t("detail.unknown")}
          {running && d.connections > 0 ? ` · ${t("detail.connectionsValue", { active: d.active, total: d.connections })}` : ""}
        </span>
      </div>
      <div
        className={`segmap${segs.length === 0 ? " indeterminate" : ""}${running ? " running" : ""}`}
        data-testid="segment-map"
        role="img"
        aria-label={`${t("detail.segments")}: ${segs.length}`}
      >
        {segs.map((s, i) => {
          const len = s.end - s.start;
          const left = (s.start / total) * 100;
          const width = (len / total) * 100;
          const fill = len > 0 ? Math.min(100, (s.done / len) * 100) : 0;
          const active = running && fill < 100;
          return (
            <div
              key={i}
              className={`seg${fill >= 100 ? " done" : ""}${active ? " active" : ""}`}
              data-testid="segment"
              style={{ insetInlineStart: `${left}%`, width: `${width}%` }}
              title={`#${i + 1}: ${formatBytes(s.done)} / ${formatBytes(len)}`}
            >
              <div className="seg-fill" style={{ width: `${fill}%` }} />
            </div>
          );
        })}
      </div>
    </div>
  );
}
