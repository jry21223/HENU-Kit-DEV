import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  claimHistoryReturn,
  clearStaleScrollRequest,
  isAncestorPath,
  readScrollOffset,
  rememberHistoryReturn,
  requestScrollRestore,
  scrollMemoryKey,
  requestScrollTop,
  scrollLandingFor,
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
  clearStaleScrollRequest("/");
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

describe("history return", () => {
  it("restores only the path the history traversal landed on", () => {
    expect(claimHistoryReturn("/food")).toBe(false);

    rememberHistoryReturn("/food");
    // 别的路径挂载不能把这次回退用掉：回退落到首页之后，读者再点进列表页
    // 仍然必须从顶部开始。
    expect(claimHistoryReturn("/library")).toBe(false);
    expect(claimHistoryReturn("/food")).toBe(true);
  });

  it("uses a traversal once", () => {
    rememberHistoryReturn("/library");
    expect(claimHistoryReturn("/library")).toBe(true);
    expect(claimHistoryReturn("/library")).toBe(false);
  });
});

describe("landing requests", () => {
  it("only serves the pathname that was requested", () => {
    requestScrollRestore("/practice");
    expect(scrollLandingFor("/practice")).toBe("restore");
    expect(scrollLandingFor("/library")).toBeNull();
    // Reading twice keeps the request: strict mode runs the effect twice.
    expect(scrollLandingFor("/practice")).toBe("restore");
  });

  it("asks for the top on a sideways or deeper navigation", () => {
    requestScrollTop("/practice/stats");
    expect(scrollLandingFor("/practice/stats")).toBe("top");
    expect(scrollLandingFor("/practice/favorites")).toBeNull();
  });

  it("drops a request the reader never landed on", () => {
    requestScrollRestore("/practice");
    clearStaleScrollRequest("/library");
    expect(scrollLandingFor("/practice")).toBeNull();
  });

  it("keeps the request for the pathname that cleared as landed", () => {
    requestScrollRestore("/practice");
    clearStaleScrollRequest("/practice");
    expect(scrollLandingFor("/practice")).toBe("restore");
  });

  it("drops a request that has gone stale before the reader lands", () => {
    requestScrollRestore("/practice");
    vi.useFakeTimers();
    vi.setSystemTime(Date.now() + 60_000);
    expect(scrollLandingFor("/practice")).toBeNull();
    clearStaleScrollRequest("/practice");
    expect(scrollLandingFor("/practice")).toBeNull();
  });
});
