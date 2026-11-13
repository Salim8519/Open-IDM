import { useI18n } from "../i18n";
import { DownloadKind } from "../lib/types";

/** Small label for downloads that are not plain HTTP: Torrent, Video site, Stream, FTP. */
export function KindBadge({ kind }: { kind?: DownloadKind }) {
  const { t } = useI18n();
  if (!kind || kind === "http") return null;
  return (
    <span className={`kind-badge kind-${kind}`} data-testid="kind-badge">
      {t(`kind.${kind}`)}
    </span>
  );
}
