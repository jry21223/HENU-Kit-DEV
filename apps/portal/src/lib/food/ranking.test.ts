import { describe, expect, it } from "vitest";

import {
  FOOD_TIERS,
  groupFoodPostsByTier,
  resolveFoodTier,
} from "./ranking";

describe("food ranking", () => {
  it("uses the five public tiers from 夯 through 拉完了", () => {
    expect(FOOD_TIERS.map(({ label }) => label)).toEqual([
      "夯",
      "顶级",
      "人上人",
      "NPC",
      "拉完了",
    ]);
  });

  it("accepts only the exact owner-supplied five-tier vocabulary", () => {
    expect(resolveFoodTier({ tags: ["面食", "夯"] })).toBe("hang");
    expect(resolveFoodTier({ tags: ["夜市", "顶级"] })).toBe("top");
    expect(resolveFoodTier({ tags: ["老字号", "人上人"] })).toBe("elite");
    expect(resolveFoodTier({ tags: ["食堂", "NPC"] })).toBe("npc");
    expect(resolveFoodTier({ tags: ["避雷", "拉完了"] })).toBe("bad");
    expect(resolveFoodTier({ tags: ["中"] })).toBeNull();
    expect(resolveFoodTier({ tags: ["拉"] })).toBeNull();
    expect(resolveFoodTier({ tags: ["尚未定档"] })).toBeNull();
  });

  it("filters hidden and off-campus posts while preserving the owner response order", () => {
    const groups = groupFoodPostsByTier(
      [
        {
          id: "hang-low",
          campus: "minglun",
          tags: ["夯"],
          likes: 5,
          hidden: false,
        },
        {
          id: "hang-high",
          campus: "minglun",
          tags: ["夯"],
          likes: 20,
          hidden: false,
        },
        {
          id: "hidden",
          campus: "minglun",
          tags: ["顶级"],
          likes: 999,
          hidden: true,
        },
        {
          id: "other-campus",
          campus: "jinming",
          tags: ["人上人"],
          likes: 99,
          hidden: false,
        },
        {
          id: "neutral",
          campus: "minglun",
          tags: ["待核验"],
          likes: 12,
          hidden: false,
        },
      ],
      "minglun"
    );

    expect(groups.map(({ tier }) => tier.label)).toEqual([
      "夯",
      "顶级",
      "人上人",
      "NPC",
      "拉完了",
    ]);
    expect(groups.find(({ tier }) => tier.key === "hang")?.posts.map(({ id }) => id)).toEqual([
      "hang-low",
      "hang-high",
    ]);
    expect(groups.find(({ tier }) => tier.key === "npc")?.posts).toEqual([]);
    expect(groups.flatMap(({ posts }) => posts).map(({ id }) => id)).not.toContain(
      "neutral"
    );
    expect(groups.flatMap(({ posts }) => posts).map(({ id }) => id)).not.toContain("hidden");
    expect(groups.flatMap(({ posts }) => posts).map(({ id }) => id)).not.toContain(
      "other-campus"
    );
  });
});
