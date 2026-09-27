import { describe, expect, it } from "vitest";
import { sameOriginPath } from "./same-origin-path";

const ORIGIN = "https://henukit.cn";

describe("sameOriginPath", () => {
  it("keeps a path on this site with its query and fragment", () => {
    expect(sameOriginPath("/account/security", ORIGIN)).toBe("/account/security");
    expect(sameOriginPath("/practice/quiz?bank_id=1&session_id=2", ORIGIN)).toBe(
      "/practice/quiz?bank_id=1&session_id=2"
    );
    expect(sameOriginPath("/food#tier-hang", ORIGIN)).toBe("/food#tier-hang");
  });

  it("rejects a missing target or one that is not a path", () => {
    expect(sameOriginPath(null, ORIGIN)).toBeNull();
    expect(sameOriginPath(undefined, ORIGIN)).toBeNull();
    expect(sameOriginPath("", ORIGIN)).toBeNull();
    expect(sameOriginPath("account", ORIGIN)).toBeNull();
    expect(sameOriginPath("https://evil.example/", ORIGIN)).toBeNull();
    expect(sameOriginPath("javascript:alert(1)", ORIGIN)).toBeNull();
  });

  it("rejects paths that the browser resolves to another site", () => {
    // 都以 / 开头：协议相对地址、反斜杠当斜杠、URL 解析时删掉的制表符和换行。
    for (const target of [
      "//evil.example/phish",
      "/\\evil.example/phish",
      "/\t/evil.example/phish",
      "/\n/evil.example/phish",
    ]) {
      expect(sameOriginPath(target, ORIGIN), JSON.stringify(target)).toBeNull();
    }
  });

  it("rejects a path that only becomes protocol-relative once normalised", () => {
    // 解析后仍在本站（https://henukit.cn//evil.example/phish），但把这段路径再交给路由，
    // 它又会被当成 //evil.example。
    expect(sameOriginPath("/..//evil.example/phish", ORIGIN)).toBeNull();
    expect(sameOriginPath("/.//evil.example/phish", ORIGIN)).toBeNull();
    expect(sameOriginPath("/%2e%2e//evil.example/phish", ORIGIN)).toBeNull();
  });

  it("rejects a target the URL parser cannot read", () => {
    expect(sameOriginPath("//[", ORIGIN)).toBeNull();
  });
});
