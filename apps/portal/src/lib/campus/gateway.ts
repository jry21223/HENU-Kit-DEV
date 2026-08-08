/**
 * Campus gateway adapter.
 *
 * 生产 / require-gateway：必须走 API；失败不静默回退 mock。
 */

import {
  fetchCampusCategories,
  fetchCampusItems,
  hasGateway,
  mockAllowed,
  PortalApiError,
} from "@/lib/api/client";
import type { CampusCategory, CampusItem, CampusMessage } from "@/lib/api/types";
import { campusStore } from "@/lib/campus/mock";

let gatewayItems: CampusItem[] | null = null;
let gatewayCategories: CampusCategory[] | null = null;
let lastError: string | null = null;
let loaded = false;

export function getCampusGatewayError(): string | null {
  return lastError;
}

export async function initCampusGateway(): Promise<void> {
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
    const [itemsResp, catsResp] = await Promise.all([
      fetchCampusItems(),
      fetchCampusCategories().catch(() => null),
    ]);
    gatewayItems = itemsResp.items;
    gatewayCategories = catsResp?.categories ?? null;
    loaded = true;
    lastError = null;
  } catch (e) {
    lastError =
      e instanceof PortalApiError
        ? e.message
        : e instanceof Error
          ? e.message
          : "加载互助平台数据失败";
    if (!mockAllowed) {
      gatewayItems = null;
      gatewayCategories = null;
      loaded = false;
    } else {
      loaded = true;
    }
  }
}

export function getGatewayItems(): CampusItem[] | null {
  return gatewayItems;
}

/**
 * 单条单子 fallback 决策（detail 页复用，与列表页同一 gateway-first 语义）：
 * gateway 已缓存 → mock store → 无。
 */
export function getCampusItemOrFallback(
  id: string
): { item: CampusItem | null; messages: CampusMessage[] } {
  if (gatewayItems) {
    const item = gatewayItems.find((candidate) => candidate.id === id);
    if (item) return { item, messages: [] };
  }
  if (mockAllowed) {
    const data = campusStore.get();
    const item = data.items.find((candidate) => candidate.id === id);
    if (item) {
      return {
        item,
        messages: data.messages.filter((m) => m.itemId === id),
      };
    }
  }
  return { item: null, messages: [] };
}

export function getGatewayCategories(): CampusCategory[] | null {
  return gatewayCategories;
}

export function isCampusReady(): boolean {
  return loaded || mockAllowed;
}
