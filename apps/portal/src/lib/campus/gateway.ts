/**
 * Campus gateway adapter.
 *
 * 生产 / require-gateway：必须走 API；失败不静默回退 mock。
 */

import { mockAllowed } from "@/lib/api/client";
import type { CampusCategory, CampusItem, CampusMessage } from "@/lib/api/types";
import { campusStore } from "@/lib/campus/mock";

let gatewayItems: CampusItem[] | null = null;
let gatewayCategories: CampusCategory[] | null = null;

/**
 * /campus 列表实时读到的单子写入共享缓存：从列表点进详情时，详情接口失败仍可回退到
 * 这条单子（#546 后不再由根布局预取）。
 */
export function rememberCampusItems(
  items: CampusItem[],
  categories: CampusCategory[] | null
): void {
  gatewayItems = items;
  gatewayCategories = categories;
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
