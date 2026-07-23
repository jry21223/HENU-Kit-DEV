/**
 * Campus gateway adapter.
 *
 * Gateway 可用时用 API 数据替换 mock；不可用时完全回退到 mock。
 */

import { hasGateway } from "@/lib/api/client";
import type { Item, Category, DealMessage } from "./mock";

const GATEWAY_URL =
  typeof window !== "undefined"
    ? process.env.NEXT_PUBLIC_PORTAL_GATEWAY_URL || ""
    : "";

async function apiFetch<T>(path: string): Promise<T | null> {
  if (!hasGateway) return null;
  try {
    const res = await fetch(`${GATEWAY_URL}${path}`, {
      credentials: "same-origin",
    });
    if (!res.ok) return null;
    return (await res.json()) as T;
  } catch {
    return null;
  }
}

export async function initCampusGateway(): Promise<void> {
  // 预热：目前 campus 数据来自 portal DB seed，无需额外初始化
}

export async function fetchCampusItems(
  typeFilter?: string,
  categoryFilter?: string
): Promise<Item[] | null> {
  const params = new URLSearchParams();
  if (typeFilter) params.set("type", typeFilter);
  if (categoryFilter) params.set("category", categoryFilter);
  const qs = params.toString();
  const resp = await apiFetch<{ items: Item[] }>(
    `/api/v1/campus/items${qs ? "?" + qs : ""}`
  );
  return resp?.items ?? null;
}

export async function fetchCampusItem(
  id: string
): Promise<{ item: Item; messages: DealMessage[] } | null> {
  return apiFetch<{ item: Item; messages: DealMessage[] }>(
    `/api/v1/campus/items/${id}`
  );
}

export async function fetchCategories(): Promise<Category[] | null> {
  const resp = await apiFetch<{ categories: Category[] }>(
    `/api/v1/campus/categories`
  );
  return resp?.categories ?? null;
}
