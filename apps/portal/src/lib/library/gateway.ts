/**
 * Library gateway adapter.
 *
 * Gateway/API 可用时加载真实 materials；
 * mock 仅在 NEXT_PUBLIC_PORTAL_ALLOW_MOCK=1 且非 require-gateway 时可用。
 * 生产环境禁止静默回退 STATIC_MATERIALS。
 */

import {
  fetchLibraryCourses,
  fetchLibraryMaterials,
  hasGateway,
  mockAllowed,
  PortalApiError,
} from "@/lib/api/client";
import type { Material as ApiMaterial } from "@/lib/api/types";
import {
  STATIC_MATERIALS,
  getMaterial,
  type Material,
} from "./mock";

let gatewayLoaded = false;
let availableIds = new Set<string>();
let cachedMaterials: Material[] | null = null;
let lastError: string | null = null;

function toMaterial(m: ApiMaterial): Material {
  return {
    id: m.id,
    type: m.type,
    subject: m.subject,
    title: m.title,
    author: m.author,
    intro: m.intro,
    toc: m.toc ?? [],
    pages: m.pages ?? [],
    pageCount: m.pages?.length ?? 0,
    price: m.price,
    previewPages: m.previewPages,
    rating: m.rating,
    downloads: m.downloads,
    favs: m.favs,
  };
}

export function getLibraryGatewayError(): string | null {
  return lastError;
}

export async function initGateway(): Promise<void> {
  if (gatewayLoaded) return;

  if (!hasGateway) {
    if (mockAllowed) {
      cachedMaterials = STATIC_MATERIALS;
      gatewayLoaded = true;
      lastError = null;
      return;
    }
    lastError =
      "Gateway 未配置。生产环境禁止 mock；请设置 NEXT_PUBLIC_PORTAL_GATEWAY_URL。";
    return;
  }

  try {
    // Prefer full materials list (portal-api); courses used as availability hint.
    const [materialsResp, coursesResp] = await Promise.all([
      fetchLibraryMaterials().catch((e) => {
        throw e;
      }),
      fetchLibraryCourses().catch(() => null),
    ]);

    cachedMaterials = materialsResp.materials.map(toMaterial);
    if (coursesResp) {
      availableIds = new Set(coursesResp.courses.map((c) => c.id));
    } else {
      availableIds = new Set(cachedMaterials.map((m) => m.id));
    }
    gatewayLoaded = true;
    lastError = null;
  } catch (e) {
    lastError =
      e instanceof PortalApiError
        ? e.message
        : e instanceof Error
          ? e.message
          : "加载资料库失败";
    // Never silent-fallback to STATIC_MATERIALS in production
    if (mockAllowed) {
      cachedMaterials = STATIC_MATERIALS;
      gatewayLoaded = true;
    } else {
      cachedMaterials = null;
      gatewayLoaded = false;
    }
  }
}

/**
 * 获取资料列表。
 * - gateway 已加载：API 数据
 * - mock 允许：STATIC_MATERIALS
 * - 否则空数组（页面应展示 error banner）
 */
export function getMaterials(): Material[] {
  if (cachedMaterials) return cachedMaterials;
  if (mockAllowed) return STATIC_MATERIALS;
  return [];
}

export function getMaterialOrFallback(id: string): Material | undefined {
  if (cachedMaterials) {
    return cachedMaterials.find((m) => m.id === id);
  }
  if (mockAllowed) return getMaterial(id);
  return undefined;
}

export function isLibraryReady(): boolean {
  return gatewayLoaded || mockAllowed;
}

export function toggleFavViaGateway(
  id: string,
  currentFavs: string[]
): string[] {
  return currentFavs.includes(id)
    ? currentFavs.filter((f) => f !== id)
    : [...currentFavs, id];
}

export function getAvailableCourseIds(): Set<string> {
  return availableIds;
}
