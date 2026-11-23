/**
 * Campus gateway adapter.
 *
 * 生产 / require-gateway：必须走 API；失败不静默回退 mock。
 */

import { apiFetchRequired, hasGateway, mockAllowed } from "@/lib/api/client";
import type { CampusCategory, CampusItem, CampusMessage } from "@/lib/api/types";
import { campusStore } from "@/lib/campus/mock";
import type { FoodPostImageInput } from "@/lib/food/submit";

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

export function forgetCampusItem(id: string): void {
  gatewayItems = gatewayItems?.filter((item) => item.id !== id) ?? null;
}

export function resetCampusGatewayCache(): void {
  gatewayItems = null;
  gatewayCategories = null;
}

/**
 * 单条单子 fallback 决策（detail 页复用，与列表页同一 gateway-first 语义）：
 * gateway 已缓存 → mock store → 无。
 */
export function getCampusItemOrFallback(
  id: string
): { item: CampusItem | null; messages: CampusMessage[] } {
  const cached = gatewayItems?.find((candidate) => candidate.id === id);
  if (cached) return { item: cached.status === "hidden" ? null : cached, messages: [] };
  if (mockAllowed && !hasGateway) {
    const data = campusStore.get();
    const item = data.items.find((candidate) => candidate.id === id && candidate.status !== "hidden");
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

export const CONTACT_CHANNELS = { qq: "QQ", wechat: "微信", phone: "手机号" } as const;
export type ContactChannel = keyof typeof CONTACT_CHANNELS;
export type ContactBook = Record<ContactChannel | "note", string> & { updated_at?: string };
export type Exchange = {
  id: string;
  requester_user_id: string;
  requester_name: string;
  owner_user_id: string;
  status: "requested" | "accepted" | "declined" | "cancelled";
  requester_channels: ContactChannel[];
  owner_channels: ContactChannel[];
  requester_snapshot?: Partial<ContactBook>;
  owner_snapshot?: Partial<ContactBook>;
};
export type CampusPublication = {
  type: "help" | "sell";
  category: string;
  title: string;
  desc: string;
  price: number;
  place: string;
  deadline?: string;
  images?: FoodPostImageInput[];
};

const ROOT = "/api/v1/campus";
const itemPath = (id: string) => `${ROOT}/items/${encodeURIComponent(id)}`;
function campusWrite<T>(path: string, body?: unknown, method = "POST") {
  return apiFetchRequired<T>(path, { method, headers: { "Content-Type": "application/json" }, body: body === undefined ? undefined : JSON.stringify(body), cache: "no-store" });
}

export const readCampusContactBook = (signal?: AbortSignal) => apiFetchRequired<{ contact: ContactBook }>(`${ROOT}/contact-book`, { cache: "no-store", signal });
export const saveCampusContactBook = ({ qq, wechat, phone, note }: ContactBook) => campusWrite<{ contact: ContactBook }>(`${ROOT}/contact-book`, { qq, wechat, phone, note }, "PUT");
export const publishCampusItem = (body: CampusPublication) => campusWrite<{ item: CampusItem }>(`${ROOT}/items`, body);
export const postCampusMessage = (id: string, text: string) => campusWrite<{ message: CampusMessage }>(`${itemPath(id)}/messages`, { text });
export const readCampusExchanges = (id: string, signal?: AbortSignal) => apiFetchRequired<{ viewer_user_id: string; is_owner: boolean; exchanges: Exchange[] }>(`${itemPath(id)}/exchanges`, { cache: "no-store", signal });
export const requestCampusExchange = (id: string, channels: ContactChannel[]) => campusWrite<{ exchange: Exchange }>(`${itemPath(id)}/exchanges`, { channels });
export const respondCampusExchange = (id: string, exchangeID: string, decision: "accept" | "decline", channels?: ContactChannel[]) => campusWrite<{ exchange: Exchange; item_status: CampusItem["status"] }>(`${itemPath(id)}/exchanges/${encodeURIComponent(exchangeID)}/respond`, { decision, channels });
export const completeCampusItem = (id: string) => campusWrite<{ item_id: string; status: "done" }>(`${itemPath(id)}/complete`);
