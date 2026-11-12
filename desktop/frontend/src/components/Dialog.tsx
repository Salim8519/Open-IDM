import { ReactNode, useEffect, useId, useRef } from "react";
import { X } from "lucide-react";
import { useI18n } from "../i18n";

interface Props {
  title: string;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  wide?: boolean;
  testId?: string;
}

/** Modal dialog: role=dialog, Escape closes, focus moves inside and is restored on close. */
export function Dialog({ title, onClose, children, footer, wide, testId }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  const titleId = useId();
  const { t } = useI18n();

  useEffect(() => {
    const prev = document.activeElement as HTMLElement | null;
    const el = ref.current;
    const first = el?.querySelector<HTMLElement>("[autofocus], input, textarea, select, button:not(.dialog-close)");
    first?.focus();
    return () => prev?.focus?.();
  }, []);

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") {
      e.stopPropagation();
      onClose();
    } else if (e.key === "Tab" && ref.current) {
      const items = ref.current.querySelectorAll<HTMLElement>(
        "button:not([disabled]), input:not([disabled]), textarea, select, [tabindex='0']",
      );
      if (!items.length) return;
      const first = items[0];
      const last = items[items.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    }
  };

  return (
    <div className="overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div
        ref={ref}
        className={`dialog${wide ? " wide" : ""}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        data-testid={testId}
        onKeyDown={onKeyDown}
      >
        <header className="dialog-head">
          <h2 id={titleId}>{title}</h2>
          <button className="icon-btn dialog-close" onClick={onClose} aria-label={t("add.cancel")} title={t("add.cancel")}>
            <X size={16} />
          </button>
        </header>
        <div className="dialog-body">{children}</div>
        {footer && <footer className="dialog-foot">{footer}</footer>}
      </div>
    </div>
  );
}
