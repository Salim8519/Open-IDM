import { describe, expect, it } from "vitest";
import { en, StringKey } from "./en";
import { asLang, isRTL, LANGUAGES, translate } from "./index";

const placeholders = (s: string) => (s.match(/\{[a-z]+\}/gi) ?? []).sort();

describe("translations", () => {
  for (const lang of LANGUAGES) {
    it(`${lang.code}: every key translated, placeholders kept`, () => {
      const table = lang.table as Record<string, string>;
      for (const key of Object.keys(en) as StringKey[]) {
        const v = table[key];
        expect(v, `${lang.code} ${key}`).toBeTruthy();
        expect(placeholders(v), `${lang.code} ${key}`).toEqual(placeholders(en[key]));
      }
      expect(Object.keys(table).length).toBe(Object.keys(en).length);
    });
  }

  it("codes are unique and RTL is set for Arabic-script languages", () => {
    const codes = LANGUAGES.map((l) => l.code);
    expect(new Set(codes).size).toBe(codes.length);
    expect(LANGUAGES.filter((l) => l.rtl).map((l) => l.code)).toEqual(["ar", "fa", "ur"]);
    expect(isRTL("fa")).toBe(true);
    expect(isRTL("ja")).toBe(false);
  });

  it("unknown languages fall back to English", () => {
    expect(asLang("xx")).toBe("en");
    expect(asLang(undefined)).toBe("en");
    expect(asLang("ja")).toBe("ja");
    expect(translate("en", "settings.version", { v: "1.2" })).toBe("Version 1.2");
  });
});
