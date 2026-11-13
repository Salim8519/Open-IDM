import { createContext, ReactNode, useContext, useMemo } from "react";
import { ar } from "./ar";
import { de } from "./de";
import { en, StringKey, Strings } from "./en";
import { es } from "./es";
import { fa } from "./fa";
import { fr } from "./fr";
import { hi } from "./hi";
import { id } from "./id";
import { it } from "./it";
import { ja } from "./ja";
import { ko } from "./ko";
import { pt } from "./pt";
import { ru } from "./ru";
import { tr } from "./tr";
import { ur } from "./ur";
import { zh } from "./zh";

/** UI languages, in the order shown in Settings. `name` is the language's own name. */
export const LANGUAGES = [
  { code: "en", name: "English", rtl: false, table: en },
  { code: "ar", name: "العربية", rtl: true, table: ar },
  { code: "de", name: "Deutsch", rtl: false, table: de },
  { code: "es", name: "Español", rtl: false, table: es },
  { code: "fa", name: "فارسی", rtl: true, table: fa },
  { code: "fr", name: "Français", rtl: false, table: fr },
  { code: "hi", name: "हिन्दी", rtl: false, table: hi },
  { code: "id", name: "Bahasa Indonesia", rtl: false, table: id },
  { code: "it", name: "Italiano", rtl: false, table: it },
  { code: "ja", name: "日本語", rtl: false, table: ja },
  { code: "ko", name: "한국어", rtl: false, table: ko },
  { code: "pt", name: "Português (Brasil)", rtl: false, table: pt },
  { code: "ru", name: "Русский", rtl: false, table: ru },
  { code: "tr", name: "Türkçe", rtl: false, table: tr },
  { code: "ur", name: "اردو", rtl: true, table: ur },
  { code: "zh", name: "简体中文", rtl: false, table: zh },
] as const;

export type Lang = (typeof LANGUAGES)[number]["code"];

const tables = Object.fromEntries(LANGUAGES.map((l) => [l.code, l.table as Strings])) as Record<Lang, Strings>;

/** Maps a stored setting to a supported language (unknown values fall back to English). */
export function asLang(code: string | undefined): Lang {
  return LANGUAGES.some((l) => l.code === code) ? (code as Lang) : "en";
}

export type TFunc = (key: StringKey, vars?: Record<string, string | number>) => string;

export function translate(lang: Lang, key: StringKey, vars?: Record<string, string | number>): string {
  let s: string = tables[lang]?.[key] ?? en[key] ?? key;
  if (vars) for (const [k, v] of Object.entries(vars)) s = s.split(`{${k}}`).join(String(v));
  return s;
}

export function isRTL(lang: Lang): boolean {
  return LANGUAGES.some((l) => l.code === lang && l.rtl);
}

const Ctx = createContext<{ lang: Lang; t: TFunc }>({ lang: "en", t: (k, v) => translate("en", k, v) });

export function I18nProvider({ lang, children }: { lang: Lang; children: ReactNode }) {
  const value = useMemo(() => ({ lang, t: ((k, v) => translate(lang, k, v)) as TFunc }), [lang]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useI18n() {
  return useContext(Ctx);
}

export type { StringKey };
