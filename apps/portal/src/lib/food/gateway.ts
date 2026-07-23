/**
 * Food gateway adapter.
 * Fetches from API when available, falls back to mock.
 */

import { hasGateway, fetchFoodPosts } from "@/lib/api/client";
import type { FoodPost } from "@/lib/api/types";

let gatewayPosts: FoodPost[] | null = null;

export async function initFoodGateway(): Promise<void> {
  if (!hasGateway) return;
  try {
    const resp = await fetchFoodPosts();
    if (resp) {
      gatewayPosts = resp.posts;
    }
  } catch {
    // silent
  }
}

export function getGatewayPosts(): FoodPost[] | null {
  return gatewayPosts;
}
