import { Download } from "../lib/types";
import { formatPercent, percent } from "../lib/format";

export function ProgressBar({ d }: { d: Download }) {
  const p = d.status === "completed" ? 100 : percent(d.downloaded, d.size);
  const label = formatPercent(p);
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
        <div className="progress-fill" style={{ width: p < 0 ? undefined : `${p}%` }} />
      </div>
      <span className="progress-pct" data-testid="pct">
        {label}
      </span>
    </div>
  );
}
