import { execSync, spawn } from "node:child_process";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const frontend = path.resolve(here, "..");
const desktop = path.resolve(frontend, "..");
export const stateFile = path.join(here, ".server.json");

export default async function globalSetup() {
  if (!process.env.E2E_SKIP_BUILD) {
    execSync("npx vite build", { cwd: frontend, stdio: "inherit" });
  }
  // Build the server binary first (instead of `go run`) so teardown can stop the real
  // process and not just the go wrapper.
  const bin = path.join(mkdtempSync(path.join(tmpdir(), "peregrine-e2ebin-")), process.platform === "win32" ? "e2eserver.exe" : "e2eserver");
  execSync(`go build -o "${bin}" ./cmd/e2eserver`, { cwd: desktop, stdio: "inherit" });

  const proc = spawn(bin, ["-dist", path.join(frontend, "dist"), "-exit-on-stdin-eof"], { cwd: desktop, stdio: ["pipe", "pipe", "inherit"] });
  const info = await new Promise<{ ui: string; files: string; home: string; downloads: string }>((resolve, reject) => {
    let buf = "";
    const timer = setTimeout(() => reject(new Error("e2eserver did not start in 60s")), 60_000);
    proc.stdout!.on("data", (chunk: Buffer) => {
      buf += chunk.toString();
      const nl = buf.indexOf("\n");
      if (nl >= 0) {
        clearTimeout(timer);
        try {
          resolve(JSON.parse(buf.slice(0, nl)));
        } catch (err) {
          reject(err);
        }
      }
    });
    proc.on("exit", (code) => reject(new Error(`e2eserver exited with ${code}`)));
  });
  proc.stdout!.resume();
  writeFileSync(stateFile, JSON.stringify({ ...info, pid: proc.pid }));
  process.env.PEREGRINE_E2E = JSON.stringify(info);
  Object.assign(globalThis, { __e2eServer: proc, __e2eBinDir: path.dirname(bin) });
}
