import { createContext, ReactNode, useContext, useMemo } from "react";
import { ar } from "./ar";
import { en, StringKey, Strings } from "./en";

export type Lang = "en" | "ar";

const tables: Record<Lang, Strings> = { en, ar };

export type TFunc = (key: StringKey, vars?: Record<string, string | number>) => string;

export function translate(lang: Lang, key: StringKey, vars?: Record<string, string | number>): string {
  let s: string = tables[lang]?.[key] ?? en[key] ?? key;
  if (vars) for (const [k, v] of Object.entries(vars)) s = s.split(`{${k}}`).join(String(v));
  return s;
}

export function isRTL(lang: Lang): boolean {
  return lang === "ar";
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
