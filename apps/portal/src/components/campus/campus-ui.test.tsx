import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { PortalHttpError, PortalNetworkError } from "@/lib/api/client";
import type { CampusItem } from "@/lib/api/types";
import {
  formatCampusUnavailableMessage,
  isCampusWritesUnavailable,
} from "./campus-actions";
import { CampusItemContent, normalizeCampusPhotos } from "./item-detail";

const BASE_ITEM: CampusItem = {
  id: "campus-item-1",
  type: "help",
  category: "express",
  title: "明伦北苑代取快递",
  desc: "一件中小包裹，送到北苑楼下即可。",
  price: 5,
  seller: "小河同学",
  credit: 98,
  dealsDone: 4,
  wants: 1,
  place: "明伦校区北苑楼下",
  deadline: "今晚 21:00 前",
  status: "open",
  time: "2026-11-23",
};

describe("campus write unavailable classification", () => {
  it("detects HTTP 503 CAMPUS_WRITES_UNAVAILABLE and excludes transient errors", () => {
    const unavailable = new PortalHttpError(
      "/api/v1/campus/contact-book",
      503,
      "CAMPUS_WRITES_UNAVAILABLE",
      "req_campus_503",
      "CAMPUS_WRITES_UNAVAILABLE",
      "互助写入服务暂未开启。"
    );
    const other503 = new PortalHttpError(
      "/api/v1/campus/contact-book",
      503,
      "CAMPUS_UPSTREAM_UNAVAILABLE",
      "req_campus_up",
      "CAMPUS_UPSTREAM_UNAVAILABLE"
    );
    const networkError = new PortalNetworkError("/api/v1/campus/contact-book");

    expect(isCampusWritesUnavailable(unavailable)).toBe(true);
    expect(formatCampusUnavailableMessage(unavailable)).toBe("互助写入服务暂未开启。");
    expect(isCampusWritesUnavailable(other503)).toBe(false);
    expect(isCampusWritesUnavailable(networkError)).toBe(false);
  });

  it("falls back to clear Chinese guidance when server message is absent", () => {
    const bare503 = new PortalHttpError(
      "/api/v1/campus/contact-book",
      503,
      "CAMPUS_WRITES_UNAVAILABLE",
      undefined,
      "CAMPUS_WRITES_UNAVAILABLE"
    );

    expect(isCampusWritesUnavailable(bare503)).toBe(true);
    expect(formatCampusUnavailableMessage(bare503)).toContain("暂未开启");
  });
});

describe("CampusItemContent multi-image rendering", () => {
  it("normalizes up to 6 non-empty image URLs", () => {
    const urls = normalizeCampusPhotos([
      "/api/v1/campus/items/1/images/0",
      "   ",
      "/api/v1/campus/items/1/images/1",
      "/api/v1/campus/items/1/images/2",
      "/api/v1/campus/items/1/images/3",
      "/api/v1/campus/items/1/images/4",
      "/api/v1/campus/items/1/images/5",
      "/api/v1/campus/items/1/images/6",
    ]);

    expect(urls).toHaveLength(6);
    expect(urls[0]).toBe("/api/v1/campus/items/1/images/0");
    expect(urls[5]).toBe("/api/v1/campus/items/1/images/5");
  });

  it("renders category code placeholder when item has no images", () => {
    const html = renderToStaticMarkup(
      <CampusItemContent initialItem={{ ...BASE_ITEM, images: [] }} initialMessages={[]} />
    );

    expect(html).toContain("bg-blueprint");
    expect(html).not.toContain('aria-label="单子图片"');
    expect(html).not.toContain("查看上一张图片");
  });

  it("renders single image stage without redundant switcher controls", () => {
    const html = renderToStaticMarkup(
      <CampusItemContent
        initialItem={{ ...BASE_ITEM, images: ["/api/v1/campus/items/1/images/0?v=1"] }}
        initialMessages={[]}
      />
    );

    expect(html).toContain("/api/v1/campus/items/1/images/0?v=1");
    expect(html).toContain("查看大图");
    expect(html).not.toContain('aria-label="单子图片"');
  });

  it("renders thumbnail strip, counter, and prev/next buttons for multiple images", () => {
    const html = renderToStaticMarkup(
      <CampusItemContent
        initialItem={{
          ...BASE_ITEM,
          images: [
            "/api/v1/campus/items/1/images/0?v=1",
            "/api/v1/campus/items/1/images/1?v=1",
            "/api/v1/campus/items/1/images/2?v=1",
          ],
        }}
        initialMessages={[]}
      />
    );

    expect(html).toContain("1 / 3");
    expect(html).toContain('aria-label="单子图片"');
    expect(html).toContain('aria-label="查看上一张图片"');
    expect(html).toContain('aria-label="查看下一张图片"');
    expect(html).toContain('aria-label="查看图片 1"');
    expect(html).toContain('aria-label="查看图片 3"');
    expect(html).toContain("focus-visible:outline-focus-ring");
  });
});
