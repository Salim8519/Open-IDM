import { describe, expect, it } from "vitest";
import { formatBytes, formatETA, formatPercent, formatSpeed, parseSize, percent } from "./format";

describe("formatBytes", () => {
  it("formats binary units", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(1023)).toBe("1023 B");
    expect(formatBytes(1024)).toBe("1 KB");
    expect(formatBytes(1536)).toBe("1.5 KB");
    expect(formatBytes(10 * 1024 * 1024)).toBe("10 MB");
    expect(formatBytes(123.4 * 1024 ** 3)).toBe("123 GB");
    expect(formatBytes(1.234 * 1024 ** 4)).toBe("1.23 TB");
  });
  it("treats negative as unknown", () => {
    expect(formatBytes(-1)).toBe("?");
    expect(formatBytes(NaN)).toBe("?");
  });
});

describe("formatSpeed", () => {
  it("formats bytes per second", () => {
    expect(formatSpeed(2 * 1024 * 1024)).toBe("2 MB/s");
    expect(formatSpeed(0)).toBe("");
  });
});

describe("formatETA", () => {
  it("formats durations", () => {
    expect(formatETA(-1)).toBe("");
    expect(formatETA(42)).toBe("42s");
    expect(formatETA(185)).toBe("3m 05s");
    expect(formatETA(3725)).toBe("1h 02m");
    expect(formatETA(90000)).toBe("1d 1h");
  });
});

describe("percent", () => {
  it("computes clamped progress", () => {
    expect(percent(50, 200)).toBe(25);
    expect(percent(300, 200)).toBe(100);
    expect(percent(10, -1)).toBe(-1);
    expect(formatPercent(42.345)).toBe("42.3%");
    expect(formatPercent(50)).toBe("50%");
    expect(formatPercent(-1)).toBe("");
  });
});

describe("parseSize", () => {
  it("parses human sizes", () => {
    expect(parseSize("")).toBe(0);
    expect(parseSize("1000")).toBe(1000);
    expect(parseSize("512K")).toBe(512 * 1024);
    expect(parseSize("1.5 MB")).toBe(1.5 * 1024 * 1024);
    expect(parseSize("2m/s")).toBe(2 * 1024 * 1024);
    expect(parseSize("abc")).toBeNull();
  });
});
