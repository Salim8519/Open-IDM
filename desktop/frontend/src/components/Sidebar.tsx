import { CircleCheck, CirclePause, CircleX, Clock, Download as DownloadIcon, LayoutGrid } from "lucide-react";
import { useI18n, StringKey } from "../i18n";
import { CATEGORIES, STATUS_FILTERS, StatusFilter } from "../lib/download";
import { CategoryIcon } from "./FileIcon";

interface Props {
  status: StatusFilter;
  category: string | null;
  statusCounts: Record<StatusFilter, number>;
  categoryCounts: Record<string, number>;
  onStatus: (s: StatusFilter) => void;
  onCategory: (c: string | null) => void;
}

const statusIcons: Record<StatusFilter, typeof LayoutGrid> = {
  all: LayoutGrid,
  downloading: DownloadIcon,
  queued: Clock,
  paused: CirclePause,
  completed: CircleCheck,
  error: CircleX,
};

export function Sidebar({ status, category, statusCounts, categoryCounts, onStatus, onCategory }: Props) {
  const { t } = useI18n();
  return (
    <nav className="sidebar" aria-label={t("sidebar.status")}>
      <h4 className="side-head">{t("sidebar.status")}</h4>
      <ul role="list">
        {STATUS_FILTERS.map((s) => {
          const Icon = statusIcons[s];
          const active = status === s && category === null;
          return (
            <li key={s}>
              <button
                className={`side-item${active ? " active" : ""} f-${s}`}
                aria-current={active ? "true" : undefined}
                data-testid={`filter-${s}`}
                onClick={() => {
                  onStatus(s);
                  onCategory(null);
                }}
              >
                <Icon size={16} aria-hidden="true" />
                <span className="side-label">{t(`filter.${s}` as StringKey)}</span>
                <span className="count">{statusCounts[s] ?? 0}</span>
              </button>
            </li>
          );
        })}
      </ul>
      <h4 className="side-head">{t("sidebar.categories")}</h4>
      <ul role="list">
        {CATEGORIES.filter((c) => c !== "Disk Images" || (categoryCounts[c] ?? 0) > 0).map((c) => {
          const active = category === c;
          return (
            <li key={c}>
              <button
                className={`side-item${active ? " active" : ""}`}
                aria-current={active ? "true" : undefined}
                data-testid={`cat-${c}`}
                onClick={() => {
                  onCategory(c);
                  onStatus("all");
                }}
              >
                <CategoryIcon category={c} />
                <span className="side-label">{t(`cat.${c}` as StringKey)}</span>
                <span className="count">{categoryCounts[c] ?? 0}</span>
              </button>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
