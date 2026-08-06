/**
 * Food gateway adapter.
 *
 * 生产 / require-gateway：必须走 API；失败不静默回退 mock。
 */

import {
  fetchFoodPosts,
  fetchFoodVenues,
  formatPortalError,
  hasGateway,
  mockAllowed,
  PortalApiError,
  PortalConfigError,
} from "@/lib/api/client";
import type { FoodPost, VenueSummary } from "@/lib/api/types";
import { foodStore } from "@/lib/food/mock";

const gatewayVenues = new Map<string, VenueSummary[]>();
let gatewayPosts: FoodPost[] | null = null;
let lastError: string | null = null;
let lastErrorObject: unknown = null;
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
      lastErrorObject = null;
      return;
    }
    lastError =
      "Gateway 未配置。生产环境禁止 mock；请设置 NEXT_PUBLIC_PORTAL_GATEWAY_URL。";
    lastErrorObject = new PortalConfigError("服务未就绪，请联系维护者。");
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
    lastErrorObject = null;
  } catch (e) {
    lastErrorObject = e;
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

export interface FoodPostsResult {
  posts: FoodPost[];
  error: string | null;
}

/**
 * 榜单统一加载入口（首页美食榜 + /food 榜单页共用）。
 *
 * 走 gateway 的 mock/live 决策：gateway 有缓存直接返回；
 * 失败时 mock 允许则回退 foodStore，否则返回 formatPortalError 文案。
 */
export async function loadFoodPosts(): Promise<FoodPostsResult> {
  await initFoodGateway();

  const cached = getGatewayPosts();
  if (cached) return { posts: cached, error: null };

  if (mockAllowed) return { posts: foodStore.get().posts, error: null };

  return {
    posts: [],
    error: lastErrorObject
      ? formatPortalError(lastErrorObject)
      : (getFoodGatewayError() ?? "榜单暂时加载不出来，请稍后刷新试试"),
  };
}
