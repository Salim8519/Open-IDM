import { useCallback, useRef, useState } from "react";
import { CircleCheck, CircleX, Info, Link2, X } from "lucide-react";
import { useI18n } from "../i18n";

export interface Toast {
  id: number;
  kind: "success" | "error" | "info" | "link";
  title: string;
  body?: string;
  action?: { label: string; run: () => void };
  timeout?: number;
}

export function useToasts() {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const next = useRef(1);
  const dismiss = useCallback((id: number) => setToasts((l) => l.filter((x) => x.id !== id)), []);
  const push = useCallback(
    (t: Omit<Toast, "id">) => {
      const id = next.current++;
      setToasts((l) => [...l.slice(-4), { ...t, id }]);
      const ms = t.timeout ?? 5000;
      if (ms > 0) setTimeout(() => dismiss(id), ms);
      return id;
    },
    [dismiss],
  );
  return { toasts, push, dismiss };
}

const icons = { success: CircleCheck, error: CircleX, info: Info, link: Link2 };

export function Toasts({ toasts, dismiss }: { toasts: Toast[]; dismiss: (id: number) => void }) {
  const { t } = useI18n();
  return (
    <div className="toasts" aria-live="polite" role="status">
      {toasts.map((x) => {
        const Icon = icons[x.kind];
        return (
          <div key={x.id} className={`toast ${x.kind}`} data-testid="toast">
            <Icon size={18} aria-hidden="true" className="toast-icon" />
            <div className="toast-text">
              <strong>{x.title}</strong>
              {x.body && <span className="ellipsis">{x.body}</span>}
            </div>
            {x.action && (
              <button
                className="btn primary small"
                onClick={() => {
                  x.action!.run();
                  dismiss(x.id);
                }}
              >
                {x.action.label}
              </button>
            )}
            <button className="icon-btn" onClick={() => dismiss(x.id)} aria-label={t("toast.dismiss")} title={t("toast.dismiss")}>
              <X size={14} />
            </button>
          </div>
        );
      })}
    </div>
  );
}
