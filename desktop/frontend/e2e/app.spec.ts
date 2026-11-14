import { existsSync, readFileSync, statSync } from "node:fs";
import path from "node:path";
import { addViaDialog, expect, pct, row, rpc, test } from "./fixtures";

// Tests share one engine instance and run in order (workers: 1).
test.describe.configure({ mode: "serial" });

const MB = 1024 * 1024;

test("empty state and version on first start", async ({ app }) => {
  await expect(app.getByTestId("empty")).toBeVisible();
  await expect(app.getByTestId("empty")).toContainText("No downloads yet");
  await expect(app.locator(".statusbar")).toContainText("Peregrine");
  await expect(app.locator("html")).toHaveAttribute("dir", "ltr");
  // A fresh install uses the Field Guide theme (ADR 0017), in the system's edition.
  await expect(app.locator("html")).toHaveAttribute("data-theme", /^field-(day|night)$/);
});

test("add via the Add dialog downloads the file with the correct size", async ({ app, info }) => {
  const name = "e2e-add.bin";
  await addViaDialog(app, `${info.files}/file?size=${3 * MB}&name=${name}`, { expectName: name, connections: 4 });
  const r = row(app, name);
  await expect(r).toBeVisible();
  await expect(r).toHaveAttribute("data-status", "completed", { timeout: 30_000 });
  await expect(r.getByTestId("row-size")).toHaveText("3 MB");
  await expect(r.getByTestId("pct")).toHaveText("100%");
  await expect(app.getByTestId("toast").filter({ hasText: "Download complete" })).toBeVisible();

  // details: SHA-256 after completion, URL shown
  await r.click();
  const detail = app.getByTestId("detail-panel");
  await expect(detail).toBeVisible();
  await expect(detail.getByTestId("sha256")).toHaveText(/^[0-9a-f]{64}$/);
  await expect(detail.getByTestId("detail-url")).toContainText(`name=${name}`);

  // the real file is on disk with the right size
  const file = path.join(info.downloads, name);
  expect(existsSync(file)).toBe(true);
  expect(statSync(file).size).toBe(3 * MB);
});

test("pause and resume a throttled download; segment map renders", async ({ app, info }) => {
  const name = "e2e-throttle.bin";
  await addViaDialog(app, `${info.files}/file?size=${6 * MB}&throttle=150000&name=${name}`, {
    expectName: name,
    connections: 4,
  });
  const r = row(app, name);
  await expect(r).toHaveAttribute("data-status", "downloading");
  await r.click();

  // segment map: one block per connection once the engine has split the file
  const map = app.getByTestId("detail-panel").getByTestId("segment-map");
  await expect(map).toBeVisible();
  await expect.poll(async () => map.getByTestId("segment").count(), { timeout: 15_000 }).toBeGreaterThanOrEqual(2);

  await expect.poll(() => pct(r)).toBeGreaterThan(0);
  await app.getByTestId("tb-pause").click();
  await expect(r).toHaveAttribute("data-status", "paused");
  const paused = await pct(r);
  await app.waitForTimeout(1500);
  expect(await pct(r)).toBe(paused); // no progress while paused

  await app.getByTestId("tb-resume").click();
  await expect(r).toHaveAttribute("data-status", "downloading");
  await expect.poll(() => pct(r), { timeout: 20_000 }).toBeGreaterThan(paused);

  // Space toggles pause for the selection
  await app.locator("body").press("Space");
  await expect(r).toHaveAttribute("data-status", "paused");
});

