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
  dishes: FoodDishView[];
  bodyBlocks: PostBlock[];
  heroImage: string | null;
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
function findDishes(blocks: PostBlock[]): { dishes: FoodDishView[]; displayedBlocks: number[] } {
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
        const dishes = candidate.items.map(toDish);
        // Only replace a whole, valid dish section. Keep ambiguous lists intact.
        if (dishes.every((dish) => dish.name)) {
          return { dishes, displayedBlocks: [headingIndex, candidateIndex] };
        }
      }
      break;
    }
  }
  return { dishes: [], displayedBlocks: [] };
}

export function buildFoodVenueDetail(post: FoodPost): FoodVenueDetail {
  const tierKey = resolveFoodTier(post);
  const priceReference = findPrice(post);
  const hoursReference = findHours(post);
  const { dishes, displayedBlocks } = findDishes(post.blocks);
  const bodyBlocks = post.blocks.filter((block, index) => {
    if (displayedBlocks.includes(index)) return false;
    if (block.type !== "p") return true;
    const text = block.text?.trim() ?? "";
    if (text && text === post.excerpt.trim()) return false;
    // Only standalone, explicit metadata is redundant; narrative price mentions stay.
    if (text.match(PRICE_REFERENCE_PATTERN)?.[1]?.trim() === priceReference) return false;
    if (text.match(HOURS_PATTERN)?.[1]?.trim() === hoursReference) return false;
    return true;
  });
  // Inline illustrations keep their editorial position, not a second gallery copy.
  const bodyImages = new Set(bodyBlocks.flatMap((block) =>
    block.type === "img" && block.src?.trim() ? [block.src.trim()] : []
  ));
  const gallery = [
    ...(post.images ?? []),
    ...post.blocks.flatMap((block) =>
      block.type === "img" && block.src ? [block.src] : []
    ),
  ]
    .map((src) => src.trim())
    .filter((src) => src && !bodyImages.has(src));
  const [heroImage, ...additionalImages] = [...new Set(gallery)];

  return {
    tier: tierKey
      ? FOOD_TIERS.find((tier) => tier.key === tierKey) ?? null
      : null,
    location: post.shop.name,
    priceReference,
    hoursReference,
    source: {
      author: post.author,
      publishedAt: formatFoodPostTime(post.time),
    },
    dishes,
    bodyBlocks,
    heroImage: heroImage ?? null,
    gallery: additionalImages,
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
