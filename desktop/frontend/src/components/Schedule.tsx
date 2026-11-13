import { useEffect, useState } from "react";
import { CalendarClock } from "lucide-react";
import { Dialog } from "./Dialog";
import { useI18n } from "../i18n";
import { PowerEvent } from "../lib/desktop";
import { ScheduleStatus } from "../lib/types";

/** "02:00" today, or "Thu 02:00" on another day, in the UI language. */
export function formatWhen(iso: string, lang: string, now = new Date()): string {
  const d = new Date(iso);
  const sameDay = d.toDateString() === now.toDateString();
  const opts: Intl.DateTimeFormatOptions = sameDay
    ? { hour: "2-digit", minute: "2-digit" }
    : { weekday: "short", hour: "2-digit", minute: "2-digit" };
  return new Intl.DateTimeFormat(lang, opts).format(d);
}

/** Status-bar note while the schedule is on, with a quick way to turn it off. */
export function ScheduleBadge({
  status,
  hasStop,
  onTurnOff,
}: {
  status: ScheduleStatus;
  hasStop: boolean;
  onTurnOff: () => void;
}) {
  const { t, lang } = useI18n();
  const text = status.open
    ? hasStop && status.nextStop
      ? t("schedule.running", { when: formatWhen(status.nextStop, lang) })
      : t("schedule.runningNoStop")
    : status.nextStart
      ? t("schedule.waiting", { when: formatWhen(status.nextStart, lang) })
      : t("schedule.runningNoStop");
  return (
    <span className={`schedule-badge${status.open ? " open" : ""}`} data-testid="schedule-badge">
      <CalendarClock size={13} aria-hidden="true" />
      <span>{text}</span>
      <button className="link" onClick={onTurnOff} data-testid="schedule-off">
        {t("schedule.off")}
      </button>
    </span>
  );
}

/** Countdown before the scheduler's when-done action; closing the dialog cancels it. */
export function PowerDialog({ event, onCancel }: { event: PowerEvent; onCancel: () => void }) {
  const { t } = useI18n();
  const [left, setLeft] = useState(event.seconds);
  useEffect(() => {
    setLeft(event.seconds);
    const started = Date.now();
    const id = window.setInterval(() => {
      setLeft(Math.max(0, event.seconds - Math.floor((Date.now() - started) / 1000)));
    }, 250);
    return () => window.clearInterval(id);
  }, [event]);
  if (!event.action || event.action === "none") return null;
  const action = event.action;
  return (
    <Dialog
      title={t("power.title")}
      onClose={onCancel}
      testId="power-dialog"
      footer={
        <button className="btn primary" autoFocus onClick={onCancel} data-testid="power-cancel">
          {t("power.cancel")}
        </button>
      }
    >
      <p role="timer" aria-live="polite">
        {t(`power.${action}`, { n: left })}
      </p>
    </Dialog>
  );
}
