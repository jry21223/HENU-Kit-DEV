import { consoleRequest, readConsoleJSON } from "./console-session";
import type { ConsolePointLedgerEntry, PlatformOperationsInboxItem } from "./console-gateway";

export interface CatalogCourse {
  id: string;
  code: string;
  name: string;
  department: string;
  version: number;
  status: "active" | "archived";
}
export interface CatalogMaterial {
  id: string;
  course_id: string;
  course_name?: string;
  title: string;
  type: string;
  description?: string;
  access_level: string;
  source_note?: string;
  version: number;
  status: "draft" | "pending_review" | "published" | "archived";
}
export interface CatalogCorrection {
  id: string;
  material_title?: string;
  title?: string;
  reason: string;
  description: string;
  status: "open" | "in_progress" | "resolved" | "dismissed";
  resolution_note?: string;
  version: number;
}
export interface CatalogWorkspace {
  degraded: boolean;
  courses: CatalogCourse[];
  materials: CatalogMaterial[];
  submissions: CatalogMaterial[];
  corrections: CatalogCorrection[];
  releases?: { release_id: string; state: string; activated_at: string; snapshot_count: number }[];
  download_stats?: { total_starts: number; last_24h_starts: number };
}
export interface CatalogReadiness {
  management: "ready" | "not_ready";
  reason: string;
  checked_at: string;
  capabilities: {
    workspace: boolean;
    commands: boolean;
    operation_results: boolean;
    courses_write?: boolean;
    materials_write?: boolean;
    review?: boolean;
    releases_read?: boolean;
  };
}
export type CatalogCommand = {
  kind: "course_create" | "course_update" | "material_create" | "material_update" | "submission_approve" | "submission_reject" | "correction_resolve";
  resource_id?: string;
  expected_version?: string;
  payload: Record<string, string>;
};
export interface CareerWorkspace {
  job_source_enabled: boolean;
  ai_mode: string;
  generated_at: string;
  counts: Record<"profiles" | "searches_total" | "searches_queued" | "searches_running" | "searches_completed" | "searches_failed" | "extractions_total" | "digests_pending" | "digests_sent", number>;
  searches: { id: string; status: string; stage: string; source_count: number; job_count: number; matched_count: number; created_at: string }[];
  extractions: { id: string; status: string; created_at: string; completed_at: string | null }[];
  digest_deliveries: { search_id: string; status: string; attempted_at: string | null; updated_at: string }[];
}
export interface CampusModerationItem {
  id: string;
  type: "help" | "sell";
  category: string;
  title: string;
  desc: string;
  price: number;
  status: "open" | "ongoing" | "done" | "hidden";
  moderation_note: string;
  version: number;
  updated_at: string;
}
export interface CampusModerationMessage {
  id: string;
  item_id: string;
  item_title?: string;
  text: string;
  hidden: boolean;
  created_at: string;
}
export interface CampusWorkspace {
  counts: Record<"open" | "ongoing" | "done" | "hidden" | "messages_total" | "messages_hidden" | "exchanges_total" | "exchanges_accepted", number>;
  items: CampusModerationItem[];
  messages: CampusModerationMessage[];
}

async function readWorkspace<T>(path: string, signal?: AbortSignal): Promise<ExtensionReadResult<T>> {
  try {
    const response = await consoleRequest(path, { signal, credentials: "same-origin", cache: "no-store" });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (!response.ok) return { state: "unavailable" };
    const data = unwrapData<T>(await safeJson(response));
    return data ? { state: "authenticated", data } : { state: "unavailable" };
  } catch {
    return { state: "unavailable" };
  }
}

async function submitWorkspaceCommand<T>(path: string, body?: object, key?: string): Promise<ExtensionWriteResult<T>> {
  try {
    const headers = new Headers();
    if (body) headers.set("Content-Type", "application/json");
    if (key) headers.set("Idempotency-Key", key);
    const response = await consoleRequest(path, { method: "POST", credentials: "same-origin", headers, body: body && JSON.stringify(body) });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 409) return { state: "conflict" };
    if (response.status === 400) return { state: "invalid" };
    if (response.status === 404) return { state: "not_found" };
    if (!response.ok) return { state: "unavailable" };
    const data = unwrapData<T>(await safeJson(response));
    return data ? { state: "succeeded", data } : { state: "unavailable" };
  } catch {
    return { state: "unavailable" };
  }
}

