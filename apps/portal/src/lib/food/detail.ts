import type { FoodPost, PostBlock } from "@/lib/api/types";
import {
  FOOD_TIERS,
  resolveFoodTier,
  type FoodTier,
} from "./ranking";

const PRICE_PATTERN =
  /(?:人均\s*)?(?:¥|￥)\s*\d+(?:\s*[–—~-]\s*\d+)?(?:\s*元)?|学生餐价/;
const PRICE_REFERENCE_PATTERN = /^价格参考[：:]\s*(.+)$/;
const DISH_HEADING_PATTERN = /^(?:点什么|推荐菜品|吃什么|必点|怎么点)[？?：:]?$/;
const HOURS_PATTERN = /^营业时间参考[：:]\s*(.+)$/;

export interface FoodDishView {
  name: string;
  note?: string;
}

export interface FoodVenueDetail {
  tier: FoodTier | null;
  location: string;
  priceReference: string | null;
  hoursReference: string | null;
  source: {
    author: string;
    publishedAt: string;
  };
  reasons: string[];
  dishes: FoodDishView[];
  gallery: string[];
}

function blockTexts(blocks: PostBlock[]): string[] {
  return blocks.flatMap((block) => [
    ...(block.text ? [block.text] : []),
    ...(block.items ?? []),
  ]);
}

function findPrice(post: FoodPost): string | null {
  for (const text of [post.excerpt, ...blockTexts(post.blocks)]) {
    const explicit = text.trim().match(PRICE_REFERENCE_PATTERN);
    if (explicit?.[1]) return explicit[1].trim();
  }
  // Compatibility for older seeded/editorial posts that only contain a price
  // phrase. Newly created posts use the explicit 价格参考 label above.
  for (const text of [post.excerpt, ...blockTexts(post.blocks)]) {
    const match = text.match(PRICE_PATTERN);
    if (match) return match[0].replace(/\s+/g, " ").trim();
  }
  return null;
}

function findHours(post: FoodPost): string | null {
  for (const block of post.blocks) {
    const match = block.text?.trim().match(HOURS_PATTERN);
    if (match?.[1]) return match[1].trim();
  }
  return null;
}

function toDish(item: string): FoodDishView {
  const [name, ...note] = item.split(/[：:]/);
  return {
    name: name.trim(),
    ...(note.length ? { note: note.join("：").trim() } : {}),
  };
}

/**
 * The wire contract has generic editorial lists. Treat one as dishes only
 * when an explicit dish heading gives it that meaning. Gallery images may sit
 * between that heading and its list; any other block ends the association.
 */
function findDishes(blocks: PostBlock[]): FoodDishView[] {
  for (let headingIndex = 0; headingIndex < blocks.length; headingIndex += 1) {
    const heading = blocks[headingIndex];
    if (
      heading.type !== "h2" ||
      !DISH_HEADING_PATTERN.test(heading.text?.trim() ?? "")
    ) {
      continue;
    }

    for (
      let candidateIndex = headingIndex + 1;
      candidateIndex < blocks.length;
      candidateIndex += 1
    ) {
      const candidate = blocks[candidateIndex];
      if (candidate.type === "img") continue;
      if (candidate.type === "list" && candidate.items?.length) {
        return candidate.items.map(toDish).filter((dish) => dish.name);
      }
      break;
    }
  }
  return [];
}

export function buildFoodVenueDetail(post: FoodPost): FoodVenueDetail {
  const tierKey = resolveFoodTier(post);
  const gallery = [
    ...(post.images ?? []),
    ...post.blocks.flatMap((block) =>
      block.type === "img" && block.src ? [block.src] : []
    ),
  ]
    .map((src) => src.trim())
    .filter(Boolean);

  return {
    tier: tierKey
      ? FOOD_TIERS.find((tier) => tier.key === tierKey) ?? null
      : null,
    location: post.shop.name,
    priceReference: findPrice(post),
    hoursReference: findHours(post),
    source: {
      author: post.author,
      publishedAt: formatFoodPostTime(post.time),
    },
    reasons: post.excerpt.trim() ? [post.excerpt.trim()] : [],
    dishes: findDishes(post.blocks),
    gallery: [...new Set(gallery)],
  };
}

export function formatFoodPostTime(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime()) || !value.includes("T")) return value;
  const parts = new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).formatToParts(parsed);
  const valueOf = (type: Intl.DateTimeFormatPartTypes) =>
    parts.find((part) => part.type === type)?.value ?? "";
  return `${valueOf("year")}-${valueOf("month")}-${valueOf("day")} ${valueOf("hour")}:${valueOf("minute")}`;
}
