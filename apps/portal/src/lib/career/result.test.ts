import { describe, expect, it } from "vitest";

import { visibleCareerMatches } from "./result";
import type { CareerSearchResult } from "../api/types";

describe("visibleCareerMatches", () => {
  it("keeps scanned jobs inspectable in descending score order", () => {
    const result: CareerSearchResult = {
      source_count: 1,
      job_count: 3,
      matched_count: 2,
      summary: "已扫描 1 个来源，发现 3 个岗位，2 个相关岗位",
      sources: [{ key: "getwork.meituan", status: "success", found: 3, fetched: 3, rejected: 0 }],
      jobs: [
        { source_key: "official.meituan", company: "美团", title: "前端实习生", location: "北京", url: "https://example.test/2", match_score: 70, match_reasons: ["匹配技术栈 React"] },
        { source_key: "official.meituan", company: "美团", title: "运营实习生", location: "北京", url: "https://example.test/3", match_score: 15, match_reasons: [] },
        { source_key: "official.meituan", company: "美团", title: "Go 后端实习生", location: "北京", url: "https://example.test/1", match_score: 92, match_reasons: ["匹配目标岗位 后端开发"] },
      ],
    };

    expect(visibleCareerMatches(result).map((job) => job.title)).toEqual([
      "Go 后端实习生",
      "前端实习生",
      "运营实习生",
    ]);
  });
});
