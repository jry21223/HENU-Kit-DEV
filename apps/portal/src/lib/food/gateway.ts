/**
 * Food gateway adapter.
 *
 * 生产 / require-gateway：必须走 API；失败不静默回退 mock。
 */

import {
  fetchFoodPosts,
  fetchFoodVenues,
  hasGateway,
  mockAllowed,
  PortalApiError,
} from "@/lib/api/client";
import type { FoodPost, VenueSummary } from "@/lib/api/types";

let gatewayVenues = new Map<string, VenueSummary[]>();
let gatewayPosts: FoodPost[] | null = null;
let lastError: string | null = null;
let loaded = false;

export function getFoodGatewayError(): string | null {
  return lastError;
}

export async function initFoodGateway(): Promise<void> {
  if (loaded) return;

  if (!hasGateway) {
    if (mockAllowed) {
      loaded = true;
      lastError = null;
      return;
    }
    lastError =
      "Gateway 未配置。生产环境禁止 mock；请设置 NEXT_PUBLIC_PORTAL_GATEWAY_URL。";
    return;
  }

  try {
    const postsResp = await fetchFoodPosts();
    gatewayPosts = postsResp.posts;

    for (const campus of ["minglun", "jinming", "longzihu"]) {
      try {
        const resp = await fetchFoodVenues(campus);
        if (resp) gatewayVenues.set(campus, resp.venues);
      } catch {
        // venues optional relative to posts
      }
    }
    loaded = true;
    lastError = null;
  } catch (e) {
    lastError =
      e instanceof PortalApiError
        ? e.message
        : e instanceof Error
          ? e.message
          : "加载美食数据失败";
    if (!mockAllowed) {
      gatewayPosts = null;
      loaded = false;
    } else {
      loaded = true;
    }
  }
}

export function getVenues(campus: string): VenueSummary[] | null {
  return gatewayVenues.get(campus) ?? null;
}

export function getGatewayPosts(): FoodPost[] | null {
  return gatewayPosts;
}

export function isFoodReady(): boolean {
  return loaded || mockAllowed;
}