test("remove asks for confirmation; remove + delete file deletes it", async ({ app, info }) => {
  // unfinished download: cancel keeps it, confirm removes it
  const r = row(app, "e2e-throttle.bin");
  await r.click();
  await app.getByTestId("tb-remove").click();
  const dlg = app.getByTestId("remove-dialog");
  await expect(dlg).toBeVisible();
  await dlg.getByRole("button", { name: "Cancel" }).first().click();
  await expect(dlg).toBeHidden();
  await expect(r).toBeVisible();

  await r.click();
  await app.keyboard.press("Delete");
  await expect(dlg).toBeVisible();
  await dlg.getByTestId("remove-confirm").click();
  await expect(r).toHaveCount(0);

  // completed download via the context menu: "Remove and delete file"
  const name = "e2e-delete-me.bin";
  await addViaDialog(app, `${info.files}/file?size=200000&name=${name}`, { expectName: name });
  const done = row(app, name);
  await expect(done).toHaveAttribute("data-status", "completed");
  const file = path.join(info.downloads, name);
  expect(existsSync(file)).toBe(true);
  await done.click({ button: "right" });
  await expect(app.getByRole("menu")).toBeVisible();
  await app.getByTestId("ctx-remove-delete").click();
  await expect(dlg).toBeVisible();
  await expect(dlg.getByTestId("remove-delete-file")).toBeChecked();
  await dlg.getByTestId("remove-confirm").click();
  await expect(done).toHaveCount(0);
  await expect.poll(() => existsSync(file)).toBe(false);
});

test("expired link shows the error state and Refresh link recovers it", async ({ app, info }) => {
  const name = "e2e-expired.bin";
  await app.getByTestId("tb-add").click();
  const dlg = app.getByTestId("add-dialog");
  await dlg.getByTestId("add-url").fill(`${info.files}/file/${name}?size=300000&expire=0`);
  await expect(dlg.getByTestId("probe")).toContainText("Could not check the link");
  await dlg.getByTestId("add-start").click();

  const r = row(app, name);
  await expect(r).toHaveAttribute("data-status", "error");
  await expect(r.getByTestId("row-status")).toHaveText("Link expired");
  await expect(app.getByTestId("toast").filter({ hasText: "Download failed" })).toBeVisible();

  await r.click();
  const box = app.getByTestId("error-box");
  await expect(box).toBeVisible();
  await expect(box).toContainText("403");
  await box.getByRole("button", { name: "Refresh link" }).click();

  const refresh = app.getByTestId("refresh-dialog");
  await expect(refresh).toBeVisible();
  await refresh.getByTestId("refresh-url").fill(`${info.files}/file/${name}?size=300000`);
  await refresh.getByTestId("refresh-save").click();
  await expect(r).toHaveAttribute("data-status", "completed");
  expect(statSync(path.join(info.downloads, name)).size).toBe(300000);
});

test("batch add expands patterns", async ({ app, info }) => {
  await app.getByTestId("tb-batch").click();
  const dlg = app.getByTestId("batch-dialog");
  await expect(dlg).toBeVisible();
  await dlg
    .getByTestId("batch-text")
    .fill(`${info.files}/file/clip[1-3].mp4?size=20000\n\nnot a link\n${info.files}/file/report.pdf?size=10000\n`);
  await expect(dlg.getByTestId("batch-count")).toHaveText("4 links");
  await dlg.getByTestId("batch-start").click();
  await expect(dlg).toBeHidden();
  for (const n of ["clip1.mp4", "clip2.mp4", "clip3.mp4", "report.pdf"]) {
    await expect(row(app, n)).toHaveAttribute("data-status", "completed");
  }
});

