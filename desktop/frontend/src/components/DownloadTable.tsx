import { ChevronDown, ChevronUp } from "lucide-react";
import { KindBadge } from "./KindBadge";
import { Download } from "../lib/types";
import { useI18n, StringKey } from "../i18n";
import { formatBytes, formatETA, formatSpeed } from "../lib/format";
import { isExpired, SortKey } from "../lib/download";
import { CategoryIcon } from "./FileIcon";
import { ProgressBar } from "./ProgressBar";

interface Props {
  rows: Download[];
  selected: Set<string>;
  sortKey: SortKey;
  sortDir: 1 | -1;
  onSort: (k: SortKey) => void;
  onRowClick: (e: React.MouseEvent, id: string) => void;
  onRowContext: (e: React.MouseEvent, id: string) => void;
  onRowOpen: (d: Download) => void;
}

const COLS: { key: SortKey; label: StringKey; cls: string }[] = [
  { key: "name", label: "col.name", cls: "c-name" },
  { key: "size", label: "col.size", cls: "c-size" },
  { key: "progress", label: "col.progress", cls: "c-progress" },
  { key: "speed", label: "col.speed", cls: "c-speed" },
  { key: "eta", label: "col.eta", cls: "c-eta" },
  { key: "status", label: "col.status", cls: "c-status" },
];

export function DownloadTable({ rows, selected, sortKey, sortDir, onSort, onRowClick, onRowContext, onRowOpen }: Props) {
  const { t } = useI18n();
  return (
    <table className="dl-table" aria-label={t("table.label")} aria-multiselectable="true" data-testid="downloads">
      <thead>
        <tr>
          {COLS.map((c) => {
            const active = sortKey === c.key;
            return (
              <th
                key={c.key}
                className={c.cls}
                scope="col"
                aria-sort={active ? (sortDir === 1 ? "ascending" : "descending") : "none"}
              >
                <button className="th-btn" onClick={() => onSort(c.key)}>
                  {t(c.label)}
                  {active && (sortDir === 1 ? <ChevronUp size={12} /> : <ChevronDown size={12} />)}
                </button>
              </th>
            );
          })}
        </tr>
      </thead>
      <tbody>
        {rows.map((d) => {
          const sel = selected.has(d.id);
          const expired = isExpired(d);
          return (
            <tr
              key={d.id}
              className={`${sel ? "selected" : ""} st-${d.status}`}
              aria-selected={sel}
              data-id={d.id}
              data-status={d.status}
              data-testid="download-row"
              tabIndex={-1}
              onMouseDown={(e) => {
                if (e.button === 0) onRowClick(e, d.id);
              }}
              onContextMenu={(e) => onRowContext(e, d.id)}
              onDoubleClick={() => onRowOpen(d)}
            >
              <td className="c-name">
                <div className="name-cell">
                  <CategoryIcon category={d.category} />
                  <span className="ellipsis" title={d.filename} data-testid="row-name">
                    {d.filename}
                  </span>
                  <KindBadge kind={d.kind} />
                </div>
              </td>
              <td className="c-size num" data-testid="row-size">
                <bdi>{d.size >= 0 ? formatBytes(d.size) : d.downloaded > 0 ? formatBytes(d.downloaded) : "—"}</bdi>
              </td>
              <td className="c-progress">
                <ProgressBar d={d} />
              </td>
              <td className="c-speed num"><bdi>{d.status === "downloading" ? formatSpeed(d.speed) : ""}</bdi></td>
              <td className="c-eta num"><bdi>{d.status === "downloading" ? formatETA(d.eta) : ""}</bdi></td>
              <td className="c-status">
                <span className={`badge status-${d.status}${expired ? " expired" : ""}`} data-testid="row-status">
                  {expired ? t("status.expired") : t(`status.${d.status}`)}
                </span>
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}
