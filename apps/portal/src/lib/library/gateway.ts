/**
 * Library gateway adapter.
 *
 * 当 Portal Gateway 可用时，用 API 数据补充 mock 数据；
 * 不可用时完全回退到 mock。
 *
 * 页面组件通过 libraryStore 交互，不需要知道数据来源。
 */

import { hasGateway, fetchLibraryCourses } from "@/lib/api/client";
import {
  STATIC_MATERIALS,
  getMaterial,
  type Material,
} from "./mock";

/** 是否已从 Gateway 拉取过数据 */
let gatewayLoaded = false;
/** Gateway 返回的课程 ID 集合（用于标记可用性） */
let availableIds = new Set<string>();

/**
 * 初始化：如果 Gateway 可用，拉取课程列表并记录可用 ID。
 * 在客户端首次订阅时调用。
 */
export async function initGateway(): Promise<void> {
  if (!hasGateway || gatewayLoaded) return;
  try {
    const resp = await fetchLibraryCourses();
    if (resp) {
      availableIds = new Set(resp.courses.map((c) => c.id));
      gatewayLoaded = true;
    }
  } catch {
    // 静默失败，继续用 mock
  }
}

/**
 * 获取资料列表（Gateway 模式下只返回 Gateway 确认可用的资料）。
 */
export function getMaterials(): Material[] {
  if (hasGateway && gatewayLoaded && availableIds.size > 0) {
    return STATIC_MATERIALS.filter((m) => availableIds.has(m.id));
  }
  return STATIC_MATERIALS;
}

/**
 * 获取单个资料（始终从 mock 获取，因为 API 不返回完整内容）。
 */
export function getMaterialOrFallback(id: string): Material | undefined {
  return getMaterial(id);
}

/**
 * Gateway 模式下的收藏操作。
 * 当前回退到本地 store；未来对接 Gateway 写接口。
 */
export function toggleFavViaGateway(
  id: string,
  currentFavs: string[]
): string[] {
  // TODO: 当 Gateway 收藏接口就绪时，改为 API 调用
  return currentFavs.includes(id)
    ? currentFavs.filter((f) => f !== id)
    : [...currentFavs, id];
}