test("filter by category and status, and search", async ({ app, info }) => {
  // a paused download for the status filter
  await addViaDialog(app, `${info.files}/file?size=100000&name=e2e-paused.zip`, { expectName: "e2e-paused.zip", paused: true });
  await expect(row(app, "e2e-paused.zip")).toHaveAttribute("data-status", "paused");

  const rows = app.getByTestId("download-row");
  await app.getByTestId("cat-Video").click();
  await expect(rows).toHaveCount(3);
  await expect(app.getByTestId("cat-Video").locator(".count")).toHaveText("3");
  await expect(rows.getByTestId("row-name")).toHaveText(["clip1.mp4", "clip2.mp4", "clip3.mp4"]);

  await app.getByTestId("cat-Documents").click();
  await expect(rows).toHaveCount(1);
  await expect(rows.getByTestId("row-name")).toHaveText(["report.pdf"]);

  await app.getByTestId("cat-Compressed").click();
  await expect(rows.getByTestId("row-name")).toHaveText(["e2e-paused.zip"]);

  await app.getByTestId("filter-paused").click();
  await expect(rows.getByTestId("row-name")).toHaveText(["e2e-paused.zip"]);

  await app.getByTestId("filter-completed").click();
  const n = await rows.count();
  expect(n).toBeGreaterThanOrEqual(6);
  for (let i = 0; i < n; i++) await expect(rows.nth(i)).toHaveAttribute("data-status", "completed");

  await app.getByTestId("filter-all").click();
  await app.getByTestId("search").fill("clip2");
  await expect(rows.getByTestId("row-name")).toHaveText(["clip2.mp4"]);
  await app.getByTestId("search").fill("nothing-matches-this");
  await expect(app.getByTestId("empty")).toContainText("Nothing matches");
  await app.getByTestId("search").fill("");
  await expect(rows).toHaveCount(n + 1);

  // sort by name descending via header clicks
  await app.getByRole("button", { name: "Name" }).click();
  await app.getByRole("button", { name: "Name" }).click();
  await expect(app.locator("th.c-name")).toHaveAttribute("aria-sort", "descending");
  const names = await rows.getByTestId("row-name").allTextContents();
  expect(names).toEqual([...names].sort((a, b) => b.localeCompare(a, undefined, { numeric: true, sensitivity: "base" })));
});

test("settings persist after reload", async ({ app }) => {
  await app.getByTestId("tb-settings").click();
  const dlg = app.getByTestId("settings-dialog");
  await expect(dlg).toBeVisible();
  await dlg.getByTestId("set-maxActive").fill("5");
  await dlg.getByTestId("set-connections").fill("12");
  await dlg.getByTestId("set-ua").fill("PeregrineE2E/1.0");
  await dlg.getByTestId("set-limit").fill("");
  await dlg.getByTestId("set-theme").selectOption("light");
  await dlg.getByTestId("settings-save").click();
  await expect(dlg).toBeHidden();
  await expect(app.locator("html")).toHaveAttribute("data-theme", "light");

  await app.reload();
  await expect(app.getByTestId("tb-settings")).toBeEnabled();
  await expect(app.locator("html")).toHaveAttribute("data-theme", "light");
  await app.getByTestId("tb-settings").click();
  await expect(dlg.getByTestId("set-maxActive")).toHaveValue("5");
  await expect(dlg.getByTestId("set-connections")).toHaveValue("12");
  await expect(dlg.getByTestId("set-ua")).toHaveValue("PeregrineE2E/1.0");
  await dlg.getByRole("button", { name: "Cancel" }).first().click();

  const s = await rpc<{ maxActive: number; connections: number }>(app, "getSettings");
  expect(s.maxActive).toBe(5);
  expect(s.connections).toBe(12);

  // the Add dialog uses the new default connection count
  await app.keyboard.press("Control+n");
  await expect(app.getByTestId("add-dialog").getByTestId("add-conn-value")).toHaveText("12");
  await app.keyboard.press("Escape");
  await expect(app.getByTestId("add-dialog")).toBeHidden();
});

test("switching to Arabic sets RTL and persists", async ({ app }) => {
  await app.getByTestId("tb-settings").click();
  const dlg = app.getByTestId("settings-dialog");
  await dlg.getByTestId("set-language").selectOption("ar");
  await dlg.getByTestId("settings-save").click();
  const html = app.locator("html");
  await expect(html).toHaveAttribute("dir", "rtl");
  await expect(html).toHaveAttribute("lang", "ar");
  await expect(app.getByTestId("tb-add")).toContainText("إضافة رابط");

  // the sidebar is mirrored: it sits on the right of the table
  const side = await app.locator(".sidebar").boundingBox();
  const main = await app.locator(".main").boundingBox();
  expect(side!.x).toBeGreaterThan(main!.x);

  await app.reload();
  await expect(html).toHaveAttribute("dir", "rtl");
  await expect(app.getByTestId("filter-all")).toContainText("الكل");

  // back to English
  await app.getByTestId("tb-settings").click();
  await dlg.getByTestId("set-language").selectOption("en");
  await dlg.getByTestId("settings-save").click();
  await expect(html).toHaveAttribute("dir", "ltr");
  await expect(app.getByTestId("tb-add")).toContainText("Add URL");
});

