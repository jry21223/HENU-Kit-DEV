/**
 * Library gateway adapter.
 *
 * Gateway/API 可用时加载真实 materials；
 * mock 仅在 NEXT_PUBLIC_PORTAL_ALLOW_MOCK=1 且非 require-gateway 时可用。
 * 生产环境禁止静默回退 STATIC_MATERIALS。
 */

import { fetchLibraryMaterials, mockAllowed } from "@/lib/api/client";
import type { Material as ApiMaterial } from "@/lib/api/types";
import {
  STATIC_MATERIALS,
  getMaterial,
  type Material,
} from "./mock";

let cachedMaterials: Material[] | null = null;
let inflight: Promise<Material[]> | null = null;

function toMaterial(m: ApiMaterial): Material {
  return {
    id: m.id,
    type: m.type,
    subject: m.subject,
    title: m.title,
    author: m.author,
    intro: m.intro,
    toc: m.toc ?? [],
    pages: [],
    pageCount: 0,
    price: m.price,
    previewPages: 0,
    rating: m.rating,
    downloads: m.downloads,
    favs: m.favs,
    downloadAvailable: m.downloadAvailable,
    fileSize: m.fileSize,
    slides: [],
  };
}

/**
 * 全量资料目录的共享读取（首页 01 区块、资料详情“相关资料”共用）。
 *
 * 进入对应页面时才请求，不在根布局预取（#546）；并发调用共用同一次请求，成功后在本次
 * 页面会话内缓存。失败不缓存、原样抛出，由调用方经 formatPortalError 映射，并自行决定
 * 能否回退 mock。
 */
export function loadLibraryMaterials(): Promise<Material[]> {
  if (cachedMaterials) return Promise.resolve(cachedMaterials);
  inflight ??= fetchLibraryMaterials()
    .then((response) => rememberLibraryMaterials(response.materials))
    .finally(() => {
      inflight = null;
    });
  return inflight;
}

/**
 * /library 列表每次进入都实时读取全量目录（统计必须是最新的）；读到后写入共享缓存，
 * 从列表点进详情时“相关资料”不必再下载一遍。
 */
export function rememberLibraryMaterials(materials: ApiMaterial[]): Material[] {
  cachedMaterials = materials.map(toMaterial);
  return cachedMaterials;
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

export function toggleFavViaGateway(
  id: string,
  currentFavs: string[]
): string[] {
  return currentFavs.includes(id)
    ? currentFavs.filter((f) => f !== id)
    : [...currentFavs, id];
}
