import { describe, expect, it } from "vitest";
import { parentRoute } from "./parent-route";

describe("parentRoute", () => {
  it("sends a module's inner pages to that module's home", () => {
    expect(parentRoute("/practice/quiz")).toEqual({ href: "/practice", label: "题库" });
    expect(parentRoute("/practice/stats")).toEqual({ href: "/practice", label: "题库" });
    expect(parentRoute("/practice/leaderboard")).toEqual({ href: "/practice", label: "题库" });
    expect(parentRoute("/practice/lists/abc")).toEqual({ href: "/practice", label: "题库" });
    expect(parentRoute("/library/item/abc")).toEqual({ href: "/library", label: "书库" });
    expect(parentRoute("/library/shelf")).toEqual({ href: "/library", label: "书库" });
    expect(parentRoute("/food/post/abc")).toEqual({ href: "/food", label: "榜单" });
    expect(parentRoute("/campus/item/abc")).toEqual({ href: "/campus", label: "市集" });
    expect(parentRoute("/career/history")).toEqual({ href: "/career", label: "求职雷达" });
  });

  it("keeps the favorites folder under the favorites overview", () => {
    // 收藏夹概览本身是 /practice 的内页，再往上一级才是题库。
    expect(parentRoute("/practice/favorites")).toEqual({ href: "/practice", label: "题库" });
    expect(parentRoute("/practice/favorites/abc")).toEqual({
      href: "/practice/favorites",
      label: "收藏夹",
    });
  });

  it("sends a module home to the platform home", () => {
    for (const moduleHome of ["/practice", "/library", "/food", "/campus", "/career"]) {
      expect(parentRoute(moduleHome)).toEqual({ href: "/", label: "henukit" });
    }
  });

  it("falls back to the platform home for paths outside the five modules", () => {
    expect(parentRoute("/")).toEqual({ href: "/", label: "henukit" });
    expect(parentRoute("/account/login")).toEqual({ href: "/", label: "henukit" });
    expect(parentRoute("/account/recover")).toEqual({ href: "/", label: "henukit" });
    expect(parentRoute("/account")).toEqual({ href: "/", label: "henukit" });
    expect(parentRoute("/somewhere-else")).toEqual({ href: "/", label: "henukit" });
  });
});
