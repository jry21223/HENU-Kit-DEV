import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  clearStaleScrollRestore,
  isAncestorPath,
  readScrollOffset,
  requestScrollRestore,
  scrollMemoryKey,
  scrollRestoreRequested,
  writeScrollOffset,
} from "./scroll-memory";

const stored = new Map<string, string>();

beforeEach(() => {
  stored.clear();
  (globalThis as { window?: unknown }).window = {
    sessionStorage: {
      getItem: (key: string) => stored.get(key) ?? null,
      setItem: (key: string, value: string) => {
        stored.set(key, value);
      },
    },
  };
});

afterEach(() => {
  vi.useRealTimers();
  clearStaleScrollRestore("/");
  delete (globalThis as { window?: unknown }).window;
});

describe("isAncestorPath", () => {
  it("recognises the level above, including the platform home", () => {
    expect(isAncestorPath("/practice", "/practice/quiz")).toBe(true);
    expect(isAncestorPath("/practice/favorites", "/practice/favorites/abc")).toBe(true);
    expect(isAncestorPath("/", "/practice")).toBe(true);
    expect(isAncestorPath("/", "/")).toBe(false);
  });

  it("keeps side-by-side tabs and deeper pages out of it", () => {
    expect(isAncestorPath("/practice/stats", "/practice/quiz")).toBe(false);
    expect(isAncestorPath("/practice/quiz/deep", "/practice/quiz")).toBe(false);
    expect(isAncestorPath("/practice", "/practice")).toBe(false);
    expect(isAncestorPath("/library", "/practice")).toBe(false);
  });
});

describe("scroll offset memory", () => {
  it("round-trips an offset per pathname", () => {
    writeScrollOffset("/food", 1234.6);
    expect(stored.get(scrollMemoryKey("/food"))).toBe("1235");
    expect(readScrollOffset("/food")).toBe(1235);
  });

  it("keeps each pathname's offset to itself", () => {
    writeScrollOffset("/food", 1200);
    writeScrollOffset("/library", 300);
    writeScrollOffset("/library/item/free-ds-graph-note", 40);

    expect(readScrollOffset("/food")).toBe(1200);
    expect(readScrollOffset("/library")).toBe(300);
    expect(readScrollOffset("/library/item/free-ds-graph-note")).toBe(40);
    expect(readScrollOffset("/food/post/post-0-0")).toBe(0);
  });

  it("treats missing, unparsable, and non-positive offsets as no memory", () => {
    expect(readScrollOffset("/food")).toBe(0);
    stored.set(scrollMemoryKey("/food"), "not-a-number");
    expect(readScrollOffset("/food")).toBe(0);
    stored.set(scrollMemoryKey("/food"), "0");
    expect(readScrollOffset("/food")).toBe(0);
    stored.set(scrollMemoryKey("/food"), "-500");
    expect(readScrollOffset("/food")).toBe(0);
    stored.set(scrollMemoryKey("/food"), "Infinity");
    expect(readScrollOffset("/food")).toBe(0);
  });
});

describe("restore requests", () => {
  it("only serves the pathname that was requested", () => {
    requestScrollRestore("/practice");
    expect(scrollRestoreRequested("/practice")).toBe(true);
    expect(scrollRestoreRequested("/library")).toBe(false);
    // Reading twice keeps the request: strict mode runs the effect twice.
    expect(scrollRestoreRequested("/practice")).toBe(true);
  });

  it("drops a request the reader never landed on", () => {
    requestScrollRestore("/practice");
    clearStaleScrollRestore("/library");
    expect(scrollRestoreRequested("/practice")).toBe(false);
  });

  it("keeps the request for the pathname that cleared as landed", () => {
    requestScrollRestore("/practice");
    clearStaleScrollRestore("/practice");
    expect(scrollRestoreRequested("/practice")).toBe(true);
  });

  it("drops a request that has gone stale before the reader lands", () => {
    requestScrollRestore("/practice");
    vi.useFakeTimers();
    vi.setSystemTime(Date.now() + 60_000);
    expect(scrollRestoreRequested("/practice")).toBe(false);
    clearStaleScrollRestore("/practice");
    expect(scrollRestoreRequested("/practice")).toBe(false);
  });
});
