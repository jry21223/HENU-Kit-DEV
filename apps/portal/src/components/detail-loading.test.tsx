import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import CampusItemDetail from "./campus/item-detail";
import { LibraryLoading } from "./library/material-states";

// 返回上一级的落点由路径决定；这里只看加载文案，路径随便给一个。
vi.mock("next/navigation", () => ({ usePathname: () => "/" }));

/**
 * 详情页加载中也只用中文，不带英文「LOADING」，中文不加宽字距、不用过淡的墨色（#545）。
 * 只取加载文案那一行来查，返回链接的样式不在这里管。
 */
function loadingLabel(html: string) {
  const label = html.match(/<p[^>]*>加载中…<\/p>/)?.[0];
  expect(label).toBeDefined();
  return label ?? "";
}

describe("detail pages while loading", () => {
  it("library detail and reader say 加载中… in Chinese only", () => {
    const html = renderToStaticMarkup(<LibraryLoading />);

    expect(html).not.toContain("LOADING");
    const label = loadingLabel(html);
    expect(label).not.toMatch(/tracking-(?!normal\b)/);
    expect(label).not.toMatch(/text-ink\/[1-5]\d\b/);
  });

  it("campus item detail says 加载中… in Chinese only", () => {
    const html = renderToStaticMarkup(<CampusItemDetail id="campus-express" />);

    expect(html).not.toContain("LOADING");
    const label = loadingLabel(html);
    expect(label).not.toMatch(/tracking-(?!normal\b)/);
    expect(label).not.toMatch(/text-ink\/[1-5]\d\b/);
  });
});
