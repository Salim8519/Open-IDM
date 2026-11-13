import { useEffect, useState } from "react";
import { Check, Copy, FolderOpen } from "lucide-react";
import { Dialog } from "./Dialog";
import { useI18n } from "../i18n";
import type { Client } from "../lib/api";
import type { BrowserKey, BrowserStatus } from "../lib/types";

interface Props {
  client: Client;
  onClose: () => void;
}

const BROWSERS: { key: BrowserKey; label: string; page: string }[] = [
  { key: "chrome", label: "Chrome", page: "chrome://extensions" },
  { key: "edge", label: "Edge", page: "edge://extensions" },
  { key: "brave", label: "Brave", page: "brave://extensions" },
];

/**
 * First-run guide for adding the browser extension. Until the extension is in the stores it
 * has to be loaded unpacked, so this walks through the three steps and turns green on its
 * own when the extension first talks to the app (a "browser" event from the host).
 */
export function ConnectBrowserDialog({ client, onClose }: Props) {
  const { t } = useI18n();
  const [status, setStatus] = useState<BrowserStatus | null>(null);
  const [browser, setBrowser] = useState<BrowserKey>("chrome");
  const [copied, setCopied] = useState(false);
  const [openFailed, setOpenFailed] = useState(false);
  const page = BROWSERS.find((b) => b.key === browser)!.page;

  useEffect(() => {
    client.browserStatus().then(setStatus, () => undefined);
    return client.subscribe((e) => {
      if (e.type === "browser" && e.browser) setStatus(e.browser);
    });
  }, [client]);

  const open = async (key: BrowserKey) => {
    setBrowser(key);
    setOpenFailed(false);
    try {
      await client.openExtensionsPage(key);
    } catch {
      setOpenFailed(true);
    }
  };

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(page);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* clipboard blocked; the address is still visible to type */
    }
  };

  const connected = !!status?.connected;

  return (
    <Dialog
      title={t("browser.title")}
      onClose={onClose}
      testId="connect-browser"
      footer={
        <button className={connected ? "btn primary" : "btn ghost"} onClick={onClose} data-testid="browser-close">
          {connected ? t("browser.done") : t("browser.later")}
        </button>
      }
    >
      <p className="browser-intro">{t("browser.intro")}</p>

      <ol className="browser-steps">
        <li>
          <span className="step-title">{t("browser.step1")}</span>
          <div className="step-actions">
            {BROWSERS.map((b) => (
              <button
                key={b.key}
                className={`btn small${browser === b.key ? " selected" : ""}`}
                onClick={() => void open(b.key)}
                data-testid={`open-${b.key}`}
              >
                {b.label}
              </button>
            ))}
          </div>
          <div className="step-address">
            <code dir="ltr">{page}</code>
            <button className="icon-btn" onClick={() => void copy()} title={t("detail.copy")} aria-label={t("detail.copy")}>
              {copied ? <Check size={14} /> : <Copy size={14} />}
            </button>
          </div>
          {openFailed && <p className="step-hint">{t("browser.openFailed")}</p>}
        </li>
        <li>
          <span className="step-title">{t("browser.step2")}</span>
        </li>
        <li>
          <span className="step-title">{t("browser.step3")}</span>
          {status?.extensionDir ? (
            <>
              <div className="step-address">
                <code dir="ltr" title={status.extensionDir}>
                  <bdi>{status.extensionDir}</bdi>
                </code>
              </div>
              <div className="step-actions">
                <button className="btn small" onClick={() => void client.showExtensionFolder()} data-testid="show-ext-folder">
                  <FolderOpen size={14} />
                  {t("browser.showFolder")}
                </button>
              </div>
            </>
          ) : (
            <p className="step-hint">{t("browser.noBundled")}</p>
          )}
        </li>
      </ol>

      <div className={`browser-status${connected ? " ok" : ""}`} role="status" data-testid="browser-status">
        <span className="status-dot" aria-hidden="true" />
        {connected ? t("browser.connected") : t("browser.waiting")}
      </div>
    </Dialog>
  );
}
