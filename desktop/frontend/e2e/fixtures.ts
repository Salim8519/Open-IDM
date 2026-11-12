import { expect, Locator, Page, test as base } from "@playwright/test";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

export interface ServerInfo {
  ui: string;
  files: string;
  home: string;
  downloads: string;
}

const here = path.dirname(fileURLToPath(import.meta.url));

export function serverInfo(): ServerInfo {
  if (process.env.PEREGRINE_E2E) return JSON.parse(process.env.PEREGRINE_E2E);
  return JSON.parse(readFileSync(path.join(here, ".server.json"), "utf8"));
}

/** Calls the RPC API directly (for setup/cleanup only; assertions go through the UI). */
export async function rpc<T = unknown>(page: Page, method: string, params?: unknown): Promise<T> {
  const ui = new URL(serverInfo().ui);
  const token = ui.searchParams.get("token")!;
  const res = await page.request.post(`${ui.origin}/rpc`, {
    headers: { Authorization: `Bearer ${token}`, Origin: ui.origin },
    data: { method, params: params ?? null },
  });
  const body = await res.json();
  if (body.error) throw new Error(body.error.message);
  return body.result as T;
}

export const test = base.extend<{ info: ServerInfo; app: Page }>({
  info: async ({}, use) => {
    await use(serverInfo());
  },
  app: async ({ page, info }, use) => {
    await page.goto(info.ui);
    await expect(page.getByTestId("tb-settings")).toBeEnabled();
    await use(page);
  },
});

export { expect };

/** The table row for a file name. */
export function row(page: Page, name: string): Locator {
  return page.getByTestId("download-row").filter({ has: page.getByTestId("row-name").getByText(name, { exact: true }) });
}

/** Adds a download through the Add dialog. */
export async function addViaDialog(
  page: Page,
  url: string,
  opts: { connections?: number; paused?: boolean; expectName?: string } = {},
) {
  await page.getByTestId("tb-add").click();
  const dlg = page.getByTestId("add-dialog");
  await expect(dlg).toBeVisible();
  await dlg.getByTestId("add-url").fill(url);
  if (opts.expectName) await expect(dlg.getByTestId("probe-name")).toHaveText(opts.expectName);
  if (opts.connections) await dlg.getByTestId("add-connections").fill(String(opts.connections));
  await dlg.getByTestId(opts.paused ? "add-paused" : "add-start").click();
  await expect(dlg).toBeHidden();
}

export async function pct(r: Locator): Promise<number> {
  const s = (await r.getByTestId("pct").textContent()) ?? "";
  return parseFloat(s) || 0;
}
