/**
 * Desktop-only bindings outside the RPC API: window focus (for system notifications), the
 * scheduler's when-done countdown, and the platform. All are no-ops in browser mode.
 */
import { WhenDone } from "./types";

export interface PowerEvent {
  action: WhenDone | "";
  seconds: number;
}

interface DesktopWindow {
  go?: {
    main?: {
      App?: {
        SetFocused?(f: boolean): Promise<void>;
        CancelPowerAction?(): Promise<void>;
      };
    };
  };
  runtime?: {
    EventsOn(name: string, cb: (...data: unknown[]) => void): () => void;
    Environment?(): Promise<{ platform: string }>;
  };
}

const w = () => window as unknown as DesktopWindow;

export function setFocused(focused: boolean): void {
  void w()
    .go?.main?.App?.SetFocused?.(focused)
    ?.catch(() => undefined);
}

export function cancelPowerAction(): void {
  void w()
    .go?.main?.App?.CancelPowerAction?.()
    ?.catch(() => undefined);
}

export function onPower(fn: (e: PowerEvent) => void): () => void {
  const off = w().runtime?.EventsOn("peregrine:power", (e) => fn(e as PowerEvent));
  return () => off?.();
}

/** "windows" | "darwin" | "linux", or "" in browser mode. */
export async function platform(): Promise<string> {
  try {
    return (await w().runtime?.Environment?.())?.platform ?? "";
  } catch {
    return "";
  }
}