export const readCatalogWorkspace = (signal?: AbortSignal) => readWorkspace<CatalogWorkspace>("/api/v1/library", signal);
export async function readCatalogReadiness(signal?: AbortSignal): Promise<ExtensionReadResult<CatalogReadiness>> {
  const result = await readWorkspace<CatalogReadiness>("/api/v1/library/readiness", signal);
  if (result.state === "authenticated" && ((result.data.management !== "ready" && result.data.management !== "not_ready") || !result.data.capabilities)) {
    return { state: "unavailable" };
  }
  return result;
}
export const writeCatalog = (command: CatalogCommand, key: string) => submitWorkspaceCommand<{ state: "succeeded"; version: number }>("/api/v1/library/commands", command, key);
export const readCareerWorkspace = (signal?: AbortSignal) => readWorkspace<CareerWorkspace>("/api/v1/career/workspace", signal);
export const retryCareerDigest = (id: string, key: string) => submitWorkspaceCommand<{ status: "retry" }>(`/api/v1/career/searches/${encodeURIComponent(id)}/retry-digest`, undefined, key);
export const readCampusWorkspace = (signal?: AbortSignal) => readWorkspace<CampusWorkspace>("/api/v1/campus/workspace", signal);
export const moderateCampusItem = (id: string, input: { expected_version: number; action: "hide" | "restore" | "close"; note?: string }) => submitWorkspaceCommand<{ id: string; status: string; version: number }>(`/api/v1/campus/items/${encodeURIComponent(id)}/moderate`, input);
export const moderateCampusMessage = (id: string, input: { action: "hide" | "restore"; note?: string }) => submitWorkspaceCommand<{ id: string; hidden: boolean }>(`/api/v1/campus/messages/${encodeURIComponent(id)}/moderate`, input);

export const workspaceWriteFeedback = {
  signed_out: "登录已过期，请重新登录。",
  denied: "你的账号没有此操作权限。",
  conflict: "记录已变更，请刷新后再试。",
  invalid: "提交内容有误，请检查后重试。",
  not_found: "未找到该记录，请刷新列表。",
  unavailable: "暂时无法确认操作结果，请稍后重试。",
} as const;

/**
 * Phase 2 扩展管理客户端：
 * 对接 Notice 来源列表、Platform 角色目录与待办流转、Account 积分只读余额/流水、
 * 以及会员订单查询/关单/退款接口（保持自动生成的 console-gateway.ts 只读不被手改）。
 */

export interface ConsoleNoticeSourceDetail {
  id: string;
  code: string;
  name: string;
  canonical_url?: string;
  created_at?: string;
}

export interface ConsoleRoleCatalogEntry {
  code: string;
  display_name: string;
  permissions: string[];
}

export interface ConsolePointsAccountDetail {
  balance: number;
  entries: ConsolePointLedgerEntry[];
}

export interface ConsoleMembershipRefundSummary {
  id: string;
  status: string;
  amount_cents: number;
  created_at?: string;
}

export interface ConsoleMembershipOrderItem {
  id: string;
  user_id?: string;
  display_name?: string;
  email?: string;
  plan: string;
  amount_cents: number;
  status: "created" | "pending_payment" | "paid" | "closed" | "refunded" | string;
  version: number;
  created_at: string;
  updated_at: string;
  paid_at?: string | null;
  closed_at?: string | null;
  refunded_at?: string | null;
  latest_refund?: ConsoleMembershipRefundSummary | null;
}

export type ExtensionReadResult<T> =
  | { state: "authenticated"; data: T }
  | { state: "signed_out" | "denied" | "not_found" | "unavailable" };

export type ExtensionWriteResult<T = Record<string, unknown>> =
  | { state: "succeeded"; data: T }
  | { state: "signed_out" | "denied" | "conflict" | "invalid" | "not_found" | "unavailable"; message?: string };

async function safeJson(response: Response): Promise<unknown> {
  try {
    return await readConsoleJSON(response);
  } catch {
    return undefined;
  }
}

function unwrapData<T>(payload: unknown): T | undefined {
  if (!payload || typeof payload !== "object") return undefined;
  if ("data" in payload) {
    return (payload as { data: T }).data;
  }
  return payload as T;
}

