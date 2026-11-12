import { describe, expect, it } from "vitest";
import { expandPattern, looksDownloadable, nameFromURL, parseBatch } from "./batch";

describe("expandPattern", () => {
  it("keeps strings without patterns", () => {
    expect(expandPattern("http://x/a.zip")).toEqual(["http://x/a.zip"]);
  });
  it("expands zero-padded numeric ranges", () => {
    expect(expandPattern("http://x/file[001-003].zip")).toEqual([
      "http://x/file001.zip",
      "http://x/file002.zip",
      "http://x/file003.zip",
    ]);
    const all = expandPattern("http://x/file[001-100].zip");
    expect(all).toHaveLength(100);
    expect(all[99]).toBe("http://x/file100.zip");
  });
  it("expands unpadded and descending ranges", () => {
    expect(expandPattern("a[8-11]")).toEqual(["a8", "a9", "a10", "a11"]);
    expect(expandPattern("a[3-1]")).toEqual(["a3", "a2", "a1"]);
  });
  it("expands letters and multiple ranges", () => {
    expect(expandPattern("[a-b][1-2]")).toEqual(["a1", "a2", "b1", "b2"]);
  });
  it("respects the limit", () => {
    expect(expandPattern("x[1-100000]", 50)).toHaveLength(50);
  });
});

describe("parseBatch", () => {
  it("parses lines, expands, filters and dedupes", () => {
    const text = "http://x/a.zip\n\n  not a url \nhttp://x/a.zip\nhttps://y/p[1-2].bin\nftp://z/f";
    expect(parseBatch(text)).toEqual(["http://x/a.zip", "https://y/p1.bin", "https://y/p2.bin"]);
  });
});

describe("looksDownloadable", () => {
  it("detects file links", () => {
    expect(looksDownloadable("https://example.com/setup.exe")).toBe(true);
    expect(looksDownloadable("https://example.com/a/movie.MKV?x=1")).toBe(true);
    expect(looksDownloadable("https://example.com/page")).toBe(false);
    expect(looksDownloadable("https://example.com/index.html")).toBe(false);
    expect(looksDownloadable("hello world")).toBe(false);
  });
  it("extracts names", () => {
    expect(nameFromURL("https://e.com/a/b%20c.zip?x")).toBe("b c.zip");
    expect(nameFromURL("https://e.com/")).toBe("e.com");
  });
});
