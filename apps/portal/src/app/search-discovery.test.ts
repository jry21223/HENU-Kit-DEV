import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

import robots from "./robots";
import sitemap from "./sitemap";
import nextConfig from "../../next.config";
import {
  campusMetadata,
  careerMetadata,
  documentTitle,
  foodMetadata,
  homeMetadata,
  libraryMetadata,
  moduleLayoutTitle,
  pageTitle,
  practiceMetadata,
  privacyMetadata,
  siteMetadata,
  termsMetadata,
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
      { url: "https://henukit.cn/privacy", changeFrequency: "yearly", priority: 0.2 },
      { url: "https://henukit.cn/terms", changeFrequency: "yearly", priority: 0.2 },
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

  it("describes practice only as the bank search and modes that exist (#549, ADR-0036)", () => {
    // 按学校 / 专业浏览没有建成，也没有题单；搜索的说法与首页刷题区块、题库页搜索框（搜索科目）一致。
    const llmsPractice = readFileSync(new URL("../../public/llms.txt", import.meta.url), "utf8")
      .split("\n")
      .find((line) => line.includes("https://henukit.cn/practice"));
    for (const text of [String(practiceMetadata.description), llmsPractice ?? ""]) {
      expect(text).toContain("按科目搜索题库");
      expect(text).not.toMatch(/学校、专业|按学校|题单/);
    }
  });

  it("describes the campus market as browse-only, like the page itself (#482)", () => {
    const llmsCampus = readFileSync(new URL("../../public/llms.txt", import.meta.url), "utf8")
      .split("\n")
      .find((line) => line.includes("https://henukit.cn/campus"));
    for (const text of [String(campusMetadata.description), llmsCampus ?? ""]) {
      expect(text).toContain("发布、接单和结算暂未开放");
    }
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

  it("formats every page title one way and leaves the brand to the root template", () => {
    expect(pageTitle("会员权益", "account")).toBe("会员权益 — 账户中心");
    expect(pageTitle("扫描历史", "career")).toBe("扫描历史 — 求职雷达");
    expect(pageTitle("登录")).toBe("登录");
    expect(documentTitle("鼓楼夜市", "food")).toBe("鼓楼夜市 — 美食榜 | HENU Kit");
    expect(documentTitle("登录")).toBe("登录 | HENU Kit");
    // 内容名来自用户投稿，里面的 `$` 要原样进标题，不能被当成替换模式展开。
    for (const name of ["收 $$ 耳机", "A $& B", "X $' Y", "P $` Q"]) {
      expect(documentTitle(name, "campus")).toBe(`${name} — 互助平台 | HENU Kit`);
    }
    // 模块布局一写标题，Next 就不再把根模板传给更深的页面；模块布局要把模板接着传下去。
    expect(moduleLayoutTitle("account")).toEqual({
      default: "账户中心",
      template: "%s | HENU Kit",
    });

    expect(
      [libraryMetadata, practiceMetadata, foodMetadata, campusMetadata, careerMetadata].map(
        (meta) => meta.title
      )
    ).toEqual(["资料库", "智能刷题", "美食榜", "互助平台", "求职雷达"]);
    for (const meta of [
      libraryMetadata,
      practiceMetadata,
      foodMetadata,
      campusMetadata,
      careerMetadata,
      privacyMetadata,
      termsMetadata,
    ]) {
      expect(meta.title).not.toMatch(/henukit|HENU Kit/i);
      expect(meta.openGraph?.title).toBe(documentTitle(String(meta.title)));
    }
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