function extractMessage(payload: unknown): string | undefined {
  if (!payload || typeof payload !== "object") return undefined;
  const obj = payload as { error?: { message?: unknown }; message?: unknown };
  if (typeof obj.error?.message === "string" && obj.error.message.trim()) return obj.error.message.trim();
  if (typeof obj.message === "string" && obj.message.trim()) return obj.message.trim();
  return undefined;
}

export async function fetchNoticeSources(
  options: { signal?: AbortSignal } = {},
): Promise<ExtensionReadResult<ConsoleNoticeSourceDetail[]>> {
  try {
    const response = await consoleRequest("/api/v1/notices/sources", {
      credentials: "same-origin",
      headers: { Accept: "application/json" },
      signal: options.signal,
    });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 404) return { state: "not_found" };
    if (!response.ok) return { state: "unavailable" };
    const payload = await safeJson(response);
    const data = unwrapData<{ sources?: ConsoleNoticeSourceDetail[] }>(payload);
    if (!data || !Array.isArray(data.sources)) return { state: "unavailable" };
    return { state: "authenticated", data: data.sources };
  } catch {
    return { state: "unavailable" };
  }
}

export async function fetchPlatformRoles(
  options: { signal?: AbortSignal } = {},
): Promise<ExtensionReadResult<ConsoleRoleCatalogEntry[]>> {
  try {
    const response = await consoleRequest("/api/v1/operations/roles", {
      credentials: "same-origin",
      headers: { Accept: "application/json" },
      signal: options.signal,
    });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 404) return { state: "not_found" };
    if (!response.ok) return { state: "unavailable" };
    const payload = await safeJson(response);
    const data = unwrapData<{ roles?: ConsoleRoleCatalogEntry[] }>(payload);
    if (!data || !Array.isArray(data.roles)) return { state: "unavailable" };
    return { state: "authenticated", data: data.roles };
  } catch {
    return { state: "unavailable" };
  }
}

export async function updatePlatformInboxItem(
  itemId: string,
  input: {
    expected_version: number;
    status?: "open" | "in_progress" | "blocked" | "resolved" | "cancelled" | "archived";
    priority?: "low" | "normal" | "high" | "urgent";
    source_product_code?: string;
    source_resource_type?: string;
    source_resource_id?: string;
  },
  idempotencyKey: string,
): Promise<ExtensionWriteResult<PlatformOperationsInboxItem>> {
  try {
    const response = await consoleRequest(
      `/api/v1/operations/inbox/${encodeURIComponent(itemId)}/updates`,
      {
        method: "POST",
        credentials: "same-origin",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          "Idempotency-Key": idempotencyKey,
        },
        body: JSON.stringify(input),
      },
    );
    const payload = await safeJson(response);
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 404) return { state: "not_found" };
    if (response.status === 409) return { state: "conflict", message: extractMessage(payload) };
    if (response.status === 400) return { state: "invalid", message: extractMessage(payload) };
    if (!response.ok) return { state: "unavailable", message: extractMessage(payload) };
    const data = unwrapData<{ item?: PlatformOperationsInboxItem } | PlatformOperationsInboxItem>(payload);
    const item = data && typeof data === "object" && "item" in data && data.item ? data.item : (data as PlatformOperationsInboxItem);
    return item ? { state: "succeeded", data: item } : { state: "unavailable" };
  } catch {
    return { state: "unavailable" };
  }
}

export async function fetchAccountPointsDetail(
  userId: string,
  limit = 20,
): Promise<ExtensionReadResult<ConsolePointsAccountDetail>> {
  try {
    const response = await consoleRequest(
      `/api/v1/account/points/${encodeURIComponent(userId)}?limit=${encodeURIComponent(String(limit))}`,
      {
        credentials: "same-origin",
        headers: { Accept: "application/json" },
      },
    );
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 404) return { state: "not_found" };
    if (!response.ok) return { state: "unavailable" };
    const payload = await safeJson(response);
    const data = unwrapData<ConsolePointsAccountDetail>(payload);
    if (!data || typeof data.balance !== "number" || !Array.isArray(data.entries)) {
      return { state: "unavailable" };
    }
    return { state: "authenticated", data };
  } catch {
    return { state: "unavailable" };
  }
}

