import { useState } from "react";
import { FolderOpen } from "lucide-react";
import { Dialog } from "./Dialog";
import { useI18n } from "../i18n";
import { Client } from "../lib/api";
import { formatBytes, parseSize } from "../lib/format";
import { Settings } from "../lib/types";
import logo from "../logo.svg";

interface Props {
  client: Client;
  settings: Settings;
  version: string;
  onClose: () => void;
  onSaved: (s: Settings) => void;
}

function sizeText(n: number): string {
  return n > 0 ? formatBytes(n).replace(" ", "").replace(/B$/, "") : "";
}

export function SettingsDialog({ client, settings, version, onClose, onSaved }: Props) {
  const { t } = useI18n();
  const [s, setS] = useState<Settings>(settings);
  const [limit, setLimit] = useState(sizeText(settings.globalLimit));
  const [minExt, setMinExt] = useState(sizeText(settings.minExtSize));
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const set = <K extends keyof Settings>(k: K, v: Settings[K]) => setS((o) => ({ ...o, [k]: v }));

  const browse = async () => {
    try {
      const r = await client.pickFolder(s.downloadDir);
      if (r.dir) set("downloadDir", r.dir);
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const save = async () => {
    const gl = parseSize(limit);
    const me = parseSize(minExt);
    if (gl === null || me === null) {
      setError(t("limit.invalid"));
      return;
    }
    setBusy(true);
    setError("");
    try {
      const saved = await client.updateSettings({ ...s, globalLimit: gl, minExtSize: me });
      onSaved(saved);
      onClose();
    } catch (err) {
      setError((err as Error).message);
      setBusy(false);
    }
  };

  return (
    <Dialog
      title={t("settings.title")}
      onClose={onClose}
      wide
      testId="settings-dialog"
      footer={
        <>
          <button className="btn ghost" onClick={onClose}>
            {t("add.cancel")}
          </button>
          <button className="btn primary" onClick={save} disabled={busy} data-testid="settings-save">
            {t("settings.save")}
          </button>
        </>
      }
    >
      <form
        className="form settings"
        onSubmit={(e) => {
          e.preventDefault();
          void save();
        }}
      >
        <fieldset>
          <legend>{t("settings.general")}</legend>
          <label className="field">
            <span>{t("settings.downloadDir")}</span>
            <div className="row">
              <input value={s.downloadDir} onChange={(e) => set("downloadDir", e.target.value)} data-testid="set-dir" />
              <button type="button" className="btn" onClick={browse}>
                <FolderOpen size={14} /> {t("add.browse")}
              </button>
            </div>
          </label>
          <label className="check">
            <input type="checkbox" checked={s.useCategories} onChange={(e) => set("useCategories", e.target.checked)} />
            {t("settings.useCategories")}
          </label>
          <div className="field-row">
            <label className="field">
              <span>{t("settings.maxActive")}</span>
              <input
                type="number"
                min={1}
                max={20}
                value={s.maxActive}
                onChange={(e) => set("maxActive", Number(e.target.value))}
                data-testid="set-maxActive"
              />
            </label>
            <label className="field">
              <span>{t("settings.connections")}</span>
              <input
                type="number"
                min={1}
                max={32}
                value={s.connections}
                onChange={(e) => set("connections", Number(e.target.value))}
                data-testid="set-connections"
              />
            </label>
            <label className="field">
              <span>{t("settings.maxRetries")}</span>
              <input type="number" min={0} max={100} value={s.maxRetries} onChange={(e) => set("maxRetries", Number(e.target.value))} />
            </label>
          </div>
        </fieldset>

        <fieldset>
          <legend>{t("settings.connection")}</legend>
          <label className="field">
            <span>{t("settings.globalLimit")}</span>
            <input value={limit} onChange={(e) => setLimit(e.target.value)} placeholder={t("settings.globalLimitHint")} data-testid="set-limit" />
          </label>
          <label className="field">
            <span>{t("settings.proxy")}</span>
            <input value={s.proxy} onChange={(e) => set("proxy", e.target.value)} placeholder={t("settings.proxyHint")} spellCheck={false} />
            <small className="muted">{t("settings.proxyHint")}</small>
          </label>
          <label className="field">
            <span>{t("settings.userAgent")}</span>
            <input
              value={s.userAgent}
              onChange={(e) => set("userAgent", e.target.value)}
              placeholder={t("settings.userAgentHint")}
              spellCheck={false}
              data-testid="set-ua"
            />
          </label>
        </fieldset>

        <fieldset>
          <legend>{t("settings.integration")}</legend>
          <label className="check">
            <input
              type="checkbox"
              checked={s.clipboardWatch}
              onChange={(e) => set("clipboardWatch", e.target.checked)}
              data-testid="set-clipboard"
            />
            {t("settings.clipboardWatch")}
          </label>
          <label className="check">
            <input type="checkbox" checked={s.autoStartExt} onChange={(e) => set("autoStartExt", e.target.checked)} />
            {t("settings.autoStartExt")}
          </label>
          <label className="field">
            <span>{t("settings.minExtSize")}</span>
            <input value={minExt} onChange={(e) => setMinExt(e.target.value)} placeholder="1M" />
          </label>
        </fieldset>

        <fieldset>
          <legend>{t("settings.appearance")}</legend>
          <div className="field-row">
            <label className="field">
              <span>{t("settings.theme")}</span>
              <select value={s.theme} onChange={(e) => set("theme", e.target.value)} data-testid="set-theme">
                <option value="system">{t("settings.themeSystem")}</option>
                <option value="light">{t("settings.themeLight")}</option>
                <option value="dark">{t("settings.themeDark")}</option>
              </select>
            </label>
            <label className="field">
              <span>{t("settings.language")}</span>
              <select value={s.language} onChange={(e) => set("language", e.target.value)} data-testid="set-language">
                <option value="en">English</option>
                <option value="ar">العربية</option>
              </select>
            </label>
          </div>
        </fieldset>

        <div className="about">
          <img src={logo} alt="" width={36} height={36} />
          <div>
            <strong>{t("app.name")}</strong>
            <div className="muted">
              {t("app.tagline")} · {t("settings.version", { v: version || "?" })}
            </div>
          </div>
        </div>
        {error && (
          <p className="form-error" role="alert">
            {error}
          </p>
        )}
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
