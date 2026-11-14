import { readFileSync } from "node:fs";
import path from "node:path";
import { expect, rpc, test } from "./fixtures";

// The e2e server's fake release API always answers v99.0.0 (ADR 0020). The version is wrapped
// in Unicode isolates so it stays left-to-right in Arabic, Persian and Urdu.
const available = `Peregrine ${String.fromCharCode(0x2066)}99.0.0${String.fromCharCode(0x2069)} is available`;

test("update check: setting, Check now, statusbar pill and release page", async ({ app, info }) => {
  // On by default, and it can be turned off and on again.
  await app.getByTestId("tb-settings").click();
  const dlg = app.getByTestId("settings-dialog");
  const box = dlg.getByTestId("set-updateCheck");
  await expect(box).toBeChecked();
  await box.uncheck();
  await dlg.getByTestId("settings-save").click();
  await expect(dlg).toBeHidden();
  expect((await rpc<{ updateCheck: boolean }>(app, "getSettings")).updateCheck).toBe(false);

  await app.getByTestId("tb-settings").click();
  await expect(box).not.toBeChecked();
  await box.check();

  // Check now finds the newer version and offers the download.
  await dlg.getByTestId("update-check-now").click();
  await expect(dlg.getByTestId("update-status")).toHaveText(available);
  await dlg.getByTestId("settings-save").click();
  await expect(dlg).toBeHidden();
  expect((await rpc<{ updateCheck: boolean }>(app, "getSettings")).updateCheck).toBe(true);

  // The statusbar shows it too, and clicking opens the release page built by the app.
  const pill = app.getByTestId("update-pill");
  await expect(pill).toHaveText(available);
  await pill.click();
  await expect
    .poll(() => readFileSync(path.join(info.home, "os-hooks.log"), "utf8").trim().split(/\r?\n/).at(-1))
    .toBe("url https://github.com/Salim8519/Peregrine/releases/tag/v99.0.0");

  // A fresh page load still knows about the update (updateStatus).
  await app.reload();
  await expect(app.getByTestId("update-pill")).toBeVisible();
});
