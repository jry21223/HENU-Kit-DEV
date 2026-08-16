import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

import robots from "./robots";
import sitemap from "./sitemap";
import nextConfig from "../../next.config";
import {
  campusMetadata,
  careerMetadata,
  foodMetadata,
  homeMetadata,
  libraryMetadata,
  practiceMetadata,
  siteMetadata,
  websiteStructuredData,
} from "../lib/seo";

describe("Portal search discovery routes", () => {
  it("publishes only stable public entry points in the sitemap", () => {
    expect(sitemap()).toEqual([
      { url: "https://henukit.cn", changeFrequency: "weekly", priority: 1 },
      { url: "https://henukit.cn/library", changeFrequency: "daily", priority: 0.9 },
      { url: "https://henukit.cn/practice", changeFrequency: "weekly", priority: 0.9 },
      { url: "https://henukit.cn/food", changeFrequency: "daily", priority: 0.8 },
      { url: "https://henukit.cn/campus", changeFrequency: "daily", priority: 0.8 },
      { url: "https://henukit.cn/career", changeFrequency: "weekly", priority: 0.7 },
    ]);
  });

  it("allows search crawlers while excluding private and write-oriented routes", () => {
    expect(robots()).toEqual({
      rules: {
        userAgent: "*",
        allow: "/",
        disallow: "/api/",
      },
      host: "https://henukit.cn",
      sitemap: "https://henukit.cn/sitemap.xml",
    });
  });

  it("publishes canonical, social, and search metadata for the public home page", () => {
    expect(siteMetadata).toMatchObject({
      metadataBase: new URL("https://henukit.cn"),
      applicationName: "HENU Kit",
      title: {
        default: "HENU Kit — 河南大学校园工具",
        template: "%s | HENU Kit",
      },
      openGraph: {
        type: "website",
        locale: "zh_CN",
        siteName: "HENU Kit",
      },
      twitter: { card: "summary" },
    });
    expect(homeMetadata.alternates).toEqual({ canonical: "/" });
    expect(homeMetadata.openGraph).toMatchObject({ url: "/" });
  });

  it("returns noindex headers on account, write, reader, and personalized routes", async () => {
    const headers = await nextConfig.headers?.();
    const noIndexRoutes = headers?.filter((entry) =>
      entry.headers.some(
        ({ key, value }) =>
          key === "X-Robots-Tag" && value === "noindex, nofollow"
      )
    );

    expect(noIndexRoutes?.map(({ source }) => source)).toEqual([
      "/account/:path*",
      "/campus/deals",
      "/campus/publish",
      "/food/publish",
      "/library/read/:path*",
      "/library/shelf",
      "/practice/favorites/:path*",
      "/practice/quiz",
      "/practice/stats",
    ]);
  });

  it("serves an honest machine-readable project and citation boundary", () => {
    const llms = readFileSync(
      new URL("../../public/llms.txt", import.meta.url),
      "utf8"
    );

    expect(llms).toContain("学生自主运营的非官方项目");
    expect(llms).toContain("河南大学及各学院官方来源优先");
    expect(llms).toContain("current, superseded, historical, unverified");
    expect(llms).toContain("https://henukit.cn/sitemap.xml");
  });

  it("publishes page-level canonical and description metadata for every stable top-level route", () => {
    expect(libraryMetadata).toMatchObject({
      alternates: { canonical: "/library" },
      description: expect.stringContaining("公开免费"),
    });
    expect(practiceMetadata).toMatchObject({
      alternates: { canonical: "/practice" },
      description: expect.stringContaining("刷题"),
    });
    expect(foodMetadata).toMatchObject({
      alternates: { canonical: "/food" },
      description: expect.stringContaining("美食"),
    });
    expect(campusMetadata).toMatchObject({
      alternates: { canonical: "/campus" },
      description: expect.stringContaining("互助"),
    });
    expect(careerMetadata).toMatchObject({
      alternates: { canonical: "/career" },
      description: expect.stringContaining("求职"),
    });
  });

  it("keeps page-level share cards honest and page-specific instead of inheriting the home card", () => {
    for (const meta of [
      libraryMetadata,
      practiceMetadata,
      foodMetadata,
      campusMetadata,
      careerMetadata,
    ]) {
      expect(meta.twitter).toMatchObject({ card: "summary" });
      expect(meta.openGraph?.title).toBe(meta.twitter?.title);
      expect(meta.openGraph?.description).toBe(meta.twitter?.description);
    }
    expect(careerMetadata.twitter?.title).toContain("求职雷达");
    expect(careerMetadata.openGraph?.url).toBe("/career");
  });

  it("identifies the site and its non-official publisher without inventing an official affiliation", () => {
    expect(websiteStructuredData()).toEqual({
      "@context": "https://schema.org",
      "@graph": [
        {
          "@type": "Organization",
          "@id": "https://henukit.cn/#community",
          name: "HENU Kit 社区维护者",
          url: "https://henukit.cn",
          description: "学生自主运营的非官方社区维护团队，不代表河南大学或任何学院。",
        },
        {
          "@type": "WebSite",
          "@id": "https://henukit.cn/#website",
          url: "https://henukit.cn",
          name: "HENU Kit",
          alternateName: "河南大学校园工具",
          description:
            "HENU Kit 是学生自主运营的非官方河南大学校园工具，提供资料库、智能刷题、美食榜、校园互助和求职雷达入口；信息以河南大学及各学院官方来源为准。",
          inLanguage: "zh-CN",
          isAccessibleForFree: true,
          publisher: { "@id": "https://henukit.cn/#community" },
        },
      ],
    });
  });
});
