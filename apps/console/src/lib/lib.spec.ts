import { afterEach, describe, expect, it, vi } from "vitest";

import { accountName, accountStatusLabel } from "./account-labels";
import { localDateTime } from "./format";
import { consoleBasePath, consolePath, isConsolePath } from "./base-path";

describe("account labels", () => {
  it("never renders a blank confirmation target", () => {
    // Legacy rows carry an empty label; an operator confirming a destructive
    // action must still see who it applies to.
    expect(accountName("")).toBe("（未设置姓名）");
    expect(accountName("   ")).toBe("（未设置姓名）");
    expect(accountName(undefined)).toBe("（未设置姓名）");
  });

  it("keeps a real name untouched", () => {
    expect(accountName("张三")).toBe("张三");
  });

  it("maps every status the gateway contract allows", () => {
    expect(accountStatusLabel("active")).toBe("正常");
    expect(accountStatusLabel("suspended")).toBe("已停用");
    expect(accountStatusLabel("deleted")).toBe("已删除");
  });

  it("falls back to the raw value for an unmodelled status", () => {
    expect(accountStatusLabel("archived")).toBe("archived");
  });
});

describe("localDateTime", () => {
  it("renders a valid UTC timestamp rather than the raw string", () => {
    const rendered = localDateTime("2026-08-19T04:05:00Z");
    expect(rendered).not.toBe("2026-08-19T04:05:00Z");
    expect(rendered).toMatch(/2026/);
  });

  it("falls back to the raw string instead of rendering a broken date", () => {
    expect(localDateTime("not a timestamp")).toBe("not a timestamp");
    expect(localDateTime("")).toBe("");
  });
});

describe("console base path", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("strips the trailing slash Vite leaves on BASE_URL", () => {
    // The default base is "/", which must resolve to "" so consolePath does
    // not produce a doubled slash.
    expect(consoleBasePath()).toBe("");
    expect(consolePath("/accounts")).toBe("/accounts");
  });

  it("normalizes a path that arrives without a leading slash", () => {
    expect(consolePath("accounts")).toBe("/accounts");
  });

  it("matches the current location ignoring trailing slashes", () => {
    vi.stubGlobal("window", { location: { pathname: "/accounts/" } });
    expect(isConsolePath("/accounts")).toBe(true);
    expect(isConsolePath("/points")).toBe(false);
  });

  it("treats the root location as a match for the root route", () => {
    vi.stubGlobal("window", { location: { pathname: "/" } });
    expect(isConsolePath("/")).toBe(true);
  });
});