export async function fetchAccountMembershipOrders(
  params: { status?: string; limit?: number } = {},
): Promise<ExtensionReadResult<ConsoleMembershipOrderItem[]>> {
  try {
    const query = new URLSearchParams();
    if (params.status && params.status !== "all") query.set("status", params.status);
    if (params.limit) query.set("limit", String(params.limit));
    const qs = query.toString();
    const response = await consoleRequest(
      `/api/v1/account/membership-orders${qs ? `?${qs}` : ""}`,
      {
        credentials: "same-origin",
        headers: { Accept: "application/json" },
      },
    );
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 404) return { state: "not_found" };
    if (!response.ok) return { state: "unavailable" };
    const payload = await safeJson(response);
    const data = unwrapData<{ orders?: ConsoleMembershipOrderItem[] }>(payload);
    if (!data || !Array.isArray(data.orders)) return { state: "unavailable" };
    return { state: "authenticated", data: data.orders };
  } catch {
    return { state: "unavailable" };
  }
}

export async function closeAccountMembershipOrder(
  orderId: string,
  input: { expected_version: number; reason: string },
  idempotencyKey: string,
): Promise<ExtensionWriteResult<{ order: ConsoleMembershipOrderItem }>> {
  try {
    const response = await consoleRequest(
      `/api/v1/account/membership-orders/${encodeURIComponent(orderId)}/closures`,
      {
        method: "POST",
        credentials: "same-origin",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          "Idempotency-Key": idempotencyKey,
        },
        body: JSON.stringify(input),
      },
    );
    const payload = await safeJson(response);
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 404) return { state: "not_found" };
    if (response.status === 409) return { state: "conflict", message: extractMessage(payload) };
    if (response.status === 400) return { state: "invalid", message: extractMessage(payload) };
    if (!response.ok) return { state: "unavailable", message: extractMessage(payload) };
    const data = unwrapData<{ order: ConsoleMembershipOrderItem }>(payload);
    return data?.order ? { state: "succeeded", data } : { state: "unavailable" };
  } catch {
    return { state: "unavailable" };
  }
}

export async function refundAccountMembershipOrder(
  orderId: string,
  input: { expected_version: number; reason: string },
  idempotencyKey: string,
): Promise<
  ExtensionWriteResult<{
    order: ConsoleMembershipOrderItem;
    refund: ConsoleMembershipRefundSummary;
  }>
> {
  try {
    const response = await consoleRequest(
      `/api/v1/account/membership-orders/${encodeURIComponent(orderId)}/refunds`,
      {
        method: "POST",
        credentials: "same-origin",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          "Idempotency-Key": idempotencyKey,
        },
        body: JSON.stringify(input),
      },
    );
    const payload = await safeJson(response);
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 404) return { state: "not_found" };
    if (response.status === 409) return { state: "conflict", message: extractMessage(payload) };
    if (response.status === 400) return { state: "invalid", message: extractMessage(payload) };
    if (!response.ok) return { state: "unavailable", message: extractMessage(payload) };
    const data = unwrapData<{
      order: ConsoleMembershipOrderItem;
      refund: ConsoleMembershipRefundSummary;
    }>(payload);
    return data?.order && data?.refund ? { state: "succeeded", data } : { state: "unavailable" };
  } catch {
    return { state: "unavailable" };
  }
}

export async function fetchAccountMembershipOrderRefund(
  orderId: string,
  refundId: string,
): Promise<
  ExtensionReadResult<{
    order: ConsoleMembershipOrderItem;
    refund: ConsoleMembershipRefundSummary;
  }>
> {
  try {
    const response = await consoleRequest(
      `/api/v1/account/membership-orders/${encodeURIComponent(orderId)}/refunds/${encodeURIComponent(refundId)}`,
      {
        credentials: "same-origin",
        headers: { Accept: "application/json" },
      },
    );
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 404) return { state: "not_found" };
    if (!response.ok) return { state: "unavailable" };
    const payload = await safeJson(response);
    const data = unwrapData<{
      order: ConsoleMembershipOrderItem;
      refund: ConsoleMembershipRefundSummary;
    }>(payload);
    return data?.order && data?.refund ? { state: "authenticated", data } : { state: "unavailable" };
  } catch {
    return { state: "unavailable" };
  }
}
