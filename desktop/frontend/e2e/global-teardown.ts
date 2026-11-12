import type { ChildProcess } from "node:child_process";
import { rmSync } from "node:fs";
import { stateFile } from "./global-setup";

export default async function globalTeardown() {
  const proc = (globalThis as unknown as { __e2eServer?: ChildProcess }).__e2eServer;
  if (proc && proc.exitCode === null) {
    // Closing stdin asks the server to shut down cleanly (it removes its temp dirs).
    const exited = new Promise((r) => proc.once("exit", r));
    proc.stdin?.end();
    await Promise.race([exited, new Promise((r) => setTimeout(r, 5000))]);
    if (proc.exitCode === null) proc.kill();
  }
  rmSync(stateFile, { force: true });
  const binDir = (globalThis as unknown as { __e2eBinDir?: string }).__e2eBinDir;
  if (binDir) rmSync(binDir, { recursive: true, force: true });
}
