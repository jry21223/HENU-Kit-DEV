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
  PortalConfigError,
} from "@/lib/api/client";
import type { FoodPost, VenueSummary } from "@/lib/api/types";
import { foodStore } from "@/lib/food/mock";

const gatewayVenues = new Map<string, VenueSummary[]>();
const pendingCreatedPosts = new Map<string, FoodPost>();
let gatewayPosts: FoodPost[] | null = null;
let lastError: unknown = null;
let loaded = false;

export async function initFoodGateway(): Promise<void> {
  if (loaded) return;

  if (!hasGateway) {
    if (mockAllowed) {
      loaded = true;
      lastError = null;
      return;
    }
    lastError = new PortalConfigError("服务未就绪，请联系维护者。");
    return;
  }

  try {
    const postsResp = await fetchFoodPosts();
    gatewayPosts = [
      ...pendingCreatedPosts.values(),
      ...postsResp.posts.filter((post) => !pendingCreatedPosts.has(post.id)),
    ];

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
    lastError = e;
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

/** Keep the shared board cache coherent after a successful public create. */
export function rememberCreatedFoodPost(post: FoodPost): void {
  pendingCreatedPosts.set(post.id, post);
  if (gatewayPosts === null) {
    loaded = false;
    lastError = null;
    return;
  }
  gatewayPosts = [post, ...gatewayPosts.filter((item) => item.id !== post.id)];
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

  // 走到这里时 initFoodGateway 必然已记录错误（无 gateway 或拉取失败），
  // ?? 仅为类型兜底；错误表示统一为 error object。
  return {
    posts: [],
    error: formatPortalError(lastError ?? new Error("加载美食数据失败")),
  };
}
