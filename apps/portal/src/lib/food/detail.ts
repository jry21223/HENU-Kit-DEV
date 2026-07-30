import type { FoodPost, PostBlock } from "@/lib/api/types";
import {
  FOOD_TIERS,
  resolveFoodTier,
  type FoodTier,
} from "./ranking";

const PRICE_PATTERN =
  /(?:人均\s*)?(?:¥|￥)\s*\d+(?:\s*[–—~-]\s*\d+)?(?:\s*元)?|学生餐价/;
const DISH_HEADING_PATTERN = /^(?:点什么|推荐菜品|吃什么|必点|怎么点)[？?：:]?$/;

export interface FoodDishView {
  name: string;
  note?: string;
}

export interface FoodVenueDetail {
  tier: FoodTier | null;
  location: string;
  coordinates: string;
  mapUrl: string;
  priceReference: string | null;
  hoursReference: null;
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
    const match = text.match(PRICE_PATTERN);
    if (match) return match[0].replace(/\s+/g, " ").trim();
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
    coordinates: `${post.shop.lat.toFixed(4)}, ${post.shop.lng.toFixed(4)}`,
    mapUrl: `https://uri.amap.com/search?keyword=${encodeURIComponent(
      post.shop.name
    )}&src=henu-kit`,
    priceReference: findPrice(post),
    hoursReference: null,
    source: {
      author: post.author,
      publishedAt: post.time,
    },
    reasons: post.excerpt.trim() ? [post.excerpt.trim()] : [],
    dishes: findDishes(post.blocks),
    gallery: [...new Set(gallery)],
  };
}