test("context menu: move up/down and speed limit", async ({ app }) => {
  const rows = app.getByTestId("download-row");
  const before = await rows.getByTestId("row-name").allTextContents();
  const target = before[before.length - 1];
  await row(app, target).click({ button: "right" });
  await app.getByTestId("ctx-up").click();
  await expect.poll(async () => (await rows.getByTestId("row-name").allTextContents()).indexOf(target)).toBe(before.length - 2);

  await row(app, "e2e-paused.zip").click({ button: "right" });
  await app.getByTestId("ctx-limit").click();
  const dlg = app.getByTestId("limit-dialog");
  await dlg.getByTestId("limit-input").fill("512K");
  await dlg.getByTestId("limit-save").click();
  await expect(dlg).toBeHidden();
  await row(app, "e2e-paused.zip").click();
  await expect(app.getByTestId("detail-panel")).toContainText("512 KB/s");
});

test("pause all, resume all and clear completed", async ({ app }) => {
  await app.getByTestId("tb-resume-all").click();
  await expect(row(app, "e2e-paused.zip")).toHaveAttribute("data-status", "completed");
  await app.getByTestId("tb-clear").click();
  await expect(app.getByTestId("empty")).toBeVisible();
  await expect(app.getByTestId("filter-completed").locator(".count")).toHaveText("0");
});

test("capture event from the extension opens a prefilled Add dialog", async ({ app, info }) => {
  const url = `${info.files}/file?size=50000&name=e2e-captured.bin`;
  const res = await rpc<{ prompted?: boolean }>(app, "capture", {
    url,
    source: "extension",
    pageUrl: "https://example.com/page",
    headers: { Referer: "https://example.com/page" },
  });
  expect(res.prompted).toBe(true);
  const dlg = app.getByTestId("add-dialog");
  await expect(dlg).toBeVisible();
  await expect(dlg.getByTestId("add-url")).toHaveValue(url);
  await expect(dlg).toContainText("Offered by your browser");
  await expect(dlg.getByTestId("probe-name")).toHaveText("e2e-captured.bin");
  await dlg.getByTestId("add-start").click();
  await expect(row(app, "e2e-captured.bin")).toHaveAttribute("data-status", "completed");
  await app.screenshot({ path: "test-results/screenshot-main.png" });
  await row(app, "e2e-captured.bin").click();
  await app.screenshot({ path: "test-results/screenshot-detail.png" });
});

test("status bar shows total speed as plain text", async ({ app }) => {
  // Guards against stray escape sequences leaking into the status bar text.
  await expect(app.getByTestId("sb-speed")).toHaveText(/^Total [⁦]?[\d.]+ (B|KB|MB|GB)\/s[⁩]?$/);
});

