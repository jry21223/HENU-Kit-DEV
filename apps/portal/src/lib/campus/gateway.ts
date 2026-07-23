/**
 * Campus gateway adapter.
 * Fetches from API when available, falls back to mock.
 */

import {
  hasGateway,
  fetchCampusItems,
  fetchCampusItemDetail,
  fetchCategories,
} from "@/lib/api/client";
import type { CampusItem, CampusMessage, Category } from "@/lib/api/types";

export async function initCampusGateway(): Promise<void> {
  // Pre-warm: no-op for now
}

export async function getItems(
  typeFilter?: string,
  categoryFilter?: string
): Promise<CampusItem[] | null> {
  if (!hasGateway) return null;
  const resp = await fetchCampusItems(typeFilter, categoryFilter);
  return resp?.items ?? null;
}

export async function getItemDetail(
  id: string
): Promise<{ item: CampusItem; messages: CampusMessage[] } | null> {
  if (!hasGateway) return null;
  return await fetchCampusItemDetail(id);
}

export async function getCategories(): Promise<Category[] | null> {
  if (!hasGateway) return null;
  const resp = await fetchCategories();
  return resp?.categories ?? null;
}
