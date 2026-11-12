import { ReactNode, useEffect, useLayoutEffect, useRef, useState } from "react";

export interface MenuItem {
  id: string;
  label: string;
  icon?: ReactNode;
  disabled?: boolean;
  danger?: boolean;
  separatorBefore?: boolean;
  onSelect: () => void;
}

interface Props {
  x: number;
  y: number;
  label: string;
  items: MenuItem[];
  onClose: () => void;
}

/** Right-click menu: role=menu, arrow keys move, Enter selects, Escape/outside click closes. */
export function ContextMenu({ x, y, label, items, onClose }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ x, y });

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    setPos({
      x: Math.max(4, Math.min(x, window.innerWidth - r.width - 4)),
      y: Math.max(4, Math.min(y, window.innerHeight - r.height - 4)),
    });
    el.querySelector<HTMLElement>("[role=menuitem]:not([disabled])")?.focus();
  }, [x, y]);

  useEffect(() => {
    const close = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) onClose();
    };
    const onBlur = () => onClose();
    window.addEventListener("mousedown", close);
    window.addEventListener("blur", onBlur);
    window.addEventListener("resize", onBlur);
    return () => {
      window.removeEventListener("mousedown", close);
      window.removeEventListener("blur", onBlur);
      window.removeEventListener("resize", onBlur);
    };
  }, [onClose]);

  const onKeyDown = (e: React.KeyboardEvent) => {
    const list = Array.from(ref.current?.querySelectorAll<HTMLElement>("[role=menuitem]:not([disabled])") ?? []);
    const i = list.indexOf(document.activeElement as HTMLElement);
    if (e.key === "ArrowDown") {
      e.preventDefault();
      list[(i + 1) % list.length]?.focus();
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      list[(i - 1 + list.length) % list.length]?.focus();
    } else if (e.key === "Escape" || e.key === "Tab") {
      e.preventDefault();
      e.stopPropagation();
      onClose();
    }
  };

  return (
    <div
      ref={ref}
      className="ctx-menu"
      role="menu"
      aria-label={label}
      style={{ left: pos.x, top: pos.y }}
      onKeyDown={onKeyDown}
      onContextMenu={(e) => e.preventDefault()}
    >
      {items.map((it) => (
        <div key={it.id} role="none">
          {it.separatorBefore && <div className="ctx-sep" role="separator" />}
          <button
            role="menuitem"
            className={`ctx-item${it.danger ? " danger" : ""}`}
            disabled={it.disabled}
            data-testid={`ctx-${it.id}`}
            onClick={() => {
              onClose();
              it.onSelect();
            }}
          >
            <span className="ctx-icon">{it.icon}</span>
            {it.label}
          </button>
        </div>
      ))}
    </div>
  );
}