test("first run asks to connect the browser and turns green when the extension checks in", async ({ app, info }) => {
  await app.goto(`${info.ui}&onboarding=1`);
  const dlg = app.getByTestId("connect-browser");
  await expect(dlg).toBeVisible();
  await expect(dlg).toContainText("Connect your browser");
  await expect(dlg.getByTestId("browser-status")).toHaveText("Waiting for the extension…");
  await expect(dlg).toContainText(path.join("Peregrine", "extension"));
  await dlg.getByTestId("open-edge").click();
  await expect(dlg.locator("code").first()).toHaveText("edge://extensions");

  // What the native messaging host sends when the extension starts.
  await rpc(app, "extensionHello");
  await expect(dlg.getByTestId("browser-status")).toContainText("Connected");
  await dlg.getByTestId("browser-close").click();
  await expect(dlg).toBeHidden();

  // Asked once: a reload does not ask again, but Settings can reopen it.
  await app.reload();
  await expect(app.getByTestId("tb-settings")).toBeEnabled();
  await expect(app.getByTestId("connect-browser")).toBeHidden();
  await app.getByTestId("tb-settings").click();
  await app.getByTestId("set-connect-browser").click();
  await expect(app.getByTestId("connect-browser").getByTestId("browser-status")).toContainText("Connected");
});

