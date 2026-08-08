export const FOOD_TIERS = [
  {
    key: "hang",
    label: "夯",
    index: "T-01",
    en: "MUST EAT",
    blurb: "必须吃",
  },
  {
    key: "top",
    label: "顶级",
    index: "T-02",
    en: "TOP TIER",
    blurb: "稳稳推荐",
  },
  {
    key: "elite",
    label: "人上人",
    index: "T-03",
    en: "PREMIUM",
    blurb: "预算足再冲",
  },
  {
    key: "npc",
    label: "NPC",
    index: "T-04",
    en: "EVERYDAY",
    blurb: "日常不出错",
  },
  {
    key: "bad",
    label: "拉完了",
    index: "T-05",
    en: "SKIP IT",
    blurb: "先别急着去",
  },
] as const;

export type FoodTierKey = (typeof FOOD_TIERS)[number]["key"];
export type FoodTier = (typeof FOOD_TIERS)[number];

/** 夯档 key：首页榜单行强调色与 /food 页标题高亮共用。 */
export const HANG_TIER_KEY: FoodTierKey = "hang";

export interface RankableFoodPost {
  id: string;
  campus: string;
  tags: string[];
  likes: number;
  hidden: boolean;
}

const EXACT_TIER_TAGS: Record<string, FoodTierKey> = {
  夯: "hang",
  顶级: "top",
  人上人: "elite",
  NPC: "npc",
  拉完了: "bad",
};

/**
 * Compatibility read for the current FoodPost wire format. Portal accepts
 * only an exact five-tier tag already supplied by the owner; it never derives
 * a tier from scores, ordinary descriptors, or a default.
 */
export function resolveFoodTier(
  post: Pick<RankableFoodPost, "tags">
): FoodTierKey | null {
  for (const tag of post.tags) {
    const tier = EXACT_TIER_TAGS[tag.trim()];
    if (tier) return tier;
  }
  return null;
}

export function groupFoodPostsByTier<T extends RankableFoodPost>(
  posts: readonly T[],
  campus: string | "all" = "all"
): Array<{ tier: FoodTier; posts: T[] }> {
  const postsByTier = new Map<FoodTierKey, T[]>(
    FOOD_TIERS.map(({ key }) => [key, []])
  );

  for (const post of posts) {
    if (post.hidden || (campus !== "all" && post.campus !== campus)) {
      continue;
    }
    const tier = resolveFoodTier(post);
    if (tier) postsByTier.get(tier)?.push(post);
  }

  return FOOD_TIERS.map((tier) => ({
    tier,
    posts: (postsByTier.get(tier.key) ?? []).sort(
      (left, right) => right.likes - left.likes || left.id.localeCompare(right.id)
    ),
  }));
}
