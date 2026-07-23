/**
 * Food gateway adapter.
 *
 * Gateway 可用时用 API 数据补充 mock；不可用时完全回退到 mock。
 */

import { hasGateway, fetchFoodVenues } from "@/lib/api/client";
import type { FoodPost } from "@/lib/api/types";

let gatewayPosts = new Map<string, FoodPost[]>();

export async function initFoodGateway(): Promise<void> {
  if (!hasGateway) return;
  for (const campus of ["minglun", "jinming", "longzihu"]) {
    try {
      const resp = await fetchFoodVenues(campus);
      if (resp) {
        gatewayPosts.set(campus, resp.posts);
      }
    } catch {
      // 静默失败
    }
  }
}

export function getGatewayPosts(campus: string): FoodPost[] | null {
  return gatewayPosts.get(campus) ?? null;
}
