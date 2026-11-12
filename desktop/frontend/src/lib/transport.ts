import { PeregrineEvent, RPCError } from "./types";

/** A transport carries RPC calls and events between the UI and the engine. */
export interface Transport {
  readonly kind: "wails" | "http";
  call<T = unknown>(method: string, params?: unknown): Promise<T>;
  subscribe(fn: (e: PeregrineEvent) => void): () => void;
}

interface WailsWindow {
  go?: { main?: { App?: { Call(method: string, params: string): Promise<string> } } };
  runtime?: { EventsOn(name: string, cb: (...data: unknown[]) => void): () => void };
}

function toError(err: unknown): RPCError {
  if (err instanceof RPCError) return err;
  const msg = typeof err === "string" ? err : err instanceof Error ? err.message : String(err);
  return new RPCError("error", msg);
}

/** Desktop app: the Go binding App.Call and Wails runtime events. */
export class WailsTransport implements Transport {
  readonly kind = "wails" as const;
  private w = window as unknown as WailsWindow;

  static available(): boolean {
    const w = window as unknown as WailsWindow;
    return !!w.go?.main?.App?.Call;
  }

  async call<T>(method: string, params?: unknown): Promise<T> {
    try {
      const out = await this.w.go!.main!.App!.Call(method, params === undefined ? "" : JSON.stringify(params));
      return (out ? JSON.parse(out) : null) as T;
    } catch (err) {
      throw toError(err);
    }
  }

  subscribe(fn: (e: PeregrineEvent) => void): () => void {
    const off = this.w.runtime?.EventsOn("peregrine:event", (e) => fn(e as PeregrineEvent));
    return () => off?.();
  }
}

/** Browser mode (e2e, debugging): same-origin POST /rpc and SSE /events. */
export class HttpTransport implements Transport {
  readonly kind = "http" as const;
  constructor(
    private token: string,
    private base = "",
  ) {}

  async call<T>(method: string, params?: unknown): Promise<T> {
    const res = await fetch(`${this.base}/rpc`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${this.token}` },
      body: JSON.stringify({ method, params: params ?? null }),
    });
    if (!res.ok) throw new RPCError("http", `HTTP ${res.status}`);
    const body = (await res.json()) as { result?: T; error?: { code: string; message: string } };
    if (body.error) throw new RPCError(body.error.code, body.error.message);
    return body.result as T;
  }

  subscribe(fn: (e: PeregrineEvent) => void): () => void {
    const es = new EventSource(`${this.base}/events?token=${encodeURIComponent(this.token)}`);
    es.onmessage = (m) => {
      try {
        fn(JSON.parse(m.data) as PeregrineEvent);
      } catch {
        /* ignore malformed */
      }
    };
    return () => es.close();
  }
}

/** Picks Wails inside the desktop app, HTTP (token from ?token=) in a browser. */
export function pickTransport(): Transport {
  if (WailsTransport.available()) return new WailsTransport();
  const token = new URLSearchParams(window.location.search).get("token") ?? "";
  return new HttpTransport(token);
}
