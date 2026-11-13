import { Download } from "../lib/types";
import { formatPercent, percent } from "../lib/format";
import { displaySegments } from "./SegmentMap";

/**
 * Row progress bar. While a download is split across connections, each connection's
 * downloaded range is drawn at its real position, so you can see the parallel parts
 * filling in — the same picture as the segment map, at row size.
 */
export function ProgressBar({ d }: { d: Download }) {
  const p = d.status === "completed" ? 100 : percent(d.downloaded, d.size);
  const label = formatPercent(p);
  const segs = d.status !== "completed" && d.size > 0 ? displaySegments(d) : [];
  const split = segs.length > 1;
  return (
    <div className="progress-cell">
      <div
        className={`progress status-${d.status}${p < 0 ? " indeterminate" : ""}`}
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={p < 0 ? undefined : Math.round(p)}
        aria-label={label || d.status}
      >
        {split ? (
          segs.map((s, i) => (
            <span
              key={i}
              className="progress-part"
              style={{
                insetInlineStart: `${(s.start / d.size) * 100}%`,
                width: `${(Math.min(s.done, s.end - s.start) / d.size) * 100}%`,
              }}
            />
          ))
        ) : (
          <div className="progress-fill" style={{ width: p < 0 ? undefined : `${p}%` }} />
        )}
      </div>
      <span className="progress-pct" data-testid="pct">
        {label}
      </span>
    </div>
  );
}