test("schedule holds the queue until turned off", async ({ app, info }) => {
  const hhmm = (d: Date) => `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
  const now = Date.now();
  await app.getByTestId("tb-settings").click();
  const dlg = app.getByTestId("settings-dialog");
  await dlg.getByTestId("set-schedule-enabled").check();
  // A window that starts in two hours, so it is closed now.
  await dlg.getByTestId("set-schedule-start").fill(hhmm(new Date(now + 2 * 3600_000)));
  await dlg.getByTestId("set-schedule-stop").fill(hhmm(new Date(now + 3 * 3600_000)));
  await dlg.getByTestId("settings-save").click();
  await expect(dlg).toBeHidden();
  await expect(app.getByTestId("schedule-badge")).toContainText("Scheduled");

  const name = "e2e-scheduled.bin";
  await rpc(app, "add", { url: `${info.files}/file?size=200000&name=${name}`, filename: name });
  const r = row(app, name);
  await expect(r).toHaveAttribute("data-status", "queued");
  await app.waitForTimeout(1500);
  await expect(r).toHaveAttribute("data-status", "queued");
  const st = await rpc<{ enabled: boolean; open: boolean; nextStart?: string }>(app, "scheduleStatus");
  expect(st.enabled).toBe(true);
  expect(st.open).toBe(false);
  expect(st.nextStart).toBeTruthy();

  await app.getByTestId("schedule-off").click();
  await expect(app.getByTestId("schedule-badge")).toBeHidden();
  await expect(r).toHaveAttribute("data-status", "completed", { timeout: 30_000 });
  const s = await rpc<{ schedule: { enabled: boolean } }>(app, "getSettings");
  expect(s.schedule.enabled).toBe(false);
});

test("row menu: pause, resume, copy link, restart, move down; Ctrl+A and Escape", async ({ app, info, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"], { origin: new URL(info.ui).origin });
  const a = "e2e-menu-a.bin";
  const b = "e2e-menu-b.bin";
  const urlA = `${info.files}/file?size=${8 * MB}&throttle=200000&name=${a}`;
  await addViaDialog(app, urlA, { expectName: a, connections: 2 });
  await addViaDialog(app, `${info.files}/file?size=${8 * MB}&name=${b}`, { expectName: b, paused: true });
  const ra = row(app, a);
  await expect(ra).toHaveAttribute("data-status", "downloading");

  // Pause and Resume from the menu
  await ra.click({ button: "right" });
  await app.getByTestId("ctx-pause").click();
  await expect(ra).toHaveAttribute("data-status", "paused");
  await ra.click({ button: "right" });
  await expect(app.getByTestId("ctx-pause")).toBeDisabled();
  await app.getByTestId("ctx-resume").click();
  await expect(ra).toHaveAttribute("data-status", "downloading");

  // Copy link puts the download's address on the clipboard
  await ra.click({ button: "right" });
  await app.getByTestId("ctx-copy").click();
  await expect.poll(() => app.evaluate(() => navigator.clipboard.readText())).toBe(urlA);

  // The details panel's copy button does the same
  await app.evaluate(() => navigator.clipboard.writeText(""));
  await ra.click();
  await app.getByTestId("detail-panel").getByRole("button", { name: "Copy" }).click();
  await expect.poll(() => app.evaluate(() => navigator.clipboard.readText())).toBe(urlA);

  // Restart starts over from zero
  await expect.poll(() => pct(ra), { timeout: 20_000 }).toBeGreaterThan(3);
  const before = await pct(ra);
  await ra.click({ button: "right" });
  await app.getByTestId("ctx-restart").click();
  await expect.poll(() => pct(ra)).toBeLessThan(before);

  // Move down swaps it with the next row
  const rows = app.getByTestId("download-row");
  const names = () => rows.getByTestId("row-name").allTextContents();
  const order = await names();
  expect(order.indexOf(a)).toBeLessThan(order.indexOf(b));
  await ra.click({ button: "right" });
  await app.getByTestId("ctx-down").click();
  await expect.poll(async () => (await names()).indexOf(a)).toBe(order.indexOf(a) + 1);

  // Ctrl+A selects every row, Escape clears the selection
  await app.locator("body").press("Control+a");
  await expect(rows.and(app.locator('[aria-selected="true"]'))).toHaveCount(await rows.count());
  await app.locator("body").press("Escape");
  await expect(rows.and(app.locator('[aria-selected="true"]'))).toHaveCount(0);

  // Pause all stops the running download
  await expect(ra).toHaveAttribute("data-status", "downloading");
  await app.getByTestId("tb-pause-all").click();
  await expect(ra).toHaveAttribute("data-status", "paused");

  // clean up: remove both and their files
  await rpc(app, "remove", { id: await ra.getAttribute("data-id"), deleteFile: true });
  await rpc(app, "remove", { id: await row(app, b).getAttribute("data-id"), deleteFile: true });
});

test("Open file and Show in folder ask the OS for the right file", async ({ app, info }) => {
  const name = "e2e-reveal.bin";
  await addViaDialog(app, `${info.files}/file?size=100000&name=${name}`, { expectName: name });
  const r = row(app, name);
  await expect(r).toHaveAttribute("data-status", "completed");
  const id = await r.getAttribute("data-id");
  // The browser-mode UI has no OS access, so both menu items are disabled there…
  await r.click({ button: "right" });
  await expect(app.getByTestId("ctx-open")).toBeDisabled();
  await expect(app.getByTestId("ctx-folder")).toBeDisabled();
  await app.keyboard.press("Escape");
  // …and the desktop app calls these API methods; the e2e server records the path.
  expect(await rpc(app, "showInFolder", { id })).toBe(true);
  expect(await rpc(app, "openFile", { id })).toBe(true);
  const log = readFileSync(path.join(info.home, "os-hooks.log"), "utf8").trim().split(/\r?\n/);
  const file = path.join(info.downloads, name);
  expect(log.slice(-2)).toEqual([`reveal ${file}`, `open ${file}`]);
});

test("every setting is saved, shown again and takes effect", async ({ app, info }) => {
  for (const d of await rpc<{ id: string }[]>(app, "list")) await rpc(app, "remove", { id: d.id, deleteFile: true });
  const picked = path.join(info.downloads, "picked");
  const dlg = app.getByTestId("settings-dialog");

  // A bad size is refused with a message, and nothing is saved.
  await app.getByTestId("tb-settings").click();
  await dlg.getByTestId("set-minExt").fill("lots");
  await dlg.getByTestId("settings-save").click();
  await expect(dlg.getByRole("alert")).toBeVisible();
  await expect(dlg).toBeVisible();

  await dlg.getByTestId("set-dir").fill(picked);
  await dlg.getByTestId("set-categories").check();
  await dlg.getByTestId("set-maxActive").fill("1");
  await dlg.getByTestId("set-retries").fill("3");
  await dlg.getByTestId("set-limit").fill("200K");
  await dlg.getByTestId("set-proxy").fill("");
  await dlg.getByTestId("set-autoStartExt").check();
  await dlg.getByTestId("set-minExt").fill("5M");
  await dlg.getByTestId("set-notify").uncheck();
  await dlg.getByTestId("set-closeToTray").uncheck();
  await dlg.getByTestId("settings-save").click();
  await expect(dlg).toBeHidden();

  // Saved in the engine…
  const s = await rpc<Record<string, unknown>>(app, "getSettings");
  expect(s).toMatchObject({
    downloadDir: picked,
    useCategories: true,
    maxActive: 1,
    maxRetries: 3,
    globalLimit: 200 * 1024,
    proxy: "",
    autoStartExt: true,
    minExtSize: 5 * MB,
    notify: false,
    closeToTray: false,
  });
  // …and shown again, in the same units, after a reload.
  await app.reload();
  await app.getByTestId("tb-settings").click();
  await expect(dlg.getByTestId("set-dir")).toHaveValue(picked);
  await expect(dlg.getByTestId("set-categories")).toBeChecked();
  await expect(dlg.getByTestId("set-maxActive")).toHaveValue("1");
  await expect(dlg.getByTestId("set-retries")).toHaveValue("3");
  await expect(dlg.getByTestId("set-limit")).toHaveValue("200K");
  await expect(dlg.getByTestId("set-autoStartExt")).toBeChecked();
  await expect(dlg.getByTestId("set-minExt")).toHaveValue("5M");
  await expect(dlg.getByTestId("set-notify")).not.toBeChecked();
  await expect(dlg.getByTestId("set-closeToTray")).not.toBeChecked();
  await dlg.getByRole("button", { name: "Cancel" }).first().click();

  // Max active 1: the second download waits in the queue.
  const a = "e2e-set-a.zip";
  const b = "e2e-set-b.zip";
  await rpc(app, "add", { url: `${info.files}/file?size=${3 * MB}&name=${a}`, filename: a });
  await rpc(app, "add", { url: `${info.files}/file?size=${MB}&name=${b}`, filename: b });
  await expect(row(app, a)).toHaveAttribute("data-status", "downloading");
  await expect(row(app, b)).toHaveAttribute("data-status", "queued");

  // Global limit 200 KB/s holds the speed down (the test server is unthrottled). Measured as
  // bytes over a 3 s window after the start: the first half second may burst above the
  // limit (one buffer per connection), and a single speed reading then is not the limit.
  type Item = { filename: string; speed: number; downloaded: number; dir: string };
  const getA = async () => (await rpc<Item[]>(app, "list")).find((d) => d.filename === a)!;
  await app.waitForTimeout(2000);
  const t0 = Date.now();
  const d0 = (await getA()).downloaded;
  await app.waitForTimeout(3000);
  const da = await getA();
  const rate = (da.downloaded - d0) / ((Date.now() - t0) / 1000);
  expect(rate).toBeGreaterThan(100 * 1024);
  expect(rate).toBeLessThan(260 * 1024);
  expect(da.speed).toBeGreaterThan(0);
  expect(da.downloaded).toBeLessThan(2 * MB);

  // Download folder + categories: a .zip goes to <picked>/Compressed.
  expect(da.dir).toBe(path.join(picked, "Compressed"));

  // Start browser downloads without asking: a capture is queued directly, no dialog.
  const c = "e2e-set-captured.zip";
  const res = await rpc<{ queued: boolean }>(app, "capture", { url: `${info.files}/file/${c}?size=1000`, source: "extension" });
  expect(res.queued).toBe(true);
  await expect(app.getByTestId("add-dialog")).toBeHidden();
  await expect(row(app, c)).toBeVisible();

  // Put everything back for the other tests.
  for (const d of await rpc<{ id: string }[]>(app, "list")) await rpc(app, "remove", { id: d.id, deleteFile: true });
  await rpc(app, "updateSettings", {
    ...s,
    downloadDir: info.downloads,
    useCategories: false,
    maxActive: 3,
    maxRetries: 10,
    globalLimit: 0,
    autoStartExt: false,
    minExtSize: MB,
    notify: true,
    closeToTray: true,
  });
});
